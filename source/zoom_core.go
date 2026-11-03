package main

const (
	// The document view always starts at the true image scale.
	initialCanvasZoom = 1
	// Selecting the magnifier preselects 8x without changing the current view.
	defaultMagnifierZoom = 8
)

func chooseDefaultMagnifierZoom(levels []int) int {
	for _, z := range levels {
		if z == defaultMagnifierZoom {
			return z
		}
	}
	if len(levels) == 0 {
		return initialCanvasZoom
	}
	return levels[len(levels)-1]
}

// magnifierSelectionSpan computes how many original pixels fit along one axis
// of the dashed zoom-selection region. Callers remove the fixed 16px margin
// from the workspace before passing the available space.
func magnifierSelectionSpan(availablePixels, targetZoom, canvasPixels int) int {
	if availablePixels < 1 || targetZoom < 1 || canvasPixels < 1 {
		return 0
	}
	return min(canvasPixels, (availablePixels+targetZoom-1)/targetZoom)
}

// magnifierWholeImageFits uses the *physical scaled size*, rather than the
// rounded-up dashed source span, to avoid skipping selection when the final
// row or column would be clipped by a few pixels.
func magnifierWholeImageFits(imageW, imageH, availableW, availableH, targetZoom int) bool {
	return imageW > 0 && imageH > 0 && targetZoom > 0 &&
		availableW > 0 && availableH > 0 &&
		imageW <= availableW/targetZoom && imageH <= availableH/targetZoom
}
