package sc4

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const installedRussianPack = "RussianLanguage.source"

func originalRussianFont(name string) string {
	return "Original-" + name + ".backup"
}

func hasInstalledRussian(active string) bool {
	return fileExists(filepath.Join(active, installedRussianPack))
}

func stageSetupRussianSource(work, installed string) (string, error) {
	dir := filepath.Join(work, "Russian")
	pack := ""
	if installed != "" {
		pack = filepath.Join(installed, installedRussianPack)
	} else {
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		for _, parent := range []string{filepath.Dir(exe), os.Getenv("SETUP_EXE_DIR")} {
			candidate := filepath.Join(parent, "Russian", russianPackFile)
			if filepath.IsAbs(parent) && fileExists(candidate) {
				pack = candidate
				break
			}
		}
		if pack == "" {
			return "", errors.New("Russian language files are missing from the installer.")
		}
	}
	if err := unpackRussianAssets(pack, dir); err != nil {
		return "", fmt.Errorf("prepare Russian language files: %w", err)
	}
	if err := copyRegular(pack, filepath.Join(dir, russianPackFile)); err != nil {
		return "", err
	}
	return dir, nil
}

// Save each original font together with the hash of its installed replacement.
func stageSetupRussianFonts(game, work, active, newActive, source, ui string) ([]setupReplacement, error) {
	if active == "" && source == "" {
		return nil, nil
	}
	changes := make([]setupReplacement, 0, len(russianFontFiles))
	for _, name := range russianFontFiles {
		backup := originalRussianFont(name)
		target := filepath.Join(game, "Fonts", name)
		info, err := os.Lstat(target)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file", target)
		}
		current, err := os.ReadFile(target)
		if err != nil {
			return nil, err
		}
		original := current
		if active != "" {
			saved, err := os.ReadFile(filepath.Join(active, backup))
			if err != nil {
				return nil, fmt.Errorf("read original game font %s: %w", name, err)
			}
			if len(saved) <= sha256.Size {
				return nil, fmt.Errorf("invalid backup for game font %s", name)
			}
			original = saved[sha256.Size:]
			hash := sha256.Sum256(current)
			if !bytes.Equal(current, original) && !bytes.Equal(hash[:], saved[:sha256.Size]) {
				return nil, fmt.Errorf("%s changed outside setup. Restore the game font before switching languages.", target)
			}
		}
		if source == "" {
			if bytes.Equal(current, original) {
				continue
			}
			staged := filepath.Join(work, backup)
			if err := os.WriteFile(staged, original, 0644); err != nil {
				return nil, err
			}
			changes = append(changes, setupReplacement{staged, target})
			continue
		}

		staged := filepath.Join(ui, "Fonts", name)
		replacement, err := os.ReadFile(staged)
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(replacement)
		if err := os.WriteFile(filepath.Join(newActive, backup), append(hash[:], original...), 0644); err != nil {
			return nil, err
		}
		if !bytes.Equal(current, replacement) {
			changes = append(changes, setupReplacement{staged, target})
		}
	}
	if source != "" {
		if err := os.Link(filepath.Join(source, russianPackFile), filepath.Join(newActive, installedRussianPack)); err != nil {
			return nil, err
		}
	}
	return changes, nil
}
