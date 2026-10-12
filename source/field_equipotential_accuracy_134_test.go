package main

import (
	"math"
	"testing"
)

func TestEquipotentialGridAccuracySteps134(t *testing.T) {
	s := fieldDefaultScene()
	s.Density = 14
	if got := fieldEquipotentialGridSteps(s, 0); got != 52 {
		t.Fatalf("preview grid=%d want 52", got)
	}
	if got := fieldEquipotentialGridSteps(s, 1); got != 60 {
		t.Fatalf("retry grid=%d want 60", got)
	}
	if got := fieldEquipotentialGridSteps(s, 2); got != 70 {
		t.Fatalf("final grid=%d want 70", got)
	}
}

func TestEquipotentialVertexReprojectionReducesResidual134(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	q := fieldDefaultObject(fieldObjectSphere)
	q.Material = fieldMaterialInsulator
	q.Charge = 2
	q.Radius = .18
	s.Objects = []fieldObject{q}
	samples := fieldElectricalSamples(s, 0)

	levelRadius := 1.25
	level := fieldPotentialAt(s, samples, fieldVec3{X: levelRadius})
	// Deliberately perturb three vertices away from the physical sphere.
	tri := fieldTriangle3{
		A:     fieldVec3{X: levelRadius * 1.12},
		B:     fieldVec3{Y: levelRadius * .88},
		C:     fieldVec3{Z: levelRadius * 1.07},
		Level: level,
	}
	maxResidual := func(tris []fieldTriangle3) float64 {
		worst := 0.0
		for _, tr := range tris {
			for _, p := range []fieldVec3{tr.A, tr.B, tr.C} {
				worst = math.Max(worst, math.Abs(fieldPotentialAt(s, samples, p)-level))
			}
		}
		return worst
	}
	before := maxResidual([]fieldTriangle3{tri})
	refined := fieldReprojectIsoSurfaceVertices(s, samples, []fieldTriangle3{tri}, level, .25, 3)
	if len(refined) != 1 {
		t.Fatalf("reprojection returned %d triangles", len(refined))
	}
	after := maxResidual(refined)
	if !(after < before*.02) {
		t.Fatalf("residual did not improve enough: before=%g after=%g", before, after)
	}
	if after > 2e-5*math.Max(1, math.Abs(level)) {
		t.Fatalf("refined residual too large: level=%g residual=%g", level, after)
	}
}

func TestEquipotentialProjectionNeverIncreasesResidual134(t *testing.T) {
	s := fieldDefaultScene()
	q := fieldDefaultObject(fieldObjectSphere)
	q.Material = fieldMaterialInsulator
	q.Charge = 3
	s.Objects = []fieldObject{q}
	samples := fieldElectricalSamples(s, 0)
	level := fieldPotentialAt(s, samples, fieldVec3{X: .9})
	start := fieldVec3{X: .52, Y: .21, Z: .17}
	before := math.Abs(fieldPotentialAt(s, samples, start) - level)
	got := fieldProjectPointToEquipotentialIterations(s, samples, start, level, .18, 5)
	after := math.Abs(fieldPotentialAt(s, samples, got) - level)
	if after > before*(1+1e-8) {
		t.Fatalf("safeguarded projection increased residual: before=%g after=%g", before, after)
	}
}
