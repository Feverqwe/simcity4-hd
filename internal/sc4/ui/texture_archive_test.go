package ui

import (
	"bytes"
	"context"
	"fmt"
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

func TestPreparedAtlasSkipsResizingOnRepeat(t *testing.T) {
	for _, scale := range []int{2, 4} {
		t.Run(fmt.Sprintf("x%d", scale), func(t *testing.T) {
			ctx := context.Background()
			cache := t.TempDir()
			stock, err := textures.EncodeRGBA(image.NewNRGBA(image.Rect(0, 0, 4, 4)))
			if err != nil {
				t.Fatal(err)
			}
			infer := upscale.CachedInference(ctx, cache, upscale.Config{Scale: 4, Tile: 64, Overlap: 16}, func([]byte) ([]byte, error) {
				return textures.EncodeRGBA(image.NewNRGBA(image.Rect(0, 0, 16, 16)))
			})
			options := atlasOptions{variant: "prepared-output", finish: func(data []byte) ([]byte, error) {
				result, err := textures.DecodeRGBA(data)
				if err != nil {
					return nil, err
				}
				result.SetNRGBA(0, 0, color.NRGBA{G: 250, A: 255})
				return textures.EncodeRGBA(result)
			}}
			id := dbpf.ID{Kind: uiImage, Group: catalogIconGroup, Instance: 123}
			cold, err := preparedUIAtlas(cache, id, stock, []int{4}, scale, options, infer)
			if err != nil {
				t.Fatal(err)
			}
			result, err := textures.DecodeRGBA(cold)
			if err != nil || result.Rect.Size() != image.Pt(scale*4, scale*4) || result.NRGBAAt(0, 0).G != 250 {
				t.Fatalf("prepared artwork has the wrong size or pixels: %v", err)
			}
			options.finish = func([]byte) ([]byte, error) {
				t.Fatal("a repeated build reprocessed the atlas")
				return nil, nil
			}
			warm, err := preparedUIAtlas(cache, id, stock, []int{4}, scale, options, nil)
			if err != nil || !bytes.Equal(cold, warm) {
				t.Fatalf("the warm build did not reuse the finished PNG: %v", err)
			}
			groups, err := texturecache.AvailableExportGroups(ctx, cache)
			if err != nil || len(groups) != 1 || groups[0].ID != fmt.Sprintf("interface-x%d", scale) {
				t.Fatalf("the cache contains unexpected intermediate images: %v (%v)", groups, err)
			}
		})
	}
}

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
	sourceFolder := texturecache.InterfaceCache(cache, 2)
	infer := upscale.CachedInference(ctx, cache, upscale.Config{Scale: 4, Tile: 64, Overlap: 16, KeepUpscaledImages: true}, func([]byte) ([]byte, error) {
		return textures.EncodeRGBA(hd)
	})
	original, err := preparedUIAtlas(cache, id, stock, []int{4}, 2, atlasOptions{}, infer)
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range []string{"texture.fsh", "package.dat", "unrelated.png"} {
		if err := os.WriteFile(filepath.Join(sourceFolder, file), original, 0600); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(t.TempDir(), "textures.zip")
	if err := texturecache.ExportTextureArchive(ctx, cache, archive, nil, texturecache.ArchiveProgress{}); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	groups, err := texturecache.AvailableExportGroups(ctx, cache)
	if err != nil || len(groups) != 2 || groups[0].ID != "interface-x2" || groups[1].ID != "original" {
		t.Fatalf("cache inventory didn’t identify the prepared x2 and original images: %v (%v)", groups, err)
	}
	if err := texturecache.ImportTextureArchive(ctx, dest, archive, texturecache.ArchiveProgress{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, upscale.InferenceCacheFolder)); !os.IsNotExist(err) {
		t.Fatalf("the default export included raw model output: %v", err)
	}
	destFolder := texturecache.InterfaceCache(dest, 2)
	entries, err := os.ReadDir(destFolder)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected only the cached atlas, got %v: %v", entries, err)
	}
	imported, err := preparedUIAtlas(dest, id, stock, []int{4}, 2, atlasOptions{}, nil)
	if err != nil || !bytes.Equal(original, imported) {
		t.Fatalf("could not reuse the imported atlas without inference: %v", err)
	}
	originalOnly := filepath.Join(t.TempDir(), "original.zip")
	if err := texturecache.ExportTextureArchive(ctx, cache, originalOnly, []string{"original"}, texturecache.ArchiveProgress{}); err != nil {
		t.Fatal(err)
	}
	onlyOriginalCache := t.TempDir()
	if err := texturecache.ImportTextureArchive(ctx, onlyOriginalCache, originalOnly, texturecache.ArchiveProgress{}); err != nil {
		t.Fatal(err)
	}
	groups, err = texturecache.AvailableExportGroups(ctx, onlyOriginalCache)
	if err != nil || len(groups) != 1 || groups[0].ID != "original" {
		t.Fatalf("exporting only originals yielded %v (%v)", groups, err)
	}
	if err := texturecache.ImportTextureArchive(ctx, dest, archive, texturecache.ArchiveProgress{}); err != nil {
		t.Fatalf("import should also work when the cache is already populated: %v", err)
	}
}
