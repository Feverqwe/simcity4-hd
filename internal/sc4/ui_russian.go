package sc4

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var russianFontFiles = [...]string{
	"Arial MT.mxf", "Arial MT (Bold).mxf", "Arial MT (Italic).mxf", "Arial MT (Bold Italic).mxf",
	"Arta.mxf", "Arta (Italic).mxf", "Stoclet ITC TT (Bold).mxf",
	"Courier LD.mxf", "Courier LD (Bold).mxf", "Courier LD (Italic).mxf", "Courier LD (Bold Italic).mxf",
	"CordiaUPC.mxf", "CordiaUPC (Bold).mxf",
	"HG-Gothic.mxf", "HG-Gothic (Bold).mxf",
	"Hy Round Gothic.mxf", "Hy Round Gothic (bold).mxf",
	"MHei B5 (Bold).mxf", "MHei GB (Bold).mxf",
}

func russianFontSourcePath(name string) string {
	return "Fonts/" + strings.TrimSuffix(name, ".mxf") + ".ttf"
}

func readRussianFonts(dir string) (map[string][]byte, error) {
	fonts := make(map[string][]byte, len(russianFontFiles))
	for _, name := range russianFontFiles {
		font, err := readRussianFont(filepath.Join(dir, russianFontSourcePath(name)))
		if err != nil {
			return nil, err
		}
		fonts[name] = font
	}
	return fonts, nil
}

const russianTextPackage = "russian-text.dat"

func russianUI(localePath string) ([]resource, error) {
	locale, err := openArchive(localePath)
	if err != nil {
		return nil, err
	}
	defer locale.file.Close()

	var entries []resource
	hasCyrillic := false
	for _, r := range locale.records {
		if r.id.kind != 0x2026960b {
			continue
		}
		data, err := locale.read(r)
		if err != nil {
			return nil, fmt.Errorf("Russian locale %s: %w", r.id, err)
		}
		if len(data) < 4 || len(data)%2 != 0 {
			return nil, fmt.Errorf("Russian locale %s: invalid text resource", r.id)
		}
		for i := 4; !hasCyrillic && i < len(data); i += 2 {
			unit := little.Uint16(data[i:])
			hasCyrillic = unit >= 0x0410 && unit <= 0x044f
		}
		entries = append(entries, resource{r.id, data})
	}
	if !hasCyrillic {
		return nil, fmt.Errorf("%s contains no Cyrillic text", localePath)
	}
	return entries, nil
}

func readRussianFont(path string) ([]byte, error) {
	font, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(font) < 12 || !bytes.HasPrefix(font, []byte{0, 1, 0, 0}) {
		return nil, fmt.Errorf("%s is not a TrueType font", path)
	}
	return encodeRussianFont(font), nil
}

func encodeRussianFont(font []byte) []byte {
	mxf := append([]byte("MXFN\x02\x62\x5a\x00\x9d"), font...)
	for i := 9; i < len(mxf); i++ {
		mxf[i] ^= 0x9d
	}
	return mxf
}

// The original locale stores its counted UTF-16 text without a terminator.
// Some translations count the terminator and also leave bytes after the text.
func normalizeRussianText(data []byte) ([]byte, error) {
	const unicodeText = 0x10000000
	const lengthMask = 0x00ffffff
	header := little.Uint32(data)
	if header&^lengthMask != unicodeText {
		return data, nil
	}
	end := 4 + 2*int(header&lengthMask)
	if end > len(data) {
		return nil, fmt.Errorf("Russian text length exceeds the resource size")
	}
	for end > 4 && little.Uint16(data[end-2:]) == 0 {
		end -= 2
	}
	little.PutUint32(data, unicodeText|uint32((end-4)/2))
	return data[:end], nil
}
