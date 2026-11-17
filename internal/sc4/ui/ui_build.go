package ui

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

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	translation "github.com/Feverqwe/simcity4-hd/internal/sc4/russian"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/uiids"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

const (
	budgetDetailsImage    = 0x140155b7
	budgetCheckboxImage   = 0x144161ea
	playlistCheckboxImage = 0x14416244
	budgetSliderGroup     = 0xe74a40c6
	budgetSliderImage     = 0x46a006a7
	mayorRatingImage      = 0x14015549
	mayorTrendImage       = 0x1401554b
	regionRatingImage     = 0x14416327
	previewGroup          = 0x4c06f888
	previewHDGroup        = 0xe74a40c0
	disasterBackground    = 0x8a2564a2
	disasterBackgroundHD  = 0xe74a40c3
)

type uiArtwork struct {
	cache     string
	scale     int
	infer     func([]byte) ([]byte, error)
	processed func()
}

func (a uiArtwork) upscale(spec layoutAtlas, original []byte) ([]byte, error) {
	return a.upscaleAtlas(spec, original, atlasOptions{})
}

func (a uiArtwork) upscaleAtlas(spec layoutAtlas, original []byte, options atlasOptions) ([]byte, error) {
	config, err := png.DecodeConfig(bytes.NewReader(original))
	if err != nil {
		return nil, err
	}
	if spec.frames <= 0 || config.Width%spec.frames != 0 {
		return nil, fmt.Errorf("invalid UI atlas frame geometry")
	}
	widths := slices.Repeat([]int{config.Width / spec.frames}, spec.frames)
	scale := a.scale
	if spec.project {
		scale = 2
	}
	options.originalAlpha = scale == 2 && spec.source == (dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: disasterBackground})
	data, err := preparedUIAtlas(a.cache, spec.source, original, widths, scale, options, a.infer)
	if err == nil && a.processed != nil {
		a.processed()
	}
	return data, err
}

