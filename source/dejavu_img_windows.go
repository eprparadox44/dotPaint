//go:build windows

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

type dejavuIMGDocument struct {
	Original         []byte
	OriginalIndices  []byte // tile-major, 144 * 24 * 24
	OriginalCanvas   []byte // BGRA snapshot used to preserve unchanged source indices
	Palette          dejavuPalette
	PalettePath      string
	ActiveTile       int
	TransparentIndex byte
}

func dejavuPaletteColorRefs(p dejavuPalette) []uint32 {
	out := make([]uint32, 16)
	for i := range out {
		out[i] = p.ColorRef(i)
	}
	return out
}

func dejavuCanvasFromIndices(indices []byte, pal dejavuPalette) (*Canvas, error) {
	sheet, err := dejavuIndicesToSheet(indices, pal)
	if err != nil {
		return nil, err
	}
	cv := NewCanvas(dejavuIMGSheetWidth, dejavuIMGSheetHeight, rgb(0, 0, 0))
	for y := 0; y < dejavuIMGSheetHeight; y++ {
		for x := 0; x < dejavuIMGSheetWidth; x++ {
			c := pal.Color(int(sheet.ColorIndexAt(x, y)))
			i := (y*cv.W + x) * 4
			cv.Pix[i+0] = c.B
			cv.Pix[i+1] = c.G
			cv.Pix[i+2] = c.R
			cv.Pix[i+3] = 255
		}
	}
	return cv, nil
}

func dejavuTileMajorOffsetFromSheetXY(x, y int) int {
	tileX, tileY := x/dejavuIMGTileSize, y/dejavuIMGTileSize
	tile := tileY*dejavuIMGSheetColumns + tileX
	lx, ly := x%dejavuIMGTileSize, y%dejavuIMGTileSize
	return tile*dejavuIMGTilePixels + ly*dejavuIMGTileSize + lx
}

func canvasPixelBGRAEqual(pixA []byte, offA int, pixB []byte, offB int) bool {
	if offA < 0 || offA+4 > len(pixA) || offB < 0 || offB+4 > len(pixB) {
		return false
	}
	return pixA[offA] == pixB[offB] && pixA[offA+1] == pixB[offB+1] && pixA[offA+2] == pixB[offB+2] && pixA[offA+3] == pixB[offB+3]
}

func (a *App) currentDejavuAllIndices() ([]byte, error) {
	if a.dejavu == nil {
		return nil, fmt.Errorf("DejaVu IMG 문서가 아닙니다")
	}
	if a.canvas == nil || a.canvas.W != dejavuIMGSheetWidth || a.canvas.H != dejavuIMGSheetHeight {
		return nil, fmt.Errorf("DejaVu IMG 캔버스 크기가 손상되었습니다: %dx%d", a.canvas.W, a.canvas.H)
	}
	doc := a.dejavu
	out := make([]byte, dejavuIMGTileCount*dejavuIMGTilePixels)
	for y := 0; y < dejavuIMGSheetHeight; y++ {
		for x := 0; x < dejavuIMGSheetWidth; x++ {
			ci := dejavuTileMajorOffsetFromSheetXY(x, y)
			po := (y*a.canvas.W + x) * 4
			if len(doc.OriginalCanvas) == len(a.canvas.Pix) && ci < len(doc.OriginalIndices) && canvasPixelBGRAEqual(a.canvas.Pix, po, doc.OriginalCanvas, po) {
				out[ci] = doc.OriginalIndices[ci] & 0x0f
				continue
			}
			if po+4 > len(a.canvas.Pix) {
				continue
			}
			alpha := a.canvas.Pix[po+3]
			if alpha == 0 {
				out[ci] = doc.TransparentIndex & 0x0f
				continue
			}
			b, g, r := a.canvas.Pix[po], a.canvas.Pix[po+1], a.canvas.Pix[po+2]
			if alpha != 255 {
				bg := doc.Palette.Color(int(doc.TransparentIndex & 0x0f))
				aa := int(alpha)
				r = byte((int(r)*aa + int(bg.R)*(255-aa) + 127) / 255)
				g = byte((int(g)*aa + int(bg.G)*(255-aa) + 127) / 255)
				b = byte((int(b)*aa + int(bg.B)*(255-aa) + 127) / 255)
			}
			out[ci] = dejavuNearestPaletteIndex(r, g, b, doc.Palette)
		}
	}
	return out, nil
}

func dejavuAutoPalettePath(imgPath string) string {
	return filepath.Join(filepath.Dir(imgPath), "DEJAVU.DAT")
}

