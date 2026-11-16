package ui

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

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/uiids"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

const mySimPortraitGroup = 0x1abe787d

var chromeInstances = []uint32{
	0x14215ed0, 0x14215ed1, 0x14215ed2, 0x14215ed3, 0x14215ed4, 0x14215ed5, 0x14215edd,
}

func cachedUIAtlas(cache string, id dbpf.ID, stock []byte, widths []int, scale int,
	infer func([]byte) ([]byte, error)) ([]byte, *image.NRGBA, error) {
	source, err := textures.DecodeRGBA(stock)
	if err != nil {
		return nil, nil, err
	}
	path := uiCachePath(cache, id, stock, len(widths), scale, hasColorKey(source))
	art, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && infer != nil {
		atlas, inferenceErr := inferAtlasCells(source, widths, scale, infer)
		if inferenceErr != nil {
			return nil, nil, inferenceErr
		}
		art, err = textures.EncodeRGBA(atlas)
		if err == nil {
			err = gamefiles.PublishFile(path, func(w io.Writer) error {
				_, err := w.Write(art)
				return err
			})
		}
	}
	if err != nil {
		return nil, nil, err
	}
	atlas, err := textures.DecodeRGBA(art)
	if err != nil {
		return nil, nil, err
	}
	if atlas.Rect.Dx() != scale*source.Rect.Dx() || atlas.Rect.Dy() != scale*source.Rect.Dy() {
		return nil, nil, fmt.Errorf("%s: invalid prepared atlas dimensions", path)
	}
	return art, atlas, nil
}

func catalogChromeEntries(stock *dbpf.Archive, cache string, scale int, infer func([]byte) ([]byte, error), progress upscale.Progress) ([]dbpf.Entry, error) {
	widths := []int{80, 53, 53, 53, 53}
	entries := make([]dbpf.Entry, 0, len(chromeInstances))
	report := progress.Track(len(chromeInstances))
	for i, instance := range chromeInstances {
		report(i)
		id := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: instance}
		index := slices.IndexFunc(stock.Records, func(r dbpf.Record) bool { return r.ID == id })
		if index < 0 {
			return nil, fmt.Errorf("missing catalog frame %s", id)
		}
		data, err := stock.Read(stock.Records[index])
		if err != nil {
			return nil, err
		}
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width != 292 || config.Height != 53 {
			return nil, fmt.Errorf("%s: invalid stock catalog frame", id)
		}
		art, _, err := cachedUIAtlas(cache, id, data, widths, 4, infer)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		if scale == 2 {
			art, err = projectUIArtwork(data, art, widths, true)
			if err != nil {
				return nil, err
			}
		}
		entries = append(entries, dbpf.Entry{ID: id, Data: art})
	}
	report(len(chromeInstances))
	return entries, nil
}

func mySimPortraitEntries(stock *dbpf.Archive, cache string, scale int, infer func([]byte) ([]byte, error), progress upscale.Progress) ([]dbpf.Entry, error) {
	pairs := make(map[uint32]map[uint32][]byte)
	for _, r := range stock.Records {
		id := r.ID
		if id.Kind != uiImage || (id.Group != mySimPortraitGroup && id.Group != uiids.CatalogFrameGroup) {
			continue
		}
		data, err := stock.Read(r)
		if err != nil {
			return nil, err
		}
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width != 36 || config.Height != 41 {
			continue
		}
		if pairs[id.Instance] == nil {
			pairs[id.Instance] = make(map[uint32][]byte)
		}
		pairs[id.Instance][id.Group] = data
	}
	instances := make([]uint32, 0, len(pairs))
	for instance := range pairs {
		instances = append(instances, instance)
	}
	if len(instances) == 0 {
		return nil, errors.New("no My Sim portrait pairs found")
	}
	slices.Sort(instances)
	entries := make([]dbpf.Entry, 0, 2*len(instances))
	report := progress.Track(len(instances))
	for i, instance := range instances {
		report(i)
		pair := pairs[instance]
		data := pair[uiids.CatalogFrameGroup]
		if data == nil || !bytes.Equal(data, pair[mySimPortraitGroup]) {
			return nil, fmt.Errorf("%08x: stock My Sim portrait pair differs", instance)
		}
		id := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: instance}
		art, atlas, err := cachedUIAtlas(cache, id, data, []int{36}, scale, infer)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		source, err := textures.DecodeRGBA(data)
		if err != nil {
			return nil, err
		}
		if hasColorKey(source) {
			for y := range atlas.Rect.Dy() {
				for x := range atlas.Rect.Dx() {
					if (source.NRGBAAt(x/scale, y/scale) == uiColorKey) != (atlas.NRGBAAt(x, y) == uiColorKey) {
						return nil, fmt.Errorf("%s: My Sim portrait transparency key changed", id)
					}
				}
			}
		}
		// The picker copies portraits at screen density into fixed-size cells.
		if scale == 4 {
			art, err = projectUIArtwork(data, art, []int{36}, true)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
		}
		entries = append(entries, dbpf.Entry{ID: dbpf.ID{Kind: uiImage, Group: mySimPortraitGroup, Instance: instance}, Data: art}, dbpf.Entry{ID: id, Data: art})
	}
	report(len(instances))
	return entries, nil
}

func RunUIAssets(name string, args []string) (result error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	readInferenceConfig := upscale.InferenceFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	config, err := readInferenceConfig()
	if err != nil {
		return err
	}
	if flags.NArg() != 3 {
		return fmt.Errorf("usage: simcity4-uhd %s [OPTIONS] GAME_DIR CACHE NEW_OUTPUT_DAT", name)
	}
	build := catalogChromeEntries
	if name == "mysim-portraits" {
		build = mySimPortraitEntries
	}
	stock, err := dbpf.Open(filepath.Join(flags.Arg(0), "SimCity_1.dat"))
	if err != nil {
		return err
	}
	defer stock.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var worker *upscale.InferenceWorker
	defer func() {
		if worker != nil {
			if result != nil {
				cancel()
			}
			result = errors.Join(result, worker.Close())
		}
	}()
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
	infer = upscale.CachedInference(ctx, flags.Arg(1), config, infer)
	entries, err := build(stock, flags.Arg(1), 4, infer, upscale.Progress{})
	if err != nil {
		return err
	}
	return gamefiles.PublishFile(flags.Arg(2), func(w io.Writer) error { return dbpf.Write(w, entries) })
}
