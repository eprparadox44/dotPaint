//go:build windows

package main

import (
	"bytes"
	"testing"
)

func TestBeta52DeleteUnrelatedSelectionPreservesEditableObjectsAndUndo(t *testing.T) {
	a := newApp()
	a.canvas = NewCanvas(24, 18, rgb(255, 255, 255))
	a.tool = ToolRectSelect
	a.objects = []MovableObject{
		cutTestObject(1, 2, 4, 4, rgb(255, 0, 0)),
		cutTestObject(16, 10, 3, 3, rgb(0, 0, 255)),
	}
	a.resetHistory("처음")
	original := copyObjects(a.objects)
	a.selection = SelectionState{Active: true, X: 9, Y: 4, W: 2, H: 3}
	if !a.deleteSelection(false) {
		t.Fatal("delete refused")
	}
	if !objectsEqual(a.objects, original) || a.objects[0].Rasterized || a.objects[1].Rasterized {
		t.Fatal("Del in unrelated pixels changed editable objects")
	}
	if !a.historyUndo() || !objectsEqual(a.objects, original) {
		t.Fatal("undo unrelated background deletion")
	}
}

func TestBeta52DeleteRealShapePartOnlyOneObjectAndUndoRedo(t *testing.T) {
	a := newApp()
	a.canvas = NewCanvas(24, 18, rgb(255, 255, 255))
	a.tool = ToolRectSelect
	a.objects = []MovableObject{
		cutTestObject(1, 2, 5, 4, rgb(255, 0, 0)),
		cutTestObject(16, 10, 3, 3, rgb(0, 0, 255)),
	}
	a.resetHistory("처음")
	before := copyObjects(a.objects)
	unrelatedPix := append([]byte(nil), a.objects[1].Pix...)
	a.selection = SelectionState{Active: true, X: 3, Y: 3, W: 2, H: 2}
	if !a.deleteSelectionTransparent() {
		t.Fatal("transparent delete refused")
	}
	if len(a.objects) != 2 || !a.objects[0].Rasterized || a.objects[1].Rasterized || !bytes.Equal(a.objects[1].Pix, unrelatedPix) {
		t.Fatal("wrong object rasterized or original editable object altered")
	}
	if a.flattenedObjectsCanvas().GetPixel(3, 3) != transparentColor {
		t.Fatal("masked portion is not transparent")
	}
	if !a.historyUndo() || !objectsEqual(a.objects, before) {
		t.Fatal("undo failed to restore original editable geometry")
	}
	if !a.historyRedo() || !a.objects[0].Rasterized || a.objects[1].Rasterized {
		t.Fatal("redo failed to restore selective rasterization")
	}
}
