//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	paletteFooterExpandID     = 0xA440
	paletteFooterLoadID       = 0xA441
	paletteFooterSaveID       = 0xA442
	paletteFooterSwatchBaseID = 0xA450
	paletteFooterPrevID       = 0xA470
	paletteFooterNextID       = 0xA471
	paletteFooterRGBID        = 0xA472
	paletteFooter16ID         = 0xA473
	paletteFooter256ID        = 0xA474
)

type colorPaletteFooter struct {
	hwnd                                                          uintptr
	originalClientHeight                                          int
	originalWindowHeight                                          int
	expanded                                                      bool
	palette                                                       []uint32
	changed                                                       bool
	selectedIndex                                                 int
	editIndex                                                     bool
	mode                                                          bool
	page                                                          int
	font                                                          uintptr
	info, expand, prev, next, pageLabel, modeRGB, mode16, mode256 uintptr
	load, save                                                    uintptr
	swatches                                                      [16]uintptr
}

var activeColorPaletteFooter *colorPaletteFooter

func (a *App) currentIndexedColors() indexed16Palette {
	if a.dejavu != nil {
		return indexedPaletteFromRefs(dejavuPaletteColorRefs(a.dejavu.Palette))
	}
	if a.indexedMode {
		return indexedPaletteFromRefs(a.palette)
	}
	return a.indexedColors
}
func (a *App) currentIndexedColorTable() []uint32 {
	if a.dejavu != nil {
		return dejavuPaletteColorRefs(a.dejavu.Palette)
	}
	if a.indexedMode {
		return append([]uint32(nil), a.palette...)
	}
	return append([]uint32(nil), a.indexedColors[:]...)
}
func (a *App) effectiveIndexedPalette() dejavuPalette {
	if a.dejavu != nil {
		return a.dejavu.Palette
	}
	return indexedPaletteAsDejavu(a.currentIndexedColors())
}
func indexedPaletteAsDejavu(p indexed16Palette) dejavuPalette {
	var d dejavuPalette
	for i, c := range p {
		d.RGB[i] = [3]byte{byte(c), byte(c >> 8), byte(c >> 16)}
	}
	return d
}
func (a *App) activeIndexedPaletteColors() []uint32 {
	if a.dejavu != nil {
		return a.palette[:min(16, len(a.palette))]
	}
	n := indexedUsableCount(len(a.palette), a.indexedActiveCount)
	if n == 0 {
		return a.palette
	}
	return a.palette[:n]
}

func (a *App) setIndexedPaletteMode(size int) { a.switchIndexedPaletteMode(size, true) }
func (a *App) switchIndexedPaletteMode(size int, convert bool) {
	if a.dejavu != nil {
		a.statusText = "DejaVu IMG는 항상 16색 인덱스 모드입니다."
		invalidate(a.hwnd)
		return
	}
	if size != 0 && !validIndexedSize(size) {
		return
	}
	if convert && a.indexedMode && a.indexedSize == size {
		return
	}
	if convert {
		if a.selection.Active && a.selection.Floating {
			a.applyFloatingSelection(a.selection.X, a.selection.Y)
		}
		a.clearSelectionState()
		a.undo.Push(a.canvas) // pre-conversion pixels and dimensions
	}
	if a.indexedMode {
		if len(a.palette) == 256 {
			a.indexedColors256 = append([]uint32(nil), a.palette...)
		}
		if len(a.palette) == 16 {
			a.indexedColors = indexedPaletteFromRefs(a.palette)
		}
	} else if size != 0 {
		a.standardPalette = append([]uint32(nil), a.palette...)
	}
	a.indexedMode = size != 0
	a.indexedSize = size
	a.indexedActiveCount = indexedUsableCount(size, size)
	a.indexedPalettePage = 0
	if size == 0 {
		if len(a.standardPalette) > 0 {
			a.palette = append([]uint32(nil), a.standardPalette...)
		}
		a.standardPalette = nil
		a.indexedPixelIndices = nil
	} else if size == 16 {
		a.palette = append([]uint32(nil), a.indexedColors[:]...)
	} else {
		if len(a.indexedColors256) != 256 {
			a.indexedColors256 = defaultIndexed256Palette(a.indexedColors[:])
		}
		a.palette = append([]uint32(nil), a.indexedColors256...)
	}
	if size != 0 {
		a.fg, a.bg = a.palette[0], a.palette[len(a.palette)-1]
		if convert {
			a.indexedPixelIndices = quantizeIndexedBGRA(a.canvas.Pix, a.palette, a.indexedActiveCount)
		}
	}
	if convert {
		a.markDirty()
		a.recordHistory("인덱스 색상 모드")
	}
	a.statusText = fmt.Sprintf("%d색 인덱스 팔레트 모드", size)
	if size == 0 {
		a.statusText = "인덱스 모드 해제: 일반 RGB"
	}
	a.updateIndexedPaletteUI()
}
func (a *App) toggleIndexedPaletteMode() {
	if a.indexedMode && a.indexedSize == 16 {
		a.setIndexedPaletteMode(0)
	} else {
		a.setIndexedPaletteMode(16)
	}
}
func (a *App) updateIndexedPaletteUI() {
	if a.hwnd == 0 {
		return
	}
	a.rebuildMenuForTheme()
	a.updateScrollbars()
	invalidate(a.hwnd)
}

