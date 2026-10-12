package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"math/rand"
	"testing"
)

func syntheticDejavuIMG(t *testing.T) ([]byte, []byte) {
	t.Helper()
	r := rand.New(rand.NewSource(0xD3A4A))
	indices := make([]byte, dejavuIMGTileCount*dejavuIMGTilePixels)
	data := make([]byte, dejavuIMGExpectedSize)
	for tile := 0; tile < dejavuIMGTileCount; tile++ {
		off := tile * dejavuIMGRecordSize
		copy(data[off:off+4], dejavuIMGRecordHeader[:])
		data[off+dejavuIMGTrailerOffset] = byte(tile ^ 0x5a)
		data[off+dejavuIMGTrailerOffset+1] = byte(tile ^ 0xa5)
		idx := indices[tile*dejavuIMGTilePixels : (tile+1)*dejavuIMGTilePixels]
		for i := range idx {
			idx[i] = byte(r.Intn(16))
		}
		if err := encodeDejavuTileIntoRecord(data[off:off+dejavuIMGRecordSize], idx); err != nil {
			t.Fatal(err)
		}
	}
	return data, indices
}

func TestDejavuIMGFullDecodeReencodeBitExact(t *testing.T) {
	original, wantIndices := syntheticDejavuIMG(t)
	gotIndices, err := decodeDejavuAllIndices(original)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotIndices, wantIndices) {
		t.Fatal("decoded indices differ")
	}
	encoded, err := encodeDejavuAllIndices(original, gotIndices)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, original) {
		a := sha256.Sum256(original)
		b := sha256.Sum256(encoded)
		t.Fatalf("round trip is not bit exact: original=%s encoded=%s", hex.EncodeToString(a[:]), hex.EncodeToString(b[:]))
	}
}

func TestDejavuIMGSingleTileKeepsHeaderTrailerAndOtherRecords(t *testing.T) {
	original, indices := syntheticDejavuIMG(t)
	tile := 73
	changed := append([]byte(nil), original...)
	tileIdx := append([]byte(nil), indices[tile*dejavuIMGTilePixels:(tile+1)*dejavuIMGTilePixels]...)
	tileIdx[0] ^= 0x0f
	rec := changed[tile*dejavuIMGRecordSize : (tile+1)*dejavuIMGRecordSize]
	if err := encodeDejavuTileIntoRecord(rec, tileIdx); err != nil {
		t.Fatal(err)
	}
	for i := range original {
		if original[i] == changed[i] {
			continue
		}
		recStart := tile * dejavuIMGRecordSize
		if i < recStart+dejavuIMGPixelOffset || i >= recStart+dejavuIMGTrailerOffset {
			t.Fatalf("byte outside tile pixel payload changed at offset %d", i)
		}
	}
	recStart := tile * dejavuIMGRecordSize
	if !bytes.Equal(changed[recStart:recStart+4], original[recStart:recStart+4]) {
		t.Fatal("tile header changed")
	}
	if !bytes.Equal(changed[recStart+dejavuIMGTrailerOffset:recStart+dejavuIMGRecordSize], original[recStart+dejavuIMGTrailerOffset:recStart+dejavuIMGRecordSize]) {
		t.Fatal("tile trailer changed")
	}
}

func TestDejavuTileSheetOrder12x12(t *testing.T) {
	var p dejavuPalette
	for i := 0; i < 16; i++ {
		p.RGB[i] = [3]byte{byte(i * 16), byte(255 - i*16), byte(i * 7)}
	}
	indices := make([]byte, dejavuIMGTileCount*dejavuIMGTilePixels)
	for tile := 0; tile < dejavuIMGTileCount; tile++ {
		fill := byte(tile % 16)
		for i := 0; i < dejavuIMGTilePixels; i++ {
			indices[tile*dejavuIMGTilePixels+i] = fill
		}
	}
	sheet, err := dejavuIndicesToSheet(indices, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, tile := range []int{0, 1, 11, 12, 67, 73, 74, 75, 80, 81, 143} {
		x := (tile%12)*24 + 12
		y := (tile/12)*24 + 12
		if got := sheet.ColorIndexAt(x, y); got != byte(tile%16) {
			t.Fatalf("tile %d mapped to wrong color index: %d", tile, got)
		}
	}
}

func TestDejavuDATPaletteUsesLast48BytesAndVGAExpansion(t *testing.T) {
	data := make([]byte, 100)
	for i := 0; i < 48; i++ {
		data[len(data)-48+i] = byte(i % 64)
	}
	p, err := decodeDejavuPaletteDAT(data)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 16; i++ {
		for c := 0; c < 3; c++ {
			v6 := byte((i*3 + c) % 64)
			want := (v6 << 2) | (v6 >> 4)
			if p.RGB[i][c] != want {
				t.Fatalf("palette[%d][%d]=%d want=%d", i, c, p.RGB[i][c], want)
			}
		}
	}
}

func TestDejavuTransparentImportUsesExplicitIndex(t *testing.T) {
	var p dejavuPalette
	p.RGB[3] = [3]byte{10, 20, 30}
	p.RGB[5] = [3]byte{200, 210, 220}
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 250, G: 0, B: 0, A: 0})
	img.SetNRGBA(1, 0, color.NRGBA{R: 200, G: 210, B: 220, A: 255})
	idx := dejavuImageToIndices(img, 2, 1, p, 3)
	if idx[0] != 3 {
		t.Fatalf("transparent pixel index=%d want=3", idx[0])
	}
	if idx[1] != 5 {
		t.Fatalf("opaque nearest index=%d want=5", idx[1])
	}
}
