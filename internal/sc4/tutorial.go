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
		changed, err := rewriteLocaleHTML(data, func(text string) string {
			return tutorialFontSize.ReplaceAllString(text, "${1}7${2}")
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.id, err)
		}
		if !changed {
			continue
		}
		entries = append(entries, resource{r.id, data})
	}
	if len(entries) == 0 {
		return nil, errors.New("no tutorial text with the expected font markup found")
	}
	return entries, nil
}

var (
	creditsTextID   = resourceID{0x2026960b, 0x8a4924f3, 0x4a5d648f}
	creditsFontSize = regexp.MustCompile(`(?i)(<font\b[^>]*?\bsize\s*=\s*["']?)3(["'\s][^>]*>|>)`)
)

func creditsEntries(locale string) ([]resource, error) {
	a, err := openArchive(locale)
	if err != nil {
		return nil, err
	}
	defer a.file.Close()
	for _, r := range a.records {
		if r.id != creditsTextID {
			continue
		}
		data, err := a.read(r)
		if err != nil {
			return nil, err
		}
		changed, err := rewriteLocaleHTML(data, func(text string) string {
			var result strings.Builder
			start := 0
			for _, m := range creditsFontSize.FindAllStringSubmatchIndex(text, -1) {
				if strings.HasPrefix(strings.TrimSpace(text[m[1]:]), "&nbsp;</font>") {
					continue
				}
				result.WriteString(text[start:m[3]])
				result.WriteByte('4')
				start = m[3] + 1
			}
			result.WriteString(text[start:])
			return result.String()
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.id, err)
		}
		if changed {
			return []resource{{r.id, data}}, nil
		}
		break
	}
	return nil, nil
}

func rewriteLocaleHTML(data []byte, rewrite func(string) string) (bool, error) {
	if len(data) < 4 || len(data)%2 != 0 {
		return false, errors.New("invalid UTF-16 text resource")
	}
	units := make([]uint16, (len(data)-4)/2)
	for i := range units {
		units[i] = little.Uint16(data[4+2*i:])
	}
	text := string(utf16.Decode(units))
	updated := rewrite(text)
	if updated == text {
		return false, nil
	}
	encoded := utf16.Encode([]rune(updated))
	if len(encoded) != len(units) || !validUTF16(units) {
		return false, errors.New("invalid UTF-16 text")
	}
	for i, unit := range encoded {
		little.PutUint16(data[4+2*i:], unit)
	}
	return true, nil
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
