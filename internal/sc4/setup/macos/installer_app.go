//go:build windows || darwin

package macos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
)

func Execute(ctx context.Context, command setup.Command, emit func(setup.Event), retry func() error) error {
	bundle := command.Game
	switch command.Action {
	case "inspect":
		if _, err := os.Lstat(macSetupTransactionDir(bundle)); err == nil {
			return errors.New("A previous setup operation was interrupted. Close the game and click Install to restore the previous state before continuing.")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if current, err := readMacSetupReceipt(bundle); err == nil {
			if !canSignMacApp && !current.NeedsSigning {
				return verifyMacSetupSignature(ctx, bundle)
			}
			if _, err := macSetupSavedFiles(bundle); err != nil {
				return setup.FieldError("game", err)
			}
			if filepath.IsAbs(current.Cache) {
				emit(setup.Event{Type: "field", Field: "cache", Path: current.Cache})
			}
			settings := current.settings()
			if canSignMacApp {
				settings.Resolution = installedMacResolution()
			}
			event := setup.Event{Type: "installed", Path: bundle, Settings: settings}
			if current.NeedsSigning {
				if current.PendingResolution != "" {
					settings.Resolution = current.PendingResolution
				}
				if canSignMacApp {
					event.Text = "The mod was installed on Windows. Click Configure, then Apply changes to sign the game on this Mac."
				} else {
					event.Text = "The mod is installed. You can change its settings here and sign the app on your Mac."
				}
			}
			emit(event)
			return nil
		}
		if err := validateMacSetupDirectories(bundle); err != nil {
			return setup.FieldError("game", err)
		}
		if err := checkMacInstallTarget(bundle); err != nil {
			return setup.FieldError("game", err)
		}
		if !canSignMacApp {
			emit(setup.Event{
				Type: "ready", Text: "Aspyr game found. Install the mod here, then finish signing the game on your Mac.",
				Settings: &setup.Settings{Resolution: "auto"},
			})
		}
		return nil
	case "install", "configure", "remove":
	default:
		return fmt.Errorf("unknown installer command %q", command.Action)
	}
	return changeMacInstallation(ctx, command, emit, retry)
}

func FindBundle(path string) string {
	if path == "" {
		return ""
	}
	for _, candidate := range []string{path,
		filepath.Join(path, "SimCity™4 Deluxe Edition.app"),
		filepath.Join(path, "Sim City 4 Deluxe Edition.app")} {
		if !strings.EqualFold(filepath.Ext(candidate), ".app") {
			continue
		}
		exe, exeErr := os.Stat(gamefiles.ExecutablePath(candidate, true))
		data, dataErr := os.Stat(filepath.Join(candidate, "Contents", "GameData", "SimCity_1.dat"))
		if dataErr != nil || !data.Mode().IsRegular() {
			continue
		}
		if exeErr == nil && exe.Mode().IsRegular() {
			return candidate
		}
		if _, err := os.Lstat(macSetupTransactionDir(candidate)); err == nil {
			return candidate
		}
	}
	return ""
}
