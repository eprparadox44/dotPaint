package main

import (
	"os"
	"strings"
	"testing"
)

func TestAdaptiveCursorWindowsMarkersA218(t *testing.T) {
	cursorData, err := os.ReadFile("tool_cursor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	mainData, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	cursorSrc := string(cursorData)
	mainSrc := string(mainData)
	for _, marker := range []string{
		"createAdaptiveColorCursorA217",
		"createAdaptiveMonoCursorA217",
		"cursorBackgroundRGBA217",
		"adaptiveCursorHandleA217",
		"contrast-light",
		"contrast-dark",
		"threshold=%d",
	} {
		if !strings.Contains(cursorSrc, marker) {
			t.Fatalf("missing adaptive cursor marker %q", marker)
		}
	}
	for _, marker := range []string{
		"A237-DARK-LIBRARIES-HELP-ABOUT-BACKGROUND",
		"Mouse capture can suppress WM_SETCURSOR",
		"procSetCursor.Call(app.cursorForClientPoint(mx, my, client))",
	} {
		if !strings.Contains(mainSrc, marker) {
			t.Fatalf("missing mouse-move cursor refresh marker %q", marker)
		}
	}
}

func TestEyedropperKeepsOriginalCursorInA218(t *testing.T) {
	data, err := os.ReadFile("tool_cursor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, marker := range []string{
		`a181EyedropperCursor = createColorCursorA182(s, "eyedropper")`,
		`a181EyedropperCursor = createMonoCursor(hinst, c, hx, hy, "eyedropper-fallback")`,
		`eyedropper=original`,
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("missing original eyedropper marker %q", marker)
		}
	}
	for _, forbidden := range []string{
		`a181EyedropperCursor = createAdaptiveColorCursorA217`,
		`a181EyedropperCursor = createAdaptiveMonoCursorA217`,
	} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("eyedropper must not be adaptive: %q", forbidden)
		}
	}
}
