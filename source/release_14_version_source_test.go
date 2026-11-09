package main

import (
	"os"
	"strings"
	"testing"
)

// Ensures that About and Help derive their user-visible version from the
// same release identifier. A future release must deliberately update the
// version contract rather than accidentally display an obsolete major label.
func TestRelease14VersionAndAboutHelpLabels(t *testing.T) {
	expected := map[string][]string{
		"main_windows.go": {`const appVersion = "2.0.0"`},
		"about_window_windows.go": {
			`fmt.Sprintf("dotPaint %s", appVersion)`,
			`fmt.Sprintf("dotPaint %s 정보", appVersion)`,
			`fmt.Sprintf("현재 빌드: %s", appVersion)`,
		},
		"help_window_windows.go": {
			`버전 정보와 2.0`,
			`dotPaint 2.0은`,
			`Windows x64`,
		},
	}
	for filename, tokens := range expected {
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		for _, token := range tokens {
			if !strings.Contains(src, token) {
				t.Errorf("%s is missing %q", filename, token)
			}
		}
	}
}
