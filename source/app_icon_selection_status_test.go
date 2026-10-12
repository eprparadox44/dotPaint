package main

import (
	"encoding/binary"
	"image/png"
	"os"
	"strings"
	"testing"
)

func TestA226AppIconSourceAndICO(t *testing.T) {
	f, err := os.Open("dotpaint_app_icon.png")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 256 || img.Bounds().Dy() != 255 {
		t.Fatalf("app icon PNG size=%dx%d want 256x255", img.Bounds().Dx(), img.Bounds().Dy())
	}
	ico, err := os.ReadFile("dotpaint.ico")
	if err != nil {
		t.Fatal(err)
	}
	if len(ico) < 6 || binary.LittleEndian.Uint16(ico[2:4]) != 1 {
		t.Fatal("dotpaint.ico is not an icon directory")
	}
	if got := int(binary.LittleEndian.Uint16(ico[4:6])); got != 10 {
		t.Fatalf("ICO image count=%d want 10", got)
	}
}

func TestA226SelectionSizeHasDedicatedStatusField(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, marker := range []string{
		`const appVersion = "A237-DARK-LIBRARIES-HELP-ABOUT-BACKGROUND"`,
		`func (a *App) selectionGeometryStatusText() string`,
		`return fmt.Sprintf("선택 %d × %d px", w, h)`,
		`return fmt.Sprintf("선택 %d × %d px", a.selection.W, a.selection.H)`,
		`geometry := a.selectionGeometryStatusText()`,
		`procLoadImageW.Call(hinst, IDI_DOTPAINT, IMAGE_ICON, 32, 32, LR_SHARED)`,
		`procSendMessageW.Call(hwnd, WM_SETICON, ICON_BIG, iconBig)`,
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("missing A226 marker %q", marker)
		}
	}
}
