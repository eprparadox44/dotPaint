//go:build windows

package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/png"
	"math"
	"syscall"
	"unsafe"
)

const (
	animalLibraryClassName = "dotPaintAnimalLibraryA222"
	animalInsertButtonID   = 7701
	animalCloseButtonID    = 7702
	animalVKReturn         = 0x0D
)

// The user supplied these three animal PNGs. They are embedded byte-for-byte
// and shown in sorted order inside the chooser.

//go:embed assets/animal/adelie_penguin_1.png
var animalAdeliePenguin1PNG []byte

//go:embed assets/animal/adelie_penguin_2.png
var animalAdeliePenguin2PNG []byte

//go:embed assets/animal/mouse.png
var animalMousePNG []byte

type animalSymbolSpec struct {
	Name string
	Data []byte
}

type animalSymbol struct {
	Name    string
	Image   ClipboardImage
	Preview []byte // opaque BGRA composited on white for chooser only
}

type animalLibraryState struct {
	owner    *App
	hwnd     uintptr
	insert   uintptr
	close    uintptr
	selected int
	symbols  []animalSymbol
}

var (
	animalLibraryWndProcCallback uintptr
	animalLibraryClassRegistered bool
	activeAnimalLibrary          *animalLibraryState
)

func animalSymbolSpecs() []animalSymbolSpec {
	return []animalSymbolSpec{
		{Name: "아델리 펭귄1", Data: animalAdeliePenguin1PNG},
		{Name: "아델리 펭귄2", Data: animalAdeliePenguin2PNG},
		{Name: "쥐", Data: animalMousePNG},
	}
}

func animalDecodePNG(data []byte) (ClipboardImage, error) {
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return ClipboardImage{}, err
	}
	b := decoded.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 || w > 4096 || h > 4096 {
		return ClipboardImage{}, fmt.Errorf("잘못된 동물 이미지 크기: %d x %d", w, h)
	}
	out := ClipboardImage{W: w, H: h, Pix: make([]byte, w*h*4)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b16, a := decoded.At(decoded.Bounds().Min.X+x, decoded.Bounds().Min.Y+y).RGBA()
			rr, gg, bb, aa := rgba16ToStraight8(r, g, b16, a)
			i := (y*w + x) * 4
			out.Pix[i+0] = bb
			out.Pix[i+1] = gg
			out.Pix[i+2] = rr
			out.Pix[i+3] = aa
		}
	}
	return out, nil
}

func animalOpaqueWhitePreview(img ClipboardImage) []byte {
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

func loadAnimalSymbols() ([]animalSymbol, error) {
	specs := animalSymbolSpecs()
	out := make([]animalSymbol, 0, len(specs))
	for _, spec := range specs {
		img, err := animalDecodePNG(spec.Data)
		if err != nil {
			return nil, fmt.Errorf("%s PNG 해석 실패: %w", spec.Name, err)
		}
		out = append(out, animalSymbol{Name: spec.Name, Image: img, Preview: animalOpaqueWhitePreview(img)})
	}
	return out, nil
}

func registerAnimalLibraryClass(hinst uintptr) bool {
	if animalLibraryClassRegistered {
		return true
	}
	if animalLibraryWndProcCallback == 0 {
		animalLibraryWndProcCallback = syscall.NewCallback(animalLibraryWndProc)
	}
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         CS_HREDRAW | CS_VREDRAW | CS_DBLCLKS,
		LpfnWndProc:   animalLibraryWndProcCallback,
		HInstance:     hinst,
		HCursor:       cursor,
		HbrBackground: uintptr(COLOR_BTNFACE + 1),
		LpszClassName: utf16Ptr(animalLibraryClassName),
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		logf("ANIMAL register class failed: %v", err)
		return false
	}
	animalLibraryClassRegistered = true
	return true
}

