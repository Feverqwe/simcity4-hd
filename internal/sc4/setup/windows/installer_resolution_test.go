//go:build windows || darwin

package windows

import (
	"strconv"
	"testing"
)

func TestAutomaticResolutionAndRestore(t *testing.T) {
	const section = "GraphicsOptions"
	fresh := make(setupINIChanges).edit(graphicsINI, "", setupOptions{}, true, false)
	if mode := setupINIValue(fresh, section, "WindowMode"); mode != "BorderlessFullScreen" {
		t.Fatalf("an automatic install must use the primary monitor in borderless mode, got %q", mode)
	}
	width, _ := strconv.Atoi(setupINIValue(fresh, section, "WindowWidth"))
	height, _ := strconv.Atoi(setupINIValue(fresh, section, "WindowHeight"))
	if width < 800 || height < 600 {
		t.Fatalf("leaving borderless mode needs a usable saved resolution, got %dx%d", width, height)
	}

	original := "[GraphicsOptions]\nWindowMode=Windowed\nWindowWidth=1280\nWindowHeight=720\n"
	changes := make(setupINIChanges)
	automatic := changes.edit(graphicsINI, original, setupOptions{}, false, true)
	if mode := setupINIValue(automatic, section, "WindowMode"); mode != "BorderlessFullScreen" {
		t.Fatalf("changing to automatic must enable borderless mode, got %q", mode)
	}
	if setupINIValue(automatic, section, "WindowWidth") != "1280" || setupINIValue(automatic, section, "WindowHeight") != "720" {
		t.Fatalf("automatic mode must preserve the existing fixed resolution, got:\n%s", automatic)
	}
	fixed := changes.edit(graphicsINI, automatic, setupOptions{width: 1920, height: 1080}, false, true)
	if mode := setupINIValue(fixed, section, "WindowMode"); mode != "FullScreen" ||
		setupINIValue(fixed, section, "WindowWidth") != "1920" || setupINIValue(fixed, section, "WindowHeight") != "1080" {
		t.Fatalf("choosing a fixed size after automatic must apply it, got:\n%s", fixed)
	}
	restored := changes.restore(graphicsINI, fixed)
	if setupINIValue(restored, section, "WindowMode") != "Windowed" ||
		setupINIValue(restored, section, "WindowWidth") != "1280" || setupINIValue(restored, section, "WindowHeight") != "720" {
		t.Fatalf("removing the mod must restore the original display settings, got:\n%s", restored)
	}
}
