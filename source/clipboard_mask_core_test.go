package main

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"
)

func TestClipboardSelectionMaskClearsOnlyUnselectedPixels(t *testing.T) {
	pix := []byte{
		1, 2, 3, 255,
		4, 5, 6, 128,
		7, 8, 9, 255,
		10, 11, 12, 0,
		13, 14, 15, 255,
		16, 17, 18, 255,
	}
	mask := []bool{true, false, true, false, false, true}
	want := []byte{
		1, 2, 3, 255,
		0, 0, 0, 0,
		7, 8, 9, 255,
		0, 0, 0, 0,
		0, 0, 0, 0,
		16, 17, 18, 255,
	}
	kept, cleared := applyClipboardSelectionMask(pix, mask, 3, 2)
	if kept != 3 || cleared != 3 {
		t.Fatalf("kept=%d cleared=%d, want 3 and 3", kept, cleared)
	}
	if !bytes.Equal(pix, want) {
		t.Fatalf("masked pixels=%v, want %v", pix, want)
	}
}

func TestClipboardSelectionMaskLeavesRectangleUnchanged(t *testing.T) {
	pix := []byte{1, 2, 3, 255, 4, 5, 6, 255}
	want := append([]byte(nil), pix...)
	kept, cleared := applyClipboardSelectionMask(pix, nil, 2, 1)
	if kept != 0 || cleared != 0 || !bytes.Equal(pix, want) {
		t.Fatalf("rectangular selection changed: kept=%d cleared=%d pix=%v", kept, cleared, pix)
	}
}

func TestClipboardSelectionMaskRejectsMalformedMask(t *testing.T) {
	pix := []byte{1, 2, 3, 255, 4, 5, 6, 255}
	want := append([]byte(nil), pix...)
	applyClipboardSelectionMask(pix, []bool{false}, 2, 1)
	if !bytes.Equal(pix, want) {
		t.Fatalf("malformed mask changed pixels: %v", pix)
	}
}

func TestClipboardSelectionMaskIsTransparentInExportedPNG(t *testing.T) {
	pix := []byte{
		30, 20, 10, 255,
		90, 80, 70, 255,
	}
	applyClipboardSelectionMask(pix, []bool{true, false}, 2, 1)
	data, err := buildTransparentClipboardPNG(2, 1, pix)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	selected := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
	outside := color.NRGBAModel.Convert(img.At(1, 0)).(color.NRGBA)
	if selected != (color.NRGBA{R: 10, G: 20, B: 30, A: 255}) {
		t.Fatalf("selected pixel=%v", selected)
	}
	if outside != (color.NRGBA{}) {
		t.Fatalf("outside pixel=%v, want fully transparent", outside)
	}
}
