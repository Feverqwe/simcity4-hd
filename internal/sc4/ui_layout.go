package sc4

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"regexp"
	"strconv"
	"strings"
)

var (
	driveButtonArea  = regexp.MustCompile(`\barea=\((-?\d+),(-?\d+),(-?\d+),(-?\d+)\)`)
	legacyRoot       = regexp.MustCompile(`(?i)<LEGACY\b(?:[^">]|"[^"]*")*>`)
	layoutSourceRect = regexp.MustCompile(`\b(imagerect)=\((-?\d+(?:,-?\d+)+)\)`)
	layoutImage      = regexp.MustCompile(`\bimage=\{([a-fA-F0-9]+),\s*([a-fA-F0-9]+)\}`)
	layoutGeometry   = regexp.MustCompile(`\b(area|imagerect|gutters|tipoffsets)=\((-?\d+(?:,-?\d+)+)\)`)
	layoutFont       = regexp.MustCompile(`\b(?:font|rootfont)=([A-Za-z][A-Za-z0-9_]*)`)
	advisorRoot      = regexp.MustCompile(`(?m)^(<LEGACY [^\r\n]*? id=0x(?:6a15c767|aa15ef06|2a1d96b1) area=)\((-?\d+),(-?\d+),(-?\d+),(-?\d+)\)`)
)

var uiFontID = resourceID{0, 0x4a87bfe8, 0x2a87bffc}

func layoutID(instance uint32) resourceID { return resourceID{0, 0x96a006b0, instance} }

type layoutAtlas struct {
	source  resourceID
	frames  int
	project bool
}

type layoutAsset struct {
	atlas  layoutAtlas
	target resourceID
}

type fontAlias struct{ name, alias string }

type windowLayouts struct {
	group    uint32
	layouts  []resource
	assets   []layoutAsset
	fonts    []fontAlias
	variants map[layoutAtlas]resourceID
	sources  map[resourceID]bool
	aliases  map[string]bool
}

func newWindowLayouts(group uint32) *windowLayouts {
	return &windowLayouts{
		group: group, variants: make(map[layoutAtlas]resourceID),
		sources: make(map[resourceID]bool), aliases: make(map[string]bool),
	}
}

