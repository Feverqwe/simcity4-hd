package macos

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
)

func FindSteamGame() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	steam := filepath.Join(home, "Library", "Application Support", "Steam")
	return setup.FindSteamGame([]string{steam}, FindBundle)
}

func ensureMacSetupGameClosed() error {
	err := exec.Command("/usr/bin/pgrep", "-f", "(^|/)Sim City 4 Deluxe Edition(sub)?( |$)").Run()
	var status *exec.ExitError
	switch {
	case err == nil:
		return errors.New("Close SimCity 4 before changing the installation.")
	case errors.As(err, &status) && status.ExitCode() == 1:
		return nil
	default:
		return fmt.Errorf("check whether SimCity 4 is running: %w", err)
	}
}
