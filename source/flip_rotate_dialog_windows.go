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

// A78: cleaned flip/rotate dialog layout; explanatory captions removed and lower controls aligned.
// Selection transforms now clear vacated source pixels to true transparency, so
// rotated corners and uncovered areas cannot reveal an unrelated background color.

const (
	rotateDialogClass = "dotPaintFlipRotateDialogA78"
	rotateAngleEditID = 7101
	ES_CENTER         = 0x0001
	SWP_NOZORDER      = 0x0004
	SWP_NOACTIVATE    = 0x0010
)

var (
	procEnableWindow       = modUser32.NewProc("EnableWindow")
	procDestroyWindow      = modUser32.NewProc("DestroyWindow")
	procGetWindowRect      = modUser32.NewProc("GetWindowRect")
	procSetWindowPos       = modUser32.NewProc("SetWindowPos")
	procIsWindow           = modUser32.NewProc("IsWindow")
	procAdjustWindowRectEx = modUser32.NewProc("AdjustWindowRectEx")
)

type FlipRotateOptions struct {
	FlipHorizontal bool
	FlipVertical   bool
	Angle          float64 // clockwise-positive degrees, normalized to [-180, 180]
	Constraint     LineConstraintMode
	Antialias      bool
}

type flipRotateDialogState struct {
	hwnd, parent  uintptr
	angleEdit     uintptr
	uiFont        uintptr
	source        *Canvas
	targetName    string
	options       FlipRotateOptions
	accepted      bool
	done          bool
	dragging      bool
	updatingEdit  bool
	scale         float64
	preview       *Canvas
	outputW       int
	outputH       int
	backDC        uintptr
	backBitmap    uintptr
	backOldBitmap uintptr
	backW         int
	backH         int
}

var (
	flipRotateDialog               *flipRotateDialogState
	flipRotateWndProcCB            uintptr
	flipRotateClassReady           bool
	flipRotateConstraintPreference = LineConstraint15Degrees
	flipRotateAntialiasPreference  = false
)

func loWord(v uintptr) int { return int(v & 0xFFFF) }
func hiWord(v uintptr) int { return int((v >> 16) & 0xFFFF) }

func normalizeRotationAngle(deg float64) float64 {
	if math.IsNaN(deg) || math.IsInf(deg, 0) {
		return 0
	}
	deg = math.Mod(deg, 360)
	if deg > 180 {
		deg -= 360
	} else if deg <= -180 {
		deg += 360
	}
	if math.Abs(deg) < 0.0005 {
		return 0
	}
	return math.Round(deg*10) / 10
}

func nearestTanRotationAngle(deg float64) float64 {
	rad := deg * math.Pi / 180
	d := nearestStrictTanStep(math.Cos(rad), math.Sin(rad))
	if d.X == 0 && d.Y == 0 {
		return 0
	}
	return normalizeRotationAngle(math.Atan2(float64(d.Y), float64(d.X)) * 180 / math.Pi)
}

func constrainedRotationAngle(deg float64, mode LineConstraintMode) float64 {
	if mode == LineConstraintTanN {
		return nearestTanRotationAngle(deg)
	}
	return normalizeRotationAngle(math.Round(deg/15) * 15)
}

func flipRotateOutputSize(w, h int, angle float64) (int, int, float64, float64) {
	if w < 1 || h < 1 {
		return 1, 1, 1, 0
	}
	angle = normalizeRotationAngle(angle)
	q := math.Round(angle / 90)
	if math.Abs(angle-q*90) < 0.0001 {
		qi := ((int(q) % 4) + 4) % 4
		switch qi {
		case 1:
			return h, w, 0, 1
		case 2:
			return w, h, -1, 0
		case 3:
			return h, w, 0, -1
		default:
			return w, h, 1, 0
		}
	}
	rad := angle * math.Pi / 180
	c, s := math.Cos(rad), math.Sin(rad)
	nw := int(math.Ceil(math.Abs(float64(w)*c) + math.Abs(float64(h)*s) - 1e-9))
	nh := int(math.Ceil(math.Abs(float64(w)*s) + math.Abs(float64(h)*c) - 1e-9))
	return max(nw, 1), max(nh, 1), c, s
}

func sourcePixelNearest(src *Canvas, x, y float64) [4]byte {
	ix, iy := int(math.Round(x)), int(math.Round(y))
	if src == nil || ix < 0 || iy < 0 || ix >= src.W || iy >= src.H {
		return [4]byte{}
	}
	i := (iy*src.W + ix) * 4
	return [4]byte{src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3]}
}

func sourcePixelBilinear(src *Canvas, x, y float64) [4]byte {
	if src == nil || x < -1 || y < -1 || x > float64(src.W) || y > float64(src.H) {
		return [4]byte{}
	}
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	weights := [4]float64{(1 - fx) * (1 - fy), fx * (1 - fy), (1 - fx) * fy, fx * fy}
	coords := [4][2]int{{x0, y0}, {x0 + 1, y0}, {x0, y0 + 1}, {x0 + 1, y0 + 1}}
	alpha := 0.0
	premul := [3]float64{}
	for j, q := range coords {
		if q[0] < 0 || q[1] < 0 || q[0] >= src.W || q[1] >= src.H {
			continue
		}
		i := (q[1]*src.W + q[0]) * 4
		a := float64(src.Pix[i+3]) / 255
		w := weights[j]
		alpha += a * w
		for ch := 0; ch < 3; ch++ {
			premul[ch] += float64(src.Pix[i+ch]) * a * w
		}
	}
	if alpha <= 0.000001 {
		return [4]byte{}
	}
	out := [4]byte{}
	for ch := 0; ch < 3; ch++ {
		out[ch] = byte(clamp(int(math.Round(premul[ch]/alpha)), 0, 255))
	}
	out[3] = byte(clamp(int(math.Round(alpha*255)), 0, 255))
	return out
}

