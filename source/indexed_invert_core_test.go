package main

import "testing"

func TestIndexedInvertPairLUT(t *testing.T) {
	pal := []uint32{0x000000, 0xffffff, 0x0000ff, 0xffff00} // BGR COLORREF
	lut := indexedInvertLookup(pal)
	for i, want := range []byte{1, 0, 3, 2} {
		if lut[i] != want {
			t.Fatalf("lut[%d]=%d want=%d", i, lut[i], want)
		}
	}
	p := []byte{0, 0, 0, 255}
	ids := []byte{0}
	applyIndexedInvertPixel(p, 0, ids, pal, lut)
	if ids[0] != 1 || p[0] != 255 || p[1] != 255 || p[2] != 255 {
		t.Fatal("first inversion not mapped to white")
	}
	applyIndexedInvertPixel(p, 0, ids, pal, lut)
	if ids[0] != 0 || p[0] != 0 || p[1] != 0 || p[2] != 0 {
		t.Fatal("second inversion did not restore black")
	}
}

func TestIndexedInvertMaskOnlyCovered(t *testing.T) {
	pal := []uint32{0, 0xffffff}
	base := []byte{0, 0, 0, 255, 255, 255, 255, 255}
	mask := []byte{255, 255, 255, 255, 255, 255, 255, 100}
	invertIndexedMask(base, mask, pal, indexedInvertLookup(pal))
	if base[0] != 255 || base[4] != 255 {
		t.Fatalf("incorrect coverage: %v", base)
	}
}
