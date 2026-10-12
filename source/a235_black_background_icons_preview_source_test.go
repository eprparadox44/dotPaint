package main

import (
	"os"
	"strings"
	"testing"
)

func a235ReadSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestA235CurveAndTextToolIconsAreEmbedded(t *testing.T) {
	if len(curveToolIconPixels) != userToolIconWidth*userToolIconHeight {
		t.Fatalf("curve icon size=%d", len(curveToolIconPixels))
	}
	if len(textToolIconPixels) != userToolIconWidth*userToolIconHeight {
		t.Fatalf("text icon size=%d", len(textToolIconPixels))
	}
	curveCount, textCount := 0, 0
	for _, c := range curveToolIconPixels {
		if c != userToolIconTransparent {
			curveCount++
		}
	}
	for _, c := range textToolIconPixels {
		if c != userToolIconTransparent {
			textCount++
		}
	}
	if curveCount != 150 || textCount != 177 {
		t.Fatalf("retained pixels curve=%d text=%d", curveCount, textCount)
	}
	src := a235ReadSource(t, "main_windows.go")
	for _, marker := range []string{
		"case ToolCurve:", "return &curveToolIconPixels, true",
		"case ToolText:", "return &textToolIconPixels, true",
		"case ToolPencil, ToolEraser, ToolFill, ToolText, ToolCurve:",
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("main_windows.go missing %q", marker)
		}
	}
}

func TestA235BlackBackgroundLabels(t *testing.T) {
	for _, file := range []string{"formula_editor_windows.go", "function_graph_editor_windows.go", "multidim_shape_editor_windows.go"} {
		src := a235ReadSource(t, file)
		if !strings.Contains(src, "검은 배경") {
			t.Fatalf("%s missing black-background label", file)
		}
	}
}

func TestA235CurveAndEllipsePreviewUseDirtyRegions(t *testing.T) {
	src := a235ReadSource(t, "main_windows.go")
	for _, marker := range []string{
		"func (a *App) ellipsePreviewCanvasBounds",
		"func (a *App) curvePreviewCanvasBounds",
		"a.cloneRasterPreviewRegion(left, top, right, bottom)",
		"a.blitRasterPreviewRegion(hdc, ox, oy, left, top, preview)",
		"a.logVectorPreview(\"ellipse\"",
		"a.logVectorPreview(\"curve\"",
		"a.requestEllipsePreviewTransition(oldCurX, oldCurY, a.curX, a.curY",
		"oldCurveRect := a.curvePreviewScreenRect()",
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("main_windows.go missing dirty-preview marker %q", marker)
		}
	}
}

func TestA235BrushShapeIconsInvertInDarkMode(t *testing.T) {
	src := a235ReadSource(t, "main_windows.go")
	for _, marker := range []string{
		"shapeColor := gdiPreviewColor(a.fg)",
		"if darkModeEnabled()",
		"shapeColor = invertToolIconColor(shapeColor)",
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("main_windows.go missing brush-shape inversion %q", marker)
		}
	}
}
