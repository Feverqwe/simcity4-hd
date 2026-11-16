package game

import (
	"errors"
	"fmt"
	"os/exec"
)

func CheckGameLaunch(mac bool) error {
	if !mac {
		return errors.New("launch this game on Windows")
	}
	command := exec.Command("/usr/bin/pgrep", "-f", "(^|/)"+MacGameExecutable+"( |$)")
	err := command.Run()
	if err == nil {
		return errors.New("close the current SimCity 4 session before continuing")
	}
	var status *exec.ExitError
	if errors.As(err, &status) && status.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("check for a running SimCity 4 session: %w", err)
}
