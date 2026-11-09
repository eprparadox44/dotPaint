package main

// PixelSelectionCut models the actual selected pixels rather than the selection
// rectangle alone. An absent full-size Mask means that every pixel is selected.
type PixelSelectionCut struct {
	X, Y, W, H int
	Mask       []bool
}

func (s PixelSelectionCut) contains(x, y int) bool {
	sx, sy := x-s.X, y-s.Y
	if sx < 0 || sy < 0 || sx >= s.W || sy >= s.H {
		return false
	}
	if len(s.Mask) == s.W*s.H {
		return s.Mask[sy*s.W+sx]
	}
	return true
}

func objectPaintsPixel(o *MovableObject, x, y int) bool {
	if o.W <= 0 || o.H <= 0 || o.Transparency >= 100 {
		return false
	}
	si := objectSpriteOffset(o, x, y)
	if si < 0 {
		return false
	}
	if len(o.ReplaceMask) != 0 {
		j := si / 4
		return j>>3 < len(o.ReplaceMask) && o.ReplaceMask[j>>3]&(1<<uint(j&7)) != 0
	}
	if o.Kind == ToolText && len(o.GlyphInvertMask) != 0 {
		if o.Opaque && o.BackgroundOperation != PaintTransparent {
			return true
		}
		return (si/4 < len(o.GlyphInvertMask) && o.GlyphInvertMask[si/4] >= 128) || o.Pix[si+3] > 0
	}
	if o.Kind == ToolText && objectHasInversion(o) {
		if o.Opaque && o.BackgroundOperation != PaintTransparent {
			return true
		}
		if o.Operation == PaintInvert {
			return o.Pix[si+3] >= 128
		}
		return o.Operation != PaintTransparent && o.Pix[si+3] > 0
	}
	if o.Operation == PaintTransparent {
		return false
	}
	if o.Operation == PaintInvert {
		return o.Pix[si+3] >= 128
	}
	// The normal RGB compositor rounds very faint alpha to zero after
	// applying object opacity. Such pixels are not actually painted.
	return (int(o.Pix[si+3])*int(100-o.Transparency)+50)/100 > 0
}

// Returns a tight bounding rectangle for the potentially visible part of an
// object in the current canvas. Pixels beyond the canvas cannot be deleted.
func clippedObjectBounds(o *MovableObject, w, h int) (int, int, int, int) {
	return max(0, o.X), max(0, o.Y), min(w, o.X+o.W), min(h, o.Y+o.H)
}

func objectTouchedByPixelSelection(o *MovableObject, s PixelSelectionCut, canvasW, canvasH int) bool {
	x0, y0, x1, y1 := clippedObjectBounds(o, canvasW, canvasH)
	x0, y0 = max(x0, s.X), max(y0, s.Y)
	x1, y1 = min(x1, s.X+s.W), min(y1, s.Y+s.H)
	if x0 >= x1 || y0 >= y1 {
		return false
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if s.contains(x, y) && objectPaintsPixel(o, x, y) {
				return true
			}
		}
	}
	return false
}

// cutSelectedObjectPixels converts only the objects whose *painted pixels*
// intersect the selection to locked raster layers, and removes only the
// selected part of those layers. Unaffected objects preserve their identity,
// order, grouping, editability, geometry and original immutable pixel cache.
//
// Ordinary RGB shapes retain true-alpha sprites. Inversion and replacement
// layers freeze the visible stack at the time of deletion, so they do not
// re-invert or double-blend. All snapshots refer to the original object stack.
func cutSelectedObjectPixels(base *Canvas, objects []MovableObject, s PixelSelectionCut) ([]MovableObject, []int) {
	if base == nil || base.W < 1 || base.H < 1 || len(objects) == 0 || s.W <= 0 || s.H <= 0 {
		return objects, nil
	}
	var result []MovableObject
	var affected []int
	for i := range objects {
		o := &objects[i]
		if !objectTouchedByPixelSelection(o, s, base.W, base.H) {
			continue
		}
		if result == nil {
			result = append([]MovableObject(nil), objects...)
		}
		x0, y0, x1, y1 := clippedObjectBounds(o, base.W, base.H)
		w, h := x1-x0, y1-y0
		// Ordinary RGB objects must retain their own alpha rather than bake
		// lower editable layers into a screenshot. Dynamic inversion and
		// replacement masks need a stack snapshot at their current position.
		pixels := make([]byte, w*h*4)
		needsReplacement := objectHasInversion(o) || len(o.ReplaceMask) != 0
		var mask []byte
		if needsReplacement {
			for y := y0; y < y1; y++ {
				copy(pixels[(y-y0)*w*4:][:w*4], base.Pix[(y*base.W+x0)*4:][:w*4])
			}
			compositeObjectsToPixels(pixels, w, h, x0, y0, objects[:i+1])
			mask = make([]byte, (w*h+7)/8)
		}
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				if !objectPaintsPixel(o, x, y) || s.contains(x, y) {
					continue
				}
				j := (y-y0)*w + x - x0
				if needsReplacement {
					mask[j>>3] |= 1 << uint(j&7)
				} else {
					si := objectSpriteOffset(o, x, y)
					copy(pixels[j*4:j*4+4], o.Pix[si:si+4])
					pixels[j*4+3] = byte((int(pixels[j*4+3])*int(100-o.Transparency) + 50) / 100)
				}
			}
		}
		frozen := *o
		frozen.X, frozen.Y, frozen.W, frozen.H = x0, y0, w, h
		frozen.SpriteW, frozen.SpriteH = w, h
		frozen.Pix, frozen.ReplaceMask = pixels, mask
		frozen.Operation, frozen.BackgroundOperation = PaintRGB, PaintRGB
		frozen.GlyphInvertMask = nil
		frozen.Transparency = 0
		frozen.Rasterized = true
		frozen.GroupID = 0
		frozen.Vector = ObjectVector{}
		frozen.Path = nil
		frozen.Text, frozen.Styles, frozen.FontName = "", nil, ""
		frozen.ShapeX0, frozen.ShapeY0, frozen.ShapeX1, frozen.ShapeY1 = 0, 0, 0, 0
		frozen.ShapeW, frozen.ShapeH, frozen.Stroke = 0, 0, 0
		frozen.ShowCenter = false
		frozen.Opaque = false
		result[i] = frozen
		affected = append(affected, i)
	}
	if result == nil {
		return objects, nil
	}
	return result, affected
}
