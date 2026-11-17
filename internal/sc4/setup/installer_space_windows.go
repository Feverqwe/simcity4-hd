package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	getVolumePath = syscall.NewLazyDLL("kernel32.dll").NewProc("GetVolumePathNameW")
	getVolumeID   = syscall.NewLazyDLL("kernel32.dll").NewProc("GetVolumeNameForVolumeMountPointW")
	getDiskFree   = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")
)

func lookupSetupVolume(path string) (setupVolume, error) {
	for {
		info, err := os.Stat(path)
		if err == nil {
			if !info.IsDir() {
				return setupVolume{}, fmt.Errorf("%s is not a folder", path)
			}
			break
		}
		parent := filepath.Dir(path)
		if !errors.Is(err, os.ErrNotExist) || parent == path {
			return setupVolume{}, err
		}
		path = parent
	}
	ptr, err := syscall.UTF16PtrFromString(strings.TrimRight(path, `\/`) + `\`)
	if err != nil {
		return setupVolume{}, err
	}
	var free uint64
	if ok, _, err := getDiskFree.Call(uintptr(unsafe.Pointer(ptr)), uintptr(unsafe.Pointer(&free)), 0, 0); ok == 0 {
		return setupVolume{}, err
	}
	root := make([]uint16, 32768)
	if ok, _, err := getVolumePath.Call(uintptr(unsafe.Pointer(ptr)), uintptr(unsafe.Pointer(&root[0])), uintptr(len(root))); ok == 0 {
		return setupVolume{}, err
	}
	name := syscall.UTF16ToString(root)
	id := name
	guid := make([]uint16, 64)
	if ok, _, _ := getVolumeID.Call(uintptr(unsafe.Pointer(&root[0])), uintptr(unsafe.Pointer(&guid[0])), uintptr(len(guid))); ok != 0 {
		id = syscall.UTF16ToString(guid)
	}
	return setupVolume{id: strings.ToLower(id), name: name, free: free}, nil
}
