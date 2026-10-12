package main

import (
	"os"
	"strings"
	"testing"
)

func TestRectSelectionWindowsGestureMarkers(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, marker := range []string{
		"pendingRectSelection",
		"armPendingRectSelection",
		"activatePendingRectSelection",
		"rectSelectionDragActivated",
		"SELECTION rect click-cancel",
		"SELECTION rect outside-canvas press",
		"a.tool == ToolRectSelect && !right && a.selection.Active && !insideExisting && !additive",
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("missing rectangular-selection gesture marker %q", marker)
		}
	}
}

func TestBlackHoleSpinControlLimitedToCurvatureGrid(t *testing.T) {
	blackHoleData, err := os.ReadFile("multidim_blackhole_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	editorData, err := os.ReadFile("multidim_shape_editor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	blackHoleSrc := string(blackHoleData)
	editorSrc := string(editorData)
	for _, marker := range []string{
		"multidimEditorBlackHoleMode(s) != mdBlackHoleModeCurvatureGrid",
		"return 0",
	} {
		if !strings.Contains(blackHoleSrc, marker) {
			t.Fatalf("missing fixed photorealistic black-hole spin marker %q", marker)
		}
	}
	for _, marker := range []string{
		"curvatureControls := multidimEditorBlackHoleMode(s) == mdBlackHoleModeCurvatureGrid",
		"multidimEditorSetDimensionEnabled(s.heightLabel, s.heightSlider, s.heightValueLabel, curvatureControls)",
		"실제 블랙홀 그림에서는 비활성",
	} {
		if !strings.Contains(editorSrc, marker) {
			t.Fatalf("missing black-hole slider disable marker %q", marker)
		}
	}
}
