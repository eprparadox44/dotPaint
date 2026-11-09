//go:build windows

package main

import (
	"bytes"
	"testing"
)

func beta4HistoryApp() *App {
	a := newApp()
	a.canvas = NewCanvas(140, 100, 0xFFFFFF)
	a.resetHistory("시작")
	return a
}

func TestBeta4CurveCommitHistoryKeepsConstructionOperation(t *testing.T) {
	for bends := 1; bends <= 4; bends++ {
		a := beta4HistoryApp()
		blank := a.canvas.ClonePixels()
		a.tool = ToolCurve
		a.curveBendCount = bends
		a.curveSize = 3
		a.curveAntialias = true
		a.curveX0, a.curveY0, a.curveX1, a.curveY1 = 10, 50, 120, 50
		a.curveC1X, a.curveC1Y, a.curveC2X, a.curveC2Y = 25, 10, 50, 85
		a.curveC3X, a.curveC3Y, a.curveC4X, a.curveC4Y = 80, 15, 105, 80
		a.curvePaintColor = rgb(200, 30, 50)
		a.curveOperation = PaintInvert
		a.curveUndoPending, a.preview = true, true
		a.undo.Push(a.canvas)
		for stage := 1; stage <= bends; stage++ {
			a.curveStage = stage
			a.recordHistoryStep("곡선 굽힘점", true)
		}
		a.fg = rgb(20, 70, 90)
		a.fgOperation = PaintRGB
		if !a.commitCurveWithControls(bends, "test") {
			t.Fatal("commit failed")
		}
		if len(a.objects) != 1 || a.objects[0].Vector.Count != bends+2 || a.objects[0].Operation != PaintInvert || a.curveUndoPending {
			t.Fatal("committed curve lost geometry or captured operation")
		}
		if !bytes.Equal(a.canvas.Pix, blank) {
			t.Fatal("kept curve changed raster base")
		}
		count := len(a.historyEntries)
		if count != bends+2 {
			t.Fatalf("duplicate completion history: %d", count)
		}
		if !a.historyUndo() || len(a.objects) != 0 || a.curveStage != bends || a.curveOperation != PaintInvert {
			t.Fatal("undo completion must restore final construction stage")
		}
		if a.fg != rgb(20, 70, 90) || a.fgOperation != PaintRGB {
			t.Fatal("undo changed selected paint preference")
		}
		if !a.historyRedo() || len(a.objects) != 1 || a.curveUndoPending {
			t.Fatal("redo completion")
		}
	}
}

func TestBeta4ObjectEditCancelAndOneHistoryStep(t *testing.T) {
	a := beta4HistoryApp()
	a.tool = ToolObjectSelect
	o, ok := a.makeVectorObject(ToolLine, 10, 20, 90, 60, false, rgb(20, 30, 40))
	if !ok {
		t.Fatal("line object")
	}
	a.addMovableObject(o)
	before := copyObjects(a.objects)
	count := len(a.historyEntries)
	a.objectSelected = 0
	a.objectDragging = true
	a.objectDragStart = o
	a.objectDragHandle = ObjectVertex0
	a.objectDragX, a.objectDragY = 10, 20
	for i := 1; i <= 20; i++ {
		a.updateObjectMove(10+i, 20+i)
	}
	if len(a.historyEntries) != count {
		t.Fatal("mousemove made history entries")
	}
	a.endObjectMove(30, 40, true)
	if len(a.historyEntries) != count+1 {
		t.Fatal("release must commit exactly one history step")
	}
	if !a.historyUndo() || !objectsEqual(a.objects, before) {
		t.Fatal("undo geometry")
	}
	if !a.historyRedo() {
		t.Fatal("redo geometry")
	}
	edited := copyObjects(a.objects)
	o = a.objects[0]
	a.objectSelected = 0
	a.objectDragging = true
	a.objectDragStart = o
	a.objectDragHandle = ObjectVertex0 + 1
	a.objectDragX, a.objectDragY = 0, 0
	a.updateObjectMove(5, 8)
	a.endObjectMove(0, 0, false)
	if !objectsEqual(a.objects, edited) {
		t.Fatal("cancel did not restore original geometry")
	}
}

