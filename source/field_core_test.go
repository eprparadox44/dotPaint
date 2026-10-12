package main

import (
	"math"
	"testing"
)

func TestFieldChargeRangeAndPlanarPlacement(t *testing.T) {
	s := fieldDefaultScene()
	s.Objects = []fieldObject{
		{Kind: fieldObjectSphere, X: 9, Y: 4, Z: -9, Charge: 8, Radius: .01},
		{Kind: fieldObjectRod, X: -9, Y: -2, Z: 9, Charge: -8, Length: 9},
		{Kind: fieldObjectPlane, Charge: .5, Size: 9},
	}
	fieldClampScene(&s)
	for i, o := range s.Objects {
		if o.Y != 0 {
			t.Fatalf("object %d left the planar layout: y=%g", i, o.Y)
		}
		if o.Charge < -4 || o.Charge > 4 {
			t.Fatalf("object %d charge escaped range: %g", i, o.Charge)
		}
	}
	if s.Objects[0].Radius < fieldSphereRadiusMin || s.Objects[1].Length > fieldRodLengthMax || s.Objects[2].Size > fieldPlaneSizeMax {
		t.Fatalf("shape dimensions were not clamped: %+v", s.Objects)
	}
}

func TestFieldDefaultAntialiasAndExtendedElectricSizeRange127(t *testing.T) {
	s := fieldDefaultScene()
	if !s.Antialias {
		t.Fatal("field antialiasing must be enabled by default")
	}
	rod := fieldDefaultObject(fieldObjectRod)
	rod.Length = 999
	plane := fieldDefaultObject(fieldObjectPlane)
	plane.Size = 999
	magnet := fieldDefaultObject(fieldObjectBarMagnet)
	magnet.Length = 999
	s.Objects = []fieldObject{rod, plane, magnet}
	fieldClampScene(&s)
	if math.Abs(s.Objects[0].Length-27.5) > 1e-12 {
		t.Fatalf("electric rod right endpoint=%g want 27.5", s.Objects[0].Length)
	}
	if math.Abs(s.Objects[1].Size-24.5) > 1e-12 {
		t.Fatalf("electric plate right endpoint=%g want 24.5", s.Objects[1].Size)
	}
	if math.Abs(s.Objects[2].Length-2.75) > 1e-12 {
		t.Fatalf("bar magnet range changed unexpectedly: %g", s.Objects[2].Length)
	}
}

func TestFieldTwoBySupersampleResolveProducesFractionalCoverage127(t *testing.T) {
	hi := fieldNewRaster(4, 4)
	// One covered subpixel in the upper-left destination pixel must resolve to
	// approximately 25% coverage, while retaining the straight source colour.
	hi.blend(0, 0, 220, 80, 40, 1)
	lo := fieldDownsampleRaster(hi, 2, 2)
	if lo.W != 2 || lo.H != 2 {
		t.Fatalf("unexpected resolve size %dx%d", lo.W, lo.H)
	}
	if a := lo.Pix[3]; a < 63 || a > 64 {
		t.Fatalf("one-of-four coverage alpha=%d want about 64", a)
	}
	if lo.Pix[2] != 220 || lo.Pix[1] != 80 || lo.Pix[0] != 40 {
		t.Fatalf("resolved straight colour changed: BGRA=%v", lo.Pix[:4])
	}
	if !lo.Mask[0] {
		t.Fatal("resolved covered pixel lost its mask")
	}
}

func TestFieldNeutralConductorInductionConservesCharge(t *testing.T) {
	s := fieldDefaultScene()
	positive := fieldDefaultObject(fieldObjectSphere)
	positive.Material = fieldMaterialInsulator
	positive.X = -1.0
	positive.Charge = 3
	neutral := fieldDefaultObject(fieldObjectSphere)
	neutral.Material = fieldMaterialConductor
	neutral.X = 1.0
	neutral.Charge = 0
	s.Objects = []fieldObject{positive, neutral}
	samples := fieldElectricalSamples(s, 1)
	total, minQ, maxQ, count := 0.0, math.Inf(1), math.Inf(-1), 0
	for _, p := range samples {
		if p.Object != 1 {
			continue
		}
		count++
		total += p.Q
		minQ = math.Min(minQ, p.Q)
		maxQ = math.Max(maxQ, p.Q)
	}
	if count < 12 {
		t.Fatalf("too few conductor panels: %d", count)
	}
	if math.Abs(total) > 1e-8 {
		t.Fatalf("neutral conductor charge was not conserved: %.12g", total)
	}
	if !(minQ < -1e-4 && maxQ > 1e-4) {
		t.Fatalf("neutral conductor did not polarize: min=%g max=%g", minQ, maxQ)
	}
}

func TestFieldChargedConductorConservesRequestedTotal(t *testing.T) {
	s := fieldDefaultScene()
	o := fieldDefaultObject(fieldObjectPlane)
	o.Material = fieldMaterialConductor
	o.Charge = -2.75
	s.Objects = []fieldObject{o}
	total := 0.0
	for _, p := range fieldElectricalSamples(s, 1) {
		total += p.Q
	}
	if math.Abs(total-o.Charge) > 1e-8 {
		t.Fatalf("conductor total mismatch: got=%g want=%g", total, o.Charge)
	}
}

func TestFieldInsulatorDoesNotRedistribute(t *testing.T) {
	s := fieldDefaultScene()
	a := fieldDefaultObject(fieldObjectSphere)
	a.Material = fieldMaterialInsulator
	a.Charge = 2
	b := fieldDefaultObject(fieldObjectSphere)
	b.Material = fieldMaterialInsulator
	b.X = 1.2
	b.Charge = -1
	s.Objects = []fieldObject{a, b}
	samples := fieldElectricalSamples(s, 1)
	var first *float64
	for _, p := range samples {
		if p.Object != 0 {
			continue
		}
		q := p.Q
		if first == nil {
			first = &q
		} else if math.Abs(q-*first) > 1e-12 {
			t.Fatalf("insulator charge redistributed: %g vs %g", q, *first)
		}
	}
}

func TestFieldObjectDimensionChangesSurface(t *testing.T) {
	small := fieldDefaultObject(fieldObjectSphere)
	large := small
	small.Radius = .25
	large.Radius = .9
	ss := fieldObjectSurface(small, 0, 1)
	ls := fieldObjectSurface(large, 0, 1)
	if len(ss) != len(ls) || len(ss) == 0 {
		t.Fatalf("unexpected sphere samples: %d %d", len(ss), len(ls))
	}
	if !(ls[0].Pos.len() > ss[0].Pos.len()*2) {
		t.Fatalf("radius did not scale surface: small=%g large=%g", ss[0].Pos.len(), ls[0].Pos.len())
	}
}

func TestFieldRender2DAnd3D(t *testing.T) {
	s := fieldDefaultScene()
	a := fieldDefaultObject(fieldObjectSphere)
	a.X, a.Charge = -.8, 2
	b := fieldDefaultObject(fieldObjectSphere)
	b.X, b.Charge = .8, -2
	b.Material = fieldMaterialConductor
	s.Objects = []fieldObject{a, b}
	for _, dim := range []fieldDimension{fieldDimension2D, fieldDimension3D} {
		s.Dimension = dim
		for _, mode := range []fieldDisplayMode{fieldDisplayEquipotential, fieldDisplayElectricLines} {
			s.Display = mode
			r := fieldRender(s, 260, 210, 0)
			if r.W != 260 || r.H != 210 || len(r.Pix) != 260*210*4 || len(r.Mask) != 260*210 {
				t.Fatalf("bad raster for dim=%d mode=%d: %+v", dim, mode, r)
			}
			on := 0
			for _, v := range r.Mask {
				if v {
					on++
				}
			}
			if on < 100 {
				t.Fatalf("render is unexpectedly empty for dim=%d mode=%d: %d pixels", dim, mode, on)
			}
		}
	}
}

func TestFieldScreenToWorldPlanarInverts3DProjection(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	p := fieldProjector{W: 640, H: 480, Dimension: s.Dimension, RotX: s.RotX, RotY: s.RotY, RotZ: s.RotZ}
	wantX, wantZ := 1.15, -.72
	sx, sy, _, ok := p.project(fieldVec3{X: wantX, Y: 0, Z: wantZ})
	if !ok {
		t.Fatal("test point did not project")
	}
	gotX, gotZ := fieldScreenToWorldPlanar(s, sx, sy, p.W, p.H)
	if math.Hypot(gotX-wantX, gotZ-wantZ) > .035 {
		t.Fatalf("3D planar inverse mismatch: got=(%.4f,%.4f) want=(%.4f,%.4f)", gotX, gotZ, wantX, wantZ)
	}
}

func TestFieldFluxLineCountIsProportionalToCharge(t *testing.T) {
	counts := make([]int, 0, 3)
	for _, q := range []float64{1, 2, 4} {
		s := fieldDefaultScene()
		s.Density = 9
		o := fieldDefaultObject(fieldObjectSphere)
		o.Charge = q
		s.Objects = []fieldObject{o}
		p, n, scale := fieldFluxLineCounts(fieldElectricalSamples(s, 1), s.Density)
		if n != 0 || scale != 1 {
			t.Fatalf("unexpected negative flux/cap for q=%g: p=%d n=%d scale=%g", q, p, n, scale)
		}
		counts = append(counts, p)
	}
	if counts[0] != 9 || counts[1] != 18 || counts[2] != 36 {
		t.Fatalf("line counts do not follow charge: %v", counts)
	}
}

func TestFieldNegativeChargeLinesEnterFromNumericalInfinity(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.Density = 8
	o := fieldDefaultObject(fieldObjectSphere)
	o.Charge = -2
	s.Objects = []fieldObject{o}
	samples := fieldElectricalSamples(s, 1)
	lines := fieldElectricLines(s, samples)
	if len(lines) != 16 {
		t.Fatalf("negative q=-2 should use 16 equal-flux lines, got %d", len(lines))
	}
	for i, ln := range lines {
		if len(ln.Points) < 2 {
			t.Fatalf("line %d too short", i)
		}
		if ln.Points[0].len() < fieldNumericalInfinityRadius*.75 {
			t.Fatalf("line %d did not start at infinity: r=%g", i, ln.Points[0].len())
		}
		dr := ln.Points[1].sub(ln.Points[0])
		e := fieldElectricFromSamples(samples, ln.Points[0])
		if dr.dot(e) <= 0 {
			t.Fatalf("line %d arrow path opposes E: dot=%g", i, dr.dot(e))
		}
	}
}

func TestFieldCoulombKernelObeysGaussLaw(t *testing.T) {
	s := fieldDefaultScene()
	o := fieldDefaultObject(fieldObjectSphere)
	o.Charge = 2.25
	o.Radius = .42
	s.Objects = []fieldObject{o}
	samples := fieldElectricalSamples(s, 2)
	const count = 2400
	const radius = 2.35
	dirs := fieldSeedDirections(count, true)
	flux := 0.0
	area := 4 * math.Pi * radius * radius / float64(len(dirs))
	for _, n := range dirs {
		p := n.mul(radius)
		flux += fieldElectricFromSamples(samples, p).dot(n) * area
	}
	// The code uses the Coulomb Green function without 1/(4*pi*epsilon0),
	// hence the normalised surface flux is 4*pi*Q.
	gotQ := flux / (4 * math.Pi)
	if math.Abs(gotQ-o.Charge) > .035 {
		t.Fatalf("Gauss flux mismatch: got Q=%g want=%g", gotQ, o.Charge)
	}
}

