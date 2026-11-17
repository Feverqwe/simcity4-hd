package setup

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
)

const (
	pythonFolder      = "python-3.12.10"
	pythonReadyMarker = "cuda-12.8-v1.ready"
	pythonURL         = "https://api.nuget.org/v3-flatcontainer/python/3.12.10/python.3.12.10.nupkg"
	pythonSHA         = "0eb85c2dfccccf1b17352de4c397f69194035b7d37149eacc16f1147d93de3b8"
)

func (d *Resources) prepareInference(phase int, modelMissing bool) (string, error) {
	var required uint64
	if modelMissing {
		required += 512 << 20
	}
	pythonDir := filepath.Join(d.Dir, pythonFolder)
	if !IsRegularFile(filepath.Join(pythonDir, "python.exe")) ||
		!IsRegularFile(filepath.Join(pythonDir, pythonReadyMarker)) {
		required += 13 << 30
	}
	if err := d.Context.Err(); err != nil {
		return "", err
	}
	if err := CheckSpace(d.Dir, required, "downloads and upscaling tools"); err != nil {
		return "", err
	}
	python, err := d.preparePython(phase)
	if err != nil {
		return "", err
	}
	d.Step(phase, "Initializing CUDA for texture upscaling")
	const check = "import torch; import numpy, PIL, safetensors, spandrel; torch.cuda.init()"
	if err := d.runPython(python, "", "-c", check); err != nil {
		if d.Context.Err() != nil {
			return "", d.Context.Err()
		}
		return "", fmt.Errorf("Could not start the CUDA upscale tools. See the installation log for details. (%w)", err)
	}
	return python, nil
}

func (d *Resources) preparePython(phase int) (string, error) {
	dir := filepath.Join(d.Dir, pythonFolder)
	python := filepath.Join(dir, "python.exe")
	if !IsRegularFile(python) {
		archive, err := d.Download(phase, "Downloading Python", "python-3.12.10.zip", pythonURL, pythonSHA, 32<<20)
		if err != nil {
			return "", err
		}
		d.Step(phase, "Preparing Python")
		if err := extractPython(d.Context, archive, dir); err != nil {
			return "", err
		}
	}
	ready := filepath.Join(dir, pythonReadyMarker)
	if !IsRegularFile(ready) {
		temp, err := os.MkdirTemp(d.Dir, ".pip-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(temp)
		if err := d.installPackages(phase, python, temp, "Installing the NVIDIA upscale tools (several GB)",
			"https://download.pytorch.org/whl/cu128", "torch==2.11.0+cu128", "torchvision==0.26.0+cu128"); err != nil {
			return "", err
		}
		if err := d.installPackages(phase, python, temp, "Installing the Python upscale dependencies", pypi, upscalePackages...); err != nil {
			return "", err
		}
		if err := os.WriteFile(ready, nil, 0600); err != nil {
			return "", err
		}
	}
	return python, nil
}

func extractPython(ctx context.Context, archive, destination string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".python-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for _, entry := range z.File {
		relative, ok := strings.CutPrefix(entry.Name, "tools/")
		if !ok || !entry.Mode().IsRegular() {
			continue
		}
		local, err := filepath.Localize(relative)
		if err != nil {
			return err
		}
		err = gamefiles.PublishFile(filepath.Join(stage, local), func(w io.Writer) error {
			r, err := entry.Open()
			if err != nil {
				return err
			}
			defer r.Close()
			_, err = io.Copy(w, gamefiles.WithContext(ctx, r))
			return err
		})
		if err != nil {
			return err
		}
	}
	if !IsRegularFile(filepath.Join(stage, "python.exe")) {
		return errors.New("Python is missing from the downloaded package")
	}
	return os.Rename(stage, destination)
}
