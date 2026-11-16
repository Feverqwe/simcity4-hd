package sc4

import (
	"fmt"
	"os"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/advisors"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/launch"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/russian"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
	uibuild "github.com/Feverqwe/simcity4-hd/internal/sc4/ui"
	worldbuild "github.com/Feverqwe/simcity4-hd/internal/sc4/world"
)

const usage = "Usage: simcity4-uhd <ui|world|russian-pack|advisors|verify|tutorial|patch-ui-exe|terrain-indexed|network-textures|model-textures|index-materials|catalog-sheets|catalog-chrome|mysim-portraits|launch> ..."

// Run dispatches the command line and returns the process exit code.
func Run(args []string, version string) int {
	if len(args) > 0 && args[0] == "setup" {
		if len(args) == 2 && args[1] == "check-update" {
			if err := setup.CheckUpdate(version, os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			return 0
		}
		return runInstaller(args[1:])
	}
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println(version)
		return 0
	}
	if len(args) == 1 && args[0] == "--help" {
		fmt.Println(usage)
		return 0
	}
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "ui":
		err = uibuild.Run(args[1:])
	case "world":
		err = worldbuild.Run(args[1:])
	case "russian-pack":
		err = russian.Run(args[1:])
	case "advisors":
		err = advisors.Run(args[1:])
	case "verify":
		err = gamefiles.VerifyPackages(args[1:], os.Stdout)
	case "tutorial":
		err = uibuild.RunTutorial(args[1:])
	case "patch-ui-exe":
		err = runUIExecutable(args[1:])
	case "terrain-indexed":
		err = worldbuild.RunIndexedTerrain(args[1:])
	case "network-textures":
		err = worldbuild.RunNetworkTextures(args[1:])
	case "model-textures":
		err = worldbuild.RunModelTextures(args[1:])
	case "index-materials":
		err = worldbuild.RunMaterialIndex(args[1:])
	case "catalog-sheets":
		err = uibuild.RunCatalogSheets(args[1:])
	case "catalog-chrome", "mysim-portraits":
		err = uibuild.RunUIAssets(args[0], args[1:])
	case "launch":
		err = launch.Run(args[1:], os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q. Use --help for the command list.\n", args[0])
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
