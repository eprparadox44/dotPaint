//go:build windows

package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// A79: classic Paint-style Stretch/Skew dialog with alpha-aware optional
// antialiasing. The command remains selection-aware through A73's shared
// transform-selection path, including active text boxes rasterized to a
// text-only floating selection before this dialog opens.

const (
	stretchSkewDialogClass = "dotPaintStretchSkewDialogA133"

	stretchHorizontalEditID  = 7301
	stretchVerticalEditID    = 7302
	skewHorizontalEditID     = 7303
	skewVerticalEditID       = 7304
	stretchSkewAAOffID       = 7305
	stretchSkewAAOnID        = 7306
	stretchHorizontalPixelID = 7307
	stretchVerticalPixelID   = 7308
)

type stretchSkewDialogState struct {
	hwnd, parent uintptr
	uiFont       uintptr

	stretchHEdit      uintptr
	stretchVEdit      uintptr
	stretchHPixelEdit uintptr
	stretchVPixelEdit uintptr
	targetLabel       uintptr
	initialized       bool
	updatingEdits     bool
	skewHEdit         uintptr
	skewVEdit         uintptr
	aaOffButton       uintptr
	aaOnButton        uintptr

	sourceW, sourceH int
	targetName       string
	options          StretchSkewOptions
	accepted         bool
	done             bool
}

var (
	stretchSkewDialog              *stretchSkewDialogState
	stretchSkewDialogWndProcCB     uintptr
	stretchSkewDialogClassReady    bool
	stretchSkewAntialiasPreference = false
)

func ensureStretchSkewDialogClass(hinst uintptr) bool {
	if stretchSkewDialogClassReady {
		return true
	}
	if stretchSkewDialogWndProcCB == 0 {
		stretchSkewDialogWndProcCB = syscall.NewCallback(stretchSkewDialogWndProc)
	}
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		LpfnWndProc:   stretchSkewDialogWndProcCB,
		HInstance:     hinst,
		HCursor:       cursor,
		HbrBackground: uintptr(COLOR_BTNFACE + 1),
		LpszClassName: utf16Ptr(stretchSkewDialogClass),
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		logf("STRETCH_SKEW RegisterClass failed err=%v", err)
		return false
	}
	stretchSkewDialogClassReady = true
	return true
}

// Use up to three decimals for both stretching and skew angles.

func stretchSkewCreateCenteredEdit(parent uintptr, text string, x, y, width, id int) uintptr {
	// ES_NUMBER rejects decimal points and therefore must not be used for the
	// percentage and skew controls. Inputs are validated when parsing instead.
	style := uintptr(WS_CHILD | WS_VISIBLE | WS_BORDER | WS_TABSTOP | ES_CENTER | ES_AUTOHSCROLL)
	hwnd := canvasCreateChild(parent, "EDIT", text, style, x, y, width, 20, id)
	if hwnd != 0 {
		sendMessage(hwnd, EM_SETMARGINS, EC_LEFTMARGIN|EC_RIGHTMARGIN, 0)
	}
	return hwnd
}

// Only accept up to 3 fractional digits; maintain one representation for
// percent <-> pixel syncing, avoiding a repeated-rounding feedback loop.

func (s *stretchSkewDialogState) updateAARadioButtons() {
	if s == nil {
		return
	}
	if s.aaOffButton != 0 {
		check := uintptr(BST_UNCHECKED)
		if !s.options.Antialias {
			check = BST_CHECKED
		}
		sendMessage(s.aaOffButton, BM_SETCHECK, check, 0)
	}
	if s.aaOnButton != 0 {
		check := uintptr(BST_UNCHECKED)
		if s.options.Antialias {
			check = BST_CHECKED
		}
		sendMessage(s.aaOnButton, BM_SETCHECK, check, 0)
	}
	if s.aaOffButton != 0 {
		procInvalidateRect.Call(s.aaOffButton, 0, 1)
	}
	if s.aaOnButton != 0 {
		procInvalidateRect.Call(s.aaOnButton, 0, 1)
	}
}

