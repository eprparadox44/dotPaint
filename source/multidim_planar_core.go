package main

import "math"

const (
	mdPlanarGridNone = iota
	mdPlanarGridMajor
	mdPlanarGridFull
)

const (
	mdCoordinatePlaneXY = iota
	mdCoordinatePlaneXZ
	mdCoordinatePlaneYZ
)

const (
	mdQuadricOneSheet = iota
	mdQuadricTwoSheet
)

const (
	mdConeCutHorizontal = iota
	mdConeCutObliqueEllipse
	mdConeCutGeneratorParallel
	mdConeCutVerticalHyperbola
	mdConeCutThroughApex
)

type mdCurve3 struct {
	points []mdVec3
	closed bool
}

type mdSurfaceMesh3 struct {
	verts []mdVec3
	faces []mdFace
	paths []mdCurve3
}

func mdClosedCurveFromIndices(verts []mdVec3, indices []int) mdCurve3 {
	pts := make([]mdVec3, 0, len(indices)+1)
	for _, idx := range indices {
		if idx >= 0 && idx < len(verts) {
			pts = append(pts, verts[idx])
		}
	}
	return mdCurve3{points: pts, closed: true}
}

func mdOpenCurveFromIndices(verts []mdVec3, indices []int) mdCurve3 {
	pts := make([]mdVec3, 0, len(indices))
	for _, idx := range indices {
		if idx >= 0 && idx < len(verts) {
			pts = append(pts, verts[idx])
		}
	}
	return mdCurve3{points: pts}
}

// mdBuildDiskSurface returns a zero-thickness triangulated disk together with
// polar guide curves.  The guide density is selected by the caller, so the
// same topology supports a plain disk and the polar-grid variant.
func mdBuildDiskSurface(segments, radialRings int, radius float64) mdSurfaceMesh3 {
	if segments < 12 {
		segments = 12
	}
	if radialRings < 1 {
		radialRings = 1
	}
	if radius <= 0 || math.IsNaN(radius) || math.IsInf(radius, 0) {
		radius = 1
	}
	mesh := mdSurfaceMesh3{verts: []mdVec3{{}}}
	rings := make([][]int, radialRings)
	for ring := 1; ring <= radialRings; ring++ {
		r := radius * float64(ring) / float64(radialRings)
		indices := make([]int, segments)
		for i := 0; i < segments; i++ {
			a := 2 * math.Pi * float64(i) / float64(segments)
			indices[i] = len(mesh.verts)
			mesh.verts = append(mesh.verts, mdVec3{x: r * math.Cos(a), y: r * math.Sin(a)})
		}
		rings[ring-1] = indices
	}
	first := rings[0]
	for i := 0; i < segments; i++ {
		n := (i + 1) % segments
		mesh.faces = append(mesh.faces, mdFace{indices: []int{0, first[i], first[n]}})
	}
	for ring := 1; ring < radialRings; ring++ {
		inner, outer := rings[ring-1], rings[ring]
		for i := 0; i < segments; i++ {
			n := (i + 1) % segments
			mesh.faces = append(mesh.faces, mdFace{indices: []int{inner[i], outer[i], outer[n], inner[n]}})
		}
	}
	for _, ring := range rings {
		mesh.paths = append(mesh.paths, mdClosedCurveFromIndices(mesh.verts, ring))
	}
	spokes := 8
	if segments >= 48 {
		spokes = 12
	}
	for i := 0; i < spokes; i++ {
		idx := i * segments / spokes
		curve := mdCurve3{points: []mdVec3{{}}}
		for ring := 0; ring < radialRings; ring++ {
			curve.points = append(curve.points, mesh.verts[rings[ring][idx]])
		}
		mesh.paths = append(mesh.paths, curve)
	}
	return mesh
}

