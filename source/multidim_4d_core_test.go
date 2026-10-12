package main

import (
	"math"
	"testing"
)

func TestTesseractCountsAndEdges(t *testing.T) {
	verts, edges, faces, cells := mdBuildTesseract()
	if len(verts) != 16 || len(edges) != 32 || len(faces) != 24 || len(cells) != 8 {
		t.Fatalf("unexpected tesseract counts: V=%d E=%d F=%d C=%d", len(verts), len(edges), len(faces), len(cells))
	}
	degree := make([]int, len(verts))
	for _, e := range edges {
		if e.a < 0 || e.a >= len(verts) || e.b < 0 || e.b >= len(verts) || e.a == e.b {
			t.Fatalf("invalid edge: %+v", e)
		}
		diff := 0
		a, b := e.a, e.b
		for axis := 0; axis < 4; axis++ {
			if ((a >> axis) & 1) != ((b >> axis) & 1) {
				diff++
			}
		}
		if diff != 1 {
			t.Fatalf("edge endpoints must differ in exactly one coordinate: %+v", e)
		}
		degree[e.a]++
		degree[e.b]++
	}
	for i, d := range degree {
		if d != 4 {
			t.Fatalf("vertex %d has degree %d, want 4", i, d)
		}
	}
}

func TestTesseractFacesAndCells(t *testing.T) {
	_, _, faces, cells := mdBuildTesseract()
	seenFaces := map[[4]int]bool{}
	for _, face := range faces {
		seen := map[int]bool{}
		for _, idx := range face.indices {
			if idx < 0 || idx >= 16 || seen[idx] {
				t.Fatalf("invalid square face: %+v", face)
			}
			seen[idx] = true
		}
		key := face.indices
		// Canonicalize just enough to detect accidental duplicate generated quads.
		minPos := 0
		for i := 1; i < 4; i++ {
			if key[i] < key[minPos] {
				minPos = i
			}
		}
		canon := [4]int{key[minPos], key[(minPos+1)%4], key[(minPos+2)%4], key[(minPos+3)%4]}
		rev := [4]int{key[minPos], key[(minPos+3)%4], key[(minPos+2)%4], key[(minPos+1)%4]}
		if rev[1] < canon[1] {
			canon = rev
		}
		if seenFaces[canon] {
			t.Fatalf("duplicate face: %+v", face)
		}
		seenFaces[canon] = true
	}
	for ci, cell := range cells {
		seen := map[int]bool{}
		for _, idx := range cell.vertices {
			seen[idx] = true
		}
		if len(seen) != 8 {
			t.Fatalf("cell %d does not contain 8 distinct vertices", ci)
		}
	}
}

func TestXWRotationPreservesFourDimensionalLength(t *testing.T) {
	v := mdVec4{x: 1.25, y: -2, z: 0.75, w: 3}
	want := mdLenSq4(v)
	for _, a := range []float64{0, 0.37, math.Pi / 2, math.Pi, 2 * math.Pi} {
		got := mdLenSq4(mdRot4XW(v, a))
		if math.Abs(got-want) > 1e-10 {
			t.Fatalf("length changed at angle %g: got %.12f want %.12f", a, got, want)
		}
	}
}

func TestWPerspectiveProducesNestedCubesAtZeroRotation(t *testing.T) {
	distance := 3.5
	nearX, _, _ := mdProject4To3Components(mdVec4{x: 1, w: 1}, distance)
	farX, _, _ := mdProject4To3Components(mdVec4{x: 1, w: -1}, distance)
	if !(nearX > farX && farX > 0) {
		t.Fatalf("expected +w cube larger than -w cube: near=%g far=%g", nearX, farX)
	}
	if math.Abs(nearX/(distance/(distance-1))-1) > 1e-12 {
		t.Fatalf("unexpected perspective scale: %g", nearX)
	}
}

func TestRegular5CellCountsAndRegularity(t *testing.T) {
	verts, edges, faces, cells := mdBuildRegular5Cell()
	if len(verts) != 5 || len(edges) != 10 || len(faces) != 10 || len(cells) != 5 {
		t.Fatalf("unexpected 5-cell counts: V=%d E=%d F=%d C=%d", len(verts), len(edges), len(faces), len(cells))
	}
	wantLenSq := -1.0
	degree := make([]int, 5)
	for _, e := range edges {
		d := mdVec4{x: verts[e.a].x - verts[e.b].x, y: verts[e.a].y - verts[e.b].y, z: verts[e.a].z - verts[e.b].z, w: verts[e.a].w - verts[e.b].w}
		got := mdLenSq4(d)
		if wantLenSq < 0 {
			wantLenSq = got
		}
		if math.Abs(got-wantLenSq) > 1e-10 {
			t.Fatalf("non-regular edge %+v: len^2=%g want %g", e, got, wantLenSq)
		}
		degree[e.a]++
		degree[e.b]++
	}
	for i, d := range degree {
		if d != 4 {
			t.Fatalf("vertex %d has degree %d, want 4", i, d)
		}
		if math.Abs(mdLenSq4(verts[i])-1) > 1e-10 {
			t.Fatalf("vertex %d is not on unit 3-sphere: %g", i, mdLenSq4(verts[i]))
		}
	}
	for i, face := range faces {
		if len(face.indices) != 3 {
			t.Fatalf("face %d has %d vertices, want 3", i, len(face.indices))
		}
	}
	for i, cell := range cells {
		if len(cell) != 4 {
			t.Fatalf("cell %d has %d vertices, want 4", i, len(cell))
		}
	}
}

