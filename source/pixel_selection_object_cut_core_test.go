package main

import (
	"bytes"
	"testing"
)

func cutTestObject(x, y, w, h int, col uint32) MovableObject {
	p := colorrefToBGRA(col)
	pix := make([]byte, w*h*4)
	for j := 0; j < w*h; j++ {
		copy(pix[j*4:j*4+4], p[:])
	}
	return MovableObject{Kind: ToolRectangle, X: x, Y: y, W: w, H: h, SpriteW: w, SpriteH: h, Pix: pix, Operation: PaintRGB}
}

func cutCanvasView(base *Canvas, objs []MovableObject) *Canvas {
	v := &Canvas{W: base.W, H: base.H, Pix: base.ClonePixels()}
	compositeObjectsToPixels(v.Pix, v.W, v.H, 0, 0, objs)
	return v
}

func TestBeta52UnrelatedSelectionDoesNotRasterizeAnyObject(t *testing.T) {
	base := NewCanvas(15, 12, rgb(250, 250, 250))
	a := cutTestObject(1, 1, 3, 3, rgb(255, 0, 0))
	b := cutTestObject(9, 7, 3, 3, rgb(0, 0, 255))
	objs := []MovableObject{a, b}
	updated, affected := cutSelectedObjectPixels(base, objs, PixelSelectionCut{X: 5, Y: 4, W: 3, H: 2})
	if len(affected) != 0 || !objectsEqual(objs, updated) {
		t.Fatalf("selection on empty background altered objects: %v", affected)
	}
	if updated[0].Rasterized || updated[1].Rasterized {
		t.Fatal("unrelated objects became locked")
	}
}

func TestBeta52SelectionMustTouchActualShapePixels(t *testing.T) {
	base := NewCanvas(12, 12, rgb(255, 255, 255))
	hollow := cutTestObject(2, 2, 7, 7, rgb(0, 0, 0))
	for y := 1; y < 6; y++ {
		for x := 1; x < 6; x++ {
			hollow.Pix[(y*7+x)*4+3] = 0
		}
	}
	original := []MovableObject{hollow}
	updated, affected := cutSelectedObjectPixels(base, original, PixelSelectionCut{X: 4, Y: 4, W: 2, H: 2})
	if len(affected) != 0 || updated[0].Rasterized {
		t.Fatal("hollow interior incorrectly counts as intersecting the shape")
	}
	updated, affected = cutSelectedObjectPixels(base, original, PixelSelectionCut{X: 2, Y: 2, W: 2, H: 2, Mask: []bool{false, false, false, true}})
	if len(affected) != 0 {
		t.Fatal("free-form mask only selected a transparent corner of the hollow rectangle")
	}
	updated, affected = cutSelectedObjectPixels(base, original, PixelSelectionCut{X: 2, Y: 2, W: 1, H: 1})
	if len(affected) != 1 || !updated[0].Rasterized {
		t.Fatal("actual outline pixel should cause only that object to rasterize")
	}
	if updated[0].Pix[3] != 0 {
		t.Fatal("selected line pixel still painted after erase")
	}
}

func TestBeta52OnlyTouchedObjectRasterizesAndCutsSelectedPixels(t *testing.T) {
	base := NewCanvas(16, 12, rgb(255, 255, 255))
	a := cutTestObject(1, 1, 4, 4, rgb(255, 0, 0))
	b := cutTestObject(10, 5, 3, 4, rgb(0, 0, 255))
	b.GroupID = 52
	objs := []MovableObject{a, b}
	before := cutCanvasView(base, objs)
	updated, affected := cutSelectedObjectPixels(base, objs, PixelSelectionCut{X: 3, Y: 2, W: 2, H: 2})
	if len(affected) != 1 || affected[0] != 0 || !updated[0].Rasterized {
		t.Fatalf("expected only first object to be locked, affected=%v", affected)
	}
	if updated[1].Rasterized || updated[1].GroupID != 52 || !bytes.Equal(updated[1].Pix, b.Pix) {
		t.Fatal("unaffected object lost geometry/group/alpha or became rasterized")
	}
	baseAfter := &Canvas{W: base.W, H: base.H, Pix: base.ClonePixels()}
	for y := 2; y < 4; y++ {
		for x := 3; x < 5; x++ {
			baseAfter.SetPixel(x, y, rgb(255, 255, 255))
		}
	}
	after := cutCanvasView(baseAfter, updated)
	for y := 0; y < base.H; y++ {
		for x := 0; x < base.W; x++ {
			off := (y*base.W + x) * 4
			if x >= 3 && x < 5 && y >= 2 && y < 4 {
				if after.Pix[off] != 255 || after.Pix[off+1] != 255 || after.Pix[off+2] != 255 {
					t.Fatalf("selected pixel (%d,%d) not cleared: %v", x, y, after.Pix[off:off+4])
				}
			} else if !bytes.Equal(after.Pix[off:off+4], before.Pix[off:off+4]) {
				t.Fatalf("pixel (%d,%d) outside selection changed", x, y)
			}
		}
	}
}

