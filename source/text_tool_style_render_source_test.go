package main

import (
	"os"
	"strings"
	"testing"
)

func TestTextToolRenderAndSizeWiring(t *testing.T) {
	b, e := os.ReadFile("main_windows.go")
	if e != nil {
		t.Fatal(e)
	}
	s := string(b)
	for _, required := range []string{
		"textScriptPointSize(style.FontSize, style.ScriptMode)",
		"textScriptTopOffset(baseInfo.height, info.height, style.ScriptMode)",
		"drawCluster(info, style, cluster, x, y+shiftY)",
		"a.requestTextRepaint(true)",
		"case ID_TEXT_SIZEEDIT:",
		"parseTextPointSize(value)",
		"drawTextToolbarStyleGlyph(dis, textColor, dx, dy)",
	} {
		if !strings.Contains(s, required) {
			t.Errorf("missing immediate text-format implementation: %s", required)
		}
	}
}
