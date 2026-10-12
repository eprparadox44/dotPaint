//go:build windows

package main

import (
	"bytes"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

const (
	customImageLibraryClassName = "dotPaintCustomImageLibraryA223"
	customImageSaveButtonID     = 7801
	customImageInsertButtonID   = 7802
	customImageDeleteButtonID   = 7803
	customImageCloseButtonID    = 7804
	customImageVKReturn         = 0x0D
)

type customImageSlot struct {
	Image   ClipboardImage
	Preview []byte
	Path    string
	Exists  bool
}

type customImageLibraryState struct {
	owner    *App
	hwnd     uintptr
	save     uintptr
	insert   uintptr
	delete   uintptr
	close    uintptr
	selected int
	dir      string
	slots    [customImageMaxSlots]customImageSlot
}

var (
	customImageLibraryWndProcCallback uintptr
	customImageLibraryClassRegistered bool
	activeCustomImageLibrary          *customImageLibraryState
)

func customImageWritableDir(path string) bool {
	if path == "" {
		return false
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return false
	}
	test := filepath.Join(path, fmt.Sprintf(".dotpaint_write_%d.tmp", time.Now().UnixNano()))
	if err := os.WriteFile(test, []byte("ok"), 0o644); err != nil {
		return false
	}
	_ = os.Remove(test)
	return true
}

func customImageStorageDir() string {
	if exe, err := os.Executable(); err == nil && exe != "" {
		candidate := filepath.Join(filepath.Dir(exe), "user_images")
		if customImageWritableDir(candidate) {
			return candidate
		}
	}
	if config, err := os.UserConfigDir(); err == nil && config != "" {
		candidate := filepath.Join(config, "dotPaint", "user_images")
		if customImageWritableDir(candidate) {
			return candidate
		}
	}
	candidate := filepath.Join(os.TempDir(), "dotPaint", "user_images")
	_ = os.MkdirAll(candidate, 0o755)
	return candidate
}

func customImageDecodePNG(data []byte) (ClipboardImage, error) {
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return ClipboardImage{}, err
	}
	img := imageToClipboardImage(decoded)
	if !validClipboardImage(img) {
		return ClipboardImage{}, fmt.Errorf("이미지 데이터가 올바르지 않습니다")
	}
	if !customImageDimensionsAllowed(img.W, img.H) {
		return ClipboardImage{}, fmt.Errorf("이미지 크기 %d×%dpx는 허용 범위 300×300px를 초과합니다", img.W, img.H)
	}
	return img, nil
}

func customImageOpaqueWhitePreview(img ClipboardImage) []byte {
	if !validClipboardImage(img) {
		return nil
	}
	out := make([]byte, len(img.Pix))
	for i := 0; i < len(img.Pix); i += 4 {
		a := int(img.Pix[i+3])
		inv := 255 - a
		out[i+0] = byte((int(img.Pix[i+0])*a + 255*inv + 127) / 255)
		out[i+1] = byte((int(img.Pix[i+1])*a + 255*inv + 127) / 255)
		out[i+2] = byte((int(img.Pix[i+2])*a + 255*inv + 127) / 255)
		out[i+3] = 255
	}
	return out
}

func (s *customImageLibraryState) loadSlots() {
	for i := 0; i < customImageMaxSlots; i++ {
		path := filepath.Join(s.dir, customImageSlotFilename(i))
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				logf("CUSTOM_IMAGE load slot=%d path=%q error=%v", i+1, path, err)
			}
			continue
		}
		img, err := customImageDecodePNG(data)
		if err != nil {
			logf("CUSTOM_IMAGE load slot=%d path=%q invalid=%v", i+1, path, err)
			continue
		}
		s.slots[i] = customImageSlot{Image: img, Preview: customImageOpaqueWhitePreview(img), Path: path, Exists: true}
	}
}