func TestFieldElectricIsNegativePotentialGradient(t *testing.T) {
	s := fieldDefaultScene()
	a := fieldDefaultObject(fieldObjectSphere)
	a.X, a.Z, a.Charge = -.75, .18, 1.6
	b := fieldDefaultObject(fieldObjectRod)
	b.X, b.Z, b.Charge = .92, -.31, -1.1
	b.RotX, b.RotY, b.RotZ = .37, .61, -.24
	s.Objects = []fieldObject{a, b}
	samples := fieldElectricalSamples(s, 1)
	p := fieldVec3{.22, .53, .71}
	h := 1e-4
	grad := fieldVec3{
		X: (fieldPotentialFromSamples(samples, p.add(fieldVec3{X: h})) - fieldPotentialFromSamples(samples, p.add(fieldVec3{X: -h}))) / (2 * h),
		Y: (fieldPotentialFromSamples(samples, p.add(fieldVec3{Y: h})) - fieldPotentialFromSamples(samples, p.add(fieldVec3{Y: -h}))) / (2 * h),
		Z: (fieldPotentialFromSamples(samples, p.add(fieldVec3{Z: h})) - fieldPotentialFromSamples(samples, p.add(fieldVec3{Z: -h}))) / (2 * h),
	}
	e := fieldElectricFromSamples(samples, p)
	if e.add(grad).len() > 2e-5*math.Max(1, e.len()) {
		t.Fatalf("E != -grad(V): E=%+v grad=%+v residual=%g", e, grad, e.add(grad).len())
	}
}

func TestFieldObjectKeepsPlanarCentreAndThreeAxisOrientation(t *testing.T) {
	s := fieldDefaultScene()
	o := fieldDefaultObject(fieldObjectPlane)
	o.X, o.Y, o.Z = .7, 4.2, -.9
	o.RotX, o.RotY, o.RotZ = .31, .77, 1.09
	s.Objects = []fieldObject{o}
	fieldClampScene(&s)
	got := s.Objects[0]
	if got.Y != 0 || got.X != .7 || got.Z != -.9 {
		t.Fatalf("centre escaped/changed placement plane: %+v", got)
	}
	ex, ey, ez := fieldObjectBasis(got)
	if math.Abs(ex.dot(ey)) > 1e-12 || math.Abs(ex.dot(ez)) > 1e-12 || math.Abs(ey.dot(ez)) > 1e-12 {
		t.Fatalf("XYZ rotation basis is not orthogonal: ex=%+v ey=%+v ez=%+v", ex, ey, ez)
	}
	if ex.sub(fieldVec3{X: 1}).len() < .1 || ey.sub(fieldVec3{Y: 1}).len() < .1 || ez.sub(fieldVec3{Z: 1}).len() < .1 {
		t.Fatalf("three-axis self rotation was not applied: ex=%+v ey=%+v ez=%+v", ex, ey, ez)
	}
}

func TestFieldConductorPanelsAreEquipotential(t *testing.T) {
	s := fieldDefaultScene()
	source := fieldDefaultObject(fieldObjectSphere)
	source.Material = fieldMaterialInsulator
	source.X, source.Charge = -1.15, 2.4
	conductor := fieldDefaultObject(fieldObjectSphere)
	conductor.Material = fieldMaterialConductor
	conductor.X, conductor.Charge = 1.05, 0
	s.Objects = []fieldObject{source, conductor}
	samples := fieldElectricalSamples(s, 1)
	lo, hi, count := math.Inf(1), math.Inf(-1), 0
	for _, panel := range samples {
		if panel.Object != 1 {
			continue
		}
		v := fieldPotentialFromSamples(samples, panel.Pos)
		lo, hi = math.Min(lo, v), math.Max(hi, v)
		count++
	}
	if count < 12 {
		t.Fatalf("too few conductor panels: %d", count)
	}
	if hi-lo > 2e-8*math.Max(1, math.Max(math.Abs(lo), math.Abs(hi))) {
		t.Fatalf("conductor is not equipotential: min=%g max=%g spread=%g", lo, hi, hi-lo)
	}
}

func TestFieldConductorInteriorIsOnePotentialAndZeroField(t *testing.T) {
	s := fieldDefaultScene()
	source := fieldDefaultObject(fieldObjectSphere)
	source.Material = fieldMaterialInsulator
	source.X, source.Z, source.Charge = 0, -1.15, 2.5
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.Material = fieldMaterialConductor
	plate.X, plate.Z, plate.Size, plate.Charge = 0, .75, 1.75, 0
	plate.RotX, plate.RotY, plate.RotZ = .18, .53, -.11
	s.Objects = []fieldObject{source, plate}
	samples := fieldElectricalSamples(s, 2)

	ex, ey, ez := fieldObjectBasis(plate)
	center := fieldObjectCenter(plate)
	points := []fieldVec3{
		center,
		center.add(ex.mul(.25)).add(ez.mul(-.31)),
		center.add(ex.mul(-.42)).add(ez.mul(.18)).add(ey.mul(.02)),
	}
	want := fieldPotentialAt(s, samples, points[0])
	for i, p := range points {
		if !fieldPointInsideObject(p, plate, 0) {
			t.Fatalf("test point %d is not inside conductor: %+v", i, p)
		}
		got := fieldPotentialAt(s, samples, p)
		if math.Abs(got-want) > 1e-12*math.Max(1, math.Abs(want)) {
			t.Fatalf("conductor interior is not equipotential at %d: got=%g want=%g", i, got, want)
		}
		if e := fieldElectricAt(s, samples, p); e.len2() != 0 {
			t.Fatalf("conductor interior field is not zero at %d: %+v", i, e)
		}
	}
}

func TestFieldEquipotentialContoursNeverEnterConductor(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	source := fieldDefaultObject(fieldObjectSphere)
	source.Material = fieldMaterialInsulator
	source.X, source.Z, source.Charge, source.Radius = 0, -1.2, 3, .34
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.Material = fieldMaterialConductor
	plate.X, plate.Z, plate.Size, plate.Charge = 0, .55, 1.65, 0
	// A nearly edge-on orientation similar to the reported screenshot.
	plate.RotX, plate.RotY, plate.RotZ = .08, 1.43, .03
	s.Objects = []fieldObject{source, plate}
	samples := fieldElectricalSamples(s, 2)
	slice := fieldPlaneSlice{Origin: fieldVec3{}, U: fieldVec3{X: 1}, V: fieldVec3{Z: 1}}
	levels := fieldPotentialLevels(s, samples, slice, 44)
	contours := fieldContourSlice(s, samples, slice, 76, levels)
	segmentCount := 0
	for level, segs := range contours {
		for _, seg := range segs {
			segmentCount++
			for _, f := range []float64{.1, .25, .5, .75, .9} {
				p := fieldSegmentPoint(seg, f)
				if fieldContainingConductor(s, p, 0) >= 0 {
					t.Fatalf("level %g contour entered conductor at t=%g p=%+v seg=%+v", level, f, p, seg)
				}
			}
		}
	}
	if segmentCount < 4 {
		t.Fatalf("contour test scene produced too few segments: %d", segmentCount)
	}
}

func TestFieldViewRotationIsActiveIn2DAndPlanarDragStillInverts(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension2D
	s.RotX, s.RotY, s.RotZ = .37, -.62, .28
	p := fieldProjector{W: 640, H: 480, Dimension: s.Dimension, RotX: s.RotX, RotY: s.RotY, RotZ: s.RotZ}
	plain := fieldProjector{W: 640, H: 480, Dimension: s.Dimension}
	point := fieldVec3{X: 1.12, Z: -.73}
	sx, sy, _, ok := p.project(point)
	px, py, _, okPlain := plain.project(point)
	if !ok || !okPlain {
		t.Fatal("test point did not project")
	}
	if math.Hypot(float64(sx-px), float64(sy-py)) < 8 {
		t.Fatalf("2D viewpoint rotation had no visible effect: rotated=(%d,%d) plain=(%d,%d)", sx, sy, px, py)
	}
	gotX, gotZ := fieldScreenToWorldPlanar(s, sx, sy, p.W, p.H)
	if math.Hypot(gotX-point.X, gotZ-point.Z) > .04 {
		t.Fatalf("rotated 2D planar inverse mismatch: got=(%.4f,%.4f) want=(%.4f,%.4f)", gotX, gotZ, point.X, point.Z)
	}
}

func TestFieldConductorSilhouetteMasksContoursAtEverySliceDepth(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.RotX, s.RotY, s.RotZ = 0, 0, 0
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.RotZ = 0 // this test deliberately needs a face-on XZ plate
	plate.Material = fieldMaterialConductor
	plate.Size = 1.6
	s.Objects = []fieldObject{plate}
	p := fieldProjector{W: 640, H: 480, Dimension: s.Dimension}
	for _, point := range []fieldVec3{{X: .2, Y: -1, Z: .1}, {X: .2, Y: 1, Z: .1}} {
		if !fieldConductorCoversProjection(s, p, point) {
			t.Fatalf("conductor silhouette did not cover projected contour point %+v", point)
		}
	}
	if fieldConductorCoversProjection(s, p, fieldVec3{X: 1.4, Y: 0, Z: 1.4}) {
		t.Fatal("conductor silhouette masked a point outside its projection")
	}
}

func TestFieldSphereUniformSeedsStayOnCommonRadius(t *testing.T) {
	o := fieldDefaultObject(fieldObjectSphere)
	o.Radius = .63
	seeds := fieldSphereUniformSeeds(o, 18, true)
	if len(seeds) != 18 {
		t.Fatalf("seed count mismatch: %d", len(seeds))
	}
	want := o.Radius + math.Max(.045, o.Radius*.06)
	for i, seed := range seeds {
		got := seed.sub(fieldObjectCenter(o)).len()
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("seed %d radius mismatch: got=%g want=%g", i, got, want)
		}
	}
}

func TestFieldRender3DEquipotentialMembranesProducesFilledSurface(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.Display = fieldDisplayEquipotential
	a := fieldDefaultObject(fieldObjectSphere)
	a.X, a.Charge = -.8, 2
	b := fieldDefaultObject(fieldObjectSphere)
	b.X, b.Charge = .9, -2
	s.Objects = []fieldObject{a, b}
	r := fieldRender(s, 260, 210, 1)
	alphaPixels := 0
	for i := 0; i < len(r.Pix); i += 4 {
		if r.Pix[i+3] > 0 {
			alphaPixels++
		}
	}
	if alphaPixels < 500 {
		t.Fatalf("3D equipotential membranes rendered too little content: %d pixels", alphaPixels)
	}
}

