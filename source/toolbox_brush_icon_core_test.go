package main

import "testing"

func TestUserProvidedUnifiedBrushIcon(t *testing.T) {
	count, minX, minY, maxX, maxY, crc := toolIconStats(&unifiedBrushToolIconPixels)
	if count != 429 {
		t.Fatalf("non-transparent pixel count = %d, want 429", count)
	}
	if minX != 6 || minY != 4 || maxX != 39 || maxY != 35 {
		t.Fatalf("bounds = (%d,%d)-(%d,%d), want (6,4)-(39,35)", minX, minY, maxX, maxY)
	}
	if crc != 0x0C8E335C {
		t.Fatalf("pixel CRC32 = %08X, want 0C8E335C", crc)
	}
}
