package main

import (
	"os"
	"strings"
	"testing"
)

func TestWindows11FlipRotateUsesDialogFrameAndMeasuredClientSize(t *testing.T) {
	b, err := os.ReadFile("flip_rotate_dialog_windows.go")
	if err != nil {
		t.Fatalf("read flip/rotate source: %v", err)
	}
	body := string(b)
	for _, want := range []string{
		`exStyle := uintptr(WS_EX_DLGMODALFRAME)`,
		`procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&clientBounds)), style, 0, exStyle)`,
		`procCreateWindowExW.Call(exStyle`,
		`fitFlipRotateDialogClientSize(hwnd, scaled(756, scale), scaled(585, scale))`,
		`procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))`,
		`fillUIRect(hdc, client, rgb(240, 240, 240))`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("flip/rotate client-frame correction missing %q", want)
		}
	}
}

func TestWindows11FlipRotateClientFitUsesPostThemeMeasurements(t *testing.T) {
	b, err := os.ReadFile("flip_rotate_dialog_windows.go")
	if err != nil {
		t.Fatalf("read flip/rotate source: %v", err)
	}
	body := string(b)
	start := strings.Index(body, "func fitFlipRotateDialogClientSize")
	if start < 0 {
		t.Fatal("flip/rotate measured client-size helper not found")
	}
	end := strings.Index(body[start:], "func (a *App) showFlipRotateDialog")
	if end < 0 {
		t.Fatal("flip/rotate dialog entry point not found after measured client-size helper")
	}
	helper := body[start : start+end]
	for _, want := range []string{
		`procGetWindowRect.Call`,
		`procGetClientRect.Call`,
		`targetW-clientW`,
		`targetH-clientH`,
		`SWP_NOMOVE|SWP_NOZORDER|SWP_NOACTIVATE|SWP_FRAMECHANGED`,
	} {
		if !strings.Contains(helper, want) {
			t.Fatalf("flip/rotate measured client-size helper missing %q", want)
		}
	}

	theme := strings.Index(body, "applyDarkThemeToWindow(hwnd, a.darkMode)")
	fit := strings.Index(body, "fitFlipRotateDialogClientSize(hwnd, scaled(756, scale), scaled(585, scale))")
	if theme < 0 || fit < 0 || fit <= theme {
		t.Fatal("client size must be corrected after the native light/dark frame is applied")
	}
}
