package main

import "math"

const (
	fieldSpecialLayoutNone = iota
	fieldSpecialLayoutTetrahedron
	fieldSpecialLayoutOctahedron
	fieldSpecialLayoutCube
)

func fieldSpecialLayoutName(layout int) string {
	switch layout {
	case fieldSpecialLayoutTetrahedron:
		return "정사면체"
	case fieldSpecialLayoutOctahedron:
		return "정팔면체"
	case fieldSpecialLayoutCube:
		return "정육면체"
	}
	return "없음"
}

// fieldPointChargePolyhedron creates true volumetric point-charge layouts.
// Manual drag still edits X/Z while preserving each generated Y coordinate.
func fieldPointChargePolyhedron(layout int) []fieldObject {
	var vertices []fieldVec3
	switch layout {
	case fieldSpecialLayoutTetrahedron:
		vertices = []fieldVec3{{1, 1, 1}, {1, -1, -1}, {-1, 1, -1}, {-1, -1, 1}}
	case fieldSpecialLayoutOctahedron:
		vertices = []fieldVec3{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
	case fieldSpecialLayoutCube:
		for _, x := range []float64{-1, 1} {
			for _, y := range []float64{-1, 1} {
				for _, z := range []float64{-1, 1} {
					vertices = append(vertices, fieldVec3{x, y, z})
				}
			}
		}
	default:
		return nil
	}
	circumRadius := 1.38
	out := make([]fieldObject, 0, len(vertices))
	for _, v := range vertices {
		n := v.norm()
		if n.len2() < 1e-12 {
			continue
		}
		o := fieldDefaultObject(fieldObjectSphere)
		o.X, o.Y, o.Z = n.X*circumRadius, n.Y*circumRadius, n.Z*circumRadius
		o.Radius = .27
		o.Charge = 1
		o.Material = fieldMaterialInsulator
		o.Volumetric = true
		out = append(out, o)
	}
	return out
}

func fieldPolyhedronEdgeLength(layout int) float64 {
	// Useful in tests and documentation.
	r := 1.38
	switch layout {
	case fieldSpecialLayoutTetrahedron:
		return 2 * r * math.Sqrt(2.0/3.0)
	case fieldSpecialLayoutOctahedron:
		return r * math.Sqrt2
	case fieldSpecialLayoutCube:
		return 2 * r / math.Sqrt(3)
	}
	return 0
}
