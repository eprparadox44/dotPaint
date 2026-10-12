package main

import (
	"math"
	"sort"
)

// mdVec3 and mdFace live in a platform-neutral file so the topology builders
// can be unit-tested on every development host. The Windows renderer consumes
// the same types directly.
type mdVec3 struct{ x, y, z float64 }
type mdFace struct{ indices []int }

type mdMat3 [3][3]float64

func mdMat3Identity() mdMat3 {
	return mdMat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
}

func mdMat3Mul(a, b mdMat3) mdMat3 {
	var out mdMat3
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			out[row][col] = a[row][0]*b[0][col] + a[row][1]*b[1][col] + a[row][2]*b[2][col]
		}
	}
	return out
}

func mdMat3Apply(m mdMat3, v mdVec3) mdVec3 {
	return mdVec3{
		x: m[0][0]*v.x + m[0][1]*v.y + m[0][2]*v.z,
		y: m[1][0]*v.x + m[1][1]*v.y + m[1][2]*v.z,
		z: m[2][0]*v.x + m[2][1]*v.y + m[2][2]*v.z,
	}
}

func mdMat3ApplyTranspose(m mdMat3, v mdVec3) mdVec3 {
	return mdVec3{
		x: m[0][0]*v.x + m[1][0]*v.y + m[2][0]*v.z,
		y: m[0][1]*v.x + m[1][1]*v.y + m[2][1]*v.z,
		z: m[0][2]*v.x + m[1][2]*v.y + m[2][2]*v.z,
	}
}

// mdEulerRotationMatrix matches the renderer's Rz * Ry * Rx order.
func mdEulerRotationMatrix(ax, ay, az float64) mdMat3 {
	cx, sx := math.Cos(ax), math.Sin(ax)
	cy, sy := math.Cos(ay), math.Sin(ay)
	cz, sz := math.Cos(az), math.Sin(az)
	return mdMat3{
		{cz * cy, cz*sy*sx - sz*cx, cz*sy*cx + sz*sx},
		{sz * cy, sz*sy*sx + cz*cx, sz*sy*cx - cz*sx},
		{-sy, cy * sx, cy * cx},
	}
}

// mdCoordinateAxisDirections returns the three actual Euler/gimbal axes used
// when each slider changes while the other two angles are held fixed. For the
// renderer's Rz * Ry * Rx convention those axes are:
//
//	X slider: Rz*Ry*X
//	Y slider: Rz*Y
//	Z slider: world Z
//
// The old overlay drew the fully object-rotated basis R*X/R*Y/R*Z. Only its X
// line coincided with the real X slider axis, which made moved Y and Z axes look
// as though the pivot had not been applied. Drawing the actual operation axes
// makes every displayed line pass through the moved pivot and match its slider.
func mdCoordinateAxisDirections(ax, ay, az float64) [3]mdVec3 {
	_ = ax // Rx does not change the X operation axis; see derivation above.
	rz := mdEulerRotationMatrix(0, 0, az)
	rzy := mdMat3Mul(rz, mdEulerRotationMatrix(0, ay, 0))
	return [3]mdVec3{
		mdMat3Apply(rzy, mdVec3{x: 1}),
		mdMat3Apply(rz, mdVec3{y: 1}),
		mdVec3{z: 1},
	}
}

// mdObjectCoordinateAxisDirections returns the fully rotated local basis of an
// object. A192 uses it as the default visible XYZ frame for every shape, so the
// axes rotate as part of the object until the user explicitly detaches and
// moves the pivot. For flat primitives, X/Y remain in the plane and Z remains
// the normal; for solids the same basis remains the object's local frame.
func mdObjectCoordinateAxisDirections(ax, ay, az float64) [3]mdVec3 {
	r := mdEulerRotationMatrix(ax, ay, az)
	return [3]mdVec3{
		mdMat3Apply(r, mdVec3{x: 1}),
		mdMat3Apply(r, mdVec3{y: 1}),
		mdMat3Apply(r, mdVec3{z: 1}),
	}
}

