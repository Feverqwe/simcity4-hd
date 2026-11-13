package sc4

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
)

const hatCheckpointSHA = "38c00c9ff871ae292617ae1127edc150f81c4e0f8f98685e48ff9c9cd87fba3b"

type worldArtwork struct {
	cache string
	ctx   context.Context
	infer func([]byte) ([]byte, error)
	scale int
}

const worldCacheVersion = "v2"

func (a worldArtwork) texture(cache func(int) string, original []byte, placement bool) ([]byte, error) {
	if err := a.ctx.Err(); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%x.fsh", sha256.Sum256(original))
	path := filepath.Join(cache(a.scale), name)
	data, err := os.ReadFile(path)
	if err == nil {
		data, err = readWorldTexture(original, data, a.scale, placement, func() ([]byte, error) {
			return os.ReadFile(filepath.Join(cache(2), name))
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return data, nil
	}
	if !errors.Is(err, os.ErrNotExist) || a.infer == nil {
		return nil, err
	}

	x2, x4, err := rebuildBCTextures(original, placement, a.upscale)
	if err != nil {
		return nil, err
	}
	// Publish x4 last so that an interrupted build can still use the x2 result.
	for _, texture := range []struct {
		scale int
		data  []byte
	}{{2, x2}, {4, x4}} {
		if err := a.ctx.Err(); err != nil {
			return nil, err
		}
		target := filepath.Join(cache(texture.scale), name)
		err := publishFile(target, func(w io.Writer) error {
			_, err := w.Write(texture.data)
			return err
		})
		if err != nil {
			// A previous attempt or a concurrent build may have published this scale.
			existing, readErr := os.ReadFile(target)
			if readErr != nil {
				return nil, err
			}
			existing, err = readWorldTexture(original, existing, texture.scale, placement, func() ([]byte, error) {
				return x2, nil
			})
			if err != nil {
				return nil, fmt.Errorf("%s: %w", target, err)
			}
			texture.data = existing
		}
		if texture.scale == a.scale {
			data = texture.data
		}
	}
	return data, nil
}

func (a worldArtwork) upscale(source *image.NRGBA) (*image.NRGBA, error) {
	png, err := encodeRGBA(source)
	if err != nil {
		return nil, err
	}
	output, err := a.infer(png)
	if err != nil {
		return nil, err
	}
	return decodeRGBA(output)
}

func validateWorldTexture(stock, hd []byte, scale int) error {
	return checkWorldTexture(stock, hd, scale, false)
}

func readWorldTexture(stock, hd []byte, scale int, placement bool, loadX2 func() ([]byte, error)) ([]byte, error) {
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
	var framesX2 []fshElement
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
		framesX2, _ = fshLayout(x2)
	}
	cached, _ := fshLayout(hd)
	return repackBCTexture(stock, scale, placement, func(i int, original fshElement, _ []byte) ([]byte, error) {
		frame := cached[i]
		top := frame.start + 16
		block := bcBlockSize(frame.code)
		end := top + bcLevelBytes(frame.width, frame.height, block)
		if original.mipCount() == 0 || scale == 2 {
			return hd[top:end], nil
		}
		if original.stored != 0 || frame.mipCount() == original.mipCount()+2 {
			return hd[top : end+bcLevelBytes(frame.width/2, frame.height/2, block)], nil
		}
		mip := framesX2[i].start + 16
		pixels := bytes.Clone(hd[top:end])
		return append(pixels, x2[mip:mip+bcLevelBytes(frame.width/2, frame.height/2, block)]...), nil
	})
}

func checkWorldTexture(stock, hd []byte, scale int, legacyZeroOffset bool) error {
	original, err := fshLayout(stock)
	if err != nil {
		return err
	}
	result, err := fshLayout(hd)
	if err != nil {
		return err
	}
	if int(little.Uint32(hd[4:])) != len(hd) || len(original) != len(result) || string(stock[12:16]) != string(hd[12:16]) {
		return errors.New("unexpected HD SHPI header")
	}
	added := 1
	if scale == 4 {
		added = 2
	}
	for i, e := range original {
		r := result[i]
		if legacyZeroOffset && e.stored == 0 && r.offsetY == e.offsetY {
			e.offsetY &= 0xfff
			r.offsetY &= 0xfff
		}
		mips := e.mipCount()
		if mips != 0 {
			mips += added
		}
		name := 16 + 8*i
		if r.code != e.code || r.width != e.width*scale || r.height != e.height*scale || r.centerX != e.centerX ||
			r.centerY != e.centerY || r.offsetX != e.offsetX || r.offsetY&0xfff != e.offsetY&0xfff ||
			r.mipCount() != mips || string(stock[name:name+4]) != string(hd[name:name+4]) {
			return fmt.Errorf("HD SHPI element %d metadata differs", i)
		}
		if err := verifyWorldTextureTail(stock, hd, e, r, scale); err != nil {
			return fmt.Errorf("HD SHPI element %d: %w", i, err)
		}
	}
	return nil
}

func verifyWorldTextureTail(stock, hd []byte, e, r fshElement, scale int) error {
	size := bcBlockSize(e.code)
	stockPayload := e.start + 16
	hdPayload := r.start + 16
	stockTop := bcLevelBytes(e.width, e.height, size)
	hdTop := bcLevelBytes(r.width, r.height, size)
	stockTail, hdTail := stockPayload+stockTop, hdPayload+hdTop
	delta := hdTop - stockTop
	if e.mipCount() != 0 {
		stockTail = stockPayload
		if scale == 4 {
			hdTail += bcLevelBytes(2*e.width, 2*e.height, size)
		}
		delta = hdTail - hdPayload
	}
	stored := e.stored
	if stored != 0 {
		stored += delta
	}
	if r.stored != stored || stockTail > e.end || hdTail > r.end ||
		!bytes.Equal(stock[stockTail:e.end], hd[hdTail:r.end]) {
		return errors.New("changed original mip data or attachments")
	}
	return nil
}

func worldInference(ctx context.Context, cache string, config inferenceConfig) (worldArtwork, func() error) {
	art := worldArtwork{cache: cache, ctx: ctx, scale: config.scale}
	config.scale = 4 // Both output scales reuse the full-resolution inference result.
	var worker *inferenceWorker
	if config.enabled() {
		art.infer = func(data []byte) ([]byte, error) {
			if worker == nil {
				var err error
				worker, err = startInference(ctx, config, os.Stderr)
				if err != nil {
					return nil, err
				}
			}
			return worker.upscalePNG(data)
		}
	}
	return art, func() error {
		if worker == nil {
			return nil
		}
		err := worker.close()
		worker = nil
		return err
	}
}
