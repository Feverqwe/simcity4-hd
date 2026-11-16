package sc4

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
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

const russianButtonCaptionGroup = 0xe74a40ca
const russianBudgetTooltipGroup = 0xe74a40cb

//go:embed localization/ru/button-captions.json
var russianButtonCaptionData []byte

//go:embed localization/ru/budget-captions.json
var russianBudgetCaptionData []byte

//go:embed localization/ru/static-captions.json
var russianStaticCaptionData []byte

func russianTooltipLabel(out *strings.Builder, area, font, showCaption, color, caption, tip string) {
	// The game's disabled label style supports hover without button activation.
	fmt.Fprintf(out, "   <LEGACY clsid=GZWinBtn iid=IGZWinBtn %s %s captionres={%s} "+
		"winflag_visible=yes winflag_enabled=no winflag_pbuff=no winflag_acceptfocus=no "+
		"style=label showcaption=%s fill=no autosize=no wrapcaption=no align=left gutters=(4,0,4,0) "+
		"colorfontdisabled=(%s) tips=yes tipsdelay=no "+
		"tipstimeout=no tipoffsets=(0,0) tipflag=0x01000000 tipres={%s} >\n",
		area, font, caption, showCaption, color, tip)
}

func russianBudgetTooltips(hud []resource) {
	// Give the visible tax caption the game's native button-label tooltip.
	for i := range hud {
		layout := &hud[i]
		if layout.id != layoutID(0xcbc3c2b9) {
			continue
		}
		var output strings.Builder
		for _, line := range strings.SplitAfter(string(layout.data), "\n") {
			if !strings.Contains(line, "captionres={6a3ff01c,cc05b9a4}") {
				output.WriteString(line)
				continue
			}
			russianTooltipLabel(&output, driveButtonArea.FindString(line), layoutFont.FindString(line),
				"yes", "51,68,102", "6a3ff01c,cc05b9a4", fmt.Sprintf("%08x,cc05b9a4", russianBudgetTooltipGroup))
		}
		layout.data = []byte(output.String())
	}
}

func encodeLocaleText(text string) []byte {
	units := utf16.Encode([]rune(text))
	data := little.AppendUint32(nil, unicodeLocaleText|uint32(len(units)))
	for _, unit := range units {
		data = little.AppendUint16(data, unit)
	}
	return data
}

var buttonCaptionReference = regexp.MustCompile(`\bcaptionres=\{([[:xdigit:]]{8}),[ \t]*([[:xdigit:]]{8})\}`)

func russianStaticCaptionEntries(hud []resource) ([]resource, error) {
	var captions map[string]string
	if err := json.Unmarshal(russianStaticCaptionData, &captions); err != nil {
		return nil, fmt.Errorf("parse Russian static captions: %w", err)
	}
	entries := make(map[resourceID]string)
	var result []resource
	for i := range hud {
		layout := &hud[i]
		if layout.id != layoutID(layout.id.instance) {
			continue
		}
		var output strings.Builder
		for _, line := range strings.SplitAfter(string(layout.data), "\n") {
			ref := buttonCaptionReference.FindStringSubmatch(line)
			if ref == nil || !strings.Contains(line, "clsid=GZWinText ") ||
				!strings.Contains(line, " notify=no ") || !strings.Contains(line, " wrapped=no ") ||
				!strings.Contains(line, "winflag_visible=yes") {
				output.WriteString(line)
				continue
			}
			full := strings.ToLower(ref[1] + "," + ref[2])
			short, ok := captions[full]
			if !ok {
				output.WriteString(line)
				continue
			}
			area, font := driveButtonArea.FindString(line), layoutFont.FindString(line)
			if area == "" || font == "" {
				return nil, fmt.Errorf("unexpected layout for Russian static caption %08x/%s", layout.id.instance, full)
			}
			instance, _ := strconv.ParseUint(ref[2], 16, 32)
			id := resourceID{0x2026960b, hdUITextGroup, uint32(instance)}
			if previous, exists := entries[id]; exists {
				if previous != short {
					return nil, fmt.Errorf("conflicting Russian static caption %s", full)
				}
			} else {
				entries[id] = short
				result = append(result, resource{id, encodeLocaleText(short)})
			}
			reference := fmt.Sprintf("%08x,%08x", id.group, id.instance)
			output.WriteString(strings.Replace(line, ref[0], "captionres={"+reference+"}", 1))
			// Keep the native text window and its ID. A transparent label in
			// the same rectangle supplies only the tooltip.
			russianTooltipLabel(&output, area, font, "no", "0,0,0", reference, full)
		}
		layout.data = []byte(output.String())
	}
	return result, nil
}

