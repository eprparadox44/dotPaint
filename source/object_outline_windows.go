//go:build windows

package main

import "unsafe"

var (
	objectCreateRgn  = modGdi32.NewProc("CreateRectRgn")
	objectExtRgn     = modGdi32.NewProc("ExtCreateRegion")
	objectCombineRgn = modGdi32.NewProc("CombineRgn")
	objectOffsetRgn  = modGdi32.NewProc("OffsetRgn")
	objectFillRgn    = modGdi32.NewProc("FillRgn")
)

type objectOutlineKey struct {
	Pixels     uintptr
	W, H, Zoom int
	Variant    int
}

func (a *App) clearObjectOutlines() {
	for _, h := range a.objectOutlineCache {
		procDeleteObject.Call(h)
	}
	a.objectOutlineCache = nil
}

func copyObjectRegion(r uintptr) uintptr {
	h, _, _ := objectCreateRgn.Call(0, 0, 0, 0)
	objectCombineRgn.Call(h, r, 0, 5)
	return h
}

func dilateObjectRegion(r uintptr, radius int) uintptr {
	out := copyObjectRegion(r)
	for axis := 0; axis < 2; axis++ {
		source := copyObjectRegion(out)
		for d := -radius; d <= radius; d++ {
			if d == 0 {
				continue
			}
			part := copyObjectRegion(source)
			dx, dy := d, 0
			if axis == 1 {
				dx, dy = 0, d
			}
			objectOffsetRgn.Call(part, uintptr(dx), uintptr(dy))
			objectCombineRgn.Call(out, out, part, 2)
			procDeleteObject.Call(part)
		}
		procDeleteObject.Call(source)
	}
	return out
}

func vectorOutlineRegion(o *MovableObject, zoom, outerRadius, innerRadius int) uintptr {
	w, h := o.spriteSize()
	rects := make([]RECT, 0, h*2)
	for y := 0; y < h; y++ {
		for x := 0; x < w; {
			if o.Pix[(y*w+x)*4+3] == 0 {
				x++
				continue
			}
			start := x
			for x < w && o.Pix[(y*w+x)*4+3] != 0 {
				x++
			}
			rects = append(rects, RECT{int32(start * zoom), int32(y * zoom), int32(x * zoom), int32((y + 1) * zoom)})
		}
	}
	if len(rects) == 0 {
		return 0
	}
	data := make([]int32, 8+len(rects)*4)
	data[0], data[1], data[2], data[3] = 32, 1, int32(len(rects)), int32(len(rects)*16)
	data[4], data[5], data[6], data[7] = 0, 0, int32(w*zoom), int32(h*zoom)
	for i, r := range rects {
		j := 8 + i*4
		data[j], data[j+1], data[j+2], data[j+3] = r.Left, r.Top, r.Right, r.Bottom
	}
	base, _, _ := objectExtRgn.Call(0, uintptr(len(data)*4), uintptr(unsafe.Pointer(&data[0])))
	if base == 0 {
		return 0
	}
	outer := dilateObjectRegion(base, outerRadius)
	if innerRadius <= 0 {
		procDeleteObject.Call(base)
		return outer
	}
	inner := dilateObjectRegion(base, innerRadius)
	objectCombineRgn.Call(outer, outer, inner, 4)
	procDeleteObject.Call(inner)
	procDeleteObject.Call(base)
	return outer
}

func objectOutlineCacheKey(o *MovableObject, zoom, variant int) objectOutlineKey {
	var p uintptr
	if len(o.Pix) > 0 {
		p = uintptr(unsafe.Pointer(&o.Pix[0]))
	}
	return objectOutlineKey{Pixels: p, W: o.SpriteW, H: o.SpriteH, Zoom: zoom, Variant: variant}
}

func (a *App) cachedVectorOutline(o *MovableObject, zoom, variant, outerRadius, innerRadius int, used map[objectOutlineKey]bool) uintptr {
	key := objectOutlineCacheKey(o, zoom, variant)
	used[key] = true
	if a.objectOutlineCache == nil {
		a.objectOutlineCache = make(map[objectOutlineKey]uintptr)
	}
	if r, ok := a.objectOutlineCache[key]; ok {
		return r
	}
	r := vectorOutlineRegion(o, zoom, outerRadius, innerRadius)
	a.objectOutlineCache[key] = r
	return r
}

func drawObjectRegion(hdc, r uintptr, x, y int, col uint32) {
	if r == 0 {
		return
	}
	brush, _, _ := procCreateSolidBrush.Call(uintptr(col))
	objectOffsetRgn.Call(r, uintptr(x), uintptr(y))
	objectFillRgn.Call(hdc, r, brush)
	objectOffsetRgn.Call(r, uintptr(-x), uintptr(-y))
	procDeleteObject.Call(brush)
}

