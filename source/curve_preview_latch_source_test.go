package main

import (
	"os"
	"strings"
	"testing"
)

func TestCurveReleasePreviewIsSynchronouslyLatched(t *testing.T) {
	src, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	required := []string{
		"func (a *App) forceCurveReleaseRepaint(oldRect RECT, reason string)",
		"procUpdateWindow.Call(a.hwnd)",
		"CURVE preview-latch",
		"oldCurveRect := a.curvePreviewScreenRect()",
		"a.forceCurveReleaseRepaint(oldCurveRect, reason)",
	}
	for _, token := range required {
		if !strings.Contains(text, token) {
			t.Fatalf("missing curve release latch token %q", token)
		}
	}
}
