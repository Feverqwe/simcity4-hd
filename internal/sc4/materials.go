package sc4

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
)

const s3dResource = 0x5ad0e817

func modelMaterialIDs(data []byte) ([]uint32, error) {
	if len(data) < 20 || string(data[:4]) != "3DMD" || string(data[8:12]) != "HEAD" {
		return nil, errors.New("expected an S3D model")
	}
	major, minor := little.Uint16(data[16:]), little.Uint16(data[18:])
	if major != 1 || (minor != 3 && minor != 5) {
		return nil, fmt.Errorf("unsupported S3D version %d.%d", major, minor)
	}
	// Some original models declare stale chunk lengths. Use the next tag
	// as the boundary and require every material group to fit within it.
	position := 8
	for _, tag := range []string{"VERT", "INDX", "PRIM", "MATS"} {
		minimum := position + 8
		if minimum > len(data) {
			return nil, fmt.Errorf("missing S3D chunk %s", tag)
		}
		found := bytes.Index(data[minimum:], []byte(tag))
		if found < 0 {
			return nil, fmt.Errorf("missing S3D chunk %s", tag)
		}
		position = minimum + found
	}
	minimum := position + 12
	if minimum > len(data) {
		return nil, errors.New("truncated S3D material header")
	}
	end := bytes.Index(data[minimum:], []byte("ANIM"))
	if end < 0 {
		return nil, errors.New("missing S3D chunk ANIM")
	}
	return parseMaterialGroups(data[position+8:minimum+end], minor)
}

func parseMaterialGroups(data []byte, minor uint16) ([]uint32, error) {
	groups := little.Uint32(data)
	data = data[4:]
	nameLengthOffset := 10
	if minor == 5 {
		nameLengthOffset = 12
	}
	var ids []uint32
	for range groups {
		if len(data) < 16 {
			return nil, errors.New("truncated material group")
		}
		count := int(data[15])
		data = data[16:]
		for range count {
			if len(data) <= nameLengthOffset {
				return nil, errors.New("truncated texture reference")
			}
			end := nameLengthOffset + 1 + int(data[nameLengthOffset])
			if end > len(data) {
				return nil, errors.New("truncated material name")
			}
			ids = append(ids, little.Uint32(data))
			data = data[end:]
		}
	}
	if len(data) != 0 {
		return nil, errors.New("unexpected data after S3D material groups")
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

type materialReference struct {
	Groups []string `json:"fsh_groups"`
	Models []string `json:"models"`
}

type materialReport struct {
	ModelCount int                          `json:"models"`
	Errors     map[string]string            `json:"parse_errors"`
	Materials  map[string]materialReference `json:"materials"`
}

func indexMaterials(game string) (*materialReport, error) {
	catalog, err := openGameCatalog(game, func(id resourceID) bool {
		return id.kind == fshResource || id.kind == s3dResource
	})
	if err != nil {
		return nil, err
	}
	defer catalog.close()

	textureGroups := make(map[uint32][]uint32)
	modelsByArchive := make(map[*archive][]record)
	var modelArchives []*archive
	report := &materialReport{
		Errors: make(map[string]string), Materials: make(map[string]materialReference),
	}
	for _, id := range catalog.order {
		if id.kind == fshResource {
			textureGroups[id.instance] = append(textureGroups[id.instance], id.group)
			continue
		}
		location := catalog.owners[id]
		if _, exists := modelsByArchive[location.archive]; !exists {
			modelArchives = append(modelArchives, location.archive)
		}
		modelsByArchive[location.archive] = append(modelsByArchive[location.archive], location.record)
		report.ModelCount++
	}
	for _, a := range modelArchives {
		models := modelsByArchive[a]
		slices.SortStableFunc(models, func(left, right record) int {
			return int(left.offset) - int(right.offset)
		})
		for _, model := range models {
			key := fmt.Sprintf("%08x:%08x", model.id.group, model.id.instance)
			data, err := a.read(model)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", a.file.Name(), key, err)
			}
			ids, err := modelMaterialIDs(data)
			if err != nil {
				report.Errors[key] = err.Error()
				continue
			}
			for _, id := range ids {
				keyID := fmt.Sprintf("%08x", id)
				reference, exists := report.Materials[keyID]
				if !exists {
					groups := textureGroups[id]
					slices.Sort(groups)
					reference.Groups = make([]string, 0, len(groups))
					for _, group := range groups {
						reference.Groups = append(reference.Groups, fmt.Sprintf("%08x", group))
					}
				}
				reference.Models = append(reference.Models, key)
				report.Materials[keyID] = reference
			}
		}
	}
	return report, nil
}

func runMaterialIndex(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: simcity4-hd index-materials GAME_DIR OUTPUT.json")
	}
	report, err := indexMaterials(args[0])
	if err != nil {
		return err
	}
	if err := publishFile(args[1], func(w io.Writer) error {
		return json.NewEncoder(w).Encode(report)
	}); err != nil {
		return err
	}
	fmt.Printf("%d models, %d material IDs, %d parse errors\n", report.ModelCount, len(report.Materials), len(report.Errors))
	return nil
}
