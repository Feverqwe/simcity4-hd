package macho

import (
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
)

func appendMachOFontMetrics(code []byte, patches []x86.Patch) ([]byte, []x86.Patch) {
	const (
		site  = 0x2bd8a7
		next  = 0x2bd8d4
		scale = 0x8f5da0 // FT_MulFix.
	)
	address := func() int { return macUIOptionalCode + len(code) }
	metrics := address()
	code = append(code, patchsrc.MacFontMetricsCode...)
	original := "488b80800000008b70204489e7e8e78463004189c7498b068b785c488b80800000008b7020e8cf8463008945d4"
	patches = append(patches, machOFeatureJump(code, site, original))
	code = append(code, x86.HexBytes("4889c7")...) // Pass the FreeType face.
	code = append(code, x86.RelativeInstruction(0xe8, address(), metrics)...)
	// Keep the two corrected face bounds across the native scaling calls.
	code = append(code, x86.HexBytes("4989c74189c4498b06488b80800000008b70204489e7")...)
	code = append(code, x86.RelativeInstruction(0xe8, address(), scale)...)
	code = append(code, x86.HexBytes("4c89ff48c1ef204189c7498b06488b80800000008b7020")...)
	code = append(code, x86.RelativeInstruction(0xe8, address(), scale)...)
	code = append(code, x86.HexBytes("8945d4")...)
	code = append(code, x86.RelativeInstruction(0xe9, address(), next)...)
	return code, patches
}
