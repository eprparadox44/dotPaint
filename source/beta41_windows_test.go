//go:build windows

package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestBeta41InvertSwatchGeometryAndReference(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(invertColorReference))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 92 || img.Bounds().Dy() != 46 {
		t.Fatalf("expected user-supplied two 46x46 theme images, got %v", img.Bounds())
	}
	invertIconOnce.Do(initInvertIcon)
	for theme := 0; theme < 2; theme++ {
		if len(invertIconPixels[theme]) != 46*46*4 {
			t.Fatalf("missing theme %d icon", theme)
		}
	}
	a := newApp()
	// Screen coordinates supplied by the actual palette painter are local.
	for _, tc := range []struct{ mode, count int }{{0, 44}, {0, 28}, {16, 16}, {256, 256}} {
		a.dejavu = nil
		a.indexedMode = tc.mode != 0
		a.indexedSize = tc.mode
		a.paletteDisplayRGB = tc.count
		a.paletteDisplay16 = tc.count
		a.paletteDisplay256 = tc.count
		rc := RECT{0, 0, int32(a.palettePanelActualWidth() - 2), 54}
		inv, tr := a.paletteSpecialAreas(rc)
		if inv != a.paletteInvertArea(rc) || tr != a.paletteTransparentArea(rc) {
			t.Fatal("palette render and click regions differ")
		}
		if inv.Right+2 != tr.Left || inv.Bottom-inv.Top != 46 || tr.Right-tr.Left != 46 {
			t.Fatalf("swatches not adjacent 46x46 squares: %v, %v", inv, tr)
		}
		if tr.Right+4 > rc.Right {
			t.Fatalf("swatches clipped by panel for mode=%d: %v panel=%v", tc.mode, tr, rc)
		}
		if inv.Left <= 70 {
			t.Fatalf("invert swatch overlays main colors: %v", inv)
		}
	}
}
