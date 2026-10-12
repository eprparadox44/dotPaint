package main

import (
	"os"
	"strings"
	"testing"
)

func readDarkFirstShowSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestWindows11DarkEditorsAreHiddenUntilThemeAndClientPaintAreReady(t *testing.T) {
	mainSource := readDarkFirstShowSource(t, "main_windows.go")
	start := strings.Index(mainSource, "func showThemedEditorWindow")
	if start < 0 {
		t.Fatal("themed first-show helper not found")
	}
	end := strings.Index(mainSource[start:], "func handleDarkEditorNonClient")
	if end < 0 {
		t.Fatal("dark non-client handler not found after themed first-show helper")
	}
	helper := mainSource[start : start+end]
	last := -1
	for _, want := range []string{
		`applyDarkThemeToWindow(hwnd, dark)`,
		`procInvalidateRect.Call(hwnd, 0, 1)`,
		`procShowWindow.Call(hwnd, SW_SHOW)`,
		`procUpdateWindow.Call(hwnd)`,
		`scheduleDarkEditorChromeRefresh(hwnd)`,
	} {
		pos := strings.Index(helper, want)
		if pos < 0 {
			t.Fatalf("themed first-show helper missing %q", want)
		}
		if pos <= last {
			t.Fatalf("themed first-show order is wrong at %q", want)
		}
		last = pos
	}

	files := []string{
		"formula_editor_windows.go",
		"function_graph_editor_windows.go",
		"multidim_shape_editor_windows.go",
		"field_editor_windows.go",
		"help_window_windows.go",
		"circuit_library_windows.go",
		"animal_library_windows.go",
		"custom_image_library_windows.go",
		"about_window_windows.go",
	}
	for _, name := range files {
		body := readDarkFirstShowSource(t, name)
		if !strings.Contains(body, `showThemedEditorWindow(hwnd, a.darkMode)`) {
			t.Fatalf("%s does not use the clean themed first-show path", name)
		}
	}

	for name, forbidden := range map[string]string{
		"formula_editor_windows.go":        `WS_OVERLAPPEDWINDOW|WS_VISIBLE`,
		"function_graph_editor_windows.go": `WS_OVERLAPPEDWINDOW|WS_VISIBLE`,
		"multidim_shape_editor_windows.go": `WS_OVERLAPPEDWINDOW|WS_VISIBLE`,
		"field_editor_windows.go":          `WS_OVERLAPPEDWINDOW|WS_VISIBLE`,
		"help_window_windows.go":           `WS_OVERLAPPEDWINDOW|WS_VISIBLE`,
		"circuit_library_windows.go":       `WS_CAPTION | WS_SYSMENU | WS_MINIMIZEBOX | WS_VISIBLE`,
		"animal_library_windows.go":        `WS_CAPTION | WS_SYSMENU | WS_MINIMIZEBOX | WS_VISIBLE`,
		"custom_image_library_windows.go":  `WS_CAPTION | WS_SYSMENU | WS_MINIMIZEBOX | WS_VISIBLE`,
		"about_window_windows.go":          `WS_CAPTION|WS_SYSMENU|WS_VISIBLE`,
	} {
		if strings.Contains(readDarkFirstShowSource(t, name), forbidden) {
			t.Fatalf("%s still exposes the top-level editor during frame setup", name)
		}
	}
}

func TestWindows11DarkChromeTimersNeverReapplyTheVisibleFrame(t *testing.T) {
	body := readDarkFirstShowSource(t, "main_windows.go")
	start := strings.Index(body, "func handleDarkEditorNonClient")
	if start < 0 {
		t.Fatal("dark non-client handler not found")
	}
	end := strings.Index(body[start:], "if !darkModeEnabled()")
	if end < 0 {
		t.Fatal("dark non-client timer block end not found")
	}
	timerBlock := body[start : start+end]
	if strings.Contains(timerBlock, `applyDarkThemeToWindow(hwnd, true)`) {
		t.Fatal("caption refresh timer must not trigger SWP_FRAMECHANGED on a visible editor")
	}
	for _, want := range []string{
		`procKillTimer.Call(hwnd, wparam)`,
		`drawDarkEditorCaption(hwnd)`,
	} {
		if !strings.Contains(timerBlock, want) {
			t.Fatalf("dark non-client timer block missing %q", want)
		}
	}
}
