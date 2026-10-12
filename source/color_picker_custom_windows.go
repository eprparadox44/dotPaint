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

// The native ChooseColor dialog is deliberately no longer the default.
// It could not be themed consistently, and its layout was not suitable for
// simultaneously viewing all 256 indexed colours. No additional GUI toolkit.
const (
	pickerWindowClass  = "dotPaintColorEditor149"
	pickerClientWidth  = 765
	pickerBaseHeight   = 432
	pickerGridLeft     = 22
	pickerGridTop      = 435
	pickerGridStep     = 22
	pickerGridSize     = 20
	pickerMainTop      = 256
	pickerMainStep     = 24
	pickerMainSize     = 20
	pickerModeTop      = 349
	pickerHueLeft      = 284
	pickerHueTop       = 37
	pickerHueWidth     = 169
	pickerHueHeight    = 142
	pickerValueLeft    = 465
	pickerNumberBase   = 50000
	pickerModeRGB      = 50200
	pickerMode16       = 50201
	pickerMode256      = 50202
	pickerToggle       = 50203
	pickerLoad         = 50204
	pickerSave         = 50205
	pickerAddCustom    = 50206
	pickerApplyIndexed = 50207
	pickerColorCount   = 50208
	pickerENKILLFOCUS  = 0x0200
	pickerENCHANGE     = 0x0300
)

type customPickerState struct {
	hwnd, parent, font, smallFont     uintptr
	edits                             [6]uintptr
	colour                            uint32
	hue, sat, value                   float64
	mode                              int // 0, 16, 256
	palette                           []uint32
	baseColors                        []uint32 // main editor RGB palette, always 28 slots
	mainSelected                      int      // 0..27 basic / 28..43 user, -1 none
	appliedMode                       int      // palette shown in the upper CURRENT palette pane
	appliedPalette                    []uint32 // immutable-size copy of the last explicitly applied indexed palette
	appliedCount, colorCount          int      // active colour count (separate from 16/256 table capacity)
	draft16Count, draft256Count       int
	displayRGB, display16, display256 int // visible palette-panel slots, not indexed-document colours
	countEditedByUser                 bool
	editTarget                        int // 0=free RGB, 1=main RGB palette, 2=indexed palette
	mainChanged                       bool
	draft16, draft256                 []uint32
	selected                          int
	edited                            bool
	expanded                          bool
	changed                           bool
	accepted, done, updating          bool
	background                        bool
	initial                           uint32
	custom                            [16]uint32
	customNext                        int
	dragging                          int
	controls                          []uintptr
	toggle, applyButton, countEdit    uintptr
	buttons                           [8]uintptr
	gridRows                          int
	width, height                     int
}

var (
	customPickerClassReady  bool
	customPickerWndCallback uintptr
	customPickerCurrent     *customPickerState
)

// Indexed Apply/Load/Save buttons sit above the swatches in one header row.
// OK/Cancel occupy one footer row. All 256 indices remain simultaneously visible.
func customPickerFooterTop(rows int) int {
	if rows == 0 {
		return pickerBaseHeight - 44
	}
	return pickerGridTop + rows*pickerGridStep + 18 // grid bottom + footer gap
}
func customPickerHeight(rows int) int {
	if rows == 0 {
		return pickerBaseHeight
	}
	return customPickerFooterTop(rows) + 42 // 28px buttons + 14px margin
}
func (s *customPickerState) rows() int {
	if !s.expanded || s.mode == 0 {
		return 0
	}
	_, rows := customPickerIndexedGrid(s.mode)
	return rows
}
func (s *customPickerState) createChild(class, label string, style uintptr, x, y, w, h, id int) uintptr {
	hnd, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16Ptr(class))), uintptr(unsafe.Pointer(utf16Ptr(label))), ownerDrawPushButtonStyle(class, style), uintptr(x), uintptr(y), uintptr(w), uintptr(h), s.hwnd, uintptr(id), app.hinst, 0)
	if hnd != 0 {
		setCompactEditorControlFont(hnd, s.font)
		applyDarkThemeToWindow(hnd, darkModeEnabled())
		s.controls = append(s.controls, hnd)
	}
	return hnd
}
func (s *customPickerState) label(text string, x, y, w int) {
	s.createChild("STATIC", text, WS_CHILD|WS_VISIBLE, x, y, w, 20, 0)
}
func (s *customPickerState) button(text string, x, y, w, h, id int) uintptr {
	return s.createChild("BUTTON", text, WS_CHILD|WS_VISIBLE|WS_TABSTOP, x, y, w, h, id)
}
func (s *customPickerState) setLabel(h uintptr, str string) { paletteFooterSetText(h, str) }
func (s *customPickerState) setFields(skip int) {
	s.updating = true
	values := [6]int{int(byte(s.colour)), int(byte(s.colour >> 8)), int(byte(s.colour >> 16)), int(math.Round(s.hue)) % 360, int(math.Round(s.sat * 100)), int(math.Round(s.value * 100))}
	for i, h := range s.edits {
		if h != 0 && i != skip {
			s.setLabel(h, strconv.Itoa(values[i]))
		}
	}
	s.updating = false
}
func (s *customPickerState) chooseColor(c uint32, mutate bool, skip int) {
	s.colour = c & 0xffffff
	s.hue, s.sat, s.value = customPickerRGBToHSV(s.colour)
	if mutate {
		s.commitEditedSwatch()
	}
	s.setFields(skip)
	procInvalidateRect.Call(s.hwnd, 0, 0)
}

// HSV selection must preserve hue and saturation even when V=0 (black) or
// S=0 (grey). RGB-to-HSV roundtripping would otherwise reset hue to zero.
func (s *customPickerState) chooseHSV(h, sat, value float64, skip int) {
	s.colour = customPickerHSVToRGB(h, sat, value)
	s.hue, s.sat, s.value = math.Mod(h, 360), sat, value
	s.commitEditedSwatch()
	s.setFields(skip)
	procInvalidateRect.Call(s.hwnd, 0, 0)
}

