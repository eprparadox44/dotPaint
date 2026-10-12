package main

import (
	"os"
	"strings"
	"testing"
)

func TestThemePreferenceLoadsBeforeMainWindowAndSavesOnExit(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	mainStart := strings.Index(src, "func main()")
	if mainStart < 0 {
		t.Fatal("main function not found")
	}
	mainSrc := src[mainStart:]
	load := strings.Index(mainSrc, "loadThemePreference()")
	createWindow := strings.Index(mainSrc, "procCreateWindowExW.Call(0")
	if load < 0 || createWindow < 0 || load >= createWindow {
		t.Fatalf("saved theme must load before main window creation: load=%d create=%d", load, createWindow)
	}
	for _, marker := range []string{
		"app.darkMode = savedDarkMode",
		"saveThemePreference(a.darkMode)",
		"saveThemePreference(app.darkMode)",
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("theme persistence path missing %q", marker)
		}
	}

	windowsData, err := os.ReadFile("theme_preference_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	windowsSrc := string(windowsData)
	for _, marker := range []string{
		`filepath.Join(dir, "dotPaint", "theme.txt")`,
		"os.UserConfigDir()",
		"os.WriteFile(tmp, encodeThemePreference(dark), 0o644)",
		"os.Rename(tmp, path)",
	} {
		if !strings.Contains(windowsSrc, marker) {
			t.Fatalf("theme preference storage missing %q", marker)
		}
	}
}
