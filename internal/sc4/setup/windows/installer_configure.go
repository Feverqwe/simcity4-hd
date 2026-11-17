//go:build windows || darwin

package windows

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/advisors"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/pe"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/russian"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
	uibuild "github.com/Feverqwe/simcity4-hd/internal/sc4/ui"
	worldbuild "github.com/Feverqwe/simcity4-hd/internal/sc4/world"
)

const inactiveWorldFolder = ".simcity4hd-world"
const installedCacheFile = "CachePath.txt"
const installedKeepImagesFile = "KeepUpscaledImages"
const installedFilesFile = "InstalledFiles.txt"
const installedWorldSelectionFile = "WorldSelection.txt"
const installedSettingsChangesFile = "SettingsChanges.json"

var worldOptionIDs = []string{"network", "terrain", "models"}

func isInterfacePackage(name string) bool {
	switch name {
	case "tutorial-text-size-7.dat", "credits-text.dat", "cursors-hd.dat", "cursors-hd-x4.dat",
		uibuild.RussianTextPackage, installedRussianPack:
		return true
	}
	for _, font := range russian.FontFiles {
		if name == originalRussianFont(font) {
			return true
		}
	}
	base, found := strings.CutSuffix(name, "-x4.dat")
	if !found {
		base, found = strings.CutSuffix(name, "-x2.dat")
	}
	if !found {
		return false
	}
	switch base {
	case "city-hud", "catalog-square", "catalog-bridge", "catalog-chrome", "disasters-background", "mysim-portraits":
		return true
	}
	return false
}

func readInstalledSettings(game string, executable []byte) (setup.Settings, x86.Options, error) {
	plugins := filepath.Join(game, "Plugins")
	active := filepath.Join(plugins, modFolder)
	entries, err := os.ReadDir(active)
	if err != nil {
		return setup.Settings{}, x86.Options{}, err
	}
	settings := setup.Settings{
		Options: make(map[string]bool),
		Choices: map[string]string{"language": "game"},
	}
	settings.Options["ui"] = executableHash(executable) != pe.StockHash
	settings.Options["world"] = false
	settings.Options["keep-upscaled-images"] = setup.IsRegularFile(filepath.Join(active, installedKeepImagesFile))
	var ui x86.Options
	if settings.Options["ui"] {
		ui, err = pe.ReadOptions(executable, setup.IsRegularFile(filepath.Join(active, "cursors-hd.dat")) ||
			setup.IsRegularFile(filepath.Join(active, "cursors-hd-x4.dat")))
		if err != nil {
			return setup.Settings{}, x86.Options{}, err
		}
	}
	if settings.Options["ui"] {
		settings.Options["hd-cursors"] = ui.HdCursors
		settings.Options["region-map"] = ui.RegionMap
		settings.Options["loading-preview"] = ui.LoadingPreview
		scale := "4"
		if setup.IsRegularFile(filepath.Join(active, "city-hud-x2.dat")) {
			scale = "2"
		}
		settings.Choices["texture-scale"] = scale
	}
	if hasInstalledRussian(active) {
		settings.Choices["language"] = "russian"
	}
	worldScale := ""
	for _, entry := range entries {
		option, scale := worldbuild.Package(entry.Name())
		if entry.Type().IsRegular() && option != "" {
			settings.Options["world"] = true
			settings.Options[option] = true
			worldScale = scale
		}
	}
	if !settings.Options["world"] {
		parked, err := os.ReadDir(filepath.Join(game, inactiveWorldFolder))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return setup.Settings{}, x86.Options{}, err
		}
		previous, err := os.ReadFile(filepath.Join(active, installedWorldSelectionFile))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return setup.Settings{}, x86.Options{}, err
		}
		selected := strings.Fields(string(previous))
		for _, entry := range parked {
			option, scale := worldbuild.Package(entry.Name())
			if entry.Type().IsRegular() && option != "" &&
				(len(selected) == 0 || slices.Contains(selected, option)) {
				settings.Options[option] = true
				worldScale = scale
			}
		}
		for _, scale := range []string{"2", "4"} {
			if slices.Contains(selected, "scale="+scale) {
				worldScale = scale
			}
		}
	}
	if worldScale != "" {
		for _, option := range worldOptionIDs {
			if _, found := settings.Options[option]; !found {
				settings.Options[option] = false
			}
		}
		if settings.Choices["texture-scale"] == "" {
			settings.Choices["texture-scale"] = worldScale
		}
	}
	graphics, err := os.ReadFile(filepath.Join(plugins, graphicsINI))
	if err != nil {
		return setup.Settings{}, x86.Options{}, err
	}
	renderer, err := os.ReadFile(filepath.Join(plugins, rendererINI))
	if err != nil {
		return setup.Settings{}, x86.Options{}, err
	}
	width := setupINIValue(string(graphics), "GraphicsOptions", "WindowWidth")
	height := setupINIValue(string(graphics), "GraphicsOptions", "WindowHeight")
	if isBorderlessMode(setupINIValue(string(graphics), "GraphicsOptions", "WindowMode")) {
		settings.Resolution = "auto"
	} else if gamefiles.ResolutionPattern.MatchString(width + "x" + height) {
		settings.Resolution = width + "x" + height
	}
	settings.Options["skip-intro"] = strings.EqualFold(setupINIValue(string(graphics), "GraphicsOptions", "EnableIntroVideo"), "false")
	switch strings.ToLower(setupINIValue(string(renderer), "SCD3D11", "DrawStats")) {
	case "on", "true", "enabled", "1":
		settings.Options["draw-stats"] = true
	default:
		settings.Options["draw-stats"] = false
	}
	return settings, ui, nil
}

