package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

type Resources struct {
	Context         context.Context
	Dir             string
	Emit            func(Event)
	WaitForRetry    func() error
	python, weights string
}

func (d *Resources) Step(phase int, label string) {
	d.Emit(Event{Type: "step", Phase: phase, Text: label})
}

func (d *Resources) model(phase int, weights string) error {
	if err := RemoveIfPresent(weights); err != nil {
		return err
	}
	archive, err := d.Download(phase, "Downloading the upscale model", "model-v3detail.zip", ModelURL, ModelSHA, 350<<20)
	if err != nil {
		return err
	}
	d.Step(phase, "Verifying the upscale model")
	if err := ExtractOne(d.Context, archive, ModelName, weights, 300<<20, upscale.HatCheckpointSHA); err != nil {
		return err
	}
	_ = os.Remove(archive)
	return nil
}

func (d *Resources) RetryResource(phase int, label string, attempt func() error) error {
	for {
		if err := d.Context.Err(); err != nil {
			return err
		}
		d.Step(phase, label)
		err := attempt()
		if err == nil {
			return nil
		}
		if d.Context.Err() != nil {
			return d.Context.Err()
		}
		err = fmt.Errorf("%s failed: %w", label, err)
		if d.WaitForRetry == nil {
			return err
		}
		d.Emit(Event{Type: "retry-resource", Phase: phase, Text: err.Error()})
		if err := d.WaitForRetry(); err != nil {
			return err
		}
	}
}

func (d *Resources) Download(phase int, label, name, url, digest string, limit int64) (string, error) {
	output := filepath.Join(d.Dir, name)
	if CheckSHA(d.Context, output, digest) == nil {
		fmt.Fprintf(os.Stderr, "Using verified cached download: %s\n", name)
		return output, nil
	}
	if err := d.Context.Err(); err != nil {
		return "", err
	}
	if err := RemoveIfPresent(output); err != nil {
		return "", err
	}
	err := d.RetryResource(phase, label, func() error {
		return DownloadSetupFile(d.Context, output, url, digest, limit, func(value int) {
			d.Emit(Measurement("progress", phase, value, 0))
		})
	})
	if err != nil {
		return "", err
	}
	return output, nil
}
