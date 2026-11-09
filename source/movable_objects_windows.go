//go:build windows

package main

import (
	"fmt"
	"unsafe"
)

// The object workspace deliberately lives above the normal raster canvas.
// Painting and geometry tools never decompress/rasterize objects per stroke:
// cached sprites are composited into only the visible canvas blit tiles.
// A normal PNG/BMP/JPEG/GIF export flattens the objects; the native editable
// model is intentionally document-session-only in this experimental version.
func (a *App) applyObjectsToTile(tile CanvasBlitTile) {
	if len(a.objects) == 0 || a.canvasTileBits == nil {
		return
	}
	dst := unsafe.Slice((*byte)(a.canvasTileBits), a.canvasTileW*a.canvasTileH*4)
	compositeObjectRows(dst, tile.X1-tile.X0, tile.Y1-tile.Y0, a.canvasTileW, tile.X0, tile.Y0, a.objects, a.objectEditingIndex)
}

func (a *App) sampledDisplayObjectPixel(x, y int, src []byte) uint32 {
	if a.canvas == nil {
		return 0
	}
	i := (y*a.canvas.W + x) * 4
	if i < 0 || i+4 > len(src) {
		return 0
	}
	b, g, r, alpha := src[i], src[i+1], src[i+2], src[i+3]
	tmp := [4]byte{b, g, r, alpha}
	for k := range a.objects {
		if k == a.objectEditingIndex {
			continue
		}
		o := &a.objects[k]
		blendObjectPixel(tmp[:], 0, o, objectSpriteOffset(o, x, y))
	}
	return bgraToColorref(tmp[0], tmp[1], tmp[2]) & 0xFFFFFF
}

func (a *App) makeShapeObject(kind Tool, x0, y0, x1, y1 int, constrained bool, col uint32) (MovableObject, bool) {
	if a.canvas == nil || a.dejavu != nil || a.indexedMode {
		return MovableObject{}, false
	}
	if kind != ToolRectangle && kind != ToolRightTriangle && kind != ToolEllipse {
		return MovableObject{}, false
	}
	savedCol := col
	if a.drawOperation == PaintInvert {
		col = 0xFFFFFF
	}
	pad := 8
	if kind == ToolRightTriangle {
		pad = max(pad, a.rightTriangleSize*2+5)
	}
	// Use the exact original geometry and rasterizer; constrain before
	// allocating, so both the sprite and stored bounds match the mouse-up.
	if kind == ToolRectangle || kind == ToolEllipse {
		x1, y1 = a.constrainedRectangleEndpoint(x0, y0, x1, y1, constrained)
	} else {
		x1, y1 = a.constrainedRightTriangleEndpoint(x0, y0, x1, y1, constrained)
	}
	left := max(0, min(x0, x1)-pad)
	top := max(0, min(y0, y1)-pad)
	right := min(a.canvas.W, max(x0, x1)+pad+1)
	bottom := min(a.canvas.H, max(y0, y1)+pad+1)
	if right <= left || bottom <= top {
		return MovableObject{}, false
	}
	c := &Canvas{W: right - left, H: bottom - top, Pix: make([]byte, (right-left)*(bottom-top)*4)}
	stroke := a.rectangleSize
	if kind == ToolRectangle {
		a.drawRasterRectangleAt(c, x0, y0, x1, y1, false, col, left, top)
	} else if kind == ToolEllipse {
		stroke = a.ellipseSize
		a.drawRasterEllipseAt(c, x0, y0, x1, y1, false, col, left, top)
	} else {
		stroke = a.rightTriangleSize
		a.drawRasterRightTriangleAt(c, x0, y0, x1, y1, constrained, col, left, top)
	}
	o, ok := trimObjectSprite(MovableObject{Kind: kind, X: left, Y: top, W: c.W, H: c.H, Pix: c.Pix, FG: savedCol, Operation: a.drawOperation})
	if !ok {
		return o, false
	}
	o.SpriteW, o.SpriteH, o.ShapeW, o.ShapeH = o.W, o.H, o.W, o.H
	o.ShapeX0, o.ShapeY0, o.ShapeX1, o.ShapeY1 = x0-o.X, y0-o.Y, x1-o.X, y1-o.Y
	o.Stroke, o.ShowCenter = stroke, a.shapeShowCenter
	return o, true
}

