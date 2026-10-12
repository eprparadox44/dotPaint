package main

import (
	"os"
	"strings"
	"testing"
)

func TestFieldEditor132PreviewDensityCapMarkers(t *testing.T) {
	data, err := os.ReadFile("field_editor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, marker := range []string{
		`func fieldPreviewDensityCap(display fieldDisplayMode) int {`,
		`case fieldDisplayElectricVectors, fieldDisplayMagneticVectors, fieldDisplayGravityVectors:`,
		`return 24`,
		`fieldSetText(s.densityLabel, "격자 밀도 (최대 2×)")`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("field editor missing 1.2.32 marker %q", marker)
		}
	}
}
