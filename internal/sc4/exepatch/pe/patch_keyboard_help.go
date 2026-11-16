package pe

import (
	"encoding/binary"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/uiids"
)

func appendKeyboardHelpPatch(stub []byte, patches []x86.Patch, base int) ([]byte, []x86.Patch) {
	const (
		initSite    = 0x36afe3 // EAX holds the Play Options root after wiring Credits.
		messageSite = 0x36b562 // The native listener has checked the source and visibility.
	)
	const imageBase = 0x400000
	ids := imageBase + base + len(stub)
	for _, id := range []uint32{uiids.KeyHelpOpen, uiids.KeyHelpPanel, uiids.KeyHelpText, uiids.KeyHelpClose, uiids.HdUITextGroup, uiids.KeyHelpContent} {
		stub = binary.LittleEndian.AppendUint32(stub, id)
	}
	entry := base + len(stub)
	stub = append(stub, patchsrc.KeyboardHelpCode...)

	initBody := binary.LittleEndian.AppendUint32([]byte{0x60, 0x68}, uint32(ids))
	initBody = append(initBody, 0x6a, 0, 0x6a, 0, 0x6a, 0, 0x50) // No event, just the root.
	initBody = append(initBody, x86.RelativeInstruction(0xe8, base+len(stub)+len(initBody), entry)...)
	initOriginal := x86.HexBytes("8bf88b442428")
	initBody = append(append(initBody, x86.HexBytes("83c41461")...), initOriginal...)
	var initPatch x86.Patch
	stub, initPatch = x86.AppendStub(stub, base, initSite, initOriginal, initBody)

	body := x86.HexBytes("8b4c241060") // Load the window message and save caller registers.
	for _, value := range []uint32{uint32(ids), 0x414250, 0x603580} {
		body = binary.LittleEndian.AppendUint32(append(body, 0x68), value)
	}
	body = append(body, 0x51, 0x57) // Message and root window.
	body = append(body, x86.RelativeInstruction(0xe8, base+len(stub)+len(body), entry)...)
	body = append(body, x86.HexBytes("83c41484c0617405")...)
	body = append(body, x86.RelativeInstruction(0xe9, base+len(stub)+len(body), 0x36b592)...)
	original := x86.HexBytes("8b4c24108b1132c0")
	body = append(body, original...)
	var patch x86.Patch
	stub, patch = x86.AppendStub(stub, base, messageSite, original, body)
	return stub, append(patches, initPatch, patch)
}
