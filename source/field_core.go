package main

import (
	"math"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// field_core.go contains the platform-neutral electrostatic/magnetostatic model
// and the small raster renderer used by the Windows field editor.
//
// Manually dragged objects are constrained to the world XZ plane (Y=0).
// Explicit polyhedron layouts may mark point charges as Volumetric and retain
// a real Y coordinate.  This keeps ordinary editing simple while supporting
// exact tetrahedral, octahedral and cubic three-dimensional charge arrays.

type fieldDimension int

const (
	fieldDimension2D fieldDimension = iota
	fieldDimension3D
)

type fieldDisplayMode int

const (
	fieldDisplayEquipotential fieldDisplayMode = iota
	fieldDisplayElectricLines
	fieldDisplayElectricVectors
	fieldDisplayMagneticLines
	fieldDisplayMagneticVectors
	fieldDisplayGravityPotential
	fieldDisplayGravityLines
	fieldDisplayGravityVectors
)

func (m fieldDisplayMode) electric() bool {
	return m == fieldDisplayEquipotential || m == fieldDisplayElectricLines || m == fieldDisplayElectricVectors
}

func (m fieldDisplayMode) magnetic() bool {
	return m == fieldDisplayMagneticLines || m == fieldDisplayMagneticVectors
}

func (m fieldDisplayMode) gravity() bool {
	return m == fieldDisplayGravityPotential || m == fieldDisplayGravityLines || m == fieldDisplayGravityVectors
}

func (m fieldDisplayMode) scalarPotential() bool {
	return m == fieldDisplayEquipotential || m == fieldDisplayGravityPotential
}

func (m fieldDisplayMode) vectorArrows() bool {
	return m == fieldDisplayElectricVectors || m == fieldDisplayMagneticVectors || m == fieldDisplayGravityVectors
}

func (m fieldDisplayMode) domain() int {
	switch {
	case m.electric():
		return 0
	case m.magnetic():
		return 1
	case m.gravity():
		return 2
	default:
		return -1
	}
}

type fieldObjectKind int

const (
	fieldObjectSphere fieldObjectKind = iota
	fieldObjectRod
	fieldObjectPlane
	fieldObjectBarMagnet
	fieldObjectCurrentWire
	fieldObjectHorseshoeMagnet
	fieldObjectCurrentLoop
	fieldObjectFerromagnet
	fieldObjectSolidMassSphere
	fieldObjectHollowMassSphere
	fieldObjectMassPlane
	fieldObjectMassRing
)

type fieldMaterial int

const (
	fieldMaterialInsulator fieldMaterial = iota
	fieldMaterialConductor
)

const fieldMaxObjects = 8

type fieldVec3 struct{ X, Y, Z float64 }

func (a fieldVec3) add(b fieldVec3) fieldVec3 { return fieldVec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a fieldVec3) sub(b fieldVec3) fieldVec3 { return fieldVec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a fieldVec3) mul(s float64) fieldVec3   { return fieldVec3{a.X * s, a.Y * s, a.Z * s} }
func (a fieldVec3) dot(b fieldVec3) float64   { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a fieldVec3) cross(b fieldVec3) fieldVec3 {
	return fieldVec3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}
func (a fieldVec3) len2() float64 { return a.dot(a) }
func (a fieldVec3) len() float64  { return math.Sqrt(a.len2()) }
func (a fieldVec3) norm() fieldVec3 {
	l := a.len()
	if l < 1e-12 || math.IsNaN(l) || math.IsInf(l, 0) {
		return fieldVec3{}
	}
	return a.mul(1 / l)
}

type fieldObject struct {
	Kind       fieldObjectKind
	Material   fieldMaterial
	X, Y, Z    float64
	Charge     float64 // electric charge or magnetic/current strength, -4..+4
	Radius     float64 // sphere radius
	Length     float64 // rod/magnet length
	Size       float64 // square plate side
	RotX       float64
	RotY       float64
	RotZ       float64
	Volumetric bool    // true only for explicit 3-D special point-charge layouts
	Spin       float64 // black-hole/Kerr-style rotation control, 0..0.99
}

func fieldDefaultObject(kind fieldObjectKind) fieldObject {
	o := fieldObject{Kind: kind, Material: fieldMaterialInsulator, Charge: 1, Radius: .48, Length: 1.35, Size: 1.25}
	if kind == fieldObjectBarMagnet || kind == fieldObjectCurrentWire || kind == fieldObjectHorseshoeMagnet || kind == fieldObjectCurrentLoop || kind == fieldObjectFerromagnet {
		o.Material = fieldMaterialConductor
	}
	if kind == fieldObjectFerromagnet {
		o.Charge = 2.0 // relative magnetic susceptibility / saturation control
		o.Radius = .58
	}
	if kind == fieldObjectSolidMassSphere || kind == fieldObjectHollowMassSphere {
		o.Charge = 3.0 // mass, 0..10 in normalized educational units
		o.Radius = .55
		o.Spin = 0.0
	}
	if kind == fieldObjectMassPlane {
		o.Charge = 3.0
		o.Size = 1.5
	}
	if kind == fieldObjectMassRing {
		o.Charge = 3.0
		o.Radius = .75
	}
	if kind == fieldObjectPlane {
		// Object centres are constrained to Y=0.  A zero-rotation plate lies in
		// that same XZ placement plane, so a newly placed point charge can
		// accidentally start inside the sheet and most streamlines terminate
		// immediately.  Stand a new plate upright instead: it spans YZ, its
		// normal is +X, and a charge can be placed cleanly on either side while
		// the default 0,0,0 camera sees an exact vertical edge.
		o.RotZ = math.Pi / 2
	}
	if kind == fieldObjectHorseshoeMagnet {
		o.Length = 1.65
	}
	if kind == fieldObjectCurrentLoop || kind == fieldObjectMassRing {
		if kind == fieldObjectCurrentLoop {
			o.Radius = .72
		}

		// The loop lies in its local YZ plane.  Turn its normal (+X) toward the
		// default camera depth (+Y), so the newly added loop is seen face-on.
		o.RotZ = math.Pi / 2
	}
	return o
}

func (o fieldObject) electric() bool {
	return o.Kind == fieldObjectSphere || o.Kind == fieldObjectRod || o.Kind == fieldObjectPlane
}

func (o fieldObject) magnetic() bool {
	return o.Kind == fieldObjectBarMagnet || o.Kind == fieldObjectCurrentWire ||
		o.Kind == fieldObjectHorseshoeMagnet || o.Kind == fieldObjectCurrentLoop || o.Kind == fieldObjectFerromagnet
}

func (o fieldObject) magneticSource() bool {
	return o.Kind == fieldObjectBarMagnet || o.Kind == fieldObjectCurrentWire ||
		o.Kind == fieldObjectHorseshoeMagnet || o.Kind == fieldObjectCurrentLoop
}

func (o fieldObject) gravity() bool {
	return o.Kind == fieldObjectSolidMassSphere || o.Kind == fieldObjectHollowMassSphere ||
		o.Kind == fieldObjectMassPlane || o.Kind == fieldObjectMassRing
}

// fieldDeleteObjectAt removes one object and returns a selection that is valid
// for the shortened slice.  Keeping this mutation in the platform-neutral core
// makes deletion semantics testable without a Win32 message loop.
func fieldDeleteObjectAt(objects []fieldObject, selected int) ([]fieldObject, int, bool) {
	if selected < 0 || selected >= len(objects) {
		return objects, selected, false
	}
	copy(objects[selected:], objects[selected+1:])
	objects = objects[:len(objects)-1]
	if selected >= len(objects) {
		selected = len(objects) - 1
	}
	return objects, selected, true
}

type fieldScene struct {
	Dimension     fieldDimension
	Display       fieldDisplayMode
	Objects       []fieldObject
	RotX          float64
	RotY          float64
	RotZ          float64
	Density       int     // force-line/equipotential density, 4..24
	VectorDensity int     // vector-arrow lattice multiplier in tenths, 10..20 = 1.0x..2.0x
	ViewDistance  float64 // observer distance, 0.1..3.0; apparent magnification is 1/distance
	Antialias     bool

	// precise3DLines is an internal render-phase flag. Interactive previews keep
	// the cached GPU vector grid for responsiveness, while a settled 3-D field-line
	// preview or export retraces the physical lines from the exact evaluator.
	// It is intentionally not a user setting and is mixed into line-cache keys only.
	precise3DLines bool
	cancel         *fieldRenderCancellation
}

// fieldRenderCancellation lets a newer editor request stop obsolete physical
// work instead of merely discarding it after completion.  The token is carried
// by the private scene snapshot and is deliberately excluded from every
// physical cache key.
type fieldRenderCancellation struct {
	flag atomic.Bool
}

func fieldNewRenderCancellation() *fieldRenderCancellation {
	return &fieldRenderCancellation{}
}

func (c *fieldRenderCancellation) Cancel() {
	if c != nil {
		c.flag.Store(true)
	}
}

func (c *fieldRenderCancellation) Cancelled() bool {
	return c != nil && c.flag.Load()
}

func (s fieldScene) cancelled() bool {
	return s.cancel != nil && s.cancel.Cancelled()
}

func fieldDefaultScene() fieldScene {
	// Start from the coordinate-system reference view.  Previous oblique
	// defaults made a uniformly sampled point charge look one-sided in the
	// projection and also made a newly placed plate appear unintentionally
	// tilted.
	return fieldScene{Dimension: fieldDimension2D, Display: fieldDisplayElectricLines, Density: 10, VectorDensity: 10, ViewDistance: 1.0, RotX: 0, RotY: 0, RotZ: 0, Antialias: true}
}

const (
	fieldViewDistanceMin = 0.1
	fieldViewDistanceMax = 3.0

	fieldExtendedSizeSliderMax = 1000
	fieldExtendedSizeKneePos   = 900
	fieldExtendedSizeKneeValue = 20.0
)

// fieldExtendedSizeFromSlider keeps the ordinary 0.35..20 range across the
// first 90% of the trackbar, then reserves the final 10% for the unusually
// large plate/rod range requested for wide diagrams.
func fieldExtendedSizeFromSlider(kind fieldObjectKind, pos int) float64 {
	pos = max(0, min(fieldExtendedSizeSliderMax, pos))
	maximum := fieldPlaneSizeMax
	if kind == fieldObjectRod {
		maximum = fieldRodLengthMax
	}
	if pos <= fieldExtendedSizeKneePos {
		t := float64(pos) / float64(fieldExtendedSizeKneePos)
		return fieldPlaneSizeMin + (fieldExtendedSizeKneeValue-fieldPlaneSizeMin)*t
	}
	t := float64(pos-fieldExtendedSizeKneePos) / float64(fieldExtendedSizeSliderMax-fieldExtendedSizeKneePos)
	return fieldExtendedSizeKneeValue + (maximum-fieldExtendedSizeKneeValue)*t
}

func fieldExtendedSizeSliderPosition(kind fieldObjectKind, value float64) int {
	maximum := fieldPlaneSizeMax
	if kind == fieldObjectRod {
		maximum = fieldRodLengthMax
	}
	value = fieldClamp(value, fieldPlaneSizeMin, maximum)
	if value <= fieldExtendedSizeKneeValue {
		t := (value - fieldPlaneSizeMin) / (fieldExtendedSizeKneeValue - fieldPlaneSizeMin)
		return int(math.Round(t * fieldExtendedSizeKneePos))
	}
	t := (value - fieldExtendedSizeKneeValue) / (maximum - fieldExtendedSizeKneeValue)
	return fieldExtendedSizeKneePos + int(math.Round(t*float64(fieldExtendedSizeSliderMax-fieldExtendedSizeKneePos)))
}

const (
	fieldSphereRadiusMin = .18
	fieldSphereRadiusMax = 1.15

	fieldRodLengthMin = .35
	// 1.2.7: only the right end of the electric rod slider is extended.
	// The previous maximum was 2.75, so 27.5 is exactly ten times larger.
	fieldRodLengthMax = 27.5

	fieldPlaneSizeMin = .35
	// 1.2.7: only the right end of the electric plate slider is extended.
	// The previous maximum was 2.45, so 24.5 is exactly ten times larger.
	fieldPlaneSizeMax = 24.5

	fieldMagneticLengthMax = 2.75

	fieldCurrentLoopRadiusMin = .25
	fieldCurrentLoopRadiusMax = 1.45
)

func fieldClampScene(s *fieldScene) {
	if s == nil {
		return
	}
	if s.ViewDistance == 0 {
		s.ViewDistance = 1
	}
	s.ViewDistance = fieldClamp(s.ViewDistance, fieldViewDistanceMin, fieldViewDistanceMax)
	if s.Density < 4 {
		s.Density = 4
	}
	if s.Density > 24 {
		s.Density = 24
	}
	if s.VectorDensity < 10 {
		s.VectorDensity = 10
	}
	if s.VectorDensity > 20 {
		s.VectorDensity = 20
	}
	if len(s.Objects) > fieldMaxObjects {
		s.Objects = s.Objects[:fieldMaxObjects]
	}
	for i := range s.Objects {
		o := &s.Objects[i]
		o.X = fieldClamp(o.X, -2.65, 2.65)
		if o.Volumetric {
			o.Y = fieldClamp(o.Y, -2.65, 2.65)
		} else {
			o.Y = 0 // planar placement
		}
		o.Z = fieldClamp(o.Z, -2.65, 2.65)
		if o.Kind == fieldObjectFerromagnet {
			o.Charge = fieldClamp(o.Charge, 0, 4)
		} else if o.gravity() {
			o.Charge = fieldClamp(o.Charge, 0, fieldGravityMassMax)
		} else {
			o.Charge = fieldClamp(o.Charge, -4, 4)
		}
		o.Spin = fieldClamp(o.Spin, 0, .99)
		if o.Kind == fieldObjectCurrentLoop {
			o.Radius = fieldClamp(o.Radius, fieldCurrentLoopRadiusMin, fieldCurrentLoopRadiusMax)
		} else if o.Kind == fieldObjectFerromagnet {
			o.Radius = fieldClamp(o.Radius, .25, 1.25)
		} else if o.Kind == fieldObjectSolidMassSphere || o.Kind == fieldObjectHollowMassSphere {
			o.Radius = fieldClamp(o.Radius, fieldGravityRadiusMin, fieldGravityRadiusMax)
		} else if o.Kind == fieldObjectMassRing {
			o.Radius = fieldClamp(o.Radius, fieldGravityRingRadiusMin, fieldGravityRingRadiusMax)
		} else {
			o.Radius = fieldClamp(o.Radius, fieldSphereRadiusMin, fieldSphereRadiusMax)
		}
		switch o.Kind {
		case fieldObjectRod:
			o.Length = fieldClamp(o.Length, fieldRodLengthMin, fieldRodLengthMax)
		case fieldObjectBarMagnet, fieldObjectCurrentWire, fieldObjectHorseshoeMagnet:
			o.Length = fieldClamp(o.Length, fieldRodLengthMin, fieldMagneticLengthMax)
		default:
			o.Length = fieldClamp(o.Length, fieldRodLengthMin, fieldMagneticLengthMax)
		}
		if o.Kind == fieldObjectPlane {
			o.Size = fieldClamp(o.Size, fieldPlaneSizeMin, fieldPlaneSizeMax)
		} else if o.Kind == fieldObjectMassPlane {
			o.Size = fieldClamp(o.Size, fieldPlaneSizeMin, 4.0)
		} else {
			o.Size = fieldClamp(o.Size, fieldPlaneSizeMin, 2.45)
		}
		o.RotX = fieldSnapRotationRadians(o.RotX)
		o.RotY = fieldSnapRotationRadians(o.RotY)
		o.RotZ = fieldSnapRotationRadians(o.RotZ)
	}
	s.RotX = fieldSnapRotationRadians(s.RotX)
	s.RotY = fieldSnapRotationRadians(s.RotY)
	s.RotZ = fieldSnapRotationRadians(s.RotZ)
}

func fieldClamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func fieldRotateXYZ(v fieldVec3, rx, ry, rz float64) fieldVec3 {
	cx, sx := math.Cos(rx), math.Sin(rx)
	cy, sy := math.Cos(ry), math.Sin(ry)
	cz, sz := math.Cos(rz), math.Sin(rz)
	v = fieldVec3{v.X, v.Y*cx - v.Z*sx, v.Y*sx + v.Z*cx}
	v = fieldVec3{v.X*cy + v.Z*sy, v.Y, -v.X*sy + v.Z*cy}
	return fieldVec3{v.X*cz - v.Y*sz, v.X*sz + v.Y*cz, v.Z}
}

func fieldObjectBasis(o fieldObject) (fieldVec3, fieldVec3, fieldVec3) {
	ex := fieldRotateXYZ(fieldVec3{X: 1}, o.RotX, o.RotY, o.RotZ).norm()
	ey := fieldRotateXYZ(fieldVec3{Y: 1}, o.RotX, o.RotY, o.RotZ).norm()
	ez := fieldRotateXYZ(fieldVec3{Z: 1}, o.RotX, o.RotY, o.RotZ).norm()
	return ex, ey, ez
}

// fieldInverseRotateXYZ converts a camera-space vector back to world space.
// fieldRotateXYZ applies Rx, then Ry, then Rz, so the inverse must undo those
// rotations in the reverse order.
func fieldInverseRotateXYZ(v fieldVec3, rx, ry, rz float64) fieldVec3 {
	cx, sx := math.Cos(rx), math.Sin(rx)
	cy, sy := math.Cos(ry), math.Sin(ry)
	cz, sz := math.Cos(rz), math.Sin(rz)
	// Rz^-1
	v = fieldVec3{v.X*cz + v.Y*sz, -v.X*sz + v.Y*cz, v.Z}
	// Ry^-1
	v = fieldVec3{v.X*cy - v.Z*sy, v.Y, v.X*sy + v.Z*cy}
	// Rx^-1
	return fieldVec3{v.X, v.Y*cx + v.Z*sx, -v.Y*sx + v.Z*cx}
}

func fieldObjectCenter(o fieldObject) fieldVec3 { return fieldVec3{o.X, o.Y, o.Z} }

func fieldDistancePointSegment(p, a, b fieldVec3) float64 {
	ab := b.sub(a)
	denom := ab.len2()
	if denom < 1e-18 {
		return p.sub(a).len()
	}
	t := fieldClamp(p.sub(a).dot(ab)/denom, 0, 1)
	return p.sub(a.add(ab.mul(t))).len()
}

func fieldHorseshoeRadius(o fieldObject) float64 {
	return math.Max(.085, math.Min(.19, o.Length*.09))
}

// fieldHorseshoeCenterline describes a uniformly magnetised U-shaped rod in
// world space.  The point order runs from the south pole to the north pole for
// positive strength: down the left leg, around the lower semicircle, and up
// the right leg.  Reversing Charge reverses the magnetisation without changing
// the drawn U geometry.
func fieldHorseshoeCenterline(o fieldObject, legSegments, arcSegments int) []fieldVec3 {
	legSegments = max(2, legSegments)
	arcSegments = max(6, arcSegments)
	ex, _, ez := fieldObjectBasis(o)
	c := fieldObjectCenter(o)
	h := math.Max(fieldRodLengthMin, o.Length)
	halfWidth := h * .32
	topZ := h * .50
	curveZ := h * -.18
	points := make([]fieldVec3, 0, legSegments*2+arcSegments+1)
	for i := 0; i <= legSegments; i++ {
		t := float64(i) / float64(legSegments)
		z := topZ + (curveZ-topZ)*t
		points = append(points, c.add(ex.mul(-halfWidth)).add(ez.mul(z)))
	}
	for i := 1; i <= arcSegments; i++ {
		a := math.Pi + math.Pi*float64(i)/float64(arcSegments)
		points = append(points, c.add(ex.mul(halfWidth*math.Cos(a))).add(ez.mul(curveZ+halfWidth*math.Sin(a))))
	}
	for i := 1; i <= legSegments; i++ {
		t := float64(i) / float64(legSegments)
		z := curveZ + (topZ-curveZ)*t
		points = append(points, c.add(ex.mul(halfWidth)).add(ez.mul(z)))
	}
	return points
}

func fieldHorseshoePoleGeometry(o fieldObject) (south, north, southOut, northOut fieldVec3) {
	points := fieldHorseshoeCenterline(o, 5, 10)
	if len(points) < 3 {
		return fieldVec3{}, fieldVec3{}, fieldVec3{}, fieldVec3{}
	}
	startTangent := points[1].sub(points[0]).norm()
	endTangent := points[len(points)-1].sub(points[len(points)-2]).norm()
	startOut := startTangent.mul(-1)
	endOut := endTangent
	if o.Charge >= 0 {
		return points[0], points[len(points)-1], startOut, endOut
	}
	return points[len(points)-1], points[0], endOut, startOut
}

func fieldObjectAxisExtent(o fieldObject) fieldVec3 {
	ex, ey, ez := fieldObjectBasis(o)
	abs := func(v float64) float64 { return math.Abs(v) }
	switch o.Kind {
	case fieldObjectSphere, fieldObjectFerromagnet, fieldObjectSolidMassSphere, fieldObjectHollowMassSphere:
		return fieldVec3{o.Radius, o.Radius, o.Radius}
	case fieldObjectRod, fieldObjectBarMagnet:
		radius := math.Max(.10, math.Min(.22, o.Length*.11))
		half := o.Length * .5
		return fieldVec3{
			abs(ex.X)*half + (abs(ey.X)+abs(ez.X))*radius,
			abs(ex.Y)*half + (abs(ey.Y)+abs(ez.Y))*radius,
			abs(ex.Z)*half + (abs(ey.Z)+abs(ez.Z))*radius,
		}
	case fieldObjectHorseshoeMagnet:
		r := fieldHorseshoeRadius(o)
		hx, hy, hz := o.Length*.32+r, r, o.Length*.52+r
		return fieldVec3{
			abs(ex.X)*hx + abs(ey.X)*hy + abs(ez.X)*hz,
			abs(ex.Y)*hx + abs(ey.Y)*hy + abs(ez.Y)*hz,
			abs(ex.Z)*hx + abs(ey.Z)*hy + abs(ez.Z)*hz,
		}
	case fieldObjectPlane, fieldObjectMassPlane:
		half := o.Size * .5
		thickness := .08
		return fieldVec3{
			(abs(ex.X)+abs(ez.X))*half + abs(ey.X)*thickness,
			(abs(ex.Y)+abs(ez.Y))*half + abs(ey.Y)*thickness,
			(abs(ex.Z)+abs(ez.Z))*half + abs(ey.Z)*thickness,
		}
	case fieldObjectCurrentWire:
		return fieldVec3{abs(ey.X)*1.15 + .12, abs(ey.Y)*1.15 + .12, abs(ey.Z)*1.15 + .12}
	case fieldObjectCurrentLoop, fieldObjectMassRing:
		tube := math.Max(.045, o.Radius*.055)
		hx, radial := tube, o.Radius+tube
		return fieldVec3{
			abs(ex.X)*hx + (abs(ey.X)+abs(ez.X))*radial,
			abs(ex.Y)*hx + (abs(ey.Y)+abs(ez.Y))*radial,
			abs(ex.Z)*hx + (abs(ey.Z)+abs(ez.Z))*radial,
		}
	}
	return fieldVec3{.5, .5, .5}
}

// fieldSceneSpan is the half-width of the physical/viewing cube needed to
// contain every rotated object.  Earlier versions hard-coded 2.8, so a large
// plate could cover the whole sampling cube or put its corners behind the
// camera and then vanish completely.
func fieldSceneSpan(scene fieldScene) float64 {
	extent := 0.0
	for _, o := range scene.Objects {
		c := fieldObjectCenter(o)
		e := fieldObjectAxisExtent(o)
		extent = math.Max(extent, math.Abs(c.X)+e.X)
		extent = math.Max(extent, math.Abs(c.Y)+e.Y)
		extent = math.Max(extent, math.Abs(c.Z)+e.Z)
	}
	return math.Max(2.8, extent*1.10+.20)
}

func fieldSceneNumericalInfinityRadius(scene fieldScene) float64 {
	return math.Max(9.5, fieldSceneSpan(scene)*2.35)
}

func fieldEquipotentialSamplingSpan(scene fieldScene) float64 {
	gridCenter := fieldEquipotentialGridCenter(scene)
	fullExtent := 0.0
	for _, o := range scene.Objects {
		active := o.electric()
		if scene.Display.gravity() {
			active = o.gravity()
		}
		if !active {
			continue
		}
		c := fieldObjectCenter(o).sub(gridCenter)
		e := fieldObjectAxisExtent(o)
		fullExtent = math.Max(fullExtent, math.Abs(c.X)+e.X)
		fullExtent = math.Max(fullExtent, math.Abs(c.Y)+e.Y)
		fullExtent = math.Max(fullExtent, math.Abs(c.Z)+e.Z)
	}
	full := math.Max(2.8, fullExtent*1.10+.20)
	if scene.Display.gravity() {
		return full
	}
	plateIndex, _, _, maxDistance, ok := fieldWidePlateChargeSide(scene)
	if !ok {
		return full
	}
	extent := 0.0
	for i, o := range scene.Objects {
		if !o.electric() || (i != plateIndex && math.Abs(o.Charge) < 1e-10) {
			continue
		}
		c := fieldObjectCenter(o).sub(gridCenter)
		extent = math.Max(extent, math.Abs(c.X))
		extent = math.Max(extent, math.Abs(c.Y))
		extent = math.Max(extent, math.Abs(c.Z))
	}
	// The plate may be twenty units wide while all non-trivial curvature is
	// confined to a few charge-to-plane distances around its centre.  Sampling
	// the entire plate made each marching-cubes cell enormous and tore the
	// surface.  Keep the camera wide, but solve the membrane in this local box.
	local := math.Max(3.2, extent+maxDistance*3.2+.45)
	return math.Min(full, local)
}

type fieldChargeSample struct {
	Pos    fieldVec3
	Normal fieldVec3
	Q      float64
	Soft   float64
	// Gate restricts an analytic image-pair basis to its physical half-space.
	// The plane equation is dot(GateNormal,p) >= GateOffset.
	GateNormal fieldVec3
	GateOffset float64
	Gate       bool
	// Self is the collocation coefficient produced by one panel's own
	// uniformly distributed charge.  A zero value falls back to the softened
	// point kernel.  Large conducting plates need this exact diagonal term;
	// otherwise a few enormous point-panels can satisfy the centres while the
	// potential between them oscillates wildly.
	Self      float64
	Object    int
	Conductor bool
}

func fieldSphereSurface(o fieldObject, objectIndex, count int) []fieldChargeSample {
	if count < 12 {
		count = 12
	}
	out := make([]fieldChargeSample, 0, count)
	c := fieldObjectCenter(o)
	golden := math.Pi * (3 - math.Sqrt(5))
	for i := 0; i < count; i++ {
		y := 1 - 2*(float64(i)+.5)/float64(count)
		r := math.Sqrt(math.Max(0, 1-y*y))
		a := golden * float64(i)
		n := fieldVec3{r * math.Cos(a), y, r * math.Sin(a)}
		out = append(out, fieldChargeSample{Pos: c.add(n.mul(o.Radius)), Normal: n, Soft: math.Max(.075, o.Radius*.34), Object: objectIndex})
	}
	return out
}

func fieldRodSurface(o fieldObject, objectIndex, axial, circum int) []fieldChargeSample {
	if axial < 4 {
		axial = 4
	}
	if circum < 6 {
		circum = 6
	}
	ex, ey, ez := fieldObjectBasis(o)
	c := fieldObjectCenter(o)
	r := math.Max(.10, math.Min(.22, o.Length*.11))
	out := make([]fieldChargeSample, 0, axial*circum+2*circum)
	for i := 0; i < axial; i++ {
		t := -0.5 + (float64(i)+.5)/float64(axial)
		base := c.add(ex.mul(o.Length * t))
		for j := 0; j < circum; j++ {
			a := 2 * math.Pi * float64(j) / float64(circum)
			n := ey.mul(math.Cos(a)).add(ez.mul(math.Sin(a))).norm()
			out = append(out, fieldChargeSample{Pos: base.add(n.mul(r)), Normal: n, Soft: math.Max(.065, r*.78), Object: objectIndex})
		}
	}
	for _, sign := range []float64{-1, 1} {
		base := c.add(ex.mul(sign * o.Length * .5))
		n := ex.mul(sign)
		for j := 0; j < circum; j++ {
			a := 2 * math.Pi * float64(j) / float64(circum)
			rr := r * .62
			p := base.add(ey.mul(rr * math.Cos(a))).add(ez.mul(rr * math.Sin(a)))
			out = append(out, fieldChargeSample{Pos: p, Normal: n, Soft: math.Max(.065, r*.75), Object: objectIndex})
		}
	}
	return out
}

func fieldPlaneSurface(o fieldObject, objectIndex, grid int) []fieldChargeSample {
	if grid < 4 {
		grid = 4
	}
	ex, ey, ez := fieldObjectBasis(o)
	_ = ey
	c := fieldObjectCenter(o)
	n := ex.cross(ez).norm()
	out := make([]fieldChargeSample, 0, grid*grid)
	cell := o.Size / float64(grid)
	// Potential at the centre of a uniformly charged square panel, per unit
	// total panel charge:
	//   4 asinh(1) / cell = 3.52549... / cell.
	// This is the analytic singular self integral, not an arbitrary softening.
	self := 4 * math.Asinh(1) / math.Max(cell, 1e-9)
	for iz := 0; iz < grid; iz++ {
		v := -0.5 + (float64(iz)+.5)/float64(grid)
		for ix := 0; ix < grid; ix++ {
			u := -0.5 + (float64(ix)+.5)/float64(grid)
			p := c.add(ex.mul(u * o.Size)).add(ez.mul(v * o.Size))
			out = append(out, fieldChargeSample{Pos: p, Normal: n, Soft: math.Max(.07, cell*.58), Self: self, Object: objectIndex})
		}
	}
	return out
}

// fieldInsulatingPlaneSurface integrates a uniformly charged finite plate by
// tensor-product two-point Gauss quadrature on square panels.  The previous
// midpoint rule placed one softened point charge at every panel centre; near a
// large plate its checkerboard phase changed whenever the plate crossed the
// scalar lattice.  Two-point Gauss quadrature is fourth-order for smooth panel
// contributions and, unlike arbitrary midpoint softening, is a genuine area
// integral approximation.  Every returned point has equal weight because the
// two Gauss-Legendre weights are both one.
func fieldInsulatingPlaneSurface(o fieldObject, objectIndex, axisSamples int) []fieldChargeSample {
	axisSamples = max(8, axisSamples)
	if axisSamples%2 != 0 {
		axisSamples++
	}
	panels := axisSamples / 2
	ex, _, ez := fieldObjectBasis(o)
	c := fieldObjectCenter(o)
	n := ex.cross(ez).norm()
	panelSize := o.Size / float64(panels)
	gauss := 1 / math.Sqrt(3)
	out := make([]fieldChargeSample, 0, axisSamples*axisSamples)
	for pz := 0; pz < panels; pz++ {
		cz := -o.Size*.5 + (float64(pz)+.5)*panelSize
		for px := 0; px < panels; px++ {
			cx := -o.Size*.5 + (float64(px)+.5)*panelSize
			for _, gz := range []float64{-gauss, gauss} {
				for _, gx := range []float64{-gauss, gauss} {
					u := cx + gx*panelSize*.5
					v := cz + gz*panelSize*.5
					out = append(out, fieldChargeSample{
						Pos: c.add(ex.mul(u)).add(ez.mul(v)), Normal: n,
						// One quadrature point represents one quarter-panel.  A small
						// cell-scaled core regularises only the exact on-sheet sample;
						// it is much smaller than the old 0.58-cell midpoint blur.
						Soft: math.Max(.010, panelSize*.085), Object: objectIndex,
					})
				}
			}
		}
	}
	return out
}

func fieldInsulatingPlaneAxisSamples(size float64, quality int) int {
	// Preview and export use the same 24 samples per axis for a large plate, so
	// changing render quality no longer changes the represented charge density.
	// Quality 2 may rise to 32 only for unusually large final exports.
	target, minimum, maximum := .92, 12, 24
	if quality > 1 {
		target, minimum, maximum = .72, 16, 32
	}
	if quality <= 1 && size >= 8 {
		return 24
	}
	axis := int(math.Ceil(math.Max(size, fieldPlaneSizeMin) / target))
	axis = max(minimum, min(maximum, axis))
	if axis%2 != 0 {
		axis++
	}
	return min(maximum, axis)
}

func fieldPlaneGridForQuality(size float64, quality int) int {
	// The old fixed 4/5/6 grid left a 20-unit plate represented by cells over
	// three units wide.  The solve could be equal only at those sparse centres,
	// producing the torn, perforated membranes seen around a point charge.
	// Keep small plates cheap, but cap large preview/export grids at sizes that
	// remain practical for the dense boundary-element solve.
	targetCell, maximum, minimum := 1.45, 14, 4
	if quality > 0 {
		targetCell, maximum, minimum = 1.10, 18, 5
	}
	if quality > 1 {
		targetCell, maximum, minimum = .88, 22, 6
	}
	grid := int(math.Ceil(math.Max(size, fieldPlaneSizeMin) / targetCell))
	return max(minimum, min(maximum, grid))
}

func fieldObjectSurface(o fieldObject, objectIndex, quality int) []fieldChargeSample {
	switch o.Kind {
	case fieldObjectSphere:
		count := 20
		if quality > 0 {
			count = 26
		}
		if quality > 1 {
			count = 34
		}
		return fieldSphereSurface(o, objectIndex, count)
	case fieldObjectRod, fieldObjectBarMagnet:
		axial, circum := 4, 6
		if quality > 0 {
			axial = 5
		}
		if quality > 1 {
			axial, circum = 6, 8
		}
		return fieldRodSurface(o, objectIndex, axial, circum)
	case fieldObjectPlane, fieldObjectMassPlane:
		grid := fieldPlaneGridForQuality(o.Size, quality)
		return fieldPlaneSurface(o, objectIndex, grid)
	}
	return nil
}

func fieldElectricalSamples(scene fieldScene, quality int) []fieldChargeSample {
	fieldClampScene(&scene)
	fixed := make([]fieldChargeSample, 0, 128)
	conductors := make([]fieldChargeSample, 0, 128)
	conductorObjects := make([]int, 0, len(scene.Objects))
	targets := make(map[int]float64)
	for oi, o := range scene.Objects {
		if !o.electric() {
			continue
		}
		// A non-conducting sphere is the visual handle for an ideal point
		// charge.  A conducting sphere retains surface panels for induction.
		if o.Kind == fieldObjectSphere && o.Material != fieldMaterialConductor {
			fixed = append(fixed, fieldChargeSample{
				Pos: fieldObjectCenter(o), Q: o.Charge,
				Soft: math.Max(.008, o.Radius*.012), Object: oi,
			})
			continue
		}
		var surf []fieldChargeSample
		if o.Kind == fieldObjectPlane && o.Material != fieldMaterialConductor {
			surf = fieldInsulatingPlaneSurface(o, oi, fieldInsulatingPlaneAxisSamples(o.Size, quality))
		} else {
			surf = fieldObjectSurface(o, oi, quality)
		}
		if len(surf) == 0 {
			continue
		}
		if o.Material == fieldMaterialConductor {
			conductorObjects = append(conductorObjects, oi)
			targets[oi] = o.Charge
			for i := range surf {
				surf[i].Conductor = true
			}
			conductors = append(conductors, surf...)
		} else {
			q := o.Charge / float64(len(surf))
			for i := range surf {
				surf[i].Q = q
			}
			fixed = append(fixed, surf...)
		}
	}
	return fieldSolveConductorPanels(scene, fixed, conductors, conductorObjects, targets, quality)
}

func fieldSolveConductorPanels(scene fieldScene, fixed, conductors []fieldChargeSample, conductorObjects []int, targets map[int]float64, quality int) []fieldChargeSample {
	if len(conductors) == 0 {
		return fixed
	}
	conductorMap := make(map[int]int, len(conductorObjects))
	for i, object := range conductorObjects {
		conductorMap[object] = i
	}
	unknown := len(conductors)
	cCount := len(conductorObjects)
	n := unknown + cCount
	A := make([][]float64, n)
	b := make([]float64, n)
	for i := range A {
		A[i] = make([]float64, n)
	}
	for i, pi := range conductors {
		vFixed := fieldPotentialFromSamples(fixed, pi.Pos)
		b[i] = -vFixed
		for j, pj := range conductors {
			if i == j && pj.Self > 0 {
				A[i][j] = pj.Self
			} else {
				d := pi.Pos.sub(pj.Pos)
				A[i][j] = 1 / math.Sqrt(d.len2()+pj.Soft*pj.Soft*.12)
			}
		}
		ci := conductorMap[pi.Object]
		A[i][unknown+ci] = -1
	}
	for ci, oi := range conductorObjects {
		row := unknown + ci
		for j, p := range conductors {
			if p.Object == oi {
				A[row][j] = 1
			}
		}
		b[row] = targets[oi]
	}
	x, ok := fieldSolveLinear(A, b)
	if !ok {
		counts := make(map[int]int)
		for _, p := range conductors {
			counts[p.Object]++
		}
		for i := range conductors {
			conductors[i].Q = targets[conductors[i].Object] / float64(max(counts[conductors[i].Object], 1))
		}
		return fieldExpandSolvedPlanePanels(scene, append(fixed, conductors...), quality)
	}
	for i := range conductors {
		q := x[i]
		if math.IsNaN(q) || math.IsInf(q, 0) {
			q = 0
		}
		conductors[i].Q = q
	}
	for _, oi := range conductorObjects {
		total, count := 0.0, 0
		for i := range conductors {
			if conductors[i].Object == oi {
				total += conductors[i].Q
				count++
			}
		}
		if count == 0 {
			continue
		}
		correction := (targets[oi] - total) / float64(count)
		for i := range conductors {
			if conductors[i].Object == oi {
				conductors[i].Q += correction
			}
		}
	}
	return fieldExpandSolvedPlanePanels(scene, append(fixed, conductors...), quality)
}

// fieldWidePlateImageSamples uses an exact image-charge decomposition for the
// sharply varying central induction on a very wide plate, then solves only the
// slowly varying finite-plate/neutral-charge correction with BEM panels.
// This is the large-plate asymptotic solution of Poisson/Laplace equations, not
// a hand-drawn shape heuristic.
func fieldWidePlateImageSamples(scene fieldScene, quality int) ([]fieldChargeSample, bool) {
	plateIndex, normal, side, _, ok := fieldWidePlateChargeSide(scene)
	if !ok {
		return nil, false
	}
	plate := scene.Objects[plateIndex]
	center := fieldObjectCenter(plate)
	gateNormal := normal.mul(side)
	gateOffset := center.dot(gateNormal)
	totalPointCharge := 0.0
	fixed := make([]fieldChargeSample, 0, 16)
	for i, o := range scene.Objects {
		if !o.electric() || i == plateIndex || math.Abs(o.Charge) < 1e-10 {
			continue
		}
		if o.Kind != fieldObjectSphere || o.Material == fieldMaterialConductor {
			return nil, false
		}
		pos := fieldObjectCenter(o)
		rel := pos.sub(center)
		d := rel.dot(normal)
		soft := math.Max(.008, o.Radius*.012)
		// The real+image pair is an exact Maxwell solution only in the source
		// half-space.  Gate both terms there; on the back side the finite-plate
		// correction panels supply the physical fringing field without exposing
		// a fictitious image singularity.
		fixed = append(fixed,
			fieldChargeSample{Pos: pos, Q: o.Charge, Soft: soft, Object: i, GateNormal: gateNormal, GateOffset: gateOffset, Gate: true},
			fieldChargeSample{Pos: pos.sub(normal.mul(2 * d)), Q: -o.Charge, Soft: soft, Object: -1, GateNormal: gateNormal, GateOffset: gateOffset, Gate: true},
		)
		totalPointCharge += o.Charge
	}
	panels := fieldPlaneSurface(plate, plateIndex, fieldPlaneGridForQuality(plate.Size, quality))
	for i := range panels {
		panels[i].Conductor = true
	}
	// The gated image pair carries the central induced total -sum(q) on the
	// source side. The finite-plate correction therefore carries Qplate+sum(q),
	// preserving the user's physical conductor charge exactly.
	targets := map[int]float64{plateIndex: plate.Charge + totalPointCharge}
	return fieldSolveConductorPanels(scene, fixed, panels, []int{plateIndex}, targets, quality), true
}

func fieldWidePlateChargeSide(scene fieldScene) (plateIndex int, normal fieldVec3, side, maxDistance float64, ok bool) {
	plateIndex = -1
	for i, o := range scene.Objects {
		if o.electric() && o.Material == fieldMaterialConductor {
			if o.Kind != fieldObjectPlane || plateIndex >= 0 {
				return -1, fieldVec3{}, 0, 0, false
			}
			plateIndex = i
		}
	}
	if plateIndex < 0 {
		return -1, fieldVec3{}, 0, 0, false
	}
	plate := scene.Objects[plateIndex]
	ex, _, ez := fieldObjectBasis(plate)
	normal = ex.cross(ez).norm()
	center := fieldObjectCenter(plate)
	pointCount := 0
	for i, o := range scene.Objects {
		if !o.electric() || i == plateIndex || math.Abs(o.Charge) < 1e-10 {
			continue
		}
		if o.Kind != fieldObjectSphere || o.Material == fieldMaterialConductor {
			return -1, fieldVec3{}, 0, 0, false
		}
		rel := fieldObjectCenter(o).sub(center)
		d := rel.dot(normal)
		if math.Abs(d) < math.Max(.12, o.Radius*.35) {
			return -1, fieldVec3{}, 0, 0, false
		}
		s := math.Copysign(1, d)
		if side == 0 {
			side = s
		} else if side != s {
			return -1, fieldVec3{}, 0, 0, false
		}
		if math.Abs(rel.dot(ex)) > plate.Size*.40 || math.Abs(rel.dot(ez)) > plate.Size*.40 {
			return -1, fieldVec3{}, 0, 0, false
		}
		maxDistance = math.Max(maxDistance, math.Abs(d))
		pointCount++
	}
	if pointCount == 0 || plate.Size < math.Max(6, maxDistance*6) {
		return -1, fieldVec3{}, 0, 0, false
	}
	return plateIndex, normal, side, maxDistance, true
}

func fieldEquipotentialElectricalSamples(scene fieldScene, quality int) []fieldChargeSample {
	if samples, ok := fieldWidePlateImageSamples(scene, quality); ok {
		return samples
	}
	return fieldElectricalSamples(scene, quality)
}

// fieldExpandSolvedPlanePanels turns each solved square panel charge into a
// small equal-weight quadrature stencil.  The BEM unknown remains one charge
// per panel, but the vacuum field no longer treats that entire charge as a
// single softened point at the panel centre.  This removes the checkerboard
// potential oscillation between sparse collocation points on a wide plate.
func fieldExpandSolvedPlanePanels(scene fieldScene, samples []fieldChargeSample, quality int) []fieldChargeSample {
	countPerAxis := 2
	_ = quality
	out := make([]fieldChargeSample, 0, len(samples)*2)
	for _, s := range samples {
		if !s.Conductor || s.Object < 0 || s.Object >= len(scene.Objects) || scene.Objects[s.Object].Kind != fieldObjectPlane || s.Self <= 0 {
			out = append(out, s)
			continue
		}
		o := scene.Objects[s.Object]
		ex, _, ez := fieldObjectBasis(o)
		cell := 4 * math.Asinh(1) / s.Self
		pieces := countPerAxis * countPerAxis
		for iz := 0; iz < countPerAxis; iz++ {
			v := -0.5 + (float64(iz)+.5)/float64(countPerAxis)
			for ix := 0; ix < countPerAxis; ix++ {
				u := -0.5 + (float64(ix)+.5)/float64(countPerAxis)
				p := s.Pos.add(ex.mul(u * cell)).add(ez.mul(v * cell))
				out = append(out, fieldChargeSample{
					Pos: p, Normal: s.Normal, Q: s.Q / float64(pieces),
					Soft:   math.Max(.028, cell/float64(countPerAxis)*.52),
					Object: s.Object, Conductor: true,
				})
			}
		}
	}
	return out
}

func fieldSolveLinear(a [][]float64, b []float64) ([]float64, bool) {
	n := len(b)
	if n == 0 || len(a) != n {
		return nil, false
	}
	m := make([][]float64, n)
	for i := 0; i < n; i++ {
		if len(a[i]) != n {
			return nil, false
		}
		m[i] = make([]float64, n+1)
		copy(m[i], a[i])
		m[i][n] = b[i]
	}
	for col := 0; col < n; col++ {
		pivot := col
		best := math.Abs(m[col][col])
		for row := col + 1; row < n; row++ {
			if v := math.Abs(m[row][col]); v > best {
				best, pivot = v, row
			}
		}
		if best < 1e-11 || math.IsNaN(best) {
			return nil, false
		}
		m[col], m[pivot] = m[pivot], m[col]
		inv := 1 / m[col][col]
		for j := col; j <= n; j++ {
			m[col][j] *= inv
		}
		for row := 0; row < n; row++ {
			if row == col {
				continue
			}
			f := m[row][col]
			if math.Abs(f) < 1e-16 {
				continue
			}
			for j := col; j <= n; j++ {
				m[row][j] -= f * m[col][j]
			}
		}
	}
	x := make([]float64, n)
	for i := range x {
		x[i] = m[i][n]
		if math.IsNaN(x[i]) || math.IsInf(x[i], 0) {
			return nil, false
		}
	}
	return x, true
}

func fieldPotentialFromSamples(samples []fieldChargeSample, p fieldVec3) float64 {
	v := 0.0
	for _, s := range samples {
		if s.Gate && p.dot(s.GateNormal) < s.GateOffset-1e-9 {
			continue
		}
		d := p.sub(s.Pos)
		if s.Self > 0 && d.len2() < 1e-20 {
			v += s.Q * s.Self
			continue
		}
		r := math.Sqrt(d.len2() + s.Soft*s.Soft*.12)
		if r > 1e-9 {
			v += s.Q / r
		}
	}
	return v
}

func fieldElectricSampleContribution(s fieldChargeSample, p fieldVec3) fieldVec3 {
	if s.Gate && p.dot(s.GateNormal) < s.GateOffset-1e-9 {
		return fieldVec3{}
	}
	d := p.sub(s.Pos)
	r2 := d.len2() + s.Soft*s.Soft*.12
	if r2 < 1e-12 {
		return fieldVec3{}
	}
	return d.mul(s.Q / (r2 * math.Sqrt(r2)))
}

func fieldElectricFromSamples(samples []fieldChargeSample, p fieldVec3) fieldVec3 {
	var e fieldVec3
	for _, s := range samples {
		e = e.add(fieldElectricSampleContribution(s, p))
	}
	return e
}

// fieldUniformInsulatingPlaneElectric evaluates the Coulomb field of one
// uniformly charged finite square sheet analytically.  The sheet occupies the
// object's local ex/ez rectangle and has zero thickness.  In the program's
// normalised units the Coulomb kernel is q*r/r^3, so no 1/(4*pi*eps0) factor
// appears here.  The normal component is the signed solid angle of the
// rectangle; the two tangential components are the corresponding logarithmic
// edge integrals (written as asinh differences for numerical stability).
//
// This exact rectangle integral is especially important for close parallel
// plates.  A 12x12 Gauss sample grid is excellent at ordinary distances but
// can badly overestimate the field when the plate gap is smaller than one
// sample spacing because opposing quadrature nodes line up almost directly.
func fieldUniformInsulatingPlaneElectric(o fieldObject, p fieldVec3) fieldVec3 {
	if o.Kind != fieldObjectPlane || o.Material == fieldMaterialConductor || math.Abs(o.Charge) < 1e-14 {
		return fieldVec3{}
	}
	ex, _, ez := fieldObjectBasis(o)
	n := ex.cross(ez).norm()
	d := p.sub(fieldObjectCenter(o))
	x := d.dot(ex)
	y := d.dot(n)
	z := d.dot(ez)
	half := math.Max(o.Size, 1e-9) * .5
	x1, x2 := x-half, x+half
	z1, z2 := z-half, z+half
	sigma := o.Charge / math.Max(o.Size*o.Size, 1e-12)

	const edgeEps = 1e-14
	xEdgeIntegral := func(xc float64) float64 {
		c := math.Sqrt(xc*xc + y*y)
		if c < edgeEps {
			c = edgeEps
		}
		return math.Asinh(z2/c) - math.Asinh(z1/c)
	}
	zEdgeIntegral := func(zc float64) float64 {
		c := math.Sqrt(zc*zc + y*y)
		if c < edgeEps {
			c = edgeEps
		}
		return math.Asinh(x2/c) - math.Asinh(x1/c)
	}
	exLocal := sigma * (xEdgeIntegral(x1) - xEdgeIntegral(x2))
	ezLocal := sigma * (zEdgeIntegral(z1) - zEdgeIntegral(z2))

	eyLocal := 0.0
	if math.Abs(y) >= edgeEps {
		corner := func(xc, zc float64) float64 {
			r := math.Sqrt(xc*xc + y*y + zc*zc)
			return math.Atan((xc * zc) / (y * r))
		}
		eyLocal = sigma * (corner(x2, z2) - corner(x1, z2) - corner(x2, z1) + corner(x1, z1))
	}
	return ex.mul(exLocal).add(n.mul(eyLocal)).add(ez.mul(ezLocal))
}

// fieldElectricFromSceneSamples keeps the existing point/panel/BEM sources,
// but replaces non-conducting finite planes by their exact rectangle field.
// excludeObject is used by the sheet boundary-condition code to obtain the
// principal external field without the target sheet's own 2*pi*sigma jump.
func fieldElectricFromSceneSamples(scene fieldScene, samples []fieldChargeSample, p fieldVec3, excludeObject int) fieldVec3 {
	var e fieldVec3
	for _, s := range samples {
		if s.Object == excludeObject {
			continue
		}
		if s.Object >= 0 && s.Object < len(scene.Objects) {
			o := scene.Objects[s.Object]
			if o.Kind == fieldObjectPlane && o.Material != fieldMaterialConductor {
				continue // evaluated analytically below
			}
		}
		e = e.add(fieldElectricSampleContribution(s, p))
	}
	for oi, o := range scene.Objects {
		if oi == excludeObject || !o.electric() || o.Kind != fieldObjectPlane || o.Material == fieldMaterialConductor {
			continue
		}
		e = e.add(fieldUniformInsulatingPlaneElectric(o, p))
	}
	return e
}

// fieldContainingConductor returns the connected conductor volume containing p.
// The finite thickness used for a plate is only a numerical representation of
// an ideal conducting sheet.  It prevents contours and streamlines from being
// drawn through the metal while leaving the outside vacuum solution unchanged.
func fieldContainingConductor(scene fieldScene, p fieldVec3, margin float64) int {
	for i, o := range scene.Objects {
		if !o.electric() || o.Material != fieldMaterialConductor {
			continue
		}
		if fieldPointInsideObject(p, o, margin) {
			return i
		}
	}
	return -1
}

// fieldConductorPotential obtains the single potential assigned to one isolated
// conductor by the boundary-element solve.  Surface collocation values differ
// only by floating-point/panel error, so their mean is a stable representation
// of the constant potential throughout the entire conductor volume.
func fieldConductorPotential(scene fieldScene, samples []fieldChargeSample, object int) (float64, bool) {
	if object < 0 || object >= len(scene.Objects) {
		return 0, false
	}
	o := scene.Objects[object]
	if !o.electric() || o.Material != fieldMaterialConductor {
		return 0, false
	}
	total, count := 0.0, 0
	for _, panel := range samples {
		if panel.Object != object || !panel.Conductor {
			continue
		}
		v := fieldPotentialFromSamples(samples, panel.Pos)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		total += v
		count++
	}
	if count == 0 {
		return 0, false
	}
	return total / float64(count), true
}

// fieldPotentialAt enforces the electrostatic conductor boundary condition in
// the represented solid volume: V is constant everywhere in a conductor.
func fieldPotentialAt(scene fieldScene, samples []fieldChargeSample, p fieldVec3) float64 {
	if object := fieldContainingConductor(scene, p, 0); object >= 0 {
		if v, ok := fieldConductorPotential(scene, samples, object); ok {
			return v
		}
	}
	return fieldPotentialFromSamples(samples, p)
}

// fieldElectricAt enforces E=0 inside a conductor.  Outside the conductor the
// field remains the Coulomb/BEM vacuum solution of the electrostatic Maxwell
// equations.
func fieldElectricAt(scene fieldScene, samples []fieldChargeSample, p fieldVec3) fieldVec3 {
	if fieldContainingConductor(scene, p, 0) >= 0 {
		return fieldVec3{}
	}
	return fieldElectricFromSceneSamples(scene, samples, p, -1)
}

// A bar magnet is not a point dipole.  The drawn object is a finite cylinder,
// so model the bound surface current of a uniformly magnetised cylinder.  A
// permanent magnet with magnetisation M has K_b=M x n on its curved wall: a
// sheet of circular currents distributed along the full bar length.  This is
// exactly equivalent to the +/- magnetic pole-face representation outside the
// magnet, while also giving the correct S -> N direction of B inside it.
//
// The former implementation placed one softened point dipole at the object's
// centre.  Consequently the bar's finite ends did not participate in the
// calculation and the result looked like one circular current loop with a bar
// merely drawn over it.
type fieldMagneticCurrentSegment struct {
	Mid  fieldVec3
	DL   fieldVec3
	I    float64
	Soft float64
}

type fieldBarMagnetModel struct {
	Object   fieldObject
	Axis     fieldVec3
	Radius   float64
	Segments []fieldMagneticCurrentSegment
}

type fieldMagneticModel struct {
	Magnets        []fieldBarMagnetModel
	FiniteCurrents []fieldMagneticCurrentSegment
	Wires          []fieldObject
}

func fieldBarMagnetRadius(o fieldObject) float64 {
	return math.Max(.10, math.Min(.22, o.Length*.11))
}

func fieldBarMagnetCurrentSegments(o fieldObject) []fieldMagneticCurrentSegment {
	const axialSlices = 10
	const circumSegments = 24
	ex, ey, ez := fieldObjectBasis(o)
	center := fieldObjectCenter(o)
	radius := fieldBarMagnetRadius(o)
	dx := o.Length / axialSlices
	// Sum(I_loop*pi*R^2) = Charge*Length, preserving the old far-field moment
	// scale while spreading the source over the actual finite magnet wall.
	loopCurrent := o.Charge * dx / (math.Pi * radius * radius)
	arc := 2 * math.Pi * radius / circumSegments
	soft := math.Max(.006, arc*.28)
	out := make([]fieldMagneticCurrentSegment, 0, axialSlices*circumSegments)
	for axial := 0; axial < axialSlices; axial++ {
		x := -o.Length*.5 + (float64(axial)+.5)*dx
		loopCenter := center.add(ex.mul(x))
		for j := 0; j < circumSegments; j++ {
			a0 := 2 * math.Pi * float64(j) / circumSegments
			a1 := 2 * math.Pi * float64(j+1) / circumSegments
			p0 := loopCenter.add(ey.mul(radius * math.Cos(a0))).add(ez.mul(radius * math.Sin(a0)))
			p1 := loopCenter.add(ey.mul(radius * math.Cos(a1))).add(ez.mul(radius * math.Sin(a1)))
			out = append(out, fieldMagneticCurrentSegment{
				Mid:  p0.add(p1).mul(.5),
				DL:   p1.sub(p0),
				I:    loopCurrent,
				Soft: soft,
			})
		}
	}
	return out
}

func fieldCircularLoopCurrentSegments(o fieldObject) []fieldMagneticCurrentSegment {
	const segments = 64
	ex, ey, ez := fieldObjectBasis(o)
	_ = ex // local +X is the loop normal; ey/ez span the conducting circle.
	c := fieldObjectCenter(o)
	radius := fieldClamp(o.Radius, fieldCurrentLoopRadiusMin, fieldCurrentLoopRadiusMax)
	arc := 2 * math.Pi * radius / segments
	soft := math.Max(.0045, arc*.22)
	out := make([]fieldMagneticCurrentSegment, 0, segments)
	for i := 0; i < segments; i++ {
		a0 := 2 * math.Pi * float64(i) / segments
		a1 := 2 * math.Pi * float64(i+1) / segments
		p0 := c.add(ey.mul(radius * math.Cos(a0))).add(ez.mul(radius * math.Sin(a0)))
		p1 := c.add(ey.mul(radius * math.Cos(a1))).add(ez.mul(radius * math.Sin(a1)))
		out = append(out, fieldMagneticCurrentSegment{Mid: p0.add(p1).mul(.5), DL: p1.sub(p0), I: o.Charge, Soft: soft})
	}
	return out
}

// A horseshoe magnet is represented as a curved, uniformly magnetised rod.
// Each short centreline element carries a circular bound-current loop whose
// normal follows the local tangent.  The superposition is the curved analogue
// of fieldBarMagnetCurrentSegments: it produces two real pole regions at the
// open tips, a strong gap field, and a return field through the U-shaped body.
func fieldHorseshoeCurrentSegments(o fieldObject) []fieldMagneticCurrentSegment {
	const circumSegments = 16
	path := fieldHorseshoeCenterline(o, 5, 10)
	if len(path) < 2 {
		return nil
	}
	_, ey, _ := fieldObjectBasis(o)
	radius := fieldHorseshoeRadius(o)
	out := make([]fieldMagneticCurrentSegment, 0, (len(path)-1)*circumSegments)
	for i := 1; i < len(path); i++ {
		a, b := path[i-1], path[i]
		delta := b.sub(a)
		ds := delta.len()
		if ds < 1e-9 {
			continue
		}
		tangent := delta.mul(1 / ds)
		u := ey.sub(tangent.mul(ey.dot(tangent))).norm()
		if u.len2() < 1e-12 {
			_, fallback, _ := fieldObjectBasis(o)
			u = fallback
		}
		v := tangent.cross(u).norm() // u x v = tangent
		center := a.add(b).mul(.5)
		loopCurrent := o.Charge * ds / (math.Pi * radius * radius)
		arc := 2 * math.Pi * radius / circumSegments
		soft := math.Max(.0055, arc*.25)
		for j := 0; j < circumSegments; j++ {
			a0 := 2 * math.Pi * float64(j) / circumSegments
			a1 := 2 * math.Pi * float64(j+1) / circumSegments
			p0 := center.add(u.mul(radius * math.Cos(a0))).add(v.mul(radius * math.Sin(a0)))
			p1 := center.add(u.mul(radius * math.Cos(a1))).add(v.mul(radius * math.Sin(a1)))
			out = append(out, fieldMagneticCurrentSegment{Mid: p0.add(p1).mul(.5), DL: p1.sub(p0), I: loopCurrent, Soft: soft})
		}
	}
	return out
}

func fieldBuildMagneticModel(scene fieldScene) fieldMagneticModel {
	model := fieldMagneticModel{}
	ferromagnets := make([]fieldObject, 0)
	for _, o := range scene.Objects {
		if !o.magnetic() || o.Kind != fieldObjectFerromagnet && math.Abs(o.Charge) < 1e-9 {
			continue
		}
		switch o.Kind {
		case fieldObjectBarMagnet:
			ex, _, _ := fieldObjectBasis(o)
			model.Magnets = append(model.Magnets, fieldBarMagnetModel{
				Object:   o,
				Axis:     ex,
				Radius:   fieldBarMagnetRadius(o),
				Segments: fieldBarMagnetCurrentSegments(o),
			})
		case fieldObjectCurrentWire:
			model.Wires = append(model.Wires, o)
		case fieldObjectHorseshoeMagnet:
			model.FiniteCurrents = append(model.FiniteCurrents, fieldHorseshoeCurrentSegments(o)...)
		case fieldObjectCurrentLoop, fieldObjectMassRing:
			model.FiniteCurrents = append(model.FiniteCurrents, fieldCircularLoopCurrentSegments(o)...)
		case fieldObjectFerromagnet:
			ferromagnets = append(ferromagnets, o)
		}
	}
	// Soft ferromagnets do not create a field on their own.  A strong external
	// field induces a saturated dipole aligned with the local B direction.  The
	// induced dipole is represented by finite bound-current loops, so the same
	// D3D11 vector-grid shader handles it without a separate approximation path.
	for _, o := range ferromagnets {
		b := model.at(fieldObjectCenter(o))
		mag := b.len()
		if mag < 1e-10 || o.Charge <= 0 {
			continue
		}
		axis := b.mul(1 / mag)
		strength := o.Charge * math.Tanh(mag*1.35)
		model.FiniteCurrents = append(model.FiniteCurrents, fieldFerromagnetCurrentSegments(o, axis, strength)...)
	}
	return model
}

func (model fieldMagneticModel) at(p fieldVec3) fieldVec3 {
	var b fieldVec3
	for _, magnet := range model.Magnets {
		for _, segment := range magnet.Segments {
			r := p.sub(segment.Mid)
			r2 := r.len2() + segment.Soft*segment.Soft
			if r2 < 1e-18 {
				continue
			}
			b = b.add(segment.DL.cross(r).mul(segment.I / (r2 * math.Sqrt(r2))))
		}
	}
	for _, segment := range model.FiniteCurrents {
		r := p.sub(segment.Mid)
		r2 := r.len2() + segment.Soft*segment.Soft
		if r2 < 1e-18 {
			continue
		}
		b = b.add(segment.DL.cross(r).mul(segment.I / (r2 * math.Sqrt(r2))))
	}
	for _, o := range model.Wires {
		c := fieldObjectCenter(o)
		r := p.sub(c)
		_, axis, _ := fieldObjectBasis(o)
		parallel := axis.mul(r.dot(axis))
		perp := r.sub(parallel)
		r2 := perp.len2() + .03
		b = b.add(axis.cross(perp).mul(o.Charge / r2))
	}
	return b
}

func fieldMagneticAt(scene fieldScene, p fieldVec3) fieldVec3 {
	return fieldBuildMagneticModel(scene).at(p)
}

type fieldLine struct {
	Points []fieldVec3
	Arrow  int
	Sign   float64
	Group  int
	Closed bool
}

func fieldPointInsideObject(p fieldVec3, o fieldObject, margin float64) bool {
	q := p.sub(fieldObjectCenter(o))
	ex, ey, ez := fieldObjectBasis(o)
	lx, ly, lz := q.dot(ex), q.dot(ey), q.dot(ez)
	switch o.Kind {
	case fieldObjectSphere, fieldObjectFerromagnet, fieldObjectSolidMassSphere, fieldObjectHollowMassSphere:
		return q.len() <= o.Radius+margin
	case fieldObjectRod, fieldObjectBarMagnet:
		r := math.Max(.10, math.Min(.22, o.Length*.11)) + margin
		return math.Abs(lx) <= o.Length*.5+margin && ly*ly+lz*lz <= r*r
	case fieldObjectHorseshoeMagnet:
		r := fieldHorseshoeRadius(o) + margin
		path := fieldHorseshoeCenterline(o, 7, 14)
		for i := 1; i < len(path); i++ {
			if fieldDistancePointSegment(p, path[i-1], path[i]) <= r {
				return true
			}
		}
		return false
	case fieldObjectPlane, fieldObjectMassPlane:
		return math.Abs(lx) <= o.Size*.5+margin && math.Abs(lz) <= o.Size*.5+margin && math.Abs(ly) <= .07+margin
	case fieldObjectCurrentWire:
		return lx*lx+lz*lz <= (.12+margin)*(.12+margin)
	case fieldObjectCurrentLoop, fieldObjectMassRing:
		tube := math.Max(.045, o.Radius*.055) + margin
		return math.Hypot(lx, math.Hypot(ly, lz)-o.Radius) <= tube
	}
	return false
}

func fieldRayBoxHit(origin, direction fieldVec3, hx, hy, hz, maxT float64) bool {
	tMin, tMax := 1e-5, maxT
	for _, axis := range [][3]float64{{origin.X, direction.X, hx}, {origin.Y, direction.Y, hy}, {origin.Z, direction.Z, hz}} {
		o, d, h := axis[0], axis[1], axis[2]
		if math.Abs(d) < 1e-12 {
			if o < -h || o > h {
				return false
			}
			continue
		}
		a, b := (-h-o)/d, (h-o)/d
		if a > b {
			a, b = b, a
		}
		tMin = math.Max(tMin, a)
		tMax = math.Min(tMax, b)
		if tMin > tMax {
			return false
		}
	}
	return tMax > 1e-5 && tMin < maxT
}

func fieldRayHitsConductor(o fieldObject, origin, direction fieldVec3, maxT float64) bool {
	if maxT <= 1e-5 {
		return false
	}
	center := fieldObjectCenter(o)
	ex, ey, ez := fieldObjectBasis(o)
	q := origin.sub(center)
	ro := fieldVec3{q.dot(ex), q.dot(ey), q.dot(ez)}
	rd := fieldVec3{direction.dot(ex), direction.dot(ey), direction.dot(ez)}
	switch o.Kind {
	case fieldObjectSphere:
		r := o.Radius + .006
		b := 2 * ro.dot(rd)
		c := ro.len2() - r*r
		disc := b*b - 4*c // rd is normalised, so a=1
		if disc < 0 {
			return false
		}
		sqrtD := math.Sqrt(disc)
		t0, t1 := (-b-sqrtD)*.5, (-b+sqrtD)*.5
		return (t0 > 1e-5 && t0 < maxT) || (t1 > 1e-5 && t1 < maxT)
	case fieldObjectPlane:
		half := o.Size*.5 + .006
		return fieldRayBoxHit(ro, rd, half, .076, half, maxT)
	case fieldObjectRod:
		half := o.Length*.5 + .006
		r := math.Max(.10, math.Min(.22, o.Length*.11)) + .006
		a := rd.Y*rd.Y + rd.Z*rd.Z
		b := 2 * (ro.Y*rd.Y + ro.Z*rd.Z)
		c := ro.Y*ro.Y + ro.Z*ro.Z - r*r
		if a > 1e-12 {
			disc := b*b - 4*a*c
			if disc >= 0 {
				sqrtD := math.Sqrt(disc)
				for _, t := range []float64{(-b - sqrtD) / (2 * a), (-b + sqrtD) / (2 * a)} {
					if t > 1e-5 && t < maxT && math.Abs(ro.X+rd.X*t) <= half {
						return true
					}
				}
			}
		}
		if math.Abs(rd.X) > 1e-12 {
			for _, xCap := range []float64{-half, half} {
				t := (xCap - ro.X) / rd.X
				if t > 1e-5 && t < maxT {
					y, z := ro.Y+rd.Y*t, ro.Z+rd.Z*t
					if y*y+z*z <= r*r {
						return true
					}
				}
			}
		}
	}
	return false
}

// The scene has no physical outer wall.  This radius is only a numerical
// infinity used to end a streamline after it has travelled far beyond the
// visible drawing.  It is deliberately much larger than the placement area.
const fieldNumericalInfinityRadius = 9.5
const fieldMaximumElectricLines = 512

func fieldOutsideNumericalInfinity(scene fieldScene, p fieldVec3) bool {
	return p.len() > fieldSceneNumericalInfinityRadius(scene)
}

func fieldFiniteVec(v fieldVec3) bool {
	return !math.IsNaN(v.X) && !math.IsNaN(v.Y) && !math.IsNaN(v.Z) &&
		!math.IsInf(v.X, 0) && !math.IsInf(v.Y, 0) && !math.IsInf(v.Z, 0)
}

func fieldDistanceToObjectSurface(o fieldObject, p fieldVec3) float64 {
	q := p.sub(fieldObjectCenter(o))
	ex, ey, ez := fieldObjectBasis(o)
	lx, ly, lz := q.dot(ex), q.dot(ey), q.dot(ez)
	switch o.Kind {
	case fieldObjectSphere, fieldObjectFerromagnet:
		return math.Abs(q.len() - o.Radius)
	case fieldObjectRod, fieldObjectBarMagnet:
		half := o.Length * .5
		radius := math.Max(.10, math.Min(.22, o.Length*.11))
		dx := math.Max(math.Abs(lx)-half, 0)
		dr := math.Max(math.Hypot(ly, lz)-radius, 0)
		if dx > 0 || dr > 0 {
			return math.Hypot(dx, dr)
		}
		return math.Min(half-math.Abs(lx), radius-math.Hypot(ly, lz))
	case fieldObjectHorseshoeMagnet:
		best := math.Inf(1)
		path := fieldHorseshoeCenterline(o, 7, 14)
		for i := 1; i < len(path); i++ {
			best = math.Min(best, fieldDistancePointSegment(p, path[i-1], path[i]))
		}
		return math.Abs(best - fieldHorseshoeRadius(o))
	case fieldObjectPlane, fieldObjectMassPlane:
		half := o.Size * .5
		dx := math.Max(math.Abs(lx)-half, 0)
		dy := math.Max(math.Abs(ly)-.07, 0)
		dz := math.Max(math.Abs(lz)-half, 0)
		if dx > 0 || dy > 0 || dz > 0 {
			return math.Sqrt(dx*dx + dy*dy + dz*dz)
		}
		return math.Min(math.Min(half-math.Abs(lx), half-math.Abs(lz)), .07-math.Abs(ly))
	case fieldObjectCurrentWire:
		return math.Abs(math.Hypot(lx, lz) - .12)
	case fieldObjectCurrentLoop, fieldObjectMassRing:
		tube := math.Max(.045, o.Radius*.055)
		return math.Abs(math.Hypot(lx, math.Hypot(ly, lz)-o.Radius) - tube)
	}
	return math.Inf(1)
}

func fieldAdaptiveElectricStep(scene fieldScene, p fieldVec3) float64 {
	nearest := math.Inf(1)
	for _, o := range scene.Objects {
		if !o.electric() {
			continue
		}
		nearest = math.Min(nearest, fieldDistanceToObjectSurface(o, p))
	}
	if math.IsInf(nearest, 1) {
		nearest = 1
	}
	far := fieldClamp(p.len()/fieldSceneNumericalInfinityRadius(scene), 0, 1)
	return fieldClamp(nearest*.20+.010+far*.035, .008, .085)
}

func fieldNearestPanelCharge(samples []fieldChargeSample, object int, p fieldVec3) float64 {
	bestD, q := math.Inf(1), 0.0
	for _, s := range samples {
		if s.Object != object {
			continue
		}
		d := p.sub(s.Pos).len2()
		if d < bestD {
			bestD, q = d, s.Q
		}
	}
	return q
}

// fieldIntegrateElectricLine follows dr/ds = E/|E|.  E is obtained from the
// Coulomb Green function, which solves the vacuum electrostatic Maxwell
// equations div(E)=rho/epsilon0 and curl(E)=0 (normalised units).  A midpoint
// step is used so curved lines do not depend strongly on pixel resolution.
func fieldIntegrateElectricLineWithEvaluator(scene fieldScene, samples []fieldChargeSample, eval fieldVectorEvaluator, seed fieldVec3) []fieldVec3 {
	if !fieldFiniteVec(seed) || eval == nil {
		return nil
	}
	policy := fieldDefaultStreamlinePolicy()
	policy.InitialStep = .035
	policy.MinStep = .003
	policy.MaxStep = .085
	policy.AbsTolerance = 2e-4
	policy.RelTolerance = 2e-3
	policy.MaxTurnRadians = 12 * math.Pi / 180
	policy.MaxSteps = 1500
	if scene.precise3DLines {
		policy.InitialStep = .022
		policy.MinStep = .0015
		policy.MaxStep = .045
		policy.AbsTolerance = 6e-5
		policy.RelTolerance = 5e-4
		policy.MaxTurnRadians = 6 * math.Pi / 180
		policy.MaxSteps = 3000
	}
	policy.MaxArcLength = math.Max(18, fieldSceneNumericalInfinityRadius(scene)*2.2)
	policy.ClosureDistance = 0 // curl(E)=0: electrostatic field lines do not close.
	if scene.cancel != nil {
		policy.Cancelled = scene.cancel.Cancelled
	}
	policy.Inside = func(p fieldVec3) bool { return !fieldOutsideNumericalInfinity(scene, p) }
	policy.Stop = func(a, b fieldVec3) (bool, fieldVec3, int) {
		return fieldFirstElectricBoundaryHit(scene, a, b)
	}
	result := fieldIntegrateStreamline(eval, seed, 1, policy)
	return fieldSanitizeElectricLineWithEvaluator(scene, samples, eval, result.Points)
}

func fieldIntegrateElectricLine(scene fieldScene, samples []fieldChargeSample, seed fieldVec3) []fieldVec3 {
	direct := func(p fieldVec3) fieldVec3 { return fieldElectricAt(scene, samples, p) }
	return fieldIntegrateElectricLineWithEvaluator(scene, samples, direct, seed)
}

// fieldSanitizeElectricLine is a final invariant guard for preview/export.  It
// keeps only finite, short segments that remain tangent to the calculated E
// field.  This catches any solver singularity without replacing the physical
// trajectory by a guessed curve.
func fieldSanitizeElectricLineWithEvaluator(scene fieldScene, samples []fieldChargeSample, eval fieldVectorEvaluator, pts []fieldVec3) []fieldVec3 {
	if len(pts) < 2 || !fieldFiniteVec(pts[0]) {
		return nil
	}
	out := []fieldVec3{pts[0]}
	maxSegment := .10
	for i := 1; i < len(pts); i++ {
		a, b := out[len(out)-1], pts[i]
		if !fieldFiniteVec(b) {
			break
		}
		seg := b.sub(a)
		length := seg.len()
		if length < 1e-7 {
			continue
		}
		if length > maxSegment {
			break
		}
		// Validate the segment against E at three positions, not only at the
		// midpoint.  This prevents a chord from looking acceptable at one sample
		// while cutting across a sharply curved field near a conductor edge.
		dir := seg.norm()
		valid := true
		for _, f := range []float64{.25, .5, .75} {
			probe := a.add(seg.mul(f))
			e := eval(probe)
			if !fieldFiniteVec(e) || e.len2() < 1e-14 || dir.dot(e.norm()) < .78 {
				valid = false
				break
			}
		}
		if !valid {
			break
		}
		out = append(out, b)
	}
	return out
}

func fieldSanitizeElectricLine(scene fieldScene, samples []fieldChargeSample, pts []fieldVec3) []fieldVec3 {
	direct := func(p fieldVec3) fieldVec3 { return fieldElectricAt(scene, samples, p) }
	return fieldSanitizeElectricLineWithEvaluator(scene, samples, direct, pts)
}

type fieldElectricLineValidation struct {
	Lines, Segments                            int
	NonFinite, LongJump, Reverse, Tunnel, Loop int
	MinAlignment, MaxSegment                   float64
}

// fieldValidateElectricLines checks geometric invariants of the already
// calculated Maxwell streamlines.  It does not replace a curve by a guessed
// one; invalid numerical output is rejected before preview/export.
func fieldValidateElectricLinesWithEvaluator(scene fieldScene, samples []fieldChargeSample, eval fieldVectorEvaluator, lines []fieldLine) fieldElectricLineValidation {
	report := fieldElectricLineValidation{Lines: len(lines), MinAlignment: 1}
	for _, line := range lines {
		pts := line.Points
		for i := 1; i < len(pts); i++ {
			a, b := pts[i-1], pts[i]
			if !fieldFiniteVec(a) || !fieldFiniteVec(b) {
				report.NonFinite++
				continue
			}
			seg := b.sub(a)
			length := seg.len()
			report.Segments++
			report.MaxSegment = math.Max(report.MaxSegment, length)
			if length > .105 {
				report.LongJump++
			}
			if length > 1e-8 {
				dir := seg.norm()
				for _, f := range []float64{.25, .5, .75} {
					e := eval(a.add(seg.mul(f)))
					if !fieldFiniteVec(e) || e.len2() < 1e-14 {
						continue
					}
					align := dir.dot(e.norm())
					report.MinAlignment = math.Min(report.MinAlignment, align)
					if align < .75 {
						report.Reverse++
					}
				}
			}
			// Entering a conductor on the final segment is physical.  Passing
			// through it and continuing on a later segment is numerical tunnelling.
			inside := false
			for sub := 1; sub <= 7; sub++ {
				q := a.add(seg.mul(float64(sub) / 8))
				if fieldContainingConductor(scene, q, .006) >= 0 {
					inside = true
					break
				}
			}
			if inside && i != len(pts)-1 {
				report.Tunnel++
			}
		}
		for i := 24; i < len(pts); i += 4 {
			for j := 0; j+18 < i; j += 4 {
				if pts[i].sub(pts[j]).len() < .022 {
					report.Loop++
					break
				}
			}
		}
	}
	return report
}

func fieldValidateElectricLines(scene fieldScene, samples []fieldChargeSample, lines []fieldLine) fieldElectricLineValidation {
	direct := func(p fieldVec3) fieldVec3 { return fieldElectricAt(scene, samples, p) }
	return fieldValidateElectricLinesWithEvaluator(scene, samples, direct, lines)
}

func fieldElectricLineValidationOK(report fieldElectricLineValidation) bool {
	return report.NonFinite == 0 && report.LongJump == 0 && report.Reverse == 0 && report.Tunnel == 0 && report.Loop == 0
}

func fieldIntegrateMagneticLineModel(scene fieldScene, model fieldMagneticModel, seed fieldVec3, direction float64) []fieldVec3 {
	policy := fieldMagneticStreamlinePolicy(scene)
	result := fieldIntegrateStreamline(model.at, seed, direction, policy)
	return result.Points
}

func fieldIntegrateMagneticLine(scene fieldScene, seed fieldVec3, direction float64) []fieldVec3 {
	return fieldIntegrateMagneticLineModel(scene, fieldBuildMagneticModel(scene), seed, direction)
}

func fieldSeedCoprimeStride(count int) int {
	if count <= 2 {
		return 1
	}
	gcd := func(a, b int) int {
		for b != 0 {
			a, b = b, a%b
		}
		if a < 0 {
			a = -a
		}
		return a
	}
	stride := int(math.Round(float64(count) * .6180339887498949))
	stride = max(1, min(count-1, stride))
	for gcd(stride, count) != 1 {
		stride--
		if stride < 1 {
			stride = 1
			break
		}
	}
	return stride
}

func fieldSeedDirections(count int, threeD bool) []fieldVec3 {
	if count < 4 {
		count = 4
	}
	out := make([]fieldVec3, 0, count)
	if !threeD {
		for i := 0; i < count; i++ {
			a := 2 * math.Pi * float64(i) / float64(count)
			out = append(out, fieldVec3{math.Cos(a), 0, math.Sin(a)})
		}
		return out
	}

	// Equal-area azimuth-stratified sphere sampling.  y is sampled uniformly
	// (equal solid-angle bands), while every projected XZ azimuth is exactly
	// 2*pi/count apart.  A coprime permutation assigns the latitude bands so
	// neighbouring screen rays do not also sit on neighbouring latitudes.
	// This retains a genuine 3-D distribution but removes the conspicuous
	// one-sided fan produced by the former Fibonacci spiral at low line counts.
	stride := fieldSeedCoprimeStride(count)
	for i := 0; i < count; i++ {
		a := 2 * math.Pi * float64(i) / float64(count)
		rank := (i * stride) % count
		y := 1 - 2*(float64(rank)+.5)/float64(count)
		r := math.Sqrt(math.Max(0, 1-y*y))
		out = append(out, fieldVec3{r * math.Cos(a), y, r * math.Sin(a)})
	}
	return out
}

type fieldFluxSeed struct {
	Pos       fieldVec3
	Normal    fieldVec3
	Weight    float64
	Soft      float64
	Clearance float64
	Object    int
}

type fieldEmissionSeedGroup struct {
	Object int
	Weight float64
	Target int
	Seeds  []fieldVec3
}

func fieldFluxTotals(samples []fieldChargeSample) (positive, negative float64) {
	for _, s := range samples {
		if s.Q > 0 {
			positive += s.Q
		} else {
			negative -= s.Q
		}
	}
	return positive, negative
}

// One displayed line represents the same electric flux quantum everywhere.
// In normalised units Gauss's law is integral(E.dA)=Q, so Density is precisely
// the number of line quanta per unit charge.  A global cap changes only the
// common flux quantum and therefore preserves all charge ratios.
func fieldFluxLineCounts(samples []fieldChargeSample, density int) (positive, negative int, scale float64) {
	qp, qn := fieldFluxTotals(samples)
	d := float64(max(1, density))
	positive = int(math.Round(qp * d))
	negative = int(math.Round(qn * d))
	if qp > 1e-8 && positive < 1 {
		positive = 1
	}
	if qn > 1e-8 && negative < 1 {
		negative = 1
	}
	largest := max(positive, negative)
	scale = 1
	if largest > fieldMaximumElectricLines {
		scale = float64(fieldMaximumElectricLines) / float64(largest)
		positive = int(math.Round(float64(positive) * scale))
		negative = int(math.Round(float64(negative) * scale))
		if qp > 1e-8 && positive < 1 {
			positive = 1
		}
		if qn > 1e-8 && negative < 1 {
			negative = 1
		}
	}
	return positive, negative, scale
}

func fieldFluxSeedCandidates(scene fieldScene, samples []fieldChargeSample) []fieldFluxSeed {
	out := make([]fieldFluxSeed, 0, len(samples)+16)
	for _, s := range samples {
		if s.Q <= 1e-10 || s.Object < 0 || s.Object >= len(scene.Objects) {
			continue
		}
		o := scene.Objects[s.Object]
		n := s.Normal.norm()
		if n.len2() < .5 {
			n = s.Pos.sub(fieldObjectCenter(o)).norm()
		}
		weight := s.Q
		// In the 2-D view we still trace genuine 3-D Maxwell streamlines and
		// project them.  Prefer seeds near the XZ viewing plane to prevent a
		// forest of almost coincident projected lines.
		if scene.Dimension == fieldDimension2D {
			planeFactor := math.Exp(-math.Pow(math.Abs(s.Pos.Y)/(math.Max(s.Soft, .08)*2.2), 2))
			weight *= .06 + .94*planeFactor
		}
		clearance := .035
		if o.Kind == fieldObjectPlane {
			// fieldPointInsideObject gives the numerical sheet a half-thickness
			// of .07.  Earlier seeds used max(.035, .30*soft), which put every
			// ray of a small/default plate inside that numerical slab.  Those
			// rays terminated before five points and were discarded, making a
			// +1 plate appear to emit no lines beside a +1 sphere.
			clearance = .095
		}
		if o.Kind == fieldObjectPlane {
			// A zero-thickness insulating sheet has a discontinuous normal field.
			// In this program's Coulomb units (E = q r/r^3), the sheet boundary
			// condition is
			//
			//   E_plus . n  = E_ext . n + 2*pi*sigma
			//   E_minus.-n =-E_ext . n + 2*pi*sigma
			//
			// where E_ext is the principal-value field at the sheet from all other
			// charge.  The sheet's own other quadrature points lie in its tangent
			// plane and therefore add zero normal component; the coincident point
			// also contributes exactly zero in fieldElectricFromSamples.  Evaluating
			// at s.Pos therefore gives the required surface-limit average directly.
			//
			// 1.2.36 instead sampled the field a fixed distance away from each face.
			// That probe distance distorted close parallel plates: depending on the
			// separation it could over- or under-estimate the finite-edge fringing
			// flux.  Keep the 0.095 geometric clearance only for where the traced
			// line starts; do not use it to decide the physical face-flux split.
			area := math.Max(o.Size*o.Size, 1e-12)
			sigma := math.Abs(o.Charge) / area
			selfNormal := 2 * math.Pi * sigma
			externalNormal := fieldElectricFromSceneSamples(scene, samples, s.Pos, s.Object).dot(n)
			minusNormal := n.mul(-1)
			plusFlux := math.Max(0, selfNormal+externalNormal)
			minusFlux := math.Max(0, selfNormal-externalNormal)
			fluxSum := plusFlux + minusFlux
			plusFraction := .5
			if fluxSum > 1e-12 && !math.IsNaN(fluxSum) && !math.IsInf(fluxSum, 0) {
				plusFraction = fieldClamp(plusFlux/fluxSum, 0, 1)
			}
			minusFraction := 1 - plusFraction
			if plusFraction > 1e-12 {
				out = append(out, fieldFluxSeed{Pos: s.Pos, Normal: n, Weight: weight * plusFraction, Soft: s.Soft, Clearance: clearance, Object: s.Object})
			}
			if minusFraction > 1e-12 {
				out = append(out, fieldFluxSeed{Pos: s.Pos, Normal: minusNormal, Weight: weight * minusFraction, Soft: s.Soft, Clearance: clearance, Object: s.Object})
			}
		} else {
			out = append(out, fieldFluxSeed{Pos: s.Pos, Normal: n, Weight: weight, Soft: s.Soft, Clearance: clearance, Object: s.Object})
		}
	}
	return out
}

func fieldSeedTangentBasis(n fieldVec3) (fieldVec3, fieldVec3) {
	axis := fieldVec3{Y: 1}
	if math.Abs(n.Y) > .82 {
		axis = fieldVec3{X: 1}
	}
	u := n.cross(axis).norm()
	if u.len2() < .5 {
		u = fieldVec3{Z: 1}
	}
	return u, n.cross(u).norm()
}

func fieldChooseWeightedSeedsStratified(candidates []fieldFluxSeed, count int, phase float64) []fieldVec3 {
	if count <= 0 || len(candidates) == 0 {
		return nil
	}
	total := 0.0
	for _, c := range candidates {
		total += math.Max(0, c.Weight)
	}
	if total <= 1e-14 {
		return nil
	}
	phase = math.Mod(phase, 1)
	if phase < 0 {
		phase += 1
	}
	out := make([]fieldVec3, 0, count)
	cum, idx := math.Max(0, candidates[0].Weight), 0
	for k := 0; k < count; k++ {
		// One sample is taken from every equal-flux stratum.  Varying phase
		// changes retry positions without ever letting the first retry round
		// occupy only the beginning of a row-major plate sample array.
		target := (float64(k) + phase) * total / float64(count)
		for idx+1 < len(candidates) && cum < target {
			idx++
			cum += math.Max(0, candidates[idx].Weight)
		}
		c := candidates[idx]
		if c.Normal.len2() < .5 {
			out = append(out, c.Pos)
			continue
		}
		u, v := fieldSeedTangentBasis(c.Normal)
		angle := 2 * math.Pi * math.Mod(float64(k)*.6180339887498949+phase*.4142135623730950, 1)
		radial := (.10 + .20*math.Mod(float64(k)*.7548776662466927+phase*.2718281828459045, 1)) * c.Soft
		jitter := u.mul(radial * math.Cos(angle)).add(v.mul(radial * math.Sin(angle)))
		offset := math.Max(math.Max(.035, c.Clearance), c.Soft*.30)
		out = append(out, c.Pos.add(jitter).add(c.Normal.mul(offset)))
	}
	return out
}

func fieldChooseWeightedSeeds(candidates []fieldFluxSeed, count int) []fieldVec3 {
	return fieldChooseWeightedSeedsStratified(candidates, count, .5)
}

// fieldBalancedPlaneSeedPool keeps every retry round spatially distributed over
// the whole plate and divides the lines between the two physical faces by the
// actual one-sided electric flux.  An isolated uniformly charged sheet still
// gives 50/50.  A + plate facing a - plate does not: most flux leaves the inner
// face and only the finite-edge fringing flux should be seeded on the outside.
//
// 1.2.35 fixed the face parity but constructed four dense per-face pools and
// consumed them sequentially.  With a row-major plate sample array, round zero
// therefore took only the first quarter of the plate (visually the lower part).
// Each retry round is now independently stratified across the full weighted
// surface, with a deterministic phase shift for later retries.
func fieldBalancedPlaneSeedPool(o fieldObject, candidates []fieldFluxSeed, target, factor int) []fieldVec3 {
	if target <= 0 || len(candidates) == 0 {
		return nil
	}
	factor = max(1, factor)
	ex, _, ez := fieldObjectBasis(o)
	planeNormal := ex.cross(ez).norm()
	positive := make([]fieldFluxSeed, 0, len(candidates)/2)
	negative := make([]fieldFluxSeed, 0, len(candidates)/2)
	positiveWeight, negativeWeight := 0.0, 0.0
	for _, c := range candidates {
		w := math.Max(0, c.Weight)
		if c.Normal.dot(planeNormal) >= 0 {
			positive = append(positive, c)
			positiveWeight += w
		} else {
			negative = append(negative, c)
			negativeWeight += w
		}
	}
	totalWeight := positiveWeight + negativeWeight
	if len(positive) == 0 || len(negative) == 0 || totalWeight <= 1e-14 {
		return fieldInterleaveSeedPool(fieldChooseWeightedSeeds(candidates, target*factor), target, factor)
	}
	positiveFraction := positiveWeight / totalWeight
	out := make([]fieldVec3, 0, target*factor)
	for round := 0; round < factor; round++ {
		// Cumulative rounding preserves the desired flux ratio over all retry
		// rounds while keeping every individual round as close as possible.
		before := int(math.Round(float64(round*target) * positiveFraction))
		after := int(math.Round(float64((round+1)*target) * positiveFraction))
		positiveCount := max(0, min(target, after-before))
		negativeCount := target - positiveCount
		phase := math.Mod(.5+float64(round)*.3819660112501051, 1)
		posSeeds := fieldChooseWeightedSeedsStratified(positive, positiveCount, phase)
		negSeeds := fieldChooseWeightedSeedsStratified(negative, negativeCount, math.Mod(1-phase+.1732050807568877, 1))
		pi, ni := 0, 0
		for k := 0; k < target; k++ {
			// Alternate display order when both faces participate.  This avoids
			// any later early-exit/retry logic preferring one face by array order.
			preferPositive := (k+round)%2 == 0
			if preferPositive && pi < len(posSeeds) || ni >= len(negSeeds) {
				if pi < len(posSeeds) {
					out = append(out, posSeeds[pi])
					pi++
				}
			} else if ni < len(negSeeds) {
				out = append(out, negSeeds[ni])
				ni++
			}
		}
		for pi < len(posSeeds) {
			out = append(out, posSeeds[pi])
			pi++
		}
		for ni < len(negSeeds) {
			out = append(out, negSeeds[ni])
			ni++
		}
	}
	return out
}

// fieldInterleaveSeedPool turns an oversampled deterministic pool into retry
// rounds.  Round zero already spans the complete source surface; later rounds
// fill any quota lost to a null, conductor edge, or numerical validation.
// Taking the first target entries of a dense pool directly would cluster a
// point charge into one azimuth sector and recreate the one-sided fan.
func fieldInterleaveSeedPool(pool []fieldVec3, target, factor int) []fieldVec3 {
	if target <= 0 || len(pool) == 0 {
		return nil
	}
	factor = max(1, factor)
	out := make([]fieldVec3, 0, len(pool))
	for round := 0; round < factor; round++ {
		for k := 0; k < target; k++ {
			i := k*factor + round
			if i < len(pool) {
				out = append(out, pool[i])
			}
		}
	}
	for i := target * factor; i < len(pool); i++ {
		out = append(out, pool[i])
	}
	return out
}

func fieldInfinitySeeds(scene fieldScene, samples []fieldChargeSample, count int) []fieldVec3 {
	if count <= 0 {
		return nil
	}
	candidateCount := max(72, count*5)
	dirs := fieldSeedDirections(candidateCount, scene.Dimension == fieldDimension3D)
	radius := fieldSceneNumericalInfinityRadius(scene) * .90
	candidates := make([]fieldFluxSeed, 0, len(dirs))
	for _, n := range dirs {
		p := n.mul(radius)
		// Negative outward flux means a physical E-line enters the scene from
		// infinity.  Weight seeds by that Maxwell flux density, not uniformly.
		w := math.Max(0, -fieldElectricFromSceneSamples(scene, samples, p, -1).dot(n))
		candidates = append(candidates, fieldFluxSeed{Pos: p, Normal: fieldVec3{}, Weight: w, Soft: .1, Object: -1})
	}
	seeds := fieldChooseWeightedSeeds(candidates, count)
	if len(seeds) == 0 {
		for i := 0; i < count; i++ {
			seeds = append(seeds, dirs[i%len(dirs)].mul(radius))
		}
	}
	return seeds
}

func fieldSphereUniformSeeds(o fieldObject, count int, threeD bool) []fieldVec3 {
	if count <= 0 {
		return nil
	}
	center := fieldObjectCenter(o)
	offset := math.Max(.045, o.Radius*.06)
	dirs := fieldSeedDirections(count, threeD)
	out := make([]fieldVec3, 0, count)
	for _, d := range dirs {
		out = append(out, center.add(d.mul(o.Radius+offset)))
	}
	return out
}

func fieldAllocateEmissionTargets(groups []fieldEmissionSeedGroup, totalCount int) {
	if totalCount <= 0 || len(groups) == 0 {
		return
	}
	totalWeight := 0.0
	for _, g := range groups {
		totalWeight += math.Max(0, g.Weight)
	}
	if totalWeight <= 1e-12 {
		return
	}
	remain := totalCount
	for i := range groups {
		groups[i].Target = int(math.Floor(float64(totalCount) * groups[i].Weight / totalWeight))
		remain -= groups[i].Target
	}
	for remain > 0 {
		best := -1
		bestFrac := -1.0
		for i, g := range groups {
			exact := float64(totalCount) * g.Weight / totalWeight
			frac := exact - float64(g.Target)
			if frac > bestFrac {
				bestFrac, best = frac, i
			}
		}
		if best < 0 {
			break
		}
		groups[best].Target++
		remain--
	}
}

func fieldPositiveEmissionSeedGroups(scene fieldScene, samples []fieldChargeSample, totalCount int) []fieldEmissionSeedGroup {
	if totalCount <= 0 {
		return nil
	}
	groups := make([]fieldEmissionSeedGroup, 0, len(scene.Objects))
	candidatesByObject := make(map[int][]fieldFluxSeed, len(scene.Objects))
	for _, c := range fieldFluxSeedCandidates(scene, samples) {
		if c.Object >= 0 && c.Object < len(scene.Objects) {
			candidatesByObject[c.Object] = append(candidatesByObject[c.Object], c)
		}
	}
	for oi, o := range scene.Objects {
		if !o.electric() {
			continue
		}
		if o.Kind == fieldObjectSphere && o.Material != fieldMaterialConductor && o.Charge > 1e-10 {
			// Start every point-charge ray on one common tiny sphere with
			// equal angular spacing.  The visible glyph is drawn later and
			// hides the short interior part, so the lines appear to emerge
			// uniformly from the charge centre.
			groups = append(groups, fieldEmissionSeedGroup{Object: oi, Weight: o.Charge})
			continue
		}
		objWeight := 0.0
		for _, s := range samples {
			if s.Object != oi || s.Q <= 1e-10 {
				continue
			}
			objWeight += s.Q
		}
		if objWeight > 1e-10 && len(candidatesByObject[oi]) > 0 {
			groups = append(groups, fieldEmissionSeedGroup{Object: oi, Weight: objWeight})
		}
	}
	fieldAllocateEmissionTargets(groups, totalCount)
	if len(groups) == 0 {
		return nil
	}
	const retryFactor = 4
	for i := range groups {
		g := &groups[i]
		if g.Target <= 0 {
			continue
		}
		poolCount := max(retryFactor, g.Target*retryFactor)
		o := scene.Objects[g.Object]
		var pool []fieldVec3
		if o.Kind == fieldObjectSphere && o.Material != fieldMaterialConductor && o.Charge > 1e-10 {
			pool = fieldSphereUniformSeeds(o, poolCount, scene.Dimension == fieldDimension3D)
			g.Seeds = fieldInterleaveSeedPool(pool, g.Target, retryFactor)
		} else if o.Kind == fieldObjectPlane && o.Material != fieldMaterialConductor {
			g.Seeds = fieldBalancedPlaneSeedPool(o, candidatesByObject[g.Object], g.Target, retryFactor)
		} else {
			pool = fieldChooseWeightedSeeds(candidatesByObject[g.Object], poolCount)
			g.Seeds = fieldInterleaveSeedPool(pool, g.Target, retryFactor)
		}
	}
	return groups
}

func fieldPositiveEmissionSeeds(scene fieldScene, samples []fieldChargeSample, totalCount int) []fieldVec3 {
	groups := fieldPositiveEmissionSeedGroups(scene, samples, totalCount)
	out := make([]fieldVec3, 0, totalCount)
	for _, g := range groups {
		for i := 0; i < g.Target && i < len(g.Seeds); i++ {
			out = append(out, g.Seeds[i])
		}
	}
	return out
}

func fieldElectricLines(scene fieldScene, samples []fieldChargeSample) []fieldLine {
	started := time.Now()
	lineCacheKey := fieldElectricLineSceneKey(scene, samples)
	if cached, ok := fieldCachedPhysicalLines("electric", lineCacheKey); ok {
		fieldRecordPhysicalLineCache("electric", len(cached), time.Since(started))
		return cached
	}
	eval, backend := fieldElectricVectorEvaluator(scene, samples)
	positiveCount, negativeCount, _ := fieldFluxLineCounts(samples, scene.Density)
	candidates := 0
	lines := make([]fieldLine, 0, max(positiveCount, negativeCount))
	appendValidated := func(pts []fieldVec3, group int) bool {
		candidates++
		if len(pts) < 5 {
			return false
		}
		line := fieldLine{Points: pts, Arrow: fieldArrowIndexAtArcFraction(pts, .58), Sign: 1, Group: group}
		if fieldElectricLineValidationOK(fieldValidateElectricLinesWithEvaluator(scene, samples, eval, []fieldLine{line})) {
			refined := fieldRefineElectricLine(scene, pts)
			refined.Group = group
			lines = append(lines, refined)
			return true
		}
		return false
	}
	for _, group := range fieldPositiveEmissionSeedGroups(scene, samples, positiveCount) {
		if scene.cancelled() {
			break
		}
		accepted := 0
		for _, seed := range group.Seeds {
			if scene.cancelled() {
				break
			}
			if accepted >= group.Target {
				break
			}
			if appendValidated(fieldIntegrateElectricLineWithEvaluator(scene, samples, eval, seed), group.Object) {
				accepted++
			}
		}
	}
	infinityTarget := max(0, negativeCount-positiveCount)
	if infinityTarget > 0 {
		const retryFactor = 4
		pool := fieldInfinitySeeds(scene, samples, infinityTarget*retryFactor)
		pool = fieldInterleaveSeedPool(pool, infinityTarget, retryFactor)
		accepted := 0
		for _, seed := range pool {
			if scene.cancelled() {
				break
			}
			if accepted >= infinityTarget {
				break
			}
			if appendValidated(fieldIntegrateElectricLineWithEvaluator(scene, samples, eval, seed), len(scene.Objects)) {
				accepted++
			}
		}
	}
	if !scene.cancelled() {
		fieldStorePhysicalLines("electric", lineCacheKey, lines)
	}
	fieldRecordStreamlineTiming("electric", backend, len(lines), candidates, time.Since(started))
	return lines
}

func fieldBarMagnetLineSeeds(o fieldObject, count int, fullAzimuth bool) []fieldVec3 {
	if count < 6 {
		count = 6
	}
	ex, ey, ez := fieldObjectBasis(o)
	northAxis := ex
	if o.Charge < 0 {
		northAxis = northAxis.mul(-1)
	}
	radius := fieldBarMagnetRadius(o)
	center := fieldObjectCenter(o)
	shell := math.Max(o.Length*.68, radius*2.8)
	out := make([]fieldVec3, 0, count)
	if !fullAzimuth {
		perp := fieldVec3{-northAxis.Z, 0, northAxis.X}.norm()
		if perp.len2() < 1e-10 {
			if math.Abs(ey.Y) < math.Abs(ez.Y) {
				perp = ey
			} else {
				perp = ez
			}
		}
		pairs := max(1, (count+1)/2)
		for i := 0; i < count; i++ {
			band := i / 2
			t := (float64(band) + .5) / float64(pairs)
			theta := fieldEqualFluxPolarAngle(t, 28, 74)
			side := 1.0
			if i%2 != 0 {
				side = -1
			}
			dir := northAxis.mul(math.Cos(theta)).add(perp.mul(side * math.Sin(theta)))
			out = append(out, center.add(dir.mul(shell)))
		}
		return out
	}
	// Keep all twelve requested screen azimuths, but stratify polar flux within
	// each band. Antipodal seeds therefore no longer start on the same orbit.
	const directionCount = 12
	if count < directionCount {
		count = directionCount
	}
	bands := (count + directionCount - 1) / directionCount
	stride := fieldSeedCoprimeStride(directionCount)
	for i := 0; i < count; i++ {
		direction := (i * stride) % directionCount
		band := i / directionCount
		a := 2 * math.Pi * float64(direction) / directionCount
		transverse := ey.mul(math.Cos(a)).add(ez.mul(math.Sin(a))).norm()
		phase := math.Mod(float64(direction)*.6180339887498949, 1)
		t := (float64(band) + .16 + .68*phase) / float64(bands)
		theta := fieldEqualFluxPolarAngle(t, 28, 74)
		dir := northAxis.mul(math.Cos(theta)).add(transverse.mul(math.Sin(theta)))
		out = append(out, center.add(dir.mul(shell)))
	}
	return out
}

func fieldCurrentWireLineSeeds(o fieldObject, count int) []fieldVec3 {
	if count < 6 {
		count = 6
	}
	ex, _, ez := fieldObjectBasis(o)
	c := fieldObjectCenter(o)
	minimum := .19
	maximum := fieldClamp(math.Max(.62, o.Length*.82), .62, 2.25)
	out := make([]fieldVec3, 0, count)
	for i := 0; i < count; i++ {
		t := (float64(i) + .5) / float64(count)
		// Logarithmic radii approximate the visually useful 1/r field-density
		// falloff while ensuring every seed lies on a genuinely different orbit.
		radius := minimum * math.Pow(maximum/minimum, t)
		phase := 2 * math.Pi * math.Mod(float64(i)*.6180339887498949, 1)
		out = append(out, c.add(ex.mul(radius*math.Cos(phase))).add(ez.mul(radius*math.Sin(phase))))
	}
	return out
}

func fieldHorseshoeLineSeeds(o fieldObject, count int) []fieldVec3 {
	const directionCount = 12
	if count < directionCount {
		count = directionCount
	}
	_, north, _, northOut := fieldHorseshoePoleGeometry(o)
	_, ey, _ := fieldObjectBasis(o)
	u := ey.sub(northOut.mul(ey.dot(northOut))).norm()
	if u.len2() < 1e-12 {
		u = fieldVec3{Y: 1}
	}
	v := northOut.cross(u).norm()
	radius := fieldHorseshoeRadius(o)
	shell := math.Max(radius*2.7, o.Length*.18)
	bands := (count + directionCount - 1) / directionCount
	stride := fieldSeedCoprimeStride(directionCount)
	out := make([]fieldVec3, 0, count)
	for i := 0; i < count; i++ {
		direction := (i * stride) % directionCount
		band := i / directionCount
		a := 2 * math.Pi * float64(direction) / directionCount
		transverse := u.mul(math.Cos(a)).add(v.mul(math.Sin(a))).norm()
		t := (float64(band) + .5) / float64(bands)
		theta := fieldEqualSolidAnglePolarAngle(t, 22, 72)
		dir := northOut.mul(math.Cos(theta)).add(transverse.mul(math.Sin(theta)))
		out = append(out, north.add(dir.mul(shell)))
	}
	return out
}

func fieldCurrentLoopLineSeeds(o fieldObject, count int) []fieldVec3 {
	const directionCount = 12
	if count < directionCount {
		count = directionCount
	}
	_, ey, ez := fieldObjectBasis(o)
	c := fieldObjectCenter(o)
	radius := fieldClamp(o.Radius, fieldCurrentLoopRadiusMin, fieldCurrentLoopRadiusMax)
	bands := (count + directionCount - 1) / directionCount
	stride := fieldSeedCoprimeStride(directionCount)
	out := make([]fieldVec3, 0, count)
	for i := 0; i < count; i++ {
		direction := (i * stride) % directionCount
		band := i / directionCount
		a := 2 * math.Pi * float64(direction) / directionCount
		radial := ey.mul(math.Cos(a)).add(ez.mul(math.Sin(a))).norm()
		phase := math.Mod(float64(direction)*.7548776662466927, 1)
		t := (float64(band) + .18 + .64*phase) / float64(bands)
		lo, hi := .28, .88
		radialFraction := math.Sqrt(lo*lo + (hi*hi-lo*lo)*t)
		out = append(out, c.add(radial.mul(radius*radialFraction)))
	}
	return out
}

func fieldMagneticSeedFamily(kind fieldObjectKind, seedIndex int) int {
	switch kind {
	case fieldObjectBarMagnet, fieldObjectCurrentLoop:
		direction := (seedIndex * fieldSeedCoprimeStride(12)) % 12
		return direction % 6
	case fieldObjectHorseshoeMagnet:
		return (seedIndex * fieldSeedCoprimeStride(12)) % 12
	case fieldObjectCurrentWire:
		// Preserve one representative for every neighbouring radius pair; the
		// screen-diversity pass may retain both when they are visibly separated.
		return seedIndex / 2
	default:
		return seedIndex
	}
}

func fieldMagneticLines(scene fieldScene) []fieldLine {
	started := time.Now()
	lineCacheKey := fieldMagneticLineSceneKey(scene)
	if cached, ok := fieldCachedMagneticLines(lineCacheKey); ok {
		fieldRecordPhysicalLineCache("magnetic", len(cached), time.Since(started))
		return cached
	}
	model := fieldBuildMagneticModel(scene)
	eval, backend := fieldMagneticVectorEvaluator(scene, model)
	policy := fieldMagneticStreamlinePolicy(scene)
	tasks := make([]fieldMagneticSeedTask, 0)
	prefiltered := 0
	for objectIndex, o := range scene.Objects {
		if !o.magneticSource() || math.Abs(o.Charge) < 1e-8 {
			continue
		}
		count := max(6, scene.Density)
		if scene.Dimension == fieldDimension3D {
			count *= 2
		}
		var seeds []fieldVec3
		switch o.Kind {
		case fieldObjectBarMagnet:
			seeds = fieldBarMagnetLineSeeds(o, count, true)
		case fieldObjectCurrentWire:
			seeds = fieldCurrentWireLineSeeds(o, count)
		case fieldObjectHorseshoeMagnet:
			seeds = fieldHorseshoeLineSeeds(o, count)
		case fieldObjectCurrentLoop:
			seeds = fieldCurrentLoopLineSeeds(o, count)
		}
		for seedIndex, seed := range seeds {
			// Reject a zero/stagnant seed before allocating a full RK45 task.  This
			// is intentionally conservative and preserves all finite field seeds.
			if direction, ok := fieldUnitVector(eval(seed), 1, policy.MinField); !ok || !fieldVec3Finite(direction) {
				prefiltered++
				continue
			}
			tasks = append(tasks, fieldMagneticSeedTask{
				Index:   len(tasks),
				Group:   objectIndex*128 + fieldMagneticSeedFamily(o.Kind, seedIndex),
				Seed:    seed,
				Spacing: fieldMagneticRenderSpacing(scene),
			})
		}
	}

	var results []fieldMagneticSeedResult
	workers := 1
	if backend == "CPU-DIRECT" || backend == "CPU-DIRECT-PRECISE-3D" {
		// Exact Biot-Savart evaluation streams through hundreds of current
		// segments. Parallel workers contend for caches on this fallback path,
		// so preserve the faster deterministic serial route when D3D11 is absent.
		results = make([]fieldMagneticSeedResult, len(tasks))
		for i, task := range tasks {
			results[i] = fieldTraceMagneticSeed(eval, policy, task)
		}
	} else {
		results, workers = fieldTraceMagneticSeedsParallel(eval, policy, tasks)
	}
	lines := make([]fieldLine, 0, len(results))
	points, evaluations := 0, 0
	var maxTask time.Duration
	for _, result := range results {
		points += result.Points
		evaluations += result.Evaluations
		if result.Elapsed > maxTask {
			maxTask = result.Elapsed
		}
		if result.Accepted {
			lines = append(lines, result.Line)
		}
	}
	elapsed := time.Since(started)
	if !scene.cancelled() {
		fieldStoreMagneticLines(lineCacheKey, lines)
	}
	fieldRecordStreamlineParallelTiming("magnetic", backend, len(lines), len(tasks), workers, prefiltered, points, evaluations, maxTask, elapsed)
	return lines
}

type fieldSegment3 struct{ A, B fieldVec3 }

type fieldPlaneSlice struct {
	Origin fieldVec3
	U      fieldVec3
	V      fieldVec3
}

// fieldWidePlateAnchoredPotentialLevels chooses iso-values from deterministic
// probes tied to the actual point-charge/plate geometry.  The original
// version sampled only a handful of points along one preferred ray, so the
// resulting contour levels visibly reflected those specific probe locations.
// Here we instead gather a much denser, quasi-uniform set of samples on
// several source-centred shells that remain in the physical source half-space.
// Levels are then taken from quantiles of that dense probe distribution, so
// the rendered membranes/contours reflect the field as a whole rather than a
// few conspicuous measurement points.
func fieldWidePlateAnchoredPotentialLevels(scene fieldScene, samples []fieldChargeSample) []float64 {
	plateIndex, normal, side, maxDistance, ok := fieldWidePlateChargeSide(scene)
	if !ok || plateIndex < 0 || plateIndex >= len(scene.Objects) {
		return nil
	}
	vc, ok := fieldConductorPotential(scene, samples, plateIndex)
	if !ok || math.IsNaN(vc) || math.IsInf(vc, 0) {
		vc = 0
	}
	plateCenter := fieldObjectCenter(scene.Objects[plateIndex])
	probeDirs := fieldSeedDirections(max(36, scene.Density*6), true)
	candidates := make([]float64, 0, 512)
	for i, o := range scene.Objects {
		if i == plateIndex || !o.electric() || o.Kind != fieldObjectSphere || o.Material == fieldMaterialConductor || math.Abs(o.Charge) < 1e-10 {
			continue
		}
		c := fieldObjectCenter(o)
		d := math.Abs(c.sub(plateCenter).dot(normal))
		if d < 1e-6 {
			d = maxDistance
		}
		base := math.Max(o.Radius*1.15, math.Max(.09, d*.075))
		for _, ratio := range []float64{1.0, 1.25, 1.55, 1.9, 2.3, 2.8, 3.5, 4.4, 5.6, 7.2, 9.2} {
			r := base * ratio
			for _, dir := range probeDirs {
				probe := c.add(dir.mul(r))
				if probe.sub(plateCenter).dot(normal)*side <= .025 {
					continue
				}
				if fieldContainingConductor(scene, probe, .004) >= 0 {
					continue
				}
				v := fieldPotentialAt(scene, samples, probe)
				if math.IsNaN(v) || math.IsInf(v, 0) {
					continue
				}
				gap := math.Max(1e-4, math.Max(math.Abs(vc)*.025, math.Abs(v)*.008))
				if o.Charge > 0 {
					if v <= vc+gap {
						continue
					}
				} else if v >= vc-gap {
					continue
				}
				candidates = append(candidates, v)
			}
		}
	}
	if len(candidates) < 8 {
		return nil
	}
	sort.Float64s(candidates)
	unique := make([]float64, 0, len(candidates))
	for _, v := range candidates {
		if len(unique) == 0 {
			unique = append(unique, v)
			continue
		}
		last := unique[len(unique)-1]
		if math.Abs(v-last) <= math.Max(1e-5, math.Max(math.Abs(v), math.Abs(last))*.012) {
			continue
		}
		unique = append(unique, v)
	}
	if len(unique) == 0 {
		return nil
	}
	fractions := []float64{.12, .22, .34, .48, .64, .80, .92}
	levels := make([]float64, 0, len(fractions))
	for _, f := range fractions {
		idx := int(math.Round(f * float64(len(unique)-1)))
		idx = max(0, min(len(unique)-1, idx))
		v := unique[idx]
		if len(levels) > 0 {
			last := levels[len(levels)-1]
			if math.Abs(v-last) <= math.Max(1e-5, math.Max(math.Abs(v), math.Abs(last))*.02) {
				continue
			}
		}
		levels = append(levels, v)
	}
	if len(levels) < 3 {
		return append([]float64(nil), unique...)
	}
	return levels
}

func fieldPotentialLevels(scene fieldScene, samples []fieldChargeSample, slice fieldPlaneSlice, steps int) []float64 {
	if anchored := fieldWidePlateAnchoredPotentialLevels(scene, samples); len(anchored) >= 3 {
		return anchored
	}
	span := fieldEquipotentialSamplingSpan(scene)
	widePlateIndex, wideNormal, wideSide, _, widePlate := fieldWidePlateChargeSide(scene)
	wideCenter := fieldVec3{}
	if widePlate {
		wideCenter = fieldObjectCenter(scene.Objects[widePlateIndex])
	}
	slices := []fieldPlaneSlice{slice}
	if scene.Dimension == fieldDimension3D {
		// A very large conducting plate can contain the entire XZ slice.  Sample
		// the other two principal sections as well so the selector sees vacuum on
		// both sides of the sheet.
		slices = append(slices,
			fieldPlaneSlice{Origin: fieldVec3{}, U: fieldVec3{X: 1}, V: fieldVec3{Y: 1}},
			fieldPlaneSlice{Origin: fieldVec3{}, U: fieldVec3{Y: 1}, V: fieldVec3{Z: 1}},
		)
	}
	positive := make([]float64, 0, len(slices)*steps*steps)
	negative := make([]float64, 0, len(slices)*steps*steps)
	for _, sampleSlice := range slices {
		for j := 0; j < steps; j++ {
			v := -span + 2*span*float64(j)/float64(steps-1)
			for i := 0; i < steps; i++ {
				u := -span + 2*span*float64(i)/float64(steps-1)
				p := sampleSlice.Origin.add(sampleSlice.U.mul(u)).add(sampleSlice.V.mul(v))
				if widePlate && p.sub(wideCenter).dot(wideNormal)*wideSide < .02 {
					continue
				}
				if fieldContainingConductor(scene, p, .006) >= 0 {
					continue
				}
				x := fieldPotentialAt(scene, samples, p)
				if math.IsInf(x, 0) || math.IsNaN(x) || math.Abs(x) <= 1e-5 {
					continue
				}
				if x > 0 {
					positive = append(positive, x)
				} else {
					negative = append(negative, -x)
				}
			}
		}
	}
	conductorV := fieldConductorPotentialValues(scene, samples)
	build := func(magnitudes []float64, sign float64) []float64 {
		if len(magnitudes) < 8 {
			return nil
		}
		sort.Float64s(magnitudes)
		// Use an upper vacuum quantile rather than |V| from both signs.  The old
		// symmetric selector generated impossible negative sheets in a positive
		// scene and often chose V almost equal to the plate potential, causing a
		// numerically thick conductor slab to be extracted as ragged membranes.
		base := magnitudes[int(float64(len(magnitudes)-1)*.96)]
		if base < 1e-5 {
			return nil
		}
		candidates := []float64{.28, .40, .55, .72, .92, 1.15}
		out := make([]float64, 0, 5)
		for _, ratio := range candidates {
			level := sign * base * ratio
			tooClose := false
			for _, cv := range conductorV {
				gap := math.Max(1e-4, math.Max(base*.035, math.Abs(cv)*.065))
				if math.Abs(level-cv) <= gap {
					tooClose = true
					break
				}
			}
			if tooClose {
				continue
			}
			out = append(out, level)
			if len(out) == 5 {
				break
			}
		}
		return out
	}
	levels := append(build(negative, -1), build(positive, 1)...)
	sort.Float64s(levels)
	return levels
}

func fieldInterpolateLevel(a, b fieldVec3, va, vb, level float64) fieldVec3 {
	t := .5
	if math.Abs(vb-va) > 1e-12 {
		t = (level - va) / (vb - va)
	}
	t = fieldClamp(t, 0, 1)
	return a.add(b.sub(a).mul(t))
}

func fieldSegmentPoint(seg fieldSegment3, t float64) fieldVec3 {
	return seg.A.add(seg.B.sub(seg.A).mul(t))
}

func fieldRefineConductorBoundary(scene fieldScene, a, b fieldVec3, aInside bool) fieldVec3 {
	lo, hi := 0.0, 1.0
	for i := 0; i < 24; i++ {
		mid := (lo + hi) * .5
		p := a.add(b.sub(a).mul(mid))
		inside := fieldContainingConductor(scene, p, .006) >= 0
		if inside == aInside {
			lo = mid
		} else {
			hi = mid
		}
	}
	return a.add(b.sub(a).mul((lo + hi) * .5))
}

// fieldClipSegmentOutsideConductors removes every part of an equipotential
// contour lying in a conducting body.  This is not a cosmetic erasure: an
// equilibrium conductor is one constant-potential volume, so a different
// equipotential contour may end at its boundary but can never pass through it.
func fieldClipSegmentOutsideConductors(scene fieldScene, seg fieldSegment3) []fieldSegment3 {
	length := seg.B.sub(seg.A).len()
	if length < 1e-9 {
		return nil
	}
	steps := int(math.Ceil(length / .018))
	steps = max(8, min(steps, 160))
	inside0 := fieldContainingConductor(scene, seg.A, .006) >= 0
	var start fieldVec3
	haveStart := !inside0
	if haveStart {
		start = seg.A
	}
	prev, prevInside := seg.A, inside0
	out := make([]fieldSegment3, 0, 2)
	for i := 1; i <= steps; i++ {
		cur := fieldSegmentPoint(seg, float64(i)/float64(steps))
		curInside := fieldContainingConductor(scene, cur, .006) >= 0
		if curInside != prevInside {
			boundary := fieldRefineConductorBoundary(scene, prev, cur, prevInside)
			if !prevInside && haveStart && boundary.sub(start).len2() > 1e-10 {
				out = append(out, fieldSegment3{start, boundary})
			}
			if prevInside && !curInside {
				start, haveStart = boundary, true
			} else if curInside {
				haveStart = false
			}
		}
		prev, prevInside = cur, curInside
	}
	if !prevInside && haveStart && seg.B.sub(start).len2() > 1e-10 {
		out = append(out, fieldSegment3{start, seg.B})
	}
	return out
}

func fieldContourSlice(scene fieldScene, samples []fieldChargeSample, slice fieldPlaneSlice, steps int, levels []float64) map[float64][]fieldSegment3 {
	if steps < 12 {
		steps = 12
	}
	points := make([]fieldVec3, steps*steps)
	values := make([]float64, steps*steps)
	span := fieldSceneSpan(scene)
	for j := 0; j < steps; j++ {
		v := -span + 2*span*float64(j)/float64(steps-1)
		for i := 0; i < steps; i++ {
			u := -span + 2*span*float64(i)/float64(steps-1)
			idx := j*steps + i
			points[idx] = slice.Origin.add(slice.U.mul(u)).add(slice.V.mul(v))
			values[idx] = fieldPotentialAt(scene, samples, points[idx])
		}
	}
	out := make(map[float64][]fieldSegment3)
	appendClipped := func(level float64, seg fieldSegment3) {
		out[level] = append(out[level], fieldClipSegmentOutsideConductors(scene, seg)...)
	}
	for _, level := range levels {
		for j := 0; j < steps-1; j++ {
			for i := 0; i < steps-1; i++ {
				i00, i10 := j*steps+i, j*steps+i+1
				i01, i11 := (j+1)*steps+i, (j+1)*steps+i+1
				ps := [4]fieldVec3{points[i00], points[i10], points[i11], points[i01]}
				vs := [4]float64{values[i00], values[i10], values[i11], values[i01]}
				edges := make([]fieldVec3, 0, 4)
				for e := 0; e < 4; e++ {
					n := (e + 1) % 4
					a, b := vs[e]-level, vs[n]-level
					if (a <= 0 && b > 0) || (a > 0 && b <= 0) {
						edges = append(edges, fieldInterpolateLevel(ps[e], ps[n], vs[e], vs[n], level))
					}
				}
				if len(edges) == 2 {
					appendClipped(level, fieldSegment3{edges[0], edges[1]})
				} else if len(edges) == 4 {
					center := (vs[0] + vs[1] + vs[2] + vs[3]) * .25
					if center > level {
						appendClipped(level, fieldSegment3{edges[0], edges[3]})
						appendClipped(level, fieldSegment3{edges[1], edges[2]})
					} else {
						appendClipped(level, fieldSegment3{edges[0], edges[1]})
						appendClipped(level, fieldSegment3{edges[2], edges[3]})
					}
				}
			}
		}
	}
	return out
}

type fieldTriangle3 struct {
	A, B, C fieldVec3
	Level   float64
	// Alpha optionally overrides the membrane face opacity. Zero keeps the
	// legacy 0.16 opacity used by conductor surfaces and older call sites.
	Alpha float64
	// Boundary marks a physical open surface such as an ideal conducting
	// sheet. Numerical orphan edges on ordinary isosurfaces remain hidden.
	Boundary bool
}

type fieldIsoIndexTriangle struct {
	A, B, C int
}

type fieldIsoIndexEdge struct {
	A, B int
}

func fieldIsoIndexEdgeKey(a, b int) fieldIsoIndexEdge {
	if b < a {
		a, b = b, a
	}
	return fieldIsoIndexEdge{A: a, B: b}
}

// fieldProjectPointToEquipotential moves a smoothed mesh vertex back onto the
// actual Maxwell/BEM potential V=level.  The marching lattice only gives a
// piecewise-linear interpolation; reprojection removes the blocky "patched"
// look without replacing the physical solution by a guessed geometric shape.
func fieldProjectPointToEquipotentialIterations(scene fieldScene, samples []fieldChargeSample, p fieldVec3, level, maxStep float64, iterations int) fieldVec3 {
	if iterations < 1 || maxStep <= 0 {
		return p
	}
	q := p
	best := p
	bestValue := fieldPotentialAt(scene, samples, p)
	bestError := math.Abs(bestValue - level)
	tolerance := 1e-6 * math.Max(1, math.Abs(level))
	for iter := 0; iter < iterations; iter++ {
		v := fieldPotentialAt(scene, samples, q)
		e := fieldElectricAt(scene, samples, q)
		e2 := e.len2()
		if e2 < 1e-12 || math.IsNaN(v) || math.IsInf(v, 0) || !fieldFiniteVec(e) {
			break
		}
		currentError := math.Abs(v - level)
		if currentError < bestError {
			best, bestError = q, currentError
		}
		if currentError <= tolerance {
			break
		}
		// E=-grad(V), hence delta=(V-level)E/|E|^2 gives
		// V(q+delta)=level to first order.  The step is safeguarded below so
		// a strong near-charge gradient can never throw the vertex across a
		// conductor or increase the residual catastrophically.
		delta := e.mul((v - level) / e2)
		if dl := delta.len(); dl > maxStep && dl > 1e-12 {
			delta = delta.mul(maxStep / dl)
		}
		accepted := false
		for damping := 0; damping < 5; damping++ {
			next := q.add(delta)
			if fieldContainingConductor(scene, next, .001) >= 0 {
				delta = delta.mul(.5)
				continue
			}
			nextValue := fieldPotentialAt(scene, samples, next)
			nextError := math.Abs(nextValue - level)
			if !math.IsNaN(nextValue) && !math.IsInf(nextValue, 0) && nextError <= currentError*(1+1e-8) {
				q = next
				accepted = true
				if nextError < bestError {
					best, bestError = next, nextError
				}
				break
			}
			delta = delta.mul(.5)
		}
		if !accepted {
			break
		}
	}
	return best
}

func fieldProjectPointToEquipotential(scene fieldScene, samples []fieldChargeSample, p fieldVec3, level, maxStep float64) fieldVec3 {
	return fieldProjectPointToEquipotentialIterations(scene, samples, p, level, maxStep, 4)
}

// fieldReprojectIsoSurfaceVertices corrects the piecewise-linear marching-cubes
// intersection against the actual electrostatic solution.  Shared vertices are
// welded before projection so adjacent triangles remain watertight, and the
// work is spread across CPU cores because each vertex correction is independent.
func fieldReprojectIsoSurfaceVertices(scene fieldScene, samples []fieldChargeSample, tris []fieldTriangle3, level, gridStep float64, iterations int) []fieldTriangle3 {
	if len(tris) == 0 || iterations < 1 {
		return tris
	}
	type indexedTriangle struct {
		A, B, C int
		Proto   fieldTriangle3
	}
	vertexByKey := make(map[fieldMembranePointKey]int, len(tris)*2)
	vertices := make([]fieldVec3, 0, len(tris))
	counts := make([]int, 0, len(tris))
	indexed := make([]indexedTriangle, 0, len(tris))
	addVertex := func(v fieldVec3) int {
		key := fieldMembranePointQuantize(v)
		if idx, ok := vertexByKey[key]; ok {
			c := float64(counts[idx])
			vertices[idx] = vertices[idx].mul(c).add(v).mul(1 / (c + 1))
			counts[idx]++
			return idx
		}
		idx := len(vertices)
		vertexByKey[key] = idx
		vertices = append(vertices, v)
		counts = append(counts, 1)
		return idx
	}
	for _, tri := range tris {
		a, b, c := addVertex(tri.A), addVertex(tri.B), addVertex(tri.C)
		if a == b || b == c || c == a {
			continue
		}
		indexed = append(indexed, indexedTriangle{A: a, B: b, C: c, Proto: tri})
	}
	if len(indexed) == 0 {
		return nil
	}
	projected := make([]fieldVec3, len(vertices))
	maxMove := math.Max(.004, gridStep*.42)
	workers := min(runtime.GOMAXPROCS(0), max(1, len(vertices)/512))
	if workers < 2 {
		for i, v := range vertices {
			projected[i] = fieldProjectPointToEquipotentialIterations(scene, samples, v, level, maxMove, iterations)
		}
	} else {
		var wg sync.WaitGroup
		chunk := (len(vertices) + workers - 1) / workers
		for worker := 0; worker < workers; worker++ {
			start := worker * chunk
			end := min(len(vertices), start+chunk)
			if start >= end {
				continue
			}
			wg.Add(1)
			go func(start, end int) {
				defer wg.Done()
				for i := start; i < end; i++ {
					projected[i] = fieldProjectPointToEquipotentialIterations(scene, samples, vertices[i], level, maxMove, iterations)
				}
			}(start, end)
		}
		wg.Wait()
	}
	out := make([]fieldTriangle3, 0, len(indexed))
	for _, it := range indexed {
		a, b, c := projected[it.A], projected[it.B], projected[it.C]
		n := b.sub(a).cross(c.sub(a))
		if n.len2() < 1e-12 {
			continue
		}
		centroid := a.add(b).add(c).mul(1.0 / 3.0)
		grad := fieldElectricAt(scene, samples, centroid).mul(-1)
		if grad.len2() > 1e-12 && n.dot(grad) < 0 {
			b, c = c, b
		}
		proto := it.Proto
		proto.A, proto.B, proto.C = a, b, c
		out = append(out, proto)
	}
	return out
}

// fieldImproveIsoSurface welds shared vertices, applies a constrained
// Laplacian relaxation, and reprojects every moved vertex onto V=level.  It
// keeps open boundaries fixed and reorients triangles against grad(V), which
// yields a smooth, consistently connected membrane instead of a pile of
// independently shaded tetrahedron fragments.
func fieldImproveIsoSurface(scene fieldScene, samples []fieldChargeSample, tris []fieldTriangle3, level, gridStep float64, iterations int) []fieldTriangle3 {
	if len(tris) < 4 || iterations < 1 {
		return tris
	}
	vertexByKey := make(map[fieldMembranePointKey]int, len(tris)*2)
	vertices := make([]fieldVec3, 0, len(tris))
	counts := make([]int, 0, len(tris))
	indexed := make([]fieldIsoIndexTriangle, 0, len(tris))
	addVertex := func(v fieldVec3) int {
		key := fieldMembranePointQuantize(v)
		if idx, ok := vertexByKey[key]; ok {
			c := float64(counts[idx])
			vertices[idx] = vertices[idx].mul(c).add(v).mul(1 / (c + 1))
			counts[idx]++
			return idx
		}
		idx := len(vertices)
		vertexByKey[key] = idx
		vertices = append(vertices, v)
		counts = append(counts, 1)
		return idx
	}
	for _, tri := range tris {
		a, b, c := addVertex(tri.A), addVertex(tri.B), addVertex(tri.C)
		if a == b || b == c || c == a {
			continue
		}
		indexed = append(indexed, fieldIsoIndexTriangle{A: a, B: b, C: c})
	}
	if len(indexed) < 4 {
		return tris
	}
	edgeCount := make(map[fieldIsoIndexEdge]int, len(indexed)*2)
	neighbors := make([]map[int]struct{}, len(vertices))
	for i := range neighbors {
		neighbors[i] = make(map[int]struct{}, 8)
	}
	for _, tri := range indexed {
		for _, edge := range [][2]int{{tri.A, tri.B}, {tri.B, tri.C}, {tri.C, tri.A}} {
			k := fieldIsoIndexEdgeKey(edge[0], edge[1])
			edgeCount[k]++
			neighbors[edge[0]][edge[1]] = struct{}{}
			neighbors[edge[1]][edge[0]] = struct{}{}
		}
	}
	boundary := make([]bool, len(vertices))
	for edge, count := range edgeCount {
		if count == 1 {
			boundary[edge.A], boundary[edge.B] = true, true
		}
	}
	maxMove := math.Max(gridStep*.32, .008)
	for iter := 0; iter < iterations; iter++ {
		next := append([]fieldVec3(nil), vertices...)
		for i, v := range vertices {
			if boundary[i] || len(neighbors[i]) < 3 {
				continue
			}
			avg := fieldVec3{}
			for n := range neighbors[i] {
				avg = avg.add(vertices[n])
			}
			avg = avg.mul(1 / float64(len(neighbors[i])))
			candidate := v.add(avg.sub(v).mul(.42))
			delta := candidate.sub(v)
			if dl := delta.len(); dl > maxMove && dl > 1e-12 {
				candidate = v.add(delta.mul(maxMove / dl))
			}
			next[i] = fieldProjectPointToEquipotential(scene, samples, candidate, level, maxMove*.85)
		}
		vertices = next
	}
	out := make([]fieldTriangle3, 0, len(indexed))
	for _, it := range indexed {
		a, b, c := vertices[it.A], vertices[it.B], vertices[it.C]
		n := b.sub(a).cross(c.sub(a))
		if n.len2() < 1e-11 {
			continue
		}
		centroid := a.add(b).add(c).mul(1.0 / 3.0)
		grad := fieldElectricAt(scene, samples, centroid).mul(-1)
		if grad.len2() > 1e-12 && n.dot(grad) < 0 {
			b, c = c, b
		}
		out = append(out, fieldTriangle3{A: a, B: b, C: c, Level: level, Alpha: .16, Boundary: true})
	}
	return out
}

// fieldFilterClosedIsoComponents removes numerical scraps after extraction.
// A rendered level that neither touches the sampling cube nor equals a
// conductor potential must be a closed surface in vacuum.  Tiny islands and
// components with one-sided mesh edges are therefore marching artefacts, not
// physical equipotential membranes.
func fieldFilterClosedIsoComponents(tris []fieldTriangle3, gridStep float64) []fieldTriangle3 {
	if len(tris) < 4 {
		return nil
	}
	edgeTris := make(map[fieldMembraneEdgeID][]int, len(tris)*2)
	adj := make([][]int, len(tris))
	for ti, tri := range tris {
		for _, e := range [][2]fieldVec3{{tri.A, tri.B}, {tri.B, tri.C}, {tri.C, tri.A}} {
			key := fieldMembraneEdgeKey(e[0], e[1], tri.Level)
			edgeTris[key] = append(edgeTris[key], ti)
		}
	}
	for _, owners := range edgeTris {
		if len(owners) == 2 {
			a, b := owners[0], owners[1]
			adj[a] = append(adj[a], b)
			adj[b] = append(adj[b], a)
		}
	}
	type component struct {
		tris     []int
		area     float64
		boundary bool
	}
	visited := make([]bool, len(tris))
	components := make([]component, 0, 4)
	for start := range tris {
		if visited[start] {
			continue
		}
		queue := []int{start}
		visited[start] = true
		c := component{}
		for len(queue) > 0 {
			ti := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			c.tris = append(c.tris, ti)
			tri := tris[ti]
			c.area += tri.B.sub(tri.A).cross(tri.C.sub(tri.A)).len() * .5
			for _, e := range [][2]fieldVec3{{tri.A, tri.B}, {tri.B, tri.C}, {tri.C, tri.A}} {
				owners := edgeTris[fieldMembraneEdgeKey(e[0], e[1], tri.Level)]
				if len(owners) != 2 {
					c.boundary = true
				}
			}
			for _, next := range adj[ti] {
				if !visited[next] {
					visited[next] = true
					queue = append(queue, next)
				}
			}
		}
		components = append(components, c)
	}
	largest := 0.0
	for _, c := range components {
		if !c.boundary && len(c.tris) >= 8 {
			largest = math.Max(largest, c.area)
		}
	}
	if largest <= 0 {
		return nil
	}
	minArea := math.Max(gridStep*gridStep*1.2, largest*.003)
	out := make([]fieldTriangle3, 0, len(tris))
	for _, c := range components {
		if c.boundary || len(c.tris) < 8 || c.area < minArea {
			continue
		}
		for _, ti := range c.tris {
			out = append(out, tris[ti])
		}
	}
	return out
}

// fieldFilterStableIsoComponents keeps the strict closed components whenever
// they exist, but salvages a large nearly-closed physical sheet when tiny
// quantisation cracks are the only reason the strict test failed.  The old
// all-or-nothing filter turned a one-edge welding mismatch into a full
// 200,000-triangle disappearance.  Small scraps are still rejected.
func fieldFilterStableIsoComponents(tris []fieldTriangle3, gridStep float64) []fieldTriangle3 {
	if strict := fieldFilterClosedIsoComponents(tris, gridStep); len(strict) > 0 {
		return strict
	}
	if len(tris) < 8 {
		return nil
	}
	edgeTris := make(map[fieldMembraneEdgeID][]int, len(tris)*2)
	adj := make([][]int, len(tris))
	for ti, tri := range tris {
		for _, e := range [][2]fieldVec3{{tri.A, tri.B}, {tri.B, tri.C}, {tri.C, tri.A}} {
			key := fieldMembraneEdgeKey(e[0], e[1], tri.Level)
			edgeTris[key] = append(edgeTris[key], ti)
		}
	}
	for _, owners := range edgeTris {
		if len(owners) == 2 {
			a, b := owners[0], owners[1]
			adj[a] = append(adj[a], b)
			adj[b] = append(adj[b], a)
		}
	}
	type component struct {
		tris          []int
		area          float64
		uniqueEdges   int
		boundaryEdges int
	}
	visited := make([]bool, len(tris))
	components := make([]component, 0, 8)
	for start := range tris {
		if visited[start] {
			continue
		}
		queue := []int{start}
		visited[start] = true
		c := component{}
		edges := make(map[fieldMembraneEdgeID]struct{})
		for len(queue) > 0 {
			ti := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			c.tris = append(c.tris, ti)
			tri := tris[ti]
			c.area += tri.B.sub(tri.A).cross(tri.C.sub(tri.A)).len() * .5
			for _, e := range [][2]fieldVec3{{tri.A, tri.B}, {tri.B, tri.C}, {tri.C, tri.A}} {
				key := fieldMembraneEdgeKey(e[0], e[1], tri.Level)
				edges[key] = struct{}{}
			}
			for _, next := range adj[ti] {
				if !visited[next] {
					visited[next] = true
					queue = append(queue, next)
				}
			}
		}
		for edge := range edges {
			c.uniqueEdges++
			if len(edgeTris[edge]) != 2 {
				c.boundaryEdges++
			}
		}
		components = append(components, c)
	}
	largest := 0.0
	for _, c := range components {
		if len(c.tris) >= 8 {
			largest = math.Max(largest, c.area)
		}
	}
	if largest <= 0 {
		return nil
	}
	minArea := math.Max(gridStep*gridStep*.8, largest*.002)
	out := make([]fieldTriangle3, 0, len(tris))
	for _, c := range components {
		if len(c.tris) < 8 || c.area < minArea || c.uniqueEdges == 0 {
			continue
		}
		leakRatio := float64(c.boundaryEdges) / float64(c.uniqueEdges)
		keep := c.boundaryEdges == 0
		if !keep && c.area >= largest*.985 {
			limit := max(24, int(math.Sqrt(float64(len(c.tris))))*10)
			keep = leakRatio <= .16 && c.boundaryEdges <= limit
		}
		if !keep && c.area >= largest*.12 {
			keep = leakRatio <= .035
		}
		if !keep {
			continue
		}
		for _, ti := range c.tris {
			out = append(out, tris[ti])
		}
	}
	return out
}

// fieldIsoTriangleNeedsAdaptiveRefinement identifies parts of an extracted
// membrane where a uniform lattice is least accurate: close to a free charge,
// close to a conductor boundary, or where |grad(V)| changes the level by a
// significant fraction over one triangle.  Refinement is geometric only after
// the Maxwell/BEM solve, and every new vertex is reprojected onto V=level.
func fieldIsoTriangleNeedsAdaptiveRefinement(scene fieldScene, samples []fieldChargeSample, tri fieldTriangle3, gridStep float64, round int) bool {
	edges := []float64{tri.B.sub(tri.A).len(), tri.C.sub(tri.B).len(), tri.A.sub(tri.C).len()}
	maxEdge := math.Max(edges[0], math.Max(edges[1], edges[2]))
	if maxEdge < math.Max(.006, gridStep*.24) {
		return false
	}
	centroid := tri.A.add(tri.B).add(tri.C).mul(1.0 / 3.0)
	nearestCharge := math.Inf(1)
	nearChargeRadius := gridStep * 5.2
	nearestConductor := math.Inf(1)
	for _, o := range scene.Objects {
		if !o.electric() {
			continue
		}
		if o.Material == fieldMaterialConductor {
			nearestConductor = math.Min(nearestConductor, fieldDistanceToObjectSurface(o, centroid))
		}
		if o.Kind == fieldObjectSphere && o.Material != fieldMaterialConductor && math.Abs(o.Charge) > 1e-10 {
			d := centroid.sub(fieldObjectCenter(o)).len()
			nearestCharge = math.Min(nearestCharge, d)
			nearChargeRadius = math.Max(nearChargeRadius, o.Radius+gridStep*4.2)
		}
	}
	if nearestCharge < nearChargeRadius {
		return true
	}
	// Refine the induced boundary layer only near the part of a conductor that
	// is influenced by a nearby free charge.  Refining an entire 24.5-unit
	// plate would add millions of triangles without improving the local field.
	if nearestConductor < gridStep*1.35 && nearestCharge < gridStep*9.0 {
		return true
	}
	if nearestCharge < gridStep*11.0 {
		e := fieldElectricAt(scene, samples, centroid)
		if fieldFiniteVec(e) {
			relativeVariation := e.len() * maxEdge / math.Max(math.Abs(tri.Level), .20)
			return relativeVariation > .34
		}
	}
	return false
}

func fieldAdaptiveRefineIsoSurface(scene fieldScene, samples []fieldChargeSample, tris []fieldTriangle3, level, gridStep float64, rounds int) []fieldTriangle3 {
	if len(tris) == 0 || rounds < 1 {
		return tris
	}
	current := tris
	for round := 0; round < rounds; round++ {
		marked := make(map[fieldMembraneEdgeID]struct{}, len(current))
		for _, tri := range current {
			if !fieldIsoTriangleNeedsAdaptiveRefinement(scene, samples, tri, gridStep, round) {
				continue
			}
			for _, edge := range [][2]fieldVec3{{tri.A, tri.B}, {tri.B, tri.C}, {tri.C, tri.A}} {
				marked[fieldMembraneEdgeKey(edge[0], edge[1], tri.Level)] = struct{}{}
			}
		}
		if len(marked) == 0 {
			break
		}
		midpoints := make(map[fieldMembraneEdgeID]fieldVec3, len(marked))
		midpoint := func(a, b fieldVec3, edge fieldMembraneEdgeID) fieldVec3 {
			if p, ok := midpoints[edge]; ok {
				return p
			}
			m := a.add(b).mul(.5)
			maxMove := math.Max(.004, b.sub(a).len()*.42)
			m = fieldProjectPointToEquipotential(scene, samples, m, level, maxMove)
			midpoints[edge] = m
			return m
		}
		next := make([]fieldTriangle3, 0, len(current)*2)
		add := func(a, b, c fieldVec3, proto fieldTriangle3) {
			if b.sub(a).cross(c.sub(a)).len2() < 1e-12 {
				return
			}
			next = append(next, fieldTriangle3{A: a, B: b, C: c, Level: proto.Level, Boundary: proto.Boundary})
		}
		for _, tri := range current {
			kab := fieldMembraneEdgeKey(tri.A, tri.B, tri.Level)
			kbc := fieldMembraneEdgeKey(tri.B, tri.C, tri.Level)
			kca := fieldMembraneEdgeKey(tri.C, tri.A, tri.Level)
			_, sab := marked[kab]
			_, sbc := marked[kbc]
			_, sca := marked[kca]
			count := 0
			if sab {
				count++
			}
			if sbc {
				count++
			}
			if sca {
				count++
			}
			if count == 0 {
				next = append(next, tri)
				continue
			}
			var mab, mbc, mca fieldVec3
			if sab {
				mab = midpoint(tri.A, tri.B, kab)
			}
			if sbc {
				mbc = midpoint(tri.B, tri.C, kbc)
			}
			if sca {
				mca = midpoint(tri.C, tri.A, kca)
			}
			switch {
			case sab && !sbc && !sca:
				add(tri.A, mab, tri.C, tri)
				add(mab, tri.B, tri.C, tri)
			case !sab && sbc && !sca:
				add(tri.B, mbc, tri.A, tri)
				add(mbc, tri.C, tri.A, tri)
			case !sab && !sbc && sca:
				add(tri.C, mca, tri.B, tri)
				add(mca, tri.A, tri.B, tri)
			case sab && sbc && !sca:
				add(tri.B, mbc, mab, tri)
				add(tri.A, mab, tri.C, tri)
				add(mab, mbc, tri.C, tri)
			case !sab && sbc && sca:
				add(tri.C, mca, mbc, tri)
				add(tri.A, tri.B, mca, tri)
				add(tri.B, mbc, mca, tri)
			case sab && !sbc && sca:
				add(tri.A, mab, mca, tri)
				add(mab, tri.B, mca, tri)
				add(tri.B, tri.C, mca, tri)
			default:
				add(tri.A, mab, mca, tri)
				add(mab, tri.B, mbc, tri)
				add(mca, mbc, tri.C, tri)
				add(mab, mbc, mca, tri)
			}
		}
		if len(next) == 0 || len(next) > 300000 {
			break
		}
		current = next
		gridStep *= .5
	}
	return current
}

type fieldGPUFaceVertex struct {
	X, Y, Z    float32
	R, G, B, A float32
}

type fieldProjectedTriangle struct {
	Tri    fieldTriangle3
	X      [3]float64
	Y      [3]float64
	Depth  [3]float64
	MinX   float64
	MaxX   float64
	MinY   float64
	MaxY   float64
	AvgDep float64
}

type fieldSurfaceEdge struct {
	A, B         fieldVec3
	Level        float64
	NormalA      fieldVec3
	NormalB      fieldVec3
	HasB         bool
	OpenBoundary bool
}

func fieldPotentialColor(level float64, dark bool) (byte, byte, byte) {
	rr, gg, bb := byte(75), byte(75), byte(75)
	if dark {
		rr, gg, bb = 185, 185, 185
	}
	if level > 0 {
		rr, gg, bb = 50, 105, 215
	} else if level < 0 {
		rr, gg, bb = 215, 70, 60
	}
	return rr, gg, bb
}

func fieldIsoInterpolate(a, b fieldVec3, va, vb, level float64) fieldVec3 {
	t := .5
	if math.Abs(vb-va) > 1e-12 {
		t = (level - va) / (vb - va)
	}
	t = fieldClamp(t, 0, 1)
	return a.add(b.sub(a).mul(t))
}

func fieldIsoTrianglesFromTet(pts [4]fieldVec3, vals [4]float64, level float64) []fieldTriangle3 {
	pos := make([]int, 0, 4)
	neg := make([]int, 0, 4)
	for i, v := range vals {
		if v >= level {
			pos = append(pos, i)
		} else {
			neg = append(neg, i)
		}
	}
	switch len(pos) {
	case 0, 4:
		return nil
	case 1:
		p := pos[0]
		a := fieldIsoInterpolate(pts[p], pts[neg[0]], vals[p], vals[neg[0]], level)
		b := fieldIsoInterpolate(pts[p], pts[neg[1]], vals[p], vals[neg[1]], level)
		c := fieldIsoInterpolate(pts[p], pts[neg[2]], vals[p], vals[neg[2]], level)
		return []fieldTriangle3{{A: a, B: b, C: c, Level: level}}
	case 3:
		n := neg[0]
		a := fieldIsoInterpolate(pts[n], pts[pos[0]], vals[n], vals[pos[0]], level)
		b := fieldIsoInterpolate(pts[n], pts[pos[1]], vals[n], vals[pos[1]], level)
		c := fieldIsoInterpolate(pts[n], pts[pos[2]], vals[n], vals[pos[2]], level)
		return []fieldTriangle3{{A: a, B: c, C: b, Level: level}}
	case 2:
		p0, p1 := pos[0], pos[1]
		n0, n1 := neg[0], neg[1]
		a := fieldIsoInterpolate(pts[p0], pts[n0], vals[p0], vals[n0], level)
		b := fieldIsoInterpolate(pts[p0], pts[n1], vals[p0], vals[n1], level)
		c := fieldIsoInterpolate(pts[p1], pts[n0], vals[p1], vals[n0], level)
		d := fieldIsoInterpolate(pts[p1], pts[n1], vals[p1], vals[n1], level)
		return []fieldTriangle3{{A: a, B: b, C: c, Level: level}, {A: c, B: b, C: d, Level: level}}
	}
	return nil
}

func fieldConductorPotentialValues(scene fieldScene, samples []fieldChargeSample) map[int]float64 {
	out := make(map[int]float64)
	for i, o := range scene.Objects {
		if !o.electric() || o.Material != fieldMaterialConductor {
			continue
		}
		if v, ok := fieldConductorPotential(scene, samples, i); ok {
			out[i] = v
			continue
		}
		// A conductor boundary is a physical equipotential even when a coarse
		// preview solve becomes singular.  Keep the membrane visible with the
		// finite vacuum potential at its centre rather than dropping it for one
		// frame.  The next full-quality solve replaces this fallback value.
		v := fieldPotentialFromSamples(samples, fieldObjectCenter(o))
		if math.IsNaN(v) || math.IsInf(v, 0) {
			v = 0
		}
		out[i] = v
	}
	return out
}

// fieldConductorEquipotentialTriangles emits the physical surface of every
// conductor as an explicit V=Vc membrane.  Marching cubes deliberately avoids
// the numerically thick conductor volume, so without this mesh the most
// important equipotential surface—the conductor itself—can never be shown.
func fieldConductorEquipotentialTriangles(scene fieldScene, samples []fieldChargeSample, quality int) []fieldTriangle3 {
	potentials := fieldConductorPotentialValues(scene, samples)
	out := make([]fieldTriangle3, 0, 512)
	appendOriented := func(a, b, c, outward fieldVec3, level float64) {
		n := b.sub(a).cross(c.sub(a))
		if n.len2() < 1e-12 {
			return
		}
		if outward.len2() > 1e-12 && n.dot(outward) < 0 {
			b, c = c, b
		}
		out = append(out, fieldTriangle3{A: a, B: b, C: c, Level: level, Alpha: .16, Boundary: true})
	}
	for object, level := range potentials {
		if object < 0 || object >= len(scene.Objects) {
			continue
		}
		o := scene.Objects[object]
		center := fieldObjectCenter(o)
		ex, ey, ez := fieldObjectBasis(o)
		switch o.Kind {
		case fieldObjectPlane:
			h := o.Size * .5
			p0 := center.add(ex.mul(-h)).add(ez.mul(-h))
			p1 := center.add(ex.mul(h)).add(ez.mul(-h))
			p2 := center.add(ex.mul(h)).add(ez.mul(h))
			p3 := center.add(ex.mul(-h)).add(ez.mul(h))
			appendOriented(p0, p1, p2, ey, level)
			appendOriented(p0, p2, p3, ey, level)
		case fieldObjectSphere:
			latSteps, lonSteps := 14, 28
			if quality > 0 {
				latSteps, lonSteps = 18, 36
			}
			if quality > 1 {
				latSteps, lonSteps = 22, 44
			}
			point := func(theta, phi float64) fieldVec3 {
				ct := math.Cos(theta)
				return center.add(ex.mul(o.Radius * ct * math.Cos(phi))).
					add(ey.mul(o.Radius * ct * math.Sin(phi))).
					add(ez.mul(o.Radius * math.Sin(theta)))
			}
			for lat := 0; lat < latSteps; lat++ {
				t0 := -math.Pi/2 + math.Pi*float64(lat)/float64(latSteps)
				t1 := -math.Pi/2 + math.Pi*float64(lat+1)/float64(latSteps)
				for lon := 0; lon < lonSteps; lon++ {
					p0a := 2 * math.Pi * float64(lon) / float64(lonSteps)
					p1a := 2 * math.Pi * float64(lon+1) / float64(lonSteps)
					a, b := point(t0, p0a), point(t0, p1a)
					c, d := point(t1, p1a), point(t1, p0a)
					if lat == 0 {
						appendOriented(a, c, d, a.add(c).add(d).mul(1.0/3.0).sub(center), level)
					} else if lat == latSteps-1 {
						appendOriented(a, b, c, a.add(b).add(c).mul(1.0/3.0).sub(center), level)
					} else {
						appendOriented(a, b, c, a.add(b).add(c).mul(1.0/3.0).sub(center), level)
						appendOriented(a, c, d, a.add(c).add(d).mul(1.0/3.0).sub(center), level)
					}
				}
			}
		case fieldObjectRod:
			segments := 28
			if quality > 0 {
				segments = 36
			}
			radius := math.Max(.10, math.Min(.22, o.Length*.11))
			left := center.add(ex.mul(-o.Length * .5))
			right := center.add(ex.mul(o.Length * .5))
			for i := 0; i < segments; i++ {
				a0 := 2 * math.Pi * float64(i) / float64(segments)
				a1 := 2 * math.Pi * float64(i+1) / float64(segments)
				r0 := ey.mul(radius * math.Cos(a0)).add(ez.mul(radius * math.Sin(a0)))
				r1 := ey.mul(radius * math.Cos(a1)).add(ez.mul(radius * math.Sin(a1)))
				l0, l1 := left.add(r0), left.add(r1)
				rr0, rr1 := right.add(r0), right.add(r1)
				appendOriented(l0, rr0, rr1, r0.add(r1), level)
				appendOriented(l0, rr1, l1, r0.add(r1), level)
				appendOriented(left, l1, l0, ex.mul(-1), level)
				appendOriented(right, rr0, rr1, ex, level)
			}
		}
	}
	return out
}

func fieldIsoSurfaceTouchesGridBoundary(tris []fieldTriangle3, minCoord, maxCoord, tolerance float64) bool {
	for _, tri := range tris {
		for _, p := range []fieldVec3{tri.A, tri.B, tri.C} {
			if p.X <= minCoord+tolerance || p.X >= maxCoord-tolerance ||
				p.Y <= minCoord+tolerance || p.Y >= maxCoord-tolerance ||
				p.Z <= minCoord+tolerance || p.Z >= maxCoord-tolerance {
				return true
			}
		}
	}
	return false
}

func fieldIsoSurfaceTouchesGridBoundaryCentered(tris []fieldTriangle3, center fieldVec3, minCoord, maxCoord, tolerance float64) bool {
	if center.len2() < 1e-24 {
		return fieldIsoSurfaceTouchesGridBoundary(tris, minCoord, maxCoord, tolerance)
	}
	for _, tri := range tris {
		for _, world := range []fieldVec3{tri.A, tri.B, tri.C} {
			p := world.sub(center)
			if p.X <= minCoord+tolerance || p.X >= maxCoord-tolerance ||
				p.Y <= minCoord+tolerance || p.Y >= maxCoord-tolerance ||
				p.Z <= minCoord+tolerance || p.Z >= maxCoord-tolerance {
				return true
			}
		}
	}
	return false
}

// fieldFilterLargeOpenIsoComponents keeps physically meaningful isosurfaces
// that leave the finite numerical cube.  This is essential for mixed-sign
// fields: the exact V=0 separator of a dipole is commonly an open plane that
// reaches infinity, so a closed-surface-only filter would erase the most
// important membrane.  Tiny disconnected marching-cubes scraps are still
// rejected by relative area and triangle count.
func fieldFilterLargeOpenIsoComponents(tris []fieldTriangle3, gridStep float64) []fieldTriangle3 {
	if len(tris) < 2 {
		return nil
	}
	edgeTris := make(map[fieldMembraneEdgeID][]int, len(tris)*2)
	adj := make([][]int, len(tris))
	for ti, tri := range tris {
		for _, e := range [][2]fieldVec3{{tri.A, tri.B}, {tri.B, tri.C}, {tri.C, tri.A}} {
			key := fieldMembraneEdgeKey(e[0], e[1], tri.Level)
			edgeTris[key] = append(edgeTris[key], ti)
		}
	}
	for _, owners := range edgeTris {
		if len(owners) == 2 {
			a, b := owners[0], owners[1]
			adj[a] = append(adj[a], b)
			adj[b] = append(adj[b], a)
		}
	}
	type component struct {
		tris []int
		area float64
	}
	visited := make([]bool, len(tris))
	components := make([]component, 0, 4)
	largest := 0.0
	for start := range tris {
		if visited[start] {
			continue
		}
		queue := []int{start}
		visited[start] = true
		c := component{}
		for len(queue) > 0 {
			ti := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			c.tris = append(c.tris, ti)
			tri := tris[ti]
			c.area += tri.B.sub(tri.A).cross(tri.C.sub(tri.A)).len() * .5
			for _, next := range adj[ti] {
				if !visited[next] {
					visited[next] = true
					queue = append(queue, next)
				}
			}
		}
		components = append(components, c)
		largest = math.Max(largest, c.area)
	}
	if largest <= 0 {
		return nil
	}
	minArea := math.Max(gridStep*gridStep*2.0, largest*.006)
	out := make([]fieldTriangle3, 0, len(tris))
	for _, c := range components {
		if len(c.tris) < 4 || c.area < minArea {
			continue
		}
		for _, ti := range c.tris {
			out = append(out, tris[ti])
		}
	}
	return out
}

func fieldPotentialGridHasBothSigns(grid fieldPotentialGrid) bool {
	negative, positive := false, false
	for i, value := range grid.Values {
		if i < len(grid.Conductor) && grid.Conductor[i] {
			continue
		}
		if value < -1e-8 {
			negative = true
		} else if value > 1e-8 {
			positive = true
		}
		if negative && positive {
			return true
		}
	}
	return false
}

func fieldCubeTouchesConductor(scene fieldScene, cube [8]fieldVec3) bool {
	for _, o := range scene.Objects {
		if !o.electric() || o.Material != fieldMaterialConductor {
			continue
		}
		ex, ey, ez := fieldObjectBasis(o)
		center := fieldObjectCenter(o)
		minX, minY, minZ := math.Inf(1), math.Inf(1), math.Inf(1)
		maxX, maxY, maxZ := math.Inf(-1), math.Inf(-1), math.Inf(-1)
		for _, p := range cube {
			q := p.sub(center)
			x, y, z := q.dot(ex), q.dot(ey), q.dot(ez)
			minX, maxX = math.Min(minX, x), math.Max(maxX, x)
			minY, maxY = math.Min(minY, y), math.Max(maxY, y)
			minZ, maxZ = math.Min(minZ, z), math.Max(maxZ, z)
		}
		hx, hy, hz := 0.0, 0.0, 0.0
		switch o.Kind {
		case fieldObjectSphere:
			hx, hy, hz = o.Radius, o.Radius, o.Radius
		case fieldObjectRod:
			hx = o.Length * .5
			hy = math.Max(.10, math.Min(.22, o.Length*.11))
			hz = hy
		case fieldObjectPlane:
			hx, hy, hz = o.Size*.5, .075, o.Size*.5
		default:
			continue
		}
		if minX <= hx && maxX >= -hx && minY <= hy && maxY >= -hy && minZ <= hz && maxZ >= -hz {
			return true
		}
	}
	return false
}

func fieldPotentialGridCPU(samples []fieldChargeSample, points []fieldVec3, cancelled func() bool) []float64 {
	values := make([]float64, len(points))
	if len(points) == 0 {
		return values
	}
	workers := min(runtime.GOMAXPROCS(0), max(1, len(points)/4096))
	if workers < 2 {
		for i, p := range points {
			if i&255 == 0 && cancelled != nil && cancelled() {
				return values
			}
			values[i] = fieldPotentialFromSamples(samples, p)
		}
		return values
	}
	var wg sync.WaitGroup
	chunk := (len(points) + workers - 1) / workers
	for worker := 0; worker < workers; worker++ {
		start := worker * chunk
		end := min(len(points), start+chunk)
		if start >= end {
			continue
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for i := start; i < end; i++ {
				if i&255 == 0 && cancelled != nil && cancelled() {
					return
				}
				values[i] = fieldPotentialFromSamples(samples, points[i])
			}
		}(start, end)
	}
	wg.Wait()
	return values
}

type fieldPotentialGrid struct {
	Steps              int
	Center             fieldVec3
	MinCoord, MaxCoord float64
	Values             []float64
	Conductor          []bool
	Backend            string
	Elapsed            time.Duration
}

func (g fieldPotentialGrid) valid() bool {
	return g.Steps >= 2 && len(g.Values) == g.Steps*g.Steps*g.Steps && g.MaxCoord > g.MinCoord
}

func (g fieldPotentialGrid) index(i, j, k int) int {
	return (k*g.Steps+j)*g.Steps + i
}

func (g fieldPotentialGrid) point(i, j, k int) fieldVec3 {
	d := (g.MaxCoord - g.MinCoord) / float64(g.Steps-1)
	return g.Center.add(fieldVec3{X: g.MinCoord + float64(i)*d, Y: g.MinCoord + float64(j)*d, Z: g.MinCoord + float64(k)*d})
}

// fieldEquipotentialGridCenter follows the electrically active scene instead
// of pinning the scalar lattice to the world origin.  The physical field is
// translation invariant, but a world-origin lattice made tiny object drags
// change the marching-cubes phase abruptly.  Centering the cube on the active
// source/conductor bounds keeps the same physical configuration aligned with
// nearly the same cells and prevents 200k-triangle surfaces from collapsing to
// zero merely because an object crossed an unrelated world-grid plane.
func fieldEquipotentialGridCenter(scene fieldScene) fieldVec3 {
	minV := fieldVec3{math.Inf(1), math.Inf(1), math.Inf(1)}
	maxV := fieldVec3{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	found := false
	for _, o := range scene.Objects {
		active := o.electric()
		if scene.Display.gravity() {
			active = o.gravity()
		}
		if !active {
			continue
		}
		c := fieldObjectCenter(o)
		e := fieldObjectAxisExtent(o)
		minV.X = math.Min(minV.X, c.X-e.X)
		minV.Y = math.Min(minV.Y, c.Y-e.Y)
		minV.Z = math.Min(minV.Z, c.Z-e.Z)
		maxV.X = math.Max(maxV.X, c.X+e.X)
		maxV.Y = math.Max(maxV.Y, c.Y+e.Y)
		maxV.Z = math.Max(maxV.Z, c.Z+e.Z)
		found = true
	}
	if !found {
		return fieldVec3{}
	}
	return minV.add(maxV).mul(.5)
}

func fieldShiftChargeSamples(samples []fieldChargeSample, center fieldVec3) []fieldChargeSample {
	if center.len2() < 1e-24 {
		return samples
	}
	shifted := append([]fieldChargeSample(nil), samples...)
	for i := range shifted {
		shifted[i].Pos = shifted[i].Pos.sub(center)
		if shifted[i].Gate {
			shifted[i].GateOffset -= shifted[i].GateNormal.dot(center)
		}
	}
	return shifted
}

func fieldBuildPotentialGrid(scene fieldScene, samples []fieldChargeSample, steps int) fieldPotentialGrid {
	if steps < 9 {
		steps = 9
	}
	span := fieldEquipotentialSamplingSpan(scene)
	center := fieldEquipotentialGridCenter(scene)
	minCoord, maxCoord := -span, span
	idx := func(i, j, k int) int { return (k*steps+j)*steps + i }
	points := make([]fieldVec3, steps*steps*steps)
	for k := 0; k < steps; k++ {
		if scene.cancelled() {
			return fieldPotentialGrid{}
		}
		z := minCoord + (maxCoord-minCoord)*float64(k)/float64(steps-1)
		for j := 0; j < steps; j++ {
			y := minCoord + (maxCoord-minCoord)*float64(j)/float64(steps-1)
			for i := 0; i < steps; i++ {
				x := minCoord + (maxCoord-minCoord)*float64(i)/float64(steps-1)
				points[idx(i, j, k)] = center.add(fieldVec3{X: x, Y: y, Z: z})
			}
		}
	}
	started := time.Now()
	// Source compatibility marker retained for the D3D11 wiring regression:
	// fieldTryGPUPotentialGrid(samples, steps, minCoord, maxCoord)
	values, gpu := fieldTryGPUPotentialGrid(fieldShiftChargeSamples(samples, center), steps, minCoord, maxCoord)
	backend := "CPU"
	if !gpu || len(values) != len(points) {
		var cancelled func() bool
		if scene.cancel != nil {
			cancelled = scene.cancel.Cancelled
		}
		values = fieldPotentialGridCPU(samples, points, cancelled)
	} else {
		backend = "D3D11"
	}
	if scene.Display.gravity() {
		fieldCorrectGravityPotentialGrid(scene, points, values)
	}
	if scene.cancelled() {
		return fieldPotentialGrid{}
	}
	conductor := make([]bool, len(points))
	conductorPotentials := fieldConductorPotentialValues(scene, samples)
	for i, point := range points {
		if i&1023 == 0 && scene.cancelled() {
			return fieldPotentialGrid{}
		}
		if object := fieldContainingConductor(scene, point, 0); object >= 0 {
			conductor[i] = true
			if v, ok := conductorPotentials[object]; ok {
				values[i] = v
			}
		}
	}
	return fieldPotentialGrid{
		Steps: steps, Center: center, MinCoord: minCoord, MaxCoord: maxCoord,
		Values: values, Conductor: conductor, Backend: backend, Elapsed: time.Since(started),
	}
}

func (g fieldPotentialGrid) sample(point fieldVec3) (float64, bool) {
	if !g.valid() {
		return 0, false
	}
	scale := float64(g.Steps-1) / (g.MaxCoord - g.MinCoord)
	local := point.sub(g.Center)
	fx := (local.X - g.MinCoord) * scale
	fy := (local.Y - g.MinCoord) * scale
	fz := (local.Z - g.MinCoord) * scale
	if fx < 0 || fy < 0 || fz < 0 || fx > float64(g.Steps-1) || fy > float64(g.Steps-1) || fz > float64(g.Steps-1) {
		return 0, false
	}
	i0, j0, k0 := int(math.Floor(fx)), int(math.Floor(fy)), int(math.Floor(fz))
	i1, j1, k1 := min(g.Steps-1, i0+1), min(g.Steps-1, j0+1), min(g.Steps-1, k0+1)
	tx, ty, tz := fx-float64(i0), fy-float64(j0), fz-float64(k0)
	lerp := func(a, b, t float64) float64 { return a + (b-a)*t }
	v000 := g.Values[g.index(i0, j0, k0)]
	v100 := g.Values[g.index(i1, j0, k0)]
	v010 := g.Values[g.index(i0, j1, k0)]
	v110 := g.Values[g.index(i1, j1, k0)]
	v001 := g.Values[g.index(i0, j0, k1)]
	v101 := g.Values[g.index(i1, j0, k1)]
	v011 := g.Values[g.index(i0, j1, k1)]
	v111 := g.Values[g.index(i1, j1, k1)]
	v00, v10 := lerp(v000, v100, tx), lerp(v010, v110, tx)
	v01, v11 := lerp(v001, v101, tx), lerp(v011, v111, tx)
	return lerp(lerp(v00, v10, ty), lerp(v01, v11, ty), tz), true
}

func (g fieldPotentialGrid) conductorAt(point fieldVec3) bool {
	if !g.valid() || len(g.Conductor) != len(g.Values) {
		return false
	}
	scale := float64(g.Steps-1) / (g.MaxCoord - g.MinCoord)
	local := point.sub(g.Center)
	i := int(math.Round((local.X - g.MinCoord) * scale))
	j := int(math.Round((local.Y - g.MinCoord) * scale))
	k := int(math.Round((local.Z - g.MinCoord) * scale))
	if i < 0 || j < 0 || k < 0 || i >= g.Steps || j >= g.Steps || k >= g.Steps {
		return false
	}
	return g.Conductor[g.index(i, j, k)]
}

type fieldPotentialBand struct {
	Sign float64
	// Low is the closed-surface floor in a one-sign scene.  In a mixed-sign
	// scene it is instead the soft logarithmic scale used to make 8% steps
	// cluster smoothly around the physical V=0 separating surface.
	Low          float64
	High         float64
	ZeroAnchored bool
}

func fieldPotentialBandLevel(b fieldPotentialBand, percent float64) float64 {
	percent = fieldClamp(percent, 0, 100)
	if b.Low <= 0 || b.High <= b.Low {
		return 0
	}
	if b.ZeroAnchored {
		// log1p/expm1 keeps the exact origin: 0% is V=0, while the
		// remaining percentages still span a very wide electrostatic range.
		magnitude := b.Low * math.Expm1(percent*.01*math.Log1p(b.High/b.Low))
		return b.Sign * magnitude
	}
	magnitude := math.Exp(math.Log(b.Low) + percent*.01*(math.Log(b.High)-math.Log(b.Low)))
	return b.Sign * magnitude
}

func fieldPotentialBandPercent(b fieldPotentialBand, value float64) (float64, bool) {
	if b.Low <= 0 || b.High <= b.Low || value*b.Sign < 0 {
		return 0, false
	}
	magnitude := math.Abs(value)
	if b.ZeroAnchored {
		if magnitude <= 1e-14 {
			return 0, true
		}
		p := math.Log1p(magnitude/b.Low) / math.Log1p(b.High/b.Low)
		return fieldClamp(p, 0, 1), true
	}
	if magnitude < b.Low*(1-1e-12) {
		return 0, false
	}
	if magnitude < b.Low {
		magnitude = b.Low
	}
	p := (math.Log(magnitude) - math.Log(b.Low)) / (math.Log(b.High) - math.Log(b.Low))
	return fieldClamp(p, 0, 1), true
}

func fieldPotentialBandsFromGrid(grid fieldPotentialGrid) []fieldPotentialBand {
	if !grid.valid() {
		return nil
	}
	type signStats struct {
		sign        float64
		magnitudes  []float64
		boundaryMax float64
	}
	stats := []signStats{{sign: -1}, {sign: 1}}
	for k := 0; k < grid.Steps; k++ {
		for j := 0; j < grid.Steps; j++ {
			for i := 0; i < grid.Steps; i++ {
				index := grid.index(i, j, k)
				if index < len(grid.Conductor) && grid.Conductor[index] {
					continue
				}
				v := grid.Values[index]
				if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) < 1e-8 {
					continue
				}
				si := 1
				if v < 0 {
					si = 0
				}
				m := math.Abs(v)
				stats[si].magnitudes = append(stats[si].magnitudes, m)
				if i == 0 || j == 0 || k == 0 || i == grid.Steps-1 || j == grid.Steps-1 || k == grid.Steps-1 {
					stats[si].boundaryMax = math.Max(stats[si].boundaryMax, m)
				}
			}
		}
	}
	mixedSigns := len(stats[0].magnitudes) >= 32 && len(stats[1].magnitudes) >= 32
	bands := make([]fieldPotentialBand, 0, 2)
	for _, st := range stats {
		if len(st.magnitudes) < 32 {
			continue
		}
		sort.Float64s(st.magnitudes)
		quantile := func(q float64) float64 {
			idx := int(math.Round(fieldClamp(q, 0, 1) * float64(len(st.magnitudes)-1)))
			return st.magnitudes[idx]
		}
		// Ignore the uppermost half percent so a grid point landing almost on a
		// softened point charge cannot stretch every other percentage level.
		high := quantile(.995)
		if mixedSigns {
			// Opposite signs guarantee a physical V=0 separator somewhere in the
			// sampled domain.  Anchor both sign bands at that exact zero instead
			// of discarding all low-magnitude values below a boundary threshold.
			soft := math.Max(1e-8, quantile(.08))
			if high <= soft*1.02 {
				continue
			}
			bands = append(bands, fieldPotentialBand{Sign: st.sign, Low: soft, High: high, ZeroAnchored: true})
			continue
		}
		// A one-sign field has V=0 only at infinity.  Keep the first displayed
		// membrane above every same-sign cube-boundary sample so it closes inside
		// the numerical volume.
		low := math.Max(st.boundaryMax*1.06, quantile(.08))
		if high <= low*1.08 {
			continue
		}
		bands = append(bands, fieldPotentialBand{Sign: st.sign, Low: low, High: high})
	}
	return bands
}

func fieldPotentialSurfaceLevels(bands []fieldPotentialBand) []float64 {
	// Keep the responsive 8%% spacing introduced in 1.2.21, but extend the
	// readable shell range from 40%% to 56%%.  A mixed-sign scene therefore has
	// fifteen membranes (-56..-8, 0, +8..+56); a one-sign scene has eight
	// membranes (0..56).  The dense 40..100%% volume remains a separate layer,
	// so the 48%% and 56%% shells intentionally sit inside that translucent
	// region and make its structure easier to read.
	levels := make([]float64, 0, len(bands)*8+1)
	zeroAdded := false
	for _, band := range bands {
		start := 0.0
		if band.ZeroAnchored {
			if !zeroAdded {
				levels = append(levels, 0)
				zeroAdded = true
			}
			start = 8
		}
		for percent := start; percent <= 56.0001; percent += 8 {
			level := fieldPotentialBandLevel(band, percent)
			if (level != 0 || !zeroAdded) && !math.IsNaN(level) && !math.IsInf(level, 0) {
				levels = append(levels, level)
			}
		}
	}
	sort.Float64s(levels)
	return levels
}

func fieldIsoSurfaceTrianglesFromGridQuality(scene fieldScene, samples []fieldChargeSample, grid fieldPotentialGrid, levels []float64, quality int) map[float64][]fieldTriangle3 {
	if !grid.valid() || len(levels) == 0 {
		return map[float64][]fieldTriangle3{}
	}
	steps := grid.Steps
	minCoord, maxCoord := grid.MinCoord, grid.MaxCoord
	idx := grid.index
	meshStarted := time.Now()
	out := make(map[float64][]fieldTriangle3)
	cubeEdges := [12][2]int{
		{0, 1}, {1, 2}, {2, 3}, {3, 0},
		{4, 5}, {5, 6}, {6, 7}, {7, 4},
		{0, 4}, {1, 5}, {2, 6}, {3, 7},
	}
	triangleCount := 0
	for k := 0; k < steps-1; k++ {
		if scene.cancelled() {
			return map[float64][]fieldTriangle3{}
		}
		for j := 0; j < steps-1; j++ {
			for i := 0; i < steps-1; i++ {
				ids := [8]int{
					idx(i, j, k), idx(i+1, j, k), idx(i+1, j+1, k), idx(i, j+1, k),
					idx(i, j, k+1), idx(i+1, j, k+1), idx(i+1, j+1, k+1), idx(i, j+1, k+1),
				}
				cubePts := [8]fieldVec3{
					grid.point(i, j, k), grid.point(i+1, j, k), grid.point(i+1, j+1, k), grid.point(i, j+1, k),
					grid.point(i, j, k+1), grid.point(i+1, j, k+1), grid.point(i+1, j+1, k+1), grid.point(i, j+1, k+1),
				}
				var cubeVals [8]float64
				minV, maxV := math.Inf(1), math.Inf(-1)
				for c, id := range ids {
					cubeVals[c] = grid.Values[id]
					minV = math.Min(minV, cubeVals[c])
					maxV = math.Max(maxV, cubeVals[c])
				}
				for _, level := range levels {
					isoLevel := level
					bias := 1e-7 * math.Max(1, math.Abs(level))
					if level > 0 {
						isoLevel += bias
					} else if level < 0 {
						isoLevel -= bias
					}
					if isoLevel < minV || isoLevel > maxV {
						continue
					}
					caseIndex := 0
					for c, v := range cubeVals {
						if v < isoLevel {
							caseIndex |= 1 << c
						}
					}
					if caseIndex == 0 || caseIndex == 255 {
						continue
					}
					var edgePoint [12]fieldVec3
					var edgeReady [12]bool
					pointOnEdge := func(edge int) fieldVec3 {
						if edgeReady[edge] {
							return edgePoint[edge]
						}
						a, b := cubeEdges[edge][0], cubeEdges[edge][1]
						edgePoint[edge] = fieldIsoInterpolate(cubePts[a], cubePts[b], cubeVals[a], cubeVals[b], isoLevel)
						edgeReady[edge] = true
						return edgePoint[edge]
					}
					row := fieldMarchingCubesTriTable[caseIndex]
					for t := 0; t+2 < len(row) && row[t] >= 0; t += 3 {
						a, b, c := pointOnEdge(int(row[t])), pointOnEdge(int(row[t+1])), pointOnEdge(int(row[t+2]))
						alpha := .065
						if math.Abs(level) < 1e-14 {
							alpha = .12
						}
						tri := fieldTriangle3{A: a, B: b, C: c, Level: level, Alpha: alpha}
						if tri.A.sub(tri.B).cross(tri.C.sub(tri.A)).len2() < 1e-10 {
							continue
						}
						centroid := tri.A.add(tri.B).add(tri.C).mul(1.0 / 3.0)
						if fieldContainingConductor(scene, centroid, .003) >= 0 {
							continue
						}
						if scene.Display.gravity() {
							insideMass := false
							for _, object := range scene.Objects {
								if object.gravity() && fieldPointInsideObject(centroid, object, .001) {
									insideMass = true
									break
								}
							}
							if insideMass {
								continue
							}
						}
						out[level] = append(out[level], tri)
						triangleCount++
					}
				}
			}
		}
	}
	gridStep := (maxCoord - minCoord) / float64(steps-1)
	smoothIterations := 0
	if quality > 0 {
		smoothIterations = 1
	}
	if quality > 1 {
		smoothIterations = 2
	}
	triangleCount = 0
	mixedSigns := fieldPotentialGridHasBothSigns(grid)
	for level, tris := range out {
		if scene.cancelled() {
			return map[float64][]fieldTriangle3{}
		}
		rawCount := len(tris)
		touchesBoundary := fieldIsoSurfaceTouchesGridBoundaryCentered(tris, grid.Center, minCoord, maxCoord, gridStep*.30)
		// A mixed-sign electrostatic field may have a perfectly physical open
		// separator (especially V=0) that reaches infinity.  Preserve its large
		// clipped component instead of applying the one-sign closed-shell rule.
		allowOpen := mixedSigns && touchesBoundary
		if touchesBoundary && !allowOpen {
			fieldRecordEquipotentialLevelStats(level, rawCount, 0, false)
			delete(out, level)
			continue
		}
		// Correct the linear marching-cubes edge intersections against the
		// actual potential before topology filtering.  A single safeguarded
		// Newton step is enough for the interactive preview; retry/final quality
		// use additional steps.  Extremely sample-heavy scenes keep the preview
		// to one step so accuracy improves without returning to multi-second UI
		// stalls.
		projectionIterations := 1
		if quality > 0 {
			projectionIterations = 2
		}
		if quality > 1 {
			projectionIterations = 3
		}
		if len(samples) > 512 {
			if quality == 0 {
				// Large boundary-element conductor models already spend most of
				// their preview budget evaluating the exact potential.  Their
				// interactive frame uses the finer scalar grid alone; retry/final
				// renders still receive exact vertex reprojection.
				projectionIterations = 0
			} else if projectionIterations > 1 {
				projectionIterations--
			}
		}
		if rawCount > 160000 && quality == 0 {
			projectionIterations = 0
		}
		refineStarted := time.Now()
		improved := fieldReprojectIsoSurfaceVertices(scene, samples, tris, level, gridStep, projectionIterations)
		improved = fieldImproveIsoSurface(scene, samples, improved, level, gridStep, smoothIterations)
		fieldRecordEquipotentialRefinement(level, rawCount, len(improved), projectionIterations, smoothIterations, time.Since(refineStarted))
		if allowOpen {
			improved = fieldFilterLargeOpenIsoComponents(improved, gridStep)
		} else {
			// Use the stable topology filter for both one-sign and mixed-sign
			// closed shells.  The strict filter is still attempted first inside
			// fieldFilterStableIsoComponents, but a tiny welding mismatch no longer
			// deletes every otherwise physical one-sign shell.  dotPaint(44).log
			// showed 4k..11k raw triangles per level and kept=0 for all eleven
			// levels; that forced an immediate quality retry on the UI thread and
			// made the editor appear frozen.
			improved = fieldFilterStableIsoComponents(improved, gridStep)
		}
		if len(improved) > 0 && quality > 0 && !allowOpen {
			// Open mixed-sign sheets can span the whole numerical cube.  Refining
			// every triangle on all 11 low-potential layers would multiply preview
			// cost without adding physical information; the doubled scalar grid
			// already controls their accuracy.
			improved = fieldAdaptiveRefineIsoSurface(scene, samples, improved, level, gridStep, 1)
			if mixedSigns {
				improved = fieldFilterStableIsoComponents(improved, gridStep*.25)
			} else {
				improved = fieldFilterClosedIsoComponents(improved, gridStep*.25)
			}
		}
		if len(improved) == 0 {
			fieldRecordEquipotentialLevelStats(level, rawCount, 0, allowOpen)
			delete(out, level)
			continue
		}
		alpha := .065
		if math.Abs(level) < 1e-14 {
			alpha = .12
		}
		for i := range improved {
			improved[i].Alpha = alpha
		}
		out[level] = improved
		fieldRecordEquipotentialLevelStats(level, rawCount, len(improved), allowOpen)
		triangleCount += len(improved)
	}
	fieldRecordEquipotentialTiming(grid.Backend, steps, len(samples), len(grid.Values), triangleCount, grid.Elapsed, time.Since(meshStarted))
	return out
}

func fieldIsoSurfaceTrianglesFromGrid(scene fieldScene, samples []fieldChargeSample, grid fieldPotentialGrid, levels []float64) map[float64][]fieldTriangle3 {
	return fieldIsoSurfaceTrianglesFromGridQuality(scene, samples, grid, levels, 1)
}

func fieldIsoSurfaceTriangles(scene fieldScene, samples []fieldChargeSample, steps int, levels []float64) map[float64][]fieldTriangle3 {
	grid := fieldBuildPotentialGrid(scene, samples, steps)
	return fieldIsoSurfaceTrianglesFromGridQuality(scene, samples, grid, levels, 1)
}

func fieldTriangleNormal(tri fieldTriangle3) fieldVec3 {
	return tri.B.sub(tri.A).cross(tri.C.sub(tri.A)).norm()
}

func fieldProjectTriangle(p fieldProjector, tri fieldTriangle3) (fieldProjectedTriangle, bool) {
	var out fieldProjectedTriangle
	out.Tri = tri
	pts := [3]fieldVec3{tri.A, tri.B, tri.C}
	for i, v := range pts {
		x, y, d, ok := p.projectFloat(v)
		if !ok || math.IsNaN(x) || math.IsNaN(y) {
			return fieldProjectedTriangle{}, false
		}
		out.X[i], out.Y[i], out.Depth[i] = x, y, d
		if i == 0 {
			out.MinX, out.MaxX = x, x
			out.MinY, out.MaxY = y, y
		} else {
			out.MinX = math.Min(out.MinX, x)
			out.MaxX = math.Max(out.MaxX, x)
			out.MinY = math.Min(out.MinY, y)
			out.MaxY = math.Max(out.MaxY, y)
		}
		out.AvgDep += d
	}
	out.AvgDep /= 3
	return out, true
}

func fieldProjectedTriangleDepthAt(t fieldProjectedTriangle, x, y float64) (float64, bool) {
	den := (t.Y[1]-t.Y[2])*(t.X[0]-t.X[2]) + (t.X[2]-t.X[1])*(t.Y[0]-t.Y[2])
	if math.Abs(den) < 1e-9 {
		return 0, false
	}
	w0 := ((t.Y[1]-t.Y[2])*(x-t.X[2]) + (t.X[2]-t.X[1])*(y-t.Y[2])) / den
	w1 := ((t.Y[2]-t.Y[0])*(x-t.X[2]) + (t.X[0]-t.X[2])*(y-t.Y[2])) / den
	w2 := 1 - w0 - w1
	if w0 < -1e-4 || w1 < -1e-4 || w2 < -1e-4 {
		return 0, false
	}
	return w0*t.Depth[0] + w1*t.Depth[1] + w2*t.Depth[2], true
}

func fieldProjectedTriangles(tris []fieldTriangle3, p fieldProjector) []fieldProjectedTriangle {
	out := make([]fieldProjectedTriangle, 0, len(tris))
	for _, tri := range tris {
		if pt, ok := fieldProjectTriangle(p, tri); ok {
			out = append(out, pt)
		}
	}
	return out
}

type fieldMembraneHit struct {
	Level float64
	Depth float64
}

type fieldMembraneCoverIndex struct {
	Triangles []fieldProjectedTriangle
	Bins      [][]int
	Cols      int
	Rows      int
	Width     float64
	Height    float64
}

func fieldBuildMembraneCoverIndex(tris []fieldProjectedTriangle, width, height int) fieldMembraneCoverIndex {
	idx := fieldMembraneCoverIndex{Triangles: tris, Width: float64(max(width, 1)), Height: float64(max(height, 1))}
	if len(tris) == 0 {
		return idx
	}
	idx.Cols = max(8, min(32, width/20))
	idx.Rows = max(8, min(32, height/20))
	idx.Bins = make([][]int, idx.Cols*idx.Rows)
	cellX := idx.Width / float64(idx.Cols)
	cellY := idx.Height / float64(idx.Rows)
	for ti := range tris {
		tri := &tris[ti]
		minCol := int(math.Floor(tri.MinX / cellX))
		maxCol := int(math.Floor(tri.MaxX / cellX))
		minRow := int(math.Floor(tri.MinY / cellY))
		maxRow := int(math.Floor(tri.MaxY / cellY))
		minCol = max(0, min(idx.Cols-1, minCol))
		maxCol = max(0, min(idx.Cols-1, maxCol))
		minRow = max(0, min(idx.Rows-1, minRow))
		maxRow = max(0, min(idx.Rows-1, maxRow))
		if maxCol < minCol || maxRow < minRow {
			continue
		}
		for row := minRow; row <= maxRow; row++ {
			for col := minCol; col <= maxCol; col++ {
				bi := row*idx.Cols + col
				idx.Bins[bi] = append(idx.Bins[bi], ti)
			}
		}
	}
	return idx
}

func fieldMembraneCoverCountCandidates(tris []fieldProjectedTriangle, candidates []int, x, y, depth float64) int {
	hits := make([]fieldMembraneHit, 0, 8)
	visit := func(ti int) {
		if ti < 0 || ti >= len(tris) {
			return
		}
		tri := &tris[ti]
		if x < tri.MinX || x > tri.MaxX || y < tri.MinY || y > tri.MaxY {
			return
		}
		d, ok := fieldProjectedTriangleDepthAt(*tri, x, y)
		if !ok || d <= depth+0.03 {
			return
		}
		hits = append(hits, fieldMembraneHit{Level: tri.Tri.Level, Depth: d})
	}
	if candidates == nil {
		for i := range tris {
			visit(i)
		}
	} else {
		for _, ti := range candidates {
			visit(ti)
		}
	}
	if len(hits) == 0 {
		return 0
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Level == hits[j].Level {
			return hits[i].Depth < hits[j].Depth
		}
		return hits[i].Level < hits[j].Level
	})
	count := 0
	lastLevel := math.Inf(-1)
	lastDepth := math.Inf(-1)
	for _, hit := range hits {
		if hit.Level != lastLevel || hit.Depth-lastDepth > .035 {
			count++
			lastLevel = hit.Level
			lastDepth = hit.Depth
		}
	}
	return count
}

