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

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/texturecache"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

func networkFamilyIDs(stock *gamefiles.Catalog, materials *materialReport) ([]dbpf.ID, error) {
	var bases []uint32
	for _, id := range stock.Order {
		if id.Group == texturecache.NetworkGroup && id.Instance&15 == 0 {
			bases = append(bases, id.Instance)
		}
	}
	slices.Sort(bases)
	var result []dbpf.ID
	for _, base := range bases {
		var family [5]dbpf.ID
		eligible := true
		for zoom := range family {
			instance := base + uint32(zoom)
			id := dbpf.ID{Kind: textures.FSHResource, Group: texturecache.NetworkGroup, Instance: instance}
			family[zoom] = id
			_, exists := stock.Owners[id]
			_, used := materials.Materials[fmt.Sprintf("%08x", instance)]
			if !exists || used {
				eligible = false
				break
			}
		}
		if !eligible {
			continue
		}
		switch filepath.Base(stock.Owners[family[4]].Archive.Name()) {
		case "SimCity_5.dat", "EP1.dat":
		default:
			continue
		}
		for zoom, id := range family {
			location := stock.Owners[id]
			data, err := location.Archive.Read(location.Record)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
			if len(data) < 16 || string(data[12:16]) != "G264" {
				eligible = false
				break
			}
			elements, err := textures.FSHLayout(data)
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

func networkTextureAtZoom(e textures.FSHElement, zoom int) bool {
	edge := 8 << zoom
	return e.Width == edge && e.Height == edge && (e.Code == textures.BC1 || e.Code == textures.BC2) && e.MipCount() == 0 &&
		e.CenterX == 0 && e.CenterY == 0 && e.OffsetX&0xfff == 8 && e.OffsetY&0xfff == 0
}

func networkEntries(stock *gamefiles.Catalog, materials *materialReport, art worldArtwork, progress upscale.Progress) ([]dbpf.Entry, error) {
	ids, err := networkFamilyIDs(stock, materials)
	if err != nil {
		return nil, err
	}
	cache := func(scale int) string {
		return texturecache.NetworkCache(art.cache, scale)
	}
	entries := make([]dbpf.Entry, 0, len(ids))
	report := progress.Track(len(ids))
	for i, id := range ids {
		report(i)
		location := stock.Owners[id]
		original, err := location.Archive.Read(location.Record)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		hd, err := art.texture(cache, original, true)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		entries = append(entries, dbpf.Entry{ID: id, Data: hd})
		if len(entries)%250 == 0 {
			fmt.Printf("Network: %d/%d textures prepared\n", len(entries), len(ids))
		}
	}
	report(len(ids))
	return entries, nil
}

func RunNetworkTextures(args []string) (result error) {
	flags := flag.NewFlagSet("network-textures", flag.ContinueOnError)
	readInferenceConfig := upscale.InferenceFlags(flags)
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
	stock, err := gamefiles.OpenCatalog(game, func(id dbpf.ID) bool {
		return id.Kind == textures.FSHResource && id.Group == texturecache.NetworkGroup
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
	entries, err := networkEntries(stock, materials, art, upscale.Progress{})
	if err != nil {
		return err
	}
	if err := gamefiles.PublishFile(output, func(w io.Writer) error {
		return dbpf.Write(w, entries)
	}); err != nil {
		return err
	}
	fmt.Printf("%d network families, %d textures\n", len(entries)/5, len(entries))
	return nil
}
