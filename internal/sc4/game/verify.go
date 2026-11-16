package game

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
)

type ResourceLocation struct {
	Archive *dbpf.Archive
	Record  dbpf.Record
}

func VerifyPackages(paths []string, output io.Writer) error {
	if len(paths) == 0 {
		return errors.New("verify requires at least one DAT path")
	}
	owners := make(map[dbpf.ID]ResourceLocation)
	overlaps, conflicts := 0, 0
	for _, path := range paths {
		a, err := dbpf.Open(path)
		if err != nil {
			return err
		}
		defer a.Close()
		fmt.Fprintf(output, "%s: %d resources\n", path, len(a.Records))
		for _, r := range a.Records {
			previous, exists := owners[r.ID]
			if !exists {
				if err := a.VerifyCompression(r); err != nil {
					return fmt.Errorf("%s %s: %w", path, r.ID, err)
				}
				owners[r.ID] = ResourceLocation{a, r}
				continue
			}
			first, err := previous.Archive.Read(previous.Record)
			if err != nil {
				return fmt.Errorf("%s %s: %w", previous.Archive.Name(), r.ID, err)
			}
			second, err := a.Read(r)
			if err != nil {
				return fmt.Errorf("%s %s: %w", path, r.ID, err)
			}
			overlaps++
			kind := "identical"
			if !bytes.Equal(first, second) {
				kind = "conflicting"
				conflicts++
			}
			fmt.Fprintf(output, "%s: %s in %s and %s\n", r.ID, kind, previous.Archive.Name(), path)
		}
	}
	fmt.Fprintf(output, "%d distinct resources, %d overlaps, %d conflicts\n", len(owners), overlaps, conflicts)
	if conflicts != 0 {
		return errors.New("conflicting DAT resources")
	}
	return nil
}
