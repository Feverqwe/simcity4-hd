//go:build !cgo

package sc4

import "os"

func moveNewMacTextureArchive(from, to string) error {
	return os.Link(from, to)
}
