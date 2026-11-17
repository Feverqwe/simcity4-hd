//go:build !cgo

package setup

import "os"

func publishTextureArchive(from, to string) error {
	return os.Link(from, to)
}
