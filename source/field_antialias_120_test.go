package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestFieldSixByPremultipliedResolveKeepsCoverageAndColour120(t *testing.T) {
	hi := fieldNewRasterAtScale(6, 6, 6)
	hi.blend(0, 0, 50, 105, 215, 1)
	lo := fieldDownsampleRasterScale(hi, 1, 1, 6)
	if lo.W != 1 || lo.H != 1 || lo.SampleScale != 1 {
		t.Fatalf("unexpected resolved raster: %dx%d scale=%d", lo.W, lo.H, lo.SampleScale)
	}
	if got := lo.Pix[3]; got != 7 { // round(255 / 36)
		t.Fatalf("one-of-36 coverage alpha=%d want 7", got)
	}
	if got := lo.Pix[:3]; !bytes.Equal(got, []byte{215, 105, 50}) {
		t.Fatalf("straight source colour changed: BGRA=%v", got)
	}
	if !lo.Mask[0] {
		t.Fatal("resolved covered pixel lost its mask")
	}
}

func TestFieldSixByContinuousStrokeHasNoRowGaps120(t *testing.T) {
	hi := fieldNewRasterAtScale(60, 60, 6)
	hi.line(29, 7, 31, 53, 50, 105, 215, 1, 1)
	lo := fieldDownsampleRasterScale(hi, 10, 10, 6)
	for y := 2; y <= 7; y++ {
		alpha := 0
		for x := 0; x < lo.W; x++ {
			alpha += int(lo.Pix[(y*lo.W+x)*4+3])
		}
		if alpha == 0 {
			t.Fatalf("continuous supersampled stroke left a blank destination row at y=%d", y)
		}
	}
}

func TestFieldEditorNoLongerForces3DEquipotentialAAOff120(t *testing.T) {
	source, err := os.ReadFile("field_editor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, "previewScene.Antialias = false") {
		t.Fatal("3-D equipotential preview still overrides the user's smooth setting")
	}
	if !strings.Contains(text, `"부드럽게 (6×)"`) {
		t.Fatal("field editor does not expose the real 6x smooth mode")
	}
}

func TestFieldSmoothToggleSelectsDistinctRasterScales120(t *testing.T) {
	scene := fieldDefaultScene()
	scene.Antialias = false
	if got := fieldRenderSampleScale(scene); got != 1 {
		t.Fatalf("smooth-off sample scale=%d want 1", got)
	}
	scene.Antialias = true
	if got := fieldRenderSampleScale(scene); got != 6 {
		t.Fatalf("smooth-on sample scale=%d want 6", got)
	}
}

func TestField3DEquipotentialSmoothToggleChangesRenderedPixels120(t *testing.T) {
	scene := fieldDefaultScene()
	scene.Dimension = fieldDimension3D
	scene.Display = fieldDisplayEquipotential
	charge := fieldDefaultObject(fieldObjectSphere)
	charge.Material = fieldMaterialInsulator
	charge.Charge = 2
	charge.X, charge.Z = -.35, .2
	scene.Objects = []fieldObject{charge}

	scene.Antialias = false
	pixel := fieldRenderForBackground(scene, 96, 72, 0, false)
	scene.Antialias = true
	smooth := fieldRenderForBackground(scene, 96, 72, 0, false)
	if pixel.W != smooth.W || pixel.H != smooth.H || len(pixel.Pix) != len(smooth.Pix) {
		t.Fatalf("toggle changed output dimensions: pixel=%dx%d smooth=%dx%d", pixel.W, pixel.H, smooth.W, smooth.H)
	}
	if bytes.Equal(pixel.Pix, smooth.Pix) {
		t.Fatal("3-D equipotential smooth toggle still produces byte-identical pixels")
	}
}
