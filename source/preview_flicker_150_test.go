package main

import (
	"os"
	"strings"
	"testing"
)

// Source-level checks are runnable in Linux even though Win32 GUI cannot run
// there. Windows cross-compilation separately checks syntax and linkage.
func Test150ShiftAutorepeatSkippedBeforeLogging(t *testing.T) {
	b, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	anchor := strings.Index(s, "case WM_KEYDOWN:")
	if anchor < 0 {
		t.Fatal("missing WM_KEYDOWN")
	}
	body := s[anchor:]
	repeat := strings.Index(body, "wparam == VK_SHIFT && (lparam&(uintptr(1)<<30)) != 0")
	log := strings.Index(body, `logf("WM_KEYDOWN vk=%d", wparam)`)
	if repeat < 0 || log < 0 || repeat >= log {
		t.Fatal("Shift autorepeat must be ignored before development logging")
	}
}

func Test150PreviewConstraintTransitionIsAtomic(t *testing.T) {
	b, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start := strings.Index(s, "func (a *App) requestConstrainedPreviewTransition(")
	if start < 0 {
		t.Fatal("constraint transition helper is missing")
	}
	end := strings.Index(s[start:], "\nfunc ")
	if end < 0 {
		t.Fatal("helper boundary missing")
	}
	body := s[start : start+end]
	for _, needle := range []string{
		"ToolLine, ToolArrow, ToolRightTriangle", "ToolRectangle", "ToolEllipse",
		"unionRECT(oldRect, newRect)", "requestRepaintRect(",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("missing constraint transition: %s", needle)
		}
	}
	if strings.Contains(body, "requestCanvasRepaint(") {
		t.Fatal("constraint shift transitions must not invalidate entire canvas")
	}
	if strings.Count(s, "app.requestConstrainedPreviewTransition(false, true, true)") != 1 ||
		strings.Count(s, "app.requestConstrainedPreviewTransition(true, false, true)") != 1 {
		t.Fatal("both Shift edges must repaint the union")
	}
	if !strings.Contains(s, "procBitBlt.Call(screenDC") || !strings.Contains(s, "case WM_ERASEBKGND:\n\t\treturn 1") {
		t.Fatal("main canvas backbuffer/no-erase invariant changed")
	}
}

func Test150RectanglePreviewUsesDirtyRegion(t *testing.T) {
	b, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start := strings.Index(s, "func (a *App) paintRasterRectanglePreview(")
	if start < 0 {
		t.Fatal("rectangle preview missing")
	}
	end := strings.Index(s[start:], "\nfunc ")
	if end < 0 {
		t.Fatal("rectangle preview boundary missing")
	}
	body := s[start : start+end]
	for _, needle := range []string{"rectanglePreviewCanvasBounds", "cloneRasterPreviewRegion", "drawRasterRectangleAt", "blitRasterPreviewRegion"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("rectangle preview must use dirty region: %s", needle)
		}
	}
	if strings.Contains(body, "ClonePixels()") || strings.Contains(body, "preview.displayPixels(") {
		t.Fatal("rectangle preview still clones/draws entire canvas")
	}
}

func Test150FormulaAndGraphPreviewBackbuffers(t *testing.T) {
	for _, filename := range []string{"formula_editor_windows.go", "function_graph_editor_windows.go"} {
		b, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if !strings.Contains(s, "previewEditorPaintBuffer") ||
			!strings.Contains(s, "s.paintBuffer.paint(hwnd, hdc, ps.RcPaint") ||
			!strings.Contains(s, "s.paintBuffer.release()") {
			t.Fatalf("%s must buffer each preview frame and release GDI objects", filename)
		}
	}
	b, err := os.ReadFile("preview_window_buffer_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, needle := range []string{"multidimEnsureBitmapBuffer", "multidimReleaseBitmapBuffer", "draw(b.dc)", "procBitBlt.Call(screenDC"} {
		if !strings.Contains(s, needle) {
			t.Fatalf("missing back-buffer operation: %s", needle)
		}
	}
}
