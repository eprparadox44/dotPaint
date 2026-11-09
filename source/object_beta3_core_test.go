package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestBeta3RasterizedSpriteCompositesWithoutBeingSelectable(t *testing.T) {
	red := MovableObject{Kind: ToolRectangle, X: 1, Y: 1, W: 2, H: 2, SpriteW: 2, SpriteH: 2, Pix: bytes.Repeat([]byte{0, 0, 255, 255}, 4)}
	blue := MovableObject{Kind: ToolEllipse, X: 1, Y: 1, W: 2, H: 2, SpriteW: 2, SpriteH: 2, Pix: bytes.Repeat([]byte{255, 0, 0, 255}, 4)}
	objs := []MovableObject{red, blue}
	first := bytes.Repeat([]byte{255, 255, 255, 255}, 5*5)
	compositeObjectsToPixels(first, 5, 5, 0, 0, objs)
	objs[1].Rasterized = true
	second := bytes.Repeat([]byte{255, 255, 255, 255}, 5*5)
	compositeObjectsToPixels(second, 5, 5, 0, 0, objs)
	if !bytes.Equal(first, second) {
		t.Fatal("locked raster image changed displayed pixels")
	}
	if i := hitObjectBody(objs, 1, 1); i != 0 {
		t.Fatalf("locked shape must not be selected, got %d", i)
	}
	if i, _ := hitObjectAtClient(objs, 1, 0, 0, 1, 1); i != 0 {
		t.Fatalf("locked shape exposes resize handles, got %d", i)
	}
	if objectsEqual(objs, []MovableObject{red, blue}) {
		t.Fatal("rasterized state ignored by history equality")
	}
}

func TestBeta3MenuAndLocalizationSourceContract(t *testing.T) {
	srcs := map[string][]string{
		"main_windows.go":            {"keepObjects:", "a.paintObjectKeepCheckbox", "a.toggleObjectKeepAt", `appendTopLevelPopup(main, selectM, "선택(&S)")`, "a.appendObjectCommands(selectM", "WM_INITMENUPOPUP", "a.keepObjects && a.drawOperation != PaintTransparent && a.dejavu == nil"},
		"beta4_windows.go":           {"if a.keepObjects {", "a.addMovableObject(o)", "a.makeVectorObject"},
		"object_menu_windows.go":     {"objectMenuRasterize", "래스터화(&S)", "a.rasterizeSelectedObject()", "func (a *App) refreshObjectSelectMenu"},
		"object_keep_windows.go":     {"ToolRectangle", "ToolRightTriangle", "ToolEllipse", "ToolText", `localizeUI("객체 유지")`},
		"movable_objects_windows.go": {"func (a *App) rasterizeSelectedObject()", "copyCanvas := &Canvas{", "a.objects[prefix:]"},
		"help_window_windows.go":     {"2.0", "객체 유지 체크는 기본 켬", "래스터화"},
	}
	for file, needles := range srcs {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range needles {
			if !strings.Contains(string(data), n) {
				t.Errorf("%s missing %q", file, n)
			}
		}
	}
	labels := beta2UIMap(t)
	for _, key := range []string{"선택(&S)", "객체 선택(&V)\tV", "객체 유지", "래스터화(&S)", "객체 래스터화", "객체 유지 켬: 새 도형·텍스트는 이동 가능한 객체로 남습니다.", "객체 유지 끔: 새 도형·텍스트는 픽셀로 확정됩니다."} {
		if labels[key] == "" {
			t.Errorf("missing English label for %q", key)
		}
	}
}
