package world

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/texturecache"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

func categoryTextureSupported(category texturecache.WorldCategory, data []byte) bool {
	directory := ""
	if len(data) >= 16 {
		directory = string(data[12:16])
	}
	if category.Placement && directory != "G264" || category.Name == "transport" && directory != "G315" {
		return false
	}
	elements, err := textures.FSHLayout(data)
	if err != nil {
		return false
	}
	placed := false
	for _, e := range elements {
		if category.Placement {
			x, y := e.OffsetX&0xfff, e.OffsetY&0xfff
			if e.CenterX != 0 || e.CenterY != 0 || y != 0 || (x != 0 && x != 1 && x != 8) {
				return false
			}
			placed = placed || x != 0
		}
		if _, err := textures.ValidateBCElement(data, e, 2, category.Placement || category.Name == "transport"); err != nil {
			return false
		}
	}
	return !category.Placement || placed
}

func buildCategoryBatches(stock *gamefiles.Catalog, art worldArtwork, output string, supplied map[dbpf.ID]bool, selection Selection, progress upscale.Progress) error {
	for _, category := range texturecache.WorldCategories {
		if category.Name == "terrain" && !selection.Terrain || category.Name != "terrain" && !selection.Models {
			continue
		}
		label := "Additional textures: " + category.Name
		if category.Placement {
			label += " (with placement)"
		}
		progress.Report(label)
		if err := buildWorldCategory(stock, art, output, category, supplied, progress); err != nil {
			return fmt.Errorf("%s: %w", category.Name, err)
		}
	}
	return nil
}

func buildWorldCategory(stock *gamefiles.Catalog, art worldArtwork, output string, category texturecache.WorldCategory, supplied map[dbpf.ID]bool, progress upscale.Progress) error {
	var ids []dbpf.ID
	for _, id := range stock.Order {
		if id.Group == category.Group && !(category.Placement && supplied[id]) {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b dbpf.ID) int {
		switch {
		case a.Instance < b.Instance:
			return -1
		case a.Instance > b.Instance:
			return 1
		default:
			return 0
		}
	})
	prefix := fmt.Sprintf("world-%s-batch-x%d", category.Name, art.scale)
	limit := 0
	if category.Placement {
		prefix = fmt.Sprintf("world-%s-placement-candidate-x%d", category.Name, art.scale)
		limit = 128 << 20
	}
	cache := func(scale int) string {
		return category.CacheFolder(art.cache, scale)
	}
	batch, count := 0, 0
	size := dbpf.HeaderSize
	var entries []dbpf.Entry
	write := func() error {
		batch++
		name := prefix + ".dat"
		if limit != 0 {
			name = fmt.Sprintf("%s-%03d.dat", prefix, batch)
		}
		return gamefiles.PublishFile(filepath.Join(output, name), func(w io.Writer) error {
			return dbpf.Write(w, entries)
		})
	}
	report := progress.Track(len(ids))
	for i, id := range ids {
		report(i)
		location := stock.Owners[id]
		data, err := location.Archive.Read(location.Record)
		if err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		if !categoryTextureSupported(category, data) {
			continue
		}
		data, err = art.texture(cache, data, category.Placement || category.Name == "transport")
		if err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		entrySize := len(data) + dbpf.IndexEntrySize
		if limit != 0 {
			if dbpf.HeaderSize+entrySize > limit {
				return fmt.Errorf("%s exceeds the package size limit", id)
			}
			if size+entrySize > limit {
				if err := write(); err != nil {
					return err
				}
				entries = nil
				size = dbpf.HeaderSize
			}
		}
		entries = append(entries, dbpf.Entry{ID: id, Data: data})
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
