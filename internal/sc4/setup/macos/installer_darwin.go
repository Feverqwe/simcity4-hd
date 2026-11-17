package macos

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

const canSignMacApp = true

func macSetupSigner() func(context.Context, string) error { return signMacExecutable }

func signMacExecutable(ctx context.Context, executable string) error {
	return runMacSetupTool(ctx, "/usr/bin/codesign", "--force", "--sign", "-", executable)
}

func finalizeMacSetupBundle(ctx context.Context, bundle string) error {
	steamLibrary := filepath.Join(bundle, macSetupSteamLibrary)
	if err := signMacExecutable(ctx, steamLibrary); err != nil {
		return err
	}
	if err := runMacSetupTool(ctx, "/usr/bin/codesign", "--force", "--sign", "-", bundle); err != nil {
		return err
	}
	return verifyMacSetupSignature(ctx, bundle)
}

func verifyMacSetupSignature(ctx context.Context, bundle string) error {
	return runMacSetupTool(ctx, "/usr/bin/codesign", "--verify", "--strict", "--deep", bundle)
}

func runMacSetupTool(ctx context.Context, tool string, args ...string) error {
	if output, err := exec.CommandContext(ctx, tool, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w\n%s", filepath.Base(tool), err, output)
	}
	return nil
}
