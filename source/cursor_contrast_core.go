package main

const cursorContrastLumaThreshold = 128

// cursorPerceivedLuma uses Rec. 601 integer weights. A neutral RGB value keeps
// the same luma, so RGB(127,127,127) is dark and RGB(128,128,128) is bright at
// the user-requested threshold.
func cursorPerceivedLuma(r, g, b byte) int {
	return (299*int(r) + 587*int(g) + 114*int(b) + 500) / 1000
}

func cursorBackgroundNeedsLight(r, g, b byte) bool {
	return cursorPerceivedLuma(r, g, b) < cursorContrastLumaThreshold
}

// cursorContrastPixel keeps a cursor pixel when it is already on the desired
// side of the threshold. Otherwise it performs a literal RGB inversion. This
// preserves user artwork and coloured accents while preventing dark-on-dark or
// light-on-light disappearance.
func cursorContrastPixel(r, g, b byte, wantLight bool) (byte, byte, byte) {
	isLight := cursorPerceivedLuma(r, g, b) >= cursorContrastLumaThreshold
	if isLight == wantLight {
		return r, g, b
	}
	return 255 - r, 255 - g, 255 - b
}