func TestTesseractPolygonFaceConversion(t *testing.T) {
	_, _, quads, _ := mdBuildTesseract()
	faces := mdTesseractFacesAsPolygons(quads)
	if len(faces) != 24 {
		t.Fatalf("got %d polygon faces, want 24", len(faces))
	}
	for i, face := range faces {
		if len(face.indices) != 4 {
			t.Fatalf("face %d has %d vertices, want 4", i, len(face.indices))
		}
	}
}

func TestDepthFadeFactorIsThirtyPercentCompoundWithEightPercentFloor(t *testing.T) {
	wants := []float64{
		1,
		0.70,
		0.49,
		0.343,
		0.2401,
		0.16807,
		0.117649,
		0.0823543,
		0.08,
		0.08,
	}
	for layers, want := range wants {
		got := mdDepthFadeFactor(layers)
		if math.Abs(got-want) > 1e-12 {
			t.Fatalf("layers=%d: got %.12f want %.12f", layers, got, want)
		}
	}
	if got := mdDepthFadeFactor(-1); got != 1 {
		t.Fatalf("negative layers: got %.12f want 1", got)
	}
}

func TestRegular16CellCountsRegularityAndIncidence(t *testing.T) {
	verts, edges, faces, cells := mdBuildRegular16Cell()
	if len(verts) != 8 || len(edges) != 24 || len(faces) != 32 || len(cells) != 16 {
		t.Fatalf("unexpected 16-cell counts: V=%d E=%d F=%d C=%d", len(verts), len(edges), len(faces), len(cells))
	}
	degree := make([]int, len(verts))
	wantLenSq := -1.0
	edgeSet := map[[2]int]bool{}
	for _, e := range edges {
		if e.a < 0 || e.a >= len(verts) || e.b < 0 || e.b >= len(verts) || e.a == e.b {
			t.Fatalf("invalid edge: %+v", e)
		}
		a, b := e.a, e.b
		if a > b {
			a, b = b, a
		}
		if edgeSet[[2]int{a, b}] {
			t.Fatalf("duplicate edge: %+v", e)
		}
		edgeSet[[2]int{a, b}] = true
		d := mdVec4{x: verts[e.a].x - verts[e.b].x, y: verts[e.a].y - verts[e.b].y, z: verts[e.a].z - verts[e.b].z, w: verts[e.a].w - verts[e.b].w}
		got := mdLenSq4(d)
		if wantLenSq < 0 {
			wantLenSq = got
		}
		if math.Abs(got-wantLenSq) > 1e-12 {
			t.Fatalf("non-regular edge %+v: len^2=%g want %g", e, got, wantLenSq)
		}
		degree[e.a]++
		degree[e.b]++
	}
	for i, d := range degree {
		if d != 6 {
			t.Fatalf("vertex %d degree=%d want 6", i, d)
		}
		if math.Abs(mdLenSq4(verts[i])-1) > 1e-12 {
			t.Fatalf("vertex %d not on unit 3-sphere: %g", i, mdLenSq4(verts[i]))
		}
	}
	faceSet := map[[3]int]bool{}
	for i, face := range faces {
		if len(face.indices) != 3 {
			t.Fatalf("face %d vertices=%d want 3", i, len(face.indices))
		}
		key := [3]int{face.indices[0], face.indices[1], face.indices[2]}
		for a := 0; a < 3; a++ {
			for b := a + 1; b < 3; b++ {
				if key[b] < key[a] {
					key[a], key[b] = key[b], key[a]
				}
			}
		}
		if faceSet[key] {
			t.Fatalf("duplicate face %v", key)
		}
		faceSet[key] = true
	}
	incidence := map[[3]int]int{}
	for i, cell := range cells {
		if len(cell) != 4 {
			t.Fatalf("cell %d vertices=%d want 4", i, len(cell))
		}
		for omitted := 0; omitted < 4; omitted++ {
			tri := make([]int, 0, 3)
			for j, v := range cell {
				if j != omitted {
					tri = append(tri, v)
				}
			}
			key := [3]int{tri[0], tri[1], tri[2]}
			for a := 0; a < 3; a++ {
				for b := a + 1; b < 3; b++ {
					if key[b] < key[a] {
						key[a], key[b] = key[b], key[a]
					}
				}
			}
			incidence[key]++
		}
	}
	if len(incidence) != 32 {
		t.Fatalf("cell boundary faces=%d want 32", len(incidence))
	}
	for face, count := range incidence {
		if count != 2 || !faceSet[face] {
			t.Fatalf("face %v incidence=%d present=%v", face, count, faceSet[face])
		}
	}
}

