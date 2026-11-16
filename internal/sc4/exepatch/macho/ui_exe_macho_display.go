package macho

import (
	"bytes"
	"encoding/binary"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
)

const (
	DisplayPreference    = "sc4.uhd.display"
	macDisplayPending    = "sc4.uhd.pending-display"
	macDisplayFailed     = "sc4.uhd.display-failed"
	macDisplaySpacesHint = "SDL_VIDEO_MAC_FULLSCREEN_SPACES"
)

func appendMachODisplayOptions(code []byte, patches []x86.Patch, hasUI bool) ([]byte, []x86.Patch) {
	const (
		entryOffset = 0x1000
		entry       = macUIOptionalCode + entryOffset
		body        = entry + 0x20
		setButtons  = 0x3a4700
	)
	context := body + len(patchsrc.MacDisplayOptionsCode)
	address := func() int { return macUIOptionalCode + len(code) }
	withContext := func() {
		code = append(code, 0x48, 0x8d)
		code = append(code, x86.RelativeInstruction(0x15, address(), context)...)
	}
	operation := func(op byte) {
		code = append(code, 0xbe, op, 0, 0, 0)
		withContext()
	}
	redirect := func(site, target int, original []byte, opcode byte) {
		replacement := x86.RelativeInstruction(opcode, site, target)
		replacement = append(replacement, bytes.Repeat([]byte{0x90}, len(original)-len(replacement))...)
		patches = append(patches, x86.Patch{RVA: site, Original: original, Replacement: replacement})
	}

	code = append(code, make([]byte, entryOffset-len(code))...)
	code = append(code, x86.HexBytes("31ff")...) // Startup has no dialog.
	operation(4)
	code = append(code, x86.RelativeInstruction(0xe9, address(), body)...)
	code = append(code, make([]byte, body-address())...)
	code = append(code, patchsrc.MacDisplayOptionsCode...)
	// Context offsets stay relative to the context to support ASLR.
	for _, target := range []int{
		0x641c53, 0x641cbe, 0x642135, // MacPreferences get, put, and synchronize
		0xb6ed40, 0x8e0340, 0x92b0f6, // Pre-init windowed flag, SDL_SetHintWithPriority and getenv
		0x63fb30, 0x63fb4e, 0x63f9a1, 0x63f948, // MacDisplay
		0x121650, 0x121738, // cSLString
		0x4879e0, 0x4878d0, 0x5f070, 0x17124, // cRZString constructors, non-deleting destructor, ToChar
		0x63f93c, 0x92aae4, 0x92aa2a, // Display ID and CoreGraphics mode enumeration
		0x92a8e6, 0x92a8ec, 0x92a95e, // CoreFoundation array operations
		0x92aa66, 0x92aa60, // Display mode pixel dimensions
		0x63f907, 0x8a8af0, 0x8a8c30, // SDL window and window data
		0x92aa30, // CGDisplayCopyDisplayMode, also used by borderless.
	} {
		code = binary.LittleEndian.AppendUint32(code, uint32(target-context))
	}
	for _, field := range []struct {
		text string
		size int
	}{
		{DisplayPreference, 16},
		{macDisplaySpacesHint, 32},
		{"1", 2},
		{"0", 2},
		{"SC4_UHD_RESOLUTION", 24},
		{macDisplayPending, 24},
		{macDisplayFailed, 24},
	} {
		code = append(code, field.text...)
		code = append(code, make([]byte, field.size-len(field.text))...)
	}
	// Cocoa reads the Spaces hint during SDL_Init, before the render mode
	// is selected. Forward the original flags and return value.
	const sdlInitSite, sdlInit = 0x2be8, 0x8d6530
	redirect(sdlInitSite, address(), x86.RelativeInstruction(0xe8, sdlInitSite, sdlInit), 0xe8)
	code = append(code, 0x57)
	operation(5)
	code = append(code, x86.RelativeInstruction(0xe8, address(), body)...)
	code = append(code, 0x5f)
	code = append(code, x86.RelativeInstruction(0xe9, address(), sdlInit)...)
	if !hasUI {
		return code, patches
	}

	for _, hook := range []struct {
		site int
		op   byte
	}{{0x3a3e29, 0}, {0x3a6908, 1}} {
		redirect(hook.site, address(), x86.RelativeInstruction(0xe8, hook.site, setButtons), 0xe8)
		code = append(code, 0x57) // Save the dialog and align the stack.
		code = append(code, x86.RelativeInstruction(0xe8, address(), setButtons)...)
		code = append(code, 0x5f)
		operation(hook.op)
		code = append(code, x86.RelativeInstruction(0xe9, address(), body)...)
	}

	for _, hook := range []struct {
		site     int
		original string
	}{{0x3a64dd, "498b064c89f7ff5038"}, {0x3a7956, "488d05fb987700"}} {
		original := x86.HexBytes(hook.original)
		redirect(hook.site, address(), original, 0xe9)
		code = append(code, x86.HexBytes("4c89f7")...) // r14 is the dialog.
		operation(2)
		code = append(code, x86.RelativeInstruction(0xe8, address(), body)...)
		code = append(code, x86.HexBytes("83f8027505")...)
		code = append(code, x86.RelativeInstruction(0xe9, address(), 0x3a7947)...)
		if hook.site == 0x3a7956 {
			code = append(code, 0x48, 0x8d)
			code = append(code, x86.RelativeInstruction(0x05, address(), 0xb21258)...) // SL::spSC4
		} else {
			code = append(code, original...)
		}
		code = append(code, x86.RelativeInstruction(0xe9, address(), hook.site+len(original))...)
	}

	const messageSite = 0x3a6266
	original := x86.HexBytes("41b60149817c2408f6597228")
	redirect(messageSite, address(), original, 0xe9)
	code = append(code, x86.HexBytes("49817c2408551271e8")...)
	otherMessage := len(code)
	code = append(code, 0x75, 0)
	code = append(code, x86.HexBytes("41817c241004444ae7")...)
	otherControl := len(code)
	code = append(code, 0x75, 0)
	code = append(code, x86.HexBytes("4c89ff")...)
	operation(3)
	code = append(code, x86.RelativeInstruction(0xe8, address(), body)...)
	code[otherMessage+1] = byte(len(code) - otherMessage - 2)
	code[otherControl+1] = byte(len(code) - otherControl - 2)
	code = append(code, original...)
	code = append(code, x86.RelativeInstruction(0xe9, address(), messageSite+len(original))...)
	return code, patches
}
