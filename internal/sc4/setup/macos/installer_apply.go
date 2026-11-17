//go:build windows || darwin

package macos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
)

func changeMacInstallation(ctx context.Context, command setup.Command, send func(setup.Event), retry func() error) error {
	action, bundle := command.Action, command.Game
	if err := ctx.Err(); err != nil {
		return err
	}
	parent := filepath.Dir(bundle)
	backup := macSetupBackup(bundle)
	unlock, err := lockMacSetup(bundle)
	if err != nil {
		return err
	}
	defer unlock()
	if err := ensureMacSetupGameClosed(); err != nil {
		return err
	}
	if err := validateMacSetupDirectories(bundle); err != nil {
		return setup.FieldError("game", err)
	}
	recovered, err := recoverMacSetup(bundle)
	if err != nil {
		return err
	}
	original := bundle
	var current macSetupRecord
	if action == "install" {
		if recovered {
			if _, err := readMacSetupReceipt(bundle); err == nil {
				send(setup.Event{Type: "recovered"})
				return nil
			}
		}
		if err := checkMacInstallTarget(bundle); err != nil {
			return setup.FieldError("game", err)
		}
	} else {
		original, err = macSetupSavedFiles(bundle)
		if err != nil {
			return err
		}
		current, err = checkManagedMacBundle(ctx, bundle)
		if err != nil {
			return err
		}
		replaced := []string{macModPlugins(bundle), macInactiveWorld(bundle), macInactiveCursor(bundle), macSetupWork(bundle)}
		if action == "remove" {
			replaced = append(replaced, backup)
			if err := setup.ProtectInstalledCache(command.Cache, replaced...); err != nil {
				return err
			}
		}
		if err := setup.ProtectInstalledCache(current.Cache, replaced...); err != nil {
			return err
		}
		if err := checkStockMacSetup(original); err != nil {
			return err
		}
	}

	var sources macSetupSources
	if action != "remove" {
		if action == "configure" {
			command = command.WithInstalledOptions(*current.settings())
		}
		sources, err = macBuildSources(command)
		if err != nil {
			return err
		}
		sources.Cache, err = ResolveInstallCache(command.Cache, current.Cache, bundle)
		if err != nil {
			return err
		}
		if !canSignMacApp {
			sources.PendingResolution = command.Resolution
		}
	}
	stage, err := prepareMacSetupWork(bundle)
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	staged := filepath.Join(stage, "files")
	if action == "remove" {
		send(setup.Event{Type: "step", Text: "Restoring the original Steam files…"})
		if err := copyMacSetupPaths(ctx, original, staged, macSetupOriginalPaths()); err != nil {
			return err
		}
		if err := ensureMacSetupGameClosed(); err != nil {
			return err
		}
		if _, err := checkManagedMacBundle(ctx, bundle); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := applyMacSetupFiles(bundle, stage, staged, "", true, func() error {
			return checkStockMacSetup(bundle)
		}); err != nil {
			return err
		}
		send(setup.Event{Type: "removed"})
		return nil
	}

	var installed string
	if action == "configure" {
		installed = bundle
		var err error
		if current.NeedsSigning && sources.resolution == 0 {
			sources.resolution, err = parseMacSetupResolution(current.PendingResolution)
			if err != nil {
				return setup.FieldError("resolution", err)
			}
		}
		sources.reusedUI, err = planMacInstalledInterface(bundle, current, sources)
		if err != nil {
			return err
		}
		// Qt disables child options while World is off. Keep the previous
		// selection available when the user turns World back on.
		if !sources.Build.World && !sources.Build.worldSelection().Any() && current.Build != nil {
			sources.Build.Network = current.Build.Network
			sources.Build.Terrain = current.Build.Terrain
			sources.Build.Models = current.Build.Models
		}
	}
	if !sources.Build.Interface {
		sources.Build.RetinaText = false
	}
	sources.world, err = planMacWorld(installed, current, *sources.Build, sources.world.active)
	if err != nil {
		return err
	}
	build := sources.textureBuild()
	build.Game, build.Work = bundle, filepath.Join(stage, "build")
	originalExe := gamefiles.ExecutablePath(original, true)
	var generated *setup.TextureBuild
	if build.Interface || build.World.Any() {
		generated = &build
		if err := os.Mkdir(build.Work, 0755); err != nil {
			return err
		}
	}
	if build.Interface {
		build.StockExecutable, err = os.ReadFile(originalExe)
		if err != nil {
			return err
		}
	}

	plan := setup.InstallationPlan{
		Textures:   build,
		CheckSpace: func() error { return checkMacSetupSpace(ctx, parent, sources, generated) },
		Stage: func(outputs setup.TextureOutputs, resources *setup.Resources) (func(context.Context) error, error) {
			prepared := sources
			if outputs.Interface != "" {
				prepared.UI = outputs.Interface
			}
			if outputs.World != "" {
				packages, err := preparedMacPackages(&outputs.World)
				if err != nil {
					return nil, err
				}
				prepared.world.active = append(prepared.world.active, packages...)
			}
			if outputs.Interface != "" || outputs.World != "" {
				if err := checkMacSetupSpace(ctx, parent, prepared, nil); err != nil {
					return nil, err
				}
			}
			resources.Step(3, "Checking the texture packages and preparing the Steam patch…")
			if err := copyMacSetupPaths(ctx, original, staged, macSetupSupportPaths()); err != nil {
				return nil, err
			}
			if err := prepareMacGame(ctx, bundle, originalExe, prepared.UI, staged, stage, prepared.world, &sources, os.Stderr, macSetupSigner()); err != nil {
				return nil, err
			}
			var stagedBackup string
			if original != filepath.Join(backup, macSetupOriginalFiles) {
				stagedBackup = filepath.Join(stage, "original")
				saved := filepath.Join(stagedBackup, macSetupOriginalFiles)
				if err := copyMacSetupPaths(ctx, original, saved, macSetupOriginalPaths()); err != nil {
					return nil, err
				}
			}
			if err := ensureMacSetupGameClosed(); err != nil {
				return nil, err
			}
			if action == "install" {
				if err := checkMacInstallTarget(bundle); err != nil {
					return nil, setup.FieldError("game", err)
				}
			} else if _, err := checkManagedMacBundle(ctx, bundle); err != nil {
				return nil, err
			}
			return func(applyCtx context.Context) error {
				return publishMacResolution(sources.resolution, func() error {
					return applyMacSetupFiles(bundle, stage, staged, stagedBackup, stagedBackup != "", func() error {
						return finalizeMacSetupBundle(applyCtx, bundle)
					})
				})
			}, nil
		},
	}
	if err := plan.Run(ctx, send, retry); err != nil {
		return err
	}
	if !canSignMacApp {
		send(setup.Event{Type: "handoff", Path: parent, Text: fmt.Sprintf(
			"Copy %q and its backup %q to the same folder on your Mac. Open the app in SimCity 4 UHD Setup, click Configure, then Apply changes to sign the game.",
			filepath.Base(bundle), filepath.Base(backup),
		)})
		return nil
	}
	send(setup.Event{Type: "installed", Path: bundle})
	return nil
}

