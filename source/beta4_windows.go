//go:build windows

package main

import (
	"bytes"
	_ "embed"
	"image/png"
	"sync"
	"syscall"
	"unsafe"
)

//go:embed invert_color_reference.png
var invertColorReference []byte
var invertIconOnce sync.Once
var invertIconPixels [2][]byte

func initInvertIcon() {
	im, err := png.Decode(bytes.NewReader(invertColorReference))
	if err != nil {
		return
	}
	for theme := 0; theme < 2; theme++ {
		pix := make([]byte, 46*46*4)
		for y := 0; y < 46; y++ {
			for x := 0; x < 46; x++ {
				r, g, b, _ := im.At(x+theme*46, y).RGBA()
				i := (y*46 + x) * 4
				pix[i], pix[i+1], pix[i+2], pix[i+3] = byte(b>>8), byte(g>>8), byte(r>>8), 255
			}
		}
		invertIconPixels[theme] = pix
	}
}

func (a *App) paintInvertIcon(hdc uintptr, r RECT) {
	invertIconOnce.Do(initInvertIcon)
	theme := 0
	if a.darkMode {
		theme = 1
	}
	pix := invertIconPixels[theme]
	if len(pix) == 0 {
		fillRect(hdc, r, uiColor(rgb(255, 255, 255)))
		drawUIText(hdc, "INV", r, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0))
		return
	}
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: 46, BiHeight: -46, BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	lines, _, _ := procStretchDIBits.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0, 0, 46, 46, uintptr(unsafe.Pointer(&pix[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, SRCCOPY)
	if lines == 0 || lines == ^uintptr(0) {
		// Never leave an apparently missing palette command if a particular
		// Windows display driver rejects StretchDIBits.
		fillRect(hdc, r, uiColor(rgb(255, 255, 255)))
		drawUIText(hdc, "INV", r, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0))
	}
}

func (a *App) paletteInvertArea(rc RECT) RECT {
	invert, _ := a.paletteSpecialAreas(rc)
	return invert
}

func (a *App) chosenOperation(right bool) PaintOperation {
	op, col := a.fgOperation, a.fg
	if right {
		op, col = a.bgOperation, a.bg
	}
	return paintMode(op, col)
}

func (a *App) chooseInvert(right bool) {
	previousFG, previousOp := a.fg, a.chosenOperation(false)
	if a.dejavu != nil {
		a.statusText = localizeUI("DejaVu IMG에서는 반전색을 사용할 수 없습니다.")
	} else {
		if right {
			a.bgOperation = PaintInvert
			a.bgPaletteSlot = -1
		} else {
			a.fgOperation = PaintInvert
			a.fgPaletteSlot = -1
		}
		if a.tool == ToolObjectSelect {
			col := a.fg
			if right {
				col = a.bg
			}
			a.changeSelectedObjectColor(col, PaintInvert, right)
		}
		if !right && a.textTool.Active {
			a.applyTextColorToSelection(previousFG, previousOp, a.fg, PaintInvert)
		}
		a.statusText = localizeUI("반전색: 아래 합성색을 뒤집습니다. 반전 객체는 불투명도 100%, 경계 포함률 50% 이상에 적용됩니다.")
	}
	a.requestStatusRepaint()
	invalidate(a.hwnd)
}

func (a *App) updateInvertTooltip(rc RECT) {
	if a.objectTooltip == 0 {
		return
	}
	label := localizeUI("반전색")
	text := syscall.StringToUTF16(label)
	// The palette painter receives content-local coordinates, but Win32
	// tooltip rectangles use the main window's client coordinates.
	r := a.paletteInvertArea(rc)
	r.Left += rc.Left
	r.Right += rc.Left
	ti := objectToolInfo{Flags: 0x10, Hwnd: a.hwnd, ID: 0xB201, Rect: r, Text: &text[0]}
	ti.Size = uint32(unsafe.Sizeof(ti))
	if !a.invertTooltipAdded {
		sendMessage(a.objectTooltip, 0x0432, 0, uintptr(unsafe.Pointer(&ti)))
		a.invertTooltipAdded = true
	}
	sendMessage(a.objectTooltip, 0x0434, 0, uintptr(unsafe.Pointer(&ti)))
	sendMessage(a.objectTooltip, 0x0439, 0, uintptr(unsafe.Pointer(&ti)))
	a.invertTooltipText = text
}

