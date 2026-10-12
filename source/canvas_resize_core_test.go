package main

import "testing"

func TestCanvasResizeDragActivated(t *testing.T) {
	tests := []struct {
		sx, sy, x, y int
		want         bool
	}{
		{100, 100, 100, 100, false},
		{100, 100, 104, 104, false},
		{100, 100, 105, 100, true},
		{100, 100, 100, 95, true},
		{100, 100, 94, 104, true},
	}
	for _, tc := range tests {
		if got := canvasResizeDragActivated(tc.sx, tc.sy, tc.x, tc.y); got != tc.want {
			t.Fatalf("canvasResizeDragActivated(%d,%d -> %d,%d)=%v want %v", tc.sx, tc.sy, tc.x, tc.y, got, tc.want)
		}
	}
}

func TestCanvasResizeCaptureChangeIsLoss(t *testing.T) {
	const own = uintptr(0x1234)
	if canvasResizeCaptureChangeIsLoss(own, own) {
		t.Fatal("self capture notification must not cancel canvas resize")
	}
	if !canvasResizeCaptureChangeIsLoss(0, own) {
		t.Fatal("capture release to no owner must cancel canvas resize")
	}
	if !canvasResizeCaptureChangeIsLoss(0x5678, own) {
		t.Fatal("capture transfer to another window must cancel canvas resize")
	}
}
