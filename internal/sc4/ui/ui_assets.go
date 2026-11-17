package ui

import (
	"bytes"
	"context"
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
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/uiids"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

const mySimPortraitGroup = 0x1abe787d

var chromeInstances = []uint32{
	0x14215ed0, 0x14215ed1, 0x14215ed2, 0x14215ed3, 0x14215ed4, 0x14215ed5, 0x14215edd,
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
		options := atlasOptions{enlargedKey: scale == 2}
		art, err := preparedUIAtlas(cache, id, data, widths, scale, options, infer)
		if err != nil {
			return nil, err
		}

		entries = append(entries, dbpf.Entry{ID: id, Data: art})
	}
	report(len(chromeInstances))
	return entries, nil
}

func mySimPortraitEntries(stock *dbpf.Archive, cache string, _ int, infer func([]byte) ([]byte, error), progress upscale.Progress) ([]dbpf.Entry, error) {
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
		// The picker copies portraits at screen density into fixed-size cells.
		options := atlasOptions{enlargedKey: true, variant: "portrait-transparency", finish: func(pixels []byte) ([]byte, error) {
			return validatePortraitKey(data, pixels)
		}}
		art, err := preparedUIAtlas(cache, id, data, []int{36}, 2, options, infer)
		if err != nil {
			return nil, err
		}

		entries = append(entries, dbpf.Entry{ID: dbpf.ID{Kind: uiImage, Group: mySimPortraitGroup, Instance: instance}, Data: art}, dbpf.Entry{ID: id, Data: art})
	}
	report(len(instances))
	return entries, nil
}

func validatePortraitKey(original, prepared []byte) ([]byte, error) {
	source, err := textures.DecodeRGBA(original)
	if err != nil {
		return nil, err
	}
	if hasColorKey(source) {
		atlas, err := textures.DecodeRGBA(prepared)
		if err != nil {
			return nil, err
		}
		for y := range atlas.Rect.Dy() {
			for x := range atlas.Rect.Dx() {
				if (source.NRGBAAt(x/2, y/2) == uiColorKey) != (atlas.NRGBAAt(x, y) == uiColorKey) {
					return nil, errors.New("My Sim portrait transparency key changed")
				}
			}
		}
	}
	return prepared, nil
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
