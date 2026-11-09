package sc4

import "path/filepath"

type gameCatalog struct {
	archives []*archive
	order    []resourceID
	owners   map[resourceID]resourceLocation
}

// openGameCatalog preserves the first occurrence order while giving later
// game archives priority for duplicate resource IDs.
func openGameCatalog(game string, include func(resourceID) bool) (*gameCatalog, error) {
	c := &gameCatalog{owners: make(map[resourceID]resourceLocation)}
	for _, name := range []string{"SimCity_1.dat", "SimCity_2.dat", "SimCity_3.dat",
		"SimCity_4.dat", "SimCity_5.dat", "EP1.dat"} {
		a, err := openArchive(filepath.Join(game, name))
		if err != nil {
			c.close()
			return nil, err
		}
		c.archives = append(c.archives, a)
		for _, r := range a.records {
			if !include(r.id) {
				continue
			}
			if _, exists := c.owners[r.id]; !exists {
				c.order = append(c.order, r.id)
			}
			c.owners[r.id] = resourceLocation{a, r}
		}
	}
	return c, nil
}

func (c *gameCatalog) close() {
	for _, a := range c.archives {
		a.file.Close()
	}
}
