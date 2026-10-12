package main

import "testing"

func TestCanvasSizeForClipboardPasteGrowsToImage(t *testing.T) {
	w, h, grew, ok := canvasSizeForClipboardPaste(320, 200, 0, 0, 640, 480)
	if !ok || !grew || w != 640 || h != 480 {
		t.Fatalf("got %dx%d grew=%v ok=%v; want 640x480 grown", w, h, grew, ok)
	}
}

func TestCanvasSizeForClipboardPasteIncludesPasteOffset(t *testing.T) {
	w, h, grew, ok := canvasSizeForClipboardPaste(640, 480, 500, 350, 300, 200)
	if !ok || !grew || w != 800 || h != 550 {
		t.Fatalf("got %dx%d grew=%v ok=%v; want 800x550 grown", w, h, grew, ok)
	}
}

func TestCanvasSizeForClipboardPasteDoesNotShrink(t *testing.T) {
	w, h, grew, ok := canvasSizeForClipboardPaste(800, 600, 10, 10, 100, 100)
	if !ok || grew || w != 800 || h != 600 {
		t.Fatalf("got %dx%d grew=%v ok=%v; want unchanged 800x600", w, h, grew, ok)
	}
}

func TestCanvasSizeForClipboardPasteRejectsUnsafeGrowth(t *testing.T) {
	w, h, grew, ok := canvasSizeForClipboardPaste(640, 480, 0, 0, 32768, 32768)
	if ok || grew || w != 640 || h != 480 {
		t.Fatalf("got %dx%d grew=%v ok=%v; want unchanged and rejected", w, h, grew, ok)
	}
}
