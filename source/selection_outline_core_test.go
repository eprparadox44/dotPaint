package main

import "testing"

func TestSelectionGuideLuminanceThreshold(t *testing.T) {
	cases := []struct {
		name      string
		r, g, b   byte
		wantWhite bool
	}{
		{"black", 0, 0, 0, true},
		{"dark-gray-126", 126, 126, 126, true},
		{"threshold-gray-127", 127, 127, 127, false},
		{"white", 255, 255, 255, false},
		{"bright-green", 0, 255, 0, false},
		{"dark-blue", 0, 0, 255, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := selectionGuideUsesWhite(tc.r, tc.g, tc.b); got != tc.wantWhite {
				t.Fatalf("selectionGuideUsesWhite(%d,%d,%d)=%v want %v", tc.r, tc.g, tc.b, got, tc.wantWhite)
			}
		})
	}
}
