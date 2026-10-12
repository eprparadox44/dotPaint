package main

import (
	"math"
	"testing"
)

func TestA112StrokeCandidatesContainEveryCoveredPixel(t *testing.T) {
	cases := []struct {
		x0, y0, x1, y1 float64
		size           int
	}{
		{10, 10, 310, 110, 8},
		{310, 110, 10, 10, 8},
		{12.5, 14.5, 392.5, 37.5, 2},
		{200, 20, 205, 360, 6},
		{20, 200, 360, 205, 1},
		{20.25, 20.75, 355.25, 344.75, 7},
	}
	for _, tc := range cases {
		half := float64(tc.size)/2 + 4
		left := int(math.Floor(math.Min(tc.x0, tc.x1) - half))
		right := int(math.Ceil(math.Max(tc.x0, tc.x1) + half))
		top := int(math.Floor(math.Min(tc.y0, tc.y1) - half))
		bottom := int(math.Ceil(math.Max(tc.y0, tc.y1) + half))
		c := newVectorCoverageCandidates(left, top, right, bottom)
		c.markStroke(tc.x0, tc.y0, tc.x1, tc.y1, tc.size)
		for y := top; y <= bottom; y++ {
			for x := left; x <= right; x++ {
				cov := arrowStrokeCoverageAt(float64(x), float64(y), tc.x0, tc.y0, tc.x1, tc.y1, tc.size)
				if cov > 0 && !c.contains(x, y) {
					t.Fatalf("covered pixel omitted for %+v at %d,%d coverage=%g", tc, x, y, cov)
				}
			}
		}
	}
}

func TestA112StrictLongEdgeCandidatesContainEveryCoveredPixel(t *testing.T) {
	shadow, ok := calculateStrictTanShadowSegment(20, 30, 5, 2, 520, 8)
	if !ok {
		t.Fatal("shadow construction failed")
	}
	left, top := 0, 0
	right, bottom := 700, 350
	c := newVectorCoverageCandidates(left, top, right, bottom)
	c.markStroke(shadow.X0, shadow.Y0, shadow.X1, shadow.Y1, 8)
	for y := top; y <= bottom; y++ {
		for x := left; x <= right; x++ {
			cov := arrowStrokeLongEdgesCoverageAt(float64(x), float64(y), shadow.X0, shadow.Y0, shadow.X1, shadow.Y1, 8)
			if cov > 0 && !c.contains(x, y) {
				t.Fatalf("strict AA candidate omitted at %d,%d coverage=%g", x, y, cov)
			}
		}
	}
}

func TestA112UnifiedArrowCandidatesMatchFullCoverageDomain(t *testing.T) {
	tests := []struct {
		x0, y0, x1, y1 int
		size, head     int
	}{
		{15, 25, 515, 215, 8, 0},
		{480, 300, 30, 35, 6, 1},
		{25, 260, 530, 45, 2, 3},
	}
	for _, tc := range tests {
		head, ok := calculateArrowHead(tc.x0, tc.y0, tc.x1, tc.y1, tc.head, tc.size)
		if !ok {
			t.Fatalf("head construction failed: %+v", tc)
		}
		fx0, fy0 := float64(tc.x0)+head.PixelOffsetX, float64(tc.y0)+head.PixelOffsetY
		fx1, fy1 := float64(tc.x1)+head.PixelOffsetX, float64(tc.y1)+head.PixelOffsetY
		halfWidth := float64(tc.size) / 2
		minX := math.Min(head.Tip.X-head.TipHalfWidth, math.Min(head.SideA.X, head.SideB.X))
		maxX := math.Max(head.Tip.X+head.TipHalfWidth, math.Max(head.SideA.X, head.SideB.X))
		minY := math.Min(head.Tip.Y-head.TipHalfWidth, math.Min(head.SideA.Y, head.SideB.Y))
		maxY := math.Max(head.Tip.Y+head.TipHalfWidth, math.Max(head.SideA.Y, head.SideB.Y))
		minX = math.Min(minX, math.Min(fx0, fx1)-halfWidth-1)
		maxX = math.Max(maxX, math.Max(fx0, fx1)+halfWidth+1)
		minY = math.Min(minY, math.Min(fy0, fy1)-halfWidth-1)
		maxY = math.Max(maxY, math.Max(fy0, fy1)+halfWidth+1)
		left, right := int(math.Floor(minX))-1, int(math.Ceil(maxX))+1
		top, bottom := int(math.Floor(minY))-1, int(math.Ceil(maxY))+1
		c := newVectorCoverageCandidates(left, top, right, bottom)
		c.markStroke(fx0, fy0, fx1, fy1, tc.size)
		headLeft := int(math.Floor(math.Min(head.Tip.X-head.TipHalfWidth, math.Min(head.SideA.X, head.SideB.X)))) - 1
		headRight := int(math.Ceil(math.Max(head.Tip.X+head.TipHalfWidth, math.Max(head.SideA.X, head.SideB.X)))) + 1
		headTop := int(math.Floor(math.Min(head.Tip.Y-head.TipHalfWidth, math.Min(head.SideA.Y, head.SideB.Y)))) - 1
		headBottom := int(math.Ceil(math.Max(head.Tip.Y+head.TipHalfWidth, math.Max(head.SideA.Y, head.SideB.Y)))) + 1
		c.markRect(headLeft, headTop, headRight, headBottom)

		fullPixels := (right - left + 1) * (bottom - top + 1)
		for y := top; y <= bottom; y++ {
			for x := left; x <= right; x++ {
				cov := arrowCompositeCoverageAt(float64(x), float64(y), true, fx0, fy0, fx1, fy1, tc.size, true, head, true)
				if cov > 0 && !c.contains(x, y) {
					t.Fatalf("unified arrow candidate omitted for %+v at %d,%d coverage=%g", tc, x, y, cov)
				}
			}
		}
		if len(c.indices)*3 >= fullPixels {
			t.Fatalf("candidate reduction too small for %+v: candidates=%d full=%d", tc, len(c.indices), fullPixels)
		}
	}
}

func TestA112VectorMaskTracksTouchedCellsOnce(t *testing.T) {
	m := newVectorAlphaMask(10, 20, 30, 40)
	m.setMaxCoverage(12, 23, 0.25)
	m.setMaxCoverage(12, 23, 0.75)
	m.setOpaque(12, 23)
	m.setOpaque(18, 29)
	m.setMaxCoverage(19, 30, 0)
	if len(m.Touched) != 2 {
		t.Fatalf("expected two unique touched cells, got %d (%v)", len(m.Touched), m.Touched)
	}
	if got := m.alphaAt(12, 23); got != 255 {
		t.Fatalf("first cell alpha=%d, want 255", got)
	}
	if got := m.alphaAt(18, 29); got != 255 {
		t.Fatalf("second cell alpha=%d, want 255", got)
	}
}
