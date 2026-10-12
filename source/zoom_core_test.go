package main

import "testing"

func TestZoomDefaults(t *testing.T) {
	if initialCanvasZoom != 1 {
		t.Fatalf("initial canvas zoom = %d, want 1", initialCanvasZoom)
	}
	if got := chooseDefaultMagnifierZoom([]int{1, 2, 4, 6, 8, 12, 15}); got != 8 {
		t.Fatalf("magnifier default = %d, want 8", got)
	}
}

func TestMagnifierDefaultFallbacks(t *testing.T) {
	if got := chooseDefaultMagnifierZoom(nil); got != 1 {
		t.Fatalf("empty zoom levels fallback = %d, want 1", got)
	}
	if got := chooseDefaultMagnifierZoom([]int{1, 2, 4, 6}); got != 6 {
		t.Fatalf("missing 8x fallback = %d, want final level 6", got)
	}
}
