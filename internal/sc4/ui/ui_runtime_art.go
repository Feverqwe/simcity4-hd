package ui

import (
	"fmt"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/uiids"
)

const tooltipFrame = 0x14416190

func runtimeChromeEntries(stock uiStock, art uiArtwork) ([]dbpf.Entry, error) {
	instances := [...]uint32{
		tooltipFrame,
		0x14315e60, 0x14315e61, 0x14315e62, // Viewport alert border states.
	}
	entries := make([]dbpf.Entry, 0, len(instances))
	for _, instance := range instances {
		entry, err := runtimeArtwork(stock, art, instance, 1, false)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func runtimeArtwork(stock uiStock, art uiArtwork, instance uint32, frames int, keepAlpha bool) (dbpf.Entry, error) {
	source := dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: instance}
	target := dbpf.ID{Kind: uiImage, Group: uiids.RuntimeArtworkGroup, Instance: instance}
	if _, exists := stock.index[target]; exists {
		return dbpf.Entry{}, fmt.Errorf("runtime image key collision: %s", target)
	}
	original, err := stock.read(source)
	if err != nil {
		return dbpf.Entry{}, err
	}
	var options atlasOptions
	if keepAlpha {
		options.variant = "screen-alpha"
		options.finish = func(data []byte) ([]byte, error) { return copyScreenAlpha(original, data) }
	}
	data, err := art.upscaleAtlas(layoutAtlas{source: source, frames: frames, project: true}, original, options)
	if err != nil {
		return dbpf.Entry{}, fmt.Errorf("runtime image %s: %w", source, err)
	}

	return dbpf.Entry{ID: target, Data: data}, nil
}

func copyScreenAlpha(original, enlarged []byte) ([]byte, error) {
	source, err := textures.DecodeRGBA(original)
	if err != nil {
		return nil, err
	}
	result, err := textures.DecodeRGBA(enlarged)
	if err != nil {
		return nil, err
	}
	if result.Rect.Dx() != 2*source.Rect.Dx() || result.Rect.Dy() != 2*source.Rect.Dy() {
		return nil, fmt.Errorf("invalid screen-density dimensions")
	}
	// Keep holes and partially transparent pixels in the original gauge masks.
	for y := range result.Rect.Dy() {
		for x := range result.Rect.Dx() {
			result.Pix[result.PixOffset(x, y)+3] = source.Pix[source.PixOffset(x/2, y/2)+3]
		}
	}
	return textures.EncodeRGBA(result)
}
