package main

import (
	"os"
	"strings"
	"testing"
)

func TestFieldEditor135GravityAndBlackHoleControls(t *testing.T) {
	data, err := os.ReadFile("field_editor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, marker := range []string{
		`fieldAddComboText(s.displayCombo, "중력 퍼텐셜선 / 퍼텐셜면")`,
		`fieldAddComboText(s.displayCombo, "중력장선")`,
		`fieldAddComboText(s.displayCombo, "중력장 (격자 벡터)")`,
		`fieldGravityPaletteKinds`,
		`fieldObjectSolidMassSphere`,
		`fieldObjectHollowMassSphere`,
		`fieldObjectMassPlane`,
		`fieldObjectMassRing`,
		`fieldIDSpin`,
		`"회전속도"`,
		`gravityObjects`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("field editor missing gravity-domain marker %q", marker)
		}
	}
}
