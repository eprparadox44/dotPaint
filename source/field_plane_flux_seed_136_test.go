package main

import (
	"math"
	"testing"
)

func fieldPlaneLocalUV(o fieldObject, p fieldVec3) (float64, float64) {
	ex, _, ez := fieldObjectBasis(o)
	d := p.sub(fieldObjectCenter(o))
	return d.dot(ex), d.dot(ez)
}

func TestPlaneFirstRetryRoundSpansFullSurface136(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.Display = fieldDisplayElectricLines
	s.Density = 10
	p := fieldDefaultObject(fieldObjectPlane)
	p.Material = fieldMaterialInsulator
	p.Charge = 1
	p.Size = 1.25
	s.Objects = []fieldObject{p}

	samples := fieldElectricalSamples(s, 1)
	positiveCount, _, _ := fieldFluxLineCounts(samples, s.Density)
	groups := fieldPositiveEmissionSeedGroups(s, samples, positiveCount)
	if len(groups) != 1 || len(groups[0].Seeds) < groups[0].Target {
		t.Fatalf("unexpected plane seed group: %+v", groups)
	}

	minV, maxV := math.Inf(1), math.Inf(-1)
	sumU, sumV := 0.0, 0.0
	for i := 0; i < groups[0].Target; i++ {
		u, v := fieldPlaneLocalUV(p, groups[0].Seeds[i])
		minV = math.Min(minV, v)
		maxV = math.Max(maxV, v)
		sumU += u
		sumV += v
	}
	if minV >= -.20 || maxV <= .20 {
		t.Fatalf("first retry round is vertically biased: minV=%.4f maxV=%.4f", minV, maxV)
	}
	meanU := sumU / float64(groups[0].Target)
	meanV := sumV / float64(groups[0].Target)
	if math.Abs(meanU) > .18 || math.Abs(meanV) > .18 {
		t.Fatalf("first retry round centroid is biased: meanU=%.4f meanV=%.4f", meanU, meanV)
	}
}

func fieldParallelPlateScene136(size, separation float64) (fieldScene, fieldObject) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	s.Display = fieldDisplayElectricLines
	s.Density = 20
	positive := fieldDefaultObject(fieldObjectPlane)
	positive.Material = fieldMaterialInsulator
	positive.Charge = 1
	positive.Size = size
	positive.X = -separation * .5
	negative := positive
	negative.Charge = -1
	negative.X = separation * .5
	s.Objects = []fieldObject{positive, negative}
	return s, positive
}

func TestParallelPlateSeedsFavorGapFace136(t *testing.T) {
	s, positive := fieldParallelPlateScene136(2.5, .4)
	samples := fieldElectricalSamples(s, 1)
	positiveCount, _, _ := fieldFluxLineCounts(samples, s.Density)
	groups := fieldPositiveEmissionSeedGroups(s, samples, positiveCount)
	if len(groups) == 0 {
		t.Fatal("missing positive plate emission group")
	}
	ex, _, ez := fieldObjectBasis(positive)
	n := ex.cross(ez).norm() // +X points into the gap for this setup.
	center := fieldObjectCenter(positive)
	inner, outer := 0, 0
	for i := 0; i < groups[0].Target && i < len(groups[0].Seeds); i++ {
		if groups[0].Seeds[i].sub(center).dot(n) > 0 {
			inner++
		} else {
			outer++
		}
	}
	if inner <= outer*2 {
		t.Fatalf("parallel-plate seed flux is still too exterior-heavy: inner=%d outer=%d", inner, outer)
	}
}

func fieldPositivePlateOuterFluxFraction136(size, separation float64) float64 {
	s, positive := fieldParallelPlateScene136(size, separation)
	candidates := fieldFluxSeedCandidates(s, fieldElectricalSamples(s, 1))
	ex, _, ez := fieldObjectBasis(positive)
	n := ex.cross(ez).norm()
	inner, outer := 0.0, 0.0
	for _, c := range candidates {
		if c.Object != 0 {
			continue
		}
		if c.Normal.dot(n) > 0 {
			inner += c.Weight
		} else {
			outer += c.Weight
		}
	}
	if inner+outer <= 0 {
		return 0
	}
	return outer / (inner + outer)
}

func TestParallelPlateFringingFluxGrowsWithSeparation136(t *testing.T) {
	closeFraction := fieldPositivePlateOuterFluxFraction136(2.5, .2)
	farFraction := fieldPositivePlateOuterFluxFraction136(2.5, 1.2)
	if !(closeFraction < farFraction && closeFraction < .30 && farFraction > .30) {
		t.Fatalf("unexpected finite-plate fringe fractions: close=%.4f far=%.4f", closeFraction, farFraction)
	}
}
