package game

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackagePathsKeepsDistinctFilesWithCaseCollidingNames(t *testing.T) {
	root := t.TempDir()
	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("HUD.dat")
	if _, err := os.Stat(filepath.Join(root, "hud.dat")); err == nil {
		t.Skip("the filesystem does not distinguish filenames by case")
	}
	write("hud.dat")

	packages, err := PackagePaths([]string{root})
	if err != nil || len(packages) != 2 {
		t.Fatalf("expected both packages, got %v (%v)", packages, err)
	}
}
