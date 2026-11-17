package setup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func insideFolder(parent, target string) (bool, error) {
	folder, err := os.Stat(parent)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	for current := target; ; current = filepath.Dir(current) {
		if current == parent || (folder == nil && strings.EqualFold(current, parent)) {
			return true, nil
		}
		if folder != nil {
			info, err := os.Stat(current)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return false, err
			}
			if info != nil && os.SameFile(folder, info) {
				return true, nil
			}
		}
		if filepath.Dir(current) == current {
			return false, nil
		}
	}
}

func ResolveSetupPath(name string) (string, error) {
	name = filepath.Clean(name)
	resolved, err := filepath.EvalSymlinks(name)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return resolved, err
	}
	parent := filepath.Dir(name)
	if parent == name {
		return "", err
	}
	resolved, err = ResolveSetupPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(name)), nil
}

func IsRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
