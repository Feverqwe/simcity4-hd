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
}

func (a worldArtwork) texture(cache string, original []byte, scale int, placement bool) ([]byte, error) {
	if err := a.ctx.Err(); err != nil {
		return nil, err
	}
	path := filepath.Join(cache, fmt.Sprintf("%x.fsh", sha256.Sum256(original)))
	data, err := os.ReadFile(path)
	if err == nil {
		if err := validateWorldTexture(original, data, scale); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return data, nil
	}
	if !errors.Is(err, os.ErrNotExist) || a.infer == nil {
		return nil, err
	}
	data, err = rebuildBCTexture(original, scale, placement, a.upscale)
	if err != nil {
		return nil, err
	}
	if err := publishFile(path, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	}); err != nil {
		return nil, err
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
	art := worldArtwork{cache: cache, ctx: ctx}
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
