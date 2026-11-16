package sc4

import _ "embed"

//go:embed patchsrc/ui_combo_arrow_macho.x86_64
var macComboCode []byte

func appendMachOCombo(code []byte, patches []instructionPatch) ([]byte, []instructionPatch) {
	const layoutSite, wheelSite = 0x2f50a9, 0x16d300
	entry := macUIOptionalCode + len(code)
	// Compute the arrow width before sizing the text area on each layout.
	call := append(relativeInstruction(0xe8, layoutSite, entry), 0x90)
	wheel := append(relativeInstruction(0xe9, wheelSite, entry+0x60), 0x90, 0x90, 0x90)
	code = append(code, macComboCode...)
	context := macUIOptionalCode + len(code)
	for _, target := range []int{0xa8c030, 0xa8d8a0, 0xadfc70, 0x2737fa} {
		code = little.AppendUint32(code, uint32(target-context))
	}
	return code, append(patches,
		instructionPatch{layoutSite, hexBytes("2bb3c0010000"), call},
		instructionPatch{wheelSite, hexBytes("554889e531c05dc3"), wheel},
	)
}
