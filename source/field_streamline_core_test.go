package main

import (
	"math"
	"testing"
)

func TestFieldRK45ConstantFieldIsStraight(t *testing.T) {
	policy := fieldDefaultStreamlinePolicy()
	policy.InitialStep = 0.07
	policy.MaxStep = 0.09
	policy.MaxArcLength = 1.0
	policy.MaxTurnRadians = 0
	policy.Inside = func(p fieldVec3) bool { return p.X <= 1.01 }

	result := fieldIntegrateStreamline(func(fieldVec3) fieldVec3 {
		return fieldVec3{X: 4}
	}, fieldVec3{}, 1, policy)
	if len(result.Points) < 5 || result.Stagnated {
		t.Fatalf("unexpected result: points=%d stagnated=%v", len(result.Points), result.Stagnated)
	}
	for i, p := range result.Points {
		if math.Abs(p.Y) > 1e-12 || math.Abs(p.Z) > 1e-12 {
			t.Fatalf("point %d left x axis: %+v", i, p)
		}
	}
}

func TestFieldRK45CircularFieldClosesAndKeepsRadius(t *testing.T) {
	policy := fieldDefaultStreamlinePolicy()
	policy.InitialStep = 0.05
	policy.MinStep = 0.0015
	policy.MaxStep = 0.07
	policy.AbsTolerance = 2e-6
	policy.RelTolerance = 2e-6
	policy.MaxArcLength = 8
	policy.ClosureDistance = 0.025
	policy.MinClosureLength = 5.5
	policy.MinClosureSteps = 60
	policy.MaxTurnRadians = 8 * math.Pi / 180

	result := fieldIntegrateStreamline(func(p fieldVec3) fieldVec3 {
		return fieldVec3{X: -p.Z, Z: p.X}
	}, fieldVec3{X: 1}, 1, policy)
	if !result.Closed {
		t.Fatalf("circle did not close: points=%d arc=%g rejected=%d", len(result.Points), result.ArcLength, result.Rejected)
	}
	maxRadiusError := 0.0
	for _, p := range result.Points {
		maxRadiusError = math.Max(maxRadiusError, math.Abs(math.Hypot(p.X, p.Z)-1))
	}
	if maxRadiusError > 2e-4 {
		t.Fatalf("circle radius drift too large: %g", maxRadiusError)
	}
}

func TestFieldRK45TighterToleranceReducesCircleDrift(t *testing.T) {
	eval := func(p fieldVec3) fieldVec3 { return fieldVec3{X: -p.Z, Z: p.X} }
	loose := fieldDefaultStreamlinePolicy()
	loose.InitialStep, loose.MinStep, loose.MaxStep = 0.11, 0.01, 0.16
	loose.AbsTolerance, loose.RelTolerance = 2e-3, 2e-3
	loose.MaxArcLength, loose.MaxTurnRadians = 3.1, 0
	strict := loose
	strict.AbsTolerance, strict.RelTolerance = 1e-8, 1e-8
	strict.MinStep = 0.0005

	a := fieldIntegrateStreamline(eval, fieldVec3{X: 1}, 1, loose)
	b := fieldIntegrateStreamline(eval, fieldVec3{X: 1}, 1, strict)
	drift := func(r fieldStreamlineResult) float64 {
		m := 0.0
		for _, p := range r.Points {
			m = math.Max(m, math.Abs(math.Hypot(p.X, p.Z)-1))
		}
		return m
	}
	if drift(b) > drift(a)+1e-10 {
		t.Fatalf("strict tolerance did not improve drift: loose=%g strict=%g", drift(a), drift(b))
	}
}

func TestFieldStreamlineStopClipsBoundary(t *testing.T) {
	policy := fieldDefaultStreamlinePolicy()
	policy.InitialStep, policy.MinStep, policy.MaxStep = 0.2, 0.02, 0.2
	policy.MaxTurnRadians = 0
	policy.Stop = func(prev, next fieldVec3) (bool, fieldVec3, int) {
		if prev.X < 0.55 && next.X >= 0.55 {
			return true, fieldVec3{X: 0.55}, 3
		}
		return false, fieldVec3{}, -1
	}
	result := fieldIntegrateStreamline(func(fieldVec3) fieldVec3 { return fieldVec3{X: 1} }, fieldVec3{}, 1, policy)
	last := result.Points[len(result.Points)-1]
	if result.HitObject != 3 || math.Abs(last.X-0.55) > 1e-12 {
		t.Fatalf("boundary clipping failed: hit=%d last=%+v", result.HitObject, last)
	}
}

