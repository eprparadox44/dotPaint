package main

import (
	"os"
	"strings"
	"testing"
)

func TestLiveLanguageSwitchDoesNotRestartOrClearDocument(t *testing.T) {
	b, err := os.ReadFile("language_switch_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(b)
	for _, forbidden := range []string{"exec.Command(", "confirmLoseChanges(", "WM_CLOSE", "undo.Reset(", "canvas =", "newApp()"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("unexpected restart or document state mutation: %s", forbidden)
		}
	}
	for _, required := range []string{"a.rebuildMenuForTheme()", "dotPaintEdition = edition", "saveUILanguagePreference(edition)", "refreshLocalizedChildControls", "WM_GETFONT", "CB_GETCURSEL"} {
		if !strings.Contains(source, required) {
			t.Errorf("missing live refresh: %s", required)
		}
	}
}

func TestLanguageMenuNoLongerSaysRestart(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "언어(&L) (재시작)") {
		t.Fatal("menu still marks live switch as restart")
	}
}