func buildHUD(game string, stock uiStock, art uiArtwork, windows bool) ([]dbpf.Entry, error) {
	if windows {
		intro, err := dbpf.Open(filepath.Join(game, "Intro.dat"))
		if err != nil {
			return nil, err
		}
		defer intro.Close()
		for _, r := range intro.Records {
			if r.ID.Kind != uiImage || r.ID.Group != uiids.CatalogFrameGroup ||
				(r.ID.Instance != 0xea7f0eaf && r.ID.Instance != 0xea7f0eae) {
				continue
			}
			if _, exists := stock.index[r.ID]; !exists {
				stock.index[r.ID] = gamefiles.ResourceLocation{Archive: intro, Record: r}
			}
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
	layouts, config, err := buildWindowEntries(stock, config, windows, art)
	if err != nil {
		return nil, err
	}
	entries = append(entries, layouts...)
	if !windows {
		scrollbar, err := aspyrScrollbarArt(stock)
		if err != nil {
			return nil, err
		}
		entries = append(entries, scrollbar...)
	}
	config, err = scaleRuntimeFonts(config, string(font), true)
	if err != nil {
		return nil, err
	}
	markers, err := buildNewsMarkers(stock, art)
	if err != nil {
		return nil, err
	}
	entries = append(entries, markers...)
	for _, instance := range []uint32{mayorRatingImage, regionRatingImage} {
		entry, err := buildRatingImage(stock, art.cache, instance)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	trendID := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: mayorTrendImage}
	trendTarget := dbpf.ID{Kind: uiImage, Group: uiids.MayorRatingGroup, Instance: mayorTrendImage}
	if _, exists := stock.index[trendTarget]; exists {
		return nil, fmt.Errorf("mayor trend image key collision: %s", trendTarget)
	}
	trend, err := stock.read(trendID)
	if err != nil {
		return nil, err
	}
	trend, err = resizeUIExact(art.cache, trend, 2, 2)
	if err != nil {
		return nil, fmt.Errorf("mayor trend arrows: %w", err)
	}
	entries = append(entries, dbpf.Entry{ID: trendTarget, Data: trend})
	// Runtime budget and playlist controls use atlases at screen density.
	for _, atlas := range []struct {
		source, target, group uint32
		frames                int
	}{
		{budgetSliderImage, budgetSliderImage, budgetSliderGroup, 4},
		{budgetCheckboxImage, budgetCheckboxImage, uiids.BudgetRowArtGroup, 8},
		{playlistCheckboxImage, playlistCheckboxImage, uiids.BudgetRowArtGroup, 8},
		{budgetDetailsImage, budgetDetailsImage, uiids.BudgetRowArtGroup, 4},
		{uiids.BudgetCloseImage, uiids.BudgetCloseHD, uiids.BudgetRowArtGroup, 4},
	} {
		source := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: atlas.source}
		target := dbpf.ID{Kind: uiImage, Group: atlas.group, Instance: atlas.target}
		if _, exists := stock.index[target]; exists {
			return nil, fmt.Errorf("runtime image key collision: %s", target)
		}
		original, err := stock.read(source)
		if err != nil {
			return nil, err
		}
		var options atlasOptions
		if atlas.source == playlistCheckboxImage {
			options.variant = "playlist-checkbox"
			options.finish = cropPlaylistCheckbox
		}
		data, err := art.upscaleAtlas(layoutAtlas{source: source, frames: atlas.frames, project: true}, original, options)
		if err != nil {
			return nil, fmt.Errorf("runtime image %s: %w", source, err)
		}

		entries = append(entries, dbpf.Entry{ID: target, Data: data})
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
	for instance := uint32(0x1441624a); instance <= 0x1441624c; instance++ {
		source := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: instance}
		target := dbpf.ID{Kind: uiImage, Group: uiids.ImportIconGroup, Instance: instance}
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
		entries = append(entries, dbpf.Entry{ID: target, Data: data})
	}
	entries = append(entries, dbpf.Entry{ID: uiFontID, Data: []byte(config)})
	restoreID := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: 0x53244588}
	data, err := stock.read(restoreID)
	if err != nil {
		return nil, err
	}
	data, err = art.upscale(layoutAtlas{source: restoreID, frames: 4}, data)
	if err != nil {
		return nil, err
	}
	entries = append(entries, dbpf.Entry{ID: dbpf.ID{Kind: uiImage, Group: 0xe74a40bd, Instance: restoreID.Instance}, Data: data})

	// The polls renderer samples discrete score rows and draws the arrows
	// at native size. Keep each arrow in its original color.
	fillID := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: 0x14015580}
	arrowID := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: 0x14015584}
	fillTarget := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: 0x2bd260ac}
	arrowTarget := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: 0x2bd260ab}
	for _, target := range []dbpf.ID{fillTarget, arrowTarget} {
		if _, exists := stock.index[target]; exists {
			return nil, fmt.Errorf("polls image key collision: %s", target)
		}
	}
	fill, err := stock.read(fillID)
	if err != nil {
		return nil, err
	}
	fill, err = resizeUIExact(art.cache, fill, 2, 1)
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
	entries = append(entries, dbpf.Entry{ID: fillTarget, Data: fill}, dbpf.Entry{ID: arrowTarget, Data: arrows})

	var previews []uint32
	for id := range stock.index {
		if id.Kind == uiImage && id.Group == previewGroup {
			previews = append(previews, id.Instance)
		}
	}
	if len(previews) == 0 {
		return nil, errors.New("no stock My Sims previews found")
	}
	slices.Sort(previews)
	for _, instance := range previews {
		id := dbpf.ID{Kind: uiImage, Group: previewGroup, Instance: instance}
		target := dbpf.ID{Kind: uiImage, Group: previewHDGroup, Instance: instance}
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
		entries = append(entries, dbpf.Entry{ID: target, Data: data})
	}
	return entries, nil
}

