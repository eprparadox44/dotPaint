package main

import (
	"os"
	"strings"
	"testing"
)

func TestClipboardAutoCanvasSourceIntegration133(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		`const appVersion = "1.2.38"`,
		`func (a *App) newDocument()`,
		`a.readSystemClipboard()`,
		`w, h = img.W, img.H`,
		`canvasSizeForClipboardPaste(oldW, oldH, x, y, cb.W, cb.H)`,
		`a.canvas.ResizeCanvas(newW, newH, 0, 0, a.bg)`,
		`a.updateScrollbars()`,
		`캔버스를 %d × %d → %d × %d로 자동 확장`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("1.2.33 clipboard auto-canvas integration missing %q", want)
		}
	}

	start := strings.Index(s, "func (a *App) pasteClipboardImageAt")
	endRel := strings.Index(s[start:], "func (a *App) pasteClipboardImage(")
	if start < 0 || endRel < 0 {
		t.Fatal("pasteClipboardImageAt source block not found")
	}
	block := s[start : start+endRel]
	undoPos := strings.Index(block, "a.undo.Push(a.canvas)")
	resizePos := strings.Index(block, "a.canvas.ResizeCanvas(newW, newH, 0, 0, a.bg)")
	if undoPos < 0 || resizePos < 0 || undoPos > resizePos {
		t.Fatal("paste undo snapshot must be captured before automatic canvas resize")
	}
}
