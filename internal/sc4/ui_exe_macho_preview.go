package sc4

import (
	"bytes"
	"debug/macho"
	_ "embed"
	"fmt"
)

//go:embed patchsrc/ui_loading_preview.x86_64
var macLoadingPreviewCode []byte

const (
	macUIOptionalCode     = 0xb9a000
	macLoadingPreviewSite = 0x32e04d
)

func patchMachOFeatures(data []byte, options uiExecutableOptions, scale int) ([]byte, error) {
	hasUI := scale != 0
	var code []byte
	var liveRenderContext int
	var patches []instructionPatch
	if options.loadingPreview {
		code, patches = appendMachOLoadingPreview(code, patches)
	}
	if options.regionMap {
		code, patches = appendMachORegionMap(data, code, patches)
	}
	if options.hdCursors {
		var err error
		code, patches, err = appendMachOCursors(code, patches, options.cursorScale)
		if err != nil {
			return nil, err
		}
	}

	code, patches = appendMachODisplayOptions(code, patches, hasUI)
	code, patches, err := appendMachOWindowResize(code, patches)
	if err != nil {
		return nil, err
	}
	if hasUI {
		code, patches, err = appendMachOEditorDecimal(code, patches)
		if err != nil {
			return nil, err
		}
		if options.russianBudgetTips {
			code, patches, err = appendMachOBudgetTooltips(code, patches)
			if err != nil {
				return nil, err
			}
		}
		code, patches = appendMachOBudgetPopups(code, patches)
		code, patches, err = appendMachOBudgetTrees(code, patches)
		if err != nil {
			return nil, err
		}
		code, patches, err = appendMachORuntimeArtwork(code, patches)
		if err != nil {
			return nil, err
		}
		code, patches, err = appendMachOHTMLScrollbars(code, patches)
		if err != nil {
			return nil, err
		}
		code, patches = appendMachODataViews(code, patches)
		code, patches = appendMachOGraphs(code, patches)
		code, patches = appendMachOImportNodes(code, patches)
		code, patches = appendMachOSnapshots(code, patches)
		code, patches = appendMachOPlaylist(code, patches)
		code, patches = appendMachOCombo(code, patches)
		code, patches, liveRenderContext = appendMachOLiveDisplay(code, patches)
		code, patches = appendMachOFontMetrics(code, patches)
		code, patches = appendAnchoredPanelFit(code, patches, macUIOptionalCode, true)
	}
	if scale == 4 {
		code, patches = appendMachODisasterBackground(code, patches)
	}
	if options.retinaText {
		if !hasUI {
			return nil, fmt.Errorf("Retina text requires the UHD interface")
		}
		code, patches, err = appendMachORetinaText(code, patches, liveRenderContext)
		if err != nil {
			return nil, err
		}
	}
	patched, err := addMachOCodeSegment(data, macUIOptionalCode, code)
	if err != nil {
		return nil, err
	}
	if options.retinaText {
		if err := reserveMachORetinaFrameState(patched); err != nil {
			return nil, err
		}
	}
	if err := applyMachOInstructionPatches(patched, patches); err != nil {
		return nil, err
	}
	return patched, nil
}

func applyMachOInstructionPatches(data []byte, patches []instructionPatch) error {
	for _, patch := range patches {
		end := patch.rva + len(patch.original)
		if patch.rva < 0 || end > len(data) {
			return fmt.Errorf("invalid Mach-O patch extent at %#x", patch.rva)
		}
		target := data[patch.rva:end]
		if len(target) != len(patch.replacement) || !bytes.Equal(target, patch.original) {
			return fmt.Errorf("unexpected Mach-O instruction at %#x", patch.rva)
		}
		copy(target, patch.replacement)
	}
	return nil
}

