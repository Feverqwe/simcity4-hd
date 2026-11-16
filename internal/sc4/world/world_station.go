package world

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/texturecache"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/upscale"
)

const (
	stationBase       = 0x03100000
	stationHDBase     = 0xe74b0000
	stationModelGroup = 0xbadb57f1
	stationNight      = 0x8000
)

func patchStationMaterial(model []byte, source, target uint32) error {
	at := bytes.Index(model, []byte("MATS"))
	if len(model) < 4 || string(model[:4]) != "3DMD" || at < 0 || len(model)-at < 32 ||
		binary.LittleEndian.Uint32(model[at+4:]) < 32 || binary.LittleEndian.Uint32(model[at+8:]) != 1 || model[at+27] != 1 ||
		binary.LittleEndian.Uint32(model[at+28:]) != source {
		return fmt.Errorf("unexpected station model material %#08x", source)
	}
	binary.LittleEndian.PutUint32(model[at+28:], target)
	return nil
}

func stationEntries(stock *gamefiles.Catalog, art worldArtwork, progress upscale.Progress) (day, night []dbpf.Entry, err error) {
	const zoomLevels, rotations, texturesPerModel = 5, 4, 2
	report := progress.Track(zoomLevels * rotations * texturesPerModel)
	cache := art.modelCache
	for zoom := range uint32(zoomLevels) {
		for rotation := range uint32(rotations) {
			processed := int(zoom*rotations+rotation) * texturesPerModel
			report(processed)
			offset := zoom*256 + rotation*16
			dayID := stationBase + offset
			targetID := stationHDBase + offset
			modelKey := dbpf.ID{Kind: S3DResource, Group: stationModelGroup, Instance: dayID}
			textureKey := dbpf.ID{Kind: textures.FSHResource, Group: texturecache.NetworkGroup, Instance: dayID}
			nightKey := dbpf.ID{Kind: textures.FSHResource, Group: texturecache.NetworkGroup, Instance: dayID | stationNight}
			model, err := readWorldStock(stock, modelKey)
			if err != nil {
				return nil, nil, err
			}
			if err := patchStationMaterial(model, dayID, targetID); err != nil {
				return nil, nil, err
			}
			original, err := readWorldStock(stock, textureKey)
			if err != nil {
				return nil, nil, err
			}
			hd, err := art.texture(cache, original, false)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", textureKey, err)
			}
			report(processed + 1)
			originalNight, err := readWorldStock(stock, nightKey)
			if err != nil {
				return nil, nil, err
			}
			hdNight, err := art.texture(cache, originalNight, false)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", nightKey, err)
			}
			textureKey.Instance = targetID
			nightKey.Instance = targetID | stationNight
			for _, key := range []dbpf.ID{textureKey, nightKey} {
				if _, exists := stock.Owners[key]; exists {
					return nil, nil, fmt.Errorf("station replacement key already exists: %s", key)
				}
			}
			day = append(day, dbpf.Entry{ID: modelKey, Data: model}, dbpf.Entry{ID: textureKey, Data: hd})
			night = append(night, dbpf.Entry{ID: nightKey, Data: hdNight})
		}
	}
	report(zoomLevels * rotations * texturesPerModel)
	return day, night, nil
}

func readWorldStock(stock *gamefiles.Catalog, id dbpf.ID) ([]byte, error) {
	source, ok := stock.Owners[id]
	if !ok {
		return nil, fmt.Errorf("missing stock resource %s", id)
	}
	data, err := source.Archive.Read(source.Record)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	return data, nil
}
