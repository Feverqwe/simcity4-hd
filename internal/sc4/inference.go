package sc4

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/Feverqwe/simcity4-hd/inference"
)

const maxInferenceImage = 64 << 20

// inferenceWorker handles sequential requests to one model instance. PNG
// frames travel over stdio, so game and cache paths never cross into WSL.
type inferenceWorker struct {
	command *exec.Cmd
	input   io.WriteCloser
	output  *bufio.Reader
}

type inferenceConfig struct {
	distro, python, weights string
	scale, tile, overlap    int
}

func startInference(ctx context.Context, config inferenceConfig, diagnostics io.Writer) (*inferenceWorker, error) {
	if config.python == "" || config.weights == "" || config.tile <= 0 || config.overlap < 0 ||
		(config.scale != 2 && config.scale != 4) {
		return nil, errors.New("invalid WSL inference configuration")
	}
	workerArgs := []string{"-u", "-c", inference.WorkerSource,
		"--weights", config.weights, "--scale", strconv.Itoa(config.scale),
		"--tile", strconv.Itoa(config.tile), "--overlap", strconv.Itoa(config.overlap)}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		var args []string
		if config.distro != "" {
			args = append(args, "--distribution", config.distro)
		}
		args = append(args, "--exec", config.python)
		command = exec.CommandContext(ctx, "wsl.exe", append(args, workerArgs...)...)
	case "linux":
		if config.distro != "" {
			return nil, errors.New("select the WSL distribution before running the Linux builder")
		}
		command = exec.CommandContext(ctx, config.python, workerArgs...)
	default:
		return nil, errors.New("HAT inference requires Windows with WSL or a local WSL session")
	}
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
		return nil, fmt.Errorf("start WSL inference: %w", err)
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
		return nil, fmt.Errorf("send image length to WSL: %w", err)
	}
	if _, err := w.input.Write(image); err != nil {
		return nil, fmt.Errorf("send image to WSL: %w", err)
	}
	if _, err := io.ReadFull(w.output, header[:]); err != nil {
		return nil, fmt.Errorf("read image length from WSL: %w", err)
	}
	length := little.Uint32(header[:])
	if length == 0 || length > maxInferenceImage {
		return nil, errors.New("invalid inference response size")
	}
	response := make([]byte, length)
	if _, err := io.ReadFull(w.output, response); err != nil {
		return nil, fmt.Errorf("read image from WSL: %w", err)
	}
	return response, nil
}

func (w *inferenceWorker) close() error {
	return errors.Join(w.input.Close(), w.command.Wait())
}
