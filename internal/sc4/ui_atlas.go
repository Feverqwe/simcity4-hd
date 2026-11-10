package sc4

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

var uiColorKey = color.NRGBA{R: 255, B: 255, A: 255}

func decodeRGBA(data []byte) (*image.NRGBA, error) {
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if rgba, ok := source.(*image.NRGBA); ok {
		return rgba, nil
	}
	result := image.NewNRGBA(source.Bounds())
	draw.Draw(result, result.Bounds(), source, source.Bounds().Min, draw.Src)
	return result, nil
}

type rgbaPNG struct{ *image.NRGBA }

func (rgbaPNG) Opaque() bool { return false }

func encodeRGBA(image *image.NRGBA) ([]byte, error) {
	var result bytes.Buffer
	err := png.Encode(&result, rgbaPNG{image})
	return result.Bytes(), err
}

func hasColorKey(source *image.NRGBA) bool {
	for y := source.Rect.Min.Y; y < source.Rect.Max.Y; y++ {
		for x := source.Rect.Min.X; x < source.Rect.Max.X; x++ {
			if source.NRGBAAt(x, y) == uiColorKey {
				return true
			}
		}
	}
	return false
}

// padKeyedCell fills the RGB of key pixels from the nearest non-key pixel
// before inference. The original key mask is reapplied after inference.
func padKeyedCell(cell *image.NRGBA) (*image.NRGBA, []bool, bool) {
	width, height := cell.Rect.Dx(), cell.Rect.Dy()
	padded := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(padded, padded.Rect, cell, cell.Rect.Min, draw.Src)
	mask := make([]bool, width*height)
	seen := make([]bool, len(mask))
	queue := make([]int, 0, len(mask))
	for i := range mask {
		mask[i] = padded.NRGBAAt(i%width, i/width) == uiColorKey
		if !mask[i] {
			queue = append(queue, i)
			seen[i] = true
		}
	}
	empty := len(queue) == 0
	for head := 0; head < len(queue) && len(queue) < len(mask); head++ {
		index := queue[head]
		x, y := index%width, index/width
		source := padded.NRGBAAt(x, y)
		for _, neighbor := range []struct {
			index int
			valid bool
		}{
			{index - 1, x > 0}, {index + 1, x+1 < width},
			{index - width, y > 0}, {index + width, y+1 < height},
		} {
			if neighbor.valid && !seen[neighbor.index] {
				seen[neighbor.index] = true
				// The key mask belongs to the original opaque pixels. Borrow
				// the neighboring RGB without copying its transparency.
				fill := source
				fill.A = uiColorKey.A
				padded.SetNRGBA(neighbor.index%width, neighbor.index/width, fill)
				queue = append(queue, neighbor.index)
			}
		}
	}
	return padded, mask, empty
}

func inferAtlasCells(source *image.NRGBA, widths []int, scale int, infer func([]byte) ([]byte, error)) (*image.NRGBA, error) {
	total := 0
	for _, width := range widths {
		if width <= 0 {
			return nil, errors.New("invalid atlas frame width")
		}
		total += width
	}
	if total != source.Rect.Dx() {
		return nil, errors.New("invalid atlas frame geometry")
	}
	height := source.Rect.Dy()
	atlas := image.NewNRGBA(image.Rect(0, 0, scale*source.Rect.Dx(), scale*height))
	x := 0
	for _, width := range widths {
		sourceRect := image.Rect(x, 0, x+width, height).Add(source.Rect.Min)
		cell := source.SubImage(sourceRect).(*image.NRGBA)
		padded, mask, empty := padKeyedCell(cell)
		var result *image.NRGBA
		if empty {
			result = image.NewNRGBA(image.Rect(0, 0, width*scale, height*scale))
		} else {
			input, err := encodeRGBA(padded)
			if err != nil {
				return nil, err
			}
			response, err := infer(input)
			if err != nil {
				return nil, err
			}
			result, err = decodeRGBA(response)
			if err != nil {
				return nil, err
			}
			if result.Rect.Dx() != width*scale || result.Rect.Dy() != height*scale {
				return nil, errors.New("inference returned incorrect atlas frame dimensions")
			}
		}
		for index, keyed := range mask {
			if keyed {
				x, y := index%width*scale, index/width*scale
				for dy := range scale {
					for dx := range scale {
						result.SetNRGBA(x+dx, y+dy, uiColorKey)
					}
				}
			}
		}
		draw.Draw(atlas, image.Rect(x*scale, 0, (x+width)*scale, height*scale), result, result.Rect.Min, draw.Src)
		x += width
	}
	return atlas, nil
}

