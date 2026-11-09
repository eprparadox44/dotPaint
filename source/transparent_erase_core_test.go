package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestBeta562TrueAlphaEraserWithoutObjects(t *testing.T) {
	base := NewCanvas(8, 7, rgb(70, 90, 110))
	mask := &Canvas{W: 3, H: 3, Pix: make([]byte, 3*3*4)}
	mask.Pix[(1*3+1)*4+3] = 255
	result, changed := eraseCompositedPixels(base, nil, mask, 2, 2)
	if !changed || len(result) != 0 {
		t.Fatalf("expected erase without new objects, changed=%v objects=%d", changed, len(result))
	}
	if base.GetPixel(3, 3) != transparentColor {
		t.Fatal("opaque point was not erased to transparency")
	}
	if base.GetPixel(2, 2) != rgb(70, 90, 110) || base.GetPixel(7, 6) != rgb(70, 90, 110) {
		t.Fatal("unselected pixels changed")
	}
}

func TestBeta562EraserTouchesOnlyIntersectedObject(t *testing.T) {
	base := NewCanvas(16, 16, rgb(255, 255, 255))
	red := MovableObject{Kind: ToolRectangle, X: 2, Y: 2, W: 4, H: 4, SpriteW: 4, SpriteH: 4, Pix: bytes.Repeat([]byte{0, 0, 255, 255}, 16)}
	blue := MovableObject{Kind: ToolRectangle, X: 11, Y: 10, W: 3, H: 3, SpriteW: 3, SpriteH: 3, Pix: bytes.Repeat([]byte{255, 0, 0, 255}, 9)}
	originalBlue := blue.Pix
	mask := &Canvas{W: 3, H: 3, Pix: bytes.Repeat([]byte{255, 255, 255, 255}, 9)}
	result, changed := eraseCompositedPixels(base, []MovableObject{red, blue}, mask, 3, 3)
	if !changed || len(result) != 2 || !result[0].Rasterized || result[1].Rasterized || !bytes.Equal(result[1].Pix, originalBlue) {
		t.Fatalf("selective rasterization not respected: %#v", result)
	}
	composited := base.ClonePixels()
	compositeObjectsToPixels(composited, 16, 16, 0, 0, result)
	if composited[(3*16+3)*4+3] != 0 {
		t.Fatal("erased object+base pixel is not transparent")
	}
	if composited[(2*16+2)*4+3] == 0 || composited[(11*16+10)*4+3] == 0 {
		t.Fatal("unaffected visual pixels disappeared")
	}
}

func TestBeta562AntialiasedEraseSingleCompositeCoverage(t *testing.T) {
	base := NewCanvas(4, 4, rgb(255, 255, 255))
	obj1 := MovableObject{Kind: ToolRectangle, X: 1, Y: 1, W: 1, H: 1, SpriteW: 1, SpriteH: 1, Pix: []byte{0, 0, 255, 255}}
	obj2 := MovableObject{Kind: ToolRectangle, X: 1, Y: 1, W: 1, H: 1, SpriteW: 1, SpriteH: 1, Pix: []byte{255, 0, 0, 255}}
	mask := &Canvas{W: 1, H: 1, Pix: []byte{255, 255, 255, 128}}
	result, changed := eraseCompositedPixels(base, []MovableObject{obj1, obj2}, mask, 1, 1)
	if !changed || len(result) != 3 || !result[0].Rasterized || !result[1].Rasterized || !result[2].Rasterized {
		t.Fatal("expected two frozen touched objects and one AA composite")
	}
	out := base.ClonePixels()
	compositeObjectsToPixels(out, 4, 4, 0, 0, result)
	i := (1*4 + 1) * 4
	if out[i+3] != 127 || out[i] != 255 || out[i+1] != 0 || out[i+2] != 0 {
		t.Fatalf("AA eraser should retain half the original top blue pixel, got %v", out[i:i+4])
	}
}

func TestBeta562ObjectPointerMatchesSuppliedPNGExactly(t *testing.T) {
	p := objectPointerPixels()
	count := 0
	for _, v := range p {
		if v == 3 {
			count++
		} else if v != 0 {
			t.Fatalf("cursor produced unexpected color value %d", v)
		}
	}
	if count != 214 || p[0] != 3 || p[31] != 0 || p[31*32] != 0 {
		t.Fatalf("cursor alpha mask changed or gained halo: count=%d", count)
	}
}

func TestBeta562ErasedObjectDOPRoundTrip(t *testing.T) {
	base := NewCanvas(9, 7, rgb(210, 220, 230))
	obj := MovableObject{Kind: ToolRectangle, X: 2, Y: 2, W: 4, H: 3, SpriteW: 4, SpriteH: 3, Pix: bytes.Repeat([]byte{30, 90, 240, 255}, 12)}
	mask := &Canvas{W: 2, H: 2, Pix: bytes.Repeat([]byte{255, 255, 255, 255}, 4)}
	newObjs, ok := eraseCompositedPixels(base, []MovableObject{obj}, mask, 3, 3)
	if !ok || !newObjs[0].Rasterized {
		t.Fatal("erase did not freeze impacted object")
	}
	project := &DOPProject{Canvas: base, Objects: newObjs, Palette: []uint32{0xFFFFFF}}
	file := filepath.Join(t.TempDir(), "투명하게 지운 도형.dop")
	if err := WriteDOPProject(file, project, nil); err != nil {
		t.Fatal(err)
	}
	reopened, err := ReadDOPProject(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reopened.Canvas.Pix, base.Pix) || !objectsEqual(reopened.Objects, newObjs) {
		t.Fatal("erased project does not round trip")
	}
	pix := reopened.Canvas.ClonePixels()
	compositeObjectsToPixels(pix, reopened.Canvas.W, reopened.Canvas.H, 0, 0, reopened.Objects)
	if pix[(3*9+3)*4+3] != 0 || pix[(2*9+2)*4+3] != 255 {
		t.Fatal("round-trip changed visible pixels")
	}
}

func TestBeta562TransparentEraserRemovesDynamicInvertAtTouchedPixel(t *testing.T) {
	base := NewCanvas(5, 5, rgb(255, 255, 255))
	inv := MovableObject{Kind: ToolLine, X: 1, Y: 1, W: 2, H: 1, SpriteW: 2, SpriteH: 1, Operation: PaintInvert, Pix: []byte{255, 255, 255, 255, 255, 255, 255, 255}}
	mask := &Canvas{W: 1, H: 1, Pix: []byte{255, 255, 255, 255}}
	objects, ok := eraseCompositedPixels(base, []MovableObject{inv}, mask, 1, 1)
	if !ok || !objects[0].Rasterized || objects[0].Operation != PaintRGB {
		t.Fatal("touched inverted object was not safely frozen")
	}
	pix := base.ClonePixels()
	compositeObjectsToPixels(pix, base.W, base.H, 0, 0, objects)
	if pix[(1*5+1)*4+3] != 0 || pix[(1*5+2)*4] != 0 || pix[(1*5+2)*4+3] != 255 {
		t.Fatalf("transparent cut failed to erase invert pixel while keeping the other pixel: erased=%v retained=%v", pix[(1*5+1)*4:][:4], pix[(1*5+2)*4:][:4])
	}
}