func (idx fieldMembraneCoverIndex) Count(x, y, depth float64) int {
	if len(idx.Triangles) == 0 {
		return 0
	}
	if len(idx.Bins) == 0 || idx.Cols < 1 || idx.Rows < 1 {
		return fieldMembraneCoverCountCandidates(idx.Triangles, nil, x, y, depth)
	}
	col := int(math.Floor(x / (idx.Width / float64(idx.Cols))))
	row := int(math.Floor(y / (idx.Height / float64(idx.Rows))))
	if col < 0 || row < 0 || col >= idx.Cols || row >= idx.Rows {
		return 0
	}
	return fieldMembraneCoverCountCandidates(idx.Triangles, idx.Bins[row*idx.Cols+col], x, y, depth)
}

func fieldMembraneCoverCount(tris []fieldProjectedTriangle, x, y, depth float64) int {
	// Count physical membrane intersections, not rasterisation triangles.
	// Marching tetrahedra may place two coplanar triangles over the same
	// screen sample; both belong to one equipotential sheet and must
	// contribute one factor of 0.7, not two.  Distinct potential levels and
	// distinct depths of the same folded sheet remain separate layers.
	return fieldMembraneCoverCountCandidates(tris, nil, x, y, depth)
}

func fieldMembraneEdgeAlpha(coverCount int) float64 {
	if coverCount < 0 {
		coverCount = 0
	}
	return math.Pow(0.7, float64(coverCount))
}

