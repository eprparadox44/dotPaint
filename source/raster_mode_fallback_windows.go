//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// TaskDialogIndirect requires the v6 common-controls activation context.
// Portable builds are not guaranteed to have one. Provide an exact-caption
// custom Win32 fallback, instead of silently degrading to generic Yes/No.
const rasterConfirmClass = "dotPaintRasterModeWarning564"

type rasterConfirmState struct {
	hwnd, owner    uintptr
	done, accepted bool
}

var rasterConfirmCurrent *rasterConfirmState
var rasterConfirmClassReady bool
var rasterConfirmWndProcCB uintptr

func rasterConfirmWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	s := rasterConfirmCurrent
	switch msg {
	case WM_CREATE:
		if s == nil {
			break
		}
		font, _, _ := procGetStockObject.Call(DEFAULT_GUI_FONT)
		makeControl := func(class, text string, style uintptr, x, y, w, h int, id uintptr) uintptr {
			control, _, _ := procCreateWindowExW.Call(0,
				uintptr(unsafe.Pointer(utf16Ptr(class))), uintptr(unsafe.Pointer(utf16Ptr(localizeUI(text)))),
				uintptr(WS_CHILD|WS_VISIBLE)|style, uintptr(x), uintptr(y), uintptr(w), uintptr(h),
				hwnd, id, app.hinst, 0)
			if control != 0 {
				sendMessage(control, WM_SETFONT, font, 1)
			}
			return control
		}
		makeControl("STATIC", "주의! 활성화하면 객체가 완전히 사라집니다.", 0, 22, 19, 480, 45, 0)
		makeControl("BUTTON", "그래도 해요", WS_TABSTOP, 252, 87, 115, 31, 1001)
		makeControl("BUTTON", "취소 할게요", WS_TABSTOP|BS_DEFPUSHBUTTON, 377, 87, 120, 31, 1002)
		return 0
	case WM_COMMAND:
		if s != nil {
			switch loWord(wparam) {
			case 1001:
				s.accepted = true
				s.done = true
				procDestroyWindow.Call(hwnd)
				return 0
			case 1002:
				s.done = true
				procDestroyWindow.Call(hwnd)
				return 0
			}
		}
	case WM_KEYDOWN:
		if s != nil && wparam == VK_ESCAPE {
			s.done = true
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case WM_CLOSE:
		if s != nil {
			s.done = true
		}
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		if s != nil {
			s.done = true
			s.hwnd = 0
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func confirmRasterModeFallback(owner uintptr) bool {
	if app == nil {
		return false
	}
	if !rasterConfirmClassReady {
		if rasterConfirmWndProcCB == 0 {
			rasterConfirmWndProcCB = syscall.NewCallback(rasterConfirmWndProc)
		}
		cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
		wc := WNDCLASSEXW{CbSize: uint32(unsafe.Sizeof(WNDCLASSEXW{})),
			LpfnWndProc: rasterConfirmWndProcCB, HInstance: app.hinst,
			HCursor: cursor, HbrBackground: uintptr(COLOR_BTNFACE + 1),
			LpszClassName: utf16Ptr(rasterConfirmClass)}
		atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if atom == 0 {
			logf("RASTER fallback dialog class failed: %v", err)
			return false // fail closed; never erase without confirmation
		}
		rasterConfirmClassReady = true
	}
	state := &rasterConfirmState{owner: owner}
	rasterConfirmCurrent = state
	defer func() { rasterConfirmCurrent = nil }()
	style := uintptr(WS_CAPTION | WS_SYSMENU)
	exStyle := uintptr(WS_EX_DLGMODALFRAME)
	bounds := RECT{0, 0, 520, 136}
	procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&bounds)), style, 0, exStyle)
	w, h := int(bounds.Right-bounds.Left), int(bounds.Bottom-bounds.Top)
	hwnd, _, err := procCreateWindowExW.Call(exStyle, uintptr(unsafe.Pointer(utf16Ptr(rasterConfirmClass))),
		uintptr(unsafe.Pointer(utf16Ptr(localizeUI("래스터화 모드")))), style,
		uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), uintptr(w), uintptr(h), owner, 0, app.hinst, 0)
	if hwnd == 0 {
		logf("RASTER fallback dialog creation failed: %v", err)
		return false
	}
	state.hwnd = hwnd
	var rc RECT
	procGetWindowRect.Call(owner, uintptr(unsafe.Pointer(&rc)))
	x, y := int(rc.Left)+(int(rc.Right-rc.Left)-w)/2, int(rc.Top)+(int(rc.Bottom-rc.Top)-h)/2
	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), SWP_NOZORDER|SWP_NOACTIVATE)
	procEnableWindow.Call(owner, 0)
	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)
	var msg MSG
	for !state.done {
		r, _, getErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			logf("RASTER fallback GetMessage interrupted: %v", getErr)
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	if state.hwnd != 0 {
		procDestroyWindow.Call(state.hwnd)
	}
	procEnableWindow.Call(owner, 1)
	procSetForegroundWindow.Call(owner)
	procSetFocus.Call(owner)
	return state.accepted
}
