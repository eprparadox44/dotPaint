package main

// eraseCompositedPixels uses the exact antialias coverage produced by existing
// vector/shape rasterizers as an eraser. It does not create a translucent object.
// Unaffected editable objects stay editable. Touched objects alone are frozen
// by the existing selective-cut engine, preserving Z-order and untouched pixels.
// Fractional edge coverage is reconstructed from a pre-erase composite so an
// AA edge cuts the whole visual stack once, rather than multiplying overlapping
// layer opacity several times.
func eraseCompositedPixels(base *Canvas, objects []MovableObject, mask *Canvas, originX, originY int) ([]MovableObject, bool) {
	if base == nil || mask == nil || base.W < 1 || base.H < 1 || mask.W < 1 || mask.H < 1 || len(mask.Pix) != mask.W*mask.H*4 {
		return objects, false
	}
	// The selection mask is limited to the existing object sprite, not to
	// the entire document. Off-canvas mask pixels are safely ignored.
	selected := make([]bool, mask.W*mask.H)
	fractional := false
	any := false
	for y := 0; y < mask.H; y++ {
		gy := originY + y
		if gy < 0 || gy >= base.H {
			continue
		}
		for x := 0; x < mask.W; x++ {
			gx := originX + x
			if gx < 0 || gx >= base.W {
				continue
			}
			a := mask.Pix[(y*mask.W+x)*4+3]
			if a == 0 {
				continue
			}
			any = true
			selected[y*mask.W+x] = true
			if a != 255 {
				fractional = true
			}
		}
	}
	if !any {
		return objects, false
	}

	// A full image composite is only needed for partially covered AA pixels;
	// opaque erasure is a direct alpha clear with no auxiliary allocations.
	var prior []byte
	if fractional {
		prior = make([]byte, len(mask.Pix))
		for y := 0; y < mask.H; y++ {
			gy := originY + y
			if gy < 0 || gy >= base.H {
				continue
			}
			for x := 0; x < mask.W; x++ {
				gx := originX + x
				if gx < 0 || gx >= base.W {
					continue
				}
				src := (gy*base.W + gx) * 4
				copy(prior[(y*mask.W+x)*4:][:4], base.Pix[src:src+4])
			}
		}
		compositeObjectsToPixels(prior, mask.W, mask.H, originX, originY, objects)
	}
	updated, _ := cutSelectedObjectPixels(base, objects, PixelSelectionCut{
		X: originX, Y: originY, W: mask.W, H: mask.H, Mask: selected,
	})
	// Wipe the bottom raster too, so removing one object cannot reveal old
	// pixels below it. In a transparent image these become true alpha-zero.
	for y := 0; y < mask.H; y++ {
		gy := originY + y
		if gy < 0 || gy >= base.H {
			continue
		}
		for x := 0; x < mask.W; x++ {
			if !selected[y*mask.W+x] {
				continue
			}
			gx := originX + x
			i := (gy*base.W + gx) * 4
			base.Pix[i], base.Pix[i+1], base.Pix[i+2], base.Pix[i+3] = 0, 0, 0, 0
		}
	}
	if fractional {
		// Re-add only the original visual color remaining under a fractional
		// eraser. This top frozen pixel layer does not double-composite RGB.
		pixels := make([]byte, len(mask.Pix))
		havePixels := false
		for i, hit := range selected {
			if !hit {
				continue
			}
			coverage := int(mask.Pix[i*4+3])
			if coverage == 255 {
				continue
			}
			newAlpha := (int(prior[i*4+3])*(255-coverage) + 127) / 255
			if newAlpha == 0 {
				continue
			}
			copy(pixels[i*4:i*4+4], prior[i*4:i*4+4])
			pixels[i*4+3] = byte(newAlpha)
			havePixels = true
		}
		if havePixels {
			updated = append(updated, MovableObject{
				Kind: ToolRectangle, X: originX, Y: originY, W: mask.W, H: mask.H,
				SpriteW: mask.W, SpriteH: mask.H, Pix: pixels, Rasterized: true,
				Operation: PaintRGB,
			})
		}
	}
	return updated, true
}
