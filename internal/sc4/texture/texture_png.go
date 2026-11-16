package texture

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
)

func DecodeRGBA(data []byte) (*image.NRGBA, error) {
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if rgba, ok := source.(*image.NRGBA); ok {
		return rgba, nil
	}
	result := image.NewNRGBA(source.Bounds())
	draw.Draw(result, result.Bounds(), source, source.Bounds().Min, draw.Src)
	return result, nil
}

type rgbaPNG struct{ *image.NRGBA }

func (rgbaPNG) Opaque() bool { return false }

func EncodeRGBA(image *image.NRGBA) ([]byte, error) {
	var result bytes.Buffer
	err := png.Encode(&result, rgbaPNG{image})
	return result.Bytes(), err
}
