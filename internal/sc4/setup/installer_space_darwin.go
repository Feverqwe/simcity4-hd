package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func lookupSetupVolume(path string) (setupVolume, error) {
	for {
		var info syscall.Statfs_t
		err := syscall.Statfs(path, &info)
		if err == nil {
			name := make([]byte, 0, len(info.Mntonname))
			for _, char := range info.Mntonname {
				if char == 0 {
					break
				}
				name = append(name, byte(char))
			}
			return setupVolume{
				id:   fmt.Sprint(info.Fsid),
				name: string(name),
				free: info.Bavail * uint64(info.Bsize),
			}, nil
		}
		parent := filepath.Dir(path)
		if !errors.Is(err, os.ErrNotExist) || parent == path {
			return setupVolume{}, err
		}
		path = parent
	}
}
