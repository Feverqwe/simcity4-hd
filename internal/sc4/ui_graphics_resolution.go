package sc4

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"image/color"
	"strings"
)

const graphicsOptionsLayout = 0x8a7e052f
const displayTextFirst = 0xe74a44e1

var graphicsComboFrameSource = resourceID{uiImage, catalogFrameGroup, 0x13f15250}

func aspyrScrollbarArt(stock uiStock) ([]resource, error) {
	// The original scrollbar is available through three resource keys.
	ids := []resourceID{
		{uiImage, catalogFrameGroup, 0x144161e3},
		{uiImage, catalogFrameGroup, 0x46a006a6},
		{uiImage, mySimPortraitGroup, 0x144161e3},
	}
	original, err := stock.read(ids[0])
	if err != nil {
		return nil, err
	}
	art, err := decodeRGBA(original)
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
	encoded, err := encodeRGBA(art)
	if err != nil {
		return nil, err
	}
	entries := make([]resource, 0, len(ids))
	for _, id := range ids {
		alias, err := stock.read(id)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(alias, original) {
			return nil, fmt.Errorf("unexpected Aspyr scrollbar art %s", id)
		}
		entries = append(entries, resource{id, encoded})
	}
	return entries, nil
}

//go:embed patchsrc/ui_graphics_resolution.i386
var graphicsResolutionCode []byte

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
				{0xe74a4402, displayTextFirst, "(7,276,252,315)"},
				{0xe74a4403, displayTextFirst + 1, "(7,315,252,347)"},
				{0xe74a4405, displayTextFirst + 2, "(267,315,455,335)"},
			}
			if mac {
				statuses = append(statuses, statusControl{0xe74a4408, displayTextFirst + 6, "(7,315,252,347)"})
			}
			for _, item := range statuses {
				status := strings.NewReplacer(
					"id=0xca57da80", fmt.Sprintf("id=0x%08x", item.id),
					"area=(4,255,246,276)", "area="+item.area,
					"winflag_visible=yes", "winflag_visible=no",
					"winflag_ignoremouse=no", "winflag_ignoremouse=yes",
					"wrapped=no", "wrapped=yes",
					`caption="Resolution" captionres={0a554ae8,ca7e17ca}`,
					fmt.Sprintf(`caption="" captionres={%08x,%08x}`, hdUITextGroup, item.caption),
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
	source, err := decodeRGBA(original)
	if err != nil {
		return nil, err
	}
	art, err := decodeRGBA(enlarged)
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
	return encodeRGBA(art)
}

func graphicsResolutionStrings(russian, mac bool) ([]resource, error) {
	data := graphicsDisplayEnglish
	if russian {
		data = graphicsDisplayRussian
	}
	var captions map[string]string
	if err := json.Unmarshal(data, &captions); err != nil {
		return nil, err
	}
	var entries []resource
	keys := []string{"desktopResolution", "saveError", "windowMode", "windowed", "fullscreen", "borderless"}
	if mac {
		keys = append(keys, "restart")
	}
	for index, key := range keys {
		caption := captions[key]
		if caption == "" {
			return nil, fmt.Errorf("missing display settings translation %q", key)
		}
		entries = append(entries, resource{resourceID{0x2026960b, hdUITextGroup, displayTextFirst + uint32(index)}, encodeLocaleText(caption)})
	}
	return entries, nil
}

func appendGraphicsResolutionPatches(stub []byte, patches []instructionPatch, base int) ([]byte, []instructionPatch) {
	const (
		imageBase   = 0x400000
		initSite    = 0x391d57
		resetSite   = 0x392eb1
		acceptSite  = 0x392c14
		messageSite = 0x392f7a
	)

	for len(stub)%4 != 0 {
		stub = append(stub, 0)
	}
	data := imageBase + base + len(stub)
	// The supported Steam EXE imports these entry points. Resolve the additional
	// Win32 APIs through them so that the existing PE import table stays intact.
	for _, field := range []uint32{0xa800f0, 0xa800cc, 0xa80810, 0x603580, 0x414250, hdUITextGroup, displayTextFirst + 3} {
		stub = little.AppendUint32(stub, field)
	}
	for _, field := range []string{
		"kernel32.dll", "user32.dll", "SC4GraphicsOptions.dll", "GetModuleFileNameW",
		"GetPrivateProfileStringW", "GetPrivateProfileIntW", "WritePrivateProfileStringW",
		"GetFileAttributesW", "GetTempFileNameW", "CopyFileW", "MoveFileExW", "DeleteFileW",
		"GetActiveWindow", "MonitorFromWindow", "GetMonitorInfoW", "EnumDisplaySettingsW",
	} {
		stub = append(stub, field...)
		stub = append(stub, 0)
	}
	if len(stub)%2 != 0 {
		stub = append(stub, 0)
	}
	for _, field := range []string{
		"SC4GraphicsOptions.ini", "GraphicsOptions", "WindowMode", "WindowWidth", "WindowHeight",
		"Borderless", "BorderlessFullScreen", "Windowed", "FullScreen", "sc4",
	} {
		for _, c := range field {
			stub = little.AppendUint16(stub, uint16(c))
		}
		stub = little.AppendUint16(stub, 0)
	}

	entry := base + len(stub)
	stub = append(stub, graphicsResolutionCode...)
	// The helper receives the graphics dialog, operation, and read-only data.
	addCall := func(body []byte, operation byte, dialogOpcode ...byte) []byte {
		body = little.AppendUint32(append(body, 0x68), uint32(data))
		body = append(body, 0x6a, operation)
		body = append(body, dialogOpcode...)
		body = append(body, relativeInstruction(0xe8, base+len(stub)+len(body), entry)...)
		return append(body, 0x83, 0xc4, 0x0c)
	}
	var patch instructionPatch
	initOriginal := hexBytes("8b106a016a018bc8")
	init := addCall(hexBytes("608d46fc"), 0, 0x50) // The constructor's ESI is &dialog->host.
	init = append(append(init, 0x61), initOriginal...)
	stub, patch = appendStub(stub, base, initSite, initOriginal, init)
	patches = append(patches, patch)

	resetOriginal := hexBytes("8bcee808d1ffff")
	reset := append(hexBytes("8bce"), relativeInstruction(0xe8, base+len(stub)+2, 0x38ffc0)...)
	reset = addCall(append(reset, 0x60), 1, 0x56)
	stub, patch = appendStub(stub, base, resetSite, resetOriginal, append(reset, 0x61))
	patches = append(patches, patch)

	acceptOriginal := hexBytes("8b068bceff501c")
	accept := addCall([]byte{0x60}, 2, 0x56)
	accept = append(accept, hexBytes("8944241c6183f8027505")...)
	accept = append(accept, relativeInstruction(0xe9, base+len(stub)+len(accept), 0x392ebf)...)
	accept = append(accept, 0x50) // Preserve the successful-save flag across native Apply.
	accept = append(accept, acceptOriginal...)
	accept = append(accept, hexBytes("5883f8017504c6461a01")...)
	stub, patch = appendStub(stub, base, acceptSite, acceptOriginal, accept)
	patches = append(patches, patch)

	// The combo's selection notification arrives after its selected index changes.
	// Preserve the stock listener for every other event.
	messageOriginal := hexBytes("8b4c24108b1132c0")
	message := hexBytes("8b4c24108339037524817904551271e8751b81790804444ae7751260")
	message = addCall(message, 3, 0x56)
	message = append(message, 0x61)
	message = append(message, messageOriginal...)
	stub, patch = appendStub(stub, base, messageSite, messageOriginal, message)
	return stub, append(patches, patch)
}
