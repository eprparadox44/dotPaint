package main

import (
	"os"
	"strings"
	"testing"
)

// The earlier editable CBS_DROPDOWN both auto-completed from the size list
// and failed to commit when the canvas was clicked, even though static parser
// tests passed. Keep typing in a *separate* Win32 EDIT, not the native combo.
func TestTextSizeIndependentNumericEditContract(t *testing.T) {
	b, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(b)
	for _, marker := range []string{
		`SizeEditHwnd`,
		`ID_TEXT_SIZEEDIT`,
		`a.textTool.SizeEditHwnd = mk("EDIT",`,
		`a.textTool.SizeCombo = mk("COMBOBOX", "", WS_CHILD|WS_TABSTOP|WS_VSCROLL|CBS_DROPDOWNLIST`,
		`procMoveWindow.Call(a.textTool.SizeEditHwnd,`,
		`procShowWindow.Call(h, SW_SHOW)`,
		`getWindowTextString(a.textTool.SizeEditHwnd)`,
		`case ID_TEXT_SIZEEDIT:`,
		`if code == EN_CHANGE {`,
		`if code == EN_KILLFOCUS {`,
		`procPostMessageW.Call(a.hwnd, WM_TEXTSIZE_COMMIT, 0, 0)`,
		`a.commitPendingTextSize("canvas mouse down")`,
		`app.commitPendingTextSize(reason)`,
		`case WM_TEXTSIZE_COMMIT:`,
		`if !a.textTool.SizeEditPending {`,
	} {
		if !strings.Contains(code, marker) {
			t.Errorf("missing numeric edit wiring: %s", marker)
		}
	}
	if strings.Contains(code, `a.textTool.SizeCombo = mk("COMBOBOX", "", WS_CHILD|WS_TABSTOP|WS_VSCROLL|CBS_DROPDOWN|`) {
		t.Fatal("editable native combo should never handle numeric typing")
	}
}

func TestTextSizeEditChangeDoesNotMutateCaretOrFormatting(t *testing.T) {
	b, _ := os.ReadFile("main_windows.go")
	s := string(b)
	start := strings.Index(s, "case ID_TEXT_SIZEEDIT:")
	if start < 0 {
		t.Fatal("size edit dispatch missing")
	}
	stop := strings.Index(s[start:], "if code == EN_KILLFOCUS {")
	if stop < 0 {
		t.Fatal("deferred blur commit missing")
	}
	handler := s[start : start+stop]
	for _, forbidden := range []string{"procSetWindowTextW", "CB_SETCURSEL", "applyTextPropertyToSelection", "persistTextPreference", "updateTextFontUI", "syncTextToolbarControls", "commitPendingTextSize", "procSetFocus"} {
		if strings.Contains(handler, forbidden) {
			t.Errorf("EN_CHANGE must remain non-mutating: %s", forbidden)
		}
	}
}

func TestTextSizeComboSequentialNumericInput(t *testing.T) {
	for _, tc := range []struct {
		digits string
		want   int
	}{
		{"25", 25}, {"100", 100}, {"11", 11}, {"72", 72}, {"1", 1},
	} {
		for n := 1; n <= len(tc.digits); n++ {
			if _, ok := parseTextPointSize(tc.digits[:n]); !ok {
				t.Errorf("prefix %q failed", tc.digits[:n])
			}
		}
		value, ok := parseTextPointSize(tc.digits)
		if !ok || value != tc.want {
			t.Errorf("input %q got %d valid=%v", tc.digits, value, ok)
		}
	}
}

// Regression: 1.3.14 placed an EDIT on top of the full-sized combobox.
// On real Windows 11, that EDIT never got focus; keystrokes selected 5 pt
// from the combo repeatedly.  The numeric EDIT and arrow must be disjoint.
func TestTextSizeNativeHitTargetsAreDisjoint(t *testing.T) {
	if textSizeEntryOffsetX != 48 || textSizeControlsY != 56 {
		t.Fatal("unexpected size toolbar alignment")
	}
	if textSizeEntryWidth < 44 {
		t.Fatal("numeric input too small for 100")
	}
	if textSizeEntryOffsetX+textSizeEntryWidth >= textSizeArrowOffsetX {
		t.Fatalf("numeric editor overlaps dropdown: edit ends at %d, arrow starts at %d",
			textSizeEntryOffsetX+textSizeEntryWidth, textSizeArrowOffsetX)
	}
	if textSizeArrowOffsetX+textSizeArrowWidth >= 119 {
		t.Fatal("dropdown overlaps preview label starting at x=119")
	}
	b, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(b)
	for _, marker := range []string{
		`procMoveWindow.Call(a.textTool.SizeCombo, uintptr(px+textSizeArrowOffsetX),`,
		`procMoveWindow.Call(a.textTool.SizeEditHwnd, uintptr(px+textSizeEntryOffsetX),`,
		`TEXT_UI size edit focused initial`,
		`TEXT_UI size dropdown selection focus`,
	} {
		if !strings.Contains(source, marker) {
			t.Errorf("missing native hit-testing repair: %s", marker)
		}
	}
}
