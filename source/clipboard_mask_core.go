package main

// applyClipboardSelectionMask converts the pixels outside an irregular
// selection into real transparent pixels before the image enters dotPaint's
// clipboard history or the Windows clipboard. Images are necessarily
// rectangular, so the mask's bounding rectangle remains the image size while
// every unselected pixel is encoded as transparent BGRA(0,0,0,0).
//
// A nil or malformed mask means an ordinary rectangular selection and leaves
// the pixel buffer unchanged.
func applyClipboardSelectionMask(pix []byte, mask []bool, w, h int) (kept, cleared int) {
	if w <= 0 || h <= 0 || len(pix) != w*h*4 || len(mask) != w*h {
		return 0, 0
	}
	for i, selected := range mask {
		if selected {
			kept++
			continue
		}
		pi := i * 4
		pix[pi+0] = 0
		pix[pi+1] = 0
		pix[pi+2] = 0
		pix[pi+3] = 0
		cleared++
	}
	return kept, cleared
}