func TestRegular24CellCountsRegularityAndCells(t *testing.T) {
	verts, edges, faces, cells := mdBuildRegular24Cell()
	if len(verts) != 24 || len(edges) != 96 || len(faces) != 96 || len(cells) != 24 {
		t.Fatalf("unexpected 24-cell counts: V=%d E=%d F=%d C=%d", len(verts), len(edges), len(faces), len(cells))
	}
	degree := make([]int, len(verts))
	edgeSet := map[[2]int]bool{}
	wantLenSq := -1.0
	for _, e := range edges {
		a, b := e.a, e.b
		if a > b {
			a, b = b, a
		}
		key := [2]int{a, b}
		if edgeSet[key] {
			t.Fatalf("duplicate edge %v", key)
		}
		edgeSet[key] = true
		d := mdVec4{x: verts[a].x - verts[b].x, y: verts[a].y - verts[b].y, z: verts[a].z - verts[b].z, w: verts[a].w - verts[b].w}
		got := mdLenSq4(d)
		if wantLenSq < 0 {
			wantLenSq = got
		}
		if math.Abs(got-wantLenSq) > 1e-12 {
			t.Fatalf("nonregular edge %v len^2=%g want %g", key, got, wantLenSq)
		}
		degree[a]++
		degree[b]++
	}
	for i, d := range degree {
		if d != 8 {
			t.Fatalf("vertex %d degree=%d want 8", i, d)
		}
		if math.Abs(mdLenSq4(verts[i])-1) > 1e-12 {
			t.Fatalf("vertex %d not on unit 3-sphere", i)
		}
	}
	faceSet := map[[3]int]bool{}
	for i, face := range faces {
		if len(face.indices) != 3 {
			t.Fatalf("face %d vertices=%d", i, len(face.indices))
		}
		key := [3]int{face.indices[0], face.indices[1], face.indices[2]}
		for a := 0; a < 3; a++ {
			for b := a + 1; b < 3; b++ {
				if key[b] < key[a] {
					key[a], key[b] = key[b], key[a]
				}
			}
		}
		if faceSet[key] {
			t.Fatalf("duplicate face %v", key)
		}
		faceSet[key] = true
		for a := 0; a < 3; a++ {
			for b := a + 1; b < 3; b++ {
				x, y := key[a], key[b]
				if x > y {
					x, y = y, x
				}
				if !edgeSet[[2]int{x, y}] {
					t.Fatalf("face %v contains non-edge %d-%d", key, x, y)
				}
			}
		}
	}
	faceIncidence := map[[3]int]int{}
	for ci, cell := range cells {
		if len(cell) != 6 {
			t.Fatalf("cell %d vertices=%d want 6", ci, len(cell))
		}
		internalEdges := 0
		for i := 0; i < len(cell); i++ {
			for j := i + 1; j < len(cell); j++ {
				a, b := cell[i], cell[j]
				if a > b {
					a, b = b, a
				}
				if edgeSet[[2]int{a, b}] {
					internalEdges++
				}
			}
		}
		if internalEdges != 12 {
			t.Fatalf("cell %d induced edges=%d want octahedron 12", ci, internalEdges)
		}
		for _, face := range faces {
			inside := 0
			for _, v := range face.indices {
				for _, cv := range cell {
					if v == cv {
						inside++
						break
					}
				}
			}
			if inside == 3 {
				key := [3]int{face.indices[0], face.indices[1], face.indices[2]}
				for a := 0; a < 3; a++ {
					for b := a + 1; b < 3; b++ {
						if key[b] < key[a] {
							key[a], key[b] = key[b], key[a]
						}
					}
				}
				faceIncidence[key]++
			}
		}
	}
	for face, count := range faceIncidence {
		if count != 2 || !faceSet[face] {
			t.Fatalf("face %v cell incidence=%d present=%v", face, count, faceSet[face])
		}
	}
	if len(faceIncidence) != 96 {
		t.Fatalf("cell boundary faces=%d want 96", len(faceIncidence))
	}
}

