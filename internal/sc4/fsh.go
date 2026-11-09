package sc4

import (
	"errors"
	"fmt"
)

const fshResource = 0x7ab50e44

type fshElement struct {
	start, end, stored int
	width, height      int
	code               byte
	centerX, centerY   uint16
	offsetX, offsetY   uint16
}

func fshLayout(data []byte) ([]fshElement, error) {
	if len(data) < 16 || string(data[:4]) != "SHPI" {
		return nil, errors.New("expected an SHPI texture")
	}
	count := int(little.Uint32(data[8:]))
	if count == 0 || count > (len(data)-16)/8 {
		return nil, errors.New("invalid SHPI directory")
	}
	directoryEnd := 16 + count*8
	elements := make([]fshElement, 0, count)
	for i := range count {
		start := int(little.Uint32(data[20+i*8:]))
		end := len(data)
		if i+1 < count {
			end = int(little.Uint32(data[28+i*8:]))
		}
		if start < directoryEnd || end > len(data) || end-start < 16 {
			return nil, fmt.Errorf("invalid SHPI element %d bounds", i)
		}
		header := data[start : start+16]
		element := fshElement{
			start: start, end: end,
			stored: int(header[1]) | int(header[2])<<8 | int(header[3])<<16,
			width:  int(little.Uint16(header[4:])), height: int(little.Uint16(header[6:])),
			code: header[0], centerX: little.Uint16(header[8:]), centerY: little.Uint16(header[10:]),
			offsetX: little.Uint16(header[12:]), offsetY: little.Uint16(header[14:]),
		}
		if element.width == 0 || element.height == 0 ||
			(element.stored != 0 && (element.stored < 16 || element.stored > end-start)) {
			return nil, fmt.Errorf("invalid SHPI element %d size", i)
		}
		elements = append(elements, element)
	}
	return elements, nil
}
