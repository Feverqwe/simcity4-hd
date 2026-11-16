package sc4

import _ "embed"

//go:embed patchsrc/ui_map_terrain_macho.x86_64
var macDataViewTerrainCode []byte

//go:embed patchsrc/data_view_legend_macho.x86_64
var macDataViewLegendCode []byte

//go:embed patchsrc/data_view_coverage_macho.x86_64
var macDataViewCoverageCode []byte

func appendMachODataViews(code []byte, patches []instructionPatch) ([]byte, []instructionPatch) {
	// DisplayLegend repositions both columns when the overlay changes.
	// Keep the horizontal bounds from whichever panel layout is active.
	entry := macUIOptionalCode + len(code)
	code = append(code, macDataViewLegendCode...)
	for _, site := range []int{0x34d2fe, 0x34d458} {
		call := append(relativeInstruction(0xe8, site, entry), 0x90)
		patches = append(patches, instructionPatch{site, hexBytes("ff90b8010000"), call})
	}
	patches = append(patches,
		instruction(0x34d2a3, "8d583d", "8d587a"), // Swatch top.
		instruction(0x34d3dc, "8d5818", "8d5830"), // Caption top.
	)

	// Update already projects each ring on the final bitmap. Scale the
	// stroke and its scan margins when that bitmap is at least 512 px wide.
	entry = macUIOptionalCode + len(code)
	code = append(code, macDataViewCoverageCode...)
	for i, hook := range []struct {
		site     int
		original string
	}{
		{0x34bd9e, "f30f100d0e9b6c00"}, // First row margin.
		{0x34bdf6, "f30f100dbe9a6c00"}, // Last row margin.
		{0x34be58, "f30f580530926c00"}, // Scan radius.
		{0x34bf91, "f30f100df7a66c00"}, // Stroke extent.
		{0x34bf9e, "f30f59055aa26c00"}, // Opacity slope.
	} {
		call := relativeInstruction(0xe8, hook.site, entry+0x40*i)
		patches = append(patches, instructionPatch{hook.site, hexBytes(hook.original), append(call, 0x90, 0x90, 0x90)})
	}
	return appendMachODataViewTerrain(code, patches)
}

func appendMachODataViewTerrain(code []byte, patches []instructionPatch) ([]byte, []instructionPatch) {
	const (
		dispatch = 0x5c6912
		next     = 0x5c698d
		original = "418b818c010000" // Read the stock minimap zoom shift.
	)
	const alignment = 16
	aligned := (len(code) + alignment - 1) &^ (alignment - 1)
	code = append(code, make([]byte, aligned-len(code))...)
	sampler := macUIOptionalCode + len(code)
	code = append(code, macDataViewTerrainCode...)
	patches = append(patches, machOFeatureJump(code, dispatch, original))

	// Pass the minimap and the stock chunk frame. Reload the caller's
	// minimap register before running either continuation.
	code = append(code, hexBytes("4c89cf488db570f3ffff")...) // rdi = r9, rsi = rbp - 0xc90.
	code = append(code, relativeInstruction(0xe8, macUIOptionalCode+len(code), sampler)...)
	code = append(code, hexBytes("4c8b8da8f3ffff85c07405")...) // Restore r9 and test the result.
	code = append(code, relativeInstruction(0xe9, macUIOptionalCode+len(code), next)...)
	code = append(code, hexBytes(original)...)
	code = append(code, relativeInstruction(0xe9, macUIOptionalCode+len(code), dispatch+len(original)/2)...)
	return code, patches
}
