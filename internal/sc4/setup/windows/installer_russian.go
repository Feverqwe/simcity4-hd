//go:build windows || darwin

package windows

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/russian"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
)

const installedRussianPack = "RussianLanguage.source"

func originalRussianFont(name string) string {
	return "Original-" + name + ".backup"
}

func hasInstalledRussian(active string) bool {
	return setup.IsRegularFile(filepath.Join(active, installedRussianPack))
}

// Save each original font together with the hash of its installed replacement.
func stageSetupRussianFonts(game, work, active, newActive, source, ui string) ([]setupReplacement, error) {
	if active == "" && source == "" {
		return nil, nil
	}
	changes := make([]setupReplacement, 0, len(russian.FontFiles))
	for _, name := range russian.FontFiles {
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
		if err := linkSetupFile(filepath.Join(source, russian.PackFile), filepath.Join(newActive, installedRussianPack)); err != nil {
			return nil, err
		}
	}
	return changes, nil
}
