package macos

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
)

const canSignMacApp = false

func ensureMacSetupGameClosed() error { return nil }

func lockMacSetup(bundle string) (func(), error) { return setup.LockFolder(bundle) }

func macSetupSigner() func(context.Context, string) error { return nil }

func installedMacResolution() string { return "" }

func publishMacResolution(_ int, publish func() error) error { return publish() }

func verifyMacSetupSignature(context.Context, string) error {
	return errors.New("Use SimCity 4 UHD Setup on your Mac to verify the signed Aspyr app.")
}

func finalizeMacSetupBundle(_ context.Context, bundle string) error {
	receipt, err := readMacSetupReceipt(bundle)
	if err != nil {
		return err
	}
	hash, err := macSetupExecutableHash(bundle)
	if err != nil {
		return err
	}
	if hash != receipt.ExecutableSHA256 {
		return errors.New("The applied executable differs from the staged executable.")
	}
	return checkStockMacSetup(filepath.Join(macSetupBackup(bundle), macSetupOriginalFiles))
}

func copyMacSetupPaths(ctx context.Context, source, destination string, paths []string) error {
	for _, rel := range paths {
		from := filepath.Join(source, rel)
		info, err := os.Lstat(from)
		if err != nil {
			return err
		}
		if (rel == macSetupSignature && !info.IsDir()) || (rel != macSetupSignature && !info.Mode().IsRegular()) {
			return fmt.Errorf("Unexpected file type: %s", from)
		}
		err = filepath.WalkDir(from, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			relative, err := filepath.Rel(source, name)
			if err != nil {
				return err
			}
			target := filepath.Join(destination, relative)
			if entry.IsDir() {
				return os.MkdirAll(target, 0755)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			return gamefiles.CopyRegular(name, target)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