func ResolveInstallCache(cache, installed, bundle string) (string, error) {
	return setup.ResolveCache(cache, installed, bundle, macSetupBackup(bundle), macSetupWork(bundle), macSetupTransactionDir(bundle))
}

func checkMacInstallTarget(bundle string) error {
	for _, path := range []string{macSetupBackup(bundle), macModPlugins(bundle), macInactiveWorld(bundle), macInactiveCursor(bundle)} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				return err
			}
			if !canSignMacApp {
				return fmt.Errorf("An existing macOS installation or backup was found at %s. Open the game in SimCity 4 UHD Setup on your Mac.", path)
			}
			if path == macSetupBackup(bundle) {
				return fmt.Errorf("A setup backup already exists at %s. Use Configure or Remove for a managed installation.", path)
			}
			return fmt.Errorf("An existing mod path needs to be removed before installing: %s", path)
		}
	}
	return checkStockMacSetup(bundle)
}

func checkMacSetupSpace(ctx context.Context, parent string, sources macSetupSources, build *setup.TextureBuild) error {
	needed := uint64(128 << 20)
	if sources.reusedUI.bundle == "" && sources.Build != nil && sources.Build.Interface {
		needed += 130 << 20
	}
	uiPackages := sources.reusedUI.packages
	if sources.UI != "" {
		var err error
		uiPackages, err = gamefiles.PackagePaths([]string{filepath.Join(sources.UI, "Plugins")})
		if err != nil {
			return err
		}
	}
	for _, packages := range [][]string{uiPackages, sources.world.active, sources.world.inactive} {
		for _, path := range packages {
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			needed += uint64(info.Size())
		}
	}
	return checkAspyrSpace(ctx, parent, needed, build)
}
