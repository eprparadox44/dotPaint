//go:build windows

package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// Run on Windows with a desktop session. Linux cross-compilation validates
// these tests but does not count as executing them or validating native IME.
func beta2WindowsApp(t *testing.T) *App {
	t.Helper()
	runtime.LockOSThread()
	previous, edition := app, dotPaintEdition
	dotPaintEdition = "ko"
	a := newApp()
	app = a
	a.zoom = 2
	a.hinst, _, _ = procGetModuleHandleW.Call(0)
	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(rawUTF16Ptr("STATIC"))), uintptr(unsafe.Pointer(rawUTF16Ptr("dotPaint beta2 test"))), WS_OVERLAPPEDWINDOW, 0, 0, 1100, 800, 0, 0, a.hinst, 0)
	if hwnd == 0 {
		app = previous
		dotPaintEdition = edition
		runtime.UnlockOSThread()
		t.Fatalf("test window: %v", err)
	}
	a.hwnd = hwnd
	a.objectSelected = -1
	a.objectEditingIndex = -1
	a.resetHistory("시작")
	t.Cleanup(func() {
		if a.objectDragging {
			a.endObjectMove(0, 0, false)
		}
		if a.textTool.Active {
			a.finishTextEditing(false)
		}
		a.destroyCanvasTileBuffer()
		a.destroyBackBuffer()
		a.destroyTextUIFont()
		a.destroyDockPanelTitleFont()
		if a.menu != 0 {
			procDestroyMenu.Call(a.menu)
		}
		procDestroyWindow.Call(hwnd)
		app = previous
		dotPaintEdition = edition
		runtime.UnlockOSThread()
	})
	return a
}

func beta2AddShape(t *testing.T, a *App, kind Tool) MovableObject {
	t.Helper()
	o, ok := a.makeShapeObject(kind, 50, 50, 100, 90, false, rgb(40, 80, 180))
	if !ok {
		t.Fatal("shape object failed")
	}
	a.addMovableObject(o)
	a.tool = ToolObjectSelect
	return o
}

func TestBeta2WindowsAllResizeHandlesAndOneHistoryStep(t *testing.T) {
	a := beta2WindowsApp(t)
	for _, kind := range []Tool{ToolRectangle, ToolEllipse, ToolRightTriangle} {
		for handle := ObjectNW; handle <= ObjectW; handle++ {
			a.objects = nil
			a.objectsRevision++
			a.resetHistory("시작")
			o := beta2AddShape(t, a, kind)
			base := a.canvas.ClonePixels()
			ox, oy := a.objectClientGeometry()
			p := objectHandlePoints(o.bounds(), a.zoom, ox, oy)[handle-1]
			a.beginObjectInteraction(p.X, p.Y)
			if !a.objectDragging || a.objectDragHandle != handle {
				t.Fatalf("kind %d handle %d not armed", kind, handle)
			}
			hist := len(a.historyEntries)
			for n := 1; n <= 10; n++ {
				a.updateObjectMove(a.objectDragX+n, a.objectDragY+n)
			}
			if len(a.historyEntries) != hist {
				t.Fatal("per-mousemove history entry")
			}
			x, y := a.objectDragX+10, a.objectDragY+10
			a.endObjectMove(x, y, true)
			if len(a.historyEntries) != hist+1 {
				t.Fatal("resize must commit one step")
			}
			if !bytes.Equal(a.canvas.Pix, base) {
				t.Fatal("resize changed underlying canvas")
			}
			if !a.historyUndo() || a.objects[0].bounds() != o.bounds() {
				t.Fatal("resize undo")
			}
			if !a.historyRedo() || a.objects[0].bounds() == o.bounds() {
				t.Fatal("resize redo")
			}
		}
	}
}

