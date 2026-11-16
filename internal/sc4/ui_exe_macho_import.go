package sc4

import _ "embed"

//go:embed patchsrc/ui_import_node_macho.x86_64
var macImportNodeCode []byte

func appendMachOImportNodes(code []byte, patches []instructionPatch) ([]byte, []instructionPatch) {
	const fontManager = 0x127da0 // RZ::WCM()
	entry := macUIOptionalCode + len(code)
	code = append(code, 0x4c, 0x8d) // Pass the font manager getter in R9.
	code = append(code, relativeInstruction(0x0d, entry+2, fontManager)...)
	code = append(code, macImportNodeCode...)
	for _, site := range []int{0x3d7f7d, 0x3d8105} { // Folder and city insertions.
		call := append(relativeInstruction(0xe8, site, entry), 0x90)
		patches = append(patches, instructionPatch{site, hexBytes("ff90c0000000"), call})
	}
	return code, patches
}
