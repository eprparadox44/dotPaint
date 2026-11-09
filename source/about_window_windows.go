//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	aboutWindowClassName     = "dotPaintAboutWindowA237"
	aboutCloseButtonID       = 7901
	aboutLicenseButtonID     = 7902
	aboutThirdPartyButtonID  = 7903
	aboutLicenseURL          = "https://github.com/eprparadox44/dotPaint/blob/main/LICENSE"
	aboutThirdPartyNoticeURL = "https://github.com/eprparadox44/dotPaint/blob/main/THIRD_PARTY_NOTICES.md"
)

type aboutWindowState struct {
	owner         *App
	hwnd          uintptr
	closeBtn      uintptr
	licenseBtn    uintptr
	thirdPartyBtn uintptr
	titleFont     uintptr
	makerFont     uintptr
	bodyFont      uintptr
}

var (
	aboutWindowWndProcCallback uintptr
	aboutWindowClassRegistered bool
	activeAboutWindow          *aboutWindowState
)

func registerAboutWindowClass(hinst uintptr) bool {
	if aboutWindowClassRegistered {
		return true
	}
	if aboutWindowWndProcCallback == 0 {
		aboutWindowWndProcCallback = syscall.NewCallback(aboutWindowWndProc)
	}
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         CS_HREDRAW | CS_VREDRAW,
		LpfnWndProc:   aboutWindowWndProcCallback,
		HInstance:     hinst,
		HCursor:       cursor,
		HbrBackground: uintptr(COLOR_BTNFACE + 1),
		LpszClassName: utf16Ptr(aboutWindowClassName),
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		logf("ABOUT register class failed: %v", err)
		return false
	}
	aboutWindowClassRegistered = true
	return true
}

// aboutWindowCaption handles both opening the dialog and switching the UI
// language while the non-modal About window is already open.
func aboutWindowCaption() string {
	if dotPaintEdition == "en" {
		return fmt.Sprintf("About dotPaint %s", appVersion)
	}
	return fmt.Sprintf("dotPaint %s 정보", appVersion)
}

// aboutRefreshLanguage changes only displayed labels. It never recreates the
// window and therefore preserves its layout, dark theme and ownership.
func aboutRefreshLanguage(s *aboutWindowState) {
	if s == nil || s.hwnd == 0 {
		return
	}
	procSetWindowTextW.Call(s.hwnd, uintptr(unsafe.Pointer(utf16Ptr(aboutWindowCaption()))))
	for _, control := range []struct {
		hwnd  uintptr
		label string
	}{
		{s.closeBtn, "닫기"},
		{s.licenseBtn, "MIT 라이선스 전문"},
		{s.thirdPartyBtn, "제3자 자료 고지"},
	} {
		if control.hwnd != 0 {
			procSetWindowTextW.Call(control.hwnd, uintptr(unsafe.Pointer(utf16Ptr(localizeUI(control.label)))))
			procInvalidateRect.Call(control.hwnd, 0, 1)
		}
	}
	procInvalidateRect.Call(s.hwnd, 0, 1)
}

func (a *App) showAboutWindow() {
	if activeAboutWindow != nil && activeAboutWindow.hwnd != 0 {
		if ok, _, _ := procIsWindow.Call(activeAboutWindow.hwnd); ok != 0 {
			procShowWindow.Call(activeAboutWindow.hwnd, SW_SHOW)
			procSetForegroundWindow.Call(activeAboutWindow.hwnd)
			return
		}
	}
	if !registerAboutWindowClass(a.hinst) {
		messageBox(a.hwnd, localizeUI("dotPaint 정보 창을 만들 수 없습니다."), localizeUI("dotPaint 정보"), MB_OK|MB_ICONERROR)
		return
	}
	state := &aboutWindowState{owner: a}
	activeAboutWindow = state
	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(aboutWindowClassName))),
		uintptr(unsafe.Pointer(utf16Ptr(aboutWindowCaption()))),
		WS_CAPTION|WS_SYSMENU,
		uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), 600, 475,
		a.hwnd, 0, a.hinst, 0,
	)
	if hwnd == 0 {
		activeAboutWindow = nil
		logf("ABOUT create failed: %v", err)
		messageBox(a.hwnd, localizeUI("dotPaint 정보 창을 만들 수 없습니다."), localizeUI("dotPaint 정보"), MB_OK|MB_ICONERROR)
		return
	}
	state.hwnd = hwnd
	showThemedEditorWindow(hwnd, a.darkMode)
	logf("ABOUT opened version=%s", appVersion)
}

