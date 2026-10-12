package main

import (
	"bytes"
	"testing"
)

func TestDejavu141TransparentPasteKeepsDestination(t *testing.T) {
	pal := testDejavuClipboardPalette()
	// Simulates transparency from a PNG on an existing non-black tile.
	dst := []byte{100, 110, 120, 255}
	src := []byte{0, 0, 0, 0}
	before := append([]byte(nil), dst...)
	dejavuCompositeIndexedPastePixel(dst, src, pal)
	if !bytes.Equal(dst, before) {
		t.Fatalf("alpha=0 unexpectedly overwrote tile: %v", dst)
	}
	if !bytes.Equal(src, []byte{0, 0, 0, 0}) {
		t.Fatal("clipboard input mutated")
	}
}

func TestDejavu141OpaquePasteMapsExactPaletteEvenBackgroundColour(t *testing.T) {
	pal := testDejavuClipboardPalette()
	dst := []byte{100, 110, 120, 255}
	src := []byte{29, 88, 233, 255}
	dejavuCompositeIndexedPastePixel(dst, src, pal)
	want := []byte{30, 90, 230, 255}
	if !bytes.Equal(dst, want) {
		t.Fatalf("got %v want %v", dst, want)
	}
}

func TestDejavu141PartialAlphaUsesUnderlyingTileAndIsMoveSafe(t *testing.T) {
	pal := testDejavuClipboardPalette()
	pal.RGB[8] = [3]byte{121, 61, 47}
	src := []byte{30, 90, 230, 128}
	original := append([]byte(nil), src...)
	a := []byte{64, 32, 12, 255}
	b := []byte{100, 110, 120, 255}
	dejavuCompositeIndexedPastePixel(a, src, pal)
	dejavuCompositeIndexedPastePixel(b, src, pal)
	if !bytes.Equal(a, []byte{47, 61, 121, 255}) {
		t.Fatalf("wrong composite into background A: %v", a)
	}
	if bytes.Equal(b, a) {
		t.Fatal("blend must depend on new underlying tile after moving")
	}
	if !bytes.Equal(src, original) {
		t.Fatal("source changed when recompositing")
	}
}

func TestDejavu141TransparentAndOpaquePasteAvoidsBlackRectangle(t *testing.T) {
	pal := testDejavuClipboardPalette()
	// Three destination tile pixels, all paletted, surrounded by alpha holes.
	dst := []byte{100, 110, 120, 255, 64, 32, 12, 255, 100, 110, 120, 255}
	base := append([]byte(nil), dst...)
	src := []byte{0, 0, 0, 0, 30, 90, 230, 255, 0, 0, 0, 0}
	for i := 0; i < 3; i++ {
		dejavuCompositeIndexedPastePixel(dst[4*i:4*i+4], src[4*i:4*i+4], pal)
	}
	if !bytes.Equal(dst[:4], base[:4]) || !bytes.Equal(dst[8:], base[8:]) {
		t.Fatalf("transparent holes changed: %v", dst)
	}
	if !bytes.Equal(dst[4:8], []byte{30, 90, 230, 255}) {
		t.Fatalf("opaque middle bad: %v", dst[4:8])
	}
}