type fieldMembranePointKey struct {
	X, Y, Z int64
}

type fieldMembraneEdgeID struct {
	Level int64
	A, B  fieldMembranePointKey
}

func fieldMembranePointQuantize(v fieldVec3) fieldMembranePointKey {
	return fieldMembranePointKey{
		X: int64(math.Round(v.X * 10000)),
		Y: int64(math.Round(v.Y * 10000)),
		Z: int64(math.Round(v.Z * 10000)),
	}
}

func fieldMembranePointLess(a, b fieldMembranePointKey) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.Z < b.Z
}

func fieldMembraneEdgeKey(a, b fieldVec3, level float64) fieldMembraneEdgeID {
	ka, kb := fieldMembranePointQuantize(a), fieldMembranePointQuantize(b)
	if fieldMembranePointLess(kb, ka) {
		ka, kb = kb, ka
	}
	return fieldMembraneEdgeID{Level: int64(math.Round(level * 1e8)), A: ka, B: kb}
}

func fieldIsoSurfaceSilhouetteEdges(tris []fieldTriangle3, p fieldProjector) []fieldSurfaceEdge {
	edges := make(map[fieldMembraneEdgeID]*fieldSurfaceEdge)
	for _, tri := range tris {
		n := fieldTriangleNormal(tri)
		for _, e := range [][2]fieldVec3{{tri.A, tri.B}, {tri.B, tri.C}, {tri.C, tri.A}} {
			key := fieldMembraneEdgeKey(e[0], e[1], tri.Level)
			if existing, ok := edges[key]; ok {
				existing.NormalB = n
				existing.HasB = true
			} else {
				edges[key] = &fieldSurfaceEdge{A: e[0], B: e[1], Level: tri.Level, NormalA: n, OpenBoundary: tri.Boundary}
			}
		}
	}
	cameraWorld := fieldVec3{}
	if p.Dimension == fieldDimension3D {
		cameraWorld = fieldInverseRotateXYZ(fieldVec3{Y: p.effectiveCamera()}, p.RotX, p.RotY, p.RotZ)
	}
	out := make([]fieldSurfaceEdge, 0, len(edges)/2)
	for _, e := range edges {
		mid := e.A.add(e.B).mul(.5)
		var view fieldVec3
		if p.Dimension == fieldDimension3D {
			view = cameraWorld.sub(mid).norm()
		} else {
			view = fieldInverseRotateXYZ(fieldVec3{Y: 1}, p.RotX, p.RotY, p.RotZ).norm()
		}
		d0 := e.NormalA.dot(view)
		// Closed vacuum isosurfaces contribute only true apparent-contour edges.
		// An ideal conducting sheet is an open physical surface, so its outer
		// boundary is also a real equipotential edge and must remain visible.
		silhouette := !e.HasB && e.OpenBoundary
		if e.HasB {
			d1 := e.NormalB.dot(view)
			silhouette = (d0 <= 0 && d1 >= 0) || (d0 >= 0 && d1 <= 0)
		}
		if silhouette {
			out = append(out, *e)
		}
	}
	return out
}

