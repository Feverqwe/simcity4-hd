package world

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

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/texturecache"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
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

func (p modelPass) prefix(scale int) string {
	name := "model-materials"
	if p.directory != "G354" {
		name += "-" + strings.ToLower(p.directory)
	}
	return fmt.Sprintf("%s%s-x%d", name, p.selection, scale)
}

func modelMaterialSources(stock *gamefiles.Catalog, report *materialReport, selection modelSelection) ([]dbpf.ID, error) {
	var ids []dbpf.ID
	if selection == railMaterials {
		for _, base := range []uint32{0x03001a00, 0x03031500, 0x08aa6000} {
			for zoom := range uint32(5) {
				id := dbpf.ID{Kind: textures.FSHResource, Group: texturecache.NetworkGroup, Instance: base + zoom}
				if _, ok := stock.Owners[id]; !ok {
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
			id := dbpf.ID{Kind: textures.FSHResource, Group: texturecache.NetworkGroup, Instance: night}
			if _, ok := stock.Owners[id]; ok {
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
			ids = append(ids, dbpf.ID{Kind: textures.FSHResource, Group: uint32(value), Instance: instance})
		}
	}
	return ids, nil
}

func modelTextureSupported(data []byte, pass modelPass) (bool, error) {
	if len(data) < 16 || string(data[12:16]) != pass.directory {
		return false, nil
	}
	elements, err := textures.FSHLayout(data)
	if err != nil {
		return false, err
	}
	preservePlacement := pass.directory == "G264" || pass.directory == "G357"
	railPlacement := false
	for _, e := range elements {
		if e.Code != textures.BC1 && e.Code != textures.BC2 || e.Width%4 != 0 || e.Height%4 != 0 {
			return false, nil
		}
		centered := e.CenterX == 0 && e.CenterY == 0
		x, y := e.OffsetX&0xfff, e.OffsetY&0xfff
		switch pass.directory {
		case "G264":
			unverified := !centered || y != 0 || (x != 0 && x != 1 && x != 8)
			if unverified && pass.selection != railMaterials {
				return false, nil
			}
			railPlacement = railPlacement || unverified
		case "G357":
			if !centered || x != 1 || y != 0 || e.MipCount() != 0 {
				return false, nil
			}
		case "G354":
			if e.HasPlacement() || e.MipCount() != 0 {
				return false, nil
			}
		default:
			if e.HasPlacement() {
				return false, nil
			}
		}
		if _, err := textures.ValidateBCElement(data, e, 2, preservePlacement); err != nil {
			return false, err
		}
	}
	return pass.selection != railMaterials || railPlacement, nil
}

func (a worldArtwork) modelCache(scale int) string {
	return texturecache.ModelCache(a.cache, scale)
}

func buildModelBatches(stock *gamefiles.Catalog, materials *materialReport, art worldArtwork, output string, progress upscale.Progress) (map[dbpf.ID]bool, error) {
	const batchSize = 128
	seen := make(map[dbpf.ID]bool)
	cache := art.modelCache
	sources := make(map[modelSelection][]dbpf.ID)
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
	report := progress.Track(total)
	processed := 0
	for _, pass := range modelPasses {
		ids := sources[pass.selection]
		prefix := pass.prefix(art.scale)
		var entries []dbpf.Entry
		batch, count := 0, 0
		write := func() error {
			batch++
			path := filepath.Join(output, fmt.Sprintf("%s-%03d.dat", prefix, batch))
			if err := gamefiles.PublishFile(path, func(w io.Writer) error {
				return dbpf.Write(w, entries)
			}); err != nil {
				return err
			}
			fmt.Printf("%s: %d textures prepared\n", prefix, count)
			return nil
		}
		for _, id := range ids {
			report(processed)
			processed++
			if id.Instance&0xfffff000 == stationBase {
				continue
			}
			location, exists := stock.Owners[id]
			if !exists {
				return nil, fmt.Errorf("model material %s missing from stock", id)
			}
			data, err := location.Archive.Read(location.Record)
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
			hd, err := art.texture(cache, data, pass.directory == "G264" || pass.directory == "G357")
			if err != nil {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
			entries = append(entries, dbpf.Entry{ID: id, Data: hd})
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
		fmt.Printf("%s: %d textures in %d packages\n", prefix, count, batch)
	}
	report(total)
	return seen, nil
}

func RunModelTextures(args []string) (result error) {
	flags := flag.NewFlagSet("model-textures", flag.ContinueOnError)
	readInferenceConfig := upscale.InferenceFlags(flags)
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
	stock, err := gamefiles.OpenCatalog(game, func(id dbpf.ID) bool {
		return id.Kind == textures.FSHResource
	})
	if err != nil {
		return err
	}
	defer stock.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	art, closeWorker := worldInference(ctx, cache, config)
	defer func() {
		if result != nil {
			cancel()
		}
		result = errors.Join(result, closeWorker())
	}()
	if _, err := buildModelBatches(stock, report, art, stage, upscale.Progress{}); err != nil {
		return err
	}
	if err := closeWorker(); err != nil {
		return err
	}
	return os.Rename(stage, output)
}
