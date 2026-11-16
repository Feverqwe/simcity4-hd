package sc4

import _ "embed"

//go:embed patchsrc/ui_playlist_wheel.x86_64
var macPlaylistWheelCode []byte

func appendMachOPlaylist(code []byte, patches []instructionPatch) ([]byte, []instructionPatch) {
	patches = append(patches,
		instruction(0x3a9934, "b9ff000000", "b9fe010000"),                     // Track-name column.
		instruction(0x3aac47, "48b8acdb6d85b006a046", "48b8acdb6d85c8404ae7"), // Both music lists.
	)

	// Both virtual entry points have converted the wheel delta to rows.
	entry := macUIOptionalCode + len(code)
	code = append(code, macPlaylistWheelCode...)
	for _, site := range []int{0x252c5c, 0x252c89} {
		call := append(relativeInstruction(0xe8, site, entry), 0x90)
		patches = append(patches, instructionPatch{site, hexBytes("ff90f8000000"), call})
	}
	return code, patches
}
