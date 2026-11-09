package main

import "math"

func drawUnifiedVectorArrowReference201(c *Canvas, x0, y0, x1, y1, size int, shaftAA bool, head ArrowHeadGeometry, headAA bool, strictPath []PixelPoint, strictShadow *strictTanShadowSegment, clr uint32) {
	size = clamp(size, 1, 40)
	hasShaft := x0 != x1 || y0 != y1
	fx0, fy0 := float64(x0)+head.PixelOffsetX, float64(y0)+head.PixelOffsetY
	fx1, fy1 := float64(x1)+head.PixelOffsetX, float64(y1)+head.PixelOffsetY
	if strictShadow != nil && strictShadow.Length > 0 {
		hasShaft = true
		fx0, fy0 = strictShadow.X0, strictShadow.Y0
		fx1, fy1 = strictShadow.X1, strictShadow.Y1
	}
	halfWidth := float64(size) / 2.0
	minX := math.Min(head.Tip.X-head.TipHalfWidth, math.Min(head.SideA.X, head.SideB.X))
	maxX := math.Max(head.Tip.X+head.TipHalfWidth, math.Max(head.SideA.X, head.SideB.X))
	minY := math.Min(head.Tip.Y-head.TipHalfWidth, math.Min(head.SideA.Y, head.SideB.Y))
	maxY := math.Max(head.Tip.Y+head.TipHalfWidth, math.Max(head.SideA.Y, head.SideB.Y))
	if hasShaft {
		minX = math.Min(minX, math.Min(fx0, fx1)-halfWidth-1)
		maxX = math.Max(maxX, math.Max(fx0, fx1)+halfWidth+1)
		minY = math.Min(minY, math.Min(fy0, fy1)-halfWidth-1)
		maxY = math.Max(maxY, math.Max(fy0, fy1)+halfWidth+1)
	}
	left := clamp(int(math.Floor(minX))-1, 0, c.W-1)
	right := clamp(int(math.Ceil(maxX))+1, 0, c.W-1)
	top := clamp(int(math.Floor(minY))-1, 0, c.H-1)
	bottom := clamp(int(math.Ceil(maxY))+1, 0, c.H-1)
	if left > right || top > bottom {
		return
	}
	mask := newVectorAlphaMask(left, top, right, bottom)
	candidates := newVectorCoverageCandidates(left, top, right, bottom)
	if hasShaft {
		candidates.markStroke(fx0, fy0, fx1, fy1, size)
	}
	headLeft := int(math.Floor(math.Min(head.Tip.X-head.TipHalfWidth, math.Min(head.SideA.X, head.SideB.X)))) - 1
	headRight := int(math.Ceil(math.Max(head.Tip.X+head.TipHalfWidth, math.Max(head.SideA.X, head.SideB.X)))) + 1
	headTop := int(math.Floor(math.Min(head.Tip.Y-head.TipHalfWidth, math.Min(head.SideA.Y, head.SideB.Y)))) - 1
	headBottom := int(math.Ceil(math.Max(head.Tip.Y+head.TipHalfWidth, math.Max(head.SideA.Y, head.SideB.Y)))) + 1
	candidates.markRect(headLeft, headTop, headRight, headBottom)
	candidates.forEach(func(x, y int) {
		cov := arrowCompositeCoverageAt(float64(x), float64(y), hasShaft, fx0, fy0, fx1, fy1, size, shaftAA, head, headAA)
		mask.setMaxCoverage(x, y, cov)
	})

	// Strict tan mode contributes its exact integer staircase to the same mask.
	if len(strictPath) > 1 {
		margin := int(math.Ceil(halfWidth + math.Max(math.Abs(head.PixelOffsetX), math.Abs(head.PixelOffsetY)) + 1.5))
		for i := 1; i < len(strictPath); i++ {
			p0, p1 := strictPath[i-1], strictPath[i]
			sx0, sy0 := float64(p0.X)+head.PixelOffsetX, float64(p0.Y)+head.PixelOffsetY
			sx1, sy1 := float64(p1.X)+head.PixelOffsetX, float64(p1.Y)+head.PixelOffsetY
			segLeft := clamp(min(p0.X, p1.X)-margin, left, right)
			segRight := clamp(max(p0.X, p1.X)+margin, left, right)
			segTop := clamp(min(p0.Y, p1.Y)-margin, top, bottom)
			segBottom := clamp(max(p0.Y, p1.Y)+margin, top, bottom)
			for y := segTop; y <= segBottom; y++ {
				for x := segLeft; x <= segRight; x++ {
					if pointInsideArrowStroke(float64(x), float64(y), sx0, sy0, sx1, sy1, size) {
						mask.setOpaque(x, y)
					}
				}
			}
		}
	}
	c.applyVectorAlphaMask(mask, clr)
}

