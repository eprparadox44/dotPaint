package main

import (
	"os"
	"strings"
	"testing"
)

func Test143IndexedPaletteDockResizeAndDialogWiring(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, marker := range []string{
		`const appVersion = "1.3.7"`,
		`x = toolboxWidth`,
		`return defWindowProc(hwnd, msg, wparam, lparam)`,
		`func (a *App) paletteColorArea`,
		`a.paletteTransparentArea(pal)`,
		`func (a *App) indexedPaletteActive() bool`,
		`colorDialogInitPaletteFooter(hwnd)`,
		`colorDialogHandlePaletteCommand(hwnd, uintptr(loword(wparam)))`,
		`colorDialogDrawPaletteSwatch`,
		`a.toggleIndexedPaletteMode()`,
		`a.loadIndexedPaletteInteractive()`,
		`a.saveIndexedPaletteInteractive()`,
		`IndexedPalettePaste: a.dejavu != nil || a.indexedMode`,
	} {
		if !strings.Contains(s, marker) {
			t.Errorf("missing wiring: %q", marker)
		}
	}
	w, err := os.ReadFile("indexed_palette_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	sw := string(w)
	for _, marker := range []string{`remapIndexedBGRA(a.undo.undo[i].Pix`, `remapIndexedBGRA(a.undo.redo[i].Pix`, `procSetWindowPos.Call(hwnd`, `paletteFooterSwatchBaseID`, `len(raw)-48`, `existing && messageBox`} {
		if !strings.Contains(sw, marker) {
			t.Errorf("missing palette guard: %q", marker)
		}
	}
}
