package main

import (
	"os"
	"strings"
	"testing"
)

func TestBeta41SelectionOutlineAndPaletteSourceContract(t *testing.T) {
	checks := map[string][]string{
		"object_outline_windows.go": {
			"a.tool != ToolObjectSelect",
			"if i == a.objectEditingIndex",
			"selectedColor, selectedFill := uiColor(rgb(0, 120, 215))",
			"plainColor, plainFill := uiColor(rgb(188, 188, 188))",
		},
		"main_windows.go": {
			"func (a *App) paletteSpecialAreas(rc RECT) (invert RECT, transparent RECT)",
			"invert = RECT{left, top, left + 46, top + 46}",
			"transparent = RECT{left + 48, top, left + 94, top + 46}",
			"ir, tr := a.paletteSpecialAreas(rc)",
			"a.paintInvertIcon(hdc, ir)",
			"a.chooseInvert(right)",
		},
		"beta4_windows.go": {
			"//go:embed invert_color_reference.png",
			"invert, _ := a.paletteSpecialAreas(rc)",
			"r.Left += rc.Left",
			"r.Right += rc.Left",
		},
	}
	for name, snippets := range checks {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, needle := range snippets {
			if !strings.Contains(string(raw), needle) {
				t.Errorf("%s missing %q", name, needle)
			}
		}
	}
}
