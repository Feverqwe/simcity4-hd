package sc4

import (
	"bytes"
	_ "embed"
)

// Build the embedded stubs with Clang and llvm-objcopy from this directory:
//
//	clang -target i386-linux-gnu -x assembler -c ui_draw_x4.asm -o /tmp/sc4-draw.o
//	clang -target i386-linux-gnu -x assembler -Wa,-defsym,screen_density=1 -c ui_draw_x4.asm -o /tmp/sc4-draw2.o
//	clang -target i386-linux-gnu -x assembler -c patchsrc/ui_source_origin.asm -o /tmp/sc4-origin.o
//	clang -target i386-linux-gnu -march=i386 -Os -mno-sse -mno-sse2 -ffreestanding -fno-pic -fno-stack-protector -c patchsrc/ui_frame_tile.c -o /tmp/sc4-tile.o
//	clang -target i386-linux-gnu -march=i386 -Os -mno-sse -mno-sse2 -ffreestanding -fno-pic -fno-stack-protector -DFRAME_SOURCE_SCALE=1 -c patchsrc/ui_frame_tile.c -o /tmp/sc4-tile2.o
//	llvm-objcopy -O binary -j .text /tmp/sc4-draw.o ui_draw_x4.i386
//	llvm-objcopy -O binary -j .text /tmp/sc4-draw2.o ui_draw_x2.i386
//	llvm-objcopy -O binary -j .text /tmp/sc4-origin.o ui_source_origin.i386
//	llvm-objcopy -O binary -j .text /tmp/sc4-tile.o ui_frame_tile.i386
//	llvm-objcopy -O binary -j .text /tmp/sc4-tile2.o ui_frame_tile_x2.i386
//
// Keep the stubs self-contained and verify that .text has no relocations.
//
//go:embed ui_draw_x4.i386
var densityDraw []byte

//go:embed ui_draw_x2.i386
var screenDensityDraw []byte

//go:embed ui_source_origin.i386
var densitySourceOrigin []byte

//go:embed ui_frame_tile.i386
var densityTile []byte

//go:embed ui_frame_tile_x2.i386
var screenDensityTile []byte

// appendDensityUIPatches keeps the control layout and click bounds in
// logical pixels. The adapters translate the source rectangle at draw time.
func appendDensityUIPatches(stub []byte, patches []instructionPatch, base, textureScale int) ([]byte, []instructionPatch) {
	const (
		directEntry       = 0x38
		slicedEntry       = 0x4b
		destinationEntry  = 0x64
		stretchEntry      = 0x74
		clippedEntry      = 0xc5
		bitmapParserEntry = 0xd9
		bitmapSourceEntry = 0xf9
		bitmapDirectEntry = 0x119
		bitmapSlicedEntry = 0x135
	)
	stub = append(stub, make([]byte, (-(base+len(stub)))&15)...)
	start := base + len(stub)
	draw, tile := densityDraw, densityTile
	if textureScale == 2 {
		draw, tile = screenDensityDraw, screenDensityTile
	}
	code := bytes.Clone(draw)
	for _, jump := range []struct{ end, target int }{
		{directEntry, 0x54fcd2},
		{destinationEntry, 0x4d8800},
		{stretchEntry, 0x4d88ea},
		{bitmapParserEntry, start + len(code)},
		{bitmapSourceEntry, 0x55016b},
		{bitmapDirectEntry, 0x5bc3ab},
	} {
		at := jump.end - 5
		copy(code[at:jump.end], relativeInstruction(0xe9, start+at, jump.target))
	}
	for _, hook := range []struct {
		site, entry int
		op          byte
		original    string
	}{
		{0x550162, bitmapParserEntry, 0xe9, "8b4dfc8b0157ff5010"},
		{0x5bc3a5, bitmapSourceEntry, 0xe9, "8db3d8000000"},
		{0x5bc409, bitmapDirectEntry, 0xe8, "ff9098000000"},
		{0x5bc436, bitmapSlicedEntry, 0xe8, "ff736ce8c2c3f1ff"},
		{0x54fcc7, 0, 0xe9, "8b4d68ff75648b01ff5030"},
		{0x5b065b, directEntry, 0xe8, "8b108bc8ff9298000000"},
		{0x5b061b, slicedEntry, 0xe8, "ff7170e8dd81f2ff"},
		{0x4d88e2, destinationEntry, 0xe9, "8b5c244c8b4c242c"},
		{0x4d8928, stretchEntry, 0xe8, "ff9298000000"},
		{0x4d89a2, stretchEntry, 0xe8, "ff9098000000"},
		{0x4d8b19, stretchEntry, 0xe8, "ff9098000000"},
		{0x4d8b93, stretchEntry, 0xe8, "ff9298000000"},
		{0x4d896a, clippedEntry, 0xe8, "ff929c000000"},
		{0x4d8a17, clippedEntry, 0xe8, "ff909c000000"},
		{0x4d8a6a, clippedEntry, 0xe8, "ff909c000000"},
		{0x4d8ab2, clippedEntry, 0xe8, "ff929c000000"},
		{0x4d8b5b, clippedEntry, 0xe8, "ff909c000000"},
	} {
		original := hexBytes(hook.original)
		patch := relativeInstruction(hook.op, hook.site, start+hook.entry)
		patch = append(patch, bytes.Repeat([]byte{0x90}, len(original)-len(patch))...)
		patches = append(patches, instructionPatch{hook.site, original, patch})
	}
	stub = append(stub, code...)
	stub = append(stub, tile...)
	// The port maps a source coordinate to source*scale+translation.
	// Keep the source origin anchored when scaling a subrectangle.
	stub, origin := appendStub(stub, base, 0x42c5c9,
		hexBytes("894424348b442430d95c2408db442434894c24348bced95c2404db442434d91c24"), densitySourceOrigin)
	return stub, append(patches, origin)
}
