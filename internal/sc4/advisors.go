package sc4

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"slices"
)

type advisorPoint [3]float64
type advisorUV [2]float64
type advisorWeight [2]float64 // point index and weight
type advisorEdge [5]float64   // two corner indices and a local displacement

type advisorArt struct {
	Anchors [5][]advisorWeight
	Deltas  []advisorPoint
	Edges   []advisorEdge
}

type advisorSurface struct {
	ID    uint16
	Flags uint32
	P     [][]advisorWeight
	UV    [][]advisorUV
	Pairs [][2]int // position and UV indices for each vertex
	Art   *advisorArt
}

type advisorClip struct {
	Stock     string // SHA-256 of the decoded original S3D
	Frames    int
	Factor    int
	Positions [][]int // byte offsets in the original S3D, one per keyframe or a fixed offset
	Surfaces  []advisorSurface
	Groups    [][3]int // surface and frame, or -1 followed by the offset and size of an unchanged group
	Head      string
	Tail      string
	Dynamic   map[int]map[int]advisorPoint // edge displacements for poses with animated UVs
}

func validAdvisorIndex(v float64, count int) bool {
	return v >= 0 && v < float64(count) && v == math.Trunc(v)
}

func (clip advisorClip) validate() error {
	if clip.Frames <= 0 || clip.Frames > 90 || (clip.Factor != 2 && clip.Factor != 6) ||
		len(clip.Groups) == 0 || len(clip.Groups) > math.MaxUint16 {
		return errors.New("invalid advisor clip dimensions")
	}
	digest, err := hex.DecodeString(clip.Stock)
	if err != nil || len(digest) != sha256.Size {
		return errors.New("invalid advisor stock fingerprint")
	}
	for _, offsets := range clip.Positions {
		if len(offsets) != 1 && len(offsets) != clip.Frames {
			return errors.New("invalid advisor source trajectory")
		}
	}
	for _, surface := range clip.Surfaces {
		for _, weights := range surface.P {
			if len(weights) == 0 {
				return errors.New("empty advisor vertex weights")
			}
			for _, pair := range weights {
				if !validAdvisorIndex(pair[0], len(clip.Positions)) {
					return errors.New("invalid advisor position index")
				}
			}
		}
		for _, track := range surface.UV {
			if len(track) != 1 && len(track) != clip.Frames {
				return errors.New("invalid advisor texture trajectory")
			}
		}
		for _, pair := range surface.Pairs {
			if pair[0] < 0 || pair[0] >= len(surface.P) || pair[1] < 0 || pair[1] >= len(surface.UV) {
				return errors.New("invalid advisor vertex indices")
			}
		}
		if err := surface.validateArt(); err != nil {
			return err
		}
	}
	for _, group := range clip.Groups {
		if group[0] == -1 {
			if group[2] < 8 || group[2] > 1024 {
				return errors.New("invalid unchanged advisor vertex group")
			}
		} else if group[0] < 0 || group[0] >= len(clip.Surfaces) || group[1] < 0 || group[1] >= clip.Frames*clip.Factor {
			return errors.New("invalid advisor animation group")
		}
	}
	for group, edits := range clip.Dynamic {
		if group < 0 || group >= len(clip.Groups) || clip.Groups[group][0] < 0 {
			return errors.New("invalid advisor textured pose")
		}
		art := clip.Surfaces[clip.Groups[group][0]].Art
		for edge := range edits {
			if art == nil || edge < 0 || edge >= len(art.Edges) {
				return errors.New("invalid advisor animated edge")
			}
		}
	}
	return nil
}

func (surface advisorSurface) validateArt() error {
	count := len(surface.Pairs)
	if art := surface.Art; art != nil {
		if len(art.Deltas) != count {
			return errors.New("invalid advisor shape offsets")
		}
		for _, anchor := range art.Anchors {
			if len(anchor) == 0 {
				return errors.New("missing advisor facial landmark")
			}
			for _, pair := range anchor {
				if !validAdvisorIndex(pair[0], count) {
					return errors.New("invalid advisor facial landmark")
				}
			}
		}
		for _, edge := range art.Edges {
			if !validAdvisorIndex(edge[0], count) || !validAdvisorIndex(edge[1], count) {
				return errors.New("invalid advisor facial edge")
			}
		}
		count += len(art.Edges)
	}
	if count == 0 || count > math.MaxUint16 {
		return errors.New("invalid advisor vertex count")
	}
	return nil
}

type advisorVertex struct {
	point advisorPoint
	uv    advisorUV
}

func advisorSample[T any](frames []T, frame int) T {
	if len(frames) == 1 {
		return frames[0]
	}
	return frames[frame]
}

func (clip advisorClip) readPositions(stock []byte) ([][]advisorPoint, error) {
	points := make([][]advisorPoint, len(clip.Positions))
	for i, offsets := range clip.Positions {
		for _, offset := range offsets {
			if offset < 32 || offset > len(stock)-12 {
				return nil, errors.New("advisor position lies outside the original S3D")
			}
			var point advisorPoint
			for axis := range point {
				point[axis] = float64(math.Float32frombits(little.Uint32(stock[offset+axis*4:])))
			}
			points[i] = append(points[i], point)
		}
	}
	return points, nil
}

