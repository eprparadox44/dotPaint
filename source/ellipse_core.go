package main

// ellipseContainsPixel tests a pixel centre against a concentric ellipse whose
// full doubled-axis spans are spanX and spanY. dx2/dy2 are doubled offsets from
// the common centre, so odd and even dimensions share the same formula.
func ellipseContainsPixel(dx2, dy2, spanX, spanY int) bool {
	if spanX <= 0 || spanY <= 0 {
		return false
	}
	dx := float64(dx2)
	dy := float64(dy2)
	sx := float64(spanX)
	sy := float64(spanY)
	// Rearranged ellipse equation. The tiny allowance only protects exact axis
	// points from floating-point roundoff; mirrored pixels never diverge because
	// only one quadrant is evaluated.
	return dx*dx*sy*sy+dy*dy*sx*sx <= sx*sx*sy*sy+1e-9
}

// symmetricFilledEllipseMask builds one concentric analytic ellipse in the
// coordinate system of a w×h outer box. spanX/spanY are the distances between
// the two extreme pixel centres, so the represented ellipse dimensions are
// spanX+1 by spanY+1. Only one quadrant is evaluated and mirrored.
func symmetricFilledEllipseMask(w, h, spanX, spanY int) []bool {
	mask := make([]bool, w*h)
	if w <= 0 || h <= 0 || spanX <= 0 || spanY <= 0 {
		return mask
	}
	outerX := w - 1
	outerY := h - 1
	maxX := outerX / 2
	maxY := outerY / 2
	for y := 0; y <= maxY; y++ {
		dy2 := 2*y - outerY
		for x := 0; x <= maxX; x++ {
			dx2 := 2*x - outerX
			if !ellipseContainsPixel(dx2, dy2, spanX, spanY) {
				continue
			}
			xs := [2]int{x, w - 1 - x}
			ys := [2]int{y, h - 1 - y}
			for _, yy := range ys {
				for _, xx := range xs {
					mask[yy*w+xx] = true
				}
			}
		}
	}
	return mask
}

// flattenEllipseCardinalCaps widens a one-pixel cardinal apex into the smallest
// perfectly symmetric flat cap: three pixels on an odd perpendicular axis and
// two pixels on an even perpendicular axis. The ellipse keeps its requested
// outer bounds; only the exact axis tip is flattened. The caller chooses each
// axis independently, which lets A93 apply this only when that requested axis
// is larger than 20 pixels.
func flattenEllipseCardinalCaps(mask []bool, w, h, spanX, spanY int, flattenVertical, flattenHorizontal bool) {
	if len(mask) != w*h || w <= 0 || h <= 0 || spanX <= 0 || spanY <= 0 {
		return
	}

	actualW := spanX + 1
	actualH := spanY + 1
	xMin := (w - 1 - spanX) / 2
	xMax := (w - 1 + spanX) / 2
	yMin := (h - 1 - spanY) / 2
	yMax := (h - 1 + spanY) / 2

	if flattenVertical && actualW >= 2 && yMin >= 0 && yMax < h {
		capWidth := 2
		if actualW%2 != 0 {
			capWidth = 3
		}
		if capWidth > actualW {
			capWidth = actualW
		}
		left := (w - capWidth) / 2
		for _, y := range [2]int{yMin, yMax} {
			for x := left; x < left+capWidth; x++ {
				mask[y*w+x] = true
			}
		}
	}

	if flattenHorizontal && actualH >= 2 && xMin >= 0 && xMax < w {
		capHeight := 2
		if actualH%2 != 0 {
			capHeight = 3
		}
		if capHeight > actualH {
			capHeight = actualH
		}
		top := (h - capHeight) / 2
		for _, x := range [2]int{xMin, xMax} {
			for y := top; y < top+capHeight; y++ {
				mask[y*w+x] = true
			}
		}
	}
}