func aboutLayout(s *aboutWindowState) {
	if s == nil || s.hwnd == 0 || s.closeBtn == 0 {
		return
	}
	var rc RECT
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := int(rc.Right-rc.Left), int(rc.Bottom-rc.Top)
	buttonY := uintptr(max(12, h-46))
	if s.licenseBtn != 0 {
		procMoveWindow.Call(s.licenseBtn, 28, buttonY, 154, 30, 1)
	}
	if s.thirdPartyBtn != 0 {
		procMoveWindow.Call(s.thirdPartyBtn, 192, buttonY, 142, 30, 1)
	}
	procMoveWindow.Call(s.closeBtn, uintptr(max(12, w-112)), buttonY, 96, 30, 1)
}

func aboutDrawTextWithFont(hdc uintptr, text string, rc RECT, flags uint32, color uint32, font uintptr) {
	oldFont := uintptr(0)
	if font != 0 {
		oldFont, _, _ = procSelectObject.Call(hdc, font)
	}
	procSetBkMode.Call(hdc, TRANSPARENT)
	drawText(hdc, text, rc, flags, color)
	if oldFont != 0 {
		procSelectObject.Call(hdc, oldFont)
	}
}

func aboutPaint(s *aboutWindowState, hdc uintptr) {
	if s == nil || hdc == 0 {
		return
	}
	var rc RECT
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&rc)))
	bg := rgb(245, 245, 245)
	fg := rgb(24, 24, 24)
	muted := rgb(75, 75, 75)
	if s.owner != nil && s.owner.darkMode {
		bg = rgb(30, 30, 30)
		fg = rgb(240, 240, 240)
		muted = rgb(190, 190, 190)
	}
	fillRect(hdc, rc, bg)

	margin := int32(28)
	width := rc.Right - rc.Left
	aboutDrawTextWithFont(hdc, fmt.Sprintf("dotPaint %s", appVersion),
		RECT{margin, 26, width - margin, 62}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, fg, s.titleFont)
	aboutDrawTextWithFont(hdc, localizeUI("만든 이 지너드"),
		RECT{margin, 67, width - margin, 95}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, fg, s.makerFont)

	bodyLeft := margin
	bodyRight := width - margin
	aboutDrawTextWithFont(hdc, localizeUI("넣고 싶은 기능을 잡탕으로 모두 넣은 그리기 툴"),
		RECT{bodyLeft, 116, bodyRight, 146}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, fg, s.bodyFont)
	aboutDrawTextWithFont(hdc, localizeUI("제작자는 프로그래머/개발자가 아닙니다."),
		RECT{bodyLeft, 158, bodyRight, 186}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, fg, s.bodyFont)
	aboutDrawTextWithFont(hdc, localizeUI("버그 제보는 받지만 수정된다고 장담하지 못합니다 ㅋㅋ"),
		RECT{bodyLeft, 188, bodyRight, 216}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, fg, s.bodyFont)
	aboutDrawTextWithFont(hdc, fmt.Sprintf(localizeUI("현재 빌드: %s"), appVersion),
		RECT{bodyLeft, 236, bodyRight, 264}, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS, muted, s.bodyFont)
	// The MIT license covers project code and materials that the project owner
	// has permission to license; third-party resources retain their own terms.
	aboutDrawTextWithFont(hdc, localizeUI("라이선스: MIT License (프로젝트 자체 코드)"),
		RECT{bodyLeft, 277, bodyRight, 302}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, fg, s.bodyFont)
	aboutDrawTextWithFont(hdc, localizeUI("Copyright © 2026 지너드 · G.nerd (eprparadox44)"),
		RECT{bodyLeft, 305, bodyRight, 330}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, muted, s.bodyFont)
	aboutDrawTextWithFont(hdc, localizeUI("제3자 코드·이미지에는 별도 사용 조건이 적용될 수 있습니다."),
		RECT{bodyLeft, 334, bodyRight, 364}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, muted, s.bodyFont)
}

func aboutRefreshTheme(s *aboutWindowState) {
	if s == nil || s.hwnd == 0 || s.owner == nil {
		return
	}
	themeWindowHandles(s.owner.darkMode, s.hwnd, s.closeBtn, s.licenseBtn, s.thirdPartyBtn)
	scheduleDarkEditorChromeRefresh(s.hwnd)
	procInvalidateRect.Call(s.hwnd, 0, 1)
}

func aboutWindowWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			logPanic("aboutWindowWndProc", r)
		}
	}()
	if r, handled := handleDarkEditorNonClient(hwnd, msg, wparam, lparam); handled {
		return r
	}
	s := activeAboutWindow
	switch msg {
	case WM_CREATE:
		if s == nil {
			return ^uintptr(0)
		}
		s.hwnd = hwnd
		s.titleFont = createGDIFont("Segoe UI", editorDPIValue(hwnd, 22), true, false, false)
		s.makerFont = createGDIFont("Segoe UI", editorDPIValue(hwnd, 15), true, false, false)
		s.bodyFont = createGDIFont("Segoe UI", editorDPIValue(hwnd, 13), false, false, false)
		s.closeBtn, _, _ = procCreateWindowExW.Call(
			0, uintptr(unsafe.Pointer(utf16Ptr("BUTTON"))), uintptr(unsafe.Pointer(utf16Ptr(localizeUI("닫기")))),
			ownerDrawPushButtonStyle("BUTTON", WS_CHILD|WS_VISIBLE|WS_TABSTOP),
			0, 0, 96, 30, hwnd, aboutCloseButtonID, s.owner.hinst, 0,
		)
		font, _, _ := procGetStockObject.Call(DEFAULT_GUI_FONT)
		sendMessage(s.closeBtn, WM_SETFONT, font, 1)
		s.licenseBtn, _, _ = procCreateWindowExW.Call(
			0, uintptr(unsafe.Pointer(utf16Ptr("BUTTON"))), uintptr(unsafe.Pointer(utf16Ptr(localizeUI("MIT 라이선스 전문")))),
			ownerDrawPushButtonStyle("BUTTON", WS_CHILD|WS_VISIBLE|WS_TABSTOP),
			0, 0, 154, 30, hwnd, aboutLicenseButtonID, s.owner.hinst, 0,
		)
		sendMessage(s.licenseBtn, WM_SETFONT, font, 1)
		s.thirdPartyBtn, _, _ = procCreateWindowExW.Call(
			0, uintptr(unsafe.Pointer(utf16Ptr("BUTTON"))), uintptr(unsafe.Pointer(utf16Ptr(localizeUI("제3자 자료 고지")))),
			ownerDrawPushButtonStyle("BUTTON", WS_CHILD|WS_VISIBLE|WS_TABSTOP),
			0, 0, 142, 30, hwnd, aboutThirdPartyButtonID, s.owner.hinst, 0,
		)
		sendMessage(s.thirdPartyBtn, WM_SETFONT, font, 1)
		aboutLayout(s)
		aboutRefreshLanguage(s)
		aboutRefreshTheme(s)
		return 0
	case WM_SIZE:
		aboutLayout(s)
		return 0
	case WM_COMMAND:
		switch int(wparam & 0xffff) {
		case aboutCloseButtonID:
			procDestroyWindow.Call(hwnd)
		case aboutLicenseButtonID:
			aboutOpenDocumentURL(hwnd, aboutLicenseURL)
		case aboutThirdPartyButtonID:
			aboutOpenDocumentURL(hwnd, aboutThirdPartyNoticeURL)
		}
		return 0
	case WM_DRAWITEM:
		if drawThemedOwnerDrawPushButton((*DRAWITEMSTRUCT)(unsafe.Pointer(lparam))) {
			return 1
		}
	case WM_KEYDOWN:
		if wparam == VK_ESCAPE {
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		aboutPaint(s, hdc)
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		if s != nil && s.hwnd == hwnd {
			for _, font := range []uintptr{s.titleFont, s.makerFont, s.bodyFont} {
				if font != 0 {
					procDeleteObject.Call(font)
				}
			}
			activeAboutWindow = nil
			logf("ABOUT closed")
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

// aboutOpenDocumentURL opens the authoritative full notices in the user's
// default browser; the short license summary remains visible offline.
func aboutOpenDocumentURL(hwnd uintptr, url string) {
	result, _, _ := procShellExecuteW.Call(
		hwnd,
		uintptr(unsafe.Pointer(utf16Ptr("open"))),
		uintptr(unsafe.Pointer(utf16Ptr(url))),
		0, 0, SW_SHOW,
	)
	if result <= 32 {
		messageBox(hwnd, localizeUI("문서를 열 수 없습니다.\n")+url, localizeUI("dotPaint 라이선스"), MB_OK|MB_ICONERROR)
	}
}
