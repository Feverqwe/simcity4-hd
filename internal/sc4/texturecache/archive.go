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
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

const TextureArchiveRoot = "SimCity4UHD-Textures-v2/"

type ExportGroup struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Prepared bool   `json:"prepared"`
}

var exportGroups = []ExportGroup{
	{"interface-x2", "Interface ×2", true},
	{"interface-x4", "Interface ×4", true},
	{"interface-common", "Shared interface textures", true},
	{"world-x2", "World ×2", true},
	{"world-x4", "World ×4", true},
	{"original", "Original upscaled PNGs", false},
}

type archiveFolder struct {
	path, group string
	pattern     *regexp.Regexp
}

var archiveFolders = textureArchiveFolders()

func textureArchiveFolders() []archiveFolder {
	var folders []archiveFolder
	add := func(path, group string, pattern *regexp.Regexp) {
		folders = append(folders, archiveFolder{filepath.ToSlash(path), group, pattern})
	}
	pngName := regexp.MustCompile(`^[0-9a-f]{64}\.png$`)
	fshName := regexp.MustCompile(`^[0-9a-f]{64}\.fsh$`)
	add(upscale.InferenceCacheFolder, "original", pngName)
	add(InterfaceCache("", 1), "interface-common", pngName)
	for _, scale := range []int{2, 4} {
		worldGroup := fmt.Sprintf("world-x%d", scale)
		add(InterfaceCache("", scale), fmt.Sprintf("interface-x%d", scale), pngName)
		add(ModelCache("", scale), worldGroup, fshName)
		add(NetworkCache("", scale), worldGroup, fshName)
		for _, category := range WorldCategories {
			add(category.CacheFolder("", scale), worldGroup, fshName)
		}
	}
	return folders
}

func textureArchiveMember(name string) bool {
	relative, ok := strings.CutPrefix(name, TextureArchiveRoot)
	if !ok {
		return false
	}
	parent, base, ok := cutArchiveFile(relative)
	if ok {
		for _, folder := range archiveFolders {
			if folder.path == parent && folder.pattern.MatchString(base) {
				return true
			}
		}
	}
	return false
}

func cutArchiveFile(name string) (string, string, bool) {
	lastSlash := strings.LastIndexByte(name, '/')
	if lastSlash < 0 {
		return "", "", false
	}
	return name[:lastSlash], name[lastSlash+1:], true
}

func AvailableExportGroups(ctx context.Context, cache string) ([]ExportGroup, error) {
	root, err := os.OpenRoot(cache)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	available := make(map[string]bool)
	for _, folder := range archiveFolders {
		if available[folder.group] {
			continue
		}
		entries, err := archiveEntries(ctx, root, folder)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.Type().IsRegular() && folder.pattern.MatchString(entry.Name()) {
				available[folder.group] = true
				break
			}
		}
	}
	var result []ExportGroup
	for _, group := range exportGroups {
		if available[group.ID] {
			result = append(result, group)
		}
	}
	return result, nil
}

func archiveEntries(ctx context.Context, root *os.Root, folder archiveFolder) ([]fs.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(root.FS(), folder.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return entries, err
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

func ExportTextureArchive(ctx context.Context, cache, archive string, groups []string, progress ArchiveProgress) error {
	root, err := os.OpenRoot(cache)
	if err != nil {
		return err
	}
	defer root.Close()

	var files []string
	selected := make(map[string]bool)
	for _, group := range exportGroups {
		if len(groups) == 0 && group.Prepared || slices.Contains(groups, group.ID) {
			selected[group.ID] = true
		}
	}
	for _, group := range groups {
		if !selected[group] {
			return fmt.Errorf("Unknown texture export category %q.", group)
		}
	}
	var bytesNeeded uint64
	for _, folder := range archiveFolders {
		if !selected[folder.group] {
			continue
		}
		entries, err := archiveEntries(ctx, root, folder)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := folder.path + "/" + entry.Name()
			if !entry.Type().IsRegular() || !folder.pattern.MatchString(entry.Name()) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
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
