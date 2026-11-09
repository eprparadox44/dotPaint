//go:build windows

package main

// A pair of monochrome-mask and coloured GDI passes reconstructs the true
// glyph coverage even for black text. The two passes share the same layout,
// so a coloured text object has the same positions as its live editor.
func (a *App) makeStyledTextObject(text string) (MovableObject, bool) {
	if a.canvas == nil || a.dejavu != nil || text == "" {
		return MovableObject{}, false
	}
	tx, ty, w, h := a.textTool.X, a.textTool.Y, a.textTool.W, a.textTool.H
	if w < 1 || h < 1 || !validObjectSize(w, h) || int64(w)*int64(h) > 40000000 {
		return MovableObject{}, false
	}
	foreground, bg, op, bop := a.fg, a.bg, a.chosenOperation(false), a.chosenOperation(true)
	obj := MovableObject{Kind: ToolText, X: tx, Y: ty, W: w, H: h, SpriteW: w, SpriteH: h, Text: text,
		Styles: append([]TextCharStyle(nil), a.textTool.Styles...), FontName: a.textTool.FontName, FontSize: a.textTool.FontSize,
		Bold: a.textTool.Bold, Italic: a.textTool.Italic, Underline: a.textTool.Underline, ScriptMode: a.textTool.ScriptMode,
		FG: foreground, BG: bg, Opaque: a.textTool.Opaque, Operation: PaintRGB, BackgroundOperation: bop}
	colored := &Canvas{W: w, H: h, Pix: make([]byte, w*h*4)}
	mask := &Canvas{W: w, H: h, Pix: make([]byte, w*h*4)}
	inverse := &Canvas{W: w, H: h, Pix: make([]byte, w*h*4)}
	for i := 3; i < len(colored.Pix); i += 4 {
		colored.Pix[i], mask.Pix[i], inverse.Pix[i] = 255, 255, 255
	}
	savedX, savedY, savedOpaque, savedMode := a.textTool.X, a.textTool.Y, a.textTool.Opaque, a.textTool.GlyphMaskMode
	defer func() {
		a.textTool.X, a.textTool.Y, a.textTool.Opaque, a.textTool.GlyphMaskMode = savedX, savedY, savedOpaque, savedMode
	}()
	a.textTool.X, a.textTool.Y, a.textTool.Opaque = 0, 0, false
	a.textTool.GlyphMaskMode = 0
	a.renderTextToTarget(colored, text, false)
	a.textTool.GlyphMaskMode = 1
	a.renderTextToTarget(mask, text, false)
	anyInvert := op == PaintInvert
	if !anyInvert {
		for _, style := range obj.Styles {
			if style.ColorSet && style.ColorOp == PaintInvert {
				anyInvert = true
				break
			}
		}
	}
	if anyInvert {
		a.textTool.GlyphMaskMode = 2
		a.renderTextToTarget(inverse, text, false)
		obj.GlyphInvertMask = make([]byte, w*h)
	}
	pix := make([]byte, w*h*4)
	for i := 0; i < w*h; i++ {
		j := i * 4
		coverage := int(mask.Pix[j+2])
		if coverage > 0 {
			pix[j+3] = byte(coverage)
			for k := 0; k < 3; k++ {
				pix[j+k] = byte(min(255, (int(colored.Pix[j+k])*255+coverage/2)/coverage))
			}
		}
		if len(obj.GlyphInvertMask) != 0 {
			obj.GlyphInvertMask[i] = inverse.Pix[j+2]
		}
	}
	// Normal opaque backgrounds may be composited into the sprite. An invert
	// background must stay dynamic and is applied by blendObjectPixel.
	if obj.Opaque && bop == PaintRGB && !anyInvert {
		background := colorrefToBGRA(bg)
		for i := 0; i < len(pix); i += 4 {
			v := [4]byte{background[0], background[1], background[2], 255}
			blendBGRA(v[:], 0, pix, i)
			copy(pix[i:i+4], v[:])
		}
		// Keep Opaque=true as an editable property. Normal compositing uses
		// the already baked background when no inversion mask is present.
	}
	obj.Pix = pix
	return obj, true
}
