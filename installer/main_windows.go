package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if len(os.Args) == 1 {
			showError(err)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func run(args []string) (result error) {
	if len(args) == 2 && args[0] == "--extract-to" {
		return extractTo(args[1])
	}

	logFile, err := openSetupLog()
	logPath := ""
	if err == nil {
		defer logFile.Close()
		logPath = logFile.Name()
		logger := log.New(logFile, "[launcher] ", log.LstdFlags|log.LUTC)
		logger.Print("Setup started (timestamps in UTC)")
		defer func() {
			if result != nil {
				logger.Printf("Setup failed: %v", result)
				result = fmt.Errorf("%w\n\nLog: %s", result, logPath)
			} else {
				logger.Print("Setup closed")
			}
		}()
	}

	var dir string
	switch {
	case len(args) == 2 && args[0] == "--runtime":
		var err error
		dir, err = filepath.Abs(args[1])
		if err != nil {
			return err
		}
	case len(args) == 0:
		var cleanup func()
		var err error
		dir, cleanup, err = setupRuntime()
		if err != nil {
			return err
		}
		defer cleanup()
	default:
		return fmt.Errorf("Use --extract-to DIRECTORY or --runtime DIRECTORY.")
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("Could not find the setup executable: %w", err)
	}
	ui := exec.Command(filepath.Join(dir, "qt", "Setup-UI.exe"),
		filepath.Join(dir, "app.exe"), filepath.Join(dir, "setup.json"))
	ui.Dir = dir
	ui.Env = append(ui.Environ(), "SETUP_EXE_DIR="+filepath.Dir(executable))
	if logFile != nil {
		ui.Stdout, ui.Stderr = logFile, logFile
	}
	if err := ui.Run(); err != nil {
		return fmt.Errorf("Could not open setup: %w", err)
	}
	return nil
}

func openSetupLog() (*os.File, error) {
	folders := []string{filepath.Join(os.TempDir(), "SimCity4HD-Logs")}
	if local := os.Getenv("LOCALAPPDATA"); filepath.IsAbs(local) {
		folders = append([]string{filepath.Join(local, "SimCity4HD", "Logs")}, folders...)
	}
	var err error
	for _, dir := range folders {
		if err = os.MkdirAll(dir, 0700); err != nil {
			continue
		}
		var f *os.File
		f, err = os.CreateTemp(dir, "setup-*.log")
		if err == nil {
			return f, nil
		}
	}
	return nil, err
}

func showError(err error) {
	message, _ := syscall.UTF16PtrFromString(err.Error())
	title, _ := syscall.UTF16PtrFromString("Setup")
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(
		0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
}

func extractTo(dir string) error {
	if err := os.Mkdir(dir, 0755); err != nil {
		return err
	}
	if err := unpackRuntime(dir); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	return nil
}
