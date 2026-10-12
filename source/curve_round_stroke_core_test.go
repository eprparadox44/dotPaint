package main

import (
	"math"
	"testing"
)

func curveNibOpaqueSet(size int) map[[2]int]bool {
	out := map[[2]int]bool{}
	for _, cell := range buildCurveRoundNib(size, false) {
		if cell.Coverage >= 1 {
			out[[2]int{cell.DX, cell.DY}] = true
		}
	}
	return out
}

func TestCurveRoundNibKeepsNominalDiameter(t *testing.T) {
	for size := 1; size <= 40; size++ {
		pixels := curveNibOpaqueSet(size)
		row, col := 0, 0
		for p := range pixels {
			if p[1] == 0 {
				row++
			}
			if p[0] == 0 {
				col++
			}
		}
		if row != size || col != size {
			t.Fatalf("size=%d central diameter row=%d col=%d want=%d", size, row, col, size)
		}
	}
}

func TestCurveRoundNibHasNoSquareCorners(t *testing.T) {
	for _, size := range []int{4, 12, 24, 40} {
		pixels := curveNibOpaqueSet(size)
		offset := curveRoundNibCentreOffset(size)
		radius := curveRoundNibRadius(size)
		for p := range pixels {
			dx := float64(p[0]) - offset
			dy := float64(p[1]) - offset
			if math.Hypot(dx, dy) > radius+1e-9 {
				t.Fatalf("size=%d square-corner spike at %v distance=%g radius=%g", size, p, math.Hypot(dx, dy), radius)
			}
		}
	}
}

func TestCurveRoundNibAntialiasAddsOnlyOnePixelFringe(t *testing.T) {
	for _, size := range []int{1, 2, 5, 12, 40} {
		offset := curveRoundNibCentreOffset(size)
		radius := curveRoundNibRadius(size)
		for _, cell := range buildCurveRoundNib(size, true) {
			d := math.Hypot(float64(cell.DX)-offset, float64(cell.DY)-offset)
			if d > radius+math.Sqrt2/2+1e-9 {
				t.Fatalf("size=%d AA fringe too far cell=%+v distance=%g radius=%g", size, cell, d, radius)
			}
		}
	}
}
