package main

import (
	"fmt"
	"math"
	"testing"
)

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestBuildConeGeometry(t *testing.T) {
	const segments = 32
	verts, faces, apex, center := mdBuildConeGeometry(segments, 2, 6)
	if len(verts) != segments+2 {
		t.Fatalf("vertices=%d want %d", len(verts), segments+2)
	}
	if len(faces) != segments*2 {
		t.Fatalf("faces=%d want %d", len(faces), segments*2)
	}
	if apex != segments || center != segments+1 {
		t.Fatalf("indices apex=%d center=%d", apex, center)
	}
	if !almostEqual(verts[apex].z, 3) || !almostEqual(verts[center].z, -3) {
		t.Fatalf("unexpected axial positions: apex=%v center=%v", verts[apex], verts[center])
	}
	for i := 0; i < segments; i++ {
		r := math.Hypot(verts[i].x, verts[i].y)
		if !almostEqual(r, 2) || !almostEqual(verts[i].z, -3) {
			t.Fatalf("ring vertex %d=%v", i, verts[i])
		}
	}
}

func TestBuildPolygonalPyramidGeometry(t *testing.T) {
	for sides := 3; sides <= 16; sides++ {
		verts, faces, apex := mdBuildPolygonalPyramidGeometry(sides, 1.5, 4)
		if len(verts) != sides+1 {
			t.Fatalf("sides=%d vertices=%d", sides, len(verts))
		}
		if len(faces) != sides+1 {
			t.Fatalf("sides=%d faces=%d", sides, len(faces))
		}
		if apex != sides || !almostEqual(verts[apex].z, 2) {
			t.Fatalf("sides=%d apex=%d vertex=%v", sides, apex, verts[apex])
		}
		base := faces[len(faces)-1].indices
		if len(base) != sides {
			t.Fatalf("sides=%d base vertices=%d", sides, len(base))
		}
		for i := 0; i < sides; i++ {
			r := math.Hypot(verts[i].x, verts[i].y)
			if !almostEqual(r, 1.5) || !almostEqual(verts[i].z, -2) {
				t.Fatalf("sides=%d base vertex %d=%v", sides, i, verts[i])
			}
		}
	}
}

func rawFaceNormal(verts []mdVec3, face mdFace) mdVec3 {
	p0 := verts[face.indices[0]]
	p1 := verts[face.indices[1]]
	p2 := verts[face.indices[2]]
	return mdCrossCore(mdSubCore(p1, p0), mdSubCore(p2, p0))
}

func faceCentroid(verts []mdVec3, face mdFace) mdVec3 {
	c := mdVec3{}
	for _, idx := range face.indices {
		c.x += verts[idx].x
		c.y += verts[idx].y
		c.z += verts[idx].z
	}
	inv := 1.0 / float64(len(face.indices))
	return mdVec3{c.x * inv, c.y * inv, c.z * inv}
}

func TestRegularPolyhedronFaceWindingPointsOutward(t *testing.T) {
	cases := []struct {
		name  string
		build func() ([]mdVec3, []mdFace)
	}{
		{"tetrahedron", mdBuildTetrahedronGeometry},
		{"cube", mdBuildCubeGeometry},
		{"octahedron", mdBuildOctahedronGeometry},
	}
	for _, tc := range cases {
		verts, faces := tc.build()
		for i, face := range faces {
			n := rawFaceNormal(verts, face)
			c := faceCentroid(verts, face)
			if mdDotCore(n, c) <= 0 {
				t.Fatalf("%s face %d winding is not outward: dot=%g", tc.name, i, mdDotCore(n, c))
			}
			fixed := mdFaceOutwardNormal(verts, face)
			if mdDotCore(fixed, c) <= 0 {
				t.Fatalf("%s face %d corrected normal is not outward", tc.name, i)
			}
		}
	}
}

func TestDefaultLightVectorSemantics(t *testing.T) {
	ray := mdDefaultLightRayDirection()
	incoming := mdDefaultIncomingLightDirection()
	if ray.x <= 0 || ray.y >= 0 || ray.z >= 0 {
		t.Fatalf("ray direction=%v want signs (+,-,-)", ray)
	}
	if incoming.x >= 0 || incoming.y <= 0 || incoming.z <= 0 {
		t.Fatalf("incoming direction=%v want signs (-,+,+)", incoming)
	}
	if math.Abs(mdDotCore(ray, incoming)+1) > 1e-12 {
		t.Fatalf("ray and incoming are not exact opposites: dot=%g", mdDotCore(ray, incoming))
	}
}

