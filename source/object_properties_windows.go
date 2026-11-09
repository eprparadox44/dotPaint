//go:build windows

package main

import (
	"fmt"
	"unsafe"
)

// requestObjectSelectionUIRepaint invalidates both the canvas and the floating
// toolbox in one explicit GUI refresh. The selected object's options are
// synchronized immediately, but requestCanvasRepaint alone cannot repaint the
// toolbox; previously the old options remained visible until the next click,
// screenshot, or unrelated WM_PAINT. This is used only on selection events,
// never during high-frequency dragging.
func (a *App) requestObjectSelectionUIRepaint() {
	if a.hwnd == 0 {
		return
	}
	if a.showToolbox && !a.bitmapView {
		var client RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
		panel := a.toolboxPanelRect(client)
		if !rectEmpty(panel) {
			invalidateArea(a.hwnd, &panel)
		}
	}
	a.requestCanvasRepaint(false)
	// Selection is a semantic boundary, not an animation frame. Do not throttle
	// the first paint after a click with the normal 12 ms drag-preview guard.
	procUpdateWindow.Call(a.hwnd)
}

// The Object Selection tool reuses the original shape-option panel for a
// single selected object. Group selections expose only group geometry.
func (a *App) selectedObjectOptionTool() Tool {
	if a.tool != ToolObjectSelect || len(a.selectedObjectIndices()) != 1 {
		return a.tool
	}
	o := a.selectedObject()
	if o == nil || o.Rasterized {
		return a.tool
	}
	return o.Kind
}

func (a *App) syncSelectedObjectOptions() {
	if a.tool != ToolObjectSelect || len(a.selectedObjectIndices()) != 1 {
		return
	}
	o := a.selectedObject()
	if o == nil || o.Rasterized {
		return
	}
	switch o.Kind {
	case ToolLine:
		a.lineSize = o.Stroke
		a.lineAntialias = o.Vector.Antialias
		a.lineConstraintMode = o.Vector.Constraint
	case ToolCurve:
		a.curveSize = o.Stroke
		a.curveAntialias = o.Vector.Antialias
		a.curveBendCount = clamp(o.Vector.Count-2, 1, 4)
	case ToolArrow:
		a.arrowLineSize = o.Stroke
		a.arrowHeadSize = o.Vector.ArrowHead
		a.arrowAntialias = o.Vector.Antialias
		a.lineConstraintMode = o.Vector.Constraint
	case ToolRectangle:
		a.rectangleSize = o.Stroke
		a.shapeShowCenter = o.ShowCenter
	case ToolEllipse:
		a.ellipseSize = o.Stroke
		a.shapeShowCenter = o.ShowCenter
	case ToolRightTriangle:
		a.rightTriangleSize = o.Stroke
		a.lineConstraintMode = o.Vector.Constraint
	}
}

func (a *App) beginSelectedObjectOptionDrag() {
	if a.selectedObjectOptionTool() == ToolObjectSelect || a.objectOptionEditing {
		return
	}
	i := a.objectSelected
	if i < 0 || i >= len(a.objects) {
		return
	}
	a.objectOptionStart = a.objects[i]
	a.objectOptionIndex = i
	a.objectOptionEditing = true
}

// No history entry is generated for individual slider pixels. Commit once at
// mouse-up and keep the rendering cache immutable for existing snapshots.
func (a *App) applySelectedObjectToolOptions(commit bool) bool {
	if a.tool != ToolObjectSelect || len(a.selectedObjectIndices()) != 1 || a.canvas == nil {
		return false
	}
	i := a.objectSelected
	if i < 0 || i >= len(a.objects) {
		return false
	}
	before := a.objects[i]
	next := before
	switch next.Kind {
	case ToolLine:
		next.Stroke = a.lineSize
		next.Vector.Antialias = a.lineAntialias
		next.Vector.Constraint = a.lineConstraintMode
	case ToolCurve:
		next.Stroke = a.curveSize
		next.Vector.Antialias = a.curveAntialias
	case ToolArrow:
		next.Stroke = a.arrowLineSize
		next.Vector.Antialias = a.arrowAntialias
		next.Vector.ArrowHead = a.arrowHeadSize
		next.Vector.Constraint = a.lineConstraintMode
	case ToolRectangle:
		next.Stroke = a.rectangleSize
		next.ShowCenter = a.shapeShowCenter
	case ToolEllipse:
		next.Stroke = a.ellipseSize
		next.ShowCenter = a.shapeShowCenter
	case ToolRightTriangle:
		next.Stroke = a.rightTriangleSize
	default:
		return false
	}
	if next.Stroke < 1 {
		next.Stroke = 1
	}
	if next.Stroke == before.Stroke && next.Vector == before.Vector && next.ShowCenter == before.ShowCenter {
		return false
	}
	var ok bool
	if vectorObjectKind(next.Kind) {
		next, ok = renderVectorObject(next, a.canvas.W, a.canvas.H)
	} else {
		oldDrag := a.objectDragStart
		a.objectDragStart = before
		ok = a.renderResizedObject(&next)
		a.objectDragStart = oldDrag
	}
	if !ok {
		return false
	}
	a.objects[i] = next
	if commit {
		a.objectsRevision++
		a.markDirty()
		a.recordHistory("객체 속성 변경")
		a.statusText = localizeUI("선택 객체의 도형 속성을 변경했습니다.")
		a.requestStatusRepaint()
	}
	a.requestCanvasRepaint(true)
	return true
}