// Construct an independent text sprite using the native GDI layout and the
// same rich per-character styles.  A contrasting sentinel background makes
// black and white letters alike distinguishable even when text is transparent.
func (a *App) makeTextObjectRGB(text string) (MovableObject, bool) {
	if a.canvas == nil || a.dejavu != nil || a.indexedMode || text == "" {
		return MovableObject{}, false
	}
	tx, ty, w, h := a.textTool.X, a.textTool.Y, a.textTool.W, a.textTool.H
	if w < 1 || h < 1 || w > 20000 || h > 20000 || int64(w)*int64(h) > 40000000 {
		return MovableObject{}, false
	}
	obj := MovableObject{SpriteW: w, SpriteH: h, Kind: ToolText, X: tx, Y: ty, W: w, H: h, Text: text, Styles: append([]TextCharStyle(nil), a.textTool.Styles...), FontName: a.textTool.FontName, FontSize: a.textTool.FontSize, Bold: a.textTool.Bold, Italic: a.textTool.Italic, Underline: a.textTool.Underline, ScriptMode: a.textTool.ScriptMode, Opaque: a.textTool.Opaque, FG: a.fg, BG: a.bg}
	canvas := &Canvas{W: w, H: h, Pix: make([]byte, w*h*4)}
	bg := a.fg ^ 0x00FFFFFF
	if a.textTool.Opaque && !isTransparentColor(a.bg) {
		bg = a.bg
	}
	bb := colorrefToBGRA(bg)
	for i := 0; i < len(canvas.Pix); i += 4 {
		copy(canvas.Pix[i:i+4], bb[:])
	}
	a.textTool.X, a.textTool.Y = 0, 0
	a.renderTextToTarget(canvas, text, false)
	a.textTool.X, a.textTool.Y = tx, ty
	if a.textTool.Opaque && !isTransparentColor(a.bg) {
		obj.Pix = canvas.Pix
		return obj, true
	}
	fg := colorrefToBGRA(a.fg)
	for i := 0; i < len(canvas.Pix); i += 4 {
		// The glyph is a mix of the foreground and a deliberately contrasting
		// sentinel, with GDI's AA coverage reconstructed for transparency.
		maxCov := 0
		for ch := 0; ch < 3; ch++ {
			delta := int(fg[ch]) - int(bb[ch])
			if delta == 0 {
				continue
			}
			cov := (int(canvas.Pix[i+ch]) - int(bb[ch])) * 255 / delta
			maxCov = max(maxCov, clamp(cov, 0, 255))
		}
		for ch := 0; ch < 3; ch++ {
			canvas.Pix[i+ch] = fg[ch]
		}
		canvas.Pix[i+3] = byte(maxCov)
	}
	obj.Pix = canvas.Pix
	return obj, true
}

func (a *App) addMovableObject(o MovableObject) {
	if a.rasterMode {
		if a.canvas != nil {
			compositeObjectsToPixels(a.canvas.Pix, a.canvas.W, a.canvas.H, 0, 0, []MovableObject{o})
			a.markDirty()
			a.recordHistory("래스터 그림")
		}
		return
	}
	a.objects = append(a.objects, o)
	a.objectsRevision++
	a.objectMultiSelected = nil
	a.objectIsolatedGroup = 0
	a.objectSelected = len(a.objects) - 1
	a.markDirty()
	a.recordHistory("객체 생성")
	a.statusText = fmt.Sprintf(localizeUI("%s 객체 생성 (%d개). 객체 선택(V)으로 이동·크기 조절, 텍스트는 F2로 재편집합니다."), localizeUI(toolNames[o.Kind]), len(a.objects))
	logf("OBJECT add kind=%d xy=%d,%d size=%dx%d count=%d", o.Kind, o.X, o.Y, o.W, o.H, len(a.objects))
}

func (a *App) hitMovableObject(x, y int) int { return hitObjectBody(a.objects, x, y) }

func (a *App) objectClientGeometry() (int, int) {
	var rc RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	ws, _, _, _ := a.layout(rc)
	return a.canvasOrigin(ws)
}

func (a *App) objectImagePoint(mx, my int) (int, int) {
	ox, oy := a.objectClientGeometry()
	return floorDivInt(mx-ox, max(1, a.zoom)), floorDivInt(my-oy, max(1, a.zoom))
}

