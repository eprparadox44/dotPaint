package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/gif"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// The production file encoders run on the host too. Lossy formats are checked
// at the center of broad solid regions; PNG and BMP are checked pixel exactly.
func TestBeta4ProductionImageExportComposite(t *testing.T) {
	c := NewCanvas(96, 32, rgb(255, 0, 0))
	for y := 0; y < c.H; y++ {
		for x := 32; x < 64; x++ {
			c.SetPixel(x, y, rgb(0, 0, 255))
		}
		for x := 64; x < 96; x++ {
			c.SetPixel(x, y, 0xFFFFFF)
		}
	}
	inv := MovableObject{Kind: ToolRectangle, X: 0, Y: 0, W: 96, H: 32, SpriteW: 96, SpriteH: 32, Pix: bytes.Repeat([]byte{255, 255, 255, 255}, 96*32), Operation: PaintInvert}
	compositeObjectsToPixels(c.Pix, c.W, c.H, 0, 0, []MovableObject{inv})
	original := c.ClonePixels()
	for _, ext := range []string{"png", "bmp", "jpg", "gif"} {
		t.Run(ext, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "composite."+ext)
			if err := encodeImageFile(path, c); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if ext == "bmp" {
				if string(data[:2]) != "BM" || binary.LittleEndian.Uint32(data[18:22]) != 96 || binary.LittleEndian.Uint32(data[22:26]) != 32 {
					t.Fatal("BMP header")
				}
				row := 96 * 3
				for y := 0; y < 32; y++ {
					for x := 0; x < 96; x++ {
						si := 54 + (31-y)*row + x*3
						di := (y*96 + x) * 4
						if !bytes.Equal(data[si:si+3], c.Pix[di:di+3]) {
							t.Fatal("BMP did not export composite")
						}
					}
				}
				return
			}
			im, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if ext == "png" {
				for y := 0; y < 32; y++ {
					for x := 0; x < 96; x++ {
						r, g, b, a := im.At(x, y).RGBA()
						i := (y*96 + x) * 4
						if byte(r>>8) != c.Pix[i+2] || byte(g>>8) != c.Pix[i+1] || byte(b>>8) != c.Pix[i] || a != 65535 {
							t.Fatal("PNG exact composite")
						}
					}
				}
			} else {
				for _, x := range []int{16, 48, 80} {
					r, g, b, _ := im.At(x, 16).RGBA()
					i := (16*96 + x) * 4
					for ch, got := range []byte{byte(b >> 8), byte(g >> 8), byte(r >> 8)} {
						if absInt(int(got)-int(c.Pix[i+ch])) > 4 {
							t.Fatal("lossy solid composite color")
						}
					}
				}
				if ext == "gif" {
					if _, err := gif.Decode(bytes.NewReader(data)); err != nil {
						t.Fatal(err)
					}
				} else {
					if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
	if !bytes.Equal(original, c.Pix) {
		t.Fatal("encoder modified document pixels")
	}
}

func TestBeta4DynamicMoveOrderAndBackground(t *testing.T) {
	base := NewCanvas(3, 1, 0xFFFFFF)
	base.SetPixel(1, 0, 0)
	base.SetPixel(2, 0, rgb(255, 0, 0))
	inv := b4PixelObject(0xFFFFFF, PaintInvert)
	for x, want := range [][]byte{{0, 0, 0, 255}, {255, 255, 255, 255}, {255, 255, 0, 255}} {
		inv.X = x
		out := base.ClonePixels()
		compositeObjectsToPixels(out, 3, 1, 0, 0, []MovableObject{inv})
		if !bytes.Equal(out[x*4:][:4], want) {
			t.Fatal("moved inverse color")
		}
	}
	blue := b4PixelObject(rgb(0, 0, 255), PaintRGB)
	inv.X = 0
	objects := []MovableObject{inv, blue}
	out := base.ClonePixels()
	compositeObjectsToPixels(out, 3, 1, 0, 0, objects)
	if out[0] != 255 || out[1] != 0 {
		t.Fatal("opaque blue")
	}
	reorderObject(objects, 0, 1)
	out = base.ClonePixels()
	compositeObjectsToPixels(out, 3, 1, 0, 0, objects)
	if !bytes.Equal(out[:4], []byte{0, 255, 255, 255}) {
		t.Fatal("reordered inverse")
	}
	base.SetPixel(0, 0, rgb(10, 20, 30))
	out = base.ClonePixels()
	compositeObjectsToPixels(out, 3, 1, 0, 0, []MovableObject{inv})
	if !bytes.Equal(out[:4], []byte{225, 235, 245, 255}) {
		t.Fatal("dynamic background")
	}
}
