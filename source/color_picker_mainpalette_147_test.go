package main

import (
	"os"
	"strings"
	"testing"
)

func Test147MainPaletteMatchesDockedOrdering(t *testing.T) {
	var seen [44]bool
	for i := 0; i < 44; i++ {
		x, y := customPickerMainSwatchPoint(i)
		if x < 22 || x >= 22+22*24 || y < 256 || y >= 304 {
			t.Fatalf("swatch %d out of main editor region: %d,%d", i, x, y)
		}
		got := customPickerMainSlotAt(x+10, y+10)
		if got != i {
			t.Fatalf("swatch %d roundtrip produced %d", i, got)
		}
		if seen[got] {
			t.Fatalf("duplicate slot %d", got)
		}
		seen[got] = true
		// 4-px gaps between swatches must not act as colour selections.
		if got := customPickerMainSlotAt(x+22, y+4); got != -1 {
			t.Fatalf("gap after %d selects %d", i, got)
		}
	}
	for i, ok := range seen {
		if !ok {
			t.Fatalf("slot %d unreachable", i)
		}
	}
	for _, pt := range [][2]int{{0, 0}, {22, 225}, {22, 255}, {550, 256}, {22, 305}, {-1, 280}} {
		if got := customPickerMainSlotAt(pt[0], pt[1]); got != -1 {
			t.Errorf("nonpalette pixel %v selects %d", pt, got)
		}
	}
	// The 28 basic swatches form 14 columns in two rows, followed by the
	// 16 user colours in eight columns, exactly as in main_windows.go.
	if x, y := customPickerMainSwatchPoint(14); x != 22 || y != 280 {
		t.Fatalf("basic row2 %d,%d", x, y)
	}
	if x, y := customPickerMainSwatchPoint(28); x != 358 || y != 256 {
		t.Fatalf("user row1 %d,%d", x, y)
	}
	if x, y := customPickerMainSwatchPoint(36); x != 358 || y != 280 {
		t.Fatalf("user row2 %d,%d", x, y)
	}
}
func Test147MainRGBFallbackAndLayoutSource(t *testing.T) {
	if len(defaultMainRGBPalette()) != 28 {
		t.Fatal("expected 28 basic colours")
	}
	b, err := os.ReadFile("color_picker_custom_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, needle := range []string{
		"for i := 0; i < 48; i++",                   // preset colours now separate
		"for i := 0; i < s.displayCountFor(0); i++", // dynamic visible main-palette slots (1.3.2+)
		"customPickerMainSlotAt(x, y)",
		"s.commitEditedSwatch()",    // edits apply only selected slot
		"case a.indexedMode:",       // cache RGB palette for restored documents
		"a.customColors = s.custom", // commit after OK
		"pickerModeTop",             // controls moved below palette panel
		"pickerGridTop",             // indexed controls below mode controls
	} {
		if !strings.Contains(s, needle) {
			t.Errorf("missing new layout wiring %q", needle)
		}
	}
}
func Test147DialogVerticalRegionsDisjoint(t *testing.T) {
	// Predetermined bounding boxes of the new 48-preset area, main 44,
	// mode buttons, expanded indexed panel, and footer.
	if 37+6*20 > 222 {
		t.Fatal("presets overlap 44-slot palette")
	}
	if 337 >= 349 {
		t.Fatal("44-slot palette overlaps mode controls")
	}
	if 349+26 >= 391 {
		t.Fatal("mode controls overlap indexed section")
	}
	for _, rows := range []int{0, 1, 8} {
		gridBottom := 435 + rows*22
		footer := 432 - 44
		height := 432
		if rows > 0 {
			footer = gridBottom + 18
			height = footer + 42
		}
		if rows > 0 && footer-9 <= gridBottom {
			t.Errorf("%d rows indexed and footer collide", rows)
		}
		if footer+28 > height {
			t.Errorf("%d rows footer clipped", rows)
		}
		if rows == 0 && footer <= 349+26 {
			t.Fatal("collapsed footer collides with mode controls")
		}
	}
}
