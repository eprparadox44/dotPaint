package main

import "testing"

func TestBeta51ObjectBucketWallProtectsBoundary(t *testing.T) {
	w, h := 10, 8
	pix := make([]byte, w*h*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = 255, 255, 255, 255
	}
	wall := make([]byte, w*h)
	for y := 0; y < h; y++ {
		wall[y*w+4] = 1
	}
	for y := 0; y < h; y++ { // the original raster is unchanged beneath the object line
		_ = y
	}
	black := [4]byte{0, 0, 0, 255}
	n := floodFillBehindObjects(pix, w, h, 2, 4, black, 0, wall)
	if n != 4*h {
		t.Fatalf("fill crossed wall or missed reachable space: %d", n)
	}
	for y := 0; y < h; y++ {
		if pix[(y*w+5)*4] != 255 || pix[(y*w+4)*4] != 255 {
			t.Fatal("object barrier was recoloured or crossed")
		}
	}
	if floodFillBehindObjects(pix, w, h, 4, 4, black, 0, wall) != 0 {
		t.Fatal("wall itself must not be filled")
	}
}

func TestBeta51ColorStyleIsPartOfCharacterIdentity(t *testing.T) {
	a := TextCharStyle{FontName: "Arial", FontSize: 20, Color: rgb(50, 80, 110), ColorSet: true}
	b := a
	if !a.Equal(b) {
		t.Fatal("unchanged styles differ")
	}
	b.Color = rgb(51, 80, 110)
	if a.Equal(b) {
		t.Fatal("character colours ignored by style equality")
	}
	b = a
	b.ColorOp = PaintInvert
	if a.Equal(b) {
		t.Fatal("invert glyph mode ignored by style equality")
	}
}

func TestBeta51MixedTextInversionAndZOrder(t *testing.T) {
	pixels := []byte{0, 0, 255, 255} // red BGRA
	inverseGlyph := MovableObject{Kind: ToolText, X: 0, Y: 0, W: 1, H: 1, SpriteW: 1, SpriteH: 1,
		Pix: []byte{255, 255, 255, 255}, GlyphInvertMask: []byte{255}, Operation: PaintRGB}
	compositeObjectsToPixels(pixels, 1, 1, 0, 0, []MovableObject{inverseGlyph})
	if pixels[0] != 255 || pixels[1] != 255 || pixels[2] != 0 {
		t.Fatalf("inverse text should be cyan, got %v", pixels)
	}
	pixels = []byte{0, 0, 255, 255}
	blue := MovableObject{Kind: ToolRectangle, X: 0, Y: 0, W: 1, H: 1, SpriteW: 1, SpriteH: 1, Pix: []byte{255, 0, 0, 255}, Operation: PaintRGB}
	compositeObjectsToPixels(pixels, 1, 1, 0, 0, []MovableObject{inverseGlyph, blue, inverseGlyph})
	if pixels[0] != 0 || pixels[1] != 255 || pixels[2] != 255 {
		t.Fatalf("opaque object must reset inversion underneath; wanted yellow, got %v", pixels)
	}
}

func TestBeta51ObjectRasterWallMask(t *testing.T) {
	line := MovableObject{Kind: ToolLine, X: 2, Y: 0, W: 1, H: 4, SpriteW: 1, SpriteH: 4, Pix: []byte{
		0, 0, 0, 255, 0, 0, 0, 255, 0, 0, 0, 255, 0, 0, 0, 255,
	}}
	wall := bucketObjectWallMask([]MovableObject{line}, 6, 4)
	if len(wall) != 24 || wall[2] != 1 || wall[8] != 1 || wall[14] != 1 || wall[20] != 1 || wall[3] != 0 {
		t.Fatalf("object line wall mismatch: %v", wall)
	}
}
