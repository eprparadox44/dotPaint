package main

import (
	"os"
	"strings"
	"testing"
)

func TestMaskedSelectionCaptureMakesOutsidePixelsTransparent(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	start := strings.Index(s, "func (a *App) captureSelectionImage() ClipboardImage")
	if start < 0 {
		t.Fatal("captureSelectionImage source block not found")
	}
	end := strings.Index(s[start:], "func (a *App) selectionMaskAllows")
	if end < 0 {
		t.Fatal("captureSelectionImage source block not found")
	}
	block := s[start : start+end]
	for _, want := range []string{
		"out.Mask = append([]bool(nil), s.Mask...)",
		"applyClipboardSelectionMask(out.Pix, out.Mask, out.W, out.H)",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("masked clipboard capture missing %q", want)
		}
	}
}