func TestRegular120CellCountsRegularityAndIncidence(t *testing.T) {
	verts, edges, faces, cells := mdBuildRegular120Cell()
	if len(verts) != 600 || len(edges) != 1200 || len(faces) != 720 || len(cells) != 120 {
		t.Fatalf("unexpected 120-cell counts: V=%d E=%d F=%d C=%d", len(verts), len(edges), len(faces), len(cells))
	}
	degree := make([]int, len(verts))
	edgeSet := make(map[[2]int]bool, len(edges))
	wantLenSq := -1.0
	for _, e := range edges {
		key := mdSortedPair(e.a, e.b)
		if key[0] < 0 || key[1] >= len(verts) || key[0] == key[1] || edgeSet[key] {
			t.Fatalf("invalid or duplicate edge %v", key)
		}
		edgeSet[key] = true
		d := mdVec4{x: verts[key[0]].x - verts[key[1]].x, y: verts[key[0]].y - verts[key[1]].y, z: verts[key[0]].z - verts[key[1]].z, w: verts[key[0]].w - verts[key[1]].w}
		got := mdLenSq4(d)
		if wantLenSq < 0 {
			wantLenSq = got
		}
		if math.Abs(got-wantLenSq) > 1e-9 {
			t.Fatalf("nonregular edge %v len^2=%g want %g", key, got, wantLenSq)
		}
		degree[key[0]]++
		degree[key[1]]++
	}
	for i, d := range degree {
		if d != 4 {
			t.Fatalf("vertex %d degree=%d want 4", i, d)
		}
		if math.Abs(mdLenSq4(verts[i])-1) > 1e-9 {
			t.Fatalf("vertex %d not on unit 3-sphere", i)
		}
	}
	faceSet := make(map[[5]int]bool, len(faces))
	edgeFaceIncidence := make(map[[2]int]int, len(edges))
	for i, face := range faces {
		if len(face.indices) != 5 {
			t.Fatalf("face %d vertices=%d want 5", i, len(face.indices))
		}
		var sorted [5]int
		copy(sorted[:], face.indices)
		for a := 0; a < 5; a++ {
			for b := a + 1; b < 5; b++ {
				if sorted[b] < sorted[a] {
					sorted[a], sorted[b] = sorted[b], sorted[a]
				}
			}
		}
		if faceSet[sorted] {
			t.Fatalf("duplicate pentagonal face %v", sorted)
		}
		faceSet[sorted] = true
		for j := 0; j < 5; j++ {
			key := mdSortedPair(face.indices[j], face.indices[(j+1)%5])
			if !edgeSet[key] {
				t.Fatalf("face %d contains non-edge %v", i, key)
			}
			edgeFaceIncidence[key]++
		}
	}
	for edge := range edgeSet {
		if edgeFaceIncidence[edge] != 3 {
			t.Fatalf("edge %v face incidence=%d want 3", edge, edgeFaceIncidence[edge])
		}
	}
	for ci, cell := range cells {
		if len(cell) != 20 {
			t.Fatalf("cell %d vertices=%d want dodecahedron 20", ci, len(cell))
		}
		internalEdges := 0
		for i := 0; i < len(cell); i++ {
			for j := i + 1; j < len(cell); j++ {
				if edgeSet[mdSortedPair(cell[i], cell[j])] {
					internalEdges++
				}
			}
		}
		if internalEdges != 30 {
			t.Fatalf("cell %d induced edges=%d want dodecahedron 30", ci, internalEdges)
		}
	}
}

func TestRegular600CellCountsRegularityAndIncidence(t *testing.T) {
	verts, edges, faces, cells := mdBuildRegular600Cell()
	if len(verts) != 120 || len(edges) != 720 || len(faces) != 1200 || len(cells) != 600 {
		t.Fatalf("600-cell counts = %d vertices, %d edges, %d faces, %d cells", len(verts), len(edges), len(faces), len(cells))
	}

	degree := make([]int, len(verts))
	edgeSet := make(map[[2]int]bool, len(edges))
	edgeLenSq := -1.0
	for _, e := range edges {
		if e.a < 0 || e.a >= len(verts) || e.b < 0 || e.b >= len(verts) || e.a == e.b {
			t.Fatalf("invalid edge %+v", e)
		}
		key := mdSortedPair(e.a, e.b)
		if edgeSet[key] {
			t.Fatalf("duplicate edge %+v", e)
		}
		edgeSet[key] = true
		degree[e.a]++
		degree[e.b]++
		d := mdVec4{x: verts[e.a].x - verts[e.b].x, y: verts[e.a].y - verts[e.b].y, z: verts[e.a].z - verts[e.b].z, w: verts[e.a].w - verts[e.b].w}
		l2 := mdLenSq4(d)
		if edgeLenSq < 0 {
			edgeLenSq = l2
		} else if math.Abs(l2-edgeLenSq) > 1e-9 {
			t.Fatalf("non-regular edge length: got %.12g want %.12g", l2, edgeLenSq)
		}
	}
	for i, d := range degree {
		if d != 12 {
			t.Fatalf("vertex %d degree=%d, want 12", i, d)
		}
	}

	faceSet := make(map[[3]int]bool, len(faces))
	edgeFaceCount := make(map[[2]int]int, len(edges))
	for _, f := range faces {
		if len(f.indices) != 3 {
			t.Fatalf("non-triangular face: %v", f.indices)
		}
		key := mdSortedTriple(f.indices[0], f.indices[1], f.indices[2])
		if faceSet[key] {
			t.Fatalf("duplicate face %v", key)
		}
		faceSet[key] = true
		pairs := [][2]int{{key[0], key[1]}, {key[0], key[2]}, {key[1], key[2]}}
		for _, pair := range pairs {
			ek := mdSortedPair(pair[0], pair[1])
			if !edgeSet[ek] {
				t.Fatalf("face uses non-edge %v", ek)
			}
			edgeFaceCount[ek]++
		}
	}
	for edge := range edgeSet {
		if edgeFaceCount[edge] != 5 {
			t.Fatalf("edge %v belongs to %d triangular faces, want 5", edge, edgeFaceCount[edge])
		}
	}

	faceCellCount := make(map[[3]int]int, len(faces))
	vertexCellCount := make([]int, len(verts))
	for ci, cell := range cells {
		if len(cell) != 4 {
			t.Fatalf("cell %d has %d vertices, want 4", ci, len(cell))
		}
		for _, v := range cell {
			vertexCellCount[v]++
		}
		for omitted := 0; omitted < 4; omitted++ {
			tri := make([]int, 0, 3)
			for i, v := range cell {
				if i != omitted {
					tri = append(tri, v)
				}
			}
			faceCellCount[mdSortedTriple(tri[0], tri[1], tri[2])]++
		}
	}
	for face := range faceSet {
		if faceCellCount[face] != 2 {
			t.Fatalf("face %v belongs to %d cells, want 2", face, faceCellCount[face])
		}
	}
	for i, count := range vertexCellCount {
		if count != 20 {
			t.Fatalf("vertex %d belongs to %d cells, want 20", i, count)
		}
	}
}

