package main

import (
	"bytes"
	"testing"
)

// Regression: the triangle used its old un-clipped microsegment renderer,
// while ToolLine already clipped all micro-caps against one global tan vector.
// Compare actual triangle drawing with the common strict-tan shaft plus the
// two unchanged axis-aligned legs. Both preview and final commit share this.
func Test1251RightTriangleTanHypotenuseUsesLineRenderer(t *testing.T) {
	white, black := rgb(255, 255, 255), rgb(0, 0, 0)
	for _, tc := range []struct {
		name                 string
		x0, y0, x1, y1, size int
	}{
		{"rising-shallow-thick", 66, 60, 226, 105, 40},
		{"falling-shallow-thick", 230, 72, 72, 130, 40},
		{"rising-steep-thick", 45, 45, 92, 224, 39},
		{"falling-steep-thick", 105, 220, 145, 55, 40},
		{"diagonal-thick", 55, 55, 135, 130, 20},
		{"shallow-one-pixel", 80, 70, 200, 98, 1},
		{"shallow-even", 75, 85, 220, 115, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, want := NewCanvas(300, 300, white), NewCanvas(300, 300, white)
			a := &App{canvas: got, rightTriangleSize: tc.size, lineConstraintMode: LineConstraintTanN}
			nx, ny := a.drawRasterRightTriangleAt(got, tc.x0, tc.y0, tc.x1, tc.y1, true, black, 0, 0)
			points, ex, ey, step, majorSteps := a.constrainedStrictTanGeometry(tc.x0, tc.y0, tc.x1, tc.y1)
			if nx != ex || ny != ey {
				t.Fatalf("endpoint got=(%d,%d) expected=(%d,%d)", nx, ny, ex, ey)
			}
			want.DrawStyledStrictTanPath(points, step.X, step.Y, majorSteps, tc.size, false, black)
			cx, cy := rightTriangleLegCorner(tc.x0, tc.y0, nx, ny)
			want.DrawStyledLine(tc.x0, tc.y0, cx, cy, tc.size, false, black)
			want.DrawStyledLine(cx, cy, nx, ny, tc.size, false, black)
			if !bytes.Equal(got.Pix, want.Pix) {
				count := 0
				for i := range got.Pix {
					if got.Pix[i] != want.Pix[i] {
						count++
					}
				}
				t.Fatalf("triangle differs from strict-tan line+legs (%d different bytes)", count)
			}
		})
	}
}

func Test1251RightTrianglePreviewRegionEqualsFullCanvas(t *testing.T) {
	white, black := rgb(255, 255, 255), rgb(0, 0, 0)
	full := NewCanvas(400, 350, white)
	a := &App{canvas: full, rightTriangleSize: 40, lineConstraintMode: LineConstraintTanN}
	x0, y0, x1, y1 := 180, 110, 318, 180
	nx, ny := a.drawRasterRightTriangleAt(full, x0, y0, x1, y1, true, black, 0, 0)
	if nx == x0 && ny == y0 {
		t.Fatal("degenerate test setup")
	}
	left, top, right, bottom := a.rightTrianglePreviewCanvasBounds(x1, y1, true)
	if left > right || top > bottom {
		t.Fatal("invalid preview region")
	}
	local := NewCanvas(right-left+1, bottom-top+1, white)
	a.drawRasterRightTriangleAt(local, x0, y0, x1, y1, true, black, left, top)
	for y := top; y <= bottom; y++ {
		for x := left; x <= right; x++ {
			iFull := (y*full.W + x) * 4
			iLocal := ((y-top)*local.W + x - left) * 4
			if !bytes.Equal(full.Pix[iFull:iFull+4], local.Pix[iLocal:iLocal+4]) {
				t.Fatalf("preview/commit pixel mismatch at (%d,%d)", x, y)
			}
		}
	}
}