func remapIndexedBGRA(pix []byte, old, new dejavuPalette) {
	for off := 0; off+3 < len(pix); off += 4 {
		if pix[off+3] == 0 {
			continue
		}
		i := dejavuNearestPaletteIndex(pix[off+2], pix[off+1], pix[off], old)
		c := new.Color(int(i))
		pix[off], pix[off+1], pix[off+2] = c.B, c.G, c.R
	}
}
func (a *App) applyIndexedColors(p indexed16Palette, path string) error {
	return a.applyIndexedColorTable(p[:], path)
}
func (a *App) applyIndexedColorTable(p []uint32, path string) error {
	return a.applyIndexedColorTableWithCount(p, path, len(p))
}
func (a *App) applyIndexedColorTableWithCount(p []uint32, path string, count int) error {
	if !validIndexedSize(len(p)) {
		return fmt.Errorf("팔레트 용량은 16색 또는 256색이어야 합니다")
	}
	if count < 1 || count > len(p) {
		return fmt.Errorf("사용 색상 수는 1~%d 사이여야 합니다", len(p))
	}
	if a.dejavu != nil && count != 16 {
		return fmt.Errorf("DejaVu IMG는 16개 색상 인덱스를 모두 사용합니다")
	}
	if a.dejavu != nil {
		if len(p) != 16 {
			return fmt.Errorf("DejaVu IMG는 4비트플레인 16색 형식입니다. 256색 팔레트를 적용할 수 없습니다")
		}
		if a.selection.Active && a.selection.Floating {
			a.applyFloatingSelection(a.selection.X, a.selection.Y)
		}
		a.clearSelectionState()
		idx, err := a.currentDejavuAllIndices()
		if err != nil {
			return err
		}
		old := a.dejavu.Palette
		var fixed indexed16Palette
		copy(fixed[:], p)
		next := indexedPaletteAsDejavu(fixed)
		cv, err := dejavuCanvasFromIndices(idx, next)
		if err != nil {
			return err
		}
		original, err := dejavuCanvasFromIndices(a.dejavu.OriginalIndices, next)
		if err != nil {
			return err
		}
		for i := range a.historyEntries {
			remapIndexedBGRA(a.historyEntries[i].Pix, old, next)
		}
		for i := range a.undo.undo {
			remapIndexedBGRA(a.undo.undo[i].Pix, old, next)
		}
		for i := range a.undo.redo {
			remapIndexedBGRA(a.undo.redo[i].Pix, old, next)
		}
		a.canvas.Pix = cv.Pix
		a.dejavu.OriginalCanvas = original.Pix
		a.dejavu.Palette = next
		if path != "" {
			a.dejavu.PalettePath = path
		}
		a.palette = append([]uint32(nil), p...)
		a.fg, a.bg = a.palette[0], a.palette[15]
		invalidateDejavuTileBrowser(a)
	} else {
		if a.selection.Active && a.selection.Floating {
			a.applyFloatingSelection(a.selection.X, a.selection.Y)
		}
		a.clearSelectionState()
		a.undo.Push(a.canvas)
		oldMode := a.indexedMode
		oldPalette := append([]uint32(nil), a.palette...)
		oldCount := indexedUsableCount(len(oldPalette), a.indexedActiveCount)
		var indices []byte
		if oldMode && validIndexedSize(len(oldPalette)) {
			indices = indexedIndicesForSnapshot(a.canvas.Pix, oldPalette, oldCount, a.indexedPixelIndices)
		}
		if !a.indexedMode || a.indexedSize != len(p) {
			a.switchIndexedPaletteMode(len(p), false)
		}
		a.indexedActiveCount = count
		a.palette = append([]uint32(nil), p...)
		if oldMode && path == "" && len(oldPalette) == len(p) && len(indices)*4 == len(a.canvas.Pix) {
			recolorIndexedBGRA(a.canvas.Pix, indices, a.palette, count)
			a.indexedPixelIndices = indices
		} else {
			a.indexedPixelIndices = quantizeIndexedBGRA(a.canvas.Pix, a.palette, count)
		}
		a.markDirty()
		a.fg, a.bg = a.palette[0], a.palette[count-1]
		a.recordHistory("인덱스 색상표 적용")
		if len(p) == 256 {
			a.indexedColors256 = append([]uint32(nil), p...)
		}
	}
	if len(p) == 16 {
		a.indexedColors = indexedPaletteFromRefs(p)
	}
	a.indexedPalettePage = 0
	a.statusText = fmt.Sprintf("%d색 팔레트(사용 %d색)를 적용했다냥.", len(p), count)
	logf("INDEXED_PALETTE applied path=%q img=%v size=%d", path, a.dejavu != nil, len(p))
	a.updateIndexedPaletteUI()
	return nil
}