// Keep the original score rows and double the width painted into the HUD.
func buildRatingImage(stock uiStock, cache string, instance uint32) (dbpf.Entry, error) {
	id := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: instance}
	target := dbpf.ID{Kind: uiImage, Group: uiids.MayorRatingGroup, Instance: instance}
	if _, exists := stock.index[target]; exists {
		return dbpf.Entry{}, fmt.Errorf("mayor rating image key collision: %s", target)
	}
	data, err := stock.read(id)
	if err != nil {
		return dbpf.Entry{}, err
	}
	source, err := textures.DecodeRGBA(data)
	if err != nil {
		return dbpf.Entry{}, err
	}
	if source.Bounds() != image.Rect(0, 0, 102, 26) {
		return dbpf.Entry{}, fmt.Errorf("unexpected mayor rating image bounds: %s", source.Bounds())
	}
	data, err = resizeUIExact(cache, data, 2, 1)
	return dbpf.Entry{ID: target, Data: data}, err
}

func cropPlaylistCheckbox(data []byte) ([]byte, error) {
	atlas, err := textures.DecodeRGBA(data)
	if err != nil {
		return nil, err
	}
	if atlas.Bounds() != image.Rect(0, 0, 256, 32) {
		return nil, fmt.Errorf("unexpected playlist atlas bounds: %s", atlas.Bounds())
	}
	// Trim the padding to match the playlist's 28-pixel rows.
	cropped := image.NewNRGBA(image.Rect(0, 0, 256, 28))
	draw.Draw(cropped, cropped.Bounds(), atlas, image.Pt(0, 2), draw.Src)
	return textures.EncodeRGBA(cropped)
}

// Use inferred coverage with the stock cursor colors and transparent key.
func (a uiArtwork) upscalePollArrows(id dbpf.ID, original []byte) ([]byte, error) {
	options := atlasOptions{variant: "poll-arrow-colors", finish: func(data []byte) ([]byte, error) {
		return colorPollArrows(original, data)
	}}
	return a.upscaleAtlas(layoutAtlas{source: id, frames: 6, project: true}, original, options)
}

func colorPollArrows(original, projected []byte) ([]byte, error) {
	source, err := textures.DecodeRGBA(original)
	if err != nil {
		return nil, err
	}
	if source.Bounds() != image.Rect(0, 0, 42, 9) {
		return nil, fmt.Errorf("unexpected poll arrow atlas bounds: %s", source.Bounds())
	}
	result, err := textures.DecodeRGBA(projected)
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
	return textures.EncodeRGBA(result)
}

func resizeUIExact(cache string, original []byte, scaleX, scaleY int) ([]byte, error) {
	scale := 1
	if scaleX == scaleY {
		scale = scaleX
	}
	path := preparedPNGPath(cache, scale, fmt.Sprintf("pixel-copy/%dx%d", scaleX, scaleY), original)
	return cachedPreparedPNG(path, func() ([]byte, error) {
		return scalePixels(original, scaleX, scaleY)
	})
}

func scalePixels(original []byte, scaleX, scaleY int) ([]byte, error) {
	source, err := textures.DecodeRGBA(original)
	if err != nil {
		return nil, err
	}
	result := image.NewNRGBA(image.Rect(0, 0, scaleX*source.Rect.Dx(), scaleY*source.Rect.Dy()))
	for y := range result.Rect.Dy() {
		for x := range result.Rect.Dx() {
			result.SetNRGBA(x, y, source.NRGBAAt(x/scaleX, y/scaleY))
		}
	}
	return textures.EncodeRGBA(result)
}