func TestKleinBottle4DMeshAndTwistedSeam(t *testing.T) {
	const uSeg, vSeg = 24, 16
	verts, faces, uEdges, vEdges := mdBuildKleinBottle4D(uSeg, vSeg, 2.0, 0.65)
	if len(verts) != uSeg*vSeg {
		t.Fatalf("vertices=%d want %d", len(verts), uSeg*vSeg)
	}
	if len(faces) != uSeg*vSeg {
		t.Fatalf("faces=%d want %d", len(faces), uSeg*vSeg)
	}
	if len(uEdges) != uSeg*vSeg || len(vEdges) != uSeg*vSeg {
		t.Fatalf("grid edges u=%d v=%d want %d each", len(uEdges), len(vEdges), uSeg*vSeg)
	}

	edgeSet := map[[2]int]bool{}
	for _, f := range faces {
		if len(f.indices) != 4 {
			t.Fatalf("non-quad face: %v", f.indices)
		}
		for i := 0; i < 4; i++ {
			a, b := f.indices[i], f.indices[(i+1)%4]
			if a > b {
				a, b = b, a
			}
			edgeSet[[2]int{a, b}] = true
		}
	}
	// A closed quadrilateral Klein mesh has E=2UV and χ=V-E+F=0.
	wantEdges := 2 * uSeg * vSeg
	if len(edgeSet) != wantEdges {
		t.Fatalf("unique edges=%d want %d", len(edgeSet), wantEdges)
	}
	if chi := len(verts) - len(edgeSet) + len(faces); chi != 0 {
		t.Fatalf("Euler characteristic=%d want 0", chi)
	}

	for i := 0; i <= 12; i++ {
		v := 2 * math.Pi * float64(i) / 12
		a := mdKleinPoint4(2*math.Pi, v, 2.0, 0.65)
		b := mdKleinPoint4(0, math.Pi-v, 2.0, 0.65)
		d := mdVec4{x: a.x - b.x, y: a.y - b.y, z: a.z - b.z, w: a.w - b.w}
		if mdLenSq4(d) > 1e-20 {
			t.Fatalf("twisted seam mismatch at v=%g: %+v vs %+v", v, a, b)
		}
	}

	// Regression for the classical bottle neck crossing.  The two sampled
	// points nearly coincide in the visible XYZ immersion but are widely
	// separated in W, proving that the fourth coordinate lifts the crossing.
	a := mdKleinPoint4(3.665191429188092, math.Pi, 2.0, 0.65)
	b := mdKleinPoint4(5.759586531581288, 0, 2.0, 0.65)
	dXYZ := mdVec3{x: a.x - b.x, y: a.y - b.y, z: a.z - b.z}
	dXYZ2 := dXYZ.x*dXYZ.x + dXYZ.y*dXYZ.y + dXYZ.z*dXYZ.z
	if dXYZ2 > 0.01 {
		t.Fatalf("classical neck samples are not a visible crossing: dXYZ²=%g", dXYZ2)
	}
	if math.Abs(a.w-b.w) < 1.0 {
		t.Fatalf("fourth-dimensional lift too small: |dw|=%g", math.Abs(a.w-b.w))
	}
}