// Only the explicitly selected palette family is edited. Changing the 48
// preset colours never silently overwrites a different palette family.
func (s *customPickerState) commitEditedSwatch() {
	switch s.editTarget {
	case 1:
		if s.mainSelected < 0 || s.mainSelected >= 44 {
			return
		}
		if s.mainSelected < 28 {
			if customPickerReplaceIndex(s.baseColors, s.mainSelected, s.colour) {
				s.mainChanged = true
			}
		} else if s.custom[s.mainSelected-28] != s.colour {
			s.custom[s.mainSelected-28] = s.colour
			s.mainChanged = true
		}
	case 2:
		if s.mode != 0 && customPickerReplaceIndex(s.palette, s.selected, s.colour) {
			s.changed = true
			s.edited = true
			if s.appliedMode == s.mode && s.selected >= 0 && s.selected < len(s.appliedPalette) {
				s.appliedPalette[s.selected] = s.colour
			}
		}
	}
}

// The native main-window palette is 14 basic + 8 user slots per row.
// The editor uses exactly the same 44-slot ordering, so an edited slot has
// identical meaning in the main window and in this dialog.
func (s *customPickerState) mainSwatch(i int) uint32 {
	if i < 28 && i >= 0 && i < len(s.baseColors) {
		return s.baseColors[i]
	}
	if i >= 28 && i < 44 {
		return s.custom[i-28]
	}
	return 0
}
func (s *customPickerState) chooseMainSlot(i int) {
	if i < 0 || i >= 44 {
		return
	}
	s.mainSelected = i
	s.selected = -1 // a main RGB slot is not also an indexed slot
	s.editTarget = 1
	s.chooseColor(s.mainSwatch(i), false, -1)
}

// The top panel displays the palette that the user has actually chosen to
// show, not an unrelated copy of the old 28+16 RGB colour strip. The Apply
// button changes this preview while the dialog is open; OK commits the edits.
func (s *customPickerState) applyIndexedToCurrent() {
	if s.mode != 16 && s.mode != 256 {
		return
	}
	if len(s.palette) != s.mode {
		return
	}
	s.appliedMode = s.mode
	s.appliedPalette = append([]uint32(nil), s.palette...)
	s.appliedCount = s.colorCount
	procInvalidateRect.Call(s.hwnd, 0, 0)
}

func (s *customPickerState) chooseCurrentIndexedSlot(i int) {
	if s.appliedMode == 0 || i < 0 || i >= len(s.appliedPalette) {
		return
	}
	if s.mode != s.appliedMode {
		s.modeChange(s.appliedMode)
		// The preview is the authoritative starting point when selecting it.
		s.palette = append([]uint32(nil), s.appliedPalette...)
	}
	s.selected = i
	s.mainSelected = -1
	s.editTarget = 2
	s.edited = false
	s.chooseColor(s.palette[i], false, -1)
}

func (s *customPickerState) fromNumbers(changed int) {
	if s.updating {
		return
	}
	vals := [6]int{}
	for i, h := range s.edits {
		n, err := strconv.Atoi(strings.TrimSpace(getWindowTextString(h)))
		if err != nil || n < 0 || (i < 3 && n > 255) || (i == 3 && n > 360) || (i >= 4 && n > 100) {
			return
		}
		vals[i] = n
	}
	if changed >= 3 {
		s.chooseHSV(float64(vals[3]), float64(vals[4])/100, float64(vals[5])/100, changed)
	} else {
		c := rgb(byte(vals[0]), byte(vals[1]), byte(vals[2]))
		s.chooseColor(c, true, changed)
	}
}
func (s *customPickerState) setCountText(text string) {
	s.updating = true
	paletteFooterSetText(s.countEdit, text)
	s.updating = false
	s.countEditedByUser = false
}
func (s *customPickerState) displayCountFor(mode int) int {
	if app != nil && app.dejavu != nil && mode == 16 {
		return 16
	}
	switch mode {
	case 16:
		return paletteDisplayClamp(16, s.display16)
	case 256:
		return paletteDisplayClamp(256, s.display256)
	default:
		return paletteDisplayClamp(0, s.displayRGB)
	}
}
func (s *customPickerState) storeDisplayCount(mode, count int) {
	count = paletteDisplayClamp(mode, count)
	switch mode {
	case 16:
		s.display16 = count
	case 256:
		s.display256 = count
	default:
		s.displayRGB = count
	}
}
func (s *customPickerState) captureDisplayCount() {
	if s.countEdit == 0 {
		return
	}
	num, err := strconv.Atoi(strings.TrimSpace(getWindowTextString(s.countEdit)))
	if err == nil && num >= 1 && num <= paletteDisplayCapacity(s.mode) {
		s.storeDisplayCount(s.mode, num)
	}
}
func (s *customPickerState) modeChange(size int) {
	if app.dejavu != nil && size != 16 {
		messageBox(s.hwnd, "DejaVu IMG는 16색 인덱스 형식입니다.", "색 편집", MB_OK|MB_ICONINFORMATION)
		return
	}
	if s.mode == size {
		return
	}
	// Keep the current visible count when switching editing tabs; no image conversion.
	s.captureDisplayCount()
	// Preserve edits when toggling between the two indexed palettes before OK.
	if s.mode == 16 {
		s.draft16 = append([]uint32(nil), s.palette...)
		s.draft16Count = s.colorCount
	}
	if s.mode == 256 {
		s.draft256 = append([]uint32(nil), s.palette...)
		s.draft256Count = s.colorCount
	}
	s.mode = size
	if size == 16 {
		s.colorCount = indexedUsableCount(16, s.draft16Count)
	}
	if size == 256 {
		s.colorCount = indexedUsableCount(256, s.draft256Count)
	}
	if size == 16 {
		// Restore the stored 16-colour palette instead of silently truncating 256.
		if len(s.draft16) == 16 {
			s.palette = append([]uint32(nil), s.draft16...)
		} else {
			p := app.indexedColors
			s.palette = append([]uint32(nil), p[:]...)
		}
	} else if size == 256 {
		if len(s.draft256) == 256 {
			s.palette = append([]uint32(nil), s.draft256...)
		} else if len(app.indexedColors256) == 256 {
			s.palette = append([]uint32(nil), app.indexedColors256...)
		} else {
			s.palette = defaultIndexed256Palette(s.palette)
		}
	}
	s.selected = -1
	s.editTarget = 0
	if size != 0 {
		s.expanded = true
	} else {
		s.expanded = false
		s.appliedMode = 0
		s.appliedPalette = nil
		s.appliedCount = 0
	}
	if size == 0 {
		s.mainSelected = customPickerInitialMainSlot(s.baseColors, s.custom, s.colour, s.mainSelected)
		if s.mainSelected >= 0 {
			s.editTarget = 1
		}
	}
	s.edited = false
	s.changed = true
	if s.countEdit != 0 {
		s.setCountText(strconv.Itoa(s.displayCountFor(size)))
	}
	if size == 256 {
		s.expanded = true
	}
	s.updateLayout()
}

