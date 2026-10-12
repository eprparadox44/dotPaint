//go:build windows

package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"
	"time"
	"unsafe"
)

// A182 keeps the four-cell cursor artwork supplied by the user. The source PNG
// is embedded into the executable, so the program remains a single
// self-contained file. The magenta frame is discarded. FF0000 marks the hot
// spot but becomes transparent; 0000FF marks the hot spot and becomes an
// actual black pixel.
//
// A183 additionally embeds the user-supplied eyedropper icon/cursor sheet. The
// cursor half has a red marker pixel that is not visible in the final cursor;
// it instead becomes the real working tip colour 3188F2 and also marks the hot
// spot.
//
//go:embed cursor_reference_mouse.png
var a182CursorReferencePNG []byte

//go:embed eyedropper_reference.png
var a183EyedropperReferencePNG []byte

var (
	procCreateCursorA181       = modUser32.NewProc("CreateCursor")
	procDestroyCursorA181      = modUser32.NewProc("DestroyCursor")
	procCreateIconIndirectA182 = modUser32.NewProc("CreateIconIndirect")
	procCreateBitmapA182       = modGdi32.NewProc("CreateBitmap")

	a181ToolCursors      map[Tool]uintptr
	a181CursorHandles    []uintptr
	a181MarqueeCursor    uintptr
	a181LassoCursor      uintptr
	a181MagicWandCursor  uintptr
	a181EraserCursor     uintptr
	a181FillCursor       uintptr
	a181EyedropperCursor uintptr
	a181MagnifierCursor  uintptr
	a181BrushCursor      uintptr

	a182SelectionCursor uintptr
	a182ColourCursor    uintptr
	a182FillCursor      uintptr
	a182CrossCursor     uintptr
	a182InvisibleCursor uintptr

	a217AdaptiveCursorPairs map[uintptr]cursorContrastPairA217
)

const (
	cursorTransparent uint8 = iota
	cursorBlack
	cursorWhite
)

type monoCursorCanvas struct {
	w, h int
	pix  []uint8
}

type iconInfoA182 struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

type cursorSpriteA182 struct {
	W, H       int
	HotX, HotY int
	Pix        []byte // tightly packed RGBA
}

type cursorContrastPairA217 struct {
	Light uintptr
	Dark  uintptr
}

func absCursorInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func newMonoCursorCanvas(w, h int) *monoCursorCanvas {
	return &monoCursorCanvas{w: w, h: h, pix: make([]uint8, w*h)}
}

func (c *monoCursorCanvas) set(x, y int, value uint8) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	c.pix[y*c.w+x] = value
}

