package pe

import (
	"encoding/binary"
	"math"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
)

func patchRegionMap(stock, stub []byte, stubRVA int, patches []x86.Patch) ([]byte, []x86.Patch) {
	// The shared tile path first aligns the bitmap planes to subpixel
	// coordinates, then builds the drawing spans and hit intervals.
	const factoryVA = 0x913c1a
	expandRVA := stubRVA + len(stub)
	stub = append(stub, patchsrc.ExpandRegionTile...)
	body := x86.HexBytes("9c608d431c68") // Save flags and registers; push factory and &tile.planes.
	body = binary.LittleEndian.AppendUint32(body, factoryVA)
	body = append(body, 0x50)
	body = append(body, x86.RelativeInstruction(0xe8, stubRVA+len(stub)+len(body), expandRVA)...)
	body = append(body, x86.HexBytes("83c408619d")...)

	var patch x86.Patch
	const beforeSpanGeneration = "8b4c241c8bb154010000"
	body = append(body, x86.HexBytes(beforeSpanGeneration)...)
	stub, patch = x86.AppendStub(stub, stubRVA, 0x3ae6ae, x86.HexBytes(beforeSpanGeneration), body)
	patches = append(patches, patch)

	// Keep stock fractional alignment in the preprocessing step. Scale
	// the rounded anchors before subtracting the viewport's pan offset.
	for _, anchor := range []struct {
		rva  int
		code string
	}{
		{0x3b3047, "daa6e8000000"},
		{0x3b3068, "db86ec000000"},
	} {
		original := x86.HexBytes(anchor.code)
		stub, patch = x86.AppendStub(stub, stubRVA, anchor.rva, original, append([]byte{0xd8, 0xc0}, original...))
		patches = append(patches, patch)
	}

	// The compositor also calculates the same anchors inline, once for
	// the city overlays and once for the selected tile's outline. Match
	// tilePosition: double the rounded coordinate before subtracting pan.
	const floorRVA = 0x5eff60
	floorAndDoubleRVA := stubRVA + len(stub)
	stub = append(stub, x86.HexBytes("ff742408ff742408")...) // Copy the two words of the double argument.
	stub = append(stub, x86.RelativeInstruction(0xe8, stubRVA+len(stub), floorRVA)...)
	stub = append(stub, x86.HexBytes("83c408d8c0c3")...)
	for _, site := range []int{0x3b4363, 0x3b437f, 0x3b4991, 0x3b49ad} {
		patches = append(patches, x86.Patch{RVA: site,
			Original:    x86.RelativeInstruction(0xe8, site, floorRVA),
			Replacement: x86.RelativeInstruction(0xe8, site, floorAndDoubleRVA)})
	}

	// Use the doubled basis for the pan limits and projected city
	// overlays. The source tile projection and normalized direction stay
	// on their original constants.
	const imageBase = 0x400000
	doubled := make(map[int]int)
	for _, projection := range []struct {
		rva, va int
		opcode  string
	}{
		{0x3ab827, 0xb0dbac, "d905"}, {0x3ab831, 0xb0dba4, "d905"},
		{0x3ab84c, 0xb0dbb0, "d905"}, {0x3ab85c, 0xb0dba8, "d905"},
		{0x3ab87f, 0xb0dbac, "d80d"}, {0x3ab8d8, 0xb0dbb0, "d825"},
		{0x3b557e, 0xb0dbcc, "d80d"}, {0x3b5588, 0xb0dbd0, "d80d"},
		{0x3b5592, 0xb0dbbc, "d80d"}, {0x3b559c, 0xb0dbc0, "d80d"},
		{0x3b5617, 0xb0dbcc, "d905"}, {0x3b562e, 0xb0dbd0, "d80d"},
		{0x3b5648, 0xb0dbc4, "d905"}, {0x3b5652, 0xb0dbc8, "d80d"},
		{0x3b5662, 0xb0dbbc, "d905"}, {0x3b566e, 0xb0dbc0, "d80d"},
	} {
		original := binary.LittleEndian.AppendUint32(x86.HexBytes(projection.opcode), uint32(projection.va))
		va, exists := doubled[projection.va]
		if !exists {
			va = imageBase + stubRVA + len(stub)
			doubled[projection.va] = va
			f := math.Float32frombits(binary.LittleEndian.Uint32(stock[projection.va-imageBase:]))
			stub = binary.LittleEndian.AppendUint32(stub, math.Float32bits(f*2))
		}
		replacement := binary.LittleEndian.AppendUint32(x86.HexBytes(projection.opcode), uint32(va))
		patches = append(patches, x86.Patch{RVA: projection.rva, Original: original, Replacement: replacement})
	}
	return stub, patches
}
