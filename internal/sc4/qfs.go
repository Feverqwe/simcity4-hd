package sc4

import "errors"

func decodeQFS(data []byte) ([]byte, error) {
	if len(data) < 9 || data[4] != 0x10 || data[5] != 0xfb {
		return nil, errors.New("invalid QFS header")
	}
	length := int(data[6])<<16 | int(data[7])<<8 | int(data[8])
	out := make([]byte, 0, length)
	input := data[9:]
	for len(input) > 0 {
		command := int(input[0])
		input = input[1:]
		literals, count, distance := 0, 0, 0
		switch {
		case command >= 252:
			literals = command & 3
		case command >= 224:
			literals = (command - 223) * 4
		default:
			extra := 1
			if command >= 192 {
				extra = 3
			} else if command >= 128 {
				extra = 2
			}
			if len(input) < extra {
				return nil, errors.New("truncated QFS command")
			}
			a := int(input[0])
			switch extra {
			case 3:
				literals = command & 3
				count = ((command & 12) << 6) + int(input[2]) + 5
				distance = ((command & 16) << 12) + (a << 8) + int(input[1]) + 1
			case 2:
				literals = a >> 6
				count = (command & 63) + 4
				distance = ((a & 63) << 8) + int(input[1]) + 1
			case 1:
				literals = command & 3
				count = ((command & 28) >> 2) + 3
				distance = ((command & 96) << 3) + a + 1
			}
			input = input[extra:]
		}
		if literals > len(input) || literals+count > length-len(out) {
			return nil, errors.New("QFS command exceeds input or output extent")
		}
		out = append(out, input[:literals]...)
		input = input[literals:]
		if distance > len(out) {
			return nil, errors.New("invalid QFS backreference")
		}
		for range count {
			out = append(out, out[len(out)-distance])
		}
		if command >= 252 {
			break
		}
	}
	if len(out) != length {
		return nil, errors.New("QFS decoded size does not match the header")
	}
	return out, nil
}
