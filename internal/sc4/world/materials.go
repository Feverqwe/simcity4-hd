package world

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/dbpf"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	textures "github.com/Feverqwe/simcity4-hd/internal/sc4/texture"
)

const S3DResource = 0x5ad0e817

func ModelMaterialIDs(data []byte) ([]uint32, error) {
	var ids []uint32
	if err := VisitModelTextures(data, func(reference []byte) {
		ids = append(ids, binary.LittleEndian.Uint32(reference))
	}); err != nil {
		return nil, err
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

func VisitModelTextures(data []byte, visit func([]byte)) error {
	if len(data) < 20 || string(data[:4]) != "3DMD" || string(data[8:12]) != "HEAD" {
		return errors.New("expected an S3D model")
	}
	major, minor := binary.LittleEndian.Uint16(data[16:]), binary.LittleEndian.Uint16(data[18:])
	if major != 1 || (minor != 3 && minor != 5) {
		return fmt.Errorf("unsupported S3D version %d.%d", major, minor)
	}
	// Some original models declare stale chunk lengths. Use the next tag
	// as the boundary and require every material group to fit within it.
	position := 8
	for _, tag := range []string{"VERT", "INDX", "PRIM", "MATS"} {
		minimum := position + 8
		if minimum > len(data) {
			return fmt.Errorf("missing S3D chunk %s", tag)
		}
		found := bytes.Index(data[minimum:], []byte(tag))
		if found < 0 {
			return fmt.Errorf("missing S3D chunk %s", tag)
		}
		position = minimum + found
	}
	minimum := position + 12
	if minimum > len(data) {
		return errors.New("truncated S3D material header")
	}
	end := bytes.Index(data[minimum:], []byte("ANIM"))
	if end < 0 {
		return errors.New("missing S3D chunk ANIM")
	}
	return visitMaterialTextures(data[position+8:minimum+end], minor, visit)
}

func visitMaterialTextures(data []byte, minor uint16, visit func([]byte)) error {
	groups := binary.LittleEndian.Uint32(data)
	data = data[4:]
	nameLengthOffset := 10
	if minor == 5 {
		nameLengthOffset = 12
	}
	for range groups {
		if len(data) < 16 {
			return errors.New("truncated material group")
		}
		count := int(data[15])
		data = data[16:]
		for range count {
			if len(data) <= nameLengthOffset {
				return errors.New("truncated texture reference")
			}
			end := nameLengthOffset + 1 + int(data[nameLengthOffset])
			if end > len(data) {
				return errors.New("truncated material name")
			}
			visit(data[:end])
			data = data[end:]
		}
	}
	if len(data) != 0 {
		return errors.New("unexpected data after S3D material groups")
	}
	return nil
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
	catalog, err := gamefiles.OpenCatalog(game, func(id dbpf.ID) bool {
		return id.Kind == textures.FSHResource || id.Kind == S3DResource
	})
	if err != nil {
		return nil, err
	}
	defer catalog.Close()

	textureGroups := make(map[uint32][]uint32)
	modelsByArchive := make(map[*dbpf.Archive][]dbpf.Record)
	var modelArchives []*dbpf.Archive
	report := &materialReport{
		Errors: make(map[string]string), Materials: make(map[string]materialReference),
	}
	for _, id := range catalog.Order {
		if id.Kind == textures.FSHResource {
			textureGroups[id.Instance] = append(textureGroups[id.Instance], id.Group)
			continue
		}
		location := catalog.Owners[id]
		if _, exists := modelsByArchive[location.Archive]; !exists {
			modelArchives = append(modelArchives, location.Archive)
		}
		modelsByArchive[location.Archive] = append(modelsByArchive[location.Archive], location.Record)
		report.ModelCount++
	}
	for _, a := range modelArchives {
		models := modelsByArchive[a]
		slices.SortStableFunc(models, func(left, right dbpf.Record) int {
			return int(left.Offset) - int(right.Offset)
		})
		for _, model := range models {
			key := fmt.Sprintf("%08x:%08x", model.ID.Group, model.ID.Instance)
			data, err := a.Read(model)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", a.Name(), key, err)
			}
			ids, err := ModelMaterialIDs(data)
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

func RunMaterialIndex(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: simcity4-uhd index-materials GAME_DIR OUTPUT.json")
	}
	report, err := indexMaterials(args[0])
	if err != nil {
		return err
	}
	if err := gamefiles.PublishFile(args[1], func(w io.Writer) error {
		return json.NewEncoder(w).Encode(report)
	}); err != nil {
		return err
	}
	fmt.Printf("%d models, %d material IDs, %d parse errors\n", report.ModelCount, len(report.Materials), len(report.Errors))
	return nil
}
