//go:build windows

package main

import "testing"

func TestSelectedTextPropertyComparison(t *testing.T) {
	a := TextCharStyle{FontName: "Arial", FontSize: 20}
	b := a
	b.Underline = true
	if a.Equal(b) {
		t.Fatal("underline must be part of run equality")
	}
	b = a
	b.ScriptMode = 1
	if a.Equal(b) {
		t.Fatal("script mode must be part of run equality")
	}
	for _, tt := range []struct {
		kind   string
		modify func(*TextCharStyle)
	}{
		{"font", func(s *TextCharStyle) { s.FontName = "Tahoma" }},
		{"size", func(s *TextCharStyle) { s.FontSize = 42 }},
		{"bold", func(s *TextCharStyle) { s.Bold = true }},
		{"italic", func(s *TextCharStyle) { s.Italic = true }},
		{"underline", func(s *TextCharStyle) { s.Underline = true }},
		{"scriptmode", func(s *TextCharStyle) { s.ScriptMode = -1 }},
	} {
		b = a
		tt.modify(&b)
		if textStylePropertyMatches(a, b, tt.kind) || !textStylePropertyMatches(b, b, tt.kind) {
			t.Fatalf("comparison failure: %s", tt.kind)
		}
	}
}
