package sc4

import (
	"bytes"
	"errors"
	"fmt"
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
	case len(args) >= 4 && len(args) <= 5 && args[0] == "stage":
		output = args[3]
		err = stageMacGame(args[1], args[2], output, args[4:])
	case len(args) == 4 && args[0] == "stage-world":
		output = args[3]
		err = stageMacGame(args[1], "", output, args[2:3])
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

func stageMacGame(stock, ui, output string, world []string) error {
	_, mac, err := gameDataDirectory(stock)
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
	var uiPackages, worldPackages []string
	if ui != "" {
		uiPackages, err = packagePaths([]string{filepath.Join(ui, "Plugins")})
		if err != nil {
			return err
		}
	}
	var inputs []string
	for _, build := range world {
		inputs = append(inputs, filepath.Join(build, "Plugins"))
	}
	if len(inputs) > 0 {
		worldPackages, err = packagePaths(inputs)
		if err != nil {
			return err
		}
	}
	worldScale := ""
	for _, path := range worldPackages {
		name := strings.ToLower(filepath.Base(path))
		match := macWorldScale.FindStringSubmatch(name)
		if match == nil {
			return fmt.Errorf("unexpected World package name: %s", filepath.Base(path))
		}
		if worldScale != "" && worldScale != match[1] {
			return errors.New("World packages must use one texture scale")
		}
		worldScale = match[1]
		if err := checkMacWorldPackage(path); err != nil {
			return err
		}
	}
	var patched []byte
	if ui != "" {
		patched, err = macSetupExecutable(stock, ui, uiPackages, worldScale)
	} else {
		var original []byte
		original, err = os.ReadFile(gameExecutablePath(stock, true))
		if err == nil {
			patched, err = patchMachOUIExecutable(original, 0)
		}
	}
	if err != nil {
		return err
	}
	packages := append(uiPackages, worldPackages...)
	if err := verifyPackages(packages, os.Stdout); err != nil {
		return err
	}

	stage, err := os.MkdirTemp(filepath.Dir(output), ".sc4-uhd-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	bundle := filepath.Join(stage, filepath.Base(output))
	if err := runMacSetupTool("/usr/bin/ditto", stock, bundle); err != nil {
		return err
	}
	executable := gameExecutablePath(bundle, true)
	if err := os.WriteFile(executable, patched, 0755); err != nil {
		return err
	}
	plugins := filepath.Join(bundle, "Contents", "GameData", "Plugins", "SimCity4HD")
	if err := os.Mkdir(plugins, 0755); err != nil {
		return err
	}
	for _, path := range packages {
		if err := copyRegular(path, filepath.Join(plugins, filepath.Base(path))); err != nil {
			return err
		}
	}
	if slices.ContainsFunc(uiPackages, func(path string) bool { return filepath.Base(path) == russianTextPackage }) {
		if err := stageMacRussianFonts(bundle, ui); err != nil {
			return err
		}
	}
	// The Aspyr launcher and game executable are separate Mach-O files.
	if err := runMacSetupTool("/usr/bin/codesign", "--force", "--sign", "-", executable); err != nil {
		return err
	}
	steamLibrary := filepath.Join(bundle, "Contents", "MacOS", "libsteam_api.dylib")
	if err := runMacSetupTool("/usr/bin/codesign", "--force", "--sign", "-", steamLibrary); err != nil {
		return err
	}
	if err := runMacSetupTool("/usr/bin/codesign", "--force", "--sign", "-", bundle); err != nil {
		return err
	}
	if err := runMacSetupTool("/usr/bin/codesign", "--verify", "--strict", "--deep", bundle); err != nil {
		return err
	}
	return os.Rename(bundle, output)
}

func stageMacRussianFonts(bundle, ui string) error {
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
		if err := copyRegular(filepath.Join(ui, "Fonts", name), target); err != nil {
			return err
		}
	}
	return nil
}

func checkMacWorldPackage(path string) error {
	archive, err := openArchive(path)
	if err != nil {
		return err
	}
	defer archive.file.Close()
	for _, record := range archive.records {
		switch record.id.kind {
		case fshResource, s3dResource:
		default:
			return fmt.Errorf("%s contains a non-World resource %s", filepath.Base(path), record.id)
		}
	}
	return nil
}

func macSetupExecutable(stock, ui string, packages []string, worldScale string) ([]byte, error) {
	hasHUD := func(name string) bool {
		return slices.ContainsFunc(packages, func(path string) bool { return filepath.Base(path) == name })
	}
	hud2, hud4 := hasHUD("city-hud-x2.dat"), hasHUD("city-hud-x4.dat")
	if hud2 == hud4 {
		return nil, errors.New("the UI build must contain one macOS HUD package")
	}
	scale := 2
	if hud4 {
		scale = 4
	}
	if worldScale != "" && worldScale != strconv.Itoa(scale) {
		return nil, errors.New("UI and World must use the same texture scale")
	}
	original, err := os.ReadFile(gameExecutablePath(stock, true))
	if err != nil {
		return nil, err
	}
	expected, err := patchMachOUIExecutable(original, scale)
	if err != nil {
		return nil, err
	}
	patched, err := os.ReadFile(filepath.Join(ui, macGameExecutable))
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(patched, expected) {
		return nil, errors.New("the UI build does not contain the matching macOS executable")
	}
	return patched, nil
}

func runMacSetupTool(tool string, args ...string) error {
	if output, err := exec.Command(tool, args...).CombinedOutput(); err != nil {
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
