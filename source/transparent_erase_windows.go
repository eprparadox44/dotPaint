//go:build windows

package main

// The geometry engine has already rendered the eraser into a tightly cropped
// sprite. It is never retained as a moveable transparent shape.
func (a *App) applyTransparentShapeMask(mask MovableObject) bool {
	if a.canvas == nil || a.dejavu != nil || a.indexedMode || !validObjectSize(mask.W, mask.H) {
		return false
	}
	updated, changed := eraseCompositedPixels(a.canvas, a.objects,
		&Canvas{W: mask.W, H: mask.H, Pix: mask.Pix}, mask.X, mask.Y)
	if changed {
		a.objects = updated
		a.objectsRevision++
		a.clearObjectSelection()
		a.statusText = localizeUI("투명색으로 실제 픽셀을 지웠습니다. 영향을 받은 객체만 부분 래스터화됩니다.")
	}
	return changed
}
