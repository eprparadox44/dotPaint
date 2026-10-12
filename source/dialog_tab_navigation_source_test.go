package main

import (
	"os"
	"strings"
	"testing"
)

func TestStretchSkewAndCanvasSizeRestoreTabNavigation(t *testing.T) {
	for _, name := range []string{"stretch_skew_dialog_windows.go", "canvas_size_dialog_windows.go"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		for _, marker := range []string{
			"case VK_TAB:",
			"moveDialogTabFocus(hwnd, keyDown(VK_SHIFT))",
		} {
			if !strings.Contains(src, marker) {
				t.Fatalf("%s missing Tab-navigation marker %q", name, marker)
			}
		}
	}

	data, err := os.ReadFile("dialog_tab_navigation_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, marker := range []string{
		"procGetNextDlgTabItem.Call(dialog, current, previous)",
		"procSetFocus.Call(next)",
		"sendMessage(next, EM_SETSEL, 0, ^uintptr(0))",
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("shared Tab-navigation helper missing %q", marker)
		}
	}
}