func (a *App) endSelectedObjectOptionDrag() {
	if !a.objectOptionEditing {
		return
	}
	i := a.objectOptionIndex
	a.objectOptionEditing = false
	if i < 0 || i >= len(a.objects) {
		return
	}
	if !objectsEqual([]MovableObject{a.objectOptionStart}, []MovableObject{a.objects[i]}) {
		a.objectsRevision++
		a.markDirty()
		a.recordHistory("객체 속성 변경")
		a.statusText = localizeUI("선택 객체의 도형 속성을 변경했습니다.")
		a.requestStatusRepaint()
	}
	a.objectOptionStart = MovableObject{}
}

// Text boxes selected outside the typing editor can change their opaque
// background using the same existing toolbox control, without leaving Object
// Selection or losing per-character styles.
func (a *App) setSelectedTextObjectOpaque(opaque bool) bool {
	if a.tool != ToolObjectSelect || len(a.selectedObjectIndices()) != 1 || a.canvas == nil {
		return false
	}
	o := a.selectedObject()
	if o == nil || o.Rasterized || o.Kind != ToolText || o.Opaque == opaque {
		return false
	}
	before := *o
	next := before
	next.Opaque = opaque
	oldDrag := a.objectDragStart
	a.objectDragStart = before
	ok := a.renderResizedObject(&next)
	a.objectDragStart = oldDrag
	if !ok {
		return false
	}
	*o = next
	a.objectsRevision++
	a.markDirty()
	a.recordHistory("텍스트 객체 배경")
	a.statusText = localizeUI("선택 텍스트 상자의 배경 표시를 변경했습니다.")
	a.requestCanvasRepaint(true)
	a.requestStatusRepaint()
	return true
}

func (a *App) changeSelectedObjectColor(col uint32, op PaintOperation, background bool) bool {
	if a.tool != ToolObjectSelect || len(a.selectedObjectIndices()) != 1 || a.canvas == nil {
		return false
	}
	o := a.selectedObject()
	if o == nil || o.Rasterized {
		return false
	}
	if background && o.Kind != ToolText {
		return false
	}
	before := *o
	next := before
	if background {
		next.BG = col
		next.BackgroundOperation = op
	} else {
		next.FG = col
		next.Operation = op
		if next.Kind == ToolText {
			styles := append([]TextCharStyle(nil), next.Styles...)
			for k := range styles {
				styles[k].Color, styles[k].ColorSet, styles[k].ColorOp = col, true, op
			}
			next.Styles = styles
		}
	}
	if before.FG == next.FG && before.BG == next.BG && before.Operation == next.Operation && before.BackgroundOperation == next.BackgroundOperation {
		return false
	}
	if objectHasInversion(&next) {
		next.Transparency = 0
	}
	var ok bool
	if vectorObjectKind(next.Kind) {
		next, ok = renderVectorObject(next, a.canvas.W, a.canvas.H)
	} else {
		oldDrag := a.objectDragStart
		a.objectDragStart = before
		ok = a.renderResizedObject(&next)
		a.objectDragStart = oldDrag
	}
	if !ok {
		return false
	}
	*o = next
	a.objectsRevision++
	a.markDirty()
	a.recordHistory("객체 색상 변경")
	a.statusText = fmt.Sprintf(localizeUI("선택 객체 색상을 변경했습니다. (%s)"), localizeUI(toolNames[next.Kind]))
	a.requestCanvasRepaint(true)
	a.requestStatusRepaint()
	return true
}
