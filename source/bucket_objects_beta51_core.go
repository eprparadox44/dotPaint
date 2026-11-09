package main

// floodFillBehindObjects uses the existing RGB color-tolerance rule while
// treating the visible object footprint as an impermeable wall. Objects are
// not flattened, deleted, or edited by this operation.
func floodFillBehindObjects(pix []byte, w, h, sx, sy int, col [4]byte, tolerance int, blocked []byte) int {
	if w < 1 || h < 1 || len(pix) != w*h*4 || len(blocked) != w*h || sx < 0 || sx >= w || sy < 0 || sy >= h {
		return 0
	}
	seed := sy*w + sx
	if blocked[seed] != 0 {
		return 0
	}
	tolerance = clampColorTolerance(tolerance)
	i := seed * 4
	original := [4]byte{pix[i], pix[i+1], pix[i+2], pix[i+3]}
	if tolerance == 0 && col == original {
		return 0
	}
	seen := make([]byte, w*h)
	stack := []int{seed}
	seen[seed] = 1
	changed := 0
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if blocked[i] != 0 || !colorPixelMatches(pix, i, original, tolerance) {
			continue
		}
		j := i * 4
		copy(pix[j:j+4], col[:])
		changed++
		x, y := i%w, i/w
		try := func(k int) {
			if blocked[k] == 0 && seen[k] == 0 {
				seen[k] = 1
				stack = append(stack, k)
			}
		}
		if x > 0 {
			try(i - 1)
		}
		if x < w-1 {
			try(i + 1)
		}
		if y > 0 {
			try(i - w)
		}
		if y < h-1 {
			try(i + w)
		}
	}
	return changed
}

func bucketObjectWallMask(objects []MovableObject, w, h int) []byte {
	if len(objects) == 0 || w < 1 || h < 1 {
		return nil
	}
	wall := make([]byte, w*h)
	for k := range objects {
		o := &objects[k]
		if o.Transparency >= 100 || o.Operation == PaintTransparent || o.W < 1 || o.H < 1 {
			continue
		}
		x0, y0 := max(0, o.X), max(0, o.Y)
		x1, y1 := min(w, o.X+o.W), min(h, o.Y+o.H)
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				si := objectSpriteOffset(o, x, y)
				if si >= 0 && si+3 < len(o.Pix) && (o.Pix[si+3] >= 128 || (o.Kind == ToolText && len(o.GlyphInvertMask) > si/4 && o.GlyphInvertMask[si/4] >= 128)) {
					wall[y*w+x] = 1
				}
			}
		}
	}
	return wall
}