func TestSphereGuideModeNamesAndSupersampling(t *testing.T) {
	if mdSphereGuideModeName(mdSphereGuidesEquator) != "적도만" ||
		mdSphereGuideModeName(mdSphereGuidesThreeAxes) != "3축" ||
		mdSphereGuideModeName(mdSphereGuidesNone) != "없음" {
		t.Fatal("unexpected sphere guide mode labels")
	}
	cases := []struct {
		smooth, interactive bool
		mode                int
		want                int
	}{
		{false, false, mdSphereGuidesThreeAxes, 1},
		{true, false, mdSphereGuidesThreeAxes, 6},
		{false, false, mdSphereGuidesNone, 1},
		{true, false, mdSphereGuidesNone, 6},
		{true, true, mdSphereGuidesNone, 6},
	}
	for _, tc := range cases {
		if got := mdSpherePreviewSupersample(tc.smooth, tc.interactive, tc.mode); got != tc.want {
			t.Fatalf("smooth=%v interactive=%v mode=%d: got %d want %d", tc.smooth, tc.interactive, tc.mode, got, tc.want)
		}
	}
}

func TestSphereMeshStepsAddRealCurveResolution(t *testing.T) {
	cases := []struct {
		smooth, interactive bool
		mode                int
		wantLat, wantLon    int
	}{
		{false, false, mdSphereGuidesThreeAxes, 16, 24},
		{true, false, mdSphereGuidesThreeAxes, 32, 48},
		{false, false, mdSphereGuidesNone, 32, 48},
		{true, false, mdSphereGuidesNone, 48, 72},
		{true, true, mdSphereGuidesNone, 16, 24},
	}
	for _, tc := range cases {
		lat, lon := mdSphereMeshSteps(tc.smooth, tc.interactive, tc.mode)
		if lat != tc.wantLat || lon != tc.wantLon {
			t.Fatalf("smooth=%v interactive=%v mode=%d: got %dx%d want %dx%d", tc.smooth, tc.interactive, tc.mode, lat, lon, tc.wantLat, tc.wantLon)
		}
	}
}

func TestEllipsoidMeshUsesSinglePolesAndClosedTriangles(t *testing.T) {
	rx, ry, rz := 1.7, 0.45, 2.2
	latSteps, lonSteps := 12, 18
	verts, faces, south, north := mdBuildEllipsoidMesh(rx, ry, rz, latSteps, lonSteps)
	wantVerts := 2 + (latSteps+1)*lonSteps
	wantFaces := 2 * (latSteps + 1) * lonSteps
	if len(verts) != wantVerts || len(faces) != wantFaces {
		t.Fatalf("unexpected ellipsoid counts: V=%d/%d F=%d/%d", len(verts), wantVerts, len(faces), wantFaces)
	}
	if south == north || south < 0 || north >= len(verts) {
		t.Fatalf("invalid pole indices south=%d north=%d", south, north)
	}
	poleCount := 0
	for i, v := range verts {
		if math.Abs(v.x) < 1e-12 && math.Abs(v.y) < 1e-12 && math.Abs(math.Abs(v.z)-rz) < 1e-12 {
			poleCount++
			if i != south && i != north {
				t.Fatalf("duplicate pole vertex at %d", i)
			}
		}
		q := v.x*v.x/(rx*rx) + v.y*v.y/(ry*ry) + v.z*v.z/(rz*rz)
		if math.Abs(q-1) > 1e-10 {
			t.Fatalf("vertex %d not on ellipsoid: q=%g", i, q)
		}
	}
	if poleCount != 2 {
		t.Fatalf("pole vertex count=%d want 2", poleCount)
	}
	for i, f := range faces {
		if len(f.indices) != 3 {
			t.Fatalf("face %d vertices=%d want triangle", i, len(f.indices))
		}
		seen := map[int]bool{}
		for _, idx := range f.indices {
			if idx < 0 || idx >= len(verts) || seen[idx] {
				t.Fatalf("face %d invalid indices=%v", i, f.indices)
			}
			seen[idx] = true
		}
	}
}

