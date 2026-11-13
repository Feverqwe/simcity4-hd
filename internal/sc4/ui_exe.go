package sc4

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const stockEXEHash = "f9b059d29940d1a28c89e6d6a7ab147e9762cd03134cdbaf02ba0e0142e1a176"

// Assemble with llvm-mc -triple=i386 -filetype=obj, then extract .text
// with llvm-objcopy -O binary -j .text.
//
//go:embed patchsrc/ui_budget_tree.i386
var budgetTreeScale []byte

//go:embed patchsrc/ui_budget_slider.i386
var budgetSliderArt []byte

//go:embed patchsrc/ui_import_node_font.i386
var importNodeFont []byte

//go:embed patchsrc/ui_loading_preview.i386
var expandLoadingPreview []byte

//go:embed patchsrc/ui_playlist_wheel.i386
var scrollPlaylist []byte

//go:embed patchsrc/ui_disasters.i386
var prepareDisasterBackground []byte

type uiExecutableOptions struct {
	hdCursors      bool
	loadingPreview bool
	regionMap      bool
}

type instructionPatch struct {
	rva                   int
	original, replacement []byte
}

func instruction(rva int, original, replacement string) instructionPatch {
	return instructionPatch{rva, hexBytes(original), hexBytes(replacement)}
}

func hexBytes(encoded string) []byte {
	data, err := hex.DecodeString(encoded)
	if err != nil {
		panic(err)
	}
	return data
}

func relativeInstruction(opcode byte, site, target int) []byte {
	return little.AppendUint32([]byte{opcode}, uint32(target-site-5))
}

// appendStub redirects a fixed-width instruction block and returns to the
// instruction immediately following it.
func appendStub(stub []byte, base, site int, original, body []byte) ([]byte, instructionPatch) {
	address := base + len(stub)
	redirect := relativeInstruction(0xe9, site, address)
	redirect = append(redirect, bytes.Repeat([]byte{0x90}, len(original)-len(redirect))...)
	stub = append(stub, body...)
	stub = append(stub, relativeInstruction(0xe9, base+len(stub), site+len(original))...)
	return stub, instructionPatch{site, original, redirect}
}

func appendDisasterBackgroundPatch(stub []byte, patches []instructionPatch, base int) ([]byte, []instructionPatch) {
	const (
		site          = 0x3e7459
		loadBitmapVA  = 0x602b00
		closeBitmapVA = 0x602be0
		bufferFactory = 0x90ddf1
	)
	entry := base + len(stub)
	stub = append(stub, prepareDisasterBackground...)
	original := hexBytes("8b5424406850010000")
	// Save the caller's state and pass the bitmap resource that it just loaded.
	body := hexBytes("9c608d442460")
	for _, arg := range []uint32{bufferFactory, closeBitmapVA, loadBitmapVA, disasterBackgroundX4} {
		body = little.AppendUint32(append(body, 0x68), arg)
	}
	body = append(body, 0x50)
	body = append(body, relativeInstruction(0xe8, base+len(stub)+len(body), entry)...)
	body = append(body, hexBytes("83c414619d")...)
	body = append(body, original...)
	var patch instructionPatch
	stub, patch = appendStub(stub, base, site, original, body)
	return stub, append(patches, patch)
}

