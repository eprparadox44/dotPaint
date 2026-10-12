package main

import (
	"os"
	"strings"
	"testing"
)

func readA233Source(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func TestA233UsesOnlyNativeSubmenuArrow(t *testing.T) {
	src := readA233Source(t, "main_windows.go")
	if strings.Contains(src, `drawText(dis.HDC, "▶"`) {
		t.Fatal("custom submenu arrow is still drawn")
	}
	for _, marker := range []string{
		`entry.Popup`,
		`Reserve the native Windows submenu-arrow column`,
		`duplicate glyph ourselves`,
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("missing native submenu-arrow marker %q", marker)
		}
	}
}

func TestA233EditorsApplyCaptionThemeInsideWMCreate(t *testing.T) {
	for _, file := range []string{
		"formula_editor_windows.go",
		"function_graph_editor_windows.go",
		"multidim_shape_editor_windows.go",
	} {
		src := readA233Source(t, file)
		create := strings.Index(src, "case WM_CREATE:")
		if create < 0 {
			t.Fatalf("%s missing WM_CREATE", file)
		}
		tail := src[create:]
		next := strings.Index(tail, "case WM_")
		if next <= 0 {
			next = len(tail)
		}
		body := tail[:next]
		for _, marker := range []string{
			`applyDarkThemeToWindow(hwnd, darkModeEnabled())`,
			`drawDarkEditorCaption(hwnd)`,
			`scheduleDarkEditorChromeRefresh(hwnd)`,
		} {
			if !strings.Contains(body, marker) {
				t.Fatalf("%s WM_CREATE missing %q", file, marker)
			}
		}
	}
}

func TestA233RightTrianglePreviewUsesDirtyRegion(t *testing.T) {
	src := readA233Source(t, "main_windows.go")
	for _, marker := range []string{
		`func (a *App) rightTrianglePreviewCanvasBounds`,
		`func (a *App) drawRasterRightTriangleAt`,
		`a.cloneRasterPreviewRegion(left, top, right, bottom)`,
		`a.blitRasterPreviewRegion(hdc, ox, oy, left, top, preview)`,
		`a.logVectorPreview("right-triangle"`,
		`a.tool == ToolLine || a.tool == ToolRightTriangle || a.tool == ToolArrow`,
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("right-triangle dirty preview missing %q", marker)
		}
	}
	start := strings.Index(src, `func (a *App) paintRasterRightTrianglePreview`)
	if start < 0 {
		t.Fatal("right-triangle preview function missing")
	}
	tail := src[start:]
	end := strings.Index(tail[1:], `func (`)
	if end >= 0 {
		tail = tail[:end+1]
	}
	if strings.Contains(tail, `a.canvas.ClonePixels()`) {
		t.Fatal("right-triangle preview still clones the full canvas")
	}
}

func TestA233HelpReadabilityFormatting(t *testing.T) {
	src := readA233Source(t, "help_window_windows.go")
	for _, marker := range []string{
		`helpReadableFont(hwnd, 11)`,
		`bodyClass = "RICHEDIT50W"`,
		`YHeight:`,
		`linePixels := max(1, (11*dpi+36)/72+2)`,
		`BLineSpacingRule: 4`,
		`helpApplyReadableBodyFormat(s)`,
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("help readability missing %q", marker)
		}
	}
}

func TestA233DarkToolIconInversion(t *testing.T) {
	src := readA233Source(t, "main_windows.go")
	for _, marker := range []string{
		`func invertToolIconColor(c uint32) uint32`,
		`case ToolPencil, ToolEraser, ToolFill, ToolText, ToolCurve:`,
		`drawUserToolIconPixels(hdc, cx, cy, pixels, darkModeEnabled() && darkInvertToolIcon(t))`,
		`textColor = invertToolIconColor(textColor)`,
		`drawText(hdc, "A"`,
		`The eyedropper intentionally keeps its user-provided original artwork`,
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("dark tool icon inversion missing %q", marker)
		}
	}
}
