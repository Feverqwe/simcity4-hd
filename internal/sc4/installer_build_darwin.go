package sc4

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type macSetupBuild struct {
	Scale              int    `json:"scale"`
	Language           string `json:"language"`
	Interface          bool   `json:"interface"`
	Cursors            bool   `json:"cursors"`
	RegionMap          bool   `json:"regionMap"`
	LoadingPreview     bool   `json:"loadingPreview"`
	RetinaText         bool   `json:"retinaText,omitempty"`
	World              bool   `json:"world"`
	Network            bool   `json:"network"`
	Terrain            bool   `json:"terrain"`
	Models             bool   `json:"models"`
	SkipIntro          *bool  `json:"skipIntro,omitempty"`
	DrawStats          bool   `json:"drawStats"`
	KeepUpscaledImages bool   `json:"keepUpscaledImages,omitempty"`
}

func (b *macSetupBuild) flags(flags *flag.FlagSet) {
	flags.IntVar(&b.Scale, "texture-scale", 2, "texture scale (2 or 4)")
	flags.StringVar(&b.Language, "language", "game", "game language (game or russian)")
	flags.BoolVar(&b.Interface, "interface", true, "install the UHD interface")
	flags.BoolVar(&b.Cursors, "hd-cursors", true, "use larger game cursors")
	flags.BoolVar(&b.RegionMap, "region-map", true, "use a larger region map")
	flags.BoolVar(&b.LoadingPreview, "loading-preview", true, "use a larger loading preview")
	flags.BoolVar(&b.RetinaText, "retina-text", false, "render text at Retina density")
	flags.BoolVar(&b.World, "world-textures", true, "install UHD world textures")
	flags.BoolVar(&b.Network, "network", true, "upscale roads and rail")
	flags.BoolVar(&b.Terrain, "terrain", true, "upscale terrain")
	flags.BoolVar(&b.Models, "models", true, "upscale buildings, props and effects")
	b.SkipIntro = new(bool)
	flags.BoolVar(b.SkipIntro, "skip-intro", true, "skip the game intro")
	flags.BoolVar(&b.KeepUpscaledImages, "keep-upscaled-images", false, "keep the original upscaled images")
	flags.BoolVar(&b.DrawStats, "draw-stats", false, "show the game's FPS counter")
}

func (b macSetupBuild) selectSources(flags *flag.FlagSet, sources *macSetupSources) error {
	selection := make(map[string]bool)
	flags.Visit(func(value *flag.Flag) { selection[value.Name] = true })
	if sources.UI != "" || sources.World != "" {
		// Existing CLI callers select components by providing their prepared paths.
		if !selection["interface"] {
			b.Interface = sources.UI != ""
		}
		if !selection["world-textures"] {
			b.World = sources.World != ""
		}
	}
	if !b.Interface {
		sources.UI = ""
	}
	if !b.World {
		sources.World = ""
	}
	if b.Scale != 2 && b.Scale != 4 {
		return errors.New("Choose a texture scale of 2 or 4.")
	}
	if b.Language != "game" && b.Language != "russian" {
		return errors.New("Choose the game's current language or Russian.")
	}
	if b.World && sources.World == "" && !b.worldSelection().any() {
		return errors.New("Select at least one category of world textures.")
	}
	sources.Build = &b
	return nil
}

func (b macSetupBuild) worldSelection() worldSelection {
	return worldSelection{network: b.Network, terrain: b.Terrain, models: b.Models}
}

func preparedMacPackages(path *string) ([]string, error) {
	absolute, err := filepath.Abs(*path)
	if err != nil {
		return nil, err
	}
	*path = absolute
	return packagePaths([]string{filepath.Join(absolute, "Plugins")})
}

func availableMacPreparedBuild(path string) string {
	if path != "" {
		if _, err := os.Stat(filepath.Join(path, "Plugins")); !errors.Is(err, os.ErrNotExist) {
			return path
		}
	}
	return ""
}

func (s *macSetupSources) readPreparedSettings(flags *flag.FlagSet) error {
	explicit := make(map[string]bool)
	flags.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	s.overrideRegionMap = explicit["region-map"]
	s.overrideLoadingPreview = explicit["loading-preview"]
	s.overrideCursors = explicit["hd-cursors"]
	s.overrideLanguage = explicit["language"]
	s.overrideRetinaText = explicit["retina-text"]
	checkScale := func(field string, prepared int) error {
		if explicit["texture-scale"] && prepared != s.Build.Scale {
			return macSetupInputError{field, fmt.Errorf(
				"The prepared build uses ×%d textures. Choose a ×%d build, change the texture scale, or clear the prepared build field.",
				prepared, s.Build.Scale,
			)}
		}
		return nil
	}
	var scale int
	if s.UI != "" {
		packages, err := preparedMacPackages(&s.UI)
		if err != nil {
			return macSetupInputError{"ui", err}
		}
		scale, err = macHUDScale(packages)
		if err != nil {
			return macSetupInputError{"ui", err}
		}
		if err := checkScale("ui", scale); err != nil {
			return err
		}
	}
	if s.World != "" {
		packages, err := preparedMacPackages(&s.World)
		if err != nil {
			return macSetupInputError{"world", err}
		}
		worldScale, err := macWorldPackageScale(packages)
		if err != nil {
			return macSetupInputError{"world", err}
		}
		if err := checkScale("world", worldScale); err != nil {
			return err
		}
		if scale != 0 && scale != worldScale {
			return macSetupInputError{"world", errors.New("UI and World must use the same texture scale")}
		}
		scale = worldScale
		present := make(map[string]bool)
		for _, path := range packages {
			category, _ := macWorldCategory(filepath.Base(path), true)
			present[category] = true
		}
		for category, selected := range map[string]*bool{
			"network": &s.Build.Network, "terrain": &s.Build.Terrain, "models": &s.Build.Models,
		} {
			if !explicit[category] {
				*selected = present[category]
			}
		}
		s.world.active = packages
	}
	if scale != 0 {
		s.Build.Scale = scale
	}
	if s.Build.World && !s.Build.worldSelection().any() {
		return macSetupInputError{"world", errors.New("Select at least one category of world textures.")}
	}
	return nil
}

