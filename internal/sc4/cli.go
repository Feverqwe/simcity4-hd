package sc4

import (
	"fmt"
	"os"
)

const usage = "Usage: simcity4-uhd <ui|world|russian-pack|verify|tutorial|patch-ui-exe|terrain-indexed|network-textures|model-textures|index-materials|catalog-sheets|catalog-chrome|mysim-portraits|stage-profile|launch> ..."

// Run dispatches the command line and returns the process exit code.
func Run(args []string, version string) int {
	if len(args) > 0 && args[0] == "setup" {
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
		err = runBuildUI(args[1:])
	case "world":
		err = runBuildWorld(args[1:])
	case "russian-pack":
		err = runRussianPack(args[1:])
	case "verify":
		err = verifyPackages(args[1:], os.Stdout)
	case "tutorial":
		err = runTutorial(args[1:])
	case "patch-ui-exe":
		err = runUIExecutable(args[1:])
	case "terrain-indexed":
		err = runIndexedTerrain(args[1:])
	case "network-textures":
		err = runNetworkTextures(args[1:])
	case "model-textures":
		err = runModelTextures(args[1:])
	case "index-materials":
		err = runMaterialIndex(args[1:])
	case "catalog-sheets":
		err = runCatalogSheets(args[1:])
	case "catalog-chrome", "mysim-portraits":
		err = runUIAssets(args[0], args[1:])
	case "launch":
		err = runLaunch(args[1:], os.Stdout)
	case "stage-profile":
		err = runStageProfile(args[1:], os.Stdout)
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
