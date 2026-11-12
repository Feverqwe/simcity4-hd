//go:build !windows

package sc4

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode"
)

func configureBackgroundProcess(*exec.Cmd) {}

func gameProfilePath(path string) (string, error) {
	if strings.ContainsFunc(path, unicode.IsSpace) {
		return "", errors.New("use a separate profile directory whose path contains no spaces")
	}
	return path, nil
}

func runInstaller([]string) int {
	fmt.Fprintln(os.Stderr, "setup requires Windows")
	return 1
}
