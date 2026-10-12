package main

// selectionGuideUsesWhite applies the user-facing luminance rule for selection
// marquees. Rec. 601 luma is used because it matches ordinary perceived
// brightness better than a simple RGB average. A luminance below 127 receives
// a white dash; 127 and above receive a black dash.
func selectionGuideUsesWhite(r, g, b byte) bool {
	luma := (299*int(r) + 587*int(g) + 114*int(b) + 500) / 1000
	return luma < 127
}