func (a *App) makeVectorObject(kind Tool, x0, y0, x1, y1 int, constrained bool, col uint32) (MovableObject, bool) {
	o := MovableObject{Kind: kind, FG: col, Operation: a.drawOperation}
	g := ObjectVector{Count: 2, Constrained: constrained, CreationConstrained: constrained, Constraint: a.lineConstraintMode}
	if kind == ToolCurve {
		polygon := a.curveControlPolygon(a.curveActiveControlCount())
		g.Count = len(polygon)
		for i, p := range polygon {
			g.Points[i] = ObjectPoint{p.X, p.Y}
		}
		o.Stroke, g.Antialias, o.Operation = a.curveSize, a.curveAntialias, a.curveOperation
	} else {
		x1, y1 = a.constrainedLineEndpoint(x0, y0, x1, y1, constrained)
		g.Points[0], g.Points[1] = ObjectPoint{x0, y0}, ObjectPoint{x1, y1}
		if kind == ToolArrow {
			o.Stroke, g.Antialias, g.ArrowHead = a.arrowLineSize, a.arrowAntialias, a.arrowHeadSize
		} else {
			o.Stroke, g.Antialias = a.lineSize, a.lineAntialias
		}
	}
	o.Vector = g
	return renderVectorObject(o, a.canvas.W, a.canvas.H)
}

