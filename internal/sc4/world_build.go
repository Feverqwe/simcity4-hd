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

func runBuildWorld(args []string) (result error) {
	flags := flag.NewFlagSet("world", flag.ContinueOnError)
	readInferenceConfig := inferenceFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	config, err := readInferenceConfig()
	if err != nil {
		return err
	}
	if flags.NArg() != 3 {
		return errors.New("usage: simcity4-hd world [OPTIONS] GAME_DIR CACHE NEW_OUTPUT_DIR")
	}
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
		return publishFile(filepath.Join(plugins, name), func(w io.Writer) error {
			return writeArchive(w, entries)
		})
	}

	report, err := indexMaterials(game)
	if err != nil {
		return err
	}
	stock, err := openGameCatalog(game, func(id resourceID) bool {
		return id.kind == fshResource || id.kind == s3dResource && id.group == stationModelGroup
	})
	if err != nil {
		return err
	}
	defer stock.close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	art, closeWorker := worldInference(ctx, cache, config)
	defer func() {
		if result != nil {
			cancel()
		}
		result = errors.Join(result, closeWorker())
	}()
	network, err := networkEntries(stock, report, art)
	if err != nil {
		return fmt.Errorf("build network: %w", err)
	}
	if err := write("network-x4.dat", network); err != nil {
		return err
	}
	indexed, err := indexedTerrainEntries(game)
	if err != nil {
		return err
	}
	if err := write("terrain-indexed-x4.dat", indexed); err != nil {
		return err
	}
	supplied, err := buildModelBatches(stock, report, art, plugins)
	if err != nil {
		return fmt.Errorf("build models: %w", err)
	}
	if err := buildCategoryBatches(stock, art, plugins, supplied); err != nil {
		return err
	}
	if err := closeWorker(); err != nil {
		return err
	}
	config.scale, config.tile, config.overlap = 4, 48, 12
	art, closeWorker = worldInference(ctx, cache, config)
	day, night, err := stationEntries(stock, art)
	if err != nil {
		return fmt.Errorf("build fire station: %w", err)
	}
	if err := closeWorker(); err != nil {
		return err
	}
	if err := write("fire-station-x4.dat", day); err != nil {
		return err
	}
	if err := write("fire-station-night-x4.dat", night); err != nil {
		return err
	}
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
