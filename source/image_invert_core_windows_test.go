package main

import "testing"

func TestInvertClipboardImageRGBPreservesAlphaAndMask(t *testing.T) {
	img := ClipboardImage{
		W: 2, H: 1,
		Pix:  []byte{10, 20, 30, 40, 200, 150, 100, 255},
		Mask: []bool{true, false},
	}
	got := invertClipboardImageRGB(img)
	want := []byte{245, 235, 225, 40, 55, 105, 155, 255}
	for i := range want {
		if got.Pix[i] != want[i] {
			t.Fatalf("pixel byte %d=%d want %d", i, got.Pix[i], want[i])
		}
	}
	if len(got.Mask) != 2 || !got.Mask[0] || got.Mask[1] {
		t.Fatalf("mask changed: %#v", got.Mask)
	}
	got.Pix[0] = 0
	if img.Pix[0] != 10 {
		t.Fatal("result aliases source pixel buffer")
	}
}
