package main

import "math"

type Canvas struct {
	W, H int
	Pix  []byte // BGRA, top-down
}

func NewCanvas(w, h int, c uint32) *Canvas {
	cv := &Canvas{W: w, H: h, Pix: make([]byte, w*h*4)}
	cv.Clear(c)
	return cv
}

func (c *Canvas) ClonePixels() []byte {
	p := make([]byte, len(c.Pix))
	copy(p, c.Pix)
	return p
}

func (c *Canvas) SetPixel(x, y int, clr uint32) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return
	}
	i := (y*c.W + x) * 4
	px := colorrefToBGRA(clr)
	c.Pix[i], c.Pix[i+1], c.Pix[i+2], c.Pix[i+3] = px[0], px[1], px[2], px[3]
}

func (c *Canvas) GetPixel(x, y int) uint32 {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return rgb(255, 255, 255)
	}
	i := (y*c.W + x) * 4
	if c.Pix[i+3] == 0 {
		return transparentColor
	}
	return bgraToColorref(c.Pix[i], c.Pix[i+1], c.Pix[i+2])
}

func (c *Canvas) Clear(clr uint32) {
	px := colorrefToBGRA(clr)
	for i := 0; i < len(c.Pix); i += 4 {
		c.Pix[i], c.Pix[i+1], c.Pix[i+2], c.Pix[i+3] = px[0], px[1], px[2], px[3]
	}
}

func (c *Canvas) DrawLine(x0, y0, x1, y1 int, clr uint32) {
	dx := int(math.Abs(float64(x1 - x0)))
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -int(math.Abs(float64(y1 - y0)))
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		c.SetPixel(x0, y0, clr)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// blendPixelCoverage composites one raster-line sample into the straight-alpha
// BGRA canvas. Transparent foreground acts as a partial eraser when coverage is
// below one, which lets anti-aliased transparent lines keep a soft alpha edge.
func (c *Canvas) blendPixelCoverage(x, y int, clr uint32, coverage float64) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H || coverage <= 0 {
		return
	}
	if coverage > 1 {
		coverage = 1
	}
	i := (y*c.W + x) * 4
	dstA := float64(c.Pix[i+3]) / 255.0
	if isTransparentColor(clr) {
		outA := dstA * (1.0 - coverage)
		c.Pix[i+3] = byte(math.Round(outA * 255.0))
		if c.Pix[i+3] == 0 {
			c.Pix[i], c.Pix[i+1], c.Pix[i+2] = 0, 0, 0
		}
		return
	}
	src := colorrefToBGRA(clr)
	srcA := coverage
	outA := srcA + dstA*(1.0-srcA)
	if outA <= 0 {
		c.Pix[i], c.Pix[i+1], c.Pix[i+2], c.Pix[i+3] = 0, 0, 0, 0
		return
	}
	for ch := 0; ch < 3; ch++ {
		dst := float64(c.Pix[i+ch])
		s := float64(src[ch])
		out := (s*srcA + dst*dstA*(1.0-srcA)) / outA
		c.Pix[i+ch] = byte(clamp(int(math.Round(out)), 0, 255))
	}
	c.Pix[i+3] = byte(clamp(int(math.Round(outA*255.0)), 0, 255))
}

func pointSegmentDistance(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	denom := dx*dx + dy*dy
	if denom <= 0 {
		return math.Hypot(px-x0, py-y0)
	}
	t := ((px-x0)*dx + (py-y0)*dy) / denom
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	qx, qy := x0+t*dx, y0+t*dy
	return math.Hypot(px-qx, py-qy)
}

// applyVectorAlphaMask composites an invisible vector contribution mask once.
// Component geometry is unioned in the mask first, avoiding overlap darkening
// and making preview and commit use the exact same alpha values.
func (c *Canvas) applyVectorAlphaMask(mask *vectorAlphaMask, clr uint32) {
	if mask == nil || mask.W <= 0 || mask.H <= 0 {
		return
	}
	// A112 records exactly which alpha cells became non-zero. Long diagonal
	// arrows used to scan the entire enclosing rectangle again during compositing,
	// even though almost all of it was empty. Iterating touched cells preserves
	// every alpha value while avoiding those empty corners a second time.
	for _, idx := range mask.Touched {
		if idx < 0 || idx >= len(mask.Alpha) {
			continue
		}
		x := mask.Left + idx%mask.W
		y := mask.Top + idx/mask.W
		if x < 0 || x >= c.W || y < 0 || y >= c.H {
			continue
		}
		a := mask.Alpha[idx]
		if a == 255 {
			c.SetPixel(x, y, clr)
		} else if a != 0 {
			c.blendPixelCoverage(x, y, clr, float64(a)/255.0)
		}
	}
}

