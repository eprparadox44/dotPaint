package main

import "math"

type strictTanStep struct {
	X int
	Y int
}

// Strict-tan menu ratios never change during a drawing session. Build the
// directions and their norms once instead of rebuilding a heap-backed
// candidate list and recomputing every norm on each mouse move. Preserve the
// exact historical ordering and floating-point expression to keep tie choices.
type strictTanDirectionCandidate struct {
	step strictTanStep
	norm float64
}

var strictTanDirectionCandidates = func() []strictTanDirectionCandidate {
	steps := []strictTanStep{{X: 1, Y: 0}, {X: -1, Y: 0}, {X: 0, Y: 1}, {X: 0, Y: -1}}
	for _, ratio := range lineTanRatios {
		for _, sx := range []int{-1, 1} {
			for _, sy := range []int{-1, 1} {
				steps = append(steps, strictTanStep{X: sx * ratio.Run, Y: sy * ratio.Rise})
			}
		}
	}
	out := make([]strictTanDirectionCandidate, len(steps))
	for i, d := range steps {
		out[i] = strictTanDirectionCandidate{step: d, norm: math.Hypot(float64(d.X), float64(d.Y))}
	}
	return out
}()

func nearestStrictTanStep(dx, dy float64) strictTanStep {
	length := math.Hypot(dx, dy)
	if length <= 0 {
		return strictTanStep{}
	}
	ux, uy := dx/length, dy/length
	best := strictTanDirectionCandidates[0].step
	bestDot := -2.0
	for _, c := range strictTanDirectionCandidates {
		d := c.step
		// Keep operator order byte-for-byte equivalent to the original comparator.
		dot := ux*float64(d.X)/c.norm + uy*float64(d.Y)/c.norm
		if dot > bestDot {
			bestDot = dot
			best = d
		}
	}
	return best
}

func nearestLineDirection(dx, dy float64, mode LineConstraintMode) (float64, float64) {
	length := math.Hypot(dx, dy)
	if length <= 0 {
		return 0, 0
	}
	if mode == LineConstraint15Degrees {
		step := math.Pi / 12.0
		angle := math.Round(math.Atan2(dy, dx)/step) * step
		return math.Cos(angle), math.Sin(angle)
	}
	d := nearestStrictTanStep(dx, dy)
	norm := math.Hypot(float64(d.X), float64(d.Y))
	if norm <= 0 {
		return 0, 0
	}
	return float64(d.X) / norm, float64(d.Y) / norm
}

// strictTanPointAt returns the pixel reached after exactly majorSteps one-pixel
// advances along the chosen integer-ratio staircase. For tan 1/3 the sequence
// is (0,0), (1,0), (2,0), (3,1), (4,1), (5,1), (6,2)... . Thus the body keeps
// the exact three-horizontal-per-one-vertical rhythm while a live drag can grow
// by one canvas pixel instead of waiting for a complete 3:1 block.
func strictTanPointAt(x0, y0 int, step strictTanStep, majorSteps int) PixelPoint {
	if majorSteps <= 0 || (step.X == 0 && step.Y == 0) {
		return PixelPoint{X: x0, Y: y0}
	}
	ax, ay := absInt(step.X), absInt(step.Y)
	sx, sy := 0, 0
	if step.X < 0 {
		sx = -1
	} else if step.X > 0 {
		sx = 1
	}
	if step.Y < 0 {
		sy = -1
	} else if step.Y > 0 {
		sy = 1
	}
	if ax >= ay {
		yOff := 0
		if ax > 0 {
			yOff = (majorSteps * ay) / ax
		}
		return PixelPoint{X: x0 + sx*majorSteps, Y: y0 + sy*yOff}
	}
	xOff := 0
	if ay > 0 {
		xOff = (majorSteps * ax) / ay
	}
	return PixelPoint{X: x0 + sx*xOff, Y: y0 + sy*majorSteps}
}

