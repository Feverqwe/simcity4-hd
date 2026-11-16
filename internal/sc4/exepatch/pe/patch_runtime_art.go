package pe

import (
	"encoding/binary"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/uiids"
)

func appendRuntimeArtPatches(stub []byte, patches []x86.Patch, base int) ([]byte, []x86.Patch) {
	const (
		loadByKey   = 0x202b00
		loadByParts = 0x202b70
		closeBitmap = 0x202be0
	)
	// Both overloads construct a temporary bitmap resource. Try the HD
	// group, then destroy the failed temporary before a stock retry.
	partsEntry := base + len(stub)
	stub = append(stub, x86.HexBytes("538bd9")...)                   // save EBX and retain the resource
	stub = append(stub, x86.HexBytes("ff742418ff742418ff742418")...) // both options and instance
	stub = binary.LittleEndian.AppendUint32(append(stub, 0x68), uiids.RuntimeArtworkGroup)
	stub = append(stub, x86.HexBytes("ff742418")...) // type
	stub = append(stub, x86.RelativeInstruction(0xe8, base+len(stub), loadByParts)...)
	stub = append(stub, x86.HexBytes("837b040074045bc214008bcb")...)
	stub = append(stub, x86.RelativeInstruction(0xe8, base+len(stub), closeBitmap)...)
	stub = append(stub, x86.HexBytes("8bcb5b")...)
	stub = append(stub, x86.RelativeInstruction(0xe9, base+len(stub), loadByParts)...)

	keyEntry := base + len(stub)
	stub = append(stub, x86.HexBytes("53568bd98b74240c")...) // retain the resource and the caller's local key
	stub = binary.LittleEndian.AppendUint32(append(stub, x86.HexBytes("c74604")...), uiids.RuntimeArtworkGroup)
	stub = append(stub, x86.HexBytes("ff742414ff742414ff742414")...)
	stub = append(stub, x86.RelativeInstruction(0xe8, base+len(stub), loadByKey)...)
	stub = binary.LittleEndian.AppendUint32(append(stub, x86.HexBytes("c74604")...), uiids.CatalogFrameGroup)
	stub = append(stub, x86.HexBytes("837b040074055e5bc20c008bcb")...)
	stub = append(stub, x86.RelativeInstruction(0xe8, base+len(stub), closeBitmap)...)
	stub = append(stub, x86.HexBytes("8bcb5e5b")...)
	stub = append(stub, x86.RelativeInstruction(0xe9, base+len(stub), loadByKey)...)

	for _, call := range []struct{ site, stock, hd int }{
		{0x16474d, loadByParts, partsEntry}, // Free Drive gauge.
		{0x3e8b35, loadByParts, partsEntry}, // Viewport alert border.
		{0x029653, loadByKey, keyEntry},     // Callout used in the tutorial.
		{0x3e7d95, loadByKey, keyEntry},     // Hover tooltip.
		{0x3eede0, loadByKey, keyEntry},     // In-city callout.
	} {
		patches = append(patches, x86.Patch{
			RVA: call.site, Original: x86.RelativeInstruction(0xe8, call.site, call.stock),
			Replacement: x86.RelativeInstruction(0xe8, call.site, call.hd),
		})
	}
	return stub, patches
}
