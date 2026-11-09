//go:build windows

package main

// Apply each glyph's colour or inversion to indexed pixels. The expensive
// nearest-colour search is cached per distinct RGB, and the inverse table is
// obtained once per text edit, not per glyph or pixel.
func (a *App) renderIndexedStyledText(target *Canvas, text string, updateIndices bool) bool {
	if a.dejavu != nil || !a.indexedMode || target == nil || text == "" {
		return false
	}
	obj, ok := a.makeStyledTextObject(text)
	if !ok {
		return false
	}
	palette := a.activeIndexedPaletteColors()
	if len(palette) == 0 {
		return false
	}
	lut := a.indexedInvertTable(palette)
	indices := []byte(nil)
	if updateIndices {
		indices = a.indexedPixelIndices
	}
	mapped := make(map[uint32]byte)
	bgIndex := byte(nearestIndexedColor(byte(obj.BG), byte(obj.BG>>8), byte(obj.BG>>16), palette))
	for ly := 0; ly < obj.H; ly++ {
		y := obj.Y + ly
		if y < 0 || y >= target.H {
			continue
		}
		for lx := 0; lx < obj.W; lx++ {
			x := obj.X + lx
			if x < 0 || x >= target.W {
				continue
			}
			pi := y*target.W + x
			si := (ly*obj.SpriteW + lx) * 4
			if si+3 >= len(obj.Pix) {
				continue
			}
			if obj.Opaque && obj.BackgroundOperation != PaintTransparent {
				if obj.BackgroundOperation == PaintInvert {
					applyIndexedInvertPixel(target.Pix, pi, indices, palette, lut)
				} else {
					v := palette[bgIndex]
					j := pi * 4
					target.Pix[j], target.Pix[j+1], target.Pix[j+2], target.Pix[j+3] = byte(v>>16), byte(v>>8), byte(v), 255
					if len(indices) == target.W*target.H {
						indices[pi] = bgIndex
					}
				}
			}
			if len(obj.GlyphInvertMask) > ly*obj.SpriteW+lx && obj.GlyphInvertMask[ly*obj.SpriteW+lx] >= 128 {
				applyIndexedInvertPixel(target.Pix, pi, indices, palette, lut)
				continue
			}
			if obj.Pix[si+3] < 128 {
				continue
			}
			col := bgraToColorref(obj.Pix[si], obj.Pix[si+1], obj.Pix[si+2])
			idx, found := mapped[col]
			if !found {
				idx = byte(nearestIndexedColor(byte(col), byte(col>>8), byte(col>>16), palette))
				mapped[col] = idx
			}
			v := palette[idx]
			j := pi * 4
			target.Pix[j], target.Pix[j+1], target.Pix[j+2], target.Pix[j+3] = byte(v>>16), byte(v>>8), byte(v), 255
			if len(indices) == target.W*target.H {
				indices[pi] = idx
			}
		}
	}
	return true
}