// The News HTML lays out the icon as an 18×18 inline image. Keep that
// box and its click position while lowering the artwork to the HD baseline.
func buildNewsMarkers(stock uiStock, art uiArtwork) ([]dbpf.Entry, error) {
	entries := make([]dbpf.Entry, 0, uiids.NewsMarkerCount)
	for i := uint32(0); i < uiids.NewsMarkerCount; i++ {
		id := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: uiids.NewsMarkerFirst + i}
		target := dbpf.ID{Kind: uiImage, Group: uiids.NewsMarkerGroup, Instance: id.Instance}
		if _, exists := stock.index[target]; exists {
			return nil, fmt.Errorf("News marker key collision: %s", target)
		}
		data, err := stock.read(id)
		if err != nil {
			return nil, err
		}
		path := preparedPNGPath(art.cache, 1, "news-marker/"+id.String(), data)
		data, err = cachedPreparedPNG(path, func() ([]byte, error) {
			return buildNewsMarker(id, data, art)
		})
		if err != nil {
			return nil, fmt.Errorf("News marker %s: %w", id, err)
		}

		entries = append(entries, dbpf.Entry{ID: target, Data: data})
	}
	return entries, nil
}

func buildNewsMarker(id dbpf.ID, data []byte, art uiArtwork) ([]byte, error) {
	const offsetY = 3
	source, err := textures.DecodeRGBA(data)
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
	atlasData, err := renderX4Atlas(art.cache, id, data, []int{18}, art.infer)
	if err != nil {
		return nil, err
	}
	atlas, err := textures.DecodeRGBA(atlasData)
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
	return textures.EncodeRGBA(result)
}

func disasterEntries(stock uiStock, art uiArtwork, mac bool) ([]dbpf.Entry, error) {
	id := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: disasterBackground}
	first, err := stock.read(id)
	if err != nil {
		return nil, err
	}
	// Keep the screen-density art for ×2 and as a fallback if the dense lookup fails.
	data, err := art.upscale(layoutAtlas{source: id, frames: 1, project: true}, first)
	if err != nil {
		return nil, err
	}
	entries := []dbpf.Entry{
		{ID: dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: disasterBackgroundHD}, Data: data},
	}
	// The Windows loader requests the background from catalogFrameGroup.
	if mac {
		second, err := stock.read(dbpf.ID{Kind: uiImage, Group: mySimPortraitGroup, Instance: disasterBackground})
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(first, second) {
			return nil, errors.New("stock Disasters background copies differ")
		}
		entries = append(entries, dbpf.Entry{ID: dbpf.ID{Kind: uiImage, Group: mySimPortraitGroup, Instance: disasterBackgroundHD}, Data: data})
	}
	if art.scale == 4 {
		target := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: uiids.DisasterBackgroundX4}
		if _, exists := stock.index[target]; exists {
			return nil, fmt.Errorf("Disasters background key collision: %s", target)
		}
		dense, err := art.upscale(layoutAtlas{source: id, frames: 1}, first)
		if err != nil {
			return nil, err
		}
		entries = append(entries, dbpf.Entry{ID: target, Data: dense})
	}
	return entries, nil
}

type uiPackage struct {
	name    string
	entries []dbpf.Entry
}

func Run(args []string) error {
	return BuildWithProgress(context.Background(), args, upscale.Progress{})
}

func BuildWithProgress(ctx context.Context, args []string, progress upscale.Progress) (result error) {
	return BuildFromExecutable(ctx, args, progress, nil)
}

