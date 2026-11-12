package sc4

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	gib = uint64(1 << 30)
	mib = uint64(1 << 20)
)

type spaceRequest struct {
	path, label string
	bytes       uint64
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

func checkVolumeSpace(requests []spaceRequest, lookup func(string) (setupVolume, error)) error {
	type reservation struct {
		setupVolume
		labels []string
		needed uint64
	}
	byID := make(map[string]*reservation)
	var volumes []*reservation
	for _, request := range requests {
		if request.bytes == 0 {
			continue
		}
		volume, err := lookup(request.path)
		if err != nil {
			return fmt.Errorf("could not check disk space for %s: %w", request.label, err)
		}
		group := byID[volume.id]
		if group == nil {
			group = &reservation{setupVolume: volume}
			byID[volume.id] = group
			volumes = append(volumes, group)
		}
		group.free = min(group.free, volume.free)
		group.needed += request.bytes
		group.labels = append(group.labels, request.label)
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

var (
	getVolumePath = syscall.NewLazyDLL("kernel32.dll").NewProc("GetVolumePathNameW")
	getVolumeID   = syscall.NewLazyDLL("kernel32.dll").NewProc("GetVolumeNameForVolumeMountPointW")
	getDiskFree   = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")
)

func lookupSetupVolume(path string) (setupVolume, error) {
	for {
		info, err := os.Stat(path)
		if err == nil {
			if !info.IsDir() {
				return setupVolume{}, fmt.Errorf("%s is not a folder", path)
			}
			break
		}
		parent := filepath.Dir(path)
		if !errors.Is(err, os.ErrNotExist) || parent == path {
			return setupVolume{}, err
		}
		path = parent
	}
	ptr, err := syscall.UTF16PtrFromString(strings.TrimRight(path, `\/`) + `\`)
	if err != nil {
		return setupVolume{}, err
	}
	var free uint64
	if ok, _, err := getDiskFree.Call(uintptr(unsafe.Pointer(ptr)), uintptr(unsafe.Pointer(&free)), 0, 0); ok == 0 {
		return setupVolume{}, err
	}
	root := make([]uint16, 32768)
	if ok, _, err := getVolumePath.Call(uintptr(unsafe.Pointer(ptr)), uintptr(unsafe.Pointer(&root[0])), uintptr(len(root))); ok == 0 {
		return setupVolume{}, err
	}
	name := syscall.UTF16ToString(root)
	id := name
	guid := make([]uint16, 64)
	if ok, _, _ := getVolumeID.Call(uintptr(unsafe.Pointer(&root[0])), uintptr(unsafe.Pointer(&guid[0])), uintptr(len(guid))); ok != 0 {
		id = syscall.UTF16ToString(guid)
	}
	return setupVolume{id: strings.ToLower(id), name: name, free: free}, nil
}

func setupDirSize(ctx context.Context, root string, ignoreTemporary bool) (uint64, error) {
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
		if !entry.Type().IsRegular() || ignoreTemporary && filepath.Ext(entry.Name()) == ".tmp" {
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

func checkSetupSpace(ctx context.Context, game, dependencies, cache string, options setupOptions) error {
	installBytes := 512 * mib
	var cacheBytes uint64
	// Reservations include headroom over the measured packages and cache.
	// Count only cache families used by the selected components.
	reserve := func(installed, desired uint64, folders ...string) error {
		installBytes += installed
		var cached uint64
		for _, folder := range folders {
			size, err := setupDirSize(ctx, folder, true)
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
	if options.ui {
		folders := []string{filepath.Join(cache, "ui-atlases-x4"), filepath.Join(cache, "ui-catalog-chrome"),
			filepath.Join(cache, "ui-portraits-x4")}
		if options.interfaceOptions.hdCursors {
			folders = append(folders, filepath.Join(cache, "ui-cursors-x4"))
		}
		if err := reserve(512*mib, 512*mib, folders...); err != nil {
			return err
		}
	}
	worldCache := filepath.Join(cache, "world", hatCheckpointSHA)
	worldFolders := func(selection worldSelection, scale int) []string {
		var folders []string
		categories := filepath.Join(worldCache, fmt.Sprintf("world-fsh-x%d-v1", scale))
		if selection.network {
			folders = append(folders, filepath.Join(worldCache, fmt.Sprintf("network-fsh-x%d-v1", scale)))
		}
		if selection.terrain {
			folders = append(folders, filepath.Join(categories, "terrain"))
		}
		if selection.models {
			folders = append(folders,
				filepath.Join(cache, "models", hatCheckpointSHA, fmt.Sprintf("fsh-x%d-v1", scale)),
				filepath.Join(cache, "fire-station-night", hatCheckpointSHA, fmt.Sprintf("x%d", scale)))
			if scale == 4 {
				folders = append(folders, filepath.Join(cache, "fire-station", hatCheckpointSHA))
			}
			for _, name := range []string{"lot", "animation", "transport"} {
				folders = append(folders, filepath.Join(categories, name))
			}
		}
		return folders
	}
	for _, component := range []struct {
		selection worldSelection
		installed uint64
		cached    uint64
	}{
		{worldSelection{network: options.world.network}, 160 * mib, 512 * mib},
		{worldSelection{terrain: options.world.terrain}, 128 * mib, 160 * mib},
		{worldSelection{models: options.world.models}, 22*gib + 64*mib, 26*gib + 64*mib},
	} {
		if !component.selection.any() {
			continue
		}
		if options.textureScale == 2 {
			// x2 keeps both the source x4 textures and the projected x2 textures.
			if err := reserve(0, component.cached, worldFolders(component.selection, 4)...); err != nil {
				return err
			}
			if err := reserve(component.installed/3, component.cached/3, worldFolders(component.selection, 2)...); err != nil {
				return err
			}
		} else if err := reserve(component.installed, component.cached, worldFolders(component.selection, 4)...); err != nil {
			return err
		}
	}
	return checkVolumeSpace([]spaceRequest{
		{game, "the HD installation", installBytes},
		{cache, "the texture cache", cacheBytes},
		{dependencies, "renderer and graphics plugin downloads", 64 * mib},
	}, lookupSetupVolume)
}