func inverseFlipRotatePoint(srcW, srcH, outW, outH int, dx, dy float64, opts FlipRotateOptions, c, s float64) (float64, float64) {
	// Pixel-center coordinates. Positive angle is clockwise in screen coordinates.
	x := dx - float64(outW-1)/2
	y := dy - float64(outH-1)/2
	sx := c*x + s*y
	sy := -s*x + c*y
	if opts.FlipHorizontal {
		sx = -sx
	}
	if opts.FlipVertical {
		sy = -sy
	}
	return sx + float64(srcW-1)/2, sy + float64(srcH-1)/2
}

func transformCanvasFlipRotate(src *Canvas, opts FlipRotateOptions) *Canvas {
	if src == nil || src.W < 1 || src.H < 1 {
		return NewCanvas(1, 1, transparentColor)
	}
	opts.Angle = normalizeRotationAngle(opts.Angle)
	ow, oh, c, s := flipRotateOutputSize(src.W, src.H, opts.Angle)
	out := NewCanvas(ow, oh, transparentColor)
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			sx, sy := inverseFlipRotatePoint(src.W, src.H, ow, oh, float64(x), float64(y), opts, c, s)
			var p [4]byte
			if opts.Antialias {
				p = sourcePixelBilinear(src, sx, sy)
			} else {
				p = sourcePixelNearest(src, sx, sy)
			}
			i := (y*ow + x) * 4
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = p[0], p[1], p[2], p[3]
		}
	}
	return out
}

func maskedSelectionCanvas(img ClipboardImage) *Canvas {
	if !validClipboardImage(img) {
		return nil
	}
	pix := append([]byte(nil), img.Pix...)
	if len(img.Mask) == img.W*img.H {
		for i, allowed := range img.Mask {
			if !allowed {
				off := i * 4
				pix[off], pix[off+1], pix[off+2], pix[off+3] = 0, 0, 0, 0
			}
		}
	}
	return &Canvas{W: img.W, H: img.H, Pix: pix}
}

// selectionTransformSource returns only the visible selection pixels. Free-form
// mask exclusions are converted to alpha zero so every transform can use one
// ordinary BGRA canvas without carrying a second geometry mask.
func (a *App) selectionTransformSource() *Canvas {
	if !a.selection.Active || a.selection.W < 1 || a.selection.H < 1 {
		return nil
	}
	if a.selection.Floating && len(a.selection.Pix) == a.selection.W*a.selection.H*4 {
		img := ClipboardImage{W: a.selection.W, H: a.selection.H, Pix: append([]byte(nil), a.selection.Pix...)}
		if len(a.selection.Mask) == a.selection.W*a.selection.H {
			img.Mask = append([]bool(nil), a.selection.Mask...)
		}
		return maskedSelectionCanvas(img)
	}
	return maskedSelectionCanvas(a.captureSelectionImage())
}

// selectionTransformBase returns the canvas that must remain underneath a
// transformed selection. A normal (not-yet-floating) selection has no stored
// underlying layer, so its old footprint is removed with true transparency.
// This is deliberately different from an ordinary selection move, which uses
// the current background color as the cut-out fill. Rotation/stretch vacancies
// therefore never introduce a color that did not exist in the source image.
func (a *App) selectionTransformBase() []byte {
	s := &a.selection
	if !s.Active {
		return a.canvas.ClonePixels()
	}
	if s.Floating && len(s.BasePix) == len(a.canvas.Pix) {
		return append([]byte(nil), s.BasePix...)
	}
	base := a.canvas.ClonePixels()
	for y := 0; y < s.H; y++ {
		for x := 0; x < s.W; x++ {
			i := y*s.W + x
			if len(s.Mask) == s.W*s.H && !s.Mask[i] {
				continue
			}
			px, py := s.X+x, s.Y+y
			if px < 0 || py < 0 || px >= a.canvas.W || py >= a.canvas.H {
				continue
			}
			off := (py*a.canvas.W + px) * 4
			base[off], base[off+1], base[off+2], base[off+3] = 0, 0, 0, 0
		}
	}
	return base
}

// installSelectionTransform installs an already-rendered transform as the new
// floating selection. anchorX/anchorY are caller-selected: rotation keeps the
// old center, while stretch keeps the old top-left corner.
func (a *App) installSelectionTransform(transformed *Canvas, anchorX, anchorY int, historyName, statusText string) bool {
	if transformed == nil || transformed.W < 1 || transformed.H < 1 || !a.selection.Active {
		return false
	}
	base := a.selectionTransformBase()
	a.undo.Push(a.canvas)
	copy(a.canvas.Pix, base)
	a.selection = SelectionState{
		Active: true, Floating: true,
		X: anchorX, Y: anchorY, W: transformed.W, H: transformed.H,
		Pix: transformed.ClonePixels(), BasePix: append([]byte(nil), base...),
	}
	a.applyFloatingSelection(anchorX, anchorY)
	a.markDirty()
	a.recordHistory(historyName)
	a.statusText = statusText
	a.requestCanvasRepaint(true)
	return true
}