func TestBeta4InverseRasterizeClipboardAndOpacity(t *testing.T) {
	a := beta4HistoryApp()
	a.tool = ToolObjectSelect
	first, ok := a.makeShapeObject(ToolRectangle, 5, 5, 40, 40, false, rgb(255, 0, 0))
	if !ok {
		t.Fatal("shape")
	}
	a.addMovableObject(first)
	a.drawOperation = PaintInvert
	second, ok := a.makeShapeObject(ToolEllipse, 5, 5, 40, 40, false, 0)
	if !ok {
		t.Fatal("inverse ellipse")
	}
	a.addMovableObject(second)
	a.objectSelected = 1
	before := a.flattenedObjectsCanvas().ClonePixels()
	if a.setSelectedObjectOpacity(50) || a.objects[1].Transparency != 0 {
		t.Fatal("partial inverse opacity accepted")
	}
	clip := a.captureSelectedObjectImage()
	if clip.W != second.W || clip.H != second.H {
		t.Fatal("clipboard bounds")
	}
	for y := 0; y < clip.H; y++ {
		for x := 0; x < clip.W; x++ {
			i := (y*clip.W + x) * 4
			if second.Pix[i+3] >= 128 {
				j := ((y+second.Y)*a.canvas.W + x + second.X) * 4
				if !bytes.Equal(clip.Pix[i:i+4], before[j:j+4]) {
					t.Fatal("inverse clipboard ignored lower composite")
				}
			}
		}
	}
	a.objectSelected = 1
	if !a.rasterizeSelectedObject() {
		t.Fatal("rasterize")
	}
	if !bytes.Equal(before, a.flattenedObjectsCanvas().Pix) || objectHasInversion(&a.objects[1]) || !a.objects[1].Rasterized {
		t.Fatal("rasterize changed composite or retained dynamic operation")
	}
	if !a.historyUndo() || a.objects[1].Rasterized || a.objects[1].Operation != PaintInvert {
		t.Fatal("undo inversion rasterization")
	}
}

func TestBeta4KeepCheckboxDoesNotOverlapCurveArrowSliders(t *testing.T) {
	if keepObjectsOptionRect(ToolCurve).Top <= int32(toolOptionsTop()+335) || keepObjectsOptionRect(ToolArrow).Top <= int32(toolOptionsTop()+335) {
		t.Fatal("Keep Object overlaps size slider")
	}
	for _, kind := range []Tool{ToolRectangle, ToolEllipse, ToolRightTriangle, ToolText, ToolLine, ToolArrow, ToolCurve} {
		if !supportsKeepObjects(kind) {
			t.Fatal("missing supported tool")
		}
	}
}

func TestBeta4RasterStrokeUndoRedoAndCancel(t *testing.T) {
	a := beta4HistoryApp()
	original := a.canvas.ClonePixels()
	a.undo.Push(a.canvas)
	a.invertStrokeActive = true
	a.drawing = true
	a.invertStroke.Line(a.canvas, 2, 2, 50, 50, 7, BrushCircle)
	a.cancelInverseStroke()
	if !bytes.Equal(original, a.canvas.Pix) || a.invertStrokeActive || len(a.invertStroke.tiles) != 0 {
		t.Fatal("cancel inverse stroke")
	}
	a.invertStroke.Line(a.canvas, 2, 2, 50, 50, 7, BrushCircle)
	a.invertStroke.Reset()
	a.recordHistory("반전색 붓질")
	painted := a.canvas.ClonePixels()
	a.fgOperation = PaintInvert
	if !a.historyUndo() || !bytes.Equal(original, a.canvas.Pix) || a.fgOperation != PaintInvert {
		t.Fatal("stroke undo")
	}
	if !a.historyRedo() || !bytes.Equal(painted, a.canvas.Pix) {
		t.Fatal("stroke redo")
	}
}
