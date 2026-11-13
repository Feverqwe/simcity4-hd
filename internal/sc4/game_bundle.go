package sc4

import (
	"errors"
	"os"
	"path/filepath"
)

const macGameExecutable = "Sim City 4 Deluxe Editionsub"

// Identify the target by its on-disk layout so the builder can run on any host.
func gameDataDirectory(path string) (directory string, mac bool, err error) {
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

func gameExecutablePath(path string, mac bool) string {
	if mac {
		return filepath.Join(path, "Contents", "MacOS", macGameExecutable)
	}
	return filepath.Join(path, "Apps", "SimCity 4.exe")
}
