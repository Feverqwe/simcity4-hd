package sc4

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type modelSelection string

const (
	baseMaterials       modelSelection = ""
	additionalMaterials modelSelection = "-additional"
	nightMaterials      modelSelection = "-night"
	railMaterials       modelSelection = "-rail-placement"
)

type modelPass struct {
	directory string
	selection modelSelection
}

var modelPasses = []modelPass{
	{"G354", baseMaterials}, {"G264", baseMaterials}, {"G341", baseMaterials}, {"G357", baseMaterials},
	{"G354", additionalMaterials}, {"G354", nightMaterials},
	{"G264", additionalMaterials}, {"G264", nightMaterials},
	{"G264", railMaterials},
}

func (p modelPass) prefix() string {
	name := "model-materials"
	if p.directory != "G354" {
		name += "-" + strings.ToLower(p.directory)
	}
	return name + string(p.selection) + "-x4"
}

func modelMaterialSources(stock *gameCatalog, report *materialReport, selection modelSelection) ([]resourceID, error) {
	var ids []resourceID
	if selection == railMaterials {
		for _, base := range []uint32{0x03001a00, 0x03031500, 0x08aa6000} {
			for zoom := range uint32(5) {
				id := resourceID{fshResource, networkGroup, base + zoom}
				if _, ok := stock.owners[id]; !ok {
					return nil, fmt.Errorf("incomplete rail family: missing %s", id)
				}
				ids = append(ids, id)
			}
		}
		return ids, nil
	}
	instances := make([]string, 0, len(report.Materials))
	for key := range report.Materials {
		instances = append(instances, key)
	}
	slices.Sort(instances)
	const standardGroup = "1abe787d"
	for _, key := range instances {
		groups := report.Materials[key].Groups
		value, err := strconv.ParseUint(key, 16, 32)
		if err != nil {
			return nil, err
		}
		instance := uint32(value)
		if selection == nightMaterials {
			const nightFlag = 0x8000
			if instance&nightFlag != 0 || !slices.Contains(groups, standardGroup) {
				continue
			}
			night := instance | nightFlag
			if _, used := report.Materials[fmt.Sprintf("%08x", night)]; used {
				continue
			}
			id := resourceID{fshResource, networkGroup, night}
			if _, ok := stock.owners[id]; ok {
				ids = append(ids, id)
			}
			continue
		}
		standard := len(groups) == 1 && groups[0] == standardGroup
		if standard != (selection == baseMaterials) {
			continue
		}
		for _, group := range groups {
			value, err := strconv.ParseUint(group, 16, 32)
			if err != nil {
				return nil, err
			}
			ids = append(ids, resourceID{fshResource, uint32(value), instance})
		}
	}
	return ids, nil
}

func modelTextureSupported(data []byte, pass modelPass) (bool, error) {
	if len(data) < 16 || string(data[12:16]) != pass.directory {
		return false, nil
	}
	elements, err := fshLayout(data)
	if err != nil {
		return false, err
	}
	preservePlacement := pass.directory == "G264" || pass.directory == "G357"
	railPlacement := false
	for _, e := range elements {
		if e.code != bc1 && e.code != bc2 || e.width%4 != 0 || e.height%4 != 0 {
			return false, nil
		}
		centered := e.centerX == 0 && e.centerY == 0
		x, y := e.offsetX&0xfff, e.offsetY&0xfff
		switch pass.directory {
		case "G264":
			unverified := !centered || y != 0 || (x != 0 && x != 1 && x != 8)
			if unverified && pass.selection != railMaterials {
				return false, nil
			}
			railPlacement = railPlacement || unverified
		case "G357":
			if !centered || x != 1 || y != 0 || e.mipCount() != 0 {
				return false, nil
			}
		case "G354":
			if e.hasPlacement() || e.mipCount() != 0 {
				return false, nil
			}
		default:
			if e.hasPlacement() {
				return false, nil
			}
		}
		if _, err := validateBCElement(data, e, 2, preservePlacement); err != nil {
			return false, err
		}
	}
	return pass.selection != railMaterials || railPlacement, nil
}

