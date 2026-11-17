package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func DefaultCache() (string, error) {
	folder, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(folder, "SimCity4HD"), nil
}

func LockFolder(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(filepath.Join(dir, ".simcity4hd-setup.lock"),
		syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("Another SimCity 4 UHD setup operation is using this folder: %w", err)
	}
	return func() { _ = syscall.Close(fd) }, nil
}
