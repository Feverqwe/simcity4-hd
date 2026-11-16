package ui

import (
	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
)

func driveGaugeEntries(stock uiStock, art uiArtwork) ([]dbpf.Entry, error) {
	// Frame counts come from the stock Free Drive exemplars. The gauge
	// selects a single frame from each horizontal strip.
	atlases := [...]struct {
		instance uint32
		frames   int
	}{
		{0xebcbb93f, 59}, {0xcbcb6e9f, 55}, {0xcbe99a5e, 60},
		{0x0c0729aa, 60}, {0x0c0729ab, 15}, {0xcbcba947, 37},
		{0xcbcba948, 55}, {0xcbcba949, 55}, {0xcbcba950, 55}, {0xcbcba952, 55},
		{0xabec941c, 60}, {0xac0da30b, 25}, {0xac0da30c, 59},
		{0xac0da30d, 9}, {0xac0da30e, 37}, {0x0beb3dbf, 16},
	}
	entries := make([]dbpf.Entry, 0, len(atlases))
	for _, atlas := range atlases {
		entry, err := runtimeArtwork(stock, art, atlas.instance, atlas.frames, true)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
