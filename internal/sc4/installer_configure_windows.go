package sc4

import (
	"bytes"
	"context"
	"debug/pe"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const inactiveWorldFolder = ".simcity4hd-world"
const installedCacheFile = "CachePath.txt"
const installedFilesFile = "InstalledFiles.txt"
const installedWorldSelectionFile = "WorldSelection.txt"
const installedSettingsChangesFile = "SettingsChanges.json"

var worldOptionIDs = []string{"network", "terrain", "models"}

type installedSettings struct {
	Options    map[string]bool   `json:"options"`
	Choices    map[string]string `json:"choices,omitempty"`
	Resolution string            `json:"resolution,omitempty"`
}

func worldPackage(name string) (option, scale string) {
	base, ok := strings.CutSuffix(name, ".dat")
	if !ok {
		return "", ""
	}
	marker := strings.LastIndex(base, "-x")
	if marker < 0 {
		return "", ""
	}
	scale, _, _ = strings.Cut(base[marker+2:], "-")
	if scale != "2" && scale != "4" {
		return "", ""
	}
	switch prefix := base[:marker]; {
	case prefix == "network":
		return "network", scale
	case prefix == "terrain-indexed", strings.HasPrefix(prefix, "world-terrain-"):
		return "terrain", scale
	case prefix == "model-overrides", strings.HasPrefix(prefix, "world-"), strings.HasPrefix(prefix, "model-materials"):
		return "models", scale
	}
	return "", ""
}

func isInterfacePackage(name string) bool {
	switch name {
	case "tutorial-text-size-7.dat", "credits-text.dat", "cursors-hd.dat", "cursors-hd-x4.dat":
		return true
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

// Inspect the patch sites used by installers that wrote only an executable hash.
func installedInterface(executable []byte, cursors bool) (uiExecutableOptions, error) {
	file, err := pe.NewFile(bytes.NewReader(executable))
	if err != nil {
		return uiExecutableOptions{}, err
	}
	defer file.Close()
	text := file.Section(".text")
	if text == nil {
		return uiExecutableOptions{}, errors.New("the installed executable is missing its code section")
	}
	data, err := text.Data()
	if err != nil {
		return uiExecutableOptions{}, err
	}
	at := func(rva uint32, length int) []byte {
		if rva < text.VirtualAddress {
			return nil
		}
		offset := int(rva - text.VirtualAddress)
		if offset > len(data)-length {
			return nil
		}
		return data[offset : offset+length]
	}
	preview := at(0x377149, 5)
	return uiExecutableOptions{
		hdCursors: cursors,
		regionMap: bytes.Equal(at(0x3ae6ae, 1), []byte{0xe9}),
		loadingPreview: len(preview) == 5 && preview[0] == 0xe8 &&
			!bytes.Equal(preview, relativeInstruction(0xe8, 0x377149, 0x1dda40)),
	}, nil
}

func readInstalledSettings(game string, executable []byte) (installedSettings, uiExecutableOptions, error) {
	plugins := filepath.Join(game, "Plugins")
	active := filepath.Join(plugins, modFolder)
	entries, err := os.ReadDir(active)
	if err != nil {
		return installedSettings{}, uiExecutableOptions{}, err
	}
	settings := installedSettings{Options: make(map[string]bool)}
	settings.Options["ui"] = executableHash(executable) != stockEXEHash
	settings.Options["world"] = false
	var ui uiExecutableOptions
	if settings.Options["ui"] {
		ui, err = installedInterface(executable, fileExists(filepath.Join(active, "cursors-hd.dat")) ||
			fileExists(filepath.Join(active, "cursors-hd-x4.dat")))
		if err != nil {
			return installedSettings{}, uiExecutableOptions{}, err
		}
	}
	if settings.Options["ui"] {
		settings.Options["hd-cursors"] = ui.hdCursors
		settings.Options["region-map"] = ui.regionMap
		settings.Options["loading-preview"] = ui.loadingPreview
		scale := "4"
		if fileExists(filepath.Join(active, "city-hud-x2.dat")) {
			scale = "2"
		}
		settings.Choices = map[string]string{"texture-scale": scale}
	}
	worldScale := ""
	for _, entry := range entries {
		option, scale := worldPackage(entry.Name())
		if entry.Type().IsRegular() && option != "" {
			settings.Options["world"] = true
			settings.Options[option] = true
			worldScale = scale
		}
	}
	if !settings.Options["world"] {
		parked, err := os.ReadDir(filepath.Join(game, inactiveWorldFolder))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return installedSettings{}, uiExecutableOptions{}, err
		}
		previous, err := os.ReadFile(filepath.Join(active, installedWorldSelectionFile))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return installedSettings{}, uiExecutableOptions{}, err
		}
		selected := strings.Fields(string(previous))
		for _, entry := range parked {
			option, scale := worldPackage(entry.Name())
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
		if settings.Choices == nil {
			settings.Choices = map[string]string{"texture-scale": worldScale}
		}
	}
	graphics, err := os.ReadFile(filepath.Join(plugins, graphicsINI))
	if err != nil {
		return installedSettings{}, uiExecutableOptions{}, err
	}
	renderer, err := os.ReadFile(filepath.Join(plugins, rendererINI))
	if err != nil {
		return installedSettings{}, uiExecutableOptions{}, err
	}
	width := setupINIValue(string(graphics), "GraphicsOptions", "WindowWidth")
	height := setupINIValue(string(graphics), "GraphicsOptions", "WindowHeight")
	if resolutionPattern.MatchString(width + "x" + height) {
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

func reportInstalled(game string, emit func(installEvent)) error {
	executable, err := os.ReadFile(filepath.Join(game, "Apps", gameEXE))
	if err != nil {
		return err
	}
	settings, _, err := readInstalledSettings(game, executable)
	if err != nil {
		return err
	}
	cache, err := os.ReadFile(filepath.Join(game, "Plugins", modFolder, installedCacheFile))
	if err == nil && filepath.IsAbs(string(cache)) {
		emit(installEvent{Type: "field", Field: "cache", Path: string(cache)})
	}
	emit(installEvent{Type: "installed", Settings: &settings})
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

func (changes setupINIChanges) edit(name, config string, options setupOptions, install, track bool) string {
	stats := "off"
	if options.drawStats {
		stats = "on"
	}
	settings := []setupINISetting{{Section: "SCD3D11", Key: "DrawStats", Applied: stats}}
	if name == graphicsINI {
		settings = []setupINISetting{
			{Section: "GraphicsOptions", Key: "WindowWidth", Applied: strconv.Itoa(options.width)},
			{Section: "GraphicsOptions", Key: "WindowHeight", Applied: strconv.Itoa(options.height)},
			{Section: "GraphicsOptions", Key: "EnableIntroVideo", Applied: strconv.FormatBool(!options.skipIntro)},
		}
		for _, setting := range []setupINISetting{
			{Section: "GraphicsOptions", Key: "Driver", Applied: "DirectX"},
			{Section: "GraphicsOptions", Key: "ColorDepth", Applied: "32"},
			{Section: "GraphicsOptions", Key: "WindowMode", Applied: "FullScreen"},
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

func configureHD(ctx context.Context, game, cache string, desired setupOptions, emit func(installEvent)) (result error) {
	applying := false
	defer func() {
		if !applying && result != nil && ctx.Err() != nil {
			result = ctx.Err()
		}
	}()
	if !filepath.IsAbs(game) {
		return errors.New("The installed game folder must be an absolute path.")
	}
	game, err := filepath.EvalSymlinks(game)
	if err != nil {
		return err
	}
	unlock, err := installLock(game)
	if err != nil {
		return err
	}
	defer unlock()
	if !modInstalled(game) {
		return errors.New("The SimCity 4 UHD installation is incomplete or has changed. Restore the Steam game before continuing.")
	}
	if err := ensureGameNotRunning(); err != nil {
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
	scale := strconv.Itoa(desired.textureScale)
	cursorScaleChanged := desired.interfaceOptions.hdCursors &&
		fileExists(filepath.Join(game, "Plugins", modFolder, "cursors-hd-x4.dat")) != (scale == "4")
	rebuildUI := desired.ui && (!settings.Options["ui"] || desired.interfaceOptions != oldUI ||
		scale != settings.Choices["texture-scale"] || cursorScaleChanged)
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
		if executableHash(stock) != stockEXEHash {
			return errors.New("The original game executable backup does not match Steam build 4644414.")
		}
	}

	plugins := filepath.Join(game, "Plugins")
	active := filepath.Join(plugins, modFolder)
	inactive := filepath.Join(game, inactiveWorldFolder)
	activeFiles, err := os.ReadDir(active)
	if err != nil {
		return err
	}
	inactiveFiles, err := os.ReadDir(inactive)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	installedFiles, err := readInstalledFiles(active)
	if err != nil {
		return err
	}
	iniChanges, err := loadSetupINIChanges(active)
	if err != nil {
		return err
	}
	worldOptions := map[string]bool{"network": desired.world.network, "terrain": desired.world.terrain,
		"models": desired.world.models}
	available := make(map[string]bool)
	for _, files := range [][]os.DirEntry{activeFiles, inactiveFiles} {
		for _, entry := range files {
			option, packageScale := worldPackage(entry.Name())
			if option != "" && packageScale == scale && entry.Type().IsRegular() {
				available[option] = true
			}
		}
	}
	missing := worldSelection{
		network: desired.world.network && !available["network"],
		terrain: desired.world.terrain && !available["terrain"],
		models:  desired.world.models && !available["models"],
	}
	cache, err = resolveSetupPath(cache)
	if err != nil {
		return setupInputError{"cache", fmt.Sprintf("Could not open the texture cache: %v", err)}
	}
	if insideFolder(game, cache) || insideFolder(cache, game) {
		return setupInputError{"cache", "Choose a dedicated texture cache outside the game folder."}
	}
	if rebuildUI || missing.any() {
		dependencies := filepath.Join(cache, "SetupDependencies")
		emit(installEvent{Type: "step", Text: "Checking available disk space"})
		required := desired
		required.ui, required.world = rebuildUI, missing
		if err := checkSetupSpace(ctx, game, dependencies, cache, required); err != nil {
			return err
		}
		unlockCache, err := installLock(cache)
		if err != nil {
			return err
		}
		defer unlockCache()
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
	newActive, newInactive := filepath.Join(work, "active"), filepath.Join(work, "inactive")
	for _, folder := range []string{newActive, newInactive} {
		if err := os.Mkdir(folder, 0755); err != nil {
			return err
		}
	}
	rememberWorld := settings.Options["world"] && !desired.world.any()
	if rememberWorld {
		selected := []string{"scale=" + settings.Choices["texture-scale"]}
		for _, option := range worldOptionIDs {
			if settings.Options[option] {
				selected = append(selected, option)
			}
		}
		if err := os.WriteFile(filepath.Join(newActive, installedWorldSelectionFile), []byte(strings.Join(selected, "\n")), 0644); err != nil {
			return err
		}
	}
	emit(installEvent{Type: "step", Text: "Preparing changes to the installed mod"})
	for _, group := range []struct {
		directory string
		entries   []os.DirEntry
	}{{active, activeFiles}, {inactive, inactiveFiles}} {
		for _, entry := range group.entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := entry.Name()
			target := newActive
			option, packageScale := worldPackage(name)
			switch {
			case option != "":
				if packageScale != scale || !worldOptions[option] {
					target = newInactive
				}
			case group.directory == inactive:
				target = newInactive
			case name == modReceipt || name == installedCacheFile || name == installedSettingsChangesFile ||
				rememberWorld && name == installedWorldSelectionFile:
				continue
			case isInterfacePackage(name) && (!desired.ui || rebuildUI):
				continue
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("Cannot preserve %s because it is not a regular file.", filepath.Join(group.directory, name))
			}
			if err := os.Link(filepath.Join(group.directory, name), filepath.Join(target, name)); err != nil {
				return err
			}
		}
	}

	phase := 1
	deps := setupDependencies{ctx: ctx, dir: filepath.Join(cache, "SetupDependencies"), emit: emit}
	progress := installerBuildProgress(&deps, &phase)
	uiStage, worldStage := filepath.Join(work, "UI"), filepath.Join(work, "World")
	var generated []string
	if rebuildUI {
		if err := buildUIFromExecutable(ctx, setupUIArgs(desired, game, cache, uiStage), progress, bytes.Clone(stock)); err != nil {
			return err
		}
		generated = append(generated, filepath.Join(uiStage, "Plugins"))
	}
	phase = 2
	if missing.any() {
		if err := buildWorldWithProgress(ctx, setupWorldArgs(desired.textureScale, game, cache, worldStage), progress, missing); err != nil {
			return err
		}
		generated = append(generated, filepath.Join(worldStage, "Plugins"))
	}
	if len(generated) > 0 {
		packages, err := packagePaths(generated)
		if err != nil {
			return err
		}
		for _, path := range packages {
			if err := moveSetupPath(path, filepath.Join(newActive, filepath.Base(path)), false); err != nil {
				return err
			}
		}
		packages, err = packagePaths([]string{newActive})
		if err != nil {
			return err
		}
		if err := verifyPackages(packages, os.Stderr); err != nil {
			return err
		}
	}
	replacements := []setupReplacement{{newActive, active}}
	parked, err := os.ReadDir(newInactive)
	if err != nil {
		return err
	}
	if len(parked) > 0 || inactiveFiles != nil {
		replacements = append(replacements, setupReplacement{newInactive, inactive})
	}
	finalEXE := current
	if replaceEXE {
		stagedEXE := filepath.Join(work, gameEXE)
		if rebuildUI {
			stagedEXE = filepath.Join(uiStage, "SimCity4-hd.exe")
			finalEXE, err = os.ReadFile(stagedEXE)
		} else {
			finalEXE = stock
			err = os.WriteFile(stagedEXE, finalEXE, 0644)
		}
		if err != nil {
			return err
		}
		if !settings.Options["ui"] {
			backup := filepath.Join(game, "Apps", stockBackup)
			existing, err := os.ReadFile(backup)
			if errors.Is(err, os.ErrNotExist) {
				stagedBackup := filepath.Join(work, stockBackup)
				if err := os.WriteFile(stagedBackup, stock, 0644); err != nil {
					return err
				}
				replacements = append(replacements, setupReplacement{stagedBackup, backup})
			} else if err != nil {
				return err
			} else if !bytes.Equal(existing, stock) {
				return errors.New("The existing executable backup does not match the original game.")
			}
		}
		replacements = append(replacements, setupReplacement{stagedEXE, exe})
	}
	if err := os.WriteFile(filepath.Join(newActive, modReceipt), []byte(executableHash(finalEXE)), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(newActive, installedCacheFile), []byte(cache), 0644); err != nil {
		return err
	}
	for _, name := range []string{graphicsINI, rendererINI} {
		target := filepath.Join(plugins, name)
		old, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		updated := iniChanges.edit(name, string(old), desired, false, !slices.Contains(installedFiles, name))
		if updated == string(old) {
			continue
		}
		staged := filepath.Join(work, name)
		if err := os.WriteFile(staged, []byte(updated), 0644); err != nil {
			return err
		}
		replacements = append(replacements, setupReplacement{staged, target})
	}
	if err := iniChanges.save(newActive); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ensureGameNotRunning(); err != nil {
		return err
	}
	observed, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	if !bytes.Equal(observed, current) {
		return errors.New("The game executable changed while setup was running. Try again.")
	}
	emit(installEvent{Type: "step", Text: "Applying changes to the installed mod"})
	applying = true
	return applySetupReplacements(work, replacements, &keepWork)
}

func removeHD(game string) error {
	if !filepath.IsAbs(game) {
		return errors.New("The installed game folder must be an absolute path.")
	}
	game, err := filepath.EvalSymlinks(game)
	if err != nil {
		return err
	}
	unlock, err := installLock(game)
	if err != nil {
		return err
	}
	defer unlock()
	if !modInstalled(game) {
		return errors.New("The SimCity 4 UHD installation is incomplete or has changed. Restore the Steam game before continuing.")
	}
	if err := ensureGameNotRunning(); err != nil {
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
	if executableHash(current) != stockEXEHash {
		stock, err := os.ReadFile(filepath.Join(game, "Apps", stockBackup))
		if err != nil {
			return fmt.Errorf("read the original game executable backup: %w", err)
		}
		if executableHash(stock) != stockEXEHash {
			return errors.New("The original game executable backup does not match Steam build 4644414.")
		}
		staged := filepath.Join(work, gameEXE)
		if err := os.WriteFile(staged, stock, 0644); err != nil {
			return err
		}
		changes = append(changes, setupReplacement{staged, exe})
	}
	plugins := filepath.Join(game, "Plugins")
	active := filepath.Join(plugins, modFolder)
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
	changes = append(changes, setupReplacement{target: active}, setupReplacement{target: filepath.Join(game, inactiveWorldFolder)})
	return applySetupReplacements(work, changes, &keepWork)
}
