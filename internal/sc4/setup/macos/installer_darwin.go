package macos

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

	"github.com/Feverqwe/simcity4-hd/internal/sc4/advisors"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/macho"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/russian"
	uibuild "github.com/Feverqwe/simcity4-hd/internal/sc4/ui"
)

var macWorldScale = regexp.MustCompile(`-x([24])(?:-[0-9]+)?\.dat$`)

func prepareMacGame(ctx context.Context, stock, originalExe, ui, bundle, stage string, world macWorldPlan, sources *macSetupSources, diagnostics io.Writer) error {
	gameData, _, err := gamefiles.DataDirectory(stock)
	if err != nil {
		return err
	}
	reusedUI := sources.reusedUI
	uiPackages := reusedUI.packages
	if ui != "" {
		uiPackages, err = gamefiles.PackagePaths([]string{filepath.Join(ui, "Plugins")})
		if err != nil {
			return err
		}
	}
	worldScale, err := macWorldPackageScale(world.active)
	if err != nil {
		return err
	}
	installed := *sources.Build
	var patched []byte
	if ui != "" {
		patched, err = macSetupExecutable(originalExe, ui, uiPackages, worldScale, &installed, sources)
	} else {
		var original []byte
		original, err = os.ReadFile(originalExe)
		if err == nil {
			var scale int
			var options x86.Options
			if reusedUI.bundle != "" {
				scale = installed.Scale
				options = x86.Options{
					HdCursors: installed.Cursors, RegionMap: installed.RegionMap, LoadingPreview: installed.LoadingPreview,
					RussianBudgetTips: installed.Language == "russian", RetinaText: installed.RetinaText,
					CursorScale: macCursorAssetScale(uiPackages),
				}
			}
			patched, err = exepatch.Patch(original, true, scale, options)
		}
	}
	if err != nil {
		return err
	}
	var inactiveCursor []string
	if !installed.Cursors {
		uiPackages = slices.DeleteFunc(uiPackages, func(path string) bool {
			if !macCursorPackage(path) {
				return false
			}
			inactiveCursor = append(inactiveCursor, path)
			return true
		})
	}
	if err := macho.PatchLaunchSettings(patched, installed.skipIntro(), installed.DrawStats); err != nil {
		return err
	}
	if len(uiPackages) != 0 {
		if err := uibuild.RefreshMacKeyboardHelp(ctx, gameData, stage, uiPackages); err != nil {
			return fmt.Errorf("update keyboard shortcuts: %w", err)
		}
	}
	worldPackages, err := prepareMacWorldPackages(ctx, gameData, worldScale, world.active, stage, diagnostics)
	if err != nil {
		return err
	}
	packages := append(uiPackages, worldPackages...)
	if ui != "" {
		advisorOutput := filepath.Join(stage, advisors.AdvisorsPackage)
		if err := advisors.BuildPackage(ctx, stock, advisorOutput); err != nil {
			return fmt.Errorf("prepare advisor models: %w", err)
		}
		packages = append(packages, advisorOutput)
	}
	if len(packages) != 0 {
		if err := gamefiles.VerifyPackages(packages, diagnostics); err != nil {
			return err
		}
	}
	executable := gamefiles.ExecutablePath(bundle, true)
	if err := os.MkdirAll(filepath.Dir(executable), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(executable, patched, 0755); err != nil {
		return err
	}
	plugins := macModPlugins(bundle)
	if err := os.MkdirAll(filepath.Dir(plugins), 0755); err != nil {
		return err
	}
	if err := os.Mkdir(plugins, 0755); err != nil {
		return err
	}
	for _, path := range packages {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := gamefiles.CopyRegular(path, filepath.Join(plugins, filepath.Base(path))); err != nil {
			return err
		}
	}
	if len(world.inactive) != 0 {
		folder := macInactiveWorld(bundle)
		if err := os.MkdirAll(folder, 0755); err != nil {
			return err
		}
		for _, path := range world.inactive {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := gamefiles.CopyRegular(path, filepath.Join(folder, filepath.Base(path))); err != nil {
				return err
			}
		}
	}
	if len(inactiveCursor) != 0 {
		folder := macInactiveCursor(bundle)
		if err := os.MkdirAll(folder, 0755); err != nil {
			return err
		}
		for _, path := range inactiveCursor {
			if err := gamefiles.CopyRegular(path, filepath.Join(folder, filepath.Base(path))); err != nil {
				return err
			}
		}
	}
	if hasMacPackage(uiPackages, uibuild.RussianTextPackage) {
		fontSource := filepath.Join(ui, "Fonts")
		if reusedUI.bundle != "" {
			fontSource = filepath.Join(reusedUI.bundle, "Contents", "GameData", "Fonts")
		}
		if err := stageMacRussianFonts(stock, bundle, fontSource); err != nil {
			return err
		}
	}
	// The Aspyr launcher and game executable are separate Mach-O files.
	if err := runMacSetupTool(ctx, "/usr/bin/codesign", "--force", "--sign", "-", executable); err != nil {
		return err
	}
	receipt := *sources
	receipt.Build = &installed
	if err := writeMacSetupReceipt(bundle, receipt, worldPackages); err != nil {
		return err
	}
	return nil
}

func signMacSetupBundle(ctx context.Context, bundle string) error {
	steamLibrary := filepath.Join(bundle, macSetupSteamLibrary)
	if err := runMacSetupTool(ctx, "/usr/bin/codesign", "--force", "--sign", "-", steamLibrary); err != nil {
		return err
	}
	if err := runMacSetupTool(ctx, "/usr/bin/codesign", "--force", "--sign", "-", bundle); err != nil {
		return err
	}
	if err := runMacSetupTool(ctx, "/usr/bin/codesign", "--verify", "--strict", "--deep", bundle); err != nil {
		return err
	}
	return nil
}

func stageMacRussianFonts(stock, bundle, fontSource string) error {
	fonts := filepath.Join(bundle, "Contents", "GameData", "Fonts")
	if err := os.MkdirAll(fonts, 0755); err != nil {
		return err
	}
	for _, name := range russian.FontFiles {
		target := filepath.Join(fonts, name)
		info, err := os.Lstat(filepath.Join(stock, "Contents", "GameData", "Fonts", name))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular game font", target)
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := gamefiles.CopyRegular(filepath.Join(fontSource, name), target); err != nil {
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

func macSetupExecutable(originalExe, ui string, packages []string, worldScale int, installed *macSetupBuild, sources *macSetupSources) ([]byte, error) {
	scale, err := macHUDScale(packages)
	if err != nil {
		return nil, err
	}
	if worldScale != 0 && worldScale != scale {
		return nil, errors.New("UI and World must use the same texture scale")
	}
	original, err := os.ReadFile(originalExe)
	if err != nil {
		return nil, err
	}
	patched, err := os.ReadFile(filepath.Join(ui, gamefiles.MacGameExecutable))
	if err != nil {
		return nil, err
	}
	if len(patched) > len(original) {
		options := macho.ReadOptions(original, patched)
		options.CursorScale = macCursorAssetScale(packages)
		options.RussianBudgetTips = hasMacPackage(packages, uibuild.RussianTextPackage)
		expected, err := exepatch.Patch(original, true, scale, options)
		if err != nil {
			return nil, err
		}
		matches := bytes.Equal(patched, expected)
		if !matches && options.HdCursors && options.CursorScale == 4 {
			// Earlier prepared ×4 executables used the ×2 point scale.
			legacy := options
			legacy.CursorScale = 2
			old, err := exepatch.Patch(original, true, scale, legacy)
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
				if options.RussianBudgetTips {
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
					options.HdCursors = installed.Cursors
				}
				installed.Cursors = options.HdCursors
				if sources.overrideRegionMap {
					options.RegionMap = installed.RegionMap
				}
				if sources.overrideLoadingPreview {
					options.LoadingPreview = installed.LoadingPreview
				}
				options.RetinaText = installed.RetinaText
				installed.RegionMap = options.RegionMap
				installed.LoadingPreview = options.LoadingPreview
				if options != preparedOptions {
					return exepatch.Patch(original, true, scale, options)
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
