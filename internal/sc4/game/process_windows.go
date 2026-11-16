package game

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func CheckGameLaunch(mac bool) error {
	if mac {
		return errors.New("launch this game on macOS")
	}
	return EnsureGameNotRunning()
}

func ConfigureBackgroundProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

func EnsureGameNotRunning() error {
	tasklist := exec.Command("tasklist.exe", "/FO", "CSV", "/NH",
		"/FI", "IMAGENAME eq SimCity 4.exe")
	ConfigureBackgroundProcess(tasklist)
	processes, err := tasklist.Output()
	if err != nil {
		return fmt.Errorf("check for a running SimCity 4 session: %w", err)
	}
	list := csv.NewReader(strings.NewReader(string(processes)))
	list.FieldsPerRecord = -1
	rows, err := list.ReadAll()
	if err != nil {
		return fmt.Errorf("read Windows process list: %w", err)
	}
	for _, row := range rows {
		if len(row) != 0 && strings.EqualFold(row[0], "SimCity 4.exe") {
			return errors.New("close the current SimCity 4 session before continuing")
		}
	}
	return nil
}
