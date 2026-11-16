package sc4

import _ "embed"

//go:embed patchsrc/live_display_macho.x86_64
var macLiveDisplayCode []byte

const macDisplaySwitchHint = "SC4_UHD_INTERNAL_DISPLAY_SWITCH"

func appendMachOLiveDisplay(code []byte, patches []instructionPatch) ([]byte, []instructionPatch, int) {
	const (
		pumpSite = 0x2aaf
		pump     = 0x8d6550
	)
	address := func() int { return macUIOptionalCode + len(code) }
	entry := address()
	code = append(code, macLiveDisplayCode...)
	context := address()
	for _, target := range []int{
		0x63f907, 0x8a8c30, 0x8a8af0, // SDL window and window data
		0xb6ed38, 0x127cc8, 0x60d536, // Active canvas, RZ::GS, cGDriver::SetVideoMode thunk
		0x61a686, 0x61a728, 0x8a97c0, // Canvas teardown and restore, SDL_SetWindowFullscreen
		0x60cc86, 0x60cf32, 0xb117b8, // Video-mode enumeration and desired screen metrics
		0x63f93c, 0x92aae4, 0x92aa30, // CoreGraphics display and mode
		0x92aa66, 0x92aa60, 0x92aa78, // Pixel dimensions and release
		0x8a5bc0, 0x641a6a, // SDL video device and macOS lock state
		0x641c53, 0x641cbe, 0x642135, 0xb13130, // Preferences and active window mode
		0x127d34,           // RZ::WM
		0xb8f3d8, 0x8e0340, // PrimaryContext, SDL_SetHintWithPriority
		0x617e76,           // Pause drawing if neither mode could restore a device.
		0xae2cc0, 0xa88520, // cWinSC4App and cSC4WinRegionScreen vtable address points
		0x30aaaa, 0x76c090, 0x237632, // AlignWindowToWindow, camera viewport, region position
		0xaebdc0, 0xb90ed0, 0xb73810, // cSC43DRender vtable, backing FBO and driver
		0x607168, 0x7699ee, 0x8aa8b0, // Reset viewport, CreateBackingStore, SDL_GL_GetProcAddress
		0x92b13e, 0x92b1da, // glBindFramebufferEXT, glDeleteFramebuffersEXT
		0xb03c50, 0xad8580, 0xadd780, // cSC4WinRegion3DView, cGZScreenBuffer, cGZDrawContextAsBuffer
		0xadd5d0, 0xaf02e0, // cGZDrawContext and cGZDrawContextRenderToSimGL
		0x64f094, 0x64f17e, // ClipReset and RecomputeClipRect
		0xaeb760, 0x75a8fa, 0x75bb02, // cSC4WinRegionView, canvas ScrollTo, InvalidateAllRegions
		0x237106,           // ComputeBaseLimits also refreshes the cloud area.
		0xb17890,           // cSC43DRender::mbBackingStoreEnabled.
		0x1745da, 0x76c7a4, // SC4ViewUtilities camera update and cSC4CameraControl scroll bounds.
		0x019fcf, // RZAtoUint32HexA, used by the native window alignment helper.
	} {
		code = little.AppendUint32(code, uint32(target-context))
	}
	for _, field := range []struct {
		value string
		size  int
	}{
		{macDisplayPreference, 16},
		{macDisplayPending, 24},
		{macDisplayFailed, 24},
		{macDisplaySwitchHint, 32},
		{"1", 2},
		{"0", 2},
		{"glGetFramebufferAttachmentParameterivEXT", 48},
		{"glDeleteRenderbuffersEXT", 28},
		{"sc4.uhd.scene-redraw", 24},
	} {
		code = append(code, field.value...)
		code = append(code, make([]byte, field.size-len(field.value))...)
	}
	// The outer framework loop runs after the Graphics Options modal has
	// unwound. Its event pump runs before the next draw.
	stub := address()
	code = append(code, hexBytes("4883ec0831f6488d")...)
	code = append(code, relativeInstruction(0x3d, address(), context)...) // lea rdi,[rip+context]
	code = append(code, relativeInstruction(0xe8, address(), entry)...)
	code = append(code, hexBytes("4883c408")...)
	code = append(code, relativeInstruction(0xe9, address(), pump)...)
	patches = append(patches, instructionPatch{
		pumpSite,
		relativeInstruction(0xe8, pumpSite, pump),
		relativeInstruction(0xe8, pumpSite, stub),
	})

	drawStub := address()
	for _, drawSite := range []int{0x23d581, 0x7816f1} {
		patches = append(patches, instructionPatch{
			drawSite, hexBytes("ff90a8000000"),
			append(relativeInstruction(0xe8, drawSite, drawStub), 0x90),
		})
	}
	code = append(code, hexBytes("4889fe488d")...) // Pass renderer and context.
	code = append(code, relativeInstruction(0x3d, address(), context)...)
	code = append(code, relativeInstruction(0xe9, address(), entry)...)

	// Aspyr's SetupDisplay returns zero even when SDL_CreateWindow fails.
	// Divert a null window to the driver's ordinary invalid-mode path.
	const windowSite = 0x60d29a
	original := hexBytes("48898350010000")
	patches = append(patches, instructionPatch{
		windowSite, original,
		append(relativeInstruction(0xe9, windowSite, address()), 0x90, 0x90),
	})
	code = append(code, original...)
	code = append(code, hexBytes("4885c00f")...)
	code = append(code, relativeInstruction(0x84, address(), 0x60d303)...)
	code = append(code, relativeInstruction(0xe9, address(), windowSite+len(original))...)
	return appendMachOLiveDisplayRender(code, patches)
}
