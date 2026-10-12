package main

// diagonalBrushBand returns the integer half-band used by the two 45-degree
// unified-brush nibs. A minimum value of 1 prevents the 1..8 px nibs from
// degenerating into a visually disconnected one-pixel diagonal.
func diagonalBrushBand(size int) int {
	if size < 1 {
		size = 1
	}
	band := (size - 1) / 8
	if band < 1 {
		band = 1
	}
	return band
}

// diagonalBrushFootprintSize allows the diagonal nib to remain visibly two
// pixels thick even when the nominal brush size is 1 px.
func diagonalBrushFootprintSize(size int) int {
	if size < 2 {
		return 2
	}
	return size
}
