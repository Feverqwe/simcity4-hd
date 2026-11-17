//go:build windows || darwin

package sc4

import (
	"os"
	"path/filepath"
	"testing"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
)

func TestResolveInstallerGameFromSelectedFile(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	windows := filepath.Join(root, "Windows copy.app")
	steam := filepath.Join(root, "Aspyr Steam folder")
	aspyr := filepath.Join(steam, "Sim City 4 Deluxe Edition.app")
	winExe := gamefiles.ExecutablePath(windows, false)
	macExe := gamefiles.ExecutablePath(aspyr, true)
	for _, file := range []string{
		winExe, filepath.Join(windows, "SimCity_1.dat"),
		macExe, filepath.Join(aspyr, "Contents", "GameData", "SimCity_1.dat"),
		filepath.Join(windows, "Apps", "Other.exe"),
	} {
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ selection, game, target string }{
		{windows, windows, "windows"},
		{winExe, windows, "windows"},
		{aspyr, aspyr, "aspyr"},
		{steam, aspyr, "aspyr"},
		{macExe, aspyr, "aspyr"},
	} {
		game, target, err := resolveInstallerGame(tc.selection)
		if err != nil || game != tc.game || target != tc.target {
			t.Errorf("selected %q: got %q (%q), %v; want %q (%q)", tc.selection, game, target, err, tc.game, tc.target)
		}
	}
	if _, _, err := resolveInstallerGame(filepath.Join(windows, "Apps", "Other.exe")); err == nil {
		t.Error("an unrelated executable must not select a game")
	}
	if err := os.MkdirAll(filepath.Join(windows, "Contents", "GameData"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveInstallerGame(winExe); err == nil {
		t.Error("a Windows EXE in a mixed layout must not be handed to the Aspyr texture builder")
	}
}
