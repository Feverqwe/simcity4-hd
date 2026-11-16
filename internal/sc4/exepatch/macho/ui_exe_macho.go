package macho

import (
	"bytes"
	"crypto/sha256"
	"debug/macho"
	"encoding/binary"
	"fmt"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/uiids"
)

const StockHash = "0d7a18ffee1d2abbdc5f58d364a7ef515b093fb7f85fa09840f8f022a8874224"

// Assemble internal/sc4/patchsrc/source/ui_density_macho.S for x86_64-linux-gnu and extract
// its relocation-free .text with llvm-objcopy -O binary -j .text.
//

const macLaunchStub = 0x1a80
const macDisplayModeStub = 0x1f00

func PatchUI(data []byte, textureScale int) ([]byte, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	if hash != StockHash {
		return nil, fmt.Errorf("expected the original Steam Aspyr 1.2.1 (185949) executable (SHA-256 %s), got %s", StockHash, hash)
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
	uiPatches := []x86.Patch{
		x86.Instruction(0x75e517, "bab006a046", "bac5404ae7"), // Region signpost.
		x86.Instruction(0x786942, "bab006a046", "bac5404ae7"), // City initialization.
		x86.Instruction(0x7880a4, "bab006a046", "bac5404ae7"), // City refresh.
		// The stock trend windows are resized in screen pixels.
		x86.Instruction(0x78838d, "8d34dd0000000029de", "6bf30e909090909090"),
		x86.Instruction(0x788632, "448d3cdd000000004129df", "446bfb0e90909090909090"),
		x86.Instruction(0x788666, "418d743715", "418d74372a"),
		x86.Instruction(0x786baa, "c7460880550114", "c74608ac60d22b"),
		x86.Instruction(0x786bce, "c7460884550114", "c74608ab60d22b"),
		x86.Instruction(0x3d7add, "bab006a046", "bac7404ae7"),
		x86.Instruction(0x3d7b3c, "bab006a046", "bac7404ae7"),
		x86.Instruction(0x3d7bcb, "bab006a046", "bac7404ae7"),
		// Query Help is bottom-aligned, with a separate X offset for ultrawide views.
		x86.Instruction(0x78940a, "83c705", "83c70a"),
		x86.Instruction(0x789410, "be05000000", "be0a000000"),
		x86.Instruction(0x789418, "8d530b", "8d5316"),
		// Restoring a collapsed HUD uses the projected button and doubled offsets.
		x86.Instruction(0x7894b2, "48b8acdb6d85b006a046", "48b8acdb6d85bd404ae7"),
		x86.Instruction(0x7896bd, "83c0e4be0c000000", "83c0c8be18000000"),
		x86.Instruction(0x78bb9b, "41b9a264258a", "41b9c3404ae7"),
		// Catalog and Disasters create their menus in code and consume
		// the projected 88-pixel cells from the same DATs as Windows.
		x86.Instruction(0x78d20d, "be2c000000ba2c000000b905000000", "be58000000ba58000000b90a000000"),
		x86.Instruction(0x78d7ac, "be2c000000ba2c000000b905000000", "be58000000ba58000000b90a000000"),
		x86.Instruction(0x78d473, "c74424101d000000c74424081b000000c7042404000000", "c74424103a000000c744240836000000c7042408000000"),
		x86.Instruction(0x78d48a, "ba35000000b91900000041b85000000041b935000000", "ba6a000000b93200000041b8a000000041b96a000000"),
		x86.Instruction(0x78d4eb, "83c7f6", "83c7ec"),
		x86.Instruction(0x78d4f1, "41b90a000000", "41b914000000"),
		x86.Instruction(0x78d9ce, "c744241022000000c744240828000000c7042406000000", "c744241044000000c744240850000000c704240c000000"),
		x86.Instruction(0x78d9e5, "ba35000000b91900000041b85e00000041b93e000000", "ba6a000000b93200000041b8bc00000041b97c000000"),
		x86.Instruction(0x78da46, "4183c5f6", "4183c5ec"),
		x86.Instruction(0x78da4e, "41b90a000000", "41b914000000"),
		x86.Instruction(0x316e10, "ba88f8064c", "bac0404ae7"), // My Sims vehicle icons.
		x86.Instruction(0x31b179, "ba88f8064c", "bac0404ae7"), // My Sims transport selection.
		// Aspyr constructs the pager headline and both News body styles
		// in code. Match the HTML font level used by the Windows UHD UI.
		x86.Instruction(0x31e886, "48b95a453d333e3c423e", "48b95a453d353e3c423e"),
		x86.Instruction(0x31e9a9, "48b9222053495a453d33", "48b9222053495a453d35"),
		x86.Instruction(0x31ea4e, "48b9222053495a453d33", "48b9222053495a453d35"),
		// Advisor messages construct their own HTML heading and both body styles.
		x86.Instruction(0x2b78a2, "48b95a453d333e3c423e", "48b95a453d353e3c423e"),
		x86.Instruction(0x2b7977, "48b9222053495a453d33", "48b9222053495a453d35"),
		x86.Instruction(0x2b7a61, "48b9222053495a453d33", "48b9222053495a453d35"),
		// Mission progress resizes its panel for active and idle text.
		x86.Instruction(0x787d49, "83c008", "83c010"),
		x86.Instruction(0x787d6c, "83c008", "83c010"),
		// Route Query uses the mission panel's upper-right anchor.
		x86.Instruction(0x792d36, "418d7424f8", "418d7424f0"),
		x86.Instruction(0x792d3d, "ba08000000", "ba10000000"),
		// Saved dialog positions need a visible edge or a fallback position.
		x86.Instruction(0x2a69df, "4183c4f8", "4183c4f0"),
		x86.Instruction(0x2a69ea, "83fb08", "83fb10"),
		x86.Instruction(0x2a69f2, "4183c7f8", "4183c7f0"),
		x86.Instruction(0x2a69fd, "83f807", "83f80f"),
		x86.Instruction(0x2a6a19, "be08000000ba08000000", "be10000000ba10000000"),
		// The tutorial exit dialog overrides the position from its layout.
		x86.Instruction(0x0b7d75, "be32000000ba78000000", "be64000000baf0000000"),
		// Callout title and body wrap against independent screen-pixel limits.
		x86.Instruction(0x3c6b7f, "41b9fa000000", "41b9f4010000"),
		x86.Instruction(0x3c6c2f, "41b9fa000000", "41b9f4010000"),
		// Building Style Control positions the budget and finance panes after
		// reading their neighbors' scaled bottom edges.
		x86.Instruction(0x7594dd, "83c002", "83c004"),
		x86.Instruction(0x75950b, "83c008", "83c010"),
	}
	// The advisor, ticker and newspaper each build the same set of marker URLs
	// with two adjacent immediate values for the group and the start of the ID.
	stockGroup := fmt.Sprintf("%08x", uiids.CatalogFrameGroup)
	markerGroup := fmt.Sprintf("%08x", uiids.NewsMarkerGroup)
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
		uiPatches = append(uiPatches, x86.Instruction(table.first, stockMarker, hdMarker),
			x86.Instruction(table.last, stockMarker, hdMarker))
		for i := 0; i < uiids.NewsMarkerCount-2; i++ {
			site := table.second + i*table.stride
			uiPatches = append(uiPatches, x86.Instruction(site, stockMarker, hdMarker))
		}
	}
	patched := bytes.Clone(data)
	if err := placeMachOCode(patched, int(text.Offset), macLaunchStub, patchsrc.MacLaunchCode); err != nil {
		return nil, err
	}
	if err := placeMachOCode(patched, int(text.Offset), macDisplayModeStub, patchsrc.MacDisplayModeCode); err != nil {
		return nil, err
	}
	patches := []x86.Patch{
		{RVA: 0x3045, Original: x86.HexBytes("e8e6140100"), Replacement: x86.RelativeInstruction(0xe8, 0x3045, macLaunchStub)},
		{RVA: 0x3972c, Original: x86.HexBytes("554889e54157"), Replacement: append(x86.RelativeInstruction(0xe9, 0x3972c, macLaunchStub+0x80), 0x90)},
		{RVA: 0x63f96c, Original: x86.HexBytes("e83f6d2900"), Replacement: x86.RelativeInstruction(0xe8, 0x63f96c, macLaunchStub+0x100)},
		{RVA: 0x60cf03, Original: x86.HexBytes("418b8628020000"), Replacement: append(x86.RelativeInstruction(0xe9, 0x60cf03, macDisplayModeStub), 0x90, 0x90)},
		{RVA: 0x5edc, Original: x86.HexBytes("e8d1210000"), Replacement: x86.RelativeInstruction(0xe8, 0x5edc, macDisplayModeStub+0x60)},
	}
	if textureScale != 0 {
		patches = append(patches, uiPatches...)
		const stub = 0x1c00
		if err := placeMachOCode(patched, macDisplayModeStub, stub, patchsrc.MacKeyboardHelpCode); err != nil {
			return nil, err
		}
		patches = append(patches,
			x86.Patch{RVA: 0x3a32b8, Original: x86.HexBytes("498b3e488b07"), Replacement: append(x86.RelativeInstruction(0xe9, 0x3a32b8, stub), 0x90)},
			x86.Patch{RVA: 0x3a3518, Original: x86.HexBytes("41833f03752e"), Replacement: append(x86.RelativeInstruction(0xe9, 0x3a3518, stub+0x80), 0x90)},
		)
	}
	if textureScale == 4 {
		const stub = 0x1600
		if err := placeMachOCode(patched, macLaunchStub, stub, patchsrc.MacDensityCode); err != nil {
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
			original := x86.HexBytes(hook.original)
			redirect := x86.RelativeInstruction(hook.op, hook.site, stub+hook.entry)
			redirect = append(redirect, bytes.Repeat([]byte{0x90}, len(original)-len(redirect))...)
			patches = append(patches, x86.Patch{RVA: hook.site, Original: original, Replacement: redirect})
		}
	}
	for _, patch := range patches {
		const imageBase = 0x100000000
		offset := uint64(patch.RVA) + imageBase - text.Addr
		end := offset + uint64(len(patch.Original))
		if offset >= text.Size || end > text.Size || len(patch.Original) != len(patch.Replacement) {
			return nil, fmt.Errorf("invalid Mach-O patch extent at %#x", patch.RVA)
		}
		target := patched[uint64(text.Offset)+offset : uint64(text.Offset)+end]
		if !bytes.Equal(target, patch.Original) {
			return nil, fmt.Errorf("unexpected Mach-O instruction at %#x", patch.RVA)
		}
		copy(target, patch.Replacement)
	}
	return patched, nil
}

func placeMachOCode(data []byte, limit, start int, code []byte) error {
	headerEnd := 32 + int(binary.LittleEndian.Uint32(data[20:]))
	end := start + len(code)
	if headerEnd > start || end > limit || !bytes.Equal(data[start:end], make([]byte, len(code))) {
		return fmt.Errorf("missing Mach-O executable padding at %#x", start)
	}
	copy(data[start:end], code)
	return nil
}
