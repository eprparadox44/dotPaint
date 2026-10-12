package main

import (
	"os"
	"strings"
	"testing"
)

// Guard against the 1.3.1 regression: typing a swatch count must not
// unexpectedly switch colour modes or quantize the current image.
func TestPaletteDisplayCountDoesNotChangeDocumentMode(t *testing.T) {
	pickerBytes, err := os.ReadFile("color_picker_custom_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	picker := string(pickerBytes)
	a := strings.Index(picker, "func (s *customPickerState) readColorCount() bool {")
	b := strings.Index(picker[a:], "func (s *customPickerState) updateLayout()")
	if a < 0 || b < 0 {
		t.Fatal("colour count handler missing")
	}
	handler := picker[a : a+b]
	for _, forbidden := range []string{"modeChange(", "s.colorCount =", "s.changed =", "quantizeIndexedBGRA", "applyIndexedColorTable"} {
		if strings.Contains(handler, forbidden) {
			t.Errorf("visible count changed image mode or quantization: %s", forbidden)
		}
	}
	if !strings.Contains(handler, "s.storeDisplayCount(") {
		t.Error("display count not updated")
	}
	mainBytes, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	main := string(mainBytes)
	for _, want := range []string{"palettePanelActualHeight", "paletteVisibleCount()", "paletteDisplayColumns(", "paletteDisplayPosition(", "paletteDisplayRGB, paletteDisplay16, paletteDisplay256"} {
		if !strings.Contains(main, want) {
			t.Errorf("missing main palette display implementation %s", want)
		}
	}
}
