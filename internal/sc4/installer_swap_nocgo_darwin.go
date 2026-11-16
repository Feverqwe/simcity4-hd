//go:build !cgo

package sc4

import (
	"errors"
	"os"
)

func swapMacApps(first, second string) error {
	return errors.New("Changing a Steam installation requires the macOS setup build with cgo enabled.")
}

func moveNewMacTextureArchive(from, to string) error {
	return os.Link(from, to)
}
