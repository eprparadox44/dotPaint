package main

import (
	"os"
	"strings"
	"testing"
)

func TestA229ForcedDarkChromeHooks(t *testing.T) {
	mainData, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	mainText := string(mainData)
	for _, marker := range []string{
		`const appVersion = "A237-DARK-LIBRARIES-HELP-ABOUT-BACKGROUND"`,
		`func drawDarkEditorCaption(hwnd uintptr)`,
		`func handleDarkEditorNonClient(hwnd uintptr, msg uint32, wparam, lparam uintptr)`,
		`case WM_NCPAINT, WM_NCACTIVATE, WM_SETTEXT, WM_SETICON:`,
		`func drawThemedOwnerDrawPushButton(dis *DRAWITEMSTRUCT) bool`,
		`func ownerDrawPushButtonStyle(class string, style uintptr) uintptr`,
	} {
		if !strings.Contains(mainText, marker) {
			t.Fatalf("missing A229 main marker %q", marker)
		}
	}
	cases := []struct {
		path    string
		markers []string
	}{
		{"formula_editor_windows.go", []string{`handleDarkEditorNonClient`, `drawThemedOwnerDrawPushButton`, `ownerDrawPushButtonStyle`}},
		{"function_graph_editor_windows.go", []string{`handleDarkEditorNonClient`, `drawThemedOwnerDrawPushButton`, `ownerDrawPushButtonStyle`}},
		{"multidim_shape_editor_windows.go", []string{`handleDarkEditorNonClient`, `drawThemedOwnerDrawPushButton`, `ownerDrawPushButtonStyle`}},
	}
	for _, tc := range cases {
		data, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, marker := range tc.markers {
			if !strings.Contains(text, marker) {
				t.Fatalf("%s missing %q", tc.path, marker)
			}
		}
	}
}
