//go:build windows

package main

import (
	"runtime"
	"syscall"
	"unsafe"
)

// TASKDIALOGCONFIG / TASKDIALOG_BUTTON match commctrl.h (Windows x64).
// A native TaskDialog provides the user's exact two button captions while
// remaining modal, keyboard-accessible, and theme-independent.
type rasterTaskDialogButton struct {
	ID   int32
	Text *uint16
}
type rasterTaskDialogConfig struct {
	Size                 uint32
	Parent               uintptr
	Instance             uintptr
	Flags                uint32
	CommonButtons        uint32
	WindowTitle          *uint16
	MainIcon             uintptr
	MainInstruction      *uint16
	Content              *uint16
	ButtonCount          uint32
	Buttons              *rasterTaskDialogButton
	DefaultButtonID      int32
	RadioButtonCount     uint32
	RadioButtons         uintptr
	DefaultRadioButtonID int32
	VerificationText     uintptr
	ExpandedInformation  uintptr
	ExpandedControlText  uintptr
	CollapsedControlText uintptr
	FooterIcon           uintptr
	Footer               uintptr
	Callback             uintptr
	CallbackData         uintptr
	Width                uint32
}

var rasterTaskDialogProc = syscall.NewLazyDLL("comctl32.dll").NewProc("TaskDialogIndirect")

func confirmEnableRasterMode(hwnd uintptr) bool {
	title := utf16Ptr("dotPaint")
	instruction := utf16Ptr(localizeUI("주의! 활성화하면 객체가 완전히 사라집니다."))
	proceed := utf16Ptr(localizeUI("그래도 해요"))
	cancel := utf16Ptr(localizeUI("취소 할게요"))
	buttons := [2]rasterTaskDialogButton{{ID: 1001, Text: proceed}, {ID: 1002, Text: cancel}}
	cfg := rasterTaskDialogConfig{
		Parent: hwnd, WindowTitle: title,
		// TD_WARNING_ICON = MAKEINTRESOURCEW(-1). A real warning icon is
		// independent of the user's light/dark mode.
		MainIcon:        0xffff,
		MainInstruction: instruction,
		Buttons:         &buttons[0], ButtonCount: uint32(len(buttons)),
		DefaultButtonID: 1002,
		Flags:           0x0008, // TDF_ALLOW_DIALOG_CANCELLATION
	}
	cfg.Size = uint32(unsafe.Sizeof(cfg))
	if err := rasterTaskDialogProc.Find(); err == nil {
		var selected int32
		hresult, _, _ := rasterTaskDialogProc.Call(
			uintptr(unsafe.Pointer(&cfg)), uintptr(unsafe.Pointer(&selected)), 0, 0)
		runtime.KeepAlive(buttons)
		runtime.KeepAlive(title)
		runtime.KeepAlive(instruction)
		if hresult == 0 { // S_OK
			return selected == 1001
		}
		logf("RASTER mode TaskDialogIndirect failed hresult=%#x", hresult)
	}
	// When common controls v6 is unavailable, use an exact-caption modal window.
	return confirmRasterModeFallback(hwnd)
}

// restoreStartupRasterMode restores a *preference*, not a user drawing action.
// This must be called on the new blank document, before opening any file.
// Do not call enterRasterMode: it would show up as an extra Undo action.
func (a *App) restoreStartupRasterMode(enabled bool) {
	if !enabled {
		return
	} // fresh-document default is editable-object mode
	a.keepObjectsBeforeRasterMode = a.keepObjects
	a.keepObjects = false
	a.rasterMode = true
	// The single starting history snapshot must agree with the UI; otherwise
	// Undo could unexpectedly restore the opposite mode on a fresh document.
	a.resetHistory("시작")
}

// UI-initiated mode toggles persist immediately, just like the theme choice;
// Windows shutdown also saves the effective mode after history navigation.
func (a *App) saveRasterModePreferenceAfterToggle() {
	if err := saveRasterModePreference(a.rasterMode); err != nil {
		logf("RASTER preference save after toggle failed enabled=%v err=%v", a.rasterMode, err)
	} else {
		logf("RASTER preference saved after toggle enabled=%v", a.rasterMode)
	}
}

// enterRasterMode has no confirmation UI; the caller MUST confirm first.
// Raster mode is a reversible, named history step: the previous snapshot
// restores the editable objects and their dynamic inversion on Ctrl+Z.
// Only the low-level temporary preview stack is reset after the transition.
func (a *App) enterRasterMode() {
	if a.rasterMode {
		return
	}
	if a.curveUndoPending {
		a.finishCurveEarlyOrCancel("rasterization-mode")
	}
	if a.textTool.Active || a.textTool.Editing {
		a.finishTextEditing(true)
	}
	if a.objectDragging {
		a.endObjectMove(0, 0, false)
	}
	hadObjects := len(a.objects) != 0
	a.flattenMovableObjectsIntoCanvas() // preserve Z-order, inversion and alpha
	// A floating selection's BasePix must not roll back the newly flattened
	// canvas on the next drag, so commit its current visible pixels and close it.
	a.clearSelectionState()
	a.keepObjectsBeforeRasterMode = a.keepObjects
	a.keepObjects = false
	a.rasterMode = true
	a.undo.Reset() // temporary Esc/cancel snapshots only; NEVER clear named history
	a.recordHistory("래스터화 모드 켜기")
	if hadObjects {
		a.markDirty()
	}
	a.statusText = localizeUI("래스터화 모드 켬: 기존 객체를 픽셀로 합쳤으며, 새 그림은 객체를 만들지 않습니다.")
	logf("RASTER mode enabled flattened=%v objects=%d", hadObjects, len(a.objects))
	a.refreshRasterModeUI()
}

func (a *App) leaveRasterMode() {
	if !a.rasterMode {
		return
	}
	a.rasterMode = false
	a.keepObjects = a.keepObjectsBeforeRasterMode
	a.recordHistory("래스터화 모드 끄기")
	a.statusText = localizeUI("래스터화 모드 끔: 새 객체 생성을 다시 사용할 수 있습니다. 히스토리에서 이전 객체 상태로 되돌릴 수도 있습니다.")
	logf("RASTER mode disabled keepObjects=%v", a.keepObjects)
	a.refreshRasterModeUI()
}

func (a *App) toggleRasterModeInteractive() {
	if a.rasterMode {
		a.leaveRasterMode()
		a.saveRasterModePreferenceAfterToggle()
		return
	}
	if !confirmEnableRasterMode(a.hwnd) {
		logf("RASTER mode activation canceled")
		return
	}
	a.enterRasterMode()
	a.saveRasterModePreferenceAfterToggle()
}

func (a *App) refreshRasterModeUI() {
	if a.hwnd == 0 {
		return
	}
	a.toggleMenuCheck(ID_IMAGE_RASTER_MODE, a.rasterMode)
	a.requestObjectSelectionUIRepaint()
	a.requestCanvasRepaint(true)
	a.requestStatusRepaint()
}
