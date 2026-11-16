//go:build !cgo

package macos

import "errors"

func installedMacResolution() string { return "" }

func publishMacResolution(requested int, publish func() error) error {
	if requested != 0 {
		return errors.New("Changing the game resolution requires the macOS setup build with cgo enabled.")
	}
	return publish()
}
