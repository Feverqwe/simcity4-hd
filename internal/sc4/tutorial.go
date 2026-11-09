package sc4

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"
)

var tutorialFontSize = regexp.MustCompile(`(?i)(<font\b[^>]*?\bsize\s*=\s*["']?)4(["'\s>])`)

func tutorialEntries(locale string) ([]resource, error) {
	a, err := openArchive(locale)
	if err != nil {
		return nil, err
	}
	defer a.file.Close()

	var entries []resource
	for _, r := range a.records {
		if r.id.kind != 0x2026960b || r.id.group != 0xca554b03 {
			continue
		}
		data, err := a.read(r)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.id, err)
		}
		if len(data) < 4 || len(data)%2 != 0 {
			return nil, fmt.Errorf("%s: invalid UTF-16 tutorial resource", r.id)
		}
		units := make([]uint16, (len(data)-4)/2)
		for i := range units {
			units[i] = little.Uint16(data[4+2*i:])
		}
		text := string(utf16.Decode(units))
		updated := tutorialFontSize.ReplaceAllString(text, "${1}7${2}")
		if updated == text {
			continue
		}
		encoded := utf16.Encode([]rune(updated))
		if len(encoded) != len(units) || !validUTF16(units) {
			return nil, fmt.Errorf("%s: invalid UTF-16 tutorial text", r.id)
		}
		for i, unit := range encoded {
			little.PutUint16(data[4+2*i:], unit)
		}
		entries = append(entries, resource{r.id, data})
	}
	if len(entries) == 0 {
		return nil, errors.New("no tutorial text with the expected font markup found")
	}
	return entries, nil
}

func validUTF16(units []uint16) bool {
	for i := 0; i < len(units); i++ {
		unit := units[i]
		if !utf16.IsSurrogate(rune(unit)) {
			continue
		}
		if unit >= 0xdc00 || i+1 == len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
			return false
		}
		i++
	}
	return true
}

func buildTutorial(locale, output string) (int, error) {
	entries, err := tutorialEntries(locale)
	if err != nil {
		return 0, err
	}
	if err := publishFile(output, func(w io.Writer) error {
		return writeArchive(w, entries)
	}); err != nil {
		return 0, err
	}
	return len(entries), nil
}

func runTutorial(args []string) error {
	if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
		return errors.New("usage: simcity4-hd tutorial GAME_DIR OUTPUT.dat")
	}
	count, err := buildTutorial(filepath.Join(args[0], "SimCityLocale.DAT"), args[1])
	if err != nil {
		return err
	}
	fmt.Printf("%s: %d tutorial text overrides\n", args[1], count)
	return nil
}
