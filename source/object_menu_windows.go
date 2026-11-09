//go:build windows

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	objectArmMove   uint8 = 1
	objectArmResize uint8 = 2
)
const (
	objectMenuMove = 0xB200 + iota
	objectMenuResize
	objectMenuForward
	objectMenuBackward
	objectMenuFront
	objectMenuBack
	objectMenuRasterize
	objectMenuOpacityBase = 0xB300
	objectMenuGroup       = 0xB210
	objectMenuUngroup     = 0xB211
)

const objectSelectionGuidance = "객체 선택(V): 몸통은 이동, 도형 8개 핸들은 크기 조절. 선·화살표는 끝점, 곡선은 끝점·굽힘점을 끌어 편집합니다. 텍스트는 F2, 우클릭은 래스터화·순서·불투명도입니다."

func (a *App) activateObjectTool() {
	a.commitPendingTextSize("object tool")
	if a.objectDragging {
		a.endObjectMove(0, 0, false)
	}
	if a.textTool.Active {
		a.finishTextEditing(true)
	}
	if a.curveUndoPending {
		a.finishCurveEarlyOrCancel("object tool")
	}
	if a.tool == ToolEyedropper {
		a.stopScreenEyedropper()
	}
	a.clearZoomPreview()
	a.clearSelectionState()
	a.objectArmed = 0
	a.tool = ToolObjectSelect
	a.statusText = localizeUI(objectSelectionGuidance)
	a.updateScrollbars()
	if a.hwnd != 0 {
		var client RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
		a.updateTextUILayout(client)
		procSetFocus.Call(a.hwnd)
	}
	invalidate(a.hwnd)
}

func (a *App) selectedObject() *MovableObject {
	i := a.objectSelected
	if i < 0 || i >= len(a.objects) {
		return nil
	}
	return &a.objects[i]
}

func (a *App) captureSelectedObjectImage() ClipboardImage {
	ids := a.selectedObjectIndices()
	bounds, ok := selectedGroupBounds(a.objects, ids)
	if !ok || !validObjectSize(bounds.W, bounds.H) {
		return ClipboardImage{}
	}
	out := ClipboardImage{W: bounds.W, H: bounds.H, Pix: make([]byte, bounds.W*bounds.H*4)}
	sprites := make([]MovableObject, 0, len(ids))
	for _, i := range ids {
		o := a.objects[i]
		if objectHasInversion(&o) {
			o = freezeInverseObject(a.canvas, a.objects, i)
		}
		sprites = append(sprites, o)
	}
	compositeObjectsToPixels(out.Pix, out.W, out.H, bounds.X, bounds.Y, sprites)
	return out
}

func (a *App) setSelectedObjectOpacity(percent int) bool {
	o := a.selectedObject()
	if o == nil {
		return false
	}
	percent = clamp(percent, 0, 100)
	if objectHasInversion(o) && percent != 100 {
		a.statusText = localizeUI("반전 객체는 불투명도 100%만 지원합니다.")
		a.requestStatusRepaint()
		return false
	}
	next := uint8(100 - percent)
	if o.Transparency == next {
		return false
	}
	o.Transparency = next
	logf("OBJECT opacity index=%d percent=%d", a.objectSelected, percent)
	a.objectsRevision++
	a.markDirty()
	a.recordHistory("객체 불투명도")
	a.statusText = fmt.Sprintf(localizeUI("객체 불투명도: %d%% (0%%=완전 투명, 100%%=완전 불투명)"), percent)
	a.requestCanvasRepaint(true)
	a.requestStatusRepaint()
	return true
}

func objectOrderDestination(command, index, count int) int {
	switch command {
	case objectMenuForward:
		return min(count-1, index+1)
	case objectMenuBackward:
		return max(0, index-1)
	case objectMenuFront:
		return count - 1
	case objectMenuBack:
		return 0
	}
	return index
}