func TestFieldVectorGridTrilinearInterpolationIsExactForAffineField(t *testing.T) {
	grid := fieldVectorGrid{
		NX: 5, NY: 4, NZ: 6,
		Min: fieldVec3{X: -2, Y: -1, Z: -3},
		Max: fieldVec3{X: 2, Y: 3, Z: 2},
	}
	affine := func(p fieldVec3) fieldVec3 {
		return fieldVec3{
			X: 2*p.X + 3*p.Y - p.Z + 1,
			Y: -p.X + 0.5*p.Z - 2,
			Z: p.Y + p.Z + 4,
		}
	}
	grid.Values = make([]fieldVec3, grid.NX*grid.NY*grid.NZ)
	for iz := 0; iz < grid.NZ; iz++ {
		for iy := 0; iy < grid.NY; iy++ {
			for ix := 0; ix < grid.NX; ix++ {
				p := fieldVec3{
					X: grid.Min.X + (grid.Max.X-grid.Min.X)*float64(ix)/float64(grid.NX-1),
					Y: grid.Min.Y + (grid.Max.Y-grid.Min.Y)*float64(iy)/float64(grid.NY-1),
					Z: grid.Min.Z + (grid.Max.Z-grid.Min.Z)*float64(iz)/float64(grid.NZ-1),
				}
				grid.Values[grid.index(ix, iy, iz)] = affine(p)
			}
		}
	}
	for _, p := range []fieldVec3{{X: -1.1, Y: 0.2, Z: -0.7}, {X: 1.7, Y: 2.4, Z: 1.3}, {X: 0, Y: 0, Z: 0}} {
		got, ok := grid.sample(p)
		want := affine(p)
		if !ok || fieldLen(fieldSub(got, want)) > 1e-11 {
			t.Fatalf("sample mismatch at %+v: got=%+v want=%+v ok=%v", p, got, want, ok)
		}
	}
}

func TestFieldMergeBidirectionalKeepsPositiveOrientation(t *testing.T) {
	backward := fieldStreamlineResult{Points: []fieldVec3{{X: 0}, {X: -1}, {X: -2}}, Accepted: 2, ArcLength: 2, MinStep: 1, MaxStep: 1}
	forward := fieldStreamlineResult{Points: []fieldVec3{{X: 0}, {X: 1}, {X: 2}}, Accepted: 2, ArcLength: 2, MinStep: 1, MaxStep: 1}
	merged := fieldMergeBidirectionalStreamlines(backward, forward)
	want := []float64{-2, -1, 0, 1, 2}
	if len(merged.Points) != len(want) {
		t.Fatalf("unexpected merged length: %d", len(merged.Points))
	}
	for i, x := range want {
		if merged.Points[i].X != x {
			t.Fatalf("point %d: got %g want %g", i, merged.Points[i].X, x)
		}
	}
}

func TestFieldStreamlineValidationDetectsMisalignment(t *testing.T) {
	eval := func(fieldVec3) fieldVec3 { return fieldVec3{X: 1} }
	good := fieldValidateStreamline(eval, []fieldVec3{{X: 0}, {X: 0.1}, {X: 0.2}}, 1, 0.11, 0.99, false, 0, 0)
	if good.Misaligned != 0 || good.LongJump != 0 {
		t.Fatalf("good line rejected: %+v", good)
	}
	bad := fieldValidateStreamline(eval, []fieldVec3{{X: 0}, {Y: 0.1}}, 1, 0.2, 0.9, false, 0, 0)
	if bad.Misaligned == 0 {
		t.Fatalf("misaligned line was not detected: %+v", bad)
	}
}

func TestFieldScreenOccupancyRejectsDuplicateButPreservesGroups(t *testing.T) {
	line := func(y float64, group int) fieldScreenLineCandidate {
		return fieldScreenLineCandidate{Group: group, Points: []fieldScreenPoint{{X: 5, Y: y}, {X: 95, Y: y}}}
	}
	candidates := []fieldScreenLineCandidate{
		line(20, 0),
		line(20.5, 0), // near duplicate of candidate 0
		line(70, 0),
		line(45, 1), // distinct physical group must survive
	}
	selected := fieldSelectScreenDiverseLines(candidates, 100, 100, 8, 3)
	if len(selected) != 3 {
		t.Fatalf("selected %d lines, want 3: %v", len(selected), selected)
	}
	seenGroup1, duplicatePair := false, false
	seen := map[int]bool{}
	for _, index := range selected {
		seen[index] = true
		if candidates[index].Group == 1 {
			seenGroup1 = true
		}
	}
	duplicatePair = seen[0] && seen[1]
	if !seenGroup1 || duplicatePair {
		t.Fatalf("group/duplicate policy failed: selected=%v", selected)
	}
}
