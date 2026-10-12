package main

import (
	"bytes"
	"testing"
)

func TestDejavu141FloatingSelectionRetainsAlphaAndRestoresBaseOnMove(t *testing.T) {
	pal := testDejavuClipboardPalette()
	bg := pal.RGB[7] // (120,110,100)
	a := &App{canvas: NewCanvas(3, 1, rgb(bg[0], bg[1], bg[2])), dejavu: &dejavuIMGDocument{Palette: pal}}
	base := a.canvas.ClonePixels()
	pasted := []byte{
		0, 0, 0, 0, // true alpha hole: never blackens underlying tile
		30, 90, 230, 255, // mapped to index 6 at current destination
		0, 0, 0, 0, // another alpha hole
	}
	a.selection = SelectionState{Active: true, Floating: true, ForceOpaque: true,
		IndexedPalettePaste: true, X: 0, Y: 0, W: 3, H: 1, Pix: pasted, BasePix: base}
	a.applyFloatingSelection(0, 0)
	if !bytes.Equal(a.canvas.Pix[:4], base[:4]) || !bytes.Equal(a.canvas.Pix[8:12], base[8:12]) {
		t.Fatalf("transparent area was changed during paste: %v", a.canvas.Pix)
	}
	if !bytes.Equal(a.canvas.Pix[4:8], []byte{30, 90, 230, 255}) {
		t.Fatalf("opaque centre not indexed: %v", a.canvas.Pix[4:8])
	}
	a.applyFloatingSelection(1, 0)
	if !bytes.Equal(a.canvas.Pix[:4], base[:4]) || !bytes.Equal(a.canvas.Pix[4:8], base[4:8]) {
		t.Fatalf("drag failed to restore old base pixels: %v", a.canvas.Pix)
	}
	if !bytes.Equal(a.canvas.Pix[8:12], []byte{30, 90, 230, 255}) {
		t.Fatalf("dragged pixel missing: %v", a.canvas.Pix[8:12])
	}
	if !bytes.Equal(a.selection.Pix, pasted) {
		t.Fatal("floating source changed across drags")
	}
}
