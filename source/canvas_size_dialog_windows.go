//go:build windows

package main

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// A77: smaller centered numeric fields; inherited roomier preset-list rows.

const (
	canvasSizeDialogClass = "dotPaintCanvasSizeDialogA77"

	canvasWidthEditID  = 7201
	canvasHeightEditID = 7202
	canvasPresetListID = 7203

	ES_NUMBER                = 0x2000
	LBS_NOTIFY               = 0x0001
	LBS_HASSTRINGS           = 0x0040
	LB_ADDSTRING             = 0x0180
	LB_GETCURSEL             = 0x0188
	LB_SETCURSEL             = 0x0186
	LB_SETITEMHEIGHT         = 0x01A0
	LB_GETITEMHEIGHT         = 0x01A1
	LBN_SELCHANGE            = 1
	BS_GROUPBOX              = 0x00000007
	BS_DEFPUSHBUTTON         = 0x00000001
	WS_EX_DLGMODALFRAME      = 0x00000001
	MONITOR_DEFAULTTONEAREST = 0x00000002
)

type MONITORINFO struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
}

type canvasMonitorSize struct {
	Handle uintptr
	W      int
	H      int
}

type canvasSizePreset struct {
	Label string
	W     int
	H     int
}

type canvasSizeDialogState struct {
	hwnd, parent uintptr
	uiFont       uintptr
	widthEdit    uintptr
	heightEdit   uintptr
	presetList   uintptr
	presets      []canvasSizePreset
	width        int
	height       int
	accepted     bool
	done         bool
	updating     bool
}

var (
	procMonitorFromWindow   = modUser32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW     = modUser32.NewProc("GetMonitorInfoW")
	procEnumDisplayMonitors = modUser32.NewProc("EnumDisplayMonitors")

	canvasSizeDialog           *canvasSizeDialogState
	canvasSizeDialogWndProcCB  uintptr
	canvasSizeDialogClassReady bool
	canvasMonitorEnumCB        uintptr
	canvasMonitorEnumResults   []canvasMonitorSize
)

func ensureCanvasSizeDialogClass(hinst uintptr) bool {
	if canvasSizeDialogClassReady {
		return true
	}
	if canvasSizeDialogWndProcCB == 0 {
		canvasSizeDialogWndProcCB = syscall.NewCallback(canvasSizeDialogWndProc)
	}
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		LpfnWndProc:   canvasSizeDialogWndProcCB,
		HInstance:     hinst,
		HCursor:       cursor,
		HbrBackground: uintptr(COLOR_BTNFACE + 1),
		LpszClassName: utf16Ptr(canvasSizeDialogClass),
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		logf("CANVAS_SIZE RegisterClass failed err=%v", err)
		return false
	}
	canvasSizeDialogClassReady = true
	return true
}

func canvasMonitorEnumProc(hmonitor, hdc, rect, lparam uintptr) uintptr {
	var mi MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	ok, _, _ := procGetMonitorInfoW.Call(hmonitor, uintptr(unsafe.Pointer(&mi)))
	if ok != 0 {
		w := int(mi.RcMonitor.Right - mi.RcMonitor.Left)
		h := int(mi.RcMonitor.Bottom - mi.RcMonitor.Top)
		if w > 0 && h > 0 {
			canvasMonitorEnumResults = append(canvasMonitorEnumResults, canvasMonitorSize{Handle: hmonitor, W: w, H: h})
		}
	}
	return 1
}

func canvasMonitorPresets(parent uintptr) (current canvasMonitorSize, other canvasMonitorSize, hasOther bool) {
	currentHandle, _, _ := procMonitorFromWindow.Call(parent, MONITOR_DEFAULTTONEAREST)
	if canvasMonitorEnumCB == 0 {
		canvasMonitorEnumCB = syscall.NewCallback(canvasMonitorEnumProc)
	}
	canvasMonitorEnumResults = canvasMonitorEnumResults[:0]
	procEnumDisplayMonitors.Call(0, 0, canvasMonitorEnumCB, 0)
	for _, m := range canvasMonitorEnumResults {
		if m.Handle == currentHandle {
			current = m
			break
		}
	}
	if current.W == 0 && len(canvasMonitorEnumResults) > 0 {
		current = canvasMonitorEnumResults[0]
	}
	for _, m := range canvasMonitorEnumResults {
		if m.Handle != current.Handle {
			other = m
			hasOther = true
			break
		}
	}
	return
}

