package sc4

func patchMachOLaunchSettings(data []byte, skipIntro, drawStats bool) error {
	var patches []instructionPatch
	if skipIntro {
		// Keep the temporary startup window's setup and teardown, including
		// its border measurement, and skip only the native movie loop.
		patches = append(patches,
			instruction(0x34b2, "c78594fbffff00000000", "e9a80200009090909090"),
			// Append -intro:off for the game engine as well.
			instruction(0x2f5b, "750f", "9090"),
		)
	}
	if drawStats {
		// Use Aspyr's FPS display. Register its debug dialog director, attach
		// the text to the renderer during initialization, then update it during paint.
		patches = append(patches,
			instruction(0x631f, "e87dba6300", "b801000000"),
			instruction(0x6cc18e, "e80e5cf7ff", "b801000000"),
			instruction(0x6cc87f, "e81d55f7ff", "b801000000"),
		)
	}
	return applyMachOInstructionPatches(data, patches)
}
