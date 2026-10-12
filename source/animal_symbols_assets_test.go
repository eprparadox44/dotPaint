package main

import (
	"image/png"
	"os"
	"testing"
)

func TestAnimalSymbolAssets(t *testing.T) {
	cases := []struct {
		path string
		w, h int
	}{
		{"assets/animal/adelie_penguin_1.png", 287, 395},
		{"assets/animal/adelie_penguin_2.png", 475, 313},
		{"assets/animal/mouse.png", 389, 225},
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