func BuildFromExecutable(ctx context.Context, args []string, progress upscale.Progress, executable []byte) (result error) {
	flags := flag.NewFlagSet("ui", flag.ContinueOnError)
	textureScale := flags.Int("texture-scale", 4, "UI texture scale (2 or 4)")
	hdCursors := flags.Bool("hd-cursors", false, "build experimental HD game cursors")
	cursorScale := flags.Int("cursor-scale", 2, "HD cursor size (2 or 4)")
	regionMap := flags.Bool("region-map-2x", false, "double the region map")
	loadingPreview := flags.Bool("loading-preview-2x", false, "double the city loading preview")
	russian := flags.String("russian", "", "directory with the Russian locale and replacement fonts")
	readInferenceConfig := upscale.InferenceFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	config, err := readInferenceConfig()
	if err != nil {
		return err
	}
	if flags.NArg() != 3 {
		return errors.New("usage: simcity4-uhd ui [OPTIONS] GAME_DIR CACHE NEW_OUTPUT_DIR")
	}
	if *textureScale != 2 && *textureScale != 4 {
		return errors.New("texture scale must be 2 or 4")
	}
	if *cursorScale != 2 && *cursorScale != 4 {
		return errors.New("cursor scale must be 2 or 4")
	}
	game, cache, output := flags.Arg(0), flags.Arg(1), flags.Arg(2)
	bundle := game
	game, mac, err := gamefiles.DataDirectory(bundle)
	if err != nil {
		return err
	}
	locale := filepath.Join(game, "SimCityLocale.DAT")
	executableName := "SimCity4-hd.exe"
	if mac {
		locale = filepath.Join(game, "English", "SimCityLocale.dat")
		executableName = gamefiles.MacGameExecutable
	}
	var russianText []dbpf.Entry
	var fonts map[string][]byte
	if *russian != "" {
		locale = filepath.Join(*russian, translation.LocaleFile)
		fonts, err = readRussianFonts(*russian)
		if err != nil {
			return err
		}
		russianText, err = russianUI(locale)
		if err != nil {
			return err
		}
	}
	tutorial, err := tutorialEntries(locale)
	if err != nil {
		return err
	}
	credits, err := creditsEntries(locale)
	if err != nil {
		return err
	}
	// Validate the executable fingerprint before doing potentially lengthy inference.
	if executable == nil {
		executable, err = os.ReadFile(gamefiles.ExecutablePath(bundle, mac))
		if err != nil {
			return err
		}
	}
	executable, err = exepatch.Patch(executable, mac, *textureScale, x86.Options{
		HdCursors: *hdCursors, LoadingPreview: *loadingPreview, RegionMap: *regionMap,
		RussianBudgetTips: *russian != "", CursorScale: *cursorScale,
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

	config.Prepare = progress.Prepare
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var worker *upscale.InferenceWorker
	closeWorker := func() error {
		if worker == nil {
			return nil
		}
		err := worker.Close()
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
	if config.Enabled() {
		infer = func(data []byte) ([]byte, error) {
			if worker == nil {
				var err error
				worker, err = upscale.StartInference(ctx, config, os.Stderr)
				if err != nil {
					return nil, err
				}
			}
			return worker.UpscalePNG(data)
		}
	}
	infer = upscale.CachedInference(ctx, cache, config, infer)
	stock, err := dbpf.Open(filepath.Join(game, "SimCity_1.dat"))
	if err != nil {
		return err
	}
	defer stock.Close()
	stockUI := indexUIStock(stock)
	progress.Report("City interface")
	art := uiArtwork{cache: cache, scale: *textureScale, infer: infer, processed: progress.Count()}
	hud, err := buildHUD(game, stockUI, art, !mac)
	if err != nil {
		return fmt.Errorf("build HUD: %w", err)
	}
	help, err := keyboardHelpEntries(stockUI, *russian != "", mac)
	if err != nil {
		return fmt.Errorf("build keyboard shortcuts: %w", err)
	}
	hud = append(hud, help...)
	displayText, err := graphicsResolutionStrings(*russian != "", mac)
	if err != nil {
		return fmt.Errorf("build display settings strings: %w", err)
	}
	hud = append(hud, displayText...)
	progress.Report("Driving gauges")
	gauges, err := driveGaugeEntries(stockUI, art)
	if err != nil {
		return err
	}
	hud = append(hud, gauges...)
	runtimeChrome, err := runtimeChromeEntries(stockUI, art)
	if err != nil {
		return err
	}
	hud = append(hud, runtimeChrome...)
	progress.Report("Building catalog")
	square, bridge, err := catalogSheetEntries(game, cache, *textureScale, infer, progress)
	if err != nil {
		return fmt.Errorf("build catalog sheets: %w", err)
	}
	progress.Report("Catalog frames")
	chrome, err := catalogChromeEntries(stock, cache, *textureScale, infer, progress)
	if err != nil {
		return fmt.Errorf("build catalog frames: %w", err)
	}
	progress.Report("Disasters background and cursors")
	art.processed = progress.Count()
	disasters, err := disasterEntries(stockUI, art, mac)
	if err != nil {
		return fmt.Errorf("build Disasters background: %w", err)
	}
	var cursors []dbpf.Entry
	if *hdCursors {
		cursors, err = cursorEntries(stock, art, *cursorScale, mac)
		if err != nil {
			return fmt.Errorf("build cursors: %w", err)
		}
	}
	suffix := fmt.Sprintf("x%d", *textureScale)
	progress.Report("Portraits")
	// The picker always consumes screen-density portraits. Both packages
	// can use the same projected artwork and inference cache.
	portraits, err := mySimPortraitEntries(stock, cache, 4, infer, progress)
	if err != nil {
		return fmt.Errorf("build My Sims portraits: %w", err)
	}
	if err := closeWorker(); err != nil {
		return err
	}
	packages := []uiPackage{
		{"city-hud-" + suffix + ".dat", hud},
		{"catalog-square-" + suffix + ".dat", square},
		{"catalog-bridge-" + suffix + ".dat", bridge},
		{"catalog-chrome-" + suffix + ".dat", chrome},
		{"disasters-background-" + suffix + ".dat", disasters},
		{"mysim-portraits-" + suffix + ".dat", portraits},
	}
	if *russian == "" {
		packages = append(packages, uiPackage{"tutorial-text-size-7.dat", tutorial})
		if len(credits) != 0 {
			packages = append(packages, uiPackage{"credits-text.dat", credits})
		}
	} else {
		russianBudgetTooltips(hud)
		captions, err := russianStaticCaptionEntries(hud)
		if err != nil {
			return err
		}
		russianText = append(russianText, captions...)
		resized := make(map[dbpf.ID][]byte, len(tutorial)+len(credits))
		for _, entry := range append(tutorial, credits...) {
			resized[entry.ID] = entry.Data
		}
		for i := range russianText {
			entry := &russianText[i]
			if data := resized[entry.ID]; data != nil {
				entry.Data = data
			}
			entry.Data, err = normalizeRussianText(entry.Data)
			if err != nil {
				return fmt.Errorf("Russian locale %s: %w", entry.ID, err)
			}
		}
		captions, err = russianButtonCaptionEntries(hud)
		if err != nil {
			return err
		}
		russianText = append(russianText, captions...)
		packages = append(packages, uiPackage{RussianTextPackage, russianText})
	}
	if len(cursors) != 0 {
		name := "cursors-hd.dat"
		if *cursorScale == 4 {
			name = "cursors-hd-x4.dat"
		}
		packages = append(packages, uiPackage{name, cursors})
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	progress.Report("Packing interface files")
	return publishUI(output, executableName, executable, packages, fonts)
}

func publishUI(output, executableName string, executable []byte, packages []uiPackage, fonts map[string][]byte) error {
	owners := make(map[dbpf.ID]string)
	for _, p := range packages {
		for _, entry := range p.entries {
			if previous := owners[entry.ID]; previous != "" {
				return fmt.Errorf("UI resource %s appears in %s and %s", entry.ID, previous, p.name)
			}
			owners[entry.ID] = p.name
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
		if err := gamefiles.PublishFile(filepath.Join(plugins, p.name), func(w io.Writer) error {
			return dbpf.Write(w, p.entries)
		}); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(stage, executableName), executable, 0755); err != nil {
		return err
	}

	if len(fonts) != 0 {
		dir := filepath.Join(stage, "Fonts")
		if err := os.Mkdir(dir, 0755); err != nil {
			return err
		}
		for name, data := range fonts {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
				return err
			}
		}
	}

	if err := os.Rename(stage, output); err != nil {
		return err
	}
	fmt.Printf("%d UI packages, %d resources\n", len(packages), len(owners))
	return nil
}
