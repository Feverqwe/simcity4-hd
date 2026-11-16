package game

import (
	"path/filepath"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
)

type Catalog struct {
	Archives []*dbpf.Archive
	Order    []dbpf.ID
	Owners   map[dbpf.ID]ResourceLocation
}

// openGameCatalog preserves the first occurrence order while giving later
// game archives priority for duplicate resource IDs.
func OpenCatalog(game string, include func(dbpf.ID) bool) (*Catalog, error) {
	c := &Catalog{Owners: make(map[dbpf.ID]ResourceLocation)}
	for _, name := range []string{"SimCity_1.dat", "SimCity_2.dat", "SimCity_3.dat",
		"SimCity_4.dat", "SimCity_5.dat", "EP1.dat"} {
		a, err := dbpf.Open(filepath.Join(game, name))
		if err != nil {
			c.Close()
			return nil, err
		}
		c.Archives = append(c.Archives, a)
		for _, r := range a.Records {
			if !include(r.ID) {
				continue
			}
			if _, exists := c.Owners[r.ID]; !exists {
				c.Order = append(c.Order, r.ID)
			}
			c.Owners[r.ID] = ResourceLocation{a, r}
		}
	}
	return c, nil
}

func (c *Catalog) Close() {
	for _, a := range c.Archives {
		a.Close()
	}
}
