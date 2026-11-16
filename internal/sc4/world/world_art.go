package world

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

type worldArtwork struct {
	cache string
	ctx   context.Context
	infer func([]byte) ([]byte, error)
	scale int
}

func (a worldArtwork) texture(cache func(int) string, original []byte, placement bool) ([]byte, error) {
	if err := a.ctx.Err(); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%x.fsh", sha256.Sum256(original))
	path := filepath.Join(cache(a.scale), name)
	validate := func(data []byte) ([]byte, error) {
		data, err := ReadWorldTexture(original, data, a.scale, placement, func() ([]byte, error) {
			return os.ReadFile(filepath.Join(cache(2), name))
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err == nil {
		return validate(data)
	}
	if !errors.Is(err, os.ErrNotExist) || a.infer == nil {
		return data, err
	}
	data, err = textures.RebuildBCTexture(original, a.scale, placement, a.upscale)
	if err != nil {
		return nil, err
	}
	err = gamefiles.PublishFile(path, func(w io.Writer) error {
		_, err := io.Copy(w, gamefiles.WithContext(a.ctx, bytes.NewReader(data)))
		return err
	})
	if err != nil {
		// A concurrent builder may have completed the same texture.
		if existing, readErr := os.ReadFile(path); readErr == nil {
			return validate(existing)
		}
		return nil, err
	}
	return data, nil
}

func (a worldArtwork) upscale(source *image.NRGBA) (*image.NRGBA, error) {
	input, err := textures.EncodeRGBA(source)
	if err != nil {
		return nil, err
	}
	output, err := a.infer(input)
	if err != nil {
		return nil, err
	}
	return textures.DecodeRGBA(output)
}

func validateWorldTexture(stock, hd []byte, scale int) error {
	return checkWorldTexture(stock, hd, scale, false)
}

func ReadWorldTexture(stock, hd []byte, scale int, placement bool, loadX2 func() ([]byte, error)) ([]byte, error) {
	err := validateWorldTexture(stock, hd, scale)
	if err == nil {
		return hd, nil
	}
	if checkWorldTexture(stock, hd, scale, true) != nil {
		return nil, err
	}

	// Reuse the cached HD pixels from entries built before zero-offset mip
	// chains were recognized. The original lower levels come from the DAT.
	var x2 []byte
	var framesX2 []textures.FSHElement
	if scale == 4 {
		x2, err = loadX2()
		if err != nil {
			return nil, fmt.Errorf("read cached x2 mip: %w", err)
		}
		if err := validateWorldTexture(stock, x2, 2); err != nil {
			if checkWorldTexture(stock, x2, 2, true) != nil {
				return nil, err
			}
		}
		framesX2, _ = textures.FSHLayout(x2)
	}
	cached, _ := textures.FSHLayout(hd)
	return textures.RepackBCTexture(stock, scale, placement, func(i int, original textures.FSHElement, _ []byte) ([]byte, error) {
		frame := cached[i]
		top := frame.Start + 16
		block := textures.BCBlockSize(frame.Code)
		end := top + textures.BCLevelBytes(frame.Width, frame.Height, block)
		if original.MipCount() == 0 || scale == 2 {
			return hd[top:end], nil
		}
		if original.Stored != 0 || frame.MipCount() == original.MipCount()+2 {
			return hd[top : end+textures.BCLevelBytes(frame.Width/2, frame.Height/2, block)], nil
		}
		mip := framesX2[i].Start + 16
		pixels := bytes.Clone(hd[top:end])
		return append(pixels, x2[mip:mip+textures.BCLevelBytes(frame.Width/2, frame.Height/2, block)]...), nil
	})
}

func checkWorldTexture(stock, hd []byte, scale int, legacyZeroOffset bool) error {
	original, err := textures.FSHLayout(stock)
	if err != nil {
		return err
	}
	result, err := textures.FSHLayout(hd)
	if err != nil {
		return err
	}
	if int(binary.LittleEndian.Uint32(hd[4:])) != len(hd) || len(original) != len(result) || string(stock[12:16]) != string(hd[12:16]) {
		return errors.New("unexpected HD SHPI header")
	}
	added := 1
	if scale == 4 {
		added = 2
	}
	for i, e := range original {
		r := result[i]
		if legacyZeroOffset && e.Stored == 0 && r.OffsetY == e.OffsetY {
			e.OffsetY &= 0xfff
			r.OffsetY &= 0xfff
		}
		mips := e.MipCount()
		if mips != 0 {
			mips += added
		}
		name := 16 + 8*i
		if r.Code != e.Code || r.Width != e.Width*scale || r.Height != e.Height*scale || r.CenterX != e.CenterX ||
			r.CenterY != e.CenterY || r.OffsetX != e.OffsetX || r.OffsetY&0xfff != e.OffsetY&0xfff ||
			r.MipCount() != mips || string(stock[name:name+4]) != string(hd[name:name+4]) {
			return fmt.Errorf("HD SHPI element %d metadata differs", i)
		}
		if err := verifyWorldTextureTail(stock, hd, e, r, scale); err != nil {
			return fmt.Errorf("HD SHPI element %d: %w", i, err)
		}
	}
	return nil
}

func verifyWorldTextureTail(stock, hd []byte, e, r textures.FSHElement, scale int) error {
	size := textures.BCBlockSize(e.Code)
	stockPayload := e.Start + 16
	hdPayload := r.Start + 16
	stockTop := textures.BCLevelBytes(e.Width, e.Height, size)
	hdTop := textures.BCLevelBytes(r.Width, r.Height, size)
	stockTail, hdTail := stockPayload+stockTop, hdPayload+hdTop
	delta := hdTop - stockTop
	if e.MipCount() != 0 {
		stockTail = stockPayload
		if scale == 4 {
			hdTail += textures.BCLevelBytes(2*e.Width, 2*e.Height, size)
		}
		delta = hdTail - hdPayload
	}
	stored := e.Stored
	if stored != 0 {
		stored += delta
	}
	if r.Stored != stored || stockTail > e.End || hdTail > r.End ||
		!bytes.Equal(stock[stockTail:e.End], hd[hdTail:r.End]) {
		return errors.New("changed original mip data or attachments")
	}
	return nil
}

func worldInference(ctx context.Context, cache string, config upscale.Config) (worldArtwork, func() error) {
	art := worldArtwork{cache: cache, ctx: ctx, scale: config.Scale}
	config.Scale = 4 // Both output scales reuse the full-resolution inference result.
	var worker *upscale.InferenceWorker
	if config.Enabled() {
		art.infer = func(data []byte) ([]byte, error) {
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
	art.infer = upscale.CachedInference(ctx, cache, config, art.infer)
	return art, func() error {
		if worker == nil {
			return nil
		}
		err := worker.Close()
		worker = nil
		return err
	}
}
