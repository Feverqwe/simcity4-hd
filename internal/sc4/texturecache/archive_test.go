package texturecache

import (
	"archive/zip"
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

func TestTextureArchiveRejectsInvalidEntries(t *testing.T) {
	cache := t.TempDir()
	imageData, err := textures.EncodeRGBA(image.NewNRGBA(image.Rect(0, 0, 4, 4)))
	if err != nil {
		t.Fatal(err)
	}
	validName := TextureArchiveRoot + upscale.InferenceCacheFolder + "/" + strings.Repeat("a", 64) + ".png"
	fshName := TextureArchiveRoot + filepath.ToSlash(ModelCache("", 2)) + "/" + strings.Repeat("a", 64) + ".fsh"
	for name, entries := range map[string][]string{
		"path traversal": {validName, TextureArchiveRoot + upscale.InferenceCacheFolder + "/../../escape.png"},
		"duplicate":      {validName, validName},
		"PNG as FSH":     {fshName},
	} {
		t.Run(name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "bad.zip")
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			writer := zip.NewWriter(file)
			for _, name := range entries {
				entry, err := writer.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := entry.Write(imageData); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if err := ImportTextureArchive(context.Background(), cache, archive, ArchiveProgress{}); err == nil {
				t.Fatal("import accepted an invalid archive")
			}
			files, err := os.ReadDir(cache)
			if err != nil || len(files) != 0 {
				t.Fatalf("import wrote files before validating the archive paths: %v %v", files, err)
			}
		})
	}
}
