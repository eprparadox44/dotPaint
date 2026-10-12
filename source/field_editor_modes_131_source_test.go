package main

import (
	"os"
	"strings"
	"testing"
)

func TestFieldEditor131ModeSeparationSourceMarkers(t *testing.T) {
	data, err := os.ReadFile("field_editor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, marker := range []string{
		`fieldAddComboText(s.displayCombo, "전기장 (격자 벡터)")`,
		`fieldAddComboText(s.displayCombo, "자기장 (격자 벡터)")`,
		`fieldElectricPaletteKinds`,
		`fieldMagneticPaletteKinds`,
		`fieldObjectFerromagnet`,
		`fieldSwitchDisplayDomain`,
		`fieldApplySpecialPointLayout`,
		`점전하: 정사면체 꼭짓점`,
		`점전하: 정팔면체 꼭짓점`,
		`점전하: 정육면체 꼭짓점`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("field editor missing 1.2.31 marker %q", marker)
		}
	}
}
