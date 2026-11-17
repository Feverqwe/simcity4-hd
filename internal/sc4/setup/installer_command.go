package setup

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/world"
)

type Event struct {
	Type     string    `json:"type"`
	Phase    int       `json:"phase,omitempty"`
	Text     string    `json:"text,omitempty"`
	Value    *int      `json:"value,omitempty"`
	Total    int       `json:"total,omitempty"`
	Path     string    `json:"path,omitempty"`
	Field    string    `json:"field,omitempty"`
	Target   string    `json:"target,omitempty"`
	Settings *Settings `json:"settings,omitempty"`
}

type Settings struct {
	Options    map[string]bool   `json:"options"`
	Choices    map[string]string `json:"choices,omitempty"`
	Resolution string            `json:"resolution,omitempty"`
}

func Measurement(kind string, phase, value, total int) Event {
	return Event{Type: kind, Phase: phase, Value: &value, Total: total}
}

type inputError struct {
	field string
	err   error
}

func (e inputError) Error() string { return e.err.Error() }
func (e inputError) Unwrap() error { return e.err }

func FieldError(field string, err error) error { return inputError{field, err} }

type BuildOptions struct {
	Scale              int
	KeepUpscaledImages bool
	Features           x86.Options
}

type Command struct {
	Action, Target, Game, Cache, Archive, Resolution   string
	PreparedUI, PreparedWorld                          string
	Interface, World, SkipIntro, DrawStats, RetinaText bool
	BuildOptions
	Categories world.Selection
	Language   string
	Explicit   map[string]bool
}

type Execute func(context.Context, Command, func(Event), func() error) error

func Run(args []string, execute Execute) int {
	stdout := os.Stdout
	protocol := json.NewEncoder(stdout)
	os.Stdout = os.Stderr // Keep builder diagnostics off the event stream.
	defer func() { os.Stdout = stdout }()
	emit := func(event Event) { _ = protocol.Encode(event) }

	command, err := parseCommand(args)
	if err == nil {
		ctx, cancel, retry := context.Background(), func() {}, (func() error)(nil)
		if command.Action != "inspect" {
			ctx, cancel, retry = operationContext()
		}
		defer cancel()
		err = execute(ctx, command, emit, retry)
	}
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, context.Canceled):
		emit(Event{Type: "stopped"})
		return ExitPaused
	default:
		var input inputError
		var field string
		if errors.As(err, &input) {
			field = input.field
		}
		emit(Event{Type: "error", Text: err.Error(), Field: field})
		return 1
	}
}

func operationContext() (context.Context, context.CancelFunc, func() error) {
	ctx, cancel := context.WithCancel(context.Background())
	retries := make(chan struct{}, 1)
	go func() {
		defer cancel() // EOF pauses work, including when the window exits.
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if scanner.Text() == "retry" {
				select {
				case retries <- struct{}{}:
				default:
				}
			}
		}
	}()
	return ctx, cancel, func() error {
		select {
		case <-ctx.Done():
		case <-retries:
		}
		return ctx.Err()
	}
}

