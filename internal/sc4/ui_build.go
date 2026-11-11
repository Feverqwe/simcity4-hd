package sc4

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"slices"
)

const (
	importIconGroup       = 0xe74a40c7
	budgetRowArtGroup     = 0xe74a40c8
	budgetDetailsImage    = 0x140155b7
	budgetCloseImage      = 0x140155b4
	budgetCloseHD         = 0x14416260
	budgetCheckboxImage   = 0x144161ea
	playlistCheckboxImage = 0x14416244
	budgetSliderGroup     = 0xe74a40c6
	budgetSliderImage     = 0x46a006a7
	mayorRatingGroup      = 0xe74a40c5
	mayorRatingImage      = 0x14015549
	regionRatingImage     = 0x14416327
	newsMarkerGroup       = 0xe74a40c4
	newsMarkerFirst       = 0x14416250
	newsMarkerCount       = 16
	previewGroup          = 0x4c06f888
	previewHDGroup        = 0xe74a40c0
	disasterBackground    = 0x8a2564a2
	disasterBackgroundHD  = 0xe74a40c3
	disasterBackgroundX4  = 0xe74a40c9
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
	if err != nil {
		return nil, err
	}
	if a.scale == 4 && !spec.project {
		return data, nil
	}
	originalAlpha := spec.source == (resourceID{uiImage, catalogFrameGroup, disasterBackground})
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
		markers, err := buildNewsMarkers(stock, art)
		if err != nil {
			return nil, err
		}
		entries = append(entries, markers...)
		for _, instance := range []uint32{mayorRatingImage, regionRatingImage} {
			entry, err := buildRatingImage(stock, art, instance)
			if err != nil {
				return nil, err
			}
			entries = append(entries, entry)
		}
		// Runtime budget and playlist controls use atlases at screen density.
		for _, atlas := range []struct {
			source, target, group uint32
			frames                int
		}{
			{budgetSliderImage, budgetSliderImage, budgetSliderGroup, 4},
			{budgetCheckboxImage, budgetCheckboxImage, budgetRowArtGroup, 8},
			{playlistCheckboxImage, playlistCheckboxImage, budgetRowArtGroup, 8},
			{budgetDetailsImage, budgetDetailsImage, budgetRowArtGroup, 4},
			{budgetCloseImage, budgetCloseHD, budgetRowArtGroup, 4},
		} {
			source := resourceID{uiImage, catalogFrameGroup, atlas.source}
			target := resourceID{uiImage, atlas.group, atlas.target}
			if _, exists := stock.index[target]; exists {
				return nil, fmt.Errorf("runtime image key collision: %s", target)
			}
			original, err := stock.read(source)
			if err != nil {
				return nil, err
			}
			data, err := art.upscale(layoutAtlas{source: source, frames: atlas.frames, project: true}, original)
			if err != nil {
				return nil, fmt.Errorf("runtime image %s: %w", source, err)
			}
			if atlas.source == playlistCheckboxImage {
				imageData, err := decodeRGBA(data)
				if err != nil {
					return nil, err
				}
				if imageData.Bounds() != image.Rect(0, 0, 256, 32) {
					return nil, fmt.Errorf("unexpected playlist atlas bounds: %s", imageData.Bounds())
				}
				// The playlist shows 28-pixel rows. Trim two pixels of
				// padding above and below each 32-pixel checkbox cell.
				cropped := image.NewNRGBA(image.Rect(0, 0, 256, 28))
				draw.Draw(cropped, cropped.Bounds(), imageData, image.Pt(0, 2), draw.Src)
				data, err = encodeRGBA(cropped)
				if err != nil {
					return nil, err
				}
			}
			entries = append(entries, resource{target, data})
		}
		for instance := uint32(0x1441624a); instance <= 0x1441624c; instance++ {
			source := resourceID{uiImage, catalogFrameGroup, instance}
			target := resourceID{uiImage, importIconGroup, instance}
			if _, exists := stock.index[target]; exists {
				return nil, fmt.Errorf("import icon key collision: %s", target)
			}
			original, err := stock.read(source)
			if err != nil {
				return nil, err
			}
			data, err := art.upscale(layoutAtlas{source: source, frames: 1, project: true}, original)
			if err != nil {
				return nil, fmt.Errorf("import icon %s: %w", source, err)
			}
			entries = append(entries, resource{target, data})
		}
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

	// The polls renderer samples discrete score rows and draws the arrows
	// at native size. Keep each arrow in its original color.
	fillID := resourceID{uiImage, catalogFrameGroup, 0x14015580}
	arrowID := resourceID{uiImage, catalogFrameGroup, 0x14015584}
	fillTarget := resourceID{uiImage, catalogFrameGroup, 0x2bd260ac}
	arrowTarget := resourceID{uiImage, catalogFrameGroup, 0x2bd260ab}
	for _, target := range []resourceID{fillTarget, arrowTarget} {
		if _, exists := stock.index[target]; exists {
			return nil, fmt.Errorf("polls image key collision: %s", target)
		}
	}
	fill, err := stock.read(fillID)
	if err != nil {
		return nil, err
	}
	fill, err = resizeUIExact(fill, 2, 1)
	if err != nil {
		return nil, fmt.Errorf("polls fill: %w", err)
	}
	arrows, err := stock.read(arrowID)
	if err != nil {
		return nil, err
	}
	arrows, err = art.upscalePollArrows(arrowID, arrows)
	if err != nil {
		return nil, fmt.Errorf("polls arrows: %w", err)
	}
	entries = append(entries, resource{fillTarget, fill}, resource{arrowTarget, arrows})

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
		data, err := art.upscale(layoutAtlas{source: id, frames: 1}, original)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		entries = append(entries, resource{target, data})
	}
	return entries, nil
}

