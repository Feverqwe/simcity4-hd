package sc4

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const defaultResolution = "3840x2160"

var resolutionPattern = regexp.MustCompile(`^[1-9][0-9]{2,4}x[1-9][0-9]{2,4}$`)

func launchPaths(game, profile string) (exe, directory, userDir string, err error) {
	game, err = filepath.Abs(game)
	if err != nil {
		return "", "", "", err
	}
	game, err = filepath.EvalSymlinks(game)
	if err != nil {
		return "", "", "", err
	}
	profile, err = filepath.Abs(profile)
	if err != nil {
		return "", "", "", err
	}
	profile, err = filepath.EvalSymlinks(profile)
	if err != nil {
		return "", "", "", err
	}
	relative, relErr := filepath.Rel(game, profile)
	if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", "", errors.New("use a separate profile directory whose path contains no spaces")
	}
	for _, name := range []string{"Plugins", "Regions"} {
		info, err := os.Stat(filepath.Join(profile, name))
		if err != nil || !info.IsDir() {
			return "", "", "", fmt.Errorf("isolated profile is missing the %s directory", name)
		}
	}
	directory = filepath.Join(game, "Apps")
	exe = filepath.Join(directory, "SimCity 4.exe")
	info, err := os.Stat(exe)
	if err != nil || !info.Mode().IsRegular() {
		return "", "", "", errors.New("game executable is missing")
	}
	userDir, err = gameProfilePath(profile)
	return exe, directory, userDir, err
}

func startIsolatedGame(exe, directory string, args []string) (int, error) {
	if runtime.GOOS != "windows" {
		return 0, errors.New("launch requires Windows")
	}
	if err := ensureGameNotRunning(); err != nil {
		return 0, err
	}
	command := exec.Command(exe, args...)
	command.Dir = directory
	command.Env = append(os.Environ(), "SteamAppId=24780", "SteamGameId=24780")
	if err := command.Start(); err != nil {
		return 0, err
	}
	pid := command.Process.Pid
	return pid, command.Process.Release()
}

func ensureGameNotRunning() error {
	tasklist := exec.Command("tasklist.exe", "/FO", "CSV", "/NH",
		"/FI", "IMAGENAME eq SimCity 4.exe")
	configureBackgroundProcess(tasklist)
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

func runLaunch(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("launch", flag.ContinueOnError)
	start := flags.Bool("start", false, "start the game instead of previewing the command")
	resolution := flags.String("resolution", defaultResolution, "custom game resolution")
	savedResolution := flags.Bool("saved-resolution", false, "use the game's saved resolution")
	showIntro := flags.Bool("show-intro", false, "play the game intro")
	drawStats := flags.Bool("draw-stats", false, "display renderer statistics")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 2 {
		return errors.New("usage: simcity4-uhd launch [OPTIONS] GAME_COPY PROFILE")
	}
	explicitResolution := false
	flags.Visit(func(f *flag.Flag) {
		explicitResolution = explicitResolution || f.Name == "resolution"
	})
	if *savedResolution && explicitResolution {
		return errors.New("choose the saved resolution or an explicit resolution")
	}
	if !resolutionPattern.MatchString(*resolution) {
		return errors.New("invalid resolution, expected WIDTHxHEIGHT")
	}
	exe, directory, profile, err := launchPaths(flags.Arg(0), flags.Arg(1))
	if err != nil {
		return err
	}
	gameArgs := []string{"-d:DirectX"}
	if !*savedResolution {
		gameArgs = append(gameArgs, "-CustomResolution:enabled", "-r"+*resolution+"x32")
	}
	gameArgs = append(gameArgs, "-f", "-Borderless")
	if !*showIntro {
		gameArgs = append(gameArgs, "-intro:off")
	}
	if *drawStats {
		gameArgs = append(gameArgs, "-DrawStats:on")
	}
	gameArgs = append(gameArgs, "-CPUcount:1", "-UserDir:"+strings.TrimRight(profile, `\/`)+`\`)
	if *start {
		pid, err := startIsolatedGame(exe, directory, gameArgs)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Started SimCity 4 (PID %d)\n", pid)
		return nil
	}
	file, err := os.Open(exe)
	if err != nil {
		return err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return err
	}
	details := struct {
		Executable, SHA256, WorkingDirectory, Profile, Arguments string
	}{exe, fmt.Sprintf("%X", digest.Sum(nil)), directory, profile, strings.Join(gameArgs, " ")}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(details)
}
