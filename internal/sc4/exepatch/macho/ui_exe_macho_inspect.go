package macho

import (
	"bytes"
	"errors"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/x86"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/patchsrc"
)

func ReadOptions(original, patched []byte) x86.Options {
	hasRedirect := func(site int) bool {
		// The patcher reports an unsupported executable even if it is too
		// short to read this call site.
		return site+5 <= len(original) && !bytes.Equal(patched[site:site+5], original[site:site+5])
	}
	return x86.Options{
		HdCursors:      hasRedirect(macCursorDirectorySite),
		LoadingPreview: hasRedirect(macLoadingPreviewSite),
		RetinaText:     hasRedirect(macRetinaGlyphCacheSite),
		RegionMap:      hasRedirect(macRegionTilesSite),
	}
}

func ValidateIsolatedLaunch(data []byte) error {
	end := macLaunchStub + len(patchsrc.MacLaunchCode)
	if end > len(data) || !bytes.Equal(data[macLaunchStub:end], patchsrc.MacLaunchCode) {
		return errors.New("stage a new macOS copy to enable isolated launch")
	}
	modeEnd := macDisplayModeStub + len(patchsrc.MacDisplayModeCode)
	if modeEnd > len(data) || !bytes.Equal(data[macDisplayModeStub:modeEnd], patchsrc.MacDisplayModeCode) {
		return errors.New("stage a new macOS copy to enable custom display modes")
	}
	return nil
}
