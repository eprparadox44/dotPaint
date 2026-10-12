package main

import (
	"image/png"
	"os"
	"testing"
)

func TestCircuitSymbolAssets(t *testing.T) {
	cases := []struct {
		path string
		w, h int
	}{
		{"assets/circuit/switch.png", 76, 60},
		{"assets/circuit/diode.png", 64, 56},
		{"assets/circuit/resistor.png", 104, 60},
		{"assets/circuit/variable_resistor.png", 111, 55},
		{"assets/circuit/capacitor.png", 51, 61},
		{"assets/circuit/ammeter.png", 78, 48},
		{"assets/circuit/voltmeter.png", 72, 52},
		{"assets/circuit/coil.png", 115, 66},
		{"assets/circuit/ac_source.png", 61, 61},
		{"assets/circuit/dc_source.png", 41, 47},
		{"assets/circuit/npn_transistor.png", 160, 114},
		{"assets/circuit/pnp_transistor.png", 160, 112},
	}
	for _, tc := range cases {
		f, err := os.Open(tc.path)
		if err != nil {
			t.Fatalf("open %s: %v", tc.path, err)
		}
		img, err := png.Decode(f)
		_ = f.Close()
		if err != nil {
			t.Fatalf("decode %s: %v", tc.path, err)
		}
		b := img.Bounds()
		if b.Dx() != tc.w || b.Dy() != tc.h {
			t.Fatalf("%s size=%dx%d want %dx%d", tc.path, b.Dx(), b.Dy(), tc.w, tc.h)
		}
		hasTransparent, hasOpaque := false, false
		for y := b.Min.Y; y < b.Max.Y && !(hasTransparent && hasOpaque); y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				_, _, _, a := img.At(x, y).RGBA()
				if a == 0 {
					hasTransparent = true
				}
				if a == 0xffff {
					hasOpaque = true
				}
			}
		}
		if !hasTransparent || !hasOpaque {
			t.Fatalf("%s must retain both transparent and opaque pixels", tc.path)
		}
	}
}
