//go:build windows

package main

import "testing"

func Test131HistoryRestoresPaletteSizeActiveCountAndOriginalImage(t *testing.T) {
	a := newApp()
	a.canvas = NewCanvas(2, 1, rgb(42, 17, 203))
	a.resetHistory("RGB 원본")
	original := append([]byte(nil), a.canvas.Pix...)
	pal := make([]uint32, 256)
	for i := range pal {
		pal[i] = indexedColorRef(byte(i), byte(i), byte(i))
	}
	pal[220] = indexedColorRef(42, 17, 203)
	if err := a.applyIndexedColorTableWithCount(pal, "", 256); err != nil {
		t.Fatal(err)
	}
	if !a.indexedMode || a.indexedSize != 256 || a.indexedActiveCount != 256 {
		t.Fatalf("mode count %d %d", a.indexedSize, a.indexedActiveCount)
	}
	if len(a.historyEntries) != 2 || a.historyEntries[1].IndexedPixels[0] != 220 {
		t.Fatalf("index lost: %v", a.historyEntries)
	}
	if !a.gotoHistory(0) || a.indexedMode || string(a.canvas.Pix) != string(original) {
		t.Fatal("RGB original lost after indexed conversion")
	}
	if !a.gotoHistory(1) || !a.indexedMode || a.indexedActiveCount != 256 || a.historyEntries[1].IndexedPixels[0] != 220 {
		t.Fatal("indexed state/index lost after history navigation")
	}
	if err := a.applyIndexedColorTableWithCount(pal, "", 24); err != nil {
		t.Fatal(err)
	}
	if a.indexedSize != 256 || a.indexedActiveCount != 24 {
		t.Fatal("active count altered table capacity")
	}
	if !a.gotoHistory(1) || a.indexedActiveCount != 256 || a.historyEntries[1].IndexedPixels[0] != 220 {
		t.Fatal("history did not restore 256-colour index")
	}
}
