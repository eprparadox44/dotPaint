package main

import "testing"

func bgraPixels(colors ...[4]byte) []byte {
	out := make([]byte, len(colors)*4)
	for i, c := range colors {
		copy(out[i*4:i*4+4], c[:])
	}
	return out
}

func TestToleranceFloodFillUsesRGBDistance(t *testing.T) {
	pix := bgraPixels(
		[4]byte{0, 0, 0, 255},
		[4]byte{3, 4, 0, 255}, // distance 5
		[4]byte{6, 8, 0, 255}, // distance 10
	)
	replacement := [4]byte{100, 110, 120, 255}
	if got := toleranceFloodFillBGRA(pix, 3, 1, 0, 0, replacement, 5); got != 2 {
		t.Fatalf("filled=%d, want 2", got)
	}
	if got := [4]byte{pix[8], pix[9], pix[10], pix[11]}; got == replacement {
		t.Fatalf("distance-10 pixel should remain unchanged")
	}
}

func TestToleranceFloodFillSameSeedColorStillNormalizesNeighbours(t *testing.T) {
	target := [4]byte{10, 10, 10, 255}
	pix := bgraPixels(target, [4]byte{13, 14, 10, 255})
	if got := toleranceFloodFillBGRA(pix, 2, 1, 0, 0, target, 5); got != 2 {
		t.Fatalf("filled=%d, want 2", got)
	}
	if got := [4]byte{pix[4], pix[5], pix[6], pix[7]}; got != target {
		t.Fatalf("neighbour=%v, want %v", got, target)
	}
}

func TestToleranceFloodFillStopsAtNonmatchingBarrier(t *testing.T) {
	black := [4]byte{0, 0, 0, 255}
	white := [4]byte{255, 255, 255, 255}
	pix := bgraPixels(black, white, black)
	replacement := [4]byte{20, 30, 40, 255}
	if got := toleranceFloodFillBGRA(pix, 3, 1, 0, 0, replacement, 0); got != 1 {
		t.Fatalf("filled=%d, want 1", got)
	}
	if got := [4]byte{pix[8], pix[9], pix[10], pix[11]}; got != black {
		t.Fatalf("disconnected black pixel changed: %v", got)
	}
}
