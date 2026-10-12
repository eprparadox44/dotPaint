package main

import (
	"math"
	"sort"
)

// mdVec4 is a point or vector in four-dimensional Euclidean space.
// The first 4D implementation deliberately keeps this type independent of
// Win32 so its geometry can be unit-tested on every build host.
type mdVec4 struct {
	x, y, z, w float64
}

type mdEdge4 struct {
	a, b int
}

type mdQuad4 struct {
	indices [4]int
}

// mdFace4 is a polygonal 2-face of a four-dimensional polytope.
// The first implementation uses triangles (5-cell) and quads (tesseract).
type mdFace4 struct {
	indices []int
}

// mdProjectedExteriorEdgeFlagsXZ marks edges that lie on the outer contour of
// the current front view. The 4D renderer projects to 3D first and then uses an
// orthographic XZ camera plane, so the visible outer contour is the convex hull
// of the projected vertices. Collinear hull vertices are handled by testing an
// entire source edge against every hull side rather than requiring its two
// endpoints to be consecutive hull corners.
func mdProjectedExteriorEdgeFlagsXZ(verts []mdVec3, edges []mdEdge4) []bool {
	flags := make([]bool, len(edges))
	if len(verts) < 2 || len(edges) == 0 {
		return flags
	}
	type point struct {
		x, z float64
	}
	pts := make([]point, len(verts))
	minX, maxX := math.Inf(1), math.Inf(-1)
	minZ, maxZ := math.Inf(1), math.Inf(-1)
	for i, v := range verts {
		pts[i] = point{x: v.x, z: v.z}
		minX = math.Min(minX, v.x)
		maxX = math.Max(maxX, v.x)
		minZ = math.Min(minZ, v.z)
		maxZ = math.Max(maxZ, v.z)
	}
	span := math.Max(maxX-minX, maxZ-minZ)
	if !(span > 0) {
		return flags
	}
	eps := span * 1e-9

	order := make([]int, len(pts))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := pts[order[i]], pts[order[j]]
		if math.Abs(a.x-b.x) > eps {
			return a.x < b.x
		}
		if math.Abs(a.z-b.z) > eps {
			return a.z < b.z
		}
		return order[i] < order[j]
	})
	unique := make([]int, 0, len(order))
	for _, idx := range order {
		if len(unique) == 0 {
			unique = append(unique, idx)
			continue
		}
		last := pts[unique[len(unique)-1]]
		cur := pts[idx]
		if math.Abs(last.x-cur.x) <= eps && math.Abs(last.z-cur.z) <= eps {
			continue
		}
		unique = append(unique, idx)
	}
	if len(unique) < 2 {
		return flags
	}
	cross := func(a, b, c point) float64 {
		return (b.x-a.x)*(c.z-a.z) - (b.z-a.z)*(c.x-a.x)
	}
	lower := make([]int, 0, len(unique))
	for _, idx := range unique {
		for len(lower) >= 2 {
			a := pts[lower[len(lower)-2]]
			b := pts[lower[len(lower)-1]]
			c := pts[idx]
			if cross(a, b, c) > eps*span {
				break
			}
			lower = lower[:len(lower)-1]
		}
		lower = append(lower, idx)
	}
	upper := make([]int, 0, len(unique))
	for i := len(unique) - 1; i >= 0; i-- {
		idx := unique[i]
		for len(upper) >= 2 {
			a := pts[upper[len(upper)-2]]
			b := pts[upper[len(upper)-1]]
			c := pts[idx]
			if cross(a, b, c) > eps*span {
				break
			}
			upper = upper[:len(upper)-1]
		}
		upper = append(upper, idx)
	}
	hull := append(lower[:len(lower)-1], upper[:len(upper)-1]...)
	if len(hull) < 2 {
		return flags
	}
	pointOnSegment := func(p, a, b point) bool {
		abx, abz := b.x-a.x, b.z-a.z
		apx, apz := p.x-a.x, p.z-a.z
		length := math.Hypot(abx, abz)
		if length <= eps {
			return math.Hypot(apx, apz) <= eps
		}
		if math.Abs(abx*apz-abz*apx) > eps*length*4 {
			return false
		}
		dot := apx*abx + apz*abz
		return dot >= -eps*length && dot <= abx*abx+abz*abz+eps*length
	}
	for ei, edge := range edges {
		if edge.a < 0 || edge.a >= len(pts) || edge.b < 0 || edge.b >= len(pts) {
			continue
		}
		a, b := pts[edge.a], pts[edge.b]
		if math.Hypot(a.x-b.x, a.z-b.z) <= eps {
			continue
		}
		for i := 0; i < len(hull); i++ {
			h0 := pts[hull[i]]
			h1 := pts[hull[(i+1)%len(hull)]]
			if pointOnSegment(a, h0, h1) && pointOnSegment(b, h0, h1) {
				flags[ei] = true
				break
			}
		}
	}
	return flags
}

