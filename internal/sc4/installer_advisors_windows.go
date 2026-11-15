package sc4

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const installedAdvisorsState = "Advisors.sha256"

func advisorsRecipeFingerprint() string {
	sum := sha256.Sum256(advisorsRecipeData)
	return fmt.Sprintf("%x", sum[:])
}

func advisorFileFingerprint(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, contextReader{ctx, file}); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func setupAdvisorsCurrent(ctx context.Context, active string) bool {
	data, err := os.ReadFile(filepath.Join(active, installedAdvisorsState))
	if err != nil {
		return false
	}
	source, installed, ok := strings.Cut(strings.TrimSpace(string(data)), "\n")
	return ok && source == advisorsRecipeFingerprint() && checkSHA(ctx, filepath.Join(active, advisorsPackage), installed) == nil
}

func stageSetupAdvisors(ctx context.Context, game, folder string) error {
	// The full game plugin is generated on the destination volume.
	if err := checkVolumeSpace([]spaceRequest{{game, "the advisor models", 130 * mib}}, lookupSetupVolume); err != nil {
		return err
	}
	destination := filepath.Join(folder, advisorsPackage)
	if err := buildAdvisorsPackage(ctx, game, destination); err != nil {
		return fmt.Errorf("prepare advisor models: %w", err)
	}
	installed, err := advisorFileFingerprint(ctx, destination)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(folder, installedAdvisorsState), []byte(advisorsRecipeFingerprint()+"\n"+installed+"\n"), 0644)
}
