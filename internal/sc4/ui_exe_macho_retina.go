package sc4

import (
	"bytes"
	"debug/macho"
	_ "embed"
	"fmt"
)

//go:embed patchsrc/ui_font_atlas_macho.x86_64
var macRetinaAtlas []byte

//go:embed patchsrc/retina_frame_macho.x86_64
var macRetinaFrame []byte

const (
	macRetinaFrameState     = 0xb99510
	macRetinaStateSize      = 40
	macRetinaGlyphCacheSite = 0x657e38
)

func appendMachORetinaText(code []byte, patches []instructionPatch, liveRenderContext int) ([]byte, []instructionPatch, error) {
	address := func() int { return macUIOptionalCode + len(code) }
	appendTargets := func(targets ...int) int {
		context := address()
		for _, target := range targets {
			code = little.AppendUint32(code, uint32(target-context))
		}
		return context
	}
	// Pass a table after the stock register arguments.
	appendStub := func(context, body, argumentCount int) int {
		start := address()
		lea := [...]string{"", "", "488d15", "488d0d", "4c8d05", "4c8d0d"}
		if argumentCount <= 5 {
			operand := hexBytes(lea[argumentCount])
			code = append(code, operand[:2]...)
			code = append(code, relativeInstruction(operand[2], address(), context)...)
			code = append(code, relativeInstruction(0xe9, address(), body)...)
			return start
		}

		// Copy the native stack arguments before appending the table address.
		stackArgs := argumentCount - 6
		stackSpace := 8 * (stackArgs + 1)
		if stackSpace%16 == 0 {
			stackSpace += 8
		}
		code = append(code, 0x48, 0x83, 0xec, byte(stackSpace))
		for i := 0; i < stackArgs; i++ {
			code = append(code, 0x48, 0x8b, 0x44, 0x24, byte(stackSpace+8+8*i))
			code = append(code, 0x48, 0x89, 0x44, 0x24, byte(8*i))
		}
		code = append(code, 0x48, 0x8d)
		code = append(code, relativeInstruction(0x05, address(), context)...)
		code = append(code, 0x48, 0x89, 0x44, 0x24, byte(8*stackArgs))
		code = append(code, relativeInstruction(0xe8, address(), body)...)
		code = append(code, 0x48, 0x83, 0xc4, byte(stackSpace), 0xc3)
		return start
	}
	replaceCall := func(site, original, stub int) {
		patches = append(patches, instructionPatch{
			site, relativeInstruction(0xe8, site, original), relativeInstruction(0xe8, site, stub),
		})
	}

	atlas := address()
	code = append(code, macRetinaAtlas...)
	fontAPI := appendTargets(
		0x658052, 0x92b420, 0x92b0b4, // Native atlas insertion and heap.
		0x8f6b80, 0x8f6770, 0x8f9720, 0x8f9800, 0x8f9870, 0x8f96e0, // FreeType.
		0xa91af0, // cGZFont interface vtable.
	)
	replaceCall(macRetinaGlyphCacheSite, 0x658052, appendStub(fontAPI, atlas, 4))
	const blitGlyph = 0x657f32
	patches = append(patches, instructionPatch{
		blitGlyph, hexBytes("ff9030010000"),
		append(relativeInstruction(0xe8, blitGlyph, atlas+0x34c), 0x90),
	})

	frame := address()
	code = append(code, macRetinaFrame...)
	frameAPI := appendTargets(
		macRetinaFrameState, 0x8aa8b0, 0x92aade, 0x92aad2, // SDL_GL_GetProcAddress, CGL.
		0x92b288, 0x92b294, 0x92b21c, 0x92b1f8, // Query, enable, disable.
		0x92b13e, 0x92b14a, 0x92b17a, 0x92b264, 0x92b270, 0x92b1da,
		0x92b30c, 0x92b24c, 0x92b162, 0x92b306, // Framebuffer storage, transfer.
		0x92b36c, 0x92b312, 0x92b372, 0x92b1c2, 0x92b216, 0x92b2ee, 0x92b2dc,
	)
	var names [16 + 32]byte
	copy(names[:16], "glPixelZoom")
	copy(names[16:], "glDeleteRenderbuffersEXT")
	code = append(code, names[:]...)

	setBacking := appendStub(frameAPI, frame, 3)
	viewport := appendStub(frameAPI, frame+0x407, 4)
	startupSetBacking := 0xb9c00b
	offset := startupSetBacking - macUIOptionalCode
	if !bytes.Equal(code[offset:offset+5], relativeInstruction(0xe8, startupSetBacking, 0x92aade)) {
		return nil, nil, fmt.Errorf("unexpected Mach-O startup backing call at %#x", startupSetBacking)
	}
	copy(code[offset:], relativeInstruction(0xe8, startupSetBacking, setBacking))
	for _, entry := range []struct{ field, original, replacement int }{
		{10, 0x92aade, setBacking},
		{17, 0x92b36c, viewport},
	} {
		offset := liveRenderContext - macUIOptionalCode + 4*entry.field
		if int(int32(little.Uint32(code[offset:])))+liveRenderContext != entry.original {
			return nil, nil, fmt.Errorf("unexpected Mach-O live render target %#x", entry.original)
		}
		little.PutUint32(code[offset:], uint32(entry.replacement-liveRenderContext))
	}
	for _, site := range []int{0x607138, 0x6071c2} {
		replaceCall(site, 0x92b36c, viewport)
	}
	const deleteContext = 0x8d6730
	release := address()
	code = append(code, 0x57) // Keep the SDL context across GetCGLContextFromSDL.
	code = append(code, relativeInstruction(0xe8, address(), 0x4edc8c)...)
	code = append(code, 0x48, 0x8d)
	code = append(code, relativeInstruction(0x0d, address(), macRetinaFrameState)...)
	// The native context owns the GL objects. Clear its identity and scale
	// before SDL can allocate another context at the same address.
	code = append(code, hexBytes("483901750e48c70100000000c74114000000005f")...)
	code = append(code, relativeInstruction(0xe9, address(), deleteContext)...)
	replaceCall(0x63ebe2, deleteContext, release)

	for _, hook := range []struct {
		offset, native, args int
		sites                []int
	}{
		{0x429, 0x92b312, 4, []int{0x6071d3}},
		{0x44b, 0x92b288, 2, []int{0x63bd4e}},
		{0x4b2, 0x92b30c, 4, []int{0x769a5e, 0x769a9d}},
		{0x4d0, 0x92b162, 10, []int{0x763e15, 0x764c0f, 0x769b9f}},
		{0x534, 0x92b372, 2, []int{0x63bd74}},
		{0x54c, 0x92b1c2, 5, []int{0x63c003, 0x63c098, 0x63c36b, 0x63c3c7}},
		{0x56e, 0x92b216, 5, []int{0x63c1ac, 0x63c2da}},
		{0x614, 0x92b306, 7, []int{0x63c260, 0x63c33d, 0x63c9d4}},
	} {
		stub := appendStub(frameAPI, frame+hook.offset, hook.args)
		for _, site := range hook.sites {
			replaceCall(site, hook.native, stub)
		}
	}
	return code, patches, nil
}

func reserveMachORetinaFrameState(data []byte) error {
	const imageBase = 0x100000000
	file, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		return err
	}
	segment := file.Segment("__DATA")
	section := file.Section("__bss")
	if segment == nil || section == nil || section.Flags&0xff != 1 ||
		section.Addr+section.Size != imageBase+macRetinaFrameState ||
		imageBase+macRetinaFrameState+macRetinaStateSize > segment.Addr+segment.Memsz {
		return fmt.Errorf("missing Mach-O zero-filled space for Retina frame state")
	}
	offset := 32
	for _, load := range file.Loads {
		if load == segment {
			for i := uint32(0); i < segment.Nsect; i++ {
				record := offset + 72 + 80*int(i)
				if bytes.Equal(bytes.TrimRight(data[record:record+16], "\x00"), []byte("__bss")) {
					little.PutUint64(data[record+40:], section.Size+macRetinaStateSize)
					return nil
				}
			}
		}
		offset += len(load.Raw())
	}
	return fmt.Errorf("missing Mach-O __bss section")
}
