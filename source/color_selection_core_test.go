package main

import "testing"

func putPixel(pix []byte, w, x, y int, b, g, r, a byte) {
	i := (y*w + x) * 4
	pix[i+0], pix[i+1], pix[i+2], pix[i+3] = b, g, r, a
}

func selected(mask []bool, x, y, w, h, px, py int) bool {
	if px < x || py < y || px >= x+w || py >= y+h {
		return false
	}
	if len(mask) != w*h {
		return true
	}
	return mask[(py-y)*w+(px-x)]
}

func TestColorSelectionContiguousVsAll(t *testing.T) {
	const w, h = 5, 3
	pix := make([]byte, w*h*4)
	for i := 3; i < len(pix); i += 4 {
		pix[i] = 255
	}
	// Two separated exact-red islands and one nearby red neighbour.
	putPixel(pix, w, 0, 0, 0, 0, 100, 255)
	putPixel(pix, w, 1, 0, 0, 0, 104, 255)
	putPixel(pix, w, 4, 2, 0, 0, 100, 255)

	x, y, sw, sh, mask, count := buildColorSelectionMask(pix, w, h, 0, 0, 4, false)
	if count != 2 || !selected(mask, x, y, sw, sh, 0, 0) || !selected(mask, x, y, sw, sh, 1, 0) || selected(mask, x, y, sw, sh, 4, 2) {
		t.Fatalf("contiguous mismatch count=%d bounds=%d,%d %dx%d mask=%v", count, x, y, sw, sh, mask)
	}

	x, y, sw, sh, mask, count = buildColorSelectionMask(pix, w, h, 0, 0, 4, true)
	if count != 3 || !selected(mask, x, y, sw, sh, 4, 2) {
		t.Fatalf("global mismatch count=%d bounds=%d,%d %dx%d mask=%v", count, x, y, sw, sh, mask)
	}
}

func TestRGBEuclideanMismatchBoundary(t *testing.T) {
	const w, h = 5, 1
	pix := make([]byte, w*h*4)
	putPixel(pix, w, 0, 0, 10, 20, 30, 255) // seed
	putPixel(pix, w, 1, 0, 10, 20, 31, 255) // distance 1
	putPixel(pix, w, 2, 0, 10, 80, 90, 255) // sqrt(60^2+60^2) ~= 84.85
	putPixel(pix, w, 3, 0, 68, 78, 88, 255) // sqrt(3*58^2) ~= 100.46
	putPixel(pix, w, 4, 0, 0, 0, 0, 0)      // transparent

	_, _, _, _, _, exact := buildColorSelectionMask(pix, w, h, 0, 0, 0, true)
	if exact != 1 {
		t.Fatalf("mismatch 0 selected %d, want 1", exact)
	}
	_, _, _, _, _, at84 := buildColorSelectionMask(pix, w, h, 0, 0, 84, true)
	if at84 != 2 {
		t.Fatalf("mismatch 84 selected %d, want 2", at84)
	}
	_, _, _, _, _, at85 := buildColorSelectionMask(pix, w, h, 0, 0, 85, true)
	if at85 != 3 {
		t.Fatalf("mismatch 85 selected %d, want 3", at85)
	}
	_, _, _, _, _, at100 := buildColorSelectionMask(pix, w, h, 0, 0, 100, true)
	if at100 != 3 {
		t.Fatalf("mismatch 100 selected %d, want 3", at100)
	}
	_, _, _, _, _, transparent := buildColorSelectionMask(pix, w, h, 4, 0, 100, true)
	if transparent != 1 {
		t.Fatalf("transparent selected %d, want 1", transparent)
	}
}

func TestColorMismatchClamp(t *testing.T) {
	if clampColorTolerance(-1) != 0 || clampColorTolerance(101) != 100 || clampColorTolerance(67) != 67 {
		t.Fatalf("unexpected mismatch clamp")
	}
}

func BenchmarkColorSelectionAllFullHD(b *testing.B) {
	const w, h = 1920, 1080
	pix := make([]byte, w*h*4)
	for i := 0; i < w*h; i++ {
		pix[i*4+0] = byte(i)
		pix[i*4+1] = byte(i >> 3)
		pix[i*4+2] = byte(i >> 7)
		pix[i*4+3] = 255
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildColorSelectionMask(pix, w, h, w/2, h/2, 40, true)
	}
}

func BenchmarkColorSelectionContiguousFullHD(b *testing.B) {
	const w, h = 1920, 1080
	pix := make([]byte, w*h*4)
	for i := 0; i < w*h; i++ {
		pix[i*4+0] = 80
		pix[i*4+1] = 120
		pix[i*4+2] = 160
		pix[i*4+3] = 255
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildColorSelectionMask(pix, w, h, w/2, h/2, 0, false)
	}
}
