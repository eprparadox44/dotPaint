package main

import (
	"os"
	"strings"
	"testing"
)

func TestCustomImageLibraryWindowsMarkersA223(t *testing.T) {
	data, err := os.ReadFile("custom_image_library_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	coreData, err := os.ReadFile("custom_image_library_core.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data) + "\n" + string(coreData)
	for _, marker := range []string{
		"dotPaintCustomImageLibraryA223",
		"user_images",
		"클립보드에서 저장",
		"buildTransparentClipboardPNG",
		"customImageMaxSlots",
		"customImageMaxDimension",
		"CUSTOM_IMAGE save",
		"CUSTOM_IMAGE insert",
		"CUSTOM_IMAGE delete",
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("missing custom-image marker %q", marker)
		}
	}
	mainData, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	mainSrc := string(mainData)
	if !strings.Contains(mainSrc, "a.showCustomImageLibrary()") {
		t.Fatal("custom image menu is not connected")
	}
	if strings.Contains(mainSrc, "INSERT placeholder image-custom") {
		t.Fatal("custom image placeholder must be removed")
	}
}
