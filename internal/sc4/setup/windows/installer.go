//go:build windows || darwin

package windows

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/advisors"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/pe"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
	worldbuild "github.com/Feverqwe/simcity4-hd/internal/sc4/world"
)

const (
	gameEXE     = "SimCity 4.exe"
	stockBackup = "SimCity 4.stock.exe"
	modFolder   = "SimCity4HD"
	modReceipt  = "Installed.json"
)

type setupOptions struct {
	ui, skipIntro, drawStats, russian bool
	setup.BuildOptions
	keepResolution bool
	world          worldbuild.Selection
	width, height  int
}

func Execute(ctx context.Context, command setup.Command, emit func(setup.Event), retry func() error) error {
	game := command.Game
	switch command.Action {
	case "inspect":
		if modInstalled(game) {
			return reportInstalled(game, emit)
		}
		if _, err := os.Lstat(filepath.Join(game, "Plugins", modFolder)); err == nil {
			return errors.New("An incomplete SimCity 4 UHD installation was found. Restore the Steam game before continuing.")
		}
		if _, err := readOriginalGame(game); err != nil {
			return err
		}
		if foreignGame {
			emit(setup.Event{Type: "ready", Text: "Windows game found. Install the mod here, then play the game on Windows."})
		}
		return nil
	case "remove":
		if err := removeHD(game, command.Cache); err != nil {
			return err
		}
		emit(setup.Event{Type: "removed"})
		return nil
	case "install", "configure":
		if command.PreparedUI != "" || command.PreparedWorld != "" {
			return errors.New("Prepared builds are supported only for the Aspyr game.")
		}
		if command.Action == "configure" {
			if err := configureHD(ctx, command, emit, retry); err != nil {
				return err
			}
		} else {
			options, err := optionsFor(command)
			if err != nil {
				return err
			}
			if err := installHD(ctx, game, command.Cache, options, emit, retry); err != nil {
				return err
			}
		}
		if foreignGame {
			emit(setup.Event{Type: "handoff", Path: game, Text: "The mod is installed. Copy the game folder to Windows to play. Close any running game before replacing its files."})
		} else {
			emit(setup.Event{Type: "installed"})
		}
		return nil
	default:
		return fmt.Errorf("unknown installer command %q", command.Action)
	}
}

func optionsFor(command setup.Command) (setupOptions, error) {
	if err := command.ValidateOptions(); err != nil {
		return setupOptions{}, err
	}
	options := setupOptions{
		ui: command.Interface, skipIntro: command.SkipIntro, drawStats: command.DrawStats,
		russian: command.Language == "russian", BuildOptions: command.BuildOptions,
		world: command.WorldSelection(),
	}
	resolution := command.Resolution
	if resolution == "" {
		options.keepResolution = command.Action == "configure"
		return options, nil
	}
	if resolution == "auto" {
		return options, nil
	}
	if !gamefiles.ResolutionPattern.MatchString(resolution) {
		return setupOptions{}, setup.FieldError("resolution", errors.New("Choose Automatic or use a game resolution in WIDTHxHEIGHT format."))
	}
	w, h, _ := strings.Cut(resolution, "x")
	options.width, _ = strconv.Atoi(w)
	options.height, _ = strconv.Atoi(h)
	if options.width < 800 || options.height < 600 {
		return setupOptions{}, setup.FieldError("resolution", errors.New("Choose a game resolution of at least 800 × 600."))
	}
	return options, nil
}

func readOriginalGame(game string) ([]byte, error) {
	exe, err := os.ReadFile(gamefiles.ExecutablePath(game, false))
	if err != nil {
		return nil, setup.FieldError("game", fmt.Errorf("Could not read the Steam game executable: %w", err))
	}
	if executableHash(exe) != pe.StockHash {
		return nil, setup.FieldError("game", errors.New("The original Steam build 4644414 is required. Verify the game files in Steam or select an unmodified installation."))
	}
	return exe, nil
}

