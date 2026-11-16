package sc4

import (
	_ "embed"
	"fmt"
)

//go:embed patchsrc/ui_runtime_art_macho.x86_64
var macRuntimeArtworkCode []byte

//go:embed patchsrc/ui_disasters.x86_64
var macDisasterBackgroundCode []byte

func appendMachODisasterBackground(code []byte, patches []instructionPatch) ([]byte, []instructionPatch) {
	const (
		site       = 0x78d983
		original   = "488b4308488945c0"
		loadByKey  = 0x043bce
		closeImage = 0x043df0
		factory    = 0x03f2d4 // RZ::GZCOM.
	)
	project := macUIOptionalCode + len(code)
	code = append(code, macDisasterBackgroundCode...)
	patches = append(patches, machOFeatureJump(code, site, original))
	// RBX owns the loaded fallback. The compositor reads its buffer slot next.
	code = little.AppendUint32(append(code, hexBytes("4889dfbe")...), disasterBackgroundX4)
	code = append(code, hexBytes("488d")...)
	code = append(code, relativeInstruction(0x15, macUIOptionalCode+len(code), loadByKey)...)
	code = append(code, hexBytes("488d")...)
	code = append(code, relativeInstruction(0x0d, macUIOptionalCode+len(code), closeImage)...)
	code = append(code, hexBytes("4c8d")...)
	code = append(code, relativeInstruction(0x05, macUIOptionalCode+len(code), factory)...)
	code = append(code, relativeInstruction(0xe8, macUIOptionalCode+len(code), project)...)
	code = append(code, hexBytes(original)...)
	code = append(code, relativeInstruction(0xe9, macUIOptionalCode+len(code), site+len(original)/2)...)
	return code, patches
}

func appendMachORuntimeArtwork(code []byte, patches []instructionPatch) ([]byte, []instructionPatch, error) {
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
	code = append(code, macRuntimeArtworkCode...)
	for _, site := range []int{0x78a3ce, 0x7933cc, 0x7a895d} { // Tooltip, city callout, tutorial callout.
		patches = append(patches, instructionPatch{site,
			relativeInstruction(0xe8, site, loadByKey), relativeInstruction(0xe8, site, entry)})
	}
	const alert = 0x787ae3
	patches = append(patches, instructionPatch{alert,
		relativeInstruction(0xe8, alert, loadByParts), relativeInstruction(0xe8, alert, entry+0x20)})

	const gaugeLoad = 0x6fa98a
	patches = append(patches, instructionPatch{gaugeLoad,
		relativeInstruction(0xe8, gaugeLoad, loadByParts), relativeInstruction(0xe8, gaugeLoad, entry+0x20)})
	// Only the Free Drive gauge needs projection when the stock strip won
	// the resource lookup. Keep that result with the native knob.
	const markGauge = 0x6fa98f
	const original = "488b7dd0488b07"
	patches = append(patches, machOFeatureJump(code, markGauge, original))
	code = append(code, hexBytes(original)...)
	code = append(code, hexBytes("80a78b010000bf81bd34ffffff")...) // Clear the flag and compare the loaded group.
	code = little.AppendUint32(code, runtimeArtworkGroup)
	code = append(code, hexBytes("7407808f8b01000040")...)
	code = append(code, relativeInstruction(0xe9, macUIOptionalCode+len(code), markGauge+7)...)

	const paintGauge = 0x7590fe
	const draw = "488bbba8000000"
	patches = append(patches, machOFeatureJump(code, paintGauge, draw))
	code = append(code, hexBytes("f6838b01000040741a4889df488d75c0488d55d0b9a8000000")...)
	code = append(code, relativeInstruction(0xe8, macUIOptionalCode+len(code), macUIOptionalCode+macBudgetTreeOffset)...)
	code = append(code, relativeInstruction(0xe9, macUIOptionalCode+len(code), 0x759115)...)
	code = append(code, hexBytes(draw)...)
	code = append(code, relativeInstruction(0xe9, macUIOptionalCode+len(code), paintGauge+7)...)
	return code, patches, nil
}
