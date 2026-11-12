package sc4

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	rendererDLL = "SCD3D11.dll"
	rendererINI = "SCD3D11.ini"
	graphicsDLL = "SC4GraphicsOptions.dll"
	graphicsINI = "SC4GraphicsOptions.ini"
)

func executableHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func modInstalled(game string) bool {
	plugins := filepath.Join(game, "Plugins")
	record, err := os.ReadFile(filepath.Join(plugins, modFolder, modReceipt))
	if err != nil {
		return false
	}
	exe, err := os.ReadFile(filepath.Join(game, "Apps", gameEXE))
	if err != nil || string(record) != executableHash(exe) {
		return false
	}
	for _, name := range []string{rendererDLL, rendererINI, graphicsDLL, graphicsINI} {
		if !fileExists(filepath.Join(plugins, name)) {
			return false
		}
	}
	return true
}

var moveFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func moveSetupPath(source, target string, replace bool) error {
	from, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	const writeThrough = 0x8
	const replaceExisting = 0x1
	flags := uintptr(writeThrough)
	if replace {
		flags |= replaceExisting
	}
	ok, _, err := moveFileEx.Call(uintptr(unsafe.Pointer(from)), uintptr(unsafe.Pointer(to)), flags)
	if ok == 0 {
		return &os.LinkError{Op: "move", Old: source, New: target, Err: err}
	}
	return nil
}

// Set the resolution in the graphics section without resetting other settings.
func setGraphicsResolution(config string, width, height int) string {
	lines := strings.Split(config, "\n")
	var result strings.Builder
	found, inGraphics := false, false
	writeMode := func() {
		fmt.Fprintf(&result, "WindowMode=FullScreen\r\nWindowWidth=%d\r\nWindowHeight=%d\r\n", width, height)
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inGraphics = strings.EqualFold(trimmed, "[GraphicsOptions]")
			if inGraphics {
				found = true
				result.WriteString(line + "\n")
				writeMode()
				continue
			}
		}
		if inGraphics {
			key, _, _ := strings.Cut(trimmed, "=")
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "windowmode", "windowwidth", "windowheight":
				continue
			}
		}
		result.WriteString(line + "\n")
	}
	if !found {
		result.WriteString("[GraphicsOptions]\r\n")
		writeMode()
	}
	return result.String()
}

func publishMod(ctx context.Context, game, work, stage, patched string, stock []byte, renderer []string, graphics string, options setupOptions) (result error) {
	apps, plugins := filepath.Join(game, "Apps"), filepath.Join(game, "Plugins")
	exe := filepath.Join(apps, gameEXE)
	hash := stockEXEHash
	if patched != "" {
		data, err := os.ReadFile(patched)
		if err != nil {
			return err
		}
		hash = executableHash(data)
	}
	if err := os.WriteFile(filepath.Join(stage, modReceipt), []byte(hash), 0644); err != nil {
		return err
	}

	rendererConfig, err := os.ReadFile(renderer[1])
	if err != nil {
		return err
	}
	if options.drawStats {
		rendererConfig = bytes.Replace(rendererConfig, []byte("DrawStats=off"), []byte("DrawStats=on"), 1)
	}
	rendererSettings := filepath.Join(work, rendererINI)
	if err := os.WriteFile(rendererSettings, rendererConfig, 0644); err != nil {
		return err
	}
	graphicsSettings := filepath.Join(work, graphicsINI)
	config := fmt.Sprintf("[GraphicsOptions]\r\nDriver=DirectX\r\nColorDepth=32\r\nEnableIntroVideo=%t\r\n", !options.skipIntro)
	graphicsTarget := filepath.Join(plugins, graphicsINI)
	var oldGraphics []byte
	if info, err := os.Lstat(graphicsTarget); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", graphicsTarget)
		}
		oldGraphics, err = os.ReadFile(graphicsTarget)
		if err != nil {
			return err
		}
		config = string(oldGraphics)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	config = setGraphicsResolution(config, options.width, options.height)
	if err := os.WriteFile(graphicsSettings, []byte(config), 0644); err != nil {
		return err
	}

	var files []string
	var installedFiles []string
	for _, source := range []string{renderer[0], rendererSettings, graphics, graphicsSettings} {
		name := filepath.Base(source)
		target := filepath.Join(plugins, name)
		if info, err := os.Lstat(target); err == nil {
			if name == graphicsINI && oldGraphics == nil {
				return errors.New("The graphics settings appeared during installation. Try again.")
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("%s is not a regular file", target)
			}
			if strings.EqualFold(filepath.Ext(name), ".dll") {
				actual, err := os.ReadFile(target)
				if err != nil {
					return err
				}
				expected, err := os.ReadFile(source)
				if err != nil {
					return err
				}
				if !bytes.Equal(actual, expected) {
					return fmt.Errorf("A different %s is already installed. Move it out of Plugins before continuing.", name)
				}
			}
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if name == graphicsINI && oldGraphics != nil {
			return errors.New("The graphics settings disappeared during installation. Try again.")
		}
		local := filepath.Join(work, "stage-"+name)
		if err := copyRegular(source, local); err != nil {
			return err
		}
		files = append(files, local)
		installedFiles = append(installedFiles, name)
	}
	if err := os.WriteFile(filepath.Join(stage, installedFilesFile), []byte(strings.Join(installedFiles, "\n")), 0644); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ensureGameNotRunning(); err != nil {
		return err
	}
	current, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, stock) {
		return errors.New("The game executable changed while setup was running. Verify the game files in Steam and try again.")
	}
	if err := os.MkdirAll(plugins, 0755); err != nil {
		return err
	}

	// Keep the original executable in place until the other files are ready.
	// Roll back files this run published if the final replacement fails.
	var published []string
	defer func() {
		if result != nil && len(published) > 0 {
			fmt.Fprintf(os.Stderr, "Rolling back %d changes: %v\n", len(published), result)
			for i := len(published) - 1; i >= 0; i-- {
				result = errors.Join(result, os.RemoveAll(published[i]))
			}
		}
	}()
	if patched != "" {
		backup := filepath.Join(apps, stockBackup)
		original, err := os.ReadFile(backup)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := publishFile(backup, func(w io.Writer) error {
				_, err := w.Write(stock)
				return err
			}); err != nil {
				return err
			}
			published = append(published, backup)
			fmt.Fprintf(os.Stderr, "Saved original game executable: %s\n", backup)
		case err != nil:
			return err
		case !bytes.Equal(original, stock):
			return fmt.Errorf("The existing executable backup %s does not match the original game.", backup)
		}
	}
	for _, source := range files {
		target := filepath.Join(plugins, strings.TrimPrefix(filepath.Base(source), "stage-"))
		if err := moveSetupPath(source, target, false); err != nil {
			return err
		}
		published = append(published, target)
	}
	destination := filepath.Join(plugins, modFolder)
	if err := moveSetupPath(stage, destination, false); err != nil {
		return err
	}
	published = append(published, destination)
	if oldGraphics != nil {
		current, err := os.ReadFile(graphicsTarget)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, oldGraphics) {
			return errors.New("The graphics settings changed during installation. Try again.")
		}
		backup := filepath.Join(work, "previous-"+graphicsINI)
		if err := os.WriteFile(backup, oldGraphics, 0644); err != nil {
			return err
		}
		if err := moveSetupPath(graphicsSettings, graphicsTarget, true); err != nil {
			return err
		}
		defer func() {
			if result != nil {
				result = errors.Join(result, moveSetupPath(backup, graphicsTarget, true))
			}
		}()
	}
	if patched != "" {
		return moveSetupPath(patched, exe, true)
	}
	return nil
}
