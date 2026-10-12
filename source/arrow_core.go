package main

import "math"

// ArrowPoint is a floating-point canvas coordinate used to rotate the filled
// arrow head without accumulating integer rounding error before rasterization.
type ArrowPoint struct {
	X float64
	Y float64
}

// ArrowHeadGeometry describes a parity-aware filled arrow head in its own
// rotated coordinate frame. Tip is the centre of the tip cap, Base is the
// centre of the rear edge, and SideA/SideB are the rear corners. The two side
// edges always have a rise:run ratio of 1:3.
type ArrowHeadGeometry struct {
	Tip           ArrowPoint
	Base          ArrowPoint
	SideA         ArrowPoint
	SideB         ArrowPoint
	UnitX         float64
	UnitY         float64
	PerpX         float64
	PerpY         float64
	Length        float64
	TipHalfWidth  float64
	BaseHalfWidth float64
	PixelOffsetX  float64
	PixelOffsetY  float64
	PhaseOffsetX  float64
	PhaseOffsetY  float64
}

// Four head-size choices.  The new 4-step head reproduces the uploaded 12x9
// compact silhouette (wing growth 4 px, length 12 px); the former 5/7/9-step
// heads remain unchanged.  Because the side angle is 1:3, the corresponding
// nominal head lengths are 12/15/21/27 px.
var arrowHeadWingSteps = []float64{4, 5, 7, 9}

func clampArrowHeadChoice(choice int) int {
	if choice < 0 {
		return 0
	}
	if choice >= len(arrowHeadWingSteps) {
		return len(arrowHeadWingSteps) - 1
	}
	return choice
}

func arrowTipHalfWidth(lineSize int) float64 {
	if lineSize%2 == 0 {
		// Even shafts are centred between two raster rows/columns. A one-pixel
		// geometric cap on each side yields an exact two-pixel tip.
		return 1.0
	}
	// Odd shafts are centred on one raster row/column. Half a pixel on each side
	// yields exactly one tip pixel.
	return 0.5
}

func arrowParityOffset(px, py float64, lineSize int) (float64, float64) {
	if lineSize%2 != 0 {
		return 0, 0
	}
	// A95: anchor even-width strokes to an actual half-pixel row or column,
	// rather than moving half a pixel along an arbitrary floating normal.
	//
	// The former normal-vector offset had fractional X and Y components at
	// oblique angles.  Although the continuous rectangle was mathematically
	// symmetric, its two ends landed at different phases of the integer pixel
	// grid, so the upper-left and lower-right caps could receive visibly
	// different AA patterns.  A grid-aligned upper/left half-pixel offset makes
	// twice the stroke centre integral again, restoring exact 180-degree raster
	// symmetry while keeping the user's requested upper-pixel convention.
	//
	// px,py is the unit perpendicular.  When the perpendicular is mainly
	// vertical the stroke is mainly horizontal, so anchor it 0.5 px upward.
	// When it is mainly horizontal the stroke is mainly vertical, so anchor it
	// 0.5 px leftward.  Exact diagonals use the upper row by convention.
	if math.Abs(px) > math.Abs(py)+1e-9 {
		return -0.5, 0
	}
	return 0, -0.5
}

// arrowStrokeOffset returns the parity offset for a segment.  Keeping this in
// the portable core lets tests verify the exact half-pixel raster axis without
// depending on Win32 drawing code.
func arrowStrokeOffset(x0, y0, x1, y1, lineSize int) (float64, float64) {
	dx := float64(x1 - x0)
	dy := float64(y1 - y0)
	length := math.Hypot(dx, dy)
	if length <= 0 || lineSize%2 != 0 {
		return 0, 0
	}
	ux, uy := dx/length, dy/length
	return arrowParityOffset(-uy, ux, lineSize)
}

// arrowStrokeCoordinates projects a pixel centre into the shaft's local frame.
// forward grows from tail to head and lateral is signed across the shaft.
func arrowStrokeCoordinates(px, py, x0, y0, x1, y1 float64) (forward, lateral, length float64, ok bool) {
	dx, dy := x1-x0, y1-y0
	length = math.Hypot(dx, dy)
	if length <= 1e-12 {
		return 0, 0, 0, false
	}
	ux, uy := dx/length, dy/length
	rx, ry := px-x0, py-y0
	forward = rx*ux + ry*uy
	lateral = rx*(-uy) + ry*ux
	return forward, lateral, length, true
}

