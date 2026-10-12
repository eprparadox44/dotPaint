package main

const (
	// Keep automatic growth within the same decoded-image safety envelope used
	// by validClipboardImage. Manual canvas dialogs intentionally retain their
	// older 16384 px UI limit; clipboard-driven sizing may need to match a wider
	// but still valid image exactly.
	clipboardAutoCanvasMaxDimension = 32768
	clipboardAutoCanvasMaxPixels    = int64(40000000)
)

// canvasSizeForClipboardPaste returns a canvas size that contains the complete
// pasted image at (x,y), while preserving the old top-left origin. Negative
// paste coordinates do not request growth on the left/top because ResizeCanvas
// only grows the right/bottom sides in this path.
func canvasSizeForClipboardPaste(canvasW, canvasH, x, y, imageW, imageH int) (newW, newH int, grew, canFit bool) {
	newW, newH = canvasW, canvasH
	if canvasW < 1 || canvasH < 1 || imageW < 1 || imageH < 1 {
		return newW, newH, false, false
	}

	right := x + imageW
	bottom := y + imageH
	if x < 0 {
		right = imageW
	}
	if y < 0 {
		bottom = imageH
	}
	if right > newW {
		newW = right
	}
	if bottom > newH {
		newH = bottom
	}
	if newW < imageW {
		newW = imageW
	}
	if newH < imageH {
		newH = imageH
	}

	if newW > clipboardAutoCanvasMaxDimension || newH > clipboardAutoCanvasMaxDimension || int64(newW)*int64(newH) > clipboardAutoCanvasMaxPixels {
		return canvasW, canvasH, false, false
	}
	grew = newW != canvasW || newH != canvasH
	return newW, newH, grew, true
}