// Some menu labels share localization keys with headings and graph titles.
// Keep the full localization key available to the button's native tooltip.
func russianButtonCaptionEntries(hud []resource) ([]resource, error) {
	var captions map[string]string
	if err := json.Unmarshal(russianButtonCaptionData, &captions); err != nil {
		return nil, fmt.Errorf("parse Russian button captions: %w", err)
	}
	var entries []resource
	added := make(map[resourceID]bool)
	for i := range hud {
		layout := &hud[i]
		if layout.id != layoutID(layout.id.instance) {
			continue
		}
		lines := strings.SplitAfter(string(layout.data), "\n")
		changed := false
		for j, line := range lines {
			if !strings.Contains(line, "clsid=GZWinBtn ") || !strings.Contains(line, "showcaption=yes") {
				continue
			}
			ref := buttonCaptionReference.FindStringSubmatch(line)
			if ref == nil {
				continue
			}
			caption, ok := captions[strings.ToLower(ref[1]+","+ref[2])]
			if !ok {
				continue
			}
			instance, _ := strconv.ParseUint(ref[2], 16, 32)
			id := resourceID{0x2026960b, russianButtonCaptionGroup, uint32(instance)}
			if !added[id] {
				entries = append(entries, resource{id, encodeLocaleText(caption)})
				added[id] = true
			}
			line = strings.Replace(line, ref[0], fmt.Sprintf("captionres={%08x,%08x}", id.group, id.instance), 1)
			if strings.Contains(line, " tips=no ") && !strings.Contains(line, " tipres=") &&
				strings.Contains(line, ` tiptext="" `) {
				line = strings.Replace(line, " tips=no ", " tips=yes ", 1)
				line = strings.Replace(line, " tipsdelay=no ", " tipsdelay=yes ", 1)
				tip := fmt.Sprintf(` tiptext="" tipres={%s,%s} `, ref[1], ref[2])
				line = strings.Replace(line, ` tiptext="" `, tip, 1)
			}
			lines[j] = line
			changed = true
		}
		if changed {
			layout.data = []byte(strings.Join(lines, ""))
		}
	}
	return entries, nil
}

func russianUI(localePath string) ([]resource, error) {
	var budgetCaptions map[string]string
	if err := json.Unmarshal(russianBudgetCaptionData, &budgetCaptions); err != nil {
		return nil, fmt.Errorf("parse Russian budget captions: %w", err)
	}
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
		// This authoring example is stored as plain text in the source
		// translation archive, under an LTEXT key.
		if r.id == (resourceID{0x2026960b, 0x55555555, 0xaa41380e}) &&
			bytes.HasPrefix(data, []byte("This is a sample string that has both an instance")) {
			continue
		}
		end, unicode, err := localeTextExtent(data)
		if err != nil {
			return nil, fmt.Errorf("Russian locale %s: %w", r.id, err)
		}
		for i := 4; unicode && !hasCyrillic && i < end; i += 2 {
			unit := little.Uint16(data[i:])
			hasCyrillic = unit >= 0x0410 && unit <= 0x044f
		}
		if r.id.group == 0x6a3ff01c {
			if caption, ok := budgetCaptions[fmt.Sprintf("%08x", r.id.instance)]; ok {
				tooltip := r.id
				tooltip.group = russianBudgetTooltipGroup
				entries = append(entries, resource{tooltip, data})
				data = encodeLocaleText(caption)
			}
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

const unicodeLocaleText = 0x10000000

func localeTextExtent(data []byte) (end int, unicode bool, err error) {
	if len(data) < 4 {
		return 0, false, fmt.Errorf("text resource is missing its length")
	}
	header := little.Uint32(data)
	count := int(header & 0x00ffffff)
	switch header & 0xff000000 {
	case 0:
	case unicodeLocaleText:
		count *= 2
		unicode = true
	default:
		return 0, false, fmt.Errorf("unsupported text resource encoding %#x", header>>24)
	}
	end = 4 + count
	if end > len(data) {
		return 0, false, fmt.Errorf("text length exceeds the resource size")
	}
	return end, unicode, nil
}

// The original locale stores its counted UTF-16 text without a terminator.
// Some translations count the terminator and also leave bytes after the text.
func normalizeRussianText(data []byte) ([]byte, error) {
	end, unicode, err := localeTextExtent(data)
	if err != nil {
		return nil, err
	}
	if !unicode {
		return data, nil
	}
	for end > 4 && little.Uint16(data[end-2:]) == 0 {
		end -= 2
	}
	little.PutUint32(data, unicodeLocaleText|uint32((end-4)/2))
	return data[:end], nil
}
