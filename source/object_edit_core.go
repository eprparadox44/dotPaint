package main

// No Win32 calls live here: the same geometry, picking, compositing and order
// functions are exercised on the host and used by the Windows front end.
type ObjectHandle uint8

const (
	ObjectBody ObjectHandle = iota
	ObjectNW
	ObjectN
	ObjectNE
	ObjectE
	ObjectSE
	ObjectS
	ObjectSW
	ObjectW
)

type ObjectPoint struct{ X, Y int }
type ObjectBounds struct{ X, Y, W, H int }

// Keep the original stroke margins (native pixels) rather than stretching
// them with the box. This keeps a resized triangle against its eight handles
// and retains the stroke's thickness and endpoint orientation.
func resizedShapeAxis(v0, v1, oldSize, newSize int) (int, int) {
	if newSize <= 1 {
		return 0, 0
	}
	left := clampBlit(min(v0, v1), 0, (newSize-1)/2)
	rightMargin := clampBlit(oldSize-1-max(v0, v1), 0, (newSize-1)/2)
	right := max(left, newSize-1-rightMargin)
	if v0 > v1 {
		return right, left
	}
	return left, right
}

const (
	objectMaxDimension = 20000
	objectMaxPixels    = 40000000
	objectHandleRadius = 4 // client pixels: independent of zoom
)

func (o MovableObject) bounds() ObjectBounds { return ObjectBounds{o.X, o.Y, o.W, o.H} }
func (o MovableObject) spriteSize() (int, int) {
	w, h := o.SpriteW, o.SpriteH
	if w == 0 || h == 0 {
		w, h = o.W, o.H
	}
	return w, h
}

func validObjectSize(w, h int) bool {
	return w > 0 && h > 0 && w <= objectMaxDimension && h <= objectMaxDimension && int64(w)*int64(h) <= objectMaxPixels
}

func objectHandlePoints(b ObjectBounds, zoom, ox, oy int) [8]ObjectPoint {
	x0, y0 := ox+b.X*zoom-6, oy+b.Y*zoom-6
	x1, y1 := ox+(b.X+b.W)*zoom+5, oy+(b.Y+b.H)*zoom+5
	xm, ym := (x0+x1)/2, (y0+y1)/2
	return [8]ObjectPoint{{x0, y0}, {xm, y0}, {x1, y0}, {x1, ym}, {x1, y1}, {xm, y1}, {x0, y1}, {x0, ym}}
}

func hitObjectHandle(b ObjectBounds, zoom, ox, oy, mx, my int) ObjectHandle {
	best, bestDist := ObjectBody, 1<<30
	for i, p := range objectHandlePoints(b, zoom, ox, oy) {
		dx, dy := mx-p.X, my-p.Y
		if dx < -objectHandleRadius || dx > objectHandleRadius || dy < -objectHandleRadius || dy > objectHandleRadius {
			continue
		}
		d := dx*dx + dy*dy
		if d < bestDist {
			best, bestDist = ObjectHandle(i+1), d
		}
	}
	return best
}

// Crossing the opposite edge pins to one pixel instead of flipping or
// dividing by zero. Invalid large allocations leave the last valid preview.
func resizedObjectBounds(b ObjectBounds, handle ObjectHandle, dx, dy int) (ObjectBounds, bool) {
	l, t, r, bot := b.X, b.Y, b.X+b.W, b.Y+b.H
	switch handle {
	case ObjectNW, ObjectW, ObjectSW:
		l = clampBlit(l+dx, r-objectMaxDimension, r-1)
	case ObjectNE, ObjectE, ObjectSE:
		r = clampBlit(r+dx, l+1, l+objectMaxDimension)
	}
	switch handle {
	case ObjectNW, ObjectN, ObjectNE:
		t = clampBlit(t+dy, bot-objectMaxDimension, bot-1)
	case ObjectSW, ObjectS, ObjectSE:
		bot = clampBlit(bot+dy, t+1, t+objectMaxDimension)
	}
	out := ObjectBounds{l, t, r - l, bot - t}
	return out, handle != ObjectBody && validObjectSize(out.W, out.H)
}

