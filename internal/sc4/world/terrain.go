package world

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

const terrainGroup = 0x891b0e1a

var TerrainPaletteHeader = []byte{0x24, 0x00, 0x00, 0x00, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

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

func upscaleTerrain(data []byte, element textures.FSHElement, scale int) ([]byte, error) {
	paletteStart := element.Start + element.Stored
	if element.Code != 0x7b || element.CenterX|element.CenterY|element.OffsetX|element.OffsetY != 0 ||
		element.Stored != 16+element.Width*element.Height || element.End-paletteStart != 784 {
		return nil, errors.New("unexpected indexed terrain layout")
	}
	palette := data[paletteStart:element.End]
	if !bytes.Equal(palette[:16], TerrainPaletteHeader) {
		return nil, errors.New("unexpected indexed terrain palette header")
	}
	for i, value := range palette[16:] {
		if value != byte(i/3) {
			return nil, errors.New("indexed terrain palette is not grayscale")
		}
	}
	width, height := element.Width*scale, element.Height*scale
	stored := 16 + width*height
	if width > 0xffff || height > 0xffff || stored > 0xffffff {
		return nil, errors.New("upscaled indexed terrain exceeds SHPI size limits")
	}
	result := make([]byte, element.Start+stored+len(palette))
	copy(result, data[:element.Start+16])
	header := result[element.Start:]
	header[1], header[2], header[3] = byte(stored), byte(stored>>8), byte(stored>>16)
	binary.LittleEndian.PutUint16(header[4:], uint16(width))
	binary.LittleEndian.PutUint16(header[6:], uint16(height))
	binary.LittleEndian.PutUint32(result[4:], uint32(len(result)))
	pixels := scaleScalarMap(data[element.Start+16:paletteStart], element.Width, element.Height, scale)
	copy(header[16:], pixels)
	copy(header[stored:], palette)
	return result, nil
}

func indexedTerrainEntries(game string, scale int, progress upscale.Progress) ([]dbpf.Entry, error) {
	catalog, err := gamefiles.OpenCatalog(game, func(id dbpf.ID) bool {
		return id.Kind == textures.FSHResource && id.Group == terrainGroup
	})
	if err != nil {
		return nil, err
	}
	defer catalog.Close()
	var entries []dbpf.Entry
	report := progress.Track(len(catalog.Order))
	for i, id := range catalog.Order {
		report(i)
		source := catalog.Owners[id]
		data, err := source.Archive.Read(source.Record)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		elements, err := textures.FSHLayout(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		if len(elements) != 1 {
			continue
		}
		element := elements[0]
		if element.Code&0x7f != 0x7b || min(element.Width, element.Height) == 1 {
			continue
		}
		result, err := upscaleTerrain(data, element, scale)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		entries = append(entries, dbpf.Entry{ID: id, Data: result})
	}
	if len(entries) == 0 {
		return nil, errors.New("no two-dimensional indexed terrain maps found")
	}
	report(len(catalog.Order))
	return entries, nil
}

func RunIndexedTerrain(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: simcity4-uhd terrain-indexed GAME_DIR OUTPUT.dat")
	}
	entries, err := indexedTerrainEntries(args[0], 4, upscale.Progress{})
	if err != nil {
		return err
	}
	if err := gamefiles.PublishFile(args[1], func(w io.Writer) error {
		return dbpf.Write(w, entries)
	}); err != nil {
		return err
	}
	fmt.Printf("%s: %d indexed maps packed\n", args[1], len(entries))
	return nil
}
