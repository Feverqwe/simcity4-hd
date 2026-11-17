//go:build windows || darwin

package macos

import (
	"context"
	"errors"
	"fmt"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/macho"
)

func checkManagedMacBundle(ctx context.Context, bundle string) (macSetupRecord, error) {
	receipt, err := readMacSetupReceipt(bundle)
	if err != nil {
		return receipt, fmt.Errorf("setup could not identify the installed mod: %w", err)
	}
	// Steam can restore a signed executable. Do not modify a stock or
	// updated bundle that merely has the setup receipt left in Plugins.
	hash, err := macSetupExecutableHash(bundle)
	if err != nil {
		return receipt, err
	}
	if hash != receipt.ExecutableSHA256 || hash == macho.StockHash {
		return receipt, errors.New("Steam has replaced the patched executable. The Steam files and original backup were left in place.")
	}
	if receipt.NeedsSigning {
		if receipt.Build == nil {
			return receipt, errors.New("The Windows installation is missing its build settings.")
		}
		// Configure rebuilds the executable from the verified stock backup.
		return receipt, ctx.Err()
	}
	err = verifyMacSetupSignature(ctx, bundle)
	if ctx.Err() != nil {
		return receipt, ctx.Err()
	}
	if err != nil {
		return receipt, fmt.Errorf("The patched game has changed since setup installed it. The Steam files and original backup were left in place: %w", err)
	}
	return receipt, nil
}
