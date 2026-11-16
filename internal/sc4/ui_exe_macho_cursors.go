package sc4

import (
	"bytes"
	_ "embed"
	"fmt"
	"math"
)

//go:embed patchsrc/ui_cursors_macho.x86_64
var macCursorCode []byte

const (
	macCursorDirectorySite = 0x577c0
	macCursorCodeOffset    = 0xd00
	macCursorPointScale    = 0x2d9
)

func appendMachOCursors(code []byte, patches []instructionPatch, cursorScale int) ([]byte, []instructionPatch, error) {
	if len(code) > macCursorCodeOffset {
		return nil, nil, fmt.Errorf("Mach-O optional UI code overlaps the cursor hook")
	}
	code = append(code, make([]byte, macCursorCodeOffset-len(code))...)
	code = append(code, macCursorCode...)
	// The image and hotspot are in points. The same cursor keeps its
	// logical size with either density of prepared pixel art.
	if cursorScale == 4 {
		offset := macCursorCodeOffset + macCursorPointScale
		if little.Uint64(code[offset:]) != math.Float64bits(0.5) {
			return nil, nil, fmt.Errorf("unexpected Mach-O cursor scale literal")
		}
		little.PutUint64(code[offset:], math.Float64bits(0.25))
	}
	for _, hook := range []struct {
		site, entry int
		opcode      byte
		original    string
	}{
		{macCursorDirectorySite, 0, 0xe9, "80bdcffbffff200f85f1020000"},
		{0x5796f, 0x80, 0xe9, "8a8da6faffff"},
		{0x57e2c, 0x220, 0xe9, "3b85b0fbffff0f45c8"},
		{0x57f58, 0x240, 0xe8, "e8191e7a00"},
		{0x7f9dde, 0x280, 0xe8, "41ffd64889c3"},
		{0x7f9dfe, 0x260, 0xe8, "f20f1045c8f20f104dc0"},
	} {
		original := hexBytes(hook.original)
		redirect := relativeInstruction(hook.opcode, hook.site, macUIOptionalCode+macCursorCodeOffset+hook.entry)
		redirect = append(redirect, bytes.Repeat([]byte{0x90}, len(original)-len(redirect))...)
		patches = append(patches, instructionPatch{hook.site, original, redirect})
	}
	return code, patches, nil
}
