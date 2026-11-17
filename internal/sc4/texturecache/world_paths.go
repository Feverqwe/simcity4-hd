package texturecache

import (
	"fmt"
	"path/filepath"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

const NetworkGroup = 0x1abe787d
const worldCacheVersion = "v2"
const PreparedFolder = "Textures-prepared"

func InterfaceCache(cache string, scale int) string {
	density := "Shared"
	if scale != 1 {
		density = fmt.Sprintf("x%d", scale)
	}
	return filepath.Join(cache, PreparedFolder, "Interface", density)
}

func worldCache(cache string, scale int) string {
	return filepath.Join(cache, PreparedFolder, "World", fmt.Sprintf("x%d", scale), upscale.HatCheckpointSHA, worldCacheVersion)
}

type WorldCategory struct {
	Name      string
	Group     uint32
	Placement bool
}

var WorldCategories = []WorldCategory{
	{Name: "terrain", Group: 0x891b0e1a, Placement: false},
	{Name: "lot", Group: 0x0986135e, Placement: false},
	{Name: "animation", Group: 0x49a593e7, Placement: false},
	{Name: "transport", Group: NetworkGroup, Placement: false},
	{Name: "lot", Group: 0x0986135e, Placement: true},
	{Name: "animation", Group: 0x49a593e7, Placement: true},
}

func CategoryCache(cache string, scale int) string {
	return filepath.Join(worldCache(cache, scale), "Categories")
}

func (category WorldCategory) CacheFolder(cache string, scale int) string {
	scope := "default"
	if category.Placement {
		scope = "placement"
	}
	return filepath.Join(CategoryCache(cache, scale), category.Name, scope)
}

func NetworkCache(cache string, scale int) string {
	return filepath.Join(worldCache(cache, scale), "Network")
}

func ModelCache(cache string, scale int) string {
	return filepath.Join(worldCache(cache, scale), "Models")
}
