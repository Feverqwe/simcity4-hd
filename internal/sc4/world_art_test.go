package sc4

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestWorldX2BuildKeepsX4Inference(t *testing.T) {
	for _, mipmapped := range []bool{false, true} {
		t.Run(fmt.Sprintf("mipmapped=%t", mipmapped), func(t *testing.T) {
			// An 8×8 BC1 texture with an optional stock mip and an attachment.
			stock := make([]byte, 24+16+32+8)
			copy(stock, "SHPI")
			little.PutUint32(stock[8:], 1)
			copy(stock[12:], "G264test")
			little.PutUint32(stock[20:], 24)
			stock[24] = bc1
			little.PutUint16(stock[28:], 8)
			little.PutUint16(stock[30:], 8)
			if mipmapped {
				stock = append(stock, make([]byte, 8)...)
				stock[25] = 16 + 32 + 8
				little.PutUint16(stock[38:], 1<<12)
			}
			copy(stock[len(stock)-8:], "sentinel")
			little.PutUint32(stock[4:], uint32(len(stock)))

			root := t.TempDir()
			cache := func(scale int) string { return filepath.Join(root, fmt.Sprint(scale)) }
			calls := 0
			infer := func(data []byte) ([]byte, error) {
				source, err := decodeRGBA(data)
				if err != nil {
					return nil, err
				}
				pixels := image.NewNRGBA(image.Rect(0, 0, source.Rect.Dx()*4, source.Rect.Dy()*4))
				for y := range pixels.Rect.Dy() {
					for x := range pixels.Rect.Dx() {
						pixels.SetNRGBA(x, y, color.NRGBA{R: uint8(x*29 + y*11), G: uint8(x*7 + y*13), B: uint8(x*3 + y*17), A: 255})
					}
				}
				return encodeRGBA(pixels)
			}
			ctx := context.Background()
			config := inferenceConfig{scale: 4, tile: 64, overlap: 16, keepUpscaledImages: true}
			art := worldArtwork{ctx: ctx, scale: 2, infer: cachedInference(ctx, root, config, func(data []byte) ([]byte, error) {
				calls++
				return infer(data)
			})}
			x2, err := art.texture(cache, stock, false)
			if err != nil {
				t.Fatal(err)
			}
			// The default build retains FSHs. Exporting this cache should
			// make another build at the same scale possible with no model.
			compiled := t.TempDir()
			config.keepUpscaledImages = false
			plain, closePlain := worldInference(ctx, compiled, config)
			defer closePlain()
			plain.scale = 2
			plain.infer = cachedInference(ctx, compiled, config, infer)
			if _, err := plain.texture(plain.modelCache, stock, false); err != nil {
				t.Fatal(err)
			}
			compiledArchive := filepath.Join(t.TempDir(), "compiled.zip")
			if err := exportTextureArchive(ctx, compiled, compiledArchive, textureArchiveProgress{}); err != nil {
				t.Fatal(err)
			}
			imported := t.TempDir()
			if err := importTextureArchive(ctx, imported, compiledArchive, textureArchiveProgress{}); err != nil {
				t.Fatal(err)
			}
			// Remove inference entirely. Passing via a PNG cache hit would
			// hide an FSH export regression.
			plain.cache, plain.infer = imported, nil
			reused, err := plain.texture(plain.modelCache, stock, false)
			if err != nil || !bytes.Equal(reused, x2) {
				t.Fatalf("FSH-only archive could not rebuild the texture: %v", err)
			}
			if _, err := os.Stat(filepath.Join(imported, inferenceCacheFolder)); !os.IsNotExist(err) {
				t.Fatalf("default build included raw inference images: %v", err)
			}
			config.keepUpscaledImages = true

			// Compare with the x2 level encoded directly from the model's pixels.
			stockPixels := decodeBC(stock[40:72], 8, 8, bc1)
			input, err := encodeRGBA(stockPixels)
			if err != nil {
				t.Fatal(err)
			}
			output, err := infer(input)
			if err != nil {
				t.Fatal(err)
			}
			hd, err := decodeRGBA(output)
			if err != nil {
				t.Fatal(err)
			}
			wantX2, err := encodeBC(halfLanczosRGB(hd), stockPixels, 2, bc1)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(x2[40:40+len(wantX2)], wantX2) {
				t.Fatal("x2 differs from the result encoded from uncompressed model output")
			}
			// Repeating a build at the same scale should reuse its FSH without
			// processing the source pixels or looking up the portable PNG.
			art.infer = nil
			warm, err := art.texture(cache, stock, false)
			if err != nil || !bytes.Equal(warm, x2) {
				t.Fatalf("same-scale rebuild did not reuse its compiled texture: %v", err)
			}
			archive := filepath.Join(t.TempDir(), "textures.zip")
			if err := exportTextureArchive(ctx, root, archive, textureArchiveProgress{}); err != nil {
				t.Fatal(err)
			}
			root = t.TempDir()
			if err := importTextureArchive(ctx, root, archive, textureArchiveProgress{}); err != nil {
				t.Fatal(err)
			}
			art.infer = cachedInference(ctx, root, config, nil)
			art.scale = 4
			x4, err := art.texture(cache, stock, false)
			if err != nil {
				t.Fatalf("x4 must be available without inference after building x2: %v", err)
			}
			for scale, data := range map[int][]byte{2: x2, 4: x4} {
				if err := validateWorldTexture(stock, data, scale); err != nil {
					t.Fatalf("invalid x%d texture: %v", scale, err)
				}
			}
			if mipmapped {
				x4TopBytes := bcLevelBytes(32, 32, bcBlockSize(bc1))
				x2TopBytes := bcLevelBytes(16, 16, bcBlockSize(bc1))
				if !bytes.Equal(x2[40:40+x2TopBytes], x4[40+x4TopBytes:40+x4TopBytes+x2TopBytes]) {
					t.Fatal("the x2 top level differs from the corresponding x4 mip")
				}
			}
			art.scale = 2
			again, err := art.texture(cache, stock, false)
			if err != nil || !bytes.Equal(x2, again) || calls != 1 {
				t.Fatalf("x2 cache replay failed (inference calls: %d): %v", calls, err)
			}
		})
	}
}

