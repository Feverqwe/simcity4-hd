package pe

import (
	"encoding/binary"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
)

func appendCatalogPatches(code []byte, patches []x86.Patch, base int) ([]byte, []x86.Patch) {
	paint := base + len(code)
	code = append(code, patchsrc.WinCatalogPaint...)
	tile := base + len(code)
	code = append(code, patchsrc.WinCatalogTile...)
	height := base + len(code)
	code = append(code, patchsrc.WinCatalogHeight...)
	button := base + len(code)
	code = append(code, patchsrc.WinCatalogButton...)

	const (
		stretch   = 0x4d9020 // RZDrawBltStretch
		stockTile = 0x4d8bc0 // RZDrawBltTiled
	)
	// Push an address without assuming a fixed image base.
	pushAddress := func(body []byte, entry, target int) []byte {
		pc := entry + len(body) + 5
		body = append(body, x86.HexBytes("e8000000005805")...) // call next; pop eax; add eax, delta
		body = binary.LittleEndian.AppendUint32(body, uint32(target-pc))
		return append(body, 0x50)
	}
	for _, site := range []struct {
		rva              int
		setup, stockCall string
	}{
		{0x39abb0, "8b54242452", "ff5074"},     // Catalog item.
		{0x39b2bd, "8b0152", "ff5074"},         // Chrome top arrow.
		{0x39b3d7, "8b96d800000052", "ff5074"}, // Chrome bottom arrow.
		{0x39b428, "8b1150", "ff5274"},         // Chrome cap.
	} {
		entry := base + len(code)
		body := append(x86.HexBytes(site.setup), 0x51) // dst (this)
		body = pushAddress(body, entry, stretch)
		body = append(body, x86.RelativeInstruction(0xe8, entry+len(body), paint)...)
		body = append(body, x86.HexBytes("83c418")...) // Original arguments and the adapter's two extra arguments.
		var patch x86.Patch
		code, patch = x86.AppendStub(code, base, site.rva, x86.HexBytes(site.setup+site.stockCall), body)
		patches = append(patches, patch)
	}

	const tileSite = 0x39b341
	entry := base + len(code)
	body := pushAddress(nil, entry, stockTile)
	body = pushAddress(body, entry, stretch)
	body = append(body, x86.RelativeInstruction(0xe8, entry+len(body), tile)...)
	body = append(body, x86.HexBytes("83c408")...) // The caller still owns its original arguments.
	var patch x86.Patch
	code, patch = x86.AppendStub(code, base, tileSite, x86.RelativeInstruction(0xe8, tileSite, stockTile), body)
	patches = append(patches, patch)

	// Leave the frame's measured height in screen pixels.
	body = x86.RelativeInstruction(0xe8, base+len(code), height)
	body = append(body, x86.HexBytes("8b54241c")...)
	code, patch = x86.AppendStub(code, base, 0x39acab, x86.HexBytes("ff52288b54241c"), body)
	patches = append(patches, patch)

	body = x86.HexBytes("8bcd") // The interface pointer from AddBridgeEntry.
	body = append(body, x86.RelativeInstruction(0xe8, base+len(code)+len(body), button)...)
	code, patch = x86.AppendStub(code, base, 0x24650a, x86.HexBytes("8bcdff5730"), body)
	patches = append(patches, patch)

	body = x86.HexBytes("53556a02") // SetStyle args for the HUD restore button.
	body = append(body, x86.RelativeInstruction(0xe8, base+len(code)+len(body), button)...)
	code, patch = x86.AppendStub(code, base, 0x3ee084, x86.HexBytes("53556a02ff5030"), body)
	return code, append(patches, patch)
}
