//go:build windows || darwin

package setup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/texturecache"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
	worldbuild "github.com/Feverqwe/simcity4-hd/internal/sc4/world"
)

const mib = uint64(1 << 20)

func cacheDirSize(ctx context.Context, root string) (uint64, error) {
	var size uint64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == root && errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) == ".tmp" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		size += uint64(info.Size())
		return nil
	})
	return size, err
}

// EstimateSpace returns headroom for the selected generated packages and
// additional cache data, excluding dependencies and staged installation copies.
func (build TextureBuild) EstimateSpace(ctx context.Context) (packageBytes, cacheBytes uint64, err error) {
	cache := build.Cache
	// Reservations include headroom over the measured packages and cache.
	// Count only cache families used by the selected components.
	reserve := func(installed, desired uint64, folders ...string) error {
		packageBytes += installed
		var cached uint64
		for _, folder := range folders {
			size, err := cacheDirSize(ctx, folder)
			if err != nil {
				return fmt.Errorf("measure texture cache: %w", err)
			}
			cached += size
		}
		if cached < desired {
			cacheBytes += desired - cached
		}
		return nil
	}
	if build.Interface {
		folders := []string{filepath.Join(cache, upscale.InterfaceAtlasFolder), filepath.Join(cache, upscale.CatalogFramesFolder),
			filepath.Join(cache, upscale.PortraitsFolder)}
		if build.Features.HdCursors {
			folders = append(folders, filepath.Join(cache, upscale.CursorsFolder))
		}
		if err := reserve(512*mib, 512*mib, folders...); err != nil {
			return 0, 0, err
		}
	}
	interfaceCacheBytes := cacheBytes
	worldFolders := func(selection worldbuild.Selection, scale int) []string {
		var folders []string
		categories := texturecache.CategoryCache(cache, scale)
		if selection.Network {
			folders = append(folders, texturecache.NetworkCache(cache, scale))
		}
		if selection.Terrain {
			folders = append(folders, filepath.Join(categories, "terrain"))
		}
		if selection.Models {
			folders = append(folders, texturecache.ModelCache(cache, scale))
			for _, name := range []string{"lot", "animation", "transport"} {
				folders = append(folders, filepath.Join(categories, name))
			}
		}
		return folders
	}
	for _, component := range []struct {
		selection worldbuild.Selection
		installed uint64
		cached    uint64
	}{
		{worldbuild.Selection{Network: build.World.Network}, 160 * mib, 512 * mib},
		{worldbuild.Selection{Terrain: build.World.Terrain}, 128 * mib, 160 * mib},
		{worldbuild.Selection{Models: build.World.Models}, 22*gib + 64*mib, 26*gib + 64*mib},
	} {
		if !component.selection.Any() {
			continue
		}
		installed := component.installed
		cached := component.cached
		if build.Scale == 2 {
			installed /= 3
			cached /= 3
		}
		if err := reserve(installed, cached, worldFolders(component.selection, build.Scale)...); err != nil {
			return 0, 0, err
		}
	}
	if imageBytes := cacheBytes - interfaceCacheBytes; build.KeepUpscaledImages && cacheBytes > 0 {
		// Missing FSH may also need full-resolution inference output. Count
		// existing PNGs against that estimate, not against the FSH we will write.
		if build.Scale == 2 {
			imageBytes *= 3
		}
		imageBytes += interfaceCacheBytes
		images, err := cacheDirSize(ctx, filepath.Join(cache, upscale.InferenceCacheFolder))
		if err != nil {
			return 0, 0, fmt.Errorf("measure upscaled image cache: %w", err)
		}
		cacheBytes += imageBytes - min(images, imageBytes)
	}
	return packageBytes, cacheBytes, nil
}
