package macho

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/uiids"
)

const macBudgetTreeOffset = 0x3800

func appendMachOEditorDecimal(code []byte, patches []x86.Patch) ([]byte, []x86.Patch, error) {
	const (
		offset = 0x3000
		strtod = 0x92b68a
		rzAtof = 0x019b34
		tax    = 0x2e4e77
	)
	if len(code) > offset {
		return nil, nil, fmt.Errorf("Mach-O features overlap the editor hooks")
	}
	base := macUIOptionalCode + offset
	code = append(code, make([]byte, offset-len(code))...)
	code = append(code, patchsrc.MacEditorDecimalCode...)
	patches = append(patches, x86.Patch{RVA: tax,
		Original: x86.RelativeInstruction(0xe8, tax, strtod), Replacement: x86.RelativeInstruction(0xe8, tax, base)})
	for _, site := range []int{0x52ad29, 0x52ad81, 0x52ae2d, 0x52aee9} {
		patches = append(patches, x86.Patch{RVA: site,
			Original: x86.RelativeInstruction(0xe8, site, rzAtof), Replacement: x86.RelativeInstruction(0xe8, site, base+0x40)})
	}
	for _, hook := range []struct {
		site, entry int
		original    string
	}{
		{0x2e4dd2, 0xc0, "4189c7498b7e20"},
		{0x2e4e8a, 0x100, "89c04801c3483b5da00f848a020000"},
	} {
		original := x86.HexBytes(hook.original)
		replacement := x86.RelativeInstruction(0xe9, hook.site, base+hook.entry)
		replacement = append(replacement, bytes.Repeat([]byte{0x90}, len(original)-len(replacement))...)
		patches = append(patches, x86.Patch{RVA: hook.site, Original: original, Replacement: replacement})
	}
	return code, patches, nil
}

func appendMachOBudgetTooltips(code []byte, patches []x86.Patch) ([]byte, []x86.Patch, error) {
	const (
		offset  = 0x3200
		addText = 0x332570
	)
	if len(code) > offset {
		return nil, nil, fmt.Errorf("Mach-O features overlap the budget tooltip hook")
	}
	code = append(code, make([]byte, offset-len(code))...)
	code = append(code, patchsrc.MacBudgetTooltipCode...)
	// Aspyr omits the unused this argument to add_text. Its caller keeps
	// the budget controller in R12 at both heading call sites.
	entry := macUIOptionalCode + len(code)
	code = append(code, x86.HexBytes("4c89e7")...) // mov rdi, r12
	code = append(code, x86.RelativeInstruction(0xe9, entry+3, macUIOptionalCode+offset)...)
	for _, site := range []int{0x3358b4, 0x336591} {
		patches = append(patches, x86.Patch{RVA: site,
			Original:    x86.RelativeInstruction(0xe8, site, addText),
			Replacement: x86.RelativeInstruction(0xe8, site, entry)})
	}
	return code, patches, nil
}

