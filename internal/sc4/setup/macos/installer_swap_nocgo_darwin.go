//go:build !cgo

package macos

import "os"

func moveNewMacTextureArchive(from, to string) error {
	return os.Link(from, to)
}
