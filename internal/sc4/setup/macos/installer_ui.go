package macos

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type macInstalledInterface struct {
	bundle   string
	packages []string
}

func macCursorPackage(path string) bool {
	switch filepath.Base(path) {
	case "cursors-hd.dat", "cursors-hd-x4.dat":
		return true
	}
	return false
}

func macCursorAssetScale(packages []string) int {
	if hasMacPackage(packages, "cursors-hd-x4.dat") {
		return 4
	}
	return 2
}

func macInactiveCursor(bundle string) string {
	return filepath.Join(bundle, "Contents", "Resources", "SimCity4HD-InactiveCursor")
}

func planMacInstalledInterface(bundle string, current macSetupRecord, sources macSetupSources) (macInstalledInterface, error) {
	old, desired := current.Build, sources.Build
	if old == nil || !old.Interface || !desired.Interface || sources.UI != "" ||
		old.Scale != desired.Scale || old.Language != desired.Language {
		return macInstalledInterface{}, nil
	}
	dir := macModPlugins(bundle)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return macInstalledInterface{}, err
	}
	plan := macInstalledInterface{bundle: bundle}
	for _, entry := range entries {
		name := entry.Name()
		category, _ := macWorldCategory(name, slices.Contains(current.WorldFiles, name))
		if category != "" || !strings.EqualFold(filepath.Ext(name), ".dat") {
			continue
		}
		if !entry.Type().IsRegular() {
			return macInstalledInterface{}, fmt.Errorf("cannot reuse %s because it is not a regular file", name)
		}
		plan.packages = append(plan.packages, filepath.Join(dir, name))
	}
	if !slices.ContainsFunc(plan.packages, macCursorPackage) {
		folder := macInactiveCursor(bundle)
		parked, err := os.ReadDir(folder)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return macInstalledInterface{}, err
		}
		for _, entry := range parked {
			if macCursorPackage(entry.Name()) && entry.Type().IsRegular() {
				plan.packages = append(plan.packages, filepath.Join(folder, entry.Name()))
			}
		}
	}
	if desired.Cursors && !slices.ContainsFunc(plan.packages, macCursorPackage) {
		return macInstalledInterface{}, nil
	}
	scale, err := macHUDScale(plan.packages)
	if err != nil {
		return macInstalledInterface{}, err
	}
	if scale != desired.Scale {
		return macInstalledInterface{}, fmt.Errorf("the installed interface uses texture scale ×%d, expected ×%d", scale, desired.Scale)
	}
	return plan, nil
}