func (a *App) applyObjectMenuCommand(command int) bool {
	if command == objectMenuGroup {
		return a.groupSelectedObjects()
	}
	if command == objectMenuUngroup {
		return a.ungroupSelectedObjects()
	}
	if a.selectedObject() == nil || a.selectedObject().Rasterized {
		return false
	}
	groupSelection := len(a.selectedObjectIndices()) > 1
	// A group has one geometry, not editable member-specific opacity/order.
	if groupSelection && command != objectMenuMove && command != objectMenuResize {
		a.statusText = localizeUI("그룹은 이동·크기 조절만 가능합니다. 더블클릭하면 내부 개체를 선택할 수 있습니다.")
		a.requestStatusRepaint()
		return false
	}
	a.objectArmed = 0
	switch command {
	case objectMenuRasterize:
		return a.rasterizeSelectedObject()
	case objectMenuMove, objectMenuResize:
		a.objectArmed = objectArmMove
		a.statusText = localizeUI("객체 이동 대기: 캔버스 작업 영역에서 왼쪽 버튼을 누른 채 끌면 이동합니다. Esc는 취소합니다.")
		if command == objectMenuResize {
			a.objectArmed = objectArmResize
			a.statusText = localizeUI("객체 크기 조절 대기: 다음 왼쪽 드래그로 오른쪽 아래 모서리를 조절합니다. Esc는 취소합니다.")
		}
		// A native popup consumes its closing button release. Do not simulate
		// input or capture it retroactively: arm the very next fresh left press.
		logf("OBJECT arm index=%d mode=%d", a.objectSelected, a.objectArmed)
		procSetCursor.Call(a.objectInteractionCursor(ObjectBody))
		a.requestCanvasRepaint(true)
		a.requestStatusRepaint()
		return true
	case objectMenuForward, objectMenuBackward, objectMenuFront, objectMenuBack:
		dest := objectOrderDestination(command, a.objectSelected, len(a.objects))
		i, changed := reorderObject(a.objects, a.objectSelected, dest)
		if !changed {
			return false
		}
		logf("OBJECT order from=%d to=%d command=%d", a.objectSelected, i, command)
		a.objectSelected = i
		a.objectsRevision++
		a.markDirty()
		a.recordHistory("객체 순서")
		a.requestCanvasRepaint(true)
		return true
	}
	if command >= objectMenuOpacityBase && command <= objectMenuOpacityBase+100 {
		return a.setSelectedObjectOpacity(command - objectMenuOpacityBase)
	}
	return false
}

func forgetObjectPopupMenus(menus ...uintptr) {
	// Main-menu registrations remain valid. Repeated context menus must not
	// accumulate owner-draw records or reset any other open menu's keys.
	menuDrawMu.Lock()
	defer menuDrawMu.Unlock()
	for _, m := range menus {
		for _, key := range menuDrawKeysByMenu[m] {
			delete(menuDrawEntries, key)
		}
		delete(menuDrawKeysByMenu, m)
	}
}

func (a *App) showObjectContextMenu(mx, my int) {
	if a.tool != ToolObjectSelect || a.objectDragging {
		return
	}
	i, _ := a.hitObjectClient(mx, my)
	if i < 0 {
		return
	}
	if !a.objectSelectionHas(i) {
		a.objectIsolatedGroup = 0
		a.selectObjectWithGroup(i)
	}
	a.objectArmed = 0
	if a.hwnd != 0 {
		procSetFocus.Call(a.hwnd)
	}
	a.requestObjectSelectionUIRepaint()
	m, _, _ := procCreatePopupMenu.Call()
	opacity, _, _ := procCreatePopupMenu.Call()
	order, _, _ := procCreatePopupMenu.Call()
	if m == 0 || opacity == 0 || order == 0 {
		for _, h := range []uintptr{m, opacity, order} {
			if h != 0 {
				procDestroyMenu.Call(h)
			}
		}
		a.statusText = localizeUI("객체 메뉴를 열지 못했습니다.")
		a.requestStatusRepaint()
		return
	}
	defer func() { forgetObjectPopupMenus(m, opacity, order); procDestroyMenu.Call(m) }()
	a.appendObjectCommands(m, opacity, order, i)
	a.applyDarkMenuBackground(m) // includes both nested popups
	pt := POINT{int32(mx), int32(my)}
	procClientToScreen.Call(a.hwnd, uintptr(unsafe.Pointer(&pt)))
	cmd, _, _ := procTrackPopupMenu.Call(m, TPM_LEFTALIGN|TPM_TOPALIGN|TPM_RETURNCMD|TPM_NONOTIFY, uintptr(pt.X), uintptr(pt.Y), 0, a.hwnd, 0)
	if cmd != 0 {
		a.applyObjectMenuCommand(int(cmd))
	}
}