func chooseIndexedPaletteFile(hwnd uintptr, save bool) (string, bool) {
	buf := make([]uint16, 32768)
	filters := "인덱스 팔레트 (*.pal;*.dpal;*.dat;*.gpl;*.act;*.rgb)\x00*.pal;*.dpal;*.dat;*.gpl;*.act;*.rgb\x00JASC PAL (*.pal)\x00*.pal\x00dotPaint (*.dpal)\x00*.dpal\x00DejaVu DAT (*.dat)\x00*.dat\x00모든 파일 (*.*)\x00*.*\x00\x00"
	if save {
		filters = "JASC PAL (*.pal)\x00*.pal\x00dotPaint (*.dpal)\x00*.dpal\x00DejaVu DAT 16색 (*.dat)\x00*.dat\x00\x00"
	}
	filter := utf16.Encode([]rune(filters))
	ofn := OPENFILENAMEW{LStructSize: uint32(unsafe.Sizeof(OPENFILENAMEW{})), HwndOwner: hwnd, LpstrFilter: &filter[0], LpstrFile: &buf[0], NMaxFile: uint32(len(buf)), Flags: OFN_PATHMUSTEXIST | OFN_HIDEREADONLY}
	var r uintptr
	if save {
		ofn.Flags |= OFN_OVERWRITEPROMPT
		ofn.LpstrDefExt = utf16Ptr("pal")
		r, _, _ = procGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	} else {
		ofn.Flags |= OFN_FILEMUSTEXIST
		r, _, _ = procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	}
	if r == 0 {
		return "", false
	}
	path := syscall.UTF16ToString(buf)
	if save && filepath.Ext(path) == "" {
		path += ".pal"
	}
	return path, true
}
func loadPaletteFromPath(path string) ([]uint32, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeIndexedPaletteFile(filepath.Base(path), raw)
}
func savePaletteToPath(path string, p []uint32, owner uintptr) error {
	if !validIndexedSize(len(p)) {
		return fmt.Errorf("팔레트는 16색 또는 256색이어야 합니다")
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pal":
		data, err := encodeIndexedPaletteJASCAny(p)
		if err != nil {
			return err
		}
		return os.WriteFile(path, data, 0644)
	case ".dpal":
		data, err := encodeIndexedPaletteDPALAny(p)
		if err != nil {
			return err
		}
		return os.WriteFile(path, data, 0644)
	case ".dat":
		if len(p) != 16 {
			return fmt.Errorf("DEJAVU.DAT는 16색 전용입니다. 256색 팔레트는 .pal 또는 .dpal로 저장하십시오")
		}
		raw, err := os.ReadFile(path)
		existing := err == nil
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			if messageBox(owner, "새 DAT는 48바이트 팔레트만 저장합니다. 게임의 전체 DEJAVU.DAT를 대체하면 안 됩니다. 계속할까요?", "DAT 팔레트", MB_YESNO|MB_ICONQUESTION) != 6 {
				return fmt.Errorf("저장 취소")
			}
			raw = make([]byte, 48)
		}
		if len(raw) < 48 {
			return fmt.Errorf("기존 DAT가 48바이트보다 짧습니다")
		}
		if existing && messageBox(owner, "기존 DAT의 마지막 48바이트를 수정합니다. 원본 백업을 권장합니다. 계속할까요?", "DAT 팔레트", MB_YESNO|MB_ICONQUESTION) != 6 {
			return fmt.Errorf("저장 취소")
		}
		for i, c := range p {
			raw[len(raw)-48+i*3] = byte((int(byte(c))*63 + 127) / 255)
			raw[len(raw)-48+i*3+1] = byte((int(byte(c>>8))*63 + 127) / 255)
			raw[len(raw)-48+i*3+2] = byte((int(byte(c>>16))*63 + 127) / 255)
		}
		return os.WriteFile(path, raw, 0644)
	default:
		return fmt.Errorf("저장은 .pal, .dpal, 16색 .dat만 지원합니다")
	}
}
func (a *App) loadIndexedPaletteInteractive() {
	path, ok := chooseIndexedPaletteFile(a.hwnd, false)
	if !ok {
		return
	}
	p, err := loadPaletteFromPath(path)
	if err == nil {
		err = a.applyIndexedColorTable(p, path)
	}
	if err != nil {
		messageBox(a.hwnd, "팔레트를 불러오지 못했습니다:\n"+err.Error(), "인덱스 팔레트", MB_OK|MB_ICONERROR)
	}
}
func (a *App) saveIndexedPaletteInteractive() {
	path, ok := chooseIndexedPaletteFile(a.hwnd, true)
	if !ok {
		return
	}
	p := a.currentIndexedColorTable()
	err := savePaletteToPath(path, p, a.hwnd)
	if err != nil {
		messageBox(a.hwnd, "팔레트를 저장하지 못했습니다:\n"+err.Error(), "인덱스 팔레트", MB_OK|MB_ICONERROR)
		return
	}
	a.statusText = fmt.Sprintf("%d색 팔레트 저장: %s", len(p), path)
	logf("INDEXED_PALETTE saved path=%q size=%d", path, len(p))
	invalidate(a.hwnd)
}

