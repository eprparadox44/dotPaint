package main

import "testing"

func TestRectSelectionDragThreshold(t *testing.T) {
	for _, tc := range []struct {
		x, y int
		want bool
	}{
		{0, 0, false}, {4, 4, false}, {5, 0, true}, {0, -5, true}, {-5, -5, true},
	} {
		if got := rectSelectionDragActivated(0, 0, tc.x, tc.y); got != tc.want {
			t.Fatalf("drag (%d,%d): got %v want %v", tc.x, tc.y, got, tc.want)
		}
	}
}
