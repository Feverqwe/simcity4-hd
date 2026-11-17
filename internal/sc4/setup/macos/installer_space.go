//go:build windows || darwin

package macos

import (
	"context"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
)

func checkAspyrSpace(ctx context.Context, parent string, stagedBytes uint64, build *setup.TextureBuild) error {
	if build == nil {
		return setup.CheckSpace(parent, stagedBytes, "the installation")
	}
	packages, textures, err := build.EstimateSpace(ctx)
	if err != nil {
		return err
	}
	// Prepared packages and their staged copies coexist before publication.
	return setup.CheckSpaceFor(
		setup.SpaceRequest{Path: parent, Purpose: "the installation", Bytes: stagedBytes + 2*packages},
		setup.SpaceRequest{Path: build.Cache, Purpose: "the texture cache", Bytes: textures},
	)
}
