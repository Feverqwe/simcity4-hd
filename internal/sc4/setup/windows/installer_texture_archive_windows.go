package windows

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/texturecache"
)

func setupTextureTransfer(command, cache, archive string, emit func(installEvent)) error {
	if !filepath.IsAbs(cache) {
		return setupInputError{"cache", "Use an absolute path for the texture cache."}
	}
	if !filepath.IsAbs(archive) {
		return errors.New("Choose a texture archive file.")
	}
	cache, err := setup.ResolveSetupPath(cache)
	if err != nil {
		return setupInputError{"cache", fmt.Sprintf("Could not open the texture cache: %v", err)}
	}
	exporting := command == "export-textures"
	if exporting {
		if _, err := os.Stat(cache); errors.Is(err, os.ErrNotExist) {
			return setupInputError{"cache", "This cache folder has no texture files yet. Choose a cache with upscaled textures."}
		}
	}
	unlock, err := installLock(cache)
	if err != nil {
		return err
	}
	defer unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		cancel()
	}()
	progress := texturecache.ArchiveProgress{
		CheckSpace: func(target string, bytes uint64) error {
			return checkVolumeSpace([]spaceRequest{{target, "the texture files", bytes}}, lookupSetupVolume)
		},
		Count: func(done, total int) {
			emit(installEvent{Type: "count", Value: done, Total: total})
		},
		Publish: moveNewTextureArchive,
	}
	if exporting {
		emit(installEvent{Type: "step", Text: "Exporting textures"})
		err = texturecache.ExportTextureArchive(ctx, cache, archive, progress)
	} else {
		emit(installEvent{Type: "step", Text: "Importing textures"})
		err = texturecache.ImportTextureArchive(ctx, cache, archive, progress)
	}
	if err != nil {
		return err
	}
	emit(installEvent{Type: "textures-transferred", Path: archive})
	return nil
}

func moveNewTextureArchive(from, to string) error {
	source, err := syscall.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	target, err := syscall.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return syscall.MoveFile(source, target)
}
