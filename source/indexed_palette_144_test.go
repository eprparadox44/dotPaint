package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestIndexed256DefaultAndQuantization(t *testing.T) {
	base := make([]uint32, 16)
	for i := range base {
		base[i] = indexedColorRef(byte(i), byte(i+1), byte(i+2))
	}
	pal := defaultIndexed256Palette(base)
	if len(pal) != 256 {
		t.Fatalf("got %d", len(pal))
	}
	for i := 0; i < 16; i++ {
		if pal[i] != base[i] {
			t.Fatalf("base changed at %d", i)
		}
	}
	pal[199] = indexedColorRef(145, 166, 187)
	if i := nearestIndexedColor(145, 166, 187, pal); i != 199 {
		t.Fatalf("expected 199, got %d", i)
	}
	before := []byte{12, 23, 34, 255}
	dst := append([]byte(nil), before...)
	compositeIndexedBGRA(dst, []byte{187, 166, 145, 0}, pal)
	if !bytes.Equal(dst, before) {
		t.Fatal("transparent paste overwrote destination")
	}
	compositeIndexedBGRA(dst, []byte{187, 166, 145, 255}, pal)
	if !bytes.Equal(dst, []byte{187, 166, 145, 255}) {
		t.Fatalf("opaque wrong %v", dst)
	}
	single := []uint32{indexedColorRef(100, 200, 250)}
	dst = []byte{0, 0, 0, 255}
	compositeIndexedBGRA(dst, []byte{255, 255, 255, 128}, single)
	if !bytes.Equal(dst, []byte{250, 200, 100, 255}) {
		t.Fatalf("alpha blend wrong %v", dst)
	}
}
func TestIndexed256FileRoundtripAndNoTruncate(t *testing.T) {
	pal := make([]uint32, 256)
	for i := range pal {
		pal[i] = indexedColorRef(byte(i), byte((i*7)%256), byte((i*13)%256))
	}
	for _, tc := range []struct {
		name string
		raw  []byte
	}{{"test.pal", mustJASC256(t, pal)}, {"test.dpal", mustDPAL256(t, pal)}} {
		got, err := decodeIndexedPaletteFile(tc.name, tc.raw)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(got) != 256 {
			t.Fatalf("%s truncated to %d", tc.name, len(got))
		}
		for i := range pal {
			if pal[i] != got[i] {
				t.Fatalf("%s index %d", tc.name, i)
			}
		}
	}
	raw := make([]byte, 768)
	for i, c := range pal {
		raw[i*3], raw[i*3+1], raw[i*3+2] = byte(c), byte(c>>8), byte(c>>16)
	}
	for _, name := range []string{"test.rgb", "test.act"} {
		got, err := decodeIndexedPaletteFile(name, raw)
		if err != nil || len(got) != 256 || got[249] != pal[249] {
			t.Fatalf("%s failed: %v", name, err)
		}
	}
	var gpl strings.Builder
	gpl.WriteString("GIMP Palette\nName: indexed256\nColumns: 16\n")
	for _, c := range pal {
		fmt.Fprintf(&gpl, "%d %d %d\n", byte(c), byte(c>>8), byte(c>>16))
	}
	got, err := decodeIndexedPaletteFile("test.gpl", []byte(gpl.String()))
	if err != nil || len(got) != 256 {
		t.Fatalf("gpl: %v", err)
	}
	if _, err := decodeIndexedPaletteFile("short.pal", []byte("JASC-PAL\n0100\n256\n0 0 0")); err == nil {
		t.Fatal("accepted truncated JASC")
	}
	if _, err := encodeIndexedPaletteDPALAny(pal[:17]); err == nil {
		t.Fatal("accepted palette of size 17")
	}
}
func mustJASC256(t *testing.T, p []uint32) []byte {
	t.Helper()
	b, e := encodeIndexedPaletteJASCAny(p)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func mustDPAL256(t *testing.T, p []uint32) []byte {
	t.Helper()
	b, e := encodeIndexedPaletteDPALAny(p)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestIndexed16LegacyCompatibility(t *testing.T) {
	p := indexedPaletteFromRefs([]uint32{0, 0xffffff, 0xff00ff})
	cases := []struct {
		name string
		buf  []byte
	}{{"a.pal", encodeIndexedPaletteJASC(p)}, {"a.dpal", encodeIndexedPaletteDPAL(p)}}
	for _, c := range cases {
		got, err := decodeIndexedPaletteFile(c.name, c.buf)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 16 {
			t.Fatalf("len=%d", len(got))
		}
		for i := range p {
			if p[i] != got[i] {
				t.Fatalf("%s[%d] mismatch", c.name, i)
			}
		}
	}
}
