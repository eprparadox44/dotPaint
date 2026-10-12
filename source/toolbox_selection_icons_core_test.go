package main

import "testing"

func TestSelectionToolIconPatternsHaveExpectedDimensionsAndPixels(t *testing.T) {
	tests := []struct {
		name        string
		icon        *[selectionToolIconHeight]string
		blackCount  int
		yellowCount int
	}{
		{"free", &selectionIconFreeSelect, 165, 0},
		{"rect", &selectionIconRectSelect, 84, 0},
		{"region", &selectionIconColorRegionSelect, 114, 0},
		{"all", &selectionIconColorAllSelect, 96, 179},
	}
	for _, tc := range tests {
		black, yellow := 0, 0
		for y, row := range tc.icon {
			if len(row) != selectionToolIconWidth {
				t.Fatalf("%s row %d width=%d want %d", tc.name, y, len(row), selectionToolIconWidth)
			}
			for _, ch := range row {
				switch ch {
				case '#':
					black++
				case 'Y':
					yellow++
				case '.':
				default:
					t.Fatalf("%s contains unsupported pixel %q", tc.name, ch)
				}
			}
		}
		if black != tc.blackCount || yellow != tc.yellowCount {
			t.Fatalf("%s black=%d yellow=%d want black=%d yellow=%d", tc.name, black, yellow, tc.blackCount, tc.yellowCount)
		}
	}
}