func installHD(ctx context.Context, game, cache string, options setupOptions, emit func(setup.Event), waitForRetry func() error) error {
	emit(setup.Event{Type: "step", Phase: 1, Text: "Checking the game installation and cache"})
	exe, err := readOriginalGame(game)
	if err != nil {
		return err
	}
	plugins := filepath.Join(game, "Plugins")
	destination := filepath.Join(plugins, modFolder)
	cache, err = ResolveInstallCache(cache, "", game)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Game: %q; cache: %q\n", game, cache)
	unlockGame, err := setup.LockFolder(game)
	if err != nil {
		return err
	}
	defer unlockGame()
	if err := ensureGameClosed(); err != nil {
		return err
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return errors.New("Plugins\\SimCity4HD already exists. Move the existing HD package out of Plugins before installing again.")
	}
	if _, err := os.Lstat(filepath.Join(game, inactiveWorldFolder)); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return errors.New("A folder with disabled HD textures already exists. Move it out of the game folder before installing again.")
	}
	work, err := os.MkdirTemp(game, ".setup-work-")
	if err != nil {
		return err
	}
	keepWork := false
	defer func() {
		if !keepWork {
			_ = os.RemoveAll(work)
		}
	}()
	build := setup.TextureBuild{
		BuildOptions: options.BuildOptions, Game: game, Cache: cache, Work: work,
		Interface: options.ui, Russian: options.russian, World: options.world,
	}
	plan := setup.InstallationPlan{
		Textures: build, NeedsCache: true,
		CheckSpace: func() error { return checkBuildSpace(ctx, build, true) },
		Stage: func(outputs setup.TextureOutputs, deps *setup.Resources) (func(context.Context) error, error) {
			var patched string
			if outputs.Interface != "" {
				patched = filepath.Join(outputs.Interface, "SimCity4-hd.exe")
			}
			const phase = 3
			deps.Step(phase, "Preparing the 4K renderer")
			renderer, err := setupRenderer(deps, phase)
			if err != nil {
				return nil, err
			}
			graphics, err := setupGraphics(deps, phase)
			if err != nil {
				return nil, err
			}
			deps.Step(phase, "Installing and checking HD files")
			stage := filepath.Join(work, "Install")
			if err := os.Mkdir(stage, 0755); err != nil {
				return nil, err
			}
			if options.ui {
				deps.Step(phase, "Preparing the advisors")
				if err := stageSetupAdvisors(ctx, game, stage); err != nil {
					return nil, err
				}
			}
			if packageDirs := outputs.PackageDirs(); len(packageDirs) > 0 {
				packages, err := gamefiles.PackagePaths(packageDirs)
				if err != nil {
					return nil, err
				}
				validation := packages
				if options.ui {
					validation = append(validation, filepath.Join(stage, advisors.AdvisorsPackage))
				}
				if err := gamefiles.VerifyPackages(validation, os.Stderr); err != nil {
					return nil, err
				}
				for _, name := range packages {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
					if err := moveSetupPath(name, filepath.Join(stage, filepath.Base(name)), false); err != nil {
						return nil, err
					}
				}
			}
			if err := writeSetupCacheSettings(stage, cache, options.KeepUpscaledImages); err != nil {
				return nil, err
			}
			var fontChanges []setupReplacement
			if options.russian {
				fontChanges, err = stageSetupRussianFonts(game, work, "", stage, outputs.RussianSource, outputs.Interface)
				if err != nil {
					return nil, err
				}
			}
			return func(applyCtx context.Context) error {
				return publishMod(applyCtx, game, work, stage, patched, exe, renderer, graphics, options, fontChanges, &keepWork)
			}, nil
		},
	}
	return plan.Run(ctx, emit, waitForRetry)
}

func ResolveInstallCache(cache, installed, game string) (string, error) {
	return setup.ResolveCache(cache, installed, game,
		filepath.Join(game, "Plugins", modFolder), filepath.Join(game, inactiveWorldFolder))
}

func linkSetupFile(source, target string) error {
	if err := os.Link(source, target); err != nil {
		if copyErr := gamefiles.CopyRegular(source, target); copyErr != nil {
			return errors.Join(err, copyErr)
		}
	}
	return nil
}