func (a *App) showAnimalLibrary() {
	if activeAnimalLibrary != nil && activeAnimalLibrary.hwnd != 0 {
		if ok, _, _ := procIsWindow.Call(activeAnimalLibrary.hwnd); ok != 0 {
			procShowWindow.Call(activeAnimalLibrary.hwnd, SW_SHOW)
			procSetForegroundWindow.Call(activeAnimalLibrary.hwnd)
			return
		}
	}
	if !registerAnimalLibraryClass(a.hinst) {
		messageBox(a.hwnd, "동물 이미지 선택 창을 만들 수 없습니다.", "삽입 > 이미지 > 동물", MB_OK|MB_ICONERROR)
		return
	}
	symbols, err := loadAnimalSymbols()
	if err != nil {
		logf("ANIMAL load failed: %v", err)
		messageBox(a.hwnd, err.Error(), "동물 이미지", MB_OK|MB_ICONERROR)
		return
	}
	state := &animalLibraryState{owner: a, selected: 0, symbols: symbols}
	activeAnimalLibrary = state
	style := uintptr(WS_CAPTION | WS_SYSMENU | WS_MINIMIZEBOX)
	hwnd, _, createErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(animalLibraryClassName))),
		uintptr(unsafe.Pointer(utf16Ptr("동물 이미지"))),
		style,
		uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), 820, 480,
		a.hwnd, 0, a.hinst, 0,
	)
	if hwnd == 0 {
		activeAnimalLibrary = nil
		logf("ANIMAL create window failed: %v", createErr)
		messageBox(a.hwnd, "동물 이미지 선택 창을 만들 수 없습니다.", "삽입 > 이미지 > 동물", MB_OK|MB_ICONERROR)
		return
	}
	state.hwnd = hwnd
	showThemedEditorWindow(hwnd, a.darkMode)
	a.statusText = "동물 이미지를 선택하십시오. 더블클릭하거나 삽입 버튼을 누르면 캔버스에 배치됩니다."
	logf("ANIMAL library opened hwnd=%#x symbols=%d", hwnd, len(symbols))
	invalidate(a.hwnd)
}

func animalCreateButton(s *animalLibraryState, title string, id int) uintptr {
	h, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr("BUTTON"))),
		uintptr(unsafe.Pointer(utf16Ptr(title))),
		ownerDrawPushButtonStyle("BUTTON", WS_CHILD|WS_VISIBLE|WS_TABSTOP),
		0, 0, 90, 30,
		s.hwnd, uintptr(id), s.owner.hinst, 0,
	)
	if h != 0 {
		font, _, _ := procGetStockObject.Call(DEFAULT_GUI_FONT)
		sendMessage(h, WM_SETFONT, font, 1)
		applyDarkThemeToWindow(h, s.owner.darkMode)
	}
	return h
}

func animalLayoutControls(s *animalLibraryState) {
	if s == nil || s.hwnd == 0 {
		return
	}
	var client RECT
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	w, h := int(client.Right-client.Left), int(client.Bottom-client.Top)
	buttonY := max(10, h-46)
	closeX := max(10, w-102)
	insertX := max(10, closeX-98)
	if s.insert != 0 {
		procMoveWindow.Call(s.insert, uintptr(insertX), uintptr(buttonY), 90, 30, 1)
	}
	if s.close != 0 {
		procMoveWindow.Call(s.close, uintptr(closeX), uintptr(buttonY), 90, 30, 1)
	}
}

func animalCardRects(client RECT, count int) []RECT {
	columns := count
	if columns < 1 {
		columns = 1
	}
	if columns > 3 {
		columns = 3
	}
	margin, gap := 12, 10
	headerH, bottomH := 42, 66
	w, h := int(client.Right-client.Left), int(client.Bottom-client.Top)
	availableW := max(columns, w-margin*2-gap*(columns-1))
	cardW := max(1, availableW/columns)
	rows := (count + columns - 1) / columns
	if rows < 1 {
		rows = 1
	}
	availableH := max(rows, h-headerH-bottomH-gap*(rows-1))
	cardH := max(1, availableH/rows)
	out := make([]RECT, count)
	for i := 0; i < count; i++ {
		col, row := i%columns, i/columns
		left := margin + col*(cardW+gap)
		top := headerH + row*(cardH+gap)
		out[i] = RECT{int32(left), int32(top), int32(left + cardW), int32(top + cardH)}
	}
	return out
}

