//go:build windows

package main

import (
	"bytes"
	"testing"
)

func Test1316UndoKeepsCurrentSelectedDrawingColors(t *testing.T) {
	a := newApp()
	a.canvas = NewCanvas(12, 8, rgb(255, 255, 255))
	a.resetHistory("시작")
	a.fg, a.bg = rgb(200, 0, 0), rgb(0, 0, 200)
	a.canvas.SetPixel(5, 4, a.fg)
	a.recordHistory("직선")
	// A color pick made after the draw must survive both undo and redo.
	a.fg, a.bg = rgb(23, 45, 67), rgb(76, 54, 32)
	a.customColors[0] = rgb(1, 2, 3)
	if !a.historyUndo() || a.canvas.GetPixel(5, 4) != rgb(255, 255, 255) {
		t.Fatal("drawing did not undo")
	}
	if a.fg != rgb(23, 45, 67) || a.bg != rgb(76, 54, 32) || a.customColors[0] != rgb(1, 2, 3) {
		t.Fatal("undo unexpectedly restored an old color selection")
	}
	if !a.historyRedo() || a.canvas.GetPixel(5, 4) != rgb(200, 0, 0) {
		t.Fatal("drawing did not redo")
	}
	if a.fg != rgb(23, 45, 67) || a.bg != rgb(76, 54, 32) {
		t.Fatal("redo unexpectedly changed current color selection")
	}
}

func Test1316CurveHistoryEachBendAndRedo(t *testing.T) {
	a := newApp()
	a.canvas = NewCanvas(32, 32, rgb(255, 255, 255))
	a.resetHistory("시작")
	blank := append([]byte(nil), a.canvas.Pix...)
	a.tool, a.curveBendCount = ToolCurve, 2
	a.curveUndoPending, a.preview = true, true
	a.curvePaintColor, a.curveSize = rgb(20, 40, 60), 2
	a.curveX0, a.curveY0, a.curveX1, a.curveY1 = 2, 10, 29, 10

	a.curveStage = 1
	a.recordHistoryStep("곡선 기준선", true)
	a.curveC1X, a.curveC1Y, a.curveStage = 10, 3, 2
	a.recordHistoryStep("곡선 꺾기 1", true)
	a.curveC2X, a.curveC2Y = 22, 20
	a.drawRasterCurve(a.canvas, 2, a.curvePaintColor)
	a.curveStage, a.curveUndoPending, a.preview = 0, false, false
	a.recordHistory("곡선")
	if len(a.historyEntries) != 4 {
		t.Fatalf("expected 3 distinct curve steps after initial state, got %d", len(a.historyEntries))
	}
	painted := append([]byte(nil), a.canvas.Pix...)
	if bytes.Equal(painted, blank) {
		t.Fatal("test curve did not draw")
	}

	if !a.historyUndo() || !a.curveUndoPending || a.curveStage != 2 || !a.preview || !bytes.Equal(a.canvas.Pix, blank) {
		t.Fatal("first undo must roll back only the last bend to an editable preview")
	}
	if !a.historyUndo() || !a.curveUndoPending || a.curveStage != 1 || !bytes.Equal(a.canvas.Pix, blank) {
		t.Fatal("second undo must roll back only the first bend")
	}
	if !a.historyUndo() || a.curveUndoPending || a.curveStage != 0 {
		t.Fatal("third undo must remove baseline without touching preceding edits")
	}
	if !a.historyRedo() || a.curveStage != 1 || !a.curveUndoPending {
		t.Fatal("redo baseline failed")
	}
	if !a.historyRedo() || a.curveStage != 2 || !a.curveUndoPending {
		t.Fatal("redo first bend failed")
	}
	if !a.historyRedo() || a.curveUndoPending || !bytes.Equal(a.canvas.Pix, painted) {
		t.Fatal("redo complete curve failed")
	}
}

func Test1316RedoBranchReplacedAfterCurveBendChange(t *testing.T) {
	a := newApp()
	a.canvas = NewCanvas(16, 16, rgb(255, 255, 255))
	a.resetHistory("시작")
	a.tool, a.curveBendCount = ToolCurve, 2
	a.curveUndoPending, a.preview, a.curveStage = true, true, 1
	a.curveX0, a.curveY0, a.curveX1, a.curveY1 = 1, 2, 14, 2
	a.curvePaintColor, a.curveSize = rgb(0, 0, 0), 1
	a.recordHistoryStep("곡선 기준선", true)
	a.curveC1X, a.curveC1Y, a.curveStage = 5, 5, 2
	a.recordHistoryStep("곡선 꺾기 1", true)
	if !a.historyUndo() || a.curveStage != 1 {
		t.Fatal("can't undo to baseline")
	}
	a.curveC1X, a.curveC1Y, a.curveStage = 9, 12, 2
	a.recordHistoryStep("곡선 꺾기 1", true)
	if a.historyRedo() || a.historyEntries[2].Curve.C1X != 9 || len(a.historyEntries) != 3 {
		t.Fatal("editing a previous bend must invalidate the old redo branch")
	}
}