func chooseDejavuDATFile(hwnd uintptr, initial string) (string, bool) {
	buf := make([]uint16, 32768)
	if initial != "" {
		copy(buf, syscall.StringToUTF16(initial))
	}
	s := "DejaVu 팔레트 (DEJAVU.DAT)\x00*.dat\x00모든 파일 (*.*)\x00*.*\x00\x00"
	filter := utf16.Encode([]rune(s))
	ofn := OPENFILENAMEW{LStructSize: uint32(unsafe.Sizeof(OPENFILENAMEW{})), HwndOwner: hwnd, LpstrFilter: &filter[0], LpstrFile: &buf[0], NMaxFile: uint32(len(buf)), Flags: OFN_PATHMUSTEXIST | OFN_FILEMUSTEXIST | OFN_HIDEREADONLY}
	r, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

func chooseDejavuPNGOpenFile(hwnd uintptr) (string, bool) {
	buf := make([]uint16, 32768)
	s := "PNG 그림 (*.png)\x00*.png\x00모든 파일 (*.*)\x00*.*\x00\x00"
	filter := utf16.Encode([]rune(s))
	ofn := OPENFILENAMEW{LStructSize: uint32(unsafe.Sizeof(OPENFILENAMEW{})), HwndOwner: hwnd, LpstrFilter: &filter[0], LpstrFile: &buf[0], NMaxFile: uint32(len(buf)), Flags: OFN_PATHMUSTEXIST | OFN_FILEMUSTEXIST | OFN_HIDEREADONLY}
	r, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

func chooseDejavuPNGSaveFile(hwnd uintptr, suggested string) (string, bool) {
	buf := make([]uint16, 32768)
	if suggested != "" {
		copy(buf, syscall.StringToUTF16(suggested))
	}
	s := "PNG 그림 (*.png)\x00*.png\x00모든 파일 (*.*)\x00*.*\x00\x00"
	filter := utf16.Encode([]rune(s))
	ofn := OPENFILENAMEW{LStructSize: uint32(unsafe.Sizeof(OPENFILENAMEW{})), HwndOwner: hwnd, LpstrFilter: &filter[0], LpstrFile: &buf[0], NMaxFile: uint32(len(buf)), Flags: OFN_PATHMUSTEXIST | OFN_OVERWRITEPROMPT, LpstrDefExt: utf16Ptr("png")}
	r, _, _ := procGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	path := syscall.UTF16ToString(buf)
	if filepath.Ext(path) == "" {
		path += ".png"
	}
	return path, true
}

func chooseDejavuIMGSaveFile(hwnd uintptr, current string) (string, bool) {
	buf := make([]uint16, 32768)
	if current != "" {
		copy(buf, syscall.StringToUTF16(current))
	}
	s := "DejaVu 타일 그래픽 (*.img)\x00*.img\x00모든 파일 (*.*)\x00*.*\x00\x00"
	filter := utf16.Encode([]rune(s))
	ofn := OPENFILENAMEW{LStructSize: uint32(unsafe.Sizeof(OPENFILENAMEW{})), HwndOwner: hwnd, LpstrFilter: &filter[0], LpstrFile: &buf[0], NMaxFile: uint32(len(buf)), Flags: OFN_PATHMUSTEXIST | OFN_OVERWRITEPROMPT, LpstrDefExt: utf16Ptr("img")}
	r, _, _ := procGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	path := syscall.UTF16ToString(buf)
	if filepath.Ext(path) == "" {
		path += ".IMG"
	}
	return path, true
}

func readDejavuPaletteFile(path string) (dejavuPalette, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return dejavuPalette{}, err
	}
	return decodeDejavuPaletteDAT(data)
}

func (a *App) resolveDejavuPalette(imgPath string) (dejavuPalette, string, bool) {
	auto := dejavuAutoPalettePath(imgPath)
	if p, err := readDejavuPaletteFile(auto); err == nil {
		return p, auto, true
	}
	answer := messageBox(a.hwnd, "IMG 파일과 같은 폴더에서 DEJAVU.DAT 팔레트를 찾지 못했습니다.\n\n팔레트 파일을 직접 선택하시겠습니까?", "DejaVu IMG", MB_YESNO|MB_ICONQUESTION)
	if answer != IDYES {
		return dejavuPalette{}, "", false
	}
	path, ok := chooseDejavuDATFile(a.hwnd, auto)
	if !ok {
		return dejavuPalette{}, "", false
	}
	p, err := readDejavuPaletteFile(path)
	if err != nil {
		messageBox(a.hwnd, "DEJAVU.DAT 팔레트를 읽을 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return dejavuPalette{}, "", false
	}
	return p, path, true
}

func (a *App) enterDejavuPalette(p dejavuPalette) {
	if a.dejavuPrevPalette == nil {
		a.dejavuPrevPalette = append([]uint32(nil), a.palette...)
		a.dejavuPrevFG, a.dejavuPrevBG = a.fg, a.bg
	}
	a.palette = dejavuPaletteColorRefs(p)
	a.fg = a.palette[0]
	a.bg = a.palette[15]
}

func (a *App) leaveDejavuIMGMode() {
	if a.dejavu == nil && a.dejavuPrevPalette == nil {
		return
	}
	closeDejavuTileBrowserForOwner(a)
	a.dejavu = nil
	if a.dejavuPrevPalette != nil {
		a.palette = append([]uint32(nil), a.dejavuPrevPalette...)
		a.fg, a.bg = a.dejavuPrevFG, a.dejavuPrevBG
	}
	a.dejavuPrevPalette = nil
	if a.indexedMode {
		if a.indexedSize == 256 {
			a.indexedColors256 = append([]uint32(nil), a.palette...)
		} else {
			a.indexedColors = indexedPaletteFromRefs(a.palette)
		}
	}
	a.dejavuPrevFG, a.dejavuPrevBG = 0, 0
	if a.hwnd != 0 {
		a.rebuildMenuForTheme()
	}
}

func (a *App) openDejavuIMGDocument(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		messageBox(a.hwnd, "IMG 파일을 열 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return false
	}
	if err := validateDejavuIMG(data); err != nil {
		messageBox(a.hwnd, "지원하는 DejaVu IMG 형식이 아닙니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return false
	}
	pal, palPath, ok := a.resolveDejavuPalette(path)
	if !ok {
		return false
	}
	indices, err := decodeDejavuAllIndices(data)
	if err != nil {
		messageBox(a.hwnd, "IMG 타일을 디코딩할 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return false
	}
	cv, err := dejavuCanvasFromIndices(indices, pal)
	if err != nil {
		messageBox(a.hwnd, "IMG 타일시트를 만들 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return false
	}

	a.leaveDejavuIMGMode()
	a.canvas = cv
	a.dejavu = &dejavuIMGDocument{
		Original:         append([]byte(nil), data...),
		OriginalIndices:  append([]byte(nil), indices...),
		OriginalCanvas:   cv.ClonePixels(),
		Palette:          pal,
		PalettePath:      palPath,
		ActiveTile:       0,
		TransparentIndex: 0,
	}
	a.enterDejavuPalette(pal)
	a.clearSelectionState()
	a.filePath = path
	a.undo.Reset()
	a.resetHistory("DejaVu IMG 열기")
	a.scrollX, a.scrollY = 0, 0
	a.clearDirty()
	a.addRecent(path)
	a.updateScrollbars()
	a.rebuildMenuForTheme()
	a.statusText = fmt.Sprintf("DejaVu IMG: 144타일 · 24×24 · 팔레트 %s · 타일 0 선택", filepath.Base(palPath))
	a.showDejavuTileBrowser()
	logf("DEJAVU_IMG open path=%q bytes=%d palette=%q tiles=%d", path, len(data), palPath, dejavuIMGTileCount)
	invalidate(a.hwnd)
	return true
}

func (a *App) dejavuTileChanged(tile int) bool {
	if a.dejavu == nil || tile < 0 || tile >= dejavuIMGTileCount || len(a.dejavu.OriginalCanvas) != len(a.canvas.Pix) {
		return true
	}
	tx := (tile % dejavuIMGSheetColumns) * dejavuIMGTileSize
	ty := (tile / dejavuIMGSheetColumns) * dejavuIMGTileSize
	for y := 0; y < dejavuIMGTileSize; y++ {
		off := ((ty+y)*a.canvas.W + tx) * 4
		n := dejavuIMGTileSize * 4
		if !bytes.Equal(a.canvas.Pix[off:off+n], a.dejavu.OriginalCanvas[off:off+n]) {
			return true
		}
	}
	return false
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".dotpaint-dejavu-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		// Windows cannot replace an existing file with Rename. The Save dialog
		// has already confirmed overwrite, so remove and retry without ever
		// touching the in-memory original record bytes.
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return err
		}
		if err2 := os.Rename(tmpName, path); err2 != nil {
			return err2
		}
	}
	ok = true
	return nil
}

func (a *App) saveDejavuIMG(forceAs bool) bool {
	if a.dejavu == nil {
		return false
	}
	path := a.filePath
	if forceAs || path == "" || !strings.EqualFold(filepath.Ext(path), ".img") {
		var ok bool
		path, ok = chooseDejavuIMGSaveFile(a.hwnd, path)
		if !ok {
			return false
		}
	}
	allIndices, err := a.currentDejavuAllIndices()
	if err != nil {
		messageBox(a.hwnd, "IMG 저장 데이터를 만들 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return false
	}
	out := append([]byte(nil), a.dejavu.Original...)
	modified := 0
	for tile := 0; tile < dejavuIMGTileCount; tile++ {
		if !a.dejavuTileChanged(tile) {
			continue
		}
		modified++
		rec := out[tile*dejavuIMGRecordSize : (tile+1)*dejavuIMGRecordSize]
		idx := allIndices[tile*dejavuIMGTilePixels : (tile+1)*dejavuIMGTilePixels]
		if err := encodeDejavuTileIntoRecord(rec, idx); err != nil {
			messageBox(a.hwnd, "IMG 타일을 인코딩할 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
			return false
		}
	}
	if err := writeFileAtomic(path, out); err != nil {
		messageBox(a.hwnd, "IMG 파일을 저장할 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return false
	}
	a.dejavu.Original = append([]byte(nil), out...)
	a.dejavu.OriginalIndices = append([]byte(nil), allIndices...)
	a.dejavu.OriginalCanvas = a.canvas.ClonePixels()
	a.filePath = path
	a.clearDirty()
	a.addRecent(path)
	a.statusText = fmt.Sprintf("DejaVu IMG 저장 완료: 수정 타일 %d개 · 헤더/미확정 2바이트 보존", modified)
	logf("DEJAVU_IMG save path=%q modified_tiles=%d bytes=%d", path, modified, len(out))
	invalidateDejavuTileBrowser(a)
	invalidate(a.hwnd)
	return true
}

func (a *App) setCanvasFromDejavuAllIndices(indices []byte, historyName string) error {
	if a.dejavu == nil {
		return fmt.Errorf("DejaVu IMG 문서가 아닙니다")
	}
	cv, err := dejavuCanvasFromIndices(indices, a.dejavu.Palette)
	if err != nil {
		return err
	}
	a.clearSelectionState()
	a.canvas.W, a.canvas.H = cv.W, cv.H
	a.canvas.Pix = cv.Pix
	a.undo.Reset()
	a.recordHistory(historyName)
	a.markDirty()
	a.updateScrollbars()
	invalidateDejavuTileBrowser(a)
	invalidate(a.hwnd)
	return nil
}

func (a *App) currentDejavuSheetPaletted() (*image.Paletted, error) {
	idx, err := a.currentDejavuAllIndices()
	if err != nil {
		return nil, err
	}
	return dejavuIndicesToSheet(idx, a.dejavu.Palette)
}

func (a *App) exportDejavuSheetPNG() {
	if a.dejavu == nil {
		return
	}
	base := strings.TrimSuffix(filepath.Base(a.filePath), filepath.Ext(a.filePath))
	suggested := filepath.Join(filepath.Dir(a.filePath), base+"_tilesheet.png")
	path, ok := chooseDejavuPNGSaveFile(a.hwnd, suggested)
	if !ok {
		return
	}
	img, err := a.currentDejavuSheetPaletted()
	if err == nil {
		var f *os.File
		f, err = os.Create(path)
		if err == nil {
			err = png.Encode(f, img)
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		messageBox(a.hwnd, "PNG 타일시트를 내보낼 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return
	}
	a.statusText = "DejaVu PNG 타일시트를 내보냈습니다: " + filepath.Base(path)
	logf("DEJAVU_IMG export-sheet path=%q", path)
	invalidate(a.hwnd)
}

func decodePNGForDejavu(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func (a *App) maybeResizeDejavuImport(img image.Image, wantW, wantH int, what string) (image.Image, bool) {
	b := img.Bounds()
	if b.Dx() == wantW && b.Dy() == wantH {
		return img, true
	}
	answer := messageBox(a.hwnd,
		fmt.Sprintf("%s 크기가 %d×%d입니다. DejaVu 형식은 %d×%d가 필요합니다.\n\n자동으로 왜곡하지 않습니다. 최근접 픽셀 방식으로 %d×%d로 변환해서 가져오시겠습니까?", what, b.Dx(), b.Dy(), wantW, wantH, wantW, wantH),
		"DejaVu IMG", MB_YESNO|MB_ICONWARNING)
	if answer != IDYES {
		return nil, false
	}
	return dejavuResizeNearest(img, wantW, wantH), true
}

func (a *App) importDejavuSheetPNG() {
	if a.dejavu == nil {
		return
	}
	path, ok := chooseDejavuPNGOpenFile(a.hwnd)
	if !ok {
		return
	}
	img, err := decodePNGForDejavu(path)
	if err != nil {
		messageBox(a.hwnd, "PNG 타일시트를 읽을 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return
	}
	img, ok = a.maybeResizeDejavuImport(img, dejavuIMGSheetWidth, dejavuIMGSheetHeight, "타일시트 PNG")
	if !ok {
		return
	}
	sheetIdx := dejavuImageToIndices(img, dejavuIMGSheetWidth, dejavuIMGSheetHeight, a.dejavu.Palette, a.dejavu.TransparentIndex)
	tileIdx, err := dejavuSheetToTileIndices(sheetIdx)
	if err != nil {
		messageBox(a.hwnd, "타일 순서를 변환할 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return
	}
	if err := a.setCanvasFromDejavuAllIndices(tileIdx, "DejaVu 타일시트 PNG 가져오기"); err != nil {
		messageBox(a.hwnd, "타일시트를 적용할 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return
	}
	a.statusText = fmt.Sprintf("PNG 타일시트를 가져왔습니다. RGB는 16색 팔레트로 변환했고 투명 픽셀은 인덱스 %d를 사용했습니다.", a.dejavu.TransparentIndex)
	logf("DEJAVU_IMG import-sheet path=%q transparent_index=%d", path, a.dejavu.TransparentIndex)
}

func (a *App) exportDejavuTilePNG() {
	if a.dejavu == nil {
		return
	}
	tile := a.dejavu.ActiveTile
	idx, err := a.currentDejavuAllIndices()
	if err != nil {
		messageBox(a.hwnd, err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return
	}
	palImg := image.NewPaletted(image.Rect(0, 0, dejavuIMGTileSize, dejavuIMGTileSize), a.dejavu.Palette.Palette())
	src := idx[tile*dejavuIMGTilePixels : (tile+1)*dejavuIMGTilePixels]
	for y := 0; y < dejavuIMGTileSize; y++ {
		for x := 0; x < dejavuIMGTileSize; x++ {
			palImg.SetColorIndex(x, y, src[y*dejavuIMGTileSize+x]&0x0f)
		}
	}
	base := strings.TrimSuffix(filepath.Base(a.filePath), filepath.Ext(a.filePath))
	suggested := filepath.Join(filepath.Dir(a.filePath), fmt.Sprintf("%s_tile_%03d.png", base, tile))
	path, ok := chooseDejavuPNGSaveFile(a.hwnd, suggested)
	if !ok {
		return
	}
	f, err := os.Create(path)
	if err == nil {
		err = png.Encode(f, palImg)
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	}
	if err != nil {
		messageBox(a.hwnd, "타일 PNG를 내보낼 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return
	}
	a.statusText = fmt.Sprintf("타일 %d PNG를 내보냈습니다.", tile)
	logf("DEJAVU_IMG export-tile tile=%d path=%q", tile, path)
	invalidate(a.hwnd)
}

func (a *App) importDejavuTilePNG() {
	if a.dejavu == nil {
		return
	}
	tile := a.dejavu.ActiveTile
	path, ok := chooseDejavuPNGOpenFile(a.hwnd)
	if !ok {
		return
	}
	img, err := decodePNGForDejavu(path)
	if err != nil {
		messageBox(a.hwnd, "타일 PNG를 읽을 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return
	}
	img, ok = a.maybeResizeDejavuImport(img, dejavuIMGTileSize, dejavuIMGTileSize, fmt.Sprintf("타일 %d PNG", tile))
	if !ok {
		return
	}
	tileIdx := dejavuImageToIndices(img, dejavuIMGTileSize, dejavuIMGTileSize, a.dejavu.Palette, a.dejavu.TransparentIndex)
	all, err := a.currentDejavuAllIndices()
	if err != nil {
		messageBox(a.hwnd, err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return
	}
	copy(all[tile*dejavuIMGTilePixels:(tile+1)*dejavuIMGTilePixels], tileIdx)
	if err := a.setCanvasFromDejavuAllIndices(all, fmt.Sprintf("DejaVu 타일 %d PNG 가져오기", tile)); err != nil {
		messageBox(a.hwnd, "타일을 적용할 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return
	}
	a.statusText = fmt.Sprintf("타일 %d PNG를 가져왔습니다. 투명 픽셀은 팔레트 인덱스 %d로 적용했습니다.", tile, a.dejavu.TransparentIndex)
	logf("DEJAVU_IMG import-tile tile=%d path=%q transparent_index=%d", tile, path, a.dejavu.TransparentIndex)
}

func (a *App) reloadDejavuPaletteInteractive() {
	if a.dejavu == nil {
		return
	}
	path, ok := chooseDejavuDATFile(a.hwnd, a.dejavu.PalettePath)
	if !ok {
		return
	}
	p, err := readDejavuPaletteFile(path)
	if err != nil {
		messageBox(a.hwnd, "팔레트를 읽을 수 없습니다.\n\n"+err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return
	}
	currentIdx, err := a.currentDejavuAllIndices()
	if err != nil {
		messageBox(a.hwnd, err.Error(), "DejaVu IMG", MB_OK|MB_ICONERROR)
		return
	}
	a.dejavu.Palette = p
	a.dejavu.PalettePath = path
	a.palette = dejavuPaletteColorRefs(p)
	a.fg, a.bg = a.palette[0], a.palette[15]
	cv, _ := dejavuCanvasFromIndices(currentIdx, p)
	a.canvas.Pix = cv.Pix
	// Baseline is recoloured from the original saved indices so unsaved edits
	// remain detectable after a palette change.
	base, _ := dejavuCanvasFromIndices(a.dejavu.OriginalIndices, p)
	a.dejavu.OriginalCanvas = base.Pix
	a.statusText = "DejaVu 팔레트를 다시 불러왔습니다: " + filepath.Base(path)
	logf("DEJAVU_IMG reload-palette path=%q", path)
	invalidateDejavuTileBrowser(a)
	invalidate(a.hwnd)
}

func (a *App) focusDejavuTile(tile int) {
	if a.dejavu == nil {
		return
	}
	tile = clamp(tile, 0, dejavuIMGTileCount-1)
	a.dejavu.ActiveTile = tile
	if a.validZoomLevel(15) && a.zoom != 15 {
		a.setZoom(15, -1, -1)
	}
	var rc RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	ws, _, _, _ := a.layout(rc)
	pageW := max(1, int(ws.Right-ws.Left)-16)
	pageH := max(1, int(ws.Bottom-ws.Top)-16)
	tx := (tile % dejavuIMGSheetColumns) * dejavuIMGTileSize
	ty := (tile / dejavuIMGSheetColumns) * dejavuIMGTileSize
	centerX := (tx + dejavuIMGTileSize/2) * a.zoom
	centerY := (ty + dejavuIMGTileSize/2) * a.zoom
	a.scrollX = centerX - pageW/2
	a.scrollY = centerY - pageH/2
	a.updateScrollbars()
	if a.zoom >= 6 && !a.showGrid {
		a.showGrid = true
		a.toggleMenuCheck(ID_VIEW_GRID, true)
	}
	a.statusText = fmt.Sprintf("DejaVu 타일 %d 선택 · 위치 (%d,%d) · 24×24 픽셀", tile, tile%12, tile/12)
	invalidateDejavuTileBrowser(a)
	invalidate(a.hwnd)
}

func (a *App) paintDejavuActiveTileOverlay(hdc uintptr, ox, oy int) {
	if a.dejavu == nil || a.zoom < 1 {
		return
	}
	tile := clamp(a.dejavu.ActiveTile, 0, dejavuIMGTileCount-1)
	tx := (tile % dejavuIMGSheetColumns) * dejavuIMGTileSize
	ty := (tile / dejavuIMGSheetColumns) * dejavuIMGTileSize
	l := ox + tx*a.zoom
	t := oy + ty*a.zoom
	r := l + dejavuIMGTileSize*a.zoom
	b := t + dejavuIMGTileSize*a.zoom
	frameUIRect(hdc, RECT{int32(l), int32(t), int32(r), int32(b)}, rgb(0, 120, 215))
	if a.zoom >= 4 {
		frameUIRect(hdc, RECT{int32(l + 1), int32(t + 1), int32(r - 1), int32(b - 1)}, rgb(255, 255, 255))
	}
}

// --- 12x12 tile browser ----------------------------------------------------

const dejavuTileBrowserClassName = "dotPaintDejavuIMGTileBrowser139"

type dejavuTileBrowserState struct {
	owner *App
	hwnd  uintptr
}

var (
	dejavuTileBrowserWndProcCallback uintptr
	dejavuTileBrowserClassRegistered bool
	activeDejavuTileBrowser          *dejavuTileBrowserState
)

func registerDejavuTileBrowserClass(hinst uintptr) bool {
	if dejavuTileBrowserClassRegistered {
		return true
	}
	if dejavuTileBrowserWndProcCallback == 0 {
		dejavuTileBrowserWndProcCallback = syscall.NewCallback(dejavuTileBrowserWndProc)
	}
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         CS_HREDRAW | CS_VREDRAW | CS_DBLCLKS,
		LpfnWndProc:   dejavuTileBrowserWndProcCallback,
		HInstance:     hinst,
		HCursor:       cursor,
		HbrBackground: uintptr(COLOR_BTNFACE + 1),
		LpszClassName: utf16Ptr(dejavuTileBrowserClassName),
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		logf("DEJAVU_IMG tile-browser register failed: %v", err)
		return false
	}
	dejavuTileBrowserClassRegistered = true
	return true
}

func (a *App) showDejavuTileBrowser() {
	if a.dejavu == nil {
		return
	}
	if activeDejavuTileBrowser != nil && activeDejavuTileBrowser.owner == a && activeDejavuTileBrowser.hwnd != 0 {
		if ok, _, _ := procIsWindow.Call(activeDejavuTileBrowser.hwnd); ok != 0 {
			procShowWindow.Call(activeDejavuTileBrowser.hwnd, SW_SHOW)
			procSetForegroundWindow.Call(activeDejavuTileBrowser.hwnd)
			invalidate(activeDejavuTileBrowser.hwnd)
			return
		}
	}
	if !registerDejavuTileBrowserClass(a.hinst) {
		messageBox(a.hwnd, "DejaVu 타일 목록 창을 만들 수 없습니다.", "DejaVu IMG", MB_OK|MB_ICONERROR)
		return
	}
	state := &dejavuTileBrowserState{owner: a}
	activeDejavuTileBrowser = state
	title := "DejaVu IMG 타일 목록"
	if a.filePath != "" {
		title += " - " + filepath.Base(a.filePath)
	}
	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(dejavuTileBrowserClassName))),
		uintptr(unsafe.Pointer(utf16Ptr(title))),
		uintptr(WS_CAPTION|WS_SYSMENU|WS_MINIMIZEBOX|WS_THICKFRAME),
		uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), 650, 710,
		a.hwnd, 0, a.hinst, 0,
	)
	if hwnd == 0 {
		activeDejavuTileBrowser = nil
		logf("DEJAVU_IMG tile-browser create failed: %v", err)
		return
	}
	state.hwnd = hwnd
	showThemedEditorWindow(hwnd, a.darkMode)
	logf("DEJAVU_IMG tile-browser opened hwnd=%#x", hwnd)
}

func closeDejavuTileBrowserForOwner(a *App) {
	if activeDejavuTileBrowser == nil || activeDejavuTileBrowser.owner != a || activeDejavuTileBrowser.hwnd == 0 {
		return
	}
	if ok, _, _ := procIsWindow.Call(activeDejavuTileBrowser.hwnd); ok != 0 {
		procDestroyWindow.Call(activeDejavuTileBrowser.hwnd)
	}
}

func invalidateDejavuTileBrowser(a *App) {
	if activeDejavuTileBrowser == nil || activeDejavuTileBrowser.owner != a || activeDejavuTileBrowser.hwnd == 0 {
		return
	}
	if ok, _, _ := procIsWindow.Call(activeDejavuTileBrowser.hwnd); ok != 0 {
		invalidate(activeDejavuTileBrowser.hwnd)
	}
}

func dejavuTileBrowserGrid(client RECT) (left, top, cell int) {
	left, top = 14, 54
	availW := max(12, int(client.Right)-left*2)
	availH := max(12, int(client.Bottom)-top-32)
	cell = min(availW/12, availH/12)
	cell = clamp(cell, 28, 48)
	return
}

func dejavuTileBrowserHit(client RECT, x, y int) int {
	left, top, cell := dejavuTileBrowserGrid(client)
	if x < left || y < top {
		return -1
	}
	col, row := (x-left)/cell, (y-top)/cell
	if col < 0 || col >= 12 || row < 0 || row >= 12 {
		return -1
	}
	if (x-left)%cell >= cell || (y-top)%cell >= cell {
		return -1
	}
	return row*12 + col
}

func paintDejavuTileThumbnail(hdc uintptr, a *App, tile int, rc RECT) {
	if a == nil || a.canvas == nil || a.canvas.W != dejavuIMGSheetWidth || a.canvas.H != dejavuIMGSheetHeight {
		return
	}
	const srcW = dejavuIMGTileSize
	const srcH = dejavuIMGTileSize
	pix := make([]byte, srcW*srcH*4)
	tx := (tile % 12) * 24
	ty := (tile / 12) * 24
	for y := 0; y < srcH; y++ {
		src := ((ty+y)*a.canvas.W + tx) * 4
		dst := y * srcW * 4
		copy(pix[dst:dst+srcW*4], a.canvas.Pix[src:src+srcW*4])
	}
	labelH := 11
	preview := RECT{rc.Left + 2, rc.Top + 2, rc.Right - 2, rc.Bottom - int32(labelH)}
	pw := max(1, int(preview.Right-preview.Left))
	ph := max(1, int(preview.Bottom-preview.Top))
	size := min(pw, ph)
	dx := int(preview.Left) + (pw-size)/2
	dy := int(preview.Top) + (ph-size)/2
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: 24, BiHeight: -24, BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	procSetStretchBltMode.Call(hdc, COLORONCOLOR)
	procStretchDIBits.Call(hdc, uintptr(dx), uintptr(dy), uintptr(size), uintptr(size), 0, 0, 24, 24, uintptr(unsafe.Pointer(&pix[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, SRCCOPY)
	label := RECT{rc.Left + 1, rc.Bottom - int32(labelH), rc.Right - 1, rc.Bottom - 1}
	drawUIText(hdc, fmt.Sprintf("%d", tile), label, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(30, 30, 30))
}

func paintDejavuTileBrowser(s *dejavuTileBrowserState, hdc uintptr) {
	if s == nil || s.owner == nil {
		return
	}
	var client RECT
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	fillUIRect(hdc, client, rgb(240, 240, 240))
	title := RECT{14, 8, client.Right - 14, 30}
	drawUIText(hdc, "144개 타일 · 12×12 · 클릭하면 해당 24×24 타일을 15배 확대해 중앙에 표시", title, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS, rgb(25, 25, 25))
	selected := 0
	if s.owner.dejavu != nil {
		selected = s.owner.dejavu.ActiveTile
	}
	info := RECT{14, 29, client.Right - 14, 51}
	drawUIText(hdc, fmt.Sprintf("선택 타일: %d   |   팔레트: %s", selected, filepath.Base(s.owner.dejavu.PalettePath)), info, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS, rgb(75, 75, 75))
	left, top, cell := dejavuTileBrowserGrid(client)
	for tile := 0; tile < 144; tile++ {
		col, row := tile%12, tile/12
		rc := RECT{int32(left + col*cell), int32(top + row*cell), int32(left + (col+1)*cell - 1), int32(top + (row+1)*cell - 1)}
		fillUIRect(hdc, rc, rgb(250, 250, 250))
		if tile == selected {
			frameUIRect(hdc, rc, rgb(0, 120, 215))
			inner := RECT{rc.Left + 1, rc.Top + 1, rc.Right - 1, rc.Bottom - 1}
			frameUIRect(hdc, inner, rgb(0, 120, 215))
		} else {
			frameUIRect(hdc, rc, rgb(170, 170, 170))
		}
		paintDejavuTileThumbnail(hdc, s.owner, tile, rc)
	}
}

func dejavuTileBrowserWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			logPanic("dejavuTileBrowserWndProc", r)
		}
	}()
	if r, handled := handleDarkEditorNonClient(hwnd, msg, wparam, lparam); handled {
		return r
	}
	s := activeDejavuTileBrowser
	switch msg {
	case WM_CREATE:
		if s == nil {
			return ^uintptr(0)
		}
		s.hwnd = hwnd
		showThemedEditorWindow(hwnd, s.owner.darkMode)
		scheduleDarkEditorChromeRefresh(hwnd)
		procSetFocus.Call(hwnd)
		return 0
	case WM_LBUTTONDOWN, WM_LBUTTONDBLCLK:
		if s != nil && s.hwnd == hwnd && s.owner != nil && s.owner.dejavu != nil {
			var client RECT
			procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
			x, y := int(signedLoword(lparam)), int(signedHiword(lparam))
			if tile := dejavuTileBrowserHit(client, x, y); tile >= 0 {
				s.owner.focusDejavuTile(tile)
				procSetFocus.Call(hwnd)
			}
		}
		return 0
	case WM_KEYDOWN:
		if s != nil && s.hwnd == hwnd && s.owner != nil && s.owner.dejavu != nil {
			tile := s.owner.dejavu.ActiveTile
			switch wparam {
			case VK_LEFT:
				if tile%12 > 0 {
					tile--
				}
			case VK_RIGHT:
				if tile%12 < 11 {
					tile++
				}
			case VK_UP:
				if tile >= 12 {
					tile -= 12
				}
			case VK_DOWN:
				if tile < 132 {
					tile += 12
				}
			case VK_ESCAPE:
				procDestroyWindow.Call(hwnd)
				return 0
			default:
				r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
				return r
			}
			s.owner.focusDejavuTile(tile)
			return 0
		}
	case WM_PAINT:
		if s != nil && s.hwnd == hwnd {
			var ps PAINTSTRUCT
			hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			paintDejavuTileBrowser(s, hdc)
			procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		}
		return 0
	case WM_SIZE:
		invalidate(hwnd)
		return 0
	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		if activeDejavuTileBrowser != nil && activeDejavuTileBrowser.hwnd == hwnd {
			activeDejavuTileBrowser.hwnd = 0
			activeDejavuTileBrowser = nil
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}
