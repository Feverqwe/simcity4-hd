package sc4

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTextureArchiveReusesInterfacePNGs(t *testing.T) {
	ctx := context.Background()
	cache := t.TempDir()
	stock, err := encodeRGBA(image.NewNRGBA(image.Rect(0, 0, 4, 4)))
	if err != nil {
		t.Fatal(err)
	}
	hd := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	hd.SetNRGBA(7, 8, color.NRGBA{R: 200, A: 255})
	id := resourceID{uiImage, catalogIconGroup, 123}
	sourceFolder := filepath.Join(cache, interfaceAtlasFolder)
	original, _, err := cachedUIAtlas(sourceFolder, id, stock, []int{4}, 4, func([]byte) ([]byte, error) {
		return encodeRGBA(hd)
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
	if err := exportTextureArchive(ctx, cache, archive, textureArchiveProgress{}); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := importTextureArchive(ctx, dest, archive, textureArchiveProgress{}); err != nil {
		t.Fatal(err)
	}
	destFolder := filepath.Join(dest, interfaceAtlasFolder)
	entries, err := os.ReadDir(destFolder)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected only the cached atlas, got %v: %v", entries, err)
	}
	imported, _, err := cachedUIAtlas(destFolder, id, stock, []int{4}, 4, nil)
	if err != nil || !bytes.Equal(original, imported) {
		t.Fatalf("could not reuse the imported atlas without inference: %v", err)
	}
	if err := importTextureArchive(ctx, dest, archive, textureArchiveProgress{}); err != nil {
		t.Fatalf("import should also work when the cache is already populated: %v", err)
	}
}

func TestTextureArchiveRejectsInvalidEntries(t *testing.T) {
	cache := t.TempDir()
	imageData, err := encodeRGBA(image.NewNRGBA(image.Rect(0, 0, 4, 4)))
	if err != nil {
		t.Fatal(err)
	}
	validName := textureArchiveRoot + inferenceCacheFolder + "/" + strings.Repeat("a", 64) + ".png"
	fshName := textureArchiveRoot + filepath.ToSlash(modelCache("", 2)) + "/" + strings.Repeat("a", 64) + ".fsh"
	for name, entries := range map[string][]string{
		"path traversal": {validName, textureArchiveRoot + inferenceCacheFolder + "/../../escape.png"},
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
			if err := importTextureArchive(context.Background(), cache, archive, textureArchiveProgress{}); err == nil {
				t.Fatal("import accepted an invalid archive")
			}
			files, err := os.ReadDir(cache)
			if err != nil || len(files) != 0 {
				t.Fatalf("import wrote files before validating the archive paths: %v %v", files, err)
			}
		})
	}
}
