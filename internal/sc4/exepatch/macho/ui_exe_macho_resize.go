package macho

import (
	"bytes"
	"fmt"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
)

func appendMachOWindowResize(code []byte, patches []x86.Patch) ([]byte, []x86.Patch, error) {
	const offset = 0x2000
	if len(code) > offset {
		return nil, nil, fmt.Errorf("Mach-O features overlap the window resize hook")
	}
	code = append(code, make([]byte, offset-len(code))...)
	code = append(code, patchsrc.MacWindowResizeCode...)
	patches = append(patches, x86.Instruction(0x63fc57, "7542", "9090"))
	for _, hook := range []struct {
		site, entry int
		original    string
	}{
		{0x63fc7b, 0, "be300100004c89ff4c89f2e853ae2e00be310100004c89ffe83aae2e00"},
		{0x21d7, 0x80, "f30f1005995eb7000f57c90f2ec175157a13488d3d885eb700488d35855eb700e818d76300"},
		{0x63fdad, 0x2e0, "803d7c334d00007519"}, // MacDisplay::GetClientRect.
	} {
		original := x86.HexBytes(hook.original)
		redirect := x86.RelativeInstruction(0xe9, hook.site, macUIOptionalCode+offset+hook.entry)
		redirect = append(redirect, bytes.Repeat([]byte{0x90}, len(original)-len(redirect))...)
		patches = append(patches, x86.Patch{RVA: hook.site, Original: original, Replacement: redirect})
	}
	for _, site := range []int{0x617f12, 0x617f26} {
		patches = append(patches, x86.Patch{
			RVA:         site,
			Original:    x86.RelativeInstruction(0xe8, site, 0x8d6600),
			Replacement: x86.RelativeInstruction(0xe8, site, macUIOptionalCode+offset+0x100),
		})
	}
	for _, hook := range []struct {
		site, entry int
		original    string
	}{
		{0x23558f, 0x280, "ff506089c3"},   // Region scroll fullscreen check.
		{0x2355b6, 0x2c0, "ff9010010000"}, // Region scroll cursor poll.
	} {
		original := x86.HexBytes(hook.original)
		redirect := x86.RelativeInstruction(0xe8, hook.site, macUIOptionalCode+offset+hook.entry)
		redirect = append(redirect, bytes.Repeat([]byte{0x90}, len(original)-len(redirect))...)
		patches = append(patches, x86.Patch{RVA: hook.site, Original: original, Replacement: redirect})
	}
	const warpSite = 0x6215c9
	patches = append(patches, x86.Patch{
		RVA:         warpSite,
		Original:    x86.RelativeInstruction(0xe8, warpSite, 0x642414),
		Replacement: x86.RelativeInstruction(0xe8, warpSite, macUIOptionalCode+offset+0x1d0),
	})
	return code, patches, nil
}
