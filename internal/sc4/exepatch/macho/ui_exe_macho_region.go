package macho

import (
	"bytes"
	"encoding/binary"
	"math"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
)

const macRegionTilesSite = 0x236e03

func appendMachORegionMap(data, code []byte, patches []x86.Patch) ([]byte, []x86.Patch) {
	const graphicSystem = 0x127cc8 // RZ::GS()

	// RecalculateImagesFromCitySnapshots uses rbx for the city and keeps
	// its four planes at +0x20. Expand after fractional alignment, before
	// allocating the derived buffer and generating the drawing and hit spans.
	expander := macUIOptionalCode + len(code)
	code = append(code, patchsrc.MacRegionTileCode...)
	stub := macUIOptionalCode + len(code)
	code = append(code, x86.HexBytes("488d7b20488d")...) // lea rdi, [rbx+0x20], then load GS into rsi.
	code = append(code, x86.RelativeInstruction(0x35, macUIOptionalCode+len(code), graphicSystem)...)
	code = append(code, x86.RelativeInstruction(0xe8, macUIOptionalCode+len(code), expander)...)
	const readGraphicSystem = "4d8bac2448020000"
	code = append(code, x86.HexBytes(readGraphicSystem)...)
	code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), macRegionTilesSite+8)...)
	patches = append(patches, x86.Patch{RVA: macRegionTilesSite, Original: x86.HexBytes(readGraphicSystem),
		Replacement: append(x86.RelativeInstruction(0xe9, macRegionTilesSite, stub), 0x90, 0x90, 0x90)})

	// All consumers use the same rounded anchor. Preserve the stock
	// fractional alignment and apply the doubled anchor before panning.
	for _, anchor := range []struct {
		rva      int
		original string
		add      string
	}{
		{0x75b6c5, "66410f3a0a461009", "f30f58c0"}, // Invalidation.
		{0x75b6e3, "66410f3a0a4e1409", "f30f58c9"},
		{0x75c353, "660f3a0a8570ffffff09", "f30f58c0"}, // Compositor, including overlays.
		{0x75c36c, "660f3a0a45c809", "f30f58c0"},
		{0x75c9df, "660f3a0a8560ffffff09", "f30f58c0"},
		{0x75c9f5, "660f3a0a8558ffffff09", "f30f58c0"},
		{0x75cccc, "66410f3a0a461009", "f30f58c0"},
		{0x75cceb, "66410f3a0a4e1409", "f30f58c9"},
		{0x75daf2, "66410f3a0a471009", "f30f58c0"}, // Screen rectangle and signposts.
		{0x75db10, "66410f3a0a4f1409", "f30f58c9"},
		{0x75dbee, "660f3a0a431009", "f30f58c0"}, // Hit testing.
		{0x75dc45, "660f3a0a45c809", "f30f58c0"},
		{0x75ddd8, "66410f3a0a461009", "f30f58c0"}, // Playfield rectangle.
		{0x75ddf8, "66410f3a0a4e1409", "f30f58c9"},
	} {
		original := x86.HexBytes(anchor.original)
		replacement := x86.RelativeInstruction(0xe9, anchor.rva, macUIOptionalCode+len(code))
		replacement = append(replacement, bytes.Repeat([]byte{0x90}, len(original)-len(replacement))...)
		code = append(code, original...)
		code = append(code, x86.HexBytes(anchor.add)...)
		code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), anchor.rva+len(original))...)
		patches = append(patches, x86.Patch{RVA: anchor.rva, Original: original, Replacement: replacement})
	}

	// Scale the pan limits, the regional 3D camera, and projected map icons
	// at their consumers. Some constants are shared with source paths.
	const savedCenterScale = 0xa19694
	doubled := make(map[int]int)
	for _, projected := range []struct {
		rva, source int
		opcode      string
	}{
		{0x237168, 0xa1b644, "f30f100d"}, // Pan limits.
		{0x237178, 0xa1b654, "f30f101d"},
		{0x2371a9, 0xa1b64c, "f30f5905"},
		{0x2371b1, 0xa1b650, "f30f5915"},
		{0x23722e, 0xa1b658, "f30f5805"},
		{0x238e47, 0xa169a8, "660f6e05"}, // Region-only 3D camera magnification.
		{0x238528, 0xa1b65c, "f30f5905"}, // Restore the saved map center.
		{0x23855e, 0xa1b65c, "f30f590d"},
		{0x23b8e9, savedCenterScale, "f30f100d"}, // Save in the stock coordinate system.
		{0x75d6ba, 0xa2a040, "f30f1005"},         // Projected city map icons.
		{0x75d6c6, 0xa2a044, "f30f100d"},
		{0x75d6d7, 0xa2a048, "f30f1015"},
		{0x75d6ec, 0xa2a04c, "f30f1005"},
		{0x75d7a1, 0xa2a040, "f30f5915"},
		{0x75d7a9, 0xa2a044, "f30f5905"},
		{0x75d7c0, 0xa2a050, "f30f590d"},
		{0x75d7d6, 0xa2a048, "f30f5915"},
		{0x75d7e7, 0xa2a04c, "f30f5905"},
	} {
		target, exists := doubled[projected.source]
		if !exists {
			target = macUIOptionalCode + len(code)
			doubled[projected.source] = target
			value := math.Float32frombits(binary.LittleEndian.Uint32(data[projected.source:]))
			scale := float32(2)
			if projected.source == savedCenterScale {
				scale = 0.5
			}
			code = binary.LittleEndian.AppendUint32(code, math.Float32bits(value*scale))
		}
		opcode := x86.HexBytes(projected.opcode)
		next := projected.rva + len(opcode) + 4
		original := binary.LittleEndian.AppendUint32(x86.HexBytes(projected.opcode), uint32(projected.source-next))
		replacement := binary.LittleEndian.AppendUint32(opcode, uint32(target-next))
		patches = append(patches, x86.Patch{RVA: projected.rva, Original: original, Replacement: replacement})
	}
	return code, patches
}