func (w *windowLayouts) add(instance uint32, data []byte, textureScale int, isPNG func(resourceID) (bool, error)) error {
	var result bytes.Buffer
	for _, line := range strings.SplitAfter(string(data), "\n") {
		if !strings.HasPrefix(strings.TrimLeft(line, " \t\r\v\f"), "<LEGACY ") {
			result.WriteString(line)
			continue
		}
		if instance == 0xc973b411 && strings.Contains(line, " id=0x99887766 ") {
			// The Query frame is 37 pixels wide. Give its backing surface the
			// full frame width, as for the Route Query button below.
			const old = "area=(95,85,131,106)"
			if strings.Count(line, old) != 1 {
				return fmt.Errorf("unexpected upper help button geometry")
			}
			line = strings.Replace(line, old, "area=(95,85,132,106)", 1)
		}
		if instance == 0xc973b411 && strings.Contains(line, " id=0x8b96b73e ") {
			const old = "area=(95,106,131,127)"
			if strings.Count(line, old) != 1 {
				return fmt.Errorf("unexpected lower help button geometry")
			}
			line = strings.Replace(line, old, "area=(95,106,132,129)", 1)
		}
		if instance == 0x4a8cc5ea && strings.Contains(line, " id=0x8a8cc775 ") {
			line = strings.Replace(line, " vscrollbar=", " font=GenBodyLight vscrollbar=", 1)
		}
		if instance == 0x8a5ab1cd && strings.Contains(line, "clsid=GZWinTreeView ") {
			line = strings.NewReplacer("icongutter=3", "icongutter=6",
				"rowheight=16", "rowheight=36", "minmaxboxsize=9", "minmaxboxsize=16").Replace(line)
		}
		if instance == 0x4bc906b5 && strings.Contains(line, "image={46a006b0,14015585}") {
			// Align the Environment, Traffic and Education outlines with
			// their centered score fills and the contours on the panel.
			line = strings.NewReplacer(
				"area=(32,67,136,78)", "area=(32,67,133,78)",
				"area=(31,111,132,122)", "area=(32,111,133,122)",
				"area=(194,111,295,122)", "area=(195,111,296,122)",
			).Replace(line)
		}
		driveMinimize := w.group == 0xe74a40be &&
			strings.Contains(line, " id=0x2c16c9cb ") && strings.Contains(line, "image={46a006b0,53244588}")
		if driveMinimize {
			// The Minimize atlas has 21×19 cells, but the stock button is
			// only 18×16. Fit the whole frame inside the HUD.
			area := driveButtonArea.FindStringSubmatch(line)
			root := driveButtonArea.FindStringSubmatch(string(legacyRoot.Find(data)))
			if len(area) != 5 || layoutNumber(area[3])-layoutNumber(area[1]) != 18 ||
				layoutNumber(area[4])-layoutNumber(area[2]) != 16 || len(root) != 5 {
				return fmt.Errorf("%s: unexpected Free Drive Minimize geometry", layoutID(instance))
			}
			left := layoutNumber(area[1])
			height := layoutNumber(root[4]) - layoutNumber(root[2])
			top := min(layoutNumber(area[2]), height-19)
			line = strings.Replace(line, area[0], fmt.Sprintf("area=(%d,%d,%d,%d)",
				left, top, left+21, top+19), 1)
		}
		frames := 1
		if strings.Contains(line, "clsid=GZWinBtn ") {
			frames = 4
		}
		if strings.Contains(line, "style=radiocheck") {
			frames = 8
		}
		// Only buttons and bitmap controls have a verified density adapter.
		// Keep native-rendered artwork in a separate resource group.
		densityControl := strings.Contains(line, "clsid=GZWinBtn ") || strings.Contains(line, "clsid=GZWinBMP ")
		queryClose := w.group == 0xe74a40b8 && frames == 4 && strings.Contains(line, ` caption="Close" `)
		edgePanel := strings.Contains(line, "clsid=GZWinBMP ") && strings.Contains(line, "edgeimage=yes")
		queryWealth := w.group == 0xe74a40b8 && strings.Contains(line, " id=0x00001053 ")
		// The News ticker stretches its hover border with the stock button painter.
		newsHover := instance == 0x2a2aed99 && strings.Contains(line, " id=0xca2aedcd ")
		nativeControl := !densityControl || queryClose || queryWealth || edgePanel || newsHover || budgetLedgerButton(instance, line)
		var imageErr error
		densityImage := false
		line = layoutImage.ReplaceAllStringFunc(line, func(reference string) string {
			if imageErr != nil {
				return reference
			}
			match := layoutImage.FindStringSubmatch(reference)
			group, groupErr := strconv.ParseUint(match[1], 16, 32)
			id, idErr := strconv.ParseUint(match[2], 16, 32)
			if groupErr != nil || idErr != nil {
				imageErr = fmt.Errorf("invalid image reference %s", reference)
				return reference
			}
			source := resourceID{uiImage, uint32(group), uint32(id)}
			if (instance == 0x898897de || instance == 0x2bc90671) &&
				strings.Contains(line, " id=0x8a517556 ") &&
				source == (resourceID{uiImage, catalogFrameGroup, mayorRatingImage}) {
				// The city replaces this bitmap at runtime. Both images
				// use the stock draw path and the same screen-size source rect.
				return fmt.Sprintf("image={%08x,%08x}", mayorRatingGroup, source.instance)
			}
			project := nativeControl || frames == 4 && nativeButtonAtlas(source)
			atlas := layoutAtlas{source: source, frames: frames, project: project}
			target, exists := w.variants[atlas]
			if !exists {
				if !w.sources[source] {
					var found bool
					found, imageErr = isPNG(source)
					if imageErr != nil {
						return reference
					}
					if !found {
						if dynamicLayoutImage(instance, source) {
							return reference
						}
						imageErr = fmt.Errorf("expected PNG for %s", source)
						return reference
					}
				}
				label := match[1] + "," + match[2]
				if w.sources[source] {
					label += ":" + strconv.Itoa(frames)
				}
				targetGroup := w.group
				if project {
					targetGroup += 0x20
				}
				target = resourceID{uiImage, targetGroup, crc32.ChecksumIEEE([]byte(label))}
				w.variants[atlas] = target
				w.sources[source] = true
				w.assets = append(w.assets, layoutAsset{atlas, target})
			}
			densityImage = !project
			return fmt.Sprintf("image={%08x,%08x}", target.group, target.instance)
		})
		if imageErr != nil {
			return fmt.Errorf("%s: %w", layoutID(instance), imageErr)
		}

		line = scaleLayoutGeometry(line, layoutGeometry, nil)
		if densityImage && textureScale == 4 {
			line = scaleLayoutGeometry(line, layoutSourceRect, nil)
		}
		line = layoutFont.ReplaceAllStringFunc(line, func(font string) string {
			attribute, value, _ := strings.Cut(font, "=")
			if attribute == "rootfont" && instance != 0x8a5ab1cd {
				return font
			}
			name := strings.ToLower(value)
			if name == "legend" &&
				(instance == 0xea287193 || instance == 0x2bc9060f || instance == 0x0b72f276) {
				// The runtime legend font already scales to fit the map column.
				return font
			}
			alias := "HD" + name
			if !w.aliases[name] {
				w.fonts = append(w.fonts, fontAlias{name, alias})
				w.aliases[name] = true
			}
			return attribute + "=" + alias
		})
		result.WriteString(line)
	}
	output := result.String()
	if instance == 0x4a160034 || instance == 0xcbc905cd {
		if len(advisorRoot.FindAllStringIndex(output, -1)) != 3 {
			return fmt.Errorf("%s: expected three advisor window roots", layoutID(instance))
		}
		output = advisorRoot.ReplaceAllStringFunc(output, func(root string) string {
			m := advisorRoot.FindStringSubmatch(root)
			return fmt.Sprintf("%s(%s,%d,%s,%d)", m[1], m[2], layoutNumber(m[3])-80, m[4], layoutNumber(m[5])-80)
		})
	}
	w.layouts = append(w.layouts, resource{layoutID(instance), []byte(output)})
	return nil
}

