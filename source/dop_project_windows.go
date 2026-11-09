//go:build windows

package main

import (
	"fmt"
	"path/filepath"
)

func (a *App) currentDOPProject() *DOPProject {
	p := &DOPProject{
		Canvas: a.canvas, Objects: a.objects,
		Palette:          append([]uint32(nil), a.palette...),
		IndexedColors:    a.indexedColors,
		IndexedColors256: append([]uint32(nil), a.indexedColors256...),
		StandardPalette:  append([]uint32(nil), a.standardPalette...),
		CustomColors:     a.customColors,
		IndexedMode:      a.indexedMode, IndexedSize: a.indexedSize, IndexedCount: a.indexedActiveCount,
		Foreground: a.fg, Background: a.bg, ForegroundOp: a.fgOperation, BackgroundOp: a.bgOperation,
		NextGroupID: a.objectNextGroupID,
	}
	if a.indexedMode {
		p.IndexedPixels = indexedIndicesForSnapshot(a.canvas.Pix, a.palette, a.indexedActiveCount, a.indexedPixelIndices)
		// The active palette is authoritative; cached color lists may lag edits.
		if a.indexedSize == 16 {
			p.IndexedColors = indexedPaletteFromRefs(a.palette)
		}
		if a.indexedSize == 256 {
			p.IndexedColors256 = append([]uint32(nil), a.palette...)
		}
	}
	return p
}

func (a *App) loadDOPDocument(path string) bool {
	p, err := ReadDOPProject(path)
	if err != nil {
		messageBox(a.hwnd, localizeUI("DOP 파일을 열 수 없습니다.\n\n")+err.Error(), "dotPaint", MB_OK|MB_ICONERROR)
		return false
	}
	// Opening an editable project must never silently erase its objects.
	// Leave raster mode before installing the newly read DOP scene; the user
	// can explicitly enable it again and accept the destructive warning.
	wasRasterMode := a.rasterMode
	if wasRasterMode {
		// Loading a new document resets document history below. Switching off
		// raster-only mode here is not a user edit: avoid allocating a redundant
		// giant history snapshot for the old document just before discarding it.
		a.rasterMode = false
		a.keepObjects = a.keepObjectsBeforeRasterMode
	}
	// The project is fully validated before changing any open-document state.
	a.leaveDejavuIMGMode()
	a.clearSelectionState()
	a.clearObjectOutlines()
	a.canvas = p.Canvas
	a.objects = p.Objects
	a.objectsRevision++
	a.objectNextGroupID = p.NextGroupID
	for _, o := range a.objects {
		if o.GroupID >= a.objectNextGroupID {
			a.objectNextGroupID = o.GroupID + 1
		}
	}
	a.clearObjectSelection()
	a.objectEditingIndex = -1
	a.objectDragging = false
	a.objectArmed = 0
	a.objectContextPending = false
	a.objectDragGroupStart = nil
	a.palette = append([]uint32(nil), p.Palette...)
	a.indexedColors = indexed16Palette(p.IndexedColors)
	a.indexedColors256 = append([]uint32(nil), p.IndexedColors256...)
	a.standardPalette = append([]uint32(nil), p.StandardPalette...)
	a.customColors = p.CustomColors
	a.indexedMode = p.IndexedMode
	a.indexedSize = p.IndexedSize
	a.indexedActiveCount = p.IndexedCount
	a.indexedPixelIndices = append([]byte(nil), p.IndexedPixels...)
	a.indexedPalettePage = 0
	a.indexedInvertLUT = nil
	a.indexedInvertPaletteHash = 0
	a.fg, a.bg = p.Foreground, p.Background
	a.fgOperation, a.bgOperation = p.ForegroundOp, p.BackgroundOp
	a.filePath = path
	a.scrollX, a.scrollY = 0, 0
	a.undo.Reset()
	a.resetHistory("DOP 열기")
	a.clearDirty()
	a.addRecent(path)
	a.updateScrollbars()
	if a.hwnd != 0 {
		a.rebuildMenuForTheme()
		invalidate(a.hwnd)
	}
	a.statusText = fmt.Sprintf(localizeUI("DOP 편집 프로젝트를 불러왔습니다: %s (객체 %d개)"), filepath.Base(path), len(a.objects))
	if wasRasterMode {
		a.statusText += localizeUI(" DOP 객체 보존을 위해 래스터화 모드를 자동 해제했습니다.")
	}
	return true
}

// A DOP document can contain locked raster layers, which are kept in the
// project. Only editable objects require prompting before flattening to PNG.
func (a *App) mustChooseDOPSaveTarget() bool {
	return a.filePath == "" || (!isDOPPath(a.filePath) && (hasEditableDOPObjects(a.objects) || (a.textTool.Active && !a.indexedMode && (a.keepObjects || a.objectEditingIndex >= 0))))
}
