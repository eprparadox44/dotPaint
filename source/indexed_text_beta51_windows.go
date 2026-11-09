//go:build windows

package main

// Quantize the committed text box once, not for every keystroke. Colour
// selection is already mapped to a palette entry, while GDI antialiasing can
// generate intermediate RGB values not present in a 16/256-colour document.
func (a *App) quantizeIndexedTextBox() {
	if !a.indexedMode || a.canvas == nil || a.dejavu != nil {
		return
	}
	palette := a.activeIndexedPaletteColors()
	if len(palette) == 0 {
		return
	}
	x0, y0 := max(0, a.textTool.X), max(0, a.textTool.Y)
	x1, y1 := min(a.canvas.W, a.textTool.X+a.textTool.W), min(a.canvas.H, a.textTool.Y+a.textTool.H)
	if x1 <= x0 || y1 <= y0 {
		return
	}
	cache := make(map[uint32]byte)
	indices := a.indexedPixelIndices
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			j := y*a.canvas.W + x
			i := j * 4
			if a.canvas.Pix[i+3] == 0 {
				continue
			}
			col := bgraToColorref(a.canvas.Pix[i], a.canvas.Pix[i+1], a.canvas.Pix[i+2])
			idx, ok := cache[col]
			if !ok {
				idx = byte(nearestIndexedColor(byte(col), byte(col>>8), byte(col>>16), palette))
				cache[col] = idx
			}
			v := palette[idx]
			a.canvas.Pix[i+0], a.canvas.Pix[i+1], a.canvas.Pix[i+2] = byte(v>>16), byte(v>>8), byte(v)
			if len(indices) == a.canvas.W*a.canvas.H {
				indices[j] = idx
			}
		}
	}
}