func (a *App) hitObjectClient(mx, my int) (int, ObjectHandle) {
	if a.tool != ToolObjectSelect || a.textTool.Active {
		return -1, ObjectBody
	}
	ox, oy := a.objectClientGeometry()
	if bounds, ok := a.objectGroupBounds(); ok {
		if handle := hitObjectHandle(bounds, a.zoom, ox, oy, mx, my); handle != ObjectBody {
			return a.objectSelected, handle
		}
	}
	return hitObjectAtClient(a.objects, a.zoom, ox, oy, mx, my)
}

func (a *App) beginObjectInteraction(mx, my int) {
	// Numeric toolbar edits were committed by onMouseDown. Object shortcuts
	// now belong to the canvas, not a previously focused child EDIT.
	if a.hwnd != 0 {
		procSetFocus.Call(a.hwnd)
	}
	i, handle := a.hitObjectClient(mx, my)
	if a.objectArmed != 0 && a.objectSelected >= 0 && a.objectSelected < len(a.objects) {
		i, handle = a.objectSelected, ObjectBody
		if a.objectArmed == objectArmResize {
			handle = ObjectSE
		}
	}
	armed := a.objectArmed
	a.objectArmed = 0
	if armed == 0 && keyDown(VK_SHIFT) {
		if i >= 0 {
			a.toggleObjectSelectionWithShift(i)
			a.statusText = fmt.Sprintf(localizeUI("Shift 다중 선택: %d개 선택. 선택 메뉴에서 묶을 수 있습니다."), len(a.selectedObjectIndices()))
			a.requestStatusRepaint()
		}
		a.requestObjectSelectionUIRepaint()
		return
	}
	if i < 0 {
		a.clearObjectSelection()
		a.requestObjectSelectionUIRepaint()
		return
	}
	a.clearSelectionState()
	if !a.objectSelectionHas(i) {
		a.objectIsolatedGroup = 0
		a.selectObjectWithGroup(i)
	}
	if len(a.selectedObjectIndices()) > 1 {
		if armed == 0 && handle != ObjectBody {
			// A member's personal handle cannot resize a group. Only the
			// surrounding group rectangle handles may resize all members.
			ox, oy := a.objectClientGeometry()
			if bounds, ok := a.objectGroupBounds(); ok {
				handle = hitObjectHandle(bounds, a.zoom, ox, oy, mx, my)
			}
		}
		if a.beginObjectGroupDrag(mx, my, handle) {
			a.requestObjectSelectionUIRepaint()
			return
		}
	}
	a.objectSelected = i
	a.objectDragHandle = handle
	a.objectDragging = true
	a.objectDragX, a.objectDragY = a.objectImagePoint(mx, my)
	a.objectDragStart = a.objects[i]
	a.objectDragStartX, a.objectDragStartY = a.objects[i].X, a.objects[i].Y
	procSetCapture.Call(a.hwnd)
	if a.hwnd != 0 && !a.ownsMouseCapture() {
		a.objectDragging = false
		a.objectDragStart = MovableObject{}
		return
	}
	logf("OBJECT begin index=%d handle=%d start=%d,%d size=%dx%d", i, handle, a.objectDragStart.X, a.objectDragStart.Y, a.objectDragStart.W, a.objectDragStart.H)
	a.statusText = localizeUI("객체 조작 중: 마우스를 놓으면 확정, Esc 또는 마우스 캡처 해제로 취소합니다.")
	if handle >= ObjectVertex0 {
		a.statusText = localizeUI("끝점·굽힘점 편집 중: 반대 끝점과 나머지 점은 고정됩니다. 놓으면 확정, Esc는 취소합니다.")
	}
	a.requestObjectSelectionUIRepaint()
	a.requestStatusRepaint()
}

func (a *App) updateObjectMove(x, y int) {
	if len(a.objectDragGroupStart) > 1 {
		a.updateObjectGroupDrag(x, y)
		return
	}
	if !a.objectDragging || a.objectSelected < 0 || a.objectSelected >= len(a.objects) {
		return
	}
	o := &a.objects[a.objectSelected]
	before := *o
	dx, dy := x-a.objectDragX, y-a.objectDragY
	if a.objectDragHandle == ObjectBody {
		o.X = clamp(a.objectDragStart.X+dx, -o.W+1, a.canvas.W-1)
		o.Y = clamp(a.objectDragStart.Y+dy, -o.H+1, a.canvas.H-1)
	} else if a.objectDragHandle >= ObjectVertex0 && vectorObjectKind(o.Kind) {
		next := a.objectDragStart
		i := int(a.objectDragHandle - ObjectVertex0)
		if i < next.Vector.Count {
			next.Vector.Points[i].X += dx
			next.Vector.Points[i].Y += dy
			next.Vector.Constrained = false
			if rendered, ok := renderVectorObject(next, a.canvas.W, a.canvas.H); ok {
				*o = rendered
			}
		}
	} else if b, ok := resizedObjectBounds(a.objectDragStart.bounds(), a.objectDragHandle, dx, dy); ok {
		o.X, o.Y, o.W, o.H = b.X, b.Y, b.W, b.H
	}
	a.repaintObjectChange(before, *o, false)
}

