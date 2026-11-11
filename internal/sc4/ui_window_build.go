package sc4

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
)

var stockPNGSignature = []byte("\x89PNG\r\n\x1a\n")

type uiStock struct {
	index map[resourceID]resourceLocation
}

func indexUIStock(stock *archive) uiStock {
	index := make(map[resourceID]resourceLocation)
	for _, r := range stock.records {
		if r.id.kind == 0 || r.id.kind == uiImage {
			index[r.id] = resourceLocation{stock, r}
		}
	}
	return uiStock{index}
}

func (s uiStock) read(id resourceID) ([]byte, error) {
	r, exists := s.index[id]
	if !exists {
		return nil, fmt.Errorf("stock UI resource %s is missing", id)
	}
	return r.archive.read(r.record)
}

func (s uiStock) isPNG(id resourceID) (bool, error) {
	r, exists := s.index[id]
	if !exists {
		return false, nil
	}
	data, err := r.archive.read(r.record)
	return bytes.HasPrefix(data, stockPNGSignature), err
}

type windowGroup struct {
	group uint32
	ids   []uint32
}

func (s uiStock) windowGroups() ([]windowGroup, error) {
	groups := []windowGroup{
		{0xe74a40b3, []uint32{0xc973b411}},
		{0xe74a40b4, []uint32{0x8a5ab1cb, 0x8a5ab1cc, 0x8a5ab1cf}},
		{0xe74a40b5, []uint32{0xca53f06e, 0xcbc3c2b9, 0x4a160034, 0xcbc905cd, 0xc9930681, 0xaa3acdfe, 0xebd0d36c, 0xebd0d36d}},
		{0xe74a40b6, []uint32{0x2bc90671, 0x898897de, 0xa991ed83, 0x09923283, 0xe9923283, 0xe9949936, 0xe99237ff,
			0x4992f764, 0x899302fc, 0x4bc906b5, 0x2bc9060f, 0x6bc9065a, 0x6bc61f19, 0x2a2aed99}},
		{0xe74a40b7, []uint32{0xea287193, 0xea2871aa, 0x0b72f276, 0x8a7e052f, 0x6a553aa4, 0xaa1f1f57}},
		{group: 0xe74a40b8},
		{group: 0xe74a40b9},
		{group: 0xe74a40be},
		{0xe74a40bb, []uint32{0x0a55161d, 0x8a4d0a17, 0x0a8cd184, 0x8a5ab1cd, 0x4a551b4c, 0x0a7df315, 0x4a89b3f2, 0x2a41436b}},
		{0xe74a40bc, []uint32{0xaa356502, 0xaa53e3ea, 0x69e3d347, 0xaaa44448}},
		{0xe74a40ba, []uint32{0x0a243d80}},
		{0xe74a40bd, []uint32{0x49889894, 0x6bb27447, 0xcb95403e, 0xca539340,
			0x4a5a89d4, 0x4a5a89d5, 0x0bbc06b6, 0x4bbc080f, 0x2bb16d50}},
		{0xe74a40bf, []uint32{0x0a2dd355, 0x0a4d0c43, 0x0a5cf71d, 0x2a41436c, 0x4a8cc5ea, 0x4bf325e8,
			0x6a5e73c0, 0x6b704690, 0x8a5ab1ce, 0x8a5ab1d0, 0x8c1a5c9f, 0xabb0120f, 0xabc0ed33, 0xabfaef15,
			0xca4d0b22, 0xca551016, 0xea89b6c3, 0xea8cc3c6, 0xeaaeec1b, 0x0a41be3e, 0x0a41be3f, 0x6a9455c9,
			0xaa5e60d1, 0xca539343, 0xca8cbf0f}},
		{0xe74a40c1, []uint32{0xaaaaf3d1, 0x8aa9aa14}},
	}
	queryRoots := []string{" id=0x10000005 ", " id=0x10000006 ", " id=0x4bcb938a "}
	for id := range s.index {
		if id.kind != 0 || id.group != layoutID(0).group {
			continue
		}
		data, err := s.read(id)
		if err != nil {
			return nil, err
		}
		root := legacyRoot.Find(data)
		for i, attr := range queryRoots {
			if bytes.Contains(root, []byte(attr)) {
				groups[5+i].ids = append(groups[5+i].ids, id.instance)
			}
		}
	}
	for i := range queryRoots {
		if len(groups[5+i].ids) == 0 {
			return nil, fmt.Errorf("no windows found for root %s", queryRoots[i])
		}
		slices.Sort(groups[5+i].ids)
	}
	return groups, nil
}

