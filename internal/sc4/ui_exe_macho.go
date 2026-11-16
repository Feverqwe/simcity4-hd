package sc4

import (
	"bytes"
	"crypto/sha256"
	"debug/macho"
	_ "embed"
	"fmt"
)

const stockMachOHash = "0d7a18ffee1d2abbdc5f58d364a7ef515b093fb7f85fa09840f8f022a8874224"

// Assemble patchsrc/ui_density_macho.S for x86_64-linux-gnu and extract
// its relocation-free .text with llvm-objcopy -O binary -j .text.
//
//go:embed patchsrc/ui_density_macho.x86_64
var macDensityCode []byte

//go:embed patchsrc/launch_macho.x86_64
var macLaunchCode []byte

const macLaunchStub = 0x1a80
const macDisplayModeStub = 0x1f00

//go:embed patchsrc/display_mode_macho.x86_64
var macDisplayModeCode []byte

//go:embed patchsrc/keyboard_help_macho.x86_64
var macKeyboardHelpCode []byte

func patchMachOUIExecutable(data []byte, textureScale int) ([]byte, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	if hash != stockMachOHash {
		return nil, fmt.Errorf("expected the original Steam Aspyr 1.2.1 (185949) executable (SHA-256 %s), got %s", stockMachOHash, hash)
	}
	file, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	text := file.Section("__text")
	if file.Cpu != macho.CpuAmd64 || text == nil || text.Seg != "__TEXT" {
		return nil, fmt.Errorf("expected an x86-64 executable with a __TEXT,__text section")
	}
	// Aspyr's three rating paths use the same score rows as the Windows build.
	// The replacement atlases double their width while preserving those rows.
	uiPatches := []instructionPatch{
		instruction(0x75e517, "bab006a046", "bac5404ae7"), // Region signpost.
		instruction(0x786942, "bab006a046", "bac5404ae7"), // City initialization.
		instruction(0x7880a4, "bab006a046", "bac5404ae7"), // City refresh.
		// The stock trend windows are resized in screen pixels.
		instruction(0x78838d, "8d34dd0000000029de", "6bf30e909090909090"),
		instruction(0x788632, "448d3cdd000000004129df", "446bfb0e90909090909090"),
		instruction(0x788666, "418d743715", "418d74372a"),
		instruction(0x786baa, "c7460880550114", "c74608ac60d22b"),
		instruction(0x786bce, "c7460884550114", "c74608ab60d22b"),
		instruction(0x3d7add, "bab006a046", "bac7404ae7"),
		instruction(0x3d7b3c, "bab006a046", "bac7404ae7"),
		instruction(0x3d7bcb, "bab006a046", "bac7404ae7"),
		// Query Help is bottom-aligned, with a separate X offset for ultrawide views.
		instruction(0x78940a, "83c705", "83c70a"),
		instruction(0x789410, "be05000000", "be0a000000"),
		instruction(0x789418, "8d530b", "8d5316"),
		// Restoring a collapsed HUD uses the projected button and doubled offsets.
		instruction(0x7894b2, "48b8acdb6d85b006a046", "48b8acdb6d85bd404ae7"),
		instruction(0x7896bd, "83c0e4be0c000000", "83c0c8be18000000"),
		instruction(0x78bb9b, "41b9a264258a", "41b9c3404ae7"),
		// Catalog and Disasters create their menus in code and consume
		// the projected 88-pixel cells from the same DATs as Windows.
		instruction(0x78d20d, "be2c000000ba2c000000b905000000", "be58000000ba58000000b90a000000"),
		instruction(0x78d7ac, "be2c000000ba2c000000b905000000", "be58000000ba58000000b90a000000"),
		instruction(0x78d473, "c74424101d000000c74424081b000000c7042404000000", "c74424103a000000c744240836000000c7042408000000"),
		instruction(0x78d48a, "ba35000000b91900000041b85000000041b935000000", "ba6a000000b93200000041b8a000000041b96a000000"),
		instruction(0x78d4eb, "83c7f6", "83c7ec"),
		instruction(0x78d4f1, "41b90a000000", "41b914000000"),
		instruction(0x78d9ce, "c744241022000000c744240828000000c7042406000000", "c744241044000000c744240850000000c704240c000000"),
		instruction(0x78d9e5, "ba35000000b91900000041b85e00000041b93e000000", "ba6a000000b93200000041b8bc00000041b97c000000"),
		instruction(0x78da46, "4183c5f6", "4183c5ec"),
		instruction(0x78da4e, "41b90a000000", "41b914000000"),
		instruction(0x316e10, "ba88f8064c", "bac0404ae7"), // My Sims vehicle icons.
		instruction(0x31b179, "ba88f8064c", "bac0404ae7"), // My Sims transport selection.
		// Aspyr constructs the pager headline and both News body styles
		// in code. Match the HTML font level used by the Windows UHD UI.
		instruction(0x31e886, "48b95a453d333e3c423e", "48b95a453d353e3c423e"),
		instruction(0x31e9a9, "48b9222053495a453d33", "48b9222053495a453d35"),
		instruction(0x31ea4e, "48b9222053495a453d33", "48b9222053495a453d35"),
		// Advisor messages construct their own HTML heading and both body styles.
		instruction(0x2b78a2, "48b95a453d333e3c423e", "48b95a453d353e3c423e"),
		instruction(0x2b7977, "48b9222053495a453d33", "48b9222053495a453d35"),
		instruction(0x2b7a61, "48b9222053495a453d33", "48b9222053495a453d35"),
		// Mission progress resizes its panel for active and idle text.
		instruction(0x787d49, "83c008", "83c010"),
		instruction(0x787d6c, "83c008", "83c010"),
		// Route Query uses the mission panel's upper-right anchor.
		instruction(0x792d36, "418d7424f8", "418d7424f0"),
		instruction(0x792d3d, "ba08000000", "ba10000000"),
		// Saved dialog positions need a visible edge or a fallback position.
		instruction(0x2a69df, "4183c4f8", "4183c4f0"),
		instruction(0x2a69ea, "83fb08", "83fb10"),
		instruction(0x2a69f2, "4183c7f8", "4183c7f0"),
		instruction(0x2a69fd, "83f807", "83f80f"),
		instruction(0x2a6a19, "be08000000ba08000000", "be10000000ba10000000"),
		// The tutorial exit dialog overrides the position from its layout.
		instruction(0x0b7d75, "be32000000ba78000000", "be64000000baf0000000"),
		// Callout title and body wrap against independent screen-pixel limits.
		instruction(0x3c6b7f, "41b9fa000000", "41b9f4010000"),
		instruction(0x3c6c2f, "41b9fa000000", "41b9f4010000"),
		// Building Style Control positions the budget and finance panes after
		// reading their neighbors' scaled bottom edges.
		instruction(0x7594dd, "83c002", "83c004"),
		instruction(0x75950b, "83c008", "83c010"),
	}
	// The advisor, ticker and newspaper each build the same set of marker URLs
	// with two adjacent immediate values for the group and the start of the ID.
	stockGroup := fmt.Sprintf("%08x", catalogFrameGroup)
	markerGroup := fmt.Sprintf("%08x", newsMarkerGroup)
	const markerTemplate = "48b9%x4889481848b9%x"
	stockMarker := fmt.Sprintf(markerTemplate, stockGroup[3:]+"/14", "HTML/"+stockGroup[:3])
	hdMarker := fmt.Sprintf(markerTemplate, markerGroup[3:]+"/14", "HTML/"+markerGroup[:3])
	for _, table := range []struct {
		first, second, stride, last int
	}{
		{0x2b7b59, 0x2b7c3c, 0xe3, 0x2b88a6}, // Advisor.
		{0x31979b, 0x31987e, 0xe3, 0x31a4e8}, // Ticker.
		{0x31eaf2, 0x31eb8f, 0x93, 0x31f3a0}, // Newspaper.
	} {
		uiPatches = append(uiPatches, instruction(table.first, stockMarker, hdMarker),
			instruction(table.last, stockMarker, hdMarker))
		for i := 0; i < newsMarkerCount-2; i++ {
			site := table.second + i*table.stride
			uiPatches = append(uiPatches, instruction(site, stockMarker, hdMarker))
		}
	}
	patched := bytes.Clone(data)
	if err := placeMachOCode(patched, int(text.Offset), macLaunchStub, macLaunchCode); err != nil {
		return nil, err
	}
	if err := placeMachOCode(patched, int(text.Offset), macDisplayModeStub, macDisplayModeCode); err != nil {
		return nil, err
	}
	patches := []instructionPatch{
		{0x3045, hexBytes("e8e6140100"), relativeInstruction(0xe8, 0x3045, macLaunchStub)},
		{0x3972c, hexBytes("554889e54157"), append(relativeInstruction(0xe9, 0x3972c, macLaunchStub+0x80), 0x90)},
		{0x63f96c, hexBytes("e83f6d2900"), relativeInstruction(0xe8, 0x63f96c, macLaunchStub+0x100)},
		{0x60cf03, hexBytes("418b8628020000"), append(relativeInstruction(0xe9, 0x60cf03, macDisplayModeStub), 0x90, 0x90)},
		{0x5edc, hexBytes("e8d1210000"), relativeInstruction(0xe8, 0x5edc, macDisplayModeStub+0x60)},
	}
	if textureScale != 0 {
		patches = append(patches, uiPatches...)
		const stub = 0x1c00
		if err := placeMachOCode(patched, macDisplayModeStub, stub, macKeyboardHelpCode); err != nil {
			return nil, err
		}
		patches = append(patches,
			instructionPatch{0x3a32b8, hexBytes("498b3e488b07"), append(relativeInstruction(0xe9, 0x3a32b8, stub), 0x90)},
			instructionPatch{0x3a3518, hexBytes("41833f03752e"), append(relativeInstruction(0xe9, 0x3a3518, stub+0x80), 0x90)},
		)
	}
	if textureScale == 4 {
		const stub = 0x1600
		if err := placeMachOCode(patched, macLaunchStub, stub, macDensityCode); err != nil {
			return nil, err
		}
		for _, hook := range []struct {
			site, entry int
			op          byte
			original    string
		}{
			{0x53faef, 0x00, 0xe9, "4889daff5060"},
			{0x540472, 0x50, 0xe9, "4889deff5020"},
			{0x5404a3, 0x90, 0xe9, "4489fe89c241ffd5"},
			{0x54051c, 0xc0, 0xe9, "488d7590ff5040"},
			{0x330851, 0xf0, 0xe9, "488b08488b4008"},
			{0x281180, 0x130, 0xe9, "488b87a8000000488bb7f80100004c8b00488d55f04889c741ff9030010000"},
			{0x2811fe, 0x180, 0xe9, "41b8010000004889c7"},
			{0x3308c1, 0x1b0, 0xe9, "41b8010000004889d9"},
			{0x330950, 0x1e0, 0xe9, "488d4db8ff9030010000"},
			{0x316e36, 0x400, 0xe9, "488b7db8488b07ff5020"},
			{0x31b1ce, 0x420, 0xe9, "488b7de8488b07ff5020"},
			{0x33071a, 0x440, 0xe8, "488b07ff5058"},
			{0x33072b, 0x440, 0xe8, "488b07ff5058"},
			{0x330746, 0x450, 0xe8, "488b07ff5060"},
			{0x330757, 0x450, 0xe8, "488b07ff5060"},
			{0x6a366c, 0x220, 0xe8, "ff9030010000"},
			{0x6a36e8, 0x220, 0xe8, "ff9030010000"},
			{0x6a3840, 0x220, 0xe8, "ff9030010000"},
			{0x6a38b9, 0x220, 0xe8, "ff9030010000"},
			{0x6a36b4, 0x240, 0xe8, "ff9038010000"},
			{0x6a3751, 0x240, 0xe8, "ff9038010000"},
			{0x6a37a4, 0x240, 0xe8, "ff9038010000"},
			{0x6a37e8, 0x240, 0xe8, "ff9038010000"},
			{0x6a3885, 0x240, 0xe8, "ff9038010000"},
		} {
			original := hexBytes(hook.original)
			redirect := relativeInstruction(hook.op, hook.site, stub+hook.entry)
			redirect = append(redirect, bytes.Repeat([]byte{0x90}, len(original)-len(redirect))...)
			patches = append(patches, instructionPatch{hook.site, original, redirect})
		}
	}
	for _, patch := range patches {
		const imageBase = 0x100000000
		offset := uint64(patch.rva) + imageBase - text.Addr
		end := offset + uint64(len(patch.original))
		if offset >= text.Size || end > text.Size || len(patch.original) != len(patch.replacement) {
			return nil, fmt.Errorf("invalid Mach-O patch extent at %#x", patch.rva)
		}
		target := patched[uint64(text.Offset)+offset : uint64(text.Offset)+end]
		if !bytes.Equal(target, patch.original) {
			return nil, fmt.Errorf("unexpected Mach-O instruction at %#x", patch.rva)
		}
		copy(target, patch.replacement)
	}
	return patched, nil
}

func placeMachOCode(data []byte, limit, start int, code []byte) error {
	headerEnd := 32 + int(little.Uint32(data[20:]))
	end := start + len(code)
	if headerEnd > start || end > limit || !bytes.Equal(data[start:end], make([]byte, len(code))) {
		return fmt.Errorf("missing Mach-O executable padding at %#x", start)
	}
	copy(data[start:end], code)
	return nil
}
