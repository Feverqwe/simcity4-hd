package pe

import (
	"encoding/binary"
	"math"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
)

func appendCoverageStrokePatches(stub []byte, patches []x86.Patch, base int) ([]byte, []x86.Patch) {
	const imageBase = 0x400000
	two := uint32(imageBase + base + len(stub))
	stub = binary.LittleEndian.AppendUint32(stub, math.Float32bits(2))
	four := uint32(imageBase + base + len(stub))
	stub = binary.LittleEndian.AppendUint32(stub, math.Float32bits(4))
	halfSlope := uint32(imageBase + base + len(stub))
	stub = binary.LittleEndian.AppendUint32(stub, math.Float32bits(64))
	one := uint32(imageBase + base + len(stub))
	stub = binary.LittleEndian.AppendUint64(stub, math.Float64bits(1))

	const (
		readBounds = "8b48082b08"       // bitmap bounds returned in EAX
		stackWidth = "8b4c242c2b4c2424" // right minus left, accounting for saved ECX
	)
	for _, operand := range []struct {
		site      int
		original  string
		readWidth string
		scaled    uint32
	}{
		{0x39e15a, "d8052812a800", readBounds, two},       // row scan radius
		{0x39e18d, "dc25282da900", stackWidth, one},       // first row margin
		{0x39e1e1, "dc05282da900", stackWidth, one},       // last row margin
		{0x39e35c, "d8155c82a800", stackWidth, four},      // stroke extent
		{0x39e369, "d80d107eab00", stackWidth, halfSlope}, // distance-to-opacity slope
	} {
		stock := x86.HexBytes(operand.original)
		body := x86.HexBytes("51" + operand.readWidth + "81f900020000597c08") // original geometry below 512 px
		body = binary.LittleEndian.AppendUint32(append(body, stock[:2]...), operand.scaled)
		body = append(body, 0xeb, byte(len(stock)))
		body = append(body, stock...)
		var patch x86.Patch
		stub, patch = x86.AppendStub(stub, base, operand.site, stock, body)
		patches = append(patches, patch)
	}
	return stub, patches
}
