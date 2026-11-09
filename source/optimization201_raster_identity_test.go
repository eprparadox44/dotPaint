package main

import (
	"bytes"
	"math/rand"
	"testing"
)

func TestDotPaint201UnifiedArrowRasterMatchesOriginal(t *testing.T) {
	rng := rand.New(rand.NewSource(20261108))
	for k := 0; k < 140; k++ {
		x0, y0 := rng.Intn(80)+35, rng.Intn(80)+35
		x1, y1 := rng.Intn(130)+20, rng.Intn(130)+20
		size := 1 + rng.Intn(40)
		head, ok := calculateArrowHead(x0, y0, x1, y1, rng.Intn(4), size)
		if !ok {
			continue
		}
		aa, headAA := k%2 == 0, k%3 != 0
		c1 := NewCanvas(190, 190, 0x5d6779)
		c2 := NewCanvas(190, 190, 0x5d6779)
		drawUnifiedVectorArrowReference201(c1, x0, y0, x1, y1, size, aa, head, headAA, nil, nil, 0x219aca)
		c2.DrawUnifiedVectorArrow(x0, y0, x1, y1, size, aa, head, headAA, nil, nil, 0x219aca)
		if !bytes.Equal(c1.Pix, c2.Pix) {
			for i := range c1.Pix {
				if c1.Pix[i] != c2.Pix[i] {
					t.Fatalf("arrow raster diff k=%d byte=%d AA=%v/%v size=%d", k, i, aa, headAA, size)
				}
			}
		}
	}
}
func TestDotPaint201TanRasterMatchesOriginal(t *testing.T) {
	rng := rand.New(rand.NewSource(20261108))
	for k := 0; k < 140; k++ {
		x0, y0 := rng.Intn(60)+40, rng.Intn(60)+40
		dx, dy := rng.Intn(81)-40, rng.Intn(81)-40
		step := nearestStrictTanStep(float64(dx), float64(dy))
		major := 1 + rng.Intn(60)
		path := strictTanPixelPrefix(x0, y0, step, major)
		size := 1 + rng.Intn(40)
		offx, offy := arrowStrokeOffset(x0, y0, x0+step.X, y0+step.Y, size)
		color := uint32(0x219aca)
		aa := k%2 == 0
		c1 := NewCanvas(190, 190, 0x5d6779)
		c2 := NewCanvas(190, 190, 0x5d6779)
		drawArrowShaftStrictTanPixelPathReference201(c1, path, step.X, step.Y, major, size, aa, offx, offy, color)
		c2.DrawArrowShaftStrictTanPixelPath(path, step.X, step.Y, major, size, aa, offx, offy, color)
		if !bytes.Equal(c1.Pix, c2.Pix) {
			for i := range c1.Pix {
				if c1.Pix[i] != c2.Pix[i] {
					t.Fatalf("tan raster differs k=%d byte=%d", k, i)
				}
			}
		}
	}
}
