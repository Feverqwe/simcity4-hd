package macos

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/macho"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
)

const macSetupReceipt = "Installed.json"

type macSetupSources struct {
	UI         string         `json:"ui,omitempty"`
	World      string         `json:"world,omitempty"`
	Cache      string         `json:"cache,omitempty"`
	Build      *macSetupBuild `json:"build,omitempty"`
	reusedUI   macInstalledInterface
	world      macWorldPlan
	resolution int

	overrideRegionMap, overrideLoadingPreview, overrideCursors, overrideLanguage bool
}

type macSetupRecord struct {
	macSetupSources
	ExecutableSHA256 string   `json:"executableSHA256"`
	WorldFiles       []string `json:"worldFiles,omitempty"`
}

type macSetupEvent struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	Field    string            `json:"field,omitempty"`
	Path     string            `json:"path,omitempty"`
	Value    *int              `json:"value,omitempty"`
	Total    int               `json:"total,omitempty"`
	Settings *macSetupSettings `json:"settings,omitempty"`
}

func runMacSetup(args []string) int {
	stdout := os.Stdout
	encoder := json.NewEncoder(stdout)
	os.Stdout = os.Stderr
	defer func() { os.Stdout = stdout }()
	send := func(event macSetupEvent) { _ = encoder.Encode(event) }
	fail := func(field string, err error) int {
		send(macSetupEvent{Type: "error", Field: field, Text: err.Error()})
		return 1
	}
	var game string
	var archive string
	var resolution string
	var sources macSetupSources
	var build macSetupBuild
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.StringVar(&game, "game", "", "Steam game bundle or installation folder")
	flags.StringVar(&sources.Cache, "cache", "", "texture cache directory")
	flags.StringVar(&archive, "archive", "", "portable texture archive")
	flags.StringVar(&resolution, "resolution", "", "initial game resolution (auto or WIDTHxHEIGHT)")
	flags.StringVar(&sources.UI, "ui", "", "prepared macOS UI build")
	flags.StringVar(&sources.World, "world", "", "prepared macOS World build")
	build.flags(flags)
	if err := flags.Parse(args[1:]); err != nil {
		return fail("", err)
	}
	if flags.NArg() != 0 {
		return fail("", errors.New("unexpected setup arguments"))
	}
	if args[0] == "import-textures" || args[0] == "export-textures" {
		ctx, cancel, _ := macSetupContext()
		defer cancel()
		if err := transferMacTextures(ctx, args[0], sources.Cache, archive, send); err != nil {
			if errors.Is(err, context.Canceled) {
				send(macSetupEvent{Type: "stopped"})
				return 0
			}
			var input macSetupInputError
			if errors.As(err, &input) {
				return fail(input.field, input.err)
			}
			return fail("", err)
		}
		return 0
	}
	detected := game == ""
	if detected {
		game = findMacSteamFolder()
	}
	bundle := findMacSetupGame(game)
	if bundle == "" {
		if args[0] == "inspect" && detected {
			return 0
		}
		return fail("game", errors.New("Choose the Steam folder containing the original SimCity 4 Deluxe app."))
	}
	bundle, err := filepath.Abs(bundle)
	if err != nil {
		return fail("game", err)
	}
	bundle, err = filepath.EvalSymlinks(bundle)
	if err != nil {
		return fail("game", err)
	}
	if args[0] == "inspect" {
		if detected {
			send(macSetupEvent{Type: "field", Field: "game", Path: game})
		}
		if _, err := os.Lstat(macSetupTransactionDir(bundle)); err == nil {
			return fail("", errors.New("A previous setup operation was interrupted. Close the game and click Install to restore the previous state before continuing."))
		} else if !errors.Is(err, os.ErrNotExist) {
			return fail("", err)
		}
		if current, err := readMacSetupReceipt(bundle); err == nil {
			send(macSetupEvent{Type: "field", Field: "ui", Path: availableMacPreparedUI(bundle, current.UI)})
			send(macSetupEvent{Type: "field", Field: "world", Path: availableMacPreparedBuild(current.World)})
			send(macSetupEvent{Type: "field", Field: "cache", Path: current.Cache})
			settings := current.settings()
			settings.Resolution = installedMacResolution()
			send(macSetupEvent{Type: "installed", Path: bundle, Settings: settings})
		}
		return 0
	}
	if args[0] != "remove" {
		sources.resolution, err = parseMacSetupResolution(resolution)
		if err != nil {
			return fail("resolution", err)
		}
		if err := build.selectSources(flags, &sources); err != nil {
			return fail("", err)
		}
		sources.Cache, err = macTextureCache(sources.Cache)
		if err != nil {
			return fail("cache", err)
		}
		if err := sources.readPreparedSettings(flags); err != nil {
			var input macSetupInputError
			if errors.As(err, &input) {
				return fail(input.field, input.err)
			}
			return fail("", err)
		}
	}
	ctx, cancel, retry := macSetupContext()
	defer cancel()
	if err := changeMacInstallation(ctx, args[0], bundle, sources, send, retry); err != nil {
		if errors.Is(err, context.Canceled) {
			send(macSetupEvent{Type: "stopped"})
			return 0
		}
		var input macSetupInputError
		if errors.As(err, &input) {
			return fail(input.field, err)
		}
		return fail("", err)
	}
	if args[0] == "remove" {
		send(macSetupEvent{Type: "removed"})
	} else {
		send(macSetupEvent{Type: "installed", Path: bundle})
	}
	return 0
}