func TestEllipsoidNormalMatchesImplicitGradient(t *testing.T) {
	rx, ry, rz := 2.0, 0.7, 1.3
	p := mdVec3{x: rx / math.Sqrt(3), y: ry / math.Sqrt(3), z: rz / math.Sqrt(3)}
	n := mdEllipsoidNormalAt(p, rx, ry, rz)
	if math.Abs(mdDotCore(n, n)-1) > 1e-12 {
		t.Fatalf("normal not unit: %+v", n)
	}
	grad := mdNormCore(mdVec3{x: p.x / (rx * rx), y: p.y / (ry * ry), z: p.z / (rz * rz)})
	if math.Abs(mdDotCore(n, grad)-1) > 1e-12 {
		t.Fatalf("normal differs from implicit gradient: n=%+v grad=%+v", n, grad)
	}
}

func TestSphereCurveStepsGuideFreeBoost(t *testing.T) {
	if got := mdSphereCurveSteps(false, false, mdSphereGuidesNone); got != 720 {
		t.Fatalf("guide-free normal curve steps=%d want 720", got)
	}
	if got := mdSphereCurveSteps(true, false, mdSphereGuidesNone); got != 2160 {
		t.Fatalf("guide-free smooth curve steps=%d want 2160", got)
	}
	if got := mdSphereCurveSteps(true, true, mdSphereGuidesNone); got != 144 {
		t.Fatalf("interactive curve steps=%d want 144", got)
	}
}

func topologyCounts(faces []mdFace) (edges int, boundaryEdges int) {
	type key struct{ a, b int }
	counts := map[key]int{}
	for _, face := range faces {
		for i := 0; i < len(face.indices); i++ {
			a := face.indices[i]
			b := face.indices[(i+1)%len(face.indices)]
			if a > b {
				a, b = b, a
			}
			counts[key{a, b}]++
		}
	}
	for _, n := range counts {
		edges++
		if n == 1 {
			boundaryEdges++
		}
	}
	return edges, boundaryEdges
}

func TestBuildTorusMeshTopologyAndSurfaceEquation(t *testing.T) {
	const majorSteps, minorSteps = 24, 12
	const R, r = 2.0, 0.6
	verts, faces := mdBuildTorusMesh(R, r, majorSteps, minorSteps)
	if len(verts) != majorSteps*minorSteps || len(faces) != majorSteps*minorSteps {
		t.Fatalf("torus counts V=%d F=%d", len(verts), len(faces))
	}
	edges, boundary := topologyCounts(faces)
	if boundary != 0 {
		t.Fatalf("torus boundary edges=%d want 0", boundary)
	}
	if chi := len(verts) - edges + len(faces); chi != 0 {
		t.Fatalf("torus Euler characteristic=%d want 0", chi)
	}
	for i, p := range verts {
		rho := math.Hypot(p.x, p.y)
		q := (rho-R)*(rho-R) + p.z*p.z
		if math.Abs(q-r*r) > 1e-9 {
			t.Fatalf("vertex %d not on torus: q=%g", i, q)
		}
	}
}

func TestBuildMobiusMeshTwistedSeamAndTopology(t *testing.T) {
	const aroundSteps, acrossSteps = 30, 8
	const radius, halfWidth = 2.0, 0.45
	verts, faces := mdBuildMobiusMesh(radius, halfWidth, aroundSteps, acrossSteps)
	wantV := aroundSteps * (acrossSteps + 1)
	wantF := aroundSteps * acrossSteps
	if len(verts) != wantV || len(faces) != wantF {
		t.Fatalf("mobius counts V=%d/%d F=%d/%d", len(verts), wantV, len(faces), wantF)
	}
	edges, boundary := topologyCounts(faces)
	if boundary != 2*aroundSteps {
		t.Fatalf("mobius boundary edges=%d want %d", boundary, 2*aroundSteps)
	}
	if chi := len(verts) - edges + len(faces); chi != 0 {
		t.Fatalf("mobius Euler characteristic=%d want 0", chi)
	}
	for j := 0; j <= acrossSteps; j++ {
		v := -halfWidth + 2*halfWidth*float64(j)/float64(acrossSteps)
		a := mdMobiusPoint(radius, 2*math.Pi, v)
		b := mdMobiusPoint(radius, 0, -v)
		if math.Sqrt(mdDotCore(mdSubCore(a, b), mdSubCore(a, b))) > 1e-10 {
			t.Fatalf("twisted seam mismatch j=%d a=%+v b=%+v", j, a, b)
		}
	}
}

