package main

import "testing"

func TestCursorLumaThreshold128(t *testing.T) {
	if !cursorBackgroundNeedsLight(127, 127, 127) {
		t.Fatal("RGB 127 must select the light cursor")
	}
	if cursorBackgroundNeedsLight(128, 128, 128) {
		t.Fatal("RGB 128 must select the dark cursor")
	}
}

func TestCursorContrastPixel(t *testing.T) {
	tests := []struct {
		name                string
		r, g, b             byte
		wantLight           bool
		wantR, wantG, wantB byte
	}{
		{"black-to-light", 0, 0, 0, true, 255, 255, 255},
		{"white-stays-light", 255, 255, 255, true, 255, 255, 255},
		{"white-to-dark", 255, 255, 255, false, 0, 0, 0},
		{"black-stays-dark", 0, 0, 0, false, 0, 0, 0},
		{"dark-colour-inverts", 20, 40, 80, true, 235, 215, 175},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, g, b := cursorContrastPixel(tc.r, tc.g, tc.b, tc.wantLight)
			if r != tc.wantR || g != tc.wantG || b != tc.wantB {
				t.Fatalf("got %d,%d,%d want %d,%d,%d", r, g, b, tc.wantR, tc.wantG, tc.wantB)
			}
		})
	}
}