type macSetupInputError struct {
	field string
	err   error
}

func (e macSetupInputError) Error() string { return e.err.Error() }
func (e macSetupInputError) Unwrap() error { return e.err }

func macSetupContext() (context.Context, context.CancelFunc, func() error) {
	ctx, cancel := context.WithCancel(context.Background())
	retries := make(chan struct{}, 1)
	go func() {
		defer cancel()
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if scanner.Text() == "retry" {
				select {
				case retries <- struct{}{}:
				default:
				}
			}
		}
	}()
	return ctx, cancel, func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-retries:
			return ctx.Err()
		}
	}
}

var macSteamLibrary = regexp.MustCompile(`(?mi)^\s*"path"\s+"((?:[^"\\]|\\.)+)"`)

func findMacSteamFolder() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	steam := filepath.Join(home, "Library", "Application Support", "Steam")
	libraries := []string{steam}
	if data, err := os.ReadFile(filepath.Join(steam, "steamapps", "libraryfolders.vdf")); err == nil {
		for _, match := range macSteamLibrary.FindAllStringSubmatch(string(data), -1) {
			libraries = append(libraries, strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(match[1]))
		}
	}
	for _, library := range libraries {
		game := filepath.Join(library, "steamapps", "common", "SimCity 4 Deluxe")
		if findMacSetupGame(game) != "" {
			return game
		}
	}
	return ""
}

