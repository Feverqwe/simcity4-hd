package world

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

type Selection struct {
	Network, Terrain, Models bool
}

func (s Selection) Any() bool { return s.Network || s.Terrain || s.Models }

func Package(name string) (option, scale string) {
	base, ok := strings.CutSuffix(name, ".dat")
	if !ok {
		return "", ""
	}
	marker := strings.LastIndex(base, "-x")
	if marker < 0 {
		return "", ""
	}
	scale, _, _ = strings.Cut(base[marker+2:], "-")
	if scale != "2" && scale != "4" {
		return "", ""
	}
	switch prefix := base[:marker]; {
	case prefix == "network":
		return "network", scale
	case prefix == "terrain-indexed", strings.HasPrefix(prefix, "world-terrain-"):
		return "terrain", scale
	case prefix == "model-overrides", strings.HasPrefix(prefix, "world-"), strings.HasPrefix(prefix, "model-materials"):
		return "models", scale
	}
	return "", ""
}

func Run(args []string) error {
	return BuildWithProgress(context.Background(), args, upscale.Progress{}, Selection{Network: true, Terrain: true, Models: true})
}

func BuildWithProgress(ctx context.Context, args []string, progress upscale.Progress, selection Selection) (result error) {
	flags := flag.NewFlagSet("world", flag.ContinueOnError)
	scale := flags.Int("texture-scale", 4, "world texture scale (2 or 4)")
	readInferenceConfig := upscale.InferenceFlags(flags)
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
	config.Scale = *scale
	game, cache, output := flags.Arg(0), flags.Arg(1), flags.Arg(2)
	game, _, err = gamefiles.DataDirectory(game)
	if err != nil {
		return err
	}
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
	write := func(name string, entries []dbpf.Entry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return gamefiles.PublishFile(filepath.Join(plugins, name), func(w io.Writer) error {
			return dbpf.Write(w, entries)
		})
	}

	progress.Report("Reading game textures")
	var report *materialReport
	if selection.Network || selection.Models {
		report, err = indexMaterials(game)
		if err != nil {
			return err
		}
	}
	stock, err := gamefiles.OpenCatalog(game, func(id dbpf.ID) bool {
		return id.Kind == textures.FSHResource || selection.Models && id.Kind == S3DResource && id.Group == stationModelGroup
	})
	if err != nil {
		return err
	}
	defer stock.Close()
	config.Prepare = progress.Prepare
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	art, closeWorker := worldInference(ctx, cache, config)
	defer func() {
		if result != nil {
			cancel()
		}
		result = errors.Join(result, closeWorker())
	}()
	if selection.Network {
		progress.Report("Roads and rail")
		network, err := networkEntries(stock, report, art, progress)
		if err != nil {
			return fmt.Errorf("build network: %w", err)
		}
		if err := write(fmt.Sprintf("network-x%d.dat", *scale), network); err != nil {
			return err
		}
	}
	if selection.Terrain {
		progress.Report("Terrain")
		indexed, err := indexedTerrainEntries(game, *scale, progress)
		if err != nil {
			return err
		}
		if err := write(fmt.Sprintf("terrain-indexed-x%d.dat", *scale), indexed); err != nil {
			return err
		}
	}
	var supplied map[dbpf.ID]bool
	if selection.Models {
		progress.Report("Buildings, props and effects")
		supplied, err = buildModelBatches(stock, report, art, plugins, progress)
		if err != nil {
			return fmt.Errorf("build models: %w", err)
		}
	}
	if err := buildCategoryBatches(stock, art, plugins, supplied, selection, progress); err != nil {
		return err
	}
	if selection.Models {
		progress.Report("Preparing model material overrides")
		day, night, err := stationEntries(stock, art, progress)
		if err != nil {
			return fmt.Errorf("build fire station: %w", err)
		}
		if err := write(fmt.Sprintf("model-overrides-x%d.dat", *scale), append(day, night...)); err != nil {
			return err
		}
	}
	if err := closeWorker(); err != nil {
		return err
	}
	progress.Report("Checking world files")
	paths, err := gamefiles.PackagePaths([]string{plugins})
	if err != nil {
		return err
	}
	if err := gamefiles.VerifyPackages(paths, io.Discard); err != nil {
		return err
	}
	if err := os.Rename(stage, output); err != nil {
		return err
	}
	fmt.Printf("%d World packages\n", len(paths))
	return nil
}
