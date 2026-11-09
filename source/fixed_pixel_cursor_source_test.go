package main

import (
	"bytes"
	"image/png"
	"os"
	"strings"
	"testing"
)

// Keep the user's exact image untouched. A native 32x32 HCURSOR alone is
// insufficient because Windows may scale it for DPI and pointer accessibility.
func TestFixedPixelObjectPointerMatchesUserArtwork563(t *testing.T) {
	image, err := png.Decode(bytes.NewReader(objectCursorReferencePNG))
	if err != nil {
		t.Fatal(err)
	}
	if image.Bounds().Dx() != 32 || image.Bounds().Dy() != 31 {
		t.Fatalf("unexpected user PNG geometry: %v", image.Bounds())
	}
	pixels := objectPointerPixels()
	visible := 0
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			v := pixels[y*32+x]
			if y == 31 && v != 0 {
				t.Fatalf("unexpected extra bottom-row pixel %d,%d", x, y)
			}
			_, _, _, a := image.At(x, min(y, 30)).RGBA()
			if y < 31 && (v == 3) != (a >= 0x8000) {
				t.Fatalf("original cursor pixel changed at (%d,%d)", x, y)
			}
			if v == 3 {
				visible++
			} else if v != 0 {
				t.Fatalf("cursor pixel not transparent or invert at (%d,%d)=%d", x, y, v)
			}
		}
	}
	if visible != 214 {
		t.Fatalf("expected 214 raw invert pixels, got %d", visible)
	}
}

func TestFixedPixelCursorUsesPhysicalClientBlit563(t *testing.T) {
	data, err := os.ReadFile("object_fixed_pointer_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, marker := range []string{
		"fixedObjectCursorSide = 32",
		"fixedObjectCursorXOR",
		"procCreateDIBSection.Call",
		"procBitBlt.Call",
		"fixedObjectTrackLeave",
		"fixedObjectPointerInvert",
		"syncFixedObjectPointer",
		"fixedObjectPointerRestore",
		"drawFixedObjectPointerOnPaint",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("missing software cursor behavior %s", marker)
		}
	}
	for _, bad := range []string{"SetSystemCursor", "SystemParametersInfo", "SetProcessDpiAwarenessContext.Call"} {
		if strings.Contains(s, bad) {
			t.Fatalf("cursor must never mutate global pointer settings or process DPI: %s", bad)
		}
	}
	main, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(main), "WM_MOUSELEAVE") || !strings.Contains(string(main), "a.drawFixedObjectPointerOnPaint(screenDC, update)") {
		t.Fatal("WM_MOUSELEAVE and final screen-only cursor repaint required")
	}
}
