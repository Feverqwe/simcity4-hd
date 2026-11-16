package sc4

import (
	"context"
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

type macSetupDependencies struct {
	ctx             context.Context
	dir             string
	emit            func(macSetupEvent)
	waitForRetry    func() error
	python, weights string
}

func (d *macSetupDependencies) inference() (string, string, error) {
	if err := d.ctx.Err(); err != nil {
		return "", "", err
	}
	if d.python != "" {
		return d.python, d.weights, nil
	}
	if runtime.GOARCH != "arm64" {
		return "", "", macSetupInputError{"cache", errors.New("Generating missing textures requires the Apple silicon installer. Import an Aspyr texture archive to build from cache.")}
	}
	if err := os.MkdirAll(d.dir, 0755); err != nil {
		return "", "", err
	}
	python, err := d.preparePython()
	if err != nil {
		return "", "", err
	}
	d.emit(macSetupEvent{Type: "step", Text: "Checking Metal for texture upscaling"})
	const probe = "import torch; assert torch.backends.mps.is_available(), 'Metal is unavailable'; (torch.ones(1, device='mps', dtype=torch.bfloat16) * 2).cpu()"
	if err := d.runPython(python, "-c", probe); err != nil {
		return "", "", fmt.Errorf("Could not start the Metal upscale tools. Use macOS 14 or later on Apple silicon, or import an Aspyr texture archive: %w", err)
	}
	weights := filepath.Join(d.dir, modelName)
	if checkSHA(d.ctx, weights, hatCheckpointSHA) != nil {
		if err := checkMacFreeSpace(d.dir, 600<<20, "the texture model"); err != nil {
			return "", "", err
		}
		if err := removeIfPresent(weights); err != nil {
			return "", "", err
		}
		archive, err := d.fetch("Downloading the upscale model", "model-v3detail.zip", modelURL, modelSHA, 350<<20)
		if err != nil {
			return "", "", err
		}
		d.emit(macSetupEvent{Type: "step", Text: "Verifying the upscale model"})
		if err := extractOne(d.ctx, archive, modelName, weights, 300<<20, hatCheckpointSHA); err != nil {
			return "", "", err
		}
		_ = os.Remove(archive)
	}
	d.python, d.weights = python, weights
	return python, weights, nil
}

func (d *macSetupDependencies) preparePython() (string, error) {
	destination := filepath.Join(d.dir, macPythonEnv)
	python := filepath.Join(destination, "bin", "python3")
	if info, err := os.Stat(python); err == nil && info.Mode().IsRegular() {
		return python, nil
	}
	if err := checkMacFreeSpace(d.dir, 3<<30, "the Python and Metal upscale tools"); err != nil {
		return "", err
	}
	archive, err := d.fetch("Downloading Python", macPythonPackage, macPythonURL, macPythonSHA, 40<<20)
	if err != nil {
		return "", err
	}
	d.emit(macSetupEvent{Type: "step", Text: "Preparing Python"})
	stage, err := os.MkdirTemp(d.dir, ".python-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if err := runMacSetupTool(d.ctx, "/usr/bin/tar", "-xzf", archive, "-C", stage); err != nil {
		return "", err
	}
	runtimeDir := filepath.Join(stage, "python")
	pipPython := filepath.Join(runtimeDir, "bin", "python3")
	if err := d.retry("Installing the Metal upscale tools", func() error {
		return d.runPython(pipPython, "-m", "pip", "--isolated", "--disable-pip-version-check",
			"install", "--no-input", "--only-binary=:all:", "--cache-dir", filepath.Join(d.dir, "pip-cache"),
			"--index-url", "https://pypi.org/simple",
			"torch==2.11.0", "torchvision==0.26.0", "numpy==2.2.6", "Pillow==11.3.0", "safetensors==0.5.3", "spandrel==0.4.2")
	}); err != nil {
		return "", err
	}
	if err := d.runPython(pipPython, "-c", "import torch, torchvision, numpy, PIL, safetensors, spandrel"); err != nil {
		return "", fmt.Errorf("verify the Metal upscale tools: %w", err)
	}
	if err := d.ctx.Err(); err != nil {
		return "", err
	}
	if err := os.Rename(runtimeDir, destination); err != nil {
		return "", err
	}
	_ = os.Remove(archive)
	return python, nil
}

func (d *macSetupDependencies) runPython(python string, args ...string) error {
	cmd := exec.CommandContext(d.ctx, python, append([]string{"-I", "-u"}, args...)...)
	cmd.Dir = filepath.Dir(python)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

func (d *macSetupDependencies) fetch(label, name, url, hash string, limit int64) (string, error) {
	output := filepath.Join(d.dir, name)
	if checkSHA(d.ctx, output, hash) == nil {
		return output, nil
	}
	if err := d.ctx.Err(); err != nil {
		return "", err
	}
	if err := removeIfPresent(output); err != nil {
		return "", err
	}
	err := d.retry(label, func() error {
		return downloadSetupFile(d.ctx, output, url, hash, limit, func(value int) {
			d.emit(macSetupEvent{Type: "progress", Value: &value})
		})
	})
	return output, err
}

func (d *macSetupDependencies) retry(label string, attempt func() error) error {
	for {
		if err := d.ctx.Err(); err != nil {
			return err
		}
		d.emit(macSetupEvent{Type: "step", Text: label})
		err := attempt()
		if err == nil {
			return nil
		}
		if err := d.ctx.Err(); err != nil {
			return err
		}
		err = fmt.Errorf("%s failed: %w", label, err)
		if d.waitForRetry == nil {
			return err
		}
		d.emit(macSetupEvent{Type: "retry-resource", Text: err.Error()})
		if err := d.waitForRetry(); err != nil {
			return err
		}
	}
}
