package sc4

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	panelGeometry  = regexp.MustCompile(`\b(area|imagerect|gutters)=\((-?\d+(?:,-?\d+)+)\)`)
	regionGeometry = regexp.MustCompile(`\b(area|imagerect|gutters|textoffsets|tipoffsets)=\((-?\d+(?:,-?\d+)+)\)`)
	regionBodyFont = regexp.MustCompile(`(?im)^(GenBodyMedium\s*=\s*"[^"\r\n]+"\s*,\s*")([0-9]+)("\s*,\s*"[^"\r\n]*"\s*,\s*)0x[0-9a-f]+`)
)

type regionAtlas struct {
	instance uint32
	frames   int
}

func regionEntries(stock uiStock, config string, textureScale int, art func(layoutAtlas, []byte) ([]byte, error)) ([]resource, string, error) {
	id := layoutID(0xaa920991)
	data, err := stock.read(id)
	if err != nil {
		return nil, "", err
	}
	text := string(data)
	const panelStart = "<LEGACY clsid=GZWinGen iid=IGZWinGen id=0x09ebee45 "
	const panelEnd = "<LEGACY clsid=0x89e1567c iid=IGZWinGen id=0x6a91dc16 "
	const regionStart = "<LEGACY clsid=0x89e1567c iid=IGZWinGen id=0x09ebe9ee "
	panel, end := strings.Index(text, panelStart), strings.Index(text, panelEnd)
	region := strings.Index(text, regionStart)
	if region < 0 || panel <= region || end <= panel {
		return nil, "", fmt.Errorf("unexpected region layout sections")
	}

	var entries []resource
	swapArt := func(section string, atlases []regionAtlas, group uint32, deferred uint32) (string, error) {
		for _, atlas := range atlases {
			source := resourceID{uiImage, catalogFrameGroup, atlas.instance}
			original := fmt.Sprintf("{%08x,%08x}", source.group, source.instance)
			// The city selection background and the settings frames use native drawing.
			project := textureScale == 4 && (atlas.instance == 0x14416307 || atlas.instance == 0x144161e4)
			targetGroup := group
			if project {
				targetGroup += 0x20
			}
			if atlas.instance != deferred {
				if !strings.Contains(section, original) {
					return "", fmt.Errorf("region atlas %s missing from layout", source)
				}
				section = strings.ReplaceAll(section, original, fmt.Sprintf("{%08x,%08x}", targetGroup, atlas.instance))
			}
			data, err := stock.read(source)
			if err != nil {
				return "", err
			}
			data, err = art(layoutAtlas{source: source, frames: atlas.frames, project: project}, data)
			if err != nil {
				return "", fmt.Errorf("%s: %w", source, err)
			}
			entries = append(entries, resource{resourceID{uiImage, targetGroup, atlas.instance}, data})
		}
		if textureScale == 2 {
			return section, nil
		}
		lines := strings.SplitAfter(section, "\n")
		marker := fmt.Sprintf("image={%08x,", group)
		for i, line := range lines {
			if strings.Contains(line, marker) {
				lines[i] = scaleLayoutGeometry(line, layoutSourceRect, nil)
			}
		}
		return strings.Join(lines, ""), nil
	}
	panelText := scaleLayoutGeometry(text[panel:end], panelGeometry, nil)
	panelText = strings.NewReplacer("area=(744,-8,784,32)", "area=(549,-8,569,12)",
		"font=GenBodyMedium", "font=HDProbeBodyMedium").Replace(panelText)
	panelText, err = swapArt(panelText, []regionAtlas{
		{0x14416301, 4}, {0x14416307, 1}, {0x14416314, 4}, {0x14416313, 4}, {0x14416312, 4},
		{0x144161e4, 1}, {0x14416315, 8}, {0x14416316, 8},
	}, 0xe74a40b1, 0x14416301)
	if err != nil {
		return nil, "", err
	}
	stockFont := regionBodyFont.FindStringSubmatch(config)
	if stockFont == nil {
		return nil, "", fmt.Errorf("missing GenBodyMedium font definition")
	}
	at := strings.Index(config, stockFont[0]) + len(stockFont[0])
	endOfLine := strings.Index(config[at:], "\r\n")
	if endOfLine < 0 {
		return nil, "", fmt.Errorf("unterminated GenBodyMedium font definition")
	}
	at += endOfLine
	alias := fmt.Sprintf("%s%d%s0xe74a40b2", strings.Replace(stockFont[1], "GenBodyMedium", "HDProbeBodyMedium", 1),
		2*layoutNumber(stockFont[2]), stockFont[3])
	config = config[:at] + "\r\n" + alias + config[at:]

	regionText := text[region:panel]
	regionText = scaleLayoutGeometry(regionText, regionGeometry, func(position int) bool {
		return strings.HasSuffix(regionText[:position], "id=0x09ebe9ee ") ||
			strings.HasSuffix(regionText[:position], "id=0x09ebee60 ")
	})
	var names []fontAlias
	seen := make(map[string]bool)
	regionText = layoutFont.ReplaceAllStringFunc(regionText, func(font string) string {
		name := strings.TrimPrefix(font, "font=")
		alias := "HDProbe" + name
		if !seen[name] {
			names = append(names, fontAlias{name, alias})
			seen[name] = true
		}
		return "font=" + alias
	})
	config, err = addLayoutFontAliases(config, names)
	if err != nil {
		return nil, "", err
	}
	regionText, err = swapArt(regionText, []regionAtlas{
		{0x14416300, 1}, {0x1441630d, 1}, {0x14416318, 4}, {0x14416317, 4}, {0x14416319, 4},
		{0x1441631a, 4}, {0x1441631b, 4}, {0x13e14fa0, 4},
	}, 0xe74a40b1, 0)
	if err != nil {
		return nil, "", err
	}

	text = strings.NewReplacer("area=(23,12,103,72)", "area=(23,12,183,132)",
		"area=(350,0,804,91)", "area=(350,0,804,139)",
		"{46a006b0,14416301}", "{e74a40b1,14416301}").Replace(text[:region] + regionText + panelText + text[end:])
	for _, pair := range [][2]string{
		{"350,0,804,139", "350,0,867,139"}, {"867,0,982,76", "867,0,1097,152"},
		{"193,12,273,72", "188,12,348,132"}, {"353,12,433,72", "353,12,513,132"},
		{"25,15,85,61", "50,30,170,122"}, {"110,0,130,20", "220,0,260,40"},
		{"212,35,285,77", "212,35,358,119"}, {"0,0,50,30", "0,0,100,60"},
	} {
		old, replacement := "area=("+pair[0]+")", "area=("+pair[1]+")"
		if strings.Count(text, old) != 1 {
			return nil, "", fmt.Errorf("unexpected region button geometry %s", old)
		}
		text = strings.Replace(text, old, replacement, 1)
	}
	for _, instance := range []uint32{0x14416302, 0x14416303, 0x14416304, 0x14416106} {
		reference := fmt.Sprintf("{%08x,%08x}", catalogFrameGroup, instance)
		if strings.Count(text, reference) != 1 {
			return nil, "", fmt.Errorf("region button atlas %s missing or ambiguous", reference)
		}
	}
	text = strings.Replace(text, "image={46a006b0,14416106} imagerect=(0,0,50,30)",
		"image={46a006b0,14416106} imagerect=(0,0,100,60)", 1)
	text, err = swapArt(text, []regionAtlas{{0x14416302, 4}, {0x14416303, 4}, {0x14416304, 4}, {0x14416106, 1}}, 0xe74a40c2, 0)
	if err != nil {
		return nil, "", err
	}
	entries = append(entries, resource{id, []byte(text)})
	return entries, config, nil
}
