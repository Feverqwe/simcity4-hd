//go:build windows || darwin

package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/texturecache"
)

func TransferTextures(ctx context.Context, command Command, emit func(Event)) error {
	if command.Action == "list-textures" {
		groups, err := texturecache.AvailableExportGroups(ctx, command.Cache)
		if err != nil {
			return FieldError("cache", err)
		}
		emit(Event{Type: "texture-groups", Groups: groups})
		return nil
	}
	if !filepath.IsAbs(command.Archive) {
		return errors.New("Choose a texture archive file.")
	}
	cache := command.Cache
	if err := ctx.Err(); err != nil {
		return err
	}
	exporting := command.Action == "export-textures"
	if exporting {
		if _, err := os.Stat(cache); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				err = errors.New("This cache folder has no texture files yet. Choose a cache with upscaled textures.")
			}
			return FieldError("cache", err)
		}
	}
	unlock, err := LockFolder(cache)
	if err != nil {
		return FieldError("cache", err)
	}
	defer unlock()

	progress := texturecache.ArchiveProgress{
		CheckSpace: func(path string, bytes uint64) error { return CheckSpace(path, bytes, "the texture files") },
		Count:      func(done, total int) { emit(Measurement("count", 0, done, total)) },
		Publish:    publishTextureArchive,
	}
	if exporting {
		emit(Event{Type: "step", Text: "Exporting textures"})
		var groups []string
		if command.ExportGroups != "" {
			groups = strings.Split(command.ExportGroups, ",")
		}
		err = texturecache.ExportTextureArchive(ctx, cache, command.Archive, groups, progress)
	} else {
		emit(Event{Type: "step", Text: "Importing textures"})
		err = texturecache.ImportTextureArchive(ctx, cache, command.Archive, progress)
	}
	if err != nil {
		return err
	}
	emit(Event{Type: "textures-transferred", Path: command.Archive})
	return nil
}