// strictTanMajorStepsFromPointer uses the dominant canvas axis as the live
// length control. A shallow tan 1/3 or 1/4 line therefore gains exactly one
// path pixel whenever the pointer advances one canvas pixel horizontally; a
// steep line does the same vertically. The minor pointer coordinate chooses
// the nearest allowed ratio but cannot make the endpoint jump by a whole motif.
func strictTanMajorStepsFromPointer(x0, y0, x1, y1 int, step strictTanStep) int {
	ax, ay := absInt(step.X), absInt(step.Y)
	if ax == 0 && ay == 0 {
		return 0
	}
	if ax >= ay {
		if step.X < 0 {
			return max(0, x0-x1)
		}
		return max(0, x1-x0)
	}
	if step.Y < 0 {
		return max(0, y0-y1)
	}
	return max(0, y1-y0)
}

// maxStrictTanMajorSteps clips the staircase without changing its phase. The
// dominant coordinate advances once per raster step; the minor coordinate is
// floor(k*minor/major), so the formulas below find the last k still inside the
// canvas exactly rather than clipping a completed line afterward.
func maxStrictTanMajorSteps(x0, y0 int, step strictTanStep, width, height int) int {
	ax, ay := absInt(step.X), absInt(step.Y)
	if ax == 0 && ay == 0 {
		return 0
	}
	xAvail, yAvail := 0, 0
	if step.X > 0 {
		xAvail = width - 1 - x0
	} else if step.X < 0 {
		xAvail = x0
	} else {
		xAvail = int(^uint(0) >> 2)
	}
	if step.Y > 0 {
		yAvail = height - 1 - y0
	} else if step.Y < 0 {
		yAvail = y0
	} else {
		yAvail = int(^uint(0) >> 2)
	}
	if ax >= ay {
		limit := xAvail
		if ay > 0 {
			minorLimit := ((yAvail+1)*ax - 1) / ay
			limit = min(limit, minorLimit)
		}
		return max(0, limit)
	}
	limit := yAvail
	if ax > 0 {
		minorLimit := ((xAvail+1)*ay - 1) / ax
		limit = min(limit, minorLimit)
	}
	return max(0, limit)
}

// strictTanPixelPrefix returns every successive native canvas pixel from the
// starting point through majorSteps. A partial final motif is intentional: it
// is what lets the preview and committed line grow one dot at a time. All full
// rows/columns before that tail still obey the selected exact integer ratio.
func strictTanPixelPrefix(x0, y0 int, step strictTanStep, majorSteps int) []PixelPoint {
	majorSteps = max(0, majorSteps)
	points := make([]PixelPoint, 0, majorSteps+1)
	for k := 0; k <= majorSteps; k++ {
		points = append(points, strictTanPointAt(x0, y0, step, k))
	}
	return points
}

func translatePixelPath(points []PixelPoint, dx, dy int) []PixelPoint {
	if len(points) == 0 || (dx == 0 && dy == 0) {
		return points
	}
	out := make([]PixelPoint, len(points))
	for i, p := range points {
		out[i] = PixelPoint{X: p.X + dx, Y: p.Y + dy}
	}
	return out
}

func truncateArrowPixelPath(points []PixelPoint, x0, y0 int, head ArrowHeadGeometry, lineSize int) []PixelPoint {
	if len(points) == 0 {
		return nil
	}
	fullLength := math.Hypot(float64(points[len(points)-1].X-x0), float64(points[len(points)-1].Y-y0))
	shaftLength := fullLength - head.Length
	if shaftLength <= 0 {
		return nil
	}
	last := -1
	for i, p := range points {
		forward := float64(p.X-x0)*head.UnitX + float64(p.Y-y0)*head.UnitY
		if forward <= shaftLength+arrowShaftHeadOverlap(lineSize) {
			last = i
			continue
		}
		break
	}
	if last < 0 {
		return nil
	}
	return points[:last+1]
}

