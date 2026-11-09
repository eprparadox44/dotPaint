//go:build windows

package main

// Individual UTF-16 units carry their own text ink. A collapsed selection
// changes only the insertion style; existing text is never recolored.
func (a *App) applyTextColorToSelection(previousColor uint32, previousOp PaintOperation, color uint32, operation PaintOperation) {
	if !a.textTool.Active || a.textTool.EditHwnd == 0 {
		return
	}
	a.syncTextModelFromInput()
	units := utf16UnitLen(a.textTool.Text)
	a.normalizeTextStyles(units)
	// Freeze legacy or inherited colours before moving the global swatch.
	for i := range a.textTool.Styles {
		if !a.textTool.Styles[i].ColorSet {
			a.textTool.Styles[i].ColorSet = true
			a.textTool.Styles[i].Color = previousColor
			a.textTool.Styles[i].ColorOp = previousOp
		}
	}
	start, end := a.textSelection()
	if end < start {
		start, end = end, start
	}
	start, end = clamp(start, 0, units), clamp(end, 0, units)
	if start != end {
		a.clearTextInsertionStyle("selected text ink")
		for i := start; i < end; i++ {
			a.textTool.Styles[i].ColorSet = true
			a.textTool.Styles[i].Color = color
			a.textTool.Styles[i].ColorOp = operation
		}
		sendMessage(a.textTool.EditHwnd, EM_SETSEL, uintptr(start), uintptr(end))
	} else {
		a.armTextInsertionStyle(start, "text ink")
		a.textTool.InsertionStyle.ColorSet = true
		a.textTool.InsertionStyle.Color = color
		a.textTool.InsertionStyle.ColorOp = operation
	}
	a.markTextLayoutDirty()
	a.syncTextLayoutControl()
	a.resetTextCaretBlink()
	a.requestTextRepaint(true)
}

func textStyleDisplayColor(style TextCharStyle, fallback uint32) uint32 {
	if !style.ColorSet {
		return fallback
	}
	if style.ColorOp == PaintInvert {
		return rgb(255, 255, 255)
	}
	return style.Color
}