func TestParametricSurfaceQualitySteps(t *testing.T) {
	if u, v := mdTorusMeshSteps(false, false); u != 64 || v != 32 {
		t.Fatalf("torus normal steps=%dx%d", u, v)
	}
	if u, v := mdTorusMeshSteps(true, false); u != 96 || v != 48 {
		t.Fatalf("torus smooth steps=%dx%d", u, v)
	}
	if u, v := mdMobiusMeshSteps(true, true); u != 40 || v != 8 {
		t.Fatalf("mobius interactive steps=%dx%d", u, v)
	}
	if mdSurfaceGridModeName(mdSurfaceGridNone) != "격자선 없음" || mdSurfaceGridModeName(mdSurfaceGridMajor) != "주요 격자선" || mdSurfaceGridModeName(mdSurfaceGridFull) != "전체 격자선" {
		t.Fatal("surface grid labels changed")
	}
}

func TestTorusRearOuterPointHasThreeCoveringSurfaceLayers(t *testing.T) {
	const majorSteps, minorSteps = 48, 24
	const R, r = 2.0, 0.5
	verts, faces := mdBuildTorusMesh(R, r, majorSteps, minorSteps)
	// u=3π/2, v=0 is the rear outer point at x=z=0, y=-(R+r).
	idx := (3 * majorSteps / 4) * minorSteps
	p := verts[idx]
	if math.Abs(p.x) > 1e-10 || math.Abs(p.z) > 1e-10 || math.Abs(p.y+(R+r)) > 1e-10 {
		t.Fatalf("unexpected rear point: %+v", p)
	}
	if got := mdUniqueSurfaceCoverCountCore(p, verts, faces, nil, 1e-6); got != 3 {
		t.Fatalf("covering layers=%d want 3", got)
	}
}

func TestUniqueSurfaceCoverCountCanIgnoreGeneratingFaces(t *testing.T) {
	verts := []mdVec3{
		{x: -1, y: 1, z: -1}, {x: 1, y: 1, z: -1}, {x: 1, y: 1, z: 1}, {x: -1, y: 1, z: 1},
		{x: -1, y: 2, z: -1}, {x: 1, y: 2, z: -1}, {x: 1, y: 2, z: 1}, {x: -1, y: 2, z: 1},
	}
	faces := []mdFace{
		{indices: []int{0, 1, 2, 3}},
		{indices: []int{4, 5, 6, 7}},
	}
	point := mdVec3{x: 0, y: 0, z: 0}
	if got := mdUniqueSurfaceCoverCountIgnoringCore(point, verts, faces, nil, 1e-6, nil); got != 2 {
		t.Fatalf("all covering layers=%d want 2", got)
	}
	if got := mdUniqueSurfaceCoverCountIgnoringCore(point, verts, faces, nil, 1e-6, []int{0}); got != 1 {
		t.Fatalf("after ignoring generating face layers=%d want 1", got)
	}
	if got := mdUniqueSurfaceCoverCountIgnoringCore(point, verts, faces, nil, 1e-6, []int{0, 1}); got != 0 {
		t.Fatalf("after ignoring both faces layers=%d want 0", got)
	}
}

func rotateXYZForTorusVisibilityTest(v mdVec3, ax, ay, az float64) mdVec3 {
	cx, sx := math.Cos(ax), math.Sin(ax)
	cy, sy := math.Cos(ay), math.Sin(ay)
	cz, sz := math.Cos(az), math.Sin(az)
	v = mdVec3{x: v.x, y: v.y*cx - v.z*sx, z: v.y*sx + v.z*cx}
	v = mdVec3{x: v.x*cy + v.z*sy, y: v.y, z: -v.x*sy + v.z*cy}
	return mdVec3{x: v.x*cz - v.y*sz, y: v.x*sz + v.y*cz, z: v.z}
}

