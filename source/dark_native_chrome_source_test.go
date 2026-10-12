package main

import (
	"os"
	"strings"
	"testing"
)

func TestA228NativeDarkChromeHooks(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, marker := range []string{
		`const appVersion = "A237-DARK-LIBRARIES-HELP-ABOUT-BACKGROUND"`,
		`nativeAllowDarkModeForWindow`,
		`nativeSetPreferredAppMode`,
		`procGetProcAddress.Call(module, 133)`,
		`procGetProcAddress.Call(module, 135)`,
		`DWMWA_CAPTION_COLOR`,
		`DWMWA_TEXT_COLOR`,
		`DWMWA_BORDER_COLOR`,
		`DarkMode_Explorer`,
		`DarkMode_CFD`,
		`SWP_FRAMECHANGED`,
		`initNativeDarkModeSupport()`,
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("missing A228 native dark marker %q", marker)
		}
	}
}