func patchUIExecutable(data []byte, textureScale int, options uiExecutableOptions) ([]byte, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	if hash != stockEXEHash {
		return nil, fmt.Errorf("expected the original Steam Deluxe 1.1.641.0 executable (SHA-256 %s), got %s", stockEXEHash, hash)
	}

	// The fingerprint pins the PE layout as well as the x86 instructions.
	pe := int(little.Uint32(data[0x3c:]))
	text := pe + 24 + int(little.Uint16(data[pe+20:]))
	rdata := text + 40
	if string(bytes.TrimRight(data[text:text+8], "\x00")) != ".text" ||
		string(bytes.TrimRight(data[rdata:rdata+8], "\x00")) != ".rdata" {
		return nil, errors.New("expected .text and .rdata sections")
	}
	virtualSize := int(little.Uint32(data[text+8:]))
	rva := int(little.Uint32(data[text+12:]))
	rawStart := int(little.Uint32(data[text+20:]))
	stubRVA := int(little.Uint32(data[pe+80:])) // SizeOfImage

	// Catalog and Disasters allocate their button frames in code.
	stub, catalog := appendStub(nil, stubRVA, 0x3eb15d,
		hexBytes("6a1d6a1b6a046a356a506a196a35"), hexBytes("6a3a6a366a086a6a68a00000006a326a6a"))
	stub, disaster := appendStub(stub, stubRVA, 0x3e749d,
		hexBytes("6a226a286a066a3e6a5e6a196a35"), hexBytes("6a446a506a0c6a7c68bc0000006a326a6a"))

	snapshotRVA := stubRVA + len(stub)
	stub = append(stub, hexBytes("8b4424048b1001d239da771b518b480401c93b4c2420770e750439da7408891089480459eb0459c20400")...)
	const snapshotAppend = 0x467a0
	stub = append(stub, relativeInstruction(0xe9, stubRVA+len(stub), snapshotAppend)...)
	patches := []instructionPatch{
		catalog, disaster,
		instruction(0x3eb17b, "83c0f6", "83c0ec"),
		instruction(0x3eb183, "6a0a", "6a14"),
		instruction(0x3e74bb, "83c0f6", "83c0ec"),
		instruction(0x3e74c3, "6a0a", "6a14"),
		instruction(0x3eaeef, "6a056a2c6a2c8bceff5030", "6a0a6a586a588bceff5030"),
		instruction(0x3e72a4, "6a056a2c6a2c8bceff5230", "6a0a6a586a588bceff5230"),
		instruction(0x3f4d19, "68a264258a", "68c3404ae7"),
		// Runtime HUD controls use image IDs and sizes outside the layouts.
		instruction(0x3ee037, "c7442440b006a046c744244488452453", "c7442440bd404ae7c744244488452453"),
		instruction(0x3ee15a, "83e81c506a0c", "83e838506a18"),
		// Polls compute frame width from the atlas. The positive arrows
		// still start at frame three in the six-frame replacement.
		instruction(0x3ed4ce, "c744243880550114", "c7442438ac60d22b"),
		instruction(0x3ed4f3, "c744243884550114", "c7442438ab60d22b"),
		instruction(0x37019c, "6888f8064c", "68c0404ae7"),
		// Both Ordinances tables use the ledger header style. The expense
		// table otherwise inherits the enlarged advisor headline.
		instruction(0x37ce15, "689e4a0faa", "6804d385ea"),
		instruction(0x37ce5b, "689e4a0faa", "6804d385ea"),
		// Tooltip titles and descriptions use doubled fonts. Scale the
		// wrap limit and leave the available width to the stock layout.
		instruction(0x39880a, "68fa000000", "68f4010000"),
		instruction(0x3988a9, "68fa000000", "68f4010000"),
		// Text and viewport-constrained capture presets scale together.
		instruction(0x3b7af5, "6a0e", "6a1c"),
		instruction(0x3b6bef, "83ea04", "83ea08"),
		instruction(0x3b6bf2, "83ef04", "83ef08"),
		instruction(0x3b6bf7, "83c004", "83c008"),
		instruction(0x3b6bfa, "83c104", "83c108"),
	}
	for _, site := range []int{0x3b7ba9, 0x3b7bc5, 0x3b7bdd, 0x3b7bfd, 0x3b7c21,
		0x3b7c42, 0x3b7c62, 0x3b7c86, 0x3b7caa, 0x3b7cce} {
		patches = append(patches, instructionPatch{site,
			relativeInstruction(0xe8, site, snapshotAppend), relativeInstruction(0xe8, site, snapshotRVA)})
	}
	if textureScale == 4 {
		stub, patches = appendDisasterBackgroundPatch(stub, patches, stubRVA)

		// Both My Sims preview loaders assign full-density bitmaps at runtime.
		// Mark them for the density painter and for physical source clipping.
		patches = append(patches, instruction(0x36eeab, "6888f8064c", "68c0404ae7"))
		for _, site := range []int{0x36ef1b, 0x3701ba} {
			var preview instructionPatch
			stub, preview = appendStub(stub, stubRVA, site,
				hexBytes("8b1150ff5210"),
				hexBytes("804923a08b1150ff5210")) // Mark flags, then SetBitmap.
			patches = append(patches, preview)
		}
		var previewCrop instructionPatch
		stub, previewCrop = appendStub(stub, stubRVA, 0x5bc459,
			hexBytes("8d86e8000000"),
			// Runtime preview source limits are in texture pixels. The
			// stock continuation clips those limits to the bitmap size.
			hexBytes("f686fb00000020740401d201c98d86e8000000"))
		patches = append(patches, previewCrop)
	}

	// The city loads its live rating sprite during initialization and
	// update. The region launcher loads its separate tiled sprite.
	for _, site := range []int{0x3e8522, 0x3ed229, 0x3b5184} {
		patches = append(patches, instructionPatch{site, little.AppendUint32(nil, catalogFrameGroup),
			little.AppendUint32(nil, mayorRatingGroup)})
	}

	// Both Audio Options playlist sources use the native checkbox painter.
	for _, site := range []int{0xf4b84, 0xf4e43} {
		patches = append(patches, instructionPatch{site,
			little.AppendUint32(nil, catalogFrameGroup), little.AppendUint32(nil, budgetRowArtGroup)})
	}

	// The import dialog loads its folder and city icons directly.
	for _, site := range []int{0x379135, 0x379182, 0x3791e5} {
		patches = append(patches, instructionPatch{site, little.AppendUint32(nil, catalogFrameGroup),
			little.AppendUint32(nil, importIconGroup)})
	}

	// The tree's root font does not apply to dynamically created rows.
	importNodeFontRVA := stubRVA + len(stub)
	stub = append(stub, importNodeFont...)
	for _, node := range []struct {
		site int
		next string
	}{
		{0x378e3f, "8b168bd88b4748"}, // Folder node.
		{0x378fc3, "8b168be88b4758"}, // City node.
	} {
		body := relativeInstruction(0xe8, stubRVA+len(stub), importNodeFontRVA)
		original := hexBytes(node.next)
		var nodeFont instructionPatch
		stub, nodeFont = appendStub(stub, stubRVA, node.site, original, append(body, original...))
		patches = append(patches, nodeFont)
	}

	// Data Views positions the legend again when switching overlays.
	// Keep each layout's swatch column and align captions beside it.
	var mapLegendSwatch instructionPatch
	stub, mapLegendSwatch = appendStub(stub, stubRVA, 0x3a081a,
		hexBytes("ff9210010000"),
		// GetArea, then copy the current left and right into the new rectangle.
		hexBytes("ff92c00000008b10899424980000008b5008899424a00000008bce8b16ff9210010000"))
	patches = append(patches, mapLegendSwatch,
		instruction(0x3a07d4, "8d573d", "8d577a"),
		instruction(0x3a080b, "8d44383d", "8d44387a"),
		instruction(0x3a08f4, "8d4f18", "8d4f30"),
		instruction(0x3a08fd, "c744243c16010000", "c744243c2c020000"),
		instruction(0x3a090b, "0516010000", "052c020000"),
		instruction(0x3a0922, "8d4c3818", "8d4c3830"))

	// The graph window builds its plot rectangle in screen pixels.
	// Leave room for the enlarged ticks, title, and line-chart legend.
	var graphLegend instructionPatch
	stub, graphLegend = appendStub(stub, stubRVA, 0x36dd4b,
		hexBytes("83e81483ea6e"), hexBytes("83e82081ead4000000"))
	patches = append(patches, graphLegend,
		instruction(0x36d834, "83e814", "83e820"),
		instruction(0x36d848, "c78424980000002d000000", "c784249800000046000000"),
		instruction(0x36d853, "c784249c00000014000000", "c784249c00000020000000"),
		instruction(0x36dd5f, "c78424a80000002d000000", "c78424a800000046000000"),
		instruction(0x36dd6a, "c78424ac00000014000000", "c78424ac00000020000000"))

	// Both legend paths need the same gutter. Line and bar charts use
	// direct-drawn swatches, while other charts create bitmap controls.
	var graphLegendStart, graphSwatches, graphLabels instructionPatch
	stub, graphLegendStart = appendStub(stub, stubRVA, 0x36e0f5,
		hexBytes("83eb6a83f802"), hexBytes("81ebca00000083f802"))
	stub, graphSwatches = appendStub(stub, stubRVA, 0x36e159,
		hexBytes("8d4aa4518b4c24205183c294"),
		hexBytes("8d8a44ffffff518b4c24205181c234ffffff"))
	stub, graphLabels = appendStub(stub, stubRVA, 0x36e1f1,
		hexBytes("8d8c24f800000083eb5a"), hexBytes("8d8c24f800000081ebba000000"))
	patches = append(patches, graphLegendStart, graphSwatches, graphLabels)

	// Draw a second scanline for data series, clamped to the top of
	// the plot. The axes, grid, and legend have separate draw paths.
	var graphSeries instructionPatch
	stub, graphSeries = appendStub(stub, stubRVA, 0x5b60f4, hexBytes("ff9080000000"),
		hexBytes("ff90800000008b86e40000003945cc7e03ff4dcc3945d47e03ff4dd4"+
			"ff75fc8d55c852ff75088d8e2c0200008b01ff9080000000"))
	patches = append(patches, graphSeries)

	stub, patches = appendBudgetUIPatches(stub, patches, stubRVA)
	if textureScale == 4 {
		var gauges instructionPatch
		// Free Drive gauges select a stock frame at runtime. Stretch the
		// destination to the doubled HUD size while keeping the source cell.
		// With ×2 textures the native draw already uses screen-sized cells.
		stub, gauges = appendStub(stub, stubRVA, 0x3628c4,
			hexBytes("897c241c895c2420"), hexBytes("8d143f8954241c8d141b89542420"))
		patches = append(patches, gauges)
	}

	// The HTML control creates its vertical scrollbar at runtime. Resize
	// the child rectangle and reserve matching space in the text viewport.
	var bottom, left instructionPatch
	stub, bottom = appendStub(stub, stubRVA, 0x4fa8b9,
		hexBytes("508b068bce"), hexBytes("83f80873056a0458eb0383e804508b068bce"))
	// SetArea takes edges, and the stock left edge depends on the child
	// width from the previous layout. Use a fixed width across resizes.
	stub, left = appendStub(stub, stubRVA, 0x4fa8c5,
		hexBytes("8b036a008bcbff90a400000033db432bd88b068bce"),
		hexBytes("6a04bbe4ffffff8b068bce"))
	patches = append(patches, bottom, left,
		instruction(0x4fbcbf, "c7868c01000016000000", "c7868c01000024000000"))
	// The horizontal update must not overwrite the vertical range.
	for _, site := range []int{0x4fa849, 0x4fa857, 0x4fa872} {
		patches = append(patches, instruction(site, "8b8ea4010000", "8b8ea8010000"))
	}
	stub, patches = appendDensityUIPatches(stub, patches, stubRVA, textureScale)
	if options.hdCursors {
		// The hardware path calls LoadCursorFromFileA, which requests the
		// system cursor size. LoadImageA with only LR_LOADFROMFILE instead
		// preserves the file dimensions and hotspot. Keep the original API
		// as a fallback for cursor formats LoadImageA cannot load.
		const loadHardware = 0x492368
		cursorLoader := stubRVA + len(stub)
		stub = append(stub, hexBytes(
			"8b4424046a106a006a006a02506a00"+ // LoadImageA(NULL, path, IMAGE_CURSOR, 0, 0, LR_LOADFROMFILE).
				"ff158405a80085c0750a"+ // On success return the new cursor handle.
				"ff742404ff158805a800"+ // Fall back to LoadCursorFromFileA(path).
				"c20400")...)
		patches = append(patches, instructionPatch{
			loadHardware, hexBytes("ff158805a800"),
			append(relativeInstruction(0xe8, loadHardware, cursorLoader), 0x90),
		},
			// The software path already calls LoadImageA. Clear LR_DEFAULTSIZE.
			instruction(0x4923a6, "6a50", "6a10"))
		// GetDIBits supplies straight BGRA for HD color cursors. The
		// original conversion forces that alpha to 255 before applying
		// the AND mask. Keep supplied alpha and retain the opaque fill
		// for monochrome and legacy color cursors with no alpha channel.
		var softwareAlpha instructionPatch
		stub, softwareAlpha = appendStub(stub, stubRVA, 0x49253d,
			hexBytes("3bf374118b45348d44b0fc804803ff83e8044e75f6"),
			hexBytes("85f6742c837d5400741189f18b453480780300751983c0044975f4"+
				"8b45348d44b0fc804803ff83e8044e75f6eb0231f6"))
		patches = append(patches, softwareAlpha)
	}
	if options.loadingPreview {
		const site = 0x377149
		const loadThumbnail = 0x1dda40
		const bufferFactory = 0x90ddf1
		expandRVA := stubRVA + len(stub)
		stub = append(stub, expandLoadingPreview...)
		wrapperRVA := stubRVA + len(stub)
		// Replay the cdecl thumbnail load. Only the loading-screen caller
		// switches to the larger buffers. The normal paint path centers
		// and blends them using the actual buffer dimensions.
		stub = append(stub, bytes.Repeat(hexBytes("ff742414"), 5)...)
		stub = append(stub, relativeInstruction(0xe8, stubRVA+len(stub), loadThumbnail)...)
		stub = append(stub, hexBytes("83c41484c0")...)
		loadFailed := len(stub)
		stub = append(stub, 0x74, 0)    // Return immediately on load failure.
		stub = append(stub, 0x50, 0x68) // Save the stock return value and pass the factory.
		stub = little.AppendUint32(stub, bufferFactory)
		stub = append(stub, hexBytes("ff742414ff742414")...) // Coverage and color output pointers.
		stub = append(stub, relativeInstruction(0xe8, stubRVA+len(stub), expandRVA)...)
		stub = append(stub, hexBytes("83c40c58")...)
		stub[loadFailed+1] = byte(len(stub) - loadFailed - 2)
		stub = append(stub, 0xc3)
		patches = append(patches, instructionPatch{site,
			relativeInstruction(0xe8, site, loadThumbnail), relativeInstruction(0xe8, site, wrapperRVA)})
	}
	// The native wheel handler already turns wheel delta into a row count.
	// Redirect only its relative-scroll call; the helper recognizes the
	// Audio Options grid and delegates every other grid to the stock call.
	playlistRVA := stubRVA + len(stub)
	stub = append(stub, scrollPlaylist...)
	wheelBody := hexBytes("83c1fc50")
	wheelBody = append(wheelBody, relativeInstruction(0xe8, stubRVA+len(stub)+len(wheelBody), playlistRVA)...)
	var wheel instructionPatch
	stub, wheel = appendStub(stub, stubRVA, 0x5a5f89, hexBytes("83c1fc8b3150ff567c"), wheelBody)
	patches = append(patches, wheel)
	if options.regionMap {
		stub, patches = patchRegionMap(data, stub, stubRVA, patches)
	}
	for _, patch := range patches {
		offset := rawStart + patch.rva - rva
		end := offset + len(patch.original)
		if offset < rawStart || end > rawStart+virtualSize || len(patch.original) != len(patch.replacement) ||
			!bytes.Equal(data[offset:end], patch.original) {
			return nil, fmt.Errorf("unexpected UI instructions at RVA %#x", patch.rva)
		}
		copy(data[offset:end], patch.replacement)
	}
	for _, template := range []struct {
		offset int
		text   string
	}{
		{0x683820, `<FONT COLOR="#8e9499" FACE="Arta" SIZE=3><I>`},
		{0x683850, `<FONT COLOR="#3f4967" FACE="Arta" SIZE=3><I>`},
		{0x683880, ` FACE="StocletITC TT" SIZE=3><B>`},
	} {
		updated := strings.Replace(template.text, "SIZE=3", "SIZE=5", 1)
		if err := patchUIString(data, rdata, template.offset, template.text, updated); err != nil {
			return nil, err
		}
	}
	for i := 0; i < newsMarkerCount; i++ {
		at := 0x6837f4 - i*0x2c
		original := fmt.Sprintf(`<IMG SRC="sc4://HTML/%08x/%08X">`, catalogFrameGroup, newsMarkerFirst+i)
		marker := data[at : at+len(original)]
		if !bytes.Equal(marker, []byte(original)) {
			return nil, fmt.Errorf("unexpected News marker HTML template at offset %#x", at)
		}
		copy(marker, fmt.Sprintf(`<IMG SRC="sc4://HTML/%08x/%08X">`, newsMarkerGroup, newsMarkerFirst+i))
	}

	data, err := appendUISection(data, pe, text, stubRVA, stub)
	if err != nil {
		return nil, err
	}
	little.PutUint16(data[pe+22:], little.Uint16(data[pe+22:])|0x20)
	little.PutUint32(data[pe+88:], peChecksum(data, pe+88))
	return data, nil
}