func writeSetupCacheSettings(stage, cache string, keepImages bool) error {
	if err := os.WriteFile(filepath.Join(stage, installedCacheFile), []byte(cache), 0644); err != nil {
		return err
	}
	if keepImages {
		return os.WriteFile(filepath.Join(stage, installedKeepImagesFile), nil, 0644)
	}
	return nil
}

func installedCachePath(game string) string {
	cache, err := os.ReadFile(filepath.Join(game, "Plugins", modFolder, installedCacheFile))
	if err != nil || !filepath.IsAbs(string(cache)) {
		return ""
	}
	return string(cache)
}

func reportInstalled(game string, emit func(setup.Event)) error {
	executable, err := os.ReadFile(filepath.Join(game, "Apps", gameEXE))
	if err != nil {
		return err
	}
	settings, _, err := readInstalledSettings(game, executable)
	if err != nil {
		return err
	}
	if cache := installedCachePath(game); cache != "" {
		emit(setup.Event{Type: "field", Field: "cache", Path: cache})
	}
	event := setup.Event{Type: "installed", Settings: &settings}
	if foreignGame {
		event.Text = "SimCity 4 UHD is installed. Play this copy on Windows."
	}
	emit(event)
	return nil
}

func setupINIValue(config, section, key string) string {
	inSection := section == ""
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSection = section == "" || strings.EqualFold(line, "["+section+"]")
		}
		name, value, found := strings.Cut(line, "=")
		if inSection && found && strings.EqualFold(strings.TrimSpace(name), key) {
			value, _, _ = strings.Cut(value, ";")
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func updateSetupINI(config, section, key, value string) string {
	if strings.EqualFold(setupINIValue(config, section, key), value) {
		return config
	}
	lines := strings.Split(config, "\n")
	lineEnding := ""
	if strings.Contains(config, "\r\n") {
		lineEnding = "\r"
	}
	insertAt := len(lines)
	inSection := section == ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") && section != "" {
			inSection = strings.EqualFold(trimmed, "["+section+"]")
			if inSection {
				insertAt = i + 1
			}
		}
		name, _, found := strings.Cut(trimmed, "=")
		if inSection && found && strings.EqualFold(strings.TrimSpace(name), key) {
			if value == "" {
				return strings.Join(slices.Delete(lines, i, i+1), "\n")
			}
			lines[i] = key + "=" + value + lineEnding
			return strings.Join(lines, "\n")
		}
	}
	line := key + "=" + value + lineEnding + "\n"
	if insertAt == len(lines) {
		if config != "" && !strings.HasSuffix(config, "\n") {
			config += lineEnding + "\n"
		}
		if section != "" {
			config += "[" + section + "]" + lineEnding + "\n"
		}
		return config + line
	}
	return strings.Join(lines[:insertAt], "\n") + "\n" + line + strings.Join(lines[insertAt:], "\n")
}

type setupINISetting struct {
	Section, Key      string
	Original, Applied string
}

// Only track settings changed in INIs which existed before setup installed the mod.
type setupINIChanges map[string][]setupINISetting

func loadSetupINIChanges(active string) (setupINIChanges, error) {
	changes := make(setupINIChanges)
	data, err := os.ReadFile(filepath.Join(active, installedSettingsChangesFile))
	if errors.Is(err, os.ErrNotExist) {
		return changes, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &changes); err != nil {
		return nil, err
	}
	if changes == nil {
		changes = make(setupINIChanges)
	}
	return changes, nil
}