func parseCommand(args []string) (Command, error) {
	if len(args) == 0 {
		return Command{}, errors.New("expected an installer command")
	}
	c := Command{Action: args[0], Explicit: make(map[string]bool)}
	flags := flag.NewFlagSet("simcity4-uhd setup "+c.Action, flag.ContinueOnError)
	flags.StringVar(&c.Target, "target", "auto", "game edition (windows, aspyr or auto)")
	flags.StringVar(&c.Game, "game", "", "Steam game bundle or installation folder")
	flags.StringVar(&c.Cache, "cache", "", "texture cache directory")
	flags.StringVar(&c.Archive, "archive", "", "portable texture archive")
	flags.StringVar(&c.Resolution, "resolution", "", "game resolution (WIDTHxHEIGHT, or auto)")
	flags.IntVar(&c.Scale, "texture-scale", 2, "texture scale (2 or 4)")
	flags.StringVar(&c.Language, "language", "game", "game language (game or russian)")
	flags.StringVar(&c.PreparedUI, "prepared-ui", "", "prepared macOS UI build")
	flags.StringVar(&c.PreparedWorld, "prepared-world", "", "prepared macOS World build")
	for _, option := range c.switches() {
		flags.BoolVar(option.value, option.name, option.initial, option.help)
	}
	if err := flags.Parse(args[1:]); err != nil {
		return c, err
	}
	if flags.NArg() != 0 {
		return c, errors.New("Unexpected installer arguments.")
	}
	if c.Target != "auto" && c.Target != "windows" && c.Target != "aspyr" {
		return c, FieldError("game", errors.New("Choose a Windows or Aspyr game."))
	}
	flags.Visit(func(f *flag.Flag) { c.Explicit[f.Name] = true })
	return c, nil
}

func (c Command) WorldSelection() world.Selection {
	if !c.World {
		return world.Selection{}
	}
	return c.Categories
}

type setupSwitch struct {
	name, help string
	value      *bool
	initial    bool
}

func (c *Command) switches() []setupSwitch {
	return []setupSwitch{
		{"keep-upscaled-images", "keep the original upscaled images", &c.KeepUpscaledImages, false},
		{"ui", "install the UHD interface", &c.Interface, true},
		{"world", "install UHD world textures", &c.World, true},
		{"hd-cursors", "use larger game cursors", &c.Features.HdCursors, true},
		{"region-map", "use a larger region map", &c.Features.RegionMap, true},
		{"loading-preview", "use a larger loading preview", &c.Features.LoadingPreview, true},
		{"network", "upscale roads and rail", &c.Categories.Network, true},
		{"terrain", "upscale terrain", &c.Categories.Terrain, true},
		{"models", "upscale buildings, props and effects", &c.Categories.Models, true},
		{"skip-intro", "skip the game intro", &c.SkipIntro, true},
		{"draw-stats", "show renderer statistics", &c.DrawStats, false},
		{"retina-text", "render text at Retina density in the Aspyr game", &c.RetinaText, true},
	}
}

func (c Command) ValidateOptions() error {
	if c.Language != "game" && c.Language != "russian" {
		return errors.New("Choose the game's current language or Russian.")
	}
	if c.Language == "russian" && !c.Interface {
		return errors.New("Russian requires the UHD interface option.")
	}
	if c.Scale != 2 && c.Scale != 4 {
		return errors.New("Choose a texture scale of 2 or 4.")
	}
	if c.World && c.PreparedWorld == "" && !c.Categories.Any() {
		return errors.New("Select at least one category of world textures.")
	}
	return nil
}

// WithInstalledOptions keeps saved values for flags omitted during Configure.
// Each target applies resolution separately because it may live in a game profile.
func (c Command) WithInstalledOptions(settings Settings) Command {
	defaultCategories := c.Categories
	for _, option := range c.switches() {
		if installed, ok := settings.Options[option.name]; ok && !c.Explicit[option.name] {
			*option.value = installed
		}
	}
	if c.PreparedUI != "" && !c.Explicit["ui"] {
		c.Interface = true
	}
	if c.PreparedWorld != "" && !c.Explicit["world"] {
		c.World = true
	}
	if c.World && !settings.Options["world"] && !c.Categories.Any() &&
		!c.Explicit["network"] && !c.Explicit["terrain"] && !c.Explicit["models"] {
		c.Categories = defaultCategories
	}
	if !c.Explicit["language"] {
		if !c.Interface {
			c.Language = "game"
		} else if language := settings.Choices["language"]; language != "" {
			c.Language = language
		}
	}
	if !c.Explicit["texture-scale"] {
		switch settings.Choices["texture-scale"] {
		case "2":
			c.Scale = 2
		case "4":
			c.Scale = 4
		}
	}
	return c
}