func TestFieldInsulatingSphereUsesOneCentralPointCharge(t *testing.T) {
	s := fieldDefaultScene()
	o := fieldDefaultObject(fieldObjectSphere)
	o.Material = fieldMaterialInsulator
	o.X, o.Z = .7, -.4
	o.Radius = .82
	o.Charge = 2.75
	s.Objects = []fieldObject{o}
	samples := fieldElectricalSamples(s, 2)
	if len(samples) != 1 {
		t.Fatalf("insulating sphere was not reduced to one point source: %d samples", len(samples))
	}
	center := fieldObjectCenter(o)
	if samples[0].Pos.sub(center).len() > 1e-12 {
		t.Fatalf("point source is not at the sphere centre: got=%+v want=%+v", samples[0].Pos, center)
	}
	if math.Abs(samples[0].Q-o.Charge) > 1e-12 {
		t.Fatalf("point source charge mismatch: got=%g want=%g", samples[0].Q, o.Charge)
	}
	for _, d := range []fieldVec3{{X: 1}, {Y: 1}, {Z: 1}, {X: -1}} {
		p := center.add(d.mul(1.7))
		e := fieldElectricFromSamples(samples, p)
		if e.norm().dot(d) < .999999 {
			t.Fatalf("point-charge field is not radial for direction %+v: E=%+v", d, e)
		}
	}
}

func TestFieldConductingSphereKeepsSurfacePanelsForInduction(t *testing.T) {
	s := fieldDefaultScene()
	o := fieldDefaultObject(fieldObjectSphere)
	o.Material = fieldMaterialConductor
	o.Charge = 1
	s.Objects = []fieldObject{o}
	samples := fieldElectricalSamples(s, 1)
	if len(samples) < 12 {
		t.Fatalf("conducting sphere lost its induction surface panels: %d", len(samples))
	}
	for _, sample := range samples {
		if !sample.Conductor {
			t.Fatal("conducting sphere sample was not marked as conductor")
		}
	}
}

func TestFieldMembraneCoverCountDeduplicatesTriangulationLayers(t *testing.T) {
	makeTri := func(level, depth float64) fieldProjectedTriangle {
		return fieldProjectedTriangle{
			Tri:   fieldTriangle3{Level: level},
			X:     [3]float64{0, 2, 0},
			Y:     [3]float64{0, 0, 2},
			Depth: [3]float64{depth, depth, depth},
			MinX:  0, MaxX: 2, MinY: 0, MaxY: 2,
		}
	}
	tris := []fieldProjectedTriangle{
		makeTri(1, 1.00), // one membrane layer
		makeTri(1, 1.01), // duplicate triangle on the same layer
		makeTri(1, 1.30), // another depth of the folded same-level membrane
		makeTri(2, 1.01), // a distinct equipotential membrane
	}
	if got := fieldMembraneCoverCount(tris, .4, .4, 0); got != 3 {
		t.Fatalf("unique membrane layer count mismatch: got=%d want=3", got)
	}
}

func TestFieldMembraneEdgeAlphaIsExactlyPointSevenPowerN(t *testing.T) {
	for n, want := range []float64{1, .7, .49, .343} {
		if got := fieldMembraneEdgeAlpha(n); math.Abs(got-want) > 1e-12 {
			t.Fatalf("membrane alpha n=%d: got=%g want=%g", n, got, want)
		}
	}
}

func TestFieldMembraneCoverSpatialIndexMatchesBruteForce125(t *testing.T) {
	tris := []fieldProjectedTriangle{
		{Tri: fieldTriangle3{Level: 1}, X: [3]float64{0, 80, 0}, Y: [3]float64{0, 0, 80}, Depth: [3]float64{1, 1, 1}, MinX: 0, MaxX: 80, MinY: 0, MaxY: 80},
		{Tri: fieldTriangle3{Level: 1}, X: [3]float64{0, 80, 80}, Y: [3]float64{80, 0, 80}, Depth: [3]float64{1.01, 1.01, 1.01}, MinX: 0, MaxX: 80, MinY: 0, MaxY: 80},
		{Tri: fieldTriangle3{Level: 2}, X: [3]float64{20, 90, 20}, Y: [3]float64{20, 20, 90}, Depth: [3]float64{2, 2, 2}, MinX: 20, MaxX: 90, MinY: 20, MaxY: 90},
	}
	index := fieldBuildMembraneCoverIndex(tris, 100, 100)
	for y := 5.0; y < 95; y += 11 {
		for x := 5.0; x < 95; x += 13 {
			got := index.Count(x, y, 0)
			want := fieldMembraneCoverCount(tris, x, y, 0)
			if got != want {
				t.Fatalf("spatial cover mismatch at %.1f,%.1f: got %d want %d", x, y, got, want)
			}
		}
	}
}

func TestFieldApplyPremultipliedGPU125(t *testing.T) {
	r := fieldNewRaster(1, 1)
	// Premultiplied BGRA for straight colour B=64,G=32,R=16 at alpha=64/255.
	pix := []byte{16, 8, 4, 64}
	if !fieldApplyPremultipliedGPU(&r, pix) {
		t.Fatal("GPU membrane pixel was not accepted")
	}
	if r.Pix[3] != 64 || r.Pix[0] < 62 || r.Pix[0] > 66 || r.Pix[1] < 30 || r.Pix[1] > 34 || r.Pix[2] < 14 || r.Pix[2] > 18 {
		t.Fatalf("unexpected unpremultiplied pixel: %v", r.Pix)
	}
}

func TestFieldMembraneGPUVerticesUseTransparentMembraneAlpha125(t *testing.T) {
	tri := fieldProjectedTriangle{Tri: fieldTriangle3{Level: 1}, X: [3]float64{0, 100, 0}, Y: [3]float64{0, 0, 100}}
	v := fieldMembraneGPUVertices([]fieldProjectedTriangle{tri}, 100, 100, false)
	if len(v) != 3 {
		t.Fatalf("expected 3 vertices, got %d", len(v))
	}
	if math.Abs(float64(v[0].X+1)) > 1e-6 || math.Abs(float64(v[0].Y-1)) > 1e-6 {
		t.Fatalf("unexpected NDC projection: %+v", v[0])
	}
	if math.Abs(float64(v[0].A)-0.16) > 1e-6 {
		t.Fatalf("unexpected membrane alpha: %f", v[0].A)
	}
}

func TestFieldMarchingCubesPointChargeSurfaceIsWatertight126(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	o := fieldDefaultObject(fieldObjectSphere)
	o.Material = fieldMaterialInsulator
	o.Charge = 1
	s.Objects = []fieldObject{o}
	samples := fieldElectricalSamples(s, 1)
	tris := fieldIsoSurfaceTriangles(s, samples, 22, []float64{.7})[.7]
	if len(tris) < 100 {
		t.Fatalf("point-charge isosurface has too few triangles: %d", len(tris))
	}
	edges := make(map[fieldMembraneEdgeID]int, len(tris)*2)
	for _, tri := range tris {
		for _, edge := range [][2]fieldVec3{{tri.A, tri.B}, {tri.B, tri.C}, {tri.C, tri.A}} {
			edges[fieldMembraneEdgeKey(edge[0], edge[1], tri.Level)]++
		}
	}
	for edge, count := range edges {
		if count != 2 {
			t.Fatalf("closed point-charge membrane has a crack at %+v: adjacent triangles=%d", edge, count)
		}
	}
}

func TestFieldCPUEquipotentialFillDoesNotExposeTriangleDiagonal126(t *testing.T) {
	r := fieldNewRaster(42, 42)
	r.fillTriangleFloat([3]float64{5, 36, 36}, [3]float64{5, 5, 36}, 50, 105, 215, .16)
	r.fillTriangleFloat([3]float64{5, 36, 5}, [3]float64{5, 36, 36}, 50, 105, 215, .16)
	minA, maxA := 255, 0
	for y := 7; y <= 33; y++ {
		for x := 7; x <= 33; x++ {
			a := int(r.Pix[(y*r.W+x)*4+3])
			if a == 0 {
				t.Fatalf("gap in two-triangle membrane at %d,%d", x, y)
			}
			minA, maxA = min(minA, a), max(maxA, a)
		}
	}
	if maxA-minA > 1 {
		t.Fatalf("shared triangle diagonal was double-painted: min alpha=%d max alpha=%d", minA, maxA)
	}
}

func TestFieldSmoothedMembraneRemainsOnPhysicalPotential126(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	o := fieldDefaultObject(fieldObjectSphere)
	o.Material = fieldMaterialInsulator
	o.Charge = 1
	s.Objects = []fieldObject{o}
	samples := fieldElectricalSamples(s, 1)
	const level = .7
	tris := fieldIsoSurfaceTriangles(s, samples, 20, []float64{level})[level]
	maxError := 0.0
	for _, tri := range tris {
		for _, p := range []fieldVec3{tri.A, tri.B, tri.C} {
			maxError = math.Max(maxError, math.Abs(fieldPotentialAt(s, samples, p)-level))
		}
	}
	if maxError > .003 {
		t.Fatalf("smoothed membrane left V=level: max error=%g", maxError)
	}
}

func TestFieldMembraneSilhouetteIgnoresOrphanMeshEdges126(t *testing.T) {
	tris := []fieldTriangle3{{
		A:     fieldVec3{X: -1, Z: -1},
		B:     fieldVec3{X: 1, Z: -1},
		C:     fieldVec3{Z: 1},
		Level: 1,
	}}
	p := fieldProjector{W: 200, H: 160, Dimension: fieldDimension3D}
	if edges := fieldIsoSurfaceSilhouetteEdges(tris, p); len(edges) != 0 {
		t.Fatalf("orphan tessellation edges were exposed as a membrane border: %d", len(edges))
	}
}

func TestFieldExtendedSizeSliderUsesFinalTenPercentForHugeRange128(t *testing.T) {
	for _, tc := range []struct {
		kind fieldObjectKind
		max  float64
	}{{fieldObjectPlane, fieldPlaneSizeMax}, {fieldObjectRod, fieldRodLengthMax}} {
		if got := fieldExtendedSizeFromSlider(tc.kind, 0); math.Abs(got-fieldPlaneSizeMin) > 1e-9 {
			t.Fatalf("kind=%d left endpoint=%g", tc.kind, got)
		}
		if got := fieldExtendedSizeFromSlider(tc.kind, fieldExtendedSizeKneePos); math.Abs(got-20) > 1e-9 {
			t.Fatalf("kind=%d 90%% knee=%g want 20", tc.kind, got)
		}
		if got := fieldExtendedSizeFromSlider(tc.kind, fieldExtendedSizeSliderMax); math.Abs(got-tc.max) > 1e-9 {
			t.Fatalf("kind=%d right endpoint=%g want %g", tc.kind, got, tc.max)
		}
		for _, value := range []float64{.35, 1, 7.5, 19.9, 20, (20 + tc.max) * .5, tc.max} {
			pos := fieldExtendedSizeSliderPosition(tc.kind, value)
			back := fieldExtendedSizeFromSlider(tc.kind, pos)
			if math.Abs(back-value) > .05 {
				t.Fatalf("kind=%d mapping roundtrip value=%g pos=%d back=%g", tc.kind, value, pos, back)
			}
		}
	}
}