func patchUIString(data []byte, section, offset int, original, updated string) error {
	start := int(little.Uint32(data[section+20:]))
	end := offset + len(original)
	slotEnd := offset + max(len(original), len(updated)) + 1
	if offset < start || slotEnd > start+int(little.Uint32(data[section+16:])) ||
		string(data[offset:end]) != original || len(bytes.Trim(data[end:slotEnd], "\x00")) != 0 {
		return fmt.Errorf("unexpected UI string at offset %#x", offset)
	}
	slot := data[offset:slotEnd]
	clear(slot)
	copy(slot, updated)
	return nil
}

func appendBudgetUIPatches(stub []byte, patches []instructionPatch, stubRVA int) ([]byte, []instructionPatch) {
	// This constructor is shared by the four dynamically built budget
	// slider paths. Give the stock slider the projected four-state atlas.
	var budgetSlider instructionPatch
	stub, budgetSlider = appendStub(stub, stubRVA, 0x379508, hexBytes("8bf08b168bce"), budgetSliderArt)
	patches = append(patches, budgetSlider)

	// Both Ordinances ledgers share a dedicated checkbox constructor.
	patches = append(patches, instructionPatch{0x37b7c1,
		little.AppendUint32(nil, catalogFrameGroup), little.AppendUint32(nil, budgetRowArtGroup)})

	// The checkbox frame grew around the old top-left anchor.
	// Center it in the ledger row before the dialog doubles its positions.
	var checkboxPosition instructionPatch
	stub, checkboxPosition = appendStub(stub, stubRVA, 0x37b90c,
		hexBytes("8b1750518bcf"), hexBytes("8b1783e80550518bcf"))
	patches = append(patches, checkboxPosition)

	// Open Details and the enlarged empty-dialog close button use
	// projected atlases. Preserve the stock group for other callers.
	var budgetDetailsArt instructionPatch
	stub, budgetDetailsArt = appendStub(stub, stubRVA, 0x37b970,
		hexBytes("5068b006a046"),
		hexBytes("503db755011474073d60624114750768c8404ae7eb0568b006a046"))
	patches = append(patches, budgetDetailsArt,
		// The active-state frame is wider than the glyph. Leave room
		// for the label that starts at X=48 in department rows.
		instruction(0x388abf, "6a12", "6a0e"),
		instruction(0x388f94, "6a12", "6a0e"))

	// Empty ledger sections share a dialog built in code. Keep the
	// root, text pane, and frame at the same size for layout and input.
	for _, site := range []int{0x37c19e, 0x37c1b0, 0x37c1d9, 0x37c2e0, 0x37c301} {
		var emptyLedgerSize instructionPatch
		stub, emptyLedgerSize = appendStub(stub, stubRVA, site,
			hexBytes("6a64682c010000"), hexBytes("68c80000006858020000"))
		patches = append(patches, emptyLedgerSize)
	}
	patches = append(patches,
		instruction(0x37c254, "6807d385ea", "687ecf0353"), // HDBdgtPopupHeader
		instruction(0x37c27e, "6808d385ea", "685e3d6e95"), // HDBdgtPopupBody
		instruction(0x37c260, "6a056a0a", "6a0a6a14"),
		instruction(0x37c286, "6a196a0f", "6a326a1e"),
		instruction(0x37c2b5, "68b4550114", "6860624114"),
		instruction(0x37c2ba, "6a0b680d010000", "6a12681a020000"))

	// The funding refresh recomputes the marker's X in stock units.
	// Its Y already comes from the resized control.
	var budgetMarker instructionPatch
	stub, budgetMarker = appendStub(stub, stubRVA, 0x38a729,
		hexBytes("508bcfff93e0000000"), hexBytes("8d0400508bcfff93e0000000"))
	patches = append(patches, budgetMarker)

	// The department income table creates its labels and amounts
	// with the stock ledger fonts, including on a row rebuild.
	for _, site := range []int{0x388517, 0x3885a0} {
		patches = append(patches, instruction(site, "6806d385ea", "68e3272344"))
	}
	for _, site := range []int{0x3885f9, 0x38873d, 0x38879b, 0x3887eb} {
		patches = append(patches, instruction(site, "6805d385ea", "68ca29131d"))
	}

	// Funding refresh reapplies fonts in separate income and expense
	// loops. Keep both branches consistent with the initial layout.
	for _, site := range []int{0x389cb2, 0x389d91, 0x38a3a0, 0x38a479} {
		patches = append(patches, instruction(site, "6806d385ea", "68e3272344"))
	}
	for _, site := range []int{0x389ecf, 0x389f71, 0x38a9ed, 0x38aa9d} {
		patches = append(patches, instruction(site, "6805d385ea", "68ca29131d"))
	}

	// Resize the completed department windows after their controls
	// have been constructed. Empty sections return earlier.
	budgetScaleRVA := stubRVA + len(stub)
	stub = append(stub, budgetTreeScale...)
	for _, layout := range []struct {
		site int
		root byte
		mode byte
		next string
	}{
		{0x3875ca, 0x57, 1, "8d8c2418010000"}, // 650 px dialog, EDI
		{0x38976c, 0x55, 1, "8d8c2458010000"}, // 500 px dialog, EBP
		{0x387c47, 0x55, 2, "8d8c2428010000"}, // 450 px Ordinances, EBP
	} {
		const fontManagerRVA = 0x513c72
		start := stubRVA + len(stub)
		body := []byte{0x50} // Preserve EAX across the adapter.
		body = append(body, relativeInstruction(0xe8, start+len(body), fontManagerRVA)...)
		body = append(body, 0x50, 0x6a, layout.mode, layout.root) // Font manager, center flag, dialog.
		body = append(body, relativeInstruction(0xe8, start+len(body), budgetScaleRVA)...)
		body = append(body, hexBytes("83c40c58")...) // Drop arguments, restore EAX.
		body = append(body, hexBytes(layout.next)...)
		var budgetTree instructionPatch
		stub, budgetTree = appendStub(stub, stubRVA, layout.site, hexBytes(layout.next), body)
		patches = append(patches, budgetTree)
	}
	return stub, patches
}

