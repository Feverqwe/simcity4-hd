package sc4

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

const terrainGroup = 0x891b0e1a

var terrainPaletteHeader = hexBytes("24000000000101000001000000000000")

// quadrupleScalarMap uses separable pixel-center bilinear interpolation on
// palette indices. For x4, the fractional positions have denominator eight.
func quadrupleScalarMap(pixels []byte, width, height int) []byte {
	const scale = 4
	stride := width * scale
	horizontal := make([]byte, stride*height)
	for y := range height {
		row := pixels[y*width : (y+1)*width]
		for x := range stride {
			left, right, fraction := terrainSample(x, width)
			horizontal[y*stride+x] = blendTerrain(row[left], row[right], fraction)
		}
	}
	result := make([]byte, stride*height*scale)
	for y := range height * scale {
		top, bottom, fraction := terrainSample(y, height)
		for x := range stride {
			result[y*stride+x] = blendTerrain(horizontal[top*stride+x], horizontal[bottom*stride+x], fraction)
		}
	}
	return result
}

func terrainSample(target, size int) (left, right, fraction int) {
	// The first target center falls three eighths before the first source center.
	position := max(0, 2*target-3)
	left = min(size-1, position/8)
	return left, min(size-1, left+1), position % 8
}

func blendTerrain(a, b byte, fraction int) byte {
	return byte((int(a)*(8-fraction) + int(b)*fraction + 4) / 8)
}

func upscaleTerrain(data []byte, element fshElement) ([]byte, error) {
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
	width, height := element.width*4, element.height*4
	stored := 16 + width*height
	if width > 0xffff || height > 0xffff || stored > 0xffffff {
		return nil, errors.New("x4 indexed terrain exceeds SHPI size limits")
	}
	result := make([]byte, element.start+stored+len(palette))
	copy(result, data[:element.start+16])
	header := result[element.start:]
	header[1], header[2], header[3] = byte(stored), byte(stored>>8), byte(stored>>16)
	little.PutUint16(header[4:], uint16(width))
	little.PutUint16(header[6:], uint16(height))
	little.PutUint32(result[4:], uint32(len(result)))
	pixels := quadrupleScalarMap(data[element.start+16:paletteStart], element.width, element.height)
	copy(header[16:], pixels)
	copy(header[stored:], palette)
	return result, nil
}

func indexedTerrainEntries(game string) ([]resource, error) {
	catalog, err := openGameCatalog(game, func(id resourceID) bool {
		return id.kind == fshResource && id.group == terrainGroup
	})
	if err != nil {
		return nil, err
	}
	defer catalog.close()
	var entries []resource
	for _, id := range catalog.order {
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
		result, err := upscaleTerrain(data, element)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		entries = append(entries, resource{id, result})
	}
	if len(entries) == 0 {
		return nil, errors.New("no two-dimensional indexed terrain maps found")
	}
	return entries, nil
}

func runIndexedTerrain(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: simcity4-hd terrain-indexed GAME_DIR OUTPUT.dat")
	}
	entries, err := indexedTerrainEntries(args[0])
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