func objectSpriteOffset(o *MovableObject, x, y int) int {
	w, h := o.spriteSize()
	if o.W <= 0 || o.H <= 0 || w <= 0 || h <= 0 || len(o.Pix) != w*h*4 || x < o.X || y < o.Y || x >= o.X+o.W || y >= o.Y+o.H {
		return -1
	}
	sx, sy := (x-o.X)*w/o.W, (y-o.Y)*h/o.H
	return (sy*w + sx) * 4
}

func objectBodyContains(o *MovableObject, x, y int) bool {
	if vectorObjectKind(o.Kind) {
		return vectorObjectContains(o, float64(x), float64(y), 0)
	}
	si := objectSpriteOffset(o, x, y)
	if si < 0 {
		return false
	}
	// Visible outlines always count. Hollow shapes also select by their
	// geometric interior; empty ellipse corners and triangle exteriors pass
	// through. Text boxes select by their full formatting rectangle.
	if o.Pix[si+3] != 0 {
		return true
	}
	switch o.Kind {
	case ToolEllipse:
		dx, dy := float64(2*(x-o.X)+1-o.W)/float64(o.W), float64(2*(y-o.Y)+1-o.H)/float64(o.H)
		return dx*dx+dy*dy <= 1
	case ToolRightTriangle:
		u, v := float64(x-o.X)+0.5, float64(y-o.Y)+0.5
		if o.ShapeX1 < o.ShapeX0 {
			u = float64(o.W) - u
		}
		if o.ShapeY1 < o.ShapeY0 {
			v = float64(o.H) - v
		}
		return v/float64(o.H) <= u/float64(o.W)
	default:
		return true
	}
}

func hitObjectBody(objects []MovableObject, x, y int) int {
	for i := len(objects) - 1; i >= 0; i-- {
		if !objects[i].Rasterized && objectBodyContains(&objects[i], x, y) {
			return i
		}
	}
	return -1
}

// A top object's body takes precedence over a lower object's handles.
func hitObjectAtClient(objects []MovableObject, zoom, ox, oy, mx, my int) (int, ObjectHandle) {
	if zoom <= 0 {
		return -1, ObjectBody
	}
	x, y := floorDivInt(mx-ox, zoom), floorDivInt(my-oy, zoom)
	for i := len(objects) - 1; i >= 0; i-- {
		if objects[i].Rasterized {
			continue
		}
		h := ObjectBody
		if vectorObjectKind(objects[i].Kind) {
			h = hitVectorHandle(&objects[i], zoom, ox, oy, mx, my)
		} else {
			h = hitObjectHandle(objects[i].bounds(), zoom, ox, oy, mx, my)
		}
		if h != ObjectBody {
			return i, h
		}
		if (vectorObjectKind(objects[i].Kind) && vectorObjectContains(&objects[i], float64(mx-ox)/float64(zoom), float64(my-oy)/float64(zoom), 5/float64(zoom))) || (!vectorObjectKind(objects[i].Kind) && objectBodyContains(&objects[i], x, y)) {
			return i, ObjectBody
		}
	}
	return -1, ObjectBody
}

// Return the final selected index; moving a slice item never reorders the
// remaining items relative to one another.
func reorderObject(objects []MovableObject, from, to int) (int, bool) {
	if from < 0 || from >= len(objects) || to < 0 || to >= len(objects) || from == to {
		return from, false
	}
	o := objects[from]
	if from < to {
		copy(objects[from:to], objects[from+1:to+1])
	} else {
		copy(objects[to+1:from+1], objects[to:from])
	}
	objects[to] = o
	return to, true
}