// md4EdgeVisibleByCoverCount is the rendering rule used throughout A161.
// An outer-contour edge is always visible. Every other point is visible in
// opaque mode exactly when no projected 2-face lies in front of it.
func md4EdgeVisibleByCoverCount(layers int, exterior bool) bool {
	return exterior || layers <= 0
}

type mdCell4 struct {
	vertices [8]int
}

func mdLenSq4(v mdVec4) float64 {
	return v.x*v.x + v.y*v.y + v.z*v.z + v.w*v.w
}

// mdRot4XW rotates in the XW coordinate plane.  A144 keeps the existing
// "w rotation" slider to this one unambiguous 4D rotation plane.
func mdRot4XW(v mdVec4, a float64) mdVec4 {
	c, s := math.Cos(a), math.Sin(a)
	return mdVec4{
		x: v.x*c - v.w*s,
		y: v.y,
		z: v.z,
		w: v.x*s + v.w*c,
	}
}

// mdRot4YZ rotates in the YZ coordinate plane. Combined with XW, the two
// orthogonal planes form an isoclinic 4D double rotation: no ordinary 3D axis
// remains fixed, so the projected motion cannot collapse into a Y-axis-like
// turn.
func mdRot4YZ(v mdVec4, a float64) mdVec4 {
	c, s := math.Cos(a), math.Sin(a)
	return mdVec4{
		x: v.x,
		y: v.y*c - v.z*s,
		z: v.y*s + v.z*c,
		w: v.w,
	}
}

// mdRot4KleinW is the dedicated Klein-bottle W turn used from A186 onward.
// XW and YZ are mutually orthogonal rotation planes and therefore commute.
// Moving both by the same angle is a genuine 4D isoclinic double rotation.
func mdRot4KleinW(v mdVec4, a float64) mdVec4 {
	return mdRot4YZ(mdRot4XW(v, a), a)
}

const (
	// A212 caps the user-facing Klein subdivision density at 30. Smooth mode
	// may still supersample the mesh internally, but the slider and renderer
	// share this single bound so UI, logs, and actual geometry cannot diverge.
	mdKleinSubdivisionMin = 12
	mdKleinSubdivisionMax = 30
)

func mdClampKleinSubdivision(v int) int {
	if v < mdKleinSubdivisionMin {
		return mdKleinSubdivisionMin
	}
	if v > mdKleinSubdivisionMax {
		return mdKleinSubdivisionMax
	}
	return v
}

const (
	mdKleinProjectionDistance = 4.34
	mdKleinProjectionMinDenom = 1.50
)

// mdKleinProjectionDenominator deliberately allows the Klein bottle to come
// much closer to the W camera than A186 did, but never lets the perspective
// denominator fall below 1.5.  This increases the visible near/far sheet
// exchange without restoring the old singular spike or sign flip.
func mdKleinProjectionDenominator(w, angle float64) float64 {
	denom := mdKleinProjectionDistance - math.Sin(angle)*w
	if denom < mdKleinProjectionMinDenom {
		return mdKleinProjectionMinDenom
	}
	return denom
}

// mdProjectKlein4To3 keeps the familiar bottle exactly unchanged at W angle
// zero, then reveals stronger bounded W depth as the double rotation turns.
// A187 relaxes the previous conservative perspective distance so the
// denominator may approach 1.5, while the explicit floor prevents projection
// spikes and sign reversal. A small oblique W-to-Z lift keeps the sheet
// exchange readable instead of looking like a single ordinary 3D axis turn.
func mdProjectKlein4To3(v mdVec4, angle float64) mdVec3 {
	strength := math.Sin(angle)
	denom := mdKleinProjectionDenominator(v.w, angle)
	k := mdKleinProjectionDistance / denom
	return mdVec3{
		x: v.x * k,
		y: v.y * k,
		z: (v.z + 0.22*strength*v.w) * k,
	}
}

// mdProject4To3Components performs perspective projection along the W axis.
// The caller applies the already-established 3D Euler rotation afterwards.
func mdProject4To3Components(v mdVec4, distance float64) (float64, float64, float64) {
	denom := distance - v.w
	if math.Abs(denom) < 1e-9 {
		denom = math.Copysign(1e-9, denom)
	}
	k := distance / denom
	return v.x * k, v.y * k, v.z * k
}

func mdTesseractVertexIndex(xBit, yBit, zBit, wBit int) int {
	return (xBit & 1) | ((yBit & 1) << 1) | ((zBit & 1) << 2) | ((wBit & 1) << 3)
}