func (a *App) commitVectorPreview(ix, iy int, col uint32) (int, int, bool) {
	constrained := keyDown(VK_SHIFT)
	// Transparent paint does not create a blank editable shape. It punches
	// transparency through the rendered image, including overlapping objects.
	if a.dejavu == nil && !a.indexedMode && a.drawOperation == PaintTransparent {
		var mask MovableObject
		var ok bool
		if vectorObjectKind(a.tool) {
			mask, ok = a.makeVectorObject(a.tool, a.startX, a.startY, ix, iy, constrained, 0xFFFFFF)
		} else {
			mask, ok = a.makeShapeObject(a.tool, a.startX, a.startY, ix, iy, constrained, 0xFFFFFF)
		}
		if ok {
			a.applyTransparentShapeMask(mask)
		} else {
			a.statusText = localizeUI("투명 지우기 영역을 만들지 못했습니다. 크기를 줄여 다시 시도하십시오.")
		}
		return ix, iy, false
	}
	if a.dejavu == nil && !a.indexedMode && (a.keepObjects || a.drawOperation == PaintInvert) {
		var o MovableObject
		var ok bool
		if vectorObjectKind(a.tool) {
			o, ok = a.makeVectorObject(a.tool, a.startX, a.startY, ix, iy, constrained, col)
		} else {
			o, ok = a.makeShapeObject(a.tool, a.startX, a.startY, ix, iy, constrained, col)
		}
		if ok {
			if a.keepObjects {
				a.addMovableObject(o)
				return ix, iy, true
			}
			compositeObjectsToPixels(a.canvas.Pix, a.canvas.W, a.canvas.H, 0, 0, []MovableObject{o})
			return ix, iy, false
		}
		a.statusText = localizeUI("객체가 너무 커서 확정하지 못했습니다. 크기를 줄여 다시 시도하십시오.")
		return ix, iy, false
	}
	if a.indexedMode && a.dejavu == nil && a.drawOperation == PaintInvert {
		left, top, right, bottom := 0, 0, a.canvas.W-1, a.canvas.H-1
		switch a.tool {
		case ToolRectangle:
			left, top, right, bottom = a.rectanglePreviewCanvasBounds(ix, iy, constrained)
		case ToolEllipse:
			left, top, right, bottom = a.ellipsePreviewCanvasBounds(ix, iy, constrained)
		case ToolRightTriangle:
			left, top, right, bottom = a.rightTrianglePreviewCanvasBounds(ix, iy, constrained)
		case ToolLine:
			left, top, right, bottom = a.vectorPreviewCanvasBounds(ix, iy, constrained, false)
		case ToolArrow:
			left, top, right, bottom = a.vectorPreviewCanvasBounds(ix, iy, constrained, true)
		}
		a.commitIndexedInvertRegion(left, top, right, bottom, func(target *Canvas, ink uint32) {
			switch a.tool {
			case ToolRectangle:
				a.drawRasterRectangleAt(target, a.startX, a.startY, ix, iy, constrained, ink, left, top)
			case ToolEllipse:
				a.drawRasterEllipseAt(target, a.startX, a.startY, ix, iy, constrained, ink, left, top)
			case ToolRightTriangle:
				a.drawRasterRightTriangleAt(target, a.startX, a.startY, ix, iy, constrained, ink, left, top)
			case ToolLine:
				a.drawRasterLineAt(target, a.startX, a.startY, ix, iy, constrained, ink, left, top)
			case ToolArrow:
				a.drawRasterArrowAt(target, a.startX, a.startY, ix, iy, constrained, ink, left, top)
			}
		})
		return ix, iy, false
	}
	switch a.tool {
	case ToolLine:
		ix, iy = a.drawRasterLine(a.canvas, a.startX, a.startY, ix, iy, constrained, col)
	case ToolArrow:
		ix, iy = a.drawRasterArrow(a.canvas, a.startX, a.startY, ix, iy, constrained, col)
	case ToolRectangle:
		ix, iy = a.drawRasterRectangle(a.canvas, a.startX, a.startY, ix, iy, constrained, col)
	case ToolEllipse:
		ix, iy = a.drawRasterEllipse(a.canvas, a.startX, a.startY, ix, iy, constrained, col)
	case ToolRightTriangle:
		ix, iy = a.drawRasterRightTriangle(a.canvas, a.startX, a.startY, ix, iy, constrained, col)
	}
	return ix, iy, false
}

func (a *App) makeTextObject(text string) (MovableObject, bool) {
	for _, st := range a.textTool.Styles {
		if st.ColorSet {
			if o, ok := a.makeStyledTextObject(text); ok {
				return o, true
			}
			break
		}
	}
	fg, bg := a.fg, a.bg
	fop, bop := a.chosenOperation(false), a.chosenOperation(true)
	if fop != PaintInvert && bop != PaintInvert {
		return a.makeTextObjectRGB(text)
	}
	opaque := a.textTool.Opaque
	a.textTool.Opaque = false
	if fop == PaintInvert || fop == PaintTransparent {
		a.fg = 0xFFFFFF
	}
	o, ok := a.makeTextObjectRGB(text)
	a.textTool.Opaque = opaque
	a.fg, a.bg = fg, bg
	if ok {
		o.Operation, o.BackgroundOperation = fop, bop
		o.FG, o.BG, o.Opaque = fg, bg, opaque
	}
	return o, ok
}