// appendUISection leaves all stock sections and their file offsets in place.
func appendUISection(data []byte, pe, table, rva int, code []byte) ([]byte, error) {
	const headerSize = 40
	const alignment = 0x1000 // Both alignment fields in the supported Steam EXE.
	count := little.Uint16(data[pe+6:])
	header := table + int(count)*headerSize
	if header+headerSize > int(little.Uint32(data[pe+84:])) ||
		!bytes.Equal(data[header:header+headerSize], make([]byte, headerSize)) ||
		rva%alignment != 0 || len(data)%alignment != 0 {
		return nil, errors.New("no space for the HD UI PE section")
	}
	size := (len(code) + alignment - 1) &^ (alignment - 1)
	copy(data[header:], ".sc4hd")
	little.PutUint32(data[header+8:], uint32(len(code)))
	little.PutUint32(data[header+12:], uint32(rva))
	little.PutUint32(data[header+16:], uint32(size))
	little.PutUint32(data[header+20:], uint32(len(data)))
	little.PutUint32(data[header+36:], 0x60000020) // Code, read, execute.
	little.PutUint16(data[pe+6:], count+1)
	little.PutUint32(data[pe+28:], little.Uint32(data[pe+28:])+uint32(size))
	little.PutUint32(data[pe+80:], uint32(rva+size))
	data = append(data, code...)
	return append(data, make([]byte, size-len(code))...), nil
}

