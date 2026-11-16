package ui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

const uiImage = 0x856ddbac
const catalogIconGroup = 0x6a386d26

func uiCachePath(cache string, id dbpf.ID, original []byte, frames, scale int, keyed bool) string {
	suffix := ""
	if keyed {
		suffix += "-keyed"
	}
	if scale != 2 {
		suffix += fmt.Sprintf("-x%d", scale)
	}
	return filepath.Join(cache, fmt.Sprintf("%08x-%08x-%08x-%d-%x%s.png",
		id.Kind, id.Group, id.Instance, frames, sha256.Sum256(original), suffix))
}

func readCatalogIcons(game string) (*gamefiles.Catalog, error) {
	isIcon := func(id dbpf.ID) bool {
		return id.Kind == uiImage && id.Group == catalogIconGroup
	}
	catalog, err := gamefiles.OpenCatalog(game, isIcon)
	if err != nil {
		return nil, err
	}
	if err := addSKUIcons(catalog, game, isIcon); err != nil {
		catalog.Close()
		return nil, err
	}
	slices.SortFunc(catalog.Order, func(left, right dbpf.ID) int {
		switch {
		case left.Instance < right.Instance:
			return -1
		case left.Instance > right.Instance:
			return 1
		default:
			return 0
		}
	})
	return catalog, nil
}

func addSKUIcons(catalog *gamefiles.Catalog, game string, isIcon func(dbpf.ID) bool) error {
	paths, err := filepath.Glob(filepath.Join(game, "Sku_Data", "*", "*.dat"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		a, err := dbpf.Open(path)
		if err != nil {
			return err
		}
		catalog.Archives = append(catalog.Archives, a)
		for _, record := range a.Records {
			if !isIcon(record.ID) {
				continue
			}
			if previous, exists := catalog.Owners[record.ID]; exists {
				stock, err := previous.Archive.Read(previous.Record)
				if err != nil {
					return err
				}
				sku, err := a.Read(record)
				if err != nil {
					return err
				}
				if !bytes.Equal(stock, sku) {
					return fmt.Errorf("%s: conflicting SKU catalog sheet %s", path, record.ID)
				}
			} else {
				catalog.Order = append(catalog.Order, record.ID)
			}
			catalog.Owners[record.ID] = gamefiles.ResourceLocation{Archive: a, Record: record}
		}
	}
	return nil
}

func catalogSheetEntries(game, cache string, scale int, infer func([]byte) ([]byte, error), progress upscale.Progress) (square, bridge []dbpf.Entry, err error) {
	catalog, err := readCatalogIcons(game)
	if err != nil {
		return nil, nil, err
	}
	defer catalog.Close()
	report := progress.Track(len(catalog.Order))
	for i, id := range catalog.Order {
		report(i)
		location := catalog.Owners[id]
		stock, err := location.Archive.Read(location.Record)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", id, err)
		}
		config, err := png.DecodeConfig(bytes.NewReader(stock))
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", id, err)
		}
		var destination *[]dbpf.Entry
		switch {
		case config.Width == 176 && config.Height == 44:
			destination = &square
		case config.Width == 356 && config.Height == 58:
			destination = &bridge
		default:
			continue
		}
		width := config.Width / 4
		art, _, err := cachedUIAtlas(cache, id, stock, []int{width, width, width, width}, 4, infer)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", id, err)
		}

		if scale == 2 {
			art, err = projectUIArtwork(stock, art, []int{width, width, width, width}, false)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", id, err)
			}
		}

		*destination = append(*destination, dbpf.Entry{ID: id, Data: art})
	}
	if len(square) == 0 || len(bridge) == 0 {
		return nil, nil, errors.New("expected square and bridge catalog sheets")
	}
	report(len(catalog.Order))
	return square, bridge, nil
}

func RunCatalogSheets(args []string) error {
	flags := flag.NewFlagSet("catalog-sheets", flag.ContinueOnError)
	readInferenceConfig := upscale.InferenceFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	config, err := readInferenceConfig()
	if err != nil {
		return err
	}
	args = flags.Args()
	if len(args) != 3 {
		return errors.New("usage: simcity4-uhd catalog-sheets [OPTIONS] GAME_DIR X4_CACHE NEW_OUTPUT_DIR")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var worker *upscale.InferenceWorker
	var infer func([]byte) ([]byte, error)
	if config.Weights != "" {
		infer = func(png []byte) ([]byte, error) {
			if worker == nil {
				var err error
				worker, err = upscale.StartInference(ctx, config, os.Stderr)
				if err != nil {
					return nil, err
				}
			}
			return worker.UpscalePNG(png)
		}
	}
	infer = upscale.CachedInference(ctx, args[1], config, infer)
	square, bridge, err := catalogSheetEntries(args[0], args[1], 4, infer, upscale.Progress{})
	if worker != nil {
		if err != nil {
			cancel()
		}
		err = errors.Join(err, worker.Close())
	}
	if err != nil {
		return err
	}
	output, err := filepath.Abs(args[2])
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return fmt.Errorf("%s already exists", output)
	}
	stage, err := os.MkdirTemp(filepath.Dir(output), ".catalog-sheets-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for _, sheet := range []struct {
		name string
		data []dbpf.Entry
	}{
		{"catalog-square-x4.dat", square},
		{"catalog-bridge-x4.dat", bridge},
	} {
		if err := gamefiles.PublishFile(filepath.Join(stage, sheet.name), func(w io.Writer) error {
			return dbpf.Write(w, sheet.data)
		}); err != nil {
			return err
		}
	}
	if err := os.Rename(stage, output); err != nil {
		return err
	}
	fmt.Printf("%d square and %d bridge catalog sheets\n", len(square), len(bridge))
	return nil
}