func drawObjectVertexHandles(hdc uintptr, o *MovableObject, ox, oy int, col, fill uint32) {
	for j, p := range objectVertexPoints(o, o.appZoomOrDefault(), ox, oy) {
		c := col
		if j > 0 && j < o.Vector.Count-1 {
			c = uiColor(rgb(195, 90, 20))
		}
		r := RECT{int32(p.X - 3), int32(p.Y - 3), int32(p.X + 4), int32(p.Y + 4)}
		fillRect(hdc, r, fill)
		frameRect(hdc, r, c)
	}
}

func (o *MovableObject) appZoomOrDefault() int {
	if o == nil {
		return 1
	}
	return 1
}

func (a *App) paintMovableObjectSelection(hdc uintptr, ox, oy int) {
	if a.tool != ToolObjectSelect {
		if len(a.objectOutlineCache) > 0 {
			a.clearObjectOutlines()
		}
		return
	}
	used := make(map[objectOutlineKey]bool)
	selectedColor, selectedFill := uiColor(rgb(0, 120, 215)), uiColor(rgb(210, 235, 255))
	plainColor, plainFill := uiColor(rgb(188, 188, 188)), uiColor(rgb(239, 239, 239))
	if a.darkMode {
		plainColor, plainFill = uiColor(rgb(128, 128, 128)), uiColor(rgb(70, 70, 70))
	}
	for i := range a.objects {
		if i == a.objectEditingIndex {
			continue
		}
		o := &a.objects[i]
		if o.Rasterized {
			continue
		}
		selected := a.objectSelectionHas(i) && len(a.selectedObjectIndices()) == 1
		col, fill := plainColor, plainFill
		if selected {
			col, fill = selectedColor, selectedFill
		}
		x, y := ox+o.X*a.zoom, oy+o.Y*a.zoom
		if vectorObjectKind(o.Kind) && len(o.Pix) > 0 {
			basic := a.cachedVectorOutline(o, a.zoom, 0, 2, 1, used)
			drawObjectRegion(hdc, basic, x, y, col)
			if selected {
				extra := a.cachedVectorOutline(o, a.zoom, 1, 4, 2, used)
				drawObjectRegion(hdc, extra, x, y, col)
			}
			for j, p := range objectVertexPoints(o, a.zoom, ox, oy) {
				c := col
				if selected && j > 0 && j < o.Vector.Count-1 {
					c = uiColor(rgb(195, 90, 20))
				}
				r := RECT{int32(p.X - 3), int32(p.Y - 3), int32(p.X + 4), int32(p.Y + 4)}
				fillRect(hdc, r, fill)
				frameRect(hdc, r, c)
			}
			continue
		}
		base := RECT{int32(x - 2), int32(y - 2), int32(x + o.W*a.zoom + 2), int32(y + o.H*a.zoom + 2)}
		frameRect(hdc, base, col)
		if selected {
			extra := RECT{int32(x - 4), int32(y - 4), int32(x + o.W*a.zoom + 4), int32(y + o.H*a.zoom + 4)}
			for k := int32(0); k < 2; k++ {
				frameRect(hdc, RECT{extra.Left + k, extra.Top + k, extra.Right - k, extra.Bottom - k}, col)
			}
		}
		for _, p := range objectHandlePoints(o.bounds(), a.zoom, ox, oy) {
			r := RECT{int32(p.X - 3), int32(p.Y - 3), int32(p.X + 4), int32(p.Y + 4)}
			fillRect(hdc, r, fill)
			frameRect(hdc, r, col)
		}
	}
	// A multi-object selection receives one group boundary and one set of
	// resize handles. Individual shapes keep their unobtrusive 1px outlines.
	if bounds, ok := a.objectGroupBounds(); ok {
		l := ox + bounds.X*a.zoom - 4
		t := oy + bounds.Y*a.zoom - 4
		r := ox + (bounds.X+bounds.W)*a.zoom + 4
		b := oy + (bounds.Y+bounds.H)*a.zoom + 4
		for k := 0; k < 2; k++ {
			frameRect(hdc, RECT{int32(l + k), int32(t + k), int32(r - k), int32(b - k)}, selectedColor)
		}
		for _, p := range objectHandlePoints(bounds, a.zoom, ox, oy) {
			handle := RECT{int32(p.X - 3), int32(p.Y - 3), int32(p.X + 4), int32(p.Y + 4)}
			fillRect(hdc, handle, selectedFill)
			frameRect(hdc, handle, selectedColor)
		}
	}
	for key, h := range a.objectOutlineCache {
		if !used[key] {
			procDeleteObject.Call(h)
			delete(a.objectOutlineCache, key)
		}
	}
}
