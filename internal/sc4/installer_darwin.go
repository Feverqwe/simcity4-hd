package sc4

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var macWorldScale = regexp.MustCompile(`-x([24])(?:-[0-9]+)?\.dat$`)

func runInstaller(args []string) int {
	var err error
	var output string
	switch {
	case len(args) > 0 && slices.Contains([]string{"inspect", "install", "configure", "remove", "import-textures", "export-textures"}, args[0]):
		return runMacSetup(args)
	case len(args) >= 4 && len(args) <= 5 && args[0] == "stage":
		output = args[3]
		var world macWorldPlan
		if len(args) == 5 {
			world.active, err = preparedMacPackages(&args[4])
		}
		if err == nil {
			err = stageMacGame(context.Background(), args[1], args[2], output, world, nil, os.Stdout)
		}
	case len(args) == 4 && args[0] == "stage-world":
		output = args[3]
		var world macWorldPlan
		world.active, err = preparedMacPackages(&args[2])
		if err == nil {
			err = stageMacGame(context.Background(), args[1], "", output, world, nil, os.Stdout)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: simcity4-uhd setup stage STOCK.app UI_BUILD NEW_COPY.app [WORLD_BUILD]")
		fmt.Fprintln(os.Stderr, "       simcity4-uhd setup stage-world STOCK.app WORLD_BUILD NEW_COPY.app")
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(output)
	return 0
}

func stageMacGame(ctx context.Context, stock, ui, output string, world macWorldPlan, sources *macSetupSources, diagnostics io.Writer) error {
	gameData, mac, err := gameDataDirectory(stock)
	if err != nil {
		return err
	}
	if !mac || filepath.Ext(output) != ".app" {
		return errors.New("select the original macOS game bundle and a new .app destination")
	}
	lock, err := os.OpenFile(output+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer os.Remove(lock.Name())
	if err := lock.Close(); err != nil {
		return err
	}
	if _, err := os.Lstat(output); err == nil {
		return fmt.Errorf("%s already exists", output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var uiPackages []string
	var reusedUI macInstalledInterface
	if sources != nil {
		reusedUI = sources.reusedUI
		uiPackages = reusedUI.packages
	}
	if ui != "" {
		uiPackages, err = packagePaths([]string{filepath.Join(ui, "Plugins")})
		if err != nil {
			return err
		}
	}
	worldScale, err := macWorldPackageScale(world.active)
	if err != nil {
		return err
	}
	var installed *macSetupBuild
	if sources != nil && sources.Build != nil {
		build := *sources.Build
		installed = &build
	}
	var patched []byte
	if ui != "" {
		patched, err = macSetupExecutable(stock, ui, uiPackages, worldScale, installed, sources)
	} else {
		var original []byte
		original, err = os.ReadFile(gameExecutablePath(stock, true))
		if err == nil {
			var scale int
			var options uiExecutableOptions
			if reusedUI.bundle != "" {
				scale = installed.Scale
				options = uiExecutableOptions{
					hdCursors: installed.Cursors, regionMap: installed.RegionMap, loadingPreview: installed.LoadingPreview,
					russianBudgetTips: installed.Language == "russian", retinaText: installed.RetinaText,
					cursorScale: macCursorAssetScale(uiPackages),
				}
			}
			patched, err = patchGameUIExecutable(original, true, scale, options)
		}
	}
	if err != nil {
		return err
	}
	var inactiveCursor []string
	if installed != nil {
		if !installed.Cursors {
			uiPackages = slices.DeleteFunc(uiPackages, func(path string) bool {
				if !macCursorPackage(path) {
					return false
				}
				inactiveCursor = append(inactiveCursor, path)
				return true
			})
		}
		if err := patchMachOLaunchSettings(patched, installed.skipIntro(), installed.DrawStats); err != nil {
			return err
		}
	}
	stage, err := os.MkdirTemp(filepath.Dir(output), ".sc4-uhd-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if len(uiPackages) != 0 {
		if err := refreshMacKeyboardHelp(ctx, gameData, stage, uiPackages); err != nil {
			return fmt.Errorf("update keyboard shortcuts: %w", err)
		}
	}
	worldPackages, err := prepareMacWorldPackages(ctx, gameData, worldScale, world.active, stage, diagnostics)
	if err != nil {
		return err
	}
	packages := append(uiPackages, worldPackages...)
	if ui != "" {
		advisors := filepath.Join(stage, advisorsPackage)
		if err := buildAdvisorsPackage(ctx, stock, advisors); err != nil {
			return fmt.Errorf("prepare advisor models: %w", err)
		}
		packages = append(packages, advisors)
	}
	if len(packages) != 0 {
		if err := verifyPackages(packages, diagnostics); err != nil {
			return err
		}
	}
	bundle := filepath.Join(stage, filepath.Base(output))
	if err := runMacSetupTool(ctx, "/usr/bin/ditto", stock, bundle); err != nil {
		return err
	}
	executable := gameExecutablePath(bundle, true)
	if err := os.WriteFile(executable, patched, 0755); err != nil {
		return err
	}
	plugins := macModPlugins(bundle)
	if err := os.Mkdir(plugins, 0755); err != nil {
		return err
	}
	for _, path := range packages {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyRegular(path, filepath.Join(plugins, filepath.Base(path))); err != nil {
			return err
		}
	}
	if len(world.inactive) != 0 {
		folder := macInactiveWorld(bundle)
		if err := os.Mkdir(folder, 0755); err != nil {
			return err
		}
		for _, path := range world.inactive {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := copyRegular(path, filepath.Join(folder, filepath.Base(path))); err != nil {
				return err
			}
		}
	}
	if len(inactiveCursor) != 0 {
		folder := macInactiveCursor(bundle)
		if err := os.Mkdir(folder, 0755); err != nil {
			return err
		}
		for _, path := range inactiveCursor {
			if err := copyRegular(path, filepath.Join(folder, filepath.Base(path))); err != nil {
				return err
			}
		}
	}
	if hasMacPackage(uiPackages, russianTextPackage) {
		fontSource := filepath.Join(ui, "Fonts")
		if reusedUI.bundle != "" {
			fontSource = filepath.Join(reusedUI.bundle, "Contents", "GameData", "Fonts")
		}
		if err := stageMacRussianFonts(bundle, fontSource); err != nil {
			return err
		}
	}
	// The Aspyr launcher and game executable are separate Mach-O files.
	if err := runMacSetupTool(ctx, "/usr/bin/codesign", "--force", "--sign", "-", executable); err != nil {
		return err
	}
	if sources != nil {
		receipt := *sources
		receipt.Build = installed
		if err := writeMacSetupReceipt(bundle, receipt, worldPackages); err != nil {
			return err
		}
	}
	steamLibrary := filepath.Join(bundle, "Contents", "MacOS", "libsteam_api.dylib")
	if err := runMacSetupTool(ctx, "/usr/bin/codesign", "--force", "--sign", "-", steamLibrary); err != nil {
		return err
	}
	if err := runMacSetupTool(ctx, "/usr/bin/codesign", "--force", "--sign", "-", bundle); err != nil {
		return err
	}
	if err := runMacSetupTool(ctx, "/usr/bin/codesign", "--verify", "--strict", "--deep", bundle); err != nil {
		return err
	}
	return os.Rename(bundle, output)
}

func stageMacRussianFonts(bundle, fontSource string) error {
	fonts := filepath.Join(bundle, "Contents", "GameData", "Fonts")
	for _, name := range russianFontFiles {
		target := filepath.Join(fonts, name)
		info, err := os.Lstat(target)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular game font", target)
		}
		if err := os.Remove(target); err != nil {
			return err
		}
		if err := copyRegular(filepath.Join(fontSource, name), target); err != nil {
			return err
		}
	}
	return nil
}

func hasMacPackage(packages []string, name string) bool {
	return slices.ContainsFunc(packages, func(path string) bool { return filepath.Base(path) == name })
}

func macHUDScale(packages []string) (int, error) {
	hud2 := hasMacPackage(packages, "city-hud-x2.dat")
	hud4 := hasMacPackage(packages, "city-hud-x4.dat")
	if hud2 == hud4 {
		return 0, errors.New("the UI build must contain one macOS HUD package")
	}
	if hud4 {
		return 4, nil
	}
	return 2, nil
}

func macWorldPackageScale(packages []string) (int, error) {
	var scale int
	for _, path := range packages {
		name := filepath.Base(path)
		match := macWorldScale.FindStringSubmatch(strings.ToLower(name))
		if match == nil {
			return 0, fmt.Errorf("unexpected World package name: %s", name)
		}
		density, _ := strconv.Atoi(match[1])
		if scale != 0 && scale != density {
			return 0, errors.New("World packages must use one texture scale")
		}
		scale = density
	}
	return scale, nil
}

func macSetupExecutable(stock, ui string, packages []string, worldScale int, installed *macSetupBuild, sources *macSetupSources) ([]byte, error) {
	scale, err := macHUDScale(packages)
	if err != nil {
		return nil, err
	}
	if worldScale != 0 && worldScale != scale {
		return nil, errors.New("UI and World must use the same texture scale")
	}
	original, err := os.ReadFile(gameExecutablePath(stock, true))
	if err != nil {
		return nil, err
	}
	patched, err := os.ReadFile(filepath.Join(ui, macGameExecutable))
	if err != nil {
		return nil, err
	}
	if len(patched) > len(original) {
		hasRedirect := func(site int) bool {
			return !bytes.Equal(patched[site:site+5], original[site:site+5])
		}
		options := uiExecutableOptions{
			hdCursors:         hasRedirect(macCursorDirectorySite),
			loadingPreview:    hasRedirect(macLoadingPreviewSite),
			retinaText:        hasRedirect(macRetinaGlyphCacheSite),
			cursorScale:       macCursorAssetScale(packages),
			regionMap:         hasRedirect(macRegionTilesSite),
			russianBudgetTips: hasMacPackage(packages, russianTextPackage),
		}
		expected, err := patchGameUIExecutable(original, true, scale, options)
		if err != nil {
			return nil, err
		}
		matches := bytes.Equal(patched, expected)
		if !matches && options.hdCursors && options.cursorScale == 4 {
			// Earlier prepared ×4 executables used the ×2 point scale.
			legacy := options
			legacy.cursorScale = 2
			old, err := patchGameUIExecutable(original, true, scale, legacy)
			if err != nil {
				return nil, err
			}
			matches = bytes.Equal(patched, old)
		}
		if matches {
			if installed != nil {
				preparedOptions := options
				installed.Scale = scale
				preparedLanguage := "game"
				if options.russianBudgetTips {
					preparedLanguage = "russian"
				}
				if sources.overrideLanguage && installed.Language != preparedLanguage {
					return nil, macSetupInputError{"ui", errors.New("Choose a prepared interface build in the selected language or clear the prepared interface field.")}
				}
				installed.Language = preparedLanguage
				if sources.overrideCursors {
					if installed.Cursors && !slices.ContainsFunc(packages, macCursorPackage) {
						return nil, macSetupInputError{"ui", errors.New("Choose a prepared interface build with HD cursors or clear the prepared interface field.")}
					}
					options.hdCursors = installed.Cursors
				}
				installed.Cursors = options.hdCursors
				if sources.overrideRegionMap {
					options.regionMap = installed.RegionMap
				}
				if sources.overrideLoadingPreview {
					options.loadingPreview = installed.LoadingPreview
				}
				if sources.overrideRetinaText {
					options.retinaText = installed.RetinaText
				}
				installed.RetinaText = options.retinaText
				installed.RegionMap = options.regionMap
				installed.LoadingPreview = options.loadingPreview
				if options != preparedOptions {
					return patchGameUIExecutable(original, true, scale, options)
				}
			}
			return expected, nil
		}
	}
	return nil, errors.New("rebuild the macOS UI package with the current builder and matching Steam executable")
}

func runMacSetupTool(ctx context.Context, tool string, args ...string) error {
	if output, err := exec.CommandContext(ctx, tool, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w\n%s", filepath.Base(tool), err, output)
	}
	return nil
}

func checkGameLaunch(mac bool) error {
	if !mac {
		return errors.New("launch this game on Windows")
	}
	command := exec.Command("/usr/bin/pgrep", "-f", "(^|/)"+macGameExecutable+"( |$)")
	err := command.Run()
	if err == nil {
		return errors.New("close the current SimCity 4 session before continuing")
	}
	var status *exec.ExitError
	if errors.As(err, &status) && status.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("check for a running SimCity 4 session: %w", err)
}
