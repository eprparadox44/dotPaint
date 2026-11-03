package main

import "testing"

func TestTextVisualLineEdge(t *testing.T) {
	// A wrapped line: a native 1px EDIT's Home/End must not control the caret.
	positions := []TextBoundaryPos{
		{Index: 0, X: 0, Y: 0},
		{Index: 1, X: 10, Y: 0},
		{Index: 2, X: 20, Y: 0},
		{Index: 3, X: 0, Y: 24},
		{Index: 4, X: 12, Y: 24},
		{Index: 5, X: 24, Y: 24},
	}
	for _, tt := range []struct {
		caret int
		toEnd bool
		want  int
	}{
		{1, false, 0}, {1, true, 2},
		{4, false, 3}, {4, true, 5},
		{-20, false, 0}, {50, true, 5},
	} {
		if got := textVisualLineEdge(positions, tt.caret, tt.toEnd); got != tt.want {
			t.Errorf("caret=%d end=%v: got %d want %d", tt.caret, tt.toEnd, got, tt.want)
		}
	}
	if textVisualLineEdge(nil, 8, true) != 0 {
		t.Fatal("empty positions")
	}
}