func TestFieldLargePlateRemainsInsideDynamicViewAndHasVacuumLevels128(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.Display = fieldDisplayEquipotential
	s.Antialias = false
	charge := fieldDefaultObject(fieldObjectSphere)
	charge.Charge = 2
	charge.Z = -1.4
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.Material = fieldMaterialConductor
	plate.Charge = 0
	plate.Size = fieldPlaneSizeMax
	plate.Z = .7
	plate.RotX = math.Pi / 2 // vertical XY plate, normal along Z
	s.Objects = []fieldObject{charge, plate}
	fieldClampScene(&s)
	span := fieldSceneSpan(s)
	if span < 13 {
		t.Fatalf("large plate did not expand scene span: %g", span)
	}
	p := fieldProjectorForScene(s, 640, 480)
	ex, _, ez := fieldObjectBasis(plate)
	for _, corner := range []fieldVec3{
		fieldObjectCenter(plate).add(ex.mul(-plate.Size * .5)).add(ez.mul(-plate.Size * .5)),
		fieldObjectCenter(plate).add(ex.mul(plate.Size * .5)).add(ez.mul(-plate.Size * .5)),
		fieldObjectCenter(plate).add(ex.mul(plate.Size * .5)).add(ez.mul(plate.Size * .5)),
		fieldObjectCenter(plate).add(ex.mul(-plate.Size * .5)).add(ez.mul(plate.Size * .5)),
	} {
		if _, _, _, ok := p.projectFloat(corner); !ok {
			t.Fatalf("large plate corner vanished behind/outside camera: %+v", corner)
		}
	}
	samples := fieldElectricalSamples(s, 0)
	levels := fieldPotentialLevels(s, samples, fieldPlaneSlice{Origin: fieldVec3{}, U: fieldVec3{X: 1}, V: fieldVec3{Z: 1}}, 22)
	if len(levels) == 0 {
		t.Fatal("large plate caused all surrounding-vacuum equipotential levels to disappear")
	}
	r := fieldRenderForBackground(s, 260, 200, 0, false)
	covered := 0
	for _, m := range r.Mask {
		if m {
			covered++
		}
	}
	if covered < 200 {
		t.Fatalf("large-plate preview rendered too little content: %d", covered)
	}
}

func TestFieldElectricLineStressHasNoJumpsOrNonFinitePoints128(t *testing.T) {
	scenes := []fieldScene{
		func() fieldScene {
			s := fieldDefaultScene()
			a, b := fieldDefaultObject(fieldObjectSphere), fieldDefaultObject(fieldObjectSphere)
			a.X, a.Charge = -1.1, 3
			b.X, b.Charge = 1.15, -2
			s.Objects = []fieldObject{a, b}
			return s
		}(),
		func() fieldScene {
			s := fieldDefaultScene()
			s.Dimension = fieldDimension3D
			a := fieldDefaultObject(fieldObjectSphere)
			a.Z, a.Charge = -1.2, 2.5
			p := fieldDefaultObject(fieldObjectPlane)
			p.Material, p.Charge, p.Z, p.Size, p.RotX = fieldMaterialConductor, 0, .7, 6.5, math.Pi/2
			s.Objects = []fieldObject{a, p}
			return s
		}(),
		func() fieldScene {
			s := fieldDefaultScene()
			r := fieldDefaultObject(fieldObjectRod)
			r.Charge, r.Length, r.RotY = 4, 20, .4
			q := fieldDefaultObject(fieldObjectSphere)
			q.Charge, q.X, q.Z = -3, 1.4, -.9
			s.Objects = []fieldObject{r, q}
			return s
		}(),
	}
	for si, s := range scenes {
		s.Density = 8
		samples := fieldElectricalSamples(s, 0)
		lines := fieldElectricLines(s, samples)
		if len(lines) == 0 {
			t.Fatalf("scene %d generated no electric lines", si)
		}
		maxJump := 0.0
		minTangent := 1.0
		segments := 0
		for li, line := range lines {
			for i, p := range line.Points {
				if !fieldFiniteVec(p) {
					t.Fatalf("scene %d line %d point %d is non-finite: %+v", si, li, i, p)
				}
				if i == 0 {
					continue
				}
				seg := p.sub(line.Points[i-1])
				jump := seg.len()
				maxJump = math.Max(maxJump, jump)
				segments++
				if jump > .111 {
					t.Fatalf("scene %d line %d jumps by %g", si, li, jump)
				}
				mid := p.add(line.Points[i-1]).mul(.5)
				e := fieldElectricAt(s, samples, mid)
				if e.len2() > 1e-14 && seg.len2() > 1e-14 {
					dot := seg.norm().dot(e.norm())
					minTangent = math.Min(minTangent, dot)
					if dot < .40 {
						t.Fatalf("scene %d line %d segment %d is not tangent to E: dot=%g", si, li, i, dot)
					}
				}
			}
		}
		t.Logf("scene=%d lines=%d segments=%d max_jump=%.6f min_tangent=%.6f infinity=%.3f", si, len(lines), segments, maxJump, minTangent, fieldSceneNumericalInfinityRadius(s))
	}
}

func TestWideNeutralPlateImageAsymptoticProducesSmoothClosedMembranes129(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.Display = fieldDisplayEquipotential
	s.Density = 10
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.Material, plate.Charge = fieldMaterialConductor, 0
	plate.Z, plate.Size, plate.RotX = .5, 20, math.Pi/2
	charge := fieldDefaultObject(fieldObjectSphere)
	charge.Charge, charge.Z, charge.Radius = 2, -1, .35
	s.Objects = []fieldObject{plate, charge}

	samples, ok := fieldWidePlateImageSamples(s, 0)
	if !ok {
		t.Fatal("wide neutral plate did not select the Maxwell image/BEM asymptotic solver")
	}
	if len(samples) < 500 {
		t.Fatalf("wide plate remained under-resolved: samples=%d", len(samples))
	}
	// Image charge plus finite-plate correction must preserve the physical
	// neutral plate total exactly.
	plateEffective := 0.0
	for _, sample := range samples {
		if sample.Object == 0 || sample.Object == -1 {
			plateEffective += sample.Q
		}
	}
	if math.Abs(plateEffective) > 1e-9 {
		t.Fatalf("effective plate charge drifted from neutral: %.12g", plateEffective)
	}

	vc, ok := fieldConductorPotential(s, samples, 0)
	if !ok {
		t.Fatal("wide plate has no conductor potential")
	}
	ex, _, ez := fieldObjectBasis(plate)
	lo, hi := math.Inf(1), math.Inf(-1)
	for iz := 0; iz <= 14; iz++ {
		for ix := 0; ix <= 14; ix++ {
			u := -.44 + .88*float64(ix)/14
			v := -.44 + .88*float64(iz)/14
			p := fieldObjectCenter(plate).add(ex.mul(u * plate.Size)).add(ez.mul(v * plate.Size))
			got := fieldPotentialFromSamples(samples, p)
			lo, hi = math.Min(lo, got), math.Max(hi, got)
		}
	}
	if spread := (hi - lo) / math.Max(math.Abs(vc), 1e-9); spread > .24 {
		t.Fatalf("wide plate is not sufficiently equipotential between collocation centres: relative spread=%g", spread)
	}

	levels := fieldPotentialLevels(s, samples, fieldPlaneSlice{Origin: fieldVec3{}, U: fieldVec3{X: 1}, V: fieldVec3{Z: 1}}, 30)
	if len(levels) < 4 {
		t.Fatalf("too few useful positive-side levels: %v", levels)
	}
	for _, level := range levels {
		if level <= 0 {
			t.Fatalf("opposite-side image singularity leaked into level selection: %v", levels)
		}
		gap := math.Max(math.Abs(vc)*.065, math.Abs(level)*.02)
		if math.Abs(level-vc) <= gap {
			t.Fatalf("level too close to conductor plateau Vc=%g: %g", vc, level)
		}
	}

	triMap := fieldIsoSurfaceTriangles(s, samples, 36, levels)
	closedLevels := 0
	for _, level := range levels {
		tris := triMap[level]
		if len(tris) == 0 {
			continue
		}
		closedLevels++
		edges := make(map[fieldMembraneEdgeID]int)
		for _, tri := range tris {
			for _, edge := range [][2]fieldVec3{{tri.A, tri.B}, {tri.B, tri.C}, {tri.C, tri.A}} {
				edges[fieldMembraneEdgeKey(edge[0], edge[1], level)]++
			}
		}
		for edge, count := range edges {
			if count != 2 {
				t.Fatalf("level %g contains an open/torn membrane edge %+v count=%d", level, edge, count)
			}
		}
	}
	if closedLevels < 3 {
		t.Fatalf("expected at least three smooth closed membranes, got %d from %v", closedLevels, levels)
	}

	// Central square plate + axial point charge is mirror symmetric in X.
	for _, p := range []fieldVec3{{X: .35, Z: -1.45}, {X: .8, Z: -2.1}, {X: 1.2, Y: .3, Z: -1.7}} {
		a := fieldPotentialFromSamples(samples, p)
		b := fieldPotentialFromSamples(samples, fieldVec3{X: -p.X, Y: p.Y, Z: p.Z})
		if math.Abs(a-b) > 2e-3*math.Max(1, math.Abs(a)) {
			t.Fatalf("wide-plate solution lost mirror symmetry at %+v: %g vs %g", p, a, b)
		}
	}
}

func TestWidePlateImageBasisIsGatedAwayFromBackside130(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.Display = fieldDisplayEquipotential
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.Material, plate.Charge = fieldMaterialConductor, 0
	plate.Z, plate.Size, plate.RotX = .5, 20, math.Pi/2
	charge := fieldDefaultObject(fieldObjectSphere)
	charge.Charge, charge.Z, charge.Radius = 2, -1, .35
	s.Objects = []fieldObject{plate, charge}

	samples, ok := fieldWidePlateImageSamples(s, 0)
	if !ok {
		t.Fatal("wide-plate gated image solver was not selected")
	}
	_, normal, _, _, _ := fieldWidePlateChargeSide(s)
	center := fieldObjectCenter(plate)
	d := fieldObjectCenter(charge).sub(center).dot(normal)
	imagePos := fieldObjectCenter(charge).sub(normal.mul(2 * d))
	back := fieldPotentialAt(s, samples, imagePos)
	if math.IsNaN(back) || math.IsInf(back, 0) || math.Abs(back) > 10 {
		t.Fatalf("fictitious image singularity leaked into the physical back side: V=%g", back)
	}
	front := fieldPotentialAt(s, samples, fieldObjectCenter(charge))
	if front < 100 {
		t.Fatalf("actual point-charge singularity disappeared from the source side: V=%g", front)
	}
	gated := 0
	for _, sample := range samples {
		if sample.Gate {
			gated++
		}
	}
	if gated != 2 {
		t.Fatalf("expected one gated real/image pair, got %d gated samples", gated)
	}
}

