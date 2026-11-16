package ui

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"image/color"
	"strings"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/uiids"
)

const graphicsOptionsLayout = 0x8a7e052f

var graphicsComboFrameSource = dbpf.ID{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: 0x13f15250}

func aspyrScrollbarArt(stock uiStock) ([]dbpf.Entry, error) {
	// The original scrollbar is available through three resource keys.
	ids := []dbpf.ID{
		{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: 0x144161e3},
		{Kind: uiImage, Group: uiids.CatalogFrameGroup, Instance: 0x46a006a6},
		{Kind: uiImage, Group: mySimPortraitGroup, Instance: 0x144161e3},
	}
	original, err := stock.read(ids[0])
	if err != nil {
		return nil, err
	}
	art, err := textures.DecodeRGBA(original)
	if err != nil {
		return nil, err
	}
	// The bitmap painter does not draw a track behind the keyed artwork.
	track := color.NRGBA{R: 222, G: 232, B: 227, A: 255}
	for y := range art.Rect.Dy() {
		for x := range art.Rect.Dx() {
			if art.NRGBAAt(x, y) == uiColorKey {
				art.SetNRGBA(x, y, track)
			}
		}
	}
	encoded, err := textures.EncodeRGBA(art)
	if err != nil {
		return nil, err
	}
	entries := make([]dbpf.Entry, 0, len(ids))
	for _, id := range ids {
		alias, err := stock.read(id)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(alias, original) {
			return nil, fmt.Errorf("unexpected Aspyr scrollbar art %s", id)
		}
		entries = append(entries, dbpf.Entry{ID: id, Data: encoded})
	}
	return entries, nil
}

//go:embed localization/en/graphics-display.json
var graphicsDisplayEnglish []byte

//go:embed localization/ru/graphics-display.json
var graphicsDisplayRussian []byte

func graphicsResolutionLayout(stock uiStock, original []byte, mac bool) ([]byte, error) {
	zodiac, err := stock.read(layoutID(0x0a243d80))
	if err != nil {
		return nil, err
	}
	var combo, frame string
	for _, line := range strings.Split(string(zodiac), "\n") {
		if strings.Contains(line, "clsid=GZWinBMP ") && strings.Contains(line, "area=(80,284,370,334)") &&
			strings.Contains(line, "imagerect=(80,284,434,381)") {
			// My Sims paints the rounded outline into this bitmap, behind the combo.
			frame = strings.NewReplacer(
				"area=(80,284,370,334)", "id=0xe74a4406 area=(1,275,185,297)",
				"imagerect=(80,284,434,381)", "imagerect=(170,309,354,331)",
				"winflag_visible=yes", "winflag_visible=no",
				"winflag_alphablend=no", "winflag_alphablend=yes",
			).Replace(line) + "\n"
		}
		if strings.Contains(line, "clsid=GZWinCombo ") && strings.Contains(line, " id=0xaa243f5e ") {
			line, _, _ = strings.Cut(line, " listelement=")
			combo = strings.NewReplacer(
				"id=0xaa243f5e", "id=0xe74a4401",
				"area=(176,310,346,330)", "area=(7,276,177,296)",
				"winflag_visible=yes", "winflag_visible=no",
				"initselection=1", "initselection=-1",
				"buttongutter=1", "buttongutter=2",
			).Replace(line) + " >\r\n"
		}
	}

	var layout strings.Builder
	var labels int
	for _, line := range strings.SplitAfter(string(original), "\n") {
		if strings.Contains(line, " id=0xca57da80 ") {
			for index, instance := range []string{"ca7e17d8", "ca7e17d9", "ca7e17da", "ca7e17db"} {
				if strings.Contains(line, "captionres={0a554ae8,"+instance+"}") {
					line = strings.Replace(line, "id=0xca57da80", fmt.Sprintf("id=0x%08x", 0xe74a4410+index), 1)
					labels++
					break
				}
			}
		}
		layout.WriteString(line)
		if strings.Contains(line, "captionres={0a554ae8,ca7e17ca}") {
			layout.WriteString(frame)
			layout.WriteString(combo)
			layout.WriteString(strings.NewReplacer("0xe74a4406", "0xe74a4407",
				"area=(1,275,185,297)", "area=(264,334,448,356)").Replace(frame))
			layout.WriteString(strings.NewReplacer("0xe74a4401", "0xe74a4404",
				"area=(7,276,177,296)", "area=(270,335,440,355)").Replace(combo))
			type statusControl struct {
				id, caption uint32
				area        string
			}
			statuses := []statusControl{
				{0xe74a4402, uiids.DisplayTextFirst, "(7,276,252,315)"},
				{0xe74a4403, uiids.DisplayTextFirst + 1, "(7,315,252,347)"},
				{0xe74a4405, uiids.DisplayTextFirst + 2, "(267,315,455,335)"},
			}
			if mac {
				statuses = append(statuses, statusControl{0xe74a4408, uiids.DisplayTextFirst + 6, "(7,315,252,347)"})
			}
			for _, item := range statuses {
				status := strings.NewReplacer(
					"id=0xca57da80", fmt.Sprintf("id=0x%08x", item.id),
					"area=(4,255,246,276)", "area="+item.area,
					"winflag_visible=yes", "winflag_visible=no",
					"winflag_ignoremouse=no", "winflag_ignoremouse=yes",
					"wrapped=no", "wrapped=yes",
					`caption="Resolution" captionres={0a554ae8,ca7e17ca}`,
					fmt.Sprintf(`caption="" captionres={%08x,%08x}`, uiids.HdUITextGroup, item.caption),
				).Replace(line)
				layout.WriteString(status)
			}
		}
	}
	if combo == "" || frame == "" || labels != 4 || !strings.Contains(layout.String(), " id=0xe74a4401 ") {
		return nil, fmt.Errorf("unexpected Graphics Options resolution controls")
	}
	return []byte(layout.String()), nil
}

