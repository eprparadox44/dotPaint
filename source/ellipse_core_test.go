package main

import "testing"

func TestSymmetricEllipseRingExactMirrors(t *testing.T) {
	for w := 3; w <= 80; w++ {
		for h := 3; h <= 80; h++ {
			for thickness := 1; thickness <= 12; thickness++ {
				m := symmetricEllipseRingMask(w, h, thickness)
				for y := 0; y < h; y++ {
					for x := 0; x < w; x++ {
						v := m[y*w+x]
						if v != m[y*w+(w-1-x)] || v != m[(h-1-y)*w+x] || v != m[(h-1-y)*w+(w-1-x)] {
							t.Fatalf("asymmetry w=%d h=%d t=%d at %d,%d", w, h, thickness, x, y)
						}
					}
				}
			}
		}
	}
}

func TestThickCircleInnerBoundaryIsConcentricCircle(t *testing.T) {
	w, h, thickness := 101, 101, 18
	m := symmetricEllipseRingMask(w, h, thickness)
	cx, cy := 50, 50
	// The centre must remain empty and the ring thickness on all four axes must
	// be identical. This directly catches the old inward chessboard growth.
	if m[cy*w+cx] {
		t.Fatal("centre unexpectedly filled")
	}
	for i := 0; i < thickness; i++ {
		pts := [][2]int{{cx, i}, {cx, h - 1 - i}, {i, cy}, {w - 1 - i, cy}}
		for _, p := range pts {
			if !m[p[1]*w+p[0]] {
				t.Fatalf("axis ring gap at %v", p)
			}
		}
	}
	for _, p := range [][2]int{{cx, thickness}, {cx, h - 1 - thickness}, {thickness, cy}, {w - 1 - thickness, cy}} {
		if m[p[1]*w+p[0]] {
			t.Fatalf("inner axis should be empty at %v", p)
		}
	}
}

func countMaskRow(mask []bool, w, y int) int {
	count := 0
	for x := 0; x < w; x++ {
		if mask[y*w+x] {
			count++
		}
	}
	return count
}

func countMaskColumn(mask []bool, w, h, x int) int {
	count := 0
	for y := 0; y < h; y++ {
		if mask[y*w+x] {
			count++
		}
	}
	return count
}

func TestLargeEllipseOuterCardinalCapsAreFlat(t *testing.T) {
	cases := []struct {
		w, h     int
		wantTop  int
		wantLeft int
	}{
		{21, 21, 3, 3},
		{22, 22, 2, 2},
		{31, 24, 3, 2},
		{24, 31, 2, 3},
	}
	for _, tc := range cases {
		m := symmetricEllipseRingMask(tc.w, tc.h, 1)
		if got := countMaskRow(m, tc.w, 0); got != tc.wantTop {
			t.Fatalf("top cap w=%d h=%d got=%d want=%d", tc.w, tc.h, got, tc.wantTop)
		}
		if got := countMaskRow(m, tc.w, tc.h-1); got != tc.wantTop {
			t.Fatalf("bottom cap w=%d h=%d got=%d want=%d", tc.w, tc.h, got, tc.wantTop)
		}
		if got := countMaskColumn(m, tc.w, tc.h, 0); got != tc.wantLeft {
			t.Fatalf("left cap w=%d h=%d got=%d want=%d", tc.w, tc.h, got, tc.wantLeft)
		}
		if got := countMaskColumn(m, tc.w, tc.h, tc.w-1); got != tc.wantLeft {
			t.Fatalf("right cap w=%d h=%d got=%d want=%d", tc.w, tc.h, got, tc.wantLeft)
		}
	}
}

func TestLargeThickCircleInnerCardinalCapsAreFlat(t *testing.T) {
	w, h, thickness := 25, 25, 4
	m := symmetricEllipseRingMask(w, h, thickness)
	cx, cy := w/2, h/2

	// The outer top cap is three pixels wide. The first inner-hole row is also
	// three pixels wide, so neither boundary has a one-pixel cardinal point.
	if got := countMaskRow(m, w, 0); got != 3 {
		t.Fatalf("outer top cap got=%d want=3", got)
	}
	innerTop := thickness
	for x := cx - 1; x <= cx+1; x++ {
		if m[innerTop*w+x] {
			t.Fatalf("inner vertical cap is not flat at %d,%d", x, innerTop)
		}
	}
	innerLeft := thickness
	for y := cy - 1; y <= cy+1; y++ {
		if m[y*w+innerLeft] {
			t.Fatalf("inner horizontal cap is not flat at %d,%d", innerLeft, y)
		}
	}
}

func TestTwentyPixelAxisKeepsOriginalApexPolicy(t *testing.T) {
	// A93 deliberately starts flattening only above 20 pixels. A 19×20 ellipse
	// therefore keeps the exact analytic pixel-centre result on its 20px axis.
	m := symmetricEllipseRingMask(19, 20, 1)
	if got := countMaskRow(m, 19, 0); got != 1 {
		t.Fatalf("20px vertical axis unexpectedly flattened: top count=%d", got)
	}
}

