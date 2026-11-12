package sc4

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type worldSelection struct {
	network, terrain, models bool
}

func (s worldSelection) any() bool { return s.network || s.terrain || s.models }

func runBuildWorld(args []string) error {
	return buildWorldWithProgress(context.Background(), args, buildProgress{}, worldSelection{network: true, terrain: true, models: true})
}

func buildWorldWithProgress(ctx context.Context, args []string, progress buildProgress, selection worldSelection) (result error) {
	flags := flag.NewFlagSet("world", flag.ContinueOnError)
	scale := flags.Int("texture-scale", 4, "world texture scale (2 or 4)")
	readInferenceConfig := inferenceFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	config, err := readInferenceConfig()
	if err != nil {
		return err
	}
	if flags.NArg() != 3 {
		return errors.New("usage: simcity4-uhd world [OPTIONS] GAME_DIR CACHE NEW_OUTPUT_DIR")
	}
	if *scale != 2 && *scale != 4 {
		return errors.New("world texture scale must be 2 or 4")
	}
	config.scale = *scale
	game, cache, output := flags.Arg(0), flags.Arg(1), flags.Arg(2)
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return fmt.Errorf("%s already exists", output)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(output), ".sc4-hd-world-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	plugins := filepath.Join(stage, "Plugins")
	if err := os.Mkdir(plugins, 0755); err != nil {
		return err
	}
	write := func(name string, entries []resource) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return publishFile(filepath.Join(plugins, name), func(w io.Writer) error {
			return writeArchive(w, entries)
		})
	}

	progress.report("Reading game textures")
	var report *materialReport
	if selection.network || selection.models {
		report, err = indexMaterials(game)
		if err != nil {
			return err
		}
	}
	stock, err := openGameCatalog(game, func(id resourceID) bool {
		return id.kind == fshResource || selection.models && id.kind == s3dResource && id.group == stationModelGroup
	})
	if err != nil {
		return err
	}
	defer stock.close()
	config.prepare = progress.prepare
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	art, closeWorker := worldInference(ctx, cache, config)
	defer func() {
		if result != nil {
			cancel()
		}
		result = errors.Join(result, closeWorker())
	}()
	if selection.network {
		progress.report("Roads and rail")
		network, err := networkEntries(stock, report, art, progress)
		if err != nil {
			return fmt.Errorf("build network: %w", err)
		}
		if err := write(fmt.Sprintf("network-x%d.dat", *scale), network); err != nil {
			return err
		}
	}
	if selection.terrain {
		progress.report("Terrain")
		indexed, err := indexedTerrainEntries(game, *scale, progress)
		if err != nil {
			return err
		}
		if err := write(fmt.Sprintf("terrain-indexed-x%d.dat", *scale), indexed); err != nil {
			return err
		}
	}
	var supplied map[resourceID]bool
	if selection.models {
		progress.report("Buildings, props and effects")
		supplied, err = buildModelBatches(stock, report, art, plugins, progress)
		if err != nil {
			return fmt.Errorf("build models: %w", err)
		}
	}
	if err := buildCategoryBatches(stock, art, plugins, supplied, selection, progress); err != nil {
		return err
	}
	if err := closeWorker(); err != nil {
		return err
	}
	if selection.models {
		config.tile, config.overlap = 48, 12
		art, closeWorker = worldInference(ctx, cache, config)
		progress.report("Preparing model material overrides")
		day, night, err := stationEntries(stock, art, progress)
		if err != nil {
			return fmt.Errorf("build fire station: %w", err)
		}
		if err := closeWorker(); err != nil {
			return err
		}
		if err := write(fmt.Sprintf("model-overrides-x%d.dat", *scale), append(day, night...)); err != nil {
			return err
		}
	}
	progress.report("Checking world files")
	paths, err := packagePaths([]string{plugins})
	if err != nil {
		return err
	}
	if err := verifyPackages(paths, io.Discard); err != nil {
		return err
	}
	if err := os.Rename(stage, output); err != nil {
		return err
	}
	fmt.Printf("%d World packages\n", len(paths))
	return nil
}
