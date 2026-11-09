package main

import (
	"bytes"
	_ "embed"
	"image/png"
)

// Original two-theme pixel art, supplied by the user. Decode exactly once;
// paint and pointer movement never decode, resize or decompress this resource.
//
//go:embed assets/toolbox/object_selection_tool.png
var objectToolReferencePNG []byte

// User-supplied 32x31 selection cursor artwork. It is converted once at
// startup into a crisp adaptive mono cursor and never resized at runtime.
//
//go:embed assets/toolbox/object_selection_cursor.png
var objectCursorReferencePNG []byte

const objectIconW, objectIconH = 41, 37
const objectArtTransparent uint32 = 0xFFFFFFFF

var objectToolIcons = decodeObjectToolIcons()

func decodeObjectToolIcons() [2][objectIconW * objectIconH]uint32 {
	var out [2][objectIconW * objectIconH]uint32
	for t := range out {
		for p := range out[t] {
			out[t][p] = objectArtTransparent
		}
	}
	im, err := png.Decode(bytes.NewReader(objectToolReferencePNG))
	if err != nil || im.Bounds().Dx() != 86 || im.Bounds().Dy() != 37 {
		return out
	}
	for theme := range out {
		for y := 0; y < objectIconH; y++ {
			for x := 0; x < objectIconW; x++ {
				r, g, b, _ := im.At(theme*45+x, y).RGBA()
				r8, g8, b8 := uint32(r>>8), uint32(g>>8), uint32(b>>8)
				bg, edge := uint32(240), uint32(210)
				if theme == 1 {
					bg, edge = 15, 45
				}
				if r8 == g8 && g8 == b8 && (r8 == bg || r8 == edge) {
					continue
				}
				out[theme][y*objectIconW+x] = r8 | (g8 << 8) | (b8 << 16)
			}
		}
	}
	return out
}

// The exact 32x31 user-supplied silhouette is a *screen-inverting* cursor.
// 3 means AND=1 XOR=1: Windows XOR-inverts the actual display pixel under
// each opaque reference pixel, independently of the dotPaint light/dark UI.
// The transparent PNG pixels stay transparent; no halo or rescaling is added.
func objectPointerPixels() [32 * 32]uint8 {
	var out [32 * 32]uint8
	im, err := png.Decode(bytes.NewReader(objectCursorReferencePNG))
	if err != nil || im.Bounds().Dx() != 32 || im.Bounds().Dy() != 31 {
		return out
	}
	for y := 0; y < 31; y++ {
		for x := 0; x < 32; x++ {
			_, _, _, alpha := im.At(x, y).RGBA()
			if alpha >= 0x8000 {
				out[y*32+x] = 3 // cursorInvert: preserve AND and set XOR.
			}
		}
	}
	return out
}

func objectArrowCursorPixels(direction int) [32 * 32]uint8 {
	var ink [32 * 32]bool
	set := func(x, y int) {
		if x >= 0 && y >= 0 && x < 32 && y < 32 {
			ink[y*32+x] = true
		}
	}
	line := func(x0, y0, x1, y1 int) {
		dx, dy := x1-x0, y1-y0
		n := max(absObject(dx), absObject(dy))
		for i := 0; i <= n; i++ {
			if n == 0 {
				set(x0, y0)
			} else {
				set(x0+dx*i/n, y0+dy*i/n)
			}
		}
	}
	arrow := func(dx, dy int) {
		x, y := 15+dx*9, 15+dy*9
		line(15, 15, x, y)
		line(x, y, x-dx*4-dy*3, y-dy*4+dx*3)
		line(x, y, x-dx*4+dy*3, y-dy*4-dx*3)
	}
	switch direction {
	case 0:
		arrow(1, 0)
		arrow(-1, 0)
		arrow(0, 1)
		arrow(0, -1)
	case 1:
		arrow(1, 0)
		arrow(-1, 0)
	case 2:
		arrow(0, 1)
		arrow(0, -1)
	case 3:
		arrow(1, 1)
		arrow(-1, -1)
	case 4:
		arrow(1, -1)
		arrow(-1, 1)
	}
	var out [32 * 32]uint8
	for y := 1; y < 31; y++ {
		for x := 1; x < 31; x++ {
			if !ink[y*32+x] {
				continue
			}
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					out[(y+dy)*32+x+dx] = 2
				}
			}
		}
	}
	for i, v := range ink {
		if v {
			out[i] = 1
		}
	}
	return out
}

func absObject(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
