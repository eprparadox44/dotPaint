package main

import "math"

// Hue [0,360), saturation/value [0,1]. RGB values use Win32 COLORREF packing.
func customPickerHSVToRGB(h, s, v float64) uint32 {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	s = math.Max(0, math.Min(1, s))
	v = math.Max(0, math.Min(1, v))
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r, g, b float64
	switch int(h / 60) {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return uint32(byte(math.Round((r+m)*255))) |
		uint32(byte(math.Round((g+m)*255)))<<8 |
		uint32(byte(math.Round((b+m)*255)))<<16
}

func customPickerRGBToHSV(color uint32) (float64, float64, float64) {
	r, g, b := float64(byte(color))/255, float64(byte(color>>8))/255, float64(byte(color>>16))/255
	maxv := math.Max(r, math.Max(g, b))
	minv := math.Min(r, math.Min(g, b))
	d := maxv - minv
	h := 0.0
	if d != 0 {
		switch maxv {
		case r:
			h = 60 * math.Mod((g-b)/d, 6)
		case g:
			h = 60 * ((b-r)/d + 2)
		default:
			h = 60 * ((r-g)/d + 4)
		}
	}
	if h < 0 {
		h += 360
	}
	s := 0.0
	if maxv > 0 {
		s = d / maxv
	}
	return h, s, maxv
}

// A palette is always addressed by its actual index. No paging, sorting or
// deduplication may change the index while editing a 16/256-colour palette.
func customPickerReplaceIndex(p []uint32, index int, colour uint32) bool {
	if index < 0 || index >= len(p) {
		return false
	}
	colour &= 0xFFFFFF
	if p[index] == colour {
		return false
	}
	p[index] = colour
	return true
}
func customPickerGridIndex(col, row, columns, rows int) int {
	if col < 0 || row < 0 || col >= columns || row >= rows {
		return -1
	}
	return row*columns + col
}

// Native main palette has 28 basic slots (14 per row) and 16 user slots
// (8 per row). Position of each slot in the colour editor matches the order
// in the docked main palette. -1 denotes a point outside either row.
func customPickerMainSwatchPoint(i int) (int, int) {
	if i < 0 || i >= 44 {
		return -1, -1
	}
	row, col := 0, 0
	if i < 28 {
		row, col = i/14, i%14
	} else {
		row, col = (i-28)/8, 14+(i-28)%8
	}
	return 22 + col*24, 256 + row*24
}
func customPickerMainSlotAt(x, y int) int {
	if x < 22 || y < 256 {
		return -1
	}
	col, row := (x-22)/24, (y-256)/24
	if row < 0 || row > 1 || col < 0 || col >= 22 || (x-22)%24 >= 20 || (y-256)%24 >= 20 {
		return -1
	}
	if col < 14 {
		return row*14 + col
	}
	return 28 + row*8 + (col - 14)
}

// Used if an indexed map was opened without a previous normal-palette
// snapshot (e.g. recovery from an imported project).
func defaultMainRGBPalette() []uint32 {
	return []uint32{
		0, 0x808080, 0x000080, 0x008080, 0x008000, 0x808000, 0x800000, 0x800080, 0x408080, 0x404000, 0xff8000, 0x804000, 0xff0040, 0x004080,
		0xffffff, 0xc0c0c0, 0x0000ff, 0x00ffff, 0x00ff00, 0xffff00, 0xff0000, 0xff00ff, 0xc0ffff, 0x80ff80, 0xffff80, 0xff8080, 0x8000ff, 0x4080ff,
	}
}

// The same RGB may occur in several slots (the 16 custom colours start white).
// Prefer the exact slot clicked by the user, but reject stale identities when
// the foreground/background colour or the underlying palette has changed.
func customPickerInitialMainSlot(base []uint32, custom [16]uint32, colour uint32, preferred int) int {
	colour &= 0xffffff
	at := func(i int) uint32 {
		if i >= 0 && i < len(base) && i < 28 {
			return base[i]
		}
		if i >= 28 && i < 44 {
			return custom[i-28]
		}
		return 0xffffffff
	}
	if preferred >= 0 && preferred < 44 && at(preferred) == colour {
		return preferred
	}
	for i := 0; i < 44; i++ {
		if at(i) == colour {
			return i
		}
	}
	return -1
}

// A 32x8 grid keeps all 256 actual indices visible without an 854px dialog.
// RGB palette order and indexed palette order must never be mixed.
func customPickerIndexedGrid(mode int) (columns, rows int) {
	if mode == 256 {
		return 32, 8
	}
	if mode == 16 {
		return 16, 1
	}
	return 0, 0
}

// The upper CURRENT palette uses its own dense 256-colour preview geometry.
// This is independent of the lower editable 32x8 indexed grid.
func customPickerCurrentIndexedGrid(mode int) (columns, rows, pitchX, pitchY, cellW, cellH int) {
	switch mode {
	case 16:
		return 16, 1, 24, 24, 20, 20
	case 256:
		return 32, 8, 16, 10, 15, 9
	}
	return 0, 0, 0, 0, 0, 0
}
func customPickerCurrentIndexedSlotAt(mode, x, y int) int {
	cols, rows, dx, dy, w, h := customPickerCurrentIndexedGrid(mode)
	if cols == 0 || x < 22 || y < 251 {
		return -1
	}
	col, row := (x-22)/dx, (y-251)/dy
	if col < 0 || col >= cols || row < 0 || row >= rows || (x-22)%dx >= w || (y-251)%dy >= h {
		return -1
	}
	return row*cols + col
}
