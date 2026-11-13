package sc4

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
)

type worldCategory struct {
	name      string
	group     uint32
	placement bool
}

var worldCategories = []worldCategory{
	{"terrain", 0x891b0e1a, false},
	{"lot", 0x0986135e, false},
	{"animation", 0x49a593e7, false},
	{"transport", networkGroup, false},
	{"lot", 0x0986135e, true},
	{"animation", 0x49a593e7, true},
}

func categoryTextureSupported(category worldCategory, data []byte) bool {
	directory := ""
	if len(data) >= 16 {
		directory = string(data[12:16])
	}
	if category.placement && directory != "G264" || category.name == "transport" && directory != "G315" {
		return false
	}
	elements, err := fshLayout(data)
	if err != nil {
		return false
	}
	placed := false
	for _, e := range elements {
		if category.placement {
			x, y := e.offsetX&0xfff, e.offsetY&0xfff
			if e.centerX != 0 || e.centerY != 0 || y != 0 || (x != 0 && x != 1 && x != 8) {
				return false
			}
			placed = placed || x != 0
		}
		if _, err := validateBCElement(data, e, 2, category.placement || category.name == "transport"); err != nil {
			return false
		}
	}
	return !category.placement || placed
}

func buildCategoryBatches(stock *gameCatalog, art worldArtwork, output string, supplied map[resourceID]bool, selection worldSelection, progress buildProgress) error {
	for _, category := range worldCategories {
		if category.name == "terrain" && !selection.terrain || category.name != "terrain" && !selection.models {
			continue
		}
		label := "Additional textures: " + category.name
		if category.placement {
			label += " (with placement)"
		}
		progress.report(label)
		if err := buildWorldCategory(stock, art, output, category, supplied, progress); err != nil {
			return fmt.Errorf("%s: %w", category.name, err)
		}
	}
	return nil
}

func worldCategoryCache(cache string, scale int) string {
	return filepath.Join(cache, "world", hatCheckpointSHA, fmt.Sprintf("world-fsh-x%d-%s", scale, worldCacheVersion))
}

func buildWorldCategory(stock *gameCatalog, art worldArtwork, output string, category worldCategory, supplied map[resourceID]bool, progress buildProgress) error {
	var ids []resourceID
	for _, id := range stock.order {
		if id.group == category.group && !(category.placement && supplied[id]) {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b resourceID) int {
		switch {
		case a.instance < b.instance:
			return -1
		case a.instance > b.instance:
			return 1
		default:
			return 0
		}
	})
	scope, prefix := "default", fmt.Sprintf("world-%s-batch-x%d", category.name, art.scale)
	limit := 0
	if category.placement {
		scope, prefix = "placement", fmt.Sprintf("world-%s-placement-candidate-x%d", category.name, art.scale)
		limit = 128 << 20
	}
	cache := func(scale int) string {
		return filepath.Join(worldCategoryCache(art.cache, scale), category.name, scope)
	}
	batch, count := 0, 0
	size := headerSize
	var entries []resource
	write := func() error {
		batch++
		name := prefix + ".dat"
		if limit != 0 {
			name = fmt.Sprintf("%s-%03d.dat", prefix, batch)
		}
		return publishFile(filepath.Join(output, name), func(w io.Writer) error {
			return writeArchive(w, entries)
		})
	}
	report := progress.track(len(ids))
	for i, id := range ids {
		report(i)
		location := stock.owners[id]
		data, err := location.archive.read(location.record)
		if err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		if !categoryTextureSupported(category, data) {
			continue
		}
		data, err = art.texture(cache, data, category.placement || category.name == "transport")
		if err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		entrySize := len(data) + indexEntrySize
		if limit != 0 {
			if headerSize+entrySize > limit {
				return fmt.Errorf("%s exceeds the package size limit", id)
			}
			if size+entrySize > limit {
				if err := write(); err != nil {
					return err
				}
				entries = nil
				size = headerSize
			}
		}
		entries = append(entries, resource{id, data})
		size += entrySize
		count++
		if count%250 == 0 {
			fmt.Printf("%s: %d textures prepared\n", prefix, count)
		}
	}
	report(len(ids))
	if len(entries) != 0 {
		if err := write(); err != nil {
			return err
		}
	}
	fmt.Printf("%s: %d textures in %d packages\n", prefix, count, batch)
	return nil
}
