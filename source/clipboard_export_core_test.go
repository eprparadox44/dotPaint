package main

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"image/png"
	"testing"
)

func TestA111OpaqueClipboardDIBCompositesTransparencyOnWhite(t *testing.T) {
	// top row: transparent black, half-alpha black
	// bottom row: opaque red, opaque blue (BGRA storage)
	pix := []byte{
		0, 0, 0, 0, 0, 0, 0, 128,
		0, 0, 255, 255, 255, 0, 0, 255,
	}
	dib, err := buildOpaqueClipboardDIB(2, 2, pix)
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.LittleEndian.Uint32(dib[4:8]); got != 2 {
		t.Fatalf("width=%d", got)
	}
	if got := binary.LittleEndian.Uint32(dib[8:12]); got != 2 {
		t.Fatalf("height=%d", got)
	}
	// Bottom-up first row must be original bottom row.
	if !bytes.Equal(dib[40:48], []byte{0, 0, 255, 255, 255, 0, 0, 255}) {
		t.Fatalf("bottom row=%v", dib[40:48])
	}
	// Transparent black becomes opaque white; 50% black becomes gray 127.
	if !bytes.Equal(dib[48:52], []byte{255, 255, 255, 255}) {
		t.Fatalf("transparent pixel=%v", dib[48:52])
	}
	if !bytes.Equal(dib[52:56], []byte{127, 127, 127, 255}) {
		t.Fatalf("half alpha pixel=%v", dib[52:56])
	}
}

func TestA111TransparentClipboardPNGPreservesAlphaAndChannels(t *testing.T) {
	pix := []byte{30, 20, 10, 40} // BGRA
	data, err := buildTransparentClipboardPNG(1, 1, pix)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	c := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
	if c.R != 10 || c.G != 20 || c.B != 30 || c.A != 40 {
		t.Fatalf("nrgba=%d,%d,%d,%d", c.R, c.G, c.B, c.A)
	}
}

func TestA219TransparentClipboardPNGPreservesFullyTransparentPixel(t *testing.T) {
	pix := []byte{90, 70, 50, 0} // BGRA: non-black hidden RGB with alpha zero
	data, err := buildTransparentClipboardPNG(1, 1, pix)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	c := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
	if c.R != 50 || c.G != 70 || c.B != 90 || c.A != 0 {
		t.Fatalf("fully transparent nrgba=%d,%d,%d,%d", c.R, c.G, c.B, c.A)
	}
}
