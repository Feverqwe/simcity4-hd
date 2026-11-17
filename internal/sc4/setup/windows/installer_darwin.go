package windows

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

const foreignGame = true

func ensureGameClosed() error {
	err := exec.Command("/usr/bin/pgrep", "-if", "(^|[/\\\\ ])SimCity 4\\.exe( |$)").Run()
	if err == nil {
		return errors.New("Close the running SimCity 4 game before continuing.")
	}
	var status *exec.ExitError
	if errors.As(err, &status) && status.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("check for a running SimCity 4 game: %w", err)
}

func moveSetupPath(source, target string, replace bool) error {
	if !replace {
		if _, err := os.Lstat(target); err == nil {
			return &os.LinkError{Op: "move", Old: source, New: target, Err: os.ErrExist}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Rename(source, target)
}
