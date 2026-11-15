package sc4

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/ulikunitz/xz"
)

const (
	advisorsPackage     = "z-UHD-Advisors.dat"
	advisorsClipLimit   = 4 << 20
	advisorsRecipeLimit = 16 << 20
	advisorsGroup       = 0xbadb57f1
	advisorsFaceGroup   = 0x1abe787d
)

//go:embed assets/advisors.zip.xz
var advisorsRecipeData []byte

var advisorFaceIDs = []uint32{
	0x51c74120, 0x51c74121, 0x51c74122, 0x51c74123,
	0x51c74124, 0x51c74125, 0x51c7412d, 0x51c74127,
}

func advisorClipIDs() []uint32 {
	ids := []uint32{0xec202410, 0xec202412}
	for advisor := range uint32(7) {
		for _, emotion := range [...]uint32{0, 2, 5, 8} {
			ids = append(ids, 0x2a42b500+16*advisor+emotion)
		}
	}
	return ids
}

func readAdvisorBytes(reader io.Reader, limit int64) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, errors.New("advisor recipe exceeds its size limit")
	}
	return content, nil
}

type advisorsRecipe struct {
	files map[string]*zip.File
}

func openAdvisorsRecipe() (advisorsRecipe, error) {
	compressed, err := (xz.ReaderConfig{SingleStream: true}).NewReader(bytes.NewReader(advisorsRecipeData))
	if err != nil {
		return advisorsRecipe{}, err
	}
	decoded, err := readAdvisorBytes(compressed, advisorsRecipeLimit)
	if err != nil {
		return advisorsRecipe{}, err
	}
	z, err := zip.NewReader(bytes.NewReader(decoded), int64(len(decoded)))
	if err != nil {
		return advisorsRecipe{}, err
	}
	expected := make(map[string]bool)
	for _, instance := range advisorClipIDs() {
		expected[fmt.Sprintf("%08x.json", instance)] = true
	}
	for _, instance := range advisorFaceIDs {
		expected[fmt.Sprintf("%08x.fsh", instance)] = true
	}
	result := advisorsRecipe{files: make(map[string]*zip.File, len(expected))}
	for _, file := range z.File {
		if !expected[file.Name] || result.files[file.Name] != nil || file.UncompressedSize64 > advisorsClipLimit {
			return advisorsRecipe{}, fmt.Errorf("unexpected advisor recipe entry %q", file.Name)
		}
		result.files[file.Name] = file
	}
	if len(result.files) != len(expected) {
		return advisorsRecipe{}, errors.New("advisor recipe is missing resources")
	}
	return result, nil
}

func (recipe advisorsRecipe) read(name string) ([]byte, error) {
	reader, err := recipe.files[name].Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return readAdvisorBytes(reader, advisorsClipLimit)
}

func (recipe advisorsRecipe) clip(instance uint32) (advisorClip, error) {
	data, err := recipe.read(fmt.Sprintf("%08x.json", instance))
	if err != nil {
		return advisorClip{}, err
	}
	var clip advisorClip
	if err := json.Unmarshal(data, &clip); err != nil {
		return advisorClip{}, err
	}
	return clip, nil
}

func (recipe advisorsRecipe) texture(instance uint32) ([]byte, error) {
	data, err := recipe.read(fmt.Sprintf("%08x.fsh", instance))
	if err != nil {
		return nil, err
	}
	elements, err := fshLayout(data)
	if err != nil {
		return nil, err
	}
	if len(elements) != 1 || int(little.Uint32(data[4:])) != len(data) || elements[0].mipCount() == 0 {
		return nil, errors.New("invalid advisor face texture")
	}
	if _, err := validateBCElement(data, elements[0], 0, true); err != nil {
		return nil, err
	}
	return data, nil
}

func (recipe advisorsRecipe) validate() error {
	for _, instance := range advisorClipIDs() {
		clip, err := recipe.clip(instance)
		if err == nil {
			err = clip.validate()
		}
		if err != nil {
			return fmt.Errorf("advisor %08x: %w", instance, err)
		}
	}
	for _, instance := range advisorFaceIDs {
		if _, err := recipe.texture(instance); err != nil {
			return fmt.Errorf("advisor texture %08x: %w", instance, err)
		}
	}
	return nil
}
