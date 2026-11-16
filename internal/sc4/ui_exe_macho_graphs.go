package sc4

import (
	"bytes"
	_ "embed"
)

//go:embed patchsrc/ui_graphs_macho.x86_64
var macGraphsCode []byte

func appendMachOGraphs(code []byte, patches []instructionPatch) ([]byte, []instructionPatch) {
	// Reserve room for the larger axis text and for both styles of legend.
	patches = append(patches,
		instruction(0x2e0bfd, "83c2ec", "83c2e0"),
		instruction(0x2e0c03, "49b82d00000014000000", "49b84600000020000000"),
		instruction(0x2e0d0b, "83c2ec", "83c2e0"),
		instruction(0x2e0d11, "48bb2d00000014000000", "48bb4600000020000000"),
	)

	entry := macUIOptionalCode + len(code)
	code = append(code, macGraphsCode...)
	for i, hook := range []struct {
		site     int
		original string
	}{
		{0x2e0d05, "83c1922b4d88"},     // Legend-bearing plot's right edge.
		{0x2e1460, "8d719483c1a4"},     // Bitmap swatch button edges.
		{0x2e152d, "4183c7a64129df"},   // Legend offset with a bitmap button.
		{0x2e153a, "4183c796442b7d88"}, // Legend offset without a bitmap button.
		{0x37e42f, "ff90f8070000"},     // Draw one data series segment.
	} {
		original := hexBytes(hook.original)
		call := relativeInstruction(0xe8, hook.site, entry+0x10*i)
		replacement := append(call, bytes.Repeat([]byte{0x90}, len(original)-len(call))...)
		patches = append(patches, instructionPatch{hook.site, original, replacement})
	}
	return code, patches
}
