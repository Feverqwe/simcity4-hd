package sc4

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const textureArchiveRoot = "SimCity4UHD-Textures-v1/"

var textureCacheFolders = map[string]*regexp.Regexp{
	inferenceCacheFolder: regexp.MustCompile(`^[0-9a-f]{64}\.png$`),
	interfaceAtlasFolder: uiAtlasName,
	catalogFramesFolder:  uiAtlasName,
	portraitsFolder:      uiAtlasName,
	cursorsFolder:        uiAtlasName,
}

var uiAtlasName = regexp.MustCompile(`^(?:[0-9a-f]{8}-){3}[1-9][0-9]*-[0-9a-f]{64}(?:-keyed)?-x4\.png$`)

func textureArchiveMember(name string) bool {
	relative, ok := strings.CutPrefix(name, textureArchiveRoot)
	if !ok {
		return false
	}
	folder, file, ok := strings.Cut(relative, "/")
	pattern := textureCacheFolders[folder]
	return ok && pattern != nil && pattern.MatchString(file)
}

type textureArchiveProgress struct {
	checkSpace func(path string, size uint64) error
	count      func(done, total int)
	publish    func(from, to string) error
}

func (p textureArchiveProgress) report(done, total int) {
	if p.count != nil {
		p.count(done, total)
	}
}

func exportTextureArchive(ctx context.Context, cache, archive string, progress textureArchiveProgress) error {
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
			bytesNeeded += uint64(info.Size()) + uint64(2*len(textureArchiveRoot+name)) + 256
		}
	}
	if len(files) == 0 {
		return errors.New("This texture cache has no images to export. Build textures first or choose another cache folder.")
	}
	if progress.checkSpace != nil {
		if err := progress.checkSpace(filepath.Dir(archive), bytesNeeded+256); err != nil {
			return err
		}
	}
	publish := progress.publish
	if publish == nil {
		publish = os.Link
	}
	return publishStagedFile(archive, func(w io.Writer) error {
		writer := zip.NewWriter(w)
		progress.report(0, len(files))
		for i, name := range files {
			file, err := root.Open(filepath.FromSlash(name))
			if err != nil {
				return err
			}
			entry, err := writer.CreateHeader(&zip.FileHeader{Name: textureArchiveRoot + name, Method: zip.Store})
			if err == nil {
				_, err = io.Copy(entry, contextReader{ctx, file})
			}
			if err := errors.Join(err, file.Close()); err != nil {
				return err
			}
			progress.report(i+1, len(files))
		}
		return errors.Join(ctx.Err(), writer.Close())
	}, publish)
}

func importTextureArchive(ctx context.Context, cache, archive string, progress textureArchiveProgress) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	if len(reader.File) == 0 {
		return errors.New("The texture archive contains no images.")
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
			entry.UncompressedSize64 == 0 || entry.UncompressedSize64 > maxInferenceImage {
			return fmt.Errorf("invalid texture archive entry %q", entry.Name)
		}
		seen[entry.Name] = true
		name := filepath.FromSlash(strings.TrimPrefix(entry.Name, textureArchiveRoot))
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
	if progress.checkSpace != nil {
		if err := progress.checkSpace(cache, bytesNeeded); err != nil {
			return err
		}
	}
	done := len(reader.File) - len(pending)
	progress.report(done, len(reader.File))
	for _, entry := range pending {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := filepath.FromSlash(strings.TrimPrefix(entry.Name, textureArchiveRoot))
		if err := importTexturePNG(ctx, root, name, entry); err != nil {
			return fmt.Errorf("import %s: %w", name, err)
		}
		done++
		progress.report(done, len(reader.File))
	}
	return ctx.Err()
}

func importTexturePNG(ctx context.Context, root *os.Root, name string, entry *zip.File) error {
	input, err := entry.Open()
	if err != nil {
		return err
	}
	defer input.Close()
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, input}, int64(entry.UncompressedSize64)+1))
	if err != nil {
		return err
	}
	if uint64(len(data)) != entry.UncompressedSize64 {
		return errors.New("incorrect PNG size")
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if uint64(config.Width)*uint64(config.Height) > 64<<20 {
		return errors.New("texture image is too large")
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		return err
	}

	// Publish only verified entries. The root prevents archive writes through
	// links that lead outside the selected cache.
	folder := filepath.Dir(name)
	if err := root.Mkdir(folder, 0755); err != nil && !errors.Is(err, os.ErrExist) {
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
