package main

import (
	"os"
	"strings"
	"testing"
)

func TestA224HiddenSentinelCannotOwnToolbarStyle(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	checks := []string{
		`const appVersion = "A237-DARK-LIBRARIES-HELP-ABOUT-BACKGROUND"`,
		`sentinelStyle := textLayoutSentinelStyle(richStyles, base, a.activeTextInsertionStyle(), a.textTool.InsertionStyleActive)`,
		`setRichCharFormat(a.textTool.LayoutHwnd, richUnits, richUnits+1, sentinelStyle, a.fg, dpi)`,
		`sendMessage(a.textTool.LayoutHwnd, EM_SETCHARFORMAT, SCF_SELECTION`,
		`TEXT layout final-style`,
	}
	for _, want := range checks {
		if !strings.Contains(s, want) {
			t.Fatalf("A224 final-character guard missing %q", want)
		}
	}
}

func TestA224SentinelPolicyPrefersLastRealStyle(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	start := strings.Index(s, "func textLayoutSentinelStyle(")
	if start < 0 {
		t.Fatal("textLayoutSentinelStyle missing")
	}
	end := strings.Index(s[start:], "\n}\n")
	if end < 0 {
		t.Fatal("textLayoutSentinelStyle body truncated")
	}
	body := s[start : start+end+3]
	lastStyle := `if len(realStyles) > 0 {
		return realStyles[len(realStyles)-1]
	}`
	if !strings.Contains(body, lastStyle) {
		t.Fatalf("sentinel must prefer the final real character style; body=%s", body)
	}
}
