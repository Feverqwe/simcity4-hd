package macho

import (
	"fmt"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
)

func appendMachOHTMLScrollbars(code []byte, patches []x86.Patch) ([]byte, []x86.Patch, error) {
	const offset = 0x4900
	if len(code) > offset {
		return nil, nil, fmt.Errorf("Mach-O features overlap the HTML scrollbar hooks")
	}
	entry := macUIOptionalCode + offset
	code = append(code, make([]byte, offset-len(code))...)
	code = append(code, patchsrc.MacHTMLScrollbarsCode...)
	for operation, hook := range []struct {
		site, call int
		original   string
	}{
		{site: 0x569ae2, original: "554889e54157"}, // SetScrollBarSizes.
		{site: 0x568b57, call: 0x56718e},           // Resize: SetHTMLPaintArea.
		{site: 0x5688be, call: 0x5688e8},           // Reflow: test which bars are needed.
		{site: 0x569179, original: "ffa0e0020000"}, // Removal: invalidate the HTML window.
	} {
		start := macUIOptionalCode + len(code)
		if hook.call == 0 {
			patches = append(patches, machOFeatureJump(code, hook.site, hook.original))
		} else {
			patches = append(patches, x86.Patch{RVA: hook.site,
				Original: x86.RelativeInstruction(0xe8, hook.site, hook.call), Replacement: x86.RelativeInstruction(0xe8, hook.site, start)})
		}
		code = append(code, 0xbe, byte(operation), 0, 0, 0)
		code = append(code, x86.RelativeInstruction(0xe9, start+5, entry)...)
	}

	// Match the right gutter to the wide vertical bar and leave an 8 px gap.
	const gutterSite = 0x566af3
	const gutters = "0f118360020000"
	patches = append(patches, machOFeatureJump(code, gutterSite, gutters))
	code = append(code, x86.HexBytes(gutters+"c7836c02000024000000")...)
	code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), gutterSite+7)...)

	// Use the horizontal interface for the horizontal range, steps and position.
	for _, site := range []int{0x568225, 0x568234, 0x56824e} {
		patches = append(patches, x86.Instruction(site, "488bbb88020000", "488bbb90020000"))
	}
	// During live resize a client may briefly have no usable width or height.
	for _, guard := range []struct {
		site     int
		original string
		minimum  string
	}{
		{0x568175, "442bbb8c010000", "4585ff440f4ef8"}, // Vertical extent in R15D.
		{0x5681f8, "442ba388010000", "4585e4440f4ee0"}, // Horizontal extent in R12D.
	} {
		patches = append(patches, machOFeatureJump(code, guard.site, guard.original))
		code = append(code, x86.HexBytes(guard.original+"b801000000"+guard.minimum)...)
		code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), guard.site+7)...)
	}
	return code, patches, nil
}
