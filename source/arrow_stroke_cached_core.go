package main

import "math"

// arrowPreparedStroke caches segment geometry across raster samples. The
// forward/lateral expressions and inclusive epsilon match pointInsideArrowStroke.
type arrowPreparedStroke struct {
	x0, y0, ux, uy, length, half float64
	valid                        bool
}

func arrowPrepareStroke(x0, y0, x1, y1 float64, lineSize int) arrowPreparedStroke {
	if lineSize < 1 {
		lineSize = 1
	}
	dx, dy := x1-x0, y1-y0
	length := math.Hypot(dx, dy)
	f := arrowPreparedStroke{x0: x0, y0: y0, length: length, half: float64(lineSize) / 2, valid: length > 1e-12}
	if f.valid {
		f.ux, f.uy = dx/length, dy/length
	}
	return f
}

func (f arrowPreparedStroke) coordinates(px, py float64) (forward, lateral float64) {
	rx, ry := px-f.x0, py-f.y0
	return rx*f.ux + ry*f.uy, rx*(-f.uy) + ry*f.ux
}

func (f arrowPreparedStroke) contains(px, py float64) bool {
	if !f.valid {
		return math.Abs(px-f.x0) <= f.half+1e-9 && math.Abs(py-f.y0) <= f.half+1e-9
	}
	forward, lateral := f.coordinates(px, py)
	return forward >= -0.5-1e-9 && forward <= f.length+0.5+1e-9 && math.Abs(lateral) <= f.half+1e-9
}

// Same 8x8 sample grid and comparisons as the original function; the length
// and unit vector are computed once per dragged line, not for every subpixel.
func arrowStrokeLongEdgesCoveragePrepared(px, py float64, f arrowPreparedStroke) float64 {
	if !f.valid || f.length <= 1.0 {
		return 0
	}
	centreForward, _ := f.coordinates(px, py)
	if centreForward <= .5 || centreForward >= f.length-.5 {
		return 0
	}
	const samples = 8
	inside := 0
	for sy := 0; sy < samples; sy++ {
		qy := py - .5 + (float64(sy)+.5)/samples
		for sx := 0; sx < samples; sx++ {
			qx := px - .5 + (float64(sx)+.5)/samples
			forward, lateral := f.coordinates(qx, qy)
			if forward >= .5 && forward <= f.length-.5 && math.Abs(lateral) <= f.half+1e-9 {
				inside++
			}
		}
	}
	return float64(inside) / float64(samples*samples)
}

// Pointer-based forms avoid copying a 16-field arrow head at every 8x8
// subpixel sample; calculations and comparisons match the reference exactly.
func arrowHeadSelectedEdgesContainPrepared(p ArrowPoint, g *ArrowHeadGeometry, edges ArrowHeadEdgeAA, antialiasedEdges bool) bool {
	rx := p.X - g.Tip.X
	ry := p.Y - g.Tip.Y
	backward := -(rx*g.UnitX + ry*g.UnitY)
	lateral := rx*g.PerpX + ry*g.PerpY
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
func pointInsideArrowHeadPrepared(p ArrowPoint, g *ArrowHeadGeometry) bool {
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

// Union-compositing is unchanged; only per-subpixel shaft geometry is cached.
func arrowCompositeCoveragePrepared(px, py float64, hasShaft bool, f arrowPreparedStroke, shaftAA bool, g *ArrowHeadGeometry, headAA bool, edges ArrowHeadEdgeAA) float64 {
	centre := ArrowPoint{X: px, Y: py}
	shaftCentre := hasShaft && f.contains(px, py)
	if shaftCentre && !shaftAA {
		return 1
	}
	// A straight finite shaft is a convex rectangle. If all four corners
	// of the pixel are inside, every 8x8 subpixel is also inside: exact
	// full coverage, without 64 repeated shaft/head calculations.
	if shaftAA && shaftCentre &&
		f.contains(px-.5, py-.5) && f.contains(px+.5, py-.5) &&
		f.contains(px-.5, py+.5) && f.contains(px+.5, py+.5) {
		return 1
	}
	headCentre := pointInsideArrowHeadPrepared(centre, g)
	if headCentre && (!headAA || !edges.any()) {
		return 1
	}
	headHardPass := true
	if headAA && edges.any() {
		headHardPass = arrowHeadSelectedEdgesContainPrepared(centre, g, edges, false)
	}
	const samples = 8
	inside := 0
	for sy := 0; sy < samples; sy++ {
		qy := py - .5 + (float64(sy)+.5)/samples
		for sx := 0; sx < samples; sx++ {
			qx := px - .5 + (float64(sx)+.5)/samples
			shaftInside := false
			if hasShaft {
				if shaftAA {
					shaftInside = f.contains(qx, qy)
				} else {
					shaftInside = shaftCentre
				}
			}
			headInside := false
			if headAA && edges.any() {
				if headHardPass {
					headInside = arrowHeadSelectedEdgesContainPrepared(ArrowPoint{X: qx, Y: qy}, g, edges, true)
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
