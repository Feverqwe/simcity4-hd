package texturecache

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

const TextureArchiveRoot = "SimCity4UHD-Textures-v1/"

var uiAtlasName = regexp.MustCompile(`^(?:[0-9a-f]{8}-){3}[1-9][0-9]*-[0-9a-f]{64}(?:-keyed)?-x4\.png$`)

var textureCacheFolders = archiveTextureFolders()

func archiveTextureFolders() map[string]*regexp.Regexp {
	folders := map[string]*regexp.Regexp{
		upscale.InferenceCacheFolder: regexp.MustCompile(`^[0-9a-f]{64}\.png$`),
		upscale.InterfaceAtlasFolder: uiAtlasName,
		upscale.CatalogFramesFolder:  uiAtlasName,
		upscale.PortraitsFolder:      uiAtlasName,
		upscale.CursorsFolder:        uiAtlasName,
	}
	fshName := regexp.MustCompile(`^[0-9a-f]{64}\.fsh$`)
	for _, scale := range []int{2, 4} {
		folders[filepath.ToSlash(ModelCache("", scale))] = fshName
		folders[filepath.ToSlash(NetworkCache("", scale))] = fshName
		for _, category := range WorldCategories {
			folders[filepath.ToSlash(category.CacheFolder("", scale))] = fshName
		}
	}
	return folders
}

func textureArchiveMember(name string) bool {
	relative, ok := strings.CutPrefix(name, TextureArchiveRoot)
	if !ok {
		return false
	}
	lastSlash := strings.LastIndexByte(relative, '/')
	if lastSlash < 0 {
		return false
	}
	pattern := textureCacheFolders[relative[:lastSlash]]
	return pattern != nil && pattern.MatchString(relative[lastSlash+1:])
}

type ArchiveProgress struct {
	CheckSpace func(path string, size uint64) error
	Count      func(done, total int)
	Publish    func(from, to string) error
}

func (p ArchiveProgress) report(done, total int) {
	if p.Count != nil {
		p.Count(done, total)
	}
}

func ExportTextureArchive(ctx context.Context, cache, archive string, progress ArchiveProgress) error {
	root, err := os.OpenRoot(cache)
	if err != nil {
		return err
	}
	defer root.Close()

	var files []string
	var bytesNeeded uint64
	for folder, pattern := range textureCacheFolders {
		entries, err := os.ReadDir(filepath.Join(cache, folder))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !entry.Type().IsRegular() || !pattern.MatchString(entry.Name()) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			name := folder + "/" + entry.Name()
			files = append(files, name)
			bytesNeeded += uint64(info.Size()) + uint64(2*len(TextureArchiveRoot+name)) + 256
		}
	}
	if len(files) == 0 {
		return errors.New("This texture cache has no files to export. Build textures first or choose another cache folder.")
	}
	if progress.CheckSpace != nil {
		if err := progress.CheckSpace(filepath.Dir(archive), bytesNeeded+256); err != nil {
			return err
		}
	}
	publish := progress.Publish
	if publish == nil {
		publish = os.Link
	}
	return gamefiles.PublishStagedFile(archive, func(w io.Writer) error {
		writer := zip.NewWriter(w)
		progress.report(0, len(files))
		for i, name := range files {
			file, err := root.Open(filepath.FromSlash(name))
			if err != nil {
				return err
			}
			entry, err := writer.CreateHeader(&zip.FileHeader{Name: TextureArchiveRoot + name, Method: zip.Store})
			if err == nil {
				_, err = io.Copy(entry, gamefiles.WithContext(ctx, file))
			}
			if err := errors.Join(err, file.Close()); err != nil {
				return err
			}
			progress.report(i+1, len(files))
		}
		return errors.Join(ctx.Err(), writer.Close())
	}, publish)
}

func ImportTextureArchive(ctx context.Context, cache, archive string, progress ArchiveProgress) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	if len(reader.File) == 0 {
		return errors.New("The texture archive contains no files.")
	}
	root, err := os.OpenRoot(cache)
	if err != nil {
		return err
	}
	defer root.Close()

	seen := make(map[string]bool)
	var pending []*zip.File
	var bytesNeeded uint64
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !textureArchiveMember(entry.Name) || !entry.Mode().IsRegular() || seen[entry.Name] ||
			entry.UncompressedSize64 == 0 || entry.UncompressedSize64 > upscale.MaxInferenceImage {
			return fmt.Errorf("invalid texture archive entry %q", entry.Name)
		}
		seen[entry.Name] = true
		name := filepath.FromSlash(strings.TrimPrefix(entry.Name, TextureArchiveRoot))
		info, err := root.Lstat(name)
		switch {
		case errors.Is(err, os.ErrNotExist):
			pending = append(pending, entry)
			bytesNeeded += entry.UncompressedSize64
		case err != nil:
			return err
		case !info.Mode().IsRegular():
			return fmt.Errorf("cache entry %q is not a regular file", name)
		}
	}
	if progress.CheckSpace != nil {
		if err := progress.CheckSpace(cache, bytesNeeded); err != nil {
			return err
		}
	}
	done := len(reader.File) - len(pending)
	progress.report(done, len(reader.File))
	for _, entry := range pending {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := filepath.FromSlash(strings.TrimPrefix(entry.Name, TextureArchiveRoot))
		if err := importTextureFile(ctx, root, name, entry); err != nil {
			return fmt.Errorf("import %s: %w", name, err)
		}
		done++
		progress.report(done, len(reader.File))
	}
	return ctx.Err()
}

func importTextureFile(ctx context.Context, root *os.Root, name string, entry *zip.File) error {
	input, err := entry.Open()
	if err != nil {
		return err
	}
	defer input.Close()
	data, err := io.ReadAll(io.LimitReader(gamefiles.WithContext(ctx, input), int64(entry.UncompressedSize64)+1))
	if err != nil {
		return err
	}
	if uint64(len(data)) != entry.UncompressedSize64 {
		return errors.New("incorrect texture size")
	}
	if err := validateArchivedTexture(name, data); err != nil {
		return err
	}

	// Publish only verified entries. The root prevents archive writes through
	// links that lead outside the selected cache.
	folder := filepath.Dir(name)
	if err := root.MkdirAll(folder, 0755); err != nil {
		return err
	}
	file, err := os.CreateTemp(root.Name(), ".texture-import-*.tmp")
	if err != nil {
		return err
	}
	temp := filepath.Base(file.Name())
	defer root.Remove(temp)
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Close(), ctx.Err()); err != nil {
		return err
	}
	return root.Link(temp, name)
}

func validateArchivedTexture(name string, data []byte) error {
	if filepath.Ext(name) == ".fsh" {
		elements, err := textures.FSHLayout(data)
		if err != nil {
			return err
		}
		if len(data) != int(binary.LittleEndian.Uint32(data[4:8])) {
			return errors.New("incorrect SHPI size")
		}
		// The builder validates the stock mip levels and metadata when it
		// uses the cache, including entries from earlier packers.
		for _, element := range elements {
			if element.Code != textures.BC1 && element.Code != textures.BC2 {
				return errors.New("unsupported SHPI encoding")
			}
			payload := element.End - element.Start - 16
			if element.Stored != 0 {
				payload = element.Stored - 16
			}
			if textures.BCLevelBytes(element.Width, element.Height, textures.BCBlockSize(element.Code)) > payload {
				return errors.New("truncated SHPI pixel data")
			}
		}
		return nil
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if uint64(config.Width)*uint64(config.Height) > 64<<20 {
		return errors.New("texture image is too large")
	}
	_, err = png.Decode(bytes.NewReader(data))
	return err
}
