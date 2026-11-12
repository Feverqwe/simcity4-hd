//go:build bundled

package main

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed runtime.zip
var payload []byte

func setupRuntime() (string, func(), error) {
	dir, err := os.MkdirTemp("", "sc4hd-setup-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if err := unpackRuntime(dir); err != nil {
		cleanup()
		return "", nil, err
	}
	return dir, cleanup, nil
}

func unpackRuntime(dir string) error {
	archive, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return err
	}
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if err := extractRuntimeFile(dir, entry); err != nil {
			return fmt.Errorf("Could not unpack setup: %w", err)
		}
	}
	return nil
}

func extractRuntimeFile(root string, entry *zip.File) error {
	relative, err := filepath.Localize(entry.Name)
	if err != nil || !entry.Mode().IsRegular() {
		return fmt.Errorf("invalid setup file %q", entry.Name)
	}
	input, err := entry.Open()
	if err != nil {
		return err
	}
	defer input.Close()
	target := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	closeErr := output.Close()
	if err != nil {
		return err
	}
	return closeErr
}