// The budget uses image-backed buttons to paint the ledger rows and
// balance summary. Those frames need the stock paint path at screen density.
func budgetLedgerButton(instance uint32, line string) bool {
	if instance != 0xcbc3c2b9 && instance != 0xaa3acdfe {
		return false
	}
	return strings.Contains(line, "clsid=GZWinBtn ") &&
		(strings.Contains(line, " id=0xaa3ac40") || strings.Contains(line, " id=0xaa3ac50"))
}

func nativeButtonAtlas(id resourceID) bool {
	if id.group != catalogFrameGroup {
		return false
	}
	switch id.instance {
	case 0x144161eb, 0x144161f5, 0x144161f6, 0xe2b66db8, 0x4bc3a5ae, 0xcc1a735d:
		// Shared dialog and keycap frames use stock edge slicing.
		return true
	case 0x13d14c20, 0x13d14c30, 0x13d14c40, 0x13d14c50:
		// Pausing or changing speed repaints these buffered buttons.
		return true
	case 0x140155cb, 0x140155cc, 0x144161ea, 0x53244588, 0x2c201cb0, 0x2c201cb1, 0x2c201cb2:
		// The game also assigns these atlases to controls it creates or updates.
		return true
	}
	return false
}

func dynamicLayoutImage(instance uint32, id resourceID) bool {
	if id.group != catalogFrameGroup {
		return false
	}
	switch instance {
	case 0x0a243d80:
		return id.instance == 0xea32f104
	case 0x4bf325e8, 0xabfaef15:
		return id.instance == 0xea32f104 || id.instance == 0x6b998f30
	}
	return false
}

func layoutNumber(value string) int {
	// Layout coordinates and font sizes are signed decimal fields in the stock data.
	number, _ := strconv.Atoi(value)
	return number
}