const mdAxisOriginDragLimit = 0.42

// mdAxisOriginAfterScreenDrag converts a preview-space mouse drag into the
// normalized XZ translation used by the multidimensional preview.  The
// preview is an orthographic XZ view, so horizontal motion moves +X and
// upward screen motion moves +Z.  Y is intentionally preserved because a
// two-dimensional drag cannot determine a depth displacement.  Clamping
// keeps the translated origin recoverable inside the preview window.
func mdAxisOriginAfterScreenDrag(start mdVec3, deltaX, deltaY, previewExtent float64) mdVec3 {
	if previewExtent <= 0 || math.IsNaN(previewExtent) || math.IsInf(previewExtent, 0) {
		return start
	}
	out := start
	out.x += deltaX / previewExtent
	out.z -= deltaY / previewExtent
	if out.x < -mdAxisOriginDragLimit {
		out.x = -mdAxisOriginDragLimit
	} else if out.x > mdAxisOriginDragLimit {
		out.x = mdAxisOriginDragLimit
	}
	if out.z < -mdAxisOriginDragLimit {
		out.z = -mdAxisOriginDragLimit
	} else if out.z > mdAxisOriginDragLimit {
		out.z = mdAxisOriginDragLimit
	}
	return out
}

// mdRotationPivotDeltaCore is the uniform translation D=P-RP that converts
// an origin-centred Euler rotation into a rotation about pivot P.
func mdRotationPivotDeltaCore(origin mdVec3, ax, ay, az float64) mdVec3 {
	rotated := mdMat3Apply(mdEulerRotationMatrix(ax, ay, az), origin)
	return mdVec3{x: origin.x - rotated.x, y: origin.y - rotated.y, z: origin.z - rotated.z}
}

// mdCompensatedPivotTranslationCore preserves the current object pose while
// the pivot is edited. It solves T1+D(P1,R)=T0+D(P0,R).
func mdCompensatedPivotTranslationCore(startTranslation, startOrigin, newOrigin mdVec3, ax, ay, az float64) mdVec3 {
	oldDelta := mdRotationPivotDeltaCore(startOrigin, ax, ay, az)
	newDelta := mdRotationPivotDeltaCore(newOrigin, ax, ay, az)
	return mdVec3{
		x: startTranslation.x + oldDelta.x - newDelta.x,
		y: startTranslation.y + oldDelta.y - newDelta.y,
		z: startTranslation.z + oldDelta.z - newDelta.z,
	}
}

func mdAxisAngleRotationMatrix(axis mdVec3, angle float64) mdMat3 {
	axis = mdNormCore(axis)
	if math.Sqrt(mdDotCore(axis, axis)) <= 1e-12 || math.Abs(angle) <= 1e-12 {
		return mdMat3Identity()
	}
	x, y, z := axis.x, axis.y, axis.z
	c, s := math.Cos(angle), math.Sin(angle)
	t := 1 - c
	return mdMat3{
		{t*x*x + c, t*x*y - s*z, t*x*z + s*y},
		{t*x*y + s*z, t*y*y + c, t*y*z - s*x},
		{t*x*z - s*y, t*y*z + s*x, t*z*z + c},
	}
}

// mdTrackballVector maps a preview pixel to the camera-facing hemisphere.
// Screen X maps to world X, screen vertical maps to world Z, and +Y faces the
// camera. Points outside the virtual sphere are clamped to its rim.
func mdTrackballVector(px, py, width, height int) mdVec3 {
	if width < 2 || height < 2 {
		return mdVec3{y: 1}
	}
	cx := float64(width-1) * 0.5
	cy := float64(height-1) * 0.5
	radius := 0.46 * math.Min(float64(width), float64(height))
	if radius < 1 {
		radius = 1
	}
	x := (float64(px) - cx) / radius
	z := (cy - float64(py)) / radius
	r2 := x*x + z*z
	if r2 >= 1 {
		inv := 1 / math.Sqrt(r2)
		return mdVec3{x: x * inv, z: z * inv}
	}
	return mdNormCore(mdVec3{x: x, y: math.Sqrt(1 - r2), z: z})
}