func flipRotateNoop(opts FlipRotateOptions) bool {
	return !opts.FlipHorizontal && !opts.FlipVertical && math.Abs(normalizeRotationAngle(opts.Angle)) < 0.0001
}

func (a *App) applyFlipRotate(source *Canvas, opts FlipRotateOptions, toSelection bool) {
	if source == nil || flipRotateNoop(opts) {
		a.statusText = "대칭 이동 또는 회전 값이 없어 이미지를 변경하지 않았습니다."
		invalidate(a.hwnd)
		return
	}
	transformed := transformCanvasFlipRotate(source, opts)
	if transformed == nil || transformed.W < 1 || transformed.H < 1 {
		messageBox(a.hwnd, "대칭 이동 및 회전 결과를 만들지 못했습니다.", "dotPaint", MB_OK|MB_ICONERROR)
		return
	}
	angle := normalizeRotationAngle(opts.Angle)
	if toSelection && a.selection.Active {
		old := a.selection
		cx := float64(old.X) + float64(old.W)/2
		cy := float64(old.Y) + float64(old.H)/2
		nx := int(math.Round(cx - float64(transformed.W)/2))
		ny := int(math.Round(cy - float64(transformed.H)/2))
		status := fmt.Sprintf("선택 영역을 좌우=%v 상하=%v, 시계 방향 %.1f°로 변환했습니다. 결과 %d × %d. 빈 부분은 투명색입니다.", opts.FlipHorizontal, opts.FlipVertical, angle, transformed.W, transformed.H)
		a.installSelectionTransform(transformed, nx, ny, "선택 대칭/회전", status)
	} else {
		a.clearSelectionState()
		a.undo.Push(a.canvas)
		a.canvas = transformed
		a.markDirty()
		a.recordHistory("대칭 이동/회전")
		a.updateScrollbars()
		a.statusText = fmt.Sprintf("전체 이미지를 좌우=%v 상하=%v, 시계 방향 %.1f°로 변환했습니다. 캔버스를 %d × %d로 자동 확장했습니다.", opts.FlipHorizontal, opts.FlipVertical, angle, transformed.W, transformed.H)
	}
	logf("FLIP_ROTATE apply selection=%v flipH=%v flipV=%v angle=%.1f constraint=%d aa=%v result=%dx%d", toSelection, opts.FlipHorizontal, opts.FlipVertical, angle, opts.Constraint, opts.Antialias, transformed.W, transformed.H)
	invalidate(a.hwnd)
	procUpdateWindow.Call(a.hwnd)
}

func ensureFlipRotateDialogClass(hinst uintptr) bool {
	if flipRotateClassReady {
		return true
	}
	if flipRotateWndProcCB == 0 {
		flipRotateWndProcCB = syscall.NewCallback(flipRotateDialogWndProc)
	}
	className := utf16Ptr(rotateDialogClass)
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         0,
		LpfnWndProc:   flipRotateWndProcCB,
		HInstance:     hinst,
		HCursor:       cursor,
		HbrBackground: 0, // WM_ERASEBKGND is handled explicitly to avoid full-window flashing.
		LpszClassName: className,
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		logf("FLIP_ROTATE RegisterClass failed err=%v", err)
		return false
	}
	flipRotateClassReady = true
	return true
}

func scaled(v int, scale float64) int { return int(math.Round(float64(v) * scale)) }
func unscaled(v int, scale float64) int {
	if scale <= 0 {
		return v
	}
	return int(math.Round(float64(v) / scale))
}

func frRect(l, t, r, b int) RECT { return RECT{int32(l), int32(t), int32(r), int32(b)} }

func (s *flipRotateDialogState) angleDialRect() RECT { return frRect(34, 181, 316, 463) }
func (s *flipRotateDialogState) previewRect() RECT   { return frRect(388, 92, 724, 408) }
func (s *flipRotateDialogState) flipHRect() RECT     { return frRect(33, 60, 168, 105) }
func (s *flipRotateDialogState) flipVRect() RECT     { return frRect(178, 60, 313, 105) }
func (s *flipRotateDialogState) mode15Rect() RECT    { return frRect(430, 470, 512, 500) }
func (s *flipRotateDialogState) modeTanRect() RECT   { return frRect(518, 470, 600, 500) }
func (s *flipRotateDialogState) aaOffRect() RECT     { return frRect(648, 470, 681, 500) }
func (s *flipRotateDialogState) aaOnRect() RECT      { return frRect(687, 470, 720, 500) }
func (s *flipRotateDialogState) resetRect() RECT     { return frRect(31, 519, 133, 556) }
func (s *flipRotateDialogState) okRect() RECT        { return frRect(520, 519, 617, 556) }
func (s *flipRotateDialogState) cancelRect() RECT    { return frRect(627, 519, 724, 556) }

func (s *flipRotateDialogState) updateAngleEdit(selectAll bool) {
	if s.angleEdit == 0 {
		return
	}
	s.updatingEdit = true
	text := fmt.Sprintf("%.1f", normalizeRotationAngle(s.options.Angle))
	procSetWindowTextW.Call(s.angleEdit, uintptr(unsafe.Pointer(utf16Ptr(text))))
	if selectAll {
		procSendMessageW.Call(s.angleEdit, EM_SETSEL, 0, ^uintptr(0))
	}
	s.updatingEdit = false
}

