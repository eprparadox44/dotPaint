package main

import (
	"os"
	"strings"
	"testing"
)

func TestCurveToolUsesRoundNibForBaselinePreviewAndFinalCurve(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	start := strings.Index(source, "func (a *App) drawRasterCurveAt")
	if start < 0 {
		t.Fatal("drawRasterCurveAt not found")
	}
	end := strings.Index(source[start:], "func (a *App) drawRasterCurve(")
	if end < 0 {
		t.Fatal("drawRasterCurveAt end not found")
	}
	body := source[start : start+end]
	if got := strings.Count(body, "DrawRoundCurvePixelPath"); got != 2 {
		t.Fatalf("round curve renderer calls=%d want=2", got)
	}
	if strings.Contains(body, "DrawStyledLine") || strings.Contains(body, "DrawStyledPixelPath") {
		t.Fatal("curve tool still routes through square arrow-shaft renderer")
	}
}
