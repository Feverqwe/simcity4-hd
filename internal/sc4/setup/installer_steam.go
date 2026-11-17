//go:build windows || darwin

package setup

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var steamLibraryPath = regexp.MustCompile(`(?mi)^\s*"path"[ \t]+"((?:[^"\\]|\\.)+)"`)
var steamPathUnescape = strings.NewReplacer(`\\`, `\`, `\"`, `"`)

func FindSteamGame(roots []string, resolve func(string) string) string {
	for _, root := range roots {
		if root == "" {
			continue
		}
		libraries := []string{root}
		if data, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf")); err == nil {
			for _, match := range steamLibraryPath.FindAllStringSubmatch(string(data), -1) {
				libraries = append(libraries, steamPathUnescape.Replace(match[1]))
			}
		}
		for _, library := range libraries {
			game := filepath.Join(library, "steamapps", "common", "SimCity 4 Deluxe")
			if found := resolve(game); found != "" {
				return found
			}
		}
	}
	return ""
}
