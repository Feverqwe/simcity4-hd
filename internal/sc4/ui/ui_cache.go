package ui

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/texturecache"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

type atlasOptions struct {
	originalAlpha, enlargedKey bool
	variant                    string
	finish                     func([]byte) ([]byte, error)
}

func preparedPNGPath(cache string, scale int, recipe string, source []byte) string {
	key := sha256.New()
	fmt.Fprintf(key, "v1/%s/%d/%s:", upscale.HatCheckpointSHA, scale, recipe)
	key.Write(source)
	return filepath.Join(texturecache.InterfaceCache(cache, scale), fmt.Sprintf("%x.png", key.Sum(nil)))
}

func cachedPreparedPNG(path string, build func() ([]byte, error)) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return data, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	data, err = build()
	if err != nil {
		return nil, err
	}
	err = gamefiles.PublishFile(path, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
	return data, err
}

func atlasRecipe(id dbpf.ID, widths []int, options atlasOptions) string {
	return fmt.Sprintf("atlas/%s/%v/%t/%t/%s", id, widths, options.originalAlpha, options.enlargedKey, options.variant)
}

func renderX4Atlas(cache string, id dbpf.ID, stock []byte, widths []int, infer func([]byte) ([]byte, error)) ([]byte, error) {
	path := preparedPNGPath(cache, 4, atlasRecipe(id, widths, atlasOptions{}), stock)
	data, err := os.ReadFile(path)
	if err == nil || !errors.Is(err, os.ErrNotExist) || infer == nil {
		return data, err
	}
	source, err := textures.DecodeRGBA(stock)
	if err != nil {
		return nil, err
	}
	atlas, err := inferAtlasCells(source, widths, 4, infer)
	if err != nil {
		return nil, err
	}
	return textures.EncodeRGBA(atlas)
}

func preparedUIAtlas(cache string, id dbpf.ID, stock []byte, widths []int, scale int,
	options atlasOptions, infer func([]byte) ([]byte, error)) ([]byte, error) {
	path := preparedPNGPath(cache, scale, atlasRecipe(id, widths, options), stock)
	return cachedPreparedPNG(path, func() ([]byte, error) {
		data, err := renderX4Atlas(cache, id, stock, widths, infer)
		if err == nil && scale == 2 {
			data, err = projectUIArtworkWithAlpha(stock, data, widths, options.originalAlpha, options.enlargedKey)
		}
		if err == nil && options.finish != nil {
			data, err = options.finish(data)
		}
		return data, err
	})
}
