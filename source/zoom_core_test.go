package main

import (
	"os"
	"strings"
	"testing"
)

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

func TestMagnifierSelectionSpanAndFullImageBypass(t *testing.T) {
	for _, tc := range []struct {
		available, scale, canvas, expected int
	}{
		{624, 20, 30, 30}, // whole 30-pixel image fits: no selection needed
		{624, 20, 31, 31}, // exactly fits
		{624, 20, 32, 32}, // region is one pixel too narrow
		{480, 8, 40, 40},  // short image fits at 8x
		{480, 8, 70, 60},  // long image requires selection
		{1, 20, 5, 1},
		{0, 8, 10, 0},
	} {
		got := magnifierSelectionSpan(tc.available, tc.scale, tc.canvas)
		if got != tc.expected {
			t.Errorf("magnifierSelectionSpan(%d,%d,%d)=%d, want %d", tc.available, tc.scale, tc.canvas, got, tc.expected)
		}
	}
}

func TestMagnifierWholeImageFitsBothDimensions(t *testing.T) {
	for _, tc := range []struct {
		name                                         string
		imageW, imageH, availableW, availableH, zoom int
		fits                                         bool
	}{
		{"tiny image at 20x", 10, 8, 624, 480, 20, true},
		{"exact boundary", 31, 24, 620, 480, 20, true},
		{"rounded dashed width must not bypass", 32, 24, 624, 480, 20, false},
		{"too tall", 25, 25, 624, 480, 20, false},
		{"large image at 8x", 1280, 720, 624, 480, 8, false},
		{"fits horizontally but not vertically", 40, 70, 624, 480, 8, false},
		{"one dimension zero", 0, 10, 624, 480, 20, false},
		{"zero zoom", 10, 10, 624, 480, 0, false},
	} {
		if got := magnifierWholeImageFits(tc.imageW, tc.imageH, tc.availableW, tc.availableH, tc.zoom); got != tc.fits {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.fits)
		}
	}
}

func TestMagnifierOptionAutoZoomIntegrationSource(t *testing.T) {
	src, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(src)
	for _, marker := range []string{
		"zoomLevels:  []int{1, 2, 4, 6, 8, 12, 15, 20}",
		"ID_ZOOM_BASE+20",
		"func (a *App) autoZoomWholeImage() bool",
		"magnifierWholeImageFits(a.canvas.W, a.canvas.H, pageW, pageH, a.zoomTarget)",
		"a.commitMagnifierZoom(x, y, w, h)",
	} {
		if !strings.Contains(code, marker) {
			t.Errorf("missing zoom integration marker %q", marker)
		}
	}
	// In the toolbar event, selecting Magnifier must only select a target:
	// do not force 8x immediately before the user can choose 20x.
	start := strings.Index(code, "} else if t == ToolMagnifier {")
	if start == -1 {
		t.Fatal("magnifier toolbar branch missing")
	}
	stop := strings.Index(code[start:], "} else if t == ToolFreeSelect {")
	if stop == -1 {
		t.Fatal("magnifier toolbar branch end missing")
	}
	if strings.Contains(code[start:start+stop], "a.autoZoomWholeImage()") {
		t.Fatal("magnifier tool activation must not auto-apply default 8x")
	}
}
