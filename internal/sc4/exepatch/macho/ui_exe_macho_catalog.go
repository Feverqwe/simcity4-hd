package macho

import (
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
)

func appendMachOCatalogPatches(code []byte, patches []x86.Patch) ([]byte, []x86.Patch) {
	const base = macUIOptionalCode
	paint := base + len(code)
	code = append(code, patchsrc.MacCatalogPaint...)
	tile := base + len(code)
	code = append(code, patchsrc.MacCatalogTile...)
	height := base + len(code)
	code = append(code, patchsrc.MacCatalogHeight...)
	button := base + len(code)
	code = append(code, patchsrc.MacCatalogButton...)

	const (
		stretch   = 0x6a391b // RZDrawBltStretch
		stockTile = 0x6a3062 // RZDrawBltTiled
	)
	for _, site := range []struct {
		rva              int
		setup, stockCall string
	}{
		{0x2fe2b6, "", "ff90f8000000"}, // Catalog item.
		{0x2fe8d5, "4c89f9", "41ffd1"}, // Chrome top arrow.
		{0x2fe9f4, "4c89e1", "41ffd1"}, // Chrome bottom arrow.
		{0x2fea52, "4531c0", "41ffd1"}, // Chrome cap.
	} {
		entry := base + len(code)
		body := append(x86.HexBytes(site.setup), 0x4c, 0x8d)
		body = append(body, x86.RelativeInstruction(0x0d, entry+len(body), stretch)...) // lea r9, [rip + stretch]
		body = append(body, x86.RelativeInstruction(0xe8, entry+len(body), paint)...)
		var patch x86.Patch
		code, patch = x86.AppendStub(code, base, site.rva, x86.HexBytes(site.setup+site.stockCall), body)
		patches = append(patches, patch)
	}
	const tileSite = 0x2fe960
	entry := base + len(code)
	body := []byte{0x4c, 0x8d}
	body = append(body, x86.RelativeInstruction(0x05, entry+len(body), stretch)...) // lea r8, [rip + stretch]
	body = append(body, 0x4c, 0x8d)
	body = append(body, x86.RelativeInstruction(0x0d, entry+len(body), stockTile)...) // lea r9, [rip + stockTile]
	body = append(body, x86.RelativeInstruction(0xe8, entry+len(body), tile)...)
	var patch x86.Patch
	code, patch = x86.AppendStub(code, base, tileSite, x86.RelativeInstruction(0xe8, tileSite, stockTile), body)
	patches = append(patches, patch)

	body = x86.RelativeInstruction(0xe8, base+len(code), height)
	body = append(body, x86.HexBytes("4501e4")...)
	code, patch = x86.AppendStub(code, base, 0x2feafe, x86.HexBytes("ff50604501e4"), body)
	patches = append(patches, patch)

	body = x86.HexBytes("4889df89c6") // rdi = button; esi = style.
	body = append(body, x86.RelativeInstruction(0xe8, base+len(code)+len(body), button)...)
	code, patch = x86.AppendStub(code, base, 0x721f7a, x86.HexBytes("4889df89c641ffd6"), body)
	patches = append(patches, patch)

	body = x86.HexBytes("4889da") // SetStyle buffer for the HUD restore button.
	body = append(body, x86.RelativeInstruction(0xe8, base+len(code)+len(body), button)...)
	code, patch = x86.AppendStub(code, base, 0x789525, x86.HexBytes("4889daff5060"), body)
	patches = append(patches, patch)

	// The stock draw context subtracts the unscaled crop origin. Once the
	// native stretch selects a 2× source, map source*scale+offset to dest.
	code, patch = x86.AppendStub(code, base, 0x64fbcd,
		x86.HexBytes("4129f9f3410f2ac14129daf3410f2acaf30f2ad2f30f2ad8f30f5ed30f57dbf3410f2ad8f30f2ae1f30f5edc4c89f7"),
		x86.HexBytes("f3410f2ac1f3410f2acaf30f2ad2f30f2ad8f30f5ed3f3410f2ad8f30f2ae1f30f5edcf30f2ae7f30f2aebf30f59e2f30f59ebf30f5cc4f30f5ccd4c89f7"))
	return code, append(patches, patch)
}
