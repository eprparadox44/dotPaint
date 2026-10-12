package main

import (
	"os"
	"strings"
	"testing"
)

func TestWindows11NativeCaptionOwnsNonClientArea135(t *testing.T) {
	b, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	for _, want := range []string{
		`const appVersion = "1.2.38"`,
		`func editorHasNativeDarkCaption(hwnd uintptr) bool`,
		`DWMWA_CAPTION_COLOR`,
		`if editorHasNativeDarkCaption(hwnd) {`,
		`return 0, false`,
		`nativeDarkCaptionColorSupport`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("native dark caption policy missing %q", want)
		}
	}
}