func appendMachOLoadingPreview(code []byte, patches []instructionPatch) ([]byte, []instructionPatch) {
	const (
		readThumbs = 0x1d2cf3
		gzcom      = 0x03f2d4
	)
	start := len(code)
	stub := macUIOptionalCode + start
	expander := stub + 0x40

	// Save both output arguments and keep SysV stack alignment. Forward all
	// arguments to the stock reader, including the two unused output slots.
	code = append(code, hexBytes("56524883ec08")...) // Save rsi and rdx, then align the stack.
	code = append(code, relativeInstruction(0xe8, macUIOptionalCode+len(code), readThumbs)...)
	// Skip expansion when the reader returned false. The caller will draw its
	// stock fallback. On success, reload the saved color and coverage slots.
	code = append(code, hexBytes("84c0")...)
	loadFailed := len(code)
	code = append(code, 0x74, 0)
	code = append(code, hexBytes("488b7c2410488b742408")...)
	code = append(code, 0x48, 0x8d)
	code = append(code, relativeInstruction(0x15, macUIOptionalCode+len(code), gzcom)...) // lea rdx, [rip+GZCOM]
	code = append(code, relativeInstruction(0xe8, macUIOptionalCode+len(code), expander)...)
	code = append(code, hexBytes("b001")...) // Return true after a successful read.
	code[loadFailed+1] = byte(len(code) - loadFailed - 2)
	code = append(code, hexBytes("4883c418c3")...) // Restore the stack and return.
	code = append(code, make([]byte, expander-macUIOptionalCode-len(code))...)
	code = append(code, macLoadingPreviewCode...)

	const site = macLoadingPreviewSite
	patches = append(patches, instructionPatch{site,
		relativeInstruction(0xe8, site, readThumbs), relativeInstruction(0xe8, site, stub)})
	return code, patches
}

// addMachOCodeSegment inserts executable pages before __LINKEDIT in the fingerprinted
// Aspyr executable. Sections and dyld segment indices before it stay in place.
func addMachOCodeSegment(data []byte, address int, code []byte) ([]byte, error) {
	const (
		imageBase   = 0x100000000
		pageSize    = 0x1000
		commandSize = 72
		headerSize  = 32
	)
	file, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	link := file.Segment("__LINKEDIT")
	segmentSize := (len(code) + pageSize - 1) &^ (pageSize - 1)
	headerEnd := headerSize + int(little.Uint32(data[20:]))
	if link == nil || link.Addr != imageBase+uint64(address) || segmentSize == 0 ||
		link.Offset+link.Filesz != uint64(len(data)) || headerEnd+commandSize > int(link.Offset) ||
		!bytes.Equal(data[headerEnd:headerEnd+commandSize], make([]byte, commandSize)) {
		return nil, fmt.Errorf("missing Mach-O space for the executable patch segment")
	}
	linkOffset := int(link.Offset)
	patched := make([]byte, len(data)+segmentSize)
	copy(patched, data[:linkOffset])
	copy(patched[linkOffset:], code)
	copy(patched[linkOffset+segmentSize:], data[linkOffset:])

	var insertion int
	offset := headerSize
	for _, load := range file.Loads {
		raw := patched[offset : offset+len(load.Raw())]
		var fields []int
		switch little.Uint32(raw) {
		case 0x19: // LC_SEGMENT_64
			if load == link {
				insertion = offset
				little.PutUint64(raw[24:], link.Addr+uint64(segmentSize))
				little.PutUint64(raw[40:], link.Offset+uint64(segmentSize))
			}
		case 0x80000022: // LC_DYLD_INFO_ONLY
			fields = []int{8, 16, 24, 32, 40}
		case 0x02: // LC_SYMTAB
			fields = []int{8, 16}
		case 0x0b: // LC_DYSYMTAB
			fields = []int{32, 40, 48, 56, 64, 72}
		case 0x1d, 0x26, 0x29: // LC_CODE_SIGNATURE, LC_FUNCTION_STARTS, LC_DATA_IN_CODE
			fields = []int{8}
		}
		for _, field := range fields {
			value := little.Uint32(raw[field:])
			if value == 0 {
				continue
			}
			if uint64(value) < link.Offset || uint64(value) >= uint64(len(data)) {
				return nil, fmt.Errorf("unexpected Mach-O link-edit offset %#x", value)
			}
			little.PutUint32(raw[field:], value+uint32(segmentSize))
		}
		offset += len(raw)
	}
	if insertion == 0 || offset != headerEnd {
		return nil, fmt.Errorf("unexpected Mach-O load commands")
	}
	copy(patched[insertion+commandSize:], patched[insertion:headerEnd])
	command := patched[insertion : insertion+commandSize]
	clear(command)
	little.PutUint32(command[0:], 0x19) // LC_SEGMENT_64
	little.PutUint32(command[4:], commandSize)
	copy(command[8:24], "__SC4UHD")
	little.PutUint64(command[24:], link.Addr)
	little.PutUint64(command[32:], uint64(segmentSize))
	little.PutUint64(command[40:], link.Offset)
	little.PutUint64(command[48:], uint64(segmentSize))
	little.PutUint32(command[56:], 5) // read and execute
	little.PutUint32(command[60:], 5)
	little.PutUint32(patched[16:], little.Uint32(data[16:])+1)
	little.PutUint32(patched[20:], little.Uint32(data[20:])+commandSize)
	return patched, nil
}