func (s *flipRotateDialogState) parseAngleEdit() bool {
	text := strings.TrimSpace(getWindowTextString(s.angleEdit))
	text = strings.TrimSuffix(text, "°")
	text = strings.ReplaceAll(text, ",", ".")
	v, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return false
	}
	s.options.Angle = normalizeRotationAngle(v)
	return true
}

func (s *flipRotateDialogState) invalidateRectLogical(rc RECT) {
	if s.hwnd == 0 {
		return
	}
	physical := RECT{
		Left:   int32(scaled(int(rc.Left), s.scale)),
		Top:    int32(scaled(int(rc.Top), s.scale)),
		Right:  int32(scaled(int(rc.Right), s.scale)),
		Bottom: int32(scaled(int(rc.Bottom), s.scale)),
	}
	procInvalidateRect.Call(s.hwnd, uintptr(unsafe.Pointer(&physical)), 0)
}

func (s *flipRotateDialogState) invalidateAnglePreview() {
	// Only the dial and the preview/output-size areas change while dragging.
	s.invalidateRectLogical(frRect(20, 164, 326, 481))
	s.invalidateRectLogical(frRect(370, 40, 738, 424))
}

func (s *flipRotateDialogState) invalidatePreviewOnly() {
	s.invalidateRectLogical(frRect(370, 40, 738, 424))
}

func (s *flipRotateDialogState) invalidateControls() {
	s.invalidateRectLogical(frRect(20, 40, 326, 126))
	s.invalidateRectLogical(frRect(338, 425, 730, 505))
}

func (s *flipRotateDialogState) invalidateAll() {
	if s.hwnd != 0 {
		procInvalidateRect.Call(s.hwnd, 0, 0)
	}
}

func (s *flipRotateDialogState) setAngleFromMouse(mx, my int) {
	dial := s.angleDialRect()
	cx := (int(dial.Left) + int(dial.Right)) / 2
	cy := (int(dial.Top) + int(dial.Bottom)) / 2
	dx, dy := float64(mx-cx), float64(my-cy)
	if math.Hypot(dx, dy) < 2 {
		return
	}
	deg := math.Atan2(dy, dx) * 180 / math.Pi
	if keyDown(VK_SHIFT) {
		deg = constrainedRotationAngle(deg, s.options.Constraint)
	}
	newAngle := normalizeRotationAngle(deg)
	if newAngle == s.options.Angle {
		return
	}
	s.options.Angle = newAngle
	s.updateAngleEdit(false)
	s.refreshPreviewCache()
	s.invalidateAnglePreview()
}

func (s *flipRotateDialogState) buildPreviewCanvas(maxW, maxH int) *Canvas {
	if s.source == nil || maxW < 1 || maxH < 1 {
		return nil
	}
	// Build the exact full-resolution result used by the Confirm action. The
	// paint path merely scales this completed raster to fit the preview box, so
	// nearest/AA sampling, output bounds and transparent corners cannot diverge
	// between preview and the applied transform.
	return transformCanvasFlipRotate(s.source, s.options)
}

func (s *flipRotateDialogState) refreshPreviewCache() {
	pr := s.previewRect()
	s.outputW, s.outputH, _, _ = flipRotateOutputSize(s.source.W, s.source.H, s.options.Angle)
	s.preview = s.buildPreviewCanvas(int(pr.Right-pr.Left)-12, int(pr.Bottom-pr.Top)-12)
}

func (s *flipRotateDialogState) disposeBackBuffer() {
	if s.backDC != 0 && s.backOldBitmap != 0 {
		procSelectObject.Call(s.backDC, s.backOldBitmap)
	}
	if s.backBitmap != 0 {
		procDeleteObject.Call(s.backBitmap)
	}
	if s.backDC != 0 {
		procDeleteDC.Call(s.backDC)
	}
	s.backDC, s.backBitmap, s.backOldBitmap = 0, 0, 0
	s.backW, s.backH = 0, 0
}

func (s *flipRotateDialogState) ensureBackBuffer(hdc uintptr, w, h int) bool {
	w, h = max(w, 1), max(h, 1)
	if s.backDC != 0 && s.backBitmap != 0 && s.backW == w && s.backH == h {
		return true
	}
	s.disposeBackBuffer()
	memDC, _, _ := procCreateCompatibleDC.Call(hdc)
	if memDC == 0 {
		return false
	}
	bitmap, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(w), uintptr(h))
	if bitmap == 0 {
		procDeleteDC.Call(memDC)
		return false
	}
	oldBitmap, _, _ := procSelectObject.Call(memDC, bitmap)
	s.backDC, s.backBitmap, s.backOldBitmap = memDC, bitmap, oldBitmap
	s.backW, s.backH = w, h
	return true
}

func paintToggleButton(hdc uintptr, rc RECT, selected bool, text string) {
	if selected {
		fillUIRect(hdc, rc, rgb(205, 230, 255))
		frameUIRect(hdc, rc, rgb(0, 120, 215))
	} else {
		fillUIRect(hdc, rc, rgb(248, 248, 248))
		frameUIRect(hdc, rc, rgb(150, 150, 150))
	}
	drawUIText(hdc, text, rc, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(20, 20, 20))
}