func TestCenterAndScale4DBoundsKleinProjection(t *testing.T) {
	verts, _, _, _ := mdBuildKleinBottle4D(72, 36, 1.8, 0.65)
	normed := mdCenterAndScale4D(verts, 2.4)
	if len(normed) != len(verts) {
		t.Fatalf("normalized vertex count=%d want %d", len(normed), len(verts))
	}
	center := mdVec4{}
	maxR := 0.0
	for _, v := range normed {
		center.x += v.x
		center.y += v.y
		center.z += v.z
		center.w += v.w
		r := math.Sqrt(mdLenSq4(v))
		if r > maxR {
			maxR = r
		}
	}
	inv := 1.0 / float64(len(normed))
	center.x *= inv
	center.y *= inv
	center.z *= inv
	center.w *= inv
	if math.Sqrt(mdLenSq4(center)) > 1e-12 {
		t.Fatalf("normalized centroid is not zero: %+v", center)
	}
	if math.Abs(maxR-2.4) > 1e-9 {
		t.Fatalf("max radius=%g want 2.4", maxR)
	}

	// A full XW rotation must stay comfortably in front of the W camera plane.
	for step := 0; step <= 96; step++ {
		a := 2 * math.Pi * float64(step) / 96
		for _, v := range normed {
			r := mdRot4XW(v, a)
			denom := 6.0 - r.w
			if denom < 3.59 {
				t.Fatalf("unsafe W denominator=%g at angle=%g point=%+v", denom, a, r)
			}
			x, y, z := mdProject4To3Components(r, 6.0)
			if math.IsNaN(x) || math.IsNaN(y) || math.IsNaN(z) || math.IsInf(x, 0) || math.IsInf(y, 0) || math.IsInf(z, 0) {
				t.Fatalf("non-finite projection at angle=%g point=%+v", a, r)
			}
		}
	}
}

func TestOpaqueDepthPass(t *testing.T) {
	if !mdOpaqueDepthPass(1.0, 1.0, 0) {
		t.Fatal("edge exactly on a face should pass")
	}
	if !mdOpaqueDepthPass(0.99, 1.0, 0.02) {
		t.Fatal("small raster difference within bias should pass")
	}
	if mdOpaqueDepthPass(0.7, 1.0, 0.02) {
		t.Fatal("edge clearly behind a face should be rejected")
	}
	if !mdOpaqueDepthPass(1.2, 1.0, 0.02) {
		t.Fatal("edge in front of a face should pass")
	}
}

func TestPrepareKleinTurnAmplifiesVisibleSheetExchange(t *testing.T) {
	const uSeg, vSeg = 72, 36
	verts, _, _, _ := mdBuildKleinBottle4D(uSeg, vSeg, 2.0, 0.65)
	prepared := mdPrepareKleinTurn4D(verts, 2.35, 1.60)
	if len(prepared) != len(verts) {
		t.Fatalf("prepared vertex count=%d want %d", len(prepared), len(verts))
	}

	center := mdVec4{}
	maxSpatial, maxAbsW := 0.0, 0.0
	for _, v := range prepared {
		center.x += v.x
		center.y += v.y
		center.z += v.z
		center.w += v.w
		spatial := math.Sqrt(v.x*v.x + v.y*v.y + v.z*v.z)
		if spatial > maxSpatial {
			maxSpatial = spatial
		}
		if aw := math.Abs(v.w); aw > maxAbsW {
			maxAbsW = aw
		}
	}
	inv := 1.0 / float64(len(prepared))
	center.x *= inv
	center.y *= inv
	center.z *= inv
	center.w *= inv
	if math.Sqrt(mdLenSq4(center)) > 1e-12 {
		t.Fatalf("prepared centroid is not zero: %+v", center)
	}
	if math.Abs(maxSpatial-2.35) > 1e-9 {
		t.Fatalf("max spatial radius=%g want 2.35", maxSpatial)
	}
	if math.Abs(maxAbsW-1.60) > 1e-9 {
		t.Fatalf("max |W|=%g want 1.60", maxAbsW)
	}

	// These grid samples are the same neck/body crossing used by the raw
	// parameter regression: u=7pi/6,v=pi and u=11pi/6,v=0.
	idxA := 42*vSeg + 18
	idxB := 66*vSeg + 0
	a0, b0 := prepared[idxA], prepared[idxB]
	xa0, ya0, za0 := mdProject4To3Orthographic(a0)
	xb0, yb0, zb0 := mdProject4To3Orthographic(b0)
	d0 := math.Sqrt((xa0-xb0)*(xa0-xb0) + (ya0-yb0)*(ya0-yb0) + (za0-zb0)*(za0-zb0))
	if d0 > 0.01 {
		t.Fatalf("W=0 should preserve the classical visible crossing, distance=%g", d0)
	}

	// A quarter XW turn rotates the strengthened W lift directly into visible
	// X. The same two sheets must now be clearly separated, which is the visual
	// inside/outside exchange A160 is intended to expose.
	a1 := mdRot4XW(a0, math.Pi/2)
	b1 := mdRot4XW(b0, math.Pi/2)
	xa1, ya1, za1 := mdProject4To3Orthographic(a1)
	xb1, yb1, zb1 := mdProject4To3Orthographic(b1)
	d1 := math.Sqrt((xa1-xb1)*(xa1-xb1) + (ya1-yb1)*(ya1-yb1) + (za1-zb1)*(za1-zb1))
	if d1 < 1.5 {
		t.Fatalf("quarter W turn does not visibly separate crossing sheets: distance=%g", d1)
	}

	for step := 0; step <= 128; step++ {
		a := 2 * math.Pi * float64(step) / 128
		for _, v := range prepared {
			r := mdRot4XW(v, a)
			x, y, z := mdProject4To3Orthographic(r)
			if math.IsNaN(x) || math.IsNaN(y) || math.IsNaN(z) || math.IsInf(x, 0) || math.IsInf(y, 0) || math.IsInf(z, 0) {
				t.Fatalf("non-finite orthographic Klein projection at angle=%g point=%+v", a, r)
			}
		}
	}
}

