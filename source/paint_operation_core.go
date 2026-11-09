package main

// PaintOperation is separate from COLORREF: no RGB value means inversion.
type PaintOperation uint8

const (
	PaintRGB PaintOperation = iota
	PaintTransparent
	PaintInvert
)

func paintMode(op PaintOperation, col uint32) PaintOperation {
	if op == PaintInvert {
		return op
	}
	if isTransparentColor(col) {
		return PaintTransparent
	}
	return PaintRGB
}

// Inversion uses binary coverage >=128. Alpha is preserved, including hidden
// RGB in alpha-zero pixels. An invisible pixel remains invisible.
func invertBGRAPixel(p []byte, i int) {
	p[i] ^= 255
	p[i+1] ^= 255
	p[i+2] ^= 255
}

func objectHasInversion(o *MovableObject) bool {
	return o.Operation == PaintInvert || (o.Kind == ToolText && (len(o.GlyphInvertMask) > 0 || (o.Opaque && o.BackgroundOperation == PaintInvert)))
}

// Each allocated 64x64 tile costs 512 bytes; untouched canvas areas cost zero.
type InvertStroke struct{ tiles map[uint64]*[64]uint64 }

func (s *InvertStroke) Reset() { s.tiles = nil }
func (s *InvertStroke) visit(x, y int) bool {
	if x < 0 || y < 0 {
		return false
	}
	if s.tiles == nil {
		s.tiles = make(map[uint64]*[64]uint64)
	}
	key := uint64(uint32(x>>6))<<32 | uint64(uint32(y>>6))
	t := s.tiles[key]
	if t == nil {
		t = new([64]uint64)
		s.tiles[key] = t
	}
	mask := uint64(1) << uint(x&63)
	row := y & 63
	if t[row]&mask != 0 {
		return false
	}
	t[row] |= mask
	return true
}

func (s *InvertStroke) Stamp(c *Canvas, cx, cy, size int, shape BrushShape) {
	size = max(1, size)
	nib := size
	if shape == BrushSlashUp || shape == BrushSlashDown {
		nib = diagonalBrushFootprintSize(size)
	}
	l, t, r, b := squareStampBounds(cx, cy, nib)
	centerX, centerY := float64(l+r-1)/2, float64(t+b-1)/2
	for y := max(0, t); y < min(c.H, b); y++ {
		for x := max(0, l); x < min(c.W, r); x++ {
			if shape == BrushCircle {
				dx, dy := float64(x)-centerX, float64(y)-centerY
				if dx*dx+dy*dy > float64(size*size)/4 {
					continue
				}
			}
			if shape == BrushSlashUp || shape == BrushSlashDown {
				dx, dy := x-l, y-t
				d := dx - dy
				if shape == BrushSlashUp {
					d = dx + dy - (nib - 1)
				}
				if absInt(d) > diagonalBrushBand(nib) {
					continue
				}
			}
			if s.visit(x, y) {
				invertBGRAPixel(c.Pix, (y*c.W+x)*4)
			}
		}
	}
}

// Indexed strokes use the exact same visited-pixel state as RGB strokes.
// The 16/256-entry complement lookup is built once outside the hot path.
func (s *InvertStroke) StampIndexed(c *Canvas, cx, cy, size int, shape BrushShape, indices []byte, palette []uint32, lookup []byte) {
	size = max(1, size)
	nib := size
	if shape == BrushSlashUp || shape == BrushSlashDown {
		nib = diagonalBrushFootprintSize(size)
	}
	l, t, r, b := squareStampBounds(cx, cy, nib)
	centerX, centerY := float64(l+r-1)/2, float64(t+b-1)/2
	for y := max(0, t); y < min(c.H, b); y++ {
		for x := max(0, l); x < min(c.W, r); x++ {
			if shape == BrushCircle {
				dx, dy := float64(x)-centerX, float64(y)-centerY
				if dx*dx+dy*dy > float64(size*size)/4 {
					continue
				}
			}
			if shape == BrushSlashUp || shape == BrushSlashDown {
				dx, dy := x-l, y-t
				d := dx - dy
				if shape == BrushSlashUp {
					d = dx + dy - (nib - 1)
				}
				if absInt(d) > diagonalBrushBand(nib) {
					continue
				}
			}
			if s.visit(x, y) {
				applyIndexedInvertPixel(c.Pix, y*c.W+x, indices, palette, lookup)
			}
		}
	}
}

func (s *InvertStroke) LineIndexed(c *Canvas, x0, y0, x1, y1, size int, shape BrushShape, indices []byte, palette []uint32, lookup []byte) {
	dx, dy := absInt(x1-x0), -absInt(y1-y0)
	sx, sy := -1, -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		s.StampIndexed(c, x0, y0, size, shape, indices, palette, lookup)
		if x0 == x1 && y0 == y1 {
			break
		}
		e := 2 * err
		if e >= dy {
			err += dy
			x0 += sx
		}
		if e <= dx {
			err += dx
			y0 += sy
		}
	}
}

func (s *InvertStroke) Line(c *Canvas, x0, y0, x1, y1, size int, shape BrushShape) {
	dx, dy := absInt(x1-x0), -absInt(y1-y0)
	sx, sy := -1, -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		s.Stamp(c, x0, y0, size, shape)
		if x0 == x1 && y0 == y1 {
			break
		}
		e := 2 * err
		if e >= dy {
			err += dy
			x0 += sx
		}
		if e <= dx {
			err += dx
			y0 += sy
		}
	}
}

// A frozen inverse is a replacement layer at its original stack position.
// Copying RGBA (instead of alpha-blending it again) prevents double blending.
func freezeInverseObject(base *Canvas, objects []MovableObject, index int) MovableObject {
	o := objects[index]
	w, h := o.W, o.H
	pix := make([]byte, w*h*4)
	mask := make([]byte, (w*h+7)/8)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			gx, gy := o.X+x, o.Y+y
			if gx >= 0 && gy >= 0 && gx < base.W && gy < base.H {
				copy(pix[(y*w+x)*4:][:4], base.Pix[(gy*base.W+gx)*4:][:4])
			}
		}
	}
	compositeObjectsToPixels(pix, w, h, o.X, o.Y, objects[:index+1])
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := objectSpriteOffset(&o, o.X+x, o.Y+y)
			if si >= 0 && (o.Pix[si+3] >= 128 || (o.Kind == ToolText && o.Opaque)) {
				j := y*w + x
				mask[j>>3] |= 1 << uint(j&7)
			}
		}
	}
	o.Pix, o.ReplaceMask, o.SpriteW, o.SpriteH = pix, mask, w, h
	o.Operation, o.BackgroundOperation, o.Transparency = PaintRGB, PaintRGB, 0
	return o
}