func TestDefaultTorusContainsCoveredTangentCandidates(t *testing.T) {
	const majorSteps, minorSteps = 64, 32
	verts, faces := mdBuildTorusMesh(190.0/135.0, 65.0/145.0, majorSteps, minorSteps)
	for i := range verts {
		verts[i] = rotateXYZForTorusVisibilityTest(verts[i], 0.61, 0.52, 0)
	}
	type edgeInfo struct {
		a, b    int
		normals []mdVec3
		faces   []int
	}
	edges := map[[2]int]*edgeInfo{}
	for fi, face := range faces {
		a, b, c := verts[face.indices[0]], verts[face.indices[1]], verts[face.indices[2]]
		n := mdNormCore(mdCrossCore(mdSubCore(b, a), mdSubCore(c, a)))
		for i := range face.indices {
			va := face.indices[i]
			vb := face.indices[(i+1)%len(face.indices)]
			if va > vb {
				va, vb = vb, va
			}
			key := [2]int{va, vb}
			e := edges[key]
			if e == nil {
				e = &edgeInfo{a: va, b: vb}
				edges[key] = e
			}
			e.normals = append(e.normals, n)
			e.faces = append(e.faces, fi)
		}
	}
	visible, covered := 0, 0
	maxLayers := 0
	for _, edge := range edges {
		if len(edge.normals) < 2 {
			continue
		}
		n0, n1 := edge.normals[0], edge.normals[1]
		if mdDotCore(n0, n1) < 0 {
			n1 = mdVec3{x: -n1.x, y: -n1.y, z: -n1.z}
		}
		if n0.y*n1.y > 1e-5 {
			continue
		}
		a, b := verts[edge.a], verts[edge.b]
		mid := mdVec3{x: (a.x + b.x) * 0.5, y: (a.y + b.y) * 0.5, z: (a.z + b.z) * 0.5}
		layers := mdUniqueSurfaceCoverCountIgnoringCore(mid, verts, faces, nil, 1e-4, edge.faces)
		if layers == 0 {
			visible++
		} else {
			covered++
			if layers > maxLayers {
				maxLayers = layers
			}
		}
	}
	if visible == 0 || covered == 0 {
		t.Fatalf("tangent candidates visible=%d covered=%d; need both classes", visible, covered)
	}
	if maxLayers < 2 {
		t.Fatalf("maximum front layers=%d want at least 2 for the marked rear tangent branches", maxLayers)
	}
}

func TestSurfaceDepthFadeFactorLeavesThirtyPercentWithEightPercentFloor(t *testing.T) {
	wants := []float64{
		1,
		0.30,
		0.09,
		0.08,
		0.08,
		0.08,
	}
	for layers, want := range wants {
		got := mdSurfaceDepthFadeFactor(layers)
		if math.Abs(got-want) > 1e-12 {
			t.Fatalf("layers=%d: got %.12f want %.12f", layers, got, want)
		}
	}
	if got := mdSurfaceDepthFadeFactor(-1); got != 1 {
		t.Fatalf("negative layers: got %.12f want 1", got)
	}
}

func TestEulerRotationMatrixRoundTrip(t *testing.T) {
	cases := [][3]float64{
		{0, 0, 0},
		{0.61, 0.52, 0},
		{1.2, -0.7, 2.4},
		{-2.1, 0.9, -1.4},
	}
	basis := []mdVec3{{x: 1}, {y: 1}, {z: 1}}
	for _, tc := range cases {
		want := mdEulerRotationMatrix(tc[0], tc[1], tc[2])
		ax, ay, az := mdEulerFromRotationMatrix(want)
		got := mdEulerRotationMatrix(ax, ay, az)
		for _, v := range basis {
			wv := mdMat3Apply(want, v)
			gv := mdMat3Apply(got, v)
			if math.Sqrt(mdDotCore(mdSubCore(wv, gv), mdSubCore(wv, gv))) > 1e-9 {
				t.Fatalf("round trip failed for %+v: want=%+v got=%+v", tc, wv, gv)
			}
		}
	}
}

func TestTrackballCenterDragMovesGrabbedPoint(t *testing.T) {
	start := mdTrackballVector(100, 100, 201, 201)
	right := mdTrackballVector(140, 100, 201, 201)
	up := mdTrackballVector(100, 60, 201, 201)
	mRight := mdTrackballDeltaMatrix(start, right)
	mUp := mdTrackballDeltaMatrix(start, up)
	gotRight := mdMat3Apply(mRight, start)
	gotUp := mdMat3Apply(mUp, start)
	if math.Sqrt(mdDotCore(mdSubCore(gotRight, right), mdSubCore(gotRight, right))) > 1e-9 {
		t.Fatalf("right drag mismatch: got=%+v want=%+v", gotRight, right)
	}
	if math.Sqrt(mdDotCore(mdSubCore(gotUp, up), mdSubCore(gotUp, up))) > 1e-9 {
		t.Fatalf("up drag mismatch: got=%+v want=%+v", gotUp, up)
	}
}