// pointInsideArrowStroke tests a square/butt-capped shaft.  Unlike a distance
// to a segment, this does not create round end caps.  The physical stroke starts
// half a pixel before the logical tail and ends half a pixel after the logical
// rear-centre, so the first and last raster columns/rows are fully opaque while
// still ending cleanly at the arrow-head base.
func pointInsideArrowStroke(px, py, x0, y0, x1, y1 float64, lineSize int) bool {
	if lineSize < 1 {
		lineSize = 1
	}
	forward, lateral, length, ok := arrowStrokeCoordinates(px, py, x0, y0, x1, y1)
	if !ok {
		half := float64(lineSize) / 2.0
		return math.Abs(px-x0) <= half+1e-9 && math.Abs(py-y0) <= half+1e-9
	}
	half := float64(lineSize) / 2.0
	return forward >= -0.5-1e-9 && forward <= length+0.5+1e-9 && math.Abs(lateral) <= half+1e-9
}

// canonicalArrowSegment gives a finite stroke one deterministic endpoint order.
// The geometric rectangle is directionless, so all coverage arithmetic should
// be bit-for-bit identical whether the user drags A->B or B->A.
func canonicalArrowSegment(x0, y0, x1, y1 float64) (float64, float64, float64, float64) {
	if x0 > x1 || (x0 == x1 && y0 > y1) {
		return x1, y1, x0, y0
	}
	return x0, y0, x1, y1
}

// arrowStrokeCoverageAt returns the pixel contribution of one finite vector
// stroke.  A97 keeps the shaft as an invisible square-capped vector rectangle
// and measures its area on one global 8x8 sub-pixel lattice.  Long sides and
// both caps therefore come from the same geometry instead of unrelated centre
// and edge rules.  Reversing the endpoints describes exactly the same vector
// rectangle and produces identical alpha values.
func arrowStrokeCoverageAt(px, py, x0, y0, x1, y1 float64, lineSize int) float64 {
	if lineSize < 1 {
		lineSize = 1
	}
	x0, y0, x1, y1 = canonicalArrowSegment(x0, y0, x1, y1)
	const samples = 8
	inside := 0
	for sy := 0; sy < samples; sy++ {
		qy := py - 0.5 + (float64(sy)+0.5)/samples
		for sx := 0; sx < samples; sx++ {
			qx := px - 0.5 + (float64(sx)+0.5)/samples
			if pointInsideArrowStroke(qx, qy, x0, y0, x1, y1, lineSize) {
				inside++
			}
		}
	}
	return float64(inside) / float64(samples*samples)
}

// arrowStrokeLongEdgesCoverageAt antialiases only the two long sides of a
// finite square-capped stroke.  The first/last half-pixel cap zones are left to
// the fully opaque integer path.  Strict tan lines therefore keep clean flat
// ends instead of growing diagonal grey "feathers" from the invisible vector.
func arrowStrokeLongEdgesCoverageAt(px, py, x0, y0, x1, y1 float64, lineSize int) float64 {
	if lineSize < 1 {
		lineSize = 1
	}
	centreForward, _, centreLength, centreOK := arrowStrokeCoordinates(px, py, x0, y0, x1, y1)
	if !centreOK || centreLength <= 1.0 || centreForward <= 0.5 || centreForward >= centreLength-0.5 {
		return 0
	}
	const samples = 8
	inside := 0
	for sy := 0; sy < samples; sy++ {
		qy := py - 0.5 + (float64(sy)+0.5)/samples
		for sx := 0; sx < samples; sx++ {
			qx := px - 0.5 + (float64(sx)+0.5)/samples
			forward, lateral, length, ok := arrowStrokeCoordinates(qx, qy, x0, y0, x1, y1)
			if !ok || length <= 1.0 {
				continue
			}
			if forward >= 0.5 && forward <= length-0.5 && math.Abs(lateral) <= float64(lineSize)/2.0+1e-9 {
				inside++
			}
		}
	}
	return float64(inside) / float64(samples*samples)
}

// arrowDirectionIsCrispEightWay identifies the four cardinal and four exact
// diagonal directions.  These directions already map cleanly to the pixel grid,
// so AA would only introduce unnecessary grey edge pixels.
func arrowDirectionIsCrispEightWay(x0, y0, x1, y1 int) bool {
	dx := absCoreInt(x1 - x0)
	dy := absCoreInt(y1 - y0)
	if dx == 0 && dy == 0 {
		return false
	}
	if dx == 0 || dy == 0 {
		return true
	}
	return absCoreInt(dx-dy) <= 1
}

func absCoreInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// arrowEffectiveAntialias keeps the user's AA preference for arbitrary slopes,
// but forces crisp binary rasterization for the eight grid-aligned directions.
func arrowEffectiveAntialias(enabled bool, x0, y0, x1, y1 int) bool {
	return enabled && !arrowDirectionIsCrispEightWay(x0, y0, x1, y1)
}

// ArrowHeadEdgeAA records which individual polygon edges need sub-pixel
// coverage.  The shaft direction alone is not enough: a horizontal arrow has
// crisp vertical caps but two 1:3 diagonal side edges, while a tan(1/3) arrow
// has one horizontal side edge and one tan(3/4) side edge.
type ArrowHeadEdgeAA struct {
	TipCap   bool
	RearBase bool
	SideA    bool
	SideB    bool
}

func (e ArrowHeadEdgeAA) any() bool {
	return e.TipCap || e.RearBase || e.SideA || e.SideB
}

// arrowVectorIsCrispEightWayFloat reports whether an edge itself is horizontal,
// vertical, or exactly 45 degrees.  Such an edge already lies cleanly on the
// pixel grid and must stay binary even when the user's AA switch is on.
func arrowVectorIsCrispEightWayFloat(dx, dy float64) bool {
	ax, ay := math.Abs(dx), math.Abs(dy)
	const eps = 1e-8
	if ax <= eps && ay <= eps {
		return false
	}
	if ax <= eps || ay <= eps {
		return true
	}
	scale := math.Max(ax, ay)
	return math.Abs(ax-ay) <= eps*math.Max(1, scale)
}

// arrowHeadEdgeAntialias classifies all four head edges independently.  The
// two caps run along the perpendicular vector.  Each 1:3 side begins at one end
// of the parity-aware tip cap and ends at its matching rear corner.
func arrowHeadEdgeAntialias(g ArrowHeadGeometry, enabled bool) ArrowHeadEdgeAA {
	if !enabled {
		return ArrowHeadEdgeAA{}
	}
	wing := g.BaseHalfWidth - g.TipHalfWidth
	capDX, capDY := g.PerpX, g.PerpY
	sideADX := -g.UnitX*g.Length + g.PerpX*wing
	sideADY := -g.UnitY*g.Length + g.PerpY*wing
	sideBDX := -g.UnitX*g.Length - g.PerpX*wing
	sideBDY := -g.UnitY*g.Length - g.PerpY*wing
	return ArrowHeadEdgeAA{
		TipCap:   !arrowVectorIsCrispEightWayFloat(capDX, capDY),
		RearBase: !arrowVectorIsCrispEightWayFloat(capDX, capDY),
		SideA:    !arrowVectorIsCrispEightWayFloat(sideADX, sideADY),
		SideB:    !arrowVectorIsCrispEightWayFloat(sideBDX, sideBDY),
	}
}

func arrowHeadLocalCoordinates(p ArrowPoint, g ArrowHeadGeometry) (backward, lateral float64) {
	rx := p.X - g.Tip.X
	ry := p.Y - g.Tip.Y
	backward = -(rx*g.UnitX + ry*g.UnitY)
	lateral = rx*g.PerpX + ry*g.PerpY
	return backward, lateral
}

func arrowHeadSelectedEdgesContain(p ArrowPoint, g ArrowHeadGeometry, edges ArrowHeadEdgeAA, antialiasedEdges bool) bool {
	backward, lateral := arrowHeadLocalCoordinates(p, g)
	allowed := g.TipHalfWidth + backward/3.0
	const eps = 1e-9
	if edges.TipCap == antialiasedEdges && backward < -eps {
		return false
	}
	if edges.RearBase == antialiasedEdges && backward > g.Length+eps {
		return false
	}
	if edges.SideA == antialiasedEdges && lateral > allowed+eps {
		return false
	}
	if edges.SideB == antialiasedEdges && -lateral > allowed+eps {
		return false
	}
	return true
}