// mdBuildTesseract returns the complete regular tesseract combinatorics:
// 16 vertices, 32 edges, 24 square faces and 8 cubic cells.
func mdBuildTesseract() ([]mdVec4, []mdEdge4, []mdQuad4, []mdCell4) {
	verts := make([]mdVec4, 16)
	for i := 0; i < 16; i++ {
		coord := func(bit int) float64 {
			if i&(1<<bit) != 0 {
				return 1
			}
			return -1
		}
		verts[i] = mdVec4{x: coord(0), y: coord(1), z: coord(2), w: coord(3)}
	}

	edges := make([]mdEdge4, 0, 32)
	for i := 0; i < 16; i++ {
		for axis := 0; axis < 4; axis++ {
			j := i ^ (1 << axis)
			if i < j {
				edges = append(edges, mdEdge4{a: i, b: j})
			}
		}
	}

	faces := make([]mdQuad4, 0, 24)
	for axisA := 0; axisA < 4; axisA++ {
		for axisB := axisA + 1; axisB < 4; axisB++ {
			fixedAxes := make([]int, 0, 2)
			for axis := 0; axis < 4; axis++ {
				if axis != axisA && axis != axisB {
					fixedAxes = append(fixedAxes, axis)
				}
			}
			for fixedMask := 0; fixedMask < 4; fixedMask++ {
				base := 0
				for j, axis := range fixedAxes {
					if fixedMask&(1<<j) != 0 {
						base |= 1 << axis
					}
				}
				faces = append(faces, mdQuad4{indices: [4]int{
					base,
					base | (1 << axisA),
					base | (1 << axisA) | (1 << axisB),
					base | (1 << axisB),
				}})
			}
		}
	}

	cells := make([]mdCell4, 0, 8)
	for fixedAxis := 0; fixedAxis < 4; fixedAxis++ {
		for fixedBit := 0; fixedBit < 2; fixedBit++ {
			cell := mdCell4{}
			pos := 0
			for i := 0; i < 16; i++ {
				if ((i >> fixedAxis) & 1) == fixedBit {
					cell.vertices[pos] = i
					pos++
				}
			}
			cells = append(cells, cell)
		}
	}

	return verts, edges, faces, cells
}

// mdBuildRegular5Cell returns a centered regular 4-simplex (regular 5-cell):
// 5 vertices, 10 edges, 10 triangular faces and 5 tetrahedral cells.
// All vertices lie on the unit 3-sphere and every pair of distinct vertices
// has the same Euclidean distance.
func mdBuildRegular5Cell() ([]mdVec4, []mdEdge4, []mdFace4, [][]int) {
	invSqrt5 := 1.0 / math.Sqrt(5.0)
	raw := []mdVec4{
		{x: 1, y: 1, z: 1, w: -invSqrt5},
		{x: 1, y: -1, z: -1, w: -invSqrt5},
		{x: -1, y: 1, z: -1, w: -invSqrt5},
		{x: -1, y: -1, z: 1, w: -invSqrt5},
		{x: 0, y: 0, z: 0, w: 4 * invSqrt5},
	}
	verts := make([]mdVec4, len(raw))
	for i, v := range raw {
		invLen := 1.0 / math.Sqrt(mdLenSq4(v))
		verts[i] = mdVec4{x: v.x * invLen, y: v.y * invLen, z: v.z * invLen, w: v.w * invLen}
	}

	edges := make([]mdEdge4, 0, 10)
	for a := 0; a < 5; a++ {
		for b := a + 1; b < 5; b++ {
			edges = append(edges, mdEdge4{a: a, b: b})
		}
	}

	faces := make([]mdFace4, 0, 10)
	for a := 0; a < 5; a++ {
		for b := a + 1; b < 5; b++ {
			for c := b + 1; c < 5; c++ {
				faces = append(faces, mdFace4{indices: []int{a, b, c}})
			}
		}
	}

	cells := make([][]int, 0, 5)
	for omitted := 0; omitted < 5; omitted++ {
		cell := make([]int, 0, 4)
		for v := 0; v < 5; v++ {
			if v != omitted {
				cell = append(cell, v)
			}
		}
		cells = append(cells, cell)
	}
	return verts, edges, faces, cells
}

