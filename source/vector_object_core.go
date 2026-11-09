package main

import "math"

type ObjectVector struct {
	Points                                      [6]ObjectPoint // local start, control points, end
	Count                                       int
	Antialias, Constrained, CreationConstrained bool
	Constraint                                  LineConstraintMode
	ArrowHead                                   int
}

func vectorObjectKind(t Tool) bool { return t == ToolLine || t == ToolArrow || t == ToolCurve }

// Geometry and pixel caches are replaced atomically, never mutated in history.
func renderVectorObject(o MovableObject, canvasW, canvasH int) (MovableObject, bool) {
	if o.Vector.Count < 2 || o.Vector.Count > 6 {
		return o, false
	}
	g := o.Vector
	l, t, r, b := int(^uint(0)>>1), int(^uint(0)>>1), -int(^uint(0)>>1), -int(^uint(0)>>1)
	polygon := make([]PixelPoint, g.Count)
	for i := 0; i < g.Count; i++ {
		p := g.Points[i]
		p.X += o.X
		p.Y += o.Y
		polygon[i] = PixelPoint{p.X, p.Y}
		l = min(l, p.X)
		t = min(t, p.Y)
		r = max(r, p.X)
		b = max(b, p.Y)
	}
	pad := max(8, o.Stroke*2+6)
	if o.Kind == ToolArrow {
		pad += 36
	}
	l -= pad
	t -= pad
	r += pad + 1
	b += pad + 1
	// Retain off-canvas geometry for future editing. Compositing clips at export.
	if !validObjectSize(r-l, b-t) {
		return o, false
	}
	c := &Canvas{W: r - l, H: b - t, Pix: make([]byte, (r-l)*(b-t)*4)}
	col := o.FG
	if o.Operation == PaintInvert {
		col = 0xFFFFFF
	}
	renderer := &vectorRenderer{unbounded: true, canvas: &Canvas{W: canvasW, H: canvasH}, lineConstraintMode: g.Constraint, lineSize: o.Stroke, arrowLineSize: o.Stroke, arrowHeadSize: g.ArrowHead, lineAntialias: g.Antialias, arrowAntialias: g.Antialias}
	p0, p1 := polygon[0], polygon[len(polygon)-1]
	var path []PixelPoint
	switch o.Kind {
	case ToolCurve:
		path = bezierRasterPath(polygon)
		if g.Count == 2 {
			path = appendRasterSegment(nil, p0.X, p0.Y, p1.X, p1.Y)
		}
		c.DrawRoundCurvePixelPath(translatePixelPath(path, -l, -t), o.Stroke, g.Antialias, col)
	case ToolLine, ToolArrow:
		// The saved endpoint is already constrained. Strict-tan still needs the
		// original staircase renderer and its phase, rather than a rounded line.
		strict := g.Constrained && g.Constraint == LineConstraintTanN
		nx, ny := p1.X, p1.Y
		if o.Kind == ToolLine {
			nx, ny = renderer.drawRasterLineAt(c, p0.X, p0.Y, p1.X, p1.Y, strict, col, l, t)
		} else {
			nx, ny = renderer.drawRasterArrowAt(c, p0.X, p0.Y, p1.X, p1.Y, strict, col, l, t)
		}
		polygon[len(polygon)-1] = PixelPoint{nx, ny}
		if strict {
			path, _, _ = renderer.constrainedStrictTanPath(p0.X, p0.Y, nx, ny)
		} else {
			path = appendRasterSegment(nil, p0.X, p0.Y, nx, ny)
		}
	}
	o.X, o.Y, o.W, o.H, o.Pix = l, t, c.W, c.H, c.Pix
	// Zero-length arrows retain an editable empty sprite instead of disappearing.
	if next, ok := trimObjectSprite(o); ok {
		o = next
	}
	o.SpriteW, o.SpriteH = o.W, o.H
	for i, p := range polygon {
		o.Vector.Points[i] = ObjectPoint{p.X - o.X, p.Y - o.Y}
	}
	o.Path = make([]ObjectPoint, len(path))
	for i, p := range path {
		o.Path[i] = ObjectPoint{p.X - o.X, p.Y - o.Y}
	}
	return o, true
}

func vectorObjectContains(o *MovableObject, x, y, tolerance float64) bool {
	if x < float64(o.X)-tolerance || y < float64(o.Y)-tolerance || x > float64(o.X+o.W)+tolerance || y > float64(o.Y+o.H)+tolerance {
		return false
	}
	if objectSpriteOffset(o, int(math.Floor(x)), int(math.Floor(y))) >= 0 && o.Kind == ToolArrow {
		si := objectSpriteOffset(o, int(math.Floor(x)), int(math.Floor(y)))
		if o.Pix[si+3] != 0 {
			return true
		}
	}
	radius := tolerance + float64(max(1, o.Stroke))/2
	for i, p := range o.Path {
		q := p
		if i > 0 {
			q = o.Path[i-1]
		}
		if pointSegmentDistance(x-float64(o.X), y-float64(o.Y), float64(q.X), float64(q.Y), float64(p.X), float64(p.Y)) <= radius {
			return true
		}
	}
	return false
}

const ObjectVertex0 ObjectHandle = 9

func objectVertexPoints(o *MovableObject, zoom, ox, oy int) []ObjectPoint {
	out := make([]ObjectPoint, o.Vector.Count)
	for i := range out {
		p := o.Vector.Points[i]
		out[i] = ObjectPoint{ox + (o.X+p.X)*zoom, oy + (o.Y+p.Y)*zoom}
	}
	return out
}

func hitVectorHandle(o *MovableObject, zoom, ox, oy, mx, my int) ObjectHandle {
	best, bestDist := ObjectBody, int(^uint(0)>>1)
	for i, p := range objectVertexPoints(o, zoom, ox, oy) {
		dx, dy := mx-p.X, my-p.Y
		d := dx*dx + dy*dy
		if absInt(dx) <= 4 && absInt(dy) <= 4 && d < bestDist {
			best, bestDist = ObjectVertex0+ObjectHandle(i), d
		}
	}
	return best
}
