//go:build !windows && !darwin

package sc4

import (
	"errors"
	"fmt"
	"os"
)

func checkGameLaunch(bool) error {
	return errors.New("launch the game on its native operating system")
}

func runInstaller([]string) int {
	fmt.Fprintln(os.Stderr, "setup requires Windows or macOS")
	return 1
}