// mdBuildRegular16Cell returns the regular 4D cross-polytope (16-cell):
// 8 axis vertices, 24 edges, 32 triangular faces and 16 tetrahedral cells.
// Vertices are the positive and negative unit vectors of the four axes.
func mdBuildRegular16Cell() ([]mdVec4, []mdEdge4, []mdFace4, [][]int) {
	verts := []mdVec4{
		{x: -1}, {x: 1},
		{y: -1}, {y: 1},
		{z: -1}, {z: 1},
		{w: -1}, {w: 1},
	}
	vertexIndex := func(axis, signBit int) int { return axis*2 + signBit }

	edges := make([]mdEdge4, 0, 24)
	for axisA := 0; axisA < 4; axisA++ {
		for axisB := axisA + 1; axisB < 4; axisB++ {
			for signA := 0; signA < 2; signA++ {
				for signB := 0; signB < 2; signB++ {
					edges = append(edges, mdEdge4{a: vertexIndex(axisA, signA), b: vertexIndex(axisB, signB)})
				}
			}
		}
	}

	faces := make([]mdFace4, 0, 32)
	for omittedAxis := 0; omittedAxis < 4; omittedAxis++ {
		axes := make([]int, 0, 3)
		for axis := 0; axis < 4; axis++ {
			if axis != omittedAxis {
				axes = append(axes, axis)
			}
		}
		for signs := 0; signs < 8; signs++ {
			faces = append(faces, mdFace4{indices: []int{
				vertexIndex(axes[0], (signs>>0)&1),
				vertexIndex(axes[1], (signs>>1)&1),
				vertexIndex(axes[2], (signs>>2)&1),
			}})
		}
	}

	cells := make([][]int, 0, 16)
	for signs := 0; signs < 16; signs++ {
		cell := make([]int, 4)
		for axis := 0; axis < 4; axis++ {
			cell[axis] = vertexIndex(axis, (signs>>axis)&1)
		}
		cells = append(cells, cell)
	}
	return verts, edges, faces, cells
}

// mdBuildRegular24Cell returns the self-dual regular 24-cell. Its vertices are
// all permutations of (±1, ±1, 0, 0), normalized onto the unit 3-sphere.
// Adjacent vertices have squared distance 1, producing 24 vertices, 96 edges,
// 96 triangular faces and 24 octahedral cells.
func mdBuildRegular24Cell() ([]mdVec4, []mdEdge4, []mdFace4, [][]int) {
	verts := make([]mdVec4, 0, 24)
	invSqrt2 := 1.0 / math.Sqrt(2)
	for axisA := 0; axisA < 4; axisA++ {
		for axisB := axisA + 1; axisB < 4; axisB++ {
			for _, signA := range []float64{-1, 1} {
				for _, signB := range []float64{-1, 1} {
					c := [4]float64{}
					c[axisA] = signA * invSqrt2
					c[axisB] = signB * invSqrt2
					verts = append(verts, mdVec4{x: c[0], y: c[1], z: c[2], w: c[3]})
				}
			}
		}
	}

	distSq := func(a, b mdVec4) float64 {
		dx, dy, dz, dw := a.x-b.x, a.y-b.y, a.z-b.z, a.w-b.w
		return dx*dx + dy*dy + dz*dz + dw*dw
	}
	edgeSet := make(map[[2]int]bool, 96)
	edges := make([]mdEdge4, 0, 96)
	for a := 0; a < len(verts); a++ {
		for b := a + 1; b < len(verts); b++ {
			if math.Abs(distSq(verts[a], verts[b])-1) < 1e-10 {
				edges = append(edges, mdEdge4{a: a, b: b})
				edgeSet[[2]int{a, b}] = true
			}
		}
	}

	faces := make([]mdFace4, 0, 96)
	for a := 0; a < len(verts); a++ {
		for b := a + 1; b < len(verts); b++ {
			if !edgeSet[[2]int{a, b}] {
				continue
			}
			for c := b + 1; c < len(verts); c++ {
				if edgeSet[[2]int{a, c}] && edgeSet[[2]int{b, c}] {
					faces = append(faces, mdFace4{indices: []int{a, b, c}})
				}
			}
		}
	}

	// The 24 supporting normals are 8 coordinate-axis directions and 16
	// half-sign directions. The six maximizing vertices form one octahedron.
	normals := make([]mdVec4, 0, 24)
	for axis := 0; axis < 4; axis++ {
		for _, sign := range []float64{-1, 1} {
			c := [4]float64{}
			c[axis] = sign
			normals = append(normals, mdVec4{x: c[0], y: c[1], z: c[2], w: c[3]})
		}
	}
	for mask := 0; mask < 16; mask++ {
		n := mdVec4{}
		coords := []*float64{&n.x, &n.y, &n.z, &n.w}
		for axis := 0; axis < 4; axis++ {
			if mask&(1<<axis) != 0 {
				*coords[axis] = 0.5
			} else {
				*coords[axis] = -0.5
			}
		}
		normals = append(normals, n)
	}
	dot4 := func(a, b mdVec4) float64 { return a.x*b.x + a.y*b.y + a.z*b.z + a.w*b.w }
	cells := make([][]int, 0, 24)
	for _, n := range normals {
		maxDot := -math.MaxFloat64
		for _, v := range verts {
			if d := dot4(v, n); d > maxDot {
				maxDot = d
			}
		}
		cell := make([]int, 0, 6)
		for i, v := range verts {
			if math.Abs(dot4(v, n)-maxDot) < 1e-10 {
				cell = append(cell, i)
			}
		}
		cells = append(cells, cell)
	}
	return verts, edges, faces, cells
}