func (s *flipRotateDialogState) paint(hdc uintptr) {
	font := s.uiFont
	if font == 0 {
		font, _, _ = procGetStockObject.Call(DEFAULT_GUI_FONT)
	}
	oldFont, _, _ := procSelectObject.Call(hdc, font)
	defer procSelectObject.Call(hdc, oldFont)
	uiPen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(20, 20, 20))))
	oldUIPen, _, _ := procSelectObject.Call(hdc, uiPen)
	defer func() { procSelectObject.Call(hdc, oldUIPen); procDeleteObject.Call(uiPen) }()
	// Paint the measured client surface, not only the nominal logical layout.
	// Windows 11 may recalculate the non-client dialog frame after the DWM theme
	// is applied.  Leaving any resulting client pixels outside 756 x 585
	// untouched exposed the black compatible-bitmap initial contents at the
	// right and bottom edges in both light and dark modes.
	client := frRect(0, 0, scaled(756, s.scale), scaled(585, s.scale))
	if s.hwnd != 0 {
		procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	}
	fillUIRect(hdc, client, rgb(240, 240, 240))

	// Two clearly separated requested parts.
	drawUIText(hdc, "대칭 이동", frRect(24, 18, 326, 43), DT_LEFT|DT_VCENTER|DT_SINGLELINE, rgb(20, 20, 20))
	frameUIRect(hdc, frRect(20, 40, 326, 126), rgb(175, 175, 175))
	paintToggleButton(hdc, s.flipHRect(), s.options.FlipHorizontal, "좌우 대칭")
	paintToggleButton(hdc, s.flipVRect(), s.options.FlipVertical, "상하 대칭")

	drawUIText(hdc, "회전", frRect(24, 142, 326, 168), DT_LEFT|DT_VCENTER|DT_SINGLELINE, rgb(20, 20, 20))
	frameUIRect(hdc, frRect(20, 164, 326, 481), rgb(175, 175, 175))
	dial := s.angleDialRect()
	cx := (int(dial.Left) + int(dial.Right)) / 2
	cy := (int(dial.Top) + int(dial.Bottom)) / 2
	radius := min(int(dial.Right-dial.Left), int(dial.Bottom-dial.Top))/2 - 18
	dialBrush, _, _ := procCreateSolidBrush.Call(uintptr(uiColor(rgb(255, 255, 255))))
	oldDialBrush := uintptr(0)
	if dialBrush != 0 {
		oldDialBrush, _, _ = procSelectObject.Call(hdc, dialBrush)
	}
	procEllipse.Call(hdc, uintptr(cx-radius), uintptr(cy-radius), uintptr(cx+radius+1), uintptr(cy+radius+1))
	// Crosshair and 15-degree ticks.
	penGray, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(190, 190, 190))))
	oldPen, _, _ := procSelectObject.Call(hdc, penGray)
	procMoveToEx.Call(hdc, uintptr(cx-radius), uintptr(cy), 0)
	procLineTo.Call(hdc, uintptr(cx+radius), uintptr(cy))
	procMoveToEx.Call(hdc, uintptr(cx), uintptr(cy-radius), 0)
	procLineTo.Call(hdc, uintptr(cx), uintptr(cy+radius))
	for i := 0; i < 24; i++ {
		a := float64(i) * math.Pi / 12
		r1 := radius - 7
		r2 := radius
		procMoveToEx.Call(hdc, uintptr(cx+int(math.Round(math.Cos(a)*float64(r1)))), uintptr(cy+int(math.Round(math.Sin(a)*float64(r1)))), 0)
		procLineTo.Call(hdc, uintptr(cx+int(math.Round(math.Cos(a)*float64(r2)))), uintptr(cy+int(math.Round(math.Sin(a)*float64(r2)))))
	}
	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(penGray)
	angleRad := s.options.Angle * math.Pi / 180
	ex := cx + int(math.Round(math.Cos(angleRad)*float64(radius-16)))
	ey := cy + int(math.Round(math.Sin(angleRad)*float64(radius-16)))
	penBlue, _, _ := procCreatePen.Call(PS_SOLID, 3, uintptr(uiColor(rgb(0, 120, 215))))
	oldPen, _, _ = procSelectObject.Call(hdc, penBlue)
	procMoveToEx.Call(hdc, uintptr(cx), uintptr(cy), 0)
	procLineTo.Call(hdc, uintptr(ex), uintptr(ey))
	procEllipse.Call(hdc, uintptr(ex-5), uintptr(ey-5), uintptr(ex+6), uintptr(ey+6))
	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(penBlue)
	if dialBrush != 0 {
		procSelectObject.Call(hdc, oldDialBrush)
		procDeleteObject.Call(dialBrush)
	}
	drawUIText(hdc, fmt.Sprintf("시계 방향 +   %.1f°", s.options.Angle), frRect(63, 442, 284, 468), DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(20, 20, 20))

	drawUIText(hdc, "실시간 미리 보기", frRect(374, 18, 724, 43), DT_LEFT|DT_VCENTER|DT_SINGLELINE, rgb(20, 20, 20))
	frameUIRect(hdc, frRect(370, 40, 738, 424), rgb(175, 175, 175))
	drawUIText(hdc, s.targetName, frRect(386, 52, 722, 78), DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(65, 65, 65))
	pr := s.previewRect()
	drawCheckerboard(hdc, pr, 10)
	frameUIRect(hdc, pr, rgb(145, 145, 145))
	pc := s.preview
	if pc != nil && len(pc.Pix) > 0 {
		disp := pc.displayPixels(1)
		availW := max(1, int(pr.Right-pr.Left)-12)
		availH := max(1, int(pr.Bottom-pr.Top)-12)
		scale := math.Min(float64(availW)/float64(pc.W), float64(availH)/float64(pc.H))
		if scale > 1 {
			scale = 1
		}
		dw := max(1, int(math.Round(float64(pc.W)*scale)))
		dh := max(1, int(math.Round(float64(pc.H)*scale)))
		dx := int(pr.Left) + (int(pr.Right-pr.Left)-dw)/2
		dy := int(pr.Top) + (int(pr.Bottom-pr.Top)-dh)/2
		bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(pc.W), BiHeight: -int32(pc.H), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
		procSetStretchBltMode.Call(hdc, COLORONCOLOR)
		procStretchDIBits.Call(hdc, uintptr(dx), uintptr(dy), uintptr(dw), uintptr(dh), 0, 0, uintptr(pc.W), uintptr(pc.H), uintptr(unsafe.Pointer(&disp[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, SRCCOPY)
	}
	ow, oh := s.outputW, s.outputH
	if ow < 1 || oh < 1 {
		ow, oh, _, _ = flipRotateOutputSize(s.source.W, s.source.H, s.options.Angle)
	}
	drawUIText(hdc, fmt.Sprintf("결과 크기: %d × %d  (잘림 방지 자동 확장)", ow, oh), frRect(382, 408, 726, 423), DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(70, 70, 70))

	drawUIText(hdc, "각도", frRect(338, 427, 400, 447), DT_LEFT|DT_VCENTER|DT_SINGLELINE, rgb(20, 20, 20))
	drawUIText(hdc, "°", frRect(466, 427, 486, 447), DT_LEFT|DT_VCENTER|DT_SINGLELINE, rgb(20, 20, 20))
	// Keep a clear vertical gap between the compact numeric field and the Shift row.
	drawUIText(hdc, "Shift 제약", frRect(338, 470, 424, 500), DT_LEFT|DT_VCENTER|DT_SINGLELINE, rgb(20, 20, 20))
	paintToggleButton(hdc, s.mode15Rect(), s.options.Constraint == LineConstraint15Degrees, "15×n°")
	paintToggleButton(hdc, s.modeTanRect(), s.options.Constraint == LineConstraintTanN, "tan n")
	drawUIText(hdc, "안티에일리어싱", frRect(636, 450, 730, 470), DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(20, 20, 20))
	paintToggleButton(hdc, s.aaOffRect(), !s.options.Antialias, "끔")
	paintToggleButton(hdc, s.aaOnRect(), s.options.Antialias, "켬")

	paintToggleButton(hdc, s.resetRect(), false, "초기화")
	paintToggleButton(hdc, s.okRect(), true, "확인")
	paintToggleButton(hdc, s.cancelRect(), false, "취소")
}

func pointInFR(x, y int, r RECT) bool {
	return x >= int(r.Left) && x < int(r.Right) && y >= int(r.Top) && y < int(r.Bottom)
}

func flipRotateDialogWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	s := flipRotateDialog
	defer func() {
		if r := recover(); r != nil {
			logPanic("flipRotateDialogWndProc", r)
		}
	}()
	switch msg {
	case WM_CREATE:
		if s == nil {
			break
		}
		s.hwnd = hwnd
		style := uintptr(WS_CHILD | WS_VISIBLE | WS_BORDER | WS_TABSTOP | ES_CENTER | ES_AUTOHSCROLL)
		s.angleEdit, _, _ = procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16Ptr("EDIT"))), uintptr(unsafe.Pointer(utf16Ptr("0.0"))), style, uintptr(scaled(408, s.scale)), uintptr(scaled(427, s.scale)), uintptr(scaled(52, s.scale)), uintptr(scaled(20, s.scale)), hwnd, rotateAngleEditID, app.hinst, 0)
		applyDarkThemeToWindow(s.angleEdit, darkModeEnabled())
		setCompactEditorControlFont(s.angleEdit, s.uiFont)
		procSendMessageW.Call(s.angleEdit, EM_SETMARGINS, EC_LEFTMARGIN|EC_RIGHTMARGIN, 0)
		s.updateAngleEdit(true)
		s.refreshPreviewCache()
		return 0
	case WM_CTLCOLOREDIT:
		if darkModeEnabled() && wparam != 0 && app != nil {
			procSetTextColor.Call(wparam, uintptr(rgb(235, 235, 235)))
			procSetBkColor.Call(wparam, uintptr(rgb(30, 30, 30)))
			if app.darkBrush == 0 {
				app.darkBrush, _, _ = procCreateSolidBrush.Call(uintptr(rgb(30, 30, 30)))
			}
			return app.darkBrush
		}
	case WM_ERASEBKGND:
		// The entire client surface is committed atomically from the back buffer.
		// Claim the erase so DefWindowProc never flashes COLOR_BTNFACE first.
		return 1
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if s != nil && hdc != 0 {
			var client RECT
			procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
			w := int(client.Right - client.Left)
			h := int(client.Bottom - client.Top)
			if s.ensureBackBuffer(hdc, w, h) {
				s.paint(s.backDC)
				rc := ps.RcPaint
				if rc.Right <= rc.Left || rc.Bottom <= rc.Top {
					rc = client
				}
				procBitBlt.Call(hdc, uintptr(rc.Left), uintptr(rc.Top), uintptr(rc.Right-rc.Left), uintptr(rc.Bottom-rc.Top), s.backDC, uintptr(rc.Left), uintptr(rc.Top), SRCCOPY)
			} else {
				s.paint(hdc)
			}
		}
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case WM_COMMAND:
		if s != nil && loWord(wparam) == rotateAngleEditID && hiWord(wparam) == EN_CHANGE && !s.updatingEdit {
			if s.parseAngleEdit() {
				s.refreshPreviewCache()
				s.invalidateAnglePreview()
			}
			return 0
		}
	case WM_LBUTTONDOWN:
		if s == nil {
			break
		}
		mx := unscaled(int(int16(loWord(lparam))), s.scale)
		my := unscaled(int(int16(hiWord(lparam))), s.scale)
		switch {
		case pointInFR(mx, my, s.flipHRect()):
			s.options.FlipHorizontal = !s.options.FlipHorizontal
			s.refreshPreviewCache()
			s.invalidateRectLogical(s.flipHRect())
			s.invalidatePreviewOnly()
		case pointInFR(mx, my, s.flipVRect()):
			s.options.FlipVertical = !s.options.FlipVertical
			s.refreshPreviewCache()
			s.invalidateRectLogical(s.flipVRect())
			s.invalidatePreviewOnly()
		case pointInFR(mx, my, s.mode15Rect()):
			s.options.Constraint = LineConstraint15Degrees
			s.invalidateRectLogical(s.mode15Rect())
			s.invalidateRectLogical(s.modeTanRect())
		case pointInFR(mx, my, s.modeTanRect()):
			s.options.Constraint = LineConstraintTanN
			s.invalidateRectLogical(s.mode15Rect())
			s.invalidateRectLogical(s.modeTanRect())
		case pointInFR(mx, my, s.aaOffRect()):
			s.options.Antialias = false
			s.refreshPreviewCache()
			s.invalidateRectLogical(s.aaOffRect())
			s.invalidateRectLogical(s.aaOnRect())
			s.invalidatePreviewOnly()
		case pointInFR(mx, my, s.aaOnRect()):
			s.options.Antialias = true
			s.refreshPreviewCache()
			s.invalidateRectLogical(s.aaOffRect())
			s.invalidateRectLogical(s.aaOnRect())
			s.invalidatePreviewOnly()
		case pointInFR(mx, my, s.resetRect()):
			s.options = FlipRotateOptions{Constraint: LineConstraint15Degrees}
			s.updateAngleEdit(true)
			s.refreshPreviewCache()
			s.invalidateControls()
			s.invalidateAnglePreview()
		case pointInFR(mx, my, s.okRect()):
			if !s.parseAngleEdit() {
				messageBox(hwnd, "각도를 숫자로 입력하십시오. 소수점 첫째 자리까지 사용할 수 있습니다.", "dotPaint", MB_OK|MB_ICONWARNING)
				s.updateAngleEdit(true)
				return 0
			}
			s.accepted = true
			s.done = true
			procDestroyWindow.Call(hwnd)
		case pointInFR(mx, my, s.cancelRect()):
			s.accepted = false
			s.done = true
			procDestroyWindow.Call(hwnd)
		case pointInFR(mx, my, s.angleDialRect()):
			s.dragging = true
			procSetCapture.Call(hwnd)
			s.setAngleFromMouse(mx, my)
		}
		return 0
	case WM_MOUSEMOVE:
		if s != nil && s.dragging && (wparam&MK_LBUTTON) != 0 {
			mx := unscaled(int(int16(loWord(lparam))), s.scale)
			my := unscaled(int(int16(hiWord(lparam))), s.scale)
			s.setAngleFromMouse(mx, my)
			return 0
		}
	case WM_LBUTTONUP:
		if s != nil && s.dragging {
			s.dragging = false
			procReleaseCapture.Call()
			return 0
		}
	case WM_KEYDOWN:
		if s != nil {
			if int(wparam) == VK_ESCAPE {
				s.accepted = false
				s.done = true
				procDestroyWindow.Call(hwnd)
				return 0
			}
			if int(wparam) == 0x0D { // Enter
				if !s.parseAngleEdit() {
					messageBox(hwnd, "각도를 숫자로 입력하십시오.", "dotPaint", MB_OK|MB_ICONWARNING)
					s.updateAngleEdit(true)
					return 0
				}
				s.accepted = true
				s.done = true
				procDestroyWindow.Call(hwnd)
				return 0
			}
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
			s.disposeBackBuffer()
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

// fitFlipRotateDialogClientSize corrects the outer window after Windows has
// applied the monitor DPI, native theme and dialog frame. AdjustWindowRectEx is
// still used for the initial creation, but the measured client rectangle is
// authoritative. Two passes cover the one additional frame recalculation that
// some Windows 11 builds perform after SWP_FRAMECHANGED.
func fitFlipRotateDialogClientSize(hwnd uintptr, targetW, targetH int) (int, int) {
	if hwnd == 0 || targetW < 1 || targetH < 1 {
		return 0, 0
	}
	for pass := 0; pass < 2; pass++ {
		var window, client RECT
		if ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&window))); ok == 0 {
			break
		}
		if ok, _, _ := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client))); ok == 0 {
			break
		}
		clientW := int(client.Right - client.Left)
		clientH := int(client.Bottom - client.Top)
		if clientW == targetW && clientH == targetH {
			return int(window.Right - window.Left), int(window.Bottom - window.Top)
		}
		outerW := max(1, int(window.Right-window.Left)+(targetW-clientW))
		outerH := max(1, int(window.Bottom-window.Top)+(targetH-clientH))
		procSetWindowPos.Call(hwnd, 0, 0, 0, uintptr(outerW), uintptr(outerH),
			SWP_NOMOVE|SWP_NOZORDER|SWP_NOACTIVATE|SWP_FRAMECHANGED)
	}
	var window RECT
	if ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&window))); ok != 0 {
		return int(window.Right - window.Left), int(window.Bottom - window.Top)
	}
	return 0, 0
}