func buildCanvasSizePresets(parent uintptr) ([]canvasSizePreset, string) {
	current, other, hasOther := canvasMonitorPresets(parent)
	presets := []canvasSizePreset{
		{"Windows 아이콘 — 32 × 32", 32, 32},
		{"작은 이미지 — 320 × 240", 320, 240},
		{"VGA — 640 × 480", 640, 480},
		{"SVGA — 800 × 600", 800, 600},
		{"XGA — 1024 × 768", 1024, 768},
		{"HD — 1280 × 720", 1280, 720},
		{"Full HD — 1920 × 1080", 1920, 1080},
		{"4K UHD — 3840 × 2160", 3840, 2160},
	}
	monitorNote := "현재 화면 크기를 확인하지 못했습니다."
	if current.W > 0 && current.H > 0 {
		presets = append(presets, canvasSizePreset{fmt.Sprintf("현재 화면 — %d × %d", current.W, current.H), current.W, current.H})
		monitorNote = fmt.Sprintf("현재 화면: %d × %d", current.W, current.H)
	}
	if hasOther {
		presets = append(presets, canvasSizePreset{fmt.Sprintf("다른 화면 — %d × %d", other.W, other.H), other.W, other.H})
		monitorNote += fmt.Sprintf("    다른 화면: %d × %d", other.W, other.H)
	} else {
		monitorNote += "    다른 화면: 감지되지 않음"
	}
	return presets, monitorNote
}

func canvasCreateChild(parent uintptr, class, title string, style uintptr, x, y, w, h, id int) uintptr {
	style = ownerDrawPushButtonStyle(class, style)
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(class))),
		uintptr(unsafe.Pointer(utf16Ptr(title))),
		style,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), app.hinst, 0,
	)
	if hwnd != 0 {
		setCompactEditorControlFont(hwnd, compactImageDialogUIFont(parent))
		applyDarkThemeToWindow(hwnd, darkModeEnabled())
	}
	return hwnd
}

func compactImageDialogUIFont(parent uintptr) uintptr {
	if canvasSizeDialog != nil && canvasSizeDialog.hwnd == parent {
		return canvasSizeDialog.uiFont
	}
	if stretchSkewDialog != nil && stretchSkewDialog.hwnd == parent {
		return stretchSkewDialog.uiFont
	}
	return 0
}

func (s *canvasSizeDialogState) setEditValues(w, h int, selectWidth bool) {
	s.updating = true
	procSetWindowTextW.Call(s.widthEdit, uintptr(unsafe.Pointer(utf16Ptr(strconv.Itoa(w)))))
	procSetWindowTextW.Call(s.heightEdit, uintptr(unsafe.Pointer(utf16Ptr(strconv.Itoa(h)))))
	s.updating = false
	if selectWidth {
		procSetFocus.Call(s.widthEdit)
		procSendMessageW.Call(s.widthEdit, EM_SETSEL, 0, ^uintptr(0))
	}
}

func (s *canvasSizeDialogState) parseSize() (int, int, bool) {
	w, errW := strconv.Atoi(strings.TrimSpace(getWindowTextString(s.widthEdit)))
	h, errH := strconv.Atoi(strings.TrimSpace(getWindowTextString(s.heightEdit)))
	if errW != nil || errH != nil || w < 1 || h < 1 || w > 16384 || h > 16384 {
		return 0, 0, false
	}
	return w, h, true
}

func canvasSizeDialogWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	s := canvasSizeDialog
	defer func() {
		if r := recover(); r != nil {
			logPanic("canvasSizeDialogWndProc", r)
		}
	}()
	switch msg {
	case WM_CREATE:
		if s == nil {
			break
		}
		s.hwnd = hwnd
		applyDarkThemeToWindow(hwnd, darkModeEnabled())
		canvasCreateChild(hwnd, "STATIC", "가로(W):", WS_CHILD|WS_VISIBLE, 31, 21, 69, 20, 0)
		s.widthEdit = canvasCreateChild(hwnd, "EDIT", strconv.Itoa(s.width), WS_CHILD|WS_VISIBLE|WS_BORDER|WS_TABSTOP|ES_NUMBER|ES_CENTER|ES_AUTOHSCROLL, 104, 20, 54, 20, canvasWidthEditID)
		// Keep both horizontal margins identical so ES_CENTER remains visually exact.
		sendMessage(s.widthEdit, EM_SETMARGINS, EC_LEFTMARGIN|EC_RIGHTMARGIN, 0)
		canvasCreateChild(hwnd, "STATIC", "px", WS_CHILD|WS_VISIBLE, 164, 21, 28, 20, 0)
		canvasCreateChild(hwnd, "STATIC", "세로(H):", WS_CHILD|WS_VISIBLE, 250, 21, 69, 20, 0)
		s.heightEdit = canvasCreateChild(hwnd, "EDIT", strconv.Itoa(s.height), WS_CHILD|WS_VISIBLE|WS_BORDER|WS_TABSTOP|ES_NUMBER|ES_CENTER|ES_AUTOHSCROLL, 323, 20, 54, 20, canvasHeightEditID)
		sendMessage(s.heightEdit, EM_SETMARGINS, EC_LEFTMARGIN|EC_RIGHTMARGIN, 0)
		canvasCreateChild(hwnd, "STATIC", "px", WS_CHILD|WS_VISIBLE, 383, 21, 28, 20, 0)

		canvasCreateChild(hwnd, "BUTTON", "전형적인 크기", WS_CHILD|WS_VISIBLE|BS_GROUPBOX, 16, 57, 450, 257, 0)
		s.presetList = canvasCreateChild(hwnd, "LISTBOX", "", WS_CHILD|WS_VISIBLE|WS_BORDER|WS_TABSTOP|WS_VSCROLL|LBS_NOTIFY|LBS_HASSTRINGS, 31, 82, 420, 214, canvasPresetListID)
		// Add exactly three physical pixels of breathing room between preset rows.
		if baseHeight := sendMessage(s.presetList, LB_GETITEMHEIGHT, 0, 0); baseHeight > 0 {
			sendMessage(s.presetList, LB_SETITEMHEIGHT, 0, baseHeight+3)
		}
		for _, p := range s.presets {
			procSendMessageW.Call(s.presetList, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(p.Label))))
		}
		canvasCreateChild(hwnd, "BUTTON", "확인", WS_CHILD|WS_VISIBLE|WS_TABSTOP|BS_DEFPUSHBUTTON, 283, 324, 82, 30, IDOK)
		canvasCreateChild(hwnd, "BUTTON", "취소", WS_CHILD|WS_VISIBLE|WS_TABSTOP, 374, 324, 82, 30, IDCANCEL)
		s.setEditValues(s.width, s.height, true)
		return 0
	case WM_ERASEBKGND:
		if darkModeEnabled() && wparam != 0 {
			var rc RECT
			procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
			fillRect(wparam, rc, rgb(30, 30, 30))
			return 1
		}
	case WM_CTLCOLOREDIT, WM_CTLCOLORLISTBOX, WM_CTLCOLORSTATIC, WM_CTLCOLORBTN, WM_CTLCOLORDLG:
		if darkModeEnabled() && app != nil && wparam != 0 {
			procSetTextColor.Call(wparam, uintptr(rgb(235, 235, 235)))
			procSetBkColor.Call(wparam, uintptr(rgb(30, 30, 30)))
			if app.darkBrush == 0 {
				app.darkBrush, _, _ = procCreateSolidBrush.Call(uintptr(rgb(30, 30, 30)))
			}
			return app.darkBrush
		}
	case WM_COMMAND:
		if s == nil {
			break
		}
		id := loWord(wparam)
		code := hiWord(wparam)
		switch {
		case id == canvasPresetListID && code == LBN_SELCHANGE:
			idx := int(sendMessage(s.presetList, LB_GETCURSEL, 0, 0))
			if idx >= 0 && idx < len(s.presets) {
				p := s.presets[idx]
				s.setEditValues(p.W, p.H, false)
			}
			return 0
		case id == IDOK:
			w, h, ok := s.parseSize()
			if !ok {
				messageBox(hwnd, "가로와 세로를 1~16384 px 범위의 정수로 입력하십시오.", "dotPaint", MB_OK|MB_ICONWARNING)
				s.setEditValues(max(s.width, 1), max(s.height, 1), true)
				return 0
			}
			s.width, s.height = w, h
			s.accepted = true
			s.done = true
			procDestroyWindow.Call(hwnd)
			return 0
		case id == IDCANCEL:
			s.accepted = false
			s.done = true
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case WM_DRAWITEM:
		if lparam != 0 && drawThemedOwnerDrawPushButton((*DRAWITEMSTRUCT)(unsafe.Pointer(lparam))) {
			return 1
		}
	case WM_CLOSE:
		if s != nil {
			s.accepted = false
			s.done = true
		}
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		if s != nil {
			if s.uiFont != 0 {
				procDeleteObject.Call(s.uiFont)
				s.uiFont = 0
			}
			s.hwnd = 0
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func (a *App) showCanvasSizeDialog() {
	if !ensureCanvasSizeDialogClass(a.hinst) {
		messageBox(a.hwnd, "캔버스 크기 창을 만들지 못했습니다.", "dotPaint", MB_OK|MB_ICONERROR)
		return
	}
	presets, monitorNote := buildCanvasSizePresets(a.hwnd)
	state := &canvasSizeDialogState{parent: a.hwnd, uiFont: createCompactEditorUIFont(), presets: presets, width: a.canvas.W, height: a.canvas.H}
	canvasSizeDialog = state

	style := uintptr(WS_CAPTION | WS_SYSMENU)
	exStyle := uintptr(WS_EX_DLGMODALFRAME)
	clientBounds := RECT{0, 0, 482, 375}
	procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&clientBounds)), style, 0, exStyle)
	w := int(clientBounds.Right - clientBounds.Left)
	h := int(clientBounds.Bottom - clientBounds.Top)
	hwnd, _, err := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(utf16Ptr(canvasSizeDialogClass))),
		uintptr(unsafe.Pointer(utf16Ptr("캔버스 크기"))),
		style,
		uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), uintptr(w), uintptr(h),
		a.hwnd, 0, a.hinst, 0,
	)
	if hwnd == 0 {
		if state.uiFont != 0 {
			procDeleteObject.Call(state.uiFont)
			state.uiFont = 0
		}
		canvasSizeDialog = nil
		logf("CANVAS_SIZE CreateWindow failed err=%v", err)
		messageBox(a.hwnd, "캔버스 크기 창을 만들지 못했습니다.", "dotPaint", MB_OK|MB_ICONERROR)
		return
	}
	state.hwnd = hwnd
	applyDarkThemeToWindow(hwnd, a.darkMode)
	var pr RECT
	procGetWindowRect.Call(a.hwnd, uintptr(unsafe.Pointer(&pr)))
	x := int(pr.Left) + (int(pr.Right-pr.Left)-w)/2
	y := int(pr.Top) + (int(pr.Bottom-pr.Top)-h)/2
	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), SWP_NOZORDER|SWP_NOACTIVATE)
	procEnableWindow.Call(a.hwnd, 0)
	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)
	procSetFocus.Call(state.widthEdit)
	logf("CANVAS_SIZE dialog open current=%dx%d presets=%d monitors=%q", a.canvas.W, a.canvas.H, len(presets), monitorNote)

	var msg MSG
	for !state.done {
		r, _, getErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			if int32(r) < 0 {
				logf("CANVAS_SIZE GetMessage failed err=%v", getErr)
			}
			break
		}
		if msg.Message == WM_KEYDOWN {
			switch int(msg.WParam) {
			case VK_TAB:
				if moveDialogTabFocus(hwnd, keyDown(VK_SHIFT)) {
					continue
				}
			case VK_ESCAPE:
				procSendMessageW.Call(hwnd, WM_COMMAND, IDCANCEL, 0)
				continue
			case 0x0D: // Enter
				procSendMessageW.Call(hwnd, WM_COMMAND, IDOK, 0)
				continue
			}
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	procEnableWindow.Call(a.hwnd, 1)
	procSetForegroundWindow.Call(a.hwnd)
	procSetFocus.Call(a.hwnd)

	accepted := state.accepted
	nw, nh := state.width, state.height
	if state.hwnd != 0 {
		procDestroyWindow.Call(state.hwnd)
	}
	canvasSizeDialog = nil
	if !accepted {
		logf("CANVAS_SIZE dialog cancel")
		return
	}
	if nw == a.canvas.W && nh == a.canvas.H {
		a.statusText = fmt.Sprintf("캔버스 크기는 이미 %d × %d px입니다.", nw, nh)
		invalidate(a.hwnd)
		return
	}
	oldW, oldH := a.canvas.W, a.canvas.H
	a.clearSelectionState()
	a.undo.Push(a.canvas)
	a.canvas.ResizeCanvas(nw, nh, 0, 0, a.bg)
	a.markDirty()
	a.recordHistory("캔버스 크기")
	a.updateScrollbars()
	a.statusText = fmt.Sprintf("캔버스 크기를 %d × %d에서 %d × %d px로 변경했습니다.", oldW, oldH, nw, nh)
	logf("CANVAS_SIZE apply old=%dx%d new=%dx%d bg=%#08x", oldW, oldH, nw, nh, a.bg)
	invalidate(a.hwnd)
}
