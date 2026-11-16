package sc4

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	keyConfigType = 0xa2e3d533
	// The game tries group + language before the base group. Give each
	// HD string a distinct instance in a single base group.
	hdUITextGroup  = 0xe74a40da
	keyHelpTitle   = 0xe74a43e1
	keyHelpContent = 0xe74a43e2

	playOptionsLayout = 0x0a7df315
	keyHelpOpen       = 0xe74a4301
	keyHelpPanel      = 0xe74a4302
	keyHelpText       = 0xe74a4303
	keyHelpClose      = 0xe74a4304
)

//go:embed localization/ru/keyboard-help.json
var russianKeyHelpData []byte

//go:embed patchsrc/ui_keyboard_help.i386
var keyboardHelpCode []byte

func appendKeyboardHelpPatch(stub []byte, patches []instructionPatch, base int) ([]byte, []instructionPatch) {
	const (
		initSite    = 0x36afe3 // EAX holds the Play Options root after wiring Credits.
		messageSite = 0x36b562 // The native listener has checked the source and visibility.
	)
	const imageBase = 0x400000
	ids := imageBase + base + len(stub)
	for _, id := range []uint32{keyHelpOpen, keyHelpPanel, keyHelpText, keyHelpClose, hdUITextGroup, keyHelpContent} {
		stub = little.AppendUint32(stub, id)
	}
	entry := base + len(stub)
	stub = append(stub, keyboardHelpCode...)

	initBody := little.AppendUint32([]byte{0x60, 0x68}, uint32(ids))
	initBody = append(initBody, 0x6a, 0, 0x6a, 0, 0x6a, 0, 0x50) // No event, just the root.
	initBody = append(initBody, relativeInstruction(0xe8, base+len(stub)+len(initBody), entry)...)
	initOriginal := hexBytes("8bf88b442428")
	initBody = append(append(initBody, hexBytes("83c41461")...), initOriginal...)
	var initPatch instructionPatch
	stub, initPatch = appendStub(stub, base, initSite, initOriginal, initBody)

	body := hexBytes("8b4c241060") // Load the window message and save caller registers.
	for _, value := range []uint32{uint32(ids), 0x414250, 0x603580} {
		body = little.AppendUint32(append(body, 0x68), value)
	}
	body = append(body, 0x51, 0x57) // Message and root window.
	body = append(body, relativeInstruction(0xe8, base+len(stub)+len(body), entry)...)
	body = append(body, hexBytes("83c41484c0617405")...)
	body = append(body, relativeInstruction(0xe9, base+len(stub)+len(body), 0x36b592)...)
	original := hexBytes("8b4c24108b1132c0")
	body = append(body, original...)
	var patch instructionPatch
	stub, patch = appendStub(stub, base, messageSite, original, body)
	return stub, append(patches, initPatch, patch)
}

func keyboardHelpLayout(stock uiStock, options []byte) ([]byte, error) {
	credits, err := stock.read(layoutID(0xca551016))
	if err != nil {
		return nil, err
	}
	album, err := stock.read(layoutID(0x4a8cc5ea))
	if err != nil {
		return nil, err
	}
	var readOnly string
	for _, line := range strings.Split(string(album), "\n") {
		if strings.Contains(line, "clsid=GZWinTextEdit ") && strings.Contains(line, " id=0xaa8cc64e ") {
			readOnly = strings.NewReplacer(
				"id=0xaa8cc64e", fmt.Sprintf("id=0x%08x", keyHelpText),
				"area=(322,156,541,297)", "area=(2,0,492,208)",
				"font=GenBodyLight ", "", // The template declares GenBodyMedium later.
				"maxtext=2048", "maxtext=32767",
			).Replace(strings.TrimSpace(line))
			break
		}
	}
	const creditsButton = " id=0x0a57da65 "
	const creditsArea = "area=(-1,174,269,204)"
	const creditsCaption = `caption="Credits" captionres={6a231eaa,4a5d6466}`
	title := fmt.Sprintf(`caption="Keyboard Shortcuts" captionres={%08x,%08x}`, hdUITextGroup, keyHelpTitle)
	var settings strings.Builder
	foundButton := false
	for _, line := range strings.SplitAfter(string(options), "\n") {
		if strings.Contains(line, creditsButton) && strings.Contains(line, creditsArea) {
			foundButton = true
			settings.WriteString(strings.Replace(line, creditsArea, "area=(-1,174,157,204)", 1))
			line = strings.NewReplacer(
				creditsButton, fmt.Sprintf(" id=0x%08x ", keyHelpOpen),
				creditsArea, "area=(165,174,370,204)",
				creditsCaption, title,
			).Replace(line)
		}
		settings.WriteString(line)
	}
	creditsRoot := legacyRoot.FindIndex(credits)
	if creditsRoot == nil {
		return nil, fmt.Errorf("Credits has no root window")
	}
	panel := string(credits[creditsRoot[0]:])
	var modal strings.Builder
	foundBody := false
	for _, line := range strings.Split(panel, "\n") {
		switch {
		case strings.Contains(line, " id=0x0a592004 "):
			line = strings.NewReplacer(
				"id=0x0a592004", fmt.Sprintf("id=0x%08x", keyHelpPanel),
				"area=(121,45,646,329)", "area=(87,119,612,403)",
				"winflag_visible=yes", "winflag_visible=no",
				"winflag_moveable=yes", "winflag_moveable=no",
				" moveable=yes", " moveable=no",
				"closevisible=yes", "closevisible=no",
			).Replace(line)
		case strings.Contains(line, " id=0x0a5d0e11 "):
			line = strings.Replace(line, creditsCaption, title, 1)
			line = strings.Replace(line, " id=0x0a5d0e11 ", " ", 1)
		case strings.Contains(line, " id=0x0a5d0e13 "):
			foundBody = true
			line = "      " + readOnly
		case strings.Contains(line, " id=0xea59207d "):
			line = strings.Replace(line, "id=0xea59207d", fmt.Sprintf("id=0x%08x", keyHelpClose), 1)
		}
		if trimmed := strings.TrimRight(line, "\r"); trimmed != "" {
			modal.WriteString("   " + trimmed + "\r\n")
		}
	}
	layout := settings.String()
	rootEnd := strings.LastIndex(layout, "</CHILDREN>")
	if !foundButton || !foundBody || readOnly == "" || rootEnd < 0 {
		return nil, fmt.Errorf("unexpected Play Options, Credits or Photo Album layout")
	}
	return []byte(layout[:rootEnd] + modal.String() + layout[rootEnd:]), nil
}