// mdTrackballDeltaMatrix returns the view-space rotation that carries the
// grabbed sphere point from start to current. Pre-multiplying the existing
// object-to-view matrix makes the preview behave like a conventional arcball.
func mdTrackballDeltaMatrix(start, current mdVec3) mdMat3 {
	start = mdNormCore(start)
	current = mdNormCore(current)
	dot := mdDotCore(start, current)
	if dot > 1 {
		dot = 1
	} else if dot < -1 {
		dot = -1
	}
	axis := mdCrossCore(start, current)
	axisLen := math.Sqrt(mdDotCore(axis, axis))
	if axisLen <= 1e-12 {
		if dot >= 0 {
			return mdMat3Identity()
		}
		axis = mdCrossCore(start, mdVec3{x: 1})
		if math.Sqrt(mdDotCore(axis, axis)) <= 1e-12 {
			axis = mdCrossCore(start, mdVec3{z: 1})
		}
		return mdAxisAngleRotationMatrix(axis, math.Pi)
	}
	return mdAxisAngleRotationMatrix(mdVec3{x: axis.x / axisLen, y: axis.y / axisLen, z: axis.z / axisLen}, math.Atan2(axisLen, dot))
}

// mdEulerFromRotationMatrix extracts one stable Rz * Ry * Rx representation.
// The returned angles are mathematically equivalent to the matrix; callers may
// normalize them to the UI's 0..2π slider range.
func mdEulerFromRotationMatrix(m mdMat3) (ax, ay, az float64) {
	sy := -m[2][0]
	if sy > 1 {
		sy = 1
	} else if sy < -1 {
		sy = -1
	}
	ay = math.Asin(sy)
	cy := math.Cos(ay)
	if math.Abs(cy) > 1e-8 {
		ax = math.Atan2(m[2][1], m[2][2])
		az = math.Atan2(m[1][0], m[0][0])
		return ax, ay, az
	}
	// Gimbal lock: keep a valid equivalent orientation with az=0.
	az = 0
	if sy >= 0 {
		ax = math.Atan2(-m[0][1], m[1][1])
	} else {
		ax = math.Atan2(m[0][1], m[1][1])
	}
	return ax, ay, az
}

func mdNormalizeAnglePositive(angle float64) float64 {
	angle = math.Mod(angle, 2*math.Pi)
	if angle < 0 {
		angle += 2 * math.Pi
	}
	return angle
}

const (
	mdSphereGuidesEquator = iota
	mdSphereGuidesThreeAxes
	mdSphereGuidesNone
)

func mdSphereGuideModeName(mode int) string {
	switch mode {
	case mdSphereGuidesEquator:
		return "적도만"
	case mdSphereGuidesNone:
		return "없음"
	default:
		return "3축"
	}
}

// mdSpherePreviewSupersample retains the platform-neutral sphere policy hook
// used by the core tests. Spheres are not in the expensive-shape set, so a
// smooth sphere stays at 6x both during interaction and at rest. Guide mode no
// longer changes the outer AA scale.
func mdSpherePreviewSupersample(smooth, interactive bool, guideMode int) int {
	_ = guideMode
	return mdPreviewSupersamplePolicy("구", smooth, interactive)
}

// mdSphereMeshSteps controls only the shaded surface tessellation. A153 draws
// the visible outline and guide ellipses analytically, so surface density can
// remain moderate while the edge AA unit becomes one half as large.

// mdSurfaceDepthFadeFactor returns the remaining line contrast after n
// independent covering surface sheets for 3D parametric surfaces such as the
// torus and Mobius strip. Each additional covering sheet removes 70 percent and leaves 30 percent of
// the remaining contrast, with the same 8-percent floor retained so the
// deepest lines never disappear completely in transparent modes.
func mdSurfaceDepthFadeFactor(layers int) float64 {
	if layers <= 0 {
		return 1
	}
	factor := math.Pow(0.30, float64(layers))
	if factor < 0.08 {
		return 0.08
	}
	return factor
}

