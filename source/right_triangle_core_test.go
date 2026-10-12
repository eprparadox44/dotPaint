package main

import "testing"

func TestRightTriangleUsesDragEndpointsAsHypotenuse(t *testing.T) {
	x0, y0, x1, y1 := 10, 20, 30, 5
	cx, cy := rightTriangleLegCorner(x0, y0, x1, y1)
	if cx != x1 || cy != y0 {
		t.Fatalf("corner=(%d,%d), want (%d,%d)", cx, cy, x1, y0)
	}
	if (cx == x0 && cy == y0) || (cx == x1 && cy == y1) {
		t.Fatal("non-degenerate right-angle corner must not equal either hypotenuse endpoint")
	}
}

func TestRightTriangleLegsAreHorizontalAndVerticalInAllQuadrants(t *testing.T) {
	cases := [][4]int{{3, 4, 9, 11}, {9, 4, 3, 11}, {3, 11, 9, 4}, {9, 11, 3, 4}}
	for _, tc := range cases {
		x0, y0, x1, y1 := tc[0], tc[1], tc[2], tc[3]
		cx, cy := rightTriangleLegCorner(x0, y0, x1, y1)
		if cy != y0 {
			t.Fatalf("%v first leg is not horizontal", tc)
		}
		if cx != x1 {
			t.Fatalf("%v second leg is not vertical", tc)
		}
	}
}

func TestRightTriangleDegenerateDragIsSafe(t *testing.T) {
	for _, tc := range [][4]int{{2, 3, 2, 9}, {2, 3, 9, 3}, {2, 3, 2, 3}} {
		cx, cy := rightTriangleLegCorner(tc[0], tc[1], tc[2], tc[3])
		if cx != tc[2] || cy != tc[1] {
			t.Fatalf("%v corner=(%d,%d)", tc, cx, cy)
		}
	}
}
