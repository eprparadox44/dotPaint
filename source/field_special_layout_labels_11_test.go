package main

import (
	"os"
	"strings"
	"testing"
)

func TestFieldSpecialLayoutVisibleLabels11(t *testing.T) {
	data, err := os.ReadFile("field_editor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`fieldAddComboText(s.specialCombo, "평면 배치")`,
		`fieldCreateChild(s, "BUTTON", "배치 적용"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing visible UI label source: %s", want)
		}
	}
	for _, old := range []string{"특수 배치 없음", "특수 배치 적용"} {
		if strings.Contains(text, old) {
			t.Fatalf("obsolete visible label remains: %s", old)
		}
	}
}
