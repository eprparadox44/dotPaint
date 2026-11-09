//go:build windows

package main

// An object receives the bucket click only on an actual painted pixel. Empty
// interiors of outline shapes remain eligible for the normal bounded fill.
func (a *App) bucketObjectAtPixel(x, y int) int {
	for i := len(a.objects) - 1; i >= 0; i-- {
		o := &a.objects[i]
		if o.Rasterized || o.Transparency >= 100 || o.Operation == PaintTransparent {
			continue
		}
		si := objectSpriteOffset(o, x, y)
		if si < 0 || si+3 >= len(o.Pix) {
			continue
		}
		if o.Pix[si+3] >= 128 || (o.Kind == ToolText && len(o.GlyphInvertMask) > si/4 && o.GlyphInvertMask[si/4] >= 128) {
			return i
		}
	}
	return -1
}

func (a *App) bucketTextCharAt(o MovableObject, x, y int) (int, int, bool) {
	previous := a.textTool
	defer func() { a.textTool = previous }()
	a.textTool = TextState{W: o.W, H: o.H, Text: o.Text, Styles: append([]TextCharStyle(nil), o.Styles...), FontName: o.FontName, FontSize: o.FontSize, Bold: o.Bold, Italic: o.Italic, Underline: o.Underline, ScriptMode: o.ScriptMode}
	hdc, release := a.createTextMeasureDC()
	defer release()
	_, lines, clusters, _ := a.layoutTextGDI(hdc, o.Text, max(1, o.W), max(1, o.H), false)
	for _, cl := range clusters {
		if cl.Break {
			continue
		}
		ch := max(2, lines[cl.Y])
		if x >= cl.X && x < cl.X+max(cl.W, 1) && y >= cl.Y && y < cl.Y+ch {
			return cl.Start, cl.End, true
		}
	}
	return 0, 0, false
}

func (a *App) bucketChangeObject(i, x, y int, col uint32, op PaintOperation) bool {
	if a.canvas == nil || i < 0 || i >= len(a.objects) {
		return false
	}
	original := a.objects[i]
	if original.Rasterized {
		return false
	}
	next := original
	if next.Kind == ToolText {
		start, end, ok := a.bucketTextCharAt(next, x-next.X, y-next.Y)
		if !ok {
			return false
		}
		length := utf16UnitLen(next.Text)
		if len(next.Styles) < length {
			styled := make([]TextCharStyle, length)
			copy(styled, next.Styles)
			for j := len(next.Styles); j < length; j++ {
				styled[j] = TextCharStyle{FontName: next.FontName, FontSize: next.FontSize, Bold: next.Bold, Italic: next.Italic, Underline: next.Underline, ScriptMode: next.ScriptMode, Color: next.FG, ColorSet: true, ColorOp: next.Operation}
			}
			next.Styles = styled
		} else {
			next.Styles = append([]TextCharStyle(nil), next.Styles...)
		}
		for j := start; j < end; j++ {
			next.Styles[j].ColorSet = true
			next.Styles[j].Color = col
			next.Styles[j].ColorOp = op
		}
		// The text object must be rerendered with its individual glyph colours.
	} else {
		next.FG, next.Operation = col, op
	}
	if objectsEqual([]MovableObject{original}, []MovableObject{next}) {
		return false
	}
	var ok bool
	if vectorObjectKind(next.Kind) {
		next, ok = renderVectorObject(next, a.canvas.W, a.canvas.H)
	} else {
		savedDrag := a.objectDragStart
		a.objectDragStart = original
		ok = a.renderResizedObject(&next)
		a.objectDragStart = savedDrag
	}
	if !ok {
		return false
	}
	// Unrelated group members and positions retain their original identity.
	next.GroupID, next.Transparency = original.GroupID, original.Transparency
	a.objects[i] = next
	a.objectsRevision++
	a.markDirty()
	a.recordHistory("색 채우기: 객체 색상")
	a.requestCanvasRepaint(true)
	return true
}

func (a *App) bucketFillWithObjectBoundaries(ix, iy int, col uint32) bool {
	if a.canvas == nil {
		return false
	}
	wall := bucketObjectWallMask(a.objects, a.canvas.W, a.canvas.H)
	if len(wall) == 0 {
		return false
	}
	replacement := colorrefToBGRA(col)
	a.undo.Push(a.canvas)
	n := floodFillBehindObjects(a.canvas.Pix, a.canvas.W, a.canvas.H, ix, iy, replacement, a.colorTolerance, wall)
	if n == 0 {
		// The mouse click was handled by the object-aware bucket, even if no
		// pixels changed (for example the seed lies on an object wall).
		return true
	}
	if a.indexedMode {
		palette := a.activeIndexedPaletteColors()
		if len(palette) > 0 && len(a.indexedPixelIndices) == a.canvas.W*a.canvas.H {
			idx := byte(nearestIndexedColor(byte(col), byte(col>>8), byte(col>>16), palette))
			for i := range wall {
				if wall[i] != 0 {
					continue
				}
				j := i * 4
				if a.canvas.Pix[j] == replacement[0] && a.canvas.Pix[j+1] == replacement[1] && a.canvas.Pix[j+2] == replacement[2] {
					a.indexedPixelIndices[i] = idx
				}
			}
		}
	}
	a.markDirty()
	a.recordHistory("색 채우기: 객체 경계")
	a.requestCanvasRepaint(true)
	return true
}