// mdBuildRegular600CellPrimal builds the regular 600-cell used as the primal
// geometry for the dual regular 120-cell.  Its 120 unit vertices are the
// standard H4 root-system coordinates.  The shortest-distance graph has 720
// edges and every 4-clique is one of the 600 tetrahedral cells.
func mdBuildRegular600CellPrimal() ([]mdVec4, []mdEdge4, [][]int) {
	phi := (1.0 + math.Sqrt(5.0)) / 2.0
	verts := make([]mdVec4, 0, 120)

	// 8 coordinate-axis vertices.
	for axis := 0; axis < 4; axis++ {
		for _, sign := range []float64{-1, 1} {
			c := [4]float64{}
			c[axis] = sign
			verts = append(verts, mdVec4{x: c[0], y: c[1], z: c[2], w: c[3]})
		}
	}

	// 16 half-sign vertices.
	for mask := 0; mask < 16; mask++ {
		c := [4]float64{}
		for axis := 0; axis < 4; axis++ {
			if mask&(1<<axis) != 0 {
				c[axis] = 0.5
			} else {
				c[axis] = -0.5
			}
		}
		verts = append(verts, mdVec4{x: c[0], y: c[1], z: c[2], w: c[3]})
	}

	// 96 vertices: even permutations of
	// (0, 1/2, phi/2, 1/(2phi)), with independent signs on nonzero entries.
	base := [4]float64{0, 0.5, phi / 2.0, 1.0 / (2.0 * phi)}
	perm := [4]int{}
	used := [4]bool{}
	var emitPermutations func(int)
	emitPermutations = func(pos int) {
		if pos == 4 {
			inversions := 0
			for i := 0; i < 4; i++ {
				for j := i + 1; j < 4; j++ {
					if perm[i] > perm[j] {
						inversions++
					}
				}
			}
			if inversions%2 != 0 {
				return
			}
			values := [4]float64{base[perm[0]], base[perm[1]], base[perm[2]], base[perm[3]]}
			nonzero := make([]int, 0, 3)
			for i, v := range values {
				if v != 0 {
					nonzero = append(nonzero, i)
				}
			}
			for signMask := 0; signMask < 8; signMask++ {
				c := values
				for i, axis := range nonzero {
					if signMask&(1<<i) == 0 {
						c[axis] = -c[axis]
					}
				}
				verts = append(verts, mdVec4{x: c[0], y: c[1], z: c[2], w: c[3]})
			}
			return
		}
		for i := 0; i < 4; i++ {
			if used[i] {
				continue
			}
			used[i] = true
			perm[pos] = i
			emitPermutations(pos + 1)
			used[i] = false
		}
	}
	emitPermutations(0)

	distSq := func(a, b mdVec4) float64 {
		dx, dy, dz, dw := a.x-b.x, a.y-b.y, a.z-b.z, a.w-b.w
		return dx*dx + dy*dy + dz*dz + dw*dw
	}
	shortestSq := 1.0 / (phi * phi)
	adj := make([][]bool, len(verts))
	for i := range adj {
		adj[i] = make([]bool, len(verts))
	}
	edges := make([]mdEdge4, 0, 720)
	for a := 0; a < len(verts); a++ {
		for b := a + 1; b < len(verts); b++ {
			if math.Abs(distSq(verts[a], verts[b])-shortestSq) < 1e-9 {
				adj[a][b], adj[b][a] = true, true
				edges = append(edges, mdEdge4{a: a, b: b})
			}
		}
	}

	cells := make([][]int, 0, 600)
	for a := 0; a < len(verts); a++ {
		for b := a + 1; b < len(verts); b++ {
			if !adj[a][b] {
				continue
			}
			for c := b + 1; c < len(verts); c++ {
				if !adj[a][c] || !adj[b][c] {
					continue
				}
				for d := c + 1; d < len(verts); d++ {
					if adj[a][d] && adj[b][d] && adj[c][d] {
						cells = append(cells, []int{a, b, c, d})
					}
				}
			}
		}
	}
	return verts, edges, cells
}

// mdBuildRegular600Cell returns the regular 600-cell itself.  The standard
// H4-root vertices, shortest-distance edges and tetrahedral cells are built by
// mdBuildRegular600CellPrimal; the 1200 triangular 2-faces are the unique
// triples shared by pairs of tetrahedral cells.
func mdBuildRegular600Cell() ([]mdVec4, []mdEdge4, []mdFace4, [][]int) {
	verts, edges, cells := mdBuildRegular600CellPrimal()
	faceSet := make(map[[3]int]bool, 1200)
	faces := make([]mdFace4, 0, 1200)
	for _, cell := range cells {
		if len(cell) != 4 {
			continue
		}
		for omitted := 0; omitted < 4; omitted++ {
			tri := make([]int, 0, 3)
			for i, v := range cell {
				if i != omitted {
					tri = append(tri, v)
				}
			}
			key := mdSortedTriple(tri[0], tri[1], tri[2])
			if faceSet[key] {
				continue
			}
			faceSet[key] = true
			faces = append(faces, mdFace4{indices: []int{key[0], key[1], key[2]}})
		}
	}
	return verts, edges, faces, cells
}

