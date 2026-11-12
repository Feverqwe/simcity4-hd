package sc4

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	pythonFolder      = "python-3.12.10"
	pythonReadyMarker = "cuda-12.8-v1.ready"
	pythonURL         = "https://api.nuget.org/v3-flatcontainer/python/3.12.10/python.3.12.10.nupkg"
	pythonSHA         = "0eb85c2dfccccf1b17352de4c397f69194035b7d37149eacc16f1147d93de3b8"
	modelURL          = "https://github.com/the-database/MangaJaNai/releases/download/3.0.0/IllustrationJaNai_V3detail.zip"
	modelSHA          = "27749962bc934779f867730cdb10617e3d7475313f920a7a966c56b75dbdbeda"
	modelName         = "4x_IllustrationJaNai_V3detail_HAT_L_28k_bf16.safetensors"
	rendererURL       = "https://github.com/caspervg/scd3d11/releases/download/v0.5.0/SCD3D11-v0.5.0.zip"
	rendererSHA       = "4b2718394c78e79ac5f3161e6438399db3786778c47aa437189df5564cf53623"
)

type setupDependencies struct {
	ctx             context.Context
	dir             string
	emit            func(installEvent)
	waitForRetry    func() error
	python, weights string
}

func (d *setupDependencies) step(phase int, text string) {
	d.emit(installEvent{Type: "step", Phase: phase, Text: text})
}

func (d *setupDependencies) inference(phase int) (string, string, error) {
	if err := d.ctx.Err(); err != nil {
		return "", "", err
	}
	if d.python != "" {
		return d.python, d.weights, nil
	}
	weights := filepath.Join(d.dir, modelName)
	weightsReady := checkSHA(d.ctx, weights, hatCheckpointSHA) == nil
	var required uint64
	if !weightsReady {
		required += 512 * mib
	}
	pythonDir := filepath.Join(d.dir, pythonFolder)
	if !fileExists(filepath.Join(pythonDir, "python.exe")) ||
		!fileExists(filepath.Join(pythonDir, pythonReadyMarker)) {
		required += 13 * gib
	}
	if err := d.ctx.Err(); err != nil {
		return "", "", err
	}
	if err := checkVolumeSpace([]spaceRequest{{d.dir, "downloads and upscaling tools", required}}, lookupSetupVolume); err != nil {
		return "", "", err
	}
	python, err := d.preparePython(phase)
	if err != nil {
		return "", "", err
	}
	d.step(phase, "Initializing CUDA for texture upscaling")
	const check = "import torch; import numpy, PIL, safetensors, spandrel; torch.cuda.init()"
	if err := d.pythonCommand(python, "-c", check); err != nil {
		if d.ctx.Err() != nil {
			return "", "", d.ctx.Err()
		}
		return "", "", fmt.Errorf("Could not start the CUDA upscale tools. See the installation log for details. (%w)", err)
	}
	if !weightsReady {
		if err := removeIfPresent(weights); err != nil {
			return "", "", err
		}
		archive, err := d.download(phase, "Downloading the upscale model", "model-v3detail.zip", modelURL, modelSHA, 350<<20)
		if err != nil {
			return "", "", err
		}
		d.step(phase, "Verifying the upscale model")
		if err := extractOne(d.ctx, archive, modelName, weights, 300<<20, hatCheckpointSHA); err != nil {
			return "", "", err
		}
		_ = os.Remove(archive)
	}
	d.python, d.weights = python, weights
	return python, weights, nil
}

