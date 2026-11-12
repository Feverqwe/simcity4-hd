package sc4

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

const terrainGroup = 0x891b0e1a

var terrainPaletteHeader = hexBytes("24000000000101000001000000000000")

// scaleScalarMap uses separable pixel-center bilinear interpolation on palette indices.
func scaleScalarMap(pixels []byte, width, height, scale int) []byte {
	denominator := 2 * scale
	stride := width * scale
	horizontal := make([]byte, stride*height)
	for y := range height {
		row := pixels[y*width : (y+1)*width]
		for x := range stride {
			left, right, fraction := terrainSample(x, width, scale)
			horizontal[y*stride+x] = blendTerrain(row[left], row[right], fraction, denominator)
		}
	}
	result := make([]byte, stride*height*scale)
	for y := range height * scale {
		top, bottom, fraction := terrainSample(y, height, scale)
		for x := range stride {
			result[y*stride+x] = blendTerrain(horizontal[top*stride+x], horizontal[bottom*stride+x], fraction, denominator)
		}
	}
	return result
}

func terrainSample(target, size, scale int) (left, right, fraction int) {
	denominator := 2 * scale
	position := max(0, 2*target+1-scale)
	left = min(size-1, position/denominator)
	return left, min(size-1, left+1), position % denominator
}

func blendTerrain(a, b byte, fraction, denominator int) byte {
	return byte((int(a)*(denominator-fraction) + int(b)*fraction + denominator/2) / denominator)
}

func upscaleTerrain(data []byte, element fshElement, scale int) ([]byte, error) {
	paletteStart := element.start + element.stored
	if element.code != 0x7b || element.centerX|element.centerY|element.offsetX|element.offsetY != 0 ||
		element.stored != 16+element.width*element.height || element.end-paletteStart != 784 {
		return nil, errors.New("unexpected indexed terrain layout")
	}
	palette := data[paletteStart:element.end]
	if !bytes.Equal(palette[:16], terrainPaletteHeader) {
		return nil, errors.New("unexpected indexed terrain palette header")
	}
	for i, value := range palette[16:] {
		if value != byte(i/3) {
			return nil, errors.New("indexed terrain palette is not grayscale")
		}
	}
	width, height := element.width*scale, element.height*scale
	stored := 16 + width*height
	if width > 0xffff || height > 0xffff || stored > 0xffffff {
		return nil, errors.New("upscaled indexed terrain exceeds SHPI size limits")
	}
	result := make([]byte, element.start+stored+len(palette))
	copy(result, data[:element.start+16])
	header := result[element.start:]
	header[1], header[2], header[3] = byte(stored), byte(stored>>8), byte(stored>>16)
	little.PutUint16(header[4:], uint16(width))
	little.PutUint16(header[6:], uint16(height))
	little.PutUint32(result[4:], uint32(len(result)))
	pixels := scaleScalarMap(data[element.start+16:paletteStart], element.width, element.height, scale)
	copy(header[16:], pixels)
	copy(header[stored:], palette)
	return result, nil
}

func indexedTerrainEntries(game string, scale int, progress buildProgress) ([]resource, error) {
	catalog, err := openGameCatalog(game, func(id resourceID) bool {
		return id.kind == fshResource && id.group == terrainGroup
	})
	if err != nil {
		return nil, err
	}
	defer catalog.close()
	var entries []resource
	report := progress.track(len(catalog.order))
	for i, id := range catalog.order {
		report(i)
		source := catalog.owners[id]
		data, err := source.archive.read(source.record)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		elements, err := fshLayout(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		if len(elements) != 1 {
			continue
		}
		element := elements[0]
		if element.code&0x7f != 0x7b || min(element.width, element.height) == 1 {
			continue
		}
		result, err := upscaleTerrain(data, element, scale)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		entries = append(entries, resource{id, result})
	}
	if len(entries) == 0 {
		return nil, errors.New("no two-dimensional indexed terrain maps found")
	}
	report(len(catalog.order))
	return entries, nil
}

func runIndexedTerrain(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: simcity4-uhd terrain-indexed GAME_DIR OUTPUT.dat")
	}
	entries, err := indexedTerrainEntries(args[0], 4, buildProgress{})
	if err != nil {
		return err
	}
	if err := publishFile(args[1], func(w io.Writer) error {
		return writeArchive(w, entries)
	}); err != nil {
		return err
	}
	fmt.Printf("%s: %d indexed maps packed\n", args[1], len(entries))
	return nil
}