type shortcut struct {
	keys []string
	name string
}

func parseShortcuts(config string, mac bool) []shortcut {
	var rows []shortcut
	index := make(map[string]int)
	for _, line := range strings.Split(config, "\n") {
		binding, command, ok := strings.Cut(strings.TrimSpace(line), "=")
		binding = strings.TrimSpace(binding)
		if !ok || strings.HasPrefix(binding, ";") || strings.HasSuffix(binding, " up") ||
			strings.HasPrefix(binding, "0xFFF") {
			continue
		}
		action, title, ok := strings.Cut(command, `"`)
		if !ok {
			continue
		}
		title, _, _ = strings.Cut(title, `"`)
		key := strings.Join(strings.Fields(action), " ") + "|" + title
		label := displayShortcut(binding, mac)
		if i, found := index[key]; found {
			rows[i].keys = append(rows[i].keys, label)
		} else {
			index[key] = len(rows)
			rows = append(rows, shortcut{[]string{label}, title})
		}
	}
	return rows
}

var shortcutDisplay = strings.NewReplacer(
	"Control", "Ctrl", "Numpad", "Num ", "Numlock", "Num Lock",
	"PageUp", "Page Up", "PageDown", "Page Down",
	"Return", "Enter", "Escape", "Esc",
)

func displayShortcut(binding string, mac bool) string {
	if binding == "?" || binding == "Alt ?" {
		binding = strings.Replace(binding, "?", "/", 1)
	}
	keys := strings.Fields(binding)
	if mac {
		for i, key := range keys {
			if key == "Alt" {
				keys[i] = "Option"
			}
		}
	}
	return shortcutDisplay.Replace(strings.Join(keys, " + "))
}

func keyboardHelpEntries(stock uiStock, russian, mac bool) ([]resource, error) {
	var translations map[string]string
	if russian {
		if err := json.Unmarshal(russianKeyHelpData, &translations); err != nil {
			return nil, fmt.Errorf("keyboard shortcuts localization: %w", err)
		}
	}
	translate := func(label string) string {
		if translated := translations[label]; translated != "" {
			return translated
		}
		return label
	}
	var text strings.Builder
	for _, mode := range []struct {
		config uint32
		title  string
	}{
		{0x6a9362ef, "City"},
		{0x6a9362f0, "Region"},
		{0x6a9362f1, "General"},
		{0x0c0eaad0, "Driving"},
	} {
		config, err := stock.read(resourceID{keyConfigType, 0x6a231eaa, mode.config})
		if err != nil {
			return nil, err
		}
		rows := parseShortcuts(string(config), mac)
		if len(rows) == 0 {
			return nil, fmt.Errorf("empty shortcut configuration %08x", mode.config)
		}
		text.WriteString(translate(mode.title) + "\n\n")
		for _, row := range rows {
			fmt.Fprintf(&text, "%s  —  %s\n", strings.Join(row.keys, " | "), translate(row.name))
		}
		text.WriteByte('\n')
	}
	// These actions are documented in the Deluxe manual and use the current
	// tool or vehicle directly rather than binding a KEYCFG command.
	for _, section := range []struct {
		title string
		rows  [][2]string
	}{
		{"Terrain tools", [][2]string{
			{"Shift", "Increase brush size and strength"},
			{"Ctrl", "Decrease brush size and strength"},
			{"Shift + 1–0", "Set brush size"},
			{"Shift + F1–F10", "Set brush strength"},
		}},
		{"Vehicle-specific actions", [][2]string{
			{"Shift", "Toggle road snapping for cars and tanks"},
			{"Home / E / Num 9", "Climb in aircraft or raise the tank barrel"},
			{"End / C / Num 3", "Descend in aircraft or lower the tank barrel"},
			{"Right mouse button", "Aim the fire hose, police searchlight or tank turret"},
			{"Left mouse button", "Fire from a military aircraft or tank"},
			{"Space", "Sound the train whistle"},
			{"Enter", "Change the train's automatic track switching"},
			{"Right mouse button", "Switch tracks manually while driving a train"},
		}},
	} {
		text.WriteString(translate(section.title) + "\n\n")
		for _, row := range section.rows {
			fmt.Fprintf(&text, "%s  —  %s\n", translate(row[0]), translate(row[1]))
		}
		text.WriteByte('\n')
	}
	return []resource{
		{resourceID{0x2026960b, hdUITextGroup, keyHelpTitle}, encodeLocaleText(translate("Keyboard Shortcuts"))},
		{resourceID{0x2026960b, hdUITextGroup, keyHelpContent}, encodeLocaleText(strings.TrimSpace(text.String()))},
	}, nil
}
