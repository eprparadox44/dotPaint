//go:build windows

package main

import "testing"

func TestDotPaint20History80UndoRedo(t *testing.T) {
	a := newApp()
	a.canvas = NewCanvas(12, 12, rgb(255, 255, 255))
	a.resetHistory("initial")
	for i := 0; i < 85; i++ {
		a.canvas.SetPixel(i%12, i/12, rgb(uint8(i+1), 0, 0))
		a.recordHistory("change")
	}
	if historyLimit != 81 || len(a.historyEntries) != 81 || a.historyIndex != 80 {
		t.Fatalf("history not capped at 81 entries: limit=%d count=%d idx=%d", historyLimit, len(a.historyEntries), a.historyIndex)
	}
	for i := 0; i < 80; i++ {
		if !a.historyUndo() {
			t.Fatalf("undo #%d failed", i+1)
		}
	}
	if a.historyUndo() {
		t.Fatal("undo beyond 80 operations should not succeed")
	}
	for i := 0; i < 80; i++ {
		if !a.historyRedo() {
			t.Fatalf("redo #%d failed", i+1)
		}
	}
	if a.historyRedo() {
		t.Fatal("redo beyond 80 operations should not succeed")
	}
}