// symmetricEllipseRingMask returns an inward-growing ellipse stroke. The outer
// ellipse occupies the requested bounds; the inner ellipse is inset by exactly
// thickness pixels on each axis. A93 additionally flattens the four cardinal
// apices whenever the corresponding requested outer axis exceeds 20 pixels.
// Both the outer silhouette and the inner hole receive the same treatment, so
// the stroke remains concentric and exactly mirrored in all four directions.
func symmetricEllipseRingMask(w, h, thickness int) []bool {
	mask := make([]bool, w*h)
	if w <= 0 || h <= 0 || thickness <= 0 {
		return mask
	}
	if thickness > w {
		thickness = w
	}
	if thickness > h {
		thickness = h
	}

	outerX := w - 1
	outerY := h - 1
	innerX := outerX - 2*thickness
	innerY := outerY - 2*thickness

	outer := symmetricFilledEllipseMask(w, h, outerX, outerY)
	flattenVertical := h > 20
	flattenHorizontal := w > 20
	flattenEllipseCardinalCaps(outer, w, h, outerX, outerY, flattenVertical, flattenHorizontal)

	var inner []bool
	if innerX > 0 && innerY > 0 {
		inner = symmetricFilledEllipseMask(w, h, innerX, innerY)
		// The user-visible shape size controls whether a cardinal direction is
		// flattened. Applying the same decision to the hole removes the matching
		// inward-facing point while retaining exact stroke symmetry.
		flattenEllipseCardinalCaps(inner, w, h, innerX, innerY, flattenVertical, flattenHorizontal)
	}

	for i, on := range outer {
		if on && (inner == nil || !inner[i]) {
			mask[i] = true
		}
	}
	bridgeExtremeEllipseGaps(mask, w, h)
	connectExtremeEllipseMicroGaps(mask, w, h)
	return mask
}

// ellipseEccentricityAbove reports whether the requested outer box is more
// eccentric than the supplied threshold.  A109 uses 0.93 because the analytic
// ring is already visually continuous below that point, while shallower arcs
// can skip a native pixel when one scanline advances several columns.
func ellipseEccentricityAbove(w, h int, threshold float64) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	major, minor := w, h
	if minor > major {
		major, minor = minor, major
	}
	if major == 0 {
		return false
	}
	r := float64(minor) / float64(major)
	e2 := 1.0 - r*r
	return e2 > threshold*threshold
}

type ellipseInterval struct {
	lo, hi int
	ok     bool
}

func ellipseLeftInterval(mask []bool, w, y int) ellipseInterval {
	if y < 0 || y >= len(mask)/w {
		return ellipseInterval{}
	}
	centre := (w - 1) / 2
	lo, hi := -1, -1
	for x := 0; x <= centre; x++ {
		if mask[y*w+x] {
			if lo < 0 {
				lo = x
			}
			hi = x
		}
	}
	return ellipseInterval{lo: lo, hi: hi, ok: lo >= 0}
}

func ellipseTopInterval(mask []bool, w, h, x int) ellipseInterval {
	if x < 0 || x >= w {
		return ellipseInterval{}
	}
	centre := (h - 1) / 2
	lo, hi := -1, -1
	for y := 0; y <= centre; y++ {
		if mask[y*w+x] {
			if lo < 0 {
				lo = y
			}
			hi = y
		}
	}
	return ellipseInterval{lo: lo, hi: hi, ok: lo >= 0}
}

func nearestIntervalEndpoints(a, b ellipseInterval) (int, int, bool) {
	if !a.ok || !b.ok {
		return 0, 0, false
	}
	if a.hi < b.lo {
		return a.hi, b.lo, true
	}
	if b.hi < a.lo {
		return a.lo, b.hi, true
	}
	return 0, 0, false
}

// setEllipseBridgePixel mirrors every repair into all four quadrants.  The
// bridge algorithm can therefore work on only the top-left quarter without
// giving up the exact horizontal/vertical symmetry guaranteed by A92.
func setEllipseBridgePixel(mask []bool, w, h, x, y int) {
	if x < 0 || x >= w || y < 0 || y >= h {
		return
	}
	for _, yy := range [2]int{y, h - 1 - y} {
		for _, xx := range [2]int{x, w - 1 - x} {
			mask[yy*w+xx] = true
		}
	}
}

