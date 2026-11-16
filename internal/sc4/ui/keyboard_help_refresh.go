package ui

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"slices"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/uiids"
)

func RefreshMacKeyboardHelp(ctx context.Context, gameData, stage string, packages []string) error {
	hud := slices.IndexFunc(packages, func(path string) bool {
		name := filepath.Base(path)
		return name == "city-hud-x2.dat" || name == "city-hud-x4.dat"
	})
	if hud < 0 {
		return nil
	}
	pkg, err := dbpf.Open(packages[hud])
	if err != nil {
		return err
	}
	defer pkg.Close()
	key := dbpf.ID{Kind: 0x2026960b, Group: uiids.HdUITextGroup, Instance: uiids.KeyHelpContent}
	entry := slices.IndexFunc(pkg.Records, func(r dbpf.Record) bool { return r.ID == key })
	if entry < 0 {
		return nil
	}
	stock, err := dbpf.Open(filepath.Join(gameData, "SimCity_1.dat"))
	if err != nil {
		return err
	}
	defer stock.Close()
	help, err := keyboardHelpEntries(indexUIStock(stock), slices.ContainsFunc(packages, func(path string) bool {
		return filepath.Base(path) == RussianTextPackage
	}), true)
	if err != nil {
		return err
	}
	current, err := pkg.Read(pkg.Records[entry])
	if err != nil {
		return err
	}
	replacement := help[1].Data
	if bytes.Equal(current, replacement) {
		return nil
	}
	entries := make([]dbpf.Entry, 0, len(pkg.Records))
	for i, record := range pkg.Records {
		if err := ctx.Err(); err != nil {
			return err
		}
		data := replacement
		if i != entry {
			// Copy stored bytes so existing texture and compression data remain intact.
			data, err = pkg.ReadRaw(record)
			if err != nil {
				return err
			}
		}
		entries = append(entries, dbpf.Entry{ID: record.ID, Data: data})
	}
	target := filepath.Join(stage, filepath.Base(packages[hud]))
	if err := gamefiles.PublishFile(target, func(w io.Writer) error { return dbpf.Write(w, entries) }); err != nil {
		return err
	}
	packages[hud] = target
	return nil
}