func (changes setupINIChanges) save(stage string) error {
	if len(changes) == 0 {
		return nil
	}
	data, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stage, installedSettingsChangesFile), data, 0644)
}

const borderlessMode = "BorderlessFullScreen"

func isBorderlessMode(mode string) bool {
	return strings.EqualFold(mode, borderlessMode) || strings.EqualFold(mode, "Borderless")
}

func (changes setupINIChanges) edit(name, config string, options setupOptions, install, track bool) string {
	stats := "off"
	if options.drawStats {
		stats = "on"
	}
	settings := []setupINISetting{{Section: "SCD3D11", Key: "DrawStats", Applied: stats}}
	if name == graphicsINI {
		settings = []setupINISetting{
			{Section: "GraphicsOptions", Key: "EnableIntroVideo", Applied: strconv.FormatBool(!options.skipIntro)},
		}
		if !options.keepResolution {
			mode := setupINIValue(config, "GraphicsOptions", "WindowMode")
			if options.width == 0 {
				settings = append(settings, setupINISetting{Section: "GraphicsOptions", Key: "WindowMode", Applied: borderlessMode})
				// The game can switch out of borderless mode without saving a new size.
				width, height, _ := strings.Cut(gamefiles.DefaultResolution, "x")
				for _, setting := range []setupINISetting{
					{Section: "GraphicsOptions", Key: "WindowWidth", Applied: width},
					{Section: "GraphicsOptions", Key: "WindowHeight", Applied: height},
				} {
					if setupINIValue(config, setting.Section, setting.Key) == "" {
						settings = append(settings, setting)
					}
				}
			} else {
				settings = append(settings,
					setupINISetting{Section: "GraphicsOptions", Key: "WindowWidth", Applied: strconv.Itoa(options.width)},
					setupINISetting{Section: "GraphicsOptions", Key: "WindowHeight", Applied: strconv.Itoa(options.height)})
				if install || mode == "" || isBorderlessMode(mode) {
					settings = append(settings, setupINISetting{Section: "GraphicsOptions", Key: "WindowMode", Applied: "FullScreen"})
				}
			}
		}
		for _, setting := range []setupINISetting{
			{Section: "GraphicsOptions", Key: "Driver", Applied: "DirectX"},
			{Section: "GraphicsOptions", Key: "ColorDepth", Applied: "32"},
		} {
			if install || setupINIValue(config, setting.Section, setting.Key) == "" {
				settings = append(settings, setting)
			}
		}
		// SC4GraphicsOptions 1.4.1 aborts parsing when these keys are absent.
		for _, setting := range []setupINISetting{
			{Section: "GraphicsOptions", Key: "PauseGameOnFocusLoss", Applied: "false"},
			{Section: "GraphicsOptions", Key: "ForceDrawOnScroll", Applied: "false"},
		} {
			if setupINIValue(config, setting.Section, setting.Key) == "" {
				settings = append(settings, setting)
			}
		}
	}
	for _, setting := range settings {
		value := setupINIValue(config, setting.Section, setting.Key)
		if strings.EqualFold(value, setting.Applied) {
			continue
		}
		if track {
			setting.Original = value
			previous := changes[name]
			i := slices.IndexFunc(previous, func(old setupINISetting) bool {
				return old.Section == setting.Section && old.Key == setting.Key
			})
			if i < 0 {
				changes[name] = append(previous, setting)
			} else {
				if strings.EqualFold(value, previous[i].Applied) {
					setting.Original = previous[i].Original
				}
				previous[i] = setting
			}
		}
		config = updateSetupINI(config, setting.Section, setting.Key, setting.Applied)
	}
	return config
}

func (changes setupINIChanges) restore(name, config string) string {
	for _, setting := range changes[name] {
		if strings.EqualFold(setupINIValue(config, setting.Section, setting.Key), setting.Applied) {
			config = updateSetupINI(config, setting.Section, setting.Key, setting.Original)
		}
	}
	return config
}

