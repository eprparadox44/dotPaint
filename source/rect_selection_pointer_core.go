package main

const rectSelectionDragThreshold = 5

// rectSelectionDragActivated distinguishes a cancellation click from a new
// rectangle drag. Chebyshev distance is appropriate for pixel UI gestures:
// five pixels horizontally, vertically, or diagonally activates selection.
func rectSelectionDragActivated(startX, startY, x, y int) bool {
	dx := x - startX
	if dx < 0 {
		dx = -dx
	}
	dy := y - startY
	if dy < 0 {
		dy = -dy
	}
	return dx >= rectSelectionDragThreshold || dy >= rectSelectionDragThreshold
}
