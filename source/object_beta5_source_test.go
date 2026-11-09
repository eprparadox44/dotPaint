package main

import (
	"os"
	"strings"
	"testing"
)

// These source contracts are runnable on Linux; native GUI interactions remain
// Windows-only and must also be verified on an actual Windows desktop.
func TestBeta5UIAndIndexedInvertIntegrationSource(t *testing.T) {
	markers := map[string][]string{
		"main_windows.go":              {"a.renderIndexedInvertText(full, text, false)", "compositeObjectRows(region.Pix", "a.changeSelectedObjectColor(c, paintMode(PaintRGB, c), right)", "a.toggleObjectKeepAt", "a.selectedObjectOptionTool()", "obj.GroupID = a.objects[i].GroupID"},
		"beta4_windows.go":             {"a.indexedInvertTable(palette)", "func (a *App) renderIndexedInvertText", "a.changeSelectedObjectColor(col, PaintInvert, right)", "applyIndexedInvertPixel"},
		"object_group_windows.go":      {"selectObjectWithGroup", "toggleObjectSelectionWithShift", "groupSelectedObjects", "ungroupSelectedObjects", "beginObjectGroupDrag", "objectGroupBounds"},
		"object_menu_windows.go":       {"objectMenuGroup", "objectMenuUngroup", "a.appendObjectCommands", "applyDarkMenuBackground"},
		"object_properties_windows.go": {"applySelectedObjectToolOptions", "endSelectedObjectOptionDrag", "setSelectedTextObjectOpaque", "changeSelectedObjectColor"},
		"movable_objects_windows.go":   {"next.GroupID = o.GroupID", "func (a *App) renderResizedObject"},
		"ui_language_windows.go":       {"선택 묶기(&G)", "Ungroup(&U)", "Selected object shape properties updated.", "Selected text box background mode updated."},
	}
	for path, tokens := range markers {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, token := range tokens {
			if !strings.Contains(s, token) {
				t.Errorf("%s missing %q", path, token)
			}
		}
	}
}
