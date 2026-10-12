package main

import (
	"bytes"
	"testing"
)

func TestIndexedPaletteIO(t *testing.T) {
	var p indexed16Palette
	for i := range p {
		p[i] = indexedColorRef(byte(i*14), byte(i*11), byte(i*8))
	}
	for _, typ := range []struct {
		name string
		data []byte
	}{{"a.dpal", encodeIndexedPaletteDPAL(p)}, {"a.pal", encodeIndexedPaletteJASC(p)}} {
		got, err := decodeExternalIndexedPalette(typ.name, typ.data)
		if err != nil || p != got {
			t.Fatalf("%s mismatch: %v", typ.name, err)
		}
	}
	if _, err := decodeExternalIndexedPalette("a.pal", []byte("JASC-PAL\n0100\n4\n")); err == nil {
		t.Fatal("non-16 palette accepted")
	}
	dat := make([]byte, 123)
	for i := 0; i < 48; i++ {
		dat[len(dat)-48+i] = byte(i % 64)
	}
	got, err := decodeExternalIndexedPalette("DEJAVU.DAT", dat)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != indexedColorRef(0, 4, 8) {
		t.Fatalf("DAT tail wrong: %x", got[0])
	}
	if !bytes.Equal(dat[:75], make([]byte, 75)) {
		t.Fatal("input altered")
	}
	if _, err := decodeExternalIndexedPalette("DEJAVU.DAT", dat[:47]); err == nil {
		t.Fatal("short DAT accepted")
	}
}