func findMacSetupGame(path string) string {
	if path == "" {
		return ""
	}
	for _, candidate := range []string{path,
		filepath.Join(path, "SimCity™4 Deluxe Edition.app"),
		filepath.Join(path, "Sim City 4 Deluxe Edition.app")} {
		if filepath.Ext(candidate) != ".app" {
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

func macSetupReceiptPath(bundle string) string {
	return filepath.Join(macModPlugins(bundle), macSetupReceipt)
}

func macModPlugins(bundle string) string {
	return filepath.Join(bundle, "Contents", "GameData", "Plugins", "SimCity4HD")
}

func macSetupBackup(bundle string) string {
	return filepath.Join(filepath.Dir(bundle), "."+filepath.Base(bundle)+".sc4-uhd-original")
}

func readMacSetupReceipt(bundle string) (record macSetupRecord, err error) {
	data, err := os.ReadFile(macSetupReceiptPath(bundle))
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, err
	}
	if (record.UI == "" && record.World == "" && record.Build == nil) || record.ExecutableSHA256 == "" {
		return record, errors.New("the macOS mod receipt is incomplete")
	}
	return record, nil
}

func writeMacSetupReceipt(bundle string, sources macSetupSources, world []string) error {
	hash, err := macSetupExecutableHash(bundle)
	if err != nil {
		return err
	}
	record := macSetupRecord{macSetupSources: sources, ExecutableSHA256: hash}
	for _, path := range world {
		record.WorldFiles = append(record.WorldFiles, filepath.Base(path))
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return os.WriteFile(macSetupReceiptPath(bundle), data, 0644)
}

func macSetupExecutableHash(bundle string) (string, error) {
	exe, err := os.ReadFile(gamefiles.ExecutablePath(bundle, true))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(exe)), nil
}

func checkStockMacSetup(bundle string) error {
	hash, err := macSetupExecutableHash(bundle)
	if err != nil {
		return err
	}
	if hash != macho.StockHash {
		return errors.New("Setup requires the original Steam Aspyr 1.2.1 (185949) game executable.")
	}
	return nil
}

func changeMacInstallation(ctx context.Context, action, bundle string, sources macSetupSources, send func(macSetupEvent), retry func() error) error {
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
		return err
	}
	if err := recoverMacSetup(bundle); err != nil {
		return err
	}
	original := bundle
	var current macSetupRecord
	if action == "install" {
		if _, err := os.Lstat(backup); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				return fmt.Errorf("A setup backup already exists at %s. Use Configure or Remove for a managed installation.", backup)
			}
			return err
		}
		for _, path := range []string{macModPlugins(bundle), macInactiveWorld(bundle), macInactiveCursor(bundle)} {
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				if err == nil {
					return fmt.Errorf("An existing mod path needs to be removed before installing: %s", path)
				}
				return err
			}
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
	}
	if err := checkStockMacSetup(original); err != nil {
		return err
	}

	stage, err := prepareMacSetupWork(bundle)
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	staged := filepath.Join(stage, "files")
	if action == "remove" {
		send(macSetupEvent{Type: "step", Text: "Restoring the original Steam files…"})
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
		return applyMacSetupFiles(bundle, stage, staged, "", true, func() error {
			return checkStockMacSetup(bundle)
		})
	}

	var installed string
	if action == "configure" {
		installed = bundle
		var err error
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
	stages := 3 // Check, prepare the patched app, apply.
	if sources.needsInterfaceBuild() {
		stages++
	}
	if sources.world.missing.Any() {
		stages++
	}
	nextStage := setup.SetupStageProgress(stages, func(done, total int) {
		send(macSetupEvent{Type: "overall", Value: &done, Total: total})
	})
	if sources.needsBuild() {
		cache, err := setup.ResolveSetupPath(sources.Cache)
		if err != nil {
			return macSetupInputError{"cache", err}
		}
		if setup.InsideFolder(bundle, cache) || setup.InsideFolder(backup, cache) || setup.InsideFolder(stage, cache) ||
			setup.InsideFolder(macSetupTransactionDir(bundle), cache) {
			return macSetupInputError{"cache", errors.New("Choose a texture cache outside the game and setup work folders.")}
		}
		sources.Cache = cache
	}
	send(macSetupEvent{Type: "step", Text: "Checking available disk space…"})
	if err := checkMacSetupSpace(parent, sources, sources.buildSpace()); err != nil {
		return err
	}
	nextStage()
	prepared := sources
	if sources.needsBuild() {
		work, err := os.MkdirTemp(stage, ".build-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(work)
		prepared, err = prepareMacSetupBuild(ctx, bundle, gamefiles.ExecutablePath(original, true), work, sources, send, retry, nextStage)
		if err != nil {
			return err
		}
		if err := checkMacSetupSpace(parent, prepared, 0); err != nil {
			return err
		}
	}
	send(macSetupEvent{Type: "step", Text: "Checking the texture packages and preparing the Steam patch…"})
	if err := copyMacSetupPaths(ctx, original, staged, macSetupSupportPaths()); err != nil {
		return err
	}
	if err := prepareMacGame(ctx, bundle, gamefiles.ExecutablePath(original, true), prepared.UI, staged, stage, prepared.world, &sources, os.Stderr); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	var stagedBackup string
	if original != filepath.Join(backup, macSetupOriginalFiles) {
		stagedBackup = filepath.Join(stage, "original")
		saved := filepath.Join(stagedBackup, macSetupOriginalFiles)
		if err := copyMacSetupPaths(ctx, original, saved, macSetupOriginalPaths()); err != nil {
			return err
		}
	}
	if err := ensureMacSetupGameClosed(); err != nil {
		return err
	}
	if action == "install" {
		if err := checkStockMacSetup(bundle); err != nil {
			return err
		}
	} else if _, err := checkManagedMacBundle(ctx, bundle); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	nextStage()
	send(macSetupEvent{Type: "step", Text: "Applying changes to the Steam app…"})
	if err := publishMacResolution(sources.resolution, func() error {
		return applyMacSetupFiles(bundle, stage, staged, stagedBackup, stagedBackup != "", func() error {
			return signMacSetupBundle(ctx, bundle)
		})
	}); err != nil {
		return err
	}
	nextStage()
	return nil
}

func checkManagedMacBundle(ctx context.Context, bundle string) (macSetupRecord, error) {
	receipt, err := readMacSetupReceipt(bundle)
	if err != nil {
		return receipt, fmt.Errorf("setup could not identify the installed mod: %w", err)
	}
	err = runMacSetupTool(ctx, "/usr/bin/codesign", "--verify", "--strict", "--deep", bundle)
	if ctx.Err() != nil {
		return receipt, ctx.Err()
	}
	if err != nil {
		return receipt, fmt.Errorf("The patched game has changed since setup installed it. The Steam files and original backup were left in place: %w", err)
	}
	// Steam can restore a signed executable. Do not modify a stock or
	// updated bundle that merely has the setup receipt left in Plugins.
	hash, err := macSetupExecutableHash(bundle)
	if err != nil {
		return receipt, err
	}
	if hash != receipt.ExecutableSHA256 || hash == macho.StockHash {
		return receipt, errors.New("Steam has replaced the patched executable. The Steam files and original backup were left in place.")
	}
	return receipt, nil
}

func ensureMacSetupGameClosed() error {
	err := exec.Command("/usr/bin/pgrep", "-f", "(^|/)Sim City 4 Deluxe Edition(sub)?( |$)").Run()
	var status *exec.ExitError
	switch {
	case err == nil:
		return errors.New("Close SimCity 4 before changing the installation.")
	case errors.As(err, &status) && status.ExitCode() == 1:
		return nil
	default:
		return fmt.Errorf("check whether SimCity 4 is running: %w", err)
	}
}

func checkMacSetupSpace(parent string, sources macSetupSources, buildBytes uint64) error {
	needed := uint64(128<<20) + buildBytes
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
	return checkMacFreeSpace(parent, needed, "the installation")
}
