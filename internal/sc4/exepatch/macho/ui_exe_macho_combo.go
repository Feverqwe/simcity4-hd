package macho

import (
	"encoding/binary"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
)

func appendMachOCombo(code []byte, patches []x86.Patch) ([]byte, []x86.Patch) {
	const layoutSite, wheelSite = 0x2f50a9, 0x16d300
	entry := macUIOptionalCode + len(code)
	// Compute the arrow width before sizing the text area on each layout.
	call := append(x86.RelativeInstruction(0xe8, layoutSite, entry), 0x90)
	wheel := append(x86.RelativeInstruction(0xe9, wheelSite, entry+0x60), 0x90, 0x90, 0x90)
	code = append(code, patchsrc.MacComboCode...)
	context := macUIOptionalCode + len(code)
	for _, target := range []int{0xa8c030, 0xa8d8a0, 0xadfc70, 0x2737fa} {
		code = binary.LittleEndian.AppendUint32(code, uint32(target-context))
	}
	return code, append(patches,
		x86.Patch{RVA: layoutSite, Original: x86.HexBytes("2bb3c0010000"), Replacement: call},
		x86.Patch{RVA: wheelSite, Original: x86.HexBytes("554889e531c05dc3"), Replacement: wheel},
	)
}