func (a *App) endObjectMove(x, y int, commit bool) {
	if len(a.objectDragGroupStart) > 1 {
		a.endObjectGroupDrag(x, y, commit)
		return
	}
	if !a.objectDragging {
		return
	}
	if commit {
		a.updateObjectMove(x, y)
	}
	// Clear BEFORE ReleaseCapture, which sends WM_CAPTURECHANGED synchronously.
	a.objectDragging = false
	if a.hwnd != 0 && a.ownsMouseCapture() {
		procReleaseCapture.Call()
	}
	if a.objectSelected < 0 || a.objectSelected >= len(a.objects) {
		return
	}
	o := &a.objects[a.objectSelected]
	changed := o.bounds() != a.objectDragStart.bounds() || o.Vector != a.objectDragStart.Vector
	if !commit {
		*o = a.objectDragStart
		changed = false
	}
	if changed && a.objectDragHandle < ObjectVertex0 && (o.W != a.objectDragStart.W || o.H != a.objectDragStart.H) {
		if !a.renderResizedObject(o) {
			*o = a.objectDragStart
			changed = false
		}
	}
	if changed {
		a.objectsRevision++
		a.markDirty()
		if a.objectDragHandle == ObjectBody {
			a.recordHistory("객체 이동")
		} else {
			if a.objectDragHandle >= ObjectVertex0 {
				a.recordHistory("객체 끝점·굽힘점 편집")
			} else {
				a.recordHistory("객체 크기 조절")
			}
		}
	}
	logf("OBJECT end index=%d commit=%v changed=%v handle=%d xy=%d,%d size=%dx%d", a.objectSelected, commit, changed, a.objectDragHandle, o.X, o.Y, o.W, o.H)
	a.objectDragStart = MovableObject{}
	a.objectArmed = 0
	a.statusText = fmt.Sprintf(localizeUI("객체 %d/%d: (%d,%d), %d×%d, 불투명도 %d%%. 우클릭으로 이동·크기·순서를 조절합니다."), a.objectSelected+1, len(a.objects), o.X, o.Y, o.W, o.H, 100-int(o.Transparency))
	a.requestCanvasRepaint(true)
	a.requestStatusRepaint()
}

// Only the dirty old/new bounds are invalidated during a drag. The raster
// canvas and every sprite are immutable until the one final commit.
func (a *App) repaintObjectChange(before, after MovableObject, immediate bool) {
	ox, oy := a.objectClientGeometry()
	r := unionRECT(objectOverlayRect(before, a.zoom, ox, oy), objectOverlayRect(after, a.zoom, ox, oy))
	a.requestRepaintRect(r, immediate)
}

