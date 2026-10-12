package main

import (
	"os"
	"strings"
	"testing"
)

func readA231Source(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func TestA232DelayedDarkCaptionRefresh(t *testing.T) {
	src := readA231Source(t, "main_windows.go")
	for _, marker := range []string{
		`darkEditorChromeTimerID     = 0xA231`,
		`darkEditorChromeLateTimerID = 0xA232`,
		`func scheduleDarkEditorChromeRefresh(hwnd uintptr)`,
		`procSetTimer.Call(hwnd, darkEditorChromeTimerID, 30, 0)`,
		`procSetTimer.Call(hwnd, darkEditorChromeLateTimerID, 220, 0)`,
		`if msg == WM_TIMER && (wparam == darkEditorChromeTimerID || wparam == darkEditorChromeLateTimerID)`,
		`applyDarkThemeToWindow(hwnd, true)`,
		`drawDarkEditorCaption(hwnd)`,
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("missing delayed-caption marker %q", marker)
		}
	}
	for _, file := range []string{"formula_editor_windows.go", "function_graph_editor_windows.go", "multidim_shape_editor_windows.go"} {
		if !strings.Contains(readA231Source(t, file), `scheduleDarkEditorChromeRefresh(hwnd)`) {
			t.Fatalf("%s does not schedule initial dark-caption repaint", file)
		}
	}
}

func TestA232FormulaGraphInvertControls(t *testing.T) {
	for file, markers := range map[string][]string{
		"formula_editor_windows.go": {
			`formulaIDInvert`, `invertBtn`, `invert`,
			`"검은 배경"`, `formulaCompositePreviewOn`, `WM_CTLCOLORSCROLLBAR`,
		},
		"function_graph_editor_windows.go": {
			`graphIDInvert`, `invertBtn`, `invert`,
			`"검은 배경"`, `formulaCompositePreviewOn`, `WM_CTLCOLORSCROLLBAR`,
		},
	} {
		src := readA231Source(t, file)
		for _, marker := range markers {
			if !strings.Contains(src, marker) {
				t.Fatalf("%s missing invert/scrollbar marker %q", file, marker)
			}
		}
	}
}

func TestA232DarkToolIconsUseHollowBrush(t *testing.T) {
	src := readA231Source(t, "main_windows.go")
	start := strings.Index(src, `func (a *App) drawToolIcon`)
	if start < 0 {
		t.Fatal("drawToolIcon not found")
	}
	end := strings.Index(src[start:], `func (`)
	if end <= 0 {
		end = len(src) - start
	}
	body := src[start : start+end]
	if !strings.Contains(body, `procGetStockObject.Call(NULL_BRUSH)`) {
		t.Fatal("tool icon renderer does not select NULL_BRUSH")
	}
}

func TestA232StrictTanOnePixelRestoresOrigin(t *testing.T) {
	src := readA231Source(t, "main_windows.go")
	start := strings.Index(src, `func (c *Canvas) DrawArrowShaftStrictTanPixelPath`)
	if start < 0 {
		t.Fatal("strict tan renderer not found")
	}
	end := strings.Index(src[start:], `func (`)
	if end <= 0 {
		end = len(src) - start
	}
	body := src[start : start+end]
	for _, marker := range []string{
		`if size == 1 && len(points) > 0`,
		`c.SetPixel(points[0].X, points[0].Y, clr)`,
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("strict tan origin fix missing %q", marker)
		}
	}
}
