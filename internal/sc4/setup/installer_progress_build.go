//go:build windows || darwin

package setup

import (
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
	"time"
)

func buildProgress(deps *Resources, phase *int) upscale.Progress {
	const updateInterval = 250 * time.Millisecond
	var label string
	var current Event
	var lastSent time.Time
	emit := deps.Emit
	return upscale.Progress{
		Prepare: func() (string, string, error) {
			python, weights, err := deps.Inference(*phase)
			if err == nil {
				emit(Event{Type: "step", Phase: *phase, Text: label})
				if current.Type != "" {
					emit(current)
					lastSent = time.Now()
				}
			}
			return python, weights, err
		},
		Step: func(step string) {
			label = step
			current = Event{}
			lastSent = time.Time{}
			emit(Event{Type: "step", Phase: *phase, Text: step})
		},
		Resources: func(done, total int) {
			previous := current
			current = Measurement("count", *phase, done, total)
			now := time.Now()
			if done <= 1 || total > 0 && done == total || previous.Total != total || now.Sub(lastSent) >= updateInterval {
				emit(current)
				lastSent = now
			}
		},
	}
}