func registerCustomImageLibraryClass(hinst uintptr) bool {
	if customImageLibraryClassRegistered {
		return true
	}
	if customImageLibraryWndProcCallback == 0 {
		customImageLibraryWndProcCallback = syscall.NewCallback(customImageLibraryWndProc)
	}
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         CS_HREDRAW | CS_VREDRAW | CS_DBLCLKS,
		LpfnWndProc:   customImageLibraryWndProcCallback,
		HInstance:     hinst,
		HCursor:       cursor,
		HbrBackground: uintptr(COLOR_BTNFACE + 1),
		LpszClassName: utf16Ptr(customImageLibraryClassName),
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		logf("CUSTOM_IMAGE register class failed: %v", err)
		return false
	}
	customImageLibraryClassRegistered = true
	return true
}

func (a *App) showCustomImageLibrary() {
	if activeCustomImageLibrary != nil && activeCustomImageLibrary.hwnd != 0 {
		if ok, _, _ := procIsWindow.Call(activeCustomImageLibrary.hwnd); ok != 0 {
			procShowWindow.Call(activeCustomImageLibrary.hwnd, SW_SHOW)
			procSetForegroundWindow.Call(activeCustomImageLibrary.hwnd)
			return
		}
	}
	if !registerCustomImageLibraryClass(a.hinst) {
		messageBox(a.hwnd, "사용자 정의 이미지 창을 만들 수 없습니다.", "삽입 > 이미지 > 사용자 정의 이미지", MB_OK|MB_ICONERROR)
		return
	}
	state := &customImageLibraryState{owner: a, selected: 0, dir: customImageStorageDir()}
	state.loadSlots()
	activeCustomImageLibrary = state
	style := uintptr(WS_CAPTION | WS_SYSMENU | WS_MINIMIZEBOX)
	hwnd, _, createErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(customImageLibraryClassName))),
		uintptr(unsafe.Pointer(utf16Ptr("사용자 정의 이미지"))),
		style,
		uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), 980, 720,
		a.hwnd, 0, a.hinst, 0,
	)
	if hwnd == 0 {
		activeCustomImageLibrary = nil
		logf("CUSTOM_IMAGE create window failed: %v", createErr)
		messageBox(a.hwnd, "사용자 정의 이미지 창을 만들 수 없습니다.", "삽입 > 이미지 > 사용자 정의 이미지", MB_OK|MB_ICONERROR)
		return
	}
	state.hwnd = hwnd
	showThemedEditorWindow(hwnd, a.darkMode)
	a.statusText = "300×300px 이하 이미지를 클립보드에서 저장하거나 저장된 이미지를 삽입하십시오."
	logf("CUSTOM_IMAGE library opened hwnd=%#x dir=%q slots=%d", hwnd, state.dir, customImageMaxSlots)
	invalidate(a.hwnd)
}

func customImageCreateButton(s *customImageLibraryState, title string, id int, width int) uintptr {
	h, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr("BUTTON"))),
		uintptr(unsafe.Pointer(utf16Ptr(title))),
		ownerDrawPushButtonStyle("BUTTON", WS_CHILD|WS_VISIBLE|WS_TABSTOP),
		0, 0, uintptr(width), 30,
		s.hwnd, uintptr(id), s.owner.hinst, 0,
	)
	if h != 0 {
		font, _, _ := procGetStockObject.Call(DEFAULT_GUI_FONT)
		sendMessage(h, WM_SETFONT, font, 1)
		applyDarkThemeToWindow(h, s.owner.darkMode)
	}
	return h
}

func customImageLayoutControls(s *customImageLibraryState) {
	if s == nil || s.hwnd == 0 {
		return
	}
	var client RECT
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	w, h := int(client.Right-client.Left), int(client.Bottom-client.Top)
	buttonY := max(10, h-46)
	closeX := max(10, w-102)
	deleteX := max(10, closeX-98)
	insertX := max(10, deleteX-98)
	saveX := max(10, insertX-166)
	if s.save != 0 {
		procMoveWindow.Call(s.save, uintptr(saveX), uintptr(buttonY), 158, 30, 1)
	}
	if s.insert != 0 {
		procMoveWindow.Call(s.insert, uintptr(insertX), uintptr(buttonY), 90, 30, 1)
	}
	if s.delete != 0 {
		procMoveWindow.Call(s.delete, uintptr(deleteX), uintptr(buttonY), 90, 30, 1)
	}
	if s.close != 0 {
		procMoveWindow.Call(s.close, uintptr(closeX), uintptr(buttonY), 90, 30, 1)
	}
}