func TestBeta2WindowsMoveClickCancelAndCaptureLoss(t *testing.T) {
	a := beta2WindowsApp(t)
	o := beta2AddShape(t, a, ToolEllipse)
	ox, oy := a.objectClientGeometry()
	mx, my := ox+(o.X+o.W/2)*a.zoom, oy+(o.Y+o.H/2)*a.zoom
	hist := len(a.historyEntries)
	a.beginObjectInteraction(mx, my)
	a.endObjectMove(a.objectDragX, a.objectDragY, true)
	if len(a.historyEntries) != hist {
		t.Fatal("selection only created history")
	}
	a.beginObjectInteraction(mx, my)
	a.updateObjectMove(a.objectDragX+12, a.objectDragY+8)
	a.endObjectMove(0, 0, false)
	if a.objects[0].bounds() != o.bounds() || len(a.historyEntries) != hist {
		t.Fatal("cancel changed object/history")
	}
	a.beginObjectInteraction(mx, my)
	a.updateObjectMove(a.objectDragX+12, a.objectDragY+8)
	other, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(rawUTF16Ptr("STATIC"))), 0, WS_OVERLAPPEDWINDOW, 0, 0, 200, 200, 0, 0, a.hinst, 0)
	defer procDestroyWindow.Call(other)
	procSetCapture.Call(other)
	wndProc(a.hwnd, WM_CAPTURECHANGED, 0, other)
	owner, _, _ := procGetCapture.Call()
	if owner != other {
		t.Fatal("capture cancellation released another window's capture")
	}
	procReleaseCapture.Call()
	if a.objectDragging || a.objects[0].bounds() != o.bounds() || len(a.historyEntries) != hist {
		t.Fatal("capture loss was not rolled back")
	}
	a.beginObjectInteraction(mx, my)
	a.updateObjectMove(a.objectDragX+10, a.objectDragY+5)
	wndProc(a.hwnd, WM_CANCELMODE, 0, 0)
	if a.objectDragging || a.objects[0].bounds() != o.bounds() {
		t.Fatal("WM_CANCELMODE")
	}
}

func TestBeta2WindowsTinyContextMoveAndResize(t *testing.T) {
	a := beta2WindowsApp(t)
	o, ok := a.makeShapeObject(ToolEllipse, 50, 50, 50, 50, false, rgb(0, 0, 0))
	if !ok {
		t.Fatal("tiny object")
	}
	a.addMovableObject(o)
	a.tool = ToolObjectSelect
	ox, oy := a.objectClientGeometry()
	mx, my := ox+o.X*a.zoom, oy+o.Y*a.zoom
	if !a.applyObjectMenuCommand(objectMenuMove) || a.objectArmed != objectArmMove || a.objectDragging {
		t.Fatal("Move must arm without synthetic dragging")
	}
	a.beginObjectInteraction(mx, my)
	if a.objectDragHandle != ObjectBody {
		t.Fatal("tiny Move was stolen by resize handle")
	}
	a.endObjectMove(a.objectDragX+4, a.objectDragY+3, true)
	if a.objects[0].X != o.X+4 {
		t.Fatal("forced Move")
	}
	a.applyObjectMenuCommand(objectMenuResize)
	a.beginObjectInteraction(mx, my)
	if a.objectDragHandle != ObjectSE {
		t.Fatal("forced Resize must pick bottom-right")
	}
	a.endObjectMove(a.objectDragX+8, a.objectDragY+6, true)
	if a.objects[0].W != o.W+8 || a.objects[0].H != o.H+6 {
		t.Fatal("forced Resize")
	}
}

func TestBeta2WindowsOpacityOrderDeletionAndHistoryBranch(t *testing.T) {
	a := beta2WindowsApp(t)
	for _, kind := range []Tool{ToolRectangle, ToolEllipse, ToolRightTriangle} {
		beta2AddShape(t, a, kind)
	}
	before := copyObjects(a.objects)
	a.fg, a.bg = rgb(12, 34, 56), rgb(65, 43, 21)
	for _, command := range []int{objectMenuBack, objectMenuForward, objectMenuFront, objectMenuBackward} {
		a.objectSelected = 1
		i := objectOrderDestination(command, 1, len(a.objects))
		if !a.applyObjectMenuCommand(command) || a.objectSelected != i {
			t.Fatal("order command")
		}
		if !a.historyUndo() || !objectsEqual(a.objects, before) {
			t.Fatal("order undo")
		}
	}
	a.objectSelected = 1
	if !a.setSelectedObjectOpacity(25) || a.objects[1].Transparency != 75 {
		t.Fatal("opacity")
	}
	if !a.historyUndo() || !objectsEqual(a.objects, before) {
		t.Fatal("opacity undo")
	}
	if !a.historyRedo() || a.objects[1].Transparency != 75 {
		t.Fatal("opacity redo")
	}
	if a.fg != rgb(12, 34, 56) || a.bg != rgb(65, 43, 21) {
		t.Fatal("undo changed selected drawing colors")
	}
	a.historyUndo()
	a.objectSelected = 1
	if !a.deleteSelectedObject() || len(a.objects) != 2 {
		t.Fatal("delete")
	}
	if a.historyRedo() {
		t.Fatal("redo branch survived new edit")
	}
	if !a.historyUndo() || !objectsEqual(a.objects, before) {
		t.Fatal("delete undo")
	}
}

