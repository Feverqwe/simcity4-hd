package sc4

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"slices"
)

const (
	previewGroup         = 0x4c06f888
	previewHDGroup       = 0xe74a40c0
	disasterBackground   = 0x8a2564a2
	disasterBackgroundHD = 0xe74a40c3
)

type uiArtwork struct {
	cache string
	infer func([]byte) ([]byte, error)
}

func (a uiArtwork) upscale(spec layoutAtlas, original []byte) ([]byte, error) {
	config, err := png.DecodeConfig(bytes.NewReader(original))
	if err != nil {
		return nil, err
	}
	if spec.frames <= 0 || config.Width%spec.frames != 0 {
		return nil, fmt.Errorf("invalid UI atlas frame geometry")
	}
	widths := slices.Repeat([]int{config.Width / spec.frames}, spec.frames)
	data, _, err := cachedUIAtlas(filepath.Join(a.cache, "ui-atlases-x4"), spec.source, original, widths, a.infer)
	return data, err
}

func buildHUD(game string, stock uiStock, art uiArtwork) ([]resource, error) {
	intro, err := openArchive(filepath.Join(game, "Intro.dat"))
	if err != nil {
		return nil, err
	}
	defer intro.file.Close()
	for _, r := range intro.records {
		if r.id.kind != uiImage || r.id.group != catalogFrameGroup ||
			(r.id.instance != 0xea7f0eaf && r.id.instance != 0xea7f0eae) {
			continue
		}
		if _, exists := stock.index[r.id]; !exists {
			stock.index[r.id] = resourceLocation{intro, r}
		}
	}
	font, err := stock.read(uiFontID)
	if err != nil {
		return nil, err
	}
	entries, config, err := regionEntries(stock, string(font), art.upscale)
	if err != nil {
		return nil, err
	}
	windows, config, err := buildWindowEntries(stock, config, false, art.upscale)
	if err != nil {
		return nil, err
	}
	entries = append(entries, windows...)
	config, err = scaleRuntimeFonts(config, string(font), true)
	if err != nil {
		return nil, err
	}
	entries = append(entries, resource{uiFontID, []byte(config)})
	restoreID := resourceID{uiImage, catalogFrameGroup, 0x53244588}
	data, err := stock.read(restoreID)
	if err != nil {
		return nil, err
	}
	data, err = art.upscale(layoutAtlas{restoreID, 4}, data)
	if err != nil {
		return nil, err
	}
	entries = append(entries, resource{resourceID{uiImage, 0xe74a40bd, restoreID.instance}, data})

	arrows, err := stock.read(resourceID{uiImage, catalogFrameGroup, 0x14015584})
	if err != nil {
		return nil, err
	}
	source, err := decodeRGBA(arrows)
	if err != nil {
		return nil, err
	}
	enlarged := image.NewNRGBA(image.Rect(0, 0, 4*source.Rect.Dx(), 4*source.Rect.Dy()))
	for y := range enlarged.Rect.Dy() {
		for x := range enlarged.Rect.Dx() {
			enlarged.SetNRGBA(x, y, source.NRGBAAt(x/4, y/4))
		}
	}
	data, err = encodeRGBA(enlarged)
	if err != nil {
		return nil, err
	}
	entries = append(entries, resource{resourceID{uiImage, catalogFrameGroup, 0x2bd260ab}, data})

	var previews []uint32
	for id := range stock.index {
		if id.kind == uiImage && id.group == previewGroup {
			previews = append(previews, id.instance)
		}
	}
	if len(previews) == 0 {
		return nil, errors.New("no stock My Sims previews found")
	}
	slices.Sort(previews)
	for _, instance := range previews {
		id := resourceID{uiImage, previewGroup, instance}
		target := resourceID{uiImage, previewHDGroup, instance}
		if _, exists := stock.index[target]; exists {
			return nil, fmt.Errorf("My Sims preview key collision: %s", target)
		}
		original, err := stock.read(id)
		if err != nil {
			return nil, err
		}
		data, err := art.upscale(layoutAtlas{id, 1}, original)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		entries = append(entries, resource{target, data})
	}
	return entries, nil
}

func disasterEntries(stock uiStock, art uiArtwork) ([]resource, error) {
	id := resourceID{uiImage, catalogFrameGroup, disasterBackground}
	first, err := stock.read(id)
	if err != nil {
		return nil, err
	}
	second, err := stock.read(resourceID{uiImage, mySimPortraitGroup, disasterBackground})
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(first, second) {
		return nil, errors.New("stock Disasters background copies differ")
	}
	data, err := art.upscale(layoutAtlas{id, 1}, first)
	if err != nil {
		return nil, err
	}
	return []resource{
		{resourceID{uiImage, catalogFrameGroup, disasterBackgroundHD}, data},
		{resourceID{uiImage, mySimPortraitGroup, disasterBackgroundHD}, data},
	}, nil
}