func mdSphereMeshSteps(smooth, interactive bool, guideMode int) (latSteps, lonSteps int) {
	if interactive {
		return 16, 24
	}
	if guideMode == mdSphereGuidesNone {
		if smooth {
			return 48, 72
		}
		return 32, 48
	}
	if smooth {
		return 32, 48
	}
	return 16, 24
}

// mdSphereCurveSteps is the sub-pixel curve resolution used by the analytic
// projected silhouette and exact principal-section ellipses. In guide-free
// mode A153 doubles the A152 unit density (half the segment size).
func mdSphereCurveSteps(smooth, interactive bool, guideMode int) int {
	if interactive {
		return 144
	}
	if guideMode == mdSphereGuidesNone {
		if smooth {
			return 2160
		}
		return 720
	}
	if smooth {
		return 960
	}
	return 360
}

// mdBuildEllipsoidMesh uses one shared vertex at each pole instead of one
// coincident pole vertex per longitude. This removes the dark pinwheel/crown
// artifact caused by degenerate quads while preserving a closed convex mesh.
func mdBuildEllipsoidMesh(rx, ry, rz float64, latSteps, lonSteps int) (verts []mdVec3, faces []mdFace, southPole, northPole int) {
	if latSteps < 4 {
		latSteps = 4
	}
	if latSteps%2 != 0 {
		latSteps++
	}
	if lonSteps < 8 {
		lonSteps = 8
	}
	southPole = 0
	verts = append(verts, mdVec3{z: -rz})
	// Add one dedicated micro-ring beside each pole. The tiny cap is rendered
	// separately with a shared pole normal, while the ordinary latitude grid
	// starts one full step away. This makes the cap sub-pixel at common sizes
	// instead of leaving a visible circular crown.
	baseStep := math.Pi / float64(latSteps)
	ringPhis := make([]float64, 0, latSteps+1)
	ringPhis = append(ringPhis, -math.Pi/2+0.25*baseStep)
	for i := 1; i < latSteps; i++ {
		ringPhis = append(ringPhis, -math.Pi/2+baseStep*float64(i))
	}
	ringPhis = append(ringPhis, math.Pi/2-0.25*baseStep)
	rings := make([][]int, len(ringPhis))
	for i, phi := range ringPhis {
		ring := make([]int, lonSteps)
		for j := 0; j < lonSteps; j++ {
			theta := 2 * math.Pi * float64(j) / float64(lonSteps)
			ring[j] = len(verts)
			verts = append(verts, mdVec3{
				x: rx * math.Cos(phi) * math.Cos(theta),
				y: ry * math.Cos(phi) * math.Sin(theta),
				z: rz * math.Sin(phi),
			})
		}
		rings[i] = ring
	}
	northPole = len(verts)
	verts = append(verts, mdVec3{z: rz})

	first := rings[0]
	last := rings[len(rings)-1]
	for j := 0; j < lonSteps; j++ {
		n := (j + 1) % lonSteps
		faces = append(faces, mdFace{indices: []int{southPole, first[n], first[j]}})
	}
	for i := 0; i+1 < len(rings); i++ {
		a, b := rings[i], rings[i+1]
		for j := 0; j < lonSteps; j++ {
			n := (j + 1) % lonSteps
			// Triangles sort more reliably than long, highly skewed quads when an
			// ellipsoid axis is compressed strongly.
			faces = append(faces,
				mdFace{indices: []int{a[j], a[n], b[n]}},
				mdFace{indices: []int{a[j], b[n], b[j]}},
			)
		}
	}
	for j := 0; j < lonSteps; j++ {
		n := (j + 1) % lonSteps
		faces = append(faces, mdFace{indices: []int{northPole, last[j], last[n]}})
	}
	return verts, faces, southPole, northPole
}