func peChecksum(data []byte, field int) uint32 {
	var total uint64
	for i := 0; i+1 < len(data); i += 2 {
		if i != field && i != field+2 {
			total += uint64(little.Uint16(data[i:]))
		}
	}
	if len(data)%2 != 0 {
		total += uint64(data[len(data)-1])
	}
	for total>>16 != 0 {
		total = (total & 0xffff) + (total >> 16)
	}
	return uint32(total + uint64(len(data)))
}

func runUIExecutable(args []string) error {
	flags := flag.NewFlagSet("patch-ui-exe", flag.ContinueOnError)
	scale := flags.Int("texture-scale", 4, "UI texture scale (2 or 4)")
	hdCursors := flags.Bool("hd-cursors", false, "enable the HD game cursor loader")
	regionMap := flags.Bool("region-map-2x", false, "double the region map")
	loadingPreview := flags.Bool("loading-preview-2x", false, "double the city loading preview")
	if err := flags.Parse(args); err != nil {
		return err
	}
	args = flags.Args()
	if *scale != 2 && *scale != 4 {
		return errors.New("texture scale must be 2 or 4")
	}
	if len(args) != 2 {
		return errors.New("usage: simcity4-uhd patch-ui-exe GAME_DIR OUTPUT.exe")
	}
	data, err := os.ReadFile(filepath.Join(args[0], "Apps", "SimCity 4.exe"))
	if err != nil {
		return err
	}
	data, err = patchUIExecutable(data, *scale, uiExecutableOptions{hdCursors: *hdCursors, loadingPreview: *loadingPreview, regionMap: *regionMap})
	if err != nil {
		return err
	}
	return publishFile(args[1], func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}
