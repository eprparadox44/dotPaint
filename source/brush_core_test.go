package main

import "testing"

func TestDiagonalBrushBandMinimum(t *testing.T) {
	for size := 1; size <= 8; size++ {
		if got := diagonalBrushBand(size); got != 1 {
			t.Fatalf("size %d band=%d, want 1", size, got)
		}
	}
}

func TestDiagonalBrushBandGrowth(t *testing.T) {
	cases := map[int]int{9: 1, 16: 1, 17: 2, 24: 2, 25: 3, 40: 4}
	for size, want := range cases {
		if got := diagonalBrushBand(size); got != want {
			t.Fatalf("size %d band=%d, want %d", size, got, want)
		}
	}
}

func TestDiagonalBrushFootprintMinimum(t *testing.T) {
	if got := diagonalBrushFootprintSize(1); got != 2 {
		t.Fatalf("size 1 footprint=%d, want 2", got)
	}
	for _, size := range []int{2, 8, 9, 40} {
		if got := diagonalBrushFootprintSize(size); got != size {
			t.Fatalf("size %d footprint=%d", size, got)
		}
	}
}
