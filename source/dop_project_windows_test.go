//go:build windows

package main

import "testing"

func TestDOPSaveTargetExistingPNGOnlyRaster(t *testing.T) {
	a := &App{filePath: "C:\\test\\original.png", objectEditingIndex: -1}
	if a.mustChooseDOPSaveTarget() {
		t.Fatal("raster-only PNG should save directly")
	}
	a.objects = []MovableObject{{Kind: ToolRectangle, Pix: []byte{0, 0, 0, 255}, SpriteW: 1, SpriteH: 1, W: 1, H: 1}}
	if !a.mustChooseDOPSaveTarget() {
		t.Fatal("editable object in PNG should prompt DOP")
	}
	a.filePath = "C:\\test\\original.dop"
	if a.mustChooseDOPSaveTarget() {
		t.Fatal("existing DOP should save directly")
	}
	a.filePath = "C:\\test\\original.png"
	a.objects[0].Rasterized = true
	if a.mustChooseDOPSaveTarget() {
		t.Fatal("pixel-only locked layer should not force DOP")
	}
}

func TestDOPSaveTargetPendingTextInPNG(t *testing.T) {
	a := &App{filePath: "C:\\test\\original.png", objectEditingIndex: -1, keepObjects: true}
	a.textTool.Active = true
	if !a.mustChooseDOPSaveTarget() {
		t.Fatal("pending editable text in a PNG requires a DOP choice")
	}
	a.keepObjects = false
	if a.mustChooseDOPSaveTarget() {
		t.Fatal("raster-only text should save existing PNG")
	}
	a.keepObjects = true
	a.filePath = ""
	if !a.mustChooseDOPSaveTarget() {
		t.Fatal("first save should choose a format")
	}
}
