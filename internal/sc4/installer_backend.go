//go:build windows || darwin

package sc4

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup/macos"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup/windows"
)

func runInstaller(args []string) int {
	return setup.Run(args, func(ctx context.Context, command setup.Command, emit func(setup.Event), retry func() error) error {
		if command.Action == "list-textures" || command.Action == "export-textures" ||
			(command.Action == "import-textures" && command.Game == "") {
			cache, err := setup.ResolveCache(command.Cache, "")
			if err != nil {
				return err
			}
			command.Cache = cache
			return setup.TransferTextures(ctx, command, emit)
		}
		if command.Game == "" && command.Action == "inspect" {
			command.Game = findInstallerGame()
			if command.Game == "" {
				emit(setup.Event{Type: "field", Field: "game"})
				return nil
			}
		}
		game, target, err := resolveInstallerGame(command.Game)
		if err != nil {
			return setup.FieldError("game", err)
		}
		if command.Target == "auto" && command.Action != "inspect" && command.Action != "import-textures" {
			return setup.FieldError("game", errors.New("Select the game again so setup can confirm its edition."))
		}
		if command.Target != "auto" && command.Target != target {
			return setup.FieldError("game", errors.New("The selected game no longer matches the detected edition. Select the game again."))
		}
		command.Game = game
		if command.Action == "import-textures" {
			if target == "aspyr" {
				command.Cache, err = macos.ResolveInstallCache(command.Cache, "", game)
			} else {
				command.Cache, err = windows.ResolveInstallCache(command.Cache, "", game)
			}
			if err != nil {
				return err
			}
			return setup.TransferTextures(ctx, command, emit)
		}
		if command.Action == "inspect" {
			emit(setup.Event{Type: "field", Field: "game", Path: game})
			emit(setup.Event{Type: "target", Target: target})
		}
		if target == "aspyr" {
			return macos.Execute(ctx, command, emit, retry)
		}
		return windows.Execute(ctx, command, emit, retry)
	})
}

func resolveInstallerGame(selection string) (game, target string, err error) {
	invalid := errors.New("Choose the Steam game folder, the Aspyr .app, or the game executable.")
	if selection == "" {
		return "", "", invalid
	}
	path, err := filepath.Abs(selection)
	if err != nil {
		return "", "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", "", fmt.Errorf("Could not open the selected game: %w", err)
	}
	if setup.IsRegularFile(path) {
		dir := filepath.Dir(path)
		switch {
		case strings.EqualFold(filepath.Base(dir), "Apps") && strings.EqualFold(filepath.Base(path), "SimCity 4.exe"):
			path = filepath.Dir(dir)
		case strings.EqualFold(filepath.Base(dir), "MacOS") && strings.EqualFold(filepath.Base(filepath.Dir(dir)), "Contents") &&
			(filepath.Base(path) == gamefiles.MacGameExecutable || filepath.Base(path) == "Sim City 4 Deluxe Edition"):
			path = filepath.Dir(filepath.Dir(dir))
		default:
			return "", "", invalid
		}
	}
	bundle := macos.FindBundle(path)
	if setup.IsRegularFile(gamefiles.ExecutablePath(path, false)) && setup.IsRegularFile(filepath.Join(path, "SimCity_1.dat")) {
		if _, mac, err := gamefiles.DataDirectory(path); err != nil || mac {
			return "", "", errors.New("Keep the Windows game folder and the Aspyr app in separate directories.")
		}
		return path, "windows", nil
	}
	if bundle == path {
		return bundle, "aspyr", nil
	}
	if bundle != "" {
		bundle, err = filepath.EvalSymlinks(bundle)
		return bundle, "aspyr", err
	}
	return "", "", invalid
}
