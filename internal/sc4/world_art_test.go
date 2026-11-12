package sc4

import (
	"bytes"
	"context"
	"fmt"
	"image"
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
			art := worldArtwork{ctx: context.Background(), scale: 2, infer: func(data []byte) ([]byte, error) {
				calls++
				source, err := decodeRGBA(data)
				if err != nil {
					return nil, err
				}
				return encodeRGBA(image.NewNRGBA(image.Rect(0, 0, source.Rect.Dx()*4, source.Rect.Dy()*4)))
			}}
			x2, err := art.texture(cache, stock, false)
			if err != nil {
				t.Fatal(err)
			}
			art.infer = nil
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
