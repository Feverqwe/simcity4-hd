package sc4

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
)

const uiImage = 0x856ddbac
const catalogIconGroup = 0x6a386d26

func uiCachePath(cache string, id resourceID, original []byte, frames, scale int, keyed bool) string {
	suffix := ""
	if keyed {
		suffix += "-keyed"
	}
	if scale != 2 {
		suffix += fmt.Sprintf("-x%d", scale)
	}
	return filepath.Join(cache, fmt.Sprintf("%08x-%08x-%08x-%d-%x%s.png",
		id.kind, id.group, id.instance, frames, sha256.Sum256(original), suffix))
}

func readCatalogIcons(game string) (*gameCatalog, error) {
	isIcon := func(id resourceID) bool {
		return id.kind == uiImage && id.group == catalogIconGroup
	}
	catalog, err := openGameCatalog(game, isIcon)
	if err != nil {
		return nil, err
	}
	if err := addSKUIcons(catalog, game, isIcon); err != nil {
		catalog.close()
		return nil, err
	}
	slices.SortFunc(catalog.order, func(left, right resourceID) int {
		switch {
		case left.instance < right.instance:
			return -1
		case left.instance > right.instance:
			return 1
		default:
			return 0
		}
	})
	return catalog, nil
}

func addSKUIcons(catalog *gameCatalog, game string, isIcon func(resourceID) bool) error {
	paths, err := filepath.Glob(filepath.Join(game, "Sku_Data", "*", "*.dat"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		a, err := openArchive(path)
		if err != nil {
			return err
		}
		catalog.archives = append(catalog.archives, a)
		for _, record := range a.records {
			if !isIcon(record.id) {
				continue
			}
			if previous, exists := catalog.owners[record.id]; exists {
				stock, err := previous.archive.read(previous.record)
				if err != nil {
					return err
				}
				sku, err := a.read(record)
				if err != nil {
					return err
				}
				if !bytes.Equal(stock, sku) {
					return fmt.Errorf("%s: conflicting SKU catalog sheet %s", path, record.id)
				}
			} else {
				catalog.order = append(catalog.order, record.id)
			}
			catalog.owners[record.id] = resourceLocation{a, record}
		}
	}
	return nil
}

func catalogSheetEntries(game, cache string, scale int, infer func([]byte) ([]byte, error), progress buildProgress) (square, bridge []resource, err error) {
	catalog, err := readCatalogIcons(game)
	if err != nil {
		return nil, nil, err
	}
	defer catalog.close()
	report := progress.track(len(catalog.order))
	for i, id := range catalog.order {
		report(i)
		location := catalog.owners[id]
		stock, err := location.archive.read(location.record)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", id, err)
		}
		config, err := png.DecodeConfig(bytes.NewReader(stock))
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", id, err)
		}
		var destination *[]resource
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

		*destination = append(*destination, resource{id, art})
	}
	if len(square) == 0 || len(bridge) == 0 {
		return nil, nil, errors.New("expected square and bridge catalog sheets")
	}
	report(len(catalog.order))
	return square, bridge, nil
}

func runCatalogSheets(args []string) error {
	flags := flag.NewFlagSet("catalog-sheets", flag.ContinueOnError)
	readInferenceConfig := inferenceFlags(flags)
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
	var worker *inferenceWorker
	var infer func([]byte) ([]byte, error)
	if config.weights != "" {
		infer = func(png []byte) ([]byte, error) {
			if worker == nil {
				var err error
				worker, err = startInference(ctx, config, os.Stderr)
				if err != nil {
					return nil, err
				}
			}
			return worker.upscalePNG(png)
		}
	}
	square, bridge, err := catalogSheetEntries(args[0], args[1], 4, infer, buildProgress{})
	if worker != nil {
		if err != nil {
			cancel()
		}
		err = errors.Join(err, worker.close())
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
		data []resource
	}{
		{"catalog-square-x4.dat", square},
		{"catalog-bridge-x4.dat", bridge},
	} {
		if err := publishFile(filepath.Join(stage, sheet.name), func(w io.Writer) error {
			return writeArchive(w, sheet.data)
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
