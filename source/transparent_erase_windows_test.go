//go:build windows

package main

import "testing"

func TestBeta562InvertCursorUsesTrueXorMask(t *testing.T) {
	p := objectPointerPixels()
	c := newMonoCursorCanvas(32, 32)
	copy(c.pix, p[:])
	andMask, xorMask := c.cursorMasks()
	if andMask[0]&0x80 == 0 || xorMask[0]&0x80 == 0 {
		t.Fatal("first opaque reference pixel must be XOR inverted")
	}
	if andMask[31]&1 == 0 || xorMask[31]&1 != 0 {
		t.Fatal("transparent reference edge changed")
	}
}

func TestBeta562TransparentShapesBecomePixelsNotObjects(t *testing.T) {
	for _, tool := range []Tool{ToolLine, ToolArrow, ToolRectangle, ToolEllipse, ToolRightTriangle} {
		a := newApp()
		a.canvas = NewCanvas(80, 80, rgb(255, 255, 255))
		a.tool = tool
		a.keepObjects = true
		a.drawOperation = PaintTransparent
		a.startX = 11
		a.startY = 13
		_, _, created := a.commitVectorPreview(52, 49, transparentColor)
		if created || len(a.objects) != 0 {
			t.Fatalf("%v made a transparent object", tool)
		}
		n := 0
		for i := 3; i < len(a.canvas.Pix); i += 4 {
			if a.canvas.Pix[i] == 0 {
				n++
			}
		}
		if n == 0 {
			t.Fatalf("%v failed to erase any pixels", tool)
		}
	}
}