// renderIndexedInvertText mirrors the RGB text path without an RGB sentinel.
// A native GDI-rendered glyph mask defines coverage; the destination is always
// a real palette entry. The index map is updated only for actual commits.
func (a *App) renderIndexedInvertText(target *Canvas, text string, updateIndices bool) bool {
	if a.dejavu != nil || !a.indexedMode || target == nil || text == "" {
		return false
	}
	w, h := a.textTool.W, a.textTool.H
	if w <= 0 || h <= 0 || !validObjectSize(w, h) {
		return false
	}
	x, y := a.textTool.X, a.textTool.Y
	// Preserve all text editing state, including the original position and
	// layout properties; only the mask rendering colour is temporarily changed.
	savedFG, savedOpaque := a.fg, a.textTool.Opaque
	a.fg, a.textTool.Opaque = rgb(255, 255, 255), false
	a.textTool.X, a.textTool.Y = 0, 0
	mask := &Canvas{W: w, H: h, Pix: make([]byte, w*h*4)}
	a.renderTextToTarget(mask, text, false)
	a.textTool.X, a.textTool.Y = x, y
	a.fg, a.textTool.Opaque = savedFG, savedOpaque

	palette := a.activeIndexedPaletteColors()
	if len(palette) == 0 {
		return false
	}
	lut := a.indexedInvertTable(palette)
	ids := []byte(nil)
	if updateIndices {
		ids = a.indexedPixelIndices
	}
	foregroundMode, backgroundMode := a.chosenOperation(false), a.chosenOperation(true)
	fgIndex := nearestIndexedColor(byte(savedFG), byte(savedFG>>8), byte(savedFG>>16), palette)
	bgIndex := nearestIndexedColor(byte(a.bg), byte(a.bg>>8), byte(a.bg>>16), palette)
	for row := 0; row < h; row++ {
		gy := y + row
		if gy < 0 || gy >= target.H {
			continue
		}
		for column := 0; column < w; column++ {
			gx := x + column
			if gx < 0 || gx >= target.W {
				continue
			}
			pi := gy*target.W + gx
			di := pi * 4
			if savedOpaque && backgroundMode != PaintTransparent {
				if backgroundMode == PaintInvert {
					applyIndexedInvertPixel(target.Pix, pi, ids, palette, lut)
				} else {
					c := palette[bgIndex]
					target.Pix[di+0], target.Pix[di+1], target.Pix[di+2], target.Pix[di+3] = byte(c>>16), byte(c>>8), byte(c), 255
					if len(ids) == target.W*target.H {
						ids[pi] = byte(bgIndex)
					}
				}
			}
			si := (row*w + column) * 4
			// Text AA is reported as RGB coverage by the native GDI renderer.
			// Match beta4's >=50% invert coverage policy.
			if mask.Pix[si+2] < 128 {
				continue
			}
			switch foregroundMode {
			case PaintInvert:
				applyIndexedInvertPixel(target.Pix, pi, ids, palette, lut)
			case PaintRGB:
				c := palette[fgIndex]
				target.Pix[di+0], target.Pix[di+1], target.Pix[di+2], target.Pix[di+3] = byte(c>>16), byte(c>>8), byte(c), 255
				if len(ids) == target.W*target.H {
					ids[pi] = byte(fgIndex)
				}
			}
		}
	}
	return true
}

func (a *App) cancelInverseStroke() {
	if !a.invertStrokeActive {
		return
	}
	a.invertStrokeActive = false
	a.invertStroke.Reset()
	a.indexedInvertLUT = nil
	a.drawing = false
	a.preview = false
	a.undo.Undo(a.canvas)
	if a.indexedMode {
		a.indexedPixelIndices = indexedIndicesForSnapshot(a.canvas.Pix, a.palette, a.indexedActiveCount, a.indexedPixelIndices)
	}
	if a.ownsMouseCapture() {
		procReleaseCapture.Call()
	}
	a.requestCanvasRepaint(true)
}

