package sc4

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMacSetupRejectsTruncatedExecutable(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "original")
	if err := os.WriteFile(original, []byte("truncated"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, macGameExecutable), []byte("longer prepared build"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := macSetupExecutable(original, root, []string{"city-hud-x2.dat"}, 0, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "expected the original Steam Aspyr") {
		t.Fatalf("expected a diagnostic for the original executable, got %v", err)
	}
}

func TestMacSetupRestoresFilesAfterSigningFailure(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "Game.app")
	work := filepath.Join(root, "work")
	staged := filepath.Join(work, "files")
	executable := gameExecutablePath("", true)
	signature := filepath.Join(macSetupSignature, "CodeResources")
	receipt := macSetupReceiptPath("")
	write := func(base, path, data string) {
		t.Helper()
		target := filepath.Join(base, path)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(path, want string) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("%s: got %q, want %q (%v)", path, data, want, err)
		}
	}
	write(bundle, executable, "original executable")
	write(bundle, signature, "original signature")
	write(bundle, "Contents/GameData/Plugins/untouched.dat", "untouched")
	write(staged, executable, "patched executable")
	write(staged, receipt, "mod receipt")
	stagedBackup := filepath.Join(work, "original")
	write(stagedBackup, executable, "original executable")
	signingFailure := errors.New("signing failed")

	err := applyMacSetupFiles(bundle, work, staged, stagedBackup, true, func() error {
		read(filepath.Join(bundle, executable), "patched executable")
		read(filepath.Join(bundle, receipt), "mod receipt")
		read(filepath.Join(macSetupBackup(bundle), executable), "original executable")
		write(bundle, signature, "partially written signature")
		return signingFailure
	})
	if !errors.Is(err, signingFailure) {
		t.Fatalf("got %v, want signing failure", err)
	}
	read(filepath.Join(bundle, executable), "original executable")
	read(filepath.Join(bundle, signature), "original signature")
	read(filepath.Join(bundle, "Contents/GameData/Plugins/untouched.dat"), "untouched")
	for _, path := range []string{filepath.Join(bundle, receipt), macSetupBackup(bundle), macSetupTransactionDir(bundle)} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %s to be absent after rollback, got %v", path, err)
		}
	}
}
