package main

import (
	"math"
	"testing"
)

func TestGravitySolidAndHollowSphereInterior(t *testing.T) {
	solid := fieldDefaultObject(fieldObjectSolidMassSphere)
	solid.Charge = 6
	solid.Radius = 1
	hollow := solid
	hollow.Kind = fieldObjectHollowMassSphere
	p := fieldVec3{X: .5}

	gs := fieldGravitySphereAt(solid, p)
	if math.Abs(gs.X+3) > 1e-9 || math.Abs(gs.Y) > 1e-12 || math.Abs(gs.Z) > 1e-12 {
		t.Fatalf("solid interior field=%+v, want (-3,0,0)", gs)
	}
	gh := fieldGravitySphereAt(hollow, p)
	if gh.len() > 1e-12 {
		t.Fatalf("hollow shell interior field=%+v, want zero", gh)
	}
	vs := fieldGravitySpherePotential(solid, p)
	vh := fieldGravitySpherePotential(hollow, p)
	if math.Abs(vs+8.25) > 1e-9 {
		t.Fatalf("solid interior potential=%g want -8.25", vs)
	}
	if math.Abs(vh+6) > 1e-9 {
		t.Fatalf("hollow interior potential=%g want -6", vh)
	}
}

func TestGravitySphereExteriorMatchesPointMass(t *testing.T) {
	for _, kind := range []fieldObjectKind{fieldObjectSolidMassSphere, fieldObjectHollowMassSphere} {
		o := fieldDefaultObject(kind)
		o.Charge = 8
		o.Radius = .7
		p := fieldVec3{X: 2}
		g := fieldGravitySphereAt(o, p)
		if math.Abs(g.X+2) > 1e-9 {
			t.Fatalf("kind=%v exterior field=%+v want x=-2", kind, g)
		}
		if math.Abs(fieldGravitySpherePotential(o, p)+4) > 1e-9 {
			t.Fatalf("kind=%v exterior potential mismatch", kind)
		}
	}
}

func TestGravityPotentialSamplesPreserveMassSign(t *testing.T) {
	scene := fieldScene{Display: fieldDisplayGravityPotential, Objects: []fieldObject{
		func() fieldObject { o := fieldDefaultObject(fieldObjectSolidMassSphere); o.Charge = 2; return o }(),
		func() fieldObject { o := fieldDefaultObject(fieldObjectMassRing); o.Charge = 3; return o }(),
	}}
	samples := fieldGravityPotentialSamples(scene, 0)
	if len(samples) < 60 {
		t.Fatalf("too few gravity samples: %d", len(samples))
	}
	sum := 0.0
	for _, s := range samples {
		sum += s.Q
	}
	if math.Abs(sum+5) > 1e-9 {
		t.Fatalf("sample mass sum=%g want -5", sum)
	}
}

func TestGravityBlackHoleUsesPhysicalCompactness138(t *testing.T) {
	o := fieldDefaultObject(fieldObjectSolidMassSphere)
	o.Charge = 9.0
	o.Radius = .10
	if got := fieldGravitySchwarzschildRadius(o); math.Abs(got-.1125) > 1e-12 {
		t.Fatalf("M=9 Schwarzschild radius=%g want .1125", got)
	}
	if got := fieldGravityBlackHoleStrength(o); got <= 0 {
		t.Fatalf("M=9 R=.1 should be black hole, strength=%g", got)
	}
	o.Charge = 9.1
	o.Radius = .20
	if got := fieldGravityBlackHoleStrength(o); got != 0 {
		t.Fatalf("M=9.1 R=.2 must not be black hole, strength=%g", got)
	}
	o.Charge = 8.9
	o.Radius = .08
	if got := fieldGravityBlackHoleStrength(o); got <= 0 {
		t.Fatalf("M=8.9 R=.08 should be black hole, strength=%g", got)
	}
}

