package sc4

import (
	"errors"
	"strconv"
	"strings"
)

const (
	macSetupDisplayDomain   = "com.aspyr.simcity4.steam"
	macAutomaticResolution  = -1
	macDisplayDimensionMask = (1 << 13) - 1
	macDisplayModeShift     = 26
	macDisplayWindowed      = 1
	macDisplayFullscreen    = 2
	macDisplayBorderless    = 3
)

func parseMacSetupResolution(input string) (int, error) {
	switch input {
	case "":
		return 0, nil
	case "auto":
		return macAutomaticResolution, nil
	}
	width, height, _ := strings.Cut(input, "x")
	w, _ := strconv.Atoi(width)
	h, _ := strconv.Atoi(height)
	if !resolutionPattern.MatchString(input) || !validMacResolution(w, h) {
		return 0, errors.New("Choose Automatic or a resolution between 800 × 600 and 7680 × 4320.")
	}
	return h<<13 | w, nil
}

func validMacResolution(w, h int) bool {
	return w >= 800 && w <= 7680 && h >= 600 && h <= 4320
}

func macResolutionLabel(packed int) string {
	w, h := packed&macDisplayDimensionMask, (packed>>13)&macDisplayDimensionMask
	mode := packed >> macDisplayModeShift
	if (mode != macDisplayWindowed && mode != macDisplayFullscreen) || !validMacResolution(w, h) {
		return "auto"
	}
	return strconv.Itoa(w) + "x" + strconv.Itoa(h)
}

func macResolutionWithMode(requested, current int) int {
	mode := current >> macDisplayModeShift
	if requested == macAutomaticResolution {
		if mode == macDisplayBorderless {
			return current
		}
		return 0
	}
	if mode != macDisplayWindowed {
		mode = macDisplayFullscreen
	}
	return mode<<macDisplayModeShift | requested
}
