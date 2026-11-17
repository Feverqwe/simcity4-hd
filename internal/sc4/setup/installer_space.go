//go:build windows || darwin

package setup

import (
	"errors"
	"fmt"
	"strings"
)

const gib = uint64(1 << 30)

type SpaceRequest struct {
	Path, Purpose string
	Bytes         uint64
}

type setupVolume struct {
	id, name string
	free     uint64
}

type insufficientSetupSpace struct {
	volume, contents string
	required, free   uint64
}

func (e insufficientSetupSpace) Error() string {
	return fmt.Sprintf("Not enough space on %s for %s. About %.1f GiB is needed; %.1f GiB is available. Free up space or change folders in Advanced options.",
		e.volume, e.contents, float64(e.required)/float64(gib), float64(e.free)/float64(gib))
}

func checkVolumeSpace(requests []SpaceRequest, lookup func(string) (setupVolume, error)) error {
	type reservation struct {
		setupVolume
		labels []string
		needed uint64
	}
	byID := make(map[string]*reservation)
	var volumes []*reservation
	for _, request := range requests {
		if request.Bytes == 0 {
			continue
		}
		volume, err := lookup(request.Path)
		if err != nil {
			return fmt.Errorf("could not check disk space for %s: %w", request.Purpose, err)
		}
		group := byID[volume.id]
		if group == nil {
			group = &reservation{setupVolume: volume}
			byID[volume.id] = group
			volumes = append(volumes, group)
		}
		group.free = min(group.free, volume.free)
		group.needed += request.Bytes
		group.labels = append(group.labels, request.Purpose)
	}
	var result error
	for _, volume := range volumes {
		if volume.needed > volume.free {
			result = errors.Join(result, insufficientSetupSpace{
				volume.name, strings.Join(volume.labels, ", "), volume.needed, volume.free,
			})
		}
	}
	return result
}

func CheckSpace(path string, bytes uint64, purpose string) error {
	return CheckSpaceFor(SpaceRequest{path, purpose, bytes})
}

func CheckSpaceFor(requests ...SpaceRequest) error {
	return checkVolumeSpace(requests, lookupSetupVolume)
}