func TestRotatingCompactStarGetsContinuousRelativity138(t *testing.T) {
	o := fieldDefaultObject(fieldObjectSolidMassSphere)
	o.Charge = 8.0
	o.Radius = .12 // compact star, but R > rs=0.1: not a black hole
	o.Spin = .99
	if fieldGravityBlackHoleStrength(o) != 0 {
		t.Fatal("compact star incorrectly classified as black hole")
	}
	p := fieldVec3{X: .30}
	c := fieldGravityRelativisticFieldCorrection(o, p)
	if c.len() == 0 || math.Abs(c.Z) < 1e-6 {
		t.Fatalf("rotating compact star has no frame-dragging correction: %+v", c)
	}
}

func TestGravityModesRenderVisiblePixels(t *testing.T) {
	sphere := fieldDefaultObject(fieldObjectSolidMassSphere)
	sphere.Charge = 4
	sphere.Radius = .5
	for _, mode := range []fieldDisplayMode{fieldDisplayGravityPotential, fieldDisplayGravityLines, fieldDisplayGravityVectors} {
		scene := fieldScene{Dimension: fieldDimension2D, Display: mode, Objects: []fieldObject{sphere}, Density: 7, VectorDensity: 10}
		r := fieldRenderForBackgroundBase(scene, 220, 180, 0, false)
		visible := 0
		for _, v := range r.Mask {
			if v {
				visible++
			}
		}
		if visible == 0 {
			t.Fatalf("gravity mode %v rendered no visible pixels", mode)
		}
	}
}

func TestGravityThreeDimensionalPotentialProducesMembrane(t *testing.T) {
	sphere := fieldDefaultObject(fieldObjectSolidMassSphere)
	sphere.Charge = 5
	sphere.Radius = .6
	scene := fieldScene{Dimension: fieldDimension3D, Display: fieldDisplayGravityPotential, Objects: []fieldObject{sphere}, Density: 4}
	r := fieldRenderForBackgroundBase(scene, 180, 150, 0, false)
	if !r.EquipotentialExpected || r.EquipotentialRenderedLevels == 0 || r.EquipotentialVacuumTriangles == 0 {
		t.Fatalf("gravity membrane missing: expected=%v levels=%d triangles=%d", r.EquipotentialExpected, r.EquipotentialRenderedLevels, r.EquipotentialVacuumTriangles)
	}
}

func TestGravityLinesRunFromNumericalInfinityToMassSurface136(t *testing.T) {
	sphere := fieldDefaultObject(fieldObjectSolidMassSphere)
	sphere.Charge = 5
	sphere.Radius = .55
	scene := fieldScene{Dimension: fieldDimension2D, Display: fieldDisplayGravityLines, Objects: []fieldObject{sphere}, Density: 4}
	lines := fieldGravityLines(scene)
	if len(lines) < 8 {
		t.Fatalf("complete gravity lines=%d, want at least 8", len(lines))
	}
	outer := fieldSceneNumericalInfinityRadius(scene) * .92
	for i, line := range lines {
		if len(line.Points) < 8 {
			t.Fatalf("line %d points=%d", i, len(line.Points))
		}
		start := line.Points[0].len()
		end := line.Points[len(line.Points)-1].sub(fieldObjectCenter(sphere)).len()
		if start < outer*.98 {
			t.Fatalf("line %d starts too near mass: r=%g outer=%g", i, start, outer)
		}
		if math.Abs(end-sphere.Radius) > .012 {
			t.Fatalf("line %d does not terminate on surface: r=%g radius=%g", i, end, sphere.Radius)
		}
	}
}

