//go:build windows

package main

func drawTextToolbarStyleGlyph(dis *DRAWITEMSTRUCT, color uint32, dx, dy int) {
	if dis == nil || dis.HDC == 0 || app == nil {
		return
	}
	rc := dis.RcItem
	rc.Left += int32(dx)
	rc.Right += int32(dx)
	rc.Top += int32(dy)
	rc.Bottom += int32(dy)
	baseFace := "Arial"
	draw := func(label string, size int, bold, italic, underline bool, target RECT) {
		font := createGDIFont(baseFace, size, bold, italic, underline)
		if font == 0 {
			return
		}
		old, _, _ := procSelectObject.Call(dis.HDC, font)
		procSetBkMode.Call(dis.HDC, TRANSPARENT)
		procSetTextColor.Call(dis.HDC, uintptr(color))
		drawTextUsingSelectedFont(dis.HDC, label, target, DT_CENTER|DT_VCENTER|DT_SINGLELINE, color)
		procSelectObject.Call(dis.HDC, old)
		procDeleteObject.Call(font)
	}
	switch dis.HwndItem {
	case app.textTool.BoldBtn:
		draw("B", 17, true, false, false, rc)
	case app.textTool.ItalicBtn:
		draw("I", 17, false, true, false, rc)
	case app.textTool.UnderlineBtn:
		draw("U", 17, false, false, true, rc)
	case app.textTool.SuperscriptBtn, app.textTool.SubscriptBtn:
		// Two deliberately distinct baselines show the actual script effect
		// instead of using a tiny Unicode literal that may have font fallback.
		mid := int(rc.Left) + 15
		base := RECT{rc.Left + 1, rc.Top + 4, int32(mid + 2), rc.Bottom - 2}
		draw("a", 16, false, false, false, base)
		small := RECT{int32(mid), rc.Top + 1, rc.Right - 1, rc.Top + 15}
		if dis.HwndItem == app.textTool.SubscriptBtn {
			small.Top = rc.Top + 12
			small.Bottom = rc.Bottom - 1
		}
		draw("a", 11, false, false, false, small)
	}
}

// The selected text units must all carry the requested property after a click.
func textStylePropertyMatches(style, requested TextCharStyle, kind string) bool {
	switch kind {
	case "font":
		return style.FontName == requested.FontName
	case "size":
		return style.FontSize == requested.FontSize
	case "bold":
		return style.Bold == requested.Bold
	case "italic":
		return style.Italic == requested.Italic
	case "underline":
		return style.Underline == requested.Underline
	case "scriptmode":
		return style.ScriptMode == requested.ScriptMode
	}
	return false
}
