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

const catalogFrameGroup = 0x46a006b0
const mySimPortraitGroup = 0x1abe787d

var chromeInstances = []uint32{
	0x14215ed0, 0x14215ed1, 0x14215ed2, 0x14215ed3, 0x14215ed4, 0x14215ed5, 0x14215edd,
}

func cachedUIAtlas(cache string, id resourceID, stock []byte, widths []int,
	infer func([]byte) ([]byte, error)) ([]byte, *image.NRGBA, error) {
	source, err := decodeRGBA(stock)
	if err != nil {
		return nil, nil, err
	}
	path := uiCachePath(cache, id, stock, len(widths), hasColorKey(source))
	art, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && infer != nil {
		atlas, inferenceErr := inferAtlasCells(source, widths, 4, infer)
		if inferenceErr != nil {
			return nil, nil, inferenceErr
		}
		art, err = encodeRGBA(atlas)
		if err == nil {
			err = publishFile(path, func(w io.Writer) error {
				_, err := w.Write(art)
				return err
			})
		}
	}
	if err != nil {
		return nil, nil, err
	}
	atlas, err := decodeRGBA(art)
	if err != nil {
		return nil, nil, err
	}
	if atlas.Rect.Dx() != 4*source.Rect.Dx() || atlas.Rect.Dy() != 4*source.Rect.Dy() {
		return nil, nil, fmt.Errorf("%s: invalid prepared atlas dimensions", path)
	}
	return art, atlas, nil
}

func catalogChromeEntries(stock *archive, cache string, infer func([]byte) ([]byte, error)) ([]resource, error) {
	widths := []int{80, 53, 53, 53, 53}
	entries := make([]resource, 0, len(chromeInstances))
	for _, instance := range chromeInstances {
		id := resourceID{uiImage, catalogFrameGroup, instance}
		index := slices.IndexFunc(stock.records, func(r record) bool { return r.id == id })
		if index < 0 {
			return nil, fmt.Errorf("missing catalog frame %s", id)
		}
		data, err := stock.read(stock.records[index])
		if err != nil {
			return nil, err
		}
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width != 292 || config.Height != 53 {
			return nil, fmt.Errorf("%s: invalid stock catalog frame", id)
		}
		data, _, err = cachedUIAtlas(cache, id, data, widths, infer)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		entries = append(entries, resource{id, data})
	}
	return entries, nil
}

func mySimPortraitEntries(stock *archive, cache string, infer func([]byte) ([]byte, error)) ([]resource, error) {
	pairs := make(map[uint32]map[uint32][]byte)
	for _, r := range stock.records {
		id := r.id
		if id.kind != uiImage || (id.group != mySimPortraitGroup && id.group != catalogFrameGroup) {
			continue
		}
		data, err := stock.read(r)
		if err != nil {
			return nil, err
		}
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width != 36 || config.Height != 41 {
			continue
		}
		if pairs[id.instance] == nil {
			pairs[id.instance] = make(map[uint32][]byte)
		}
		pairs[id.instance][id.group] = data
	}
	instances := make([]uint32, 0, len(pairs))
	for instance := range pairs {
		instances = append(instances, instance)
	}
	if len(instances) == 0 {
		return nil, errors.New("no My Sim portrait pairs found")
	}
	slices.Sort(instances)
	entries := make([]resource, 0, 2*len(instances))
	for _, instance := range instances {
		pair := pairs[instance]
		data := pair[catalogFrameGroup]
		if data == nil || !bytes.Equal(data, pair[mySimPortraitGroup]) {
			return nil, fmt.Errorf("%08x: stock My Sim portrait pair differs", instance)
		}
		id := resourceID{uiImage, catalogFrameGroup, instance}
		art, atlas, err := cachedUIAtlas(cache, id, data, []int{36}, infer)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		source, err := decodeRGBA(data)
		if err != nil {
			return nil, err
		}
		if hasColorKey(source) {
			for y := range atlas.Rect.Dy() {
				for x := range atlas.Rect.Dx() {
					if (source.NRGBAAt(x/4, y/4) == uiColorKey) != (atlas.NRGBAAt(x, y) == uiColorKey) {
						return nil, fmt.Errorf("%s: My Sim portrait transparency key changed", id)
					}
				}
			}
		}
		entries = append(entries, resource{resourceID{uiImage, mySimPortraitGroup, instance}, art}, resource{id, art})
	}
	return entries, nil
}

func runUIAssets(name string, args []string) (result error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	distro := flags.String("wsl-distro", "", "WSL distribution for inference")
	python := flags.String("wsl-python", "python3", "Python interpreter in WSL")
	weights := flags.String("wsl-weights", "", "checkpoint path in WSL for cache misses")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 3 {
		return fmt.Errorf("usage: simcity4-hd %s [OPTIONS] GAME_DIR CACHE NEW_OUTPUT_DAT", name)
	}
	build := catalogChromeEntries
	if name == "mysim-portraits" {
		build = mySimPortraitEntries
	}
	stock, err := openArchive(filepath.Join(flags.Arg(0), "SimCity_1.dat"))
	if err != nil {
		return err
	}
	defer stock.file.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var worker *inferenceWorker
	defer func() {
		if worker != nil {
			if result != nil {
				cancel()
			}
			result = errors.Join(result, worker.close())
		}
	}()
	var infer func([]byte) ([]byte, error)
	if *weights != "" {
		infer = func(png []byte) ([]byte, error) {
			if worker == nil {
				var err error
				worker, err = startInference(ctx, inferenceConfig{
					distro: *distro, python: *python, weights: *weights, scale: 4, tile: 64, overlap: 16,
				}, os.Stderr)
				if err != nil {
					return nil, err
				}
			}
			return worker.upscalePNG(png)
		}
	}
	entries, err := build(stock, flags.Arg(1), infer)
	if err != nil {
		return err
	}
	return publishFile(flags.Arg(2), func(w io.Writer) error { return writeArchive(w, entries) })
}
