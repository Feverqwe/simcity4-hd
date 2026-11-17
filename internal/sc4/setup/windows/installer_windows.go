package windows

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
)

func steamPath(root syscall.Handle, key, name string) string {
	var handle syscall.Handle
	if syscall.RegOpenKeyEx(root, syscall.StringToUTF16Ptr(key), 0, syscall.KEY_READ, &handle) != nil {
		return ""
	}
	defer syscall.RegCloseKey(handle)
	var size, kind uint32
	value := syscall.StringToUTF16Ptr(name)
	if syscall.RegQueryValueEx(handle, value, nil, &kind, nil, &size) != nil ||
		kind != syscall.REG_SZ || size < 2 || size > 65536 || size%2 != 0 {
		return ""
	}
	buf := make([]uint16, size/2)
	if syscall.RegQueryValueEx(handle, value, nil, &kind, (*byte)(unsafe.Pointer(&buf[0])), &size) != nil {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func FindSteamGame() string {
	roots := []string{
		steamPath(syscall.HKEY_CURRENT_USER, `Software\Valve\Steam`, "SteamPath"),
		steamPath(syscall.HKEY_LOCAL_MACHINE, `Software\WOW6432Node\Valve\Steam`, "InstallPath"),
	}
	if programFiles := os.Getenv("ProgramFiles(x86)"); programFiles != "" {
		roots = append(roots, filepath.Join(programFiles, "Steam"))
	}
	return setup.FindSteamGame(roots, func(game string) string {
		if setup.IsRegularFile(gamefiles.ExecutablePath(game, false)) &&
			setup.IsRegularFile(filepath.Join(game, "SimCity_1.dat")) {
			return game
		}
		return ""
	})
}

const foreignGame = false

func ensureGameClosed() error { return gamefiles.EnsureGameNotRunning() }

func moveSetupPath(source, target string, replace bool) error {
	return setup.MovePath(source, target, replace)
}
