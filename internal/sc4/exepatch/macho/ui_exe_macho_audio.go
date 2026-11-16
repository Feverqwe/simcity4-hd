package macho

import (
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
)

func appendMachOPlaylist(code []byte, patches []x86.Patch) ([]byte, []x86.Patch) {
	patches = append(patches,
		x86.Instruction(0x3a9934, "b9ff000000", "b9fe010000"),                     // Track-name column.
		x86.Instruction(0x3aac47, "48b8acdb6d85b006a046", "48b8acdb6d85c8404ae7"), // Both music lists.
	)

	// Both virtual entry points have converted the wheel delta to rows.
	entry := macUIOptionalCode + len(code)
	code = append(code, patchsrc.MacPlaylistWheelCode...)
	for _, site := range []int{0x252c5c, 0x252c89} {
		call := append(x86.RelativeInstruction(0xe8, site, entry), 0x90)
		patches = append(patches, x86.Patch{RVA: site, Original: x86.HexBytes("ff90f8000000"), Replacement: call})
	}
	return code, patches
}
