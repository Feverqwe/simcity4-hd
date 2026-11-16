package macho

import (
	"encoding/binary"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
)

func appendMachOLiveDisplayRender(code []byte, patches []x86.Patch) ([]byte, []x86.Patch, int) {
	address := func() int { return macUIOptionalCode + len(code) }
	body := address()
	code = append(code, patchsrc.MacLiveDisplayRenderCode...)
	context := address()
	for _, target := range []int{
		0x8e04a0, 0x60b26e, 0x60b3a0, 0x63fcbe, 0x63fb99, // SDL hint and stock lifecycle
		0xb8f3d8, 0xb8f4e8, 0x8496a0, 0x849f50, // PrimaryContext, SDL window, setWindow:, Cocoa_GL_MakeCurrent
		0x4edc8c, 0x92aade, 0x92aad2, // CGL context and backing size
		0x849600, 0x849620, 0x92b228, 0x92b1f8, // Drawable update, glFinish, glDisable
		0xb8f4d4, 0x92b36c, // Render size and glViewport
		0x8a89b0, 0x8a97c0, 0x8a9040, 0x4edc2e, // SDL window flags, mode, size and placement
		0x63f8d5, 0x8a8fb0, // MacDisplay::ReleaseDisplay, SDL_SetWindowResizable
		0x8a6790, 0x8d6740, 0x8d6750, // SDL window display, usable bounds and borders
	} {
		code = binary.LittleEndian.AppendUint32(code, uint32(target-context))
	}
	code = append(code, macDisplaySwitchHint...)
	code = append(code, make([]byte, 32-len(macDisplaySwitchHint))...)

	for operation, hooks := range [][]struct {
		site, target int
		opcode       byte
	}{
		{{0x60d3cd, 0x60b3a0, 0xe8}},
		{{0x60d4ac, 0x60b26e, 0xe8}},
		{{0x63ce89, 0x63fcbe, 0xe8}, {0x63daa1, 0x63fcbe, 0xe9}},
		{{0x63cea5, 0x63fb99, 0xe8}},
		// DestroyViewport releases the SDL window after tearing down the driver.
		{{0x60d3ef, 0x63f8d5, 0xe9}},
	} {
		stub := address()
		// Move the native arguments behind the operation selector and context.
		code = append(code, x86.HexBytes("4989c94989d04889f14889fa")...)
		code = append(code, 0xbe, byte(operation), 0, 0, 0)
		code = append(code, 0x48, 0x8d)
		code = append(code, x86.RelativeInstruction(0x3d, address(), context)...)
		code = append(code, x86.RelativeInstruction(0xe9, address(), body)...)
		for _, hook := range hooks {
			patches = append(patches, x86.Patch{
				RVA:         hook.site,
				Original:    x86.RelativeInstruction(hook.opcode, hook.site, hook.target),
				Replacement: x86.RelativeInstruction(hook.opcode, hook.site, stub),
			})
		}
	}

	// Keep the context available to the rollback if drawable resize fails.
	const storeSite, primaryContext = 0x63ceaa, 0xb8f3d8
	patches = append(patches, x86.Patch{
		RVA: storeSite, Original: x86.HexBytes("48890527255500"),
		Replacement: append(x86.RelativeInstruction(0xe9, storeSite, address()), 0x90, 0x90),
	})
	code = append(code, x86.HexBytes("4885c074074889")...)
	code = append(code, x86.RelativeInstruction(0x05, address(), primaryContext)...)
	code = append(code, x86.RelativeInstruction(0xe9, address(), storeSite+7)...)

	// MacDisplay::SetupDisplay has stored the new render dimensions here.
	// Reuse the existing SDL window when a mode switch is in progress.
	const windowSite = 0x63f828
	patches = append(patches, x86.Patch{
		RVA: windowSite, Original: x86.HexBytes("7472488b3df7384d00"),
		Replacement: append(x86.RelativeInstruction(0xe9, windowSite, address()), 0x90, 0x90, 0x90, 0x90),
	})
	code = append(code, x86.HexBytes("410fb6d4be05000000488d")...) // windowed flag and ResizeWindow operation
	code = append(code, x86.RelativeInstruction(0x3d, address(), context)...)
	code = append(code, x86.RelativeInstruction(0xe8, address(), body)...)
	code = append(code, x86.HexBytes("83f8fe740789c3")...)                     // -2 uses the stock window creation path.
	code = append(code, x86.RelativeInstruction(0xe9, address(), 0x63f8c6)...) // Return SDL status.
	code = append(code, x86.HexBytes("4584e40f")...)
	code = append(code, x86.RelativeInstruction(0x84, address(), 0x63f89c)...) // Stock fullscreen window.
	code = append(code, x86.HexBytes("488b")...)
	code = append(code, x86.RelativeInstruction(0x3d, address(), 0xb13128)...) // Stock window title.
	code = append(code, x86.RelativeInstruction(0xe9, address(), windowSite+9)...)
	code, patches = appendMachOUIProjection(code, patches)
	return code, patches, context
}

func appendMachOUIProjection(code []byte, patches []x86.Patch) ([]byte, []x86.Patch) {
	const site, matrix, guard = 0x61eb0f, 0xb8f260, 0xb8f2a0
	address := func() int { return macUIOptionalCode + len(code) }
	update := address()
	code = append(code, patchsrc.MacUIProjectionCode...)
	patches = append(patches, x86.Patch{
		RVA: site, Original: x86.HexBytes("8a058b075700"),
		Replacement: append(x86.RelativeInstruction(0xe9, site, address()), 0x90),
	})
	// Update the two size-dependent scales before Aspyr uses the matrix.
	// Its native first-use initialization still sets the other coefficients.
	code = append(code, x86.HexBytes("4c89fe4c89f2488d")...) // rsi=r15, rdx=r14, lea rdi,[rip+matrix]
	code = append(code, x86.RelativeInstruction(0x3d, address(), matrix)...)
	code = append(code, x86.RelativeInstruction(0xe8, address(), update)...)
	code = append(code, 0x8a)
	code = append(code, x86.RelativeInstruction(0x05, address(), guard)...)
	code = append(code, x86.RelativeInstruction(0xe9, address(), site+6)...)
	return code, patches
}
