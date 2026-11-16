package ui

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/texturecache"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

func TestTextureArchiveReusesInterfacePNGs(t *testing.T) {
	ctx := context.Background()
	cache := t.TempDir()
	stock, err := textures.EncodeRGBA(image.NewNRGBA(image.Rect(0, 0, 4, 4)))
	if err != nil {
		t.Fatal(err)
	}
	hd := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	hd.SetNRGBA(7, 8, color.NRGBA{R: 200, A: 255})
	id := dbpf.ID{Kind: uiImage, Group: catalogIconGroup, Instance: 123}
	sourceFolder := filepath.Join(cache, upscale.InterfaceAtlasFolder)
	original, _, err := cachedUIAtlas(sourceFolder, id, stock, []int{4}, 4, func([]byte) ([]byte, error) {
		return textures.EncodeRGBA(hd)
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range []string{"texture.fsh", "package.dat", "unrelated.png"} {
		if err := os.WriteFile(filepath.Join(sourceFolder, file), original, 0600); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(t.TempDir(), "textures.zip")
	if err := texturecache.ExportTextureArchive(ctx, cache, archive, texturecache.ArchiveProgress{}); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := texturecache.ImportTextureArchive(ctx, dest, archive, texturecache.ArchiveProgress{}); err != nil {
		t.Fatal(err)
	}
	destFolder := filepath.Join(dest, upscale.InterfaceAtlasFolder)
	entries, err := os.ReadDir(destFolder)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected only the cached atlas, got %v: %v", entries, err)
	}
	imported, _, err := cachedUIAtlas(destFolder, id, stock, []int{4}, 4, nil)
	if err != nil || !bytes.Equal(original, imported) {
		t.Fatalf("could not reuse the imported atlas without inference: %v", err)
	}
	if err := texturecache.ImportTextureArchive(ctx, dest, archive, texturecache.ArchiveProgress{}); err != nil {
		t.Fatalf("import should also work when the cache is already populated: %v", err)
	}
}