func (a *App) showFlipRotateDialog() {
	if !ensureFlipRotateDialogClass(a.hinst) {
		messageBox(a.hwnd, "대칭 이동 및 회전 창을 만들지 못했습니다.", "dotPaint", MB_OK|MB_ICONERROR)
		return
	}
	toSelection := a.selection.Active
	var source *Canvas
	targetName := "대상: 전체 이미지"
	if toSelection {
		source = a.selectionTransformSource()
		targetName = fmt.Sprintf("대상: 선택 영역 %d × %d", a.selection.W, a.selection.H)
	} else {
		source = &Canvas{W: a.canvas.W, H: a.canvas.H, Pix: a.canvas.ClonePixels()}
	}
	if source == nil {
		return
	}
	dpi := 96
	if r, _, _ := procGetDpiForWindow.Call(a.hwnd); r >= 96 {
		dpi = int(r)
	}
	// dotPaint's custom chrome is already rendered in physical pixels. Keep the
	// dialog coordinate system identical so child controls and hit testing never
	// drift apart on high-DPI displays.
	scale := 1.0
	state := &flipRotateDialogState{parent: a.hwnd, source: source, targetName: targetName, options: FlipRotateOptions{Constraint: flipRotateConstraintPreference, Antialias: flipRotateAntialiasPreference}, scale: scale, uiFont: createCompactEditorUIFont()}
	state.refreshPreviewCache()
	flipRotateDialog = state
	style := uintptr(WS_CAPTION | WS_SYSMENU)
	exStyle := uintptr(WS_EX_DLGMODALFRAME)
	clientBounds := RECT{0, 0, int32(scaled(756, scale)), int32(scaled(585, scale))}
	procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&clientBounds)), style, 0, exStyle)
	w := int(clientBounds.Right - clientBounds.Left)
	h := int(clientBounds.Bottom - clientBounds.Top)
	hwnd, _, err := procCreateWindowExW.Call(exStyle, uintptr(unsafe.Pointer(utf16Ptr(rotateDialogClass))), uintptr(unsafe.Pointer(utf16Ptr("대칭 이동 및 회전"))), style, uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), uintptr(w), uintptr(h), a.hwnd, 0, a.hinst, 0)
	if hwnd == 0 {
		if state.uiFont != 0 {
			procDeleteObject.Call(state.uiFont)
			state.uiFont = 0
		}
		flipRotateDialog = nil
		logf("FLIP_ROTATE CreateWindow failed err=%v", err)
		messageBox(a.hwnd, "대칭 이동 및 회전 창을 만들지 못했습니다.", "dotPaint", MB_OK|MB_ICONERROR)
		return
	}
	state.hwnd = hwnd
	applyDarkThemeToWindow(hwnd, a.darkMode)
	if fittedW, fittedH := fitFlipRotateDialogClientSize(hwnd, scaled(756, scale), scaled(585, scale)); fittedW > 0 && fittedH > 0 {
		w, h = fittedW, fittedH
	}
	var pr RECT
	procGetWindowRect.Call(a.hwnd, uintptr(unsafe.Pointer(&pr)))
	x := int(pr.Left) + (int(pr.Right-pr.Left)-w)/2
	y := int(pr.Top) + (int(pr.Bottom-pr.Top)-h)/2
	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), SWP_NOZORDER|SWP_NOACTIVATE)
	procEnableWindow.Call(a.hwnd, 0)
	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)
	procSetFocus.Call(state.angleEdit)
	var finalClient RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&finalClient)))
	logf("FLIP_ROTATE dialog open selection=%v source=%dx%d dpi=%d client=%dx%d outer=%dx%d", toSelection, source.W, source.H, dpi,
		int(finalClient.Right-finalClient.Left), int(finalClient.Bottom-finalClient.Top), w, h)
	var msg MSG
	for !state.done {
		r, _, getErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			if int32(r) < 0 {
				logf("FLIP_ROTATE GetMessage failed err=%v", getErr)
			}
			break
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
		flipRotateConstraintPreference = opts.Constraint
		flipRotateAntialiasPreference = opts.Antialias
	}
	if state.hwnd != 0 {
		procDestroyWindow.Call(state.hwnd)
	}
	flipRotateDialog = nil
	if accepted {
		a.applyFlipRotate(source, opts, toSelection)
	} else {
		a.statusText = "대칭 이동 및 회전을 취소했습니다."
		invalidate(a.hwnd)
		logf("FLIP_ROTATE dialog cancel")
	}
}
