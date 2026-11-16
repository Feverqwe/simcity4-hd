package texture

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"math"
)

func (e FSHElement) MipCount() int {
	return int(e.OffsetY >> 12)
}

func (e FSHElement) HasPlacement() bool {
	return e.CenterX != 0 || e.CenterY != 0 || e.OffsetX&0xfff != 0 || e.OffsetY&0xfff != 0
}

func BCLevelBytes(width, height, block int) int {
	return (width + 3) / 4 * ((height + 3) / 4) * block
}

func ValidateBCElement(data []byte, e FSHElement, added int, preservePlacement bool) (int, error) {
	if !preservePlacement && e.HasPlacement() {
		return 0, errors.New("texture placement needs a caller policy")
	}
	if e.Code != BC1 && e.Code != BC2 {
		return 0, fmt.Errorf("unsupported SHPI encoding %#x", e.Code)
	}
	if e.Width%4 != 0 || e.Height%4 != 0 {
		return 0, errors.New("non-block-aligned dimensions")
	}
	if max(e.Width, e.Height)<<added > math.MaxUint16 {
		return 0, errors.New("upscaled texture exceeds SHPI dimensions")
	}
	block := BCBlockSize(e.Code)
	top := BCLevelBytes(e.Width, e.Height, block)
	end := e.End - e.Start
	if e.Stored != 0 {
		end = e.Stored
	}
	if top > end-16 {
		return 0, errors.New("truncated texture payload")
	}
	if mips := e.MipCount(); mips != 0 {
		if mips+added > 15 || min(e.Width, e.Height)>>mips == 0 {
			return 0, errors.New("unsupported mip chain")
		}
		chain, aligned := 16, 16
		for level := 0; level <= mips; level++ {
			size := BCLevelBytes(e.Width>>level, e.Height>>level, block)
			chain += size
			aligned += (size + 15) &^ 15
		}
		if e.Stored == 0 {
			// With no attachment offset, the native reader still walks every
			// mip on 16-byte boundaries within the frame.
			last := BCLevelBytes(e.Width>>mips, e.Height>>mips, block)
			if end < aligned-((last+15)&^15)+last {
				return 0, errors.New("truncated mip chain")
			}
		} else if e.Stored != chain && e.Stored != aligned {
			if e.Stored != (chain+15)&^15 || len(bytes.Trim(data[e.Start+chain:e.Start+e.Stored], "\x00")) != 0 {
				return 0, errors.New("unexpected mip chain storage")
			}
		}
	}
	return top, nil
}

func RebuildBCTexture(data []byte, scale int, preservePlacement bool,
	upscale func(*image.NRGBA) (*image.NRGBA, error)) ([]byte, error) {
	return RepackBCTexture(data, scale, preservePlacement, func(_ int, e FSHElement, payload []byte) ([]byte, error) {
		source := DecodeBC(payload, e.Width, e.Height, e.Code)
		art, err := upscale(source)
		if err != nil {
			return nil, err
		}
		if scale == 2 {
			return EncodeBC(HalfLanczosRGB(art), source, 2, e.Code)
		}
		encoded, err := EncodeBC(art, source, 4, e.Code)
		if err != nil || e.MipCount() == 0 {
			return encoded, err
		}
		mip, err := EncodeBC(HalfLanczosRGB(art), source, 2, e.Code)
		if err != nil {
			return nil, err
		}
		return append(encoded, mip...), nil
	})
}

func RepackBCTexture(data []byte, scale int, preservePlacement bool,
	encode func(int, FSHElement, []byte) ([]byte, error)) ([]byte, error) {
	added := 1
	switch scale {
	case 2:
	case 4:
		added = 2
	default:
		return nil, errors.New("expected texture scale x2 or x4")
	}
	elements, err := FSHLayout(data)
	if err != nil {
		return nil, err
	}
	sizes := make([]int, len(elements))
	for i, e := range elements {
		sizes[i], err = ValidateBCElement(data, e, added, preservePlacement)
		if err != nil {
			return nil, fmt.Errorf("SHPI element %d: %w", i, err)
		}
	}
	result := bytes.Clone(data[:elements[0].Start])
	for i, e := range elements {
		size := sizes[i]
		payload := e.Start + 16
		end := payload + size
		encoded, err := encode(i, e, data[payload:end])
		if err != nil {
			return nil, err
		}
		header := [16]byte(data[e.Start:payload])
		binary.LittleEndian.PutUint16(header[4:], uint16(e.Width*scale))
		binary.LittleEndian.PutUint16(header[6:], uint16(e.Height*scale))
		delta := len(encoded) - size
		if e.MipCount() != 0 {
			// Keep the complete original chain byte for byte.
			end = payload
			delta = len(encoded)
			binary.LittleEndian.PutUint16(header[14:], e.OffsetY+uint16(added<<12))
		}
		if e.Stored != 0 {
			stored := e.Stored + delta
			if stored >= 1<<24 {
				return nil, errors.New("upscaled SHPI element exceeds 24-bit size field")
			}
			header[1], header[2], header[3] = byte(stored), byte(stored>>8), byte(stored>>16)
		}
		if uint64(len(result))+16+uint64(len(encoded))+uint64(e.End-end) > math.MaxUint32 {
			return nil, errors.New("upscaled SHPI exceeds 32-bit size field")
		}
		binary.LittleEndian.PutUint32(result[20+8*i:], uint32(len(result)))
		result = append(result, header[:]...)
		result = append(result, encoded...)
		result = append(result, data[end:e.End]...)
	}
	binary.LittleEndian.PutUint32(result[4:], uint32(len(result)))
	return result, nil
}