func TestProjectedExteriorEdgeFlagsXZMarksHullNotDiagonal(t *testing.T) {
	verts := []mdVec3{
		{x: -1, z: -1},
		{x: 1, z: -1},
		{x: 1, z: 1},
		{x: -1, z: 1},
		{x: 0, z: -1}, // collinear split point on the bottom hull side
	}
	edges := []mdEdge4{
		{a: 0, b: 4}, {a: 4, b: 1},
		{a: 1, b: 2}, {a: 2, b: 3}, {a: 3, b: 0},
		{a: 0, b: 2}, // diagonal, never an exterior edge
	}
	flags := mdProjectedExteriorEdgeFlagsXZ(verts, edges)
	if len(flags) != len(edges) {
		t.Fatalf("flags=%d want %d", len(flags), len(edges))
	}
	for i := 0; i < 5; i++ {
		if !flags[i] {
			t.Fatalf("hull edge %d was not marked exterior", i)
		}
	}
	if flags[5] {
		t.Fatal("diagonal was incorrectly marked exterior")
	}
}

func Test4DEdgeVisibleByCoverCount(t *testing.T) {
	if !md4EdgeVisibleByCoverCount(0, false) {
		t.Fatal("zero-cover non-exterior edge must be visible")
	}
	if md4EdgeVisibleByCoverCount(1, false) {
		t.Fatal("covered non-exterior edge must be hidden in opaque mode")
	}
	if !md4EdgeVisibleByCoverCount(9, true) {
		t.Fatal("outer contour must remain visible")
	}
}

func TestMovedPivotInverseAndWRotationStayFixedInProjection(t *testing.T) {
	ax, ay, az, aw := 0.44, 0.73, 0.28, 0.61
	origin := mdVec3{x: 0.31, y: 0, z: -0.18}
	currentOffset := mdVec3{x: 0.06, y: -0.02, z: 0.05}
	target := mdVec3{x: origin.x - currentOffset.x, y: origin.y - currentOffset.y, z: origin.z - currentOffset.z}
	invXYZ := mdMat3ApplyTranspose(mdEulerRotationMatrix(ax, ay, az), target)

	// Same local-W=0 inverse used by the Windows editor for regular 4D shapes.
	c, sn := math.Cos(aw), math.Sin(aw)
	denom := 4*c + invXYZ.x*sn
	lx := 4 * invXYZ.x / denom
	k := 4 / (4 - lx*sn)
	localPivot := mdVec4{x: lx, y: invXYZ.y / k, z: invXYZ.z / k}

	project := func(wAngle float64) mdVec3 {
		p := mdRot4XW(localPivot, wAngle)
		x, y, z := mdProject4To3Components(p, 4.0)
		return mdMat3Apply(mdEulerRotationMatrix(ax, ay, az), mdVec3{x: x, y: y, z: z})
	}
	baseProjected := project(aw)
	if math.Abs(baseProjected.x-target.x) > 1e-10 || math.Abs(baseProjected.y-target.y) > 1e-10 || math.Abs(baseProjected.z-target.z) > 1e-10 {
		t.Fatalf("4D inverse did not reproduce rebased target: got=%+v want=%+v", baseProjected, target)
	}
	baseOffset := mdVec3{x: origin.x - baseProjected.x, y: origin.y - baseProjected.y, z: origin.z - baseProjected.z}
	if math.Abs(baseOffset.x-currentOffset.x) > 1e-10 || math.Abs(baseOffset.y-currentOffset.y) > 1e-10 || math.Abs(baseOffset.z-currentOffset.z) > 1e-10 {
		t.Fatalf("4D rebase translation mismatch: got=%+v want=%+v", baseOffset, currentOffset)
	}

	projectedAfterW := project(aw + 0.83)
	offsetAfterW := mdVec3{x: origin.x - projectedAfterW.x, y: origin.y - projectedAfterW.y, z: origin.z - projectedAfterW.z}
	fixed := mdVec3{x: projectedAfterW.x + offsetAfterW.x, y: projectedAfterW.y + offsetAfterW.y, z: projectedAfterW.z + offsetAfterW.z}
	if math.Abs(fixed.x-origin.x) > 1e-12 || math.Abs(fixed.y-origin.y) > 1e-12 || math.Abs(fixed.z-origin.z) > 1e-12 {
		t.Fatalf("W/XW rotation did not keep moved pivot fixed: got=%+v want=%+v", fixed, origin)
	}
}