func TestTrackballComposedRotationRemainsOrthonormal(t *testing.T) {
	startRot := mdEulerRotationMatrix(0.61, 0.52, 0.33)
	a := mdTrackballVector(80, 90, 240, 180)
	b := mdTrackballVector(175, 45, 240, 180)
	combined := mdMat3Mul(mdTrackballDeltaMatrix(a, b), startRot)
	cols := []mdVec3{
		{x: combined[0][0], y: combined[1][0], z: combined[2][0]},
		{x: combined[0][1], y: combined[1][1], z: combined[2][1]},
		{x: combined[0][2], y: combined[1][2], z: combined[2][2]},
	}
	for i := range cols {
		if math.Abs(math.Sqrt(mdDotCore(cols[i], cols[i]))-1) > 1e-9 {
			t.Fatalf("column %d length=%g", i, math.Sqrt(mdDotCore(cols[i], cols[i])))
		}
		for j := i + 1; j < len(cols); j++ {
			if math.Abs(mdDotCore(cols[i], cols[j])) > 1e-9 {
				t.Fatalf("columns %d,%d not perpendicular: %g", i, j, mdDotCore(cols[i], cols[j]))
			}
		}
	}
}

func TestCoordinateAxisDirectionsMatchActualEulerSliderAxes(t *testing.T) {
	matClose := func(got, want mdMat3, label string) {
		for r := 0; r < 3; r++ {
			for c := 0; c < 3; c++ {
				if math.Abs(got[r][c]-want[r][c]) > 1e-10 {
					t.Fatalf("%s matrix mismatch at %d,%d: got=%g want=%g", label, r, c, got[r][c], want[r][c])
				}
			}
		}
	}
	for _, angles := range [][3]float64{
		{0, 0, 0},
		{0.37, 0.91, 1.42},
		{math.Pi / 2, math.Pi / 3, math.Pi / 4},
	} {
		ax, ay, az := angles[0], angles[1], angles[2]
		axes := mdCoordinateAxisDirections(ax, ay, az)
		rz := mdEulerRotationMatrix(0, 0, az)
		rzy := mdMat3Mul(rz, mdEulerRotationMatrix(0, ay, 0))
		want := [3]mdVec3{
			mdMat3Apply(rzy, mdVec3{x: 1}),
			mdMat3Apply(rz, mdVec3{y: 1}),
			{z: 1},
		}
		for i := 0; i < 3; i++ {
			if math.Abs(axes[i].x-want[i].x) > 1e-12 || math.Abs(axes[i].y-want[i].y) > 1e-12 || math.Abs(axes[i].z-want[i].z) > 1e-12 {
				t.Fatalf("axis %d mismatch: got %+v want %+v", i, axes[i], want[i])
			}
			if math.Abs(mdDotCore(axes[i], axes[i])-1) > 1e-12 {
				t.Fatalf("axis %d is not unit length: %+v", i, axes[i])
			}
		}

		// A tiny change in each slider must equal a world-space rotation about
		// the line displayed for that slider. This catches the A176 bug where
		// only the X overlay coincided with the actual Euler operation axis.
		d := 1e-4
		base := mdEulerRotationMatrix(ax, ay, az)
		baseInv := func() mdMat3 {
			var out mdMat3
			for r := 0; r < 3; r++ {
				for c := 0; c < 3; c++ {
					out[r][c] = base[c][r]
				}
			}
			return out
		}()
		changed := [3]mdMat3{
			mdEulerRotationMatrix(ax+d, ay, az),
			mdEulerRotationMatrix(ax, ay+d, az),
			mdEulerRotationMatrix(ax, ay, az+d),
		}
		for i := 0; i < 3; i++ {
			delta := mdMat3Mul(changed[i], baseInv)
			wantDelta := mdAxisAngleRotationMatrix(axes[i], d)
			matClose(delta, wantDelta, fmt.Sprintf("axis %d", i))
		}
	}
}