// Keep the original score rows and double the width painted into the HUD.
func buildRatingImage(stock uiStock, art uiArtwork, instance uint32) (resource, error) {
	id := resourceID{uiImage, catalogFrameGroup, instance}
	target := resourceID{uiImage, mayorRatingGroup, instance}
	if _, exists := stock.index[target]; exists {
		return resource{}, fmt.Errorf("mayor rating image key collision: %s", target)
	}
	data, err := stock.read(id)
	if err != nil {
		return resource{}, err
	}
	source, err := decodeRGBA(data)
	if err != nil {
		return resource{}, err
	}
	if source.Bounds() != image.Rect(0, 0, 102, 26) {
		return resource{}, fmt.Errorf("unexpected mayor rating image bounds: %s", source.Bounds())
	}
	data, err = art.upscaleScoreRows(id, data)
	return resource{target, data}, err
}

// Use inferred coverage with the stock cursor colors and transparent key.
func (a uiArtwork) upscalePollArrows(id resourceID, original []byte) ([]byte, error) {
	source, err := decodeRGBA(original)
	if err != nil {
		return nil, err
	}
	if source.Bounds() != image.Rect(0, 0, 42, 9) {
		return nil, fmt.Errorf("unexpected poll arrow atlas bounds: %s", source.Bounds())
	}
	projected, err := a.upscale(layoutAtlas{source: id, frames: 6, project: true}, original)
	if err != nil {
		return nil, err
	}
	result, err := decodeRGBA(projected)
	if err != nil {
		return nil, err
	}
	for cell := range 6 {
		ink := source.NRGBAAt(7*cell+3, 4)
		if ink.A != 255 || ink == uiColorKey {
			return nil, fmt.Errorf("unexpected poll arrow color in cell %d", cell)
		}
		for y := range 18 {
			for x := 14 * cell; x < 14*(cell+1); x++ {
				pixel := color.NRGBA{R: 255, B: 255}
				if alpha := result.NRGBAAt(x, y).A; alpha >= 16 {
					pixel = ink
					pixel.A = alpha
				}
				result.SetNRGBA(x, y, pixel)
			}
		}
	}
	return encodeRGBA(result)
}

func resizeUIExact(original []byte, scaleX, scaleY int) ([]byte, error) {
	source, err := decodeRGBA(original)
	if err != nil {
		return nil, err
	}
	result := image.NewNRGBA(image.Rect(0, 0, scaleX*source.Rect.Dx(), scaleY*source.Rect.Dy()))
	for y := range result.Rect.Dy() {
		for x := range result.Rect.Dx() {
			result.SetNRGBA(x, y, source.NRGBAAt(x/scaleX, y/scaleY))
		}
	}
	return encodeRGBA(result)
}

// Score rows represent discrete values. Sample within each inferred row
// and keep the original transparency and color key at the new width.
func (a uiArtwork) upscaleScoreRows(id resourceID, original []byte) ([]byte, error) {
	source, err := decodeRGBA(original)
	if err != nil {
		return nil, err
	}
	width, height := source.Rect.Dx(), source.Rect.Dy()
	_, atlas, err := cachedUIAtlas(filepath.Join(a.cache, "ui-atlases-x4"), id, original, []int{width}, 4, a.infer)
	if err != nil {
		return nil, err
	}
	result := image.NewNRGBA(image.Rect(0, 0, 2*width, height))
	for y := range height {
		for x := range result.Rect.Dx() {
			stockPixel := source.NRGBAAt(x/2, y)
			pixel := stockPixel
			if stockPixel != uiColorKey && stockPixel.A != 0 {
				pixel = atlas.NRGBAAt(2*x+1, 4*y+2)
				pixel.A = stockPixel.A
			}
			result.SetNRGBA(x, y, pixel)
		}
	}
	return encodeRGBA(result)
}

