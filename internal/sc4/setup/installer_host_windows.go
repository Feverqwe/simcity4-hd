package setup

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func LockFolder(data string) (func(), error) {
	if err := os.MkdirAll(data, 0755); err != nil {
		return nil, err
	}
	path, err := syscall.UTF16PtrFromString(filepath.Join(data, ".simcity4hd-setup.lock"))
	if err != nil {
		return nil, err
	}
	const deleteOnClose = 0x04000000
	const deleteAccess = 0x00010000
	handle, err := syscall.CreateFile(path, deleteAccess, 0, nil, syscall.OPEN_ALWAYS, deleteOnClose, 0)
	if errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(303)) { // Sharing violation or delete pending.
		return nil, errors.New("Another SimCity 4 UHD installation is already running.")
	}
	if err != nil {
		return nil, err
	}
	return func() { syscall.CloseHandle(handle) }, nil
}

func DefaultCache() (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if !filepath.IsAbs(local) {
		return "", errors.New("Windows could not find your Local AppData folder.")
	}
	return filepath.Join(local, "SimCity4HD", "Cache"), nil
}
