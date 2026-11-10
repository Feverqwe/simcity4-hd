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
	budgetSliderGroup    = 0xe74a40c6
	budgetSliderImage    = 0x46a006a7
	previewGroup         = 0x4c06f888
	previewHDGroup       = 0xe74a40c0
	disasterBackground   = 0x8a2564a2
	disasterBackgroundHD = 0xe74a40c3
)

type uiArtwork struct {
	cache string
	scale int
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
	data, _, err := cachedUIAtlas(filepath.Join(a.cache, "ui-atlases-x4"), spec.source, original, widths, 4, a.infer)
	if err != nil || a.scale == 4 && !spec.project {
		return data, err
	}
	originalAlpha := spec.source.group == previewGroup ||
		spec.source == (resourceID{uiImage, catalogFrameGroup, disasterBackground})
	return projectUIArtworkWithAlpha(original, data, widths, originalAlpha, false)
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
	entries, config, err := regionEntries(stock, string(font), art.scale, art.upscale)
	if err != nil {
		return nil, err
	}
	windows, config, err := buildWindowEntries(stock, config, false, art.scale, art.upscale)
	if err != nil {
		return nil, err
	}
	entries = append(entries, windows...)
	config, err = scaleRuntimeFonts(config, string(font), true)
	if err != nil {
		return nil, err
	}
	if art.scale == 4 {
		slider, err := buildBudgetSliderImage(stock, art)
		if err != nil {
			return nil, err
		}
		entries = append(entries, slider)
		config, err = addLayoutFontAliases(config, []fontAlias{
			{"BdgtPopupHeader", "HDBdgtPopupHeader"},
			{"BdgtPopupBody", "HDBdgtPopupBody"},
			{"GenHeader", "HDgenheader"},
			{"GenButton", "HDgenbutton"},
			{"BdgtLedgerHeader", "HDbdgtledgerheader"},
			{"BdgtLedgerLineLite", "HDbdgtledgerlinelite"},
			{"BdgtLedgerLineLiteUnder", "HDbdgtledgerlineliteunder"},
			{"BdgtLedgerLineHeavy", "HDbdgtledgerlineheavy"},
		})
		if err != nil {
			return nil, err
		}
	}
	entries = append(entries, resource{uiFontID, []byte(config)})
	restoreID := resourceID{uiImage, catalogFrameGroup, 0x53244588}
	data, err := stock.read(restoreID)
	if err != nil {
		return nil, err
	}
	data, err = art.upscale(layoutAtlas{source: restoreID, frames: 4, project: true}, data)
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
	// The polls control uses atlas dimensions as the displayed size.
	const arrowScale = 2
	enlarged := image.NewNRGBA(image.Rect(0, 0, arrowScale*source.Rect.Dx(), arrowScale*source.Rect.Dy()))
	for y := range enlarged.Rect.Dy() {
		for x := range enlarged.Rect.Dx() {
			enlarged.SetNRGBA(x, y, source.NRGBAAt(x/arrowScale, y/arrowScale))
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
		data, err := art.upscale(layoutAtlas{source: id, frames: 1, project: true}, original)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		entries = append(entries, resource{target, data})
	}
	return entries, nil
}

func buildBudgetSliderImage(stock uiStock, art uiArtwork) (resource, error) {
	source := resourceID{uiImage, catalogFrameGroup, budgetSliderImage}
	target := resourceID{uiImage, budgetSliderGroup, budgetSliderImage}
	if _, exists := stock.index[target]; exists {
		return resource{}, fmt.Errorf("budget slider image key collision: %s", target)
	}
	original, err := stock.read(source)
	if err != nil {
		return resource{}, err
	}
	data, err := art.upscale(layoutAtlas{source: source, frames: 4, project: true}, original)
	return resource{target, data}, err
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
	// The runtime Disasters panel shares the catalog's native copy path.
	data, err := art.upscale(layoutAtlas{source: id, frames: 1, project: true}, first)
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
	textureScale := flags.Int("texture-scale", 4, "UI texture scale (2 or 4)")
	distro := flags.String("wsl-distro", "", "WSL distribution for inference")
	python := flags.String("wsl-python", "python3", "Python interpreter in WSL")
	weights := flags.String("wsl-weights", "", "checkpoint path in WSL for cache misses")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 3 {
		return errors.New("usage: simcity4-hd ui [OPTIONS] GAME_DIR CACHE NEW_OUTPUT_DIR")
	}
	if *textureScale != 2 && *textureScale != 4 {
		return errors.New("texture scale must be 2 or 4")
	}
	game, cache, output := flags.Arg(0), flags.Arg(1), flags.Arg(2)
	// Validate the executable fingerprint before doing potentially lengthy inference.
	executable, err := os.ReadFile(filepath.Join(game, "Apps", "SimCity 4.exe"))
	if err != nil {
		return err
	}
	executable, err = patchUIExecutable(executable, *textureScale)
	if err != nil {
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
	inferenceScale := 4
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
					distro: *distro, python: *python, weights: *weights, scale: inferenceScale, tile: 64, overlap: 16,
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
	art := uiArtwork{cache, *textureScale, infer}
	hud, err := buildHUD(game, stockUI, art)
	if err != nil {
		return fmt.Errorf("build HUD: %w", err)
	}
	// The runtime catalog shares native copy routines with other windows.
	// Project its x4 art to the doubled frame geometry those routines expect.
	square, bridge, err := catalogSheetEntries(game, filepath.Join(cache, "ui-atlases-x4"), 2, infer)
	if err != nil {
		return fmt.Errorf("build catalog sheets: %w", err)
	}
	chrome, err := catalogChromeEntries(stock, filepath.Join(cache, "ui-catalog-chrome"), 2, infer)
	if err != nil {
		return fmt.Errorf("build catalog frames: %w", err)
	}
	disasters, err := disasterEntries(stockUI, art)
	if err != nil {
		return fmt.Errorf("build Disasters background: %w", err)
	}
	if *textureScale == 2 {
		if err := closeWorker(); err != nil {
			return err
		}
		inferenceScale = 2
	}
	suffix := fmt.Sprintf("x%d", *textureScale)
	portraits, err := mySimPortraitEntries(stock, filepath.Join(cache, "ui-portraits-"+suffix), *textureScale, infer)
	if err != nil {
		return fmt.Errorf("build My Sims portraits: %w", err)
	}
	if err := closeWorker(); err != nil {
		return err
	}
	locale := filepath.Join(game, "SimCityLocale.DAT")
	tutorial, err := tutorialEntries(locale)
	if err != nil {
		return err
	}
	credits, err := creditsEntries(locale)
	if err != nil {
		return err
	}
	packages := []uiPackage{
		{"city-hud-" + suffix + ".dat", hud},
		{"catalog-square-" + suffix + ".dat", square},
		{"catalog-bridge-" + suffix + ".dat", bridge},
		{"catalog-chrome-" + suffix + ".dat", chrome},
		{"disasters-background-" + suffix + ".dat", disasters},
		{"mysim-portraits-" + suffix + ".dat", portraits},
		{"tutorial-text-size-7.dat", tutorial},
	}
	if len(credits) != 0 {
		packages = append(packages, uiPackage{"credits-text.dat", credits})
	}
	return publishUI(output, executable, packages)
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
