package upscale

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
)

const (
	InferenceCacheFolder = "UpscaledImages"
	InterfaceAtlasFolder = "InterfaceAtlases"
	CatalogFramesFolder  = "CatalogFrames"
	PortraitsFolder      = "MySimPortraits"
	CursorsFolder        = "Cursors"
)

// The key describes the model's input and settings. Changes to the output
// packages, texture compression or atlas layout do not change the key.
func CachedInference(ctx context.Context, cache string, config Config, infer func([]byte) ([]byte, error)) func([]byte) ([]byte, error) {
	return func(input []byte) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		source, err := textures.DecodeRGBA(input)
		if err != nil {
			return nil, err
		}
		width, height := source.Rect.Dx(), source.Rect.Dy()
		key := sha256.New()
		fmt.Fprintf(key, "%s/%d/%d/%d/%d/%d:", HatCheckpointSHA, config.Scale, config.Tile, config.Overlap, width, height)
		for y := range height {
			start := y * source.Stride
			key.Write(source.Pix[start : start+4*width])
		}
		path := filepath.Join(cache, InferenceCacheFolder, fmt.Sprintf("%x.png", key.Sum(nil)))
		validate := func(output []byte) ([]byte, error) {
			cfg, err := png.DecodeConfig(bytes.NewReader(output))
			if err != nil {
				return nil, err
			}
			if cfg.Width != width*config.Scale || cfg.Height != height*config.Scale {
				return nil, errors.New("upscaler returned incorrect image dimensions")
			}
			return output, nil
		}

		data, err := os.ReadFile(path)
		if err == nil {
			return validate(data)
		}
		if !errors.Is(err, os.ErrNotExist) || infer == nil {
			return nil, err
		}
		output, err := infer(input)
		if err != nil {
			return nil, err
		}
		output, err = validate(output)
		if err != nil || !config.KeepUpscaledImages {
			return output, err
		}
		err = gamefiles.PublishFile(path, func(w io.Writer) error {
			_, err := io.Copy(w, gamefiles.WithContext(ctx, bytes.NewReader(output)))
			return err
		})
		if err != nil {
			if existing, readErr := os.ReadFile(path); readErr == nil {
				return validate(existing)
			}
			return nil, err
		}
		return output, nil
	}
}
