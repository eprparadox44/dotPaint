//go:build windows

package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestBeta565RasterModeFlattensObjectsButKeepsUndoHistory(t *testing.T) {
	a := beta3TwoObjectCanvas(t)
	before := append([]byte(nil), a.flattenedObjectsCanvas().Pix...)
	if len(a.historyEntries) < 2 {
		t.Fatal("history setup failed")
	}
	previousHistoryCount := len(a.historyEntries)
	previousHistoryIndex := a.historyIndex
	previousObjects := copyObjects(a.objects)
	previousBase := a.canvas.ClonePixels()
	a.enterRasterMode()
	if !a.rasterMode || a.keepObjects || len(a.objects) != 0 || a.objectSelected != -1 {
		t.Fatal("raster mode did not remove all editable objects")
	}
	if !bytes.Equal(before, a.canvas.Pix) {
		t.Fatal("flattened scene changed color, alpha or object order")
	}
	if len(a.historyEntries) != previousHistoryCount+1 || a.historyIndex != previousHistoryIndex+1 {
		t.Fatal("enabling raster mode must append exactly one history entry, not reset history")
	}
	if !a.historyUndo() || a.rasterMode || !a.keepObjects || !objectsEqual(a.objects, previousObjects) || !bytes.Equal(a.canvas.Pix, previousBase) {
		t.Fatal("Ctrl+Z must restore the exact previous editable objects, pixels, and mode")
	}
	if !a.historyRedo() || !a.rasterMode || len(a.objects) != 0 || !bytes.Equal(a.canvas.Pix, before) {
		t.Fatal("Ctrl+Y must restore the flattened raster canvas and mode")
	}
	if !a.mustChooseDOPSaveTarget() {
		// An unsaved document still prompts even when raster-only.
		t.Fatal("unsaved raster document must still choose save target")
	}
	a.leaveRasterMode()
	if a.rasterMode || !a.keepObjects || len(a.objects) != 0 {
		t.Fatal("turning mode off must restore preference but not erased objects")
	}
}

func TestBeta565RasterModePreservesDynamicInverseAppearance(t *testing.T) {
	a := newApp()
	a.canvas = NewCanvas(4, 4, rgb(12, 23, 34))
	a.objectSelected = 0
	a.objects = []MovableObject{
		{Kind: ToolRectangle, X: 1, Y: 1, W: 2, H: 2, SpriteW: 2, SpriteH: 2,
			Operation: PaintInvert, Pix: bytes.Repeat([]byte{255, 255, 255, 255}, 4)},
	}
	a.resetHistory("editable inverse")
	before := append([]byte(nil), a.flattenedObjectsCanvas().Pix...)
	a.enterRasterMode()
	if !bytes.Equal(before, a.canvas.Pix) || len(a.objects) != 0 {
		t.Fatal("inverted object lost its visible color or alpha")
	}
}

func TestBeta565RasterModeDoesNotAcceptEditableObjectPaste(t *testing.T) {
	a := newApp()
	a.canvas = NewCanvas(10, 10, rgb(255, 255, 255))
	a.keepObjects = false // previous user preference
	a.resetHistory("raster")
	a.enterRasterMode()
	if a.pasteEditableObjects(objectClipboardPacket{}) {
		t.Fatal("raster mode must reject an editable packet and use PNG/DIB fallback")
	}
	a.leaveRasterMode()
	if a.keepObjects {
		t.Fatal("prior unchecked Keep Object preference must be restored")
	}
}

func TestBeta565RasterModeNewObjectIsImmediatelyFlattened(t *testing.T) {
	a := newApp()
	a.canvas = NewCanvas(5, 5, rgb(255, 255, 255))
	a.enterRasterMode()
	a.addMovableObject(MovableObject{Kind: ToolRectangle, X: 2, Y: 2, W: 1, H: 1, SpriteW: 1, SpriteH: 1, Pix: []byte{0, 0, 255, 255}})
	if len(a.objects) != 0 || a.canvas.Pix[(2*5+2)*4+2] != 255 {
		t.Fatal("object insertion bypassed global rasterization mode")
	}
}

func TestBeta565OpeningDOPProtectsEditableObjects(t *testing.T) {
	doc := dopTestProject()
	path := filepath.Join(t.TempDir(), "existing-editable.dop")
	if err := WriteDOPProject(path, doc, nil); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	a.enterRasterMode()
	if !a.rasterMode {
		t.Fatal("setup")
	}
	if !a.loadDOPDocument(path) {
		t.Fatal("DOP open failed")
	}
	if a.rasterMode || !objectsEqual(a.objects, doc.Objects) || !a.keepObjects {
		t.Fatal("DOP open must leave raster mode before restoring editable objects")
	}
}

