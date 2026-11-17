package macos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
)

func copyMacSetupPaths(ctx context.Context, source, destination string, paths []string) error {
	for _, rel := range paths {
		from, to := filepath.Join(source, rel), filepath.Join(destination, rel)
		info, err := os.Lstat(from)
		if err != nil {
			return err
		}
		if (rel == macSetupSignature && !info.IsDir()) || (rel != macSetupSignature && !info.Mode().IsRegular()) {
			return fmt.Errorf("Unexpected file type: %s", from)
		}
		if err := os.MkdirAll(filepath.Dir(to), 0755); err != nil {
			return err
		}
		if err := runMacSetupTool(ctx, "/usr/bin/ditto", from, to); err != nil {
			return err
		}
		switch rel {
		case macSetupLauncher, gamefiles.ExecutablePath("", true):
			if err := os.Chmod(to, info.Mode().Perm()|0111); err != nil {
				return err
			}
		}
	}
	return nil
}

func lockMacSetup(bundle string) (func(), error) {
	path := macSetupSidecar(bundle, ".patch-lock")
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("Could not lock the Steam installation: %w", err)
	}
	return func() { _ = syscall.Close(fd) }, nil
}