func mdSortedPair(a, b int) [2]int {
	if a > b {
		a, b = b, a
	}
	return [2]int{a, b}
}

func mdSortedTriple(a, b, c int) [3]int {
	v := [3]int{a, b, c}
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			if v[j] < v[i] {
				v[i], v[j] = v[j], v[i]
			}
		}
	}
	return v
}

// mdOrderDualPentagon orders the five dual vertices around one pentagonal
// face.  The induced dual-edge graph is a simple 5-cycle.
func mdOrderDualPentagon(vertices []int, edgeSet map[[2]int]bool) []int {
	if len(vertices) != 5 {
		return append([]int(nil), vertices...)
	}
	start := vertices[0]
	for _, v := range vertices[1:] {
		if v < start {
			start = v
		}
	}
	neighbors := func(v int) []int {
		out := make([]int, 0, 2)
		for _, other := range vertices {
			if other != v && edgeSet[mdSortedPair(v, other)] {
				out = append(out, other)
			}
		}
		if len(out) == 2 && out[1] < out[0] {
			out[0], out[1] = out[1], out[0]
		}
		return out
	}
	firstNeighbors := neighbors(start)
	if len(firstNeighbors) != 2 {
		return append([]int(nil), vertices...)
	}
	ordered := []int{start}
	prev, current := start, firstNeighbors[0]
	for len(ordered) < 5 {
		ordered = append(ordered, current)
		nb := neighbors(current)
		if len(nb) != 2 {
			return append([]int(nil), vertices...)
		}
		next := nb[0]
		if next == prev {
			next = nb[1]
		}
		prev, current = current, next
	}
	return ordered
}

// mdBuildRegular120Cell constructs the regular 120-cell as the geometric dual
// of the regular 600-cell.  Tetrahedral cell centers become its 600 vertices;
// shared triangular faces become 1200 edges; the five tetrahedra around each
// primal edge become one pentagonal face; and the 20 tetrahedra around each
// primal vertex form one dodecahedral cell.
func mdBuildRegular120Cell() ([]mdVec4, []mdEdge4, []mdFace4, [][]int) {
	primalVerts, _, primalCells := mdBuildRegular600CellPrimal()
	verts := make([]mdVec4, len(primalCells))
	for i, cell := range primalCells {
		center := mdVec4{}
		for _, vi := range cell {
			v := primalVerts[vi]
			center.x += v.x
			center.y += v.y
			center.z += v.z
			center.w += v.w
		}
		invLen := 1.0 / math.Sqrt(mdLenSq4(center))
		verts[i] = mdVec4{x: center.x * invLen, y: center.y * invLen, z: center.z * invLen, w: center.w * invLen}
	}

	triToCells := make(map[[3]int][]int, 1200)
	primalEdgeToCells := make(map[[2]int][]int, 720)
	vertexToCells := make([][]int, len(primalVerts))
	for ci, cell := range primalCells {
		for _, v := range cell {
			vertexToCells[v] = append(vertexToCells[v], ci)
		}
		for a := 0; a < 4; a++ {
			for b := a + 1; b < 4; b++ {
				key := mdSortedPair(cell[a], cell[b])
				primalEdgeToCells[key] = append(primalEdgeToCells[key], ci)
			}
		}
		for omitted := 0; omitted < 4; omitted++ {
			tri := make([]int, 0, 3)
			for i, v := range cell {
				if i != omitted {
					tri = append(tri, v)
				}
			}
			key := mdSortedTriple(tri[0], tri[1], tri[2])
			triToCells[key] = append(triToCells[key], ci)
		}
	}

	edges := make([]mdEdge4, 0, 1200)
	edgeSet := make(map[[2]int]bool, 1200)
	for _, pair := range triToCells {
		if len(pair) != 2 {
			continue
		}
		key := mdSortedPair(pair[0], pair[1])
		if !edgeSet[key] {
			edgeSet[key] = true
			edges = append(edges, mdEdge4{a: key[0], b: key[1]})
		}
	}

	faces := make([]mdFace4, 0, 720)
	for _, around := range primalEdgeToCells {
		faces = append(faces, mdFace4{indices: mdOrderDualPentagon(around, edgeSet)})
	}

	cells := make([][]int, len(vertexToCells))
	for i, list := range vertexToCells {
		cells[i] = append([]int(nil), list...)
	}
	return verts, edges, faces, cells
}

