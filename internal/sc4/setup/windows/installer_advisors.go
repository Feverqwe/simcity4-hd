//go:build windows || darwin

package windows

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/advisors"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
)

const installedAdvisorsState = "Advisors.sha256"

func advisorsRecipeFingerprint() string {
	sum := sha256.Sum256(advisors.AdvisorsRecipeData)
	return fmt.Sprintf("%x", sum[:])
}

func advisorFileFingerprint(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, gamefiles.WithContext(ctx, file)); err != nil {
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
	return ok && source == advisorsRecipeFingerprint() && setup.CheckSHA(ctx, filepath.Join(active, advisors.AdvisorsPackage), installed) == nil
}

func stageSetupAdvisors(ctx context.Context, game, folder string) error {
	// The full game plugin is generated on the destination volume.
	if err := setup.CheckSpace(game, 130*mib, "the advisor models"); err != nil {
		return err
	}
	destination := filepath.Join(folder, advisors.AdvisorsPackage)
	if err := advisors.BuildPackage(ctx, game, destination); err != nil {
		return fmt.Errorf("prepare advisor models: %w", err)
	}
	installed, err := advisorFileFingerprint(ctx, destination)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(folder, installedAdvisorsState), []byte(advisorsRecipeFingerprint()+"\n"+installed+"\n"), 0644)
}
