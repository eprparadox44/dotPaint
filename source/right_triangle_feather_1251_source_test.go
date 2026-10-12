package main

import (
	"os"
	"strings"
	"testing"
)

func Test1251TriangleThickTanUsesStrictTanShaftSource(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	start := strings.Index(s, "func (a *App) drawRasterRightTriangleAt(")
	if start < 0 {
		t.Fatal("triangle raster function missing")
	}
	end := strings.Index(s[start:], "func (a *App) drawRasterRightTriangle(")
	if end < 0 {
		t.Fatal("triangle raster function end missing")
	}
	body := s[start : start+end]
	for _, part := range []string{
		"a.constrainedStrictTanGeometry(x0, y0, x1, y1)",
		"c.DrawStyledStrictTanPath(translatePixelPath(points, -originX, -originY), step.X, step.Y, majorSteps, a.rightTriangleSize, false, col)",
		"c.DrawStyledLine(x0-originX, y0-originY, cornerX-originX, cornerY-originY, a.rightTriangleSize, false, col)",
		"c.DrawStyledLine(cornerX-originX, cornerY-originY, nx-originX, ny-originY, a.rightTriangleSize, false, col)",
	} {
		if !strings.Contains(body, part) {
			t.Fatalf("triangle renderer missing %q", part)
		}
	}
	if strings.Contains(body, "DrawStyledPixelPath(") {
		t.Fatal("legacy feather-producing tan pixel path still in triangle")
	}
	preview := strings.Index(s, "func (a *App) paintRasterRightTrianglePreview(")
	if preview < 0 || !strings.Contains(s[preview:], "a.drawRasterRightTriangleAt(preview,") {
		t.Fatal("preview no longer shares triangle renderer")
	}
}

// The portable strict-tan cap is the one now shared between line and triangle.
// Confirm that it rejects the excess end-cap contributions of short segments.
func Test1251GlobalTanCapRejectsEndFeathers(t *testing.T) {
	for _, tc := range []struct {
		name                string
		sx, sy, steps, size int
	}{
		{"shallow", 3, 1, 120, 40}, {"steep", 1, -5, 120, 40}, {"diagonal", -1, -1, 60, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, ok := calculateStrictTanShadowSegment(100, 100, tc.sx, tc.sy, tc.steps, tc.size)
			if !ok {
				t.Fatal("shadow calculation failed")
			}
			// A sample beyond the global end cap must not survive.
			x := s.X1 + s.UnitX*2
			y := s.Y1 + s.UnitY*2
			if s.containsRasterCentre(x, y) {
				t.Fatalf("cap accepts feather sample beyond end at %.3f %.3f", x, y)
			}
			midx := (s.X0 + s.X1) * 0.5
			midy := (s.Y0 + s.Y1) * 0.5
			if !s.containsRasterCentre(midx, midy) {
				t.Fatalf("global cap rejects stroke midpoint")
			}
		})
	}
}