func mdTesseractFacesAsPolygons(quads []mdQuad4) []mdFace4 {
	faces := make([]mdFace4, len(quads))
	for i, q := range quads {
		faces[i] = mdFace4{indices: []int{q.indices[0], q.indices[1], q.indices[2], q.indices[3]}}
	}
	return faces
}

// mdDepthFadeFactor returns the remaining line contrast after n independent
// 30-percent fade layers. The deepest lines remain visible at an 8-percent
// contrast floor.
func mdDepthFadeFactor(layers int) float64 {
	if layers <= 0 {
		return 1
	}
	factor := math.Pow(0.70, float64(layers))
	if factor < 0.08 {
		return 0.08
	}
	return factor
}

// mdKleinPoint4 evaluates a bottle-shaped embedding of the Klein bottle in R^4.
//
// The first three coordinates are the familiar classical "tube through the
// wall" immersion.  The fourth coordinate lifts the two sheets apart at the
// apparent 3D self-intersection.  The parameter seam obeys
// K(2π,v)=K(0,π-v), so the final U strip is joined to the first strip with a
// reversed-and-half-shifted V index.  This keeps the recognizable bottle
// silhouette while retaining a genuine fourth-dimensional separation.
func mdKleinPoint4(u, v, ringRadius, tubeRadius float64) mdVec4 {
	cu, su := math.Cos(u), math.Sin(u)
	cv, sv := math.Cos(v), math.Sin(v)

	// ringRadius controls the directrix (body/neck) scale.  tubeRadius
	// controls the circular tube radius independently.  These constants are
	// the normalized Dickson-style classical Klein-bottle proportions.
	body := ringRadius
	radial := 4.0 * tubeRadius * (1.0 - 0.5*cu)
	baseX := 3.0 * body * cu * (1.0 + su)
	baseY := 8.0 * body * su

	x, y := 0.0, 0.0
	if u < math.Pi {
		x = baseX + radial*cu*cv
		y = baseY + radial*su*cv
	} else {
		x = baseX - radial*cv // cos(v+π)
		y = baseY
	}
	z := radial * sv

	// The classical R^3 surface intersects itself around the returning neck.
	// A seam-compatible fourth coordinate separates those sheets.  It is zero
	// at u=0 and u=2π, and numerical regression tests cover the former crossing.
	w := 2.0 * tubeRadius * math.Sin(0.5*u) * cv
	return mdVec4{x: x, y: y, z: z, w: w}
}

// mdBuildKleinBottle4D builds a closed, non-orientable quadrilateral mesh.
// uEdges run in the twisted U direction; vEdges run around each tube section.
func mdBuildKleinBottle4D(uSegments, vSegments int, ringRadius, tubeRadius float64) (verts []mdVec4, faces []mdFace4, uEdges, vEdges []mdEdge4) {
	if uSegments < 8 {
		uSegments = 8
	}
	if vSegments < 6 {
		vSegments = 6
	}
	if ringRadius <= 0 {
		ringRadius = 2
	}
	if tubeRadius <= 0 {
		tubeRadius = 0.65
	}
	if ringRadius < tubeRadius*1.15 {
		ringRadius = tubeRadius * 1.15
	}
	index := func(iu, iv int) int {
		iu %= uSegments
		if iu < 0 {
			iu += uSegments
		}
		iv %= vSegments
		if iv < 0 {
			iv += vSegments
		}
		return iu*vSegments + iv
	}
	reverseV := func(iv int) int {
		iv %= vSegments
		if iv < 0 {
			iv += vSegments
		}
		// Classical bottle coordinates close with v -> π-v.  An even V
		// count makes the half-turn exact on the discrete grid.
		return (vSegments/2 - iv + vSegments) % vSegments
	}

	verts = make([]mdVec4, uSegments*vSegments)
	for iu := 0; iu < uSegments; iu++ {
		u := 2 * math.Pi * float64(iu) / float64(uSegments)
		for iv := 0; iv < vSegments; iv++ {
			v := 2 * math.Pi * float64(iv) / float64(vSegments)
			verts[index(iu, iv)] = mdKleinPoint4(u, v, ringRadius, tubeRadius)
		}
	}

	faces = make([]mdFace4, 0, uSegments*vSegments)
	uEdges = make([]mdEdge4, 0, uSegments*vSegments)
	vEdges = make([]mdEdge4, 0, uSegments*vSegments)
	for iu := 0; iu < uSegments; iu++ {
		nextU := iu + 1
		seam := nextU == uSegments
		for iv := 0; iv < vSegments; iv++ {
			nextV := (iv + 1) % vSegments
			a := index(iu, iv)
			d := index(iu, nextV)
			var b, c int
			if seam {
				b = index(0, reverseV(iv))
				c = index(0, reverseV(nextV))
			} else {
				b = index(nextU, iv)
				c = index(nextU, nextV)
			}
			faces = append(faces, mdFace4{indices: []int{a, b, c, d}})
			uEdges = append(uEdges, mdEdge4{a: a, b: b})
			vEdges = append(vEdges, mdEdge4{a: a, b: d})
		}
	}
	return verts, faces, uEdges, vEdges
}

