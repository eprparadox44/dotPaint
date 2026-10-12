package main

import (
	"os"
	"strings"
	"testing"
)

func a236Read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestA236BlackBackgroundDoesNotInvertForegroundPixels(t *testing.T) {
	formula := a236Read(t, "formula_editor_windows.go")
	refreshStart := strings.Index(formula, "func formulaRefreshPreview")
	refreshEnd := strings.Index(formula[refreshStart:], "func formulaInsertMarkup")
	if refreshStart < 0 || refreshEnd < 0 {
		t.Fatal("formula refresh block not found")
	}
	formulaRefresh := formula[refreshStart : refreshStart+refreshEnd]
	if strings.Contains(formulaRefresh, "invertClipboardImageRGB") {
		t.Fatal("formula black-background still inverts formula pixels")
	}
	for _, marker := range []string{"previewBG = rgb(18, 18, 18)", "formulaCompositePreviewOn(s.preview, bgByte)"} {
		if !strings.Contains(formula, marker) {
			t.Fatalf("formula background marker missing %q", marker)
		}
	}

	graph := a236Read(t, "function_graph_editor_windows.go")
	if strings.Contains(graph, "s.preview = invertClipboardImageRGB(s.preview)") {
		t.Fatal("graph black-background still inverts graph pixels")
	}
	for _, marker := range []string{"bgByte = 18", "formulaCompositePreviewOn(s.preview, bgByte)"} {
		if !strings.Contains(graph, marker) {
			t.Fatalf("graph background marker missing %q", marker)
		}
	}

	multidim := a236Read(t, "multidim_shape_editor_windows.go")
	for _, marker := range []string{
		"return rgb(18, 18, 18)",
		"if s != nil && multidimEditorInvertEnabled(s) && !s.insertMaskPass",
	} {
		if !strings.Contains(multidim, marker) {
			t.Fatalf("multidim background marker missing %q", marker)
		}
	}
	if strings.Contains(multidim, "DSTINVERT") {
		t.Fatal("multidim black-background still uses whole-scene RGB inversion")
	}
	if strings.Contains(multidim, "b, g, r = 255-b, 255-g, 255-r") {
		t.Fatal("multidim insertion still inverts extracted shape pixels")
	}
}

func TestA236FlipRotatePreviewUsesExactFinalRaster(t *testing.T) {
	src := a236Read(t, "flip_rotate_dialog_windows.go")
	start := strings.Index(src, "func (s *flipRotateDialogState) buildPreviewCanvas")
	end := strings.Index(src[start:], "func (s *flipRotateDialogState) refreshPreviewCache")
	if start < 0 || end < 0 {
		t.Fatal("flip/rotate preview builder not found")
	}
	body := src[start : start+end]
	if !strings.Contains(body, "return transformCanvasFlipRotate(s.source, s.options)") {
		t.Fatal("preview does not use the exact final transform raster")
	}
	if strings.Contains(body, "inverseFlipRotatePoint") {
		t.Fatal("preview still recalculates transform on scaled preview coordinates")
	}
	for _, marker := range []string{
		"scale := math.Min(float64(availW)/float64(pc.W), float64(availH)/float64(pc.H))",
		"procStretchDIBits.Call(hdc, uintptr(dx), uintptr(dy), uintptr(dw), uintptr(dh)",
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("exact-raster preview paint marker missing %q", marker)
		}
	}
}
