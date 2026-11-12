package sc4

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/Feverqwe/simcity4-hd/inference"
)

const maxInferenceImage = 64 << 20

// inferenceWorker handles sequential requests to one model instance. PNG
// frames travel over stdio, so the worker only needs access to the checkpoint.
type inferenceWorker struct {
	command *exec.Cmd
	input   io.WriteCloser
	output  *bufio.Reader
}

type inferenceConfig struct {
	distro, python, weights string
	scale, tile, overlap    int
	wsl                     bool
	prepare                 func() (python, weights string, err error)
}

func inferenceFlags(flags *flag.FlagSet) func() (inferenceConfig, error) {
	pythonDefault := "python3"
	if runtime.GOOS == "windows" {
		pythonDefault = "python.exe"
	}
	python := flags.String("python", pythonDefault, "local Python interpreter for inference")
	weights := flags.String("weights", "", "local checkpoint path for cache misses")
	wslDistro := flags.String("wsl-distro", "", "WSL distribution for inference")
	wslPython := flags.String("wsl-python", "python3", "Python interpreter in WSL")
	wslWeights := flags.String("wsl-weights", "", "checkpoint path in WSL for cache misses")
	return func() (inferenceConfig, error) {
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
			return inferenceConfig{}, errors.New("choose either local Python options or WSL options for inference")
		}
		config := inferenceConfig{python: *python, weights: *weights, scale: 4, tile: 64, overlap: 16}
		if wsl {
			config.wsl = true
			config.distro, config.python, config.weights = *wslDistro, *wslPython, *wslWeights
		}
		return config, nil
	}
}

func startInference(ctx context.Context, config inferenceConfig, diagnostics io.Writer) (*inferenceWorker, error) {
	if config.prepare != nil {
		var err error
		config.python, config.weights, err = config.prepare()
		if err != nil {
			return nil, err
		}
	}
	if config.python == "" || config.weights == "" || config.tile <= 0 || config.overlap < 0 ||
		(config.scale != 2 && config.scale != 4) {
		return nil, errors.New("invalid inference configuration")
	}
	workerArgs := []string{"-u", "-c", inference.WorkerSource,
		"--weights", config.weights, "--scale", strconv.Itoa(config.scale),
		"--tile", strconv.Itoa(config.tile), "--overlap", strconv.Itoa(config.overlap)}
	var command *exec.Cmd
	switch {
	case runtime.GOOS == "windows" && config.wsl:
		var args []string
		if config.distro != "" {
			args = append(args, "--distribution", config.distro)
		}
		args = append(args, "--exec", config.python)
		command = exec.CommandContext(ctx, "wsl.exe", append(args, workerArgs...)...)
	case runtime.GOOS == "windows", runtime.GOOS == "linux":
		if config.distro != "" {
			return nil, errors.New("select the WSL distribution before running the Linux builder")
		}
		if config.prepare != nil {
			workerArgs = append([]string{"-I"}, workerArgs...)
		}
		command = exec.CommandContext(ctx, config.python, workerArgs...)
		if config.prepare != nil {
			command.Dir = filepath.Dir(config.python)
		}
	default:
		return nil, errors.New("HAT inference requires Windows or Linux")
	}
	configureBackgroundProcess(command)
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
	return &inferenceWorker{command, input, bufio.NewReader(output)}, nil
}

func (w *inferenceWorker) upscalePNG(image []byte) ([]byte, error) {
	if len(image) == 0 || len(image) > maxInferenceImage {
		return nil, errors.New("invalid inference input size")
	}
	var header [4]byte
	little.PutUint32(header[:], uint32(len(image)))
	if _, err := w.input.Write(header[:]); err != nil {
		return nil, fmt.Errorf("send image length to inference worker: %w", err)
	}
	if _, err := w.input.Write(image); err != nil {
		return nil, fmt.Errorf("send image to inference worker: %w", err)
	}
	if _, err := io.ReadFull(w.output, header[:]); err != nil {
		return nil, fmt.Errorf("read image length from inference worker: %w", err)
	}
	length := little.Uint32(header[:])
	if length == 0 || length > maxInferenceImage {
		return nil, errors.New("invalid inference response size")
	}
	response := make([]byte, length)
	if _, err := io.ReadFull(w.output, response); err != nil {
		return nil, fmt.Errorf("read image from inference worker: %w", err)
	}
	return response, nil
}

func (w *inferenceWorker) close() error {
	return errors.Join(w.input.Close(), w.command.Wait())
}

// buildProgress lets the installer provision inference only on a cache miss.
type buildProgress struct {
	prepare   func() (string, string, error)
	step      func(string)
	resources func(done, total int)
}

func (p buildProgress) report(step string) {
	if p.step != nil {
		p.step(step)
	}
}

func (p buildProgress) track(total int) func(int) {
	if p.resources == nil || total == 0 {
		return func(int) {}
	}
	return func(done int) { p.resources(done, total) }
}

func (p buildProgress) count() func() {
	if p.resources == nil {
		return nil
	}
	done := 0
	p.resources(done, 0)
	return func() {
		done++
		p.resources(done, 0)
	}
}

func (c inferenceConfig) enabled() bool { return c.weights != "" || c.prepare != nil }
