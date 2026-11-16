package macho

import (
	"encoding/binary"
	"fmt"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/uiids"
)

func appendMachODisasterBackground(code []byte, patches []x86.Patch) ([]byte, []x86.Patch) {
	const (
		site       = 0x78d983
		original   = "488b4308488945c0"
		loadByKey  = 0x043bce
		closeImage = 0x043df0
		factory    = 0x03f2d4 // RZ::GZCOM.
	)
	project := macUIOptionalCode + len(code)
	code = append(code, patchsrc.MacDisasterBackgroundCode...)
	patches = append(patches, machOFeatureJump(code, site, original))
	// RBX owns the loaded fallback. The compositor reads its buffer slot next.
	code = binary.LittleEndian.AppendUint32(append(code, x86.HexBytes("4889dfbe")...), uiids.DisasterBackgroundX4)
	code = append(code, x86.HexBytes("488d")...)
	code = append(code, x86.RelativeInstruction(0x15, macUIOptionalCode+len(code), loadByKey)...)
	code = append(code, x86.HexBytes("488d")...)
	code = append(code, x86.RelativeInstruction(0x0d, macUIOptionalCode+len(code), closeImage)...)
	code = append(code, x86.HexBytes("4c8d")...)
	code = append(code, x86.RelativeInstruction(0x05, macUIOptionalCode+len(code), factory)...)
	code = append(code, x86.RelativeInstruction(0xe8, macUIOptionalCode+len(code), project)...)
	code = append(code, x86.HexBytes(original)...)
	code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), site+len(original)/2)...)
	return code, patches
}

func appendMachORuntimeArtwork(code []byte, patches []x86.Patch) ([]byte, []x86.Patch, error) {
	const (
		offset      = 0x4400
		loadByKey   = 0x043bce
		loadByParts = 0x043c72
	)
	if len(code) > offset {
		return nil, nil, fmt.Errorf("Mach-O features overlap the runtime artwork hooks")
	}
	entry := macUIOptionalCode + offset
	code = append(code, make([]byte, offset-len(code))...)
	code = append(code, patchsrc.MacRuntimeArtworkCode...)
	for _, site := range []int{0x78a3ce, 0x7933cc, 0x7a895d} { // Tooltip, city callout, tutorial callout.
		patches = append(patches, x86.Patch{RVA: site,
			Original: x86.RelativeInstruction(0xe8, site, loadByKey), Replacement: x86.RelativeInstruction(0xe8, site, entry)})
	}
	const alert = 0x787ae3
	patches = append(patches, x86.Patch{RVA: alert,
		Original: x86.RelativeInstruction(0xe8, alert, loadByParts), Replacement: x86.RelativeInstruction(0xe8, alert, entry+0x20)})

	const gaugeLoad = 0x6fa98a
	patches = append(patches, x86.Patch{RVA: gaugeLoad,
		Original: x86.RelativeInstruction(0xe8, gaugeLoad, loadByParts), Replacement: x86.RelativeInstruction(0xe8, gaugeLoad, entry+0x20)})
	// Only the Free Drive gauge needs projection when the stock strip won
	// the resource lookup. Keep that result with the native knob.
	const markGauge = 0x6fa98f
	const original = "488b7dd0488b07"
	patches = append(patches, machOFeatureJump(code, markGauge, original))
	code = append(code, x86.HexBytes(original)...)
	code = append(code, x86.HexBytes("80a78b010000bf81bd34ffffff")...) // Clear the flag and compare the loaded group.
	code = binary.LittleEndian.AppendUint32(code, uiids.RuntimeArtworkGroup)
	code = append(code, x86.HexBytes("7407808f8b01000040")...)
	code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), markGauge+7)...)

	const paintGauge = 0x7590fe
	const draw = "488bbba8000000"
	patches = append(patches, machOFeatureJump(code, paintGauge, draw))
	code = append(code, x86.HexBytes("f6838b01000040741a4889df488d75c0488d55d0b9a8000000")...)
	code = append(code, x86.RelativeInstruction(0xe8, macUIOptionalCode+len(code), macUIOptionalCode+macBudgetTreeOffset)...)
	code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), 0x759115)...)
	code = append(code, x86.HexBytes(draw)...)
	code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), paintGauge+7)...)
	return code, patches, nil
}
