package main

import (
	"os"
	"strings"
	"testing"
)

func TestFieldDeleteObjectAtKeepsSelectionValid(t *testing.T) {
	objects := []fieldObject{
		fieldDefaultObject(fieldObjectSolidMassSphere),
		fieldDefaultObject(fieldObjectMassPlane),
		fieldDefaultObject(fieldObjectMassRing),
	}

	remaining, selected, ok := fieldDeleteObjectAt(objects, 1)
	if !ok {
		t.Fatal("expected middle object deletion to succeed")
	}
	if len(remaining) != 2 || selected != 1 {
		t.Fatalf("after middle deletion: len=%d selected=%d, want 2 and 1", len(remaining), selected)
	}
	if remaining[1].Kind != fieldObjectMassRing {
		t.Fatalf("remaining object shifted incorrectly: kind=%v", remaining[1].Kind)
	}

	remaining, selected, ok = fieldDeleteObjectAt(remaining, 1)
	if !ok || len(remaining) != 1 || selected != 0 {
		t.Fatalf("after last deletion: ok=%v len=%d selected=%d", ok, len(remaining), selected)
	}
	remaining, selected, ok = fieldDeleteObjectAt(remaining, 0)
	if !ok || len(remaining) != 0 || selected != -1 {
		t.Fatalf("after final deletion: ok=%v len=%d selected=%d", ok, len(remaining), selected)
	}
}

func TestFieldEditorEmptySceneClearsStablePreviewSource(t *testing.T) {
	data, err := os.ReadFile("field_editor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	required := []string{
		"func fieldResetPreviewForEmptyScene",
		"s.stablePreview = fieldRaster{}",
		"s.previewSerial++",
		"if fieldResetPreviewForEmptyScene(s, \"refresh\")",
		"FIELD_PREVIEW empty-scene-reset",
	}
	for _, marker := range required {
		if !strings.Contains(source, marker) {
			t.Fatalf("missing empty-scene recovery marker %q", marker)
		}
	}
}
