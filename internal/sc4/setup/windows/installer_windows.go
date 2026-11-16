package windows

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/advisors"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/pe"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
	uibuild "github.com/Feverqwe/simcity4-hd/internal/sc4/ui"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
	worldbuild "github.com/Feverqwe/simcity4-hd/internal/sc4/world"
)

const (
	gameEXE     = "SimCity 4.exe"
	stockBackup = "SimCity 4.stock.exe"
	modFolder   = "SimCity4HD"
	modReceipt  = "Installed.json"
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

var libraryPath = regexp.MustCompile(`(?mi)^\s*"path"[ \t]+"((?:[^"\\]|\\.)+)"`)

func findSteamGame() string {
	roots := []string{
		steamPath(syscall.HKEY_CURRENT_USER, `Software\Valve\Steam`, "SteamPath"),
		steamPath(syscall.HKEY_LOCAL_MACHINE, `Software\WOW6432Node\Valve\Steam`, "InstallPath"),
	}
	if programFiles := os.Getenv("ProgramFiles(x86)"); programFiles != "" {
		roots = append(roots, filepath.Join(programFiles, "Steam"))
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		libraries := []string{root}
		if data, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf")); err == nil {
			for _, entry := range libraryPath.FindAllStringSubmatch(string(data), -1) {
				libraries = append(libraries, strings.ReplaceAll(entry[1], `\\`, `\`))
			}
		}
		for _, library := range libraries {
			game := filepath.Join(library, "steamapps", "common", "SimCity 4 Deluxe")
			if fileExists(filepath.Join(game, "Apps", "SimCity 4.exe")) &&
				fileExists(filepath.Join(game, "SimCity_1.dat")) {
				return game
			}
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

type setupOptions struct {
	ui, skipIntro, drawStats, russian bool
	russianSource                     string
	keepUpscaledImages                bool
	interfaceOptions                  x86.Options
	world                             worldbuild.Selection
	width, height                     int
	textureScale                      int
}

type setupInputError struct {
	field   string
	message string
}

func (e setupInputError) Error() string { return e.message }

type installEvent struct {
	Type     string             `json:"type"`
	Phase    int                `json:"phase,omitempty"`
	Text     string             `json:"text,omitempty"`
	Value    int                `json:"value"`
	Total    int                `json:"total,omitempty"`
	Path     string             `json:"path,omitempty"`
	Field    string             `json:"field,omitempty"`
	Settings *installedSettings `json:"settings,omitempty"`
}

func Run(args []string) int {
	protocol := json.NewEncoder(os.Stdout)
	stdout := os.Stdout
	os.Stdout = os.Stderr // Keep existing builder diagnostics off the event stream.
	defer func() { os.Stdout = stdout }()
	emit := func(event installEvent) { _ = protocol.Encode(event) }
	err := installerCommand(args, emit)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			emit(installEvent{Type: "stopped"})
			return 2
		}
		var input setupInputError
		if errors.As(err, &input) {
			emit(installEvent{Type: "error", Text: err.Error(), Field: input.field})
		} else {
			emit(installEvent{Type: "error", Text: err.Error()})
		}
		return 1
	}
	return 0
}

func installerCommand(args []string, emit func(installEvent)) error {
	if len(args) == 0 {
		return errors.New("expected an installer command")
	}
	local := os.Getenv("LOCALAPPDATA")
	if !filepath.IsAbs(local) {
		return errors.New("Windows could not find your Local AppData folder.")
	}
	data := filepath.Join(local, "SimCity4HD")
	flags := flag.NewFlagSet("installer", flag.ContinueOnError)
	game := flags.String("game", "", "original Steam game directory")
	cache := flags.String("cache", filepath.Join(data, "Cache"), "texture cache directory")
	archive := flags.String("archive", "", "portable texture archive")
	ui := flags.Bool("ui", true, "upscale interface textures and apply UI fixes")
	language := flags.String("language", "game", "game language (game or russian)")
	keepUpscaledImages := flags.Bool("keep-upscaled-images", false, "keep the original upscaled images")
	textureScale := flags.Int("texture-scale", 2, "texture and cursor scale (2 or 4)")
	world := flags.Bool("world", true, "upscale world textures")
	network := flags.Bool("network", true, "upscale roads and rail")
	terrain := flags.Bool("terrain", true, "upscale terrain")
	models := flags.Bool("models", true, "upscale buildings, props and effects")
	resolution := flags.String("resolution", gamefiles.DefaultResolution, "game resolution (WIDTHxHEIGHT)")
	skipIntro := flags.Bool("skip-intro", true, "skip the game intro")
	drawStats := flags.Bool("draw-stats", false, "show renderer statistics")
	hdCursors := flags.Bool("hd-cursors", true, "use larger cursors")
	regionMap := flags.Bool("region-map", true, "use a larger region map")
	loadingPreview := flags.Bool("loading-preview", true, "use a larger city loading preview")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("Unexpected installer arguments.")
	}
	switch args[0] {
	case "export-textures", "import-textures":
		return setupTextureTransfer(args[0], *cache, *archive, emit)
	case "inspect":
		if *game == "" {
			*game = findSteamGame()
		}
		emit(installEvent{Type: "field", Field: "game", Path: *game})
		if *game != "" {
			if !fileExists(filepath.Join(*game, "Apps", gameEXE)) || !fileExists(filepath.Join(*game, "SimCity_1.dat")) {
				return setupInputError{"game", "Choose the Steam SimCity 4 Deluxe folder containing Apps and SimCity_1.dat."}
			}
			if modInstalled(*game) {
				return reportInstalled(*game, emit)
			}
			if _, err := os.Lstat(filepath.Join(*game, "Plugins", modFolder)); err == nil {
				return errors.New("An incomplete SimCity 4 UHD installation was found. Restore the Steam game before continuing.")
			}
		}
		return nil
	case "remove":
		if err := removeHD(*game); err != nil {
			return err
		}
		emit(installEvent{Type: "removed"})
		return nil
	case "install", "configure":
		if *language != "game" && *language != "russian" {
			return errors.New("Choose the game's current language or Russian.")
		}
		if *language == "russian" && !*ui {
			return errors.New("Russian requires the UHD interface option.")
		}
		if *textureScale != 2 && *textureScale != 4 {
			return errors.New("Choose a texture scale of 2 or 4.")
		}
		if !filepath.IsAbs(*cache) {
			return setupInputError{"cache", "Use an absolute path for the cache."}
		}
		if !gamefiles.ResolutionPattern.MatchString(*resolution) {
			return errors.New("Use a game resolution in WIDTHxHEIGHT format.")
		}
		w, h, _ := strings.Cut(*resolution, "x")
		width, _ := strconv.Atoi(w)
		height, _ := strconv.Atoi(h)
		if width < 800 || height < 600 {
			return errors.New("Choose a game resolution of at least 800 × 600.")
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		retry := make(chan struct{}, 1)
		// EOF pauses the operation, including when the setup window exits unexpectedly.
		go func() {
			defer cancel()
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				if scanner.Text() == "retry" {
					select {
					case retry <- struct{}{}:
					default:
					}
				}
			}
		}()
		waitForRetry := func() error {
			select {
			case <-retry:
			case <-ctx.Done():
			}
			return ctx.Err()
		}
		options := setupOptions{
			ui: *ui, skipIntro: *skipIntro, drawStats: *drawStats, russian: *language == "russian",
			width: width, height: height,
			textureScale:       *textureScale,
			keepUpscaledImages: *keepUpscaledImages,
			interfaceOptions:   x86.Options{HdCursors: *hdCursors, RegionMap: *regionMap, LoadingPreview: *loadingPreview},
			world: worldbuild.Selection{
				Network: *world && *network,
				Terrain: *world && *terrain,
				Models:  *world && *models,
			},
		}
		if args[0] == "configure" {
			if err := configureHD(ctx, *game, *cache, options, emit, waitForRetry); err != nil {
				return err
			}
		} else if err := installHD(ctx, *game, *cache, options, emit, waitForRetry); err != nil {
			return err
		}
		return reportInstalled(*game, emit)
	default:
		return fmt.Errorf("unknown installer command %q", args[0])
	}
}

func installLock(data string) (func(), error) {
	if err := os.MkdirAll(data, 0755); err != nil {
		return nil, err
	}
	path, err := syscall.UTF16PtrFromString(filepath.Join(data, ".simcity4hd-setup.lock"))
	if err != nil {
		return nil, err
	}
	const deleteOnClose = 0x04000000
	const deleteAccess = 0x00010000
	handle, err := syscall.CreateFile(path, deleteAccess, 0, nil, syscall.OPEN_ALWAYS, deleteOnClose, 0)
	if errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(303)) { // Sharing violation or delete pending.
		return nil, errors.New("Another SimCity 4 UHD installation is already running.")
	}
	if err != nil {
		return nil, err
	}
	return func() { syscall.CloseHandle(handle) }, nil
}

func installHD(ctx context.Context, game, cache string, options setupOptions, emit func(installEvent), waitForRetry func() error) error {
	stages := 4 // Check, prepare renderer, check packages, apply.
	if options.ui {
		stages++
	}
	if options.world.Any() {
		stages++
	}
	nextStage := setup.SetupStageProgress(stages, func(done, total int) {
		emit(installEvent{Type: "overall", Value: done, Total: total})
	})
	emit(installEvent{Type: "step", Phase: 1, Text: "Checking the game installation and cache"})
	fmt.Fprintf(os.Stderr, "Game: %q; cache: %q; resolution: %dx%d\n", game, cache, options.width, options.height)
	if !filepath.IsAbs(game) {
		return setupInputError{"game", "Choose your original SimCity 4 Deluxe folder."}
	}
	game, err := filepath.EvalSymlinks(game)
	if err != nil {
		return setupInputError{"game", fmt.Sprintf("Could not open the game folder: %v", err)}
	}
	exe, err := os.ReadFile(filepath.Join(game, "Apps", "SimCity 4.exe"))
	if err != nil {
		return setupInputError{"game", "Choose the Steam SimCity 4 Deluxe folder containing Apps and SimCity_1.dat."}
	}
	if executableHash(exe) != pe.StockHash {
		return setupInputError{"game", "The original Steam build 4644414 is required. Verify the game files in Steam or select an unmodified installation."}
	}
	if !fileExists(filepath.Join(game, "SimCity_1.dat")) {
		return setupInputError{"game", "SimCity_1.dat is missing. Verify your game files in Steam."}
	}
	plugins := filepath.Join(game, "Plugins")
	destination := filepath.Join(plugins, modFolder)
	if setup.InsideFolder(game, cache) || setup.InsideFolder(cache, game) {
		return setupInputError{"cache", "Choose a dedicated texture cache outside the game folder."}
	}
	cache, err = setup.ResolveSetupPath(cache)
	if err != nil {
		return setupInputError{"cache", fmt.Sprintf("Could not open the texture cache: %v", err)}
	}
	if setup.InsideFolder(game, cache) || setup.InsideFolder(cache, game) {
		return setupInputError{"cache", "Choose a dedicated texture cache outside the game folder."}
	}
	unlockGame, err := installLock(game)
	if err != nil {
		return err
	}
	defer unlockGame()
	if err := gamefiles.EnsureGameNotRunning(); err != nil {
		return err
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return errors.New("Plugins\\SimCity4HD already exists. Move the existing HD package out of Plugins before installing again.")
	}
	if _, err := os.Lstat(filepath.Join(game, inactiveWorldFolder)); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return errors.New("A folder with disabled HD textures already exists. Move it out of the game folder before installing again.")
	}
	dependencyDir := filepath.Join(cache, "SetupDependencies")
	emit(installEvent{Type: "step", Phase: 1, Text: "Checking available disk space"})
	if err := checkSetupSpace(ctx, game, dependencyDir, cache, options); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	unlockCache, err := installLock(cache)
	if err != nil {
		return err
	}
	defer unlockCache()
	work, err := os.MkdirTemp(game, ".setup-work-")
	if err != nil {
		return err
	}
	keepWork := false
	defer func() {
		if !keepWork {
			_ = os.RemoveAll(work)
		}
	}()
	ui, world := filepath.Join(work, "UI"), filepath.Join(work, "World")
	phase := 1
	deps := setupDependencies{ctx: ctx, dir: dependencyDir, emit: emit, waitForRetry: waitForRetry}
	progress := installerBuildProgress(&deps, &phase)
	run := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		nextStage()
		var patched string
		var packageDirs []string
		if options.russian {
			options.russianSource, err = stageSetupRussianSource(work, "")
			if err != nil {
				return err
			}
		}
		if options.ui {
			if err := uibuild.BuildWithProgress(ctx, setupUIArgs(options, game, cache, ui), progress); err != nil {
				return err
			}
			patched = filepath.Join(ui, "SimCity4-hd.exe")
			packageDirs = append(packageDirs, filepath.Join(ui, "Plugins"))
			nextStage()
		} else {
			fmt.Fprintln(os.Stderr, "Interface: skipped by request")
		}
		phase = 2
		if options.world.Any() {
			if err := worldbuild.BuildWithProgress(ctx, setupWorldArgs(options, game, cache, world), progress, options.world); err != nil {
				return err
			}
			packageDirs = append(packageDirs, filepath.Join(world, "Plugins"))
			nextStage()
		} else {
			fmt.Fprintln(os.Stderr, "World textures: skipped by request")
		}
		phase = 3
		progress.Report("Preparing the 4K renderer")
		renderer, err := deps.renderer(phase)
		if err != nil {
			return err
		}
		graphics, err := deps.graphics(phase)
		if err != nil {
			return err
		}
		nextStage()
		progress.Report("Installing and checking HD files")
		stage := filepath.Join(work, "Install")
		if err := os.Mkdir(stage, 0755); err != nil {
			return err
		}
		if options.ui {
			progress.Report("Preparing the advisors")
			if err := stageSetupAdvisors(ctx, game, stage); err != nil {
				return err
			}
		}
		if len(packageDirs) > 0 {
			packages, err := gamefiles.PackagePaths(packageDirs)
			if err != nil {
				return err
			}
			validation := packages
			if options.ui {
				validation = append(validation, filepath.Join(stage, advisors.AdvisorsPackage))
			}
			if err := gamefiles.VerifyPackages(validation, os.Stderr); err != nil {
				return err
			}
			for _, name := range packages {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := moveSetupPath(name, filepath.Join(stage, filepath.Base(name)), false); err != nil {
					return err
				}
			}
		}
		if err := writeSetupCacheSettings(stage, cache, options.keepUpscaledImages); err != nil {
			return err
		}
		nextStage()
		progress.Report("Applying the mod to the game folder")
		var fontChanges []setupReplacement
		if options.russian {
			fontChanges, err = stageSetupRussianFonts(game, work, "", stage, options.russianSource, ui)
			if err != nil {
				return err
			}
		}
		if err := publishMod(ctx, game, work, stage, patched, exe, renderer, graphics, options, fontChanges, &keepWork); err != nil {
			return err
		}
		nextStage()
		return nil
	}
	err = run()
	if err != nil && ctx.Err() != nil && !keepWork {
		if !errors.Is(err, ctx.Err()) {
			fmt.Fprintf(os.Stderr, "Error while pausing: %v\n", err)
		}
		return ctx.Err()
	}
	return err
}

func setupUIArgs(options setupOptions, game, cache, output string) []string {
	args := []string{
		fmt.Sprintf("-texture-scale=%d", options.textureScale),
		fmt.Sprintf("-keep-upscaled-images=%t", options.keepUpscaledImages),
		fmt.Sprintf("-cursor-scale=%d", options.textureScale),
		fmt.Sprintf("-hd-cursors=%t", options.interfaceOptions.HdCursors),
		fmt.Sprintf("-region-map-2x=%t", options.interfaceOptions.RegionMap),
		fmt.Sprintf("-loading-preview-2x=%t", options.interfaceOptions.LoadingPreview),
	}
	if options.russianSource != "" {
		args = append(args, "-russian", options.russianSource)
	}
	return append(args, game, cache, output)
}

func setupWorldArgs(options setupOptions, game, cache, output string) []string {
	return []string{fmt.Sprintf("-texture-scale=%d", options.textureScale),
		fmt.Sprintf("-keep-upscaled-images=%t", options.keepUpscaledImages), game, cache, output}
}

func installerBuildProgress(deps *setupDependencies, phase *int) upscale.Progress {
	const updateInterval = 250 * time.Millisecond
	var label string
	var current installEvent
	var lastSent time.Time
	emit := deps.emit
	return upscale.Progress{
		Prepare: func() (string, string, error) {
			python, weights, err := deps.inference(*phase)
			if err == nil {
				emit(installEvent{Type: "step", Phase: *phase, Text: label})
				if current.Type != "" {
					emit(current)
					lastSent = time.Now()
				}
			}
			return python, weights, err
		},
		Step: func(step string) {
			label = step
			current = installEvent{}
			lastSent = time.Time{}
			emit(installEvent{Type: "step", Phase: *phase, Text: step})
		},
		Resources: func(done, total int) {
			previous := current
			current = installEvent{Type: "count", Phase: *phase, Value: done, Total: total}
			now := time.Now()
			if done <= 1 || total > 0 && done == total || previous.Total != total || now.Sub(lastSent) >= updateInterval {
				emit(current)
				lastSent = now
			}
		},
	}
}
