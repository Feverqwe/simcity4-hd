package pe

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/uiids"
)

const StockHash = "f9b059d29940d1a28c89e6d6a7ab147e9762cd03134cdbaf02ba0e0142e1a176"

// Assemble with llvm-mc -triple=i386 -filetype=obj, then extract .text
// with llvm-objcopy -O binary -j .text.
//

// appendStub redirects a fixed-width instruction block and returns to the
// instruction immediately following it.

func appendDisasterBackgroundPatch(stub []byte, patches []x86.Patch, base int) ([]byte, []x86.Patch) {
	const (
		site          = 0x3e7459
		loadBitmapVA  = 0x602b00
		closeBitmapVA = 0x602be0
	)
	entry := base + len(stub)
	stub = append(stub, patchsrc.PrepareDisasterBackground...)
	original := x86.HexBytes("8b5424406850010000")
	// Save the caller's state and pass the bitmap resource that it just loaded.
	body := x86.HexBytes("9c608d442460")
	for _, arg := range []uint32{closeBitmapVA, loadBitmapVA, uiids.DisasterBackgroundX4} {
		body = binary.LittleEndian.AppendUint32(append(body, 0x68), arg)
	}
	body = append(body, 0x50)
	body = append(body, x86.RelativeInstruction(0xe8, base+len(stub)+len(body), entry)...)
	body = append(body, x86.HexBytes("83c410619d")...)
	body = append(body, original...)
	var patch x86.Patch
	stub, patch = x86.AppendStub(stub, base, site, original, body)
	return stub, append(patches, patch)
}

func appendDataMapPatches(stub []byte, patches []x86.Patch, base int) ([]byte, []x86.Patch) {
	// A small city's 64 tiles need 8 pixels each on the 512-pixel map.
	// Keep that zoom available even if the geometry sampler is unavailable.
	const terrainDispatch = 0x3a8560
	const nextTerrainChunk = 0x3a85b0
	tile := base + len(stub)
	stub = append(stub, patchsrc.ExpandTerrainChunk...)
	sampler := base + len(stub)
	stub = append(stub, patchsrc.RenderTerrainChunk...)
	dispatchEntry := base + len(stub)
	// Sample the terrain directly on the 512-pixel canvas. Pass the stock
	// chunk frame, which has its origin, terrain object, light and zone colors.
	body := x86.HexBytes("81bbe4000000000200007500")
	skipSampler := len(body) - 1
	body = append(body, x86.HexBytes("608d4c242051535056")...)
	body = append(body, x86.RelativeInstruction(0xe8, dispatchEntry+len(body), sampler)...)
	body = append(body, x86.HexBytes("83c41085c0617405")...)
	body = append(body, x86.RelativeInstruction(0xe9, dispatchEntry+len(body), nextTerrainChunk)...)
	body[skipSampler] = byte(len(body) - skipSampler - 1)

	body = append(body, x86.HexBytes("8d4a0283f9ff750a")...) // Select the stock tile zoom.
	body = binary.LittleEndian.AppendUint32(append(body, 0xb9), uint32(0x400000+tile))
	body = append(body, x86.RelativeInstruction(0xe9, dispatchEntry+len(body), 0x3a8590)...)
	body = append(body, x86.HexBytes("83f9047605")...)
	body = append(body, x86.RelativeInstruction(0xe9, dispatchEntry+len(body), nextTerrainChunk)...)
	body = append(body, x86.HexBytes("ff248d28867a00")...) // Original writer table.
	var dispatch x86.Patch
	stub, dispatch = x86.AppendStub(stub, base, terrainDispatch,
		x86.HexBytes("8d4a0283f9047748ff248d28867a00"), body)
	patches = append(patches, dispatch)
	// The map already draws coverage circles from world coordinates into its
	// final raster. Scale the native stroke and its scan bounds with the map.
	stub, patches = appendCoverageStrokePatches(stub, patches, base)
	return stub, patches
}

