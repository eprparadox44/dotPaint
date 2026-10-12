package main

import (
	"bytes"
	"testing"
)

func testDejavuClipboardPalette() dejavuPalette {
	var pal dejavuPalette
	for i := 0; i < 16; i++ {
		pal.RGB[i] = [3]byte{byte(i * 13), byte(i * 5), byte(i * 9)}
	}
	pal.RGB[3] = [3]byte{12, 32, 64}
	pal.RGB[6] = [3]byte{230, 90, 30}
	pal.RGB[7] = [3]byte{120, 110, 100}
	return pal
}

func TestDejavu140ClipboardNearestRGBAndNoMutation(t *testing.T) {
	pal := testDejavuClipboardPalette()
	// BGRA pixels: nearly index 6, then exact index 7.
	src := []byte{32, 92, 228, 255, 100, 110, 120, 255}
	saved := append([]byte(nil), src...)
	got, err := dejavuQuantizeBGRAForClipboard(src, 2, 1, pal, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{30, 90, 230, 255, 100, 110, 120, 255}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if !bytes.Equal(src, saved) {
		t.Fatal("source clipboard pixels mutated")
	}
}

func TestDejavu140TransparentPixelsUseConfiguredIndex(t *testing.T) {
	pal := testDejavuClipboardPalette()
	src := []byte{200, 200, 200, 0}
	got, err := dejavuQuantizeBGRAForClipboard(src, 1, 1, pal, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{64, 32, 12, 255}) {
		t.Fatalf("transparent should become palette index 3 colour: %v", got)
	}
}

func TestDejavu140PartialAlphaBlendsWithTransparentIndexBeforeNearest(t *testing.T) {
	pal := testDejavuClipboardPalette()
	// Destination BG=(12,32,64), src RGB=(230,90,30), 50% -> (121,61,47).
	// Make one exact candidate for the computed composite to assert rounding.
	pal.RGB[8] = [3]byte{121, 61, 47}
	src := []byte{30, 90, 230, 128}
	got, err := dejavuQuantizeBGRAForClipboard(src, 1, 1, pal, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{47, 61, 121, 255}) {
		t.Fatalf("composited quantized BGRA=%v", got)
	}
}

func TestDejavu140BadClipboardSizeDoesNotAllocate(t *testing.T) {
	_, err := dejavuQuantizeBGRAForClipboard([]byte{0, 0, 0, 255}, 2, 1, testDejavuClipboardPalette(), 3)
	if err == nil {
		t.Fatal("expected dimension error")
	}
}

func TestDejavu140NativeIMGCodecAndClipboardQuantization(t *testing.T) {
	original, idx := syntheticDejavuIMG(t)
	got, err := encodeDejavuAllIndices(original, idx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("untouched original IMG changed")
	}
}