func mdEllipsoidNormalAt(p mdVec3, rx, ry, rz float64) mdVec3 {
	if rx == 0 || ry == 0 || rz == 0 {
		return mdVec3{}
	}
	return mdNormCore(mdVec3{x: p.x / (rx * rx), y: p.y / (ry * ry), z: p.z / (rz * rz)})
}

// mdFaceOutwardNormal returns a unit face normal that points away from the
// origin. All current closed 3D solids are centered at the origin, so this
// also repairs accidental winding reversals before lighting and visibility
// tests use the normal.
func mdFaceOutwardNormal(verts []mdVec3, face mdFace) mdVec3 {
	if len(face.indices) < 3 {
		return mdVec3{}
	}
	p0 := verts[face.indices[0]]
	p1 := verts[face.indices[1]]
	p2 := verts[face.indices[2]]
	n := mdCrossCore(mdSubCore(p1, p0), mdSubCore(p2, p0))
	centroid := mdVec3{}
	for _, idx := range face.indices {
		centroid.x += verts[idx].x
		centroid.y += verts[idx].y
		centroid.z += verts[idx].z
	}
	inv := 1.0 / float64(len(face.indices))
	centroid.x *= inv
	centroid.y *= inv
	centroid.z *= inv
	if mdDotCore(n, centroid) < 0 {
		n.x, n.y, n.z = -n.x, -n.y, -n.z
	}
	l := math.Sqrt(mdDotCore(n, n))
	if l == 0 {
		return n
	}
	return mdVec3{n.x / l, n.y / l, n.z / l}
}

func mdSubCore(a, b mdVec3) mdVec3 { return mdVec3{a.x - b.x, a.y - b.y, a.z - b.z} }
func mdCrossCore(a, b mdVec3) mdVec3 {
	return mdVec3{a.y*b.z - a.z*b.y, a.z*b.x - a.x*b.z, a.x*b.y - a.y*b.x}
}
func mdDotCore(a, b mdVec3) float64 { return a.x*b.x + a.y*b.y + a.z*b.z }

func mdNormCore(v mdVec3) mdVec3 {
	l := math.Sqrt(mdDotCore(v, v))
	if l == 0 {
		return v
	}
	return mdVec3{v.x / l, v.y / l, v.z / l}
}

// The ray vector is the physical direction in which parallel light travels.
// Lambert shading uses the opposite direction, from the surface toward the
// incoming light. Keeping both definitions here prevents a position-vector /
// ray-vector sign mix-up from returning in individual shape renderers.
func mdDefaultLightRayDirection() mdVec3 { return mdNormCore(mdVec3{1, -1, -1}) }
func mdDefaultIncomingLightDirection() mdVec3 {
	r := mdDefaultLightRayDirection()
	return mdVec3{-r.x, -r.y, -r.z}
}

// Platform-neutral regular-polyhedron builders keep face winding consistent
// and make the light-direction regression tests available on every host.
func mdBuildTetrahedronGeometry() ([]mdVec3, []mdFace) {
	verts := []mdVec3{{1, 1, 1}, {1, -1, -1}, {-1, 1, -1}, {-1, -1, 1}}
	faces := []mdFace{{[]int{0, 1, 2}}, {[]int{0, 3, 1}}, {[]int{0, 2, 3}}, {[]int{1, 3, 2}}}
	return verts, faces
}

func mdBuildCubeGeometry() ([]mdVec3, []mdFace) {
	verts := []mdVec3{{-1, -1, -1}, {1, -1, -1}, {1, 1, -1}, {-1, 1, -1}, {-1, -1, 1}, {1, -1, 1}, {1, 1, 1}, {-1, 1, 1}}
	faces := []mdFace{{[]int{0, 3, 2, 1}}, {[]int{4, 5, 6, 7}}, {[]int{0, 1, 5, 4}}, {[]int{1, 2, 6, 5}}, {[]int{2, 3, 7, 6}}, {[]int{3, 0, 4, 7}}}
	return verts, faces
}

