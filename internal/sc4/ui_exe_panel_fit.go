package sc4

import _ "embed"

//go:embed patchsrc/ui_panel_fit.i386
var windowsPanelFitCode []byte

//go:embed patchsrc/ui_panel_fit.x86_64
var macPanelFitCode []byte

func appendAnchoredPanelFit(code []byte, patches []instructionPatch, base int, mac bool) ([]byte, []instructionPatch) {
	site, original, payload := 0x38d7fe, "ff90e4000000", windowsPanelFitCode
	if mac {
		site, original, payload = 0x30acfb, "ff90c8010000", macPanelFitCode
	}
	// The native anchor has calculated the move. Keep wide HUD panels inside
	// the viewport while preserving their own pixel geometry.
	redirect := relativeInstruction(0xe8, site, base+len(code))
	return append(code, payload...), append(patches, instructionPatch{site, hexBytes(original), append(redirect, 0x90)})
}