func advisorWeighted(weights []advisorWeight, point func(int) advisorPoint) advisorPoint {
	var result advisorPoint
	for _, weight := range weights {
		source := point(int(weight[0]))
		for axis, v := range source {
			result[axis] += float64(v * weight[1])
		}
	}
	return result
}

func (surface advisorSurface) positions(points [][]advisorPoint, keyframes int) [][]advisorPoint {
	result := make([][]advisorPoint, keyframes)
	for frame := range result {
		result[frame] = make([]advisorPoint, len(surface.P))
		for i, weights := range surface.P {
			result[frame][i] = advisorWeighted(weights, func(point int) advisorPoint {
				return advisorSample(points[point], frame)
			})
		}
	}
	return result
}

func advisorInterpolate(a, b, t float64) float64 { return a + float64((b-a)*t) }

func (surface advisorSurface) vertices(positions [][]advisorPoint, factor, frame int) []advisorVertex {
	first := frame / factor
	next := (first + 1) % len(positions)
	step := float64(frame%factor) / float64(factor)
	vertices := make([]advisorVertex, len(surface.Pairs))
	for i, pair := range surface.Pairs {
		p, uv := pair[0], pair[1]
		for axis, v := range positions[first][p] {
			vertices[i].point[axis] = float64(float32(advisorInterpolate(v, positions[next][p][axis], step)))
		}
		a, b := advisorSample(surface.UV[uv], first), advisorSample(surface.UV[uv], next)
		for axis, v := range a {
			vertices[i].uv[axis] = float64(float32(advisorInterpolate(v, b[axis], step)))
		}
	}
	return vertices
}

func (p advisorPoint) minus(q advisorPoint) advisorPoint {
	return advisorPoint{p[0] - q[0], p[1] - q[1], p[2] - q[2]}
}

func (p advisorPoint) dot(q advisorPoint) float64 {
	return float64(p[0]*q[0]) + float64(p[1]*q[1]) + float64(p[2]*q[2])
}

func (p advisorPoint) unit() (advisorPoint, error) {
	length := math.Sqrt(p.dot(p))
	if length <= 1e-6 || math.IsNaN(length) || math.IsInf(length, 0) {
		return advisorPoint{}, errors.New("invalid advisor facial axes")
	}
	return advisorPoint{p[0] / length, p[1] / length, p[2] / length}, nil
}

func (p advisorPoint) cross(q advisorPoint) advisorPoint {
	return advisorPoint{
		float64(p[1]*q[2]) - float64(p[2]*q[1]),
		float64(p[2]*q[0]) - float64(p[0]*q[2]),
		float64(p[0]*q[1]) - float64(p[1]*q[0]),
	}
}

func (art advisorArt) axes(vertices []advisorVertex) ([3]advisorPoint, error) {
	var landmarks [5]advisorPoint
	for i, anchor := range art.Anchors {
		landmarks[i] = advisorWeighted(anchor, func(point int) advisorPoint { return vertices[point].point })
	}
	x, err := landmarks[1].minus(landmarks[0]).unit()
	if err != nil {
		return [3]advisorPoint{}, err
	}
	v := landmarks[2].minus(landmarks[3])
	projection := v.dot(x)
	for i := range v {
		v[i] -= float64(projection * x[i])
	}
	y, err := v.unit()
	if err != nil {
		return [3]advisorPoint{}, err
	}
	z := x.cross(y)
	var center advisorPoint
	for _, vertex := range vertices {
		for i, value := range vertex.point {
			center[i] += value
		}
	}
	for i := range center {
		center[i] /= float64(len(vertices))
	}
	if z.dot(landmarks[4].minus(center)) < 0 {
		for i := range z {
			z[i] = -z[i]
		}
	}
	return [3]advisorPoint{x, y, z}, nil
}

func (vertex advisorVertex) move(offset advisorPoint, axes [3]advisorPoint) advisorVertex {
	for i, value := range vertex.point {
		axis := advisorPoint{axes[0][i], axes[1][i], axes[2][i]}
		vertex.point[i] = value + offset.dot(axis)
	}
	return vertex
}

func (art advisorArt) sculpt(source []advisorVertex, dynamic map[int]advisorPoint) ([]advisorVertex, error) {
	axes, err := art.axes(source)
	if err != nil {
		return nil, err
	}
	result := make([]advisorVertex, 0, len(source)+len(art.Edges))
	for i, vertex := range source {
		result = append(result, vertex.move(art.Deltas[i], axes))
	}
	for i, edge := range art.Edges {
		a, b := source[int(edge[0])], source[int(edge[1])]
		var middle advisorVertex
		for axis, value := range a.point {
			middle.point[axis] = (value + b.point[axis]) * .5
		}
		for axis, value := range a.uv {
			middle.uv[axis] = (value + b.uv[axis]) * .5
		}
		offset := advisorPoint{edge[2], edge[3], edge[4]}
		if applied, ok := dynamic[i]; ok {
			offset = applied
		}
		result = append(result, middle.move(offset, axes))
	}
	return result, nil
}

