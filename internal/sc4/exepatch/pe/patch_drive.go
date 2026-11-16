package pe

import (
	"encoding/binary"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/uiids"
)

func appendDriveGaugePatches(stub []byte, patches []x86.Patch, base int) ([]byte, []x86.Patch) {
	// Record the density for the upcoming SetBitmap. The temporary resource
	// at ESP+0x60 stores the group used by the successful loader.
	loaded := x86.HexBytes("8b4c24108b442464")
	body := append([]byte(nil), loaded...)
	body = append(body, x86.HexBytes("80a107010000bf817c246c")...)
	body = binary.LittleEndian.AppendUint32(body, uiids.RuntimeArtworkGroup)
	body = append(body, x86.HexBytes("750780890701000040")...)
	var density x86.Patch
	stub, density = x86.AppendStub(stub, base, 0x164752, loaded, body)
	patches = append(patches, density)

	// Paint receives the window interface four bytes into the gauge object.
	// An HD frame already matches screen density. For a stock fallback use
	// the existing doubled destination, with the same origin and source crop.
	original := x86.HexBytes("897c241c895c2420")
	body = x86.HexBytes("f686030100004075108d143f8954241c8d141b89542420eb08")
	body = append(body, original...)
	var paint x86.Patch
	stub, paint = x86.AppendStub(stub, base, 0x3628c4, original, body)
	return stub, append(patches, paint)
}
