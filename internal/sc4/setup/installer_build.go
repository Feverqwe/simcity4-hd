//go:build windows || darwin

package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/russian"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/ui"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/world"
)

func (b BuildOptions) args() []string {
	return []string{fmt.Sprintf("-texture-scale=%d", b.Scale),
		fmt.Sprintf("-keep-upscaled-images=%t", b.KeepUpscaledImages)}
}

func (b BuildOptions) buildInterface(ctx context.Context, game, cache, output, translation string, progress upscale.Progress, stock []byte) error {
	args := append(b.args(), fmt.Sprintf("-cursor-scale=%d", b.Scale),
		fmt.Sprintf("-hd-cursors=%t", b.Features.HdCursors),
		fmt.Sprintf("-region-map-2x=%t", b.Features.RegionMap),
		fmt.Sprintf("-loading-preview-2x=%t", b.Features.LoadingPreview))
	if translation != "" {
		args = append(args, "-russian", translation)
	}
	return ui.BuildFromExecutable(ctx, append(args, game, cache, output), progress, stock)
}

func (b BuildOptions) buildWorld(ctx context.Context, game, cache, output string, progress upscale.Progress, selection world.Selection) error {
	return world.BuildWithProgress(ctx, append(b.args(), game, cache, output), progress, selection)
}

func stageRussianSource(work, pack string) (string, error) {
	dir := filepath.Join(work, "Russian")
	if pack == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		binary := filepath.Dir(exe)
		for _, parent := range []string{binary, os.Getenv("SETUP_EXE_DIR"), filepath.Join(binary, "..", "Resources")} {
			candidate := filepath.Join(parent, "Russian", russian.PackFile)
			if !filepath.IsAbs(parent) {
				continue
			}
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
				pack = candidate
				break
			}
		}
		if pack == "" {
			return "", errors.New("Russian language files are missing from the installer.")
		}
	}
	if err := russian.Unpack(pack, dir); err != nil {
		return "", fmt.Errorf("prepare Russian language files: %w", err)
	}
	if err := gamefiles.CopyRegular(pack, filepath.Join(dir, russian.PackFile)); err != nil {
		return "", err
	}
	return dir, nil
}

// BuildTextures prepares the requested outputs without touching the installation.
// StockExecutable is needed when the game already contains a patched executable.
type TextureBuild struct {
	BuildOptions
	Game, Cache, Work, RussianPack string
	Interface, Russian             bool
	World                          world.Selection
	StockExecutable                []byte
}

type TextureOutputs struct {
	Interface, World, RussianSource string
}

func (b TextureBuild) Run(ctx context.Context, deps *Resources, nextStage func()) (out TextureOutputs, err error) {
	// An interrupted inference process can report a pipe error after stdin
	// closes. No game files have been published, so report a clean pause.
	defer func() {
		if err != nil && ctx.Err() != nil {
			err = ctx.Err()
		}
	}()
	if err := ctx.Err(); err != nil {
		return out, err
	}
	phase := 1
	progress := buildProgress(deps, &phase)
	if b.Interface {
		if b.Russian {
			out.RussianSource, err = stageRussianSource(b.Work, b.RussianPack)
			if err != nil {
				return out, err
			}
		}
		out.Interface = filepath.Join(b.Work, "UI")
		if err := b.buildInterface(ctx, b.Game, b.Cache, out.Interface, out.RussianSource, progress, b.StockExecutable); err != nil {
			return out, err
		}
		nextStage()
	}
	if b.World.Any() {
		phase = 2
		out.World = filepath.Join(b.Work, "World")
		if err := b.buildWorld(ctx, b.Game, b.Cache, out.World, progress, b.World); err != nil {
			return out, err
		}
		nextStage()
	}
	return out, nil
}

func (o TextureOutputs) PackageDirs() []string {
	var dirs []string
	for _, output := range []string{o.Interface, o.World} {
		if output != "" {
			dirs = append(dirs, filepath.Join(output, "Plugins"))
		}
	}
	return dirs
}
