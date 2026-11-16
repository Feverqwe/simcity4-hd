package sc4

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func refreshMacKeyboardHelp(ctx context.Context, gameData, stage string, packages []string) error {
	hud := slices.IndexFunc(packages, func(path string) bool {
		name := filepath.Base(path)
		return name == "city-hud-x2.dat" || name == "city-hud-x4.dat"
	})
	if hud < 0 {
		return nil
	}
	pkg, err := openArchive(packages[hud])
	if err != nil {
		return err
	}
	defer pkg.file.Close()
	key := resourceID{0x2026960b, hdUITextGroup, keyHelpContent}
	entry := slices.IndexFunc(pkg.records, func(r record) bool { return r.id == key })
	if entry < 0 {
		return nil
	}
	stock, err := openArchive(filepath.Join(gameData, "SimCity_1.dat"))
	if err != nil {
		return err
	}
	defer stock.file.Close()
	help, err := keyboardHelpEntries(indexUIStock(stock), hasMacPackage(packages, russianTextPackage), true)
	if err != nil {
		return err
	}
	current, err := pkg.read(pkg.records[entry])
	if err != nil {
		return err
	}
	replacement := help[1].data
	if bytes.Equal(current, replacement) {
		return nil
	}
	entries := make([]resource, 0, len(pkg.records))
	for i, record := range pkg.records {
		if err := ctx.Err(); err != nil {
			return err
		}
		data := replacement
		if i != entry {
			// Copy stored bytes so existing texture and compression data remain intact.
			data = make([]byte, record.size)
			if _, err := pkg.file.ReadAt(data, int64(record.offset)); err != nil {
				return err
			}
		}
		entries = append(entries, resource{record.id, data})
	}
	target := filepath.Join(stage, filepath.Base(packages[hud]))
	if err := publishFile(target, func(w io.Writer) error { return writeArchive(w, entries) }); err != nil {
		return err
	}
	packages[hud] = target
	return nil
}

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

func availableMacPreparedUI(bundle, path string) string {
	path = availableMacPreparedBuild(path)
	if path == "" {
		return ""
	}
	packages, err := preparedMacPackages(&path)
	if err != nil {
		return ""
	}
	stock, err := macSetupSavedFiles(bundle)
	if err != nil {
		return ""
	}
	// An old executable in a remembered build would override the installed
	// assets on Configure. Only offer a build that can be selected again.
	if _, err := macSetupExecutable(gameExecutablePath(stock, true), path, packages, 0, nil, nil); err != nil {
		return ""
	}
	return path
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
