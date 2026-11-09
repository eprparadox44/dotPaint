//go:build windows

package main

import (
	"bytes"
	"testing"
)

func beta3TwoObjectCanvas(t *testing.T) *App {
	t.Helper()
	a := newApp()
	a.canvas = NewCanvas(80, 80, rgb(255, 255, 255))
	a.tool = ToolObjectSelect
	a.resetHistory("start")
	first, ok := a.makeShapeObject(ToolRectangle, 7, 7, 34, 38, false, rgb(255, 0, 0))
	if !ok {
		t.Fatal("first object")
	}
	second, ok := a.makeShapeObject(ToolEllipse, 15, 17, 46, 48, false, rgb(0, 0, 200))
	if !ok {
		t.Fatal("second object")
	}
	a.objects = []MovableObject{first, second}
	a.objectsRevision++
	a.recordHistory("shapes")
	return a
}

func TestBeta3KeepObjectsDefaultAndClick(t *testing.T) {
	a := newApp()
	if !a.keepObjects {
		t.Fatal("Keep Object must start checked")
	}
	for _, tool := range []Tool{ToolRectangle, ToolEllipse, ToolRightTriangle, ToolText, ToolLine, ToolArrow, ToolCurve} {
		if !supportsKeepObjects(tool) {
			t.Fatalf("missing Keep Object for %v", tool)
		}
	}
	if supportsKeepObjects(ToolPencil) || supportsKeepObjects(ToolFill) {
		t.Fatal("non-object tool received checkbox")
	}
	a.tool = ToolRectangle
	rect := keepObjectsOptionRect()
	if !a.toggleObjectKeepAt(int(rect.Left)+10, int(rect.Top)+10, RECT{0, 0, 98, 740}) || a.keepObjects {
		t.Fatal("checkbox did not turn off")
	}
	if !a.toggleObjectKeepAt(int(rect.Left)+10, int(rect.Top)+10, RECT{0, 0, 98, 740}) || !a.keepObjects {
		t.Fatal("checkbox did not turn on")
	}
}

func TestBeta3RasterizeBottomObjectRetainsTopAndUndo(t *testing.T) {
	a := beta3TwoObjectCanvas(t)
	before := a.flattenedObjectsCanvas().ClonePixels()
	a.objectSelected = 0
	if !a.rasterizeSelectedObject() {
		t.Fatal("rasterize refused")
	}
	if len(a.objects) != 1 || a.objects[0].Kind != ToolEllipse || a.objectSelected != -1 {
		t.Fatal("incorrect object removal")
	}
	if !bytes.Equal(before, a.flattenedObjectsCanvas().Pix) {
		t.Fatal("composite changed after rasterizing bottom")
	}
	if !a.historyUndo() || len(a.objects) != 2 {
		t.Fatal("undo did not restore editable object")
	}
	if !a.historyRedo() || len(a.objects) != 1 {
		t.Fatal("redo did not rasterize")
	}
}

func TestBeta3RasterizeMiddlePreservesZOrderAndHitTesting(t *testing.T) {
	a := beta3TwoObjectCanvas(t)
	before := a.flattenedObjectsCanvas().ClonePixels()
	a.objectSelected = 1
	if !a.applyObjectMenuCommand(objectMenuRasterize) {
		t.Fatal("menu rasterize failed")
	}
	if len(a.objects) != 2 || !a.objects[1].Rasterized || a.objects[1].Text != "" {
		t.Fatal("raster layer not locked")
	}
	if !bytes.Equal(before, a.flattenedObjectsCanvas().Pix) {
		t.Fatal("z-order composition changed")
	}
	// Locked layer is still visible but cannot intercept clicking or handles.
	if idx := hitObjectBody(a.objects, 16, 18); idx == 1 {
		t.Fatal("rasterized layer selected")
	}
	if idx, _ := hitObjectAtClient(a.objects, 1, 0, 0, 15, 17); idx == 1 {
		t.Fatal("rasterized layer exposes handles")
	}
	if !a.historyUndo() || a.objects[1].Rasterized {
		t.Fatal("undo must restore editability")
	}
	if !a.historyRedo() || !a.objects[1].Rasterized {
		t.Fatal("redo must lock layer")
	}
}