func TestAxisOriginScreenDragMapsToXZAndClamps(t *testing.T) {
	start := mdVec3{x: 0.10, y: 0.25, z: -0.20}
	got := mdAxisOriginAfterScreenDrag(start, 50, -25, 200)
	want := mdVec3{x: 0.35, y: 0.25, z: -0.075}
	if math.Abs(got.x-want.x) > 1e-12 || math.Abs(got.y-want.y) > 1e-12 || math.Abs(got.z-want.z) > 1e-12 {
		t.Fatalf("drag mapping mismatch: got=%+v want=%+v", got, want)
	}

	clamped := mdAxisOriginAfterScreenDrag(mdVec3{}, 10000, -10000, 100)
	if clamped.x != mdAxisOriginDragLimit || clamped.z != mdAxisOriginDragLimit {
		t.Fatalf("clamp mismatch: got=%+v limit=%g", clamped, mdAxisOriginDragLimit)
	}

	invalid := mdAxisOriginAfterScreenDrag(start, 10, 10, 0)
	if invalid != start {
		t.Fatalf("invalid preview extent changed origin: got=%+v want=%+v", invalid, start)
	}
}

func TestPivotMoveCompensationKeepsCurrentObjectPose(t *testing.T) {
	ax, ay, az := 0.47, 0.83, 0.21
	startOrigin := mdVec3{x: -0.18, z: 0.11}
	newOrigin := mdVec3{x: 0.27, z: -0.23}
	startTranslation := mdVec3{x: 0.035, y: -0.014, z: 0.062}

	newTranslation := mdCompensatedPivotTranslationCore(startTranslation, startOrigin, newOrigin, ax, ay, az)
	oldDelta := mdRotationPivotDeltaCore(startOrigin, ax, ay, az)
	newDelta := mdRotationPivotDeltaCore(newOrigin, ax, ay, az)
	oldTotal := mdVec3{x: startTranslation.x + oldDelta.x, y: startTranslation.y + oldDelta.y, z: startTranslation.z + oldDelta.z}
	newTotal := mdVec3{x: newTranslation.x + newDelta.x, y: newTranslation.y + newDelta.y, z: newTranslation.z + newDelta.z}

	if math.Abs(oldTotal.x-newTotal.x) > 1e-12 || math.Abs(oldTotal.y-newTotal.y) > 1e-12 || math.Abs(oldTotal.z-newTotal.z) > 1e-12 {
		t.Fatalf("moving pivot changed current object pose: old=%+v new=%+v", oldTotal, newTotal)
	}
}

func TestRotationPivotDeltaLeavesPivotFixed(t *testing.T) {
	origin := mdVec3{x: 0.31, y: 0, z: -0.19}
	ax, ay, az := 0.62, 0.35, 0.91
	rotated := mdMat3Apply(mdEulerRotationMatrix(ax, ay, az), origin)
	delta := mdRotationPivotDeltaCore(origin, ax, ay, az)
	fixed := mdVec3{x: rotated.x + delta.x, y: rotated.y + delta.y, z: rotated.z + delta.z}
	if math.Abs(fixed.x-origin.x) > 1e-12 || math.Abs(fixed.y-origin.y) > 1e-12 || math.Abs(fixed.z-origin.z) > 1e-12 {
		t.Fatalf("pivot was not fixed by D=P-RP: got=%+v want=%+v", fixed, origin)
	}
}

func TestMovedPivotCanBeRebasedWithoutMovingObject(t *testing.T) {
	ax, ay, az := 0.61, 0.52, 0.17
	rotation := mdEulerRotationMatrix(ax, ay, az)
	newOrigin := mdVec3{x: 0.29, y: 0, z: -0.16}
	currentOffset := mdVec3{x: 0.07, y: -0.03, z: 0.04}
	target := mdVec3{x: newOrigin.x - currentOffset.x, y: newOrigin.y - currentOffset.y, z: newOrigin.z - currentOffset.z}
	localPivot := mdMat3ApplyTranspose(rotation, target)
	projected := mdMat3Apply(rotation, localPivot)
	recoveredOffset := mdVec3{x: newOrigin.x - projected.x, y: newOrigin.y - projected.y, z: newOrigin.z - projected.z}
	if math.Abs(recoveredOffset.x-currentOffset.x) > 1e-12 || math.Abs(recoveredOffset.y-currentOffset.y) > 1e-12 || math.Abs(recoveredOffset.z-currentOffset.z) > 1e-12 {
		t.Fatalf("pivot rebasing moved object: got=%+v want=%+v", recoveredOffset, currentOffset)
	}
}

