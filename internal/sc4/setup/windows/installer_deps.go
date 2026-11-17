//go:build windows || darwin

package windows

import (
	"path/filepath"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
)

const (
	rendererURL = "https://github.com/caspervg/scd3d11/releases/download/v0.5.0/SCD3D11-v0.5.0.zip"
	rendererSHA = "4b2718394c78e79ac5f3161e6438399db3786778c47aa437189df5564cf53623"
)

func setupRenderer(d *setup.Resources, phase int) ([]string, error) {
	archive, err := d.Download(phase, "Downloading the Direct3D 11 renderer", "SCD3D11-v0.5.0.zip", rendererURL, rendererSHA, 64<<20)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, name := range []string{rendererDLL, rendererINI} {
		output := filepath.Join(d.Dir, name)
		if err := setup.RemoveIfPresent(output); err != nil {
			return nil, err
		}
		if err := setup.ExtractOne(d.Context, archive, name, output, 32<<20, ""); err != nil {
			return nil, err
		}
		files = append(files, output)
	}
	return files, nil
}

func setupGraphics(d *setup.Resources, phase int) (string, error) {
	const url = "https://github.com/0xC0000054/sc4-graphics-options/releases/download/v1.4.1/SC4GraphicsOptions.zip"
	const hash = "4e3fce35d0fc03da63839a83a7b11160043195c7c0d3c9410a02f4f503da2e6c"
	archive, err := d.Download(phase, "Downloading the Steam launch graphics plugin", "SC4GraphicsOptions-v1.4.1.zip", url, hash, 1<<20)
	if err != nil {
		return "", err
	}
	output := filepath.Join(d.Dir, graphicsDLL)
	if err := setup.RemoveIfPresent(output); err != nil {
		return "", err
	}
	return output, setup.ExtractOne(d.Context, archive, graphicsDLL, output, 1<<20, "")
}