func customImageCardRects(client RECT) []RECT {
	const columns = 5
	const rows = 4
	margin, gap := 12, 8
	headerH, bottomH := 58, 68
	w, h := int(client.Right-client.Left), int(client.Bottom-client.Top)
	availableW := max(columns, w-margin*2-gap*(columns-1))
	availableH := max(rows, h-headerH-bottomH-gap*(rows-1))
	cardW := max(1, availableW/columns)
	cardH := max(1, availableH/rows)
	out := make([]RECT, customImageMaxSlots)
	for i := 0; i < customImageMaxSlots; i++ {
		col, row := i%columns, i/columns
		left := margin + col*(cardW+gap)
		top := headerH + row*(cardH+gap)
		out[i] = RECT{int32(left), int32(top), int32(left + cardW), int32(top + cardH)}
	}
	return out
}

func customImageHitTest(s *customImageLibraryState, x, y int) int {
	if s == nil || s.hwnd == 0 {
		return -1
	}
	var client RECT
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	for i, rc := range customImageCardRects(client) {
		if x >= int(rc.Left) && x < int(rc.Right) && y >= int(rc.Top) && y < int(rc.Bottom) {
			return i
		}
	}
	return -1
}

func customImagePaintSlot(hdc uintptr, slot customImageSlot, index int, rc RECT) {
	labelH := 24
	previewArea := RECT{rc.Left + 5, rc.Top + 5, rc.Right - 5, rc.Bottom - int32(labelH+3)}
	fillRect(hdc, previewArea, rgb(255, 255, 255))
	frameRect(hdc, previewArea, rgb(205, 205, 205))
	if slot.Exists && validClipboardImage(slot.Image) && len(slot.Preview) == slot.Image.W*slot.Image.H*4 {
		availW := max(1, int(previewArea.Right-previewArea.Left)-8)
		availH := max(1, int(previewArea.Bottom-previewArea.Top)-8)
		scale := math.Min(float64(availW)/float64(slot.Image.W), float64(availH)/float64(slot.Image.H))
		if scale > 1.0 {
			scale = 1.0
		}
		dw := max(1, int(math.Round(float64(slot.Image.W)*scale)))
		dh := max(1, int(math.Round(float64(slot.Image.H)*scale)))
		dx := int(previewArea.Left) + (int(previewArea.Right-previewArea.Left)-dw)/2
		dy := int(previewArea.Top) + (int(previewArea.Bottom-previewArea.Top)-dh)/2
		bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{
			BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(slot.Image.W), BiHeight: -int32(slot.Image.H),
			BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB,
		}}
		procSetStretchBltMode.Call(hdc, COLORONCOLOR)
		procStretchDIBits.Call(
			hdc, uintptr(dx), uintptr(dy), uintptr(dw), uintptr(dh),
			0, 0, uintptr(slot.Image.W), uintptr(slot.Image.H),
			uintptr(unsafe.Pointer(&slot.Preview[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, SRCCOPY,
		)
	} else {
		empty := RECT{previewArea.Left + 4, previewArea.Top + 4, previewArea.Right - 4, previewArea.Bottom - 4}
		drawUIText(hdc, "빈 슬롯", empty, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(145, 145, 145))
	}
	labelText := fmt.Sprintf("%02d", index+1)
	if slot.Exists {
		labelText = fmt.Sprintf("%02d  %d×%d", index+1, slot.Image.W, slot.Image.H)
	}
	label := RECT{rc.Left + 3, rc.Bottom - int32(labelH), rc.Right - 3, rc.Bottom - 1}
	drawUIText(hdc, labelText, label, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS, rgb(20, 20, 20))
}

func customImagePaintLibrary(s *customImageLibraryState, hdc uintptr) {
	var client RECT
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	fillUIRect(hdc, client, rgb(240, 240, 240))
	header := RECT{12, 7, client.Right - 12, 31}
	drawUIText(hdc, "클립보드 이미지 저장: 최대 300×300px · 최대 20개 · PNG로 영구 보관", header, DT_LEFT|DT_VCENTER|DT_SINGLELINE, rgb(25, 25, 25))
	pathLine := RECT{12, 30, client.Right - 12, 52}
	drawUIText(hdc, "저장 위치: "+s.dir, pathLine, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS, rgb(80, 80, 80))

	cards := customImageCardRects(client)
	for i, rc := range cards {
		fillUIRect(hdc, rc, rgb(248, 248, 248))
		if i == s.selected {
			frameUIRect(hdc, rc, rgb(0, 110, 215))
			inner := RECT{rc.Left + 1, rc.Top + 1, rc.Right - 1, rc.Bottom - 1}
			frameUIRect(hdc, inner, rgb(0, 110, 215))
		} else {
			frameUIRect(hdc, rc, rgb(170, 170, 170))
		}
		customImagePaintSlot(hdc, s.slots[i], i, rc)
	}

	selected := fmt.Sprintf("선택 슬롯: %02d · 비어 있음", s.selected+1)
	if s.selected >= 0 && s.selected < customImageMaxSlots && s.slots[s.selected].Exists {
		slot := s.slots[s.selected]
		selected = fmt.Sprintf("선택 슬롯: %02d · %d×%dpx", s.selected+1, slot.Image.W, slot.Image.H)
	}
	footer := RECT{12, client.Bottom - 46, client.Right - 450, client.Bottom - 16}
	drawUIText(hdc, selected, footer, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS, rgb(35, 35, 35))
}

func (s *customImageLibraryState) occupiedFlags() []bool {
	out := make([]bool, customImageMaxSlots)
	for i := range out {
		out[i] = s.slots[i].Exists
	}
	return out
}

func customImageSaveFromClipboard(s *customImageLibraryState) bool {
	if s == nil || s.owner == nil {
		return false
	}
	img, source, err := s.owner.readSystemClipboard()
	if err != nil {
		messageBox(s.hwnd, "클립보드에서 이미지를 읽지 못했습니다.\n\n"+err.Error(), "사용자 정의 이미지", MB_OK|MB_ICONERROR)
		return false
	}
	if !customImageDimensionsAllowed(img.W, img.H) {
		messageBox(s.hwnd, fmt.Sprintf("이미지 크기는 300×300px 이하여야 합니다.\n\n현재 크기: %d×%dpx", img.W, img.H), "사용자 정의 이미지", MB_OK|MB_ICONWARNING)
		return false
	}
	slotIndex := customImageChooseSaveSlot(s.occupiedFlags(), s.selected)
	if slotIndex < 0 {
		messageBox(s.hwnd, "사용자 정의 이미지 슬롯 20개가 모두 찼습니다.\n기존 이미지를 삭제한 뒤 다시 시도하십시오.", "사용자 정의 이미지", MB_OK|MB_ICONWARNING)
		return false
	}
	pngData, err := buildTransparentClipboardPNG(img.W, img.H, img.Pix)
	if err != nil {
		messageBox(s.hwnd, "PNG 저장 데이터를 만들지 못했습니다.\n\n"+err.Error(), "사용자 정의 이미지", MB_OK|MB_ICONERROR)
		return false
	}
	path := filepath.Join(s.dir, customImageSlotFilename(slotIndex))
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, pngData, 0o644); err != nil {
		messageBox(s.hwnd, "이미지를 임시 파일에 저장하지 못했습니다.\n\n"+err.Error(), "사용자 정의 이미지", MB_OK|MB_ICONERROR)
		return false
	}
	// A slot can look empty when a damaged old PNG failed to load. Remove any
	// stale destination before the Windows rename so the new valid PNG wins.
	_ = os.Remove(path)
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		messageBox(s.hwnd, "이미지 저장을 완료하지 못했습니다.\n\n"+err.Error(), "사용자 정의 이미지", MB_OK|MB_ICONERROR)
		return false
	}
	s.slots[slotIndex] = customImageSlot{Image: cloneClipboardImage(img), Preview: customImageOpaqueWhitePreview(img), Path: path, Exists: true}
	s.selected = slotIndex
	s.owner.statusText = fmt.Sprintf("사용자 이미지 %02d에 %d×%dpx 이미지를 저장했습니다.", slotIndex+1, img.W, img.H)
	logf("CUSTOM_IMAGE save slot=%d source=%q path=%q w=%d h=%d", slotIndex+1, source, path, img.W, img.H)
	invalidate(s.hwnd)
	invalidate(s.owner.hwnd)
	return true
}