func (a *App) renderResizedObject(o *MovableObject) bool {
	if !validObjectSize(o.W, o.H) {
		return false
	}
	if vectorObjectKind(o.Kind) {
		next := *o
		old := a.objectDragStart
		for i := 0; i < next.Vector.Count; i++ {
			p := old.Vector.Points[i]
			next.Vector.Points[i] = ObjectPoint{p.X * next.W / max(1, old.W), p.Y * next.H / max(1, old.H)}
		}
		rendered, ok := renderVectorObject(next, a.canvas.W, a.canvas.H)
		if ok {
			*o = rendered
		}
		return ok
	}
	if o.Kind == ToolText {
		oldFop, oldBop := a.fgOperation, a.bgOperation
		a.fgOperation, a.bgOperation = o.Operation, o.BackgroundOperation
		defer func() { a.fgOperation, a.bgOperation = oldFop, oldBop }()
		previous, fg, bg := a.textTool, a.fg, a.bg
		defer func() { a.textTool, a.fg, a.bg = previous, fg, bg }()
		a.textTool = TextState{X: o.X, Y: o.Y, W: o.W, H: o.H, Text: o.Text, Styles: append([]TextCharStyle(nil), o.Styles...), FontName: o.FontName, FontSize: o.FontSize, Bold: o.Bold, Italic: o.Italic, Underline: o.Underline, ScriptMode: o.ScriptMode, Opaque: o.Opaque}
		a.fg, a.bg = o.FG, o.BG
		next, ok := a.makeTextObject(o.Text)
		if !ok {
			return false
		}
		next.Transparency = o.Transparency
		next.GroupID = o.GroupID
		*o = next
		return true
	}
	col := o.FG
	if o.Operation == PaintInvert {
		col = 0xFFFFFF
	}
	c := &Canvas{W: o.W, H: o.H, Pix: make([]byte, o.W*o.H*4)}
	stroke := max(1, o.Stroke)
	switch o.Kind {
	case ToolRectangle:
		c.DrawRectOuter(0, 0, o.W-1, o.H-1, stroke, col)
	case ToolEllipse:
		c.DrawEllipseOuter(0, 0, o.W-1, o.H-1, stroke, col)
	case ToolRightTriangle:
		x0, x1 := resizedShapeAxis(o.ShapeX0, o.ShapeX1, o.ShapeW, o.W)
		y0, y1 := resizedShapeAxis(o.ShapeY0, o.ShapeY1, o.ShapeH, o.H)
		cx, cy := rightTriangleLegCorner(x0, y0, x1, y1)
		c.DrawStyledLine(x0, y0, x1, y1, stroke, false, col)
		c.DrawStyledLine(x0, y0, cx, cy, stroke, false, col)
		c.DrawStyledLine(cx, cy, x1, y1, stroke, false, col)
	default:
		return false
	}
	if o.ShowCenter && o.Kind != ToolRightTriangle {
		c.DrawCenterMarker(0, 0, o.W, o.H, col)
	}
	o.Pix, o.SpriteW, o.SpriteH = c.Pix, o.W, o.H
	return true
}

func (a *App) deleteSelectedObject() bool {
	if a.objectDragging {
		a.endObjectMove(0, 0, false)
	}
	i := a.objectSelected
	if a.tool != ToolObjectSelect || i < 0 || i >= len(a.objects) {
		return false
	}
	a.objectArmed = 0
	if ids := a.selectedObjectIndices(); len(ids) > 1 {
		for n := len(ids) - 1; n >= 0; n-- {
			j := ids[n]
			a.objects = append(a.objects[:j], a.objects[j+1:]...)
		}
	} else {
		a.objects = append(a.objects[:i], a.objects[i+1:]...)
	}
	a.objectsRevision++
	a.clearObjectSelection()
	a.markDirty()
	a.recordHistory("객체 삭제")
	a.requestCanvasRepaint(true)
	return true
}

func (a *App) beginEditingSelectedTextObject() bool {
	if a.objectDragging {
		a.endObjectMove(0, 0, false)
	}
	a.objectArmed = 0
	i := a.objectSelected
	if i < 0 || i >= len(a.objects) || len(a.selectedObjectIndices()) > 1 || a.objects[i].Kind != ToolText || a.dejavu != nil {
		return false
	}
	if a.textTool.Active {
		a.finishTextEditing(true)
	}
	o := a.objects[i]
	a.objectEditingIndex = i
	a.tool = ToolText
	a.showTextPanel = true
	a.toggleMenuCheck(ID_VIEW_TEXTBAR, true)
	a.ensureTextControls()
	a.fg, a.bg = o.FG, o.BG
	a.fgOperation, a.bgOperation = o.Operation, o.BackgroundOperation
	a.textTool.FontName = o.FontName
	a.textTool.FontSize = o.FontSize
	a.textTool.Bold = o.Bold
	a.textTool.Italic = o.Italic
	a.textTool.Underline = o.Underline
	a.textTool.ScriptMode = o.ScriptMode
	a.textTool.Opaque = o.Opaque
	a.textTool.Text = o.Text
	a.textTool.Styles = append([]TextCharStyle(nil), o.Styles...)
	a.beginTextEditing(o.X, o.Y, o.W, o.H)
	a.textTool.Text = o.Text
	a.textTool.Styles = append([]TextCharStyle(nil), o.Styles...)
	a.textTool.ControlSync = true
	procSetWindowTextW.Call(a.textTool.EditHwnd, uintptr(unsafe.Pointer(rawUTF16Ptr(o.Text))))
	a.textTool.ControlSync = false
	a.markTextLayoutDirty()
	a.statusText = localizeUI("텍스트 객체 다시 편집: 수정 후 바깥을 클릭하면 객체가 갱신됩니다. Esc는 변경 취소.")
	invalidate(a.hwnd)
	return true
}

