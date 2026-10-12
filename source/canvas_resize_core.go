package main

const canvasResizeDragThreshold = 5

// canvasResizeDragActivated requires a deliberate drag before a canvas handle
// takes exclusive mouse capture. Chebyshev distance matches pixel UI behavior:
// moving five pixels horizontally, vertically, or diagonally is sufficient.
func canvasResizeDragActivated(startX, startY, x, y int) bool {
	dx := x - startX
	if dx < 0 {
		dx = -dx
	}
	dy := y - startY
	if dy < 0 {
		dy = -dy
	}
	return dx >= canvasResizeDragThreshold || dy >= canvasResizeDragThreshold
}

// canvasResizeCaptureChangeIsLoss distinguishes a real capture transfer from
// a redundant/self notification. WM_CAPTURECHANGED supplies the new owner in
// lParam; a notification naming our own window must not cancel the resize.
func canvasResizeCaptureChangeIsLoss(newOwner, ownWindow uintptr) bool {
	return newOwner != ownWindow
}