func translateArrowHeadGeometry(g ArrowHeadGeometry, dx, dy float64) ArrowHeadGeometry {
	g.Tip.X, g.Tip.Y = g.Tip.X+dx, g.Tip.Y+dy
	g.Base.X, g.Base.Y = g.Base.X+dx, g.Base.Y+dy
	g.SideA.X, g.SideA.Y = g.SideA.X+dx, g.SideA.Y+dy
	g.SideB.X, g.SideB.Y = g.SideB.X+dx, g.SideB.Y+dy
	return g
}

func translateStrictTanShadow(s strictTanShadowSegment, dx, dy float64) strictTanShadowSegment {
	s.X0, s.Y0 = s.X0+dx, s.Y0+dy
	s.X1, s.Y1 = s.X1+dx, s.Y1+dy
	return s
}

func rectangleOuterBounds(x0, y0, x1, y1 int) (left, top, right, bottom, width, height int) {
	left, right = x0, x1
	if left > right {
		left, right = right, left
	}
	top, bottom = y0, y1
	if top > bottom {
		top, bottom = bottom, top
	}
	width = right - left + 1
	height = bottom - top + 1
	return
}
func (a *vectorRenderer) constrainedStrictTanGeometry(x0, y0, x1, y1 int) ([]PixelPoint, int, int, strictTanStep, int) {
	dx, dy := float64(x1-x0), float64(y1-y0)
	step := nearestStrictTanStep(dx, dy)
	majorSteps := strictTanMajorStepsFromPointer(x0, y0, x1, y1, step)
	majorSteps = clamp(majorSteps, 0, maxStrictTanMajorSteps(x0, y0, step, a.canvas.W, a.canvas.H))
	points := strictTanPixelPrefix(x0, y0, step, majorSteps)
	last := points[len(points)-1]
	return points, last.X, last.Y, step, majorSteps
}
func (a *vectorRenderer) constrainedStrictTanPath(x0, y0, x1, y1 int) ([]PixelPoint, int, int) {
	points, nx, ny, _, _ := a.constrainedStrictTanGeometry(x0, y0, x1, y1)
	return points, nx, ny
}
func (a *vectorRenderer) constrainedLineEndpoint(x0, y0, x1, y1 int, constrain bool) (int, int) {
	if a.unbounded && !constrain {
		return x1, y1
	}
	if !constrain || (x0 == x1 && y0 == y1) {
		return clamp(x1, 0, a.canvas.W-1), clamp(y1, 0, a.canvas.H-1)
	}
	dx, dy := float64(x1-x0), float64(y1-y0)
	length := math.Hypot(dx, dy)
	if a.lineConstraintMode == LineConstraintTanN {
		_, nx, ny := a.constrainedStrictTanPath(x0, y0, x1, y1)
		return nx, ny
	}
	ux, uy := nearestLineDirection(dx, dy, a.lineConstraintMode)
	if ux == 0 && uy == 0 {
		return x0, y0
	}
	maxLength := length
	if ux > 0 {
		maxLength = math.Min(maxLength, float64(a.canvas.W-1-x0)/ux)
	} else if ux < 0 {
		maxLength = math.Min(maxLength, float64(0-x0)/ux)
	}
	if uy > 0 {
		maxLength = math.Min(maxLength, float64(a.canvas.H-1-y0)/uy)
	} else if uy < 0 {
		maxLength = math.Min(maxLength, float64(0-y0)/uy)
	}
	if maxLength < 0 {
		maxLength = 0
	}
	nx := x0 + int(math.Round(ux*maxLength))
	ny := y0 + int(math.Round(uy*maxLength))
	return clamp(nx, 0, a.canvas.W-1), clamp(ny, 0, a.canvas.H-1)
}
func (a *vectorRenderer) drawRasterLineAt(c *Canvas, x0, y0, x1, y1 int, constrain bool, col uint32, originX, originY int) (int, int) {
	if constrain && a.lineConstraintMode == LineConstraintTanN {
		points, nx, ny, step, majorSteps := a.constrainedStrictTanGeometry(x0, y0, x1, y1)
		points = translatePixelPath(points, -originX, -originY)
		c.DrawStyledStrictTanPath(points, step.X, step.Y, majorSteps, a.lineSize, a.lineAntialias, col)
		return nx, ny
	}
	nx, ny := a.constrainedLineEndpoint(x0, y0, x1, y1, constrain)
	c.DrawStyledLine(x0-originX, y0-originY, nx-originX, ny-originY, a.lineSize, a.lineAntialias, col)
	return nx, ny
}
func (a *vectorRenderer) constrainedRectangleEndpoint(x0, y0, x1, y1 int, square bool) (int, int) {
	x1 = clamp(x1, 0, a.canvas.W-1)
	y1 = clamp(y1, 0, a.canvas.H-1)
	if !square || (x0 == x1 && y0 == y1) {
		return x1, y1
	}
	dx, dy := x1-x0, y1-y0
	sx, sy := 1, 1
	if dx < 0 {
		sx = -1
	}
	if dy < 0 {
		sy = -1
	}
	side := max(absInt(dx), absInt(dy))
	maxX := a.canvas.W - 1 - x0
	if sx < 0 {
		maxX = x0
	}
	maxY := a.canvas.H - 1 - y0
	if sy < 0 {
		maxY = y0
	}
	side = min(side, min(maxX, maxY))
	return x0 + sx*side, y0 + sy*side
}
func (a *vectorRenderer) drawRasterRectangleAt(c *Canvas, x0, y0, x1, y1 int, square bool, col uint32, originX, originY int) (int, int) {
	nx, ny := a.constrainedRectangleEndpoint(x0, y0, x1, y1, square)
	c.DrawRectOuter(x0-originX, y0-originY, nx-originX, ny-originY, a.rectangleSize, col)
	if a.shapeShowCenter {
		left, top, _, _, width, height := rectangleOuterBounds(x0, y0, nx, ny)
		c.DrawCenterMarker(left-originX, top-originY, width, height, col)
	}
	return nx, ny
}
func (a *vectorRenderer) drawRasterEllipseAt(c *Canvas, x0, y0, x1, y1 int, circle bool, col uint32, originX, originY int) (int, int) {
	nx, ny := a.constrainedRectangleEndpoint(x0, y0, x1, y1, circle)
	c.DrawEllipseOuter(x0-originX, y0-originY, nx-originX, ny-originY, a.ellipseSize, col)
	if a.shapeShowCenter {
		left, top, _, _, width, height := rectangleOuterBounds(x0, y0, nx, ny)
		c.DrawCenterMarker(left-originX, top-originY, width, height, col)
	}
	return nx, ny
}
func (a *vectorRenderer) constrainedRightTriangleEndpoint(x0, y0, x1, y1 int, constrain bool) (int, int) {
	return a.constrainedLineEndpoint(x0, y0, x1, y1, constrain)
}
func (a *vectorRenderer) drawRasterRightTriangleAt(c *Canvas, x0, y0, x1, y1 int, constrain bool, col uint32, originX, originY int) (int, int) {
	nx, ny := x1, y1
	if constrain && a.lineConstraintMode == LineConstraintTanN {
		// The triangle hypotenuse must use the SAME strict-tan renderer as
		// ToolLine. The old per-microsegment square caps in DrawStyledPixelPath
		// protruded as black "feathers" at thick triangle endpoints. The
		// global tan-direction butt-cap clip removes these protrusions while
		// keeping the exact integer staircase and the existing zero-AA policy.
		points, tx, ty, step, majorSteps := a.constrainedStrictTanGeometry(x0, y0, x1, y1)
		nx, ny = tx, ty
		c.DrawStyledStrictTanPath(translatePixelPath(points, -originX, -originY), step.X, step.Y, majorSteps, a.rightTriangleSize, false, col)
	} else {
		nx, ny = a.constrainedRightTriangleEndpoint(x0, y0, x1, y1, constrain)
		c.DrawStyledLine(x0-originX, y0-originY, nx-originX, ny-originY, a.rightTriangleSize, false, col)
	}

	cornerX, cornerY := rightTriangleLegCorner(x0, y0, nx, ny)
	c.DrawStyledLine(x0-originX, y0-originY, cornerX-originX, cornerY-originY, a.rightTriangleSize, false, col)
	c.DrawStyledLine(cornerX-originX, cornerY-originY, nx-originX, ny-originY, a.rightTriangleSize, false, col)
	return nx, ny
}
func (a *vectorRenderer) drawRasterArrowAt(c *Canvas, x0, y0, x1, y1 int, constrain bool, col uint32, originX, originY int) (int, int) {
	nx, ny := x1, y1
	var strictPath []PixelPoint
	var strictStep strictTanStep
	majorSteps := 0
	strictMode := constrain && a.lineConstraintMode == LineConstraintTanN
	if strictMode {
		strictPath, nx, ny, strictStep, majorSteps = a.constrainedStrictTanGeometry(x0, y0, x1, y1)
	} else {
		nx, ny = a.constrainedLineEndpoint(x0, y0, x1, y1, constrain)
	}

	var head ArrowHeadGeometry
	var ok bool
	var fullShadow strictTanShadowSegment
	shadowOK := false
	if strictMode {
		fullShadow, shadowOK = calculateStrictTanShadowSegment(x0, y0, strictStep.X, strictStep.Y, majorSteps, a.arrowLineSize)
		if shadowOK {
			head, ok = calculateArrowHeadDirected(
				float64(nx), float64(ny),
				fullShadow.UnitX, fullShadow.UnitY, fullShadow.Length,
				a.arrowHeadSize, a.arrowLineSize,
				fullShadow.PhaseOffsetX, fullShadow.PhaseOffsetY,
			)
		}
	} else {
		head, ok = calculateArrowHead(x0, y0, nx, ny, a.arrowHeadSize, a.arrowLineSize)
	}
	if !ok {
		return nx, ny
	}

	shaftAA := arrowEffectiveAntialias(a.arrowAntialias, x0, y0, nx, ny)
	if strictMode {
		shaftAA = arrowEffectiveAntialias(a.arrowAntialias, 0, 0, strictStep.X, strictStep.Y)
	}
	sx, sy := arrowShaftJoinedLogicalEnd(head, a.arrowLineSize)
	hasShaft := math.Hypot(float64(sx-x0), float64(sy-y0)) > 0.01
	if !hasShaft {
		sx, sy = x0, y0
	}
	var shaftPath []PixelPoint
	if len(strictPath) > 0 {
		shaftPath = truncateArrowPixelPath(strictPath, x0, y0, head, a.arrowLineSize)
	}

	var shaftShadow *strictTanShadowSegment
	if strictMode && shadowOK && hasShaft {
		shaftLength := fullShadow.Length - head.Length + arrowShaftHeadOverlap(a.arrowLineSize)
		if shaftLength > 0 {
			trimmed := fullShadow.prefix(shaftLength)
			shaftShadow = &trimmed
		}
	}
	localHead := translateArrowHeadGeometry(head, -float64(originX), -float64(originY))
	localPath := translatePixelPath(shaftPath, -originX, -originY)
	var localShadow *strictTanShadowSegment
	if shaftShadow != nil {
		translated := translateStrictTanShadow(*shaftShadow, -float64(originX), -float64(originY))
		localShadow = &translated
	}
	c.DrawUnifiedVectorArrow(x0-originX, y0-originY, sx-originX, sy-originY, a.arrowLineSize, shaftAA, localHead, a.arrowAntialias, localPath, localShadow, col)
	return nx, ny
}

type vectorRenderer struct {
	unbounded bool // preserved off-canvas object geometry only

	canvas                                                                                *Canvas
	lineConstraintMode                                                                    LineConstraintMode
	lineSize, rectangleSize, ellipseSize, rightTriangleSize, arrowLineSize, arrowHeadSize int
	lineAntialias, arrowAntialias, shapeShowCenter                                        bool
}
