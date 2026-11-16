package sc4

import _ "embed"

//go:embed patchsrc/ui_snapshot_macho.x86_64
var macSnapshotCode []byte

func appendMachOSnapshots(code []byte, patches []instructionPatch) ([]byte, []instructionPatch) {
	const (
		alignment = 16
		presets   = 0x3a04e8
		resume    = 0x3a07a7
		expand    = 0x3a2a64 // cRZSize vector growth.
		inset     = 0x3a10f6
		save      = 0x3a1458
		saveState = "4c8b75a0448b7dd0"
	)
	aligned := (len(code) + alignment - 1) &^ (alignment - 1)
	code = append(code, make([]byte, aligned-len(code))...)
	entry := macUIOptionalCode + len(code)
	code = append(code, macSnapshotCode...)

	patches = append(patches, machOFeatureJump(code, presets, "48b8a000000078000000"))
	// Pass the size vector, current viewport and native growth function.
	code = append(code, hexBytes("4c89f74489fe8b55ac488d")...)
	code = append(code, relativeInstruction(0x0d, macUIOptionalCode+len(code), expand)...)
	code = append(code, relativeInstruction(0xe8, macUIOptionalCode+len(code), entry)...)
	code = append(code, relativeInstruction(0xe9, macUIOptionalCode+len(code), resume)...)

	patches = append(patches, machOFeatureJump(code, save, saveState))
	// Leave the album and file writer alone when the driver made no bitmap.
	code = append(code, hexBytes("4d85e47505")...) // test r12, r12; jne restore.
	code = append(code, relativeInstruction(0xe9, macUIOptionalCode+len(code), 0x3a155d)...)
	code = append(code, hexBytes(saveState)...)
	code = append(code, relativeInstruction(0xe9, macUIOptionalCode+len(code), save+len(saveState)/2)...)

	return code, append(patches,
		instruction(0x3a0439, "b90e000000", "b91c000000"), // Fallback font size.
		instructionPatch{inset, hexBytes("660ffe05e2d36700"),
			append(relativeInstruction(0xe8, inset, entry+0xc0), 0x90, 0x90, 0x90)},
		instruction(0x3a127f, "0f84db010000", "0f84d8020000"), // No graphics driver.
		// A failed capture can use the nearby null-bitmap cleanup. Keep the
		// following instruction, which the successful capture jumps to.
		instruction(0x3a1354, "31db", "ebf6"), // Jump to 0x3a134c.
	)
}
