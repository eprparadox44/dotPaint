package main

import (
	"os"
	"strings"
	"testing"
)

func readDarkDialogSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestWindows11DarkCaptionStopsAtRealClientBoundary(t *testing.T) {
	body := readDarkDialogSource(t, "main_windows.go")
	for _, want := range []string{
		`const appVersion = "1.2.38"`,
		`procClientToScreen`,
		`modUser32.NewProc("ClientToScreen")`,
		`measured := int(clientOrigin.Y - wr.Top)`,
		`chromeH = measured`,
		`buttonBottom := min(max(chromeH-1, buttonTop+1), buttonTop+buttonH)`,
		`const dockPanelTitleFontPixelHeight = 12`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dark caption boundary policy missing %q", want)
		}
	}
	if strings.Contains(body, `chromeH := max(frameY+captionH, buttonH+frameY)`) {
		t.Fatal("old title-bar overpaint estimate remains")
	}
}

func TestFormulaPopupMenuIsMeasuredAndPaintedByFormulaOwner(t *testing.T) {
	body := readDarkDialogSource(t, "formula_editor_windows.go")
	for _, want := range []string{
		`case WM_MENUCHAR:`,
		`handleThemedMenuChar(wparam, lparam)`,
		`case WM_MEASUREITEM:`,
		`app.measureThemedMenuItem((*MEASUREITEMSTRUCT)(unsafe.Pointer(lparam)))`,
		`app.drawThemedMenuItem(dis)`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("formula popup owner-draw route missing %q", want)
		}
	}
}

func TestTransformDialogsUseCompactFlatDarkUI(t *testing.T) {
	flip := readDarkDialogSource(t, "flip_rotate_dialog_windows.go")
	for _, want := range []string{
		`uiFont: createCompactEditorUIFont()`,
		`setCompactEditorControlFont(s.angleEdit, s.uiFont)`,
		`procCreateSolidBrush.Call(uintptr(uiColor(rgb(255, 255, 255))))`,
		`procDeleteObject.Call(s.uiFont)`,
	} {
		if !strings.Contains(flip, want) {
			t.Fatalf("flip/rotate compact dark UI missing %q", want)
		}
	}

	stretch := readDarkDialogSource(t, "stretch_skew_dialog_windows.go")
	for _, want := range []string{
		`uiFont: createCompactEditorUIFont()`,
		`buttonStyle := uintptr(WS_CHILD | WS_VISIBLE | WS_TABSTOP | BS_OWNERDRAW)`,
		`s.drawOwnerDrawButton((*DRAWITEMSTRUCT)(unsafe.Pointer(lparam)))`,
		`procDeleteObject.Call(s.uiFont)`,
	} {
		if !strings.Contains(stretch, want) {
			t.Fatalf("stretch/skew compact flat UI missing %q", want)
		}
	}

	canvas := readDarkDialogSource(t, "canvas_size_dialog_windows.go")
	for _, want := range []string{
		`style = ownerDrawPushButtonStyle(class, style)`,
		`setCompactEditorControlFont(hwnd, compactImageDialogUIFont(parent))`,
		`uiFont: createCompactEditorUIFont()`,
		`clientBounds := RECT{0, 0, 482, 375}`,
		`drawThemedOwnerDrawPushButton((*DRAWITEMSTRUCT)(unsafe.Pointer(lparam)))`,
	} {
		if !strings.Contains(canvas, want) {
			t.Fatalf("canvas-size compact flat UI missing %q", want)
		}
	}
	if strings.Contains(canvas, "기존 그림은 왼쪽 위에 고정됩니다") {
		t.Fatal("removed canvas-size explanation is still present")
	}
}
