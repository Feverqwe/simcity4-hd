//go:build !cgo

package sc4

func installedMacResolution() string { return "" }

func publishMacResolution(_ int, publish func() error) error {
	return publish() // The app swap reports that installation requires cgo.
}
