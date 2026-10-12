package main

import "testing"

func TestTextIMEAnchoredRangePreservesSuffix(t *testing.T) {
	base := []uint16{'A', '?'}
	start, end, ok := textIMEAnchoredRange(base, []uint16{'A', 'X', '?'}, 1, 1)
	if !ok || start != 1 || end != 2 {
		t.Fatalf("got %d..%d ok=%v", start, end, ok)
	}
}

func TestTextIMEAnchoredRangeHandlesTransientShrink(t *testing.T) {
	base := []uint16{'A', '?'}
	start, end, ok := textIMEAnchoredRange(base, []uint16{'A', '?'}, 1, 1)
	if !ok || start != 1 || end != 1 {
		t.Fatalf("got %d..%d ok=%v", start, end, ok)
	}
}

func TestTextIMEAnchoredRangeRepeatedNeighbour(t *testing.T) {
	base := []uint16{'기', '?'}
	start, end, ok := textIMEAnchoredRange(base, []uint16{'기', '기', '?'}, 1, 1)
	if !ok || start != 1 || end != 2 {
		t.Fatalf("got %d..%d ok=%v", start, end, ok)
	}
}