func customImageInsertSelected(s *customImageLibraryState) bool {
	if s == nil || s.owner == nil || s.selected < 0 || s.selected >= customImageMaxSlots || !s.slots[s.selected].Exists {
		return false
	}
	slot := s.slots[s.selected]
	img := cloneClipboardImage(slot.Image)
	if !s.owner.pasteClipboardImage(img, fmt.Sprintf("사용자 이미지 %02d", s.selected+1)) {
		messageBox(s.hwnd, "선택한 이미지를 캔버스에 삽입하지 못했습니다.", "사용자 정의 이미지", MB_OK|MB_ICONERROR)
		return false
	}
	s.owner.statusText = fmt.Sprintf("사용자 이미지 %02d를 삽입했습니다. 떠 있는 선택 영역을 이동하십시오.", s.selected+1)
	logf("CUSTOM_IMAGE insert slot=%d path=%q w=%d h=%d", s.selected+1, slot.Path, slot.Image.W, slot.Image.H)
	procDestroyWindow.Call(s.hwnd)
	return true
}

func customImageDeleteSelected(s *customImageLibraryState) bool {
	if s == nil || s.selected < 0 || s.selected >= customImageMaxSlots || !s.slots[s.selected].Exists {
		return false
	}
	response := messageBox(s.hwnd, fmt.Sprintf("사용자 이미지 %02d를 삭제하시겠습니까?", s.selected+1), "사용자 정의 이미지", MB_YESNOCANCEL|MB_ICONQUESTION)
	if response != IDYES {
		return false
	}
	path := s.slots[s.selected].Path
	if path == "" {
		path = filepath.Join(s.dir, customImageSlotFilename(s.selected))
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		messageBox(s.hwnd, "저장 파일을 삭제하지 못했습니다.\n\n"+err.Error(), "사용자 정의 이미지", MB_OK|MB_ICONERROR)
		return false
	}
	logf("CUSTOM_IMAGE delete slot=%d path=%q", s.selected+1, path)
	s.slots[s.selected] = customImageSlot{}
	invalidate(s.hwnd)
	return true
}