func dialogPaletteLabel(hwnd uintptr, text string, x, y, w, h int, id uintptr) uintptr {
	ptr := utf16Ptr(text)
	cls := utf16Ptr("STATIC")
	r, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(ptr)), WS_CHILD|WS_VISIBLE, uintptr(x), uintptr(y), uintptr(w), uintptr(h), hwnd, id, 0, 0)
	return r
}
func dialogPaletteButton(hwnd uintptr, text string, x, y, w, h int, id uintptr) uintptr {
	ptr := utf16Ptr(text)
	cls := utf16Ptr("BUTTON")
	r, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(ptr)), WS_CHILD|WS_VISIBLE|BS_OWNERDRAW, uintptr(x), uintptr(y), uintptr(w), uintptr(h), hwnd, id, 0, 0)
	return r
}
func paletteFooterSetText(hwnd uintptr, text string) {
	if hwnd != 0 {
		procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(text))))
	}
}
func colorDialogFooterControls(f *colorPaletteFooter) []uintptr {
	c := []uintptr{f.prev, f.next, f.pageLabel, f.modeRGB, f.mode16, f.mode256, f.load, f.save, f.info}
	c = append(c, f.swatches[:]...)
	return c
}
func colorDialogToggleFooter(hwnd uintptr, expanded bool) {
	f := activeColorPaletteFooter
	if f == nil || f.hwnd != hwnd {
		return
	}
	f.expanded = expanded
	extra := 37
	if expanded {
		extra = 170
	}
	var r RECT
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	procSetWindowPos.Call(hwnd, 0, 0, 0, uintptr(int(r.Right-r.Left)), uintptr(f.originalWindowHeight+extra), SWP_NOMOVE|0x0004|0x0010)
	for _, h := range colorDialogFooterControls(f) {
		if h != 0 {
			v := uintptr(0)
			if expanded {
				v = 5
			}
			procShowWindow.Call(h, v)
		}
	}
	colorDialogUpdateFooter(hwnd)
	procInvalidateRect.Call(hwnd, 0, 1)
}
func colorDialogInitPaletteFooter(hwnd uintptr) {
	f := activeColorPaletteFooter
	if f == nil || hwnd == 0 {
		return
	}
	f.hwnd = hwnd
	var client, win RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&win)))
	f.originalClientHeight = int(client.Bottom - client.Top)
	f.originalWindowHeight = int(win.Bottom - win.Top)
	w := int(win.Right - win.Left)
	if w < 555 {
		w = 555
		f.originalWindowHeight = int(win.Bottom - win.Top)
	}
	top := f.originalClientHeight + 5
	f.font = createGDIFont("맑은 고딕", 12, false, false, false)
	f.expand = dialogPaletteButton(hwnd, "▸ 인덱스 팔레트 설정", 18, top, 226, 25, paletteFooterExpandID)
	f.modeRGB = dialogPaletteButton(hwnd, "일반 RGB", 18, top+34, 90, 24, paletteFooterRGBID)
	f.mode16 = dialogPaletteButton(hwnd, "16색", 113, top+34, 62, 24, paletteFooter16ID)
	f.mode256 = dialogPaletteButton(hwnd, "256색", 180, top+34, 65, 24, paletteFooter256ID)
	f.prev = dialogPaletteButton(hwnd, "◀", 273, top+34, 32, 24, paletteFooterPrevID)
	f.pageLabel = dialogPaletteLabel(hwnd, "", 310, top+37, 86, 18, 0)
	f.next = dialogPaletteButton(hwnd, "▶", 395, top+34, 32, 24, paletteFooterNextID)
	for i := range f.swatches {
		f.swatches[i] = dialogPaletteButton(hwnd, "", 18+i*26, top+65, 24, 24, paletteFooterSwatchBaseID+uintptr(i))
	}
	f.load = dialogPaletteButton(hwnd, "팔레트 불러오기...", 18, top+99, 151, 24, paletteFooterLoadID)
	f.save = dialogPaletteButton(hwnd, "팔레트 저장...", 174, top+99, 134, 24, paletteFooterSaveID)
	f.info = dialogPaletteLabel(hwnd, "RGB 색을 선택하고 확인하면 선택한 인덱스가 변경됩니다.", 18, top+130, 507, 21, 0)
	for _, h := range append([]uintptr{f.expand}, colorDialogFooterControls(f)...) {
		setCompactEditorControlFont(h, f.font)
	}
	themeWindowHandles(darkModeEnabled(), append([]uintptr{f.expand}, colorDialogFooterControls(f)...)...)
	colorDialogToggleFooter(hwnd, false)
	logf("COLOR_DIALOG indexed-palette collapse-default size=%d", len(f.palette))
}
func colorDialogUpdateFooter(hwnd uintptr) {
	f := activeColorPaletteFooter
	if f == nil || f.hwnd != hwnd {
		return
	}
	name := "▸ 인덱스 팔레트 설정"
	if f.expanded {
		name = "▾ 인덱스 팔레트 설정"
	}
	paletteFooterSetText(f.expand, name)
	if !f.mode {
		paletteFooterSetText(f.modeRGB, "[일반 RGB]")
		paletteFooterSetText(f.mode16, "16색")
		paletteFooterSetText(f.mode256, "256색")
	}
	if f.mode && len(f.palette) == 16 {
		paletteFooterSetText(f.modeRGB, "일반 RGB")
		paletteFooterSetText(f.mode16, "[16색]")
		paletteFooterSetText(f.mode256, "256색")
	}
	if f.mode && len(f.palette) == 256 {
		paletteFooterSetText(f.modeRGB, "일반 RGB")
		paletteFooterSetText(f.mode16, "16색")
		paletteFooterSetText(f.mode256, "[256색]")
	}
	maxPage := (len(f.palette) - 1) / 16
	if f.page > maxPage {
		f.page = maxPage
	}
	if f.page < 0 {
		f.page = 0
	}
	start := f.page * 16
	paletteFooterSetText(f.pageLabel, fmt.Sprintf("%d~%d", start, start+15))
	for _, h := range f.swatches {
		if h != 0 {
			procInvalidateRect.Call(h, 0, 1)
		}
	}
}
func colorDialogHandlePaletteCommand(hwnd, id uintptr) bool {
	f := activeColorPaletteFooter
	if f == nil || f.hwnd != hwnd {
		return false
	}
	switch {
	case id == paletteFooterExpandID:
		colorDialogToggleFooter(hwnd, !f.expanded)
		return true
	case id == paletteFooterRGBID || id == paletteFooter16ID || id == paletteFooter256ID:
		if app != nil && app.dejavu != nil && id != paletteFooter16ID {
			messageBox(hwnd, "DejaVu IMG는 16색 인덱스 고정 규격입니다.", "인덱스", MB_OK|MB_ICONINFORMATION)
			return true
		}
		f.mode = id != paletteFooterRGBID
		switch id {
		case paletteFooter16ID:
			if len(f.palette) != 16 {
				if app != nil {
					base := app.indexedColors
					f.palette = append([]uint32(nil), base[:]...)
				} else {
					f.palette = append([]uint32(nil), f.palette[:16]...)
				}
			}
			f.changed = true
			f.page = 0
		case paletteFooter256ID:
			if len(f.palette) != 256 {
				if app != nil && len(app.indexedColors256) == 256 {
					f.palette = append([]uint32(nil), app.indexedColors256...)
				} else {
					f.palette = defaultIndexed256Palette(f.palette)
				}
			}
			f.changed = true
			f.page = 0
		}
		f.editIndex = false
		f.selectedIndex = -1
		colorDialogUpdateFooter(hwnd)
		return true
	case id == paletteFooterPrevID:
		if f.page > 0 {
			f.page--
		}
		colorDialogUpdateFooter(hwnd)
		return true
	case id == paletteFooterNextID:
		if f.page < (len(f.palette)-1)/16 {
			f.page++
		}
		colorDialogUpdateFooter(hwnd)
		return true
	case id == paletteFooterLoadID:
		path, ok := chooseIndexedPaletteFile(hwnd, false)
		if !ok {
			return true
		}
		p, err := loadPaletteFromPath(path)
		if err == nil && app != nil && app.dejavu != nil && len(p) != 16 {
			err = fmt.Errorf("DejaVu IMG는 16색만 가능합니다")
		}
		if err != nil {
			messageBox(hwnd, err.Error(), "팔레트 불러오기", MB_OK|MB_ICONERROR)
			return true
		}
		f.palette = p
		f.changed = true
		f.selectedIndex = -1
		f.editIndex = false
		f.mode = true
		f.page = 0
		colorDialogUpdateFooter(hwnd)
		paletteFooterSetText(f.info, fmt.Sprintf("%d색 팔레트 불러옴 · 확인 시 적용", len(p)))
		return true
	case id == paletteFooterSaveID:
		path, ok := chooseIndexedPaletteFile(hwnd, true)
		if !ok {
			return true
		}
		if err := savePaletteToPath(path, f.palette, hwnd); err != nil {
			messageBox(hwnd, err.Error(), "팔레트 저장", MB_OK|MB_ICONERROR)
		}
		return true
	case id >= paletteFooterSwatchBaseID && id < paletteFooterSwatchBaseID+16:
		idx := f.page*16 + int(id-paletteFooterSwatchBaseID)
		if idx >= len(f.palette) {
			return true
		}
		f.selectedIndex = idx
		f.editIndex = true
		f.mode = true
		paletteFooterSetText(f.info, fmt.Sprintf("인덱스 %d 선택 · RGB 색을 선택하고 확인", idx))
		colorDialogUpdateFooter(hwnd)
		return true
	}
	return false
}
func colorDialogDrawPaletteSwatch(hwnd uintptr, dis *DRAWITEMSTRUCT) bool {
	f := activeColorPaletteFooter
	if f == nil || f.hwnd != hwnd || dis == nil || dis.CtlID < paletteFooterSwatchBaseID || dis.CtlID >= paletteFooterSwatchBaseID+16 {
		return false
	}
	idx := f.page*16 + int(dis.CtlID-paletteFooterSwatchBaseID)
	if idx >= len(f.palette) {
		return true
	}
	c := f.palette[idx]
	fillRect(dis.HDC, dis.RcItem, c)
	stroke := rgb(105, 105, 105)
	if idx == f.selectedIndex {
		stroke = rgb(253, 161, 39)
	}
	frameRect(dis.HDC, dis.RcItem, stroke)
	fg := rgb(255, 255, 255)
	if (int(byte(c))*299+int(byte(c>>8))*587+int(byte(c>>16))*114)/1000 > 140 {
		fg = rgb(0, 0, 0)
	}
	drawToolOptionText(dis.HDC, fmt.Sprintf("%d", idx), dis.RcItem, DT_CENTER|DT_VCENTER|DT_SINGLELINE, fg, 11)
	return true
}
