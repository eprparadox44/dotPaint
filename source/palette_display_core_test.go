package main

import "testing"

func TestPaletteDisplayIsVisibilityOnly(t *testing.T) {
	for _, mode := range []int{0, 16, 256} {
		cap := paletteDisplayCapacity(mode)
		if got := paletteDisplayClamp(mode, 0); got != cap {
			t.Fatalf("%d default=%d", mode, got)
		}
		for count := 1; count <= cap; count++ {
			cols := paletteDisplayColumns(mode, count)
			rows := paletteDisplayRows(mode, count)
			if cols < 1 || rows < 1 || cols > 22 || rows > 16 {
				t.Fatalf("mode=%d count=%d layout=%dx%d", mode, count, cols, rows)
			}
			seen := map[[2]int]bool{}
			for i := 0; i < count; i++ {
				x, y := paletteDisplayPosition(mode, i, cols)
				if x < 0 || y < 0 || x >= cols || y >= rows || seen[[2]int{x, y}] {
					t.Fatalf("mode=%d n=%d index=%d coord=%d,%d", mode, count, i, x, y)
				}
				seen[[2]int{x, y}] = true
			}
		}
	}
	if paletteDisplayColumns(0, 44) != 22 || paletteDisplayRows(0, 44) != 2 {
		t.Fatal("44 colour legacy geometry")
	}
	if paletteDisplayColumns(16, 16) != 8 || paletteDisplayRows(16, 16) != 2 {
		t.Fatal("16 indexed geometry")
	}
	if paletteDisplayColumns(256, 256) != 16 || paletteDisplayRows(256, 256) != 16 {
		t.Fatal("256 indexed must show all")
	}
}