// The native tooltip tracks the same client rectangle as toolbar hit testing.
// Its text buffer stays owned by App; no Go pointer is retained without a root.
type objectToolInfo struct {
	Size, Flags     uint32
	Hwnd, ID        uintptr
	Rect            RECT
	Instance        uintptr
	Text            *uint16
	Param, Reserved uintptr
}

func (a *App) updateObjectToolTooltip() {
	if a.hwnd == 0 {
		return
	}
	const ttmAddToolW = 0x0432
	const ttmNewToolRectW = 0x0434
	const ttmUpdateTipTextW = 0x0439
	var rc RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	_, tb, pal, _ := a.layout(rc)
	tr := RECT{}
	if a.showToolbox && !a.bitmapView {
		for i, t := range visibleTools {
			if t == ToolObjectSelect {
				tr = RECT{tb.Left + int32(4+(i%2)*toolCellWidth), tb.Top + int32(4+(i/2)*toolCellHeight), 0, 0}
				tr.Right, tr.Bottom = tr.Left+toolButtonWidth, tr.Top+toolButtonHeight
				break
			}
		}
	}
	label := localizeUI("객체 선택 (V)")
	created := false
	if a.objectTooltip == 0 {
		icc := INITCOMMONCONTROLSEX{DwSize: uint32(unsafe.Sizeof(INITCOMMONCONTROLSEX{})), DwICC: 0x000000FF}
		procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))
		h, _, _ := procCreateWindowExW.Call(0x00000008, uintptr(unsafe.Pointer(rawUTF16Ptr("tooltips_class32"))), 0, 0x80000000|1|2, 0, 0, 0, 0, a.hwnd, 0, a.hinst, 0)
		if h == 0 {
			return
		}
		a.objectTooltip = h
		created = true
	}
	previousText := a.objectTooltipText
	textChanged := a.objectTooltipLabel != label
	if textChanged || created {
		a.objectTooltipText = syscall.StringToUTF16(label)
		a.objectTooltipLabel = label
	}
	ti := objectToolInfo{Flags: 0x10, Hwnd: a.hwnd, ID: 0xB200, Rect: tr, Text: &a.objectTooltipText[0]}
	ti.Size = uint32(unsafe.Sizeof(ti))
	if created {
		sendMessage(a.objectTooltip, ttmAddToolW, 0, uintptr(unsafe.Pointer(&ti)))
	}
	if tr != a.objectTooltipRect {
		sendMessage(a.objectTooltip, ttmNewToolRectW, 0, uintptr(unsafe.Pointer(&ti)))
		a.objectTooltipRect = tr
	}
	if textChanged {
		sendMessage(a.objectTooltip, ttmUpdateTipTextW, 0, uintptr(unsafe.Pointer(&ti)))
	}
	runtime.KeepAlive(previousText)
	if created || a.objectTooltipDark != a.darkMode {
		applyDarkThemeToWindow(a.objectTooltip, a.darkMode)
		// Disable visual-style fill only for this tooltip, permitting explicit
		// readable foreground/background colors in both themes.
		procSetWindowTheme.Call(a.objectTooltip, uintptr(unsafe.Pointer(rawUTF16Ptr(""))), uintptr(unsafe.Pointer(rawUTF16Ptr(""))))
		bg, fg := rgb(255, 255, 235), rgb(15, 15, 15)
		if a.darkMode {
			bg, fg = rgb(38, 38, 38), rgb(240, 240, 240)
		}
		sendMessage(a.objectTooltip, 0x0413, uintptr(bg), 0)
		sendMessage(a.objectTooltip, 0x0414, uintptr(fg), 0)
		a.objectTooltipDark = a.darkMode
	}
	if a.showPalette && !a.bitmapView {
		a.updateInvertTooltip(pal)
	} else {
		a.updateInvertTooltip(RECT{-1000, -1000, -1000, -1000})
	}
}

