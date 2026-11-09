package main

import (
	"math"
	"math/rand"
	"testing"
)

func TestDotPaint201CachedShaftMatchesOriginalPixelForPixel(t *testing.T) {
	r := rand.New(rand.NewSource(20261108))
	for k := 0; k < 1800; k++ {
		x0, y0 := float64(r.Intn(75)-30), float64(r.Intn(75)-30)
		x1, y1 := x0+float64(r.Intn(5)-2), y0+float64(r.Intn(5)-2)
		size := 1 + r.Intn(40)
		if k%2 == 0 {
			x0 += .5
			y0 -= .5
			x1 += .5
			y1 -= .5
		}
		f := arrowPrepareStroke(x0, y0, x1, y1, size)
		for j := 0; j < 70; j++ {
			px, py := float64(r.Intn(90)-36), float64(r.Intn(90)-36)
			if a, b := pointInsideArrowStroke(px, py, x0, y0, x1, y1, size), f.contains(px, py); a != b {
				t.Fatalf("shaft mismatch: k=%d p=(%g,%g) f=%+v", k, px, py, f)
			}
		}
	}
}
func TestDotPaint201CachedArrowCompositeMatchesOriginal(t *testing.T) {
	r := rand.New(rand.NewSource(20261108))
	for k := 0; k < 100; k++ {
		x0, y0 := r.Intn(25)+10, r.Intn(25)+10
		x1, y1 := x0+r.Intn(100)-50, y0+r.Intn(100)-50
		size := 1 + r.Intn(22)
		head, ok := calculateArrowHead(x0, y0, x1, y1, r.Intn(4), size)
		if !ok {
			continue
		}
		fx0, fy0 := float64(x0)+head.PixelOffsetX, float64(y0)+head.PixelOffsetY
		fx1, fy1 := float64(x1)+head.PixelOffsetX, float64(y1)+head.PixelOffsetY
		f := arrowPrepareStroke(fx0, fy0, fx1, fy1, size)
		for _, shaftAA := range []bool{false, true} {
			for _, headAA := range []bool{false, true} {
				edges := arrowHeadEdgeAntialias(head, headAA)
				for j := 0; j < 42; j++ {
					px, py := float64(x0+r.Intn(95)-45), float64(y0+r.Intn(95)-45)
					a := arrowCompositeCoverageAt(px, py, true, fx0, fy0, fx1, fy1, size, shaftAA, head, headAA)
					b := arrowCompositeCoveragePrepared(px, py, true, f, shaftAA, &head, headAA, edges)
					if math.Float64bits(a) != math.Float64bits(b) {
						t.Fatalf("arrow mismatch k=%d AA=%t/%t p=(%g,%g) want=%g got=%g", k, shaftAA, headAA, px, py, a, b)
					}
				}
			}
		}
	}
}
func TestDotPaint201CachedStrictTanAAEqualsOriginal(t *testing.T) {
	r := rand.New(rand.NewSource(20261108))
	for k := 0; k < 200; k++ {
		x0, y0 := float64(r.Intn(40))+0.5, float64(r.Intn(40))-.5
		x1, y1 := x0+float64(r.Intn(260)-130), y0+float64(r.Intn(260)-130)
		f := arrowPrepareStroke(x0, y0, x1, y1, 1+r.Intn(40))
		size := int(f.half * 2)
		for j := 0; j < 20; j++ {
			px, py := float64(r.Intn(160)-50), float64(r.Intn(160)-50)
			a := arrowStrokeLongEdgesCoverageAt(px, py, x0, y0, x1, y1, size)
			b := arrowStrokeLongEdgesCoveragePrepared(px, py, f)
			if math.Float64bits(a) != math.Float64bits(b) {
				t.Fatalf("AA mismatch k=%d p=(%g,%g) a=%g b=%g", k, px, py, a, b)
			}
		}
	}
}
