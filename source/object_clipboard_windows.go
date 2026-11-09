//go:build windows

package main

import (
	"errors"
	"fmt"
	"unsafe"
)

var clipboardEditableObjectFormat uintptr
var clipboardSVGFormat uintptr

func ensureEditableClipboardFormats() {
	if clipboardEditableObjectFormat == 0 {
		name := utf16Ptr(objectClipboardFormatName)
		clipboardEditableObjectFormat, _, _ = procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(name)))
	}
	if clipboardSVGFormat == 0 {
		name := utf16Ptr("image/svg+xml")
		clipboardSVGFormat, _, _ = procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(name)))
	}
}

type editableClipboardExport struct {
	Native []byte
	SVG    []byte
}

func (a *App) selectedObjectsForClipboard() (editableClipboardExport, error) {
	ids := a.selectedObjectIndices()
	packet, err := makeObjectClipboardPacket(a.objects, ids)
	if err != nil {
		return editableClipboardExport{}, err
	}
	native, err := encodeObjectClipboard(packet)
	if err != nil {
		return editableClipboardExport{}, err
	}
	visual := make([]MovableObject, 0, len(ids))
	canSVG := true
	for _, i := range ids {
		o := a.objects[i]
		if objectHasInversion(&o) || len(o.ReplaceMask) > 0 || o.Transparency > 0 {
			// SVG's independent images cannot preserve dynamic inversion or
			// replacement-mask semantics on arbitrary foreign backgrounds.
			canSVG = false
		}
		visual = append(visual, o)
	}
	result := editableClipboardExport{Native: native}
	if canSVG {
		svg, svgErr := buildObjectClipboardSVG(packet, visual)
		if svgErr == nil {
			result.SVG = svg
		} else {
			logf("CLIPBOARD SVG skipped: %v", svgErr)
		}
	}
	return result, nil
}

// Return an explicit error for malformed/untrusted clipboard formats; callers
// should fall back to PNG/DIB when native data is absent or unsupported.
func readEditableObjectClipboard() (objectClipboardPacket, error) {
	ensureEditableClipboardFormats()
	if clipboardEditableObjectFormat == 0 {
		return objectClipboardPacket{}, errors.New("객체 클립보드 형식을 등록하지 못했습니다")
	}
	if !openClipboardRetry(0) {
		return objectClipboardPacket{}, errors.New("Windows 클립보드를 열지 못했습니다")
	}
	defer procCloseClipboard.Call()
	if ok, _, _ := procIsClipboardFormatAvailable.Call(clipboardEditableObjectFormat); ok == 0 {
		return objectClipboardPacket{}, errors.New("dotPaint 객체 데이터 없음")
	}
	h, _, _ := procGetClipboardData.Call(clipboardEditableObjectFormat)
	if h == 0 {
		return objectClipboardPacket{}, errors.New("dotPaint 객체 클립보드 핸들 없음")
	}
	size, _, _ := procGlobalSize.Call(h)
	if size == 0 || size > objectClipboardMaxPacked {
		return objectClipboardPacket{}, errors.New("dotPaint 객체 데이터가 너무 큽니다")
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return objectClipboardPacket{}, errors.New("dotPaint 객체 메모리를 잠글 수 없습니다")
	}
	data := append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(ptr)), int(size))...)
	procGlobalUnlock.Call(h)
	return decodeObjectClipboard(data)
}

// Paste is a single undoable operation. New group IDs never collide with the
// destination document; editable objects retain original shape/text/vector
// metadata, including their dynamic inversion, and are not rasterized.
func (a *App) pasteEditableObjects(packet objectClipboardPacket) bool {
	if a.canvas == nil || a.dejavu != nil || a.indexedMode || a.rasterMode {
		return false
	}
	x, y := a.viewportTopLeftImage()
	next, ids, groupID, err := pasteObjectsFromPacket(packet, x, y, a.objects, a.objectNextGroupID)
	if err != nil {
		logf("CLIPBOARD object paste invalid err=%v", err)
		return false
	}
	oldW, oldH := a.canvas.W, a.canvas.H
	newW, newH, grew, canFit := canvasSizeForClipboardPaste(oldW, oldH, x, y, packet.Width, packet.Height)
	if grew {
		a.canvas.ResizeCanvas(newW, newH, 0, 0, a.bg)
		a.updateScrollbars()
	}
	if !canFit {
		logf("CLIPBOARD objects outside auto growth limit: %dx%d", packet.Width, packet.Height)
	}
	a.clearSelectionState()
	a.objects = next
	a.objectNextGroupID = groupID
	a.objectsRevision++
	a.objectArmed = 0
	a.objectMultiSelected = nil
	a.objectIsolatedGroup = 0
	a.objectSelected = ids[len(ids)-1]
	if len(ids) > 1 {
		a.objectMultiSelected = make(map[int]bool, len(ids))
		for _, idx := range ids {
			a.objectMultiSelected[idx] = true
		}
	}
	a.tool = ToolObjectSelect
	a.syncSelectedObjectOptions()
	a.markDirty()
	a.recordHistory("객체 붙여넣기")
	a.statusText = fmt.Sprintf("편집 가능한 객체 %d개를 붙였습니다. 위치·크기·색상·반전색·그룹을 다시 편집할 수 있습니다.", len(ids))
	a.requestObjectSelectionUIRepaint()
	a.requestCanvasRepaint(true)
	logf("CLIPBOARD native-object paste n=%d at=%d,%d grew=%v groups=%d", len(ids), x, y, grew, groupID)
	return true
}

func (a *App) pasteHistoryClipboard(index int) bool {
	if index < 0 || index >= len(a.clipboardHistory) {
		return false
	}
	entry := a.clipboardHistory[index]
	if len(entry.EditableObjectData) > 0 && a.dejavu == nil && !a.indexedMode && !a.rasterMode {
		if packet, err := decodeObjectClipboard(entry.EditableObjectData); err == nil {
			return a.pasteEditableObjects(packet)
		} else {
			logf("CLIPBOARD history native-object decode failed: %v", err)
		}
	}
	return a.pasteClipboardImage(entry.Image, fmt.Sprintf("클립보드 기록 %d", index+1))
}
