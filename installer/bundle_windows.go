//go:build bundled

package main

import (
	"archive/tar"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
)

//go:embed runtime.tar.xz
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
	stream, err := (xz.ReaderConfig{SingleStream: true}).NewReader(bytes.NewReader(payload))
	if err != nil {
		return err
	}
	archive := tar.NewReader(stream)
	for {
		entry, err := archive.Next()
		if err == io.EOF {
			// Read the stream footer to verify the XZ integrity check.
			_, err = io.Copy(io.Discard, stream)
			return err
		}
		if err != nil {
			return err
		}
		if entry.Typeflag == tar.TypeDir {
			continue
		}
		relative, err := filepath.Localize(strings.TrimPrefix(entry.Name, "./"))
		if err != nil || entry.Typeflag != tar.TypeReg {
			return fmt.Errorf("invalid setup file %q", entry.Name)
		}
		if err := extractRuntimeFile(filepath.Join(dir, relative), archive); err != nil {
			return fmt.Errorf("could not unpack setup: %w", err)
		}
	}
}

func extractRuntimeFile(target string, input io.Reader) error {
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
