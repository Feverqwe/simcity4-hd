//go:build windows || darwin

package windows

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/pe"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
)

const (
	rendererDLL = "SCD3D11.dll"
	rendererINI = "SCD3D11.ini"
	graphicsDLL = "SC4GraphicsOptions.dll"
	graphicsINI = "SC4GraphicsOptions.ini"
)

func executableHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func modInstalled(game string) bool {
	plugins := filepath.Join(game, "Plugins")
	record, err := os.ReadFile(filepath.Join(plugins, modFolder, modReceipt))
	if err != nil {
		return false
	}
	digest, err := hex.DecodeString(string(record))
	if err != nil || len(digest) != sha256.Size {
		return false
	}
	exe, err := os.ReadFile(filepath.Join(game, "Apps", gameEXE))
	if err != nil {
		return false
	}
	hash := executableHash(exe)
	// Steam may restore the original executable while leaving setup's files installed.
	if hash != string(record) && hash != pe.StockHash {
		return false
	}
	for _, name := range []string{rendererDLL, rendererINI, graphicsDLL, graphicsINI} {
		if !setup.IsRegularFile(filepath.Join(plugins, name)) {
			return false
		}
	}
	return true
}

func publishMod(ctx context.Context, game, work, stage, patched string, stock []byte, renderer []string, graphics string, options setupOptions, fontChanges []setupReplacement, keepWork *bool) (result error) {
	apps, plugins := filepath.Join(game, "Apps"), filepath.Join(game, "Plugins")
	exe := filepath.Join(apps, gameEXE)
	hash := pe.StockHash
	if patched != "" {
		data, err := os.ReadFile(patched)
		if err != nil {
			return err
		}
		hash = executableHash(data)
	}
	if err := os.WriteFile(filepath.Join(stage, modReceipt), []byte(hash), 0644); err != nil {
		return err
	}

	iniChanges := make(setupINIChanges)
	type iniReplacement struct {
		setupReplacement
		backup   string
		original []byte
	}
	var editedSettings []iniReplacement
	var files []setupReplacement
	var installedFiles []string
	for _, name := range []string{rendererDLL, rendererINI, graphicsDLL, graphicsINI} {
		target := filepath.Join(plugins, name)
		info, err := os.Lstat(target)
		exists := err == nil
		if exists && !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", target)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		local := filepath.Join(work, name)
		switch name {
		case rendererDLL, graphicsDLL:
			source := graphics
			if name == rendererDLL {
				source = renderer[0]
			}
			if exists {
				actual, err := os.ReadFile(target)
				if err != nil {
					return err
				}
				expected, err := os.ReadFile(source)
				if err != nil {
					return err
				}
				if !bytes.Equal(actual, expected) {
					return fmt.Errorf("A different %s is already installed. Move it out of Plugins before continuing.", name)
				}
				continue
			}
			if err := gamefiles.CopyRegular(source, local); err != nil {
				return err
			}
		case rendererINI, graphicsINI:
			var original []byte
			if exists || name == rendererINI {
				source := target
				if !exists {
					source = renderer[1]
				}
				original, err = os.ReadFile(source)
				if err != nil {
					return err
				}
			}
			config := iniChanges.edit(name, string(original), options, true, exists)
			if exists && config == string(original) {
				continue
			}
			if err := os.WriteFile(local, []byte(config), 0644); err != nil {
				return err
			}
			if exists {
				backup := filepath.Join(work, "previous-"+name)
				if err := os.WriteFile(backup, original, 0644); err != nil {
					return err
				}
				editedSettings = append(editedSettings, iniReplacement{setupReplacement{local, target}, backup, original})
				continue
			}
		}
		files = append(files, setupReplacement{local, target})
		installedFiles = append(installedFiles, name)
	}
	if err := iniChanges.save(stage); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, installedFilesFile), []byte(strings.Join(installedFiles, "\n")), 0644); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ensureGameClosed(); err != nil {
		return err
	}
	current, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, stock) {
		return errors.New("The game executable changed while setup was running. Verify the game files in Steam and try again.")
	}
	if err := os.MkdirAll(plugins, 0755); err != nil {
		return err
	}

	// Keep the original executable in place until the other files are ready.
	// Roll back files this run published if the final replacement fails.
	var published []string
	defer func() {
		if result != nil && len(published) > 0 {
			fmt.Fprintf(os.Stderr, "Rolling back %d changes: %v\n", len(published), result)
			for i := len(published) - 1; i >= 0; i-- {
				result = errors.Join(result, os.RemoveAll(published[i]))
			}
		}
	}()
	if patched != "" {
		backup := filepath.Join(apps, stockBackup)
		original, err := os.ReadFile(backup)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := gamefiles.PublishFile(backup, func(w io.Writer) error {
				_, err := w.Write(stock)
				return err
			}); err != nil {
				return err
			}
			published = append(published, backup)
			fmt.Fprintf(os.Stderr, "Saved original game executable: %s\n", backup)
		case err != nil:
			return err
		case !bytes.Equal(original, stock):
			return fmt.Errorf("The existing executable backup %s does not match the original game.", backup)
		}
	}
	for _, file := range files {
		if err := moveSetupPath(file.staged, file.target, false); err != nil {
			return err
		}
		published = append(published, file.target)
	}
	destination := filepath.Join(plugins, modFolder)
	if err := moveSetupPath(stage, destination, false); err != nil {
		return err
	}
	published = append(published, destination)
	for _, change := range editedSettings {
		current, err := os.ReadFile(change.target)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, change.original) {
			return fmt.Errorf("%s changed during installation. Try again.", change.target)
		}
		if err := moveSetupPath(change.staged, change.target, true); err != nil {
			return err
		}
		defer func() {
			if result != nil {
				if err := moveSetupPath(change.backup, change.target, true); err != nil {
					*keepWork = true
					result = errors.Join(result, fmt.Errorf("restore %s from %s: %w", change.target, change.backup, err))
				}
			}
		}()
	}
	if len(fontChanges) != 0 {
		return applySetupReplacements(work, append(fontChanges, setupReplacement{patched, exe}), keepWork)
	}
	if patched != "" {
		return moveSetupPath(patched, exe, true)
	}
	return nil
}
