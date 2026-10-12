package main

import (
	"math"
	"testing"
)

func fieldPlaneSeedSide(o fieldObject, p fieldVec3) float64 {
	ex, _, ez := fieldObjectBasis(o)
	n := ex.cross(ez).norm()
	return p.sub(fieldObjectCenter(o)).dot(n)
}

func TestUniformInsulatingPlaneSeedsBothFacesAtLowDensity135(t *testing.T) {
	for _, density := range []int{4, 5, 10, 16, 24} {
		s := fieldDefaultScene()
		s.Dimension = fieldDimension3D
		s.Display = fieldDisplayElectricLines
		s.Density = density
		p := fieldDefaultObject(fieldObjectPlane)
		p.Material = fieldMaterialInsulator
		p.Charge = 1
		p.Size = 1.25
		s.Objects = []fieldObject{p}
		samples := fieldElectricalSamples(s, 1)
		positiveCount, _, _ := fieldFluxLineCounts(samples, density)
		groups := fieldPositiveEmissionSeedGroups(s, samples, positiveCount)
		if len(groups) != 1 || groups[0].Target != positiveCount {
			t.Fatalf("density=%d unexpected plane groups: %+v", density, groups)
		}
		plus, minus := 0, 0
		for i := 0; i < groups[0].Target; i++ {
			side := fieldPlaneSeedSide(p, groups[0].Seeds[i])
			if side > 0 {
				plus++
			} else if side < 0 {
				minus++
			}
		}
		if plus == 0 || minus == 0 || int(math.Abs(float64(plus-minus))) > 1 {
			t.Fatalf("density=%d first emission round is one-sided: +face=%d -face=%d", density, plus, minus)
		}
	}
}

func TestUniformInsulatingPlaneAcceptedLinesAppearOnBothFaces135(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.Display = fieldDisplayElectricLines
	s.Density = 4
	s.RotZ = -5 * math.Pi / 180
	s.ViewDistance = .5
	s.precise3DLines = true
	p := fieldDefaultObject(fieldObjectPlane)
	p.Material = fieldMaterialInsulator
	p.Charge = 1
	p.Size = 1.25
	s.Objects = []fieldObject{p}

	lines := fieldElectricLines(s, fieldElectricalSamples(s, 1))
	plus, minus := 0, 0
	for _, line := range lines {
		if len(line.Points) == 0 {
			continue
		}
		side := fieldPlaneSeedSide(p, line.Points[0])
		if side > 0 {
			plus++
		} else if side < 0 {
			minus++
		}
	}
	if plus == 0 || minus == 0 {
		t.Fatalf("density=4 accepted electric lines are one-sided: +face=%d -face=%d total=%d", plus, minus, len(lines))
	}

	projector := fieldProjectorForScene(s, 755, 780)
	selected := fieldSelectScreenDiverseLineSet("electric", projector, lines)
	plus, minus = 0, 0
	for _, line := range selected {
		if len(line.Points) == 0 {
			continue
		}
		if fieldPlaneSeedSide(p, line.Points[0]) > 0 {
			plus++
		} else {
			minus++
		}
	}
	if plus == 0 || minus == 0 {
		t.Fatalf("screen-diversity pass removed an entire plane face: +face=%d -face=%d selected=%d", plus, minus, len(selected))
	}
}