func maskGraphicsComboBackdrop(original, enlarged []byte, scale int) ([]byte, error) {
	source, err := textures.DecodeRGBA(original)
	if err != nil {
		return nil, err
	}
	art, err := textures.DecodeRGBA(enlarged)
	if err != nil {
		return nil, err
	}
	if source.Rect.Dx() < 354 || source.Rect.Dy() < 331 ||
		art.Rect.Dx() != scale*source.Rect.Dx() || art.Rect.Dy() != scale*source.Rect.Dy() {
		return nil, fmt.Errorf("invalid dimensions for the My Sims frame atlas")
	}
	// Clear the old panel color in the corners of the rounded frame.
	backdrop := source.NRGBAAt(170, 309)
	for y := 309; y < 331; y++ {
		for x := 170; x < 354; x++ {
			if source.NRGBAAt(x, y) != backdrop {
				continue
			}
			for dy := range scale {
				for dx := range scale {
					art.Pix[art.PixOffset(x*scale+dx, y*scale+dy)+3] = 0
				}
			}
		}
	}
	return textures.EncodeRGBA(art)
}

func graphicsResolutionStrings(russian, mac bool) ([]dbpf.Entry, error) {
	data := graphicsDisplayEnglish
	if russian {
		data = graphicsDisplayRussian
	}
	var captions map[string]string
	if err := json.Unmarshal(data, &captions); err != nil {
		return nil, err
	}
	var entries []dbpf.Entry
	keys := []string{"desktopResolution", "saveError", "windowMode", "windowed", "fullscreen", "borderless"}
	if mac {
		keys = append(keys, "restart")
	}
	for index, key := range keys {
		caption := captions[key]
		if caption == "" {
			return nil, fmt.Errorf("missing display settings translation %q", key)
		}
		entries = append(entries, dbpf.Entry{ID: dbpf.ID{Kind: 0x2026960b, Group: uiids.HdUITextGroup, Instance: uiids.DisplayTextFirst + uint32(index)}, Data: encodeLocaleText(caption)})
	}
	return entries, nil
}
