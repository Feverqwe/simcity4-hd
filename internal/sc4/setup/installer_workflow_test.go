//go:build windows || darwin

package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallationPauseAtPublication(t *testing.T) {
	for _, stopBefore := range []bool{true, false} {
		name := "during publication"
		if stopBefore {
			name = "before publication"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dir := t.TempDir()
			game, staged := filepath.Join(dir, "game"), filepath.Join(dir, "staged")
			if err := os.WriteFile(game, []byte("original"), 0644); err != nil {
				t.Fatal(err)
			}
			plan := InstallationPlan{
				Stage: func(TextureOutputs, *Resources) (func(context.Context) error, error) {
					if err := os.WriteFile(staged, []byte("patched"), 0644); err != nil {
						return nil, err
					}
					if stopBefore {
						cancel()
					}
					return func(applyCtx context.Context) error {
						cancel()
						if err := applyCtx.Err(); err != nil {
							return err
						}
						return os.Rename(staged, game)
					}, nil
				},
			}
			want := "patched"
			err := plan.Run(ctx, func(Event) {}, nil)
			if stopBefore {
				want = "original"
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("expected a pause before publication, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("an in-progress publication should finish, got %v", err)
			}
			data, err := os.ReadFile(game)
			if err != nil || string(data) != want {
				t.Fatalf("game contains %q, want %q (%v)", data, want, err)
			}
		})
	}
}

func TestInstallCacheRejectsGameAndBackupAliases(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Game")
	if err := os.Mkdir(game, 0755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "game")
	if _, err := os.Stat(alias); errors.Is(err, os.ErrNotExist) {
		t.Skip("requires a case-insensitive test directory")
	} else if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(root, "Game-Backup")
	for _, cache := range []string{filepath.Join(alias, "Cache"), filepath.Join(root, "game-backup", "Cache"), root} {
		if _, err := ResolveCache(cache, "", game, backup); err == nil {
			t.Fatalf("accepted an overlapping cache: %s", cache)
		}
	}
	safe := filepath.Join(root, "TextureCache")
	if _, err := ResolveCache(safe, "", game, backup); err != nil {
		t.Fatalf("rejected an external cache: %v", err)
	}
	cache := filepath.Join(alias, "PreviouslySavedCache")
	if err := os.Mkdir(cache, 0755); err != nil {
		t.Fatal(err)
	}
	if err := ProtectInstalledCache(cache, game); err == nil {
		t.Fatal("an existing cache inside a folder being replaced was not protected")
	}
	if err := os.Rename(cache, safe); err != nil {
		t.Fatal(err)
	}
	if err := ProtectInstalledCache(cache, game); err != nil {
		t.Fatalf("operation still blocked after moving the cache: %v", err)
	}
}
