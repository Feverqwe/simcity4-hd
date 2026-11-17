//go:build windows || darwin

package setup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

// InstallationPlan describes the target-specific work around the shared
// installer. Stage prepares files in the work directory and returns the
// transaction that will change the installed game.
type InstallationPlan struct {
	Textures   TextureBuild
	NeedsCache bool // The target needs cached resources even without a texture build.
	CheckSpace func() error
	Stage      func(TextureOutputs, *Resources) (apply func(context.Context) error, err error)
}

func (p InstallationPlan) Run(ctx context.Context, emit func(Event), retry func() error) (result error) {
	applying := false
	defer func() {
		if result != nil && !applying && ctx.Err() != nil {
			result = ctx.Err()
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	stages := 3 // Check space, prepare files, apply.
	if p.Textures.Interface {
		stages++
	}
	if p.Textures.World.Any() {
		stages++
	}
	advance := StageProgress(stages, emit)
	if p.NeedsCache || p.Textures.Interface || p.Textures.World.Any() {
		unlock, err := LockFolder(p.Textures.Cache)
		if err != nil {
			return FieldError("cache", err)
		}
		defer unlock()
	}
	if p.CheckSpace != nil {
		emit(Event{Type: "step", Text: "Checking available disk space"})
		if err := p.CheckSpace(); err != nil {
			return err
		}
	}
	advance()
	resources := Resources{Context: ctx, Dir: filepath.Join(p.Textures.Cache, "SetupDependencies"), Emit: emit, WaitForRetry: retry}
	outputs, err := p.Textures.Run(ctx, &resources, advance)
	if err != nil {
		return err
	}
	apply, err := p.Stage(outputs, &resources)
	if err != nil {
		return err
	}
	advance()
	if err := ctx.Err(); err != nil {
		return err
	}
	applying = true
	emit(Event{Type: "step", Text: "Applying changes to the game"})
	if err := apply(context.WithoutCancel(ctx)); err != nil {
		return err
	}
	advance()
	emit(Event{Type: "field", Field: "cache", Path: p.Textures.Cache})
	return nil
}

func ResolveCache(cache, installed string, excluded ...string) (string, error) {
	if cache == "" && filepath.IsAbs(installed) {
		cache = installed
	}
	if cache == "" {
		var err error
		cache, err = DefaultCache()
		if err != nil {
			return "", FieldError("cache", err)
		}
	}
	if !filepath.IsAbs(cache) {
		return "", FieldError("cache", errors.New("Use an absolute path for the texture cache."))
	}
	resolved, err := ResolveSetupPath(cache)
	if err != nil {
		return "", FieldError("cache", fmt.Errorf("Could not open the texture cache: %w", err))
	}
	for _, folder := range excluded {
		folder, err := ResolveSetupPath(folder)
		if err != nil {
			return "", FieldError("cache", fmt.Errorf("Could not check the texture cache location: %w", err))
		}
		for _, paths := range [][2]string{{folder, resolved}, {resolved, folder}} {
			inside, err := insideFolder(paths[0], paths[1])
			if err != nil {
				return "", FieldError("cache", fmt.Errorf("Could not check the texture cache location: %w", err))
			}
			if inside {
				return "", FieldError("cache", errors.New("Choose a dedicated texture cache outside the game and setup work folders."))
			}
		}
	}
	return resolved, nil
}

// ProtectInstalledCache also handles caches saved by older installers.
func ProtectInstalledCache(cache string, replaced ...string) error {
	if !filepath.IsAbs(cache) {
		return nil
	}
	resolved, err := filepath.EvalSymlinks(cache)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("Could not check the installed texture cache: %w", err)
	}
	for _, folder := range replaced {
		inside, err := insideFolder(folder, resolved)
		if err != nil {
			return fmt.Errorf("Could not check the installed texture cache: %w", err)
		}
		if inside {
			return fmt.Errorf("Move the texture cache at %s outside the game and setup folders before making changes.", cache)
		}
	}
	return nil
}
