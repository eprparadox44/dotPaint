//go:build windows

package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestRasterModePreferenceResidesAlongsideTheme(t *testing.T) {
	theme, err := themePreferencePath()
	if err != nil {
		t.Skipf("user config unavailable: %v", err)
	}
	raster, err := rasterModePreferencePath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(theme) != filepath.Dir(raster) || filepath.Base(raster) != "raster_mode.txt" {
		t.Fatalf("theme=%q raster=%q", theme, raster)
	}
}

func TestRasterModeStartupPreferenceDoesNotCreateAnUndoStep(t *testing.T) {
	a := newApp()
	before := a.canvas.ClonePixels()
	a.restoreStartupRasterMode(true)
	if !a.rasterMode || a.keepObjects || !a.keepObjectsBeforeRasterMode {
		t.Fatalf("restored state raster=%v keep=%v prior=%v", a.rasterMode, a.keepObjects, a.keepObjectsBeforeRasterMode)
	}
	if a.historyIndex != 0 || len(a.historyEntries) != 1 || !a.historyEntries[0].RasterMode {
		t.Fatalf("unexpected startup history: index=%d entries=%d", a.historyIndex, len(a.historyEntries))
	}
	if a.historyUndo() || a.dirty || len(a.objects) != 0 || !bytes.Equal(before, a.canvas.Pix) {
		t.Fatal("startup restoration must not change document or create undoable action")
	}
	a.leaveRasterMode()
	if a.rasterMode || !a.keepObjects {
		t.Fatal("switching mode off after restored startup did not enable objects")
	}
}

func TestRasterModeStartupPreferenceAbsentLeavesDefault(t *testing.T) {
	a := newApp()
	a.restoreStartupRasterMode(false)
	if a.rasterMode || !a.keepObjects || len(a.historyEntries) != 1 || a.historyIndex != 0 {
		t.Fatalf("default changed: raster=%v keep=%v history=%d", a.rasterMode, a.keepObjects, len(a.historyEntries))
	}
}
