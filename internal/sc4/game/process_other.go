//go:build !windows

package game

import (
	"errors"
	"os/exec"
	"strings"
	"unicode"
)

func ConfigureBackgroundProcess(*exec.Cmd) {}

func ProfilePath(path string) (string, error) {
	if strings.ContainsFunc(path, unicode.IsSpace) {
		return "", errors.New("use a separate profile directory whose path contains no spaces")
	}
	return path, nil
}
