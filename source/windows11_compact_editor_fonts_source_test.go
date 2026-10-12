package main

import (
	"os"
	"strings"
	"testing"
)

func readCompactEditorFontSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestWindows11CompactEditorUIFontPolicy(t *testing.T) {
	mainSource := readCompactEditorFontSource(t, "main_windows.go")
	for _, want := range []string{
		`const compactEditorUIFontPixelHeight = 12`,
		`return createGDIFont("맑은 고딕", compactEditorUIFontPixelHeight, false, false, false)`,
		`func setCompactEditorControlFont(hwnd, compactFont uintptr)`,
	} {
		if !strings.Contains(mainSource, want) {
			t.Fatalf("compact editor font policy missing %q", want)
		}
	}

	files := []string{
		"formula_editor_windows.go",
		"function_graph_editor_windows.go",
		"multidim_shape_editor_windows.go",
		"field_editor_windows.go",
	}
	for _, name := range files {
		body := readCompactEditorFontSource(t, name)
		for _, want := range []string{
			`uiFont`,
			`createCompactEditorUIFont()`,
			`setCompactEditorControlFont(h, s.uiFont)`,
			`procDeleteObject.Call(s.uiFont)`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s does not enforce compact font lifecycle: missing %q", name, want)
			}
		}
	}
}

func TestWindows11DenseDockTitlesAndWRotationLayout(t *testing.T) {
	mainSource := readCompactEditorFontSource(t, "main_windows.go")
	for _, want := range []string{
		`const dockPanelTitleFontPixelHeight = 12`,
		`dockPanelTitleFont uintptr`,
		`a.ensureDockPanelTitleFont()`,
		`app.destroyDockPanelTitleFont()`,
		`paintPanelFrame(hdc, panel, "도구")`,
		`paintPanelFrame(hdc, panel, "색상표")`,
		`paintPanelFrame(hdc, panel, "클립보드")`,
		`paintPanelFrame(hdc, panel, "히스토리")`,
	} {
		if !strings.Contains(mainSource, want) {
			t.Fatalf("compact dock-title policy missing %q", want)
		}
	}

	multidimSource := readCompactEditorFontSource(t, "multidim_shape_editor_windows.go")
	for _, want := range []string{
		`labelW := int32(44)`,
		`columnW := (width - margin*2 - columnGap*3) / 4`,
		`sliderW := columnW - labelW - gap - gap - valueW`,
		`layoutSlider(s.wLabel, s.wSlider, s.wValueLabel, col4X, row1Y)`,
	} {
		if !strings.Contains(multidimSource, want) {
			t.Fatalf("multidimensional W-rotation layout missing %q", want)
		}
	}
}

func TestFormulaContentSizingRemainsIndependent(t *testing.T) {
	body := readCompactEditorFontSource(t, "formula_editor_windows.go")
	for _, want := range []string{
		`var formulaSizeChoices = []int{10, 12, 16, 20, 24, 28, 32, 36, 48, 64, 72}`,
		`state := &formulaEditorState{owner: a, fontSize: 20`,
		`boldFont := createGDIFont("Cambria", 15, true, false, false)`,
		`italicFont := createGDIFont("Cambria", 15, false, true, false)`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("formula content/style sizing regression: missing %q", want)
		}
	}
}