func buildWindowEntries(stock uiStock, config string, graphics4K bool, textureScale int, art func(layoutAtlas, []byte) ([]byte, error)) ([]resource, string, error) {
	groups, err := stock.windowGroups()
	if err != nil {
		return nil, "", err
	}
	seen := make(map[resourceID]bool)
	var entries []resource
	for _, group := range groups {
		window := newWindowLayouts(group.group)
		for _, instance := range group.ids {
			id := layoutID(instance)
			data, err := stock.read(id)
			if err != nil {
				return nil, "", err
			}
			if err := window.add(instance, data, textureScale, stock.isPNG); err != nil {
				return nil, "", err
			}
		}
		config, err = addLayoutFontAliases(config, window.fonts)
		if err != nil {
			return nil, "", err
		}
		for _, asset := range window.assets {
			if _, exists := stock.index[asset.target]; seen[asset.target] || exists {
				return nil, "", fmt.Errorf("HD atlas key collision: %s", asset.target)
			}
			seen[asset.target] = true
			original, err := stock.read(asset.atlas.source)
			if err != nil {
				return nil, "", err
			}
			data, err := art(asset.atlas, original)
			if err != nil {
				return nil, "", fmt.Errorf("%s: %w", asset.atlas.source, err)
			}
			joinID, artScale := asset.target, textureScale
			if asset.atlas.project {
				joinID.group -= 0x20
				artScale = 2
			}
			if usesStockWindowAlpha(joinID) {
				edgeFrames := 0
				if (joinID.group == 0xe74a40b7 && joinID.instance == 0xbdd0852c) ||
					(joinID.group == 0xe74a40b6 && joinID.instance == 0xa0653af9) {
					// The model brightens the Add My Sim border and the
					// corners of the Polls outline where it covers the fill.
					edgeFrames = asset.atlas.frames
				}
				data, err = preserveStockAlpha(original, data, artScale, edgeFrames)
				if err != nil {
					return nil, "", fmt.Errorf("%s: %w", asset.target, err)
				}
			}
			entries = append(entries, resource{asset.target, data})
		}
		for _, layout := range window.layouts {
			if graphics4K && layout.id == layoutID(0x8a7e052f) {
				const caption = `caption="1600x1200" captionres={0a554ae8,ca7e17db}`
				if strings.Count(string(layout.data), caption) != 1 {
					return nil, "", fmt.Errorf("unexpected fourth Graphics Options resolution label")
				}
				layout.data = bytes.Replace(layout.data, []byte(caption), []byte(`caption="3840x2160"`), 1)
			}
			if seen[layout.id] {
				return nil, "", fmt.Errorf("duplicate HUD layout %s", layout.id)
			}
			seen[layout.id] = true
			entries = append(entries, layout)
		}
	}
	return entries, config, nil
}

func usesStockWindowAlpha(id resourceID) bool {
	switch id.group {
	case 0xe74a40b3:
		// Mayor backdrop pieces meet at a hard edge.
		return id.instance == 0x39436963 || id.instance == 0x15cb44b4
	case 0xe74a40b6:
		// The Polls outline overlays the score fill.
		return id.instance == 0x9d46b8f0 || id.instance == 0x0cd075f5 || id.instance == 0x95d9244f ||
			id.instance == 0xa0653af9
	case 0xe74a40b7:
		// My Sims HUD portrait and Add My Sim overlays.
		return id.instance == 0x94e742d5 || id.instance == 0xbdd0852c
	case 0xe74a40ba:
		// The Select a My Sim button frames have a thin outline.
		return id.instance == 0x649fd6a1
	}
	return false
}

func preserveStockAlpha(stock, enlarged []byte, scale, edgeFrames int) ([]byte, error) {
	source, err := decodeRGBA(stock)
	if err != nil {
		return nil, err
	}
	art, err := decodeRGBA(enlarged)
	if err != nil {
		return nil, err
	}
	if art.Rect.Dx() != scale*source.Rect.Dx() || art.Rect.Dy() != scale*source.Rect.Dy() {
		return nil, fmt.Errorf("invalid dimensions for stock alpha")
	}
	frameWidth := 0
	if edgeFrames > 0 {
		frameWidth = source.Rect.Dx() / edgeFrames
	}
	for y := range art.Rect.Dy() {
		for x := range art.Rect.Dx() {
			sx, sy := x/scale, y/scale
			pixel := source.NRGBAAt(sx, sy)
			if frameWidth > 0 && pixel.A != 0 && (pixel.A != 255 ||
				sx%frameWidth == 0 || sx%frameWidth == frameWidth-1 ||
				source.NRGBAAt(sx-1, sy).A == 0 || source.NRGBAAt(sx+1, sy).A == 0 ||
				source.NRGBAAt(sx, sy-1).A == 0 || source.NRGBAAt(sx, sy+1).A == 0) {
				art.SetNRGBA(x, y, pixel)
			} else {
				art.Pix[art.PixOffset(x, y)+3] = pixel.A
			}
		}
	}
	return encodeRGBA(art)
}
