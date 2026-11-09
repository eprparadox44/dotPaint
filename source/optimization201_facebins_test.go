package main

import (
	"math/rand"
	"slices"
	"testing"
)

func mdFaceBinsOld201(bounds []mdFaceBinRange, grid int) [][]int {
	bins := make([][]int, grid*grid)
	for i, b := range bounds {
		if !b.valid {
			continue
		}
		for z := b.z0; z <= b.z1; z++ {
			for x := b.x0; x <= b.x1; x++ {
				idx := z*grid + x
				bins[idx] = append(bins[idx], i)
			}
		}
	}
	return bins
}
func mdFaceBinsSamples201(faceCount, grid int) []mdFaceBinRange {
	r := rand.New(rand.NewSource(20261108))
	out := make([]mdFaceBinRange, faceCount)
	for i := range out {
		x, z := r.Intn(grid), r.Intn(grid)
		dx, dz := r.Intn(3), r.Intn(3)
		x1, z1 := min(grid-1, x+dx), min(grid-1, z+dz)
		out[i] = mdFaceBinRange{x0: x, x1: x1, z0: z, z1: z1, valid: i%29 != 0}
	}
	return out
}
func TestDotPaint201PackedFaceBinsSameFacesAndOrder(t *testing.T) {
	for _, grid := range []int{2, 4, 8, 28, 48} {
		for _, size := range []int{0, 1, 200, 4800} {
			input := mdFaceBinsSamples201(size, grid)
			old, newbins := mdFaceBinsOld201(input, grid), mdPackProjectedFaceBins(input, grid)
			if len(old) != len(newbins) {
				t.Fatalf("grid=%d size=%d wrong grid count", grid, size)
			}
			for k := range old {
				if !slices.Equal(old[k], newbins[k]) {
					t.Fatalf("grid=%d size=%d bin=%d differs", grid, size, k)
				}
			}
		}
	}
}
func BenchmarkDotPaint201FaceBinsOld(b *testing.B) {
	v := mdFaceBinsSamples201(9600, 28)
	b.ReportAllocs()
	b.ResetTimer()
	for k := 0; k < b.N; k++ {
		out := mdFaceBinsOld201(v, 28)
		if len(out) < 100 {
			b.Fatal("empty")
		}
	}
}
func BenchmarkDotPaint201FaceBinsPacked(b *testing.B) {
	v := mdFaceBinsSamples201(9600, 28)
	b.ReportAllocs()
	b.ResetTimer()
	for k := 0; k < b.N; k++ {
		out := mdPackProjectedFaceBins(v, 28)
		if len(out) < 100 {
			b.Fatal("empty")
		}
	}
}
