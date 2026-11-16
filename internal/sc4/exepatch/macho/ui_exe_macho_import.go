package macho

import (
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
)

func appendMachOImportNodes(code []byte, patches []x86.Patch) ([]byte, []x86.Patch) {
	const fontManager = 0x127da0 // RZ::WCM()
	entry := macUIOptionalCode + len(code)
	code = append(code, 0x4c, 0x8d) // Pass the font manager getter in R9.
	code = append(code, x86.RelativeInstruction(0x0d, entry+2, fontManager)...)
	code = append(code, patchsrc.MacImportNodeCode...)
	for _, site := range []int{0x3d7f7d, 0x3d8105} { // Folder and city insertions.
		call := append(x86.RelativeInstruction(0xe8, site, entry), 0x90)
		patches = append(patches, x86.Patch{RVA: site, Original: x86.HexBytes("ff90c0000000"), Replacement: call})
	}
	return code, patches
}
