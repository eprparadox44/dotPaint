package main

import (
	"math"
	"testing"
)

func TestFieldArcResamplingKeepsPathAndUniformSpacing130(t *testing.T) {
	points := []fieldVec3{{X: 0}, {X: .03}, {X: .27}, {X: 1}}
	got := fieldResamplePolylineByArc(points, .1)
	if len(got) < 10 || math.Abs(got[0].X) > 1e-12 || math.Abs(got[len(got)-1].X-1) > 1e-12 {
		t.Fatalf("unexpected resampling: %#v", got)
	}
	for i := 1; i < len(got)-1; i++ {
		if d := got[i].sub(got[i-1]).len(); math.Abs(d-.1) > 1e-9 {
			t.Fatalf("spacing %d=%g", i, d)
		}
	}
	if arrow := fieldArrowIndexAtArcFraction(got, .6); arrow < 5 || arrow > 7 {
		t.Fatalf("arc arrow index=%d", arrow)
	}
}

func TestFieldEqualFluxAnglesAreMonotonic130(t *testing.T) {
	last := 0.0
	for i := 0; i <= 20; i++ {
		a := fieldEqualFluxPolarAngle(float64(i)/20, 24, 74)
		if i > 0 && a <= last {
			t.Fatalf("angle not monotonic at %d: %g <= %g", i, a, last)
		}
		last = a
	}
}