// Read ONLY the number of swatches displayed in the floating palette window.
// This must never switch RGB/16/256 mode or reduce indexed quantization count.
func (s *customPickerState) readColorCount() bool {
	if s.countEdit == 0 {
		return true
	}
	if app.dejavu != nil {
		return true
	}
	num, err := strconv.Atoi(strings.TrimSpace(getWindowTextString(s.countEdit)))
	cap := paletteDisplayCapacity(s.mode)
	if err != nil || num < 1 || num > cap {
		messageBox(s.hwnd, fmt.Sprintf("표시 색상 수는 1~%d 사이여야 합니다.\n색상 모드와 이미지 인덱스는 변경되지 않습니다.", cap), "색상표 표시 개수", MB_OK|MB_ICONWARNING)
		return false
	}
	s.storeDisplayCount(s.mode, num)
	return true
}

func (s *customPickerState) updateLayout() {
	rows := s.rows()
	if s.countEdit != 0 {
		if app.dejavu == nil {
			procEnableWindow.Call(s.countEdit, 1)
		} else {
			procEnableWindow.Call(s.countEdit, 0)
		}
	}
	s.gridRows = rows
	s.height = customPickerHeight(rows)
	if s.applyButton != 0 {
		if rows > 0 {
			procShowWindow.Call(s.applyButton, SW_SHOW)
		} else {
			procShowWindow.Call(s.applyButton, SW_HIDE)
		}
	}
	for i, h := range s.buttons {
		if i >= 4 && i <= 5 && h != 0 {
			show := rows > 0
			if show {
				procShowWindow.Call(h, SW_SHOW)
			} else {
				procShowWindow.Call(h, SW_HIDE)
			}
		}
	}
	if s.toggle != 0 {
		label := "▸ 인덱스 팔레트"
		if s.expanded && s.mode != 0 {
			label = "▾ 인덱스 팔레트"
		}
		paletteFooterSetText(s.toggle, label)
	}
	for i, name := range []string{"일반 RGB", "16색", "256색"} {
		if s.buttons[i] != 0 {
			if (i == 0 && s.mode == 0) || (i == 1 && s.mode == 16) || (i == 2 && s.mode == 256) {
				name = "[" + name + "]"
			}
			paletteFooterSetText(s.buttons[i], name)
		}
	}
	// Native dialog font and controls stay stationary; only the indexed section grows.
	var bounds RECT
	procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&bounds)), WS_CAPTION|WS_SYSMENU, 0, WS_EX_DLGMODALFRAME)
	w := pickerClientWidth + int(bounds.Right-bounds.Left)
	h := s.height + int(bounds.Bottom-bounds.Top)
	var old RECT
	procGetWindowRect.Call(s.hwnd, uintptr(unsafe.Pointer(&old)))
	x, y := int(old.Left), int(old.Top)
	x, y = pickerVisiblePosition(s.parent, x, y, w, h)
	procSetWindowPos.Call(s.hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), SWP_NOZORDER)
	s.placeBottomButtons()
	procInvalidateRect.Call(s.hwnd, 0, 1)
}

// Reposition after expanding so the 16x16 grid does not disappear below
// the taskbar when the colour dialog was initially centered as a small window.
func pickerVisiblePosition(owner uintptr, x, y, w, h int) (int, int) {
	monitor, _, _ := procMonitorFromWindow.Call(owner, MONITOR_DEFAULTTONEAREST)
	if monitor == 0 {
		return x, y
	}
	info := MONITORINFO{CbSize: uint32(unsafe.Sizeof(MONITORINFO{}))}
	if ok, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return x, y
	}
	work := info.RcWork
	if x+w > int(work.Right) {
		x = int(work.Right) - w
	}
	if y+h > int(work.Bottom) {
		y = int(work.Bottom) - h
	}
	if x < int(work.Left) {
		x = int(work.Left)
	}
	if y < int(work.Top) {
		y = int(work.Top)
	}
	return x, y
}

