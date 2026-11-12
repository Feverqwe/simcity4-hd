package sc4

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"slices"
)

const cursorResource = 0xaa5c3144

type cursorFrame struct {
	pixels  *image.NRGBA
	hotspot image.Point
}

func cursorEntries(stock *archive, art uiArtwork, scale int) ([]resource, error) {
	var entries []resource
	for _, record := range stock.records {
		if record.id.kind != cursorResource {
			continue
		}
		original, err := stock.read(record)
		if err != nil {
			return nil, err
		}
		if len(original) < 6 {
			return nil, fmt.Errorf("%s: truncated cursor directory", record.id)
		}
		if little.Uint16(original[2:]) != 2 {
			continue // The archive also has Windows icons under this type.
		}
		frames, err := decodeCursor(original)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", record.id, err)
		}
		for i, frame := range frames {
			if scale*max(frame.pixels.Rect.Dx(), frame.pixels.Rect.Dy()) > 256 {
				return nil, fmt.Errorf("%s frame %d: upscaled cursor exceeds Windows cursor dimensions", record.id, i)
			}
			upscaled, err := upscaleCursorFrame(frame.pixels, art, scale)
			if err != nil {
				return nil, fmt.Errorf("%s frame %d: %w", record.id, i, err)
			}
			frames[i] = cursorFrame{upscaled, frame.hotspot.Mul(scale)}
		}
		// Windows uses the first entry when loading at its file size.
		slices.SortStableFunc(frames, func(a, b cursorFrame) int {
			return b.pixels.Rect.Dx()*b.pixels.Rect.Dy() - a.pixels.Rect.Dx()*a.pixels.Rect.Dy()
		})
		entries = append(entries, resource{record.id, encodeCursor(frames)})
	}
	if len(entries) == 0 {
		return nil, errors.New("no stock Windows cursors found")
	}
	return entries, nil
}

func decodeCursor(data []byte) ([]cursorFrame, error) {
	if len(data) < 6 || little.Uint16(data) != 0 || little.Uint16(data[2:]) != 2 {
		return nil, errors.New("invalid Windows cursor header")
	}
	count := int(little.Uint16(data[4:]))
	directoryEnd := 6 + 16*count
	if count == 0 || directoryEnd > len(data) {
		return nil, errors.New("invalid Windows cursor directory")
	}
	frames := make([]cursorFrame, 0, count)
	for at := 6; at < directoryEnd; at += 16 {
		entry := data[at : at+16]
		w, h := int(entry[0]), int(entry[1])
		hotspot := image.Pt(int(little.Uint16(entry[4:])), int(little.Uint16(entry[6:])))
		size, start := uint64(little.Uint32(entry[8:])), uint64(little.Uint32(entry[12:]))
		if w == 0 || h == 0 || w > 127 || h > 127 || !hotspot.In(image.Rect(0, 0, w, h)) ||
			start < uint64(directoryEnd) || start+size > uint64(len(data)) {
			return nil, errors.New("invalid Windows cursor frame extent")
		}
		pixels, err := decodeCursorBitmap(data[start:start+size], w, h)
		if err != nil {
			return nil, err
		}
		frames = append(frames, cursorFrame{pixels, hotspot})
	}
	return frames, nil
}

func decodeCursorBitmap(dib []byte, width, height int) (*image.NRGBA, error) {
	if len(dib) < 40 || little.Uint32(dib) != 40 ||
		little.Uint32(dib[4:]) != uint32(width) || little.Uint32(dib[8:]) != uint32(2*height) ||
		little.Uint16(dib[12:]) != 1 || little.Uint32(dib[16:]) != 0 {
		return nil, errors.New("unsupported cursor bitmap header")
	}
	bits := int(little.Uint16(dib[14:]))
	paletteCount := int(little.Uint32(dib[32:]))
	switch bits {
	case 1, 4, 8:
		if paletteCount == 0 {
			paletteCount = 1 << bits
		}
		if paletteCount < 0 || paletteCount > 1<<bits {
			return nil, errors.New("invalid cursor palette size")
		}
	case 24, 32:
		if paletteCount != 0 {
			return nil, errors.New("unexpected cursor palette")
		}
	default:
		return nil, fmt.Errorf("unsupported %d-bit cursor bitmap", bits)
	}

	stride := ((width*bits + 31) / 32) * 4
	maskStride := ((width + 31) / 32) * 4
	start := 40 + 4*paletteCount
	maskStart := start + stride*height
	if maskStart+maskStride*height > len(dib) {
		return nil, errors.New("truncated cursor bitmap or transparency mask")
	}
	hasAlpha := false
	if bits == 32 {
		for at := start + 3; at < maskStart; at += 4 {
			hasAlpha = hasAlpha || dib[at] != 0
		}
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		row := height - 1 - y
		xor := dib[start+row*stride:]
		mask := dib[maskStart+row*maskStride:]
		for x := range width {
			var pixel color.NRGBA
			if paletteCount != 0 {
				shift := 8 - bits - (x*bits)%8
				index := int(xor[x*bits/8]>>shift) & ((1 << bits) - 1)
				if index >= paletteCount {
					return nil, errors.New("cursor pixel outside palette")
				}
				entry := dib[40+4*index:]
				pixel = color.NRGBA{R: entry[2], G: entry[1], B: entry[0], A: 255}
				if bits == 1 && index == 1 && mask[x/8]&(0x80>>(x%8)) != 0 {
					return nil, errors.New("cursor requires an XOR-inverted background pixel")
				}
			} else {
				entry := xor[x*(bits/8):]
				pixel = color.NRGBA{R: entry[2], G: entry[1], B: entry[0], A: 255}
				if hasAlpha {
					pixel.A = entry[3]
				}
			}
			if !hasAlpha && mask[x/8]&(0x80>>(x%8)) != 0 {
				pixel.A = 0
			}
			pixels.SetNRGBA(x, y, pixel)
		}
	}
	return pixels, nil
}

