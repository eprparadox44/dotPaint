//go:build windows

package main

import "fmt"

func (a *App) selectedObjectIndices() []int {
	if a.objectSelected < 0 || a.objectSelected >= len(a.objects) || a.objects[a.objectSelected].Rasterized {
		return nil
	}
	if len(a.objectMultiSelected) == 0 {
		return []int{a.objectSelected}
	}
	indices := make([]int, 0, len(a.objectMultiSelected))
	for i := range a.objects {
		if a.objectMultiSelected[i] && !a.objects[i].Rasterized {
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		return []int{a.objectSelected}
	}
	return indices
}

func (a *App) objectSelectionHas(index int) bool {
	if a.objectSelected < 0 {
		return false
	}
	if len(a.objectMultiSelected) == 0 {
		return index == a.objectSelected
	}
	return a.objectMultiSelected[index]
}

func (a *App) clearObjectSelection() {
	a.objectSelected = -1
	a.objectMultiSelected = nil
	a.objectIsolatedGroup = 0
}

func (a *App) selectObjectWithGroup(i int) {
	if i < 0 || i >= len(a.objects) || a.objects[i].Rasterized {
		a.clearObjectSelection()
		return
	}
	ids := expandGroupSelection(a.objects, i, a.objectIsolatedGroup)
	a.objectSelected = i
	if len(ids) <= 1 {
		a.objectMultiSelected = nil
		a.syncSelectedObjectOptions()
		return
	}
	a.objectMultiSelected = make(map[int]bool, len(ids))
	for _, n := range ids {
		a.objectMultiSelected[n] = true
	}
}

// A Shift-click adds or removes an entire existing group. In group-isolation
// mode it adds only the individual clicked member.
func (a *App) toggleObjectSelectionWithShift(i int) {
	if i < 0 || i >= len(a.objects) || a.objects[i].Rasterized {
		return
	}
	ids := expandGroupSelection(a.objects, i, a.objectIsolatedGroup)
	if a.objectMultiSelected == nil {
		a.objectMultiSelected = make(map[int]bool)
	}
	if a.objectSelected >= 0 && len(a.objectMultiSelected) == 0 {
		a.objectMultiSelected[a.objectSelected] = true
	}
	all := true
	for _, n := range ids {
		if !a.objectMultiSelected[n] {
			all = false
			break
		}
	}
	for _, n := range ids {
		if all {
			delete(a.objectMultiSelected, n)
		} else {
			a.objectMultiSelected[n] = true
		}
	}
	if len(a.objectMultiSelected) == 0 {
		a.clearObjectSelection()
		return
	}
	if !all || !a.objectMultiSelected[a.objectSelected] {
		a.objectSelected = i
	}
	if !a.objectMultiSelected[a.objectSelected] {
		for n := range a.objectMultiSelected {
			a.objectSelected = n
			break
		}
	}
	if len(a.objectMultiSelected) == 1 {
		a.objectMultiSelected = nil
		a.syncSelectedObjectOptions()
	}
}

func (a *App) groupSelectedObjects() bool {
	ids := a.selectedObjectIndices()
	if len(ids) < 2 {
		return false
	}
	a.objectNextGroupID++
	// A redo/undo can restore a group identifier larger than the counter.
	for _, o := range a.objects {
		if o.GroupID >= a.objectNextGroupID {
			a.objectNextGroupID = o.GroupID + 1
		}
	}
	id := a.objectNextGroupID
	if !assignObjectGroup(a.objects, ids, id) {
		return false
	}
	a.objectIsolatedGroup = 0
	a.objectsRevision++
	a.markDirty()
	a.recordHistory("객체 선택 묶기")
	a.statusText = fmt.Sprintf(localizeUI("%d개 객체를 묶었습니다. 더블클릭하면 내부 객체를 편집합니다."), len(ids))
	a.requestObjectSelectionUIRepaint()
	a.requestStatusRepaint()
	return true
}

func (a *App) ungroupSelectedObjects() bool {
	o := a.selectedObject()
	if o == nil || o.GroupID == 0 {
		return false
	}
	id := o.GroupID
	if !dissolveObjectGroup(a.objects, id) {
		return false
	}
	a.objectIsolatedGroup = 0
	a.objectMultiSelected = nil
	a.objectsRevision++
	a.markDirty()
	a.recordHistory("객체 묶기 해제")
	a.statusText = localizeUI("객체 묶기를 해제했습니다.")
	a.requestObjectSelectionUIRepaint()
	a.requestStatusRepaint()
	return true
}

func (a *App) objectGroupBounds() (ObjectBounds, bool) {
	ids := a.selectedObjectIndices()
	if len(ids) < 2 {
		return ObjectBounds{}, false
	}
	return selectedGroupBounds(a.objects, ids)
}

func (a *App) beginObjectGroupDrag(mx, my int, handle ObjectHandle) bool {
	ids := a.selectedObjectIndices()
	if len(ids) < 2 {
		return false
	}
	bounds, ok := selectedGroupBounds(a.objects, ids)
	if !ok {
		return false
	}
	a.objectDragGroupStart = make(map[int]MovableObject, len(ids))
	for _, i := range ids {
		a.objectDragGroupStart[i] = a.objects[i]
	}
	a.objectDragGroupBounds = bounds
	a.objectDragHandle = handle
	a.objectDragging = true
	a.objectDragX, a.objectDragY = a.objectImagePoint(mx, my)
	procSetCapture.Call(a.hwnd)
	if a.hwnd != 0 && !a.ownsMouseCapture() {
		a.objectDragging = false
		a.objectDragGroupStart = nil
		return false
	}
	a.statusText = localizeUI("그룹 조작 중: 이동하거나 전체 크기를 조절합니다. 놓으면 확정, Esc는 취소합니다.")
	a.requestCanvasRepaint(true)
	a.requestStatusRepaint()
	return true
}

func (a *App) updateObjectGroupDrag(x, y int) {
	if len(a.objectDragGroupStart) < 2 || a.canvas == nil {
		return
	}
	dx, dy := x-a.objectDragX, y-a.objectDragY
	old := a.objectDragGroupBounds
	if a.objectDragHandle == ObjectBody {
		// The group follows one translation; never clamp children independently.
		dx = clamp(dx, -old.X-old.W+1, a.canvas.W-1-old.X)
		dy = clamp(dy, -old.Y-old.H+1, a.canvas.H-1-old.Y)
		for i, o := range a.objectDragGroupStart {
			n := o
			n.X += dx
			n.Y += dy
			a.objects[i] = n
		}
	} else {
		b, ok := resizedObjectBounds(old, a.objectDragHandle, dx, dy)
		if !ok {
			return
		}
		for i, o := range a.objectDragGroupStart {
			scaled, ok := scaleObjectBoundsWithinGroup(o.bounds(), old, b)
			if !ok {
				return
			}
			n := o
			n.X, n.Y, n.W, n.H = scaled.X, scaled.Y, scaled.W, scaled.H
			a.objects[i] = n
		}
	}
	// This operation is only active in Object Selection. The heavyweight
	// protected field/line/3D drawing engines have no new event-loop work.
	a.requestCanvasRepaint(true)
}

func (a *App) endObjectGroupDrag(x, y int, commit bool) {
	if len(a.objectDragGroupStart) < 2 {
		return
	}
	if commit {
		a.updateObjectGroupDrag(x, y)
	}
	a.objectDragging = false
	if a.hwnd != 0 && a.ownsMouseCapture() {
		procReleaseCapture.Call()
	}
	backup := a.objectDragGroupStart
	a.objectDragGroupStart = nil
	changed := false
	if commit {
		for i, old := range backup {
			if i < 0 || i >= len(a.objects) {
				continue
			}
			now := a.objects[i]
			if now.bounds() != old.bounds() {
				changed = true
			}
			if a.objectDragHandle != ObjectBody && (now.W != old.W || now.H != old.H) {
				prev := a.objectDragStart
				a.objectDragStart = old
				if !a.renderResizedObject(&now) {
					commit = false
				} else {
					a.objects[i] = now
				}
				a.objectDragStart = prev
			}
		}
	}
	if !commit {
		for i, old := range backup {
			if i >= 0 && i < len(a.objects) {
				a.objects[i] = old
			}
		}
		changed = false
	}
	if changed {
		a.objectsRevision++
		a.markDirty()
		if a.objectDragHandle == ObjectBody {
			a.recordHistory("객체 그룹 이동")
		} else {
			a.recordHistory("객체 그룹 크기 조절")
		}
	}
	a.objectDragStart = MovableObject{}
	a.objectArmed = 0
	a.requestCanvasRepaint(true)
	a.requestStatusRepaint()
}