func projectAtlas(atlas *image.NRGBA, widths []int, keyed bool) (*image.NRGBA, error) {
	total := 0
	for _, width := range widths {
		if width <= 0 {
			return nil, errors.New("invalid atlas cell width")
		}
		total += width
	}
	if atlas.Rect.Dx() != 4*total || atlas.Rect.Dy()%4 != 0 {
		return nil, errors.New("invalid x4 atlas dimensions")
	}
	result := image.NewNRGBA(image.Rect(0, 0, 2*total, atlas.Rect.Dy()/2))
	x := 0
	for _, width := range widths {
		rectangle := image.Rect(4*x, 0, 4*(x+width), atlas.Rect.Dy()).Add(atlas.Rect.Min)
		cell := atlas.SubImage(rectangle).(*image.NRGBA)
		resized := projectAtlasCell(cell, keyed)
		draw.Draw(result, image.Rect(2*x, 0, 2*(x+width), result.Rect.Dy()), resized, resized.Rect.Min, draw.Src)
		x += width
	}
	return result, nil
}

// Pillow resizes RGBA through premultiplied eight-bit channels and rounds after
// each separable pass. Filtering each cell separately prevents state bleed.
func projectAtlasCell(cell *image.NRGBA, keyed bool) *image.NRGBA {
	width, height := cell.Rect.Dx(), cell.Rect.Dy()
	horizontal := image.NewNRGBA(image.Rect(0, 0, width/2, height))
	for y := range height {
		for x := range width / 2 {
			horizontal.SetNRGBA(x, y, filterAtlasPair(x, width, keyed, func(i int) color.NRGBA {
				p := cell.NRGBAAt(cell.Rect.Min.X+i, cell.Rect.Min.Y+y)
				return color.NRGBA{premultiply(p.R, p.A), premultiply(p.G, p.A), premultiply(p.B, p.A), p.A}
			}))
		}
	}
	result := image.NewNRGBA(image.Rect(0, 0, width/2, height/2))
	for y := range height / 2 {
		for x := range width / 2 {
			p := filterAtlasPair(y, height, keyed, func(i int) color.NRGBA {
				return horizontal.NRGBAAt(x, i)
			})
			if p.A != 0 {
				p.R, p.G, p.B = unpremultiply(p.R, p.A), unpremultiply(p.G, p.A), unpremultiply(p.B, p.A)
			}
			result.SetNRGBA(x, y, p)
		}
	}
	return result
}

func premultiply(value, alpha byte) byte   { return byte((int(value)*int(alpha) + 127) / 255) }
func unpremultiply(value, alpha byte) byte { return byte(min(255, int(value)*255/int(alpha))) }

func filterAtlasPair(index, size int, keyed bool, at func(int) color.NRGBA) color.NRGBA {
	start, weights := 2*index-1, []int{1, 3, 3, 1}
	if keyed {
		start, weights = 2*index, []int{1, 1}
	}
	var r, g, b, a, total int
	for i, weight := range weights {
		if position := start + i; position >= 0 && position < size {
			p := at(position)
			r += int(p.R) * weight
			g += int(p.G) * weight
			b += int(p.B) * weight
			a += int(p.A) * weight
			total += weight
		}
	}
	return color.NRGBA{byte((r + total/2) / total), byte((g + total/2) / total),
		byte((b + total/2) / total), byte((a + total/2) / total)}
}

func projectUIArtwork(original, enlarged []byte, widths []int, useEnlargedKey bool) ([]byte, error) {
	return projectUIArtworkWithAlpha(original, enlarged, widths, false, useEnlargedKey)
}

func projectUIArtworkWithAlpha(original, enlarged []byte, widths []int, originalAlpha, useEnlargedKey bool) ([]byte, error) {
	source, err := decodeRGBA(original)
	if err != nil {
		return nil, err
	}
	atlas, err := decodeRGBA(enlarged)
	if err != nil {
		return nil, err
	}
	keyed := hasColorKey(source)
	if useEnlargedKey {
		keyed = hasColorKey(atlas)
	}
	projected, err := projectAtlas(atlas, widths, keyed)
	if err != nil {
		return nil, err
	}
	if originalAlpha {
		restoreLanczosAlpha(source, projected)
	}
	return encodeRGBA(projected)
}