func TestKleinWTurnIsTrueDoubleRotation(t *testing.T) {
	v := mdVec4{x: 1.2, y: 0.4, z: -0.7, w: 0.9}
	r := mdRot4KleinW(v, math.Pi/2)
	want := mdVec4{x: -v.w, y: -v.z, z: v.y, w: v.x}
	if math.Abs(r.x-want.x) > 1e-12 || math.Abs(r.y-want.y) > 1e-12 || math.Abs(r.z-want.z) > 1e-12 || math.Abs(r.w-want.w) > 1e-12 {
		t.Fatalf("quarter isoclinic turn=%+v want %+v", r, want)
	}
	if math.Abs(mdLenSq4(r)-mdLenSq4(v)) > 1e-12 {
		t.Fatalf("double rotation changed 4D length: got=%g want=%g", mdLenSq4(r), mdLenSq4(v))
	}
	full := mdRot4KleinW(v, 2*math.Pi)
	if math.Abs(full.x-v.x) > 1e-12 || math.Abs(full.y-v.y) > 1e-12 || math.Abs(full.z-v.z) > 1e-12 || math.Abs(full.w-v.w) > 1e-12 {
		t.Fatalf("full double turn did not return to start: got=%+v want=%+v", full, v)
	}
}

func TestKleinProjectionDenominatorFloorA187(t *testing.T) {
	if got := mdKleinProjectionDenominator(4.0, math.Pi/2); math.Abs(got-mdKleinProjectionMinDenom) > 1e-12 {
		t.Fatalf("Klein denominator floor=%g want %g", got, mdKleinProjectionMinDenom)
	}
	if got := mdKleinProjectionDenominator(0, math.Pi/2); math.Abs(got-mdKleinProjectionDistance) > 1e-12 {
		t.Fatalf("zero-W denominator=%g want distance %g", got, mdKleinProjectionDistance)
	}
	if gain := mdKleinProjectionDistance / mdKleinProjectionMinDenom; gain < 2.8 || gain > 3.0 {
		t.Fatalf("unexpected maximum Klein perspective gain=%g", gain)
	}
}

func TestKleinProjectionBaselineAndFiniteTurn(t *testing.T) {
	v := mdVec4{x: 1.2, y: 0.4, z: -0.7, w: 0.9}
	base := mdProjectKlein4To3(v, 0)
	if math.Abs(base.x-v.x) > 1e-12 || math.Abs(base.y-v.y) > 1e-12 || math.Abs(base.z-v.z) > 1e-12 {
		t.Fatalf("zero W angle must preserve classical XYZ: got=%+v want=%+v", base, mdVec3{x: v.x, y: v.y, z: v.z})
	}

	verts, _, _, _ := mdBuildKleinBottle4D(72, 36, 2.0, 0.65)
	verts = mdPrepareKleinTurn4D(verts, 2.35, 1.60)
	for step := 0; step <= 128; step++ {
		a := 2 * math.Pi * float64(step) / 128
		for _, p := range verts {
			q := mdProjectKlein4To3(mdRot4KleinW(p, a), a)
			if math.IsNaN(q.x) || math.IsNaN(q.y) || math.IsNaN(q.z) || math.IsInf(q.x, 0) || math.IsInf(q.y, 0) || math.IsInf(q.z, 0) {
				t.Fatalf("non-finite A187 Klein projection at angle=%g point=%+v", a, p)
			}
		}
	}
}

func TestKleinMovedPivotInverseAtQuarterTurn(t *testing.T) {
	for _, a := range []float64{0, 0.73, math.Pi / 2, math.Pi, 5.21} {
		target := mdVec3{x: 0.31, y: -0.12, z: 0.19}
		postTurn := mdVec4{x: target.x, y: target.y, z: target.z, w: 0}
		local := mdRot4KleinW(postTurn, -a)
		projected := mdProjectKlein4To3(mdRot4KleinW(local, a), a)
		if math.Abs(projected.x-target.x) > 1e-11 || math.Abs(projected.y-target.y) > 1e-11 || math.Abs(projected.z-target.z) > 1e-11 {
			t.Fatalf("Klein pivot inverse failed at angle=%g: got=%+v want=%+v", a, projected, target)
		}
	}
}

func TestKleinSubdivisionClampA212(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, mdKleinSubdivisionMin},
		{11, mdKleinSubdivisionMin},
		{12, 12},
		{24, 24},
		{30, 30},
		{31, mdKleinSubdivisionMax},
		{48, mdKleinSubdivisionMax},
	}
	for _, tc := range cases {
		if got := mdClampKleinSubdivision(tc.in); got != tc.want {
			t.Fatalf("mdClampKleinSubdivision(%d)=%d want %d", tc.in, got, tc.want)
		}
	}
}