func TestConductorBoundaryIsExplicitEquipotentialMembrane130(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.RotZ = 0 // isolate the intended X-axis quarter turn in this test
	plate.Material, plate.Charge = fieldMaterialConductor, 0
	plate.Size, plate.RotX = 8, math.Pi/2
	charge := fieldDefaultObject(fieldObjectSphere)
	charge.Charge, charge.Z = 1, -1
	s.Objects = []fieldObject{plate, charge}
	samples := fieldEquipotentialElectricalSamples(s, 0)
	vc, ok := fieldConductorPotential(s, samples, 0)
	if !ok {
		t.Fatal("conductor potential was not solved")
	}
	tris := fieldConductorEquipotentialTriangles(s, samples, 0)
	if len(tris) != 2 {
		t.Fatalf("ideal plate should be emitted as two membrane triangles, got %d", len(tris))
	}
	for i, tri := range tris {
		if !tri.Boundary {
			t.Fatalf("plate triangle %d was not marked as a physical open boundary", i)
		}
		if math.Abs(tri.Level-vc) > 1e-9 {
			t.Fatalf("plate triangle %d has level %g, want conductor Vc=%g", i, tri.Level, vc)
		}
	}
	p := fieldProjectorForScene(s, 320, 240)
	if edges := fieldIsoSurfaceSilhouetteEdges(tris, p); len(edges) != 4 {
		t.Fatalf("plate equipotential membrane should expose four physical border edges, got %d", len(edges))
	}
}

func TestWidePlateEquipotentialLevelsRemainPresentAcrossPreviewChanges1211(t *testing.T) {
	for _, size := range []float64{8, 12, 20, 24.5} {
		for _, dz := range []float64{-1.55, -1.50, -1.45} {
			s := fieldDefaultScene()
			s.Dimension = fieldDimension3D
			s.Display = fieldDisplayEquipotential
			plate := fieldDefaultObject(fieldObjectPlane)
			plate.Material, plate.Charge = fieldMaterialConductor, 0
			plate.Z, plate.Size, plate.RotX = .5, size, math.Pi/2
			charge := fieldDefaultObject(fieldObjectSphere)
			charge.Charge, charge.Z, charge.Radius = 2, dz, .35
			s.Objects = []fieldObject{plate, charge}

			samples := fieldEquipotentialElectricalSamples(s, 0)
			levelsA := fieldPotentialLevels(s, samples, fieldPlaneSlice{Origin: fieldVec3{}, U: fieldVec3{X: 1}, V: fieldVec3{Z: 1}}, 30)
			levelsB := fieldPotentialLevels(s, samples, fieldPlaneSlice{Origin: fieldVec3{}, U: fieldVec3{X: 1}, V: fieldVec3{Z: 1}}, 30)
			if len(levelsA) < 3 || len(levelsB) < 3 {
				t.Fatalf("size=%g dz=%g dropped levels: %v / %v", size, dz, levelsA, levelsB)
			}
			if len(levelsA) != len(levelsB) {
				t.Fatalf("size=%g dz=%g non-deterministic level count: %v / %v", size, dz, levelsA, levelsB)
			}
			for i := range levelsA {
				if math.Abs(levelsA[i]-levelsB[i]) > 1e-12 {
					t.Fatalf("size=%g dz=%g non-deterministic level %d: %g / %g", size, dz, i, levelsA[i], levelsB[i])
				}
			}
			if tris := fieldConductorEquipotentialTriangles(s, samples, 0); len(tris) != 2 {
				t.Fatalf("size=%g dz=%g conductor membrane disappeared: triangles=%d", size, dz, len(tris))
			}
			for _, density := range []int{8, 10, 14} {
				s.Density = density
				r := fieldRenderForBackground(s, 360, 280, 0, false)
				covered := 0
				for _, m := range r.Mask {
					if m {
						covered++
					}
				}
				if covered < 150 {
					t.Fatalf("size=%g dz=%g density=%d preview dropped equipotentials: covered=%d", size, dz, density, covered)
				}
			}
		}
	}
}

func TestConductorMembraneHasFiniteFallbackPotential1211(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.Material, plate.Charge, plate.Size, plate.RotX = fieldMaterialConductor, 0, 10, math.Pi/2
	s.Objects = []fieldObject{plate}
	tris := fieldConductorEquipotentialTriangles(s, nil, 0)
	if len(tris) != 2 {
		t.Fatalf("conductor membrane vanished when preview solve had no samples: %d", len(tris))
	}
	for _, tri := range tris {
		if math.IsNaN(tri.Level) || math.IsInf(tri.Level, 0) {
			t.Fatalf("fallback conductor level is non-finite: %g", tri.Level)
		}
	}
}

func TestFieldDefaultViewAndUprightPlateOrientation1212(t *testing.T) {
	s := fieldDefaultScene()
	if math.Abs(s.RotX)+math.Abs(s.RotY)+math.Abs(s.RotZ) > 1e-12 {
		t.Fatalf("default camera is not 0,0,0: (%g,%g,%g)", s.RotX, s.RotY, s.RotZ)
	}
	plate := fieldDefaultObject(fieldObjectPlane)
	if math.Abs(plate.RotX) > 1e-12 || math.Abs(plate.RotY) > 1e-12 || math.Abs(plate.RotZ-math.Pi/2) > 1e-12 {
		t.Fatalf("default plate is not exactly upright: rotations=(%g,%g,%g)", plate.RotX, plate.RotY, plate.RotZ)
	}
	ex, _, ez := fieldObjectBasis(plate)
	n := ex.cross(ez).norm()
	if n.dot(fieldVec3{X: 1}) < .999999999 {
		t.Fatalf("upright plate normal=%+v want +X", n)
	}
	p := fieldProjector{W: 640, H: 480, Dimension: fieldDimension3D}
	c := fieldObjectCenter(plate)
	x0 := 0
	for index, uv := range [][2]float64{{-1, -1}, {-1, 1}, {1, -1}, {1, 1}} {
		point := c.add(ex.mul(uv[0] * plate.Size * .5)).add(ez.mul(uv[1] * plate.Size * .5))
		x, _, _, ok := p.project(point)
		if !ok {
			t.Fatal("default upright plate corner did not project")
		}
		if index == 0 {
			x0 = x
		} else if math.Abs(float64(x-x0)) > 1 {
			t.Fatalf("upright plate is not an exact vertical edge at camera 0: first x=%d current=%d", x0, x)
		}
	}
}

func TestFieldThreeDPointSeedProjectedAzimuthsAreUniform1212(t *testing.T) {
	for _, count := range []int{10, 15, 24} {
		dirs := fieldSeedDirections(count, true)
		if len(dirs) != count {
			t.Fatalf("count=%d generated %d directions", count, len(dirs))
		}
		for i, d := range dirs {
			if math.Abs(d.len()-1) > 1e-12 {
				t.Fatalf("count=%d direction %d is not unit: %+v", count, i, d)
			}
			got := math.Atan2(d.Z, d.X)
			if got < 0 {
				got += 2 * math.Pi
			}
			want := 2 * math.Pi * float64(i) / float64(count)
			if math.Abs(got-want) > 1e-10 {
				t.Fatalf("count=%d direction %d projected azimuth=%g want=%g", count, i, got, want)
			}
		}
	}
}

func TestFieldUprightPlateAllowsPlanarPointChargeOnEitherSide1212(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.Material, plate.Charge = fieldMaterialConductor, 0
	plate.X = .6
	charge := fieldDefaultObject(fieldObjectSphere)
	charge.X = -.8
	s.Objects = []fieldObject{plate, charge}
	if fieldContainingConductor(s, fieldObjectCenter(charge), 0) >= 0 {
		t.Fatal("default upright plate still contains a planar-offset point charge")
	}
	samples := fieldElectricalSamples(s, 0)
	lines := fieldElectricLines(s, samples)
	if len(lines) < 8 {
		t.Fatalf("upright plate scene generated too few electric lines: %d", len(lines))
	}
	center := fieldObjectCenter(charge)
	sectors := [4]int{}
	for _, line := range lines {
		if len(line.Points) == 0 || line.Points[0].sub(center).len() > charge.Radius+.2 {
			continue // induced-conductor line, not a point-charge emission seed
		}
		d := line.Points[0].sub(center)
		a := math.Atan2(d.Z, d.X)
		if a < 0 {
			a += 2 * math.Pi
		}
		sectors[int(a/(math.Pi/2))%4]++
	}
	for i, n := range sectors {
		if n == 0 {
			t.Fatalf("point-charge rays still collapse to one side; empty screen quadrant %d counts=%v", i, sectors)
		}
	}
}

func TestFieldDefaultViewStartsAtExactZero1214(t *testing.T) {
	s := fieldDefaultScene()
	if s.RotX != 0 || s.RotY != 0 || s.RotZ != 0 {
		t.Fatalf("default view rotation=(%g,%g,%g), want exact zero", s.RotX, s.RotY, s.RotZ)
	}
}

func TestFieldEquipotentialLatticeDoublesEveryAxis1214(t *testing.T) {
	s := fieldDefaultScene()
	old3D := 18 + s.Density/2
	got3D := fieldEquipotentialGridSteps(s, 0)
	if got3D < old3D*2 {
		t.Fatalf("3-D grid steps=%d, want at least twice old %d", got3D, old3D)
	}
	old2D := 38 + s.Density
	got2D := fieldEquipotentialContourSteps(s, 0)
	if got2D < old2D*2 {
		t.Fatalf("2-D grid steps=%d, want at least twice old %d", got2D, old2D)
	}
}

func TestFieldAdaptiveEquipotentialSubdivisionIsConforming1214(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	charge := fieldDefaultObject(fieldObjectSphere)
	charge.Material = fieldMaterialInsulator
	charge.Charge = 2
	charge.Radius = .18
	s.Objects = []fieldObject{charge}
	samples := fieldElectricalSamples(s, 0)
	r := .70
	level := fieldPotentialAt(s, samples, fieldVec3{X: r})
	v := []fieldVec3{{X: r}, {X: -r}, {Y: r}, {Y: -r}, {Z: r}, {Z: -r}}
	idx := [][3]int{
		{0, 2, 4}, {4, 2, 1}, {1, 2, 5}, {5, 2, 0},
		{4, 3, 0}, {1, 3, 4}, {5, 3, 1}, {0, 3, 5},
	}
	tris := make([]fieldTriangle3, 0, len(idx))
	for _, f := range idx {
		tris = append(tris, fieldTriangle3{A: v[f[0]], B: v[f[1]], C: v[f[2]], Level: level, Boundary: true})
	}
	refined := fieldAdaptiveRefineIsoSurface(s, samples, tris, level, .30, 1)
	if len(refined) <= len(tris) {
		t.Fatalf("adaptive refinement did not add triangles: before=%d after=%d", len(tris), len(refined))
	}
	edges := make(map[fieldMembraneEdgeID]int)
	maxResidual := 0.0
	for _, tri := range refined {
		for _, p := range []fieldVec3{tri.A, tri.B, tri.C} {
			if !fieldFiniteVec(p) {
				t.Fatalf("non-finite refined vertex: %+v", p)
			}
			maxResidual = math.Max(maxResidual, math.Abs(fieldPotentialAt(s, samples, p)-level))
		}
		for _, e := range [][2]fieldVec3{{tri.A, tri.B}, {tri.B, tri.C}, {tri.C, tri.A}} {
			edges[fieldMembraneEdgeKey(e[0], e[1], tri.Level)]++
		}
	}
	for edge, count := range edges {
		if count != 2 {
			t.Fatalf("adaptive mesh has hanging/open edge %+v count=%d", edge, count)
		}
	}
	if maxResidual > .035 {
		t.Fatalf("refined vertices left V=level: max residual=%g", maxResidual)
	}
}

