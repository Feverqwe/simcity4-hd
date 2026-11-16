package x86

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
)

type Options struct {
	HdCursors         bool
	LoadingPreview    bool
	RegionMap         bool
	RussianBudgetTips bool
	RetinaText        bool
	CursorScale       int
}

type Patch struct {
	RVA                   int
	Original, Replacement []byte
}

func Instruction(rva int, original, replacement string) Patch {
	return Patch{rva, HexBytes(original), HexBytes(replacement)}
}

func HexBytes(encoded string) []byte {
	data, err := hex.DecodeString(encoded)
	if err != nil {
		panic(err)
	}
	return data
}

func RelativeInstruction(opcode byte, site, target int) []byte {
	return binary.LittleEndian.AppendUint32([]byte{opcode}, uint32(target-site-5))
}

func AppendStub(stub []byte, base, site int, original, body []byte) ([]byte, Patch) {
	address := base + len(stub)
	redirect := RelativeInstruction(0xe9, site, address)
	redirect = append(redirect, bytes.Repeat([]byte{0x90}, len(original)-len(redirect))...)
	stub = append(stub, body...)
	stub = append(stub, RelativeInstruction(0xe9, base+len(stub), site+len(original))...)
	return stub, Patch{site, original, redirect}
}
