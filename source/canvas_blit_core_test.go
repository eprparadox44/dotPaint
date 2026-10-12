package main

import "testing"

func TestCanvasVisibleBlitTilesLargeCanvas(t *testing.T) {
	clip := CanvasBlitClip{Left: 98, Top: 0, Right: 1067, Bottom: 659}
	tiles := canvasVisibleBlitTiles(1269, 884, 1, 106, 8, clip)
	if len(tiles) < 2 {
		t.Fatalf("expected multiple tiles, got %d", len(tiles))
	}
	if tiles[0].X0 != 0 || tiles[0].X1 != 961 || tiles[0].Y0 != 0 {
		t.Fatalf("unexpected first tile: %+v", tiles[0])
	}
	last := tiles[len(tiles)-1]
	if last.Y1 != 651 {
		t.Fatalf("visible bottom row mismatch: %+v", last)
	}
	for _, tile := range tiles {
		bytes := (tile.Y1 - tile.Y0) * 1269 * 4
		if bytes > canvasBlitTargetBytes && tile.Y1-tile.Y0 > 1 {
			t.Fatalf("tile too large: %+v bytes=%d", tile, bytes)
		}
	}
}

func TestCanvasVisibleBlitTilesScrolledZoomed(t *testing.T) {
	clip := CanvasBlitClip{Left: 100, Top: 0, Right: 900, Bottom: 600}
	tiles := canvasVisibleBlitTiles(2000, 1000, 4, -300, -120, clip)
	if len(tiles) == 0 {
		t.Fatal("expected visible tiles")
	}
	if tiles[0].X0 != 100 || tiles[0].Y0 != 30 {
		t.Fatalf("unexpected visible start: %+v", tiles[0])
	}
	last := tiles[len(tiles)-1]
	if last.X1 != 300 || last.Y1 != 180 {
		t.Fatalf("unexpected visible end: %+v", last)
	}
}

func TestCanvasVisibleBlitTilesOutside(t *testing.T) {
	clip := CanvasBlitClip{Left: 0, Top: 0, Right: 100, Bottom: 100}
	if got := canvasVisibleBlitTiles(640, 480, 1, 200, 200, clip); len(got) != 0 {
		t.Fatalf("expected no visible tiles, got %+v", got)
	}
}

func TestCanvasVisibleBlitTilesNoRowGapsAtA165Failure(t *testing.T) {
	clip := CanvasBlitClip{Left: 106, Top: 8, Right: 1250, Bottom: 868}
	tiles := canvasVisibleBlitTiles(1144, 860, 1, 106, 8, clip)
	if len(tiles) < 2 {
		t.Fatalf("expected multiple tiles, got %d", len(tiles))
	}
	expectedY := 0
	covered571 := false
	for _, tile := range tiles {
		if tile.Y0 != expectedY {
			t.Fatalf("row gap or overlap before %+v: expected y0=%d", tile, expectedY)
		}
		if tile.Y0 <= 571 && 571 < tile.Y1 {
			covered571 = true
		}
		expectedY = tile.Y1
	}
	if expectedY != 860 {
		t.Fatalf("visible bottom not covered: got %d", expectedY)
	}
	if !covered571 {
		t.Fatal("A165 failure row 571 was not covered by any tile")
	}
}
