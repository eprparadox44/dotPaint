package main

import (
	"math"
	"testing"
)

type fieldBenchmarkSegment struct {
	mid, dl fieldVec3
	current float64
	soft2   float64
}

func BenchmarkFieldSyntheticMagneticDirect320Segments(b *testing.B) {
	segments := make([]fieldBenchmarkSegment, 320)
	for i := range segments {
		t := float64(i) * 0.019
		segments[i] = fieldBenchmarkSegment{
			mid:     fieldVec3{X: 0.7 * math.Cos(t), Y: 0.4 * math.Sin(t), Z: 0.5 * math.Sin(2*t)},
			dl:      fieldVec3{X: -0.02 * math.Sin(t), Y: 0.02 * math.Cos(t), Z: 0.01},
			current: 1,
			soft2:   0.0025,
		}
	}
	p := fieldVec3{X: 0.31, Y: -0.17, Z: 0.23}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out fieldVec3
		for _, segment := range segments {
			r := fieldSub(p, segment.mid)
			r2 := fieldDot(r, r) + segment.soft2
			out = fieldAdd(out, fieldMul(fieldCross(segment.dl, r), segment.current/(r2*math.Sqrt(r2))))
		}
		fieldBenchmarkSink = out
	}
}

func BenchmarkFieldVectorGridTrilinearSample52Cube(b *testing.B) {
	grid := fieldVectorGrid{NX: 52, NY: 52, NZ: 52, Min: fieldVec3{X: -2, Y: -2, Z: -2}, Max: fieldVec3{X: 2, Y: 2, Z: 2}}
	grid.Values = make([]fieldVec3, grid.NX*grid.NY*grid.NZ)
	for iz := 0; iz < grid.NZ; iz++ {
		for iy := 0; iy < grid.NY; iy++ {
			for ix := 0; ix < grid.NX; ix++ {
				grid.Values[grid.index(ix, iy, iz)] = fieldVec3{X: float64(ix), Y: float64(iy), Z: float64(iz)}
			}
		}
	}
	p := fieldVec3{X: 0.31, Y: -0.17, Z: 0.23}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fieldBenchmarkSink, _ = grid.sample(p)
	}
}

var fieldBenchmarkSink fieldVec3
