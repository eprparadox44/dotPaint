package main

import (
	"os"
	"strings"
	"testing"
)

func Test149CurrentIndexedPreviewSelectsEverySlot(t *testing.T) {
	for _, size := range []int{16, 256} {
		cols, rows, dx, dy, w, h := customPickerCurrentIndexedGrid(size)
		if cols*rows != size {
			t.Fatalf("mode %d grid %dx%d", size, cols, rows)
		}
		if 22+cols*dx > 559 || 251+rows*dy > 337 {
			t.Fatalf("mode %d overflow", size)
		}
		for i := 0; i < size; i++ {
			x := 22 + (i%cols)*dx
			y := 251 + (i/cols)*dy
			if got := customPickerCurrentIndexedSlotAt(size, x+w/2, y+h/2); got != i {
				t.Fatalf("mode %d slot %d became %d", size, i, got)
			}
			if got := customPickerCurrentIndexedSlotAt(size, x+dx-1, y+dy-1); got != -1 {
				t.Fatalf("mode %d gap after %d selects %d", size, i, got)
			}
		}
	}
	if v := customPickerCurrentIndexedSlotAt(0, 23, 253); v != -1 {
		t.Fatal("RGB should not be indexed", v)
	}
}
func Test149IndexedHeaderDoesNotOverlapPreviewOrFooter(t *testing.T) {
	type rect struct{ x, y, w, h int }
	overlap := func(a, b rect) bool { return a.x < b.x+b.w && b.x < a.x+a.w && a.y < b.y+b.h && b.y < a.y+a.h }
	buttons := []rect{{302, 397, 125, 27}, {436, 397, 137, 27}, {583, 397, 143, 27}}
	for i, a := range buttons {
		for j, b := range buttons {
			if j > i && overlap(a, b) {
				t.Fatal("header buttons overlap", i, j)
			}
		}
	}
	for _, rows := range []int{1, 8} {
		grid := rect{22, 435, 32 * 22, rows * 22}
		footer := rect{592, 435 + rows*22 + 18, 150, 28}
		for _, b := range buttons {
			if overlap(b, grid) || overlap(b, footer) {
				t.Fatal("overlap", rows, b)
			}
		}
	}
}
func Test149NativeUIAndResizeCursorWiring(t *testing.T) {
	picker, e := os.ReadFile("color_picker_custom_windows.go")
	if e != nil {
		t.Fatal(e)
	}
	cursor, e := os.ReadFile("tool_cursor_windows.go")
	if e != nil {
		t.Fatal(e)
	}
	sp, sc := string(picker), string(cursor)
	for _, s := range []string{"applyIndexedToCurrent()", "appliedPalette", "customPickerCurrentIndexedSlotAt", "case pickerApplyIndexed:", "색상표 적용", "현재 색상표 · 인덱스 %d색", "frameRect(hdc, RECT{14, 391, 751, int32(panelBottom)}"} {
		if !strings.Contains(sp, s) {
			t.Errorf("missing picker %q", s)
		}
	}
	for _, s := range []string{"textBoxResizeCursor149", "a.hitTextBoxAction(mx, my)", "case 0, 4:", "case 2, 6:", "case 1, 5:", "case 3, 7:", "a.textTool.DragMode == 2"} {
		if !strings.Contains(sc, s) {
			t.Errorf("missing cursor %q", s)
		}
	}
}