// bridgeEllipsePoints draws a native-pixel Bresenham bridge and mirrors it.
// It is only used between neighbouring scanlines/columns, so the repair adds
// the minimum staircase pixels needed for 8-connected continuity.
func bridgeEllipsePoints(mask []bool, w, h, x0, y0, x1, y1 int) {
	dx := absCoreInt(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -absCoreInt(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		setEllipseBridgePixel(mask, w, h, x0, y0)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// bridgeExtremeEllipseGaps repairs the one-pixel holes that appear when an
// ellipse is so shallow that consecutive analytic scanlines move by more than
// one column (or vice versa).  It never changes the requested outer bounds and
// runs only above eccentricity 0.93.
func bridgeExtremeEllipseGaps(mask []bool, w, h int) {
	if len(mask) != w*h || !ellipseEccentricityAbove(w, h, 0.93) {
		return
	}
	if w >= h {
		for y := 0; y < (h-1)/2; y++ {
			a := ellipseLeftInterval(mask, w, y)
			b := ellipseLeftInterval(mask, w, y+1)
			x0, x1, ok := nearestIntervalEndpoints(a, b)
			if ok && absCoreInt(x1-x0) > 1 {
				bridgeEllipsePoints(mask, w, h, x0, y, x1, y+1)
			}
		}
		return
	}
	for x := 0; x < (w-1)/2; x++ {
		a := ellipseTopInterval(mask, w, h, x)
		b := ellipseTopInterval(mask, w, h, x+1)
		y0, y1, ok := nearestIntervalEndpoints(a, b)
		if ok && absCoreInt(y1-y0) > 1 {
			bridgeEllipsePoints(mask, w, h, x, y0, x+1, y1)
		}
	}
}

// labelEllipseComponents returns 8-connected component labels for the current
// ring mask. Zero means background; positive values identify components.
func labelEllipseComponents(mask []bool, w, h int) ([]int, int) {
	labels := make([]int, len(mask))
	count := 0
	queue := make([]int, 0, 64)
	for start, on := range mask {
		if !on || labels[start] != 0 {
			continue
		}
		count++
		labels[start] = count
		queue = queue[:0]
		queue = append(queue, start)
		for head := 0; head < len(queue); head++ {
			idx := queue[head]
			x, y := idx%w, idx/w
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := x+dx, y+dy
					if nx < 0 || nx >= w || ny < 0 || ny >= h {
						continue
					}
					n := ny*w + nx
					if mask[n] && labels[n] == 0 {
						labels[n] = count
						queue = append(queue, n)
					}
				}
			}
		}
	}
	return labels, count
}

// mirroredEllipseCandidateIndices returns one symmetry-preserving repair group.
// A candidate near a cardinal tip is always added together with its horizontal
// and vertical mirrors, so no scan-order choice can distort the ellipse.
func mirroredEllipseCandidateIndices(w, h, x, y int) []int {
	seen := make(map[int]struct{}, 4)
	out := make([]int, 0, 4)
	for _, yy := range [2]int{y, h - 1 - y} {
		for _, xx := range [2]int{x, w - 1 - x} {
			if xx < 0 || xx >= w || yy < 0 || yy >= h {
				continue
			}
			idx := yy*w + xx
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			out = append(out, idx)
		}
	}
	return out
}

type ellipseRepairRun struct {
	lo, hi int
	label  int
}

type ellipseRepairCandidate struct {
	indices              []int
	gap, minorDist, edge int
	canonical            int
}

func uniqueSortedEllipseIndices(indices []int) []int {
	if len(indices) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(indices))
	out := make([]int, 0, len(indices))
	for _, idx := range indices {
		if _, ok := seen[idx]; ok {
			continue
		}
		seen[idx] = struct{}{}
		out = append(out, idx)
	}
	for i := 1; i < len(out); i++ {
		v := out[i]
		j := i - 1
		for j >= 0 && out[j] > v {
			out[j+1] = out[j]
			j--
		}
		out[j+1] = v
	}
	return out
}

func betterEllipseRepairCandidate(a, b ellipseRepairCandidate) bool {
	if len(a.indices) == 0 {
		return false
	}
	if len(b.indices) == 0 {
		return true
	}
	if a.gap != b.gap {
		return a.gap < b.gap
	}
	if a.minorDist != b.minorDist {
		return a.minorDist < b.minorDist
	}
	if a.edge != b.edge {
		return a.edge < b.edge
	}
	return a.canonical < b.canonical
}

