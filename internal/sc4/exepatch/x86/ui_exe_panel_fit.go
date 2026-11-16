package x86

import (
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
)

func AppendAnchoredPanelFit(code []byte, patches []Patch, base int, mac bool) ([]byte, []Patch) {
	site, original, payload := 0x38d7fe, "ff90e4000000", patchsrc.WindowsPanelFitCode
	if mac {
		site, original, payload = 0x30acfb, "ff90c8010000", patchsrc.MacPanelFitCode
	}
	// The native anchor has calculated the move. Keep wide HUD panels inside
	// the viewport while preserving their own pixel geometry.
	redirect := RelativeInstruction(0xe8, site, base+len(code))
	return append(code, payload...), append(patches, Patch{site, HexBytes(original), append(redirect, 0x90)})
}
