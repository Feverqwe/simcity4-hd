package sc4

import (
	"errors"
	"testing"
)

func TestSetupSpaceCombinesRequestsForSameVolume(t *testing.T) {
	requests := []spaceRequest{
		{path: "install", label: "HD installation", bytes: 6 * gib},
		{path: "cache", label: "texture cache", bytes: 5 * gib},
	}
	lookup := func(path string) (setupVolume, error) {
		if path == "cache" {
			// A second mount path may still use the same disk.
			return setupVolume{id: "same-volume", name: "C:\\CacheMount\\", free: 9 * gib}, nil
		}
		return setupVolume{id: "same-volume", name: "C:\\", free: 9 * gib}, nil
	}
	err := checkVolumeSpace(requests, lookup)
	var insufficient insufficientSetupSpace
	if !errors.As(err, &insufficient) || insufficient.required != 11*gib || insufficient.free != 9*gib {
		t.Fatalf("expected a combined reservation of 11 GiB against 9 GiB, got %v", err)
	}
}

func TestSetupSpaceAllowsInstallAndCacheOnDifferentVolumes(t *testing.T) {
	requests := []spaceRequest{
		{path: "install", label: "HD installation", bytes: 6 * gib},
		{path: "cache", label: "texture cache", bytes: 5 * gib},
	}
	lookup := func(path string) (setupVolume, error) {
		return setupVolume{id: path, name: path, free: 6 * gib}, nil
	}
	if err := checkVolumeSpace(requests, lookup); err != nil {
		t.Fatalf("independent volumes have enough space: %v", err)
	}
}
