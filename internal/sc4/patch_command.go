package sc4

import (
	"errors"
	"flag"
	"io"
	"os"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
)

func runUIExecutable(args []string) error {
	flags := flag.NewFlagSet("patch-ui-exe", flag.ContinueOnError)
	scale := flags.Int("texture-scale", 4, "UI texture scale (2 or 4)")
	hdCursors := flags.Bool("hd-cursors", false, "enable the HD game cursor loader")
	cursorScale := flags.Int("cursor-scale", 2, "density of the prepared HD cursor assets (2 or 4)")
	regionMap := flags.Bool("region-map-2x", false, "double the region map")
	loadingPreview := flags.Bool("loading-preview-2x", false, "double the city loading preview")
	russianBudgetTips := flags.Bool("russian-budget-tips", false, "use the Russian budget tooltips")
	retinaText := flags.Bool("retina-text", false, "enable experimental Retina text on macOS")
	if err := flags.Parse(args); err != nil {
		return err
	}
	args = flags.Args()
	if *scale != 2 && *scale != 4 {
		return errors.New("texture scale must be 2 or 4")
	}
	if *cursorScale != 2 && *cursorScale != 4 {
		return errors.New("cursor scale must be 2 or 4")
	}
	if len(args) != 2 {
		return errors.New("usage: simcity4-uhd patch-ui-exe GAME_DIR_OR_APP OUTPUT")
	}
	_, mac, err := gamefiles.DataDirectory(args[0])
	if err != nil {
		return err
	}
	data, err := os.ReadFile(gamefiles.ExecutablePath(args[0], mac))
	if err != nil {
		return err
	}
	data, err = exepatch.Patch(data, mac, *scale, x86.Options{
		HdCursors: *hdCursors, LoadingPreview: *loadingPreview, RegionMap: *regionMap, RussianBudgetTips: *russianBudgetTips, RetinaText: *retinaText, CursorScale: *cursorScale,
	})
	if err != nil {
		return err
	}
	return gamefiles.PublishFile(args[1], func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}