func readInstalledFiles(active string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(active, installedFilesFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return strings.Fields(string(data)), err
}

type setupReplacement struct {
	staged, target string
}

func applySetupReplacements(work string, replacements []setupReplacement, keepWork *bool) (result error) {
	var undo []setupReplacement
	defer func() {
		if result == nil {
			return
		}
		for i := len(undo) - 1; i >= 0; i-- {
			step := undo[i]
			if err := moveSetupPath(step.staged, step.target, false); err != nil {
				*keepWork = true
				result = errors.Join(result, fmt.Errorf("restore %s from %s: %w", step.target, step.staged, err))
			}
		}
	}()
	for i, change := range replacements {
		backup := filepath.Join(work, fmt.Sprintf("backup-%d", i))
		if _, err := os.Lstat(change.target); err == nil {
			if err := moveSetupPath(change.target, backup, false); err != nil {
				return err
			}
			undo = append(undo, setupReplacement{backup, change.target})
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if change.staged == "" {
			continue
		}
		if err := moveSetupPath(change.staged, change.target, false); err != nil {
			return err
		}
		undo = append(undo, setupReplacement{change.target, change.staged})
	}
	return nil
}

func configureHD(ctx context.Context, command setup.Command, emit func(setup.Event), waitForRetry func() error) error {
	game := command.Game
	unlock, err := setup.LockFolder(game)
	if err != nil {
		return err
	}
	defer unlock()
	if !modInstalled(game) {
		return errors.New("The SimCity 4 UHD installation is incomplete or has changed. Restore the Steam game before continuing.")
	}
	if err := ensureGameClosed(); err != nil {
		return err
	}
	exe := filepath.Join(game, "Apps", gameEXE)
	current, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	settings, oldUI, err := readInstalledSettings(game, current)
	if err != nil {
		return err
	}
	desired, err := optionsFor(command.WithInstalledOptions(settings))
	if err != nil {
		return err
	}
	scale := strconv.Itoa(desired.Scale)
	cursorScaleChanged := desired.Features.HdCursors &&
		setup.IsRegularFile(filepath.Join(game, "Plugins", modFolder, "cursors-hd-x4.dat")) != (scale == "4")
	wasRussian := settings.Choices["language"] == "russian"
	rebuildUI := desired.ui && (!settings.Options["ui"] || desired.Features != oldUI ||
		scale != settings.Choices["texture-scale"] || cursorScaleChanged || desired.russian != wasRussian ||
		desired.russian && !setup.IsRegularFile(filepath.Join(game, "Plugins", modFolder, uibuild.RussianTextPackage)))
	replaceEXE := rebuildUI || !desired.ui && settings.Options["ui"]
	var stock []byte
	if replaceEXE {
		stock = current
		if settings.Options["ui"] {
			stock, err = os.ReadFile(filepath.Join(game, "Apps", stockBackup))
			if err != nil {
				return fmt.Errorf("read the original game executable backup: %w", err)
			}
		}
		if executableHash(stock) != pe.StockHash {
			return errors.New("The original game executable backup does not match Steam build 4644414.")
		}
	}

	plugins := filepath.Join(game, "Plugins")
	active := filepath.Join(plugins, modFolder)
	inactive := filepath.Join(game, inactiveWorldFolder)
	installedCache := installedCachePath(game)
	if err := setup.ProtectInstalledCache(installedCache, active, inactive); err != nil {
		return err
	}
	activeFiles, err := os.ReadDir(active)
	if err != nil {
		return err
	}
	inactiveFiles, err := os.ReadDir(inactive)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	rebuildAdvisors := desired.ui && !setupAdvisorsCurrent(ctx, active)
	installedFiles, err := readInstalledFiles(active)
	if err != nil {
		return err
	}
	iniChanges, err := loadSetupINIChanges(active)
	if err != nil {
		return err
	}
	worldOptions := map[string]bool{"network": desired.world.Network, "terrain": desired.world.Terrain,
		"models": desired.world.Models}
	available := make(map[string]bool)
	for _, files := range [][]os.DirEntry{activeFiles, inactiveFiles} {
		for _, entry := range files {
			option, packageScale := worldbuild.Package(entry.Name())
			if option != "" && packageScale == scale && entry.Type().IsRegular() {
				available[option] = true
			}
		}
	}
	missing := worldbuild.Selection{
		Network: desired.world.Network && !available["network"],
		Terrain: desired.world.Terrain && !available["terrain"],
		Models:  desired.world.Models && !available["models"],
	}
	cache, err := ResolveInstallCache(command.Cache, installedCache, game)
	if err != nil {
		return err
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
		BuildOptions: desired.BuildOptions, Game: game, Cache: cache, Work: work,
		Interface: rebuildUI, Russian: desired.russian, World: missing, StockExecutable: bytes.Clone(stock),
	}
	previousRussian := ""
	if wasRussian {
		previousRussian = active
		build.RussianPack = filepath.Join(active, installedRussianPack)
	}
	plan := setup.InstallationPlan{
		Textures:   build,
		CheckSpace: func() error { return checkBuildSpace(ctx, build, false) },
		Stage: func(outputs setup.TextureOutputs, deps *setup.Resources) (func(context.Context) error, error) {
			newActive, newInactive := filepath.Join(work, "active"), filepath.Join(work, "inactive")
			for _, folder := range []string{newActive, newInactive} {
				if err := os.Mkdir(folder, 0755); err != nil {
					return nil, err
				}
			}
			rememberWorld := settings.Options["world"] && !desired.world.Any()
			if rememberWorld {
				selected := []string{"scale=" + settings.Choices["texture-scale"]}
				for _, option := range worldOptionIDs {
					if settings.Options[option] {
						selected = append(selected, option)
					}
				}
				if err := os.WriteFile(filepath.Join(newActive, installedWorldSelectionFile), []byte(strings.Join(selected, "\n")), 0644); err != nil {
					return nil, err
				}
			}
			emit(setup.Event{Type: "step", Text: "Preparing changes to the installed mod"})
			for _, group := range []struct {
				directory string
				entries   []os.DirEntry
			}{{active, activeFiles}, {inactive, inactiveFiles}} {
				for _, entry := range group.entries {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
					name := entry.Name()
					target := newActive
					option, packageScale := worldbuild.Package(name)
					switch {
					case option != "":
						if packageScale != scale || !worldOptions[option] {
							target = newInactive
						}
					case group.directory == inactive:
						target = newInactive
					case name == modReceipt || name == installedCacheFile || name == installedKeepImagesFile || name == installedSettingsChangesFile ||
						rememberWorld && name == installedWorldSelectionFile:
						continue
					case name == advisors.AdvisorsPackage || name == installedAdvisorsState:
						if !desired.ui || rebuildAdvisors {
							continue
						}
					case isInterfacePackage(name) && (!desired.ui || rebuildUI):
						continue
					}
					if !entry.Type().IsRegular() {
						return nil, fmt.Errorf("Cannot preserve %s because it is not a regular file.", filepath.Join(group.directory, name))
					}
					if err := linkSetupFile(filepath.Join(group.directory, name), filepath.Join(target, name)); err != nil {
						return nil, err
					}
				}
			}

			generated := outputs.PackageDirs()
			deps.Step(2, "Checking changes to the installed mod")
			if rebuildAdvisors {
				deps.Step(2, "Preparing the advisors")
				if err := stageSetupAdvisors(ctx, game, newActive); err != nil {
					return nil, err
				}
			}
			if len(generated) > 0 {
				packages, err := gamefiles.PackagePaths(generated)
				if err != nil {
					return nil, err
				}
				for _, path := range packages {
					if err := moveSetupPath(path, filepath.Join(newActive, filepath.Base(path)), false); err != nil {
						return nil, err
					}
				}
			}
			if len(generated) > 0 || rebuildAdvisors {
				packages, err := gamefiles.PackagePaths([]string{newActive})
				if err != nil {
					return nil, err
				}
				if err := gamefiles.VerifyPackages(packages, os.Stderr); err != nil {
					return nil, err
				}
			}
			replacements := []setupReplacement{{newActive, active}}
			if wasRussian != desired.russian || desired.russian && rebuildUI {
				fontChanges, err := stageSetupRussianFonts(game, work, previousRussian, newActive, outputs.RussianSource, outputs.Interface)
				if err != nil {
					return nil, err
				}
				replacements = append(replacements, fontChanges...)
			}
			parked, err := os.ReadDir(newInactive)
			if err != nil {
				return nil, err
			}
			if len(parked) > 0 || inactiveFiles != nil {
				replacements = append(replacements, setupReplacement{newInactive, inactive})
			}
			finalEXE := current
			if replaceEXE {
				stagedEXE := filepath.Join(work, gameEXE)
				if rebuildUI {
					stagedEXE = filepath.Join(outputs.Interface, "SimCity4-hd.exe")
					finalEXE, err = os.ReadFile(stagedEXE)
				} else {
					finalEXE = stock
					err = os.WriteFile(stagedEXE, finalEXE, 0644)
				}
				if err != nil {
					return nil, err
				}
				if !settings.Options["ui"] {
					backup := filepath.Join(game, "Apps", stockBackup)
					existing, err := os.ReadFile(backup)
					if errors.Is(err, os.ErrNotExist) {
						stagedBackup := filepath.Join(work, stockBackup)
						if err := os.WriteFile(stagedBackup, stock, 0644); err != nil {
							return nil, err
						}
						replacements = append(replacements, setupReplacement{stagedBackup, backup})
					} else if err != nil {
						return nil, err
					} else if !bytes.Equal(existing, stock) {
						return nil, errors.New("The existing executable backup does not match the original game.")
					}
				}
				replacements = append(replacements, setupReplacement{stagedEXE, exe})
			}
			if err := os.WriteFile(filepath.Join(newActive, modReceipt), []byte(executableHash(finalEXE)), 0644); err != nil {
				return nil, err
			}
			if err := writeSetupCacheSettings(newActive, cache, desired.KeepUpscaledImages); err != nil {
				return nil, err
			}
			for _, name := range []string{graphicsINI, rendererINI} {
				target := filepath.Join(plugins, name)
				old, err := os.ReadFile(target)
				if err != nil {
					return nil, err
				}
				updated := iniChanges.edit(name, string(old), desired, false, !slices.Contains(installedFiles, name))
				if updated == string(old) {
					continue
				}
				staged := filepath.Join(work, name)
				if err := os.WriteFile(staged, []byte(updated), 0644); err != nil {
					return nil, err
				}
				replacements = append(replacements, setupReplacement{staged, target})
			}
			if err := iniChanges.save(newActive); err != nil {
				return nil, err
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := ensureGameClosed(); err != nil {
				return nil, err
			}
			observed, err := os.ReadFile(exe)
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(observed, current) {
				return nil, errors.New("The game executable changed while setup was running. Try again.")
			}
			return func(context.Context) error {
				return applySetupReplacements(work, replacements, &keepWork)
			}, nil
		},
	}
	return plan.Run(ctx, emit, waitForRetry)
}

func removeHD(game, cache string) error {
	unlock, err := setup.LockFolder(game)
	if err != nil {
		return err
	}
	defer unlock()
	if !modInstalled(game) {
		return errors.New("The SimCity 4 UHD installation is incomplete or has changed. Restore the Steam game before continuing.")
	}
	if err := ensureGameClosed(); err != nil {
		return err
	}
	active := filepath.Join(game, "Plugins", modFolder)
	inactive := filepath.Join(game, inactiveWorldFolder)
	if err := setup.ProtectInstalledCache(installedCachePath(game), active, inactive); err != nil {
		return err
	}
	if err := setup.ProtectInstalledCache(cache, active, inactive); err != nil {
		return err
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
	exe := filepath.Join(game, "Apps", gameEXE)
	current, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	var changes []setupReplacement
	if executableHash(current) != pe.StockHash {
		stock, err := os.ReadFile(filepath.Join(game, "Apps", stockBackup))
		if err != nil {
			return fmt.Errorf("read the original game executable backup: %w", err)
		}
		if executableHash(stock) != pe.StockHash {
			return errors.New("The original game executable backup does not match Steam build 4644414.")
		}
		staged := filepath.Join(work, gameEXE)
		if err := os.WriteFile(staged, stock, 0644); err != nil {
			return err
		}
		changes = append(changes, setupReplacement{staged, exe})
	}
	plugins := filepath.Join(game, "Plugins")
	files, err := readInstalledFiles(active)
	if err != nil {
		return err
	}
	for _, name := range files {
		switch name {
		case rendererDLL, rendererINI, graphicsDLL, graphicsINI:
			changes = append(changes, setupReplacement{target: filepath.Join(plugins, name)})
		}
	}
	iniChanges, err := loadSetupINIChanges(active)
	if err != nil {
		return err
	}
	for _, name := range []string{graphicsINI, rendererINI} {
		if len(iniChanges[name]) == 0 || slices.Contains(files, name) {
			continue
		}
		target := filepath.Join(plugins, name)
		old, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		updated := iniChanges.restore(name, string(old))
		if updated == string(old) {
			continue
		}
		staged := filepath.Join(work, name)
		if err := os.WriteFile(staged, []byte(updated), 0644); err != nil {
			return err
		}
		changes = append(changes, setupReplacement{staged, target})
	}
	if hasInstalledRussian(active) {
		fontChanges, err := stageSetupRussianFonts(game, work, active, "", "", "")
		if err != nil {
			return err
		}
		changes = append(changes, fontChanges...)
	}
	changes = append(changes, setupReplacement{target: active}, setupReplacement{target: inactive})
	return applySetupReplacements(work, changes, &keepWork)
}
