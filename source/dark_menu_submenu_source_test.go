package main

import (
	"os"
	"strings"
	"testing"
)

func TestDarkNestedPopupUsesNormalMenuColumnsA218(t *testing.T) {
	data, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, marker := range []string{
		"Popup    bool",
		"registerMenuDrawEntry(text, false, true)",
		"func appendTopLevelPopup",
		`appendTopLevelPopup(main, file, "파일(&F)")`,
		`appendPopup(view, zoomM, "확대/축소(&Z)")`,
		`appendPopup(insertM, insertImageM, "이미지(&I)")`,
		`Reserve the native Windows submenu-arrow column`,
		"dis.RcItem.Left + 30",
		"rightReserve = 30",
	} {
		if !strings.Contains(src, marker) {
			t.Fatalf("missing dark submenu alignment marker %q", marker)
		}
	}
}
