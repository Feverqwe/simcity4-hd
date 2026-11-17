//go:build windows || darwin

package windows

import (
	"context"
	"path/filepath"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
)

const mib = uint64(1 << 20)

func checkBuildSpace(ctx context.Context, build setup.TextureBuild, renderer bool) error {
	if !renderer && !build.Interface && !build.World.Any() {
		return nil
	}
	packages, textures, err := build.EstimateSpace(ctx)
	if err != nil {
		return err
	}
	space := []setup.SpaceRequest{
		{Path: build.Game, Purpose: "the HD game files", Bytes: packages + 512*mib},
		{Path: build.Cache, Purpose: "the texture cache", Bytes: textures},
	}
	if renderer {
		space = append(space, setup.SpaceRequest{
			Path: filepath.Join(build.Cache, "SetupDependencies"), Purpose: "renderer and graphics plugin downloads", Bytes: 64 * mib,
		})
	}
	return setup.CheckSpaceFor(space...)
}
