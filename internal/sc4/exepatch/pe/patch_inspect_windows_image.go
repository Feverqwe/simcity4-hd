package pe

import (
	"bytes"
	"debug/pe"
	"errors"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
)

// Inspect the patch sites used by installers that wrote only an executable hash.
func ReadOptions(executable []byte, cursors bool) (x86.Options, error) {
	file, err := pe.NewFile(bytes.NewReader(executable))
	if err != nil {
		return x86.Options{}, err
	}
	defer file.Close()
	text := file.Section(".text")
	if text == nil {
		return x86.Options{}, errors.New("the installed executable is missing its code section")
	}
	data, err := text.Data()
	if err != nil {
		return x86.Options{}, err
	}
	at := func(rva uint32, length int) []byte {
		if rva < text.VirtualAddress {
			return nil
		}
		offset := int(rva - text.VirtualAddress)
		if offset > len(data)-length {
			return nil
		}
		return data[offset : offset+length]
	}
	preview := at(0x377149, 5)
	return x86.Options{
		HdCursors: cursors,
		RegionMap: bytes.Equal(at(0x3ae6ae, 1), []byte{0xe9}),
		LoadingPreview: len(preview) == 5 && preview[0] == 0xe8 &&
			!bytes.Equal(preview, x86.RelativeInstruction(0xe8, 0x377149, 0x1dda40)),
	}, nil
}
