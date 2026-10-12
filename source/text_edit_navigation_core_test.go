package main

import "testing"

func TestTextEditOutsideOwnedSpan(t *testing.T) {
	tests := []struct {
		name                    string
		prefix, anchor, spanEnd int
		want                    bool
	}{
		{"same caret", 37, 37, 37, false},
		{"IME span", 24, 24, 27, false},
		{"backspace neighbour", 36, 37, 37, false},
		{"moved far left", 24, 37, 37, true},
		{"moved right", 40, 37, 37, true},
		{"start boundary", 0, 0, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := textEditOutsideOwnedSpan(tc.prefix, tc.anchor, tc.spanEnd); got != tc.want {
				t.Fatalf("textEditOutsideOwnedSpan(%d,%d,%d)=%v want %v", tc.prefix, tc.anchor, tc.spanEnd, got, tc.want)
			}
		})
	}
}