// DrawStyledLine is the single source of truth for preview and commit. A91
// shares the arrow shaft's square brush, flat caps, odd/even half-pixel axis,
// and eight-way AA bypass. Horizontal, vertical, and exact 45-degree lines stay
// fully opaque even when AA is enabled; only arbitrary slopes receive coverage.
func (c *Canvas) DrawStyledLine(x0, y0, x1, y1, size int, antialias bool, clr uint32) {
	size = clamp(size, 1, 40)
	offsetX, offsetY := arrowStrokeOffset(x0, y0, x1, y1, size)
	effectiveAA := arrowEffectiveAntialias(antialias, x0, y0, x1, y1)
	c.DrawArrowShaftLine(x0, y0, x1, y1, size, effectiveAA, offsetX, offsetY, clr)
}

// DrawStyledPixelPath keeps strict tan-n staircase geometry while using the
// same square brush and parity-aware axis as arrows. It never replaces the
// integer path with an unrelated floating vector.
func (c *Canvas) DrawStyledPixelPath(points []PixelPoint, size int, antialias bool, clr uint32) {
	if len(points) == 0 {
		return
	}
	size = clamp(size, 1, 40)
	first := points[0]
	last := points[len(points)-1]
	offsetX, offsetY := arrowStrokeOffset(first.X, first.Y, last.X, last.Y, size)
	effectiveAA := arrowEffectiveAntialias(antialias, first.X, first.Y, last.X, last.Y)
	c.DrawArrowShaftPixelPath(points, size, effectiveAA, offsetX, offsetY, clr)
}

// DrawRoundCurvePixelPath renders the curve tool with one filled circular nib
// stamped along its complete integer centreline.  The former implementation
// reused the arrow shaft's square, butt-capped rectangles for every one-pixel
// Bezier segment.  At a change of tangent those tiny rotated rectangles exposed
// their corners as outward "thorns", and their projected width varied with
// direction.  A union of identical circular nibs gives round joins/caps and a
// direction-independent nominal thickness.  Preview and commit share this
// exact renderer.
func (c *Canvas) DrawRoundCurvePixelPath(points []PixelPoint, size int, antialias bool, clr uint32) {
	if len(points) == 0 || c == nil || c.W <= 0 || c.H <= 0 {
		return
	}
	size = clamp(size, 1, 40)
	nib := buildCurveRoundNib(size, antialias)
	if len(nib) == 0 {
		return
	}

	minX, maxX := points[0].X, points[0].X
	minY, maxY := points[0].Y, points[0].Y
	minDX, maxDX := nib[0].DX, nib[0].DX
	minDY, maxDY := nib[0].DY, nib[0].DY
	for _, p := range points[1:] {
		minX, maxX = min(minX, p.X), max(maxX, p.X)
		minY, maxY = min(minY, p.Y), max(maxY, p.Y)
	}
	for _, cell := range nib[1:] {
		minDX, maxDX = min(minDX, cell.DX), max(maxDX, cell.DX)
		minDY, maxDY = min(minDY, cell.DY), max(maxDY, cell.DY)
	}
	left, right := minX+minDX, maxX+maxDX
	top, bottom := minY+minDY, maxY+maxDY
	if right < 0 || bottom < 0 || left >= c.W || top >= c.H {
		return
	}
	left, right = clamp(left, 0, c.W-1), clamp(right, 0, c.W-1)
	top, bottom = clamp(top, 0, c.H-1), clamp(bottom, 0, c.H-1)
	mask := newVectorAlphaMask(left, top, right, bottom)
	for _, p := range points {
		for _, cell := range nib {
			x, y := p.X+cell.DX, p.Y+cell.DY
			if cell.Coverage >= 1 {
				mask.setOpaque(x, y)
			} else {
				mask.setMaxCoverage(x, y, cell.Coverage)
			}
		}
	}
	c.applyVectorAlphaMask(mask, clr)
}

// DrawStyledStrictTanPath preserves the exact integer tan staircase as the
// fully opaque core, then derives its AA fringe from one ideal continuous line
// joining the first and last staircase pixels.  It deliberately does not
// antialias every one-pixel subsegment: doing so creates a grey cap around each
// micro-step and can put grey pixels on the visually wrong side of a descending
// line.  Preview and commit share this path.
func (c *Canvas) DrawStyledStrictTanPath(points []PixelPoint, stepX, stepY, majorSteps, size int, antialias bool, clr uint32) {
	if len(points) == 0 {
		return
	}
	size = clamp(size, 1, 40)
	first := points[0]
	// The selected reduced ratio, not the current partial staircase endpoint,
	// owns the parity axis and AA policy. Otherwise tan 1/3 would temporarily
	// look horizontal for its first two pixels and change phase as it grows.
	offsetX, offsetY := arrowStrokeOffset(first.X, first.Y, first.X+stepX, first.Y+stepY, size)
	effectiveAA := arrowEffectiveAntialias(antialias, 0, 0, stepX, stepY)
	c.DrawArrowShaftStrictTanPixelPath(points, stepX, stepY, majorSteps, size, effectiveAA, offsetX, offsetY, clr)
}