// fieldUseGPUMembraneFaces remains deliberately false in 1.2.20.
//
// The D3D11 scalar-potential grid remains enabled, because its readback is
// deterministic and was verified in real Windows logs.  The separate D3D11
// translucent-membrane raster, however, produced a valid-looking off-screen
// texture while the completed Windows preview showed only the 40..100% volume
// and no 0..40% sheets.  The platform-neutral CPU triangle blender is the same
// path used by the diagnostic renderer and preserves every membrane over the
// already-rendered volume.  Keep this as an explicit policy flag so a future
// GPU re-enable must be deliberate and covered by a real-window regression.
const fieldUseGPUMembraneFaces = false

func fieldTriangleFaceAlpha(tri fieldTriangle3) float64 {
	if tri.Alpha > 0 {
		return fieldClamp(tri.Alpha, 0, 1)
	}
	return .16
}

func fieldMembraneGPUVertices(projected []fieldProjectedTriangle, width, height int, dark bool) []fieldGPUFaceVertex {
	if width < 1 || height < 1 || len(projected) == 0 {
		return nil
	}
	out := make([]fieldGPUFaceVertex, 0, len(projected)*3)
	for i := range projected {
		tri := &projected[i]
		rr, gg, bb := fieldPotentialColor(tri.Tri.Level, dark)
		for v := 0; v < 3; v++ {
			out = append(out, fieldGPUFaceVertex{
				X: float32(2*tri.X[v]/float64(width) - 1),
				Y: float32(1 - 2*tri.Y[v]/float64(height)),
				Z: .5,
				R: float32(rr) / 255, G: float32(gg) / 255, B: float32(bb) / 255, A: float32(fieldTriangleFaceAlpha(tri.Tri)),
			})
		}
	}
	return out
}

