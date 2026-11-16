package upscale

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/Feverqwe/simcity4-hd/inference"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
)

const MaxInferenceImage = 64 << 20

// inferenceWorker handles sequential requests to one model instance. PNG
// frames travel over stdio, so the worker only needs access to the checkpoint.
type InferenceWorker struct {
	command *exec.Cmd
	input   io.WriteCloser
	output  *bufio.Reader
}

type Config struct {
	distro, python, Weights string
	Scale, Tile, Overlap    int
	wsl, KeepUpscaledImages bool
	Prepare                 func() (python, weights string, err error)
}

func InferenceFlags(flags *flag.FlagSet) func() (Config, error) {
	pythonDefault := "python3"
	if runtime.GOOS == "windows" {
		pythonDefault = "python.exe"
	}
	python := flags.String("python", pythonDefault, "local Python interpreter for inference")
	weights := flags.String("weights", "", "local checkpoint path for cache misses")
	keepUpscaledImages := flags.Bool("keep-upscaled-images", false, "cache the model output before texture packing")
	wslDistro := flags.String("wsl-distro", "", "WSL distribution for inference")
	wslPython := flags.String("wsl-python", "python3", "Python interpreter in WSL")
	wslWeights := flags.String("wsl-weights", "", "checkpoint path in WSL for cache misses")
	return func() (Config, error) {
		var local, wsl bool
		flags.Visit(func(option *flag.Flag) {
			switch option.Name {
			case "python", "weights":
				local = true
			case "wsl-distro", "wsl-python", "wsl-weights":
				wsl = true
			}
		})
		if local && wsl {
			return Config{}, errors.New("choose either local Python options or WSL options for inference")
		}
		config := Config{python: *python, Weights: *weights, Scale: 4, Tile: 64, Overlap: 16, KeepUpscaledImages: *keepUpscaledImages}
		if wsl {
			config.wsl = true
			config.distro, config.python, config.Weights = *wslDistro, *wslPython, *wslWeights
		}
		return config, nil
	}
}

func StartInference(ctx context.Context, config Config, diagnostics io.Writer) (*InferenceWorker, error) {
	if config.Prepare != nil {
		var err error
		config.python, config.Weights, err = config.Prepare()
		if err != nil {
			return nil, err
		}
	}
	if config.python == "" || config.Weights == "" || config.Tile <= 0 || config.Overlap < 0 ||
		(config.Scale != 2 && config.Scale != 4) {
		return nil, errors.New("invalid inference configuration")
	}
	workerArgs := []string{"-u", "-c", inference.WorkerSource,
		"--weights", config.Weights, "--scale", strconv.Itoa(config.Scale),
		"--tile", strconv.Itoa(config.Tile), "--overlap", strconv.Itoa(config.Overlap)}
	var command *exec.Cmd
	switch {
	case runtime.GOOS == "windows" && config.wsl:
		var args []string
		if config.distro != "" {
			args = append(args, "--distribution", config.distro)
		}
		args = append(args, "--exec", config.python)
		command = exec.CommandContext(ctx, "wsl.exe", append(args, workerArgs...)...)
	case runtime.GOOS == "windows", runtime.GOOS == "linux", runtime.GOOS == "darwin":
		if config.distro != "" || runtime.GOOS == "darwin" && config.wsl {
			return nil, errors.New("WSL inference must run from the Windows builder")
		}
		if config.Prepare != nil {
			workerArgs = append([]string{"-I"}, workerArgs...)
		}
		command = exec.CommandContext(ctx, config.python, workerArgs...)
		if config.Prepare != nil {
			command.Dir = filepath.Dir(config.python)
		}
	default:
		return nil, errors.New("HAT inference requires Windows, Linux, or macOS")
	}
	gamefiles.ConfigureBackgroundProcess(command)
	command.Stderr = diagnostics
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	if err := command.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, fmt.Errorf("start inference: %w", err)
	}
	return &InferenceWorker{command, input, bufio.NewReader(output)}, nil
}

func (w *InferenceWorker) UpscalePNG(image []byte) ([]byte, error) {
	if len(image) == 0 || len(image) > MaxInferenceImage {
		return nil, errors.New("invalid inference input size")
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(image)))
	if _, err := w.input.Write(header[:]); err != nil {
		return nil, fmt.Errorf("send image length to inference worker: %w", err)
	}
	if _, err := w.input.Write(image); err != nil {
		return nil, fmt.Errorf("send image to inference worker: %w", err)
	}
	if _, err := io.ReadFull(w.output, header[:]); err != nil {
		return nil, fmt.Errorf("read image length from inference worker: %w", err)
	}
	length := binary.LittleEndian.Uint32(header[:])
	if length == 0 || length > MaxInferenceImage {
		return nil, errors.New("invalid inference response size")
	}
	response := make([]byte, length)
	if _, err := io.ReadFull(w.output, response); err != nil {
		return nil, fmt.Errorf("read image from inference worker: %w", err)
	}
	return response, nil
}

func (w *InferenceWorker) Close() error {
	return errors.Join(w.input.Close(), w.command.Wait())
}

// buildProgress lets the installer provision inference only on a cache miss.
type Progress struct {
	Prepare   func() (string, string, error)
	Step      func(string)
	Resources func(done, total int)
}

func (p Progress) Report(step string) {
	if p.Step != nil {
		p.Step(step)
	}
}

func (p Progress) Track(total int) func(int) {
	if p.Resources == nil || total == 0 {
		return func(int) {}
	}
	return func(done int) { p.Resources(done, total) }
}

func (p Progress) Count() func() {
	if p.Resources == nil {
		return nil
	}
	done := 0
	p.Resources(done, 0)
	return func() {
		done++
		p.Resources(done, 0)
	}
}

func (c Config) Enabled() bool { return c.Weights != "" || c.Prepare != nil }
