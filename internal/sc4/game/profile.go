package game

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func PackagePaths(inputs []string) ([]string, error) {
	var packages []string
	seen := make(map[string][]os.FileInfo)
	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return nil, err
		}
		paths := []string{input}
		if info.IsDir() {
			children, err := os.ReadDir(input)
			if err != nil {
				return nil, err
			}
			paths = nil
			for _, child := range children {
				if !child.IsDir() && strings.EqualFold(filepath.Ext(child.Name()), ".dat") {
					paths = append(paths, filepath.Join(input, child.Name()))
				}
			}
			if len(paths) == 0 {
				return nil, fmt.Errorf("%s contains no DAT packages", input)
			}
		}
		for _, path := range paths {
			if !strings.EqualFold(filepath.Ext(path), ".dat") {
				return nil, fmt.Errorf("%s is not a DAT package", path)
			}
			absolute, err := filepath.Abs(path)
			if err != nil {
				return nil, err
			}
			absolute, err = filepath.EvalSymlinks(absolute)
			if err != nil {
				return nil, err
			}
			info, err := os.Stat(absolute)
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("%s is not a regular file", path)
			}
			key := strings.ToLower(absolute)
			if !slices.ContainsFunc(seen[key], func(previous os.FileInfo) bool { return os.SameFile(info, previous) }) {
				seen[key] = append(seen[key], info)
				packages = append(packages, absolute)
			}
		}
	}
	if len(packages) == 0 {
		return nil, errors.New("select at least one DAT package or package directory")
	}
	return packages, nil
}

func CopyRegular(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", source)
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	return errors.Join(copyErr, output.Close())
}