func blendObjectPixel(dst []byte, di int, o *MovableObject, si int) {
	if si < 0 {
		return
	}
	if len(o.ReplaceMask) > 0 {
		j := si / 4
		if j>>3 < len(o.ReplaceMask) && o.ReplaceMask[j>>3]&(1<<uint(j&7)) != 0 {
			copy(dst[di:di+4], o.Pix[si:si+4])
		}
		return
	}
	if o.Kind == ToolText && len(o.GlyphInvertMask) > 0 {
		if o.Opaque && o.BackgroundOperation == PaintInvert {
			invertBGRAPixel(dst, di)
		} else if o.Opaque && o.BackgroundOperation == PaintRGB {
			bg := colorrefToBGRA(o.BG)
			blendBGRA(dst, di, bg[:], 0)
		}
		if si/4 < len(o.GlyphInvertMask) && o.GlyphInvertMask[si/4] >= 128 {
			invertBGRAPixel(dst, di)
		} else {
			blendBGRA(dst, di, o.Pix, si)
		}
		return
	}
	if o.Kind == ToolText && objectHasInversion(o) {
		if o.Opaque && o.BackgroundOperation != PaintTransparent {
			if o.BackgroundOperation == PaintInvert {
				invertBGRAPixel(dst, di)
			} else {
				bg := colorrefToBGRA(o.BG)
				blendBGRA(dst, di, bg[:], 0)
			}
		}
		if o.Operation == PaintInvert {
			if o.Pix[si+3] >= 128 {
				invertBGRAPixel(dst, di)
			}
		} else if o.Operation != PaintTransparent {
			blendBGRA(dst, di, o.Pix, si)
		}
		return
	}
	if o.Operation == PaintTransparent {
		return
	}
	if o.Operation == PaintInvert {
		if o.Pix[si+3] >= 128 {
			invertBGRAPixel(dst, di)
		}
		return
	}
	if o.Transparency == 0 {
		blendBGRA(dst, di, o.Pix, si)
		return
	}
	if o.Transparency >= 100 {
		return
	}
	src := [4]byte{o.Pix[si], o.Pix[si+1], o.Pix[si+2], byte((int(o.Pix[si+3])*int(100-o.Transparency) + 50) / 100)}
	blendBGRA(dst, di, src[:], 0)
}

func compositeObjectsToPixels(dst []byte, dstW, dstH, offsetX, offsetY int, objects []MovableObject) {
	if len(objects) == 0 {
		return
	}
	compositeObjectRows(dst, dstW, dstH, dstW, offsetX, offsetY, objects, -1)
}

// stride can exceed the clipped tile width. Fast unscaled sprites retain the
// old contiguous scanline path; scaling is used only by a live resize preview.
func compositeObjectRows(dst []byte, w, h, stride, ox, oy int, objects []MovableObject, skip int) {
	if w <= 0 || h <= 0 || stride < w || len(dst) < ((h-1)*stride+w)*4 {
		return
	}
	for i := range objects {
		if i == skip {
			continue
		}
		o := &objects[i]
		sw, sh := o.spriteSize()
		if o.Transparency >= 100 || o.W <= 0 || o.H <= 0 || sw <= 0 || sh <= 0 || len(o.Pix) != sw*sh*4 {
			continue
		}
		x0, y0 := max(ox, o.X), max(oy, o.Y)
		x1, y1 := min(ox+w, o.X+o.W), min(oy+h, o.Y+o.H)
		if x1 <= x0 || y1 <= y0 {
			continue
		}
		for y := y0; y < y1; y++ {
			di := ((y-oy)*stride + x0 - ox) * 4
			if sw == o.W && sh == o.H {
				si := ((y-o.Y)*sw + x0 - o.X) * 4
				if o.Transparency == 0 && o.Operation == PaintRGB && !objectHasInversion(o) && len(o.GlyphInvertMask) == 0 && len(o.ReplaceMask) == 0 {
					for x := x0; x < x1; x++ {
						blendBGRA(dst, di, o.Pix, si)
						di += 4
						si += 4
					}
				} else {
					for x := x0; x < x1; x++ {
						blendObjectPixel(dst, di, o, si)
						di += 4
						si += 4
					}
				}
			} else {
				for x := x0; x < x1; x++ {
					blendObjectPixel(dst, di, o, objectSpriteOffset(o, x, y))
					di += 4
				}
			}
		}
	}
}
