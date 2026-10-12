package main

import (
	"os"
	"strings"
	"testing"
)

func TestFieldInteractivePreviewUsesOneByAndPreservesFinalPolicy121(t *testing.T) {
	scene := fieldDefaultScene()
	scene.Antialias = true

	fast := fieldPreviewSceneForInteraction(scene, true)
	if got := fieldRenderSampleScale(fast); got != 1 {
		t.Fatalf("interactive sample scale=%d want 1", got)
	}
	if !scene.Antialias {
		t.Fatal("interactive snapshot changed the persistent editor scene")
	}

	final := fieldPreviewSceneForInteraction(scene, false)
	if got := fieldRenderSampleScale(final); got != fieldSmoothSampleScale {
		t.Fatalf("final sample scale=%d want %d", got, fieldSmoothSampleScale)
	}

	scene.Antialias = false
	if got := fieldRenderSampleScale(fieldPreviewSceneForInteraction(scene, false)); got != 1 {
		t.Fatalf("smooth-off final sample scale=%d want 1", got)
	}
}

func TestFieldInteractiveStaleFrameMayDisplayOnlyAheadOfInteractivePending121(t *testing.T) {
	tests := []struct {
		name                       string
		result, latest             uint64
		interactive, pending, fast bool
		want                       bool
	}{
		{"exact final", 8, 8, false, false, false, true},
		{"stale interactive with newer interactive", 8, 11, true, true, true, true},
		{"stale interactive before final", 8, 11, true, true, false, false},
		{"stale final before interactive", 8, 11, false, true, true, false},
		{"stale interactive without pending", 8, 11, true, false, false, false},
		{"future serial rejected", 12, 11, true, true, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := fieldPreviewResultCurrentEnough(tc.result, tc.latest, tc.interactive, tc.pending, tc.fast); got != tc.want {
				t.Fatalf("current-enough=%v want %v", got, tc.want)
			}
		})
	}
}

func TestFieldEditorQueuesPhysicalInteractivePreview121(t *testing.T) {
	source, err := os.ReadFile("field_editor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, marker := range []string{
		`fieldQueuePreview(s, "interactive", true)`,
		`previewScene = fieldPreviewSceneForInteraction(previewScene, interactive)`,
		`fieldLaunchPreviewWorker(s, latest, pendingReason, pendingFast, nextCancel)`,
		`s.previewCancel.Cancel()`,
		`"interactive-1x"`,
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("field editor is missing interactive preview marker %q", marker)
		}
	}
}
