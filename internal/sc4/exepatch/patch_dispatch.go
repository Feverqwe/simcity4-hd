package exepatch

import (
	"errors"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/macho"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/pe"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
)

func Patch(data []byte, mac bool, scale int, options x86.Options) ([]byte, error) {
	if !mac {
		if options.RetinaText {
			return nil, errors.New("Retina text is available on macOS")
		}
		return pe.Patch(data, scale, options)
	}
	patched, err := macho.PatchUI(data, scale)
	if err != nil {
		return patched, err
	}
	return macho.PatchFeatures(patched, options, scale)
}