// DrawArrowShaftLine renders a square/butt-capped arrow shaft on the same
// parity-aware floating axis as its head.  Even widths use the upper/left
// half-pixel axis chosen by arrowStrokeOffset.  The flat tail and rear edge avoid
// the round blobs produced by distance-to-segment caps.
func (c *Canvas) DrawArrowShaftLine(x0, y0, x1, y1, size int, antialias bool, offsetX, offsetY float64, clr uint32) {
	size = clamp(size, 1, 40)
	fx0, fy0 := float64(x0)+offsetX, float64(y0)+offsetY
	fx1, fy1 := float64(x1)+offsetX, float64(y1)+offsetY
	halfWidth := float64(size) / 2.0
	margin := int(math.Ceil(halfWidth + math.Max(math.Abs(offsetX), math.Abs(offsetY)) + 1.5))
	left := clamp(min(x0, x1)-margin, 0, c.W-1)
	right := clamp(max(x0, x1)+margin, 0, c.W-1)
	top := clamp(min(y0, y1)-margin, 0, c.H-1)
	bottom := clamp(max(y0, y1)+margin, 0, c.H-1)
	mask := newVectorAlphaMask(left, top, right, bottom)
	candidates := newVectorCoverageCandidates(left, top, right, bottom)
	candidates.markStroke(fx0, fy0, fx1, fy1, size)
	candidates.forEach(func(x, y int) {
		if !antialias {
			if pointInsideArrowStroke(float64(x), float64(y), fx0, fy0, fx1, fy1, size) {
				mask.setOpaque(x, y)
			}
			return
		}
		mask.setMaxCoverage(x, y, arrowStrokeCoverageAt(float64(x), float64(y), fx0, fy0, fx1, fy1, size))
	})
	c.applyVectorAlphaMask(mask, clr)
}

// DrawArrowShaftPixelPath applies the same single perpendicular parity offset
// and square/butt-capped brush to every segment of a strict tan staircase.  The
// path remains a true integer tan-n staircase while cardinal subsegments stay
// fully opaque even with AA enabled.
func (c *Canvas) DrawArrowShaftPixelPath(points []PixelPoint, size int, antialias bool, offsetX, offsetY float64, clr uint32) {
	if len(points) == 0 {
		return
	}
	if len(points) == 1 {
		c.DrawArrowShaftLine(points[0].X, points[0].Y, points[0].X, points[0].Y, size, antialias, offsetX, offsetY, clr)
		return
	}
	size = clamp(size, 1, 40)
	halfWidth := float64(size) / 2.0
	margin := int(math.Ceil(halfWidth + math.Max(math.Abs(offsetX), math.Abs(offsetY)) + 1.0))
	if !antialias {
		for i := 1; i < len(points); i++ {
			p0, p1 := points[i-1], points[i]
			fx0, fy0 := float64(p0.X)+offsetX, float64(p0.Y)+offsetY
			fx1, fy1 := float64(p1.X)+offsetX, float64(p1.Y)+offsetY
			left := clamp(min(p0.X, p1.X)-margin, 0, c.W-1)
			right := clamp(max(p0.X, p1.X)+margin, 0, c.W-1)
			top := clamp(min(p0.Y, p1.Y)-margin, 0, c.H-1)
			bottom := clamp(max(p0.Y, p1.Y)+margin, 0, c.H-1)
			for y := top; y <= bottom; y++ {
				for x := left; x <= right; x++ {
					if pointInsideArrowStroke(float64(x), float64(y), fx0, fy0, fx1, fy1, size) {
						c.SetPixel(x, y, clr)
					}
				}
			}
		}
		return
	}

	coverage := make(map[int]float64, len(points)*(size+2))
	for i := 1; i < len(points); i++ {
		p0, p1 := points[i-1], points[i]
		fx0, fy0 := float64(p0.X)+offsetX, float64(p0.Y)+offsetY
		fx1, fy1 := float64(p1.X)+offsetX, float64(p1.Y)+offsetY
		left := clamp(min(p0.X, p1.X)-margin, 0, c.W-1)
		right := clamp(max(p0.X, p1.X)+margin, 0, c.W-1)
		top := clamp(min(p0.Y, p1.Y)-margin, 0, c.H-1)
		bottom := clamp(max(p0.Y, p1.Y)+margin, 0, c.H-1)
		for y := top; y <= bottom; y++ {
			for x := left; x <= right; x++ {
				cov := arrowStrokeCoverageAt(float64(x), float64(y), fx0, fy0, fx1, fy1, size)
				if cov <= 0 {
					continue
				}
				idx := y*c.W + x
				if cov > coverage[idx] {
					coverage[idx] = cov
				}
			}
		}
	}
	for idx, cov := range coverage {
		c.blendPixelCoverage(idx%c.W, idx/c.W, clr, cov)
	}
}