// Shared between canvas right-click and the top-level Selection menu.
func (a *App) appendObjectCommands(m, opacity, order uintptr, i int) {
	groupFlags := uint32(MF_STRING)
	if len(a.selectedObjectIndices()) < 2 {
		groupFlags |= MF_GRAYED
	}
	ungroupFlags := uint32(MF_STRING)
	if i < 0 || i >= len(a.objects) || a.objects[i].GroupID == 0 {
		ungroupFlags |= MF_GRAYED
	}
	appendItem(m, groupFlags, objectMenuGroup, "선택 묶기(&G)")
	appendItem(m, ungroupFlags, objectMenuUngroup, "묶기 해제(&U)")
	appendSep(m)
	appendItem(m, MF_STRING, objectMenuMove, "이동(&M)")
	appendItem(m, MF_STRING, objectMenuResize, "크기 조절(&R)")
	appendSep(m)
	appendItem(m, MF_STRING, objectMenuRasterize, "래스터화(&S)")
	appendSep(m)
	for _, p := range []int{0, 10, 20, 25, 30, 40, 50, 60, 70, 75, 80, 90, 100} {
		flags := uint32(MF_STRING)
		if i >= 0 && i < len(a.objects) && objectHasInversion(&a.objects[i]) && p != 100 {
			flags |= MF_GRAYED
		}
		if i >= 0 && i < len(a.objects) && p == 100-int(a.objects[i].Transparency) {
			flags |= MF_CHECKED
		}
		appendItem(opacity, flags, objectMenuOpacityBase+p, fmt.Sprintf("%d%%", p))
	}
	appendPopup(m, opacity, "불투명도(&O)")
	for _, entry := range []struct {
		id    int
		label string
	}{
		{objectMenuForward, "앞으로 한 단계(&F)"}, {objectMenuBackward, "뒤로 한 단계(&B)"},
		{objectMenuFront, "맨 앞으로(&T)"}, {objectMenuBack, "맨 뒤로(&K)"},
	} {
		flags := uint32(MF_STRING)
		if i < 0 || i >= len(a.objects) || objectOrderDestination(entry.id, i, len(a.objects)) == i {
			flags |= MF_GRAYED
		}
		appendItem(order, flags, entry.id, entry.label)
	}
	appendPopup(m, order, "순서(&Z)")
}

// The top menu persists across selections, so refresh enabled and checked
// states just before Windows opens each owner-drawn popup.
func (a *App) refreshObjectSelectMenu(menu uintptr) {
	if menu != a.objectSelectMenu && menu != a.objectSelectOpacityMenu && menu != a.objectSelectOrderMenu {
		return
	}
	i := a.objectSelected
	enabled := a.tool == ToolObjectSelect && i >= 0 && i < len(a.objects) && !a.objects[i].Rasterized
	groupSelection := len(a.selectedObjectIndices()) > 1
	groupFlags := uint32(MF_BYCOMMAND)
	if !enabled || !groupSelection {
		groupFlags |= MF_GRAYED
	}
	procEnableMenuItem.Call(a.objectSelectMenu, uintptr(objectMenuGroup), uintptr(groupFlags))
	ungroupFlags := uint32(MF_BYCOMMAND)
	if !enabled || a.objects[i].GroupID == 0 {
		ungroupFlags |= MF_GRAYED
	}
	procEnableMenuItem.Call(a.objectSelectMenu, uintptr(objectMenuUngroup), uintptr(ungroupFlags))
	menuIds := []int{objectMenuMove, objectMenuResize, objectMenuRasterize, objectMenuForward, objectMenuBackward, objectMenuFront, objectMenuBack}
	for _, id := range menuIds {
		state := uint32(MF_BYCOMMAND)
		if !enabled || (groupSelection && id != objectMenuMove && id != objectMenuResize) || (id >= objectMenuForward && id <= objectMenuBack && objectOrderDestination(id, i, len(a.objects)) == i) {
			state |= MF_GRAYED
		}
		if id >= objectMenuForward && id <= objectMenuBack {
			procEnableMenuItem.Call(a.objectSelectOrderMenu, uintptr(id), uintptr(state))
		} else {
			procEnableMenuItem.Call(a.objectSelectMenu, uintptr(id), uintptr(state))
		}
	}
	for _, p := range []int{0, 10, 20, 25, 30, 40, 50, 60, 70, 75, 80, 90, 100} {
		flags := uint32(MF_BYCOMMAND)
		if !enabled || groupSelection || (enabled && objectHasInversion(&a.objects[i]) && p != 100) {
			flags |= MF_GRAYED
		}
		procEnableMenuItem.Call(a.objectSelectOpacityMenu, uintptr(objectMenuOpacityBase+p), uintptr(flags))
		check := uint32(MF_BYCOMMAND)
		if enabled && p == 100-int(a.objects[i].Transparency) {
			check |= MF_CHECKED
		}
		procCheckMenuItem.Call(a.objectSelectOpacityMenu, uintptr(objectMenuOpacityBase+p), uintptr(check))
	}
}
