package main

import "math"

// vectorAlphaMask is an invisible native-resolution alpha image produced from
// supersampled vector geometry.  The editor never shows this mask directly:
// each byte records how much of one canvas pixel is covered by the vector
// preview, and the canvas receives the colour only once after all component
// shapes have been unioned.
type vectorAlphaMask struct {
	Left, Top int
	W, H      int
	Alpha     []uint8
	Touched   []int
}

func newVectorAlphaMask(left, top, right, bottom int) *vectorAlphaMask {
	if right < left || bottom < top {
		return &vectorAlphaMask{Left: left, Top: top}
	}
	w, h := right-left+1, bottom-top+1
	return &vectorAlphaMask{
		Left: left, Top: top, W: w, H: h,
		Alpha:   make([]uint8, w*h),
		Touched: make([]int, 0, 2*(w+h)),
	}
}

func (m *vectorAlphaMask) index(x, y int) (int, bool) {
	if m == nil || x < m.Left || y < m.Top || x >= m.Left+m.W || y >= m.Top+m.H {
		return 0, false
	}
	return (y-m.Top)*m.W + (x - m.Left), true
}

func coverageToAlpha(coverage float64) uint8 {
	if coverage <= 0 {
		return 0
	}
	if coverage >= 1 {
		return 255
	}
	return uint8(math.Round(coverage * 255.0))
}

func (m *vectorAlphaMask) setMaxCoverage(x, y int, coverage float64) {
	idx, ok := m.index(x, y)
	if !ok {
		return
	}
	a := coverageToAlpha(coverage)
	if a > m.Alpha[idx] {
		if m.Alpha[idx] == 0 {
			m.Touched = append(m.Touched, idx)
		}
		m.Alpha[idx] = a
	}
}

func (m *vectorAlphaMask) setOpaque(x, y int) {
	idx, ok := m.index(x, y)
	if ok {
		if m.Alpha[idx] == 0 {
			m.Touched = append(m.Touched, idx)
		}
		m.Alpha[idx] = 255
	}
}

func (m *vectorAlphaMask) alphaAt(x, y int) uint8 {
	idx, ok := m.index(x, y)
	if !ok {
		return 0
	}
	return m.Alpha[idx]
}

// vectorCoverageCandidates stores only pixels that can possibly receive
// non-zero vector coverage. The expensive 8x8 coverage function is then called
// only for these pixels, while the final alpha calculation remains unchanged.
type vectorCoverageCandidates struct {
	Left, Top int
	W, H      int
	seen      []bool
	indices   []int
}

func newVectorCoverageCandidates(left, top, right, bottom int) *vectorCoverageCandidates {
	if right < left || bottom < top {
		return &vectorCoverageCandidates{Left: left, Top: top}
	}
	w, h := right-left+1, bottom-top+1
	return &vectorCoverageCandidates{
		Left: left, Top: top, W: w, H: h,
		seen:    make([]bool, w*h),
		indices: make([]int, 0, w+h),
	}
}

func (c *vectorCoverageCandidates) mark(x, y int) {
	if c == nil || x < c.Left || y < c.Top || x >= c.Left+c.W || y >= c.Top+c.H {
		return
	}
	idx := (y-c.Top)*c.W + (x - c.Left)
	if c.seen[idx] {
		return
	}
	c.seen[idx] = true
	c.indices = append(c.indices, idx)
}

func (c *vectorCoverageCandidates) contains(x, y int) bool {
	if c == nil || x < c.Left || y < c.Top || x >= c.Left+c.W || y >= c.Top+c.H {
		return false
	}
	return c.seen[(y-c.Top)*c.W+(x-c.Left)]
}

func (c *vectorCoverageCandidates) markRect(left, top, right, bottom int) {
	if c == nil || c.W <= 0 || c.H <= 0 {
		return
	}
	if left < c.Left {
		left = c.Left
	}
	if top < c.Top {
		top = c.Top
	}
	if right >= c.Left+c.W {
		right = c.Left + c.W - 1
	}
	if bottom >= c.Top+c.H {
		bottom = c.Top + c.H - 1
	}
	for y := top; y <= bottom; y++ {
		for x := left; x <= right; x++ {
			c.mark(x, y)
		}
	}
}

// markStroke walks the vector centreline at no more than one native pixel per
// step and marks a conservative square neighbourhood. Every pixel that can
// receive even one 8x8 sub-sample is included, but the vast empty corners of a
// diagonal bounding rectangle are skipped. The exact old coverage function is
// still used afterwards, so this changes speed only, never visual output.
func (c *vectorCoverageCandidates) markStroke(x0, y0, x1, y1 float64, lineSize int) {
	if c == nil || c.W <= 0 || c.H <= 0 {
		return
	}
	if lineSize < 1 {
		lineSize = 1
	}
	dx, dy := x1-x0, y1-y0
	steps := int(math.Ceil(math.Max(math.Abs(dx), math.Abs(dy))))
	if steps < 1 {
		steps = 1
	}
	// half-width + one pixel for sub-pixel coverage + one pixel for the gap
	// between adjacent DDA centres. This is intentionally conservative.
	margin := int(math.Ceil(float64(lineSize)/2.0 + 2.0))
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		cx, cy := x0+dx*t, y0+dy*t
		ix, iy := int(math.Round(cx)), int(math.Round(cy))
		c.markRect(ix-margin, iy-margin, ix+margin, iy+margin)
	}
}

func (c *vectorCoverageCandidates) forEach(fn func(x, y int)) {
	if c == nil || fn == nil || c.W <= 0 {
		return
	}
	for _, idx := range c.indices {
		fn(c.Left+idx%c.W, c.Top+idx/c.W)
	}
}
