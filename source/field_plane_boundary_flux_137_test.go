package main

import (
	"math"
	"testing"
)

func TestUniformInsulatingPlaneExactNormalField137(t *testing.T) {
	o := fieldDefaultObject(fieldObjectPlane)
	o.Material = fieldMaterialInsulator
	o.Charge = 1
	o.Size = 2.5
	ex, _, ez := fieldObjectBasis(o)
	n := ex.cross(ez).norm()
	distance := 0.4
	p := fieldObjectCenter(o).add(n.mul(distance))
	got := fieldUniformInsulatingPlaneElectric(o, p).dot(n)

	half := o.Size * .5
	sigma := o.Charge / (o.Size * o.Size)
	expected := sigma * 4 * math.Atan((half*half)/(distance*math.Sqrt(distance*distance+2*half*half)))
	if math.Abs(got-expected) > 1e-12 {
		t.Fatalf("exact rectangle normal field mismatch: got=%.15g expected=%.15g", got, expected)
	}
}

func TestParallelPlateBoundaryFluxFractions137(t *testing.T) {
	// Continuous-square reference values for L=2.5, obtained from the exact
	// finite-rectangle field and area-averaged over the emitting plate.  The
	// tolerances include only the target-side Gauss quadrature used to place
	// deterministic seeds; the source plate itself is evaluated analytically.
	cases := []struct {
		separation float64
		expected   float64
		tolerance  float64
	}{
		{0.10, 0.0869, 0.006},
		{0.20, 0.1423, 0.006},
		{0.40, 0.2179, 0.006},
		{0.80, 0.3104, 0.006},
		{1.20, 0.3654, 0.006},
	}
	for _, tc := range cases {
		got := fieldPositivePlateOuterFluxFraction136(2.5, tc.separation)
		if math.Abs(got-tc.expected) > tc.tolerance {
			t.Fatalf("separation %.2f: outer flux got %.6f want %.6f ± %.6f", tc.separation, got, tc.expected, tc.tolerance)
		}
	}
}

func TestCloseParallelPlateDoesNotDependOnSeedClearance137(t *testing.T) {
	// 1.2.36 sampled E at the 0.095 seed clearance.  At d=0.10 that probe sat
	// almost on the opposite plate and collapsed the exterior fraction to about
	// 0.5%%.  The surface boundary condition should instead retain the finite-edge
	// fringing flux, about 8.7%% for this geometry.
	got := fieldPositivePlateOuterFluxFraction136(2.5, 0.10)
	if got < 0.07 || got > 0.11 {
		t.Fatalf("close-gap boundary flux still probe-biased: outer=%.6f", got)
	}
}

func TestIsolatedUniformPlaneRemainsHalfHalf137(t *testing.T) {
	s := fieldDefaultScene()
	s.Dimension = fieldDimension3D
	p := fieldDefaultObject(fieldObjectPlane)
	p.Material = fieldMaterialInsulator
	p.Charge = 1
	p.Size = 2.5
	s.Objects = []fieldObject{p}
	candidates := fieldFluxSeedCandidates(s, fieldElectricalSamples(s, 1))
	ex, _, ez := fieldObjectBasis(p)
	n := ex.cross(ez).norm()
	plus, minus := 0.0, 0.0
	for _, c := range candidates {
		if c.Object != 0 {
			continue
		}
		if c.Normal.dot(n) >= 0 {
			plus += c.Weight
		} else {
			minus += c.Weight
		}
	}
	if plus+minus <= 0 {
		t.Fatal("isolated plane produced no seed flux")
	}
	frac := plus / (plus + minus)
	if math.Abs(frac-.5) > 1e-10 {
		t.Fatalf("isolated sheet lost 50/50 symmetry: plus fraction=%.12f", frac)
	}
}

func TestParallelPlateDisplayedSeedCounts137(t *testing.T) {
	cases := []struct {
		separation float64
		wantInner  int
		wantOuter  int
	}{
		{0.10, 18, 2},
		{0.40, 16, 4},
	}
	for _, tc := range cases {
		s, positive := fieldParallelPlateScene136(2.5, tc.separation)
		samples := fieldElectricalSamples(s, 1)
		positiveCount, _, _ := fieldFluxLineCounts(samples, s.Density)
		groups := fieldPositiveEmissionSeedGroups(s, samples, positiveCount)
		if len(groups) == 0 {
			t.Fatalf("separation %.2f: missing emission group", tc.separation)
		}
		ex, _, ez := fieldObjectBasis(positive)
		n := ex.cross(ez).norm()
		center := fieldObjectCenter(positive)
		inner, outer := 0, 0
		for i := 0; i < groups[0].Target && i < len(groups[0].Seeds); i++ {
			if groups[0].Seeds[i].sub(center).dot(n) > 0 {
				inner++
			} else {
				outer++
			}
		}
		if inner != tc.wantInner || outer != tc.wantOuter {
			t.Fatalf("separation %.2f: displayed seed count inner/outer=%d/%d want %d/%d", tc.separation, inner, outer, tc.wantInner, tc.wantOuter)
		}
	}
}
