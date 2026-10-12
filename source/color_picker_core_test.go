package main

import "testing"

func TestCustomPickerHSVKnownColours(t *testing.T) {
	for _, p := range []struct {
		h, s, v float64
		want    uint32
	}{
		{0, 1, 1, 0x0000ff}, {120, 1, 1, 0x00ff00}, {240, 1, 1, 0xff0000},
		{360, 1, 1, 0x0000ff}, {0, 0, 1, 0xffffff}, {30, 1, 1, 0x0080ff},
	} {
		if got := customPickerHSVToRGB(p.h, p.s, p.v); got != p.want {
			t.Errorf("HSV %v = %#x, want %#x", p, got, p.want)
		}
	}
}
func TestCustomPickerRGBHSVAllColoursRoundtrip(t *testing.T) {
	for r := 0; r <= 255; r += 17 {
		for g := 0; g <= 255; g += 17 {
			for b := 0; b <= 255; b += 17 {
				c := uint32(r) | uint32(g)<<8 | uint32(b)<<16
				h, s, v := customPickerRGBToHSV(c)
				back := customPickerHSVToRGB(h, s, v)
				if back != c {
					t.Fatalf("roundtrip %#06x got %#06x", c, back)
				}
			}
		}
	}
}

func TestCustomPicker256GridShowsEachIndexExactlyOnce(t *testing.T) {
	seen := make(map[int]bool)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			idx := customPickerGridIndex(x, y, 16, 16)
			if idx < 0 || idx > 255 || seen[idx] {
				t.Fatalf("bad index (%d,%d)=%d", x, y, idx)
			}
			seen[idx] = true
		}
	}
	if len(seen) != 256 {
		t.Fatalf("missing indexed cells: %d", len(seen))
	}
	for _, p := range [][2]int{{-1, 0}, {16, 0}, {0, -1}, {0, 16}} {
		if customPickerGridIndex(p[0], p[1], 16, 16) != -1 {
			t.Fatalf("outside cell accepted: %v", p)
		}
	}
}
func TestCustomPickerEditingDoesNotChangeOtherIndices(t *testing.T) {
	p := make([]uint32, 256)
	for i := range p {
		p[i] = uint32(i) | uint32(255-i)<<16
	}
	before := append([]uint32(nil), p...)
	if !customPickerReplaceIndex(p, 253, 0x010203) {
		t.Fatal("failed to edit")
	}
	for i := range p {
		if i == 253 {
			if p[i] != 0x010203 {
				t.Fatalf("wrong colour %#x", p[i])
			}
		} else if p[i] != before[i] {
			t.Fatalf("modified unrelated index %d", i)
		}
	}
	if customPickerReplaceIndex(p, 256, 0xffffff) {
		t.Fatal("out-of-range palette index modified")
	}
}