func (s *customPickerState) savePalette() {
	path, ok := chooseIndexedPaletteFile(s.hwnd, true)
	if !ok {
		return
	}
	if err := savePaletteToPath(path, s.palette, s.hwnd); err != nil {
		messageBox(s.hwnd, err.Error(), "팔레트 저장", MB_OK|MB_ICONERROR)
	}
}
func (s *customPickerState) loadPalette() {
	path, ok := chooseIndexedPaletteFile(s.hwnd, false)
	if !ok {
		return
	}
	p, err := loadPaletteFromPath(path)
	if err != nil {
		messageBox(s.hwnd, err.Error(), "팔레트 불러오기", MB_OK|MB_ICONERROR)
		return
	}
	if app.dejavu != nil && len(p) != 16 {
		messageBox(s.hwnd, "DejaVu IMG는 16색만 지원합니다.", "팔레트 불러오기", MB_OK|MB_ICONERROR)
		return
	}
	s.palette = append([]uint32(nil), p...)
	s.mode = len(p)
	s.colorCount = len(p)
	if s.mode == 16 {
		s.draft16Count = s.colorCount
	} else {
		s.draft256Count = s.colorCount
	}
	if s.countEdit != 0 {
		s.setCountText(strconv.Itoa(s.displayCountFor(s.mode)))
	}
	s.selected = -1
	s.editTarget = 0
	s.changed = true
	s.expanded = true
	s.updateLayout()
}
func (s *customPickerState) customPreset(i int) uint32 {
	if i >= 48 {
		return s.custom[i-48]
	}
	if i < 8 {
		v := byte(255 - i*32)
		return rgb(v, v, v)
	}
	h := float64((i-8)%8) * 45
	v := 1.0
	sat := 1.0
	if i >= 24 && i < 32 {
		v = 0.55
	}
	if i >= 16 && i < 24 {
		sat = 0.55
	}
	if i >= 32 && i < 40 {
		sat = 0.27
	}
	if i >= 40 && i < 48 {
		sat = 0.65
		v = 0.45
	}
	return customPickerHSVToRGB(h, sat, v)
}
func (s *customPickerState) clickXY(x, y int, drag bool) {
	if x >= 22 && x < 22+8*28 && y >= 37 && y < 37+6*20 && !drag {
		col := (x - 22) / 28
		row := (y - 37) / 20
		if col >= 0 && col < 8 && row >= 0 && row < 6 {
			s.chooseColor(s.customPreset(row*8+col), true, -1)
		}
		return
	}
	if !drag {
		if s.appliedMode == 0 {
			if idx := customPickerMainSlotAt(x, y); idx >= 0 && idx < s.displayCountFor(0) {
				s.chooseMainSlot(idx)
				return
			}
		} else if idx := customPickerCurrentIndexedSlotAt(s.appliedMode, x, y); idx >= 0 && idx < s.displayCountFor(s.appliedMode) {
			s.chooseCurrentIndexedSlot(idx)
			return
		}
	}
	if x >= pickerHueLeft && x < pickerHueLeft+pickerHueWidth && y >= pickerHueTop && y < pickerHueTop+pickerHueHeight {
		s.hue = 360 * float64(x-pickerHueLeft) / float64(pickerHueWidth)
		s.sat = 1 - float64(y-pickerHueTop)/float64(pickerHueHeight-1)
		s.chooseHSV(s.hue, s.sat, s.value, -1)
		return
	}
	if x >= pickerValueLeft && x < pickerValueLeft+20 && y >= pickerHueTop && y < pickerHueTop+pickerHueHeight {
		s.value = 1 - float64(y-pickerHueTop)/float64(pickerHueHeight-1)
		s.chooseHSV(s.hue, s.sat, s.value, -1)
		return
	}
	if drag || s.gridRows == 0 {
		return
	}
	columns, _ := customPickerIndexedGrid(s.mode)
	if x >= pickerGridLeft && x < pickerGridLeft+columns*pickerGridStep && y >= pickerGridTop && y < pickerGridTop+s.gridRows*pickerGridStep {
		col := (x - pickerGridLeft) / pickerGridStep
		row := (y - pickerGridTop) / pickerGridStep
		idx := customPickerGridIndex(col, row, columns, s.gridRows)
		if idx >= 0 && idx < len(s.palette) {
			s.selected = idx
			s.editTarget = 2
			s.edited = false
			s.chooseColor(s.palette[idx], false, -1)
		}
	}
}
func (s *customPickerState) paint(hdc uintptr) {
	dark := darkModeEnabled()
	bg := rgb(240, 240, 240)
	fg := rgb(28, 28, 28)
	edge := rgb(130, 130, 130)
	if dark {
		bg = rgb(30, 30, 30)
		fg = rgb(235, 235, 235)
		edge = rgb(100, 100, 100)
	}
	var client RECT
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	fillRect(hdc, client, bg)
	// The upper 48 are presets only. The actual 28+16 main palette lives
	// in its own panel below, matching the main window slot ordering.
	for i := 0; i < 48; i++ {
		x := 22 + (i%8)*28
		y := 37 + (i/8)*20
		rc := RECT{int32(x), int32(y), int32(x + 24), int32(y + 18)}
		fillRect(hdc, rc, s.customPreset(i))
		frameRect(hdc, rc, edge)
	}
	// CURRENT palette: RGB 28+16 while in normal mode, or exactly the
	// 16/256-index palette chosen with "색상표 적용" (or open in the document).
	frameRect(hdc, RECT{14, 222, 751, 337}, edge)
	procSelectObject.Call(hdc, s.font)
	name := fmt.Sprintf("현재 색상표 · RGB 기본 28색 / 사용자 16색 (표시 %d/44)", s.displayCountFor(0))
	if s.appliedMode != 0 {
		name = fmt.Sprintf("현재 색상표 · 인덱스 %d색 / 사용 %d색 / 표시 %d색", s.appliedMode, s.appliedCount, s.displayCountFor(s.appliedMode))
	}
	drawText(hdc, name, RECT{22, 227, 550, 249}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, fg)
	if s.appliedMode == 0 {
		for i := 0; i < s.displayCountFor(0); i++ {
			x, y := customPickerMainSwatchPoint(i)
			r := RECT{int32(x), int32(y), int32(x + pickerMainSize), int32(y + pickerMainSize)}
			fillRect(hdc, r, s.mainSwatch(i))
			frameRect(hdc, r, edge)
			if i == s.mainSelected && s.editTarget == 1 {
				frameRect(hdc, RECT{r.Left - 2, r.Top - 2, r.Right + 2, r.Bottom + 2}, rgb(255, 155, 20))
				frameRect(hdc, RECT{r.Left + 1, r.Top + 1, r.Right - 1, r.Bottom - 1}, rgb(255, 155, 20))
			}
		}
		fillRect(hdc, RECT{352, 253, 353, 308}, edge)
	} else {
		cols, _, dx, dy, cw, ch := customPickerCurrentIndexedGrid(s.appliedMode)
		oldFont, _, _ := procSelectObject.Call(hdc, s.smallFont)
		for i, c := range s.appliedPalette {
			if i >= s.displayCountFor(s.appliedMode) {
				break
			}
			x := 22 + (i%cols)*dx
			y := 251 + (i/cols)*dy
			r := RECT{int32(x), int32(y), int32(x + cw), int32(y + ch)}
			fillRect(hdc, r, c)
			frameRect(hdc, r, edge)
			if s.appliedMode == 16 {
				contrast := rgb(255, 255, 255)
				if (int(byte(c))*299+int(byte(c>>8))*587+int(byte(c>>16))*114)/1000 > 135 {
					contrast = rgb(0, 0, 0)
				}
				drawText(hdc, strconv.Itoa(i), r, DT_CENTER|DT_VCENTER|DT_SINGLELINE, contrast)
			}
			if s.editTarget == 2 && s.mode == s.appliedMode && i == s.selected {
				frameRect(hdc, RECT{r.Left - 1, r.Top - 1, r.Right + 1, r.Bottom + 1}, rgb(255, 158, 10))
			}
		}
		procSelectObject.Call(hdc, oldFont)
	}
	selectedLabel := "표시 칸 수만 변경 · 이미지 색상 유지"
	if s.appliedMode == 0 && s.editTarget == 1 && s.mainSelected >= 0 {
		if s.mainSelected < 28 {
			selectedLabel = fmt.Sprintf("기본 RGB %d번", s.mainSelected+1)
		} else {
			selectedLabel = fmt.Sprintf("사용자 %d번", s.mainSelected-27)
		}
	} else if s.appliedMode != 0 && s.editTarget == 2 && s.mode == s.appliedMode && s.selected >= 0 {
		selectedLabel = fmt.Sprintf("인덱스 %d번 편집", s.selected)
	}
	drawText(hdc, "표시 색상 수", RECT{552, 255, 641, 280}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, fg)
	drawText(hdc, selectedLabel, RECT{559, 285, 737, 330}, DT_LEFT|DT_VCENTER|DT_WORDBREAK, fg)
	// HSV hue x saturation rectangle; brightness handled by adjacent strip.
	pix := make([]byte, pickerHueWidth*pickerHueHeight*4)
	for y := 0; y < pickerHueHeight; y++ {
		for x := 0; x < pickerHueWidth; x++ {
			c := customPickerHSVToRGB(360*float64(x)/float64(pickerHueWidth), 1-float64(y)/float64(pickerHueHeight-1), s.value)
			off := (y*pickerHueWidth + x) * 4
			pix[off], pix[off+1], pix[off+2], pix[off+3] = byte(c>>16), byte(c>>8), byte(c), 255
		}
	}
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: pickerHueWidth, BiHeight: -pickerHueHeight, BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	procStretchDIBits.Call(hdc, pickerHueLeft, pickerHueTop, pickerHueWidth, pickerHueHeight, 0, 0, pickerHueWidth, pickerHueHeight, uintptr(unsafe.Pointer(&pix[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, SRCCOPY)
	frameRect(hdc, RECT{pickerHueLeft - 1, pickerHueTop - 1, pickerHueLeft + pickerHueWidth + 1, pickerHueTop + pickerHueHeight + 1}, edge)
	for y := 0; y < pickerHueHeight; y++ {
		v := 1 - float64(y)/float64(pickerHueHeight-1)
		rc := RECT{pickerValueLeft, int32(pickerHueTop + y), pickerValueLeft + 18, int32(pickerHueTop + y + 1)}
		fillRect(hdc, rc, customPickerHSVToRGB(s.hue, s.sat, v))
	}
	frameRect(hdc, RECT{pickerValueLeft - 1, pickerHueTop - 1, pickerValueLeft + 19, pickerHueTop + pickerHueHeight + 1}, edge)
	valueY := pickerHueTop + int((1-s.value)*float64(pickerHueHeight-1))
	frameRect(hdc, RECT{pickerValueLeft - 2, int32(valueY - 2), pickerValueLeft + 21, int32(valueY + 3)}, fg)
	// Distinguish the original document colour from the colour currently being
	// edited. This is deliberately independent of the indexed palette below.
	originalSwatch := RECT{542, 174, 580, 203}
	currentSwatch := RECT{586, 174, 625, 203}
	fillRect(hdc, originalSwatch, s.initial)
	frameRect(hdc, originalSwatch, edge)
	fillRect(hdc, currentSwatch, s.colour)
	frameRect(hdc, currentSwatch, edge)
	procSelectObject.Call(hdc, s.font)
	drawText(hdc, "기본색 48 · 색상 견본", RECT{22, 10, 230, 31}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, fg)
	drawText(hdc, "색상 · 채도 / 명도", RECT{284, 10, 486, 31}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, fg)
	drawText(hdc, "기존", RECT{542, 151, 580, 171}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, fg)
	drawText(hdc, "편집", RECT{586, 151, 625, 171}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, fg)
	drawText(hdc, fmt.Sprintf("#%02X%02X%02X", byte(s.colour), byte(s.colour>>8), byte(s.colour>>16)), RECT{635, 175, 750, 204}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, fg)
	if s.gridRows > 0 {
		modeName := "16색"
		if s.mode == 256 {
			modeName = "256색"
		}
		// Single clean panel, with apply/load/save buttons in the header row.
		panelBottom := pickerGridTop + s.gridRows*pickerGridStep + 8
		frameRect(hdc, RECT{14, 391, 751, int32(panelBottom)}, edge)
		drawText(hdc, fmt.Sprintf("인덱스 색상표 · %s (사용 %d개, 투명 별도)", modeName, s.colorCount), RECT{22, 397, 295, 425}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, fg)
		oldFont, _, _ := procSelectObject.Call(hdc, s.smallFont)
		columns, _ := customPickerIndexedGrid(s.mode)
		for i, c := range s.palette {
			col := i % columns
			row := i / columns
			if row >= s.gridRows {
				break
			}
			x := pickerGridLeft + col*pickerGridStep
			y := pickerGridTop + row*pickerGridStep
			r := RECT{int32(x), int32(y), int32(x + pickerGridSize), int32(y + pickerGridSize)}
			fillRect(hdc, r, c)
			stroke := edge
			if s.editTarget == 2 && i == s.selected {
				stroke = rgb(255, 158, 10)
			}
			frameRect(hdc, r, stroke)
			if s.editTarget == 2 && i == s.selected {
				frameRect(hdc, RECT{r.Left + 1, r.Top + 1, r.Right - 1, r.Bottom - 1}, stroke)
			}
			txt := fg
			if (int(byte(c))*299+int(byte(c>>8))*587+int(byte(c>>16))*114)/1000 > 135 {
				txt = rgb(0, 0, 0)
			} else {
				txt = rgb(255, 255, 255)
			}
			drawText(hdc, strconv.Itoa(i), r, DT_CENTER|DT_VCENTER|DT_SINGLELINE, txt)
		}
		procSelectObject.Call(hdc, oldFont)
	}

}
func customPickerProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	s := customPickerCurrent
	defer func() {
		if r := recover(); r != nil {
			logPanic("customPickerProc", r)
		}
	}()
	switch msg {
	case WM_CREATE:
		if s == nil {
			break
		}
		s.hwnd = hwnd
		applyDarkThemeToWindow(hwnd, darkModeEnabled())
		s.font = createCompactEditorUIFont()
		s.smallFont = createGDIFont("맑은 고딕", 10, false, false, false)
		for i, label := range []string{"빨강", "초록", "파랑", "색상", "채도", "명도"} {
			x := 548 + (i/3)*111
			y := 34 + (i%3)*37
			s.label(label, x, y+2, 43)
			val := 0
			switch i {
			case 0:
				val = int(byte(s.colour))
			case 1:
				val = int(byte(s.colour >> 8))
			case 2:
				val = int(byte(s.colour >> 16))
			case 3:
				val = int(math.Round(s.hue))
			case 4:
				val = int(math.Round(s.sat * 100))
			case 5:
				val = int(math.Round(s.value * 100))
			}
			s.edits[i] = s.createChild("EDIT", strconv.Itoa(val), WS_CHILD|WS_VISIBLE|WS_BORDER|WS_TABSTOP|ES_NUMBER|ES_AUTOHSCROLL, x+43, y, 48, 23, pickerNumberBase+i)
		}
		s.countEdit = s.createChild("EDIT", "44", WS_CHILD|WS_VISIBLE|WS_BORDER|WS_TABSTOP|ES_NUMBER|ES_AUTOHSCROLL, 648, 257, 70, 22, pickerColorCount)
		s.buttons[0] = s.button("일반 RGB", 22, pickerModeTop, 106, 26, pickerModeRGB)
		s.buttons[1] = s.button("16색", 134, pickerModeTop, 77, 26, pickerMode16)
		s.buttons[2] = s.button("256색", 218, pickerModeTop, 77, 26, pickerMode256)
		s.buttons[3] = s.button("사용자 색 등록", 542, pickerModeTop, 145, 26, pickerAddCustom)
		s.toggle = s.button("▸ 인덱스 팔레트", 303, pickerModeTop, 170, 26, pickerToggle)
		s.applyButton = s.button("색상표 적용", 302, 397, 125, 27, pickerApplyIndexed)
		s.buttons[4] = s.button("팔레트 불러오기", 436, 397, 137, 27, pickerLoad)
		s.buttons[5] = s.button("팔레트 저장", 583, 397, 143, 27, pickerSave)
		s.buttons[6] = s.button("확인", 592, pickerBaseHeight-44, 72, 28, IDOK)
		s.buttons[7] = s.button("취소", 670, pickerBaseHeight-44, 72, 28, IDCANCEL)
		// Buttons are located at the bottom of the enlarged panel in indexed modes.
		s.updateLayout()
		procSetFocus.Call(s.edits[0])
		return 0
	case WM_COMMAND:
		if s == nil {
			break
		}
		id := int(loWord(wparam))
		code := int(hiWord(wparam))
		if id == pickerColorCount && code == pickerENCHANGE && s.countEdit != 0 {
			if !s.updating {
				s.countEditedByUser = true
				s.captureDisplayCount()
				procInvalidateRect.Call(hwnd, 0, 0)
			}
			return 0
		}
		if id == pickerColorCount && code == pickerENKILLFOCUS {
			// Invalid intermediate input (e.g. while deleting) is only checked
			// when focus leaves the field; OK/Apply also validate it.
			return 0
		}
		if id >= pickerNumberBase && id < pickerNumberBase+6 && code == pickerENCHANGE {
			s.fromNumbers(id - pickerNumberBase)
			return 0
		}
		switch id {
		case pickerModeRGB:
			s.modeChange(0)
		case pickerMode16:
			s.modeChange(16)
		case pickerMode256:
			s.modeChange(256)
		case pickerToggle:
			if s.mode != 0 {
				s.expanded = !s.expanded
				s.updateLayout()
			}
		case pickerLoad:
			s.loadPalette()
		case pickerSave:
			s.savePalette()
		case pickerApplyIndexed:
			if !s.readColorCount() {
				return 0
			}
			s.applyIndexedToCurrent()
		case pickerAddCustom:
			slot := s.customNext % 16
			s.custom[slot] = s.colour
			s.mainSelected = 28 + slot
			s.editTarget = 1
			s.mainChanged = true
			s.customNext++
			procInvalidateRect.Call(hwnd, 0, 0)
		case IDOK:
			if !s.readColorCount() {
				return 0
			}
			s.accepted = true
			s.done = true
			procDestroyWindow.Call(hwnd)
		case IDCANCEL:
			s.done = true
			procDestroyWindow.Call(hwnd)
		default:
			return 0
		}
		if id == pickerModeRGB || id == pickerMode16 || id == pickerMode256 || id == pickerToggle || id == pickerLoad {
			s.placeBottomButtons()
		}
		procInvalidateRect.Call(hwnd, 0, 1)
		return 0
	case WM_LBUTTONDOWN:
		if s != nil {
			x := int(int16(loWord(lparam)))
			y := int(int16(hiWord(lparam)))
			s.clickXY(x, y, false)
			if x >= pickerHueLeft && x < pickerHueLeft+pickerHueWidth && y >= pickerHueTop && y < pickerHueTop+pickerHueHeight {
				s.dragging = 1
				procSetCapture.Call(hwnd)
			}
			if x >= pickerValueLeft && x < pickerValueLeft+20 && y >= pickerHueTop && y < pickerHueTop+pickerHueHeight {
				s.dragging = 2
				procSetCapture.Call(hwnd)
			}
			return 0
		}
	case WM_MOUSEMOVE:
		if s != nil && s.dragging != 0 {
			x := int(int16(loWord(lparam)))
			y := int(int16(hiWord(lparam)))
			if s.dragging == 1 {
				x = max(pickerHueLeft, min(pickerHueLeft+pickerHueWidth-1, x))
				y = max(pickerHueTop, min(pickerHueTop+pickerHueHeight-1, y))
			} else {
				x = pickerValueLeft
				y = max(pickerHueTop, min(pickerHueTop+pickerHueHeight-1, y))
			}
			s.clickXY(x, y, true)
			return 0
		}
	case WM_LBUTTONUP:
		if s != nil && s.dragging != 0 {
			s.dragging = 0
			procReleaseCapture.Call()
			return 0
		}
	case WM_CAPTURECHANGED:
		if s != nil {
			s.dragging = 0
		}
	case WM_ERASEBKGND:
		return 1
	case WM_PAINT:
		if s != nil {
			var ps PAINTSTRUCT
			hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			if hdc != 0 {
				s.paint(hdc)
				procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			}
			return 0
		}
	case WM_DRAWITEM:
		if lparam != 0 && drawThemedOwnerDrawPushButton((*DRAWITEMSTRUCT)(unsafe.Pointer(lparam))) {
			return 1
		}
	case WM_CTLCOLORSTATIC, WM_CTLCOLORDLG, WM_CTLCOLORBTN, WM_CTLCOLOREDIT:
		if darkModeEnabled() && wparam != 0 {
			procSetTextColor.Call(wparam, uintptr(rgb(235, 235, 235)))
			procSetBkColor.Call(wparam, uintptr(rgb(30, 30, 30)))
			return colorDialogDarkBrush()
		}
	case WM_CLOSE:
		if s != nil {
			s.done = true
		}
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		if s != nil {
			s.hwnd = 0
			if s.font != 0 {
				procDeleteObject.Call(s.font)
				s.font = 0
			}
			if s.smallFont != 0 {
				procDeleteObject.Call(s.smallFont)
				s.smallFont = 0
			}
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}
func (s *customPickerState) placeBottomButtons() {
	if s.hwnd == 0 {
		return
	}
	footerY := customPickerFooterTop(s.gridRows)
	// Palette I/O always belongs to the indexed section. OK and Cancel
	// are on a separate footer row, even with a single 16-colour row.
	// Indexed buttons stay fixed above the swatches; there is no second
	// blank rectangle or extra file-control row beneath the palette.
	if s.applyButton != 0 {
		procSetWindowPos.Call(s.applyButton, 0, 302, 397, 125, 27, SWP_NOZORDER)
	}
	if s.buttons[4] != 0 {
		procSetWindowPos.Call(s.buttons[4], 0, 436, 397, 137, 27, SWP_NOZORDER)
	}
	if s.buttons[5] != 0 {
		procSetWindowPos.Call(s.buttons[5], 0, 583, 397, 143, 27, SWP_NOZORDER)
	}
	if s.buttons[6] != 0 {
		procSetWindowPos.Call(s.buttons[6], 0, 592, uintptr(footerY), 72, 28, SWP_NOZORDER)
	}
	if s.buttons[7] != 0 {
		procSetWindowPos.Call(s.buttons[7], 0, 670, uintptr(footerY), 72, 28, SWP_NOZORDER)
	}
}
func ensureCustomPickerClass(hinst uintptr) bool {
	if customPickerClassReady {
		return true
	}
	if customPickerWndCallback == 0 {
		customPickerWndCallback = syscall.NewCallback(customPickerProc)
	}
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{CbSize: uint32(unsafe.Sizeof(WNDCLASSEXW{})), LpfnWndProc: customPickerWndCallback, HInstance: hinst, HCursor: cursor, HbrBackground: uintptr(COLOR_BTNFACE + 1), LpszClassName: utf16Ptr(pickerWindowClass)}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		logf("COLOR_PICKER RegisterClass failed: %v", err)
		return false
	}
	customPickerClassReady = true
	return true
}
func (a *App) editColorTarget(background bool) {
	if !ensureCustomPickerClass(a.hinst) {
		messageBox(a.hwnd, "색 편집창을 초기화하지 못했습니다.", "dotPaint", MB_OK|MB_ICONERROR)
		return
	}
	initial := a.fg
	if background {
		initial = a.bg
	}
	if isTransparentColor(initial) {
		initial = rgb(0, 0, 0)
	}
	s := &customPickerState{parent: a.hwnd, initial: initial, colour: initial, background: background, mode: 0, selected: -1, mainSelected: -1, width: pickerClientWidth, custom: a.customColors, displayRGB: a.paletteDisplayRGB, display16: a.paletteDisplay16, display256: a.paletteDisplay256}
	// Keep a separate draft of the RGB window palette even if an IMG or a
	// 256-colour indexed document has replaced app.palette temporarily.
	for _, src := range [][]uint32{a.standardPalette, a.dejavuPrevPalette, a.palette} {
		if len(src) == 28 {
			s.baseColors = append([]uint32(nil), src...)
			break
		}
	}
	if len(s.baseColors) != 28 {
		s.baseColors = defaultMainRGBPalette()
	}
	preferred := a.fgPaletteSlot
	if background {
		preferred = a.bgPaletteSlot
	}
	s.mainSelected = customPickerInitialMainSlot(s.baseColors, s.custom, initial, preferred)
	s.hue, s.sat, s.value = customPickerRGBToHSV(initial)
	s.palette = a.currentIndexedColorTable()
	if a.dejavu != nil {
		s.mode = 16
		s.expanded = false
	} else if a.indexedMode {
		s.mode = a.indexedSize
		s.colorCount = indexedUsableCount(s.mode, a.indexedActiveCount)
		s.expanded = a.indexedSize == 256
	}
	if s.mode != 0 {
		if a.dejavu != nil {
			s.colorCount = 16
		}
		if s.colorCount == 0 {
			s.colorCount = s.mode
		}
		if s.mode == 16 {
			s.draft16Count = s.colorCount
		} else {
			s.draft256Count = s.colorCount
		}
		s.appliedMode = s.mode
		s.appliedPalette = append([]uint32(nil), s.palette...)
		s.appliedCount = s.colorCount
	}
	// At first open, show and bind the exact remembered RGB slot, not the
	// first equal RGB value. Indexed documents use their own palette family.
	if s.mode == 0 && s.mainSelected >= 0 {
		s.editTarget = 1
	}
	if s.mode != 0 {
		for i, colour := range s.palette {
			if colour == initial {
				s.selected = i
				s.editTarget = 2
				break
			}
		}
	}
	s.height = customPickerHeight(s.rows())
	s.gridRows = s.rows()
	customPickerCurrent = s
	defer func() { customPickerCurrent = nil }()
	style := uintptr(WS_CAPTION | WS_SYSMENU)
	exStyle := uintptr(WS_EX_DLGMODALFRAME)
	bounds := RECT{0, 0, pickerClientWidth, int32(s.height)}
	procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&bounds)), style, 0, exStyle)
	w := int(bounds.Right - bounds.Left)
	h := int(bounds.Bottom - bounds.Top)
	hwnd, _, err := procCreateWindowExW.Call(exStyle, uintptr(unsafe.Pointer(utf16Ptr(pickerWindowClass))), uintptr(unsafe.Pointer(utf16Ptr("색"))), style, uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), uintptr(w), uintptr(h), a.hwnd, 0, a.hinst, 0)
	if hwnd == 0 {
		logf("COLOR_PICKER CreateWindow failed: %v", err)
		messageBox(a.hwnd, "색 편집창을 열지 못했습니다.", "dotPaint", MB_OK|MB_ICONERROR)
		return
	}
	s.hwnd = hwnd
	if s.countEdit != 0 {
		s.setCountText(strconv.Itoa(s.displayCountFor(s.mode)))
	}
	s.updateLayout()
	s.placeBottomButtons()
	var rc RECT
	procGetWindowRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	x := int(rc.Left) + (int(rc.Right-rc.Left)-w)/2
	y := int(rc.Top) + (int(rc.Bottom-rc.Top)-h)/2
	x, y = pickerVisiblePosition(a.hwnd, x, y, w, h)
	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), SWP_NOZORDER|SWP_NOACTIVATE)
	procEnableWindow.Call(a.hwnd, 0)
	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)
	logf("COLOR_PICKER open indexed=%d expanded=%v selected=%d bounds=%dx%d", s.mode, s.expanded, s.selected, w, h)
	var msg MSG
	for !s.done {
		r, _, getErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			if int32(r) < 0 {
				logf("COLOR_PICKER GetMessage failed: %v", getErr)
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
			case 0x0D:
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
	if s.hwnd != 0 {
		procDestroyWindow.Call(s.hwnd)
	}
	if !s.accepted {
		logf("COLOR_PICKER cancelled")
		return
	}
	// UI-only preference: no document pixels, palette indices or history change.
	a.paletteDisplayRGB, a.paletteDisplay16, a.paletteDisplay256 = s.displayRGB, s.display16, s.display256
	a.customColors = s.custom
	// The main palette must be updated only on OK; while indexed/IMG
	// editing, preserve these RGB colours for the next normal document.
	if s.mainChanged {
		switch {
		case a.dejavu != nil:
			if len(a.dejavuPrevPalette) == 28 {
				a.dejavuPrevPalette = append([]uint32(nil), s.baseColors...)
			}
		case a.indexedMode:
			if len(a.standardPalette) == 28 {
				a.standardPalette = append([]uint32(nil), s.baseColors...)
			}
		default:
			a.palette = append([]uint32(nil), s.baseColors...)
		}
	}
	if a.dejavu == nil && s.mode == 0 && a.indexedMode {
		a.setIndexedPaletteMode(0)
	}
	// One atomic transition: quantize against the FINAL chosen palette/count,
	// never quantize first to default 16 then again to edited 256 colours.
	if s.mode != 0 && (s.changed || (!a.indexedMode && a.dejavu == nil) || (a.dejavu == nil && (a.indexedSize != s.mode || a.indexedActiveCount != s.colorCount))) {
		if err := a.applyIndexedColorTableWithCount(s.palette, "", s.colorCount); err != nil {
			messageBox(a.hwnd, err.Error(), "팔레트", MB_OK|MB_ICONERROR)
			return
		}
	}
	chosen := s.colour
	if a.indexedPaletteActive() {
		if s.editTarget == 2 && s.selected >= 0 && s.selected < len(a.activeIndexedPaletteColors()) {
			chosen = a.palette[s.selected]
		} else {
			chosen = a.palette[nearestIndexedColor(byte(s.colour), byte(s.colour>>8), byte(s.colour>>16), a.activeIndexedPaletteColors())]
		}
	}
	slot := -1
	if s.mode == 0 && s.editTarget == 1 && s.mainSelected >= 0 && s.mainSelected < 44 {
		slot = s.mainSelected
	}
	if background {
		a.bg = chosen
		a.bgPaletteSlot = slot
	} else {
		a.fg = chosen
		a.fgPaletteSlot = slot
	}
	a.statusText = fmt.Sprintf("색 편집 완료: #%02X%02X%02X · 인덱스 %d색", byte(chosen), byte(chosen>>8), byte(chosen>>16), s.mode)
	logf("COLOR_PICKER accepted color=%#06x indexed=%d changed=%v selected=%d main-slot=%d main-changed=%v", chosen, s.mode, s.changed, s.selected, s.mainSelected, s.mainChanged)
	invalidate(a.hwnd)
}