type macSetupSettings struct {
	Resolution string            `json:"resolution,omitempty"`
	Options    map[string]bool   `json:"options"`
	Choices    map[string]string `json:"choices,omitempty"`
}

func (b macSetupBuild) skipIntro() bool { return b.SkipIntro == nil || *b.SkipIntro }

func (s macSetupSources) settings() *macSetupSettings {
	settings := &macSetupSettings{Options: map[string]bool{
		"interface": s.UI != "", "world-textures": s.World != "", "skip-intro": true,
	}}
	if b := s.Build; b != nil {
		settings.Options = map[string]bool{
			"interface": b.Interface, "world-textures": b.World,
			"hd-cursors": b.Cursors, "region-map": b.RegionMap, "loading-preview": b.LoadingPreview,
			"retina-text": b.RetinaText,
			"network":     b.Network, "terrain": b.Terrain, "models": b.Models,
			"skip-intro": b.skipIntro(), "draw-stats": b.DrawStats,
			"keep-upscaled-images": b.KeepUpscaledImages,
		}
		settings.Choices = map[string]string{"texture-scale": strconv.Itoa(b.Scale), "language": b.Language}
	}
	return settings
}

func (s macSetupSources) needsBuild() bool {
	return s.Build != nil && (s.needsInterfaceBuild() || s.world.missing.any())
}

func (s macSetupSources) needsInterfaceBuild() bool {
	return s.Build.Interface && s.UI == "" && s.reusedUI.bundle == ""
}

func (s macSetupSources) buildSpace() uint64 {
	if !s.needsBuild() {
		return 0
	}
	var installed uint64
	if s.needsInterfaceBuild() {
		installed += 512 << 20
	}
	if s.world.missing.any() {
		world := uint64(0)
		if s.world.missing.network {
			world += 160 << 20
		}
		if s.world.missing.terrain {
			world += 128 << 20
		}
		if s.world.missing.models {
			world += 23 << 30
		}
		if s.Build.Scale == 2 {
			world /= 3
		}
		installed += world
	}
	return 2 * installed // Prepared packages and their staged game copies coexist.
}

func prepareMacSetupBuild(ctx context.Context, stock, work string, sources macSetupSources, send func(macSetupEvent), retry func() error, nextStage func()) (macSetupSources, error) {
	cache := sources.Cache
	unlock, err := lockMacTextureCache(cache)
	if err != nil {
		return sources, macSetupInputError{"cache", err}
	}
	defer unlock()
	b := sources.Build
	deps := macSetupDependencies{ctx: ctx, dir: filepath.Join(cache, "SetupDependencies"), emit: send, waitForRetry: retry}
	progress := buildProgress{
		prepare: deps.inference,
		step:    func(text string) { send(macSetupEvent{Type: "step", Text: text}) },
		resources: func(done, total int) {
			send(macSetupEvent{Type: "count", Value: &done, Total: total})
		},
	}
	common := []string{"-texture-scale", strconv.Itoa(b.Scale),
		fmt.Sprintf("-keep-upscaled-images=%t", b.KeepUpscaledImages)}
	if sources.needsInterfaceBuild() {
		output := filepath.Join(work, "Interface")
		args := append([]string{}, common...)
		if b.Cursors {
			args = append(args, "-hd-cursors", "-cursor-scale", strconv.Itoa(b.Scale))
		}
		if b.RegionMap {
			args = append(args, "-region-map-2x")
		}
		if b.LoadingPreview {
			args = append(args, "-loading-preview-2x")
		}
		if b.Language == "russian" {
			exe, err := os.Executable()
			if err != nil {
				return sources, err
			}
			russian := filepath.Join(work, "Russian")
			pack := filepath.Join(filepath.Dir(exe), "..", "Resources", "Russian", russianPackFile)
			if err := unpackRussianAssets(pack, russian); err != nil {
				return sources, fmt.Errorf("prepare Russian language files: %w", err)
			}
			args = append(args, "-russian", russian)
		}
		if err := buildUIWithProgress(ctx, append(args, stock, cache, output), progress); err != nil {
			return sources, err
		}
		sources.UI = output
		nextStage()
	}
	if sources.world.missing.any() {
		output := filepath.Join(work, "World")
		if err := buildWorldWithProgress(ctx, append(common, stock, cache, output), progress, sources.world.missing); err != nil {
			return sources, err
		}
		packages, err := preparedMacPackages(&output)
		if err != nil {
			return sources, err
		}
		sources.world.active = append(sources.world.active, packages...)
		nextStage()
	}
	return sources, nil
}