func animalHitTest(s *animalLibraryState, x, y int) int {
	if s == nil || s.hwnd == 0 {
		return -1
	}
	var client RECT
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	for i, rc := range animalCardRects(client, len(s.symbols)) {
		if x >= int(rc.Left) && x < int(rc.Right) && y >= int(rc.Top) && y < int(rc.Bottom) {
			return i
		}
	}
	return -1
}

func animalPaintSymbol(hdc uintptr, symbol animalSymbol, rc RECT) {
	if symbol.Image.W < 1 || symbol.Image.H < 1 || len(symbol.Preview) != symbol.Image.W*symbol.Image.H*4 {
		return
	}
	labelH := 28
	previewArea := RECT{rc.Left + 7, rc.Top + 7, rc.Right - 7, rc.Bottom - int32(labelH+5)}
	fillRect(hdc, previewArea, rgb(255, 255, 255))
	frameRect(hdc, previewArea, rgb(205, 205, 205))
	availW := max(1, int(previewArea.Right-previewArea.Left)-10)
	availH := max(1, int(previewArea.Bottom-previewArea.Top)-10)
	scale := math.Min(float64(availW)/float64(symbol.Image.W), float64(availH)/float64(symbol.Image.H))
	if scale > 2.5 {
		scale = 2.5
	}
	if scale <= 0 {
		return
	}
	dw := max(1, int(math.Round(float64(symbol.Image.W)*scale)))
	dh := max(1, int(math.Round(float64(symbol.Image.H)*scale)))
	dx := int(previewArea.Left) + (int(previewArea.Right-previewArea.Left)-dw)/2
	dy := int(previewArea.Top) + (int(previewArea.Bottom-previewArea.Top)-dh)/2
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{
		BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(symbol.Image.W), BiHeight: -int32(symbol.Image.H),
		BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB,
	}}
	procSetStretchBltMode.Call(hdc, COLORONCOLOR)
	procStretchDIBits.Call(
		hdc, uintptr(dx), uintptr(dy), uintptr(dw), uintptr(dh),
		0, 0, uintptr(symbol.Image.W), uintptr(symbol.Image.H),
		uintptr(unsafe.Pointer(&symbol.Preview[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, SRCCOPY,
	)
	label := RECT{rc.Left + 4, rc.Bottom - int32(labelH), rc.Right - 4, rc.Bottom - 2}
	drawUIText(hdc, symbol.Name, label, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS, rgb(20, 20, 20))
}

func animalPaintLibrary(s *animalLibraryState, hdc uintptr) {
	var client RECT
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	fillUIRect(hdc, client, rgb(240, 240, 240))
	header := RECT{12, 8, client.Right - 12, 36}
	drawUIText(hdc, "동물 이미지를 선택하세요. 더블클릭하면 바로 삽입됩니다.", header, DT_LEFT|DT_VCENTER|DT_SINGLELINE, rgb(25, 25, 25))

	cards := animalCardRects(client, len(s.symbols))
	for i, rc := range cards {
		fillUIRect(hdc, rc, rgb(248, 248, 248))
		if i == s.selected {
			frameUIRect(hdc, rc, rgb(0, 110, 215))
			inner := RECT{rc.Left + 1, rc.Top + 1, rc.Right - 1, rc.Bottom - 1}
			frameUIRect(hdc, inner, rgb(0, 110, 215))
		} else {
			frameUIRect(hdc, rc, rgb(170, 170, 170))
		}
		animalPaintSymbol(hdc, s.symbols[i], rc)
	}

	selected := "선택된 동물 없음"
	if s.selected >= 0 && s.selected < len(s.symbols) {
		sym := s.symbols[s.selected]
		selected = fmt.Sprintf("선택: %s  ·  원본 %d × %d px", sym.Name, sym.Image.W, sym.Image.H)
	}
	footer := RECT{12, client.Bottom - 46, client.Right - 220, client.Bottom - 16}
	drawUIText(hdc, selected, footer, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS, rgb(35, 35, 35))
}

func animalInsertSelected(s *animalLibraryState) bool {
	if s == nil || s.owner == nil || s.selected < 0 || s.selected >= len(s.symbols) {
		return false
	}
	symbol := s.symbols[s.selected]
	img := cloneClipboardImage(symbol.Image)
	if !s.owner.pasteClipboardImage(img, "동물: "+symbol.Name) {
		messageBox(s.hwnd, "선택한 동물 이미지를 캔버스에 삽입하지 못했습니다.", "동물 이미지", MB_OK|MB_ICONERROR)
		return false
	}
	s.owner.statusText = fmt.Sprintf("%s 이미지를 삽입했습니다. 떠 있는 선택 영역을 원하는 위치로 이동하십시오.", symbol.Name)
	logf("ANIMAL insert name=%q w=%d h=%d alpha=true", symbol.Name, symbol.Image.W, symbol.Image.H)
	procDestroyWindow.Call(s.hwnd)
	return true
}

func animalMoveSelection(s *animalLibraryState, delta int) {
	if s == nil || len(s.symbols) == 0 {
		return
	}
	next := s.selected + delta
	if next < 0 {
		next = 0
	}
	if next >= len(s.symbols) {
		next = len(s.symbols) - 1
	}
	if next != s.selected {
		s.selected = next
		invalidate(s.hwnd)
	}
}

func animalLibraryWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			logPanic("animalLibraryWndProc", r)
		}
	}()
	if r, handled := handleDarkEditorNonClient(hwnd, msg, wparam, lparam); handled {
		return r
	}
	s := activeAnimalLibrary
	switch msg {
	case WM_CREATE:
		if s == nil {
			return ^uintptr(0)
		}
		s.hwnd = hwnd
		s.insert = animalCreateButton(s, "삽입", animalInsertButtonID)
		s.close = animalCreateButton(s, "닫기", animalCloseButtonID)
		animalLayoutControls(s)
		themeWindowHandles(s.owner.darkMode, s.hwnd, s.insert, s.close)
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
			animalLayoutControls(s)
			invalidate(hwnd)
		}
		return 0
	case WM_COMMAND:
		if s != nil && s.hwnd == hwnd {
			switch int(wparam & 0xffff) {
			case animalInsertButtonID:
				animalInsertSelected(s)
			case animalCloseButtonID:
				procDestroyWindow.Call(hwnd)
			}
		}
		return 0
	case WM_LBUTTONDOWN:
		if s != nil && s.hwnd == hwnd {
			x, y := int(signedLoword(lparam)), int(signedHiword(lparam))
			if hit := animalHitTest(s, x, y); hit >= 0 {
				s.selected = hit
				procSetFocus.Call(hwnd)
				invalidate(hwnd)
				logf("ANIMAL select index=%d name=%q", hit, s.symbols[hit].Name)
			}
		}
		return 0
	case WM_LBUTTONDBLCLK:
		if s != nil && s.hwnd == hwnd {
			x, y := int(signedLoword(lparam)), int(signedHiword(lparam))
			if hit := animalHitTest(s, x, y); hit >= 0 {
				s.selected = hit
				animalInsertSelected(s)
			}
		}
		return 0
	case WM_DRAWITEM:
		if drawThemedOwnerDrawPushButton((*DRAWITEMSTRUCT)(unsafe.Pointer(lparam))) {
			return 1
		}
	case WM_KEYDOWN:
		if s != nil && s.hwnd == hwnd {
			switch wparam {
			case VK_ESCAPE:
				procDestroyWindow.Call(hwnd)
			case animalVKReturn:
				animalInsertSelected(s)
			case VK_LEFT:
				animalMoveSelection(s, -1)
			case VK_RIGHT:
				animalMoveSelection(s, 1)
			case VK_UP:
				animalMoveSelection(s, -3)
			case VK_DOWN:
				animalMoveSelection(s, 3)
			}
		}
		return 0
	case WM_PAINT:
		if s != nil && s.hwnd == hwnd {
			var ps PAINTSTRUCT
			hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			animalPaintLibrary(s, hdc)
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
			logf("ANIMAL library closed")
			activeAnimalLibrary = nil
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}