func (a *App) flattenedObjectsCanvas() *Canvas {
	if len(a.objects) == 0 {
		return a.canvas
	}
	copyCanvas := &Canvas{W: a.canvas.W, H: a.canvas.H, Pix: a.canvas.ClonePixels()}
	compositeObjectsToPixels(copyCanvas.Pix, copyCanvas.W, copyCanvas.H, 0, 0, a.objects)
	return copyCanvas
}

// Full-image transformations or indexed palette conversion must not silently
// discard the visible movable layers. Flatten only on an accepted operation;
// prior history entries still retain their separate objects for Undo.
func (a *App) clearMovableObjectsWithoutHistory() {
	if len(a.objects) == 0 {
		return
	}
	a.objects = nil
	a.objectsRevision++
	a.clearObjectSelection()
	a.objectEditingIndex = -1
	a.objectDragging = false
	a.objectDragGroupStart = nil
	a.objectArmed = 0
	a.objectDragStart = MovableObject{}
}
func (a *App) flattenMovableObjectsIntoCanvas() {
	if len(a.objects) == 0 {
		return
	}
	a.canvas = a.flattenedObjectsCanvas()
	a.clearMovableObjectsWithoutHistory()
}

// rasterizeSelectedObject removes shape/text editability while preserving the
// exact compositing order. A lowest object can be merged into canvas pixels.
// Higher objects must become locked pixel-only layers: merging them directly
// into the base image would incorrectly draw them behind lower editable objects.
func (a *App) rasterizeSelectedObject() bool {
	if a.objectDragging {
		a.endObjectMove(0, 0, false)
	}
	i := a.objectSelected
	if i < 0 || i >= len(a.objects) || a.objects[i].Rasterized || a.textTool.Active {
		return false
	}
	// A non-bottom selection becomes a locked pixel-only layer. It is never
	// selected/edited again; its place in the stack preserves the appearance.
	o := &a.objects[i]
	if objectHasInversion(o) {
		*o = freezeInverseObject(a.canvas, a.objects, i)
	}
	o.Vector = ObjectVector{}
	o.Path = nil
	o.Rasterized = true
	o.Text, o.Styles, o.FontName = "", nil, ""
	o.ShapeW, o.ShapeH, o.ShapeX0, o.ShapeY0, o.ShapeX1, o.ShapeY1 = 0, 0, 0, 0, 0, 0
	o.Stroke, o.ShowCenter = 0, false
	// Consolidate consecutive locked pixel layers at the bottom of the stack
	// into genuine canvas pixels, preserving exact alpha and compositing order.
	prefix := 0
	for prefix < len(a.objects) && a.objects[prefix].Rasterized {
		prefix++
	}
	if prefix > 0 {
		copyCanvas := &Canvas{W: a.canvas.W, H: a.canvas.H, Pix: a.canvas.ClonePixels()}
		compositeObjectsToPixels(copyCanvas.Pix, copyCanvas.W, copyCanvas.H, 0, 0, a.objects[:prefix])
		a.canvas = copyCanvas
		a.objects = a.objects[prefix:]
	}
	a.objectSelected = -1
	a.objectArmed = 0
	a.objectsRevision++
	a.markDirty()
	a.recordHistory("객체 래스터화")
	a.statusText = localizeUI("선택 객체를 래스터화했습니다. 편집 핸들은 제거되며 Ctrl+Z로 되돌릴 수 있습니다.")
	logf("OBJECT rasterize index=%d remaining-layers=%d", i, len(a.objects))
	a.requestCanvasRepaint(true)
	a.requestStatusRepaint()
	return true
}

func objectOverlayRect(o MovableObject, z, ox, oy int) RECT {
	l, t, r, b := ox+o.X*z, oy+o.Y*z, ox+(o.X+o.W)*z, oy+(o.Y+o.H)*z
	if vectorObjectKind(o.Kind) {
		for _, p := range objectVertexPoints(&o, z, ox, oy) {
			l = min(l, p.X)
			t = min(t, p.Y)
			r = max(r, p.X)
			b = max(b, p.Y)
		}
	}
	return RECT{int32(l - 10), int32(t - 10), int32(r + 11), int32(b + 11)}
}
