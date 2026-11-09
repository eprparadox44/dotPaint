//go:build windows

package main

import "testing"

func TestBeta5WindowsMultiSelectionGroupUndoAndMemberColor(t *testing.T) {
	a := beta2WindowsApp(t)
	beta2AddShape(t, a, ToolRectangle)
	beta2AddShape(t, a, ToolEllipse)
	beta2AddShape(t, a, ToolRightTriangle)
	a.tool = ToolObjectSelect
	a.selectObjectWithGroup(0)
	a.toggleObjectSelectionWithShift(1)
	if len(a.selectedObjectIndices()) != 2 {
		t.Fatal("Shift multi-selection did not include two objects")
	}
	h := len(a.historyEntries)
	if !a.groupSelectedObjects() {
		t.Fatal("group selection failed")
	}
	if len(a.historyEntries) != h+1 || a.objects[0].GroupID == 0 || a.objects[0].GroupID != a.objects[1].GroupID {
		t.Fatal("group ID/history failed")
	}
	if !a.historyUndo() || a.objects[0].GroupID != 0 || a.objects[1].GroupID != 0 {
		t.Fatal("group undo failed")
	}
	if !a.historyRedo() || a.objects[0].GroupID == 0 || a.objects[1].GroupID != a.objects[0].GroupID {
		t.Fatal("group redo failed")
	}
	a.selectObjectWithGroup(0)
	a.toggleObjectSelectionWithShift(2)
	if !a.groupSelectedObjects() || a.objects[2].GroupID != a.objects[0].GroupID {
		t.Fatal("nested merge failed")
	}
	a.objectIsolatedGroup = a.objects[1].GroupID
	a.objectSelected = 1
	a.objectMultiSelected = nil
	a.syncSelectedObjectOptions()
	id := a.objects[1].GroupID
	if !a.changeSelectedObjectColor(rgb(40, 80, 180), PaintInvert, false) {
		t.Fatal("member color update failed")
	}
	if a.objects[1].GroupID != id || a.objects[1].Operation != PaintInvert {
		t.Fatal("member color change lost group")
	}
	if !a.ungroupSelectedObjects() || a.objects[1].GroupID != 0 {
		t.Fatal("ungroup failed")
	}
}

func TestBeta5WindowsSelectedShapeOptionsAndOpacity(t *testing.T) {
	a := beta2WindowsApp(t)
	beta2AddShape(t, a, ToolRectangle)
	a.tool = ToolObjectSelect
	a.selectObjectWithGroup(0)
	if a.selectedObjectOptionTool() != ToolRectangle {
		t.Fatal("rectangle tool option not shown")
	}
	size := a.objects[0].Stroke + 2
	a.rectangleSize = size
	if !a.applySelectedObjectToolOptions(true) || a.objects[0].Stroke != size {
		t.Fatal("stroke update failed")
	}
	if !a.historyUndo() || a.objects[0].Stroke == size {
		t.Fatal("stroke undo failed")
	}
}
