package game

import (
	"regexp"
)

const DefaultResolution = "3840x2160"

var ResolutionPattern = regexp.MustCompile(`^[1-9][0-9]{2,4}x[1-9][0-9]{2,4}$`)