func (c *monoCursorCanvas) line(x0, y0, x1, y1 int, value uint8) {
	dx := absCursorInt(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -absCursorInt(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		c.set(x0, y0, value)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func (c *monoCursorCanvas) thickLine(x0, y0, x1, y1, radius int, value uint8) {
	dx := absCursorInt(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -absCursorInt(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		for oy := -radius; oy <= radius; oy++ {
			for ox := -radius; ox <= radius; ox++ {
				if ox*ox+oy*oy <= radius*radius+radius {
					c.set(x0+ox, y0+oy, value)
				}
			}
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func pointInCursorPolygon(x, y int, pts []PixelPoint) bool {
	inside := false
	j := len(pts) - 1
	for i := 0; i < len(pts); i++ {
		xi, yi := pts[i].X, pts[i].Y
		xj, yj := pts[j].X, pts[j].Y
		if (yi > y) != (yj > y) {
			crossX := float64(xj-xi)*float64(y-yi)/float64(yj-yi) + float64(xi)
			if float64(x) < crossX {
				inside = !inside
			}
		}
		j = i
	}
	return inside
}

func (c *monoCursorCanvas) fillPolygon(pts []PixelPoint, value uint8) {
	if len(pts) < 3 {
		return
	}
	left, right := pts[0].X, pts[0].X
	top, bottom := pts[0].Y, pts[0].Y
	for _, p := range pts[1:] {
		left = min(left, p.X)
		right = max(right, p.X)
		top = min(top, p.Y)
		bottom = max(bottom, p.Y)
	}
	for y := top; y <= bottom; y++ {
		for x := left; x <= right; x++ {
			if pointInCursorPolygon(x, y, pts) {
				c.set(x, y, value)
			}
		}
	}
}

func (c *monoCursorCanvas) outlinePolygon(pts []PixelPoint, value uint8) {
	if len(pts) < 2 {
		return
	}
	for i := range pts {
		j := (i + 1) % len(pts)
		c.line(pts[i].X, pts[i].Y, pts[j].X, pts[j].Y, value)
	}
}

func (c *monoCursorCanvas) circle(cx, cy, r int, value uint8) {
	x, y := r, 0
	err := 1 - r
	for x >= y {
		for _, p := range [][2]int{{cx + x, cy + y}, {cx + y, cy + x}, {cx - y, cy + x}, {cx - x, cy + y}, {cx - x, cy - y}, {cx - y, cy - x}, {cx + y, cy - x}, {cx + x, cy - y}} {
			c.set(p[0], p[1], value)
		}
		y++
		if err < 0 {
			err += 2*y + 1
		} else {
			x--
			err += 2*(y-x) + 1
		}
	}
}

func (c *monoCursorCanvas) cursorMasks() (andMask, xorMask []byte) {
	rowBytes := (c.w + 7) / 8
	andMask = make([]byte, rowBytes*c.h)
	xorMask = make([]byte, rowBytes*c.h)
	for i := range andMask {
		andMask[i] = 0xFF
	}
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			v := c.pix[y*c.w+x]
			if v == cursorTransparent {
				continue
			}
			idx := y*rowBytes + x/8
			bit := byte(0x80 >> uint(x&7))
			andMask[idx] &^= bit
			if v == cursorWhite {
				xorMask[idx] |= bit
			}
		}
	}
	return andMask, xorMask
}

func createMonoCursor(hinst uintptr, c *monoCursorCanvas, hotX, hotY int, name string) uintptr {
	andMask, xorMask := c.cursorMasks()
	if len(andMask) == 0 || len(xorMask) == 0 {
		return 0
	}
	h, _, err := procCreateCursorA181.Call(
		hinst,
		uintptr(hotX), uintptr(hotY),
		uintptr(c.w), uintptr(c.h),
		uintptr(unsafe.Pointer(&andMask[0])),
		uintptr(unsafe.Pointer(&xorMask[0])),
	)
	if h == 0 {
		logf("CURSOR create failed name=%q hotspot=%d,%d err=%v", name, hotX, hotY, err)
		return 0
	}
	a181CursorHandles = append(a181CursorHandles, h)
	logf("CURSOR create ok name=%q handle=%#x hotspot=%d,%d", name, h, hotX, hotY)
	return h
}

func extractCursorSpriteA182(src image.Image, cell int) (cursorSpriteA182, bool) {
	// The supplied sheet consists of four 48 px cells.  Two magenta top/bottom
	// rows and the vertical magenta separators are framing only.  Each usable
	// cell therefore contains an exact 46 x 41 cursor design.
	if cell < 0 || cell > 3 || src == nil {
		return cursorSpriteA182{}, false
	}
	b := src.Bounds()
	if b.Dx() < 193 || b.Dy() < 45 {
		logf("CURSOR reference invalid size=%dx%d", b.Dx(), b.Dy())
		return cursorSpriteA182{}, false
	}
	x0 := []int{1, 49, 97, 145}[cell]
	y0, w, h := 2, 46, 41
	out := cursorSpriteA182{W: w, H: h, HotX: -1, HotY: -1, Pix: make([]byte, w*h*4)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r16, g16, b16, a16 := src.At(b.Min.X+x0+x, b.Min.Y+y0+y).RGBA()
			r, g, bl, a := byte(r16>>8), byte(g16>>8), byte(b16>>8), byte(a16>>8)
			// FF0000: hot spot marker only, not cursor artwork.
			if a != 0 && r == 0xFF && g == 0x00 && bl == 0x00 {
				out.HotX, out.HotY = x, y
				r, g, bl, a = 0, 0, 0, 0
			}
			// 0000FF: hot spot marker that is a real black design pixel.
			if a != 0 && r == 0x00 && g == 0x00 && bl == 0xFF {
				out.HotX, out.HotY = x, y
				r, g, bl, a = 0, 0, 0, 0xFF
			}
			// The exact magenta frame is never part of the cursor.
			if a != 0 && r == 0xFF && g == 0x00 && bl == 0xFF {
				r, g, bl, a = 0, 0, 0, 0
			}
			i := (y*w + x) * 4
			out.Pix[i+0], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = r, g, bl, a
		}
	}
	if out.HotX < 0 || out.HotY < 0 {
		logf("CURSOR reference cell=%d missing hotspot marker", cell+1)
		return cursorSpriteA182{}, false
	}
	return out, true
}

func createColorCursorA182(sprite cursorSpriteA182, name string) uintptr {
	if sprite.W <= 0 || sprite.H <= 0 || len(sprite.Pix) != sprite.W*sprite.H*4 {
		return 0
	}
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{
		BiSize:        uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
		BiWidth:       int32(sprite.W),
		BiHeight:      -int32(sprite.H),
		BiPlanes:      1,
		BiBitCount:    32,
		BiCompression: BI_RGB,
	}}
	var bits unsafe.Pointer
	hbmColor, _, err := procCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbmColor == 0 || bits == nil {
		logf("CURSOR color bitmap failed name=%q err=%v", name, err)
		return 0
	}
	bgra := unsafe.Slice((*byte)(bits), sprite.W*sprite.H*4)
	for i := 0; i < sprite.W*sprite.H; i++ {
		r := sprite.Pix[i*4+0]
		g := sprite.Pix[i*4+1]
		b := sprite.Pix[i*4+2]
		a := sprite.Pix[i*4+3]
		// DIB sections use BGRA.  The supplied artwork is fully opaque or fully
		// transparent, so premultiplication leaves visible RGB unchanged.
		bgra[i*4+0] = byte(uint16(b) * uint16(a) / 255)
		bgra[i*4+1] = byte(uint16(g) * uint16(a) / 255)
		bgra[i*4+2] = byte(uint16(r) * uint16(a) / 255)
		bgra[i*4+3] = a
	}
	maskRowBytes := ((sprite.W + 15) / 16) * 2
	maskBits := make([]byte, maskRowBytes*sprite.H)
	// A 1 bit in the AND mask is transparent on non-alpha fallbacks.  The bit
	// order used by monochrome DDB scanlines is most-significant bit first.
	for y := 0; y < sprite.H; y++ {
		for x := 0; x < sprite.W; x++ {
			if sprite.Pix[(y*sprite.W+x)*4+3] != 0 {
				continue
			}
			row := sprite.H - 1 - y
			maskBits[row*maskRowBytes+x/8] |= byte(0x80 >> uint(x&7))
		}
	}
	hbmMask, _, maskErr := procCreateBitmapA182.Call(uintptr(sprite.W), uintptr(sprite.H), 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))
	if hbmMask == 0 {
		procDeleteObject.Call(hbmColor)
		logf("CURSOR mask bitmap failed name=%q err=%v", name, maskErr)
		return 0
	}
	info := iconInfoA182{FIcon: 0, XHotspot: uint32(sprite.HotX), YHotspot: uint32(sprite.HotY), HbmMask: hbmMask, HbmColor: hbmColor}
	h, _, createErr := procCreateIconIndirectA182.Call(uintptr(unsafe.Pointer(&info)))
	procDeleteObject.Call(hbmMask)
	procDeleteObject.Call(hbmColor)
	if h == 0 {
		logf("CURSOR colour create failed name=%q hotspot=%d,%d err=%v", name, sprite.HotX, sprite.HotY, createErr)
		return 0
	}
	a181CursorHandles = append(a181CursorHandles, h)
	logf("CURSOR colour create ok name=%q handle=%#x size=%dx%d hotspot=%d,%d", name, h, sprite.W, sprite.H, sprite.HotX, sprite.HotY)
	return h
}

func contrastCursorSpriteA217(src cursorSpriteA182, wantLight bool) cursorSpriteA182 {
	out := cursorSpriteA182{W: src.W, H: src.H, HotX: src.HotX, HotY: src.HotY, Pix: append([]byte(nil), src.Pix...)}
	for i := 0; i+3 < len(out.Pix); i += 4 {
		if out.Pix[i+3] == 0 {
			continue
		}
		out.Pix[i+0], out.Pix[i+1], out.Pix[i+2] = cursorContrastPixel(out.Pix[i+0], out.Pix[i+1], out.Pix[i+2], wantLight)
	}
	return out
}

func registerAdaptiveCursorPairA217(base, light, dark uintptr) {
	if base == 0 {
		return
	}
	if light == 0 {
		light = base
	}
	if dark == 0 {
		dark = base
	}
	if a217AdaptiveCursorPairs == nil {
		a217AdaptiveCursorPairs = make(map[uintptr]cursorContrastPairA217)
	}
	a217AdaptiveCursorPairs[base] = cursorContrastPairA217{Light: light, Dark: dark}
}

func createAdaptiveColorCursorA217(sprite cursorSpriteA182, name string) uintptr {
	base := createColorCursorA182(sprite, name)
	if base == 0 {
		return 0
	}
	light := createColorCursorA182(contrastCursorSpriteA217(sprite, true), name+"-contrast-light")
	dark := createColorCursorA182(contrastCursorSpriteA217(sprite, false), name+"-contrast-dark")
	registerAdaptiveCursorPairA217(base, light, dark)
	return base
}

func contrastMonoCursorCanvasA217(src *monoCursorCanvas, wantLight bool) *monoCursorCanvas {
	if src == nil {
		return nil
	}
	out := newMonoCursorCanvas(src.w, src.h)
	value := uint8(cursorBlack)
	if wantLight {
		value = cursorWhite
	}
	for i, pixel := range src.pix {
		if pixel != cursorTransparent {
			out.pix[i] = value
		}
	}
	return out
}

func createAdaptiveMonoCursorA217(hinst uintptr, canvas *monoCursorCanvas, hotX, hotY int, name string) uintptr {
	base := createMonoCursor(hinst, canvas, hotX, hotY, name)
	if base == 0 {
		return 0
	}
	light := createMonoCursor(hinst, contrastMonoCursorCanvasA217(canvas, true), hotX, hotY, name+"-contrast-light")
	dark := createMonoCursor(hinst, contrastMonoCursorCanvasA217(canvas, false), hotX, hotY, name+"-contrast-dark")
	registerAdaptiveCursorPairA217(base, light, dark)
	return base
}

func createInvisibleCursorA182(hinst uintptr) uintptr {
	c := newMonoCursorCanvas(32, 32)
	return createMonoCursor(hinst, c, 0, 0, "invisible-eraser")
}

// The eyedropper and magnifier were not replaced by the supplied four-cell
// sheet, so A181's compact monochrome designs remain for those two tools.
func buildEyedropperCursor() (*monoCursorCanvas, int, int) {
	c := newMonoCursorCanvas(32, 32)
	body := []PixelPoint{{18, 5}, {26, 13}, {21, 18}, {19, 16}, {9, 26}, {5, 26}, {5, 22}, {15, 12}, {13, 10}}
	c.fillPolygon(body, cursorWhite)
	c.outlinePolygon(body, cursorBlack)
	c.line(18, 5, 22, 1, cursorBlack)
	c.line(22, 1, 30, 9, cursorBlack)
	c.line(30, 9, 26, 13, cursorBlack)
	c.line(5, 26, 3, 29, cursorBlack)
	c.set(3, 29, cursorBlack)
	return c, 3, 29
}

func buildMagnifierCursor() (*monoCursorCanvas, int, int) {
	c := newMonoCursorCanvas(32, 32)
	c.circle(12, 12, 9, cursorBlack)
	c.circle(12, 12, 7, cursorWhite)
	c.thickLine(18, 18, 28, 28, 2, cursorBlack)
	c.thickLine(19, 19, 27, 27, 1, cursorWhite)
	c.line(8, 12, 16, 12, cursorWhite)
	c.line(12, 8, 12, 16, cursorWhite)
	c.set(12, 12, cursorBlack)
	return c, 12, 12
}

func extractEyedropperCursorSpriteA183(src image.Image) (cursorSpriteA182, bool) {
	if src == nil {
		return cursorSpriteA182{}, false
	}
	b := src.Bounds()
	if b.Dx() < 74 || b.Dy() < 33 {
		logf("CURSOR eyedropper reference invalid size=%dx%d", b.Dx(), b.Dy())
		return cursorSpriteA182{}, false
	}
	// The supplied A183 sheet uses the left 41x37 area as the toolbox icon and
	// the right transparent 19x30 area as the actual canvas cursor.
	x0, y0, w, h := 55, 3, 19, 30
	out := cursorSpriteA182{W: w, H: h, HotX: -1, HotY: -1, Pix: make([]byte, w*h*4)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r16, g16, b16, a16 := src.At(b.Min.X+x0+x, b.Min.Y+y0+y).RGBA()
			r, g, bl, a := byte(r16>>8), byte(g16>>8), byte(b16>>8), byte(a16>>8)
			if a != 0 && r == 0xFF && g == 0x00 && bl == 0x00 {
				out.HotX, out.HotY = x, y
				r, g, bl, a = 0x31, 0x88, 0xF2, 0xFF
			}
			i := (y*w + x) * 4
			out.Pix[i+0], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = r, g, bl, a
		}
	}
	if out.HotX < 0 || out.HotY < 0 {
		logf("CURSOR eyedropper reference missing hotspot marker")
		return cursorSpriteA182{}, false
	}
	return out, true
}

func initToolCursors(hinst uintptr) {
	destroyToolCursors()
	a181ToolCursors = make(map[Tool]uintptr)
	a217AdaptiveCursorPairs = make(map[uintptr]cursorContrastPairA217)

	decoded, err := png.Decode(bytes.NewReader(a182CursorReferencePNG))
	if err != nil {
		logf("CURSOR reference decode failed err=%v", err)
	} else {
		if s, ok := extractCursorSpriteA182(decoded, 0); ok {
			a182SelectionCursor = createAdaptiveColorCursorA217(s, "user-selection-text-area")
		}
		if s, ok := extractCursorSpriteA182(decoded, 1); ok {
			a182ColourCursor = createAdaptiveColorCursorA217(s, "user-colour-selection")
		}
		if s, ok := extractCursorSpriteA182(decoded, 2); ok {
			a182FillCursor = createAdaptiveColorCursorA217(s, "user-fill")
		}
		if s, ok := extractCursorSpriteA182(decoded, 3); ok {
			a182CrossCursor = createAdaptiveColorCursorA217(s, "user-brush-shapes-cross")
		}
	}
	a182InvisibleCursor = createInvisibleCursorA182(hinst)
	decodedEyedropper, err := png.Decode(bytes.NewReader(a183EyedropperReferencePNG))
	if err != nil {
		logf("CURSOR eyedropper reference decode failed err=%v", err)
	} else if s, ok := extractEyedropperCursorSpriteA183(decodedEyedropper); ok {
		a181EyedropperCursor = createColorCursorA182(s, "eyedropper")
	}
	if a181EyedropperCursor == 0 {
		if c, hx, hy := buildEyedropperCursor(); c != nil {
			a181EyedropperCursor = createMonoCursor(hinst, c, hx, hy, "eyedropper-fallback")
		}
	}
	if c, hx, hy := buildMagnifierCursor(); c != nil {
		a181MagnifierCursor = createAdaptiveMonoCursorA217(hinst, c, hx, hy, "magnifier")
	}

	a181LassoCursor = a182SelectionCursor
	a181MarqueeCursor = a182SelectionCursor
	a181MagicWandCursor = a182ColourCursor
	a181FillCursor = a182FillCursor
	a181EraserCursor = a182InvisibleCursor
	a181BrushCursor = a182CrossCursor

	a181ToolCursors[ToolFreeSelect] = a182SelectionCursor
	a181ToolCursors[ToolRectSelect] = a182SelectionCursor
	a181ToolCursors[ToolColorRegionSelect] = a182ColourCursor
	a181ToolCursors[ToolColorAllSelect] = a182ColourCursor
	a181ToolCursors[ToolEraser] = a182InvisibleCursor
	a181ToolCursors[ToolFill] = a182FillCursor
	a181ToolCursors[ToolEyedropper] = a181EyedropperCursor
	a181ToolCursors[ToolMagnifier] = a181MagnifierCursor
	a181ToolCursors[ToolPencil] = a182CrossCursor
	a181ToolCursors[ToolBrush] = a182CrossCursor
	a181ToolCursors[ToolAirbrush] = a182CrossCursor
	logf("CURSOR A218 adaptive user-art ready handles=%d threshold=%d pairs=%d eyedropper=original", len(a181CursorHandles), cursorContrastLumaThreshold, len(a217AdaptiveCursorPairs))
}

func destroyToolCursors() {
	for _, h := range a181CursorHandles {
		if h != 0 {
			procDestroyCursorA181.Call(h)
		}
	}
	a181CursorHandles = nil
	a181ToolCursors = nil
	a181MarqueeCursor = 0
	a181LassoCursor = 0
	a181MagicWandCursor = 0
	a181EraserCursor = 0
	a181FillCursor = 0
	a181EyedropperCursor = 0
	a181MagnifierCursor = 0
	a181BrushCursor = 0
	a182SelectionCursor = 0
	a182ColourCursor = 0
	a182FillCursor = 0
	a182CrossCursor = 0
	a182InvisibleCursor = 0
	a217AdaptiveCursorPairs = nil
}

func loadSystemCursorA181(id uintptr) uintptr {
	h, _, _ := procLoadCursorW.Call(0, id)
	return h
}

func resizeCursorForHandleA181(handle int) uintptr {
	switch handle {
	case 0, 7:
		return loadSystemCursorA181(IDC_SIZENWSE)
	case 2, 5:
		return loadSystemCursorA181(IDC_SIZENESW)
	case 1, 6:
		return loadSystemCursorA181(IDC_SIZENS)
	case 3, 4:
		return loadSystemCursorA181(IDC_SIZEWE)
	default:
		return loadSystemCursorA181(IDC_ARROW)
	}
}

func (a *App) pointOverAnyPanelA181(mx, my int, client RECT) (bool, uintptr) {
	if action := a.hitTextPanel(mx, my, client); action != "" {
		if action == "drag" {
			return true, loadSystemCursorA181(IDC_SIZEALL)
		}
		return true, loadSystemCursorA181(IDC_ARROW)
	}
	if action, _ := a.hitClipboardPanel(mx, my, client); action != "" {
		if action == "drag" {
			return true, loadSystemCursorA181(IDC_SIZEALL)
		}
		return true, loadSystemCursorA181(IDC_ARROW)
	}
	if action, _ := a.hitHistoryPanel(mx, my, client); action != "" {
		if action == "drag" {
			return true, loadSystemCursorA181(IDC_SIZEALL)
		}
		return true, loadSystemCursorA181(IDC_ARROW)
	}
	if action := a.hitThumbnailPanel(mx, my, client); action != "" {
		switch action {
		case "drag":
			return true, loadSystemCursorA181(IDC_SIZEALL)
		case "jump":
			return true, loadSystemCursorA181(IDC_HAND)
		default:
			return true, loadSystemCursorA181(IDC_ARROW)
		}
	}
	if action := a.hitToolboxPanel(mx, my, client); action != "" {
		if action == "drag" {
			return true, loadSystemCursorA181(IDC_SIZEALL)
		}
		return true, loadSystemCursorA181(IDC_ARROW)
	}
	if action := a.hitPalettePanel(mx, my, client); action != "" {
		if action == "drag" {
			return true, loadSystemCursorA181(IDC_SIZEALL)
		}
		return true, loadSystemCursorA181(IDC_ARROW)
	}
	return false, 0
}

func (a *App) cursorBackgroundRGBA217(mx, my int) (byte, byte, byte, bool) {
	ix, iy, inside := a.clientToImage(mx, my)
	if !inside || a.canvas == nil || ix < 0 || iy < 0 || ix >= a.canvas.W || iy >= a.canvas.H {
		return 0, 0, 0, false
	}
	i := (iy*a.canvas.W + ix) * 4
	b, g, r := a.canvas.Pix[i+0], a.canvas.Pix[i+1], a.canvas.Pix[i+2]
	alpha := int(a.canvas.Pix[i+3])
	if alpha < 255 {
		cell := max(1, 8/max(a.zoom, 1))
		checker, alternate := 255, 224
		if a.darkMode {
			checker, alternate = 12, 62
		}
		if ((ix/cell)+(iy/cell))%2 != 0 {
			checker = alternate
		}
		b = byte((int(b)*alpha + checker*(255-alpha) + 127) / 255)
		g = byte((int(g)*alpha + checker*(255-alpha) + 127) / 255)
		r = byte((int(r)*alpha + checker*(255-alpha) + 127) / 255)
	}
	return r, g, b, true
}

func (a *App) adaptiveCursorHandleA217(base uintptr, mx, my int) uintptr {
	pair, ok := a217AdaptiveCursorPairs[base]
	if !ok {
		return base
	}
	r, g, b, inside := a.cursorBackgroundRGBA217(mx, my)
	if !inside {
		return base
	}
	luma := cursorPerceivedLuma(r, g, b)
	variant := 2 // dark cursor on luma >= 128
	handle := pair.Dark
	variantName := "dark"
	if luma < cursorContrastLumaThreshold {
		variant = 1
		handle = pair.Light
		variantName = "light"
	}
	if handle == 0 {
		handle = base
	}
	if a.adaptiveCursorBase != base || a.adaptiveCursorVariant != variant {
		now := time.Now()
		if a.adaptiveCursorBase != base || now.Sub(a.adaptiveCursorLastLog) >= 250*time.Millisecond {
			logf("CURSOR adaptive variant=%s luma=%d threshold=%d rgb=%d,%d,%d client=%d,%d", variantName, luma, cursorContrastLumaThreshold, r, g, b, mx, my)
			a.adaptiveCursorLastLog = now
		}
		a.adaptiveCursorBase = base
		a.adaptiveCursorVariant = variant
	}
	return handle
}

// Text handles use the same eight-handle ordering as hitTextBoxAction:
// TL, top, TR, right, BR, bottom, BL, left. The resize cursor also stays
// stable while the pointer is captured during a drag.
func textBoxResizeCursor149(handle int) uintptr {
	switch handle {
	case 0, 4:
		return loadSystemCursorA181(IDC_SIZENWSE)
	case 2, 6:
		return loadSystemCursorA181(IDC_SIZENESW)
	case 1, 5:
		return loadSystemCursorA181(IDC_SIZENS)
	case 3, 7:
		return loadSystemCursorA181(IDC_SIZEWE)
	default:
		return loadSystemCursorA181(IDC_ARROW)
	}
}

func (a *App) cursorForClientPoint(mx, my int, client RECT) uintptr {
	if a.pendingCanvasResize || a.resizingCanvas {
		return resizeCursorForHandleA181(a.resizeHandle)
	}
	if h, ok := a.hitCanvasHandle(mx, my); ok {
		return resizeCursorForHandleA181(h)
	}
	if over, cursor := a.pointOverAnyPanelA181(mx, my, client); over {
		return cursor
	}
	if a.tool == ToolText && a.textTool.Active {
		if a.textTool.DragMode == 2 {
			return textBoxResizeCursor149(a.textTool.DragHandle)
		}
		if a.textTool.DragMode == 1 {
			return loadSystemCursorA181(IDC_SIZEALL)
		}
		mode, handle := a.hitTextBoxAction(mx, my)
		switch mode {
		case 2:
			return textBoxResizeCursor149(handle)
		case 1:
			return loadSystemCursorA181(IDC_SIZEALL)
		case 3:
			return loadSystemCursorA181(IDC_IBEAM)
		}
	}
	// The user requested the normal I-beam only after a text area exists and
	// the pointer is inside it.  Area creation itself uses cursor design #1.
	if a.textTool.Active && pointInRect(mx, my, a.textScreenRect()) {
		return loadSystemCursorA181(IDC_IBEAM)
	}
	ix, iy, inside := a.clientToImage(mx, my)
	if !inside {
		return loadSystemCursorA181(IDC_ARROW)
	}
	if a.selection.Moving {
		return loadSystemCursorA181(IDC_SIZEALL)
	}
	if (a.tool == ToolFreeSelect || a.tool == ToolRectSelect) && a.selection.Active && a.selection.Contains(ix, iy) {
		return loadSystemCursorA181(IDC_SIZEALL)
	}
	switch a.tool {
	case ToolText:
		if a182SelectionCursor != 0 {
			return a.adaptiveCursorHandleA217(a182SelectionCursor, mx, my)
		}
		return loadSystemCursorA181(IDC_CROSS)
	case ToolLine, ToolCurve, ToolRectangle, ToolEllipse, ToolRightTriangle, ToolArrow:
		if a182CrossCursor != 0 {
			return a.adaptiveCursorHandleA217(a182CrossCursor, mx, my)
		}
		return loadSystemCursorA181(IDC_CROSS)
	default:
		if h := a181ToolCursors[a.tool]; h != 0 {
			return a.adaptiveCursorHandleA217(h, mx, my)
		}
		return loadSystemCursorA181(IDC_CROSS)
	}
}