func mdBuildRectangleSurface(halfWidth, halfHeight float64) mdSurfaceMesh3 {
	if halfWidth <= 0 {
		halfWidth = 1
	}
	if halfHeight <= 0 {
		halfHeight = 0.75
	}
	verts := []mdVec3{
		{x: -halfWidth, y: -halfHeight},
		{x: halfWidth, y: -halfHeight},
		{x: halfWidth, y: halfHeight},
		{x: -halfWidth, y: halfHeight},
	}
	return mdSurfaceMesh3{
		verts: verts,
		faces: []mdFace{{indices: []int{0, 1, 2, 3}}},
		paths: []mdCurve3{{points: []mdVec3{verts[0], verts[1], verts[2], verts[3]}, closed: true}},
	}
}

func mdCoordinatePlaneBasis(mode int) (u, v mdVec3, uLabel, vLabel byte) {
	switch mode {
	case mdCoordinatePlaneXZ:
		return mdVec3{x: 1}, mdVec3{z: 1}, 'x', 'z'
	case mdCoordinatePlaneYZ:
		return mdVec3{y: 1}, mdVec3{z: 1}, 'y', 'z'
	default:
		return mdVec3{x: 1}, mdVec3{y: 1}, 'x', 'y'
	}
}

func mdPlanePoint(u, v mdVec3, a, b float64) mdVec3 {
	return mdVec3{x: u.x*a + v.x*b, y: u.y*a + v.y*b, z: u.z*a + v.z*b}
}

func mdBuildCoordinatePlaneSurface(mode int, extent float64) mdSurfaceMesh3 {
	if extent <= 0 {
		extent = 1.2
	}
	u, v, _, _ := mdCoordinatePlaneBasis(mode)
	verts := []mdVec3{
		mdPlanePoint(u, v, -extent, -extent),
		mdPlanePoint(u, v, extent, -extent),
		mdPlanePoint(u, v, extent, extent),
		mdPlanePoint(u, v, -extent, extent),
	}
	return mdSurfaceMesh3{
		verts: verts,
		faces: []mdFace{{indices: []int{0, 1, 2, 3}}},
		paths: []mdCurve3{{points: []mdVec3{verts[0], verts[1], verts[2], verts[3]}, closed: true}},
	}
}

// mdBuildTripleCoordinatePlaneSurface returns the three mutually perpendicular
// XY, XZ, and YZ square planes sharing one origin.  The separate four-vertex
// blocks preserve each face's own winding while allowing the renderer to depth
// sort all three planes together.
func mdBuildTripleCoordinatePlaneSurface(extent float64) mdSurfaceMesh3 {
	if extent <= 0 {
		extent = 1.2
	}
	mesh := mdSurfaceMesh3{}
	for mode := mdCoordinatePlaneXY; mode <= mdCoordinatePlaneYZ; mode++ {
		plane := mdBuildCoordinatePlaneSurface(mode, extent)
		base := len(mesh.verts)
		mesh.verts = append(mesh.verts, plane.verts...)
		for _, face := range plane.faces {
			indices := make([]int, len(face.indices))
			for i, idx := range face.indices {
				indices[i] = base + idx
			}
			mesh.faces = append(mesh.faces, mdFace{indices: indices})
		}
		mesh.paths = append(mesh.paths, plane.paths...)
	}
	return mesh
}

