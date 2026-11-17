//go:build windows || darwin

package macos

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
	worldbuild "github.com/Feverqwe/simcity4-hd/internal/sc4/world"
)

func macBuildSources(command setup.Command) (macSetupSources, error) {
	if err := command.ValidateOptions(); err != nil {
		return macSetupSources{}, err
	}
	sources := macSetupSources{
		UI: command.PreparedUI, World: command.PreparedWorld, Build: macBuildOptions(command),
	}
	if !command.Interface {
		sources.UI = ""
	}
	if !command.World {
		sources.World = ""
	}
	var err error
	sources.resolution, err = parseMacSetupResolution(command.Resolution)
	if err != nil {
		return sources, setup.FieldError("resolution", err)
	}
	if err := sources.readPreparedSettings(command.Explicit); err != nil {
		return sources, err
	}
	return sources, nil
}

func (b macSetupBuild) worldSelection() worldbuild.Selection {
	return worldbuild.Selection{Network: b.Network, Terrain: b.Terrain, Models: b.Models}
}

func preparedMacPackages(path *string) ([]string, error) {
	absolute, err := filepath.Abs(*path)
	if err != nil {
		return nil, err
	}
	*path = absolute
	return gamefiles.PackagePaths([]string{filepath.Join(absolute, "Plugins")})
}

func (s *macSetupSources) readPreparedSettings(explicit map[string]bool) error {
	s.overrideRegionMap = explicit["region-map"]
	s.overrideLoadingPreview = explicit["loading-preview"]
	s.overrideCursors = explicit["hd-cursors"]
	s.overrideLanguage = explicit["language"]
	checkScale := func(field string, prepared int) error {
		if explicit["texture-scale"] && prepared != s.Build.Scale {
			return setup.FieldError(field, fmt.Errorf(
				"The prepared build uses ×%d textures. Choose a ×%d build, change the texture scale, or clear the prepared build field.",
				prepared, s.Build.Scale,
			))
		}
		return nil
	}
	var scale int
	if s.UI != "" {
		packages, err := preparedMacPackages(&s.UI)
		if err != nil {
			return setup.FieldError("prepared-ui", err)
		}
		scale, err = macHUDScale(packages)
		if err != nil {
			return setup.FieldError("prepared-ui", err)
		}
		if err := checkScale("prepared-ui", scale); err != nil {
			return err
		}
	}
	if s.World != "" {
		packages, err := preparedMacPackages(&s.World)
		if err != nil {
			return setup.FieldError("prepared-world", err)
		}
		worldScale, err := macWorldPackageScale(packages)
		if err != nil {
			return setup.FieldError("prepared-world", err)
		}
		if err := checkScale("prepared-world", worldScale); err != nil {
			return err
		}
		if scale != 0 && scale != worldScale {
			return setup.FieldError("prepared-world", errors.New("UI and World must use the same texture scale"))
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
	if s.Build.World && !s.Build.worldSelection().Any() {
		return setup.FieldError("prepared-world", errors.New("Select at least one category of world textures."))
	}
	return nil
}

func (s macSetupSources) settings() *setup.Settings {
	settings := &setup.Settings{Options: map[string]bool{
		"ui": s.UI != "", "world": s.World != "", "skip-intro": true,
	}}
	if b := s.Build; b != nil {
		settings.Options = map[string]bool{
			"ui": b.Interface, "world": b.World,
			"network": b.Network, "terrain": b.Terrain, "models": b.Models,
			"skip-intro": b.skipIntro(), "draw-stats": b.DrawStats,
			"keep-upscaled-images": b.KeepUpscaledImages,
		}
		if b.Interface {
			settings.Options["hd-cursors"] = b.Cursors
			settings.Options["region-map"] = b.RegionMap
			settings.Options["loading-preview"] = b.LoadingPreview
			settings.Options["retina-text"] = b.RetinaText
		}
		settings.Choices = map[string]string{"texture-scale": strconv.Itoa(b.Scale), "language": b.Language}
	}
	return settings
}

func (s macSetupSources) needsInterfaceBuild() bool {
	return s.Build.Interface && s.UI == "" && s.reusedUI.bundle == ""
}

func (s macSetupSources) textureBuild() setup.TextureBuild {
	b := s.Build
	return setup.TextureBuild{
		BuildOptions: setup.BuildOptions{
			Scale: b.Scale, KeepUpscaledImages: b.KeepUpscaledImages,
			Features: x86.Options{HdCursors: b.Cursors, RegionMap: b.RegionMap, LoadingPreview: b.LoadingPreview},
		},
		Cache: s.Cache, Interface: s.needsInterfaceBuild(), Russian: b.Language == "russian", World: s.world.missing,
	}
}
