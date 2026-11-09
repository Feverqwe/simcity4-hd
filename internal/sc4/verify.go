package sc4

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

type resourceLocation struct {
	archive *archive
	record  record
}

func verifyPackages(paths []string, output io.Writer) error {
	if len(paths) == 0 {
		return errors.New("verify requires at least one DAT path")
	}
	owners := make(map[resourceID]resourceLocation)
	overlaps, conflicts := 0, 0
	for _, path := range paths {
		a, err := openArchive(path)
		if err != nil {
			return err
		}
		defer a.file.Close()
		fmt.Fprintf(output, "%s: %d resources\n", path, len(a.records))
		for _, r := range a.records {
			previous, exists := owners[r.id]
			if !exists {
				owners[r.id] = resourceLocation{a, r}
				continue
			}
			first, err := previous.archive.read(previous.record)
			if err != nil {
				return fmt.Errorf("%s %s: %w", previous.archive.file.Name(), r.id, err)
			}
			second, err := a.read(r)
			if err != nil {
				return fmt.Errorf("%s %s: %w", path, r.id, err)
			}
			overlaps++
			kind := "identical"
			if !bytes.Equal(first, second) {
				kind = "conflicting"
				conflicts++
			}
			fmt.Fprintf(output, "%s: %s in %s and %s\n", r.id, kind, previous.archive.file.Name(), path)
		}
	}
	fmt.Fprintf(output, "%d distinct resources, %d overlaps, %d conflicts\n", len(owners), overlaps, conflicts)
	if conflicts != 0 {
		return errors.New("conflicting DAT resources")
	}
	return nil
}