func TestWorldZeroOffsetMipChain(t *testing.T) {
	// This 8×8 BC1 frame has two mip levels and no attachment offset.
	stock := make([]byte, 32+16+32+16+8)
	copy(stock, "SHPI")
	little.PutUint32(stock[4:], uint32(len(stock)))
	little.PutUint32(stock[8:], 1)
	copy(stock[12:], "G264test")
	little.PutUint32(stock[20:], 32)
	stock[32] = bc1
	little.PutUint16(stock[36:], 8)
	little.PutUint16(stock[38:], 8)
	little.PutUint16(stock[46:], 2<<12)
	copy(stock[80:], "firstMip")
	copy(stock[96:], "lastMip!")

	root := t.TempDir()
	cache := func(scale int) string { return filepath.Join(root, fmt.Sprint(scale)) }
	art := worldArtwork{ctx: context.Background(), scale: 2, infer: func(data []byte) ([]byte, error) {
		return encodeRGBA(image.NewNRGBA(image.Rect(0, 0, 32, 32)))
	}}
	built := make(map[int][]byte)
	for _, scale := range []int{2, 4} {
		art.scale = scale
		data, err := art.texture(cache, stock, false)
		if err != nil {
			t.Fatal(err)
		}
		built[scale] = data
		frame := int(little.Uint32(data[20:]))
		width := 8 * scale
		cursor := frame + 16
		for width > 8 {
			cursor += width * width / 2
			width /= 2
		}
		if data[frame+15]>>4 != byte(2+scale/2) ||
			!bytes.Equal(data[cursor:], stock[48:]) {
			t.Fatalf("x%d did not retain the stock levels at their native mip offsets", scale)
		}

		// Recreate the previous cache payload, with only the largest HD
		// level before the original mip tail.
		top := frame + 16
		end := top + (8*scale)*(8*scale)/2
		legacy := append(bytes.Clone(data[:end]), stock[80:]...)
		legacy[frame+15] = stock[47]
		little.PutUint32(legacy[4:], uint32(len(legacy)))
		if err := os.MkdirAll(cache(scale), 0755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(cache(scale), fmt.Sprintf("%x.fsh", sha256.Sum256(stock)))
		if err := os.WriteFile(path, legacy, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	art.infer = nil
	for _, scale := range []int{2, 4} {
		art.scale = scale
		data, err := art.texture(cache, stock, false)
		if err != nil || !bytes.Equal(data, built[scale]) {
			t.Fatalf("legacy x%d cache did not restore the original mips: %v", scale, err)
		}
	}
}