func Patch(data []byte, textureScale int, options x86.Options) ([]byte, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	if hash != StockHash {
		return nil, fmt.Errorf("expected the original Steam Deluxe 1.1.641.0 executable (SHA-256 %s), got %s", StockHash, hash)
	}

	// The fingerprint pins the PE layout as well as the x86 instructions.
	pe := int(binary.LittleEndian.Uint32(data[0x3c:]))
	text := pe + 24 + int(binary.LittleEndian.Uint16(data[pe+20:]))
	rdata := text + 40
	if string(bytes.TrimRight(data[text:text+8], "\x00")) != ".text" ||
		string(bytes.TrimRight(data[rdata:rdata+8], "\x00")) != ".rdata" {
		return nil, errors.New("expected .text and .rdata sections")
	}
	virtualSize := int(binary.LittleEndian.Uint32(data[text+8:]))
	rva := int(binary.LittleEndian.Uint32(data[text+12:]))
	rawStart := int(binary.LittleEndian.Uint32(data[text+20:]))
	stubRVA := int(binary.LittleEndian.Uint32(data[pe+80:])) // SizeOfImage

	// Catalog and Disasters allocate their button frames in code.
	stub, catalog := x86.AppendStub(nil, stubRVA, 0x3eb15d,
		x86.HexBytes("6a1d6a1b6a046a356a506a196a35"), x86.HexBytes("6a3a6a366a086a6a68a00000006a326a6a"))
	stub, disaster := x86.AppendStub(stub, stubRVA, 0x3e749d,
		x86.HexBytes("6a226a286a066a3e6a5e6a196a35"), x86.HexBytes("6a446a506a0c6a7c68bc0000006a326a6a"))

	snapshotRVA := stubRVA + len(stub)
	stub = append(stub, x86.HexBytes("8b4424048b1001d239da771b518b480401c93b4c2420770e750439da7408891089480459eb0459c20400")...)
	const snapshotAppend = 0x467a0
	stub = append(stub, x86.RelativeInstruction(0xe9, stubRVA+len(stub), snapshotAppend)...)
	patches := []x86.Patch{
		catalog, disaster,
		x86.Instruction(0x3eb17b, "83c0f6", "83c0ec"),
		x86.Instruction(0x3eb183, "6a0a", "6a14"),
		x86.Instruction(0x3e74bb, "83c0f6", "83c0ec"),
		x86.Instruction(0x3e74c3, "6a0a", "6a14"),
		x86.Instruction(0x3eaeef, "6a056a2c6a2c8bceff5030", "6a0a6a586a588bceff5030"),
		x86.Instruction(0x3e72a4, "6a056a2c6a2c8bceff5230", "6a0a6a586a588bceff5230"),
		x86.Instruction(0x3f4d19, "68a264258a", "68c3404ae7"),
		// Query help has a bottom offset. Route Query sits inside
		// the upper-right corner of its parent.
		x86.Instruction(0x3edfa2, "8d4205", "8d420a"),
		x86.Instruction(0x3edfa7, "83c70b", "83c716"),
		x86.Instruction(0x3ecff3, "6a08", "6a10"),
		x86.Instruction(0x3ed003, "b9f8ffffff", "b9f0ffffff"),
		// Reopening a movable dialog can restore a saved position. Scale
		// the visible margin and the fallback used for off-screen positions.
		x86.Instruction(0x203796, "83c3f8", "83c3f0"),
		x86.Instruction(0x2037a1, "83c7f8", "83c7f0"),
		x86.Instruction(0x2037aa, "83fd08", "83fd10"),
		x86.Instruction(0x2037b1, "83f808", "83f810"),
		x86.Instruction(0x2037cd, "6a086a08", "6a106a10"),
		// Runtime HUD controls use image IDs and sizes outside the layouts.
		x86.Instruction(0x3ee037, "c7442440b006a046c744244488452453", "c7442440bd404ae7c744244488452453"),
		x86.Instruction(0x3ee15a, "83e81c506a0c", "83e838506a18"),
		// Polls compute frame width from the atlas. The positive arrows
		// still start at frame three in the six-frame replacement.
		x86.Instruction(0x3ed4ce, "c744243880550114", "c7442438ac60d22b"),
		x86.Instruction(0x3ed4f3, "c744243884550114", "c7442438ab60d22b"),
		x86.Instruction(0x37019c, "6888f8064c", "68c0404ae7"),
		// Both Ordinances tables use the ledger header style. The expense
		// table otherwise inherits the enlarged advisor headline.
		x86.Instruction(0x37ce15, "689e4a0faa", "6804d385ea"),
		x86.Instruction(0x37ce5b, "689e4a0faa", "6804d385ea"),
		// Tooltip titles and descriptions use doubled fonts. Scale the
		// wrap limit and leave the available width to the stock layout.
		x86.Instruction(0x39880a, "68fa000000", "68f4010000"),
		x86.Instruction(0x3988a9, "68fa000000", "68f4010000"),
		// Mission progress resizes its panel for active and idle text.
		x86.Instruction(0x3ebcc5, "83c008", "83c010"),
		x86.Instruction(0x3ebce3, "83c008", "83c010"),
		// Text and viewport-constrained capture presets scale together.
		x86.Instruction(0x3b7af5, "6a0e", "6a1c"),
		x86.Instruction(0x3b6bef, "83ea04", "83ea08"),
		x86.Instruction(0x3b6bf2, "83ef04", "83ef08"),
		x86.Instruction(0x3b6bf7, "83c004", "83c008"),
		x86.Instruction(0x3b6bfa, "83c104", "83c108"),
	}
	// The tutorial exit menu overwrites the layout's authored origin.
	var tutorialExit x86.Patch
	stub, tutorialExit = x86.AppendStub(stub, stubRVA, 0x41f82,
		x86.HexBytes("6a786a32ff90e0000000"), x86.HexBytes("68f00000006a64ff90e0000000"))
	patches = append(patches, tutorialExit)
	for _, site := range []int{0x3b7ba9, 0x3b7bc5, 0x3b7bdd, 0x3b7bfd, 0x3b7c21,
		0x3b7c42, 0x3b7c62, 0x3b7c86, 0x3b7caa, 0x3b7cce} {
		patches = append(patches, x86.Patch{RVA: site,
			Original: x86.RelativeInstruction(0xe8, site, snapshotAppend), Replacement: x86.RelativeInstruction(0xe8, site, snapshotRVA)})
	}
	if textureScale == 4 {
		stub, patches = appendDisasterBackgroundPatch(stub, patches, stubRVA)
		stub, patches = appendCatalogPatches(stub, patches, stubRVA)

		// Both My Sims preview loaders assign full-density bitmaps at runtime.
		// Mark them for the density painter and for physical source clipping.
		patches = append(patches, x86.Instruction(0x36eeab, "6888f8064c", "68c0404ae7"))
		for _, site := range []int{0x36ef1b, 0x3701ba} {
			var preview x86.Patch
			stub, preview = x86.AppendStub(stub, stubRVA, site,
				x86.HexBytes("8b1150ff5210"),
				x86.HexBytes("804923a08b1150ff5210")) // Mark flags, then SetBitmap.
			patches = append(patches, preview)
		}
		var previewCrop x86.Patch
		stub, previewCrop = x86.AppendStub(stub, stubRVA, 0x5bc459,
			x86.HexBytes("8d86e8000000"),
			// Runtime preview source limits are in texture pixels. The
			// stock continuation clips those limits to the bitmap size.
			x86.HexBytes("f686fb00000020740401d201c98d86e8000000"))
		patches = append(patches, previewCrop)
	}

	// The city loads its live rating sprite during initialization and
	// update. The region launcher loads its separate tiled sprite.
	for _, site := range []int{0x3e8522, 0x3ed229, 0x3b5184} {
		patches = append(patches, x86.Patch{RVA: site, Original: binary.LittleEndian.AppendUint32(nil, uiids.CatalogFrameGroup),
			Replacement: binary.LittleEndian.AppendUint32(nil, uiids.MayorRatingGroup)})
	}
	// Trend windows resize at runtime in screen pixels. Keep the negative
	// window's right edge fixed as the number of arrows changes.
	patches = append(patches,
		x86.Instruction(0x3e87b1, "6bf607", "6bf60e"),
		x86.Instruction(0x3e89d7, "6bc907", "6bc90e"),
		x86.Instruction(0x3e8a02, "6bc907", "6bc90e"),
	)

	// Both Audio Options playlist sources use the native checkbox painter.
	for _, site := range []int{0xf4b84, 0xf4e43} {
		patches = append(patches, x86.Patch{RVA: site,
			Original: binary.LittleEndian.AppendUint32(nil, uiids.CatalogFrameGroup), Replacement: binary.LittleEndian.AppendUint32(nil, uiids.BudgetRowArtGroup)})
	}
	// Audio Options replaces the track-name column width after loading its layout.
	patches = append(patches, x86.Instruction(0xf4b43, "68ff000000", "68fe010000"))

	// The import dialog loads its folder and city icons directly.
	for _, site := range []int{0x379135, 0x379182, 0x3791e5} {
		patches = append(patches, x86.Patch{RVA: site, Original: binary.LittleEndian.AppendUint32(nil, uiids.CatalogFrameGroup),
			Replacement: binary.LittleEndian.AppendUint32(nil, uiids.ImportIconGroup)})
	}

	// The tree's root font does not apply to dynamically created rows.
	importNodeFontRVA := stubRVA + len(stub)
	stub = append(stub, patchsrc.ImportNodeFont...)
	for _, node := range []struct {
		site int
		next string
	}{
		{0x378e3f, "8b168bd88b4748"}, // Folder node.
		{0x378fc3, "8b168be88b4758"}, // City node.
	} {
		body := x86.RelativeInstruction(0xe8, stubRVA+len(stub), importNodeFontRVA)
		original := x86.HexBytes(node.next)
		var nodeFont x86.Patch
		stub, nodeFont = x86.AppendStub(stub, stubRVA, node.site, original, append(body, original...))
		patches = append(patches, nodeFont)
	}

	stub, patches = appendDataMapPatches(stub, patches, stubRVA)

	// Data Views positions the legend again when switching overlays.
	// Keep each layout's swatch column and align captions beside it.
	var mapLegendSwatch x86.Patch
	stub, mapLegendSwatch = x86.AppendStub(stub, stubRVA, 0x3a081a,
		x86.HexBytes("ff9210010000"),
		// GetArea, then copy the current left and right into the new rectangle.
		x86.HexBytes("ff92c00000008b10899424980000008b5008899424a00000008bce8b16ff9210010000"))
	patches = append(patches, mapLegendSwatch,
		x86.Instruction(0x3a07d4, "8d573d", "8d577a"),
		x86.Instruction(0x3a080b, "8d44383d", "8d44387a"),
		x86.Instruction(0x3a08f4, "8d4f18", "8d4f30"),
		x86.Instruction(0x3a08fd, "c744243c16010000", "c744243c2c020000"),
		x86.Instruction(0x3a090b, "0516010000", "052c020000"),
		x86.Instruction(0x3a0922, "8d4c3818", "8d4c3830"))

	// The graph window builds its plot rectangle in screen pixels.
	// Leave room for the enlarged ticks, title, and line-chart legend.
	var graphLegend x86.Patch
	stub, graphLegend = x86.AppendStub(stub, stubRVA, 0x36dd4b,
		x86.HexBytes("83e81483ea6e"), x86.HexBytes("83e82081ead4000000"))
	patches = append(patches, graphLegend,
		x86.Instruction(0x36d834, "83e814", "83e820"),
		x86.Instruction(0x36d848, "c78424980000002d000000", "c784249800000046000000"),
		x86.Instruction(0x36d853, "c784249c00000014000000", "c784249c00000020000000"),
		x86.Instruction(0x36dd5f, "c78424a80000002d000000", "c78424a800000046000000"),
		x86.Instruction(0x36dd6a, "c78424ac00000014000000", "c78424ac00000020000000"))

	// Both legend paths need the same gutter. Line and bar charts use
	// direct-drawn swatches, while other charts create bitmap controls.
	var graphLegendStart, graphSwatches, graphLabels x86.Patch
	stub, graphLegendStart = x86.AppendStub(stub, stubRVA, 0x36e0f5,
		x86.HexBytes("83eb6a83f802"), x86.HexBytes("81ebca00000083f802"))
	stub, graphSwatches = x86.AppendStub(stub, stubRVA, 0x36e159,
		x86.HexBytes("8d4aa4518b4c24205183c294"),
		x86.HexBytes("8d8a44ffffff518b4c24205181c234ffffff"))
	stub, graphLabels = x86.AppendStub(stub, stubRVA, 0x36e1f1,
		x86.HexBytes("8d8c24f800000083eb5a"), x86.HexBytes("8d8c24f800000081ebba000000"))
	patches = append(patches, graphLegendStart, graphSwatches, graphLabels)

	// Draw a second scanline for data series, clamped to the top of
	// the plot. The axes, grid, and legend have separate draw paths.
	var graphSeries x86.Patch
	stub, graphSeries = x86.AppendStub(stub, stubRVA, 0x5b60f4, x86.HexBytes("ff9080000000"),
		x86.HexBytes("ff90800000008b86e40000003945cc7e03ff4dcc3945d47e03ff4dd4"+
			"ff75fc8d55c852ff75088d8e2c0200008b01ff9080000000"))
	patches = append(patches, graphSeries)

	// The My Sims zodiac and display-resolution combos draw their arrows without images. Reserve
	// twice the stock button width, bounded by the current control width.
	// This path runs on every resize and is separate from image-backed arrows.
	var zodiacArrow x86.Patch
	stub, zodiacArrow = x86.AppendStub(stub, stubRVA, 0x5cf007,
		x86.HexBytes("89be20010000"),
		x86.HexBytes("89be20010000817e105e3f24aa7412817e1001444ae77409817e1004444ae775228b44240c0fbe96340100002bc233d285c07e096a205a39d07d028bd089961c010000"))
	patches = append(patches, zodiacArrow)

	stub, patches = appendBudgetUIPatches(stub, patches, stubRVA)
	stub, patches = x86.AppendAnchoredPanelFit(stub, patches, stubRVA, false)
	stub, patches = appendEditorDecimalPatches(stub, patches, stubRVA)
	stub, patches = appendKeyboardHelpPatch(stub, patches, stubRVA)
	stub, patches = appendGraphicsResolutionPatches(stub, patches, stubRVA)
	if options.RussianBudgetTips {
		stub, patches = appendBudgetTooltipPatch(stub, patches, stubRVA)
	}
	stub, patches = appendRuntimeArtPatches(stub, patches, stubRVA)
	stub, patches = appendDriveGaugePatches(stub, patches, stubRVA)

	// The HTML control creates its vertical scrollbar at runtime. Resize
	// the child rectangle and reserve matching space in the text viewport.
	var bottom, left, horizontalRight x86.Patch
	stub, bottom = x86.AppendStub(stub, stubRVA, 0x4fa8b9,
		x86.HexBytes("508b068bce"), x86.HexBytes("83f80873056a0458eb0383e804508b068bce"))
	// SetArea takes edges, and the stock left edge depends on the child
	// width from the previous layout. Use a fixed width across resizes.
	stub, left = x86.AppendStub(stub, stubRVA, 0x4fa8c5,
		x86.HexBytes("8b036a008bcbff90a400000033db432bd88b068bce"),
		x86.HexBytes("6a04bbe4ffffff8b068bce"))
	// Leave the bottom-right corner to the vertical bar when both are present.
	stub, horizontalRight = x86.AppendStub(stub, stubRVA, 0x4fa91f,
		x86.HexBytes("508b038bcb"), x86.HexBytes("83bea401000000740783e81c790233c0508b038bcb"))
	patches = append(patches, bottom, left, horizontalRight,
		x86.Instruction(0x4fbcbf, "c7868c01000016000000", "c7868c01000024000000"))
	// Initial layout only runs when the HTML view creates its bars. Keep
	// their bounds in sync when an existing view is resized and reflows.
	const resizeHTML = 0x4facef
	resizeBars := stubRVA + len(stub)
	body := x86.HexBytes("8bce") // mov ecx, esi
	body = append(body, x86.RelativeInstruction(0xe8, resizeBars+len(body), 0x4fa88a)...)
	body = append(body, x86.HexBytes("8bce")...)
	body = append(body, x86.RelativeInstruction(0xe8, resizeBars+len(body), 0x4fa559)...)
	var resize x86.Patch
	stub, resize = x86.AppendStub(stub, stubRVA, resizeHTML, x86.HexBytes("8bcee863f8ffff"), body)
	patches = append(patches, resize)
	// The horizontal update must not overwrite the vertical range.
	for _, site := range []int{0x4fa849, 0x4fa857, 0x4fa872} {
		patches = append(patches, x86.Instruction(site, "8b8ea4010000", "8b8ea8010000"))
	}
	stub, patches = appendDensityUIPatches(stub, patches, stubRVA, textureScale)
	if options.HdCursors {
		// The hardware path calls LoadCursorFromFileA, which requests the
		// system cursor size. LoadImageA with only LR_LOADFROMFILE instead
		// preserves the file dimensions and hotspot. Keep the original API
		// as a fallback for cursor formats LoadImageA cannot load.
		const loadHardware = 0x492368
		cursorLoader := stubRVA + len(stub)
		stub = append(stub, x86.HexBytes(
			"8b4424046a106a006a006a02506a00"+ // LoadImageA(NULL, path, IMAGE_CURSOR, 0, 0, LR_LOADFROMFILE).
				"ff158405a80085c0750a"+ // On success return the new cursor handle.
				"ff742404ff158805a800"+ // Fall back to LoadCursorFromFileA(path).
				"c20400")...)
		patches = append(patches, x86.Patch{
			RVA: loadHardware, Original: x86.HexBytes("ff158805a800"),
			Replacement: append(x86.RelativeInstruction(0xe8, loadHardware, cursorLoader), 0x90),
		},
			// The software path already calls LoadImageA. Clear LR_DEFAULTSIZE.
			x86.Instruction(0x4923a6, "6a50", "6a10"))
		// GetDIBits supplies straight BGRA for HD color cursors. The
		// original conversion forces that alpha to 255 before applying
		// the AND mask. Keep supplied alpha and retain the opaque fill
		// for monochrome and legacy color cursors with no alpha channel.
		var softwareAlpha x86.Patch
		stub, softwareAlpha = x86.AppendStub(stub, stubRVA, 0x49253d,
			x86.HexBytes("3bf374118b45348d44b0fc804803ff83e8044e75f6"),
			x86.HexBytes("85f6742c837d5400741189f18b453480780300751983c0044975f4"+
				"8b45348d44b0fc804803ff83e8044e75f6eb0231f6"))
		patches = append(patches, softwareAlpha)
	}
	if options.LoadingPreview {
		const site = 0x377149
		const loadThumbnail = 0x1dda40
		const bufferFactory = 0x90ddf1
		expandRVA := stubRVA + len(stub)
		stub = append(stub, patchsrc.ExpandLoadingPreview...)
		wrapperRVA := stubRVA + len(stub)
		// Replay the cdecl thumbnail load. Only the loading-screen caller
		// switches to the larger buffers. The normal paint path centers
		// and blends them using the actual buffer dimensions.
		stub = append(stub, bytes.Repeat(x86.HexBytes("ff742414"), 5)...)
		stub = append(stub, x86.RelativeInstruction(0xe8, stubRVA+len(stub), loadThumbnail)...)
		stub = append(stub, x86.HexBytes("83c41484c0")...)
		loadFailed := len(stub)
		stub = append(stub, 0x74, 0)    // Return immediately on load failure.
		stub = append(stub, 0x50, 0x68) // Save the stock return value and pass the factory.
		stub = binary.LittleEndian.AppendUint32(stub, bufferFactory)
		stub = append(stub, x86.HexBytes("ff742414ff742414")...) // Coverage and color output pointers.
		stub = append(stub, x86.RelativeInstruction(0xe8, stubRVA+len(stub), expandRVA)...)
		stub = append(stub, x86.HexBytes("83c40c58")...)
		stub[loadFailed+1] = byte(len(stub) - loadFailed - 2)
		stub = append(stub, 0xc3)
		patches = append(patches, x86.Patch{RVA: site,
			Original: x86.RelativeInstruction(0xe8, site, loadThumbnail), Replacement: x86.RelativeInstruction(0xe8, site, wrapperRVA)})
	}
	// The native wheel handler already turns wheel delta into a row count.
	// Redirect only its relative-scroll call; the helper recognizes the
	// Audio Options grid and delegates every other grid to the stock call.
	playlistRVA := stubRVA + len(stub)
	stub = append(stub, patchsrc.ScrollPlaylist...)
	wheelBody := x86.HexBytes("83c1fc50")
	wheelBody = append(wheelBody, x86.RelativeInstruction(0xe8, stubRVA+len(stub)+len(wheelBody), playlistRVA)...)
	var wheel x86.Patch
	stub, wheel = x86.AppendStub(stub, stubRVA, 0x5a5f89, x86.HexBytes("83c1fc8b3150ff567c"), wheelBody)
	patches = append(patches, wheel)
	if options.RegionMap {
		stub, patches = patchRegionMap(data, stub, stubRVA, patches)
	}
	for _, patch := range patches {
		offset := rawStart + patch.RVA - rva
		end := offset + len(patch.Original)
		if offset < rawStart || end > rawStart+virtualSize || len(patch.Original) != len(patch.Replacement) ||
			!bytes.Equal(data[offset:end], patch.Original) {
			return nil, fmt.Errorf("unexpected UI instructions at RVA %#x", patch.RVA)
		}
		copy(data[offset:end], patch.Replacement)
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
	for i := 0; i < uiids.NewsMarkerCount; i++ {
		at := 0x6837f4 - i*0x2c
		original := fmt.Sprintf(`<IMG SRC="sc4://HTML/%08x/%08X">`, uiids.CatalogFrameGroup, uiids.NewsMarkerFirst+i)
		marker := data[at : at+len(original)]
		if !bytes.Equal(marker, []byte(original)) {
			return nil, fmt.Errorf("unexpected News marker HTML template at offset %#x", at)
		}
		copy(marker, fmt.Sprintf(`<IMG SRC="sc4://HTML/%08x/%08X">`, uiids.NewsMarkerGroup, uiids.NewsMarkerFirst+i))
	}

	data, err := appendUISection(data, pe, text, stubRVA, stub)
	if err != nil {
		return nil, err
	}
	binary.LittleEndian.PutUint16(data[pe+22:], binary.LittleEndian.Uint16(data[pe+22:])|0x20)
	binary.LittleEndian.PutUint32(data[pe+88:], peChecksum(data, pe+88))
	return data, nil
}

func patchUIString(data []byte, section, offset int, original, updated string) error {
	start := int(binary.LittleEndian.Uint32(data[section+20:]))
	end := offset + len(original)
	slotEnd := offset + max(len(original), len(updated)) + 1
	if offset < start || slotEnd > start+int(binary.LittleEndian.Uint32(data[section+16:])) ||
		string(data[offset:end]) != original || len(bytes.Trim(data[end:slotEnd], "\x00")) != 0 {
		return fmt.Errorf("unexpected UI string at offset %#x", offset)
	}
	slot := data[offset:slotEnd]
	clear(slot)
	copy(slot, updated)
	return nil
}

func appendBudgetTooltipPatch(stub []byte, patches []x86.Patch, base int) ([]byte, []x86.Patch) {
	const (
		heading          = 0x386e83
		textHeading      = 0x379b80
		buttonHeading    = 0x379ca0
		site             = 0x3875d1
		releaseBitmapRVA = 0x202be0
	)
	// Construct only the two abbreviated headings as native button labels.
	// Both constructors take the same arguments except for the alignment enum.
	start := base + len(stub)
	choose := x86.HexBytes("81be38010000ba54564a740c81be380100009e5456ea750f")
	choose = append(choose, x86.HexBytes("c744241800000000")...) // Centered button caption.
	choose = append(choose, x86.RelativeInstruction(0xe8, start+len(choose), buttonHeading)...)
	choose = append(choose, x86.HexBytes("eb05")...)
	choose = append(choose, x86.RelativeInstruction(0xe8, start+len(choose), textHeading)...)
	var constructor x86.Patch
	stub, constructor = x86.AppendStub(stub, base, heading, x86.RelativeInstruction(0xe8, heading, textHeading), choose)

	entry := base + len(stub)
	stub = append(stub, patchsrc.BudgetTooltip...)
	// The wide department ledger has completed its layout and HD font pass.
	// ESI still holds its controller with the heading's parent window.
	body := []byte{0x60} // Preserve the original caller registers.
	for _, arg := range []uint32{0x414250, 0x603580, uiids.RussianBudgetTooltipGroup} {
		body = binary.LittleEndian.AppendUint32(append(body, 0x68), arg)
	}
	body = append(body, x86.HexBytes("ffb638010000ff7610")...)
	body = append(body, x86.RelativeInstruction(0xe8, base+len(stub)+len(body), entry)...)
	body = append(body, x86.HexBytes("83c41461")...)
	markReady := x86.HexBytes("c6460801")
	body = append(body, markReady...)
	body = append(body, x86.RelativeInstruction(0xe8, base+len(stub)+len(body), releaseBitmapRVA)...)
	original := append(markReady, x86.RelativeInstruction(0xe8, site+len(markReady), releaseBitmapRVA)...)
	var patch x86.Patch
	stub, patch = x86.AppendStub(stub, base, site, original, body)
	return stub, append(patches, constructor, patch)
}

func appendEditorDecimalPatches(stub []byte, patches []x86.Patch, base int) ([]byte, []x86.Patch) {
	// Localized editors and the CRT may use different decimal separators.
	// Only intercept user-entered numbers held in a temporary edit buffer.
	addParser := func(parser int, sites ...int) {
		entry := base + len(stub)
		stub = append(stub, patchsrc.EditorDecimal...)
		stub = append(stub, x86.RelativeInstruction(0xe9, base+len(stub), parser)...)
		for _, site := range sites {
			patches = append(patches, x86.Patch{
				RVA: site, Original: x86.RelativeInstruction(0xe8, site, parser),
				Replacement: x86.RelativeInstruction(0xe8, site, entry),
			})
		}
	}
	addParser(0x5f0eae, 0x369188)                               // Tax-rate editor (strtod).
	addParser(0x50fc5b, 0x26cea5, 0x26cefb, 0x26d03a, 0x26d125) // Curve editor (atof).
	return stub, patches
}

func appendBudgetUIPatches(stub []byte, patches []x86.Patch, stubRVA int) ([]byte, []x86.Patch) {
	patches = append(patches,
		// Reject notifications outside the twelve rate editors.
		x86.Instruction(0x3690fc, "7909", "9090"),
		x86.Instruction(0x369101, "0f8d55010000", "0f8355010000"),
	)
	// Require a nonempty, fully parsed number. On failure reformat the
	// spinner's value without writing a rate from the stock fallback table.
	const checkTaxEnd = 0x3691a5
	entry := stubRVA + len(stub)
	stub = append(stub, x86.HexBytes("39d1740b03c239c17505")...) // end != start && end == start + len
	stub = append(stub, x86.RelativeInstruction(0xe9, stubRVA+len(stub), 0x3691c6)...)
	stub = append(stub, x86.RelativeInstruction(0xe9, stubRVA+len(stub), 0x3691ed)...)
	patches = append(patches, x86.Patch{RVA: checkTaxEnd, Original: x86.HexBytes("03c23bc1741b"),
		Replacement: append(x86.RelativeInstruction(0xe9, checkTaxEnd, entry), 0x90)})

	// Keep the gaps between the tabs, main ledger and finance controls
	// when the game rebuilds the Budget window.
	patches = append(patches,
		x86.Instruction(0x369583, "83c002", "83c004"),
		x86.Instruction(0x3695a6, "83c008", "83c010"),
	)
	// This constructor is shared by the four dynamically built budget
	// slider paths. Give the stock slider the projected four-state atlas.
	var budgetSlider x86.Patch
	stub, budgetSlider = x86.AppendStub(stub, stubRVA, 0x379508, x86.HexBytes("8bf08b168bce"), patchsrc.BudgetSliderArt)
	patches = append(patches, budgetSlider)

	// Both Ordinances ledgers share a dedicated checkbox constructor.
	patches = append(patches, x86.Patch{RVA: 0x37b7c1,
		Original: binary.LittleEndian.AppendUint32(nil, uiids.CatalogFrameGroup), Replacement: binary.LittleEndian.AppendUint32(nil, uiids.BudgetRowArtGroup)})

	// The checkbox frame grew around the old top-left anchor.
	// Center it in the ledger row before the dialog doubles its positions.
	var checkboxPosition x86.Patch
	stub, checkboxPosition = x86.AppendStub(stub, stubRVA, 0x37b90c,
		x86.HexBytes("8b1750518bcf"), x86.HexBytes("8b1783e80550518bcf"))
	patches = append(patches, checkboxPosition)

	// Open Details and the enlarged empty-dialog close button use
	// projected atlases. Preserve the stock group for other callers.
	var budgetDetailsArt x86.Patch
	stub, budgetDetailsArt = x86.AppendStub(stub, stubRVA, 0x37b970,
		x86.HexBytes("5068b006a046"),
		x86.HexBytes("503db755011474073d60624114750768c8404ae7eb0568b006a046"))
	patches = append(patches, budgetDetailsArt,
		// The active-state frame is wider than the glyph. Leave room
		// for the label that starts at X=48 in department rows.
		x86.Instruction(0x388abf, "6a12", "6a0e"),
		x86.Instruction(0x388f94, "6a12", "6a0e"))

	// Empty ledger sections share a dialog built in code. Keep the
	// root, text pane, and frame at the same size for layout and input.
	for _, site := range []int{0x37c19e, 0x37c1b0, 0x37c1d9, 0x37c2e0, 0x37c301} {
		var emptyLedgerSize x86.Patch
		stub, emptyLedgerSize = x86.AppendStub(stub, stubRVA, site,
			x86.HexBytes("6a64682c010000"), x86.HexBytes("68c80000006858020000"))
		patches = append(patches, emptyLedgerSize)
	}
	patches = append(patches,
		x86.Instruction(0x37c254, "6807d385ea", "687ecf0353"), // HDBdgtPopupHeader
		x86.Instruction(0x37c27e, "6808d385ea", "685e3d6e95"), // HDBdgtPopupBody
		x86.Instruction(0x37c260, "6a056a0a", "6a0a6a14"),
		x86.Instruction(0x37c286, "6a196a0f", "6a326a1e"),
		x86.Instruction(0x37c2b5, "68b4550114", "6860624114"),
		x86.Instruction(0x37c2ba, "6a0b680d010000", "6a12681a020000"))

	stub, patches = appendOrdinanceDetailsPatch(stub, patches, stubRVA)

	// The funding refresh recomputes the marker's X in stock units.
	// Its Y already comes from the resized control.
	var budgetMarker x86.Patch
	stub, budgetMarker = x86.AppendStub(stub, stubRVA, 0x38a729,
		x86.HexBytes("508bcfff93e0000000"), x86.HexBytes("8d0400508bcfff93e0000000"))
	patches = append(patches, budgetMarker)

	// The department income table creates its labels and amounts
	// with the stock ledger fonts, including on a row rebuild.
	for _, site := range []int{0x388517, 0x3885a0} {
		patches = append(patches, x86.Instruction(site, "6806d385ea", "68e3272344"))
	}
	for _, site := range []int{0x3885f9, 0x38873d, 0x38879b, 0x3887eb} {
		patches = append(patches, x86.Instruction(site, "6805d385ea", "68ca29131d"))
	}

	// Funding refresh reapplies fonts in separate income and expense
	// loops. Keep both branches consistent with the initial layout.
	for _, site := range []int{0x389cb2, 0x389d91, 0x38a3a0, 0x38a479} {
		patches = append(patches, x86.Instruction(site, "6806d385ea", "68e3272344"))
	}
	for _, site := range []int{0x389ecf, 0x389f71, 0x38a9ed, 0x38aa9d} {
		patches = append(patches, x86.Instruction(site, "6805d385ea", "68ca29131d"))
	}

	// Neighbor Deals reassigns the row fonts as the income and expense lists scroll.
	for _, site := range []int{
		0x3826eb, 0x3827b1, 0x382841, 0x382f3a, 0x383000, 0x383090,
		0x383615, 0x3836da, 0x383769, 0x383f32, 0x383ff7, 0x384086,
		0x384615, 0x3846db, 0x38476b, 0x384cf5, 0x384dbb, 0x384e4b,
	} {
		patches = append(patches, x86.Instruction(site, "6806d385ea", "68e3272344"))
	}
	for _, site := range []int{
		0x382b1e, 0x382b99, 0x382c55, 0x383226, 0x3832a1, 0x38335d,
		0x383902, 0x383990, 0x383a3f, 0x38421f, 0x3842ad, 0x38435c,
		0x384901, 0x38497c, 0x384a38, 0x384fe1, 0x38505c, 0x385118,
	} {
		patches = append(patches, x86.Instruction(site, "6805d385ea", "68ca29131d"))
	}

	// Resize the completed ledger windows. The shared Ordinances and
	// Neighbor Deals exits can also receive an already-scaled message.
	budgetScaleRVA := stubRVA + len(stub)
	stub = append(stub, patchsrc.BudgetTreeScale...)
	for _, layout := range []struct {
		site  int
		root  byte
		width uint32
		next  string
	}{
		{0x3875ca, 0x57, 1, "8d8c2418010000"},   // 650 px dialog, EDI
		{0x38976c, 0x55, 1, "8d8c2458010000"},   // 500 px dialog, EBP
		{0x387c47, 0x55, 450, "8d8c2428010000"}, // Ordinances, EBP
		{0x387d62, 0x55, 500, "8d8c2428010000"}, // Neighbor Deals, EBP
	} {
		const fontManagerRVA = 0x513c72
		start := stubRVA + len(stub)
		body := []byte{0x50} // Preserve EAX across the adapter.
		body = append(body, x86.RelativeInstruction(0xe8, start+len(body), fontManagerRVA)...)
		body = binary.LittleEndian.AppendUint32(append(body, 0x50, 0x68), layout.width) // Font manager and expected width.
		body = append(body, layout.root)
		body = append(body, x86.RelativeInstruction(0xe8, start+len(body), budgetScaleRVA)...)
		body = append(body, x86.HexBytes("83c40c58")...) // Drop arguments, restore EAX.
		body = append(body, x86.HexBytes(layout.next)...)
		var budgetTree x86.Patch
		stub, budgetTree = x86.AppendStub(stub, stubRVA, layout.site, x86.HexBytes(layout.next), body)
		patches = append(patches, budgetTree)
	}
	return stub, patches
}

func appendOrdinanceDetailsPatch(stub []byte, patches []x86.Patch, stubRVA int) ([]byte, []x86.Patch) {
	// Open Details builds a new window for the selected ordinance. Set
	// the pane size before the stock text constructor wraps the body.
	for _, block := range []struct {
		site          int
		before, after string
	}{
		// The controller still holds the stock width of the Ordinances ledger.
		{0x38b99d, "8b116a7d83eb3c53", "8b1168fa0000008d5c1b8853"},
		{0x38b9ae, "8b016a7d53", "8b0168fa00000053"},
		{0x38bace, "6a7d538bc8", "68fa000000538bc8"},
		{0x38baea, "6a7d538bc8", "68fa000000538bc8"},
		// The row Y is already in screen pixels. Clamp it against the
		// actual parent height with the new popup height.
		{0x38b9b9, "8b87800000008b4c241483c083", "8bcd8b01ff90a80000002dfa00000033d285c00f48c28b4c2414"},
	} {
		var patch x86.Patch
		stub, patch = x86.AppendStub(stub, stubRVA, block.site, x86.HexBytes(block.before), x86.HexBytes(block.after))
		patches = append(patches, patch)
	}
	return stub, append(patches,
		x86.Instruction(0x38b9d7, "6a1e", "6a3c"),
		x86.Instruction(0x38ba14, "6807d385ea", "687ecf0353"), // HDBdgtPopupHeader
		x86.Instruction(0x38ba29, "6a056a0a", "6a0a6a14"),
		x86.Instruction(0x38ba60, "6808d385ea", "685e3d6e95"), // HDBdgtPopupBody
		x86.Instruction(0x38ba75, "6a196a0f", "6a326a1e"),
		x86.Instruction(0x38baa8, "68b4550114", "6860624114"),
		x86.Instruction(0x38baad, "6a0b8d53e1", "6a168d53c2"))
}

// appendUISection leaves all stock sections and their file offsets in place.
func appendUISection(data []byte, pe, table, rva int, code []byte) ([]byte, error) {
	const headerSize = 40
	const alignment = 0x1000 // Both alignment fields in the supported Steam EXE.
	count := binary.LittleEndian.Uint16(data[pe+6:])
	header := table + int(count)*headerSize
	if header+headerSize > int(binary.LittleEndian.Uint32(data[pe+84:])) ||
		!bytes.Equal(data[header:header+headerSize], make([]byte, headerSize)) ||
		rva%alignment != 0 || len(data)%alignment != 0 {
		return nil, errors.New("no space for the HD UI PE section")
	}
	size := (len(code) + alignment - 1) &^ (alignment - 1)
	copy(data[header:], ".sc4hd")
	binary.LittleEndian.PutUint32(data[header+8:], uint32(len(code)))
	binary.LittleEndian.PutUint32(data[header+12:], uint32(rva))
	binary.LittleEndian.PutUint32(data[header+16:], uint32(size))
	binary.LittleEndian.PutUint32(data[header+20:], uint32(len(data)))
	binary.LittleEndian.PutUint32(data[header+36:], 0x60000020) // Code, read, execute.
	binary.LittleEndian.PutUint16(data[pe+6:], count+1)
	binary.LittleEndian.PutUint32(data[pe+28:], binary.LittleEndian.Uint32(data[pe+28:])+uint32(size))
	binary.LittleEndian.PutUint32(data[pe+80:], uint32(rva+size))
	data = append(data, code...)
	return append(data, make([]byte, size-len(code))...), nil
}

func peChecksum(data []byte, field int) uint32 {
	var total uint64
	for i := 0; i+1 < len(data); i += 2 {
		if i != field && i != field+2 {
			total += uint64(binary.LittleEndian.Uint16(data[i:]))
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