func appendMachOBudgetPopups(code []byte, patches []x86.Patch) ([]byte, []x86.Patch) {
	// Resize the empty popup, its text pane, and both frame controls together.
	for _, site := range []int{0x33e4f2, 0x33e509, 0x33e542, 0x33e6ad, 0x33e6d5} {
		patches = append(patches, x86.Instruction(site, "be2c010000ba64000000", "be58020000bac8000000"))
	}
	// Both popups use the same titles, wrapped text and close button.
	for _, popup := range []struct {
		title, body, close int
	}{
		{0x33e5e4, 0x33e628, 0x33e687},
		{0x34813b, 0x3481a2, 0x348202},
	} {
		patches = append(patches,
			x86.Instruction(popup.title, "c744240807d385ea", "c74424087ecf0353"), // HDBdgtPopupHeader.
			x86.Instruction(popup.title+0x14, "b90a00000041b805000000", "b91400000041b80a000000"),
			x86.Instruction(popup.body, "c744240808d385ea", "c74424085e3d6e95"), // HDBdgtPopupBody.
			x86.Instruction(popup.body+0x14, "b90f00000041b819000000", "b91e00000041b832000000"),
			x86.Instruction(popup.close, "41b80b000000", "41b816000000"),
			x86.Patch{RVA: popup.close + 8, Original: binary.LittleEndian.AppendUint32(nil, uiids.BudgetCloseImage), Replacement: binary.LittleEndian.AppendUint32(nil, uiids.BudgetCloseHD)},
		)
	}
	patches = append(patches,
		x86.Instruction(0x33e682, "b90d010000", "b91a020000"), // Empty popup close button X.
		x86.Instruction(0x3480df, "be1e000000", "be3c000000"),
	)
	for _, site := range []int{0x3480a5, 0x3480b9, 0x34822b, 0x348251} {
		patches = append(patches, x86.Instruction(site, "ba7d000000", "bafa000000"))
	}

	// The popup fills the live ledger width with a 60-pixel inset on either side.
	const widthSite = 0x348092
	patches = append(patches, machOFeatureJump(code, widthSite, "458ba7f4000000418d5c24c4"))
	code = append(code, x86.HexBytes("4c89f7488b07ff90480100008d5888")...)
	code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), widthSite+12)...)

	// Use the live parent height for the lower edge and clamp the upper bound to zero.
	const clampSite = 0x3480c6
	patches = append(patches, machOFeatureJump(code, clampSite, "418b97f000000083c283"))
	code = append(code, x86.HexBytes("4c89f7488b07ff90500100002dfa00000031d285c00f49d0")...)
	code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), clampSite+10)...)

	// Anchor the larger close button to the resized popup width minus 62.
	const closeSite = 0x3481f2
	patches = append(patches, machOFeatureJump(code, closeSite, "4183c4a5895c2408"))
	code = append(code, x86.HexBytes("448b65c44183ec3e895c2408")...)
	code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), closeSite+8)...)

	// Only the enlarged close button takes its image from the projected atlas.
	// The shared image-button constructor receives the instance in EAX here.
	const imageSite = 0x332e3e
	original := "31f6bab006a046"
	patches = append(patches, machOFeatureJump(code, imageSite, original))
	code = append(code, x86.HexBytes(original)...)
	code = binary.LittleEndian.AppendUint32(append(code, 0x3d), uiids.BudgetCloseHD)
	code = binary.LittleEndian.AppendUint32(append(code, 0x41, 0xba), uiids.BudgetRowArtGroup)
	code = append(code, x86.HexBytes("410f44d2")...)
	code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), imageSite+7)...)
	return code, patches
}

func machOFeatureJump(code []byte, site int, original string) x86.Patch {
	before := x86.HexBytes(original)
	redirect := x86.RelativeInstruction(0xe9, site, macUIOptionalCode+len(code))
	redirect = append(redirect, bytes.Repeat([]byte{0x90}, len(before)-len(redirect))...)
	return x86.Patch{RVA: site, Original: before, Replacement: redirect}
}