// arrowHeadCoverageAt performs selective edge AA.  Crisp edges are decided once
// at the pixel centre, so they never produce a grey fringe.  Only edges whose
// own angle is neither cardinal nor 45 degrees participate in 4x4 coverage.
func arrowHeadCoverageAt(px, py float64, g ArrowHeadGeometry, antialias bool) float64 {
	centre := ArrowPoint{X: px, Y: py}
	if !antialias {
		if pointInsideArrowHead(centre, g) {
			return 1
		}
		return 0
	}
	edges := arrowHeadEdgeAntialias(g, true)
	if !edges.any() {
		if pointInsideArrowHead(centre, g) {
			return 1
		}
		return 0
	}
	// Hard edges use the centre sample only. This is what keeps a horizontal,
	// vertical, or 45-degree boundary completely opaque/transparent.
	if !arrowHeadSelectedEdgesContain(centre, g, edges, false) {
		return 0
	}
	const samples = 4
	inside := 0
	for sy := 0; sy < samples; sy++ {
		for sx := 0; sx < samples; sx++ {
			sample := ArrowPoint{
				X: px - 0.5 + (float64(sx)+0.5)/samples,
				Y: py - 0.5 + (float64(sy)+0.5)/samples,
			}
			if arrowHeadSelectedEdgesContain(sample, g, edges, true) {
				inside++
			}
		}
	}
	return float64(inside) / float64(samples*samples)
}

// arrowCompositeCoverageAt measures the union of the shaft rectangle and the
// rotated arrow-head polygon before any colour is written to the canvas.  This
// is the hidden-vector-mask path: overlap at the neck is resolved in alpha
// space, so the body and head cannot darken each other or leave a one-pixel gap.
// Grid-aligned shaft/head edges remain binary under the existing crisp policy;
// only genuinely oblique edges use the shared 8x8 contribution lattice.
func arrowCompositeCoverageAt(px, py float64, hasShaft bool, x0, y0, x1, y1 float64, lineSize int, shaftAA bool, g ArrowHeadGeometry, headAA bool) float64 {
	centre := ArrowPoint{X: px, Y: py}
	shaftCentre := hasShaft && pointInsideArrowStroke(px, py, x0, y0, x1, y1, lineSize)
	if shaftCentre && !shaftAA {
		return 1
	}

	edges := arrowHeadEdgeAntialias(g, headAA)
	headCentre := pointInsideArrowHead(centre, g)
	if headCentre && (!headAA || !edges.any()) {
		return 1
	}
	headHardPass := true
	if headAA && edges.any() {
		headHardPass = arrowHeadSelectedEdgesContain(centre, g, edges, false)
	}

	const samples = 8
	inside := 0
	for sy := 0; sy < samples; sy++ {
		qy := py - 0.5 + (float64(sy)+0.5)/samples
		for sx := 0; sx < samples; sx++ {
			qx := px - 0.5 + (float64(sx)+0.5)/samples
			shaftInside := false
			if hasShaft {
				if shaftAA {
					shaftInside = pointInsideArrowStroke(qx, qy, x0, y0, x1, y1, lineSize)
				} else {
					shaftInside = shaftCentre
				}
			}

			headInside := false
			if headAA && edges.any() {
				if headHardPass {
					headInside = arrowHeadSelectedEdgesContain(ArrowPoint{X: qx, Y: qy}, g, edges, true)
				}
			} else {
				headInside = headCentre
			}
			if shaftInside || headInside {
				inside++
			}
		}
	}
	return float64(inside) / float64(samples*samples)
}