func appendAdvisorVertices(dst []byte, surface advisorSurface, verts []advisorVertex) []byte {
	dst = little.AppendUint16(dst, surface.ID)
	dst = little.AppendUint16(dst, uint16(len(verts)))
	dst = little.AppendUint32(dst, surface.Flags)
	for _, vertex := range verts {
		for _, v := range vertex.point {
			dst = little.AppendUint32(dst, math.Float32bits(float32(v)))
		}
		for _, v := range vertex.uv {
			dst = little.AppendUint32(dst, math.Float32bits(float32(v)))
		}
	}
	return dst
}

func (clip advisorClip) build(ctx context.Context, stock []byte) ([]byte, error) {
	if actual := sha256.Sum256(stock); hex.EncodeToString(actual[:]) != clip.Stock {
		return nil, errors.New("the original advisor resource differs from the supported Windows game")
	}
	if err := clip.validate(); err != nil {
		return nil, err
	}
	head, err := hex.DecodeString(clip.Head)
	if err != nil || len(head) != 20 || string(head[:4]) != "3DMD" {
		return nil, errors.New("invalid advisor model header")
	}
	tail, err := hex.DecodeString(clip.Tail)
	if err != nil || len(tail) < 12 || string(tail[:4]) != "INDX" {
		return nil, errors.New("invalid advisor model sections")
	}
	points, err := clip.readPositions(stock)
	if err != nil {
		return nil, err
	}
	poses := make([][][]advisorPoint, len(clip.Surfaces))
	for i, surface := range clip.Surfaces {
		poses[i] = surface.positions(points, clip.Frames)
	}
	result := append(head, "VERT\x00\x00\x00\x00"...)
	result = little.AppendUint32(result, uint32(len(clip.Groups)))
	for i, group := range clip.Groups {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if group[0] == -1 {
			start, size := group[1], group[2]
			if start < 32 || start > len(stock)-size {
				return nil, errors.New("unchanged advisor group lies outside the original model")
			}
			result = append(result, stock[start:start+size]...)
			continue
		}
		surface := clip.Surfaces[group[0]]
		vertices := surface.vertices(poses[group[0]], clip.Factor, group[1])
		if surface.Art != nil {
			vertices, err = surface.Art.sculpt(vertices, clip.Dynamic[i])
			if err != nil {
				return nil, err
			}
		}
		result = appendAdvisorVertices(result, surface, vertices)
	}
	little.PutUint32(result[24:], uint32(len(result)-20))
	return append(result, tail...), nil
}

func buildAdvisorsPackage(ctx context.Context, game, output string) error {
	recipe, err := openAdvisorsRecipe()
	if err != nil {
		return err
	}
	gameData, mac, err := gameDataDirectory(game)
	if err != nil {
		return err
	}
	stock, err := openArchive(filepath.Join(gameData, "SimCity_1.dat"))
	if err != nil {
		return err
	}
	defer stock.file.Close()
	known := make(map[uint32]bool)
	for _, instance := range advisorClipIDs() {
		known[instance] = true
	}
	entries := make([]resource, 0, len(known)+len(advisorFaceIDs))
	for _, entry := range stock.records {
		if entry.id.kind != s3dResource || entry.id.group != advisorsGroup || !known[entry.id.instance] {
			continue
		}
		if entry.size > advisorsClipLimit {
			return fmt.Errorf("%s: original advisor resource is too large", entry.id)
		}
		raw, err := stock.read(entry)
		if err != nil {
			return err
		}
		clip, err := recipe.clip(entry.id.instance)
		if err == nil {
			raw, err = clip.build(ctx, raw)
		}
		if err == nil && mac {
			err = visitModelTextures(raw, func(reference []byte) {
				if slices.Contains(advisorFaceIDs, little.Uint32(reference)) {
					reference[7] = 1 // Stock Aspyr advisor filter, without mip sampling.
				}
			})
		}
		if err != nil {
			return fmt.Errorf("build %s: %w", entry.id, err)
		}
		entries = append(entries, resource{entry.id, raw})
		delete(known, entry.id.instance)
	}
	if len(known) != 0 {
		return errors.New("the original game is missing advisor model resources")
	}
	for _, instance := range advisorFaceIDs {
		texture, err := recipe.texture(instance)
		if err != nil {
			return err
		}
		if mac {
			texture, err = advisorAspyrTexture(texture)
			if err != nil {
				return fmt.Errorf("advisor texture %08x: %w", instance, err)
			}
		}
		entries = append(entries, resource{resourceID{fshResource, advisorsFaceGroup, instance}, texture})
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return publishFile(output, func(writer io.Writer) error {
		return writeArchive(writer, entries)
	})
}

func runAdvisors(args []string) error {
	switch {
	case len(args) == 1 && args[0] == "verify":
		recipe, err := openAdvisorsRecipe()
		if err != nil {
			return err
		}
		return recipe.validate()
	case len(args) == 3 && args[0] == "build":
		return buildAdvisorsPackage(context.Background(), args[1], args[2])
	default:
		return errors.New("usage: advisors verify | build GAME OUTPUT.dat")
	}
}
