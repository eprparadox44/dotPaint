//go:build windows

package main

import (
	"bytes"
	"testing"
)

func Test150MoveableShapesPixelExactWithoutChangingBase(t *testing.T) {
	for _, tt := range []struct {
		name  string
		kind  Tool
		shift bool
	}{
		{"rectangle", ToolRectangle, false},
		{"square-Shift", ToolRectangle, true},
		{"ellipse", ToolEllipse, false},
		{"circle-Shift", ToolEllipse, true},
		{"triangle", ToolRightTriangle, false},
		{"triangle-strict-tan-Shift", ToolRightTriangle, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := newApp()
			a.canvas = NewCanvas(108, 91, rgb(246, 244, 242))
			a.rectangleSize = 4
			a.rightTriangleSize = 5
			a.lineConstraintMode = LineConstraintTanN
			prior := a.canvas.ClonePixels()
			color := rgb(12, 111, 202)
			obj, ok := a.makeShapeObject(tt.kind, 17, 9, 79, 65, tt.shift, color)
			if !ok {
				t.Fatal("object failed")
			}
			if !bytes.Equal(prior, a.canvas.Pix) {
				t.Fatal("object changed raster base")
			}
			expected := &Canvas{W: a.canvas.W, H: a.canvas.H, Pix: append([]byte(nil), prior...)}
			if tt.kind == ToolRectangle {
				a.drawRasterRectangle(expected, 17, 9, 79, 65, tt.shift, color)
			} else if tt.kind == ToolEllipse {
				a.drawRasterEllipse(expected, 17, 9, 79, 65, tt.shift, color)
			} else {
				a.drawRasterRightTriangle(expected, 17, 9, 79, 65, tt.shift, color)
			}
			a.objects = []MovableObject{obj}
			merged := a.flattenedObjectsCanvas()
			if !bytes.Equal(expected.Pix, merged.Pix) {
				for i := 0; i < len(expected.Pix); i++ {
					if expected.Pix[i] != merged.Pix[i] {
						t.Fatalf("pixel mismatch at byte %d expected=%d got=%d", i, expected.Pix[i], merged.Pix[i])
					}
				}
			}
			// Moving the sprite never erases or fills its prior position.
			a.objects[0].X += 7
			a.objects[0].Y += 5
			if !bytes.Equal(prior, a.canvas.Pix) {
				t.Fatal("moving object changed raster base")
			}
		})
	}
}

func Test150ObjectAlphaOrderAndHistoryImmutability(t *testing.T) {
	// A 50% opaque object over a solid black background.
	dst := []byte{0, 0, 0, 255}
	src := []byte{255, 100, 0, 128}
	blendBGRA(dst, 0, src, 0)
	if dst[0] < 127 || dst[0] > 129 || dst[3] != 255 {
		t.Fatalf("bad alpha blend %v", dst)
	}
	// Object geometry is copied, but immutable sprite bytes are safely shared.
	objects := []MovableObject{{Kind: ToolText, X: 3, Y: 4, W: 1, H: 1, Pix: []byte{1, 2, 3, 255}, Text: "안녕", Styles: []TextCharStyle{{FontSize: 20}}}}
	copied := copyObjects(objects)
	copied[0].X = 90
	copied[0].Styles[0].FontSize = 30
	if objects[0].X != 3 || objects[0].Styles[0].FontSize != 20 {
		t.Fatal("snapshot changed source metadata")
	}
	if &objects[0].Pix[0] != &copied[0].Pix[0] {
		t.Fatal("immutable object image not shared")
	}
	if !objectsEqual(objects, objects) {
		t.Fatal("self equality failed")
	}
}

func Test150ObjectHistoryUndoRedo(t *testing.T) {
	a := newApp()
	a.canvas = NewCanvas(64, 64, rgb(255, 255, 255))
	a.objects = nil
	a.objectsRevision = 0
	a.objectSelected = -1
	a.objectEditingIndex = -1
	a.resetHistory("시작")
	before := a.canvas.ClonePixels()
	obj, ok := a.makeShapeObject(ToolRectangle, 6, 7, 21, 23, false, rgb(0, 0, 0))
	if !ok {
		t.Fatal("create failed")
	}
	a.objects = append(a.objects, obj)
	a.objectsRevision++
	a.recordHistory("사각형 객체")
	a.objects[0].X += 12
	a.objectsRevision++
	a.recordHistory("객체 이동")
	if len(a.historyEntries) != 3 {
		t.Fatalf("want 3 histories got %d", len(a.historyEntries))
	}
	if &a.historyEntries[0].Pix[0] != &a.historyEntries[1].Pix[0] {
		t.Fatal("object-only history should reuse base pixel buffer")
	}
	if !a.historyUndo() || len(a.objects) != 1 || a.objects[0].X != obj.X {
		t.Fatal("undo move failed")
	}
	if !a.historyUndo() || len(a.objects) != 0 {
		t.Fatal("undo object creation failed")
	}
	if !a.historyRedo() || len(a.objects) != 1 {
		t.Fatal("redo creation failed")
	}
	if !bytes.Equal(a.canvas.Pix, before) {
		t.Fatal("undo/redo changed raster pixels")
	}
}
