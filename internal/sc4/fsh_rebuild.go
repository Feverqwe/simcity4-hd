package sc4

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"math"
)

func (e fshElement) mipCount() int {
	if e.stored == 0 {
		return 0
	}
	return int(e.offsetY >> 12)
}

func (e fshElement) hasPlacement() bool {
	return e.centerX != 0 || e.centerY != 0 || e.offsetX&0xfff != 0 || e.offsetY&0xfff != 0
}

func bcLevelBytes(width, height, block int) int {
	return (width + 3) / 4 * ((height + 3) / 4) * block
}

func validateBCElement(data []byte, e fshElement, added int, preservePlacement bool) (int, error) {
	if !preservePlacement && e.hasPlacement() {
		return 0, errors.New("texture placement needs a caller policy")
	}
	if e.code != bc1 && e.code != bc2 {
		return 0, fmt.Errorf("unsupported SHPI encoding %#x", e.code)
	}
	if e.width%4 != 0 || e.height%4 != 0 {
		return 0, errors.New("non-block-aligned dimensions")
	}
	if max(e.width, e.height)<<added > math.MaxUint16 {
		return 0, errors.New("upscaled texture exceeds SHPI dimensions")
	}
	block := bcBlockSize(e.code)
	top := bcLevelBytes(e.width, e.height, block)
	end := e.end - e.start
	if e.stored != 0 {
		end = e.stored
	}
	if top > end-16 {
		return 0, errors.New("truncated texture payload")
	}
	if mips := e.mipCount(); mips != 0 {
		if mips+added > 15 || min(e.width, e.height)>>mips == 0 {
			return 0, errors.New("unsupported mip chain")
		}
		chain, aligned := 16, 16
		for level := 0; level <= mips; level++ {
			size := bcLevelBytes(e.width>>level, e.height>>level, block)
			chain += size
			aligned += (size + 15) &^ 15
		}
		if e.stored != chain && e.stored != aligned {
			if e.stored != (chain+15)&^15 || len(bytes.Trim(data[e.start+chain:e.start+e.stored], "\x00")) != 0 {
				return 0, errors.New("unexpected mip chain storage")
			}
		}
	}
	return top, nil
}

func rebuildBCTextures(data []byte, preservePlacement bool,
	upscale func(*image.NRGBA) (*image.NRGBA, error)) (x2, x4 []byte, err error) {
	var levelsX2 [][]byte
	x4, err = repackBCTexture(data, 4, preservePlacement, func(_ int, e fshElement, payload []byte) ([]byte, error) {
		source := decodeBC(payload, e.width, e.height, e.code)
		art, err := upscale(source)
		if err != nil {
			return nil, err
		}
		if art.Rect.Dx() != e.width*4 || art.Rect.Dy() != e.height*4 {
			return nil, errors.New("upscaler returned incorrect texture dimensions")
		}
		level, err := encodeBC(halfLanczosRGB(art), source, 2, e.code)
		if err != nil {
			return nil, err
		}
		levelsX2 = append(levelsX2, level)
		encoded, err := encodeBC(art, source, 4, e.code)
		if err != nil {
			return nil, err
		}
		if e.mipCount() != 0 {
			encoded = append(encoded, level...)
		}
		return encoded, nil
	})
	if err != nil {
		return nil, nil, err
	}
	x2, err = repackBCTexture(data, 2, preservePlacement, func(i int, _ fshElement, _ []byte) ([]byte, error) {
		return levelsX2[i], nil
	})
	if err != nil {
		return nil, nil, err
	}
	return x2, x4, nil
}

func repackBCTexture(data []byte, scale int, preservePlacement bool,
	encode func(int, fshElement, []byte) ([]byte, error)) ([]byte, error) {
	added := 1
	switch scale {
	case 2:
	case 4:
		added = 2
	default:
		return nil, errors.New("expected texture scale x2 or x4")
	}
	elements, err := fshLayout(data)
	if err != nil {
		return nil, err
	}
	sizes := make([]int, len(elements))
	for i, e := range elements {
		sizes[i], err = validateBCElement(data, e, added, preservePlacement)
		if err != nil {
			return nil, fmt.Errorf("SHPI element %d: %w", i, err)
		}
	}
	result := bytes.Clone(data[:elements[0].start])
	for i, e := range elements {
		size := sizes[i]
		payload := e.start + 16
		end := payload + size
		encoded, err := encode(i, e, data[payload:end])
		if err != nil {
			return nil, err
		}
		header := [16]byte(data[e.start:payload])
		little.PutUint16(header[4:], uint16(e.width*scale))
		little.PutUint16(header[6:], uint16(e.height*scale))
		delta := len(encoded) - size
		if e.mipCount() != 0 {
			// Keep the complete original chain byte for byte.
			end = payload
			delta = len(encoded)
			little.PutUint16(header[14:], e.offsetY+uint16(added<<12))
		}
		if e.stored != 0 {
			stored := e.stored + delta
			if stored >= 1<<24 {
				return nil, errors.New("upscaled SHPI element exceeds 24-bit size field")
			}
			header[1], header[2], header[3] = byte(stored), byte(stored>>8), byte(stored>>16)
		}
		if uint64(len(result))+16+uint64(len(encoded))+uint64(e.end-end) > math.MaxUint32 {
			return nil, errors.New("upscaled SHPI exceeds 32-bit size field")
		}
		little.PutUint32(result[20+8*i:], uint32(len(result)))
		result = append(result, header[:]...)
		result = append(result, encoded...)
		result = append(result, data[end:e.end]...)
	}
	little.PutUint32(result[4:], uint32(len(result)))
	return result, nil
}
