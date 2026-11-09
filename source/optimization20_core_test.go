package main

import (
	"bytes"
	"math"
	"math/rand"
	"testing"
)

// This reference reproduces the beta5.6.5 tan-N candidate enumeration order.
func nearestStrictTanStepReference20(dx, dy float64) strictTanStep {
	length := math.Hypot(dx, dy)
	if length <= 0 {
		return strictTanStep{}
	}
	ux, uy := dx/length, dy/length
	candidates := []strictTanStep{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for _, ratio := range lineTanRatios {
		for _, sx := range []int{-1, 1} {
			for _, sy := range []int{-1, 1} {
				candidates = append(candidates, strictTanStep{sx * ratio.Run, sy * ratio.Rise})
			}
		}
	}
	best, bestDot := candidates[0], -2.0
	for _, d := range candidates {
		norm := math.Hypot(float64(d.X), float64(d.Y))
		dot := ux*float64(d.X)/norm + uy*float64(d.Y)/norm
		if dot > bestDot {
			bestDot, best = dot, d
		}
	}
	return best
}

func TestDotPaint20StrictTanDirectionEqualsBeta565(t *testing.T) {
	for dx := -60; dx <= 60; dx++ {
		for dy := -60; dy <= 60; dy++ {
			want := nearestStrictTanStepReference20(float64(dx), float64(dy))
			got := nearestStrictTanStep(float64(dx), float64(dy))
			if got != want {
				t.Fatalf("tan candidate mismatch at (%d,%d): got %+v want %+v", dx, dy, got, want)
			}
		}
	}
	r := rand.New(rand.NewSource(20261108))
	for k := 0; k < 10000; k++ {
		dx, dy := (r.Float64()-.5)*10000, (r.Float64()-.5)*10000
		got, want := nearestStrictTanStep(dx, dy), nearestStrictTanStepReference20(dx, dy)
		if got != want {
			t.Fatalf("random mismatch: (%.12g,%.12g) got %+v want %+v", dx, dy, got, want)
		}
	}
}

func drawRectOuterReference20(c *Canvas, x0, y0, x1, y1, size int, clr uint32) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	x0 = clamp(x0, 0, c.W-1)
	x1 = clamp(x1, 0, c.W-1)
	y0 = clamp(y0, 0, c.H-1)
	y1 = clamp(y1, 0, c.H-1)
	w, h := x1-x0+1, y1-y0+1
	if w <= 0 || h <= 0 {
		return
	}
	size = clamp(size, 1, 40)
	thickness := min(size, min(w, h))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if x-x0 < thickness || x1-x < thickness || y-y0 < thickness || y1-y < thickness {
				c.SetPixel(x, y, clr)
			}
		}
	}
}

func TestDotPaint20RectanglePixelIdentity(t *testing.T) {
	r := rand.New(rand.NewSource(20261108))
	for k := 0; k < 1500; k++ {
		a, b := NewCanvas(87, 83, 0x204060), NewCanvas(87, 83, 0x204060)
		x0, x1, y0, y1 := r.Intn(180)-45, r.Intn(180)-45, r.Intn(160)-40, r.Intn(160)-40
		size := 1 + r.Intn(45)
		color := []uint32{0xFF8800, transparentColor, 0, 0xFFFFFF, 0x010203}[k%5]
		a.DrawRectOuter(x0, y0, x1, y1, size, color)
		drawRectOuterReference20(b, x0, y0, x1, y1, size, color)
		if !bytes.Equal(a.Pix, b.Pix) {
			t.Fatalf("rectangle mismatch trial=%d (%d,%d)-(%d,%d) size=%d", k, x0, y0, x1, y1, size)
		}
	}
}

func BenchmarkDotPaint20TanDirection(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		step := nearestStrictTanStep(float64((i*37)%127-63), float64((i*53)%131-65))
		if step.X > 100 {
			b.Fatal(step)
		}
	}
}
func BenchmarkDotPaint20LargeRect(b *testing.B) {
	c := NewCanvas(1600, 1000, 0xffffff)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.DrawRectOuter(20, 30, 1500, 900, 3, 0x123456)
	}
}
func BenchmarkDotPaint20TanLine(b *testing.B) {
	c := NewCanvas(1024, 768, 0xffffff)
	r := vectorRenderer{canvas: c, lineConstraintMode: LineConstraintTanN, lineSize: 5, lineAntialias: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.drawRasterLineAt(c, 14, 24, 900, 400, true, 0, 0, 0)
	}
}
func BenchmarkDotPaint20TanArrow(b *testing.B) {
	c := NewCanvas(1024, 768, 0xffffff)
	r := vectorRenderer{canvas: c, lineConstraintMode: LineConstraintTanN, arrowLineSize: 5, arrowHeadSize: 3, arrowAntialias: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.drawRasterArrowAt(c, 14, 24, 900, 400, true, 0, 0, 0)
	}
}

func TestDotPaint20EllipseRingPixelIdentity(t *testing.T) {
	r := rand.New(rand.NewSource(20261108))
	for k := 0; k < 2200; k++ {
		w, h := 1+r.Intn(200), 1+r.Intn(200)
		if k%15 == 0 {
			w, h = 10+r.Intn(500), 1+r.Intn(13)
		}
		if k%21 == 0 {
			w, h = 1+r.Intn(13), 10+r.Intn(500)
		}
		thickness := 1 + r.Intn(46)
		got, want := symmetricEllipseRingMask(w, h, thickness), symmetricEllipseRingMaskReference20(w, h, thickness)
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("ellipse mismatch w=%d h=%d t=%d i=%d", w, h, thickness, i)
			}
		}
	}
}
func symmetricEllipseRingMaskReference20(w, h, thickness int) []bool {
	mask := make([]bool, w*h)
	if w <= 0 || h <= 0 || thickness <= 0 {
		return mask
	}
	if thickness > w {
		thickness = w
	}
	if thickness > h {
		thickness = h
	}

	outerX := w - 1
	outerY := h - 1
	innerX := outerX - 2*thickness
	innerY := outerY - 2*thickness

	outer := symmetricFilledEllipseMask(w, h, outerX, outerY)
	flattenVertical := h > 20
	flattenHorizontal := w > 20
	flattenEllipseCardinalCaps(outer, w, h, outerX, outerY, flattenVertical, flattenHorizontal)

	var inner []bool
	if innerX > 0 && innerY > 0 {
		inner = symmetricFilledEllipseMask(w, h, innerX, innerY)
		// The user-visible shape size controls whether a cardinal direction is
		// flattened. Applying the same decision to the hole removes the matching
		// inward-facing point while retaining exact stroke symmetry.
		flattenEllipseCardinalCaps(inner, w, h, innerX, innerY, flattenVertical, flattenHorizontal)
	}

	for i, on := range outer {
		if on && (inner == nil || !inner[i]) {
			mask[i] = true
		}
	}
	bridgeExtremeEllipseGaps(mask, w, h)
	connectExtremeEllipseMicroGaps(mask, w, h)
	return mask
}
