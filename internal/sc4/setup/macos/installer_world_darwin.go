package macos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	worldbuild "github.com/Feverqwe/simcity4-hd/internal/sc4/world"
)

type macWorldPlan struct {
	active, inactive []string
	missing          worldbuild.Selection
}

func macWorldCategory(name string, explicit bool) (string, string) {
	name = strings.ToLower(name)
	scale := macWorldScale.FindStringSubmatch(name)
	if scale == nil {
		return "", ""
	}
	category, _ := worldbuild.Package(name)
	if category == "" && strings.HasPrefix(name, "terrain-") {
		category = "terrain" // Older prepared macOS builds.
	}
	if category == "" && explicit {
		category = "models"
		if strings.HasPrefix(name, "network-") {
			category = "network"
		}
	}
	return category, scale[1]
}

func planMacWorld(bundle string, current macSetupRecord, desired macSetupBuild, prepared []string) (macWorldPlan, error) {
	plan := macWorldPlan{}
	scale := strconv.Itoa(desired.Scale)
	selected := map[string]bool{
		"network": desired.World && desired.Network,
		"terrain": desired.World && desired.Terrain,
		"models":  desired.World && desired.Models,
	}
	supplied, available := make(map[string]bool), make(map[string]bool)
	for _, path := range prepared {
		category, _ := macWorldCategory(filepath.Base(path), true)
		if selected[category] {
			plan.active = append(plan.active, path)
			supplied[category] = true
			available[category] = true
		}
	}
	if bundle != "" {
		seen := make(map[string]bool)
		inactive := macInactiveWorld(bundle)
		for _, folder := range []string{macModPlugins(bundle), inactive} {
			parked := folder == inactive
			entries, err := os.ReadDir(folder)
			if errors.Is(err, os.ErrNotExist) && parked {
				continue
			}
			if err != nil {
				return macWorldPlan{}, err
			}
			for _, entry := range entries {
				name := entry.Name()
				category, density := macWorldCategory(name, parked || slices.Contains(current.WorldFiles, name))
				if category == "" || seen[strings.ToLower(name)] {
					continue
				}
				if !entry.Type().IsRegular() {
					return macWorldPlan{}, fmt.Errorf("cannot preserve %s because it is not a regular file", filepath.Join(folder, name))
				}
				seen[strings.ToLower(name)] = true
				path := filepath.Join(folder, name)
				if density != scale || !selected[category] {
					plan.inactive = append(plan.inactive, path)
				} else if !supplied[category] {
					plan.active = append(plan.active, path)
					available[category] = true
				}
			}
		}
	}
	plan.missing = worldbuild.Selection{
		Network: selected["network"] && !available["network"],
		Terrain: selected["terrain"] && !available["terrain"],
		Models:  selected["models"] && !available["models"],
	}
	return plan, nil
}

func macInactiveWorld(bundle string) string {
	return filepath.Join(bundle, "Contents", "Resources", "SimCity4HD-InactiveWorld")
}

// The original macOS ×2 packages did not retain the mip chains of these
// zero-offset crop and model textures. Reuse their cached top levels.
var macLegacyWorldMips = map[dbpf.ID]bool{
	{Kind: textures.FSHResource, Group: 0x0986135e, Instance: 0x09bee100}: true,
	{Kind: textures.FSHResource, Group: 0x0986135e, Instance: 0x09bee200}: true,
	{Kind: textures.FSHResource, Group: 0x1abe787d, Instance: 0x8821ea00}: true,
}

// These Windows model materials are absent from the supported Aspyr build.
var windowsOnlyWorldMaterials = map[dbpf.ID]bool{
	{Kind: textures.FSHResource, Group: 0x1abe787d, Instance: 0x0401ffdf}: true,
	{Kind: textures.FSHResource, Group: 0x1abe787d, Instance: 0x0401ffff}: true,
}

func prepareMacWorldPackages(ctx context.Context, gameData string, scale int, paths []string, stage string, out io.Writer) ([]string, error) {
	var stock *gamefiles.Catalog
	if scale == 2 {
		var err error
		stock, err = gamefiles.OpenCatalog(gameData, func(id dbpf.ID) bool { return macLegacyWorldMips[id] })
		if err != nil {
			return nil, err
		}
		defer stock.Close()
	}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		prepared, err := prepareMacWorldPackage(stock, path, stage, out)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		result = append(result, prepared)
	}
	return result, nil
}

func prepareMacWorldPackage(stock *gamefiles.Catalog, path, stage string, out io.Writer) (string, error) {
	pkg, err := dbpf.Open(path)
	if err != nil {
		return "", err
	}
	defer pkg.Close()
	repaired := make(map[dbpf.ID][]byte)
	windowsOnly := 0
	for _, entry := range pkg.Records {
		if entry.ID.Kind != textures.FSHResource && entry.ID.Kind != worldbuild.S3DResource {
			return "", fmt.Errorf("non-World resource %s", entry.ID)
		}
		if windowsOnlyWorldMaterials[entry.ID] {
			windowsOnly++
			continue
		}
		if stock == nil || !macLegacyWorldMips[entry.ID] {
			continue
		}
		source, found := stock.Owners[entry.ID]
		if !found {
			return "", fmt.Errorf("missing original Aspyr texture %s", entry.ID)
		}
		original, err := source.Archive.Read(source.Record)
		if err != nil {
			return "", err
		}
		cached, err := pkg.Read(entry)
		if err != nil {
			return "", err
		}
		fixed, err := worldbuild.ReadWorldTexture(original, cached, 2, false, nil)
		if err != nil {
			return "", fmt.Errorf("texture %s: %w", entry.ID, err)
		}
		if !bytes.Equal(cached, fixed) {
			repaired[entry.ID] = fixed
		}
	}
	if len(repaired) == 0 && windowsOnly == 0 {
		return path, nil
	}

	entries := make([]dbpf.Entry, 0, len(pkg.Records)-windowsOnly)
	for _, entry := range pkg.Records {
		if windowsOnlyWorldMaterials[entry.ID] {
			continue
		}
		data, ok := repaired[entry.ID]
		if !ok {
			data, err = pkg.Read(entry)
			if err != nil {
				return "", err
			}
		}
		entries = append(entries, dbpf.Entry{ID: entry.ID, Data: data})
	}
	target := filepath.Join(stage, filepath.Base(path))
	if err := gamefiles.PublishFile(target, func(w io.Writer) error { return dbpf.Write(w, entries) }); err != nil {
		return "", err
	}
	if len(repaired) != 0 {
		fmt.Fprintf(out, "Restored Aspyr mip levels in %d textures from %s\n", len(repaired), filepath.Base(path))
	}
	if windowsOnly != 0 {
		fmt.Fprintf(out, "Omitted %d Windows model materials absent from Aspyr in %s\n", windowsOnly, filepath.Base(path))
	}
	return target, nil
}
