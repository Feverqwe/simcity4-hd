package setup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const (
	macPythonPackage = "cpython-3.12.13+20260414-aarch64-apple-darwin-install_only.tar.gz"
	macPythonURL     = "https://github.com/astral-sh/python-build-standalone/releases/download/20260414/cpython-3.12.13%2B20260414-aarch64-apple-darwin-install_only.tar.gz"
	macPythonSHA     = "8966b2bcd9fa03ba22c080ad15a86bc12e41a00122b16f4b3740e302261124d9"
	macPythonEnv     = "python-3.12.13-torch-2.11.0-arm64"
)

func (d *Resources) prepareInference(phase int, modelMissing bool) (string, error) {
	if runtime.GOARCH != "arm64" {
		return "", FieldError("cache", errors.New("Generating missing textures requires the Apple silicon installer. Import a texture archive to build from cache."))
	}
	if modelMissing {
		if err := CheckSpace(d.Dir, 600<<20, "the texture model"); err != nil {
			return "", err
		}
	}
	python, err := d.preparePython(phase)
	if err != nil {
		return "", err
	}
	d.Step(phase, "Checking Metal for texture upscaling")
	const probe = "import torch; assert torch.backends.mps.is_available(), 'Metal is unavailable'; (torch.ones(1, device='mps', dtype=torch.bfloat16) * 2).cpu()"
	if err := d.runPython(python, "", "-c", probe); err != nil {
		return "", fmt.Errorf("Could not start the Metal upscale tools. Use macOS 14 or later on Apple silicon, or import a texture archive: %w", err)
	}
	return python, nil
}

func (d *Resources) preparePython(phase int) (string, error) {
	destination := filepath.Join(d.Dir, macPythonEnv)
	python := filepath.Join(destination, "bin", "python3")
	if info, err := os.Stat(python); err == nil && info.Mode().IsRegular() {
		return python, nil
	}
	if err := CheckSpace(d.Dir, 3<<30, "the Python and Metal upscale tools"); err != nil {
		return "", err
	}
	archive, err := d.Download(phase, "Downloading Python", macPythonPackage, macPythonURL, macPythonSHA, 40<<20)
	if err != nil {
		return "", err
	}
	d.Step(phase, "Preparing Python")
	stage, err := os.MkdirTemp(d.Dir, ".python-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	cmd := exec.CommandContext(d.Context, "/usr/bin/tar", "-xzf", archive, "-C", stage)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	runtimeDir := filepath.Join(stage, "python")
	pipPython := filepath.Join(runtimeDir, "bin", "python3")
	if err := d.installPackages(phase, pipPython, "", "Installing the Metal upscale tools", pypi, append([]string{"torch==2.11.0", "torchvision==0.26.0"}, upscalePackages...)...); err != nil {
		return "", err
	}
	if err := d.runPython(pipPython, "", "-c", "import torch, torchvision, numpy, PIL, safetensors, spandrel"); err != nil {
		return "", fmt.Errorf("verify the Metal upscale tools: %w", err)
	}
	if err := d.Context.Err(); err != nil {
		return "", err
	}
	if err := os.Rename(runtimeDir, destination); err != nil {
		return "", err
	}
	_ = os.Remove(archive)
	return python, nil
}
