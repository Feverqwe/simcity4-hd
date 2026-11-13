package sc4

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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
	data, mac, err := gameDataDirectory(game)
	if err != nil {
		return "", "", "", err
	}
	if mac {
		directory = data
	}
	exe = gameExecutablePath(game, mac)
	info, err := os.Stat(exe)
	if err != nil || !info.Mode().IsRegular() {
		return "", "", "", errors.New("game executable is missing")
	}
	userDir, err = gameProfilePath(profile)
	return exe, directory, userDir, err
}

func startIsolatedGame(exe, directory, profile, resolution string, args []string, foreground bool, out io.Writer) error {
	mac := filepath.Base(exe) == macGameExecutable
	if err := checkGameLaunch(mac); err != nil {
		return err
	}
	command := exec.Command(exe, args...)
	command.Dir = directory
	command.Env = append(os.Environ(), "SteamAppId=24780", "SteamGameId=24780")
	if mac {
		data, err := os.ReadFile(exe)
		if err != nil {
			return err
		}
		end := macLaunchStub + len(macLaunchCode)
		if end > len(data) || !bytes.Equal(data[macLaunchStub:end], macLaunchCode) {
			return errors.New("stage a new macOS copy to enable isolated launch")
		}
		modeEnd := macDisplayModeStub + len(macDisplayModeCode)
		if modeEnd > len(data) || !bytes.Equal(data[macDisplayModeStub:modeEnd], macDisplayModeCode) {
			return errors.New("stage a new macOS copy to enable custom display modes")
		}
		command.Env = append(command.Env, "SC4_UHD_PROFILE="+profile, "SC4_UHD_ARGUMENTS= "+strings.Join(args, " "), "SC4_UHD_RESOLUTION="+resolution)
	}
	if foreground {
		command.Stdout = out
		command.Stderr = out
	}
	if err := command.Start(); err != nil {
		return err
	}
	fmt.Fprintf(out, "Started SimCity 4 (PID %d)\n", command.Process.Pid)
	if foreground {
		return command.Wait()
	}
	return command.Process.Release()
}

func runLaunch(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("launch", flag.ContinueOnError)
	start := flags.Bool("start", false, "start the game instead of previewing the command")
	foreground := flags.Bool("foreground", false, "wait for the game and show its output")
	resolution := flags.String("resolution", defaultResolution, "custom game resolution")
	savedResolution := flags.Bool("saved-resolution", false, "use the game's saved resolution")
	showIntro := flags.Bool("show-intro", false, "play the game intro")
	drawStats := flags.Bool("draw-stats", false, "display renderer statistics")
	windowed := flags.Bool("windowed", false, "run in a window")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 2 {
		return errors.New("usage: simcity4-uhd launch [OPTIONS] GAME_COPY PROFILE")
	}
	if *foreground && !*start {
		return errors.New("-foreground requires -start")
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
	mac := filepath.Base(exe) == macGameExecutable
	if mac && !*savedResolution {
		width, height, _ := strings.Cut(*resolution, "x")
		w, _ := strconv.Atoi(width)
		h, _ := strconv.Atoi(height)
		if w < 800 || w > 7680 || h < 600 || h > 4320 {
			return errors.New("macOS resolution must be between 800×600 and 7680×4320")
		}
	}
	var gameArgs []string
	if !mac {
		gameArgs = append(gameArgs, "-d:DirectX")
	}
	if !*savedResolution {
		gameArgs = append(gameArgs, "-CustomResolution:enabled", "-r"+*resolution+"x32")
	}
	if *windowed {
		gameArgs = append(gameArgs, "-w")
	} else {
		gameArgs = append(gameArgs, "-f")
		if !mac {
			gameArgs = append(gameArgs, "-Borderless")
		}
	}
	if !*showIntro {
		gameArgs = append(gameArgs, "-intro:off")
	}
	if *drawStats {
		gameArgs = append(gameArgs, "-DrawStats:on")
	}
	userDir := strings.TrimRight(profile, `\/`) + `\`
	if mac {
		userDir = strings.TrimRight(profile, "/") + "/"
	}
	gameArgs = append(gameArgs, "-CPUcount:1")
	if !mac {
		gameArgs = append(gameArgs, "-UserDir:"+userDir)
	}
	if *start {
		requested := *resolution
		if *savedResolution {
			requested = ""
		}
		return startIsolatedGame(exe, directory, userDir, requested, gameArgs, *foreground, out)
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