func fieldApplyPremultipliedGPU(r *fieldRaster, pix []byte) bool {
	if r == nil || len(pix) != r.W*r.H*4 {
		return false
	}
	covered := 0
	for i := 0; i < r.W*r.H; i++ {
		o := i * 4
		sa := float64(pix[o+3]) / 255
		if sa <= 0 {
			continue
		}
		covered++
		da := float64(r.Pix[o+3]) / 255
		outA := sa + da*(1-sa)
		if outA <= 1e-12 {
			continue
		}
		// D3D11's transparent render target contains premultiplied RGB after
		// ordinary source-alpha blending.  Composite that over the raster's
		// existing straight-alpha volume instead of replacing it.  The old
		// assignment discarded the volume and made the final result depend on
		// driver/readback details.
		for c := 0; c < 3; c++ {
			srcPremul := float64(pix[o+c]) / 255
			dstPremul := float64(r.Pix[o+c]) / 255 * da
			outPremul := srcPremul + dstPremul*(1-sa)
			r.Pix[o+c] = byte(fieldClamp(outPremul/outA*255, 0, 255) + .5)
		}
		r.Pix[o+3] = byte(fieldClamp(outA*255, 0, 255) + .5)
		if len(r.Mask) == r.W*r.H {
			r.Mask[i] = true
		}
	}
	return covered > 0
}