// The News HTML lays out the icon as an 18×18 inline image. Keep that
// box and its click position while lowering the artwork to the HD baseline.
func buildNewsMarkers(stock uiStock, art uiArtwork) ([]resource, error) {
	const offsetY = 3
	entries := make([]resource, 0, newsMarkerCount)
	for i := uint32(0); i < newsMarkerCount; i++ {
		id := resourceID{uiImage, catalogFrameGroup, newsMarkerFirst + i}
		target := resourceID{uiImage, newsMarkerGroup, id.instance}
		if _, exists := stock.index[target]; exists {
			return nil, fmt.Errorf("News marker key collision: %s", target)
		}
		data, err := stock.read(id)
		if err != nil {
			return nil, err
		}
		source, err := decodeRGBA(data)
		if err != nil {
			return nil, err
		}
		if source.Bounds() != image.Rect(0, 0, 18, 18) {
			return nil, fmt.Errorf("unexpected News marker bounds: %s", id)
		}
		background := source.NRGBAAt(0, 0)
		for y := 18 - offsetY; y < 18; y++ {
			for x := range 18 {
				if source.NRGBAAt(x, y) != background {
					return nil, fmt.Errorf("News marker has artwork in the bottom margin: %s", id)
				}
			}
		}
		_, atlas, err := cachedUIAtlas(filepath.Join(art.cache, "ui-atlases-x4"), id, data, []int{18}, 4, art.infer)
		if err != nil {
			return nil, fmt.Errorf("News marker %s: %w", id, err)
		}
		keyed := hasColorKey(source)
		projected := projectAtlasCell(projectAtlasCell(atlas, keyed), keyed)
		for y := range 18 - offsetY {
			for x := range 18 {
				stockPixel := source.NRGBAAt(x, y)
				pixel := stockPixel
				if stockPixel != background && stockPixel != uiColorKey && stockPixel.A != 0 {
					pixel = projected.NRGBAAt(x, y)
					pixel.A = stockPixel.A
				}
				source.SetNRGBA(x, y, pixel)
			}
		}
		result := image.NewNRGBA(source.Bounds())
		draw.Draw(result, image.Rect(0, 0, 18, offsetY), image.NewUniform(background), image.Point{}, draw.Src)
		draw.Draw(result, image.Rect(0, offsetY, 18, 18), source, image.Point{}, draw.Src)
		data, err = encodeRGBA(result)
		if err != nil {
			return nil, err
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
	// Keep a screen-density fallback for the runtime compositor.
	data, err := art.upscale(layoutAtlas{source: id, frames: 1, project: true}, first)
	if err != nil {
		return nil, err
	}
	entries := []resource{
		{resourceID{uiImage, catalogFrameGroup, disasterBackgroundHD}, data},
		{resourceID{uiImage, mySimPortraitGroup, disasterBackgroundHD}, data},
	}
	if art.scale == 4 {
		target := resourceID{uiImage, catalogFrameGroup, disasterBackgroundX4}
		if _, exists := stock.index[target]; exists {
			return nil, fmt.Errorf("Disasters background key collision: %s", target)
		}
		dense, err := art.upscale(layoutAtlas{source: id, frames: 1}, first)
		if err != nil {
			return nil, err
		}
		entries = append(entries, resource{target, dense})
	}
	return entries, nil
}

type uiPackage struct {
	name    string
	entries []resource
}

func runBuildUI(args []string) (result error) {
	flags := flag.NewFlagSet("ui", flag.ContinueOnError)
	textureScale := flags.Int("texture-scale", 4, "UI texture scale (2 or 4)")
	hdCursors := flags.Bool("hd-cursors", false, "build experimental HD game cursors")
	regionMap := flags.Bool("region-map-2x", false, "double the region map")
	loadingPreview := flags.Bool("loading-preview-2x", false, "double the city loading preview")
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
	executable, err = patchUIExecutable(executable, *textureScale, uiExecutableOptions{
		hdCursors: *hdCursors, loadingPreview: *loadingPreview, regionMap: *regionMap,
	})
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
	var cursors []resource
	if *hdCursors {
		cursors, err = cursorEntries(stock, art)
		if err != nil {
			return fmt.Errorf("build cursors: %w", err)
		}
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
	if len(cursors) != 0 {
		packages = append(packages, uiPackage{"cursors-hd.dat", cursors})
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
