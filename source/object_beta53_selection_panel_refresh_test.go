package main

import (
	"os"
	"strings"
	"testing"
)

// Source-level regression: the Windows toolbox is a floating panel, not part
// of the dirty canvas rectangle. Selection changes must explicitly redraw it.
func TestBeta53ObjectSelectionRefreshesToolbox(t *testing.T) {
	props, err := os.ReadFile("object_properties_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"func (a *App) requestObjectSelectionUIRepaint()", "a.toolboxPanelRect(client)", "invalidateArea(a.hwnd, &panel)", "a.requestCanvasRepaint(false)", "procUpdateWindow.Call(a.hwnd)"} {
		if !strings.Contains(string(props), part) {
			t.Fatalf("missing immediate toolbox refresh step %q", part)
		}
	}
	cases := []struct{ file, callsite string }{
		{"movable_objects_windows.go", "func (a *App) beginObjectInteraction("},
		{"object_group_windows.go", "func (a *App) groupSelectedObjects("},
		{"object_group_windows.go", "func (a *App) ungroupSelectedObjects("},
		{"object_menu_windows.go", "func (a *App) showObjectContextMenu("},
		{"main_windows.go", "func (a *App) onMouseDoubleClick("},
	}
	for _, tc := range cases {
		data, err := os.ReadFile(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		suffix := string(data)
		idx := strings.Index(suffix, tc.callsite)
		if idx < 0 {
			t.Errorf("missing %q", tc.callsite)
			continue
		}
		suffix = suffix[idx+len(tc.callsite):]
		next := strings.Index(suffix, "\nfunc ")
		if next >= 0 {
			suffix = suffix[:next]
		}
		if !strings.Contains(suffix, "a.requestObjectSelectionUIRepaint()") {
			t.Errorf("%s %s missing object-selection UI repaint", tc.file, tc.callsite)
		}
	}
}