func TestFieldElectricLineValidationStress1214(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.Density = 18
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.Material, plate.Charge = fieldMaterialConductor, 0
	plate.X, plate.Size = .75, 12
	charge := fieldDefaultObject(fieldObjectSphere)
	charge.Material, charge.Charge = fieldMaterialInsulator, 2.5
	charge.X, charge.Z = -.80, .18
	s.Objects = []fieldObject{plate, charge}
	samples := fieldElectricalSamples(s, 1)
	lines := fieldElectricLines(s, samples)
	if len(lines) < 12 {
		t.Fatalf("stress scene generated too few validated lines: %d", len(lines))
	}
	report := fieldValidateElectricLines(s, samples, lines)
	t.Logf("validated lines=%d segments=%d max_segment=%.6f min_alignment=%.6f", report.Lines, report.Segments, report.MaxSegment, report.MinAlignment)
	if !fieldElectricLineValidationOK(report) {
		t.Fatalf("electric-line validation failed: %+v", report)
	}
	if report.Segments < 100 || report.MinAlignment < .90 {
		t.Fatalf("electric-line tangent quality too low: %+v", report)
	}
}

func TestFieldNormalizedPotentialBandsCreateEightEightPercentLevels1222(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.Display = fieldDisplayEquipotential
	q := fieldDefaultObject(fieldObjectSphere)
	q.Charge = 2
	q.X, q.Z = .15, -.20
	s.Objects = []fieldObject{q}
	samples := fieldEquipotentialElectricalSamples(s, 0)
	grid := fieldBuildPotentialGrid(s, samples, 36)
	bands := fieldPotentialBandsFromGrid(grid)
	if len(bands) != 1 || bands[0].Sign != 1 {
		t.Fatalf("expected one positive full-grid band, got %+v", bands)
	}
	levels := fieldPotentialSurfaceLevels(bands)
	if len(levels) != 8 {
		t.Fatalf("0..56%% at 8%% intervals must create 8 levels, got %d: %v", len(levels), levels)
	}
	for i, level := range levels {
		percent, ok := fieldPotentialBandPercent(bands[0], level)
		if !ok {
			t.Fatalf("level %d did not map back into its band: %g", i, level)
		}
		want := float64(i) * .08
		if math.Abs(percent-want) > 1e-10 {
			t.Fatalf("level %d percent=%g want=%g", i, percent, want)
		}
	}
}

func TestFieldHighPotentialVolumeBeginsContinuouslyAtFortyPercent1215(t *testing.T) {
	if got := fieldPotentialVolumeDensity(.40); got != 0 {
		t.Fatalf("40%% boundary must start transparent, got density %g", got)
	}
	if got := fieldPotentialVolumeDensity(.399); got != 0 {
		t.Fatalf("below 40%% must not enter the volume, got density %g", got)
	}
	d44 := fieldPotentialVolumeDensity(.44)
	d70 := fieldPotentialVolumeDensity(.70)
	d100 := fieldPotentialVolumeDensity(1.0)
	if !(d44 > 0 && d44 < d70 && d70 < d100) {
		t.Fatalf("volume density must rise smoothly from 40 to 100%%: %g %g %g", d44, d70, d100)
	}
	if math.Abs(d100-1) > 1e-12 {
		t.Fatalf("100%% density=%g want 1", d100)
	}
}

func TestFieldNormalizedBandLowerBoundClosesInsideSamplingCube1215(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	q := fieldDefaultObject(fieldObjectSphere)
	q.Charge = 1.5
	s.Objects = []fieldObject{q}
	samples := fieldEquipotentialElectricalSamples(s, 0)
	grid := fieldBuildPotentialGrid(s, samples, 34)
	bands := fieldPotentialBandsFromGrid(grid)
	if len(bands) != 1 {
		t.Fatalf("expected one band, got %+v", bands)
	}
	maxBoundary := 0.0
	for k := 0; k < grid.Steps; k++ {
		for j := 0; j < grid.Steps; j++ {
			for i := 0; i < grid.Steps; i++ {
				if i != 0 && j != 0 && k != 0 && i != grid.Steps-1 && j != grid.Steps-1 && k != grid.Steps-1 {
					continue
				}
				maxBoundary = math.Max(maxBoundary, grid.Values[grid.index(i, j, k)])
			}
		}
	}
	if bands[0].Low <= maxBoundary {
		t.Fatalf("0%% level must lie above every same-sign cube-boundary sample: low=%g boundary=%g", bands[0].Low, maxBoundary)
	}
}

func TestEqualChargeSphereAndPlateReceiveEqualAcceptedFluxQuotas1216(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.Display = fieldDisplayElectricLines
	s.Density = 10

	sphere := fieldDefaultObject(fieldObjectSphere)
	sphere.X, sphere.Charge = -.65, 1
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.X, plate.Charge = .85, 1
	// Keep the default small plate: this was the exact case where .30*soft
	// placed every seed inside the numerical sheet thickness.
	plate.Size = 1.25
	s.Objects = []fieldObject{sphere, plate}

	samples := fieldElectricalSamples(s, 0)
	positiveCount, _, _ := fieldFluxLineCounts(samples, s.Density)
	groups := fieldPositiveEmissionSeedGroups(s, samples, positiveCount)
	if len(groups) != 2 {
		t.Fatalf("expected one source quota for sphere and plate, got %+v", groups)
	}
	acceptedByObject := map[int]int{}
	for _, group := range groups {
		if group.Target != 10 {
			t.Fatalf("equal +1 sources must each receive 10 of 20 lines: object=%d target=%d", group.Object, group.Target)
		}
		for _, seed := range group.Seeds {
			if acceptedByObject[group.Object] >= group.Target {
				break
			}
			pts := fieldIntegrateElectricLine(s, samples, seed)
			line := fieldLine{Points: pts, Arrow: len(pts) * 3 / 5, Sign: 1}
			if len(pts) >= 5 && fieldElectricLineValidationOK(fieldValidateElectricLines(s, samples, []fieldLine{line})) {
				acceptedByObject[group.Object]++
			}
		}
		if acceptedByObject[group.Object] != group.Target {
			t.Fatalf("source quota was lost after validation: object=%d accepted=%d target=%d", group.Object, acceptedByObject[group.Object], group.Target)
		}
	}
	if got := len(fieldElectricLines(s, samples)); got != 20 {
		t.Fatalf("equal +1 sphere/plate should render all 20 accepted lines, got %d", got)
	}
}

func TestSmallPlateFluxSeedsStartOutsideNumericalSheet1216(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.Size, plate.Charge = 1.25, 1
	s.Objects = []fieldObject{plate}
	samples := fieldElectricalSamples(s, 0)
	groups := fieldPositiveEmissionSeedGroups(s, samples, 10)
	if len(groups) != 1 || len(groups[0].Seeds) < groups[0].Target {
		t.Fatalf("plate seed group missing: %+v", groups)
	}
	for i := 0; i < groups[0].Target; i++ {
		if fieldPointInsideObject(groups[0].Seeds[i], plate, .010) {
			t.Fatalf("plate seed %d started inside numerical sheet: %+v", i, groups[0].Seeds[i])
		}
	}
}

func TestFieldMixedSignsAnchorAtExactZeroAndCreateFifteenLevels1222(t *testing.T) {
	grid := fieldPotentialGrid{Steps: 5, MinCoord: -1, MaxCoord: 1}
	grid.Values = make([]float64, grid.Steps*grid.Steps*grid.Steps)
	grid.Conductor = make([]bool, len(grid.Values))
	for k := 0; k < grid.Steps; k++ {
		for j := 0; j < grid.Steps; j++ {
			for i := 0; i < grid.Steps; i++ {
				x := -1 + 2*float64(i)/float64(grid.Steps-1)
				// Add a small transverse term so both signs have ample nonzero
				// samples while the exact x=0 separator remains present.
				grid.Values[grid.index(i, j, k)] = x * (1 + .05*float64(j+k))
			}
		}
	}
	bands := fieldPotentialBandsFromGrid(grid)
	if len(bands) != 2 || !bands[0].ZeroAnchored || !bands[1].ZeroAnchored {
		t.Fatalf("mixed-sign grid must create two zero-anchored bands: %+v", bands)
	}
	levels := fieldPotentialSurfaceLevels(bands)
	if len(levels) != 15 {
		t.Fatalf("zero plus seven 8%% levels on each sign must create 15 levels, got %d: %v", len(levels), levels)
	}
	zeroCount := 0
	for _, level := range levels {
		if math.Abs(level) < 1e-14 {
			zeroCount++
		}
	}
	if zeroCount != 1 {
		t.Fatalf("exact V=0 level must appear once, got %d in %v", zeroCount, levels)
	}
	if levels[0] >= 0 || levels[len(levels)-1] <= 0 {
		t.Fatalf("levels must cover both signs around zero: %v", levels)
	}
	for _, band := range bands {
		if got := fieldPotentialBandLevel(band, 0); got != 0 {
			t.Fatalf("zero-anchored 0%% level=%g want exact zero", got)
		}
		l8 := fieldPotentialBandLevel(band, 8)
		p8, ok := fieldPotentialBandPercent(band, l8)
		if !ok || math.Abs(p8-.08) > 1e-10 {
			t.Fatalf("8%% round trip failed: level=%g percent=%g ok=%v", l8, p8, ok)
		}
	}
}

func TestFieldExactZeroOpenSeparatorSurvivesFiniteGrid1217(t *testing.T) {
	steps := 18
	grid := fieldPotentialGrid{
		Steps: steps, MinCoord: -1, MaxCoord: 1,
		Values:    make([]float64, steps*steps*steps),
		Conductor: make([]bool, steps*steps*steps), Backend: "TEST",
	}
	for k := 0; k < steps; k++ {
		for j := 0; j < steps; j++ {
			for i := 0; i < steps; i++ {
				x := grid.MinCoord + (grid.MaxCoord-grid.MinCoord)*float64(i)/float64(steps-1)
				grid.Values[grid.index(i, j, k)] = x
			}
		}
	}
	tris := fieldIsoSurfaceTrianglesFromGridQuality(fieldScene{}, nil, grid, []float64{0}, 0)[0]
	if len(tris) == 0 {
		t.Fatal("the physical open V=0 plane was removed by closed-surface filtering")
	}
	if !fieldIsoSurfaceTouchesGridBoundary(tris, grid.MinCoord, grid.MaxCoord, .02) {
		t.Fatal("test separator should reach the finite numerical boundary")
	}
	for _, tri := range tris {
		for _, p := range []fieldVec3{tri.A, tri.B, tri.C} {
			if math.Abs(p.X) > 1e-8 {
				t.Fatalf("zero separator vertex left x=0: %+v", p)
			}
		}
	}
}

func TestFieldMembraneFacesUseStableCPURasterPolicy1218(t *testing.T) {
	if fieldUseGPUMembraneFaces {
		t.Fatal("1.2.18 must keep equipotential faces on the verified CPU alpha compositor")
	}
}