func mdBuildParaboloidSurface(radius, halfHeight float64, radialSegments, angularSegments int) mdSurfaceMesh3 {
	if radialSegments < 4 {
		radialSegments = 4
	}
	if angularSegments < 16 {
		angularSegments = 16
	}
	if radius <= 0 {
		radius = 1
	}
	if halfHeight <= 0 {
		halfHeight = 1
	}
	mesh := mdSurfaceMesh3{verts: []mdVec3{{z: -halfHeight}}}
	rings := make([][]int, radialSegments)
	for ir := 1; ir <= radialSegments; ir++ {
		t := float64(ir) / float64(radialSegments)
		r := radius * t
		z := -halfHeight + 2*halfHeight*t*t
		ring := make([]int, angularSegments)
		for ia := 0; ia < angularSegments; ia++ {
			a := 2 * math.Pi * float64(ia) / float64(angularSegments)
			ring[ia] = len(mesh.verts)
			mesh.verts = append(mesh.verts, mdVec3{x: r * math.Cos(a), y: r * math.Sin(a), z: z})
		}
		rings[ir-1] = ring
	}
	for ia := 0; ia < angularSegments; ia++ {
		n := (ia + 1) % angularSegments
		mesh.faces = append(mesh.faces, mdFace{indices: []int{0, rings[0][ia], rings[0][n]}})
	}
	for ir := 1; ir < radialSegments; ir++ {
		lo, hi := rings[ir-1], rings[ir]
		for ia := 0; ia < angularSegments; ia++ {
			n := (ia + 1) % angularSegments
			mesh.faces = append(mesh.faces, mdFace{indices: []int{lo[ia], hi[ia], hi[n], lo[n]}})
		}
	}
	for ir, ring := range rings {
		if ir == len(rings)-1 || ir%(mdMaxCore(1, radialSegments/5)) == 0 {
			mesh.paths = append(mesh.paths, mdClosedCurveFromIndices(mesh.verts, ring))
		}
	}
	meridians := 8
	for ia := 0; ia < meridians; ia++ {
		idx := ia * angularSegments / meridians
		curve := mdCurve3{points: []mdVec3{mesh.verts[0]}}
		for _, ring := range rings {
			curve.points = append(curve.points, mesh.verts[ring[idx]])
		}
		mesh.paths = append(mesh.paths, curve)
	}
	return mesh
}

