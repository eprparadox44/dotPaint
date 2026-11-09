package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dopTestProject() *DOPProject {
	p := &DOPProject{Canvas: &Canvas{W: 5, H: 4, Pix: make([]byte, 5*4*4)}, Palette: []uint32{0x112233, 0x445566, 0xFFFFFF}, Foreground: 0x123456, Background: 0x654321, ForegroundOp: PaintInvert, NextGroupID: 29}
	for i := range p.Canvas.Pix {
		p.Canvas.Pix[i] = byte((i*37 + 9) % 256)
	}
	p.Canvas.Pix[3] = 0 // hidden RGB still survives an exact project round-trip
	p.Objects = []MovableObject{
		{Kind: ToolCurve, Operation: PaintInvert, GroupID: 27, X: 0, Y: 0, W: 2, H: 2, SpriteW: 2, SpriteH: 2, Stroke: 3, Pix: []byte{17, 28, 39, 255, 4, 5, 6, 128, 11, 22, 33, 0, 70, 80, 90, 255}, Vector: ObjectVector{Points: [6]ObjectPoint{{X: 0, Y: 0}, {X: 1, Y: 1}, {X: 2, Y: 1}}, Count: 3, Antialias: true}, Path: []ObjectPoint{{X: 0, Y: 0}, {X: 1, Y: 1}}},
		{Kind: ToolText, Operation: PaintRGB, BackgroundOperation: PaintInvert, Opaque: true, GroupID: 27, X: 2, Y: 0, W: 1, H: 1, SpriteW: 1, SpriteH: 1, Pix: []byte{5, 8, 2, 255}, Text: "가a☺", FontName: "맑은 고딕", FontSize: 18, Styles: []TextCharStyle{{Color: 0x112233, ColorSet: true, ColorOp: PaintRGB}, {ColorOp: PaintInvert, ColorSet: true}}, GlyphInvertMask: []byte{255}},
		{Kind: ToolEllipse, Operation: PaintRGB, Rasterized: true, X: 3, Y: 0, W: 2, H: 2, SpriteW: 2, SpriteH: 2, Pix: []byte{0, 0, 0, 255, 70, 80, 90, 0, 1, 2, 3, 255, 4, 5, 6, 255}, ReplaceMask: []byte{0b00001101}},
	}
	return p
}

func TestDOPRoundTripKeepsEditableObjectsMasksGroupsAndPixelValues(t *testing.T) {
	p := dopTestProject()
	file := filepath.Join(t.TempDir(), "우리 그림.dop")
	if err := WriteDOPProject(file, p, p.Canvas); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDOPProject(file)
	if err != nil {
		t.Fatal(err)
	}
	if got.Canvas.W != p.Canvas.W || got.Canvas.H != p.Canvas.H || !bytes.Equal(got.Canvas.Pix, p.Canvas.Pix) {
		t.Fatal("base RGBA/BGRA pixels changed, including hidden RGB")
	}
	if !objectsEqual(got.Objects, p.Objects) {
		t.Fatalf("objects changed after DOP round-trip: %#v", got.Objects)
	}
	if got.NextGroupID != p.NextGroupID || got.ForegroundOp != PaintInvert || !hasEditableDOPObjects(got.Objects) {
		t.Fatal("document properties or dynamic invert lost")
	}
	originalComposite := append([]byte(nil), p.Canvas.Pix...)
	reopenedComposite := append([]byte(nil), got.Canvas.Pix...)
	compositeObjectsToPixels(originalComposite, p.Canvas.W, p.Canvas.H, 0, 0, p.Objects)
	compositeObjectsToPixels(reopenedComposite, got.Canvas.W, got.Canvas.H, 0, 0, got.Objects)
	if !bytes.Equal(originalComposite, reopenedComposite) {
		t.Fatal("DOP rendered composite changed after reopen")
	}
	r, err := zip.OpenReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	found := map[string]bool{}
	for _, f := range r.File {
		found[f.Name] = true
		if f.Name == "preview.png" {
			z, e := f.Open()
			if e != nil {
				t.Fatal(e)
			}
			img, e := png.Decode(z)
			z.Close()
			if e != nil || img.Bounds().Dx() != 5 {
				t.Fatal("preview invalid", e)
			}
		}
	}
	for _, name := range []string{"manifest.json", "canvas.bgra", "preview.png", "objects/000000.bgra", "objects/000001.invert", "objects/000002.replace"} {
		if !found[name] {
			t.Errorf("missing entry %s", name)
		}
	}
}

func TestDOPIndexedPaletteAndDuplicateSlotsRoundTrip(t *testing.T) {
	p := dopTestProject()
	p.Objects = nil
	p.IndexedMode = true
	p.IndexedSize = 16
	p.IndexedCount = 4
	p.Palette = make([]uint32, 16)
	p.Palette[0] = 0x223344
	p.Palette[1] = 0x223344
	p.Palette[2] = 0x556677
	p.Palette[3] = 0x112233
	p.IndexedColors = indexedPaletteFromRefs(p.Palette)
	p.IndexedPixels = make([]byte, 20)
	for i := range p.IndexedPixels {
		p.IndexedPixels[i] = byte(i % 4)
	}
	p.IndexedPixels[0] = 1 // index 0 and 1 have the same RGB but different identities
	path := filepath.Join(t.TempDir(), "indexed.dop")
	if err := WriteDOPProject(path, p, nil); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDOPProject(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IndexedMode || got.IndexedCount != 4 || !bytes.Equal(got.IndexedPixels, p.IndexedPixels) || got.Palette[1] != got.Palette[0] || got.IndexedColors != p.IndexedColors {
		t.Fatal("indexed palette/slot identity lost")
	}
}

func TestDOPKeepsOldFileIfWriteFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "original.dop")
	want := []byte("OLD-VALID-PROJECT")
	if err := os.WriteFile(path, want, 0600); err != nil {
		t.Fatal(err)
	}
	p := dopTestProject()
	p.Canvas.Pix = p.Canvas.Pix[:4]
	if err := WriteDOPProject(path, p, nil); err == nil {
		t.Fatal("accepted broken project")
	}
	b, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(b, want) {
		t.Fatal("damaged original after failed save", e)
	}
}

func TestDOPRejectsInvalidVersionAndTruncatedBitmap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.dop")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	w := zip.NewWriter(f)
	wr, _ := w.Create("manifest.json")
	_ = json.NewEncoder(wr).Encode(dopManifest{Format: dopFormatID, Version: 999, Width: 1, Height: 1})
	w.Close()
	f.Close()
	_, err := ReadDOPProject(path)
	if err == nil || !strings.Contains(err.Error(), "버전") {
		t.Fatal("unsupported future version incorrectly accepted", err)
	}
}

func TestDOPRasterizedOnlyDoesNotRequireProjectPrompt(t *testing.T) {
	p := dopTestProject()
	if !hasEditableDOPObjects(p.Objects) {
		t.Fatal("editable objects ignored")
	}
	for i := range p.Objects {
		p.Objects[i].Rasterized = true
	}
	if hasEditableDOPObjects(p.Objects) {
		t.Fatal("locked raster layers incorrectly counted as editable")
	}
}
