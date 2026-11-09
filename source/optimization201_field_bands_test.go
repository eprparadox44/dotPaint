package main

import (
	"math"
	"math/rand"
	"testing"
)

func fieldBands201() []fieldPotentialBand {
	return []fieldPotentialBand{{Sign: 1, Low: .000001, High: 1000000, ZeroAnchored: true}, {Sign: -1, Low: .000001, High: 1000000, ZeroAnchored: true}, {Sign: 1, Low: .0001, High: 1000}, {Sign: -1, Low: .0001, High: 1000}}
}
func TestDotPaint201PreparedPotentialBandsExactlyMatchOriginal(t *testing.T) {
	rng := rand.New(rand.NewSource(20261108))
	cases := []fieldPotentialBand{{}, {Sign: 1, Low: 1, High: .5}, {Sign: 1, Low: 1, High: 20}, {Sign: -1, Low: 1, High: 20}, {Sign: 1, Low: .001, High: 123, ZeroAnchored: true}, {Sign: -1, Low: .001, High: 123, ZeroAnchored: true}}
	values := []float64{0, 1e-20, 1e-14, 1e-12, .000999, .001, 1, 20, 1000, -1000, -.01, math.NaN(), math.Inf(1)}
	for k := 0; k < 4000; k++ {
		values = append(values, (rng.Float64()-.5)*1e5)
	}
	for _, band := range cases {
		pp := fieldPreparePotentialBands([]fieldPotentialBand{band})[0]
		for _, v := range values {
			p0, ok0 := fieldPotentialBandPercent(band, v)
			p1, ok1 := fieldPreparedBandPercent(pp, v)
			if ok0 != ok1 || math.Float64bits(p0) != math.Float64bits(p1) {
				t.Fatalf("band=%+v v=%g old=(%g,%v) new=(%g,%v)", band, v, p0, ok0, p1, ok1)
			}
		}
	}
}
func BenchmarkDotPaint201FieldPotentialPercentOriginal(b *testing.B) {
	bands := fieldBands201()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		value := math.Sin(float64(i)*.123) * 250
		p, _, _ := fieldPotentialPercentForValue(bands, value)
		if p < 0 {
			b.Fatal(p)
		}
	}
}
func BenchmarkDotPaint201FieldPotentialPercentPrepared(b *testing.B) {
	bands := fieldPreparePotentialBands(fieldBands201())
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		value := math.Sin(float64(i)*.123) * 250
		p, _, _ := fieldPotentialPercentForValuePrepared(bands, value)
		if p < 0 {
			b.Fatal(p)
		}
	}
}
