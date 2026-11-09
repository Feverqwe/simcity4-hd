package sc4

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const stockEXEHash = "f9b059d29940d1a28c89e6d6a7ab147e9762cd03134cdbaf02ba0e0142e1a176"

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

// catalogBlitBody doubles the source rectangle on the stack while reusing the
// same destination and clip flags. The game keeps its existing button bounds.
func catalogBlitBody(original []byte) []byte {
	dispatch := byte(0x78)
	if original[1] == 0x52 {
		dispatch = 0x7a
	}
	body := []byte{0x57, 0x8b, dispatch, 0x74}
	body = append(body, hexBytes("538bdc83ec108b430c")...)
	for offset := byte(0); offset < 16; offset += 4 {
		body = append(body, 0x8b, 0x50, offset, 0xd1, 0xe2, 0x89, 0x54, 0x24, offset)
	}
	body = append(body, hexBytes("ff73148d442404ff731050ff7308ffd783c4105b5f8d642410")...)
	return append(body, original[3:]...)
}

func patchUIExecutable(data []byte) error {
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	if hash != stockEXEHash {
		return fmt.Errorf("expected the original Steam Deluxe 1.1.641.0 executable (SHA-256 %s), got %s", stockEXEHash, hash)
	}

	// The fingerprint pins the PE layout as well as the x86 instructions.
	pe := int(little.Uint32(data[0x3c:]))
	text := pe + 24 + int(little.Uint16(data[pe+20:]))
	rdata := text + 40
	if string(bytes.TrimRight(data[text:text+8], "\x00")) != ".text" ||
		string(bytes.TrimRight(data[rdata:rdata+8], "\x00")) != ".rdata" {
		return errors.New("expected .text and .rdata sections")
	}
	virtualSize := int(little.Uint32(data[text+8:]))
	rva := int(little.Uint32(data[text+12:]))
	rawSize := int(little.Uint32(data[text+16:]))
	rawStart := int(little.Uint32(data[text+20:]))
	stubRVA := (rva + virtualSize + 15) &^ 15
	stubStart := rawStart + stubRVA - rva

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
		instruction(0x3e73d8, "8d5c03ff", "8d5c032f"),
		instruction(0x3f4d19, "68a264258a", "68c3404ae7"),
		// Runtime HUD controls use image IDs and sizes outside the layouts.
		instruction(0x3ee037, "c7442440b006a046c744244488452453", "c7442440bd404ae7c744244488452453"),
		instruction(0x3ee15a, "83e81c506a0c", "83e838506a18"),
		instruction(0x3ed4f3, "c744243884550114", "c7442438ab60d22b"),
		instruction(0x3bf2db, "83c303", "83c306"),
		instruction(0x37019c, "6888f8064c", "68c0404ae7"),
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
	for _, site := range []struct {
		rva  int
		code string
	}{
		{0x39abb5, "ff50748b4e68"},
		{0x39b2c0, "ff50748b8ee0000000"},
		{0x39b3de, "ff50748b8600010000"},
		{0x39b42b, "ff52748b3f"},
	} {
		original := hexBytes(site.code)
		var patch instructionPatch
		stub, patch = appendStub(stub, stubRVA, site.rva, original, catalogBlitBody(original))
		patches = append(patches, patch)
	}
	stubEnd := stubStart + len(stub)
	if stubEnd > rawStart+rawSize || !bytes.Equal(data[stubStart:stubEnd], make([]byte, len(stub))) {
		return errors.New("no empty .text padding for the UI stubs")
	}
	for _, patch := range patches {
		offset := rawStart + patch.rva - rva
		end := offset + len(patch.original)
		if offset < rawStart || end > rawStart+virtualSize || len(patch.original) != len(patch.replacement) ||
			!bytes.Equal(data[offset:end], patch.original) {
			return fmt.Errorf("unexpected UI instructions at RVA %#x", patch.rva)
		}
		copy(data[offset:end], patch.replacement)
	}
	for _, template := range []struct {
		offset int
		text   string
		size   byte
	}{
		{0x683820, `<FONT COLOR="#8e9499" FACE="Arta" SIZE=3><I>`, '6'},
		{0x683850, `<FONT COLOR="#3f4967" FACE="Arta" SIZE=3><I>`, '6'},
		{0x683880, ` FACE="StocletITC TT" SIZE=3><B>`, '5'},
	} {
		start := int(little.Uint32(data[rdata+20:]))
		end := template.offset + len(template.text)
		if template.offset < start || end > start+int(little.Uint32(data[rdata+16:])) ||
			string(data[template.offset:end]) != template.text {
			return fmt.Errorf("unexpected news HTML template at offset %#x", template.offset)
		}
		offset := bytes.Index([]byte(template.text), []byte("SIZE=3")) + len("SIZE=")
		data[template.offset+offset] = template.size
	}

	copy(data[stubStart:stubEnd], stub)
	little.PutUint32(data[text+8:], uint32(stubRVA-rva+len(stub)))
	little.PutUint16(data[pe+22:], little.Uint16(data[pe+22:])|0x20)
	little.PutUint32(data[pe+88:], peChecksum(data, pe+88))
	return nil
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
	if len(args) != 2 {
		return errors.New("usage: simcity4-hd patch-ui-exe GAME_DIR OUTPUT.exe")
	}
	data, err := os.ReadFile(filepath.Join(args[0], "Apps", "SimCity 4.exe"))
	if err != nil {
		return err
	}
	if err := patchUIExecutable(data); err != nil {
		return err
	}
	return publishFile(args[1], func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}