func drawArrowShaftStrictTanPixelPathReference201(c *Canvas, points []PixelPoint, stepX, stepY, majorSteps, size int, antialias bool, offsetX, offsetY float64, clr uint32) {
	if len(points) == 0 {
		return
	}
	if len(points) == 1 {
		c.DrawArrowShaftPixelPath(points, size, antialias, offsetX, offsetY, clr)
		return
	}

	size = clamp(size, 1, 40)
	halfWidth := float64(size) / 2.0
	margin := int(math.Ceil(halfWidth + math.Max(math.Abs(offsetX), math.Abs(offsetY)) + 1.5))
	minX, maxX := points[0].X, points[0].X
	minY, maxY := points[0].Y, points[0].Y
	for _, p := range points[1:] {
		minX, maxX = min(minX, p.X), max(maxX, p.X)
		minY, maxY = min(minY, p.Y), max(maxY, p.Y)
	}
	left := clamp(minX-margin, 0, c.W-1)
	right := clamp(maxX+margin, 0, c.W-1)
	top := clamp(minY-margin, 0, c.H-1)
	bottom := clamp(maxY+margin, 0, c.H-1)
	mask := newVectorAlphaMask(left, top, right, bottom)

	// Invisible ideal vector fringe. A97 derives it from the selected reduced
	// slope and dominant-axis length, not from the staircase's current last
	// pixel. Its fixed motif-centred phase balances AA on the two sides and does
	// not drift when the line grows by another native pixel.
	first := points[0]
	shadow, shadowOK := calculateStrictTanShadowSegment(first.X, first.Y, stepX, stepY, majorSteps, size)
	if antialias && shadowOK {
		candidates := newVectorCoverageCandidates(left, top, right, bottom)
		candidates.markStroke(shadow.X0, shadow.Y0, shadow.X1, shadow.Y1, size)
		candidates.forEach(func(x, y int) {
			mask.setMaxCoverage(x, y, arrowStrokeLongEdgesCoverageAt(float64(x), float64(y), shadow.X0, shadow.Y0, shadow.X1, shadow.Y1, size))
		})
	}

	// Exact integer staircase is unioned as the fully opaque core before the
	// mask is composited, rather than painted in a separate pass.
	for i := 1; i < len(points); i++ {
		p0, p1 := points[i-1], points[i]
		sx0, sy0 := float64(p0.X)+offsetX, float64(p0.Y)+offsetY
		sx1, sy1 := float64(p1.X)+offsetX, float64(p1.Y)+offsetY
		segLeft := clamp(min(p0.X, p1.X)-margin, left, right)
		segRight := clamp(max(p0.X, p1.X)+margin, left, right)
		segTop := clamp(min(p0.Y, p1.Y)-margin, top, bottom)
		segBottom := clamp(max(p0.Y, p1.Y)+margin, top, bottom)
		for y := segTop; y <= segBottom; y++ {
			for x := segLeft; x <= segRight; x++ {
				if shadowOK && !shadow.containsRasterCentre(float64(x), float64(y)) {
					continue
				}
				if pointInsideArrowStroke(float64(x), float64(y), sx0, sy0, sx1, sy1, size) {
					mask.setOpaque(x, y)
				}
			}
		}
	}
	c.applyVectorAlphaMask(mask, clr)
	if size == 1 && len(points) > 0 {
		// The continuous-shadow clip can reject the first raster centre for a
		// one-pixel strict tan staircase. Restore the logical origin explicitly;
		// this is visually identical to the intended exact pixel path and is used
		// by both live preview and final commit.
		c.SetPixel(points[0].X, points[0].Y, clr)
	}
}