func fieldEquipotentialGridSteps(scene fieldScene, quality int) int {
	// Keep interactive previews responsive while giving the membrane extractor
	// a modestly finer scalar lattice.  At density 14 the sequence is now
	// 52/60/70 samples per axis for preview/retry/final insertion, compared with
	// 50/56/64 in 1.2.33.  Accuracy grows without the eightfold cost of doubling
	// every axis.
	steps := 38 + scene.Density
	if quality > 0 {
		steps += 8
	}
	if quality > 1 {
		steps += 10
	}
	spanRatio := fieldSceneSpan(scene)/2.8 - 1
	if spanRatio > 0 {
		steps += min(18, int(math.Ceil(spanRatio*5)))
	}
	return max(34, min(88, steps))
}

func fieldEquipotentialContourSteps(scene fieldScene, quality int) int {
	// The old default 2-D lattice was 48x48.  Use at least 96x96 so contours
	// and 3-D membranes are based on comparable physical resolution.
	steps := 76 + scene.Density*2
	if quality > 0 {
		steps += 20
	}
	return max(80, min(160, steps))
}

func fieldProjectorScreenRay(p fieldProjector, x, y float64) (fieldVec3, fieldVec3) {
	scale := .44 * float64(min(p.W, p.H)) / p.effectiveSpan() * fieldObserverMagnification(p.Distance)
	if scale < 1e-9 {
		return fieldVec3{}, fieldVec3{}
	}
	cameraX := (x - float64(p.W)/2) / scale
	cameraZ := -(y - float64(p.H)/2) / scale
	if p.Dimension == fieldDimension3D {
		camera := p.effectiveCamera()
		originCamera := fieldVec3{Y: camera}
		targetCamera := fieldVec3{X: cameraX, Y: 0, Z: cameraZ}
		origin := fieldInverseRotateXYZ(originCamera, p.RotX, p.RotY, p.RotZ)
		direction := fieldInverseRotateXYZ(targetCamera.sub(originCamera), p.RotX, p.RotY, p.RotZ).norm()
		return origin, direction
	}
	originCamera := fieldVec3{X: cameraX, Y: p.effectiveCamera(), Z: cameraZ}
	origin := fieldInverseRotateXYZ(originCamera, p.RotX, p.RotY, p.RotZ)
	direction := fieldInverseRotateXYZ(fieldVec3{Y: -1}, p.RotX, p.RotY, p.RotZ).norm()
	return origin, direction
}

