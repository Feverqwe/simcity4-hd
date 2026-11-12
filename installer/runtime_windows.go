//go:build !bundled

package main

import (
	"errors"
	"os"
	"path/filepath"
)

func setupRuntime() (string, func(), error) {
	self, err := os.Executable()
	return filepath.Dir(self), func() {}, err
}

func unpackRuntime(string) error {
	return errors.New("This development build has no embedded runtime.")
}