func maskComponentCount8(mask []bool, w, h int) int {
	seen := make([]bool, len(mask))
	count := 0
	for i, on := range mask {
		if !on || seen[i] {
			continue
		}
		count++
		stack := []int{i}
		seen[i] = true
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := cur%w, cur/w
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := x+dx, y+dy
					if nx < 0 || nx >= w || ny < 0 || ny >= h {
						continue
					}
					ni := ny*w + nx
					if mask[ni] && !seen[ni] {
						seen[ni] = true
						stack = append(stack, ni)
					}
				}
			}
		}
	}
	return count
}

func TestA109ExtremeEllipseRingIsEightConnected(t *testing.T) {
	for _, tc := range []struct{ w, h int }{{117, 21}, {21, 117}, {151, 35}, {35, 151}} {
		m := symmetricEllipseRingMask(tc.w, tc.h, 1)
		if got := maskComponentCount8(m, tc.w, tc.h); got != 1 {
			t.Fatalf("extreme ellipse %dx%d has %d components", tc.w, tc.h, got)
		}
		for y := 0; y < tc.h; y++ {
			for x := 0; x < tc.w; x++ {
				v := m[y*tc.w+x]
				if v != m[y*tc.w+(tc.w-1-x)] || v != m[(tc.h-1-y)*tc.w+x] {
					t.Fatalf("repair lost symmetry for %dx%d at %d,%d", tc.w, tc.h, x, y)
				}
			}
		}
	}
}

func TestA109EccentricityThreshold(t *testing.T) {
	if !ellipseEccentricityAbove(117, 21, 0.93) {
		t.Fatal("117x21 should exceed eccentricity 0.93")
	}
	if ellipseEccentricityAbove(100, 40, 0.93) {
		t.Fatal("100x40 should remain below eccentricity 0.93")
	}
}

func TestA110VeryFlatEllipseScreenshotCaseIsOneComponent(t *testing.T) {
	for _, tc := range []struct{ w, h int }{{152, 13}, {13, 152}, {153, 14}, {14, 153}} {
		m := symmetricEllipseRingMask(tc.w, tc.h, 1)
		if got := maskComponentCount8(m, tc.w, tc.h); got != 1 {
			t.Fatalf("very flat ellipse %dx%d has %d components", tc.w, tc.h, got)
		}
		for y := 0; y < tc.h; y++ {
			for x := 0; x < tc.w; x++ {
				v := m[y*tc.w+x]
				if v != m[y*tc.w+(tc.w-1-x)] || v != m[(tc.h-1-y)*tc.w+x] {
					t.Fatalf("micro-gap closure lost symmetry for %dx%d at %d,%d", tc.w, tc.h, x, y)
				}
			}
		}
	}
}

func TestA110MicroGapClosureDoesNotFillConnectedInnerHole(t *testing.T) {
	w, h, thickness := 151, 17, 3
	m := symmetricEllipseRingMask(w, h, thickness)
	if m[(h/2)*w+w/2] {
		t.Fatal("component-aware closure filled the ellipse centre")
	}
}

func countTrueIntervalsA112(mask []bool, w, fixed, length int, horizontal bool) int {
	count := 0
	inside := false
	for i := 0; i < length; i++ {
		idx := fixed*w + i
		if !horizontal {
			idx = i*w + fixed
		}
		on := mask[idx]
		if on && !inside {
			count++
		}
		inside = on
	}
	return count
}

func TestA112ExtremeEllipseHasNoRedundantCardinalSpurs(t *testing.T) {
	for _, tc := range []struct{ w, h int }{{291, 10}, {10, 291}, {305, 11}, {11, 305}} {
		mask := symmetricEllipseRingMask(tc.w, tc.h, 1)
		for y := 0; y < tc.h; y++ {
			for x := 0; x < tc.w; x++ {
				v := mask[y*tc.w+x]
				if v != mask[y*tc.w+(tc.w-1-x)] || v != mask[(tc.h-1-y)*tc.w+x] {
					t.Fatalf("A112 repair lost symmetry for %dx%d at %d,%d", tc.w, tc.h, x, y)
				}
			}
		}
		_, components := labelEllipseComponents(mask, tc.w, tc.h)
		if components != 1 {
			t.Fatalf("%dx%d should remain one connected ring, got %d components", tc.w, tc.h, components)
		}
		if tc.w >= tc.h {
			for y := 0; y < tc.h; y++ {
				if got := countTrueIntervalsA112(mask, tc.w, y, tc.w, true); got > 2 {
					t.Fatalf("%dx%d row %d has %d intervals; redundant tip spur remains", tc.w, tc.h, y, got)
				}
			}
		} else {
			for x := 0; x < tc.w; x++ {
				if got := countTrueIntervalsA112(mask, tc.w, x, tc.h, false); got > 2 {
					t.Fatalf("%dx%d column %d has %d intervals; redundant tip spur remains", tc.w, tc.h, x, got)
				}
			}
		}
	}
}
