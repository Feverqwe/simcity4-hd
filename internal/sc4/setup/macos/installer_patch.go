package macos

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/russian"
)

const (
	macSetupLauncher     = "Contents/MacOS/Sim City 4 Deluxe Edition"
	macSetupSteamLibrary = "Contents/MacOS/libsteam_api.dylib"
	macSetupSignature    = "Contents/_CodeSignature"
	macSetupBackupPath   = "Contents/Resources/SimCity4HD-Original"
)

func macSetupSupportPaths() []string {
	paths := []string{macSetupLauncher, macSetupSteamLibrary, macSetupSignature}
	for _, font := range russian.FontFiles {
		paths = append(paths, filepath.Join("Contents", "GameData", "Fonts", font))
	}
	return paths
}

func macSetupOriginalPaths() []string {
	return append(macSetupSupportPaths(), gamefiles.ExecutablePath("", true))
}

func macSetupPatchPaths() []string {
	paths := append(macSetupOriginalPaths(), macModPlugins(""), macInactiveWorld(""), macInactiveCursor(""))
	for i, rel := range paths {
		paths[i] = filepath.ToSlash(rel)
	}
	return paths
}

func macSetupBackup(bundle string) string {
	return filepath.Join(bundle, macSetupBackupPath)
}

func macSetupSavedFiles(bundle string) (string, error) {
	saved := macSetupBackup(bundle)
	info, err := os.Lstat(saved)
	if errors.Is(err, os.ErrNotExist) {
		return "", errors.New("The original game files are missing from this Aspyr app. Select a clean Steam installation.")
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("The original game backup is invalid: %s", saved)
	}
	return saved, nil
}

func validateMacSetupDirectories(bundle string) error {
	for _, rel := range []string{"", "Contents", "Contents/MacOS", "Contents/Resources",
		"Contents/GameData", "Contents/GameData/Fonts", "Contents/GameData/Plugins"} {
		path := filepath.Join(bundle, rel)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("Expected a game directory at %s", path)
		}
	}
	return nil
}

func macSetupSidecar(bundle, suffix string) string {
	return filepath.Join(filepath.Dir(bundle), "."+filepath.Base(bundle)+".sc4-setup"+suffix)
}

func macSetupTransactionDir(bundle string) string {
	return macSetupSidecar(bundle, ".transaction")
}

func macSetupWork(bundle string) string {
	return macSetupSidecar(bundle, ".work")
}

func prepareMacSetupWork(bundle string) (string, error) {
	work := macSetupWork(bundle)
	if err := os.Mkdir(work, 0700); errors.Is(err, os.ErrExist) {
		info, statErr := os.Lstat(work)
		if statErr != nil {
			return "", statErr
		}
		if !info.IsDir() {
			return "", fmt.Errorf("The setup work path is occupied: %s", work)
		}
		marker, markerErr := os.Lstat(filepath.Join(work, ".sc4-setup"))
		if markerErr != nil || !marker.Mode().IsRegular() {
			return "", fmt.Errorf("The setup work directory is occupied: %s", work)
		}
		if err := os.RemoveAll(work); err != nil {
			return "", err
		}
		if err := os.Mkdir(work, 0700); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(work, ".sc4-setup"), nil, 0600); err != nil {
		_ = os.Remove(work)
		return "", err
	}
	return work, nil
}

type macSetupReplacement struct {
	Path        string
	HadOriginal bool
}

func macSetupPrevious(transaction string, index int) string {
	return filepath.Join(transaction, fmt.Sprintf("previous-%d", index))
}

func restoreMacSetupFiles(bundle string, changes []macSetupReplacement) error {
	transaction := macSetupTransactionDir(bundle)
	var result error
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		target := filepath.Join(bundle, change.Path)
		previous := macSetupPrevious(transaction, i)
		_, err := os.Lstat(previous)
		if errors.Is(err, os.ErrNotExist) {
			if !change.HadOriginal {
				result = errors.Join(result, os.RemoveAll(target))
			}
			continue
		}
		if err == nil {
			err = os.RemoveAll(target)
		}
		if err == nil {
			err = os.Rename(previous, target)
		}
		if err != nil {
			result = errors.Join(result, fmt.Errorf("restore %s: %w", target, err))
		}
	}
	if result != nil {
		return fmt.Errorf("Could not restore the Steam installation from %s: %w", transaction, result)
	}
	cleanup, err := os.MkdirTemp(filepath.Dir(transaction), ".sc4-setup-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(cleanup)
	return os.Rename(transaction, filepath.Join(cleanup, "restored"))
}

func recoverMacSetup(bundle string) (bool, error) {
	transaction := macSetupTransactionDir(bundle)
	info, err := os.Lstat(transaction)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("Invalid setup transaction: %s", transaction)
	}
	data, err := os.ReadFile(filepath.Join(transaction, "changes.json"))
	if err != nil {
		return false, err
	}
	var changes []macSetupReplacement
	if err := json.Unmarshal(data, &changes); err != nil {
		return false, err
	}
	allowed := make(map[string]bool)
	for _, rel := range append(macSetupPatchPaths(), macSetupBackupPath) {
		allowed[rel] = true
	}
	for _, change := range changes {
		if !allowed[change.Path] {
			return false, fmt.Errorf("Unexpected path in the setup transaction: %q", change.Path)
		}
		delete(allowed, change.Path)
	}
	if err := restoreMacSetupFiles(bundle, changes); err != nil {
		return false, err
	}
	return true, nil
}

// Publish the journal before moving any live files. An interrupted run can
// restore the previous versions by looking for the files already moved aside.
func applyMacSetupFiles(bundle, work, staged string, replaceBackup bool, verify func() error) (result error) {
	draft := filepath.Join(work, "journal")
	if err := os.Mkdir(draft, 0700); err != nil {
		return err
	}
	paths := macSetupPatchPaths()
	if replaceBackup {
		paths = append(paths, macSetupBackupPath)
	}
	changes := make([]macSetupReplacement, 0, len(paths))
	for _, rel := range paths {
		_, err := os.Lstat(filepath.Join(bundle, rel))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		changes = append(changes, macSetupReplacement{Path: rel, HadOriginal: err == nil})
	}
	data, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	if err := gamefiles.PublishFile(filepath.Join(draft, "changes.json"), func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	}); err != nil {
		return err
	}
	transaction := macSetupTransactionDir(bundle)
	if err := os.Rename(draft, transaction); err != nil {
		return err
	}
	defer func() {
		if result != nil {
			result = errors.Join(result, restoreMacSetupFiles(bundle, changes))
		}
	}()
	for i, change := range changes {
		target := filepath.Join(bundle, change.Path)
		if change.HadOriginal {
			if err := os.Rename(target, macSetupPrevious(transaction, i)); err != nil {
				return err
			}
		}
		replacement := filepath.Join(staged, change.Path)
		if _, err := os.Lstat(replacement); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := os.Rename(replacement, target); err != nil {
			return err
		}
	}
	if err := verify(); err != nil {
		return err
	}
	return os.Rename(transaction, draft)
}
