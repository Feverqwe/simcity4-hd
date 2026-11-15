package sc4

import (
	"bytes"
	"io"
	"path/filepath"
	"testing"
)

func TestRussianLocaleImportsGameText(t *testing.T) {
	city := resource{resourceID{0x2026960b, 1, 1}, encodeLocaleText("Город")}
	ascii := resource{resourceID{0x2026960b, 1, 2}, append(little.AppendUint32(nil, 3), "Map"...)}
	example := resource{
		resourceID{0x2026960b, 0x55555555, 0xaa41380e},
		[]byte("This is a sample string that has both an instance (aa41380e) and group (55555555) in its name.\r\n"),
	}
	validPath := filepath.Join(t.TempDir(), "locale.dat")
	if err := publishFile(validPath, func(w io.Writer) error {
		return writeArchive(w, []resource{example, city, ascii})
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := russianUI(validPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].id != city.id || entries[1].id != ascii.id ||
		!bytes.Equal(entries[0].data, city.data) || !bytes.Equal(entries[1].data, ascii.data) {
		t.Fatalf("import did not preserve supported text while omitting the authoring example: %v", entries)
	}

	truncated := resource{resourceID{0x2026960b, 1, 3}, little.AppendUint32(nil, 0x10000200)}
	brokenPath := filepath.Join(t.TempDir(), "broken.dat")
	if err := publishFile(brokenPath, func(w io.Writer) error {
		return writeArchive(w, []resource{city, truncated})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := russianUI(brokenPath); err == nil {
		t.Fatal("import accepted text extending past the resource")
	}
}