// mdCenterAndScale4D translates a 4D point cloud to its centroid and applies
// one uniform scale so every point stays inside targetRadius. Uniform scaling
// preserves the Klein bottle silhouette while guaranteeing a safe W range for
// perspective rotation.
func mdCenterAndScale4D(verts []mdVec4, targetRadius float64) []mdVec4 {
	if len(verts) == 0 {
		return nil
	}
	if targetRadius <= 0 {
		targetRadius = 1
	}
	center := mdVec4{}
	for _, v := range verts {
		center.x += v.x
		center.y += v.y
		center.z += v.z
		center.w += v.w
	}
	invN := 1.0 / float64(len(verts))
	center.x *= invN
	center.y *= invN
	center.z *= invN
	center.w *= invN

	out := make([]mdVec4, len(verts))
	maxRadius := 0.0
	for i, v := range verts {
		v = mdVec4{x: v.x - center.x, y: v.y - center.y, z: v.z - center.z, w: v.w - center.w}
		out[i] = v
		r := math.Sqrt(mdLenSq4(v))
		if r > maxRadius {
			maxRadius = r
		}
	}
	if maxRadius <= 1e-12 {
		return out
	}
	scale := targetRadius / maxRadius
	for i := range out {
		out[i].x *= scale
		out[i].y *= scale
		out[i].z *= scale
		out[i].w *= scale
	}
	return out
}

// mdPrepareKleinTurn4D centers the complete Klein mesh, scales the familiar
// three-dimensional bottle silhouette and the fourth-dimensional sheet lift
// independently, and returns a bounded point cloud for the dedicated Klein
// W-turn preview.
//
// A159 used one uniform 4D scale. The classical bottle coordinates span much
// farther in XYZ than in W, so that step reduced the useful W lift to about
// one tenth of the visible bottle radius. The result was stable but looked
// almost like an ordinary 3D rotation. Keeping a strong, bounded W extent
// makes the two sheets at the apparent self-intersection visibly exchange
// spatial position when W is rotated into X.
func mdPrepareKleinTurn4D(verts []mdVec4, spatialRadius, wRadius float64) []mdVec4 {
	if len(verts) == 0 {
		return nil
	}
	if spatialRadius <= 0 {
		spatialRadius = 2.35
	}
	if wRadius <= 0 {
		wRadius = 1.55
	}
	center := mdVec4{}
	for _, v := range verts {
		center.x += v.x
		center.y += v.y
		center.z += v.z
		center.w += v.w
	}
	invN := 1.0 / float64(len(verts))
	center.x *= invN
	center.y *= invN
	center.z *= invN
	center.w *= invN

	out := make([]mdVec4, len(verts))
	maxSpatial, maxAbsW := 0.0, 0.0
	for i, v := range verts {
		v = mdVec4{x: v.x - center.x, y: v.y - center.y, z: v.z - center.z, w: v.w - center.w}
		out[i] = v
		spatial := math.Sqrt(v.x*v.x + v.y*v.y + v.z*v.z)
		if spatial > maxSpatial {
			maxSpatial = spatial
		}
		if aw := math.Abs(v.w); aw > maxAbsW {
			maxAbsW = aw
		}
	}
	spatialScale, wScale := 1.0, 1.0
	if maxSpatial > 1e-12 {
		spatialScale = spatialRadius / maxSpatial
	}
	if maxAbsW > 1e-12 {
		wScale = wRadius / maxAbsW
	}
	for i := range out {
		out[i].x *= spatialScale
		out[i].y *= spatialScale
		out[i].z *= spatialScale
		out[i].w *= wScale
	}
	return out
}

// mdProject4To3Orthographic discards the post-rotation W coordinate. For the
// Klein bottle this is intentional: at W=0 the preview is exactly the familiar
// classical bottle immersion, while an XW turn moves the lifted fourth
// coordinate directly into visible X without a perspective denominator
// masking the exchange or creating a projection singularity.
func mdProject4To3Orthographic(v mdVec4) (float64, float64, float64) {
	return v.x, v.y, v.z
}

// mdOpaqueDepthPass reports whether a line sample is on or in front of the
// opaque face depth stored at the same screen location. Camera depth increases
// toward +Y, so larger values are nearer. A small positive bias keeps genuine
// face-boundary edges visible despite raster rounding.
func mdOpaqueDepthPass(lineDepth, faceDepth, bias float64) bool {
	if bias < 0 {
		bias = 0
	}
	return lineDepth >= faceDepth-bias
}
