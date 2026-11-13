package sc4

import (
	"bytes"
	"fmt"
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
		little.Uint32(model[at+4:]) < 32 || little.Uint32(model[at+8:]) != 1 || model[at+27] != 1 ||
		little.Uint32(model[at+28:]) != source {
		return fmt.Errorf("unexpected station model material %#08x", source)
	}
	little.PutUint32(model[at+28:], target)
	return nil
}

func stationEntries(stock *gameCatalog, art worldArtwork, progress buildProgress) (day, night []resource, err error) {
	const zoomLevels, rotations, texturesPerModel = 5, 4, 2
	report := progress.track(zoomLevels * rotations * texturesPerModel)
	cache := art.modelCache
	for zoom := range uint32(zoomLevels) {
		for rotation := range uint32(rotations) {
			processed := int(zoom*rotations+rotation) * texturesPerModel
			report(processed)
			offset := zoom*256 + rotation*16
			dayID := stationBase + offset
			targetID := stationHDBase + offset
			modelKey := resourceID{s3dResource, stationModelGroup, dayID}
			textureKey := resourceID{fshResource, networkGroup, dayID}
			nightKey := resourceID{fshResource, networkGroup, dayID | stationNight}
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
			textureKey.instance = targetID
			nightKey.instance = targetID | stationNight
			for _, key := range []resourceID{textureKey, nightKey} {
				if _, exists := stock.owners[key]; exists {
					return nil, nil, fmt.Errorf("station replacement key already exists: %s", key)
				}
			}
			day = append(day, resource{modelKey, model}, resource{textureKey, hd})
			night = append(night, resource{nightKey, hdNight})
		}
	}
	report(zoomLevels * rotations * texturesPerModel)
	return day, night, nil
}

func readWorldStock(stock *gameCatalog, id resourceID) ([]byte, error) {
	source, ok := stock.owners[id]
	if !ok {
		return nil, fmt.Errorf("missing stock resource %s", id)
	}
	data, err := source.archive.read(source.record)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	return data, nil
}
