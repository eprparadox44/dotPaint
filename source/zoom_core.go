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