func (d *setupDependencies) preparePython(phase int) (string, error) {
	dir := filepath.Join(d.dir, pythonFolder)
	python := filepath.Join(dir, "python.exe")
	if !fileExists(python) {
		archive, err := d.download(phase, "Downloading Python", "python-3.12.10.zip", pythonURL, pythonSHA, 32<<20)
		if err != nil {
			return "", err
		}
		d.step(phase, "Preparing Python")
		if err := extractPython(d.ctx, archive, dir); err != nil {
			return "", err
		}
	}
	ready := filepath.Join(dir, pythonReadyMarker)
	if !fileExists(ready) {
		temp, err := os.MkdirTemp(d.dir, ".pip-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(temp)
		pip := func(index string, packages ...string) error {
			args := []string{"-m", "pip", "--isolated", "--disable-pip-version-check", "install", "--no-input",
				"--only-binary=:all:", "--cache-dir", filepath.Join(d.dir, "pip-cache"), "--index-url", index}
			return d.runPython(python, temp, append(args, packages...)...)
		}
		if err := d.retryResource(phase, "Installing the NVIDIA upscale tools (several GB)", func() error {
			return pip("https://download.pytorch.org/whl/cu128", "torch==2.11.0+cu128", "torchvision==0.26.0+cu128")
		}); err != nil {
			return "", err
		}
		if err := d.retryResource(phase, "Installing the Python upscale dependencies", func() error {
			return pip("https://pypi.org/simple", "numpy==2.2.6", "Pillow==11.3.0", "safetensors==0.5.3", "spandrel==0.4.2")
		}); err != nil {
			return "", err
		}
		if err := os.WriteFile(ready, nil, 0600); err != nil {
			return "", err
		}
	}
	return python, nil
}

func (d *setupDependencies) pythonCommand(python string, args ...string) error {
	return d.runPython(python, "", args...)
}

func (d *setupDependencies) runPython(python, temp string, args ...string) error {
	cmd := exec.CommandContext(d.ctx, python, append([]string{"-I", "-u"}, args...)...)
	if temp != "" {
		cmd.Env = append(os.Environ(), "TEMP="+temp, "TMP="+temp)
	}
	configureBackgroundProcess(cmd)
	cmd.Dir = filepath.Dir(python)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

func (d *setupDependencies) renderer(phase int) ([]string, error) {
	archive, err := d.download(phase, "Downloading the Direct3D 11 renderer", "SCD3D11-v0.5.0.zip", rendererURL, rendererSHA, 64<<20)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, name := range []string{rendererDLL, rendererINI} {
		output := filepath.Join(d.dir, name)
		if err := removeIfPresent(output); err != nil {
			return nil, err
		}
		if err := extractOne(d.ctx, archive, name, output, 32<<20, ""); err != nil {
			return nil, err
		}
		files = append(files, output)
	}
	return files, nil
}

func (d *setupDependencies) graphics(phase int) (string, error) {
	const url = "https://github.com/0xC0000054/sc4-graphics-options/releases/download/v1.4.1/SC4GraphicsOptions.zip"
	const hash = "4e3fce35d0fc03da63839a83a7b11160043195c7c0d3c9410a02f4f503da2e6c"
	archive, err := d.download(phase, "Downloading the Steam launch graphics plugin", "SC4GraphicsOptions-v1.4.1.zip", url, hash, 1<<20)
	if err != nil {
		return "", err
	}
	output := filepath.Join(d.dir, graphicsDLL)
	if err := removeIfPresent(output); err != nil {
		return "", err
	}
	return output, extractOne(d.ctx, archive, graphicsDLL, output, 1<<20, "")
}

func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func checkSHA(ctx context.Context, file, want string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, contextReader{ctx, f}); err != nil {
		return err
	}
	if fmt.Sprintf("%x", h.Sum(nil)) != want {
		return fmt.Errorf("unexpected SHA-256 for %s", filepath.Base(file))
	}
	return nil
}

func (d *setupDependencies) retryResource(phase int, label string, attempt func() error) error {
	for {
		if err := d.ctx.Err(); err != nil {
			return err
		}
		d.step(phase, label)
		err := attempt()
		if err == nil {
			return nil
		}
		if d.ctx.Err() != nil {
			return d.ctx.Err()
		}
		err = fmt.Errorf("%s failed: %w", label, err)
		if d.waitForRetry == nil {
			return err
		}
		d.emit(installEvent{Type: "retry-resource", Phase: phase, Text: err.Error()})
		if err := d.waitForRetry(); err != nil {
			return err
		}
	}
}

func (d *setupDependencies) download(phase int, label, name, url, digest string, limit int64) (string, error) {
	output := filepath.Join(d.dir, name)
	if checkSHA(d.ctx, output, digest) == nil {
		fmt.Fprintf(os.Stderr, "Using verified cached download: %s\n", name)
		return output, nil
	}
	if err := d.ctx.Err(); err != nil {
		return "", err
	}
	if err := removeIfPresent(output); err != nil {
		return "", err
	}
	err := d.retryResource(phase, label, func() error {
		return d.downloadResource(phase, output, url, digest, limit)
	})
	if err != nil {
		return "", err
	}
	return output, nil
}

func (d *setupDependencies) downloadResource(phase int, output, url, digest string, limit int64) error {
	req, err := http.NewRequestWithContext(d.ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 2 * time.Hour}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > limit {
		return fmt.Errorf("HTTP %s or unexpected file size", resp.Status)
	}
	return publishFile(output, func(w io.Writer) error {
		h := sha256.New()
		reader := io.LimitReader(resp.Body, limit+1)
		buf := make([]byte, 128<<10)
		var total int64
		last := -1
		w = io.MultiWriter(w, h)
		for {
			n, err := reader.Read(buf)
			total += int64(n)
			if total > limit {
				return errors.New("download exceeds the expected file size")
			}
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			if resp.ContentLength > 0 {
				pct := int(total * 100 / resp.ContentLength)
				if pct != last {
					d.emit(installEvent{Type: "progress", Phase: phase, Value: pct})
					last = pct
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
		}
		if fmt.Sprintf("%x", h.Sum(nil)) != digest {
			return fmt.Errorf("unexpected SHA-256 for %s", filepath.Base(output))
		}
		return nil
	})
}

func extractOne(ctx context.Context, archive, name, output string, max uint64, want string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, entry := range z.File {
		if path.Base(entry.Name) != name || !entry.Mode().IsRegular() || entry.UncompressedSize64 > max {
			continue
		}
		return publishFile(output, func(w io.Writer) error {
			r, err := entry.Open()
			if err != nil {
				return err
			}
			defer r.Close()
			h := sha256.New()
			if _, err := io.Copy(io.MultiWriter(w, h), contextReader{ctx, r}); err != nil {
				return err
			}
			if want != "" && fmt.Sprintf("%x", h.Sum(nil)) != want {
				return fmt.Errorf("unexpected SHA-256 for %s", name)
			}
			return nil
		})
	}
	return fmt.Errorf("%s is missing from %s", name, filepath.Base(archive))
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
		err = publishFile(filepath.Join(stage, local), func(w io.Writer) error {
			r, err := entry.Open()
			if err != nil {
				return err
			}
			defer r.Close()
			_, err = io.Copy(w, contextReader{ctx, r})
			return err
		})
		if err != nil {
			return err
		}
	}
	if !fileExists(filepath.Join(stage, "python.exe")) {
		return errors.New("Python is missing from the downloaded package")
	}
	return os.Rename(stage, destination)
}
