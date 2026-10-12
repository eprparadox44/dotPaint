package main

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"os"
	"strings"
	"testing"
)

func Test148MainPaletteDuplicateRGBRemembersExactSlot(t *testing.T) {
	base := defaultMainRGBPalette()
	var custom [16]uint32
	for i := range custom {
		custom[i] = 0xffffff
	}
	for _, slot := range []int{14, 28, 29, 36, 43} {
		got := customPickerInitialMainSlot(base, custom, 0xffffff, slot)
		if got != slot {
			t.Fatalf("duplicate white: wanted %d, got %d", slot, got)
		}
	}
	if got := customPickerInitialMainSlot(base, custom, 0xffffff, -1); got != 14 {
		t.Fatalf("fallback should use first white, got %d", got)
	}
	if got := customPickerInitialMainSlot(base, custom, 0xffffff, 2); got != 14 {
		t.Fatalf("stale slot should not win, got %d", got)
	}
	if got := customPickerInitialMainSlot(base, custom, 0x123456, -1); got != -1 {
		t.Fatalf("unknown RGB should be free, got %d", got)
	}
}

func Test148IndexedGridKeepsAll256Indices(t *testing.T) {
	for _, tc := range []struct{ mode, cols, rows int }{{0, 0, 0}, {16, 16, 1}, {256, 32, 8}} {
		cols, rows := customPickerIndexedGrid(tc.mode)
		if cols != tc.cols || rows != tc.rows {
			t.Fatalf("grid %d => %dx%d", tc.mode, cols, rows)
		}
		seen := make([]bool, tc.mode)
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				idx := customPickerGridIndex(x, y, cols, rows)
				if idx < 0 || idx >= len(seen) || seen[idx] {
					t.Fatalf("invalid/duplicate grid index %d", idx)
				}
				seen[idx] = true
			}
		}
		for i, v := range seen {
			if !v {
				t.Fatalf("palette %d misses %d", tc.mode, i)
			}
		}
	}
}

func Test148GIFEncodingIsReallyGIFAndPreservesTransparency(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	im.Set(0, 0, color.NRGBA{R: 255, A: 255})
	im.Set(1, 0, color.NRGBA{G: 255, A: 255})
	im.Set(2, 0, color.NRGBA{B: 255, A: 255})
	var buf bytes.Buffer
	if err := encodeStillGIF(&buf, im); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("GIF8")) {
		t.Fatalf("not GIF: %q", buf.Bytes()[:8])
	}
	decoded, err := gif.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != im.Bounds() {
		t.Fatal("wrong bounds")
	}
	_, _, _, a := decoded.At(1, 1).RGBA()
	if a != 0 {
		t.Fatalf("lost transparent pixel: alpha=%d", a)
	}
}

func Test148NativeColorPickerWiring(t *testing.T) {
	win, err := os.ReadFile("color_picker_custom_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	main, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []string{"preferred := a.fgPaletteSlot", "customPickerInitialMainSlot(", "s.editTarget = 1", "customPickerIndexedGrid(s.mode)"} {
		if !strings.Contains(string(win), x) {
			t.Errorf("picker missing %q", x)
		}
	}
	for _, x := range []string{"a.bgPaletteSlot = slot", "a.fgPaletteSlot = slot", "return encodeStillGIF(f, c.toRGBA())"} {
		if !strings.Contains(string(main), x) {
			t.Errorf("main missing %q", x)
		}
	}
}

func Test148UndoBudgetKeepsNewestCheckpoint(t *testing.T) {
	for _, tc := range []struct {
		sizes        []int
		budget, want int
	}{
		{[]int{10, 10, 10}, 25, 1},
		{[]int{200, 200, 200}, 256, 2},
		{[]int{500}, 256, 0},
		{[]int{100, 500}, 256, 1},
		{[]int{1, 2, 3}, 100, 0},
		{[]int{}, 1, 0},
	} {
		if got := undoOldestDropCount(tc.sizes, tc.budget); got != tc.want {
			t.Fatalf("undo sizes %v budget %d: discard %d, want %d", tc.sizes, tc.budget, got, tc.want)
		}
	}
}