func TestBeta52FreeSelectionOnlyRasterizesTouchedMaskPixel(t *testing.T) {
	base := NewCanvas(12, 12, rgb(255, 255, 255))
	a := cutTestObject(2, 2, 4, 4, rgb(0, 0, 0))
	mask := make([]bool, 4*4)
	mask[0] = true     // empty/outside shape
	mask[2*4+2] = true // actual shape pixel (3,3)
	updated, affected := cutSelectedObjectPixels(base, []MovableObject{a}, PixelSelectionCut{X: 1, Y: 1, W: 4, H: 4, Mask: mask})
	if len(affected) != 1 || !updated[0].Rasterized {
		t.Fatalf("actual mask intersection not detected: %v", affected)
	}
	after := cutCanvasView(base, updated)
	if col := after.GetPixel(3, 3); col != rgb(255, 255, 255) {
		t.Fatalf("masked pixel not erased: %x", col)
	}
	if col := after.GetPixel(2, 2); col != rgb(0, 0, 0) {
		t.Fatalf("unmasked pixel erased: %x", col)
	}
}

func TestBeta52InverseObjectFrozenOnlyOnActualCut(t *testing.T) {
	base := NewCanvas(6, 3, rgb(255, 0, 0))
	o := cutTestObject(1, 1, 4, 1, rgb(0, 0, 0))
	o.Operation = PaintInvert
	original := []MovableObject{o}
	before := cutCanvasView(base, original)
	updated, affected := cutSelectedObjectPixels(base, original, PixelSelectionCut{X: 2, Y: 1, W: 1, H: 1})
	if len(affected) != 1 || !updated[0].Rasterized || updated[0].Operation != PaintRGB {
		t.Fatalf("inverse object not frozen: %v", affected)
	}
	base.SetPixel(2, 1, rgb(255, 255, 255))
	base.SetPixel(3, 1, rgb(0, 0, 255)) // frozen pixel should not re-invert new blue backdrop
	after := cutCanvasView(base, updated)
	if after.GetPixel(2, 1) != rgb(255, 255, 255) {
		t.Fatalf("deleted inversion still showing at selected pixel: %x", after.GetPixel(2, 1))
	}
	if got, want := after.GetPixel(3, 1), before.GetPixel(3, 1); got != want {
		t.Fatalf("frozen inverse pixel changed with new background %x != %x", got, want)
	}
}

func TestBeta52RGBAlphaLayerDoesNotCaptureOtherObject(t *testing.T) {
	base := NewCanvas(5, 5, rgb(255, 255, 255))
	lower := cutTestObject(0, 0, 5, 5, rgb(255, 0, 0))
	upper := cutTestObject(1, 1, 3, 3, rgb(0, 0, 255))
	upper.Transparency = 50
	objs := []MovableObject{lower, upper}
	updated, affected := cutSelectedObjectPixels(base, objs, PixelSelectionCut{X: 1, Y: 1, W: 1, H: 1})
	if len(affected) != 2 {
		t.Fatalf("both overlapping objects should be cut: %v", affected)
	}
	if updated[1].ReplaceMask != nil {
		t.Fatal("ordinary translucent object must retain alpha, not capture lower composite")
	}
	if updated[1].Pix[((2*3+2)*4)+3] != 128 {
		t.Fatalf("rasterized transparency incorrect: alpha=%d", updated[1].Pix[((2*3+2)*4)+3])
	}
}

func TestBeta52UnchangedTransparentObjectAndResizedHit(t *testing.T) {
	base := NewCanvas(8, 8, rgb(255, 255, 255))
	o := cutTestObject(1, 1, 5, 5, rgb(255, 0, 0))
	o.SpriteW, o.SpriteH = 1, 1
	o.Pix = append([]byte(nil), o.Pix[:4]...)
	o.Operation = PaintTransparent
	updated, affected := cutSelectedObjectPixels(base, []MovableObject{o}, PixelSelectionCut{X: 3, Y: 3, W: 1, H: 1})
	if len(affected) > 0 || updated[0].Rasterized {
		t.Fatal("invisible paint operation should not be rasterized")
	}
	o.Operation = PaintRGB
	updated, affected = cutSelectedObjectPixels(base, []MovableObject{o}, PixelSelectionCut{X: 3, Y: 3, W: 1, H: 1})
	if len(affected) != 1 || updated[0].W != 5 || len(updated[0].Pix) != 5*5*4 {
		t.Fatal("resized sprite must be rasterized at display resolution")
	}
}

func TestBeta52InvisibleAfterOpacityDoesNotRasterize(t *testing.T) {
	base := NewCanvas(3, 3, rgb(255, 255, 255))
	o := cutTestObject(1, 1, 1, 1, rgb(255, 0, 0))
	o.Pix[3] = 1
	o.Transparency = 99
	out, affected := cutSelectedObjectPixels(base, []MovableObject{o}, PixelSelectionCut{X: 1, Y: 1, W: 1, H: 1})
	if len(affected) != 0 || out[0].Rasterized {
		t.Fatal("rounded alpha-zero RGB pixel should not trigger rasterization")
	}
}
