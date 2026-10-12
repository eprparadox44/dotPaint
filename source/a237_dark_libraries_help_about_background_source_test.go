package main

import (
	"os"
	"strings"
	"testing"
)

func a237Read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestA237MultidimRasterUsesSelectedBackground(t *testing.T) {
	src := a237Read(t, "multidim_shape_editor_windows.go")
	for _, marker := range []string{
		`func multidimPreviewBackgroundColor`,
		`rasterBackground := multidimPreviewBackgroundColor(s)`,
		`mdBlendedColorOnBackground`,
		`mdD3D11RenderFaceTriangles(label, w, h, gpuVerts, multidimPreviewBackgroundColor(s))`,
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("multidim missing %q", marker)
		}
	}
	gpu := a237Read(t, "multidim_gpu_d3d11_windows.go")
	for _, marker := range []string{
		`func mdD3D11RenderFaceTriangles(label string, w, h int, vertices []mdD3D11FaceVertex, background uint32)`,
		`float32(background&0xFF) / 255.0`,
	} {
		if !strings.Contains(gpu, marker) {
			t.Fatalf("gpu face background missing %q", marker)
		}
	}
}

func TestA237LibrariesUseForcedDarkChromeAndOwnerDrawButtons(t *testing.T) {
	for _, name := range []string{"circuit_library_windows.go", "animal_library_windows.go", "custom_image_library_windows.go"} {
		src := a237Read(t, name)
		for _, marker := range []string{
			`handleDarkEditorNonClient(hwnd, msg, wparam, lparam)`,
			`ownerDrawPushButtonStyle("BUTTON"`,
			`scheduleDarkEditorChromeRefresh(hwnd)`,
			`drawThemedOwnerDrawPushButton`,
		} {
			if !strings.Contains(src, marker) {
				t.Fatalf("%s missing %q", name, marker)
			}
		}
	}
}

func TestA237HelpBodyAndAboutLayout(t *testing.T) {
	help := a237Read(t, "help_window_windows.go")
	for _, marker := range []string{
		`EM_SETBKGNDCOLOR`, `CrTextColor: textColor`, `backgroundColor = rgb(18, 18, 18)`,
	} {
		if !strings.Contains(help, marker) {
			t.Fatalf("help missing %q", marker)
		}
	}
	about := a237Read(t, "about_window_windows.go")
	for _, marker := range []string{
		`DT_CENTER|DT_VCENTER|DT_SINGLELINE`,
		`"제작자는 프로그래머/개발자가 아닙니다."`,
		`aboutRefreshTheme`,
	} {
		if !strings.Contains(about, marker) {
			t.Fatalf("about missing %q", marker)
		}
	}
}
