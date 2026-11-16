package sc4

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func macTextureCache(path string) (string, error) {
	if path == "" {
		parent, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(parent, "SimCity4HD")
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("Use an absolute path for the texture cache.")
	}
	return filepath.Clean(path), nil
}

func lockMacTextureCache(cache string) (func(), error) {
	if err := os.MkdirAll(cache, 0755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(cache, ".simcity4hd-setup.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.New("Another SimCity 4 UHD setup operation is using this cache.")
		}
		return nil, err
	}
	return func() { _ = file.Close() }, nil
}

func transferMacTextures(ctx context.Context, command, cache, archive string, send func(macSetupEvent)) error {
	cache, err := macTextureCache(cache)
	if err != nil {
		return macSetupInputError{"cache", err}
	}
	if !filepath.IsAbs(archive) {
		return errors.New("Choose a texture archive file.")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	exporting := command == "export-textures"
	if exporting {
		if _, err := os.Stat(cache); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				err = errors.New("This cache folder has no images yet. Choose a cache with upscaled textures.")
			}
			return macSetupInputError{"cache", err}
		}
	}
	unlock, err := lockMacTextureCache(cache)
	if err != nil {
		return macSetupInputError{"cache", err}
	}
	defer unlock()

	progress := textureArchiveProgress{
		checkSpace: func(path string, needed uint64) error {
			return checkMacFreeSpace(path, needed, "the texture images")
		},
		count: func(done, total int) {
			send(macSetupEvent{Type: "count", Value: &done, Total: total})
		},
		publish: moveNewMacTextureArchive,
	}
	if exporting {
		send(macSetupEvent{Type: "step", Text: "Exporting texture images"})
		err = exportTextureArchive(ctx, cache, archive, progress)
	} else {
		send(macSetupEvent{Type: "step", Text: "Importing texture images"})
		err = importTextureArchive(ctx, cache, archive, progress)
	}
	if err != nil {
		return err
	}
	send(macSetupEvent{Type: "textures-transferred", Path: archive})
	return nil
}

func checkMacFreeSpace(path string, needed uint64, purpose string) error {
	var stats syscall.Statfs_t
	if err := syscall.Statfs(path, &stats); err != nil {
		return err
	}
	if available := stats.Bavail * uint64(stats.Bsize); available < needed {
		return fmt.Errorf("Not enough space for %s. %.1f GiB is needed; only %.1f GiB is available.", purpose, float64(needed)/(1<<30), float64(available)/(1<<30))
	}
	return nil
}
