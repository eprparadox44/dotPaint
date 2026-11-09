//go:build windows

package main

// Keep Object is a persistent creation preference, independent of Undo: an
// existing object's identity is never changed simply by switching this box.
func supportsKeepObjects(t Tool) bool {
	return t == ToolRectangle || t == ToolRightTriangle || t == ToolEllipse || t == ToolText || t == ToolLine || t == ToolArrow || t == ToolCurve
}

// Placed below the longest right-triangle size slider, in the toolbox option
// footer. This exact rectangle is shared by drawing and hit testing.
func keepObjectsOptionRect(tools ...Tool) RECT {
	top := toolOptionsTop() + 315
	if len(tools) > 0 && (tools[0] == ToolCurve || tools[0] == ToolArrow) {
		top = toolOptionsTop() + 350
	}
	return RECT{10, int32(top), 88, int32(top + 27)}
}

func (a *App) toggleObjectKeepAt(x, y int, toolbox RECT) bool {
	if !supportsKeepObjects(a.tool) || !pointInRect(x, y, keepObjectsOptionRect(a.tool)) ||
		y >= int(toolbox.Bottom)-8 {
		return false
	}
	if a.rasterMode {
		a.statusText = localizeUI("래스터화 모드에서는 객체 유지를 켤 수 없습니다.")
		a.requestStatusRepaint()
		return true
	}
	a.keepObjects = !a.keepObjects
	if a.keepObjects {
		a.statusText = localizeUI("객체 유지 켬: 새 도형·텍스트는 이동 가능한 객체로 남습니다.")
	} else {
		a.statusText = localizeUI("객체 유지 끔: 새 도형·텍스트는 픽셀로 확정됩니다.")
	}
	logf("OBJECT keep-default %v", a.keepObjects)
	a.requestStatusRepaint()
	return true
}

func (a *App) paintObjectKeepCheckbox(hdc uintptr, toolbox RECT) {
	r := keepObjectsOptionRect(a.tool)
	if r.Bottom >= toolbox.Bottom-8 {
		return
	}
	box := RECT{r.Left + 2, r.Top + 5, r.Left + 16, r.Top + 19}
	fillUIRect(hdc, box, rgb(255, 255, 255))
	frameUIRect(hdc, box, rgb(70, 70, 70))
	if a.keepObjects {
		c := uiColor(rgb(0, 112, 200))
		fillUIRect(hdc, RECT{box.Left + 3, box.Top + 7, box.Left + 6, box.Top + 10}, c)
		fillUIRect(hdc, RECT{box.Left + 6, box.Top + 9, box.Left + 9, box.Top + 12}, c)
		fillUIRect(hdc, RECT{box.Left + 8, box.Top + 4, box.Left + 11, box.Top + 10}, c)
	}
	label := RECT{box.Right + 3, r.Top, r.Right, r.Bottom}
	textColor := rgb(25, 25, 25)
	if a.rasterMode {
		textColor = rgb(136, 136, 136)
	}
	drawToolOptionText(hdc, localizeUI("객체 유지"), label, DT_LEFT|DT_VCENTER|DT_SINGLELINE, textColor, 7)
}