func (s *stretchSkewDialogState) drawOwnerDrawButton(dis *DRAWITEMSTRUCT) bool {
	if dis == nil {
		return false
	}
	copyDis := *dis
	switch int(dis.CtlID) {
	case stretchSkewAAOffID:
		if !s.options.Antialias {
			copyDis.ItemState |= ODS_SELECTED
		}
	case stretchSkewAAOnID:
		if s.options.Antialias {
			copyDis.ItemState |= ODS_SELECTED
		}
	}
	return drawThemedOwnerDrawPushButton(&copyDis)
}

func parseStretchPercentEdit(hwnd uintptr) (float64, bool) {
	return stretchSkewParseDecimal(getWindowTextString(hwnd), 0.001, 500)
}

func parseSkewAngleEdit(hwnd uintptr) (float64, bool) {
	text := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(getWindowTextString(hwnd)), "°"))
	return stretchSkewParseDecimal(text, -89, 89)
}

func stretchSkewSetEditText(hwnd uintptr, text string) {
	if hwnd != 0 && getWindowTextString(hwnd) != text {
		procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(text))))
	}
}

// WM_COMMAND/EN_CHANGE is sent synchronously by SetWindowText. The guard
// prevents a programmatic update of the paired input from triggering another
// update, while leaving the actively edited field and caret untouched.
func (s *stretchSkewDialogState) syncStretchEdit(id int) {
	if s == nil || !s.initialized || s.updatingEdits {
		return
	}
	s.updatingEdits = true
	defer func() { s.updatingEdits = false }()
	var percentEdit, pixelEdit uintptr
	var source int
	switch id {
	case stretchHorizontalEditID, stretchHorizontalPixelID:
		percentEdit, pixelEdit, source = s.stretchHEdit, s.stretchHPixelEdit, s.sourceW
	case stretchVerticalEditID, stretchVerticalPixelID:
		percentEdit, pixelEdit, source = s.stretchVEdit, s.stretchVPixelEdit, s.sourceH
	default:
		return
	}
	if id == stretchHorizontalEditID || id == stretchVerticalEditID {
		if p, ok := parseStretchPercentEdit(percentEdit); ok {
			stretchSkewSetEditText(pixelEdit, strconv.Itoa(stretchSkewScaledPixels(source, p)))
		}
	} else if pixels, ok := stretchSkewParsePixel(getWindowTextString(pixelEdit), source); ok {
		p := stretchSkewPercentForPixels(source, pixels)
		if p >= 0.001 && p <= 500 {
			stretchSkewSetEditText(percentEdit, stretchSkewCompactFloat(p))
		}
	}
}

// This preview calls the same geometry routine used on confirmation, so the
// arrow includes the final expanded bounds after BOTH shear directions.
func (s *stretchSkewDialogState) updateSizePreview() {
	if s == nil || s.targetLabel == 0 {
		return
	}
	label := s.targetName + " → 입력 중…"
	if opts, ok := s.parseOptions(); ok {
		_, _, ow, oh, geometryOK := stretchSkewGeometry(s.sourceW, s.sourceH, opts)
		if geometryOK {
			label = fmt.Sprintf("%s → %d × %d", s.targetName, ow, oh)
			if ow > 16384 || oh > 16384 || int64(ow)*int64(oh) > 64*1024*1024 {
				label += " (허용 크기 초과)"
			}
		}
	}
	if getWindowTextString(s.targetLabel) != label {
		procSetWindowTextW.Call(s.targetLabel, uintptr(unsafe.Pointer(utf16Ptr(label))))
	}
}