func TestBeta565RasterModeOffIsUndoableAndRedone(t *testing.T) {
	a := beta3TwoObjectCanvas(t)
	baseObjects := copyObjects(a.objects)
	baseIndex := a.historyIndex
	a.enterRasterMode()
	rasterPixels := a.canvas.ClonePixels()
	a.leaveRasterMode()
	if a.rasterMode || len(a.objects) != 0 || a.historyIndex != baseIndex+2 {
		t.Fatal("switching mode off must be its own history step")
	}
	if !a.historyUndo() || !a.rasterMode || len(a.objects) != 0 || !bytes.Equal(a.canvas.Pix, rasterPixels) {
		t.Fatal("first Undo must restore raster mode on")
	}
	if !a.historyUndo() || a.rasterMode || !objectsEqual(a.objects, baseObjects) {
		t.Fatal("second Undo must restore editable objects")
	}
	if !a.historyRedo() || !a.rasterMode || len(a.objects) != 0 {
		t.Fatal("first Redo must re-flatten objects")
	}
	if !a.historyRedo() || a.rasterMode || len(a.objects) != 0 {
		t.Fatal("second Redo must leave raster mode and preserve flattened pixels")
	}
}

func TestBeta565RasterModeUndoRestoresInverseOperationAndGroup(t *testing.T) {
	a := newApp()
	a.canvas = NewCanvas(4, 4, rgb(12, 23, 34))
	a.objects = []MovableObject{{Kind: ToolRectangle, X: 1, Y: 1, W: 2, H: 2,
		SpriteW: 2, SpriteH: 2, GroupID: 17, Operation: PaintInvert,
		Pix: bytes.Repeat([]byte{255, 255, 255, 255}, 4)}}
	a.objectsRevision++
	a.resetHistory("before inversion flatten")
	expected := copyObjects(a.objects)
	a.enterRasterMode()
	if len(a.objects) != 0 || !a.rasterMode {
		t.Fatal("flatten setup failed")
	}
	if !a.historyUndo() || a.rasterMode || !objectsEqual(a.objects, expected) || a.objects[0].Operation != PaintInvert || a.objects[0].GroupID != 17 {
		t.Fatal("undo must recover the dynamic inverse operation and original group")
	}
}

func TestBeta565RasterModeNoObjectsModeHistoryAndBranch(t *testing.T) {
	a := newApp()
	a.canvas = NewCanvas(3, 3, rgb(255, 255, 255))
	a.resetHistory("plain raster")
	a.enterRasterMode()
	if len(a.historyEntries) != 2 || a.historyIndex != 1 {
		t.Fatal("mode change without objects still must be history navigable")
	}
	if !a.historyUndo() || a.rasterMode {
		t.Fatal("mode-only Undo failed")
	}
	// A new edit after Undo must discard only the future branch, not previous history.
	a.recordHistoryStep("separate branch", true)
	if len(a.historyEntries) != 2 || a.historyRedo() {
		t.Fatal("redo branch should be replaced by new edit")
	}
}

func TestBeta565UndoRasterStrokeThenRasterModeToOriginalObjects(t *testing.T) {
	a := beta3TwoObjectCanvas(t)
	beforeObjects := copyObjects(a.objects)
	beforePixels := a.canvas.ClonePixels()
	a.enterRasterMode()
	flattenedPixels := a.canvas.ClonePixels()
	// A normal pixel change after rasterization is a separate undo step.
	a.canvas.Pix[0] ^= 0xff
	a.recordHistory("래스터 붓")
	if !a.historyUndo() || !a.rasterMode || len(a.objects) != 0 || !bytes.Equal(a.canvas.Pix, flattenedPixels) {
		t.Fatal("first Undo must undo only the raster brush edit")
	}
	if !a.historyUndo() || a.rasterMode || !objectsEqual(a.objects, beforeObjects) || !bytes.Equal(a.canvas.Pix, beforePixels) {
		t.Fatal("second Undo must undo raster mode transition and recover original objects")
	}
	if !a.historyRedo() || !a.rasterMode || len(a.objects) != 0 || !bytes.Equal(a.canvas.Pix, flattenedPixels) {
		t.Fatal("first Redo must restore raster mode and flattened pixels")
	}
	if !a.historyRedo() || a.canvas.Pix[0] == flattenedPixels[0] {
		t.Fatal("second Redo must restore the raster brush edit")
	}
}
