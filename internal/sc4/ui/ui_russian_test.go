package ui

import (
	"bytes"
	"encoding/binary"
	"io"
	"path/filepath"
	"testing"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
)

func TestRussianLocaleImportsGameText(t *testing.T) {
	city := dbpf.Entry{ID: dbpf.ID{Kind: 0x2026960b, Group: 1, Instance: 1}, Data: encodeLocaleText("Город")}
	ascii := dbpf.Entry{ID: dbpf.ID{Kind: 0x2026960b, Group: 1, Instance: 2}, Data: append(binary.LittleEndian.AppendUint32(nil, 3), "Map"...)}
	example := dbpf.Entry{
		ID:   dbpf.ID{Kind: 0x2026960b, Group: 0x55555555, Instance: 0xaa41380e},
		Data: []byte("This is a sample string that has both an instance (aa41380e) and group (55555555) in its name.\r\n"),
	}
	validPath := filepath.Join(t.TempDir(), "locale.dat")
	if err := gamefiles.PublishFile(validPath, func(w io.Writer) error {
		return dbpf.Write(w, []dbpf.Entry{example, city, ascii})
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := russianUI(validPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].ID != city.ID || entries[1].ID != ascii.ID ||
		!bytes.Equal(entries[0].Data, city.Data) || !bytes.Equal(entries[1].Data, ascii.Data) {
		t.Fatalf("import did not preserve supported text while omitting the authoring example: %v", entries)
	}

	truncated := dbpf.Entry{ID: dbpf.ID{Kind: 0x2026960b, Group: 1, Instance: 3}, Data: binary.LittleEndian.AppendUint32(nil, 0x10000200)}
	brokenPath := filepath.Join(t.TempDir(), "broken.dat")
	if err := gamefiles.PublishFile(brokenPath, func(w io.Writer) error {
		return dbpf.Write(w, []dbpf.Entry{city, truncated})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := russianUI(brokenPath); err == nil {
		t.Fatal("import accepted text extending past the resource")
	}
}
