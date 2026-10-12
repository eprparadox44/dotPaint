package main

import "testing"

func TestCustomImageLimitsAndSlots(t *testing.T) {
	for _, tc := range []struct {
		w, h int
		ok   bool
	}{
		{1, 1, true},
		{300, 300, true},
		{301, 20, false},
		{20, 301, false},
		{0, 10, false},
	} {
		if got := customImageDimensionsAllowed(tc.w, tc.h); got != tc.ok {
			t.Fatalf("dimensions %dx%d got %v want %v", tc.w, tc.h, got, tc.ok)
		}
	}
	if got := customImageSlotFilename(0); got != "image_01.png" {
		t.Fatalf("slot 0 filename=%q", got)
	}
	if got := customImageSlotFilename(19); got != "image_20.png" {
		t.Fatalf("slot 19 filename=%q", got)
	}
	if got := customImageSlotFilename(20); got != "" {
		t.Fatalf("invalid slot filename=%q", got)
	}
	occupied := make([]bool, customImageMaxSlots)
	occupied[0] = true
	if got := customImageChooseSaveSlot(occupied, 5); got != 5 {
		t.Fatalf("selected empty slot got %d", got)
	}
	occupied[5] = true
	if got := customImageChooseSaveSlot(occupied, 5); got != 1 {
		t.Fatalf("first empty slot got %d", got)
	}
	for i := range occupied {
		occupied[i] = true
	}
	if got := customImageChooseSaveSlot(occupied, 0); got != -1 {
		t.Fatalf("full library got %d", got)
	}
}