func scaleLayoutGeometry(text string, pattern *regexp.Regexp, preserveOrigin func(int) bool) string {
	var result strings.Builder
	start := 0
	for _, indexes := range pattern.FindAllStringSubmatchIndex(text, -1) {
		pos := indexes[0]
		name := text[indexes[2]:indexes[3]]
		raw := strings.Split(text[indexes[4]:indexes[5]], ",")
		coords := make([]int, len(raw))
		for i, value := range raw {
			coords[i] = 2 * layoutNumber(value)
		}
		if name == "area" && preserveOrigin != nil && preserveOrigin(pos) {
			x, y := coords[0]/2, coords[1]/2
			coords[0], coords[1], coords[2], coords[3] = x, y, coords[2]-x, coords[3]-y
		}
		for i, value := range coords {
			raw[i] = strconv.Itoa(value)
		}
		result.WriteString(text[start:pos] + name + "=(" + strings.Join(raw, ",") + ")")
		start = indexes[1]
	}
	result.WriteString(text[start:])
	return result.String()
}

func addLayoutFontAliases(config string, aliases []fontAlias) (string, error) {
	var additions []string
	for _, font := range aliases {
		alias := regexp.MustCompile(`(?im)^\s*` + regexp.QuoteMeta(font.alias) + `[ \t]*=`)
		if alias.MatchString(config) {
			continue
		}
		definition := regexp.MustCompile(`(?im)^` + regexp.QuoteMeta(font.name) +
			`\s*=\s*("[^"\r\n]+"\s*,\s*")([0-9]+)("\s*,\s*"[^"\r\n]*"\s*,\s*)0x[0-9a-f]+`)
		m := definition.FindStringSubmatch(config)
		if m == nil {
			return "", fmt.Errorf("unknown font %q", font.name)
		}
		additions = append(additions, fmt.Sprintf("%s = %s%d%s0x%08x", font.alias,
			m[1], 2*layoutNumber(m[2]), m[3], crc32.ChecksumIEEE([]byte(font.alias))))
	}
	if len(additions) == 0 {
		return config, nil
	}
	at := strings.Index(config, "[Font Aliases]")
	if at < 0 {
		return "", fmt.Errorf("missing Font Aliases section")
	}
	return config[:at] + strings.Join(additions, "\r\n") + "\r\n\r\n" + config[at:], nil
}

func scaleRuntimeFonts(config, stock string, snapshotHelp bool) (string, error) {
	names := []string{
		"NewsHeader", "NewsBody", "NewsBodyUnderline", "AdvisorNewsTitle", "AdvisorName",
		"AdvisorHeadline", "AdvisorNewsBody", "MessageHeader", "MessageBody",
		"MessageBodyUnderline", "ToolTip", "ToolTipTitle",
		"GraphInsetHeader", "GraphInsetLegend", "ChartLabel", "ChartTickText", "Legend",
	}
	if snapshotHelp {
		names = append(names, "SnapshotTaker")
	}
	for _, name := range names {
		definition := regexp.MustCompile(`(?im)^(` + name + `[ \t\v\f]*=[^\r\n]*?",\s*")([0-9]+)("[^\r\n]*)`)
		matches := definition.FindAllStringSubmatch(stock, -1)
		if len(matches) != 1 {
			return "", fmt.Errorf("expected one stock runtime font %q", name)
		}
		m := matches[0]
		size := 2 * layoutNumber(m[2])
		switch name {
		case "GraphInsetLegend", "ChartTickText", "Legend",
			"MessageHeader", "MessageBody", "MessageBodyUnderline", "NewsBody", "NewsBodyUnderline":
			// Constrained legends, ticks, and long messages use a smaller size.
			size = (3*layoutNumber(m[2]) + 1) / 2
		}
		line := m[1] + strconv.Itoa(size) + m[3]
		if strings.Contains(config, line) {
			continue
		}
		if strings.Count(config, m[0]) != 1 {
			return "", fmt.Errorf("unexpected runtime font definition %q", name)
		}
		config = strings.Replace(config, m[0], line, 1)
	}
	return config, nil
}
