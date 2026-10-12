package main

import (
	"math"
	"testing"
)

func TestGravitySchwarzschildCalibration139(t *testing.T) {
	o := fieldDefaultObject(fieldObjectSolidMassSphere)
	o.Charge = 8
	if got := fieldGravitySchwarzschildRadius(o); math.Abs(got-.1) > 1e-12 {
		t.Fatalf("M=8 Schwarzschild radius=%g want .1", got)
	}
	cases := []struct {
		mass, radius float64
		blackHole    bool
	}{
		{9.0, .10, true},
		{9.1, .20, false},
		{8.9, .08, true},
		{8.0, .10, true},
		{7.99, .10, false},
	}
	for _, tc := range cases {
		o.Charge, o.Radius = tc.mass, tc.radius
		got := fieldGravityBlackHoleStrength(o) > 0
		if got != tc.blackHole {
			t.Fatalf("M=%g R=%g blackHole=%v want %v rs=%g", tc.mass, tc.radius, got, tc.blackHole, fieldGravitySchwarzschildRadius(o))
		}
	}
}

func TestObserverDistanceUsesInverseMagnification139(t *testing.T) {
	if got := fieldObserverMagnification(.1); math.Abs(got-10) > 1e-12 {
		t.Fatalf("distance .1 magnification=%g want 10", got)
	}
	if got := fieldObserverMagnification(2); math.Abs(got-.5) > 1e-12 {
		t.Fatalf("distance 2 magnification=%g want .5", got)
	}
	scene := fieldDefaultScene()
	scene.Dimension = fieldDimension2D
	scene.Objects = []fieldObject{fieldDefaultObject(fieldObjectSolidMassSphere)}
	offsets := func(distance float64) float64 {
		scene.ViewDistance = distance
		p := fieldProjectorForScene(scene, 400, 400)
		x, _, _, ok := p.projectFloat(fieldVec3{X: .25})
		if !ok {
			t.Fatalf("projection failed at distance %g", distance)
		}
		return x - 200
	}
	near, far := offsets(.1), offsets(2)
	if ratio := near / far; math.Abs(ratio-20) > 1e-9 {
		t.Fatalf("apparent size ratio=%g want 20", ratio)
	}
}

func fieldGravityRelativisticFieldCorrection2ndOrderBaseline(o fieldObject, p fieldVec3) fieldVec3 {
	rhat, axis, r, u, spin, _, ok := fieldGravityRelativityState(o, p)
	if !ok || o.Charge <= 0 {
		return fieldVec3{}
	}
	g0 := o.Charge / (r * r)
	cosTheta := fieldClamp(rhat.dot(axis), -1, 1)
	p2 := .5 * (3*cosTheta*cosTheta - 1)
	angular2 := 1 + .5*spin*spin*p2
	radialExtra := g0 * (2*u + 3*angular2*u*u)
	out := rhat.mul(-radialExtra)
	poleward := axis.sub(rhat.mul(cosTheta))
	out = out.add(poleward.mul(g0 * 1.5 * spin * spin * u * u * cosTheta))
	phi := axis.cross(rhat).norm()
	drag := spin * (1.20*u + 1.80*u*u)
	return out.add(phi.mul(g0 * drag))
}

func TestFourthOrderGravityCorrectionAddsDenseStarTerms139(t *testing.T) {
	o := fieldDefaultObject(fieldObjectSolidMassSphere)
	o.Charge = 8
	o.Radius = .1
	o.Spin = .99
	p := fieldVec3{X: .14, Z: .03}
	second := fieldGravityRelativisticFieldCorrection2ndOrderBaseline(o, p)
	fourth := fieldGravityRelativisticFieldCorrection(o, p)
	if fourth.len() <= second.len() {
		t.Fatalf("fourth-order correction=%g should exceed second-order=%g near compact body", fourth.len(), second.len())
	}
	if !fieldFinite(fourth.X) || !fieldFinite(fourth.Y) || !fieldFinite(fourth.Z) {
		t.Fatalf("fourth-order correction not finite: %+v", fourth)
	}
}

func BenchmarkGravityRelativity2ndOrderBaseline139(b *testing.B) {
	o := fieldDefaultObject(fieldObjectSolidMassSphere)
	o.Charge, o.Radius, o.Spin = 8, .1, .99
	p := fieldVec3{X: .21, Y: .07, Z: .04}
	b.ResetTimer()
	var v fieldVec3
	for i := 0; i < b.N; i++ {
		v = fieldGravityRelativisticFieldCorrection2ndOrderBaseline(o, p)
	}
	_ = v
}

func BenchmarkGravityRelativity4thOrder139(b *testing.B) {
	o := fieldDefaultObject(fieldObjectSolidMassSphere)
	o.Charge, o.Radius, o.Spin = 8, .1, .99
	p := fieldVec3{X: .21, Y: .07, Z: .04}
	b.ResetTimer()
	var v fieldVec3
	for i := 0; i < b.N; i++ {
		v = fieldGravityRelativisticFieldCorrection(o, p)
	}
	_ = v
}
