//go:build !windows

package sc4

import (
	"errors"
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