func (s *stretchSkewDialogState) parseOptions() (StretchSkewOptions, bool) {
	if s == nil {
		return StretchSkewOptions{}, false
	}
	hp, okHP := parseStretchPercentEdit(s.stretchHEdit)
	vp, okVP := parseStretchPercentEdit(s.stretchVEdit)
	hs, okHS := parseSkewAngleEdit(s.skewHEdit)
	vs, okVS := parseSkewAngleEdit(s.skewVEdit)
	if !okHP || !okVP || !okHS || !okVS {
		return StretchSkewOptions{}, false
	}
	hpx, okHPx := stretchSkewParsePixel(getWindowTextString(s.stretchHPixelEdit), s.sourceW)
	vpx, okVPx := stretchSkewParsePixel(getWindowTextString(s.stretchVPixelEdit), s.sourceH)
	if !okHPx || !okVPx || hpx != stretchSkewScaledPixels(s.sourceW, hp) || vpx != stretchSkewScaledPixels(s.sourceH, vp) {
		return StretchSkewOptions{}, false
	}
	return StretchSkewOptions{
		StretchHorizontal: hp,
		StretchVertical:   vp,
		SkewHorizontal:    hs,
		SkewVertical:      vs,
		Antialias:         s.options.Antialias,
	}, true
}

func (s *stretchSkewDialogState) resetEditValues() {
	if s == nil {
		return
	}
	procSetWindowTextW.Call(s.stretchHEdit, uintptr(unsafe.Pointer(utf16Ptr(stretchSkewCompactFloat(s.options.StretchHorizontal)))))
	procSetWindowTextW.Call(s.stretchVEdit, uintptr(unsafe.Pointer(utf16Ptr(stretchSkewCompactFloat(s.options.StretchVertical)))))
	procSetWindowTextW.Call(s.skewHEdit, uintptr(unsafe.Pointer(utf16Ptr(stretchSkewCompactFloat(s.options.SkewHorizontal)))))
	procSetWindowTextW.Call(s.skewVEdit, uintptr(unsafe.Pointer(utf16Ptr(stretchSkewCompactFloat(s.options.SkewVertical)))))
	stretchSkewSetEditText(s.stretchHPixelEdit, strconv.Itoa(stretchSkewScaledPixels(s.sourceW, s.options.StretchHorizontal)))
	stretchSkewSetEditText(s.stretchVPixelEdit, strconv.Itoa(stretchSkewScaledPixels(s.sourceH, s.options.StretchVertical)))
	s.updateAARadioButtons()
	s.updateSizePreview()
}

func stretchSkewDialogWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	s := stretchSkewDialog
	defer func() {
		if r := recover(); r != nil {
			logPanic("stretchSkewDialogWndProc", r)
		}
	}()
	switch msg {
	case WM_CREATE:
		if s == nil {
			break
		}
		s.hwnd = hwnd
		applyDarkThemeToWindow(hwnd, darkModeEnabled())
		s.targetLabel = canvasCreateChild(hwnd, "STATIC", s.targetName, WS_CHILD|WS_VISIBLE, 18, 10, 452, 20, 0)

		canvasCreateChild(hwnd, "BUTTON", "늘이기", WS_CHILD|WS_VISIBLE|BS_GROUPBOX, 12, 34, 335, 96, 0)
		canvasCreateChild(hwnd, "STATIC", "가로(H):", WS_CHILD|WS_VISIBLE, 31, 61, 90, 20, 0)
		s.stretchHEdit = stretchSkewCreateCenteredEdit(hwnd, stretchSkewCompactFloat(s.options.StretchHorizontal), 145, 59, 69, stretchHorizontalEditID)
		canvasCreateChild(hwnd, "STATIC", "%", WS_CHILD|WS_VISIBLE, 218, 61, 18, 20, 0)
		s.stretchHPixelEdit = stretchSkewCreateCenteredEdit(hwnd, strconv.Itoa(s.sourceW), 244, 59, 65, stretchHorizontalPixelID)
		canvasCreateChild(hwnd, "STATIC", "px", WS_CHILD|WS_VISIBLE, 314, 61, 25, 20, 0)
		canvasCreateChild(hwnd, "STATIC", "세로(V):", WS_CHILD|WS_VISIBLE, 31, 94, 90, 20, 0)
		s.stretchVEdit = stretchSkewCreateCenteredEdit(hwnd, stretchSkewCompactFloat(s.options.StretchVertical), 145, 92, 69, stretchVerticalEditID)
		canvasCreateChild(hwnd, "STATIC", "%", WS_CHILD|WS_VISIBLE, 218, 94, 18, 20, 0)
		s.stretchVPixelEdit = stretchSkewCreateCenteredEdit(hwnd, strconv.Itoa(s.sourceH), 244, 92, 65, stretchVerticalPixelID)
		canvasCreateChild(hwnd, "STATIC", "px", WS_CHILD|WS_VISIBLE, 314, 94, 25, 20, 0)

		canvasCreateChild(hwnd, "BUTTON", "기울이기", WS_CHILD|WS_VISIBLE|BS_GROUPBOX, 12, 138, 335, 96, 0)
		canvasCreateChild(hwnd, "STATIC", "가로(O):", WS_CHILD|WS_VISIBLE, 31, 165, 90, 20, 0)
		s.skewHEdit = stretchSkewCreateCenteredEdit(hwnd, stretchSkewCompactFloat(s.options.SkewHorizontal), 145, 163, 69, skewHorizontalEditID)
		canvasCreateChild(hwnd, "STATIC", "도", WS_CHILD|WS_VISIBLE, 218, 165, 24, 20, 0)
		canvasCreateChild(hwnd, "STATIC", "세로(E):", WS_CHILD|WS_VISIBLE, 31, 198, 90, 20, 0)
		s.skewVEdit = stretchSkewCreateCenteredEdit(hwnd, stretchSkewCompactFloat(s.options.SkewVertical), 145, 196, 69, skewVerticalEditID)
		canvasCreateChild(hwnd, "STATIC", "도", WS_CHILD|WS_VISIBLE, 218, 198, 24, 20, 0)

		canvasCreateChild(hwnd, "BUTTON", "안티에일리어싱", WS_CHILD|WS_VISIBLE|BS_GROUPBOX, 355, 34, 114, 96, 0)
		buttonStyle := uintptr(WS_CHILD | WS_VISIBLE | WS_TABSTOP | BS_OWNERDRAW)
		s.aaOffButton = canvasCreateChild(hwnd, "BUTTON", "끔", buttonStyle, 372, 76, 34, 29, stretchSkewAAOffID)
		s.aaOnButton = canvasCreateChild(hwnd, "BUTTON", "켬", buttonStyle, 418, 76, 34, 29, stretchSkewAAOnID)
		s.updateAARadioButtons()
		s.initialized = true
		s.updateSizePreview()

		canvasCreateChild(hwnd, "BUTTON", "확인", WS_CHILD|WS_VISIBLE|WS_TABSTOP|BS_DEFPUSHBUTTON, 371, 150, 82, 30, IDOK)
		canvasCreateChild(hwnd, "BUTTON", "취소", WS_CHILD|WS_VISIBLE|WS_TABSTOP, 371, 190, 82, 30, IDCANCEL)
		procSetFocus.Call(s.stretchHEdit)
		procSendMessageW.Call(s.stretchHEdit, EM_SETSEL, 0, ^uintptr(0))
		return 0

	case WM_ERASEBKGND:
		if darkModeEnabled() && wparam != 0 {
			var rc RECT
			procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
			fillRect(wparam, rc, rgb(30, 30, 30))
			return 1
		}
	case WM_CTLCOLOREDIT, WM_CTLCOLORSTATIC, WM_CTLCOLORBTN, WM_CTLCOLORDLG:
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
		case code == EN_CHANGE && (id == stretchHorizontalEditID || id == stretchVerticalEditID || id == stretchHorizontalPixelID || id == stretchVerticalPixelID || id == skewHorizontalEditID || id == skewVerticalEditID):
			if s.initialized && !s.updatingEdits {
				s.syncStretchEdit(id)
				s.updateSizePreview()
			}
			return 0
		case id == stretchSkewAAOffID && code == BN_CLICKED:
			s.options.Antialias = false
			s.updateAARadioButtons()
			return 0
		case id == stretchSkewAAOnID && code == BN_CLICKED:
			s.options.Antialias = true
			s.updateAARadioButtons()
			return 0
		case id == IDOK:
			opts, ok := s.parseOptions()
			if !ok {
				messageBox(hwnd, "늘이기는 0.001~500% (소수 셋째 자리까지) 또는 픽셀 정수로, 기울이기는 -89~89도 (소수 셋째 자리까지)로 입력하십시오.", "dotPaint", MB_OK|MB_ICONWARNING)
				s.resetEditValues()
				procSetFocus.Call(s.stretchHEdit)
				procSendMessageW.Call(s.stretchHEdit, EM_SETSEL, 0, ^uintptr(0))
				return 0
			}
			_, _, ow, oh, okGeometry := stretchSkewGeometry(s.sourceW, s.sourceH, opts)
			if !okGeometry || ow > 16384 || oh > 16384 || int64(ow)*int64(oh) > 64*1024*1024 {
				messageBox(hwnd, fmt.Sprintf("결과 크기 %d × %d가 너무 큽니다. 늘이기나 기울이기 값을 줄이십시오.", ow, oh), "dotPaint", MB_OK|MB_ICONWARNING)
				return 0
			}
			s.options = opts
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
		if s != nil && lparam != 0 && s.drawOwnerDrawButton((*DRAWITEMSTRUCT)(unsafe.Pointer(lparam))) {
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

func resizeCanvasBilinearClamped(src *Canvas, nw, nh int) *Canvas {
	if src == nil || src.W < 1 || src.H < 1 || nw < 1 || nh < 1 {
		return nil
	}
	out := NewCanvas(nw, nh, transparentColor)
	for y := 0; y < nh; y++ {
		sy := (float64(y)+0.5)*float64(src.H)/float64(nh) - 0.5
		sy = math.Max(0, math.Min(float64(src.H-1), sy))
		for x := 0; x < nw; x++ {
			sx := (float64(x)+0.5)*float64(src.W)/float64(nw) - 0.5
			sx = math.Max(0, math.Min(float64(src.W-1), sx))
			p := sourcePixelBilinear(src, sx, sy)
			i := (y*nw + x) * 4
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = p[0], p[1], p[2], p[3]
		}
	}
	return out
}

// stretchSkewGeometry returns the scaled dimensions, then the final expanded
// dimensions after horizontal shear followed by vertical shear. Edge-space
// coordinates make 100%/0° an exact identity and percentage scaling exact.

func shearCanvasExpanded(src *Canvas, horizontalDeg, verticalDeg float64, antialias bool) (*Canvas, int, int) {
	if src == nil || src.W < 1 || src.H < 1 {
		return nil, 0, 0
	}
	sh := math.Tan(horizontalDeg * math.Pi / 180)
	sv := math.Tan(verticalDeg * math.Pi / 180)
	if math.Abs(sh) < 1e-12 && math.Abs(sv) < 1e-12 {
		return &Canvas{W: src.W, H: src.H, Pix: src.ClonePixels()}, 0, 0
	}
	// Horizontal then vertical shear. The determinant is exactly one.
	a, b := 1.0, sh
	c, d := sv, 1+sv*sh
	corners := [4][2]float64{{0, 0}, {float64(src.W), 0}, {0, float64(src.H)}, {float64(src.W), float64(src.H)}}
	minX, maxX := math.Inf(1), math.Inf(-1)
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, p := range corners {
		x := a*p[0] + b*p[1]
		y := c*p[0] + d*p[1]
		minX, maxX = math.Min(minX, x), math.Max(maxX, x)
		minY, maxY = math.Min(minY, y), math.Max(maxY, y)
	}
	left := int(math.Floor(minX + 1e-9))
	right := int(math.Ceil(maxX - 1e-9))
	top := int(math.Floor(minY + 1e-9))
	bottom := int(math.Ceil(maxY - 1e-9))
	ow, oh := max(1, right-left), max(1, bottom-top)
	out := NewCanvas(ow, oh, transparentColor)
	for y := 0; y < oh; y++ {
		gy := float64(top+y) + 0.5
		for x := 0; x < ow; x++ {
			gx := float64(left+x) + 0.5
			// Inverse of [[1,sh],[sv,1+sv*sh]], determinant=1.
			sourceEdgeX := d*gx - b*gy
			sourceEdgeY := -c*gx + a*gy
			sx, sy := sourceEdgeX-0.5, sourceEdgeY-0.5
			var p [4]byte
			if antialias {
				p = sourcePixelBilinear(src, sx, sy)
			} else {
				p = sourcePixelNearest(src, sx, sy)
			}
			i := (y*ow + x) * 4
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = p[0], p[1], p[2], p[3]
		}
	}
	return out, left, top
}

func transformCanvasStretchSkew(src *Canvas, opts StretchSkewOptions) (*Canvas, int, int) {
	if src == nil || src.W < 1 || src.H < 1 {
		return nil, 0, 0
	}
	nw := stretchSkewScaledPixels(src.W, opts.StretchHorizontal)
	nh := stretchSkewScaledPixels(src.H, opts.StretchVertical)
	var scaledCanvas *Canvas
	if opts.Antialias {
		scaledCanvas = resizeCanvasBilinearClamped(src, nw, nh)
	} else {
		scaledCanvas = &Canvas{W: src.W, H: src.H, Pix: src.ClonePixels()}
		scaledCanvas.ResizeNearest(nw, nh, transparentColor)
	}
	if scaledCanvas == nil {
		return nil, 0, 0
	}
	return shearCanvasExpanded(scaledCanvas, opts.SkewHorizontal, opts.SkewVertical, opts.Antialias)
}

func stretchSkewNoop(opts StretchSkewOptions) bool {
	return opts.StretchHorizontal == 100 && opts.StretchVertical == 100 && math.Abs(opts.SkewHorizontal) < 0.0005 && math.Abs(opts.SkewVertical) < 0.0005
}

func (a *App) applyStretchSkew(source *Canvas, opts StretchSkewOptions, toSelection bool) {
	if source == nil || stretchSkewNoop(opts) {
		a.statusText = "늘이기 또는 기울이기 값이 없어 이미지를 변경하지 않았습니다."
		invalidate(a.hwnd)
		return
	}
	transformed, offsetX, offsetY := transformCanvasStretchSkew(source, opts)
	if transformed == nil || transformed.W < 1 || transformed.H < 1 {
		messageBox(a.hwnd, "늘이기 및 기울이기 결과를 만들지 못했습니다.", "dotPaint", MB_OK|MB_ICONERROR)
		return
	}
	if transformed.W > 16384 || transformed.H > 16384 || int64(transformed.W)*int64(transformed.H) > 64*1024*1024 {
		messageBox(a.hwnd, fmt.Sprintf("결과 크기 %d × %d가 너무 큽니다.", transformed.W, transformed.H), "dotPaint", MB_OK|MB_ICONWARNING)
		return
	}
	if toSelection && a.selection.Active {
		nx := a.selection.X + offsetX
		ny := a.selection.Y + offsetY
		status := fmt.Sprintf("선택 영역을 가로 %s%%, 세로 %s%%로 늘이고 가로 %.3f°, 세로 %.3f° 기울였습니다. 결과 %d × %d, 빈 부분은 투명색입니다.", stretchSkewCompactFloat(opts.StretchHorizontal), stretchSkewCompactFloat(opts.StretchVertical), opts.SkewHorizontal, opts.SkewVertical, transformed.W, transformed.H)
		a.installSelectionTransform(transformed, nx, ny, "선택 늘이기/기울이기", status)
	} else {
		a.clearSelectionState()
		a.undo.Push(a.canvas)
		a.canvas = transformed
		a.markDirty()
		a.recordHistory("늘이기/기울이기")
		a.updateScrollbars()
		a.statusText = fmt.Sprintf("전체 이미지를 가로 %s%%, 세로 %s%%로 늘이고 가로 %.3f°, 세로 %.3f° 기울였습니다. 결과 %d × %d, 빈 부분은 투명색입니다.", stretchSkewCompactFloat(opts.StretchHorizontal), stretchSkewCompactFloat(opts.StretchVertical), opts.SkewHorizontal, opts.SkewVertical, transformed.W, transformed.H)
	}
	logf("STRETCH_SKEW apply selection=%v stretch=%.3f,%.3f skew=%.3f,%.3f aa=%v offset=%d,%d result=%dx%d", toSelection, opts.StretchHorizontal, opts.StretchVertical, opts.SkewHorizontal, opts.SkewVertical, opts.Antialias, offsetX, offsetY, transformed.W, transformed.H)
	invalidate(a.hwnd)
	procUpdateWindow.Call(a.hwnd)
}

func (a *App) showStretchSkewDialog() {
	if !ensureStretchSkewDialogClass(a.hinst) {
		messageBox(a.hwnd, "늘이기 및 기울이기 창을 만들지 못했습니다.", "dotPaint", MB_OK|MB_ICONERROR)
		return
	}
	toSelection := a.selection.Active
	var source *Canvas
	targetName := fmt.Sprintf("대상: 전체 이미지 %d × %d", a.canvas.W, a.canvas.H)
	if toSelection {
		source = a.selectionTransformSource()
		targetName = fmt.Sprintf("대상: 선택 영역 %d × %d", a.selection.W, a.selection.H)
	} else {
		source = &Canvas{W: a.canvas.W, H: a.canvas.H, Pix: a.canvas.ClonePixels()}
	}
	if source == nil {
		return
	}
	state := &stretchSkewDialogState{
		parent: a.hwnd, uiFont: createCompactEditorUIFont(), sourceW: source.W, sourceH: source.H, targetName: targetName,
		options: StretchSkewOptions{StretchHorizontal: 100, StretchVertical: 100, Antialias: stretchSkewAntialiasPreference},
	}
	stretchSkewDialog = state

	style := uintptr(WS_CAPTION | WS_SYSMENU)
	exStyle := uintptr(WS_EX_DLGMODALFRAME)
	clientBounds := RECT{0, 0, 484, 252}
	procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&clientBounds)), style, 0, exStyle)
	w := int(clientBounds.Right - clientBounds.Left)
	h := int(clientBounds.Bottom - clientBounds.Top)
	hwnd, _, err := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(utf16Ptr(stretchSkewDialogClass))),
		uintptr(unsafe.Pointer(utf16Ptr("늘이기 및 기울이기"))),
		style,
		uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), uintptr(w), uintptr(h),
		a.hwnd, 0, a.hinst, 0,
	)
	if hwnd == 0 {
		if state.uiFont != 0 {
			procDeleteObject.Call(state.uiFont)
			state.uiFont = 0
		}
		stretchSkewDialog = nil
		logf("STRETCH_SKEW CreateWindow failed err=%v", err)
		messageBox(a.hwnd, "늘이기 및 기울이기 창을 만들지 못했습니다.", "dotPaint", MB_OK|MB_ICONERROR)
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
	procSetFocus.Call(state.stretchHEdit)
	logf("STRETCH_SKEW dialog open selection=%v source=%dx%d aa=%v", toSelection, source.W, source.H, state.options.Antialias)

	var msg MSG
	for !state.done {
		r, _, getErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			if int32(r) < 0 {
				logf("STRETCH_SKEW GetMessage failed err=%v", getErr)
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
	opts := state.options
	if accepted {
		stretchSkewAntialiasPreference = opts.Antialias
	}
	if state.hwnd != 0 {
		procDestroyWindow.Call(state.hwnd)
	}
	stretchSkewDialog = nil
	if accepted {
		a.applyStretchSkew(source, opts, toSelection)
	} else {
		a.statusText = "늘이기 및 기울이기를 취소했습니다."
		invalidate(a.hwnd)
		logf("STRETCH_SKEW dialog cancel")
	}
}
