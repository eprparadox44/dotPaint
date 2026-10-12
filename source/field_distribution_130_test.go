package main

import (
	"math"
	"testing"
)

func TestCurrentWireSeedsUseDistinctConcentricRadii130(t *testing.T) {
	wire := fieldDefaultObject(fieldObjectCurrentWire)
	wire.RotX, wire.RotY, wire.RotZ = 0, 0, 0
	seeds := fieldCurrentWireLineSeeds(wire, 18)
	if len(seeds) != 18 {
		t.Fatalf("wire seeds=%d want=18", len(seeds))
	}
	center := fieldObjectCenter(wire)
	_, axis, _ := fieldObjectBasis(wire)
	last := 0.0
	for i, seed := range seeds {
		d := seed.sub(center)
		perp := d.sub(axis.mul(d.dot(axis)))
		radius := perp.len()
		if i > 0 && radius <= last+1e-9 {
			t.Fatalf("wire radius did not increase: i=%d radius=%g last=%g", i, radius, last)
		}
		last = radius
	}
	if last < .8 {
		t.Fatalf("wire outer radius too small: %g", last)
	}
}

func TestBarAntipodalAzimuthsDoNotDuplicateSameOrbitSeed130(t *testing.T) {
	bar := fieldDefaultObject(fieldObjectBarMagnet)
	seeds := fieldBarMagnetLineSeeds(bar, 12, true)
	if len(seeds) != 12 {
		t.Fatalf("bar seeds=%d", len(seeds))
	}
	center := fieldObjectCenter(bar)
	ex, _, _ := fieldObjectBasis(bar)
	polar := make([]float64, 12)
	for i, seed := range seeds {
		d := seed.sub(center).norm()
		polar[i] = math.Acos(fieldClamp(d.dot(ex), -1, 1))
	}
	// Locate seeds whose transverse directions are antipodal by bucket and make
	// sure their polar angles differ, so they cannot be the same axisymmetric loop.
	for i := 0; i < 6; i++ {
		if math.Abs(polar[i]-polar[i+6]) < 1e-4 {
			t.Fatalf("antipodal seeds %d/%d still duplicate polar=%g", i, i+6, polar[i])
		}
	}
}

func TestScreenHaloRejectsAdjacentCellNearDuplicate130(t *testing.T) {
	line := func(y float64) fieldScreenLineCandidate {
		return fieldScreenLineCandidate{Group: 0, Points: []fieldScreenPoint{{X: 4, Y: y}, {X: 116, Y: y}}}
	}
	candidates := []fieldScreenLineCandidate{line(24), line(31), line(70)}
	selected, stats := fieldSelectScreenDiverseLinesDetailed(candidates, 120, 100, 6, 3)
	seen := map[int]bool{}
	for _, index := range selected {
		seen[index] = true
	}
	if seen[0] && seen[1] {
		t.Fatalf("adjacent-cell near duplicates both survived: selected=%v stats=%+v", selected, stats)
	}
	if !seen[2] {
		t.Fatalf("distant line was rejected: selected=%v stats=%+v", selected, stats)
	}
}