type uiPackage struct {
	name    string
	entries []resource
}

func runBuildUI(args []string) (result error) {
	flags := flag.NewFlagSet("ui", flag.ContinueOnError)
	distro := flags.String("wsl-distro", "", "WSL distribution for inference")
	python := flags.String("wsl-python", "python3", "Python interpreter in WSL")
	weights := flags.String("wsl-weights", "", "checkpoint path in WSL for cache misses")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 3 {
		return errors.New("usage: simcity4-hd ui [OPTIONS] GAME_DIR CACHE NEW_OUTPUT_DIR")
	}
	game, cache, output := flags.Arg(0), flags.Arg(1), flags.Arg(2)
	// Validate the executable fingerprint before doing potentially lengthy inference.
	executable, err := os.ReadFile(filepath.Join(game, "Apps", "SimCity 4.exe"))
	if err != nil {
		return err
	}
	if err := patchUIExecutable(executable); err != nil {
		return err
	}
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return fmt.Errorf("%s already exists", output)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var worker *inferenceWorker
	closeWorker := func() error {
		if worker == nil {
			return nil
		}
		err := worker.close()
		worker = nil
		return err
	}
	defer func() {
		if result != nil {
			cancel()
		}
		result = errors.Join(result, closeWorker())
	}()
	var infer func([]byte) ([]byte, error)
	if *weights != "" {
		infer = func(data []byte) ([]byte, error) {
			if worker == nil {
				var err error
				worker, err = startInference(ctx, inferenceConfig{
					distro: *distro, python: *python, weights: *weights, scale: 4, tile: 64, overlap: 16,
				}, os.Stderr)
				if err != nil {
					return nil, err
				}
			}
			return worker.upscalePNG(data)
		}
	}
	stock, err := openArchive(filepath.Join(game, "SimCity_1.dat"))
	if err != nil {
		return err
	}
	defer stock.file.Close()
	stockUI := indexUIStock(stock)
	art := uiArtwork{cache, infer}
	hud, err := buildHUD(game, stockUI, art)
	if err != nil {
		return fmt.Errorf("build HUD: %w", err)
	}
	square, bridge, err := catalogSheetEntries(game, filepath.Join(cache, "ui-atlases-x4"), infer)
	if err != nil {
		return fmt.Errorf("build catalog sheets: %w", err)
	}
	chrome, err := catalogChromeEntries(stock, filepath.Join(cache, "ui-catalog-chrome"), infer)
	if err != nil {
		return fmt.Errorf("build catalog frames: %w", err)
	}
	disasters, err := disasterEntries(stockUI, art)
	if err != nil {
		return fmt.Errorf("build Disasters background: %w", err)
	}
	portraits, err := mySimPortraitEntries(stock, filepath.Join(cache, "ui-portraits-x4"), infer)
	if err != nil {
		return fmt.Errorf("build My Sims portraits: %w", err)
	}
	if err := closeWorker(); err != nil {
		return err
	}
	tutorial, err := tutorialEntries(filepath.Join(game, "SimCityLocale.DAT"))
	if err != nil {
		return err
	}
	return publishUI(output, executable, []uiPackage{
		{"city-hud-x4.dat", hud},
		{"catalog-square-x4.dat", square},
		{"catalog-bridge-x4.dat", bridge},
		{"catalog-chrome-x4.dat", chrome},
		{"disasters-background-x4.dat", disasters},
		{"mysim-portraits-x4.dat", portraits},
		{"tutorial-text-size-7.dat", tutorial},
	})
}

func publishUI(output string, executable []byte, packages []uiPackage) error {
	owners := make(map[resourceID]string)
	for _, p := range packages {
		for _, entry := range p.entries {
			if previous := owners[entry.id]; previous != "" {
				return fmt.Errorf("UI resource %s appears in %s and %s", entry.id, previous, p.name)
			}
			owners[entry.id] = p.name
		}
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(output), ".sc4-hd-ui-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	plugins := filepath.Join(stage, "Plugins")
	if err := os.Mkdir(plugins, 0755); err != nil {
		return err
	}
	for _, p := range packages {
		if err := publishFile(filepath.Join(plugins, p.name), func(w io.Writer) error {
			return writeArchive(w, p.entries)
		}); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(stage, "SimCity4-hd.exe"), executable, 0755); err != nil {
		return err
	}
	if err := os.Rename(stage, output); err != nil {
		return err
	}
	fmt.Printf("%d UI packages, %d resources\n", len(packages), len(owners))
	return nil
}