func appendMachOBudgetTrees(code []byte, patches []x86.Patch) ([]byte, []x86.Patch, error) {
	const offset = macBudgetTreeOffset
	if len(code) > offset {
		return nil, nil, fmt.Errorf("Mach-O features overlap the budget ledger hooks")
	}
	entry := macUIOptionalCode + offset
	code = append(code, make([]byte, offset-len(code))...)
	code = append(code, patchsrc.MacBudgetTreeCode...)
	const (
		noAtlas = iota
		ordinanceCheckboxes
		fundingSliders
		detailsButton
	)
	for _, ledger := range []struct {
		site, controls          int
		window, width, original string
	}{
		{0x338319, ordinanceCheckboxes, "4889df", "b9c2010000", "f685b0feffff01"},
		{0x339c97, noAtlas, "488b7da8", "b9f4010000", "f68598feffff01"},
		{0x336bc2, fundingSliders, "4c89ff", "418b8c24f4000000", "41c644240c01"}, // Department.
		{0x33df6c, fundingSliders, "4c89ff", "418b8c24f4000000", "41c644240c01"}, // Facility.
	} {
		patches = append(patches, machOFeatureJump(code, ledger.site, ledger.original))
		code = append(code, x86.HexBytes(ledger.window)...)
		code = append(code, x86.HexBytes("31f631d2")...) // No bitmap source or destination.
		code = append(code, x86.HexBytes(ledger.width)...)
		code = binary.LittleEndian.AppendUint32(append(code, 0x41, 0xb8), uint32(ledger.controls))
		code = append(code, x86.RelativeInstruction(0xe8, macUIOptionalCode+len(code), entry)...)
		original := x86.HexBytes(ledger.original)
		code = append(code, original...)
		code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), ledger.site+len(original))...)
	}
	// Aspyr replaces the art after doubling the bounds. Center its 32-pixel
	// checkbox in the 36-pixel row by adding one pixel before scaling.
	const addCheckbox = 0x332bc2
	checkbox := macUIOptionalCode + len(code)
	code = append(code, x86.HexBytes("4183c001")...) // add r8d, 1 (Y)
	code = append(code, x86.RelativeInstruction(0xe9, checkbox+4, addCheckbox)...)
	for _, site := range []int{0x337380, 0x33796a} {
		patches = append(patches, x86.Patch{RVA: site,
			Original: x86.RelativeInstruction(0xe8, site, addCheckbox), Replacement: x86.RelativeInstruction(0xe8, site, checkbox)})
	}

	// Use the projected eye in both department and ordinance details rows.
	// Keep the constructor's stack arguments and save parent and button ID.
	const addDetails = 0x332e10
	details := macUIOptionalCode + len(code)
	code = append(code, x86.HexBytes("4883ec28488b44243048890424488b4424384889442408488974241089542418")...)
	code = append(code, x86.RelativeInstruction(0xe8, macUIOptionalCode+len(code), addDetails)...)
	code = append(code, x86.HexBytes("488b7c241031f631d28b4c2418")...)
	code = binary.LittleEndian.AppendUint32(append(code, 0x41, 0xb8), detailsButton)
	code = append(code, x86.RelativeInstruction(0xe8, macUIOptionalCode+len(code), entry)...)
	code = append(code, x86.HexBytes("4883c428c3")...)
	for _, site := range []int{0x335afc, 0x335d11, 0x337408, 0x3379f5} {
		patches = append(patches, x86.Patch{RVA: site,
			Original: x86.RelativeInstruction(0xe8, site, addDetails), Replacement: x86.RelativeInstruction(0xe8, site, details)})
	}
	for _, site := range []int{0x335ae8, 0x335cfd} {
		patches = append(patches, x86.Instruction(site, "b912000000", "b90e000000"))
	}

	// Funding refresh recalculates the marker X in stock coordinates.
	// Its Y comes from the current control area and is already doubled.
	const markerSite = 0x33fc4b
	const markerArgs = "4889df8b55a0"
	patches = append(patches, machOFeatureJump(code, markerSite, markerArgs))
	code = append(code, x86.HexBytes("01f6"+markerArgs)...)
	code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), markerSite+6)...)

	// Budget bitmaps have stock sources and a doubled destination. Branch
	// before the density hook so both UI texture scales retain their own path.
	const paintSite = 0x33093b
	const draw = "488bbba0000000"
	patches = append(patches, machOFeatureJump(code, paintSite, draw))
	code = append(code, x86.HexBytes("f6837b01000040741a4889df")...)   // Test the budget flag and pass RBX.
	code = append(code, x86.HexBytes("488d75c8488d55b8b9a0000000")...) // Source, destination and draw context offset.
	code = append(code, x86.RelativeInstruction(0xe8, macUIOptionalCode+len(code), entry)...)
	code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), 0x33095a)...)
	code = append(code, x86.HexBytes(draw)...)
	code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), paintSite+7)...)

	// Refreshing funding and scrolling Neighbor Deals reapply ledger fonts.
	for _, fonts := range []struct {
		stock, hd string
		sites     []int
	}{
		{"be06d385ea", "bee3272344", []int{
			0x33eff1, 0x33f0ef, 0x33f8d3, 0x33f9bf,
			0x340ace, 0x340bd6, 0x340c8a, 0x3416a6, 0x3417a9, 0x34185d,
			0x3423b1, 0x3424b7, 0x34256b, 0x342c65, 0x342d6b, 0x342e1f,
			0x34351b, 0x343621, 0x3436d5, 0x343e00, 0x343f01, 0x343fb5,
		}},
		{"be05d385ea", "beca29131d", []int{
			0x33f275, 0x33f33c, 0x33fefc, 0x33ffca,
			0x340e9f, 0x340f34, 0x341000, 0x341a73, 0x341b21, 0x341be0,
			0x34278c, 0x342821, 0x3428eb, 0x34303f, 0x3430e9, 0x3431a8,
			0x343909, 0x34399e, 0x343a68, 0x34420a, 0x34429f, 0x344369,
		}},
	} {
		for _, site := range fonts.sites {
			patches = append(patches, x86.Instruction(site, fonts.stock, fonts.hd))
		}
	}
	// A deal refresh can also recreate the action buttons in stock coordinates.
	const addImageButton = 0x332e10
	moveButton := macUIOptionalCode + len(code)
	code = append(code, x86.HexBytes("01c94501c0")...) // Double the X and Y arguments.
	code = append(code, x86.RelativeInstruction(0xe9, macUIOptionalCode+len(code), addImageButton)...)
	for _, site := range []int{0x341edd, 0x3420c6} {
		patches = append(patches, x86.Patch{RVA: site,
			Original: x86.RelativeInstruction(0xe8, site, addImageButton), Replacement: x86.RelativeInstruction(0xe8, site, moveButton)})
	}
	for _, site := range []int{0x341ef6, 0x341f1d, 0x3420df, 0x342106} {
		patches = append(patches, x86.Instruction(site, "beb4000000ba1e000000", "be68010000ba3c000000"))
	}
	return code, patches, nil
}