func fieldRayCubeInterval(origin, direction fieldVec3, minCoord, maxCoord float64) (float64, float64, bool) {
	tMin, tMax := 0.0, math.Inf(1)
	for _, axis := range [][2]float64{{origin.X, direction.X}, {origin.Y, direction.Y}, {origin.Z, direction.Z}} {
		o, d := axis[0], axis[1]
		if math.Abs(d) < 1e-12 {
			if o < minCoord || o > maxCoord {
				return 0, 0, false
			}
			continue
		}
		a, b := (minCoord-o)/d, (maxCoord-o)/d
		if a > b {
			a, b = b, a
		}
		tMin = math.Max(tMin, a)
		tMax = math.Min(tMax, b)
		if tMax <= tMin {
			return 0, 0, false
		}
	}
	return tMin, tMax, tMax > tMin
}

func fieldPotentialPercentForValue(bands []fieldPotentialBand, value float64) (float64, float64, bool) {
	for _, band := range bands {
		if percent, ok := fieldPotentialBandPercent(band, value); ok {
			return percent, band.Sign, true
		}
	}
	return 0, 0, false
}

func fieldPotentialVolumeDensity(percent float64) float64 {
	if percent <= .40 {
		return 0
	}
	return math.Pow(fieldClamp((percent-.40)/.60, 0, 1), 1.40)
}

// fieldRenderPotentialVolume ray-marches the already computed potential grid.
// Only the dense 40..100% part of each sign band contributes opacity; the
// lower 0..40% structure remains readable as separate 8%-spaced membranes.
func fieldRenderPotentialVolume(r *fieldRaster, scene fieldScene, projector fieldProjector, grid fieldPotentialGrid, bands []fieldPotentialBand, quality int, dark bool) {
	if r == nil || r.W < 1 || r.H < 1 || !grid.valid() || len(bands) == 0 {
		return
	}
	// Volume integration is deliberately performed on a smooth low-resolution
	// buffer and bilinearly expanded.  The scalar grid already carries the
	// physical resolution; tracing one ray for every 2x-AA subpixel would only
	// repeat nearly identical work and make the live editor unusable.
	maxW, maxH := 360, 270
	if quality > 0 {
		maxW, maxH = 440, 330
	}
	scale := math.Min(1, math.Min(float64(maxW)/float64(r.W), float64(maxH)/float64(r.H)))
	vw, vh := max(1, int(math.Ceil(float64(r.W)*scale))), max(1, int(math.Ceil(float64(r.H)*scale)))
	vol := fieldNewRaster(vw, vh)
	vp := projector
	vp.W, vp.H = vw, vh
	raySteps := 28
	if quality > 0 {
		raySteps = 36
	}
	if quality > 1 {
		raySteps = 46
	}
	workers := min(runtime.GOMAXPROCS(0), max(1, vh/20))
	var wg sync.WaitGroup
	rows := (vh + workers - 1) / workers
	for worker := 0; worker < workers; worker++ {
		y0, y1 := worker*rows, min(vh, (worker+1)*rows)
		if y0 >= y1 {
			continue
		}
		wg.Add(1)
		go func(y0, y1 int) {
			defer wg.Done()
			for y := y0; y < y1; y++ {
				if scene.cancelled() {
					return
				}
				for x := 0; x < vw; x++ {
					origin, direction := fieldProjectorScreenRay(vp, float64(x)+.5, float64(y)+.5)
					if direction.len2() < .5 {
						continue
					}
					t0, t1, ok := fieldRayCubeInterval(origin.sub(grid.Center), direction, grid.MinCoord, grid.MaxCoord)
					if !ok {
						continue
					}
					dt := (t1 - t0) / float64(raySteps)
					if dt <= 0 {
						continue
					}
					accA, accR, accG, accB := 0.0, 0.0, 0.0, 0.0
					for step := 0; step < raySteps && accA < .72; step++ {
						t := t0 + (float64(step)+.5)*dt
						point := origin.add(direction.mul(t))
						value, ok := grid.sample(point)
						if !ok {
							continue
						}
						percent, sign, ok := fieldPotentialPercentForValue(bands, value)
						if !ok || percent < .40 || grid.conductorAt(point) {
							continue
						}
						density := fieldPotentialVolumeDensity(percent)
						if density <= 1e-6 {
							continue
						}
						optical := density * 1.15 * dt / math.Max(1e-6, grid.MaxCoord-grid.MinCoord)
						a := 1 - math.Exp(-optical)
						rr, gg, bb := fieldPotentialColor(sign, dark)
						remain := 1 - accA
						accR += float64(rr) * a * remain
						accG += float64(gg) * a * remain
						accB += float64(bb) * a * remain
						accA += a * remain
					}
					if accA > 1e-4 {
						rr := byte(fieldClamp(accR/accA, 0, 255) + .5)
						gg := byte(fieldClamp(accG/accA, 0, 255) + .5)
						bb := byte(fieldClamp(accB/accA, 0, 255) + .5)
						vol.blend(x, y, rr, gg, bb, math.Min(.52, accA))
					}
				}
			}
		}(y0, y1)
	}
	wg.Wait()
	// Bilinear expansion keeps the high-potential cloud smooth without adding
	// artificial isosurface edges.
	for y := 0; y < r.H; y++ {
		if scene.cancelled() {
			return
		}
		fy := (float64(y)+.5)*float64(vh)/float64(r.H) - .5
		y0 := max(0, min(vh-1, int(math.Floor(fy))))
		y1 := max(0, min(vh-1, y0+1))
		ty := fieldClamp(fy-float64(y0), 0, 1)
		for x := 0; x < r.W; x++ {
			fx := (float64(x)+.5)*float64(vw)/float64(r.W) - .5
			x0 := max(0, min(vw-1, int(math.Floor(fx))))
			x1 := max(0, min(vw-1, x0+1))
			tx := fieldClamp(fx-float64(x0), 0, 1)
			weights := [4]float64{(1 - tx) * (1 - ty), tx * (1 - ty), (1 - tx) * ty, tx * ty}
			coords := [4][2]int{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}}
			var sumA, sumR, sumG, sumB float64
			for i, c := range coords {
				o := (c[1]*vw + c[0]) * 4
				a := float64(vol.Pix[o+3]) / 255
				w := weights[i]
				sumA += a * w
				sumR += float64(vol.Pix[o+2]) * a * w
				sumG += float64(vol.Pix[o+1]) * a * w
				sumB += float64(vol.Pix[o+0]) * a * w
			}
			if sumA <= 1e-5 {
				continue
			}
			rr := byte(fieldClamp(sumR/sumA, 0, 255) + .5)
			gg := byte(fieldClamp(sumG/sumA, 0, 255) + .5)
			bb := byte(fieldClamp(sumB/sumA, 0, 255) + .5)
			o := (y*r.W + x) * 4
			if r.Pix[o+3] == 0 {
				// The potential volume is the first layer in the equipotential
				// renderer.  Store its straight-alpha sample directly instead of
				// running the general four-channel compositor against transparent
				// black for millions of 6x subpixels.
				r.Pix[o+0], r.Pix[o+1], r.Pix[o+2] = bb, gg, rr
				r.Pix[o+3] = byte(fieldClamp(sumA, 0, 1)*255 + .5)
				if len(r.Mask) == r.W*r.H {
					r.Mask[y*r.W+x] = true
				}
			} else {
				r.blend(x, y, rr, gg, bb, sumA)
			}
		}
	}
}

func fieldRenderEquipotentialMembranes(r *fieldRaster, scene fieldScene, p fieldProjector, samples []fieldChargeSample, levels []float64, quality int, dark bool) {
	geometry, ok := fieldPotentialGeometryForScene(scene, samples, levels, quality)
	if !ok || scene.cancelled() {
		return
	}
	r.EquipotentialExpected = len(geometry.Levels) > 0
	r.EquipotentialRequestedLevels = len(geometry.Levels)
	r.EquipotentialRenderedLevels = geometry.RenderedLevels
	r.EquipotentialVacuumTriangles = geometry.VacuumTriangles
	r.EquipotentialConductorTriangles = geometry.ConductorTriangles
	// Render the dense high-potential region first. The separate 0..40%
	// membranes and their outlines remain visible on top of this volume.
	fieldRenderPotentialVolume(r, scene, p, geometry.Grid, geometry.Bands, quality, dark)
	if scene.cancelled() {
		return
	}
	all := make([]fieldTriangle3, 0, len(geometry.Vacuum)+len(geometry.Conductor))
	all = append(all, geometry.Vacuum...)
	all = append(all, geometry.Conductor...)
	projected := fieldProjectedTriangles(all, p)
	if scene.cancelled() {
		return
	}
	sort.Slice(projected, func(i, j int) bool { return projected[i].AvgDep < projected[j].AvgDep })
	faceGPU := false
	if fieldUseGPUMembraneFaces {
		if vertices := fieldMembraneGPUVertices(projected, r.W, r.H, dark); len(vertices) > 0 {
			if pix, ok := fieldTryGPUMembraneRaster(r.W, r.H, vertices); ok {
				faceGPU = fieldApplyPremultipliedGPU(r, pix)
			}
		}
	}
	if !faceGPU {
		// Stable reference path: blend every completed membrane directly over
		// the already-rendered high-potential volume.  This is intentionally the
		// same implementation used by non-Windows diagnostics.
		for i := range projected {
			if i&1023 == 0 && scene.cancelled() {
				return
			}
			tri := &projected[i]
			rr, gg, bb := fieldPotentialColor(tri.Tri.Level, dark)
			r.fillTriangleFloat(tri.X, tri.Y, rr, gg, bb, fieldTriangleFaceAlpha(tri.Tri))
		}
	}
	edges := fieldIsoSurfaceSilhouetteEdges(all, p)
	if scene.cancelled() {
		return
	}
	coverIndex := fieldBuildMembraneCoverIndex(projected, r.W, r.H)
	for i, edge := range edges {
		if i&1023 == 0 && scene.cancelled() {
			return
		}
		x0, y0, _, ok0 := p.project(edge.A)
		x1, y1, _, ok1 := p.project(edge.B)
		if !ok0 || !ok1 {
			continue
		}
		mx, my, md, okm := p.projectFloat(edge.A.add(edge.B).mul(.5))
		if !okm {
			continue
		}
		n := coverIndex.Count(mx, my, md)
		alpha := fieldMembraneEdgeAlpha(n)
		rr, gg, bb := fieldPotentialColor(edge.Level, dark)
		r.line(x0, y0, x1, y1, rr, gg, bb, alpha, 1)
	}
}

type fieldRaster struct {
	W, H        int
	Pix         []byte // BGRA, top-down
	Mask        []bool
	SampleScale int // 1 for ordinary pixels; >1 for supersample subpixels
	// Equipotential metadata lets the live editor distinguish a valid volume
	// cloud from a failed membrane extraction.  Previously triangles=0 still
	// had many translucent volume pixels and was committed as a "successful"
	// final frame, erasing the last good preview.
	EquipotentialExpected           bool
	EquipotentialRequestedLevels    int
	EquipotentialRenderedLevels     int
	EquipotentialVacuumTriangles    int
	EquipotentialConductorTriangles int
}

func fieldNewRaster(w, h int) fieldRaster {
	return fieldNewRasterAtScale(w, h, 1)
}

func fieldNewRasterAtScale(w, h, sampleScale int) fieldRaster {
	if w < 1 || h < 1 {
		return fieldRaster{}
	}
	if sampleScale < 1 {
		sampleScale = 1
	}
	pix := fieldAcquireRasterPixels(w*h*4, sampleScale)
	var mask []bool
	if sampleScale == 1 {
		mask = make([]bool, w*h)
	}
	return fieldRaster{W: w, H: h, Pix: pix, Mask: mask, SampleScale: sampleScale}
}

var fieldSupersamplePixelPools sync.Map // map[int]*sync.Pool, keyed by byte length

func fieldAcquireRasterPixels(bytes, sampleScale int) []byte {
	if bytes < 1 {
		return nil
	}
	if sampleScale <= 1 {
		return make([]byte, bytes)
	}
	value, _ := fieldSupersamplePixelPools.LoadOrStore(bytes, &sync.Pool{})
	pool := value.(*sync.Pool)
	if cached := pool.Get(); cached != nil {
		pix := cached.([]byte)
		if cap(pix) >= bytes {
			pix = pix[:bytes]
			clear(pix)
			return pix
		}
	}
	return make([]byte, bytes)
}

func fieldReleaseSupersampleRaster(r *fieldRaster) {
	if r == nil || r.SampleScale <= 1 || len(r.Pix) == 0 {
		return
	}
	bytes := len(r.Pix)
	value, _ := fieldSupersamplePixelPools.LoadOrStore(bytes, &sync.Pool{})
	value.(*sync.Pool).Put(r.Pix[:bytes])
	r.Pix = nil
	r.Mask = nil
}

func (r *fieldRaster) sampleScale() int {
	if r != nil && r.SampleScale > 1 {
		return r.SampleScale
	}
	return 1
}

func fieldRasterVisiblePixelCount(r fieldRaster) int {
	if r.W < 1 || r.H < 1 || len(r.Pix) != r.W*r.H*4 {
		return 0
	}
	n := 0
	for i := 3; i < len(r.Pix); i += 4 {
		if r.Pix[i] >= 8 {
			n++
		}
	}
	return n
}

func fieldOverlayPreviewGeometry(r *fieldRaster, scene fieldScene) bool {
	if r == nil || r.W < 1 || r.H < 1 || len(scene.Objects) == 0 {
		return false
	}
	p := fieldProjectorForScene(scene, r.W, r.H)
	for _, o := range scene.Objects {
		fieldDrawObject(r, p, o)
	}
	return true
}

func fieldEnsurePreviewGeometry(r *fieldRaster, scene fieldScene) bool {
	if r == nil || r.W < 1 || r.H < 1 || len(scene.Objects) == 0 {
		return false
	}
	// A few charge-marker pixels do not count as a usable final preview.  The
	// threshold scales mildly with the number of objects while remaining small
	// enough not to interfere with legitimate sparse field-line scenes.
	minimum := 32 + len(scene.Objects)*12
	if fieldRasterVisiblePixelCount(*r) >= minimum {
		return false
	}
	return fieldOverlayPreviewGeometry(r, scene)
}

func (r *fieldRaster) blend(x, y int, rr, gg, bb byte, alpha float64) {
	if r == nil || x < 0 || y < 0 || x >= r.W || y >= r.H || alpha <= 0 {
		return
	}
	if alpha > 1 {
		alpha = 1
	}
	i := y*r.W + x
	o := i * 4
	oldA := float64(r.Pix[o+3]) / 255
	newA := alpha + oldA*(1-alpha)
	if newA < 1e-9 {
		return
	}
	blendChan := func(old byte, val byte) byte {
		return byte(fieldClamp((float64(val)*alpha+float64(old)*oldA*(1-alpha))/newA, 0, 255) + .5)
	}
	r.Pix[o+0] = blendChan(r.Pix[o+0], bb)
	r.Pix[o+1] = blendChan(r.Pix[o+1], gg)
	r.Pix[o+2] = blendChan(r.Pix[o+2], rr)
	r.Pix[o+3] = byte(newA*255 + .5)
	if len(r.Mask) == r.W*r.H {
		r.Mask[i] = true
	}
}

func (r *fieldRaster) lineLegacy(x0, y0, x1, y1 int, rr, gg, bb byte, alpha float64, thickness int) {
	dx, dy := math.Abs(float64(x1-x0)), math.Abs(float64(y1-y0))
	steps := int(math.Max(dx, dy))
	if steps < 1 {
		steps = 1
	}
	if thickness < 1 {
		thickness = 1
	}
	rad := thickness / 2
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(x0) + float64(x1-x0)*t))
		y := int(math.Round(float64(y0) + float64(y1-y0)*t))
		for yy := y - rad; yy <= y+rad; yy++ {
			for xx := x - rad; xx <= x+rad; xx++ {
				if (xx-x)*(xx-x)+(yy-y)*(yy-y) <= rad*rad+1 {
					r.blend(xx, yy, rr, gg, bb, alpha)
				}
			}
		}
	}
}

// line draws one continuous round-capped stroke. Smooth-off deliberately keeps
// the established pixel renderer. A supersampled raster instead receives a
// geometrically continuous capsule whose width is scaled with the raster; this
// prevents short projected field/membrane segments from exposing one-subpixel
// joints before the final area resolve.
func (r *fieldRaster) line(x0, y0, x1, y1 int, rr, gg, bb byte, alpha float64, thickness int) {
	if r == nil {
		return
	}
	scale := r.sampleScale()
	if scale == 1 {
		r.lineLegacy(x0, y0, x1, y1, rr, gg, bb, alpha, thickness)
		return
	}
	r.lineDevice(x0, y0, x1, y1, rr, gg, bb, alpha, max(1, thickness)*scale)
}

// lineDevice is the supersample-space primitive used when a caller already
// has a width measured in high-resolution raster pixels.
func (r *fieldRaster) lineDevice(x0, y0, x1, y1 int, rr, gg, bb byte, alpha float64, thickness int) {
	if r == nil || thickness < 1 {
		return
	}
	ax, ay := float64(x0), float64(y0)
	bx, by := float64(x1), float64(y1)
	dx, dy := bx-ax, by-ay
	len2 := dx*dx + dy*dy
	radius := float64(thickness) * .5
	margin := radius + 1
	minX := max(0, int(math.Floor(math.Min(ax, bx)-margin)))
	maxX := min(r.W-1, int(math.Ceil(math.Max(ax, bx)+margin)))
	minY := max(0, int(math.Floor(math.Min(ay, by)-margin)))
	maxY := min(r.H-1, int(math.Ceil(math.Max(ay, by)+margin)))
	radius2 := radius * radius
	for y := minY; y <= maxY; y++ {
		py := float64(y)
		for x := minX; x <= maxX; x++ {
			px := float64(x)
			t := 0.0
			if len2 > 1e-12 {
				t = fieldClamp(((px-ax)*dx+(py-ay)*dy)/len2, 0, 1)
			}
			qx, qy := ax+t*dx, ay+t*dy
			ddx, ddy := px-qx, py-qy
			if ddx*ddx+ddy*ddy <= radius2 {
				r.blend(x, y, rr, gg, bb, alpha)
			}
		}
	}
}

func (r *fieldRaster) circle(cx, cy, radius int, rr, gg, bb byte, alpha float64, fill bool) {
	if radius < 1 {
		return
	}
	if fill {
		for y := -radius; y <= radius; y++ {
			xm := int(math.Sqrt(math.Max(0, float64(radius*radius-y*y))))
			for x := -xm; x <= xm; x++ {
				r.blend(cx+x, cy+y, rr, gg, bb, alpha)
			}
		}
		return
	}
	steps := max(24, radius*5)
	px, py := cx+radius, cy
	for i := 1; i <= steps; i++ {
		a := 2 * math.Pi * float64(i) / float64(steps)
		x, y := cx+int(math.Round(float64(radius)*math.Cos(a))), cy+int(math.Round(float64(radius)*math.Sin(a)))
		r.line(px, py, x, y, rr, gg, bb, alpha, 1)
		px, py = x, y
	}
}

func fieldEdge2D(ax, ay, bx, by, px, py float64) float64 {
	return (px-ax)*(by-ay) - (py-ay)*(bx-ax)
}

func fieldTopLeftEdge(ax, ay, bx, by float64) bool {
	dy, dx := by-ay, bx-ax
	return dy < 0 || (math.Abs(dy) < 1e-12 && dx > 0)
}

func (r *fieldRaster) fillTriangleFloat(xs, ys [3]float64, rr, gg, bb byte, alpha float64) {
	if r == nil {
		return
	}
	x0, x1, x2 := xs[0], xs[1], xs[2]
	y0, y1, y2 := ys[0], ys[1], ys[2]
	area := fieldEdge2D(x0, y0, x1, y1, x2, y2)
	if math.Abs(area) < 1e-10 {
		return
	}
	if area < 0 {
		x1, x2 = x2, x1
		y1, y2 = y2, y1
	}
	minX := max(0, int(math.Floor(math.Min(x0, math.Min(x1, x2))-.5)))
	maxX := min(r.W-1, int(math.Ceil(math.Max(x0, math.Max(x1, x2))-.5)))
	minY := max(0, int(math.Floor(math.Min(y0, math.Min(y1, y2))-.5)))
	maxY := min(r.H-1, int(math.Ceil(math.Max(y0, math.Max(y1, y2))-.5)))
	tl01 := fieldTopLeftEdge(x0, y0, x1, y1)
	tl12 := fieldTopLeftEdge(x1, y1, x2, y2)
	tl20 := fieldTopLeftEdge(x2, y2, x0, y0)
	inside := func(e float64, topLeft bool) bool {
		return e > 1e-9 || (math.Abs(e) <= 1e-9 && topLeft)
	}
	for y := minY; y <= maxY; y++ {
		py := float64(y) + .5
		for x := minX; x <= maxX; x++ {
			px := float64(x) + .5
			e01 := fieldEdge2D(x0, y0, x1, y1, px, py)
			e12 := fieldEdge2D(x1, y1, x2, y2, px, py)
			e20 := fieldEdge2D(x2, y2, x0, y0, px, py)
			if inside(e01, tl01) && inside(e12, tl12) && inside(e20, tl20) {
				r.blend(x, y, rr, gg, bb, alpha)
			}
		}
	}
}

func (r *fieldRaster) fillPolygon(points [][2]int, rr, gg, bb byte, alpha float64) {
	if len(points) < 3 {
		return
	}
	minY, maxY := points[0][1], points[0][1]
	for _, p := range points[1:] {
		minY, maxY = min(minY, p[1]), max(maxY, p[1])
	}
	for y := minY; y <= maxY; y++ {
		xs := make([]int, 0, len(points))
		for i := range points {
			a, b := points[i], points[(i+1)%len(points)]
			if (a[1] <= y && b[1] > y) || (b[1] <= y && a[1] > y) {
				x := a[0] + (y-a[1])*(b[0]-a[0])/(b[1]-a[1])
				xs = append(xs, x)
			}
		}
		sort.Ints(xs)
		for i := 0; i+1 < len(xs); i += 2 {
			for x := xs[i]; x <= xs[i+1]; x++ {
				r.blend(x, y, rr, gg, bb, alpha)
			}
		}
	}
}

func (r *fieldRaster) polygon(points [][2]int, rr, gg, bb byte, alpha float64, fill bool) {
	if len(points) < 2 {
		return
	}
	if fill && len(points) >= 3 {
		minY, maxY := points[0][1], points[0][1]
		for _, p := range points[1:] {
			minY, maxY = min(minY, p[1]), max(maxY, p[1])
		}
		for y := minY; y <= maxY; y++ {
			xs := make([]int, 0, len(points))
			for i := range points {
				a, b := points[i], points[(i+1)%len(points)]
				if (a[1] <= y && b[1] > y) || (b[1] <= y && a[1] > y) {
					x := a[0] + (y-a[1])*(b[0]-a[0])/(b[1]-a[1])
					xs = append(xs, x)
				}
			}
			sort.Ints(xs)
			for i := 0; i+1 < len(xs); i += 2 {
				for x := xs[i]; x <= xs[i+1]; x++ {
					r.blend(x, y, rr, gg, bb, alpha)
				}
			}
		}
	}
	for i := range points {
		a, b := points[i], points[(i+1)%len(points)]
		r.line(a[0], a[1], b[0], b[1], rr, gg, bb, alpha, 2)
	}
}

type fieldProjector struct {
	W, H      int
	Dimension fieldDimension
	RotX      float64
	RotY      float64
	RotZ      float64
	Span      float64
	Camera    float64
	Distance  float64
}

func fieldObserverDistance(distance float64) float64 {
	if distance == 0 {
		distance = 1
	}
	return fieldClamp(distance, fieldViewDistanceMin, fieldViewDistanceMax)
}

func fieldObserverMagnification(distance float64) float64 {
	return 1 / fieldObserverDistance(distance)
}

func fieldProjectorForScene(scene fieldScene, w, h int) fieldProjector {
	span := fieldSceneSpan(scene)
	distance := fieldObserverDistance(scene.ViewDistance)
	baseCamera := math.Max(6.2, span*2.45)
	// Moving the observer closer increases perspective, but never let the
	// near plane enter the scene.  The exact apparent scale remains 1/distance.
	camera := math.Max(baseCamera*distance, span*1.18+1.0)
	return fieldProjector{
		W: w, H: h, Dimension: scene.Dimension,
		RotX: scene.RotX, RotY: scene.RotY, RotZ: scene.RotZ,
		Span: span, Camera: camera, Distance: distance,
	}
}

func (p fieldProjector) effectiveSpan() float64 {
	if p.Span > 1e-9 {
		return p.Span
	}
	return 2.8
}

func (p fieldProjector) effectiveCamera() float64 {
	if p.Camera > 1e-9 {
		return p.Camera
	}
	return 6.2
}

func (p fieldProjector) projectFloat(v fieldVec3) (float64, float64, float64, bool) {
	// View rotation is independent of the 2-D/3-D source representation.  The
	// 2-D mode uses a freely rotatable orthographic view; the 3-D mode adds
	// perspective after applying the same XYZ camera orientation.
	q := fieldRotateXYZ(v, p.RotX, p.RotY, p.RotZ)
	depth := q.Y
	if p.Dimension == fieldDimension3D {
		camera := p.effectiveCamera()
		den := camera - q.Y
		if den < 1.0 {
			return 0, 0, depth, false
		}
		q.X *= camera / den
		q.Z *= camera / den
	}
	scale := .44 * float64(min(p.W, p.H)) / p.effectiveSpan() * fieldObserverMagnification(p.Distance)
	x := float64(p.W)/2 + q.X*scale
	y := float64(p.H)/2 - q.Z*scale
	return x, y, depth, x > -100 && y > -100 && x < float64(p.W+100) && y < float64(p.H+100)
}

func (p fieldProjector) project(v fieldVec3) (int, int, float64, bool) {
	x, y, depth, ok := p.projectFloat(v)
	return int(math.Round(x)), int(math.Round(y)), depth, ok
}

func fieldConductorOccludesPoint(scene fieldScene, projector fieldProjector, point fieldVec3) bool {
	var direction fieldVec3
	maxT := 24.0
	if projector.Dimension == fieldDimension3D {
		cameraWorld := fieldInverseRotateXYZ(fieldVec3{Y: projector.effectiveCamera()}, projector.RotX, projector.RotY, projector.RotZ)
		toCamera := cameraWorld.sub(point)
		maxT = toCamera.len() - .012
		if maxT <= .012 {
			return false
		}
		direction = toCamera.norm()
	} else {
		// Orthographic camera looks from +camera-Y toward the origin.
		direction = fieldInverseRotateXYZ(fieldVec3{Y: 1}, projector.RotX, projector.RotY, projector.RotZ).norm()
	}
	for _, o := range scene.Objects {
		if !o.electric() || o.Material != fieldMaterialConductor {
			continue
		}
		if fieldRayHitsConductor(o, point.add(direction.mul(.012)), direction, maxT) {
			return true
		}
	}
	return false
}

// fieldConductorCoversProjection reports whether the screen ray through point
// intersects a conductor at any depth.  Equipotential cross-section lines are
// deliberately suppressed over the conductor silhouette, even when another
// sampled slice lies in front of it, because otherwise several 3-D slices look
// as though they physically penetrate the same equipotential metal body.
func fieldConductorCoversProjection(scene fieldScene, projector fieldProjector, point fieldVec3) bool {
	var origin, direction fieldVec3
	maxT := 32.0
	if projector.Dimension == fieldDimension3D {
		origin = fieldInverseRotateXYZ(fieldVec3{Y: projector.effectiveCamera()}, projector.RotX, projector.RotY, projector.RotZ)
		direction = point.sub(origin).norm()
		if direction.len2() < .5 {
			return false
		}
	} else {
		toCamera := fieldInverseRotateXYZ(fieldVec3{Y: 1}, projector.RotX, projector.RotY, projector.RotZ).norm()
		origin = point.add(toCamera.mul(12))
		direction = toCamera.mul(-1)
		maxT = 24
	}
	for _, o := range scene.Objects {
		if !o.electric() || o.Material != fieldMaterialConductor {
			continue
		}
		if fieldRayHitsConductor(o, origin, direction, maxT) {
			return true
		}
	}
	return false
}

