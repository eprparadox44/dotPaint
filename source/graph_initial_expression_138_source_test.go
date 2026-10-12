package main

import (
	"os"
	"strings"
	"testing"
)

func TestGraphInitialExpressionIsRevealedAfterLayout(t *testing.T) {
	b, err := os.ReadFile("function_graph_editor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	required := []string{
		`graphCreateChild(s, "EDIT", "x^2"`,
		`func graphFocusInitialExpression(s *functionGraphEditorState)`,
		`sendMessage(s.input, EM_SETSEL, 0, 0)`,
		`sendMessage(s.input, graphEMScrollCaret, 0, 0)`,
		`sendMessage(s.input, EM_SETSEL, 0, ^uintptr(0))`,
	}
	for _, needle := range required {
		if !strings.Contains(src, needle) {
			t.Fatalf("missing initial-expression reveal guard %q", needle)
		}
	}

	create := strings.Index(src, "case WM_CREATE:")
	if create < 0 {
		t.Fatal("WM_CREATE not found")
	}
	tail := src[create:]
	layout := strings.Index(tail, "graphEditorLayoutControls(s, client)")
	focus := strings.Index(tail, "graphFocusInitialExpression(s)")
	if layout < 0 || focus < 0 || focus <= layout {
		t.Fatalf("initial expression must be focused/revealed after layout: layout=%d focus=%d", layout, focus)
	}
}
