package texture

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const FSHResource = 0x7ab50e44

type FSHElement struct {
	Start, End, Stored int
	Width, Height      int
	Code               byte
	CenterX, CenterY   uint16
	OffsetX, OffsetY   uint16
}

func FSHLayout(data []byte) ([]FSHElement, error) {
	if len(data) < 16 || string(data[:4]) != "SHPI" {
		return nil, errors.New("expected an SHPI texture")
	}
	count := int(binary.LittleEndian.Uint32(data[8:]))
	if count == 0 || count > (len(data)-16)/8 {
		return nil, errors.New("invalid SHPI directory")
	}
	directoryEnd := 16 + count*8
	elements := make([]FSHElement, 0, count)
	for i := range count {
		start := int(binary.LittleEndian.Uint32(data[20+i*8:]))
		end := len(data)
		if i+1 < count {
			end = int(binary.LittleEndian.Uint32(data[28+i*8:]))
		}
		if start < directoryEnd || end > len(data) || end-start < 16 {
			return nil, fmt.Errorf("invalid SHPI element %d bounds", i)
		}
		header := data[start : start+16]
		element := FSHElement{
			Start: start, End: end,
			Stored: int(header[1]) | int(header[2])<<8 | int(header[3])<<16,
			Width:  int(binary.LittleEndian.Uint16(header[4:])), Height: int(binary.LittleEndian.Uint16(header[6:])),
			Code: header[0], CenterX: binary.LittleEndian.Uint16(header[8:]), CenterY: binary.LittleEndian.Uint16(header[10:]),
			OffsetX: binary.LittleEndian.Uint16(header[12:]), OffsetY: binary.LittleEndian.Uint16(header[14:]),
		}
		if element.Width == 0 || element.Height == 0 ||
			(element.Stored != 0 && (element.Stored < 16 || element.Stored > end-start)) {
			return nil, fmt.Errorf("invalid SHPI element %d size", i)
		}
		elements = append(elements, element)
	}
	return elements, nil
}
