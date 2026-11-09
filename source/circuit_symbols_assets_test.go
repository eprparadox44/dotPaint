package main

import (
	"dotpaint/internal/webplossless"
	"os"
	"testing"
)

func TestCircuitSymbolAssets(t *testing.T) {
	cases := []struct {
		path string
		w, h int
	}{
		{"assets/circuit/switch.webp", 76, 60},
		{"assets/circuit/diode.webp", 64, 56},
		{"assets/circuit/resistor.webp", 104, 60},
		{"assets/circuit/variable_resistor.webp", 111, 55},
		{"assets/circuit/capacitor.webp", 51, 61},
		{"assets/circuit/ammeter.webp", 78, 48},
		{"assets/circuit/voltmeter.webp", 72, 52},
		{"assets/circuit/coil.webp", 115, 66},
		{"assets/circuit/ac_source.webp", 61, 61},
		{"assets/circuit/dc_source.webp", 41, 47},
		{"assets/circuit/npn_transistor.webp", 160, 114},
		{"assets/circuit/pnp_transistor.webp", 160, 112},
	}
	for _, tc := range cases {
		f, err := os.Open(tc.path)
		if err != nil {
			t.Fatalf("open %s: %v", tc.path, err)
		}
		data, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatalf("read %s: %v", tc.path, err)
		}
		img, err := webplossless.Decode(data)
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
