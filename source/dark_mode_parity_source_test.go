package main

import (
	"os"
	"strings"
	"testing"
)

func TestA227DarkModeUsesSharedMenuGeometry(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	required := []string{
		`const appVersion = "A237-DARK-LIBRARIES-HELP-ABOUT-BACKGROUND"`,
		`func (a *App) measureThemedMenuItem`,
		`func (a *App) drawThemedMenuItem`,
		`key := registerMenuDrawEntry(text, false, false)`,
		`procAppendMenuW.Call(menu, uintptr(flags|MF_OWNERDRAW)`,
		`key := registerMenuDrawEntry(text, false, true)`,
		`key := registerMenuDrawEntry(text, true, true)`,
		`dis.RcItem.Left + 30`,
		`rightReserve = 30`,
		`case WM_MENUCHAR:`,
		`handleThemedMenuChar(wparam, lparam)`,
	}
	for _, marker := range required {
		if !strings.Contains(src, marker) {
			t.Fatalf("missing A227 shared menu marker %q", marker)
		}
	}
}

func TestA232DarkModeKeepsDocumentButSyncsEditorInvert(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	required := []string{
		`canvas_unchanged=true`,
		`editor_black_background_sync=true`,
		`func (a *App) defaultNewCanvasColor() uint32`,
		`return rgb(255, 255, 255)`,
		`activeCircuitLibrary`,
		`activeAnimalLibrary`,
		`activeCustomImageLibrary`,
	}
	for _, marker := range required {
		if !strings.Contains(src, marker) {
			t.Fatalf("missing A231 document/invert marker %q", marker)
		}
	}
	forbidden := []string{
		`a.fg = rgb(255, 255, 255)`,
		`a.bg = rgb(0, 0, 0)`,
		`a.canvas.Clear(newDefault)`,
	}
	for _, marker := range forbidden {
		if strings.Contains(src, marker) {
			t.Fatalf("dark mode still changes document state: %q", marker)
		}
	}
}

func TestA232EditorInvertFollowsTheme(t *testing.T) {
	mainData, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	mainSrc := string(mainData)
	for _, marker := range []string{
		`s.invert = a.darkMode`,
		`sendMessage(s.invertCheck, BM_SETCHECK, check, 0)`,
		`editor_black_background_sync=true`,
	} {
		if !strings.Contains(mainSrc, marker) {
			t.Fatalf("missing A231 editor-invert marker %q", marker)
		}
	}

	multiData, err := os.ReadFile("multidim_shape_editor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	multiSrc := string(multiData)
	if !strings.Contains(multiSrc, `if s.app != nil && s.app.darkMode`) {
		t.Fatal("multidimensional invert is not initialized from dark mode")
	}
}

func TestA227MenuMnemonicParsing(t *testing.T) {
	cases := map[string]rune{
		"파일(&F)":           'F',
		"붙여넣기(&P)\tCtrl+V": 'P',
		"A && B(&C)":       'C',
		"다차원 도형(&D)":       'D',
	}
	for text, want := range cases {
		if got := menuMnemonic(text); got != want {
			t.Fatalf("menuMnemonic(%q)=%q want %q", text, got, want)
		}
	}
}