func (a *App) moveInverseStroke(mx, my int) {
	x, y, _ := a.clientToImage(mx, my)
	x = clamp(x, 0, a.canvas.W-1)
	y = clamp(y, 0, a.canvas.H-1)
	if keyDown(VK_SHIFT) {
		x, y = a.constrainedEightWayEndpoint(a.lastX, a.lastY, x, y)
	}
	if a.indexedMode {
		palette := a.activeIndexedPaletteColors()
		if len(a.indexedInvertLUT) != len(palette) {
			a.indexedInvertLUT = indexedInvertLookup(palette)
		}
		a.invertStroke.LineIndexed(a.canvas, a.lastX, a.lastY, x, y, a.brushSize, a.brushShape, a.indexedPixelIndices, palette, a.indexedInvertLUT)
	} else {
		a.invertStroke.Line(a.canvas, a.lastX, a.lastY, x, y, a.brushSize, a.brushShape)
	}
	a.lastX, a.lastY, a.curX, a.curY = x, y, x, y
	a.requestCanvasRepaint(true)
}

func (a *App) previewOperation() PaintOperation {
	if a.tool == ToolCurve {
		return a.curveOperation
	}
	return a.drawOperation
}

// An indexed invert table is rebuilt only when the actual palette changes.
// A 256-entry palette costs O(256) to hash per preview, rather than an
// O(256²) nearest-color search during every mouse-move event.
func (a *App) indexedInvertTable(palette []uint32) []byte {
	if len(palette) == 0 {
		return nil
	}
	hash := uint64(14695981039346656037)
	for _, c := range palette {
		hash ^= uint64(c)
		hash *= 1099511628211
	}
	hash ^= uint64(len(palette))
	if len(a.indexedInvertLUT) != len(palette) || hash != a.indexedInvertPaletteHash {
		a.indexedInvertLUT = indexedInvertLookup(palette)
		a.indexedInvertPaletteHash = hash
	}
	return a.indexedInvertLUT
}

func (a *App) applyIndexedMaskToCanvas(mask *Canvas, offsetX, offsetY int) {
	if a.canvas == nil || mask == nil || !a.indexedMode {
		return
	}
	palette := a.activeIndexedPaletteColors()
	lut := a.indexedInvertTable(palette)
	for y := 0; y < mask.H; y++ {
		gy := offsetY + y
		if gy < 0 || gy >= a.canvas.H {
			continue
		}
		for x := 0; x < mask.W; x++ {
			gx := offsetX + x
			if gx < 0 || gx >= a.canvas.W || mask.Pix[(y*mask.W+x)*4+3] < 128 {
				continue
			}
			applyIndexedInvertPixel(a.canvas.Pix, gy*a.canvas.W+gx, a.indexedPixelIndices, palette, lut)
		}
	}
}

func (a *App) commitIndexedInvertRegion(left, top, right, bottom int, draw func(*Canvas, uint32)) bool {
	if !a.indexedMode || a.canvas == nil {
		return false
	}
	left, top = max(0, left), max(0, top)
	right, bottom = min(a.canvas.W-1, right), min(a.canvas.H-1, bottom)
	if left > right || top > bottom {
		return false
	}
	mask := &Canvas{W: right - left + 1, H: bottom - top + 1}
	mask.Pix = make([]byte, mask.W*mask.H*4)
	draw(mask, 0xFFFFFF)
	a.applyIndexedMaskToCanvas(mask, left, top)
	return true
}

func (a *App) paintOperationPreview(target *Canvas, x, y int, draw func(*Canvas, uint32), col uint32) {
	if a.previewOperation() != PaintInvert {
		draw(target, col)
		return
	}
	mask := &Canvas{W: target.W, H: target.H, Pix: make([]byte, len(target.Pix))}
	draw(mask, 0xFFFFFF)
	if a.indexedMode && a.dejavu == nil {
		palette := a.activeIndexedPaletteColors()
		invertIndexedMask(target.Pix, mask.Pix, palette, a.indexedInvertTable(palette))
	} else {
		for i := 0; i < len(mask.Pix); i += 4 {
			if mask.Pix[i+3] >= 128 {
				invertBGRAPixel(target.Pix, i)
			}
		}
	}
	if !a.keepObjects {
		compositeObjectsToPixels(target.Pix, target.W, target.H, x, y, a.objects)
	}
}