func upscaleCursorFrame(source *image.NRGBA, art uiArtwork, scale int) (*image.NRGBA, error) {
	w, h := source.Rect.Dx(), source.Rect.Dy()
	opaque := make([]image.Point, 0, w*h)
	for y := range h {
		for x := range w {
			if source.NRGBAAt(x, y).A != 0 {
				opaque = append(opaque, image.Pt(x, y))
			}
		}
	}
	if len(opaque) == 0 {
		return image.NewNRGBA(image.Rect(0, 0, scale*w, scale*h)), nil
	}
	// Extend edge colors into clear pixels before RGB inference. The alpha
	// channel comes directly from the original bitmap.
	rgb := image.NewNRGBA(source.Rect)
	for y := range h {
		for x := range w {
			pixel := source.NRGBAAt(x, y)
			if pixel.A == 0 {
				distance := w*w + h*h
				for _, p := range opaque {
					dx, dy := p.X-x, p.Y-y
					if squared := dx*dx + dy*dy; squared < distance {
						distance = squared
						pixel = source.NRGBAAt(p.X, p.Y)
					}
				}
			}
			pixel.A = 255
			rgb.SetNRGBA(x, y, pixel)
		}
	}
	input, err := encodeRGBA(rgb)
	if err != nil {
		return nil, err
	}
	// Variants with identical RGB share inference even when their masks differ.
	_, enlarged, err := art.atlas("ui-cursors-x4", resourceID{kind: cursorResource}, input, []int{w})
	if err != nil {
		return nil, err
	}
	result := enlarged
	if scale == 2 {
		result = halfLanczosRGB(enlarged)
	}
	restoreLanczosAlpha(source, result)
	return result, nil
}

func encodeCursor(frames []cursorFrame) []byte {
	data := make([]byte, 6+16*len(frames))
	little.PutUint16(data[2:], 2)
	little.PutUint16(data[4:], uint16(len(frames)))
	for i, frame := range frames {
		w, h := frame.pixels.Rect.Dx(), frame.pixels.Rect.Dy()
		maskStride := ((w + 31) / 32) * 4
		colorBytes := 4 * w * h
		bitmap := make([]byte, 40+colorBytes+maskStride*h)
		little.PutUint32(bitmap, 40)
		little.PutUint32(bitmap[4:], uint32(w))
		little.PutUint32(bitmap[8:], uint32(2*h))
		little.PutUint16(bitmap[12:], 1)
		little.PutUint16(bitmap[14:], 32)
		little.PutUint32(bitmap[20:], uint32(colorBytes))
		for y := range h {
			row := h - 1 - y
			pixels := bitmap[40+4*w*row:]
			mask := bitmap[40+colorBytes+maskStride*row:]
			for x := range w {
				p := frame.pixels.NRGBAAt(x, y)
				if p.A == 0 {
					mask[x/8] |= 0x80 >> (x % 8)
				} else {
					copy(pixels[4*x:], []byte{p.B, p.G, p.R, p.A})
				}
			}
		}
		entry := data[6+16*i:]
		entry[0], entry[1] = byte(w), byte(h)
		little.PutUint16(entry[4:], uint16(frame.hotspot.X))
		little.PutUint16(entry[6:], uint16(frame.hotspot.Y))
		little.PutUint32(entry[8:], uint32(len(bitmap)))
		little.PutUint32(entry[12:], uint32(len(data)))
		data = append(data, bitmap...)
	}
	return data
}
