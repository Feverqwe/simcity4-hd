//go:build windows || darwin

package setup

import (
	"os"
	"os/exec"
	"path/filepath"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

const pypi = "https://pypi.org/simple"

var upscalePackages = []string{"numpy==2.2.6", "Pillow==11.3.0", "safetensors==0.5.3", "spandrel==0.4.2"}

func (d *Resources) Inference(phase int) (string, string, error) {
	if err := d.Context.Err(); err != nil {
		return "", "", err
	}
	if d.python != "" {
		return d.python, d.weights, nil
	}
	if err := os.MkdirAll(d.Dir, 0755); err != nil {
		return "", "", err
	}
	weights := filepath.Join(d.Dir, ModelName)
	modelMissing := CheckSHA(d.Context, weights, upscale.HatCheckpointSHA) != nil
	if err := d.Context.Err(); err != nil {
		return "", "", err
	}
	python, err := d.prepareInference(phase, modelMissing)
	if err != nil {
		return "", "", err
	}
	if modelMissing {
		if err := d.model(phase, weights); err != nil {
			return "", "", err
		}
	}
	d.python, d.weights = python, weights
	return python, weights, nil
}

func (d *Resources) installPackages(phase int, python, temp, label, index string, packages ...string) error {
	return d.RetryResource(phase, label, func() error {
		args := []string{"-m", "pip", "--isolated", "--disable-pip-version-check", "install", "--no-input",
			"--only-binary=:all:", "--cache-dir", filepath.Join(d.Dir, "pip-cache"), "--index-url", index}
		return d.runPython(python, temp, append(args, packages...)...)
	})
}

func (d *Resources) runPython(python, temp string, args ...string) error {
	cmd := exec.CommandContext(d.Context, python, append([]string{"-I", "-u"}, args...)...)
	if temp != "" {
		cmd.Env = append(os.Environ(), "TEMP="+temp, "TMP="+temp)
	}
	gamefiles.ConfigureBackgroundProcess(cmd)
	cmd.Dir = filepath.Dir(python)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	err := cmd.Run()
	if d.Context.Err() != nil {
		return d.Context.Err()
	}
	return err
}