func mdBuildOctahedronGeometry() ([]mdVec3, []mdFace) {
	verts := []mdVec3{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
	faces := []mdFace{{[]int{0, 2, 4}}, {[]int{2, 1, 4}}, {[]int{1, 3, 4}}, {[]int{3, 0, 4}}, {[]int{0, 5, 2}}, {[]int{2, 5, 1}}, {[]int{1, 5, 3}}, {[]int{3, 5, 0}}}
	return verts, faces
}

// mdBuildConeGeometry creates a triangulated smooth-cone approximation.
// Ring vertices occupy [0, segments), followed by the apex and base center.
func mdBuildConeGeometry(segments int, radius, height float64) (verts []mdVec3, faces []mdFace, apexIndex, baseCenterIndex int) {
	if segments < 3 {
		segments = 3
	}
	if radius < 0 {
		radius = -radius
	}
	if height < 0 {
		height = -height
	}
	verts = make([]mdVec3, 0, segments+2)
	for i := 0; i < segments; i++ {
		t := 2 * math.Pi * float64(i) / float64(segments)
		verts = append(verts, mdVec3{radius * math.Cos(t), radius * math.Sin(t), -height / 2})
	}
	apexIndex = len(verts)
	verts = append(verts, mdVec3{0, 0, height / 2})
	baseCenterIndex = len(verts)
	verts = append(verts, mdVec3{0, 0, -height / 2})

	faces = make([]mdFace, 0, segments*2)
	for i := 0; i < segments; i++ {
		n := (i + 1) % segments
		faces = append(faces, mdFace{indices: []int{i, n, apexIndex}})
		faces = append(faces, mdFace{indices: []int{baseCenterIndex, n, i}})
	}
	return verts, faces, apexIndex, baseCenterIndex
}

// mdBuildPolygonalPyramidGeometry creates a right pyramid whose apex lies over
// the center of a regular polygonal base. The final vertex is the apex.
func mdBuildPolygonalPyramidGeometry(sides int, radius, height float64) (verts []mdVec3, faces []mdFace, apexIndex int) {
	if sides < 3 {
		sides = 3
	}
	if radius < 0 {
		radius = -radius
	}
	if height < 0 {
		height = -height
	}
	verts = make([]mdVec3, 0, sides+1)
	// A quarter-turn places a vertex at the top in the unrotated screen-facing
	// view, matching the existing regular-polyhedron visual convention.
	for i := 0; i < sides; i++ {
		t := 2*math.Pi*float64(i)/float64(sides) + math.Pi/2
		verts = append(verts, mdVec3{radius * math.Cos(t), radius * math.Sin(t), -height / 2})
	}
	apexIndex = len(verts)
	verts = append(verts, mdVec3{0, 0, height / 2})

	faces = make([]mdFace, 0, sides+1)
	for i := 0; i < sides; i++ {
		n := (i + 1) % sides
		faces = append(faces, mdFace{indices: []int{i, n, apexIndex}})
	}
	base := make([]int, sides)
	// Reverse the base winding; the renderer also corrects winding against the
	// interior origin, but this keeps the topology independently well formed.
	for i := 0; i < sides; i++ {
		base[i] = sides - 1 - i
	}
	faces = append(faces, mdFace{indices: base})
	return verts, faces, apexIndex
}

const (
	mdSurfaceGridNone = iota
	mdSurfaceGridMajor
	mdSurfaceGridFull
)

func mdSurfaceGridModeName(mode int) string {
	switch mode {
	case mdSurfaceGridNone:
		return "격자선 없음"
	case mdSurfaceGridFull:
		return "전체 격자선"
	default:
		return "주요 격자선"
	}
}

// mdTorusMeshSteps keeps live rotation responsive and restores a dense final
// mesh after the thumb is released.  The final smooth mesh is fine enough for
// the projected inner and outer silhouettes to remain clean at A149's 3x AA.
func mdTorusMeshSteps(smooth, interactive bool) (majorSteps, minorSteps int) {
	if interactive {
		return 32, 16
	}
	if smooth {
		return 96, 48
	}
	return 64, 32
}

// mdMobiusMeshSteps gives the long direction more samples because one full
// trip also carries the half twist.  Across-strip subdivisions are kept lower
// so the surface stays fast enough for live rotation.
func mdMobiusMeshSteps(smooth, interactive bool) (aroundSteps, acrossSteps int) {
	if interactive {
		return 40, 8
	}
	if smooth {
		return 144, 24
	}
	return 96, 16
}

// mdBuildTorusMesh constructs a closed orientable torus.  The parameter order
// is chosen so every quad winds toward the analytic outward normal.
func mdBuildTorusMesh(majorRadius, minorRadius float64, majorSteps, minorSteps int) (verts []mdVec3, faces []mdFace) {
	if majorRadius < 0 {
		majorRadius = -majorRadius
	}
	if minorRadius < 0 {
		minorRadius = -minorRadius
	}
	if majorRadius < 1e-6 {
		majorRadius = 1e-6
	}
	if minorRadius < 1e-6 {
		minorRadius = 1e-6
	}
	if majorSteps < 8 {
		majorSteps = 8
	}
	if minorSteps < 6 {
		minorSteps = 6
	}
	idx := func(i, j int) int {
		i = (i%majorSteps + majorSteps) % majorSteps
		j = (j%minorSteps + minorSteps) % minorSteps
		return i*minorSteps + j
	}
	verts = make([]mdVec3, 0, majorSteps*minorSteps)
	for i := 0; i < majorSteps; i++ {
		u := 2 * math.Pi * float64(i) / float64(majorSteps)
		cu, su := math.Cos(u), math.Sin(u)
		for j := 0; j < minorSteps; j++ {
			v := 2 * math.Pi * float64(j) / float64(minorSteps)
			cv, sv := math.Cos(v), math.Sin(v)
			rho := majorRadius + minorRadius*cv
			verts = append(verts, mdVec3{x: rho * cu, y: rho * su, z: minorRadius * sv})
		}
	}
	faces = make([]mdFace, 0, majorSteps*minorSteps)
	for i := 0; i < majorSteps; i++ {
		for j := 0; j < minorSteps; j++ {
			faces = append(faces, mdFace{indices: []int{
				idx(i, j), idx(i+1, j), idx(i+1, j+1), idx(i, j+1),
			}})
		}
	}
	return verts, faces
}

// mdMobiusPoint is the standard embedded Mobius strip.  v is the signed
// across-strip coordinate, with |v| <= halfWidth.
func mdMobiusPoint(radius, u, v float64) mdVec3 {
	half := 0.5 * u
	c, s := math.Cos(u), math.Sin(u)
	ch, sh := math.Cos(half), math.Sin(half)
	rho := radius + v*ch
	return mdVec3{x: rho * c, y: rho * s, z: v * sh}
}

// mdBuildMobiusMesh closes the u seam with reversed across-strip indexing:
// (2π,v) is identified with (0,-v).  The result has one boundary component,
// Euler characteristic zero, and no artificial duplicate seam vertices.
func mdBuildMobiusMesh(radius, halfWidth float64, aroundSteps, acrossSteps int) (verts []mdVec3, faces []mdFace) {
	if radius < 0 {
		radius = -radius
	}
	if halfWidth < 0 {
		halfWidth = -halfWidth
	}
	if radius < 1e-6 {
		radius = 1e-6
	}
	if halfWidth < 1e-6 {
		halfWidth = 1e-6
	}
	if aroundSteps < 12 {
		aroundSteps = 12
	}
	if acrossSteps < 2 {
		acrossSteps = 2
	}
	cols := acrossSteps + 1
	idx := func(i, j int) int { return i*cols + j }
	verts = make([]mdVec3, 0, aroundSteps*cols)
	for i := 0; i < aroundSteps; i++ {
		u := 2 * math.Pi * float64(i) / float64(aroundSteps)
		for j := 0; j <= acrossSteps; j++ {
			v := -halfWidth + 2*halfWidth*float64(j)/float64(acrossSteps)
			verts = append(verts, mdMobiusPoint(radius, u, v))
		}
	}
	faces = make([]mdFace, 0, aroundSteps*acrossSteps)
	for i := 0; i < aroundSteps; i++ {
		next := i + 1
		wrapped := false
		if next == aroundSteps {
			next = 0
			wrapped = true
		}
		for j := 0; j < acrossSteps; j++ {
			j0, j1 := j, j+1
			if wrapped {
				j0 = acrossSteps - j
				j1 = acrossSteps - (j + 1)
			}
			faces = append(faces, mdFace{indices: []int{
				idx(i, j), idx(next, j0), idx(next, j1), idx(i, j+1),
			}})
		}
	}
	return verts, faces
}

func mdFaceDepthAtXZCore(point mdVec3, verts []mdVec3, face mdFace) (float64, bool) {
	if len(face.indices) < 3 {
		return 0, false
	}
	a := verts[face.indices[0]]
	front := math.Inf(-1)
	found := false
	for i := 1; i+1 < len(face.indices); i++ {
		b := verts[face.indices[i]]
		c := verts[face.indices[i+1]]
		den := (b.z-c.z)*(a.x-c.x) + (c.x-b.x)*(a.z-c.z)
		if math.Abs(den) < 1e-12 {
			continue
		}
		l1 := ((b.z-c.z)*(point.x-c.x) + (c.x-b.x)*(point.z-c.z)) / den
		l2 := ((c.z-a.z)*(point.x-c.x) + (a.x-c.x)*(point.z-c.z)) / den
		l3 := 1 - l1 - l2
		if l1 < -1e-8 || l2 < -1e-8 || l3 < -1e-8 {
			continue
		}
		y := l1*a.y + l2*b.y + l3*c.y
		if !found || y > front {
			front = y
			found = true
		}
	}
	return front, found
}

// mdUniqueSurfaceCoverCountCore counts distinct surface sheets in front of a
// point along the +Y orthographic viewing ray. Triangulated halves meeting at
// one sheet depth are merged, so the rear outer point of a ring torus reports
// three covering sheets rather than duplicate triangle hits.
func mdUniqueSurfaceCoverCountIgnoringCore(point mdVec3, verts []mdVec3, faces []mdFace, candidates []int, depthEps float64, ignoredFaces []int) int {
	if depthEps < 1e-9 {
		depthEps = 1e-9
	}
	depths := make([]float64, 0, 8)
	isIgnored := func(fi int) bool {
		for _, ignored := range ignoredFaces {
			if fi == ignored {
				return true
			}
		}
		return false
	}
	visit := func(fi int) {
		if fi < 0 || fi >= len(faces) {
			return
		}
		if isIgnored(fi) {
			return
		}
		y, hit := mdFaceDepthAtXZCore(point, verts, faces[fi])
		if hit && y > point.y+depthEps {
			depths = append(depths, y)
		}
	}
	if candidates == nil {
		for fi := range faces {
			visit(fi)
		}
	} else {
		for _, fi := range candidates {
			visit(fi)
		}
	}
	if len(depths) == 0 {
		return 0
	}
	sort.Float64s(depths)
	mergeEps := math.Max(depthEps*2.5, 1e-7)
	layers := 1
	last := depths[0]
	for _, d := range depths[1:] {
		if math.Abs(d-last) > mergeEps {
			layers++
			last = d
		}
	}
	return layers
}

func mdUniqueSurfaceCoverCountCore(point mdVec3, verts []mdVec3, faces []mdFace, candidates []int, depthEps float64) int {
	return mdUniqueSurfaceCoverCountIgnoringCore(point, verts, faces, candidates, depthEps, nil)
}