func buildModelBatches(stock *gameCatalog, materials *materialReport, art worldArtwork, output string, progress buildProgress) (map[resourceID]bool, error) {
	const batchSize = 128
	seen := make(map[resourceID]bool)
	cache := filepath.Join(art.cache, "models", hatCheckpointSHA, "fsh-x4-v1")
	sources := make(map[modelSelection][]resourceID)
	total := 0
	for _, pass := range modelPasses {
		ids, known := sources[pass.selection]
		if !known {
			var err error
			ids, err = modelMaterialSources(stock, materials, pass.selection)
			if err != nil {
				return nil, err
			}
			sources[pass.selection] = ids
		}
		total += len(ids)
	}
	report := progress.track(total)
	processed := 0
	for _, pass := range modelPasses {
		ids := sources[pass.selection]
		var entries []resource
		batch, count := 0, 0
		write := func() error {
			batch++
			path := filepath.Join(output, fmt.Sprintf("%s-%03d.dat", pass.prefix(), batch))
			if err := publishFile(path, func(w io.Writer) error {
				return writeArchive(w, entries)
			}); err != nil {
				return err
			}
			fmt.Printf("%s: %d textures prepared\n", pass.prefix(), count)
			return nil
		}
		for _, id := range ids {
			report(processed)
			processed++
			if id.instance&0xfffff000 == 0x03100000 {
				continue
			}
			location, exists := stock.owners[id]
			if !exists {
				return nil, fmt.Errorf("model material %s missing from stock", id)
			}
			data, err := location.archive.read(location.record)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
			supported, err := modelTextureSupported(data, pass)
			if !supported {
				if pass.selection == railMaterials {
					return nil, fmt.Errorf("%s: unexpected stock rail layout", id)
				}
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s: skipping unsupported model texture: %v\n", id, err)
				}
				continue
			}
			if seen[id] {
				return nil, fmt.Errorf("duplicate model texture %s", id)
			}
			hd, err := art.texture(cache, data, 4, pass.directory == "G264" || pass.directory == "G357")
			if err != nil {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
			entries = append(entries, resource{id, hd})
			seen[id] = true
			count++
			if len(entries) == batchSize {
				if err := write(); err != nil {
					return nil, err
				}
				entries = nil
			}
		}
		if len(entries) != 0 {
			if err := write(); err != nil {
				return nil, err
			}
		}
		fmt.Printf("%s: %d textures in %d packages\n", pass.prefix(), count, batch)
	}
	report(total)
	return seen, nil
}

func runModelTextures(args []string) (result error) {
	flags := flag.NewFlagSet("model-textures", flag.ContinueOnError)
	readInferenceConfig := inferenceFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	config, err := readInferenceConfig()
	if err != nil {
		return err
	}
	if flags.NArg() != 3 {
		return errors.New("usage: simcity4-uhd model-textures [OPTIONS] GAME_DIR CACHE NEW_OUTPUT_DIR")
	}
	game, cache, output := flags.Arg(0), flags.Arg(1), flags.Arg(2)
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return fmt.Errorf("%s already exists", output)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(output), ".sc4-hd-models-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	report, err := indexMaterials(game)
	if err != nil {
		return err
	}
	stock, err := openGameCatalog(game, func(id resourceID) bool {
		return id.kind == fshResource
	})
	if err != nil {
		return err
	}
	defer stock.close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	art, closeWorker := worldInference(ctx, cache, config)
	defer func() {
		if result != nil {
			cancel()
		}
		result = errors.Join(result, closeWorker())
	}()
	if _, err := buildModelBatches(stock, report, art, stage, buildProgress{}); err != nil {
		return err
	}
	if err := closeWorker(); err != nil {
		return err
	}
	return os.Rename(stage, output)
}
