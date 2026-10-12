package main

import (
	"os"
	"strings"
	"testing"
)

func TestA219GeneralClipboardPublishesTransparentPNGAndWhiteDIB(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	start := strings.Index(s, "func (a *App) writeSystemClipboard")
	end := strings.Index(s[start:], "func (a *App) pasteClipboardImageAt")
	if start < 0 || end < 0 {
		t.Fatal("writeSystemClipboard source block not found")
	}
	block := s[start : start+end]
	for _, want := range []string{
		"buildOpaqueClipboardDIB",
		"buildTransparentClipboardPNG",
		"clipboardPNGFormat",
		"clipboardImagePNGFormat",
		"transparent_pixels",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("general clipboard path missing %q", want)
		}
	}
	if strings.Contains(block, "copy(data[40+y*stride") {
		t.Fatal("raw-alpha CF_DIB copy path returned")
	}
}
