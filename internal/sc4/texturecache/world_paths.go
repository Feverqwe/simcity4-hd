package texturecache

import (
	"fmt"
	"path/filepath"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

const NetworkGroup = 0x1abe787d
const worldCacheVersion = "v2"

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
	return filepath.Join(cache, "world", upscale.HatCheckpointSHA, fmt.Sprintf("world-fsh-x%d-%s", scale, worldCacheVersion))
}

func (category WorldCategory) CacheFolder(cache string, scale int) string {
	scope := "default"
	if category.Placement {
		scope = "placement"
	}
	return filepath.Join(CategoryCache(cache, scale), category.Name, scope)
}

func NetworkCache(cache string, scale int) string {
	return filepath.Join(cache, "world", upscale.HatCheckpointSHA, fmt.Sprintf("network-fsh-x%d-%s", scale, worldCacheVersion))
}

func ModelCache(cache string, scale int) string {
	return filepath.Join(cache, "models", upscale.HatCheckpointSHA, fmt.Sprintf("fsh-x%d-%s", scale, worldCacheVersion))
}
