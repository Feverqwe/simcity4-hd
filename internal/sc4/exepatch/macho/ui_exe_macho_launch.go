package macho

import (
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
)

func PatchLaunchSettings(data []byte, skipIntro, drawStats bool) error {
	var patches []x86.Patch
	if skipIntro {
		// Keep the temporary startup window's setup and teardown, including
		// its border measurement, and skip only the native movie loop.
		patches = append(patches,
			x86.Instruction(0x34b2, "c78594fbffff00000000", "e9a80200009090909090"),
			// Append -intro:off for the game engine as well.
			x86.Instruction(0x2f5b, "750f", "9090"),
		)
	}
	if drawStats {
		// Use Aspyr's FPS display. Register its debug dialog director, attach
		// the text to the renderer during initialization, then update it during paint.
		patches = append(patches,
			x86.Instruction(0x631f, "e87dba6300", "b801000000"),
			x86.Instruction(0x6cc18e, "e80e5cf7ff", "b801000000"),
			x86.Instruction(0x6cc87f, "e81d55f7ff", "b801000000"),
		)
	}
	return applyMachOInstructionPatches(data, patches)
}