// bestExtremeEllipseRunBridge finds the shortest empty run separating two
// differently labelled ring components. For a horizontal extreme ellipse it
// searches row runs; for a vertical one it searches column runs. The complete
// gap is mirrored into all four quadrants before it is added. This is the
// minimal cardinal bridge that A110's batched one-pixel rule was trying to
// approximate, but it cannot create a redundant outward spur.
func bestExtremeEllipseRunBridge(mask []bool, labels []int, w, h int) ellipseRepairCandidate {
	best := ellipseRepairCandidate{}
	if w >= h {
		for y := 0; y < h; y++ {
			runs := make([]ellipseRepairRun, 0, 4)
			for x := 0; x < w; {
				if !mask[y*w+x] {
					x++
					continue
				}
				lo := x
				label := labels[y*w+x]
				for x+1 < w && mask[y*w+x+1] {
					x++
				}
				runs = append(runs, ellipseRepairRun{lo: lo, hi: x, label: label})
				x++
			}
			for i := 1; i < len(runs); i++ {
				a, b := runs[i-1], runs[i]
				if a.label == 0 || b.label == 0 || a.label == b.label || b.lo-a.hi <= 1 {
					continue
				}
				indices := make([]int, 0, 4*(b.lo-a.hi-1))
				for x := a.hi + 1; x < b.lo; x++ {
					indices = append(indices, mirroredEllipseCandidateIndices(w, h, x, y)...)
				}
				indices = uniqueSortedEllipseIndices(indices)
				if len(indices) == 0 {
					continue
				}
				candidate := ellipseRepairCandidate{
					indices:   indices,
					gap:       b.lo - a.hi - 1,
					minorDist: absCoreInt(2*y - (h - 1)),
					edge:      min(a.hi, w-1-b.lo),
					canonical: indices[0],
				}
				if betterEllipseRepairCandidate(candidate, best) {
					best = candidate
				}
			}
		}
		return best
	}

	for x := 0; x < w; x++ {
		runs := make([]ellipseRepairRun, 0, 4)
		for y := 0; y < h; {
			if !mask[y*w+x] {
				y++
				continue
			}
			lo := y
			label := labels[y*w+x]
			for y+1 < h && mask[(y+1)*w+x] {
				y++
			}
			runs = append(runs, ellipseRepairRun{lo: lo, hi: y, label: label})
			y++
		}
		for i := 1; i < len(runs); i++ {
			a, b := runs[i-1], runs[i]
			if a.label == 0 || b.label == 0 || a.label == b.label || b.lo-a.hi <= 1 {
				continue
			}
			indices := make([]int, 0, 4*(b.lo-a.hi-1))
			for y := a.hi + 1; y < b.lo; y++ {
				indices = append(indices, mirroredEllipseCandidateIndices(w, h, x, y)...)
			}
			indices = uniqueSortedEllipseIndices(indices)
			if len(indices) == 0 {
				continue
			}
			candidate := ellipseRepairCandidate{
				indices:   indices,
				gap:       b.lo - a.hi - 1,
				minorDist: absCoreInt(2*x - (w - 1)),
				edge:      min(a.hi, h-1-b.lo),
				canonical: indices[0],
			}
			if betterEllipseRepairCandidate(candidate, best) {
				best = candidate
			}
		}
	}
	return best
}

// connectExtremeEllipseMicroGaps adds only the minimum symmetry-preserving
// cardinal bridge required to join the current components. It recomputes labels
// after every bridge. Therefore a centre-side bridge that already joins the
// ring prevents the redundant outer-side pixel that caused the user's tiny
// endpoint protrusion.
func connectExtremeEllipseMicroGaps(mask []bool, w, h int) {
	if len(mask) != w*h || !ellipseEccentricityAbove(w, h, 0.93) {
		return
	}
	for pass := 0; pass < 16; pass++ {
		labels, count := labelEllipseComponents(mask, w, h)
		if count <= 1 {
			return
		}
		candidate := bestExtremeEllipseRunBridge(mask, labels, w, h)
		if len(candidate.indices) == 0 {
			return
		}
		for _, idx := range candidate.indices {
			mask[idx] = true
		}
	}
}