func pointSegmentDistanceCore(px, py, x0, y0, x1, y1 float64) float64 {
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

// calculateArrowHead rotates a right-facing parity-aware reference head onto
// the shaft direction. The drag start is the tail and the drag end is the arrow
// tip. The side edges use |perpendicular| = tipHalfWidth + backwardDistance/3,
// so the requested angle is exactly 1:3. Very short arrows reduce only the wing
// growth; the one-pixel (odd) or two-pixel (even) tip cap is preserved.
func calculateArrowHeadDirected(tipX, tipY, ux, uy, availableLength float64, choice, lineSize int, phaseX, phaseY float64) (ArrowHeadGeometry, bool) {
	norm := math.Hypot(ux, uy)
	if norm <= 1e-12 || availableLength <= 0 {
		return ArrowHeadGeometry{}, false
	}
	ux, uy = ux/norm, uy/norm
	choice = clampArrowHeadChoice(choice)
	wing := arrowHeadWingSteps[choice]
	headLength := 3.0 * wing
	if headLength > availableLength {
		headLength = availableLength
		wing = headLength / 3.0
	}

	px, py := -uy, ux
	offsetX, offsetY := arrowParityOffset(px, py, lineSize)
	tip := ArrowPoint{X: tipX + offsetX + phaseX, Y: tipY + offsetY + phaseY}
	base := ArrowPoint{X: tip.X - ux*headLength, Y: tip.Y - uy*headLength}
	tipHalf := arrowTipHalfWidth(lineSize)
	baseHalf := tipHalf + wing

	return ArrowHeadGeometry{
		Tip:           tip,
		Base:          base,
		SideA:         ArrowPoint{X: base.X + px*baseHalf, Y: base.Y + py*baseHalf},
		SideB:         ArrowPoint{X: base.X - px*baseHalf, Y: base.Y - py*baseHalf},
		UnitX:         ux,
		UnitY:         uy,
		PerpX:         px,
		PerpY:         py,
		Length:        headLength,
		TipHalfWidth:  tipHalf,
		BaseHalfWidth: baseHalf,
		PixelOffsetX:  offsetX,
		PixelOffsetY:  offsetY,
		PhaseOffsetX:  phaseX,
		PhaseOffsetY:  phaseY,
	}, true
}

func calculateArrowHead(x0, y0, x1, y1, choice, lineSize int) (ArrowHeadGeometry, bool) {
	dx := float64(x1 - x0)
	dy := float64(y1 - y0)
	length := math.Hypot(dx, dy)
	if length <= 0 {
		return ArrowHeadGeometry{}, false
	}
	return calculateArrowHeadDirected(float64(x1), float64(y1), dx, dy, length, choice, lineSize, 0, 0)
}

// pointInsideArrowHead tests one sample point against the symmetric 1:3 head.
// backward is measured from the tip toward the tail; lateral is measured across
// the shaft. The tiny epsilon keeps exact boundary samples stable on all CPUs.
func pointInsideArrowHead(p ArrowPoint, g ArrowHeadGeometry) bool {
	rx := p.X - g.Tip.X
	ry := p.Y - g.Tip.Y
	backward := -(rx*g.UnitX + ry*g.UnitY)
	if backward < -1e-9 || backward > g.Length+1e-9 {
		return false
	}
	lateral := math.Abs(rx*g.PerpX + ry*g.PerpY)
	allowed := g.TipHalfWidth + backward/3.0
	return lateral <= allowed+1e-9
}

// arrowShaftLogicalEnd converts the floating rear-centre coordinate back to the
// logical integer point expected by DrawStyledLine. Removing PixelOffset keeps
// even-width raster lines centred on the same half-pixel axis as the head.
func arrowShaftLogicalEnd(g ArrowHeadGeometry) (int, int) {
	return int(math.Round(g.Base.X - g.PixelOffsetX)), int(math.Round(g.Base.Y - g.PixelOffsetY))
}

// arrowShaftHeadOverlap intentionally advances the square shaft a short
// distance inside the filled head.  At exact 45 degrees, rounding the floating
// rear-centre to one raster point can otherwise leave the shaft touching the
// head at only a single diagonal pixel, which reads visually as a cut neck.
// The overlap remains far behind the tip and is fully covered by the wider head.
func arrowShaftHeadOverlap(lineSize int) float64 {
	if lineSize < 1 {
		lineSize = 1
	}
	// 1.5 px is enough to cross the worst half-pixel/diagonal rounding gap.
	// Wider shafts gain a modest extra overlap so their full square cross-section
	// meets the head cleanly without ever approaching the arrow tip.
	return math.Max(1.5, float64(lineSize)/2.0+0.5)
}

// arrowShaftJoinedLogicalEnd returns the logical integer endpoint used by the
// shaft renderer after advancing from the rear edge into the head.  The stored
// parity offset is removed before rounding because DrawArrowShaftLine reapplies
// the same half-pixel axis to both endpoints.
func arrowShaftJoinedLogicalEnd(g ArrowHeadGeometry, lineSize int) (int, int) {
	overlap := arrowShaftHeadOverlap(lineSize)
	x := g.Base.X + g.UnitX*overlap - g.PixelOffsetX
	y := g.Base.Y + g.UnitY*overlap - g.PixelOffsetY
	return int(math.Round(x)), int(math.Round(y))
}

func arrowDirectionAngleDegrees(x0, y0, x1, y1 int) float64 {
	if x0 == x1 && y0 == y1 {
		return 0
	}
	degrees := math.Atan2(float64(-(y1-y0)), float64(x1-x0)) * 180.0 / math.Pi
	for degrees > 180 {
		degrees -= 360
	}
	for degrees <= -180 {
		degrees += 360
	}
	if math.Abs(degrees) < 0.0000001 {
		degrees = 0
	}
	return degrees
}