func TestBeta2WindowsTextReflowAndEditFormatting(t *testing.T) {
	a := beta2WindowsApp(t)
	a.textTool = TextState{X: 20, Y: 20, W: 160, H: 100, Text: "한글Ab", FontName: "Arial", FontSize: 20, Styles: []TextCharStyle{
		{FontName: "Arial", FontSize: 20, Bold: true}, {FontName: "Arial", FontSize: 20, Italic: true},
		{FontName: "Arial", FontSize: 12, Underline: true}, {FontName: "Arial", FontSize: 12, ScriptMode: 1},
	}}
	o, ok := a.makeTextObject(a.textTool.Text)
	if !ok {
		t.Fatal("text sprite")
	}
	o.Transparency = 50
	a.addMovableObject(o)
	a.tool = ToolObjectSelect
	styles := append([]TextCharStyle(nil), o.Styles...)
	a.objects[0].W = 90
	fg, bg := a.fg, a.bg
	if !a.renderResizedObject(&a.objects[0]) {
		t.Fatal("text resize")
	}
	if a.fg != fg || a.bg != bg {
		t.Fatal("resize changed drawing colors")
	}
	for i, s := range styles {
		if !a.objects[0].Styles[i].Equal(s) {
			t.Fatal("text reflow changed formatting")
		}
	}
	if !a.beginEditingSelectedTextObject() {
		t.Fatal("F2 text edit")
	}
	if a.textTool.Text != o.Text || len(a.textTool.Styles) != len(styles) {
		t.Fatal("text edit discarded content/styles")
	}
	a.finishTextEditing(false)
	if a.objects[0].Text != o.Text || a.objects[0].Transparency != 50 {
		t.Fatal("Esc changed saved text")
	}
	if !a.beginEditingSelectedTextObject() {
		t.Fatal("text edit restart")
	}
	procSetWindowTextW.Call(a.textTool.EditHwnd, uintptr(unsafe.Pointer(rawUTF16Ptr(""))))
	a.finishTextEditing(true)
	if len(a.objects) != 0 {
		t.Fatal("empty text edit must remove object")
	}
	if !a.historyUndo() || len(a.objects) != 1 {
		t.Fatal("empty text deletion undo")
	}
}

func TestBeta2WindowsClipboardExportAndIndexedConversion(t *testing.T) {
	a := beta2WindowsApp(t)
	beta2AddShape(t, a, ToolEllipse)
	a.setSelectedObjectOpacity(50)
	flat := a.flattenedObjectsCanvas()
	a.selection = SelectionState{Active: true, W: a.canvas.W, H: a.canvas.H}
	clip := a.captureSelectionImage()
	if !bytes.Equal(clip.Pix, flat.Pix) {
		t.Fatal("clipboard dropped object composition")
	}
	objClip := a.captureSelectedObjectImage()
	if len(objClip.Pix) == 0 {
		t.Fatal("object clipboard empty")
	}
	encoded, err := buildTransparentClipboardPNG(objClip.W, objClip.H, objClip.Pix)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(encoded)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "objects.png")
	if err := encodeImageFile(path, flat); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	im, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if im.Bounds().Dx() != a.canvas.W {
		t.Fatal("image export size")
	}
	for _, size := range []int{16, 256} {
		a.selection = SelectionState{}
		a.switchIndexedPaletteMode(size, true)
		if len(a.objects) != 0 || !a.indexedMode || a.indexedSize != size {
			t.Fatal("indexed object flatten")
		}
		if _, ok := a.makeShapeObject(ToolEllipse, 5, 5, 20, 20, false, a.fg); ok {
			t.Fatal("true-color object leaked into indexed mode")
		}
		if len(a.indexedPixelIndices) != a.canvas.W*a.canvas.H {
			t.Fatal("indexed storage changed")
		}
	}
}

func TestBeta2WindowsLanguageThemeMatrixPreservesDocument(t *testing.T) {
	a := beta2WindowsApp(t)
	beta2AddShape(t, a, ToolEllipse)
	a.setSelectedObjectOpacity(75)
	before := copyObjects(a.objects)
	revision := a.objectsRevision
	hist := len(a.historyEntries)
	for _, lang := range []string{"ko", "en"} {
		for _, dark := range []bool{false, true} {
			dotPaintEdition = lang
			a.darkMode = dark
			a.applyWindowTheme()
			a.rebuildMenuForTheme()
			a.updateObjectToolTooltip()
			a.refreshObjectGuidance()
			if lang == "en" && (strings.Contains(a.statusText, "객체") || localizeUI("객체 선택") == "객체 선택") {
				t.Fatal("English object UI not localized")
			}
			if !objectsEqual(before, a.objects) || a.objectsRevision != revision || len(a.historyEntries) != hist {
				t.Fatal("language/theme changed document")
			}
		}
	}
}
