package sc4

import (
	"fmt"
	"image"
	"image/color"
)

const (
	bc1 byte = 0x60
	bc2 byte = 0x61
)

func bcBlockSize(format byte) int {
	if format == bc2 {
		return 16
	}
	return 8
}

func rgb565(value uint16) color.NRGBA {
	r := byte(value>>11) << 3
	g := byte(value>>5&63) << 2
	b := byte(value&31) << 3
	return color.NRGBA{R: r | r>>5, G: g | g>>6, B: b | b>>5, A: 255}
}

func pack565(p color.NRGBA) uint16 {
	return uint16(p.R>>3)<<11 | uint16(p.G>>2)<<5 | uint16(p.B>>3)
}

func mixBC(a, b color.NRGBA, weightA, weightB int) color.NRGBA {
	mix := func(x, y byte) byte {
		return byte((int(x)*weightA + int(y)*weightB) / (weightA + weightB))
	}
	return color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 255}
}

func decodeBC(data []byte, width, height int, format byte) *image.NRGBA {
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	size := bcBlockSize(format)
	for y := 0; y < height; y += 4 {
		for x := 0; x < width; x += 4 {
			block := data[:size]
			data = data[size:]
			values := block[size-8:]
			first, second := little.Uint16(values), little.Uint16(values[2:])
			palette := [4]color.NRGBA{rgb565(first), rgb565(second)}
			if first > second || format == bc2 {
				palette[2] = mixBC(palette[0], palette[1], 2, 1)
				palette[3] = mixBC(palette[0], palette[1], 1, 2)
			} else {
				palette[2] = mixBC(palette[0], palette[1], 1, 1)
			}
			for row := range min(4, height-y) {
				for column := range min(4, width-x) {
					pixel := palette[values[4+row]>>(2*column)&3]
					if format == bc2 {
						pixel.A = (block[2*row+column/2] >> (4 * (column % 2)) & 15) * 17
					}
					result.SetNRGBA(x+column, y+row, pixel)
				}
			}
		}
	}
	return result
}

// The endpoint choice and selector thresholds match Pillow's BcnEncode.c
// (MIT-CMU, see licenses/Pillow.txt). BC2 alpha is written independently.
func encodeBCColor(dst []byte, pixels [16]color.NRGBA, alpha bool) {
	var low, high uint16
	found, transparent := false, false
	for _, pixel := range pixels {
		if alpha && pixel.A == 0 {
			transparent = true
			continue
		}
		value := pack565(pixel)
		if !found {
			low, high, found = value, value, true
		} else {
			low, high = min(low, value), max(high, value)
		}
	}
	if transparent {
		little.PutUint16(dst, low)
		little.PutUint16(dst[2:], high)
	} else {
		little.PutUint16(dst, high)
		little.PutUint16(dst[2:], low)
	}
	a, b := rgb565(low), rgb565(high)
	for index, pixel := range pixels {
		selector := bcSelector(pixel, a, b, transparent)
		dst[4+index/4] |= selector << (2 * (index % 4))
	}
}

func bcSelector(pixel, low, high color.NRGBA, transparent bool) byte {
	if transparent && pixel.A == 0 {
		return 3
	}
	var distance float32
	channels := 0
	for _, c := range [3][3]byte{{pixel.R, low.R, high.R}, {pixel.G, low.G, high.G}, {pixel.B, low.B, high.B}} {
		delta := int(c[2]) - int(c[1])
		if delta != 0 {
			fromLow := int(c[0]) - int(c[1])
			distance += float32(max(fromLow, -fromLow)) / float32(max(delta, -delta))
			channels++
		}
	}
	if channels == 0 {
		return 0
	}
	if transparent {
		switch scaled := distance * float32(4/channels); {
		case scaled < 1:
			return 0
		case scaled < 3:
			return 2
		default:
			return 1
		}
	}
	switch scaled := distance * float32(6/channels); {
	case scaled < 1:
		return 1
	case scaled < 3:
		return 3
	case scaled < 5:
		return 2
	default:
		return 0
	}
}

func encodeBC(art, source *image.NRGBA, scale int, format byte) ([]byte, error) {
	width, height := art.Rect.Dx(), art.Rect.Dy()
	size := bcBlockSize(format)
	result := make([]byte, width/4*(height/4)*size)
	offset := 0
	for y := 0; y < height; y += 4 {
		for x := 0; x < width; x += 4 {
			var pixels [16]color.NRGBA
			block := result[offset : offset+size]
			for i := range pixels {
				u, v := x+i%4, y+i/4
				pixels[i] = art.NRGBAAt(u, v)
				pixels[i].A = source.NRGBAAt(u/scale, v/scale).A
				if format == bc2 {
					block[i/2] |= (pixels[i].A / 17) << (4 * (i % 2))
				}
			}
			colorBlock := block[size-8:]
			encodeBCColor(colorBlock, pixels, format == bc1)
			if format == bc1 && little.Uint16(colorBlock) <= little.Uint16(colorBlock[2:]) {
				for i, pixel := range pixels {
					transparent := colorBlock[4+i/4]>>(2*(i%4))&3 == 3
					if transparent != (pixel.A == 0) {
						return nil, fmt.Errorf("BC1 alpha changed in block (%d,%d)", x, y)
					}
				}
			}
			offset += size
		}
	}
	return result, nil
}