// DrawArrowShaftStrictTanPixelPath is the directed AA renderer for strict tan
// staircases.  The integer staircase itself remains fully opaque and exact.
// Only pixels outside that core may receive coverage from the single ideal
// line that the staircase approximates.  This removes repeated subsegment-cap
// halos without deleting AA outright.
func (c *Canvas) DrawArrowShaftStrictTanPixelPath(points []PixelPoint, stepX, stepY, majorSteps, size int, antialias bool, offsetX, offsetY float64, clr uint32) {
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
		preparedShadow := arrowPrepareStroke(shadow.X0, shadow.Y0, shadow.X1, shadow.Y1, size)
		candidates.forEach(func(x, y int) {
			mask.setMaxCoverage(x, y, arrowStrokeLongEdgesCoveragePrepared(float64(x), float64(y), preparedShadow))
		})
	}

	// Exact integer staircase is unioned as the fully opaque core before the
	// mask is composited, rather than painted in a separate pass.
	for i := 1; i < len(points); i++ {
		p0, p1 := points[i-1], points[i]
		sx0, sy0 := float64(p0.X)+offsetX, float64(p0.Y)+offsetY
		sx1, sy1 := float64(p1.X)+offsetX, float64(p1.Y)+offsetY
		segment := arrowPrepareStroke(sx0, sy0, sx1, sy1, size)
		segLeft := clamp(min(p0.X, p1.X)-margin, left, right)
		segRight := clamp(max(p0.X, p1.X)+margin, left, right)
		segTop := clamp(min(p0.Y, p1.Y)-margin, top, bottom)
		segBottom := clamp(max(p0.Y, p1.Y)+margin, top, bottom)
		for y := segTop; y <= segBottom; y++ {
			for x := segLeft; x <= segRight; x++ {
				if shadowOK && !shadow.containsRasterCentre(float64(x), float64(y)) {
					continue
				}
				if segment.contains(float64(x), float64(y)) {
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

// DrawUnifiedVectorArrow builds one hidden alpha mask for the shaft and head.
// The two vector components are unioned at every sub-pixel sample and written
// to the canvas once, eliminating neck seams and double-dark overlap pixels.
func (c *Canvas) DrawUnifiedVectorArrow(x0, y0, x1, y1, size int, shaftAA bool, head ArrowHeadGeometry, headAA bool, strictPath []PixelPoint, strictShadow *strictTanShadowSegment, clr uint32) {
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
	preparedShaft := arrowPrepareStroke(fx0, fy0, fx1, fy1, size)
	preparedHeadEdges := arrowHeadEdgeAntialias(head, headAA)
	candidates.forEach(func(x, y int) {
		cov := arrowCompositeCoveragePrepared(float64(x), float64(y), hasShaft, preparedShaft, shaftAA, &head, headAA, preparedHeadEdges)
		mask.setMaxCoverage(x, y, cov)
	})

	// Strict tan mode contributes its exact integer staircase to the same mask.
	if len(strictPath) > 1 {
		margin := int(math.Ceil(halfWidth + math.Max(math.Abs(head.PixelOffsetX), math.Abs(head.PixelOffsetY)) + 1.5))
		for i := 1; i < len(strictPath); i++ {
			p0, p1 := strictPath[i-1], strictPath[i]
			sx0, sy0 := float64(p0.X)+head.PixelOffsetX, float64(p0.Y)+head.PixelOffsetY
			sx1, sy1 := float64(p1.X)+head.PixelOffsetX, float64(p1.Y)+head.PixelOffsetY
			segment := arrowPrepareStroke(sx0, sy0, sx1, sy1, size)
			segLeft := clamp(min(p0.X, p1.X)-margin, left, right)
			segRight := clamp(max(p0.X, p1.X)+margin, left, right)
			segTop := clamp(min(p0.Y, p1.Y)-margin, top, bottom)
			segBottom := clamp(max(p0.Y, p1.Y)+margin, top, bottom)
			for y := segTop; y <= segBottom; y++ {
				for x := segLeft; x <= segRight; x++ {
					if segment.contains(float64(x), float64(y)) {
						mask.setOpaque(x, y)
					}
				}
			}
		}
	}
	c.applyVectorAlphaMask(mask, clr)
}

// FillStyledArrowHead rasterizes the parity-aware 1:3 arrow head directly
// into the native canvas. Each of the four polygon edges chooses AA from its own
// angle: horizontal, vertical, and exact 45-degree edges stay binary, while all
// other edges use 4x4 coverage. Thus a tan(1/3) arrow correctly gets one crisp
// horizontal side and one antialiased tan(3/4) side.
func (c *Canvas) FillStyledArrowHead(g ArrowHeadGeometry, antialias bool, clr uint32) {
	minX := math.Min(g.Tip.X-g.TipHalfWidth, math.Min(g.SideA.X, g.SideB.X))
	maxX := math.Max(g.Tip.X+g.TipHalfWidth, math.Max(g.SideA.X, g.SideB.X))
	minY := math.Min(g.Tip.Y-g.TipHalfWidth, math.Min(g.SideA.Y, g.SideB.Y))
	maxY := math.Max(g.Tip.Y+g.TipHalfWidth, math.Max(g.SideA.Y, g.SideB.Y))
	left := clamp(int(math.Floor(minX))-1, 0, c.W-1)
	right := clamp(int(math.Ceil(maxX))+1, 0, c.W-1)
	top := clamp(int(math.Floor(minY))-1, 0, c.H-1)
	bottom := clamp(int(math.Ceil(maxY))+1, 0, c.H-1)
	if left > right || top > bottom {
		return
	}
	for y := top; y <= bottom; y++ {
		for x := left; x <= right; x++ {
			coverage := arrowHeadCoverageAt(float64(x), float64(y), g, antialias)
			if coverage <= 0 {
				continue
			}
			if coverage >= 1 {
				c.SetPixel(x, y, clr)
			} else {
				c.blendPixelCoverage(x, y, clr, coverage)
			}
		}
	}
}

func cubicBezierPoint(p0, p1, p2, p3 int, t float64) int {
	u := 1.0 - t
	v := u*u*u*float64(p0) + 3*u*u*t*float64(p1) + 3*u*t*t*float64(p2) + t*t*t*float64(p3)
	return int(math.Round(v))
}

func (c *Canvas) DrawBezier(x0, y0, c1x, c1y, c2x, c2y, x1, y1 int, clr uint32) {
	distance := math.Hypot(float64(x1-x0), float64(y1-y0)) +
		math.Hypot(float64(c1x-x0), float64(c1y-y0)) +
		math.Hypot(float64(c2x-x1), float64(c2y-y1))
	steps := int(distance * 2)
	if steps < 48 {
		steps = 48
	}
	if steps > 4096 {
		steps = 4096
	}
	px, py := x0, y0
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		nx := cubicBezierPoint(x0, c1x, c2x, x1, t)
		ny := cubicBezierPoint(y0, c1y, c2y, y1, t)
		c.DrawLine(px, py, nx, ny, clr)
		px, py = nx, ny
	}
}

func quarticBezierPoint(p0, p1, p2, p3, p4 int, t float64) int {
	u := 1.0 - t
	v := u*u*u*u*float64(p0) + 4*u*u*u*t*float64(p1) + 6*u*u*t*t*float64(p2) + 4*u*t*t*t*float64(p3) + t*t*t*t*float64(p4)
	return int(math.Round(v))
}

// bezierCoordinate evaluates a Bezier curve of any degree with de Casteljau's
// algorithm. Curve stages use degree 1 through 5 progressively, so a bend
// point that has not yet been chosen cannot influence the visible curve.
func bezierCoordinate(values []int, t float64) int {
	if len(values) == 0 {
		return 0
	}
	work := make([]float64, len(values))
	for i, v := range values {
		work[i] = float64(v)
	}
	for n := len(work) - 1; n > 0; n-- {
		for i := 0; i < n; i++ {
			work[i] = work[i]*(1.0-t) + work[i+1]*t
		}
	}
	return int(math.Round(work[0]))
}

func appendRasterSegment(points []PixelPoint, x0, y0, x1, y1 int) []PixelPoint {
	dx := absInt(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -absInt(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		if len(points) == 0 || points[len(points)-1].X != x0 || points[len(points)-1].Y != y0 {
			points = append(points, PixelPoint{X: x0, Y: y0})
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
	return points
}

// bezierRasterPath samples a Bezier curve and connects every sample with an
// integer Bresenham segment. The resulting spine is shared by preview and
// commit, including thick and anti-aliased curves.
func bezierRasterPath(control []PixelPoint) []PixelPoint {
	if len(control) < 2 {
		return append([]PixelPoint(nil), control...)
	}
	distance := 0.0
	for i := 1; i < len(control); i++ {
		distance += math.Hypot(float64(control[i].X-control[i-1].X), float64(control[i].Y-control[i-1].Y))
	}
	steps := int(math.Ceil(distance * 2.0))
	if steps < 48 {
		steps = 48
	}
	if steps > 8192 {
		steps = 8192
	}
	xs := make([]int, len(control))
	ys := make([]int, len(control))
	for i, p := range control {
		xs[i], ys[i] = p.X, p.Y
	}
	path := make([]PixelPoint, 0, steps+1)
	px, py := control[0].X, control[0].Y
	path = append(path, PixelPoint{X: px, Y: py})
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		nx := bezierCoordinate(xs, t)
		ny := bezierCoordinate(ys, t)
		path = appendRasterSegment(path, px, py, nx, ny)
		px, py = nx, ny
	}
	return path
}

// DrawQuarticBezier draws a smooth curve with three independently adjustable
// control points. This gives dotPaint one more bend-adjustment stage than the
// classic two-control-point curve tool.
func (c *Canvas) DrawQuarticBezier(x0, y0, c1x, c1y, c2x, c2y, c3x, c3y, x1, y1 int, clr uint32) {
	distance := math.Hypot(float64(x1-x0), float64(y1-y0)) +
		math.Hypot(float64(c1x-x0), float64(c1y-y0)) +
		math.Hypot(float64(c2x-c1x), float64(c2y-c1y)) +
		math.Hypot(float64(c3x-c2x), float64(c3y-c2y)) +
		math.Hypot(float64(x1-c3x), float64(y1-c3y))
	steps := int(distance * 2)
	if steps < 64 {
		steps = 64
	}
	if steps > 8192 {
		steps = 8192
	}
	px, py := x0, y0
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		nx := quarticBezierPoint(x0, c1x, c2x, c3x, x1, t)
		ny := quarticBezierPoint(y0, c1y, c2y, c3y, y1, t)
		c.DrawLine(px, py, nx, ny, clr)
		px, py = nx, ny
	}
}

func (c *Canvas) StampCircle(cx, cy, radius int, clr uint32) {
	rr := radius * radius
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			if x*x+y*y <= rr {
				c.SetPixel(cx+x, cy+y, clr)
			}
		}
	}
}

func squareStampBounds(cx, cy, size int) (left, top, right, bottom int) {
	if size < 1 {
		size = 1
	}
	left = cx - (size-1)/2
	top = cy - (size-1)/2
	right = left + size
	bottom = top + size
	return
}

func (c *Canvas) StampSquare(cx, cy, size int, clr uint32) {
	left, top, right, bottom := squareStampBounds(cx, cy, size)
	for y := top; y < bottom; y++ {
		for x := left; x < right; x++ {
			c.SetPixel(x, y, clr)
		}
	}
}

func (c *Canvas) DrawBrushLine(x0, y0, x1, y1, radius int, clr uint32) {
	dx := int(math.Abs(float64(x1 - x0)))
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -int(math.Abs(float64(y1 - y0)))
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		c.StampCircle(x0, y0, radius, clr)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func (c *Canvas) DrawEraserLine(x0, y0, x1, y1, size int, clr uint32) {
	dx := int(math.Abs(float64(x1 - x0)))
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -int(math.Abs(float64(y1 - y0)))
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		c.StampSquare(x0, y0, size, clr)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func (c *Canvas) StampBrush(cx, cy, size int, shape BrushShape, clr uint32) {
	if size < 1 {
		size = 1
	}
	left, top, right, bottom := squareStampBounds(cx, cy, size)
	switch shape {
	case BrushCircle:
		centerX := (float64(left) + float64(right-1)) / 2.0
		centerY := (float64(top) + float64(bottom-1)) / 2.0
		radius := float64(size) / 2.0
		if radius < 0.5 {
			radius = 0.5
		}
		for y := top; y < bottom; y++ {
			for x := left; x < right; x++ {
				dx := float64(x) - centerX
				dy := float64(y) - centerY
				if dx*dx+dy*dy <= radius*radius {
					c.SetPixel(x, y, clr)
				}
			}
		}
	case BrushSlashUp, BrushSlashDown:
		// A diagonal nib uses at least a 2×2 footprint and the same minimum band
		// used from 9 px upward. This prevents the striped, detached appearance
		// produced by a nominally one-pixel diagonal on curved strokes.
		nibSize := diagonalBrushFootprintSize(size)
		nibLeft, nibTop, _, _ := squareStampBounds(cx, cy, nibSize)
		band := diagonalBrushBand(nibSize)
		for y := 0; y < nibSize; y++ {
			for x := 0; x < nibSize; x++ {
				d := x - y
				if shape == BrushSlashUp {
					d = x + y - (nibSize - 1)
				}
				if absInt(d) <= band {
					c.SetPixel(nibLeft+x, nibTop+y, clr)
				}
			}
		}
	default:
		c.StampSquare(cx, cy, size, clr)
	}
}

func (c *Canvas) DrawUnifiedBrushLine(x0, y0, x1, y1, size int, shape BrushShape, clr uint32) {
	dx := absInt(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -absInt(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		c.StampBrush(x0, y0, size, shape, clr)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// DrawRectOuter draws an unfilled rectangle whose drag coordinates are the
// outside pixel bounds. Stroke thickness grows only inward, so changing the
// width never expands the visible rectangle beyond the requested outer size.
func (c *Canvas) DrawRectOuter(x0, y0, x1, y1, size int, clr uint32) {
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
	// A hollow rectangle has only an O(perimeter*thickness) painted border.
	// The old full-area scan wasted work on every untouched inner pixel. All
	// written pixels, including transparent/inverted RGBAs, are unchanged.
	px := colorrefToBGRA(clr)
	fillSpan := func(y, lo, hi int) {
		for x := lo; x <= hi; x++ {
			i := (y*c.W + x) * 4
			c.Pix[i], c.Pix[i+1], c.Pix[i+2], c.Pix[i+3] = px[0], px[1], px[2], px[3]
		}
	}
	topEnd := min(y1, y0+thickness-1)
	for y := y0; y <= topEnd; y++ {
		fillSpan(y, x0, x1)
	}
	bottomStart := max(topEnd+1, y1-thickness+1)
	for y := bottomStart; y <= y1; y++ {
		fillSpan(y, x0, x1)
	}
	for y := topEnd + 1; y < bottomStart; y++ {
		fillSpan(y, x0, x0+thickness-1)
		fillSpan(y, x1-thickness+1, x1)
	}
}

func (c *Canvas) DrawRect(x0, y0, x1, y1 int, clr uint32) {
	c.DrawRectOuter(x0, y0, x1, y1, 1, clr)
}

func (c *Canvas) DrawEllipseOuter(x0, y0, x1, y1, size int, clr uint32) {
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
	w := x1 - x0 + 1
	h := y1 - y0 + 1
	if w <= 0 || h <= 0 {
		return
	}
	size = clamp(size, 1, 40)
	thickness := min(size, min(w, h))
	if w <= 2 || h <= 2 {
		c.DrawRectOuter(x0, y0, x1, y1, thickness, clr)
		return
	}

	// A93: build the stroke as the exact difference of two concentric analytic
	// ellipses and flatten cardinal tips on axes larger than 20px. The inner and
	// outer boundaries use the same tiny symmetric cap, removing axis vertices
	// without changing the requested bounds or the inward-only stroke thickness.
	// Quadrant mirroring keeps left/right and top/bottom pixels exactly identical.
	mask := symmetricEllipseRingMask(w, h, thickness)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mask[y*w+x] {
				c.SetPixel(x0+x, y0+y, clr)
			}
		}
	}
}

func (c *Canvas) DrawEllipse(x0, y0, x1, y1 int, clr uint32) {
	c.DrawEllipseOuter(x0, y0, x1, y1, 1, clr)
}

func (c *Canvas) Invert() {
	for i := 0; i < len(c.Pix); i += 4 {
		c.Pix[i] = 255 - c.Pix[i]
		c.Pix[i+1] = 255 - c.Pix[i+1]
		c.Pix[i+2] = 255 - c.Pix[i+2]
	}
}

func (c *Canvas) FlipHorizontal() {
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W/2; x++ {
			a := (y*c.W + x) * 4
			b := (y*c.W + (c.W - 1 - x)) * 4
			for k := 0; k < 4; k++ {
				c.Pix[a+k], c.Pix[b+k] = c.Pix[b+k], c.Pix[a+k]
			}
		}
	}
}

func (c *Canvas) FlipVertical() {
	row := c.W * 4
	tmp := make([]byte, row)
	for y := 0; y < c.H/2; y++ {
		a := y * row
		b := (c.H - 1 - y) * row
		copy(tmp, c.Pix[a:a+row])
		copy(c.Pix[a:a+row], c.Pix[b:b+row])
		copy(c.Pix[b:b+row], tmp)
	}
}

func (c *Canvas) ResizeNearest(nw, nh int, bg uint32) {
	if nw < 1 || nh < 1 {
		return
	}
	out := NewCanvas(nw, nh, bg)
	for y := 0; y < nh; y++ {
		sy := y * c.H / nh
		for x := 0; x < nw; x++ {
			sx := x * c.W / nw
			si := (sy*c.W + sx) * 4
			di := (y*nw + x) * 4
			copy(out.Pix[di:di+4], c.Pix[si:si+4])
		}
	}
	c.W, c.H, c.Pix = out.W, out.H, out.Pix
}

// ResizeCanvas changes only the canvas bounds. Existing pixels are copied
// without scaling. offsetX/offsetY specify where the old top-left pixel is
// placed in the new canvas, allowing all eight resize handles to crop or add
// space from any side.
func (c *Canvas) ResizeCanvas(nw, nh, offsetX, offsetY int, bg uint32) {
	if nw < 1 || nh < 1 {
		return
	}
	out := NewCanvas(nw, nh, bg)
	for y := 0; y < c.H; y++ {
		ny := y + offsetY
		if ny < 0 || ny >= nh {
			continue
		}
		for x := 0; x < c.W; x++ {
			nx := x + offsetX
			if nx < 0 || nx >= nw {
				continue
			}
			si := (y*c.W + x) * 4
			di := (ny*nw + nx) * 4
			copy(out.Pix[di:di+4], c.Pix[si:si+4])
		}
	}
	c.W, c.H, c.Pix = out.W, out.H, out.Pix
}

func rgb(r, g, b byte) uint32 {
	return uint32(r) | uint32(g)<<8 | uint32(b)<<16
}

func isTransparentColor(c uint32) bool { return c == transparentColor }

func colorrefToBGRA(c uint32) [4]byte {
	if isTransparentColor(c) {
		return [4]byte{0, 0, 0, 0}
	}
	return [4]byte{byte((c >> 16) & 0xff), byte((c >> 8) & 0xff), byte(c & 0xff), 255}
}

func bgraToColorref(b, g, r byte) uint32 { return rgb(r, g, b) }

func rgba16ToStraight8(r, g, b, a uint32) (byte, byte, byte, byte) {
	if a == 0 {
		return 0, 0, 0, 0
	}
	convert := func(v uint32) byte {
		straight := (uint64(v)*0xffff + uint64(a)/2) / uint64(a)
		if straight > 0xffff {
			straight = 0xffff
		}
		return byte(straight >> 8)
	}
	return convert(r), convert(g), convert(b), byte(a >> 8)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

const transparentColor uint32 = 0x01000000

type BrushShape int

const (
	BrushSquare BrushShape = iota
	BrushCircle
	BrushSlashUp
	BrushSlashDown
)

type LineConstraintMode int

const (
	LineConstraint15Degrees LineConstraintMode = iota
	LineConstraintTanN
)

type LineTanRatio struct {
	Rise int
	Run  int
}

// Each tan-n direction is stored as an exact reduced integer ratio. The
// rasterizer follows each ratio as a repeating one-pixel staircase rather
// than distributing rounding error over the whole line. Thus tan 1/3 keeps
// exactly three horizontal pixels per vertical step while partial prefixes can
// still appear naturally at the live endpoint.
var lineTanRatios = []LineTanRatio{
	{Rise: 1, Run: 5},
	{Rise: 1, Run: 4},
	{Rise: 1, Run: 3},
	{Rise: 1, Run: 2},
	{Rise: 2, Run: 3},
	{Rise: 1, Run: 1},
	{Rise: 3, Run: 2},
	{Rise: 2, Run: 1},
	{Rise: 3, Run: 1},
	{Rise: 4, Run: 1},
	{Rise: 5, Run: 1},
}

type PixelPoint struct{ X, Y int }

func shapeCenterMarkerBounds(left, top, width, height int) (mx0, my0, mx1, my1 int) {
	mw := 1
	mh := 1
	if width%2 == 0 {
		mw = 2
	}
	if height%2 == 0 {
		mh = 2
	}
	mx0 = left + (width-mw)/2
	my0 = top + (height-mh)/2
	mx1 = mx0 + mw - 1
	my1 = my0 + mh - 1
	return
}

func (c *Canvas) DrawCenterMarker(left, top, width, height int, clr uint32) {
	if width <= 0 || height <= 0 {
		return
	}
	mx0, my0, mx1, my1 := shapeCenterMarkerBounds(left, top, width, height)
	for y := my0; y <= my1; y++ {
		for x := mx0; x <= mx1; x++ {
			c.SetPixel(x, y, clr)
		}
	}
}
