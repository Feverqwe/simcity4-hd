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
)

const networkGroup = 0x1abe787d

func networkFamilyIDs(stock *gameCatalog, materials *materialReport) ([]resourceID, error) {
	var bases []uint32
	for _, id := range stock.order {
		if id.group == networkGroup && id.instance&15 == 0 {
			bases = append(bases, id.instance)
		}
	}
	slices.Sort(bases)
	var result []resourceID
	for _, base := range bases {
		var family [5]resourceID
		eligible := true
		for zoom := range family {
			instance := base + uint32(zoom)
			id := resourceID{fshResource, networkGroup, instance}
			family[zoom] = id
			_, exists := stock.owners[id]
			_, used := materials.Materials[fmt.Sprintf("%08x", instance)]
			if !exists || used {
				eligible = false
				break
			}
		}
		if !eligible {
			continue
		}
		switch filepath.Base(stock.owners[family[4]].archive.file.Name()) {
		case "SimCity_5.dat", "EP1.dat":
		default:
			continue
		}
		for zoom, id := range family {
			location := stock.owners[id]
			data, err := location.archive.read(location.record)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
			if len(data) < 16 || string(data[12:16]) != "G264" {
				eligible = false
				break
			}
			elements, err := fshLayout(data)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
			if len(elements) != 1 || !networkTextureAtZoom(elements[0], zoom) {
				eligible = false
				break
			}
		}
		if eligible {
			result = append(result, family[:]...)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("no complete stock network texture families found")
	}
	return result, nil
}

func networkTextureAtZoom(e fshElement, zoom int) bool {
	edge := 8 << zoom
	return e.width == edge && e.height == edge && (e.code == bc1 || e.code == bc2) && e.mipCount() == 0 &&
		e.centerX == 0 && e.centerY == 0 && e.offsetX&0xfff == 8 && e.offsetY&0xfff == 0
}

func networkCache(cache string, scale int) string {
	return filepath.Join(cache, "world", hatCheckpointSHA, fmt.Sprintf("network-fsh-x%d-%s", scale, worldCacheVersion))
}

func networkEntries(stock *gameCatalog, materials *materialReport, art worldArtwork, progress buildProgress) ([]resource, error) {
	ids, err := networkFamilyIDs(stock, materials)
	if err != nil {
		return nil, err
	}
	cache := func(scale int) string {
		return networkCache(art.cache, scale)
	}
	entries := make([]resource, 0, len(ids))
	report := progress.track(len(ids))
	for i, id := range ids {
		report(i)
		location := stock.owners[id]
		original, err := location.archive.read(location.record)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		hd, err := art.texture(cache, original, true)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		entries = append(entries, resource{id, hd})
		if len(entries)%250 == 0 {
			fmt.Printf("Network: %d/%d textures prepared\n", len(entries), len(ids))
		}
	}
	report(len(ids))
	return entries, nil
}

func runNetworkTextures(args []string) (result error) {
	flags := flag.NewFlagSet("network-textures", flag.ContinueOnError)
	readInferenceConfig := inferenceFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	config, err := readInferenceConfig()
	if err != nil {
		return err
	}
	if flags.NArg() != 3 {
		return errors.New("usage: simcity4-uhd network-textures [OPTIONS] GAME_DIR CACHE NEW_OUTPUT_DAT")
	}
	game, cache, output := flags.Arg(0), flags.Arg(1), flags.Arg(2)
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return fmt.Errorf("%s already exists", output)
	}
	materials, err := indexMaterials(game)
	if err != nil {
		return err
	}
	stock, err := openGameCatalog(game, func(id resourceID) bool {
		return id.kind == fshResource && id.group == networkGroup
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
	entries, err := networkEntries(stock, materials, art, buildProgress{})
	if err != nil {
		return err
	}
	if err := publishFile(output, func(w io.Writer) error {
		return writeArchive(w, entries)
	}); err != nil {
		return err
	}
	fmt.Printf("%d network families, %d textures\n", len(entries)/5, len(entries))
	return nil
}