func TestRotatingBlackHoleAddsFirstAndSecondOrderBending136(t *testing.T) {
	o := fieldDefaultObject(fieldObjectSolidMassSphere)
	o.Charge = fieldGravityMassMax
	o.Radius = fieldGravityRadiusFromSlider(0)
	o.Spin = .99
	p := fieldVec3{X: o.Radius * 2.2}
	newtonian := fieldGravitySphereAt(o, p)
	corrected := newtonian.add(fieldGravityRelativisticFieldCorrection(o, p))
	if corrected.len() <= newtonian.len() {
		t.Fatalf("GR4 correction did not strengthen near-horizon field: newtonian=%+v corrected=%+v", newtonian, corrected)
	}
	if math.Abs(corrected.Z) < newtonian.len()*.05 {
		t.Fatalf("a*=0.99 frame dragging not visible: corrected=%+v", corrected)
	}
	basePotential := fieldGravitySpherePotential(o, p)
	correctedPotential := basePotential + fieldGravityRelativisticPotentialCorrection(o, p)
	if correctedPotential >= basePotential {
		t.Fatalf("corrected potential=%g should be deeper than Newtonian=%g", correctedPotential, basePotential)
	}
}

func TestBlackHoleRotationAxisChangesGravityBending136(t *testing.T) {
	o := fieldDefaultObject(fieldObjectSolidMassSphere)
	o.Charge = fieldGravityMassMax
	o.Radius = fieldGravityRadiusFromSlider(0)
	o.Spin = .99
	p := fieldVec3{Z: o.Radius * 2.4}
	gY := fieldGravityRelativisticFieldCorrection(o, p)
	o.RotZ = -math.Pi / 2 // rotate local +Y spin axis to world +X
	gX := fieldGravityRelativisticFieldCorrection(o, p)
	if math.Abs(gY.X) < 1e-6 || math.Abs(gX.Y) < 1e-6 {
		t.Fatalf("spin-axis bending missing: default=%+v rotated=%+v", gY, gX)
	}
	if gY.sub(gX).len() < .05*math.Max(gY.len(), gX.len()) {
		t.Fatalf("object rotation barely changed black-hole field: default=%+v rotated=%+v", gY, gX)
	}
}

func TestGravityDistributedMassLinesReachSurface137(t *testing.T) {
	for _, kind := range []fieldObjectKind{fieldObjectMassPlane, fieldObjectMassRing} {
		o := fieldDefaultObject(kind)
		o.Charge = 4
		scene := fieldScene{Dimension: fieldDimension3D, Display: fieldDisplayGravityLines, Objects: []fieldObject{o}, Density: 8}
		lines := fieldGravityLines(scene)
		if len(lines) < 4 {
			t.Fatalf("kind=%v complete lines=%d, want at least 4", kind, len(lines))
		}
		outer := fieldGravityLineOuterRadius(scene)
		for i, line := range lines {
			if len(line.Points) < 8 {
				t.Fatalf("kind=%v line=%d points=%d", kind, i, len(line.Points))
			}
			if line.Points[0].len() < outer*.97 {
				t.Fatalf("kind=%v line=%d start radius=%g outer=%g", kind, i, line.Points[0].len(), outer)
			}
			end := line.Points[len(line.Points)-1]
			if !fieldPointInsideObject(end, o, .003) {
				t.Fatalf("kind=%v line=%d endpoint is not on mass surface: %+v", kind, i, end)
			}
		}
	}
}

func TestGravityLineModeReturnsPhysicalLinesNotOnlyObjectPixels137(t *testing.T) {
	o := fieldDefaultObject(fieldObjectSolidMassSphere)
	o.Charge = 4
	o.Radius = .5
	scene := fieldScene{Dimension: fieldDimension3D, Display: fieldDisplayGravityLines, Objects: []fieldObject{o}, Density: 14}
	lines := fieldGravityLines(scene)
	if len(lines) != 14 {
		t.Fatalf("gravity physical lines=%d want 14", len(lines))
	}
	for i, line := range lines {
		if fieldPolylineArcLength(line.Points) < fieldGravityLineOuterRadius(scene)*.70 {
			t.Fatalf("line %d too short: arc=%g", i, fieldPolylineArcLength(line.Points))
		}
	}
}