func TestFieldPremultipliedGPUCompositePreservesExistingVolume1218(t *testing.T) {
	r := fieldNewRaster(1, 1)
	// Existing straight-alpha blue volume, alpha 0.50.
	r.Pix[0], r.Pix[1], r.Pix[2], r.Pix[3] = 255, 0, 0, 128
	r.Mask[0] = true
	// Premultiplied red membrane, source alpha 0.25: Rpremul=64.
	pix := []byte{0, 0, 64, 64}
	if !fieldApplyPremultipliedGPU(&r, pix) {
		t.Fatal("valid premultiplied membrane was rejected")
	}
	if r.Pix[3] <= 128 {
		t.Fatalf("source membrane was not composited over volume: alpha=%d", r.Pix[3])
	}
	if r.Pix[0] == 0 || r.Pix[2] == 0 {
		t.Fatalf("over composite lost one layer: BGRA=%v", r.Pix[:4])
	}
}

func TestFieldMixedSignEquipotentialRenderContainsVisibleSheetFaces1218(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.Display = fieldDisplayEquipotential
	s.Antialias = false
	s.Density = 10
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.X, plate.Charge = -1, 1
	sphere := fieldDefaultObject(fieldObjectSphere)
	sphere.X, sphere.Charge = 1, -1
	s.Objects = []fieldObject{plate, sphere}

	samples := fieldEquipotentialElectricalSamples(s, 0)
	grid := fieldBuildPotentialGrid(s, samples, fieldEquipotentialGridSteps(s, 0))
	bands := fieldPotentialBandsFromGrid(grid)
	levels := fieldPotentialSurfaceLevels(bands)
	triMap := fieldIsoSurfaceTrianglesFromGridQuality(s, samples, grid, levels, 0)
	triangles := 0
	for _, level := range levels {
		triangles += len(triMap[level])
	}
	if triangles < 100 {
		t.Fatalf("mixed-sign membranes were not generated: levels=%d triangles=%d", len(levels), triangles)
	}

	p := fieldProjectorForScene(s, 320, 240)
	r := fieldNewRaster(320, 240)
	all := make([]fieldTriangle3, 0, triangles)
	for _, level := range levels {
		all = append(all, triMap[level]...)
	}
	projected := fieldProjectedTriangles(all, p)
	for i := range projected {
		tri := &projected[i]
		rr, gg, bb := fieldPotentialColor(tri.Tri.Level, false)
		r.fillTriangleFloat(tri.X, tri.Y, rr, gg, bb, fieldTriangleFaceAlpha(tri.Tri))
	}
	covered := 0
	for i := 3; i < len(r.Pix); i += 4 {
		if r.Pix[i] > 0 {
			covered++
		}
	}
	if covered < 2000 {
		t.Fatalf("generated membranes did not raster visibly: projected=%d covered=%d", len(projected), covered)
	}
}

func TestFinalPreviewGeometryFallback1219(t *testing.T) {
	scene := fieldDefaultScene()
	scene.Dimension = fieldDimension3D
	scene.Display = fieldDisplayEquipotential
	o := fieldDefaultObject(fieldObjectSphere)
	o.Material = fieldMaterialConductor
	o.Charge = 1
	scene.Objects = []fieldObject{o}

	r := fieldNewRaster(320, 240)
	if got := fieldRasterVisiblePixelCount(r); got != 0 {
		t.Fatalf("new raster visible pixels=%d, want 0", got)
	}
	if !fieldEnsurePreviewGeometry(&r, scene) {
		t.Fatal("final preview geometry fallback was not applied to an empty physical frame")
	}
	if got := fieldRasterVisiblePixelCount(r); got < 44 {
		t.Fatalf("geometry fallback visible pixels=%d, want at least 44", got)
	}
	if fieldEnsurePreviewGeometry(&r, scene) {
		t.Fatal("geometry fallback should not be applied again once the scene is visible")
	}
}

func TestFieldInsulatingPlaneUsesStableGaussAreaQuadrature1220(t *testing.T) {
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.Material = fieldMaterialInsulator
	plate.Size = 20
	plate.Charge = 1.75
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.Display = fieldDisplayEquipotential
	s.Objects = []fieldObject{plate}

	q0 := fieldElectricalSamples(s, 0)
	q1 := fieldElectricalSamples(s, 1)
	if len(q0) != 24*24 || len(q1) != 24*24 {
		t.Fatalf("large preview/export plate must share 24x24 Gauss samples, got q0=%d q1=%d", len(q0), len(q1))
	}
	for label, samples := range map[string][]fieldChargeSample{"q0": q0, "q1": q1} {
		total := 0.0
		for _, sample := range samples {
			total += sample.Q
			if sample.Soft >= plate.Size/12*.20 {
				t.Fatalf("%s retained the old broad midpoint softening: soft=%g", label, sample.Soft)
			}
		}
		if math.Abs(total-plate.Charge) > 1e-12 {
			t.Fatalf("%s Gauss quadrature did not preserve total charge: got=%g want=%g", label, total, plate.Charge)
		}
	}
}

func TestFieldPotentialGridTranslationFollowsActiveScene1220(t *testing.T) {
	base := fieldDefaultScene()
	base.Dimension = fieldDimension3D
	base.Display = fieldDisplayEquipotential
	plate := fieldDefaultObject(fieldObjectPlane)
	plate.Size, plate.Charge, plate.X, plate.Z = 5, 1, -1.1, .35
	point := fieldDefaultObject(fieldObjectSphere)
	point.Charge, point.X, point.Z = -1, 1.2, -.25
	base.Objects = []fieldObject{plate, point}

	shifted := base
	shifted.Objects = append([]fieldObject(nil), base.Objects...)
	const dx, dz = .37, -.29
	for i := range shifted.Objects {
		shifted.Objects[i].X += dx
		shifted.Objects[i].Z += dz
	}
	g0 := fieldBuildPotentialGrid(base, fieldEquipotentialElectricalSamples(base, 0), 20)
	g1 := fieldBuildPotentialGrid(shifted, fieldEquipotentialElectricalSamples(shifted, 0), 20)
	if math.Abs((g1.Center.X-g0.Center.X)-dx) > 1e-12 || math.Abs((g1.Center.Z-g0.Center.Z)-dz) > 1e-12 {
		t.Fatalf("grid center did not follow translated scene: c0=%+v c1=%+v", g0.Center, g1.Center)
	}
	if len(g0.Values) != len(g1.Values) {
		t.Fatalf("translated grids differ in size: %d vs %d", len(g0.Values), len(g1.Values))
	}
	for i := range g0.Values {
		if math.Abs(g0.Values[i]-g1.Values[i]) > 2e-10*math.Max(1, math.Abs(g0.Values[i])) {
			t.Fatalf("translation changed relative scalar sample %d: %g vs %g", i, g0.Values[i], g1.Values[i])
		}
	}
}

func TestFieldMixedSignMembranesSurviveSubcellDrags1220(t *testing.T) {
	for _, shift := range []float64{-0.06, 0, .06} {
		s := fieldDefaultScene()
		s.Dimension = fieldDimension3D
		s.Display = fieldDisplayEquipotential
		s.Density = 10
		plate := fieldDefaultObject(fieldObjectPlane)
		plate.Size, plate.Charge, plate.X = 8, 1, -1+shift
		point := fieldDefaultObject(fieldObjectSphere)
		point.Charge, point.X = -1, 1+shift
		s.Objects = []fieldObject{plate, point}
		samples := fieldEquipotentialElectricalSamples(s, 0)
		grid := fieldBuildPotentialGrid(s, samples, 34)
		levels := fieldPotentialSurfaceLevels(fieldPotentialBandsFromGrid(grid))
		triMap := fieldIsoSurfaceTrianglesFromGridQuality(s, samples, grid, levels, 0)
		renderedLevels, triangles := 0, 0
		for _, level := range levels {
			if n := len(triMap[level]); n > 0 {
				renderedLevels++
				triangles += n
			}
		}
		if renderedLevels < 3 || triangles < 100 {
			t.Fatalf("subcell drag shift=%g collapsed membranes: levels=%d triangles=%d requested=%d", shift, renderedLevels, triangles, len(levels))
		}
	}
}

func TestFieldOneSignEquipotentialShellsSurviveTopologyFilter1222(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.Display = fieldDisplayEquipotential
	s.Antialias = false
	s.Density = 10
	a := fieldDefaultObject(fieldObjectSphere)
	a.X, a.Z, a.Charge = -0.85, -0.2, 1
	b := fieldDefaultObject(fieldObjectSphere)
	b.X, b.Z, b.Charge = 0.95, 0.25, 1
	s.Objects = []fieldObject{a, b}

	samples := fieldEquipotentialElectricalSamples(s, 0)
	if len(samples) != 2 {
		t.Fatalf("two ideal insulating spheres should produce two samples, got %d", len(samples))
	}
	grid := fieldBuildPotentialGrid(s, samples, fieldEquipotentialGridSteps(s, 0))
	levels := fieldPotentialSurfaceLevels(fieldPotentialBandsFromGrid(grid))
	if len(levels) != 8 {
		t.Fatalf("one-sign 0..56%% at 8%% spacing must produce 8 levels, got %d: %v", len(levels), levels)
	}
	triMap := fieldIsoSurfaceTrianglesFromGridQuality(s, samples, grid, levels, 0)
	rendered, triangles := 0, 0
	for _, level := range levels {
		if n := len(triMap[level]); n > 0 {
			rendered++
			triangles += n
		}
	}
	if rendered < 6 || triangles < 1000 {
		t.Fatalf("one-sign physical shells were removed: rendered=%d/%d triangles=%d", rendered, len(levels), triangles)
	}
}

func TestFieldMixedSignExtendedLevels1222(t *testing.T) {
	grid := fieldPotentialGrid{Steps: 5, MinCoord: -1, MaxCoord: 1}
	grid.Values = make([]float64, grid.Steps*grid.Steps*grid.Steps)
	grid.Conductor = make([]bool, len(grid.Values))
	for k := 0; k < grid.Steps; k++ {
		for j := 0; j < grid.Steps; j++ {
			for i := 0; i < grid.Steps; i++ {
				x := -1 + 2*float64(i)/float64(grid.Steps-1)
				grid.Values[grid.index(i, j, k)] = x * (1 + .05*float64(j+k))
			}
		}
	}
	levels := fieldPotentialSurfaceLevels(fieldPotentialBandsFromGrid(grid))
	if len(levels) != 15 {
		t.Fatalf("mixed-sign -56..+56%% at 8%% spacing must produce 15 levels, got %d: %v", len(levels), levels)
	}
	neg, zero, pos := 0, 0, 0
	for _, level := range levels {
		switch {
		case level < 0:
			neg++
		case level > 0:
			pos++
		default:
			zero++
		}
	}
	if neg != 7 || zero != 1 || pos != 7 {
		t.Fatalf("extended mixed-sign level split=%d/%d/%d want 7/1/7", neg, zero, pos)
	}
}

