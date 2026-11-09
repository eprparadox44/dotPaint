package main

import (
	"math"
	"math/rand"
	"testing"
)

func fieldElectricFromSamplesReference201(samples []fieldChargeSample, p fieldVec3) fieldVec3 {
	var e fieldVec3
	for _, s := range samples {
		e = e.add(fieldElectricSampleContribution(s, p))
	}
	return e
}
func fieldElectricFixture201(n int) []fieldChargeSample {
	r := rand.New(rand.NewSource(20261108))
	a := make([]fieldChargeSample, n)
	for i := range a {
		a[i] = fieldChargeSample{Pos: fieldVec3{X: r.Float64() * 5, Y: r.Float64() * 5, Z: r.Float64() * 5}, Q: r.Float64()*4 - 2, Soft: .05 + r.Float64()*.2, Gate: i%7 == 0, GateNormal: fieldVec3{X: 0, Y: 1}, GateOffset: 2.5}
	}
	return a
}
func TestDotPaint201ElectricFromSamplesBitwiseIdentical(t *testing.T) {
	samples := fieldElectricFixture201(480)
	r := rand.New(rand.NewSource(20261108))
	for i := 0; i < 1000; i++ {
		p := fieldVec3{X: r.Float64() * 5, Y: r.Float64() * 5, Z: r.Float64() * 5}
		a, b := fieldElectricFromSamplesReference201(samples, p), fieldElectricFromSamples(samples, p)
		if math.Float64bits(a.X) != math.Float64bits(b.X) || math.Float64bits(a.Y) != math.Float64bits(b.Y) || math.Float64bits(a.Z) != math.Float64bits(b.Z) {
			t.Fatalf("electric mismatch: ref=%+v got=%+v", a, b)
		}
	}
}
func BenchmarkDotPaint201ElectricSamplesReference(b *testing.B) {
	samples := fieldElectricFixture201(480)
	p := fieldVec3{X: .1, Y: 1, Z: 2}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v := fieldElectricFromSamplesReference201(samples, p)
		if math.IsNaN(v.X) {
			b.Fatal(v)
		}
	}
}
func BenchmarkDotPaint201ElectricSamplesOptimized(b *testing.B) {
	samples := fieldElectricFixture201(480)
	p := fieldVec3{X: .1, Y: 1, Z: 2}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v := fieldElectricFromSamples(samples, p)
		if math.IsNaN(v.X) {
			b.Fatal(v)
		}
	}
}