func TestRebasedMovedPivotStaysFixedForEachXYZSlider(t *testing.T) {
	baseAngles := [3]float64{0.61, 0.52, 0.17}
	origin := mdVec3{x: 0.29, y: 0, z: -0.16}
	currentOffset := mdVec3{x: 0.07, y: -0.03, z: 0.04}
	baseRotation := mdEulerRotationMatrix(baseAngles[0], baseAngles[1], baseAngles[2])
	localPivot := mdMat3ApplyTranspose(baseRotation, mdVec3{
		x: origin.x - currentOffset.x,
		y: origin.y - currentOffset.y,
		z: origin.z - currentOffset.z,
	})

	// Rebasing must preserve the exact object translation at the current pose.
	projectedBase := mdMat3Apply(baseRotation, localPivot)
	baseOffset := mdVec3{x: origin.x - projectedBase.x, y: origin.y - projectedBase.y, z: origin.z - projectedBase.z}
	if math.Abs(baseOffset.x-currentOffset.x) > 1e-12 || math.Abs(baseOffset.y-currentOffset.y) > 1e-12 || math.Abs(baseOffset.z-currentOffset.z) > 1e-12 {
		t.Fatalf("rebase translation mismatch: got=%+v want=%+v", baseOffset, currentOffset)
	}

	for axis := 0; axis < 3; axis++ {
		angles := baseAngles
		angles[axis] += 0.73
		rotation := mdEulerRotationMatrix(angles[0], angles[1], angles[2])
		projected := mdMat3Apply(rotation, localPivot)
		offset := mdVec3{x: origin.x - projected.x, y: origin.y - projected.y, z: origin.z - projected.z}
		fixed := mdVec3{x: projected.x + offset.x, y: projected.y + offset.y, z: projected.z + offset.z}
		if math.Abs(fixed.x-origin.x) > 1e-12 || math.Abs(fixed.y-origin.y) > 1e-12 || math.Abs(fixed.z-origin.z) > 1e-12 {
			t.Fatalf("axis %d did not keep moved pivot fixed: got=%+v want=%+v", axis, fixed, origin)
		}
	}
}

func TestObjectCoordinateAxisDirectionsFollowRotatedBasis(t *testing.T) {
	ax, ay, az := 0.73, 1.14, 2.02
	dirs := mdObjectCoordinateAxisDirections(ax, ay, az)
	r := mdEulerRotationMatrix(ax, ay, az)
	want := [3]mdVec3{
		mdMat3Apply(r, mdVec3{x: 1}),
		mdMat3Apply(r, mdVec3{y: 1}),
		mdMat3Apply(r, mdVec3{z: 1}),
	}
	for i := range dirs {
		if math.Abs(dirs[i].x-want[i].x) > 1e-12 || math.Abs(dirs[i].y-want[i].y) > 1e-12 || math.Abs(dirs[i].z-want[i].z) > 1e-12 {
			t.Fatalf("axis %d mismatch got=%+v want=%+v", i, dirs[i], want[i])
		}
		if math.Abs(mdDotCore(dirs[i], dirs[i])-1) > 1e-12 {
			t.Fatalf("axis %d not unit: %+v", i, dirs[i])
		}
	}
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			if math.Abs(mdDotCore(dirs[i], dirs[j])) > 1e-12 {
				t.Fatalf("axes %d,%d not orthogonal: %g", i, j, mdDotCore(dirs[i], dirs[j]))
			}
		}
	}
}

func TestSphereCurveStepsA193AABoost(t *testing.T) {
	if got := mdSphereCurveSteps(true, false, mdSphereGuidesThreeAxes); got != 960 {
		t.Fatalf("smooth guided curve steps=%d want 960", got)
	}
	if got := mdSphereCurveSteps(true, false, mdSphereGuidesNone); got != 2160 {
		t.Fatalf("smooth guide-free curve steps=%d want 2160", got)
	}
}