func TestFiniteBarMagnetFieldUsesFiniteLengthAndInteriorDirection123(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	m := fieldDefaultObject(fieldObjectBarMagnet)
	m.Length = 2.0
	m.Charge = 1
	s.Objects = []fieldObject{m}
	ex, _, ez := fieldObjectBasis(m)
	c := fieldObjectCenter(m)
	r := fieldBarMagnetRadius(m)

	// Outside the north pole B leaves the magnet; outside the south pole it
	// points back toward the magnet.  Both vectors therefore have +ex component.
	for name, p := range map[string]fieldVec3{
		"north": c.add(ex.mul(m.Length*.5 + .18)),
		"south": c.add(ex.mul(-m.Length*.5 - .18)),
	} {
		b := fieldMagneticAt(s, p)
		if got := b.dot(ex); !(got > 0) {
			t.Fatalf("%s axial field=%g want positive", name, got)
		}
	}
	// At the equatorial side the exterior field returns N -> S, opposite ex.
	if got := fieldMagneticAt(s, c.add(ez.mul(r*3.0))).dot(ex); !(got < 0) {
		t.Fatalf("equatorial axial field=%g want negative", got)
	}
	// B inside a permanent magnet goes S -> N.  The old centre-point dipole
	// produced the opposite direction at r=0 because of its softened singularity.
	if got := fieldMagneticAt(s, c).dot(ex); !(got > 0) {
		t.Fatalf("interior axial field=%g want positive", got)
	}
}

func TestFiniteBarMagnetRotationRotatesFiniteField123(t *testing.T) {
	s := fieldDefaultScene()
	m := fieldDefaultObject(fieldObjectBarMagnet)
	m.Length = 1.8
	m.Charge = 1
	m.RotY = math.Pi / 2
	s.Objects = []fieldObject{m}
	ex, _, _ := fieldObjectBasis(m)
	p := fieldObjectCenter(m).add(ex.mul(m.Length*.5 + .2))
	b := fieldMagneticAt(s, p).norm()
	if got := b.dot(ex); got < .97 {
		t.Fatalf("rotated north-pole field alignment=%g want >= .97", got)
	}
}

func TestBarMagnetLinesStartInNorthernVacuumAndTraverseActualLength123(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension2D
	s.Display = fieldDisplayMagneticLines
	s.Density = 8
	m := fieldDefaultObject(fieldObjectBarMagnet)
	m.Length = 1.8
	m.Charge = 1
	s.Objects = []fieldObject{m}
	lines := fieldMagneticLines(s)
	if len(lines) < 6 {
		t.Fatalf("bar magnet produced only %d lines", len(lines))
	}
	ex, _, _ := fieldObjectBasis(m)
	c := fieldObjectCenter(m)
	northX := m.Length * .5
	foundSouth, foundInterior := false, false
	for _, ln := range lines {
		if len(ln.Points) < 20 {
			continue
		}
		startLocal := ln.Points[0].sub(c).dot(ex)
		if startLocal <= 0 || ln.Points[0].sub(c).len() <= northX {
			t.Fatalf("line seed was not in northern vacuum: axial=%g radius=%g", startLocal, ln.Points[0].sub(c).len())
		}
		for _, p := range ln.Points {
			local := p.sub(c).dot(ex)
			if local < -m.Length*.45 {
				foundSouth = true
			}
			if fieldPointInsideObject(p, m, 0) {
				foundInterior = true
			}
		}
	}
	if !foundSouth {
		t.Fatal("no magnetic line reached the south-pole end")
	}
	if !foundInterior {
		t.Fatal("magnetic B lines did not pass through the bar body")
	}
}

func TestBarMagnet3DSeedsCoverTwelveAzimuthDirections124(t *testing.T) {
	m := fieldDefaultObject(fieldObjectBarMagnet)
	m.Length = 2.0
	m.Charge = 1
	seeds := fieldBarMagnetLineSeeds(m, 12, true)
	if len(seeds) != 12 {
		t.Fatalf("3-D bar magnet seed count=%d want 12", len(seeds))
	}
	ex, ey, ez := fieldObjectBasis(m)
	center := fieldObjectCenter(m)
	seen := map[int]bool{}
	for _, seed := range seeds {
		v := seed.sub(center)
		transverse := v.sub(ex.mul(v.dot(ex))).norm()
		if transverse.len2() < 1e-12 {
			t.Fatal("seed has no transverse direction")
		}
		angle := math.Atan2(transverse.dot(ez), transverse.dot(ey))
		if angle < 0 {
			angle += 2 * math.Pi
		}
		index := int(math.Round(angle/(2*math.Pi)*12)) % 12
		seen[index] = true
	}
	if len(seen) != 12 {
		t.Fatalf("covered azimuth directions=%d want 12: %v", len(seen), seen)
	}
}

func TestPreviewGeometryOverlayAlwaysShowsCurrentObjects124(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	o := fieldDefaultObject(fieldObjectSphere)
	o.X, o.Z, o.Charge = .7, -.35, 1
	s.Objects = []fieldObject{o}
	r := fieldNewRaster(320, 240)
	if !fieldOverlayPreviewGeometry(&r, s) {
		t.Fatal("geometry overlay was not installed")
	}
	if got := fieldRasterVisiblePixelCount(r); got < 20 {
		t.Fatalf("geometry overlay visible pixels=%d want >=20", got)
	}
}

func TestBarMagnetOrthographicEndOnStillCoversTwelveDirections125(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension2D // orthographic projection, not planar physics
	s.Display = fieldDisplayMagneticLines
	s.Density = 10
	m := fieldDefaultObject(fieldObjectBarMagnet)
	m.Length = 1.35
	m.Charge = 1
	// Rotate the magnet axis (+X) onto the camera depth axis (+Y).  The former
	// 2-D seed branch selected just one meridional plane here, yielding the two
	// opposite vertical rays reported by the user.
	m.RotZ = math.Pi / 2
	s.Objects = []fieldObject{m}

	lines := fieldMagneticLines(s)
	if len(lines) < 12 {
		t.Fatalf("end-on orthographic bar magnet lines=%d want >=12", len(lines))
	}
	projector := fieldProjectorForScene(s, 560, 430)
	cx, cy, _, ok := projector.project(fieldObjectCenter(m))
	if !ok {
		t.Fatal("magnet centre did not project")
	}
	seen := map[int]bool{}
	for _, line := range lines {
		for _, point := range line.Points {
			x, y, _, visible := projector.project(point)
			if !visible {
				continue
			}
			dx, dy := float64(x-cx), float64(y-cy)
			if math.Hypot(dx, dy) < 25 {
				continue
			}
			angle := math.Atan2(dy, dx)
			if angle < 0 {
				angle += 2 * math.Pi
			}
			bucket := int(math.Round(angle/(2*math.Pi)*12)) % 12
			seen[bucket] = true
		}
	}
	if len(seen) != 12 {
		t.Fatalf("end-on orthographic directions=%d want 12: %v", len(seen), seen)
	}
}

func TestCircularCurrentLoopUsesFiniteBiotSavartRing126(t *testing.T) {
	loop := fieldDefaultObject(fieldObjectCurrentLoop)
	loop.RotZ = 0
	loop.Charge = 1
	scene := fieldDefaultScene()
	scene.Dimension = fieldDimension3D
	scene.Display = fieldDisplayMagneticLines
	scene.Objects = []fieldObject{loop}

	model := fieldBuildMagneticModel(scene)
	if got := len(model.FiniteCurrents); got != 64 {
		t.Fatalf("current-loop finite segments=%d want 64", got)
	}
	ex, _, _ := fieldObjectBasis(loop)
	centre := fieldObjectCenter(loop)
	if got := fieldMagneticAt(scene, centre).dot(ex); !(got > 0) {
		t.Fatalf("positive loop centre field=%g want +normal", got)
	}
	loop.Charge = -1
	scene.Objects[0] = loop
	if got := fieldMagneticAt(scene, centre).dot(ex); !(got < 0) {
		t.Fatalf("negative loop centre field=%g want -normal", got)
	}
}

func TestHorseshoeMagnetGapFieldPointsNorthToSouth126(t *testing.T) {
	horse := fieldDefaultObject(fieldObjectHorseshoeMagnet)
	horse.Charge = 1
	scene := fieldDefaultScene()
	scene.Dimension = fieldDimension3D
	scene.Display = fieldDisplayMagneticLines
	scene.Objects = []fieldObject{horse}

	south, north, _, _ := fieldHorseshoePoleGeometry(horse)
	ex, _, ez := fieldObjectBasis(horse)
	gap := south.add(north).mul(.5).add(ez.mul(.08))
	if got := fieldMagneticAt(scene, gap).dot(ex); !(got < 0) {
		t.Fatalf("positive horseshoe gap field=%g want N(right)->S(left)", got)
	}
	horse.Charge = -1
	scene.Objects[0] = horse
	if got := fieldMagneticAt(scene, gap).dot(ex); !(got > 0) {
		t.Fatalf("reversed horseshoe gap field=%g want left->right", got)
	}
}

func TestNewMagneticSeedsCoverTwelveAzimuthDirections126(t *testing.T) {
	check := func(name string, seeds []fieldVec3, origin, axis, u, v fieldVec3) {
		t.Helper()
		if len(seeds) < 12 {
			t.Fatalf("%s seeds=%d want >=12", name, len(seeds))
		}
		seen := map[int]bool{}
		for _, seed := range seeds[:12] {
			d := seed.sub(origin)
			transverse := d.sub(axis.mul(d.dot(axis))).norm()
			if transverse.len2() < 1e-12 {
				t.Fatalf("%s seed has no transverse component", name)
			}
			a := math.Atan2(transverse.dot(v), transverse.dot(u))
			if a < 0 {
				a += 2 * math.Pi
			}
			seen[int(math.Round(a/(2*math.Pi)*12))%12] = true
		}
		if len(seen) != 12 {
			t.Fatalf("%s azimuth buckets=%d want 12: %v", name, len(seen), seen)
		}
	}

	horse := fieldDefaultObject(fieldObjectHorseshoeMagnet)
	_, north, _, northOut := fieldHorseshoePoleGeometry(horse)
	_, hey, _ := fieldObjectBasis(horse)
	hu := hey.sub(northOut.mul(hey.dot(northOut))).norm()
	hv := northOut.cross(hu).norm()
	check("horseshoe", fieldHorseshoeLineSeeds(horse, 12), north, northOut, hu, hv)

	loop := fieldDefaultObject(fieldObjectCurrentLoop)
	loop.RotZ = 0
	ex, ey, ez := fieldObjectBasis(loop)
	check("current-loop", fieldCurrentLoopLineSeeds(loop, 12), fieldObjectCenter(loop), ex, ey, ez)
}

func TestNewMagneticObjectsProduceFieldLinesAndVisibleGeometry126(t *testing.T) {
	for _, kind := range []fieldObjectKind{fieldObjectHorseshoeMagnet, fieldObjectCurrentLoop} {
		scene := fieldDefaultScene()
		scene.Dimension = fieldDimension3D
		scene.Display = fieldDisplayMagneticLines
		scene.Density = 10
		object := fieldDefaultObject(kind)
		if kind == fieldObjectCurrentLoop {
			object.RotZ = 0
		}
		scene.Objects = []fieldObject{object}
		if lines := fieldMagneticLines(scene); len(lines) < 12 {
			t.Fatalf("kind=%d magnetic lines=%d want >=12", kind, len(lines))
		}
		raster := fieldNewRaster(360, 280)
		fieldDrawObject(&raster, fieldProjectorForScene(scene, raster.W, raster.H), object)
		if visible := fieldRasterVisiblePixelCount(raster); visible < 40 {
			t.Fatalf("kind=%d visible geometry=%d want >=40", kind, visible)
		}
	}
}
