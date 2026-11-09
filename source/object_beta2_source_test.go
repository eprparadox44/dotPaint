package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"image/png"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func beta2ParseSource(t *testing.T, path string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func beta2UIMap(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	ast.Inspect(beta2ParseSource(t, "ui_language_windows.go"), func(n ast.Node) bool {
		v, ok := n.(*ast.ValueSpec)
		if !ok || len(v.Names) == 0 || v.Names[0].Name != "uiTranslations" {
			return true
		}
		lit := v.Values[0].(*ast.CompositeLit)
		for _, e := range lit.Elts {
			kv := e.(*ast.KeyValueExpr)
			k, _ := strconv.Unquote(kv.Key.(*ast.BasicLit).Value)
			val, _ := strconv.Unquote(kv.Value.(*ast.BasicLit).Value)
			if _, exists := out[k]; exists {
				t.Fatalf("duplicate UI translation %q", k)
			}
			out[k] = val
		}
		return false
	})
	return out
}

func containsHangul(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return unicode.In(r, unicode.Hangul) })
}

// This tests four data/configuration combinations. It deliberately does not
// claim to exercise native Windows popup tracking, focus or IME input.
func TestBeta2KoreanEnglishLightDarkDataMatrix(t *testing.T) {
	translations := beta2UIMap(t)
	var labels []string
	for _, path := range []string{"object_menu_windows.go", "object_view_windows.go", "movable_objects_windows.go", "beta4_windows.go", "object_outline_windows.go", "object_keep_windows.go"} {
		ast.Inspect(beta2ParseSource(t, path), func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, _ := strconv.Unquote(lit.Value)
			if containsHangul(s) {
				labels = append(labels, s)
			}
			return true
		})
	}
	labels = append(labels, "객체 선택", "객체 선택 (V)", "%d. 버전 정보와 2.0.0")
	for _, language := range []string{"ko", "en"} {
		for theme := 0; theme < 2; theme++ {
			t.Run(fmt.Sprintf("%s_theme%d", language, theme), func(t *testing.T) {
				if objectToolIcons[theme][4*objectIconW+6] == objectArtTransparent {
					t.Fatal("reference icon missing")
				}
				for _, ko := range labels {
					value := ko
					if language == "en" {
						var ok bool
						value, ok = translations[ko]
						if !ok {
							t.Errorf("untranslated object label %q", ko)
							continue
						}
					}
					if value == "" || (language == "en" && containsHangul(value)) {
						t.Errorf("language=%s theme=%d label=%q", language, theme, value)
					}
				}
			})
		}
	}
}

func TestBeta2ToolbarOrderAndResourceLifecycle(t *testing.T) {
	var names []string
	ast.Inspect(beta2ParseSource(t, "main_windows.go"), func(n ast.Node) bool {
		v, ok := n.(*ast.ValueSpec)
		if !ok || len(v.Names) == 0 || v.Names[0].Name != "visibleTools" {
			return true
		}
		for _, e := range v.Values[0].(*ast.CompositeLit).Elts {
			names = append(names, e.(*ast.Ident).Name)
		}
		return false
	})
	for i, s := range names {
		if s == "ToolObjectSelect" {
			if i == 0 || i+1 >= len(names) || names[i-1] != "ToolEyedropper" || names[i+1] != "ToolEraser" {
				t.Fatal("object tool is not between eyedropper and eraser")
			}
			if ToolEraser != 4 || ToolEyedropper != 6 || ToolArrow != 17 {
				t.Fatal("legacy tool IDs changed")
			}
			return
		}
	}
	t.Fatal("Object Selection not in toolbar")
}

func TestBeta2NativePopupCaptureAndThemeWiring(t *testing.T) {
	read := func(path string) string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	menu := read("object_menu_windows.go")
	for _, text := range []string{"a.applyDarkMenuBackground(m)", "appendPopup(m, opacity", "appendPopup(m, order", "MF_GRAYED", "TPM_RETURNCMD", "forgetObjectPopupMenus", "a.objectArmed = objectArmMove"} {
		if !strings.Contains(menu, text) {
			t.Errorf("missing menu contract %q", text)
		}
	}
	for _, unsafeDrag := range []string{"SendInput", "mouse_event", "WM_LBUTTONDOWN"} {
		if strings.Contains(menu, unsafeDrag) {
			t.Error("popup synthesizes a mouse press")
		}
	}
	main := read("main_windows.go")
	if !strings.Contains(main, "if right && a.objectContextPending") {
		t.Fatal("context menu must open on right release")
	}
	moveStart := strings.Index(main, "func (a *App) onMouseMove(")
	moveEnd := strings.Index(main[moveStart:], "func (a *App) onMouseUp(")
	if strings.Contains(main[moveStart:moveStart+moveEnd], "updateObjectToolTooltip") {
		t.Fatal("tooltip layout added to every drawing mouse move")
	}
	obj := read("movable_objects_windows.go")
	if !strings.Contains(obj, "if a.hwnd != 0 && a.ownsMouseCapture()") {
		t.Fatal("capture cancellation can release another window")
	}
}

func TestBeta2ClipboardPNGRespectsOrderOpacityAndNoOverlay(t *testing.T) {
	base := bytes.Repeat([]byte{255, 255, 255, 255}, 10*10)
	objects := []MovableObject{testObject(ToolRectangle, 2, 2, 5, 5, 0, 0, 255, 255), testObject(ToolText, 3, 3, 4, 4, 255, 0, 0, 255)}
	objects[1].Transparency = 50
	compositeObjectsToPixels(base, 10, 10, 0, 0, objects)
	encoded, err := buildTransparentClipboardPNG(10, 10, base)
	if err != nil {
		t.Fatal(err)
	}
	im, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := im.At(3, 3).RGBA()
	if r>>8 != 127 || g != 0 || b>>8 != 128 || a>>8 != 255 {
		t.Fatalf("wrong composition RGBA %d,%d,%d,%d", r>>8, g>>8, b>>8, a>>8)
	}
	r, g, b, _ = im.At(1, 1).RGBA()
	if r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatal("editing handle leaked into PNG")
	}
}
