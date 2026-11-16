package game

import (
	"errors"
	"os"
	"path/filepath"
)

const MacGameExecutable = "Sim City 4 Deluxe Editionsub"

// Identify the target by its on-disk layout so the builder can run on any host.
func DataDirectory(path string) (directory string, mac bool, err error) {
	data := filepath.Join(path, "Contents", "GameData")
	info, err := os.Stat(data)
	if errors.Is(err, os.ErrNotExist) {
		return path, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.IsDir() {
		return "", false, &os.PathError{Op: "open game data directory", Path: data, Err: os.ErrInvalid}
	}
	return data, true, nil
}

func ExecutablePath(path string, mac bool) string {
	if mac {
		return filepath.Join(path, "Contents", "MacOS", MacGameExecutable)
	}
	return filepath.Join(path, "Apps", "SimCity 4.exe")
}
