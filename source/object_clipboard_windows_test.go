//go:build windows

package main

import "testing"

// Intended for an actual Windows desktop session. Cross-compilation verifies
// the integration but is not a substitute for OS clipboard interoperability.
func TestBeta56WindowsObjectClipboardFullRoundTrip(t *testing.T) {
	a := beta2WindowsApp(t)
	beta2AddShape(t, a, ToolRectangle)
	beta2AddShape(t, a, ToolEllipse)
	a.tool = ToolObjectSelect
	a.selectObjectWithGroup(0)
	a.toggleObjectSelectionWithShift(1)
	if !a.groupSelectedObjects() {
		t.Fatal("group failed")
	}
	img := a.captureSelectedObjectImage()
	if !validClipboardImage(img) {
		t.Fatal("invalid grouped preview")
	}
	extras, err := a.selectedObjectsForClipboard()
	if err != nil || len(extras.Native) == 0 {
		t.Fatal("native copy", err)
	}
	if err := a.writeSystemClipboard(img, extras); err != nil {
		t.Fatal("system clipboard write", err)
	}
	got, err := readEditableObjectClipboard()
	if err != nil || len(got.Objects) != 2 {
		t.Fatal("native clipboard read", err)
	}
	raster, _, err := a.readSystemClipboard()
	if err != nil || !validClipboardImage(raster) || raster.W != img.W || raster.H != img.H {
		t.Fatal("raster fallback", err)
	}
	original := len(a.objects)
	if !a.pasteEditableObjects(got) {
		t.Fatal("native paste failed")
	}
	if len(a.objects) != original+2 || a.objects[original].GroupID == 0 || a.objects[original].GroupID == a.objects[0].GroupID {
		t.Fatal("re-attach or group collision")
	}
	if !a.historyUndo() || len(a.objects) != original {
		t.Fatal("native paste undo")
	}
	if !a.historyRedo() || len(a.objects) != original+2 {
		t.Fatal("native paste redo")
	}
}