func customImageMoveSelection(s *customImageLibraryState, dx, dy int) {
	if s == nil {
		return
	}
	col := s.selected % 5
	row := s.selected / 5
	col += dx
	row += dy
	if col < 0 {
		col = 0
	}
	if col > 4 {
		col = 4
	}
	if row < 0 {
		row = 0
	}
	if row > 3 {
		row = 3
	}
	next := row*5 + col
	if next != s.selected {
		s.selected = next
		invalidate(s.hwnd)
	}
}

func customImageLibraryWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			logPanic("customImageLibraryWndProc", r)
		}
	}()
	if r, handled := handleDarkEditorNonClient(hwnd, msg, wparam, lparam); handled {
		return r
	}
	s := activeCustomImageLibrary
	switch msg {
	case WM_CREATE:
		if s == nil {
			return ^uintptr(0)
		}
		s.hwnd = hwnd
		s.save = customImageCreateButton(s, "클립보드에서 저장", customImageSaveButtonID, 158)
		s.insert = customImageCreateButton(s, "삽입", customImageInsertButtonID, 90)
		s.delete = customImageCreateButton(s, "삭제", customImageDeleteButtonID, 90)
		s.close = customImageCreateButton(s, "닫기", customImageCloseButtonID, 90)
		customImageLayoutControls(s)
		themeWindowHandles(s.owner.darkMode, s.hwnd, s.save, s.insert, s.delete, s.close)
		scheduleDarkEditorChromeRefresh(hwnd)
		procSetFocus.Call(hwnd)
		return 0
	case WM_CTLCOLORSTATIC, WM_CTLCOLORBTN, WM_CTLCOLORLISTBOX, WM_CTLCOLOREDIT, WM_CTLCOLORDLG:
		if darkModeEnabled() && wparam != 0 && app != nil {
			procSetBkMode.Call(wparam, OPAQUE)
			procSetTextColor.Call(wparam, uintptr(rgb(235, 235, 235)))
			procSetBkColor.Call(wparam, uintptr(rgb(30, 30, 30)))
			if app.darkBrush == 0 {
				app.darkBrush, _, _ = procCreateSolidBrush.Call(uintptr(rgb(30, 30, 30)))
			}
			return app.darkBrush
		}
	case WM_SIZE:
		if s != nil && s.hwnd == hwnd {
			customImageLayoutControls(s)
			invalidate(hwnd)
		}
		return 0
	case WM_COMMAND:
		if s != nil && s.hwnd == hwnd {
			switch int(wparam & 0xffff) {
			case customImageSaveButtonID:
				customImageSaveFromClipboard(s)
			case customImageInsertButtonID:
				customImageInsertSelected(s)
			case customImageDeleteButtonID:
				customImageDeleteSelected(s)
			case customImageCloseButtonID:
				procDestroyWindow.Call(hwnd)
			}
		}
		return 0
	case WM_LBUTTONDOWN:
		if s != nil && s.hwnd == hwnd {
			x, y := int(signedLoword(lparam)), int(signedHiword(lparam))
			if hit := customImageHitTest(s, x, y); hit >= 0 {
				s.selected = hit
				procSetFocus.Call(hwnd)
				invalidate(hwnd)
				logf("CUSTOM_IMAGE select slot=%d exists=%v", hit+1, s.slots[hit].Exists)
			}
		}
		return 0
	case WM_LBUTTONDBLCLK:
		if s != nil && s.hwnd == hwnd {
			x, y := int(signedLoword(lparam)), int(signedHiword(lparam))
			if hit := customImageHitTest(s, x, y); hit >= 0 {
				s.selected = hit
				if s.slots[hit].Exists {
					customImageInsertSelected(s)
				} else {
					customImageSaveFromClipboard(s)
				}
			}
		}
		return 0
	case WM_DRAWITEM:
		if drawThemedOwnerDrawPushButton((*DRAWITEMSTRUCT)(unsafe.Pointer(lparam))) {
			return 1
		}
	case WM_KEYDOWN:
		if s != nil && s.hwnd == hwnd {
			if (wparam == 'V' || wparam == 'v') && keyDown(VK_CONTROL) {
				customImageSaveFromClipboard(s)
				return 0
			}
			switch wparam {
			case VK_ESCAPE:
				procDestroyWindow.Call(hwnd)
			case customImageVKReturn:
				if s.slots[s.selected].Exists {
					customImageInsertSelected(s)
				} else {
					customImageSaveFromClipboard(s)
				}
			case VK_DELETE:
				customImageDeleteSelected(s)
			case VK_LEFT:
				customImageMoveSelection(s, -1, 0)
			case VK_RIGHT:
				customImageMoveSelection(s, 1, 0)
			case VK_UP:
				customImageMoveSelection(s, 0, -1)
			case VK_DOWN:
				customImageMoveSelection(s, 0, 1)
			}
		}
		return 0
	case WM_PAINT:
		if s != nil && s.hwnd == hwnd {
			var ps PAINTSTRUCT
			hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			customImagePaintLibrary(s, hdc)
			procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			return 0
		}
	case WM_ERASEBKGND:
		return 1
	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		if s != nil && s.hwnd == hwnd {
			logf("CUSTOM_IMAGE library closed")
			activeCustomImageLibrary = nil
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}
