package sc4

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func insideFolder(parent, target string) bool {
	rel, err := filepath.Rel(parent, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func resolveSetupPath(name string) (string, error) {
	name = filepath.Clean(name)
	resolved, err := filepath.EvalSymlinks(name)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return resolved, err
	}
	parent := filepath.Dir(name)
	if parent == name {
		return "", err
	}
	resolved, err = resolveSetupPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(name)), nil
}