func mdMaxCore(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// mdBuildHyperboloidSurface creates either a one-sheet or two-sheet surface of
// revolution.  The eccentricity is used directly through q=sqrt(e²-1).  The
// final radius and height stay bounded by the UI sliders, while q controls the
// waist/tip position and therefore the visible curvature.
func mdBuildHyperboloidSurface(eccentricity float64, family int, radius, halfHeight float64, axialSegments, angularSegments int) mdSurfaceMesh3 {
	if eccentricity <= 1 {
		return mdBuildParaboloidSurface(radius, halfHeight, axialSegments, angularSegments)
	}
	if axialSegments < 8 {
		axialSegments = 8
	}
	if angularSegments < 16 {
		angularSegments = 16
	}
	if radius <= 0 {
		radius = 1
	}
	if halfHeight <= 0 {
		halfHeight = 1
	}
	q := math.Sqrt(math.Max(1e-8, eccentricity*eccentricity-1))
	mesh := mdSurfaceMesh3{}
	addRing := func(z, r float64) []int {
		ring := make([]int, angularSegments)
		for ia := 0; ia < angularSegments; ia++ {
			a := 2 * math.Pi * float64(ia) / float64(angularSegments)
			ring[ia] = len(mesh.verts)
			mesh.verts = append(mesh.verts, mdVec3{x: r * math.Cos(a), y: r * math.Sin(a), z: z})
		}
		return ring
	}
	connect := func(rings [][]int) {
		for iz := 0; iz+1 < len(rings); iz++ {
			lo, hi := rings[iz], rings[iz+1]
			for ia := 0; ia < angularSegments; ia++ {
				n := (ia + 1) % angularSegments
				mesh.faces = append(mesh.faces, mdFace{indices: []int{lo[ia], hi[ia], hi[n], lo[n]}})
			}
		}
	}
	if family == mdQuadricTwoSheet {
		c := halfHeight / eccentricity
		if c > halfHeight*0.985 {
			c = halfHeight * 0.985
		}
		den := math.Sqrt(math.Max(1e-9, (halfHeight/c)*(halfHeight/c)-1))
		branches := make([][]int, 0, 2*(axialSegments+1))
		for _, sign := range []float64{-1, 1} {
			rings := make([][]int, axialSegments+1)
			for iz := 0; iz <= axialSegments; iz++ {
				t := float64(iz) / float64(axialSegments)
				zAbs := c + (halfHeight-c)*t
				r := radius * math.Sqrt(math.Max(0, (zAbs/c)*(zAbs/c)-1)) / den
				rings[iz] = addRing(sign*zAbs, r)
			}
			connect(rings)
			branches = append(branches, rings...)
			for iz, ring := range rings {
				if iz == 0 || iz == len(rings)-1 || iz%(mdMaxCore(1, axialSegments/4)) == 0 {
					mesh.paths = append(mesh.paths, mdClosedCurveFromIndices(mesh.verts, ring))
				}
			}
		}
		_ = branches
		meridians := 8
		for _, sign := range []float64{-1, 1} {
			for ia := 0; ia < meridians; ia++ {
				angleIdx := ia * angularSegments / meridians
				curve := mdCurve3{}
				for iz := 0; iz <= axialSegments; iz++ {
					t := float64(iz) / float64(axialSegments)
					zAbs := c + (halfHeight-c)*t
					r := radius * math.Sqrt(math.Max(0, (zAbs/c)*(zAbs/c)-1)) / den
					a := 2 * math.Pi * float64(angleIdx) / float64(angularSegments)
					curve.points = append(curve.points, mdVec3{x: r * math.Cos(a), y: r * math.Sin(a), z: sign * zAbs})
				}
				mesh.paths = append(mesh.paths, curve)
			}
		}
		return mesh
	}

	// One-sheet: r(z)=R*sqrt(1+(qz/H)^2)/sqrt(1+q^2).  The waist radius is R/e.
	rings := make([][]int, axialSegments+1)
	for iz := 0; iz <= axialSegments; iz++ {
		t := float64(iz) / float64(axialSegments)
		z := -halfHeight + 2*halfHeight*t
		r := radius * math.Sqrt(1+q*q*(z*z)/(halfHeight*halfHeight)) / math.Sqrt(1+q*q)
		rings[iz] = addRing(z, r)
	}
	connect(rings)
	for iz, ring := range rings {
		if iz == 0 || iz == len(rings)-1 || iz%(mdMaxCore(1, axialSegments/5)) == 0 {
			mesh.paths = append(mesh.paths, mdClosedCurveFromIndices(mesh.verts, ring))
		}
	}
	meridians := 8
	for ia := 0; ia < meridians; ia++ {
		idx := ia * angularSegments / meridians
		curve := mdCurve3{}
		for _, ring := range rings {
			curve.points = append(curve.points, mesh.verts[ring[idx]])
		}
		mesh.paths = append(mesh.paths, curve)
	}
	return mesh
}

func mdBuildDoubleConeSurface(radius, halfHeight float64, segments, radialSegments int) mdSurfaceMesh3 {
	if segments < 16 {
		segments = 16
	}
	if radialSegments < 2 {
		radialSegments = 2
	}
	if radius <= 0 {
		radius = 1
	}
	if halfHeight <= 0 {
		halfHeight = 1
	}
	mesh := mdSurfaceMesh3{verts: []mdVec3{{}}}
	for _, sign := range []float64{-1, 1} {
		rings := make([][]int, radialSegments)
		for ir := 1; ir <= radialSegments; ir++ {
			t := float64(ir) / float64(radialSegments)
			r := radius * t
			z := sign * halfHeight * t
			ring := make([]int, segments)
			for ia := 0; ia < segments; ia++ {
				a := 2 * math.Pi * float64(ia) / float64(segments)
				ring[ia] = len(mesh.verts)
				mesh.verts = append(mesh.verts, mdVec3{x: r * math.Cos(a), y: r * math.Sin(a), z: z})
			}
			rings[ir-1] = ring
		}
		for ia := 0; ia < segments; ia++ {
			n := (ia + 1) % segments
			mesh.faces = append(mesh.faces, mdFace{indices: []int{0, rings[0][ia], rings[0][n]}})
		}
		for ir := 1; ir < radialSegments; ir++ {
			lo, hi := rings[ir-1], rings[ir]
			for ia := 0; ia < segments; ia++ {
				n := (ia + 1) % segments
				mesh.faces = append(mesh.faces, mdFace{indices: []int{lo[ia], hi[ia], hi[n], lo[n]}})
			}
		}
		baseCenter := len(mesh.verts)
		mesh.verts = append(mesh.verts, mdVec3{z: sign * halfHeight})
		base := rings[len(rings)-1]
		for ia := 0; ia < segments; ia++ {
			n := (ia + 1) % segments
			if sign > 0 {
				mesh.faces = append(mesh.faces, mdFace{indices: []int{baseCenter, base[ia], base[n]}})
			} else {
				mesh.faces = append(mesh.faces, mdFace{indices: []int{baseCenter, base[n], base[ia]}})
			}
		}
		mesh.paths = append(mesh.paths, mdClosedCurveFromIndices(mesh.verts, base))
		for ia := 0; ia < 8; ia++ {
			idx := ia * segments / 8
			curve := mdCurve3{points: []mdVec3{{}}}
			for _, ring := range rings {
				curve.points = append(curve.points, mesh.verts[ring[idx]])
			}
			mesh.paths = append(mesh.paths, curve)
		}
	}
	return mesh
}

type mdConeCutPlane struct {
	center mdVec3
	u      mdVec3
	v      mdVec3
	half   float64
}

func mdConeCutOffsetFraction(percent int) float64 {
	if percent < -90 {
		percent = -90
	}
	if percent > 90 {
		percent = 90
	}
	return float64(percent) / 100.0
}

func mdConeCutSignedMagnitude(offset, base, span float64) float64 {
	sign := 1.0
	if offset < 0 {
		sign = -1.0
	}
	return sign * (base + span*math.Abs(offset))
}

// mdDoubleConeFaceNormal returns a stable analytic normal for a finite double
// cone.  The generic outward-normal repair compares a face normal with its
// centroid.  That test is numerically ambiguous on a cone because every side
// tangent plane passes through the apex and n·centroid is theoretically zero.
// Tiny floating-point errors therefore flipped neighbouring patches and
// produced the checkerboard/spacecraft shading seen in A188.
func mdDoubleConeFaceNormal(verts []mdVec3, face mdFace, radius, halfHeight float64) mdVec3 {
	if len(face.indices) < 3 || radius <= 0 || halfHeight <= 0 {
		return mdVec3{}
	}
	centroid := mdVec3{}
	minZ, maxZ := math.Inf(1), math.Inf(-1)
	for _, idx := range face.indices {
		if idx < 0 || idx >= len(verts) {
			return mdVec3{}
		}
		p := verts[idx]
		centroid.x += p.x
		centroid.y += p.y
		centroid.z += p.z
		if p.z < minZ {
			minZ = p.z
		}
		if p.z > maxZ {
			maxZ = p.z
		}
	}
	inv := 1.0 / float64(len(face.indices))
	centroid.x *= inv
	centroid.y *= inv
	centroid.z *= inv

	// Base-cap triangles all lie in z=±H and must share one uniform normal.
	eps := math.Max(1e-9, halfHeight*1e-7)
	if maxZ-minZ <= eps && math.Abs(math.Abs(centroid.z)-halfHeight) <= eps*2 {
		if centroid.z < 0 {
			return mdVec3{z: -1}
		}
		return mdVec3{z: 1}
	}

	k := radius / halfHeight
	// Gradient of x²+y²-k²z²=0 points radially out of both nappes.
	return mdNormCore(mdVec3{x: centroid.x, y: centroid.y, z: -k * k * centroid.z})
}

func mdAppendValidCurveSegments(dst []mdCurve3, samples []mdVec3, valid []bool, closedDomain bool) []mdCurve3 {
	if len(samples) != len(valid) || len(samples) == 0 {
		return dst
	}
	segments := []mdCurve3{}
	current := mdCurve3{}
	flush := func() {
		if len(current.points) >= 2 {
			segments = append(segments, current)
		}
		current = mdCurve3{}
	}
	for i, p := range samples {
		if valid[i] {
			current.points = append(current.points, p)
		} else {
			flush()
		}
	}
	flush()
	if closedDomain && len(segments) >= 2 && valid[0] && valid[len(valid)-1] {
		joined := mdCurve3{}
		joined.points = append(joined.points, segments[len(segments)-1].points...)
		joined.points = append(joined.points, segments[0].points...)
		segments[0] = joined
		segments = segments[:len(segments)-1]
	}
	if closedDomain && len(segments) == 1 {
		allValid := true
		for _, ok := range valid {
			if !ok {
				allValid = false
				break
			}
		}
		segments[0].closed = allValid
	}
	return append(dst, segments...)
}

// mdConeCutGeometry returns the visible finite intersection curves and a finite
// display patch for the cutting plane.  The cone is x²+y²=(R/H)²z²,
// |z|<=H.  Presets correspond to circle, ellipse, parabola, hyperbola and the
// degenerate two-line section through the common apex.
func mdConeCutGeometry(mode int, radius, halfHeight float64, offsetPercent, samples int) ([]mdCurve3, mdConeCutPlane) {
	if radius <= 0 {
		radius = 1
	}
	if halfHeight <= 0 {
		halfHeight = 1
	}
	if samples < 120 {
		samples = 120
	}
	k := radius / halfHeight
	offset := mdConeCutOffsetFraction(offsetPercent)
	planeHalf := 1.55 * math.Max(radius, halfHeight)
	curves := []mdCurve3{}
	if mode == mdConeCutVerticalHyperbola || mode == mdConeCutThroughApex {
		d := 0.0
		if mode == mdConeCutVerticalHyperbola {
			d = radius * mdConeCutSignedMagnitude(offset, 0.08, 0.72)
			if math.Abs(d) >= radius*0.95 {
				d = math.Copysign(radius*0.95, d)
			}
		}
		plane := mdConeCutPlane{center: mdVec3{x: d}, u: mdVec3{z: 1}, v: mdVec3{y: 1}, half: planeHalf}
		if mode == mdConeCutThroughApex {
			curves = append(curves,
				mdCurve3{points: []mdVec3{{y: -radius, z: -halfHeight}, {}, {y: radius, z: halfHeight}}},
				mdCurve3{points: []mdVec3{{y: radius, z: -halfHeight}, {}, {y: -radius, z: halfHeight}}},
			)
			return curves, plane
		}
		zMin := math.Abs(d) / k
		for _, signZ := range []float64{-1, 1} {
			plus := mdCurve3{}
			minus := mdCurve3{}
			for i := 0; i <= samples/2; i++ {
				t := float64(i) / float64(samples/2)
				zAbs := zMin + (halfHeight-zMin)*t
				y := math.Sqrt(math.Max(0, k*k*zAbs*zAbs-d*d))
				z := signZ * zAbs
				plus.points = append(plus.points, mdVec3{x: d, y: y, z: z})
				minus.points = append(minus.points, mdVec3{x: d, y: -y, z: z})
			}
			curves = append(curves, plus, minus)
		}
		return curves, plane
	}

	b := halfHeight * mdConeCutSignedMagnitude(offset, 0.10, 0.72)
	ak := 0.0
	switch mode {
	case mdConeCutObliqueEllipse:
		ak = 0.48
	case mdConeCutGeneratorParallel:
		ak = 1.0
	default:
		ak = 0
	}
	a := ak / k
	u := mdVec3{x: 1, z: a}
	ul := math.Sqrt(u.x*u.x + u.z*u.z)
	u.x /= ul
	u.z /= ul
	plane := mdConeCutPlane{center: mdVec3{z: b}, u: u, v: mdVec3{y: 1}, half: planeHalf}
	for _, branch := range []int{1, -1} {
		pts := make([]mdVec3, samples+1)
		valid := make([]bool, samples+1)
		for i := 0; i <= samples; i++ {
			theta := 2 * math.Pi * float64(i) / float64(samples)
			denom := 1 - ak*math.Cos(theta)
			if branch < 0 {
				denom = 1 + ak*math.Cos(theta)
			}
			if math.Abs(denom) < 1e-8 {
				continue
			}
			z := b / denom
			if branch > 0 && z < 0 || branch < 0 && z > 0 || math.Abs(z) > halfHeight {
				continue
			}
			r := k * math.Abs(z)
			pts[i] = mdVec3{x: r * math.Cos(theta), y: r * math.Sin(theta), z: z}
			valid[i] = true
		}
		curves = mdAppendValidCurveSegments(curves, pts, valid, true)
	}
	return curves, plane
}

func mdConeSurfaceResidual(p mdVec3, radius, halfHeight float64) float64 {
	if halfHeight == 0 {
		return math.Inf(1)
	}
	k := radius / halfHeight
	return p.x*p.x + p.y*p.y - k*k*p.z*p.z
}

// mdBuildSaddleSurface creates a bounded hyperbolic paraboloid
// z = halfDepth * ((x/halfWidth)^2 - (y/halfHeight)^2).
// Paths are stored as every constant-y row followed by every constant-x
// column, allowing the Windows renderer to select no/major/full grid density
// without rebuilding the face mesh.
func mdBuildSaddleSurface(halfWidth, halfHeight, halfDepth float64, xSegments, ySegments int) mdSurfaceMesh3 {
	if halfWidth <= 0 || math.IsNaN(halfWidth) || math.IsInf(halfWidth, 0) {
		halfWidth = 1.2
	}
	if halfHeight <= 0 || math.IsNaN(halfHeight) || math.IsInf(halfHeight, 0) {
		halfHeight = 1.0
	}
	if halfDepth <= 0 || math.IsNaN(halfDepth) || math.IsInf(halfDepth, 0) {
		halfDepth = 0.8
	}
	if xSegments < 4 {
		xSegments = 4
	}
	if ySegments < 4 {
		ySegments = 4
	}
	mesh := mdSurfaceMesh3{}
	rows := make([][]int, ySegments+1)
	for iy := 0; iy <= ySegments; iy++ {
		y := -halfHeight + 2*halfHeight*float64(iy)/float64(ySegments)
		row := make([]int, xSegments+1)
		for ix := 0; ix <= xSegments; ix++ {
			x := -halfWidth + 2*halfWidth*float64(ix)/float64(xSegments)
			xn := x / halfWidth
			yn := y / halfHeight
			z := halfDepth * (xn*xn - yn*yn)
			row[ix] = len(mesh.verts)
			mesh.verts = append(mesh.verts, mdVec3{x: x, y: y, z: z})
		}
		rows[iy] = row
	}
	for iy := 0; iy < ySegments; iy++ {
		for ix := 0; ix < xSegments; ix++ {
			mesh.faces = append(mesh.faces, mdFace{indices: []int{
				rows[iy][ix], rows[iy][ix+1], rows[iy+1][ix+1], rows[iy+1][ix],
			}})
		}
	}
	for iy := 0; iy <= ySegments; iy++ {
		mesh.paths = append(mesh.paths, mdOpenCurveFromIndices(mesh.verts, rows[iy]))
	}
	for ix := 0; ix <= xSegments; ix++ {
		indices := make([]int, ySegments+1)
		for iy := 0; iy <= ySegments; iy++ {
			indices[iy] = rows[iy][ix]
		}
		mesh.paths = append(mesh.paths, mdOpenCurveFromIndices(mesh.verts, indices))
	}
	return mesh
}

// mdSaddleFaceNormal returns the analytic, consistently oriented normal of a
// saddle patch. The +Z orientation prevents the generic closed-solid
// centroid test from flipping neighbouring open-surface patches.
func mdSaddleFaceNormal(verts []mdVec3, face mdFace, halfWidth, halfHeight, halfDepth float64) mdVec3 {
	if halfWidth <= 0 {
		halfWidth = 1
	}
	if halfHeight <= 0 {
		halfHeight = 1
	}
	if halfDepth <= 0 {
		halfDepth = 1
	}
	c := mdVec3{}
	count := 0
	for _, idx := range face.indices {
		if idx < 0 || idx >= len(verts) {
			continue
		}
		v := verts[idx]
		c.x += v.x
		c.y += v.y
		c.z += v.z
		count++
	}
	if count > 0 {
		inv := 1 / float64(count)
		c.x *= inv
		c.y *= inv
		c.z *= inv
	}
	n := mdVec3{
		x: -2 * halfDepth * c.x / (halfWidth * halfWidth),
		y: 2 * halfDepth * c.y / (halfHeight * halfHeight),
		z: 1,
	}
	length := math.Sqrt(n.x*n.x + n.y*n.y + n.z*n.z)
	if length <= 1e-12 || math.IsNaN(length) || math.IsInf(length, 0) {
		return mdVec3{z: 1}
	}
	return mdVec3{x: n.x / length, y: n.y / length, z: n.z / length}
}