func fieldRefineVisibilityBoundary(scene fieldScene, projector fieldProjector, a, b fieldVec3, aVisible bool) fieldVec3 {
	lo, hi := 0.0, 1.0
	for i := 0; i < 22; i++ {
		mid := (lo + hi) * .5
		p := a.add(b.sub(a).mul(mid))
		visible := !fieldConductorCoversProjection(scene, projector, p)
		if visible == aVisible {
			lo = mid
		} else {
			hi = mid
		}
	}
	return a.add(b.sub(a).mul((lo + hi) * .5))
}

// fieldVisibleContourPieces performs hidden-line removal against opaque
// conductors.  A transparent UI fill must not make an equipotential contour
// appear to pass through the metal, so visibility is decided geometrically in
// camera space rather than by paint order or alpha blending.
func fieldVisibleContourPieces(scene fieldScene, projector fieldProjector, seg fieldSegment3) []fieldSegment3 {
	x0, y0, _, ok0 := projector.project(seg.A)
	x1, y1, _, ok1 := projector.project(seg.B)
	if !ok0 || !ok1 {
		return nil
	}
	steps := int(math.Ceil(math.Hypot(float64(x1-x0), float64(y1-y0)) / 2.0))
	steps = max(5, min(steps, 96))
	visible0 := !fieldConductorCoversProjection(scene, projector, seg.A)
	var start fieldVec3
	haveStart := visible0
	if haveStart {
		start = seg.A
	}
	prev, prevVisible := seg.A, visible0
	out := make([]fieldSegment3, 0, 2)
	for i := 1; i <= steps; i++ {
		cur := fieldSegmentPoint(seg, float64(i)/float64(steps))
		curVisible := !fieldConductorCoversProjection(scene, projector, cur)
		if curVisible != prevVisible {
			boundary := fieldRefineVisibilityBoundary(scene, projector, prev, cur, prevVisible)
			if prevVisible && haveStart && boundary.sub(start).len2() > 1e-10 {
				out = append(out, fieldSegment3{start, boundary})
			}
			if !prevVisible && curVisible {
				start, haveStart = boundary, true
			} else if !curVisible {
				haveStart = false
			}
		}
		prev, prevVisible = cur, curVisible
	}
	if prevVisible && haveStart && seg.B.sub(start).len2() > 1e-10 {
		out = append(out, fieldSegment3{start, seg.B})
	}
	return out
}

func fieldDrawArrow(r *fieldRaster, p fieldProjector, a, b fieldVec3, rr, gg, bb byte) {
	x0, y0, _, ok0 := p.project(a)
	x1, y1, _, ok1 := p.project(b)
	if !ok0 || !ok1 {
		return
	}
	dx, dy := float64(x1-x0), float64(y1-y0)
	l := math.Hypot(dx, dy)
	sampleScale := float64(r.sampleScale())
	if l < 3*sampleScale {
		return
	}
	dx, dy = dx/l, dy/l
	px, py := -dy, dx
	s := 6.0 * sampleScale
	leftX := int(math.Round(float64(x1) - dx*s + px*s*.55))
	leftY := int(math.Round(float64(y1) - dy*s + py*s*.55))
	rightX := int(math.Round(float64(x1) - dx*s - px*s*.55))
	rightY := int(math.Round(float64(y1) - dy*s - py*s*.55))
	r.line(x1, y1, leftX, leftY, rr, gg, bb, .95, 2)
	r.line(x1, y1, rightX, rightY, rr, gg, bb, .95, 2)
}

type fieldProjectedLineSegment struct {
	x0, y0 int
	x1, y1 int
	depth  float64
}

func field3DLineAlpha(depth, span float64) float64 {
	if !(span > 1e-9) {
		span = 1
	}
	// Camera looks from +Y. Map roughly the useful diagram depth range to a
	// stronger but still readable front/back contrast. This does not change the
	// physical trajectory; it only stops rear curves from visually dominating
	// nearer curves when many genuine 3-D streamlines overlap in projection.
	t := fieldClamp(.5+.5*depth/span, 0, 1)
	return .24 + .71*t
}

func fieldDrawLineSet(r *fieldRaster, p fieldProjector, lines []fieldLine, rr, gg, bb byte) {
	if p.Dimension != fieldDimension3D {
		for _, ln := range lines {
			for i := 1; i < len(ln.Points); i++ {
				x0, y0, _, ok0 := p.project(ln.Points[i-1])
				x1, y1, _, ok1 := p.project(ln.Points[i])
				if ok0 && ok1 {
					r.line(x0, y0, x1, y1, rr, gg, bb, .90, 1)
				}
			}
			if ln.Arrow > 0 && ln.Arrow < len(ln.Points) {
				fieldDrawArrow(r, p, ln.Points[ln.Arrow-1], ln.Points[ln.Arrow], rr, gg, bb)
			}
		}
		return
	}

	segments := make([]fieldProjectedLineSegment, 0)
	for _, ln := range lines {
		for i := 1; i < len(ln.Points); i++ {
			x0, y0, d0, ok0 := p.project(ln.Points[i-1])
			x1, y1, d1, ok1 := p.project(ln.Points[i])
			if ok0 && ok1 {
				segments = append(segments, fieldProjectedLineSegment{x0: x0, y0: y0, x1: x1, y1: y1, depth: (d0 + d1) * .5})
			}
		}
	}
	// Painter order for transparent line work: far geometry first, near geometry
	// last. Previously line slice order decided crossings, which made a rear
	// streamline look as if it jumped in front of a nearer one.
	sort.SliceStable(segments, func(i, j int) bool { return segments[i].depth < segments[j].depth })
	span := math.Max(1, p.effectiveSpan())
	for _, seg := range segments {
		alpha := field3DLineAlpha(seg.depth, span)
		r.line(seg.x0, seg.y0, seg.x1, seg.y1, rr, gg, bb, alpha, 1)
	}
	// Arrowheads remain crisp orientation markers. They are intentionally drawn
	// after the sorted strokes so line direction is readable at ordinary sizes.
	for _, ln := range lines {
		if ln.Arrow > 0 && ln.Arrow < len(ln.Points) {
			fieldDrawArrow(r, p, ln.Points[ln.Arrow-1], ln.Points[ln.Arrow], rr, gg, bb)
		}
	}
}

func fieldChargeColor(q float64) (byte, byte, byte) {
	if q > .03 {
		return 220, 60, 50
	}
	if q < -.03 {
		return 50, 95, 220
	}
	return 120, 120, 120
}

func fieldDrawObject(r *fieldRaster, p fieldProjector, o fieldObject) {
	center := fieldObjectCenter(o)
	cr, cg, cb := fieldChargeColor(o.Charge)
	if o.magnetic() {
		cr, cg, cb = 70, 145, 75
	} else if o.gravity() {
		cr, cg, cb = 155, 105, 45
	}
	fillA := .22
	if o.Material == fieldMaterialConductor {
		fillA = .36
	}
	scale := .44 * float64(min(p.W, p.H)) / p.effectiveSpan() * fieldObserverMagnification(p.Distance)
	sampleScale := r.sampleScale()
	switch o.Kind {
	case fieldObjectFerromagnet:
		cx, cy, _, ok := p.project(center)
		if !ok {
			return
		}
		radius := max(4*sampleScale, int(math.Round(o.Radius*scale)))
		r.circle(cx, cy, radius, 90, 105, 115, .46, true)
		r.circle(cx, cy, radius, 210, 220, 225, .96, false)
		// Permeability hatch: compact diagonal guides distinguish passive soft
		// iron from permanent magnets without suggesting fixed N/S poles.
		for off := -radius; off <= radius; off += max(5*sampleScale, radius/3) {
			x0 := cx + off - radius/2
			y0 := cy + radius
			x1 := cx + off + radius/2
			y1 := cy - radius
			r.line(x0, y0, x1, y1, 190, 200, 205, .48, 1)
		}
	case fieldObjectSphere, fieldObjectSolidMassSphere, fieldObjectHollowMassSphere:
		if o.gravity() {
			if fieldDrawGravityBlackHole(r, p, o, fieldGravityBlackHoleStrength(o)) {
				break
			}
		}
		cx, cy, _, ok := p.project(center)
		if !ok {
			return
		}
		radius := max(3*sampleScale, int(math.Round(o.Radius*scale)))
		if o.Kind != fieldObjectHollowMassSphere {
			r.circle(cx, cy, radius, cr, cg, cb, fillA, true)
		} else {
			r.circle(cx, cy, max(2*sampleScale, radius-3*sampleScale), cr, cg, cb, .20, false)
		}
		r.circle(cx, cy, radius, cr, cg, cb, .98, false)
		if p.Dimension == fieldDimension3D {
			// Three material great circles rotate with the sphere.  The
			// electrostatic field remains rotationally symmetric, while the
			// guides make the selected object's XYZ self-rotation visible.
			ex, ey, ez := fieldObjectBasis(o)
			for _, plane := range [][2]fieldVec3{{ex, ey}, {ex, ez}, {ey, ez}} {
				var prev fieldVec3
				for i := 0; i <= 40; i++ {
					a := 2 * math.Pi * float64(i) / 40
					v := center.add(plane[0].mul(o.Radius * math.Cos(a))).add(plane[1].mul(o.Radius * math.Sin(a)))
					if i > 0 {
						x0, y0, _, ok0 := p.project(prev)
						x1, y1, _, ok1 := p.project(v)
						if ok0 && ok1 {
							r.line(x0, y0, x1, y1, cr, cg, cb, .42, 1)
						}
					}
					prev = v
				}
			}
		}
	case fieldObjectRod, fieldObjectBarMagnet:
		ex, ey, ez := fieldObjectBasis(o)
		radius := math.Max(.10, math.Min(.22, o.Length*.11))
		ends := []fieldVec3{center.add(ex.mul(-o.Length * .5)), center.add(ex.mul(o.Length * .5))}
		for _, off := range []fieldVec3{ey.mul(radius), ey.mul(-radius), ez.mul(radius), ez.mul(-radius)} {
			a, b := ends[0].add(off), ends[1].add(off)
			x0, y0, _, ok0 := p.project(a)
			x1, y1, _, ok1 := p.project(b)
			if ok0 && ok1 {
				r.line(x0, y0, x1, y1, cr, cg, cb, .92, 2)
			}
		}
		for _, e := range ends {
			var pts [][2]int
			for i := 0; i < 20; i++ {
				a := 2 * math.Pi * float64(i) / 20
				v := e.add(ey.mul(radius * math.Cos(a))).add(ez.mul(radius * math.Sin(a)))
				x, y, _, ok := p.project(v)
				if ok {
					pts = append(pts, [2]int{x, y})
				}
			}
			if len(pts) > 2 {
				r.polygon(pts, cr, cg, cb, fillA, false)
			}
		}
	case fieldObjectPlane, fieldObjectMassPlane:
		ex, _, ez := fieldObjectBasis(o)
		corners := []fieldVec3{
			center.add(ex.mul(-o.Size * .5)).add(ez.mul(-o.Size * .5)),
			center.add(ex.mul(o.Size * .5)).add(ez.mul(-o.Size * .5)),
			center.add(ex.mul(o.Size * .5)).add(ez.mul(o.Size * .5)),
			center.add(ex.mul(-o.Size * .5)).add(ez.mul(o.Size * .5)),
		}
		pts := make([][2]int, 0, 4)
		for _, v := range corners {
			x, y, _, ok := p.project(v)
			if ok {
				pts = append(pts, [2]int{x, y})
			}
		}
		if len(pts) == 4 {
			r.polygon(pts, cr, cg, cb, fillA, true)
		}
	case fieldObjectCurrentWire:
		_, axis, _ := fieldObjectBasis(o)
		a := center.add(axis.mul(-1.15))
		b := center.add(axis.mul(1.15))
		x0, y0, _, ok0 := p.project(a)
		x1, y1, _, ok1 := p.project(b)
		if ok0 && ok1 {
			r.line(x0, y0, x1, y1, cr, cg, cb, .95, 3)
		}
	case fieldObjectHorseshoeMagnet:
		path := fieldHorseshoeCenterline(o, 12, 24)
		width := max(4*sampleScale, int(math.Round(fieldHorseshoeRadius(o)*scale*1.65)))
		for i := 1; i < len(path); i++ {
			x0, y0, _, ok0 := p.project(path[i-1])
			x1, y1, _, ok1 := p.project(path[i])
			if ok0 && ok1 {
				if sampleScale == 1 {
					r.line(x0, y0, x1, y1, cr, cg, cb, .32, width+3)
				} else {
					r.lineDevice(x0, y0, x1, y1, cr, cg, cb, .32, width+3*sampleScale)
				}
				r.line(x0, y0, x1, y1, cr, cg, cb, .96, 2)
			}
		}
		south, north, _, _ := fieldHorseshoePoleGeometry(o)
		_, ey, _ := fieldObjectBasis(o)
		ex, _, _ := fieldObjectBasis(o)
		capRadius := fieldHorseshoeRadius(o)
		for _, cap := range []struct {
			center  fieldVec3
			r, g, b byte
		}{{south, 55, 95, 220}, {north, 220, 65, 55}} {
			pts := make([][2]int, 0, 20)
			for i := 0; i < 20; i++ {
				a := 2 * math.Pi * float64(i) / 20
				v := cap.center.add(ex.mul(capRadius * math.Cos(a))).add(ey.mul(capRadius * math.Sin(a)))
				x, y, _, ok := p.project(v)
				if ok {
					pts = append(pts, [2]int{x, y})
				}
			}
			if len(pts) > 2 {
				r.polygon(pts, cap.r, cap.g, cap.b, .62, true)
			}
		}
	case fieldObjectCurrentLoop, fieldObjectMassRing:
		_, ey, ez := fieldObjectBasis(o)
		const loopDrawSegments = 72
		var first, prev fieldVec3
		for i := 0; i <= loopDrawSegments; i++ {
			a := 2 * math.Pi * float64(i) / loopDrawSegments
			v := center.add(ey.mul(o.Radius * math.Cos(a))).add(ez.mul(o.Radius * math.Sin(a)))
			if i == 0 {
				first, prev = v, v
				continue
			}
			x0, y0, _, ok0 := p.project(prev)
			x1, y1, _, ok1 := p.project(v)
			if ok0 && ok1 {
				r.line(x0, y0, x1, y1, cr, cg, cb, .96, 3)
			}
			prev = v
		}
		_ = first
		if o.Kind == fieldObjectCurrentLoop {
			a0, a1 := .08*2*math.Pi, .12*2*math.Pi
			if o.Charge < 0 {
				a0, a1 = a1, a0
			}
			v0 := center.add(ey.mul(o.Radius * math.Cos(a0))).add(ez.mul(o.Radius * math.Sin(a0)))
			v1 := center.add(ey.mul(o.Radius * math.Cos(a1))).add(ez.mul(o.Radius * math.Sin(a1)))
			fieldDrawArrow(r, p, v0, v1, cr, cg, cb)
		}
	}
	cx, cy, _, ok := p.project(center)
	if ok && o.Kind != fieldObjectFerromagnet && !o.gravity() {
		marker := 4 * sampleScale
		// Small central sign glyph.  It remains readable even on a rotated plate.
		if o.Charge > .03 {
			r.line(cx-marker, cy, cx+marker, cy, 255, 255, 255, .95, 1)
			r.line(cx, cy-marker, cx, cy+marker, 255, 255, 255, .95, 1)
		} else if o.Charge < -.03 {
			r.line(cx-marker, cy, cx+marker, cy, 255, 255, 255, .95, 1)
		}
	}
}

func fieldDrawObjectMarker(r *fieldRaster, p fieldProjector, o fieldObject) {
	center := fieldObjectCenter(o)
	cx, cy, _, ok := p.project(center)
	if !ok {
		return
	}
	marker := 4 * r.sampleScale()
	if o.Charge > .03 {
		r.line(cx-marker, cy, cx+marker, cy, 255, 255, 255, .95, 1)
		r.line(cx, cy-marker, cx, cy+marker, 255, 255, 255, .95, 1)
	} else if o.Charge < -.03 {
		r.line(cx-marker, cy, cx+marker, cy, 255, 255, 255, .95, 1)
	}
}

func fieldRender(scene fieldScene, width, height int, quality int) fieldRaster {
	return fieldRenderForBackground(scene, width, height, quality, false)
}

const fieldSmoothSampleScale = 6

func fieldRenderSampleScale(scene fieldScene) int {
	if scene.Antialias {
		return fieldSmoothSampleScale
	}
	return 1
}

// fieldPreviewSceneForInteraction separates responsive editor feedback from
// the final render policy.  The user's Antialias choice is preserved in the
// editor scene and in every insertion/final preview.  Only a transient scene
// snapshot used while a slider or placed object is moving is forced to 1x.
func fieldPreviewSceneForInteraction(scene fieldScene, interactive bool) fieldScene {
	if interactive {
		scene.Antialias = false
	}
	scene.precise3DLines = !interactive && fieldSceneNeedsPrecise3DLines(scene)
	return scene
}

func fieldSceneNeedsPrecise3DLines(scene fieldScene) bool {
	if scene.Dimension != fieldDimension3D {
		return false
	}
	switch scene.Display {
	case fieldDisplayElectricLines, fieldDisplayMagneticLines, fieldDisplayGravityLines:
		return true
	default:
		return false
	}
}

// fieldPreviewResultCurrentEnough allows an already-computed 1x interaction
// frame to be displayed while a newer 1x request is waiting.  Without this
// rule, a continuous stream of mouse events makes every worker result stale
// before it reaches the UI and the preview appears frozen.  Final 6x results
// remain strict: they are installed only when their serial is exactly current.
func fieldPreviewResultCurrentEnough(resultSerial, latest uint64, interactive, pending, pendingInteractive bool) bool {
	if resultSerial == latest {
		return true
	}
	return interactive && pending && pendingInteractive && resultSerial < latest
}

func fieldRenderForBackground(scene fieldScene, width, height int, quality int, dark bool) fieldRaster {
	fieldClampScene(&scene)
	if scene.cancelled() {
		return fieldRaster{}
	}
	sampleScale := fieldRenderSampleScale(scene)
	if sampleScale > 1 && width > 0 && height > 0 {
		// Render every field primitive, membrane and object at six times the
		// linear resolution, then resolve exact premultiplied coverage back to the
		// requested image. Six samples per axis remove the long stair steps seen
		// on shallow equipotential silhouettes while keeping CPU and D3D11 output
		// under one deterministic AA rule.
		hiScene := scene
		hiScene.Antialias = false
		hi := fieldRenderForBackgroundBaseAtScale(hiScene, width*sampleScale, height*sampleScale, quality, dark, sampleScale)
		if scene.cancelled() {
			fieldReleaseSupersampleRaster(&hi)
			return fieldRaster{}
		}
		resolved := fieldDownsampleRasterScale(hi, width, height, sampleScale)
		fieldReleaseSupersampleRaster(&hi)
		return resolved
	}
	return fieldRenderForBackgroundBase(scene, width, height, quality, dark)
}

func fieldDownsampleRaster(src fieldRaster, dstW, dstH int) fieldRaster {
	if dstW < 1 || dstH < 1 || src.W%dstW != 0 || src.H%dstH != 0 {
		return fieldNewRaster(dstW, dstH)
	}
	scale := src.W / dstW
	if scale < 2 || src.H/dstH != scale {
		return fieldNewRaster(dstW, dstH)
	}
	return fieldDownsampleRasterScale(src, dstW, dstH, scale)
}

func fieldDownsampleRasterScale(src fieldRaster, dstW, dstH, scale int) fieldRaster {
	dst := fieldNewRaster(dstW, dstH)
	dst.EquipotentialExpected = src.EquipotentialExpected
	dst.EquipotentialRequestedLevels = src.EquipotentialRequestedLevels
	dst.EquipotentialRenderedLevels = src.EquipotentialRenderedLevels
	dst.EquipotentialVacuumTriangles = src.EquipotentialVacuumTriangles
	dst.EquipotentialConductorTriangles = src.EquipotentialConductorTriangles
	if dst.W == 0 || scale < 2 || src.W != dstW*scale || src.H != dstH*scale || len(src.Pix) != src.W*src.H*4 {
		return dst
	}
	workers := min(runtime.GOMAXPROCS(0), dstH)
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		y0, y1 := dstH*worker/workers, dstH*(worker+1)/workers
		wg.Add(1)
		go func(y0, y1 int) {
			defer wg.Done()
			area := uint64(scale * scale)
			for y := y0; y < y1; y++ {
				for x := 0; x < dstW; x++ {
					var sumA, sumB, sumG, sumR uint64
					for oy := 0; oy < scale; oy++ {
						si := ((y*scale+oy)*src.W + x*scale) * 4
						for ox := 0; ox < scale; ox++ {
							a := uint64(src.Pix[si+3])
							sumA += a
							sumB += uint64(src.Pix[si+0]) * a
							sumG += uint64(src.Pix[si+1]) * a
							sumR += uint64(src.Pix[si+2]) * a
							si += 4
						}
					}
					if sumA == 0 {
						continue
					}
					di := (y*dstW + x) * 4
					dst.Pix[di+0] = byte((sumB + sumA/2) / sumA)
					dst.Pix[di+1] = byte((sumG + sumA/2) / sumA)
					dst.Pix[di+2] = byte((sumR + sumA/2) / sumA)
					dst.Pix[di+3] = byte((sumA + area/2) / area)
					dst.Mask[y*dstW+x] = true
				}
			}
		}(y0, y1)
	}
	wg.Wait()
	return dst
}

func fieldRenderForBackgroundBase(scene fieldScene, width, height int, quality int, dark bool) fieldRaster {
	return fieldRenderForBackgroundBaseAtScale(scene, width, height, quality, dark, 1)
}

func fieldRenderForBackgroundBaseAtScale(scene fieldScene, width, height int, quality int, dark bool, sampleScale int) fieldRaster {
	fieldClampScene(&scene)
	r := fieldNewRasterAtScale(width, height, sampleScale)
	if r.W == 0 {
		return r
	}
	p := fieldProjectorForScene(scene, width, height)
	if scene.Display.gravity() {
		samples := fieldGravityPotentialSamples(scene, quality)
		if scene.Display == fieldDisplayGravityVectors {
			fieldDrawVectorField(&r, p, scene, samples, false, true, dark)
		} else if scene.Display == fieldDisplayGravityLines {
			if dark {
				fieldDrawLineSet(&r, p, fieldSelectScreenDiverseLineSet("gravity", p, fieldGravityLines(scene)), 235, 190, 95)
			} else {
				fieldDrawLineSet(&r, p, fieldSelectScreenDiverseLineSet("gravity", p, fieldGravityLines(scene)), 150, 95, 25)
			}
		} else {
			steps := fieldEquipotentialContourSteps(scene, quality)
			if quality > 1 {
				steps += 10
			}
			baseSlice := fieldPlaneSlice{Origin: fieldVec3{}, U: fieldVec3{X: 1}, V: fieldVec3{Z: 1}}
			if scene.Dimension == fieldDimension3D {
				fieldRenderEquipotentialMembranes(&r, scene, p, samples, nil, quality, dark)
			} else {
				model := fieldBuildGravityModel(scene, quality)
				levels := fieldGravityPotentialLevels(scene, model, baseSlice, 30)
				contours := fieldGravityContourSlice(scene, model, baseSlice, steps, levels)
				for level, segs := range contours {
					rr, gg, bb := fieldPotentialColor(level, dark)
					for _, seg := range segs {
						for _, visible := range fieldVisibleContourPieces(scene, p, seg) {
							x0, y0, _, ok0 := p.project(visible.A)
							x1, y1, _, ok1 := p.project(visible.B)
							if ok0 && ok1 {
								r.line(x0, y0, x1, y1, rr, gg, bb, .76, 1)
							}
						}
					}
				}
			}
		}
	} else if scene.Display.magnetic() {
		if scene.Display == fieldDisplayMagneticVectors {
			fieldDrawVectorField(&r, p, scene, nil, true, false, dark)
		} else if dark {
			fieldDrawLineSet(&r, p, fieldSelectScreenDiverseLineSet("magnetic", p, fieldMagneticLines(scene)), 105, 215, 125)
		} else {
			fieldDrawLineSet(&r, p, fieldSelectScreenDiverseLineSet("magnetic", p, fieldMagneticLines(scene)), 45, 135, 70)
		}
	} else {
		samples := fieldElectricalSamples(scene, quality)
		if scene.Display == fieldDisplayEquipotential {
			samples = fieldEquipotentialElectricalSamples(scene, quality)
		}
		if scene.Display == fieldDisplayElectricVectors {
			fieldDrawVectorField(&r, p, scene, samples, false, false, dark)
		} else if scene.Display == fieldDisplayElectricLines {
			if dark {
				fieldDrawLineSet(&r, p, fieldSelectScreenDiverseLineSet("electric", p, fieldElectricLines(scene, samples)), 225, 225, 225)
			} else {
				fieldDrawLineSet(&r, p, fieldSelectScreenDiverseLineSet("electric", p, fieldElectricLines(scene, samples)), 35, 35, 35)
			}
		} else {
			steps := fieldEquipotentialContourSteps(scene, quality)
			if quality > 1 {
				steps += 10
			}
			baseSlice := fieldPlaneSlice{Origin: fieldVec3{}, U: fieldVec3{X: 1}, V: fieldVec3{Z: 1}}
			if scene.Dimension == fieldDimension3D {
				// Three-dimensional levels are derived from the complete scalar grid
				// inside fieldRenderEquipotentialMembranes.  No preferred probe point
				// or slice determines the 0..100% potential scale.
				fieldRenderEquipotentialMembranes(&r, scene, p, samples, nil, quality, dark)
			} else {
				levels := fieldPotentialLevels(scene, samples, baseSlice, 30)
				slices := []fieldPlaneSlice{{Origin: fieldVec3{}, U: fieldVec3{X: 1}, V: fieldVec3{Z: 1}}}
				for _, slice := range slices {
					contours := fieldContourSlice(scene, samples, slice, steps, levels)
					for level, segs := range contours {
						rr, gg, bb := fieldPotentialColor(level, dark)
						for _, seg := range segs {
							for _, visible := range fieldVisibleContourPieces(scene, p, seg) {
								x0, y0, _, ok0 := p.project(visible.A)
								x1, y1, _, ok1 := p.project(visible.B)
								if ok0 && ok1 {
									r.line(x0, y0, x1, y1, rr, gg, bb, .76, 1)
								}
							}
						}
					}
				}
			}
		}
	}
	// Draw sources last so calculation lines never obscure ordinary objects.
	// In equipotential mode a conductor surface has already been rendered as
	// its exact V=Vc membrane; repainting it as an opaque grey object would hide
	// that result, so only its charge marker is added here.
	for _, o := range scene.Objects {
		if scene.Display.scalarPotential() && o.electric() && o.Material == fieldMaterialConductor {
			fieldDrawObjectMarker(&r, p, o)
			continue
		}
		fieldDrawObject(&r, p, o)
	}
	return r
}

func fieldScreenToWorld2DSpanDistance(px, py float64, w, h int, span, distance float64) (float64, float64) {
	if w < 1 || h < 1 {
		return 0, 0
	}
	if span < 1e-9 {
		span = 2.8
	}
	s := .44 * float64(min(w, h)) / span * fieldObserverMagnification(distance)
	if s < 1e-9 {
		return 0, 0
	}
	x := (px - float64(w)/2) / s
	z := -(py - float64(h)/2) / s
	return fieldClamp(x, -2.65, 2.65), fieldClamp(z, -2.65, 2.65)
}

func fieldScreenToWorld2DSpan(px, py, w, h int, span float64) (float64, float64) {
	return fieldScreenToWorld2DSpanDistance(float64(px), float64(py), w, h, span, 1)
}

func fieldScreenToWorld2D(px, py, w, h int) (float64, float64) {
	return fieldScreenToWorld2DSpan(px, py, w, h, 2.8)
}

func fieldObjectScreenHit(scene fieldScene, px, py, w, h int) int {
	p := fieldProjectorForScene(scene, w, h)
	best, bestD := -1, 1e9
	for i := range scene.Objects {
		x, y, _, ok := p.project(fieldObjectCenter(scene.Objects[i]))
		if !ok {
			continue
		}
		d := math.Hypot(float64(px-x), float64(py-y))
		radius := 16.0
		o := scene.Objects[i]
		scale := .44 * float64(min(w, h)) / p.effectiveSpan() * fieldObserverMagnification(p.Distance)
		switch o.Kind {
		case fieldObjectSphere, fieldObjectFerromagnet, fieldObjectSolidMassSphere, fieldObjectHollowMassSphere:
			radius = math.Max(radius, o.Radius*scale)
		case fieldObjectRod, fieldObjectBarMagnet, fieldObjectHorseshoeMagnet:
			radius = math.Max(radius, o.Length*scale*.35)
		case fieldObjectPlane, fieldObjectMassPlane:
			radius = math.Max(radius, o.Size*scale*.42)
		case fieldObjectCurrentLoop, fieldObjectMassRing:
			radius = math.Max(radius, o.Radius*scale*1.05)
		}
		if d <= radius && d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// fieldScreenToWorldAtY converts a pointer in the *current preview* back to
// the constant-world-Y placement plane used by the selected object.  Both the
// initial estimate and the Newton refinement use the same observer distance,
// perspective and XYZ view rotation as fieldProjector.projectFloat.  This is
// crucial when the preview is enlarged: a screen drag at distance 0.1 must move
// one tenth as far in world space as the same pixel drag at distance 1.0.
func fieldScreenToWorldAtY(scene fieldScene, px, py float64, w, h int, targetY float64) (float64, float64) {
	proj := fieldProjectorForScene(scene, w, h)
	if scene.Dimension == fieldDimension2D && math.Abs(scene.RotX)+math.Abs(scene.RotY)+math.Abs(scene.RotZ) < 1e-12 {
		return fieldScreenToWorld2DSpanDistance(px, py, w, h, proj.effectiveSpan(), proj.Distance)
	}
	x, z := fieldScreenToWorld2DSpanDistance(px, py, w, h, proj.effectiveSpan(), proj.Distance)
	for iter := 0; iter < 16; iter++ {
		sx, sy, _, ok := proj.projectFloat(fieldVec3{X: x, Y: targetY, Z: z})
		if !ok {
			break
		}
		ex, ey := px-sx, py-sy
		if math.Hypot(ex, ey) < .08 {
			break
		}
		eps := .001
		sxX, syX, _, okX := proj.projectFloat(fieldVec3{X: x + eps, Y: targetY, Z: z})
		sxZ, syZ, _, okZ := proj.projectFloat(fieldVec3{X: x, Y: targetY, Z: z + eps})
		if !okX || !okZ {
			break
		}
		j00 := (sxX - sx) / eps
		j10 := (syX - sy) / eps
		j01 := (sxZ - sx) / eps
		j11 := (syZ - sy) / eps
		det := j00*j11 - j01*j10
		if math.Abs(det) < 1e-8 {
			break
		}
		dx := (ex*j11 - j01*ey) / det
		dz := (j00*ey - ex*j10) / det
		dx = fieldClamp(dx, -.55, .55)
		dz = fieldClamp(dz, -.55, .55)
		x = fieldClamp(x+dx, -2.65, 2.65)
		z = fieldClamp(z+dz, -2.65, 2.65)
	}
	return x, z
}

// fieldScreenToWorldPlanar retains the original Y=0 API used by palette drops
// and ordinary planar objects.
func fieldScreenToWorldPlanar(scene fieldScene, px, py, w, h int) (float64, float64) {
	return fieldScreenToWorldAtY(scene, float64(px), float64(py), w, h, 0)
}
