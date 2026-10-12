package main

import (
	"os"
	"strings"
	"testing"
)

func TestColorDialogUsesFlatButtonsInBothThemesAndDarkPaletteOnlyWhenNeeded(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, marker := range []string{
		`CC_ENABLEHOOK = 0x00000010`,
		`func colorDialogHookProc`,
		`case WM_INITDIALOG:`,
		`procEnumChildWindows.Call(hwnd, colorDialogConfigureChildCallback, 1)`,
		`flatStyle := ownerDrawPushButtonStyle(class, style)`,
		`procSetWindowLongW.Call(hwnd, uintptr(GWL_STYLE), flatStyle)`,
		`case WM_DRAWITEM:`,
		`drawThemedOwnerDrawPushButton`,
		`case WM_ERASEBKGND:`,
		`WM_CTLCOLORSTATIC, WM_CTLCOLORBTN, WM_CTLCOLORSCROLLBAR, WM_CTLCOLORDLG`,
		`func refreshColorDialogTheme`,
		`procSetWindowTheme.Call(hwnd, uintptr(unsafe.Pointer(empty)), uintptr(unsafe.Pointer(empty)))`,
		`colorDialogDarkRefreshTimerID`,
		`colorDialogDarkLateRefreshTimerID`,
		`case WM_TIMER:`,
		`refreshColorDialogTheme(hwnd)`,
		`Flags:        CC_RGBINIT | CC_FULLOPEN | CC_ENABLEHOOK`,
		`LpfnHook:     colorDialogHookCallback`,
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("missing dark colour-dialog marker %q", marker)
		}
	}
}

func TestFormulaSpecialButtonsUseSharedFlatSquareSurface(t *testing.T) {
	data, err := os.ReadFile("formula_editor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, marker := range []string{
		`paintThemedOwnerDrawPushButtonBase(dis)`,
		`paintThemedOwnerDrawPushButtonFocus(dis, focusColor)`,
		`formulaDrawScriptsCategoryButton`,
		`formulaDrawVectorCategoryButton`,
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("formula special-button flat style missing %q", marker)
		}
	}
	if strings.Contains(src, `DrawFrameControl`) {
		t.Fatal("formula special buttons still use the classic raised frame")
	}
}

func TestColorDialogKeepsNativeColourEngineAndLightPath(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, marker := range []string{
		`procChooseColorW.Call`,
		`LpCustColors: &a.customColors[0]`,
		`Flags:        CC_RGBINIT | CC_FULLOPEN | CC_ENABLEHOOK`,
		`if r != 0 {`,
		`a.fg = chosenColor`,
		`chosenColor := cc.RgbResult`,
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("native ChooseColor behaviour changed or missing %q", marker)
		}
	}
}
