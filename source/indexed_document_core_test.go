package main

import "testing"

func Test131Quantize256DoesNotTruncateTo16PlusAlpha(t *testing.T) {
	pal := make([]uint32, 256)
	for i := range pal {
		pal[i] = indexedColorRef(byte(i), byte(i), byte(i))
	}
	pix := []byte{220, 220, 220, 255, 19, 19, 19, 255, 19, 19, 19, 0, 220, 220, 220, 99}
	indices := quantizeIndexedBGRA(pix, pal, 256)
	if len(indices) != 4 || indices[0] != 220 || indices[1] != 19 || indices[3] != 220 {
		t.Fatalf("256 indices: %v", indices)
	}
	if pix[8] != 19 || pix[11] != 0 || pix[15] != 99 {
		t.Fatalf("alpha/transparency modified: %v", pix)
	}
	pix2 := []byte{220, 220, 220, 255}
	ix2 := quantizeIndexedBGRA(pix2, pal, 16)
	if ix2[0] != 15 {
		t.Fatalf("16 colour quantization unexpected %v", ix2)
	}
}
func Test131PaletteRecolorAndHistoryPreservesDuplicateRGBIndices(t *testing.T) {
	pal := make([]uint32, 256)
	pal[201] = indexedColorRef(33, 88, 177)
	pal[202] = pal[201] // duplicate RGB, distinct real palette index
	pix := []byte{177, 88, 33, 255}
	previous := []byte{202}
	indices := indexedIndicesForSnapshot(pix, pal, 256, previous)
	if indices[0] != 202 {
		t.Fatalf("duplicate index identity collapsed: %d", indices[0])
	}
	edited := append([]uint32(nil), pal...)
	edited[202] = indexedColorRef(101, 202, 77)
	recolorIndexedBGRA(pix, indices, edited, 256)
	if pix[0] != 77 || pix[1] != 202 || pix[2] != 101 {
		t.Fatalf("palette slot recolor failed: %v", pix)
	}
	restoreIndexedSnapshot(pix, previous, pal)
	if pix[0] != 177 || pix[1] != 88 || pix[2] != 33 || indices[0] != 202 {
		t.Fatalf("history restore failed %v %v", pix, indices)
	}
}
func Test131ReducedActiveCountKeepsFull256EntryTable(t *testing.T) {
	pal := make([]uint32, 256)
	pal[0] = indexedColorRef(0, 0, 0)
	pal[16] = indexedColorRef(0, 0, 255)
	pal[200] = indexedColorRef(255, 0, 0)
	pix := []byte{255, 0, 0, 255}
	indices := quantizeIndexedBGRA(pix, pal, 17)
	if len(pal) != 256 || indices[0] != 16 {
		t.Fatalf("table capacity or count invalid: %v size %d", indices, len(pal))
	}
	pix2 := []byte{255, 0, 0, 255}
	indices = quantizeIndexedBGRA(pix2, pal, 16)
	if indices[0] == 16 || indices[0] >= 16 {
		t.Fatalf("16 active limit ignored: %v", indices)
	}
	if indexedUsableCount(256, 17) != 17 || indexedUsableCount(16, 99) != 16 {
		t.Fatal("active count clamp")
	}
}

func Test131ClipboardCompositingUsesActiveCountAndPreservesTransparency(t *testing.T) {
	pal := make([]uint32, 256)
	for i := range pal {
		pal[i] = indexedColorRef(byte(i), byte(i), byte(i))
	}
	// A full-opacity colour with index #230 must survive with 256 usable slots.
	dst := []byte{0, 0, 0, 255}
	compositeIndexedBGRA(dst, []byte{230, 230, 230, 255}, pal[:256])
	if dst[0] != 230 || dst[1] != 230 || dst[2] != 230 || dst[3] != 255 {
		t.Fatalf("256 paste unexpectedly reduced to 16 colours: %v", dst)
	}
	// Alpha zero keeps the underlying destination pixel untouched.
	copyDst := append([]byte(nil), dst...)
	compositeIndexedBGRA(dst, []byte{12, 12, 12, 0}, pal[:256])
	for i, v := range copyDst {
		if dst[i] != v {
			t.Fatalf("transparent paste modified underlying pixel: %v", dst)
		}
	}
	// A limited active count cannot select a disabled higher index.
	limited := []byte{0, 0, 0, 255}
	compositeIndexedBGRA(limited, []byte{230, 230, 230, 255}, pal[:16])
	if limited[0] != 15 {
		t.Fatalf("active 16-slot limit ignored during paste: %v", limited)
	}
}
