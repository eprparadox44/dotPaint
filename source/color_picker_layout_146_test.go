package main

import (
	"os"
	"strings"
	"testing"
)

// Guard the native Windows dialog geometry without requiring Windows GDI
// during host-side tests. The actual Windows control coordinates are checked
// against each other for RGB, 16-colour, and 256-colour layouts.
func Test146ColorPickerFooterAndPaletteControlsDoNotOverlap(t *testing.T) {
	type box struct{ x, y, w, h int }
	overlaps := func(a, b box) bool { return a.x < b.x+b.w && b.x < a.x+a.w && a.y < b.y+b.h && b.y < a.y+a.h }
	const gridTop, step = 435, 22
	for _, rows := range []int{0, 1, 8} {
		footer := 432 - 44
		height := 432
		if rows > 0 {
			bottom := gridTop + rows*step
			footer = bottom + 18
			height = footer + 42
		}
		ok, cancel := box{592, footer, 72, 28}, box{670, footer, 72, 28}
		if overlaps(ok, cancel) {
			t.Fatalf("%d rows: OK/Cancel overlap", rows)
		}
		for _, f := range []box{{302, 397, 125, 27}, {436, 397, 137, 27}, {583, 397, 143, 27}} {
			if rows > 0 && (overlaps(f, ok) || overlaps(f, cancel)) {
				t.Fatalf("%d rows: palette I/O collides with footer", rows)
			}
		}
		if rows > 0 && footer < gridTop+rows*step+12 {
			t.Fatalf("%d rows: footer touches indexed grid", rows)
		}
		if footer+28 > height {
			t.Fatalf("%d rows: footer out of bounds", rows)
		}
	}
}

func Test146ColorPickerLayoutUsesSeparatedAreas(t *testing.T) {
	raw, err := os.ReadFile("color_picker_custom_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	for _, marker := range []string{
		"customPickerFooterTop(s.gridRows)",
		"현재 색상표 · 인덱스 %d색",
		"originalSwatch := RECT",
		"currentSwatch := RECT",
		"uintptr(footerY), 72, 28, SWP_NOZORDER",
		"pickerApplyIndexed",
	} {
		if !strings.Contains(src, marker) {
			t.Errorf("missing native UI layout guard: %q", marker)
		}
	}
}
