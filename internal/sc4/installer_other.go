//go:build !windows && !darwin

package sc4

import (
	"fmt"
	"os"
)

func runInstaller([]string) int {
	fmt.Fprintln(os.Stderr, "setup requires Windows or macOS")
	return 1
}
