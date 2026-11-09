//go:build windows

package main

import "unsafe"

// Windows can scale a CreateCursor HCURSOR to the current DPI and to the
// accessibility mouse-pointer size. This dotPaint-only screen-inverting
// selection pointer instead XORs the user's exact 32x31 pixels into the
// *front* window DC. Its 32x32 source DIB never enters the persistent canvas,
// print, screenshot, clipboard, or saved DOP document.
//
// The normal Windows mouse cursor remains present, but transparent. Other
// cursor shapes (move/resize and all non-object tools) stay native.
const (
	fixedObjectCursorSide = 32
	fixedObjectCursorXOR  = 0x00660046 // SRCINVERT: dst RGB XOR src RGB
	fixedObjectTrackLeave = 0x00000002 // TME_LEAVE
)

var procTrackMouseEventObject = modUser32.NewProc("TrackMouseEvent")

type objectTrackMouseEvent struct {
	Size      uint32
	Flags     uint32
	HWND      uintptr
	HoverTime uint32
}

type fixedObjectPointerState struct {
	dc, bitmap, oldBitmap uintptr
	hiddenCursor          uintptr // owned by a181CursorHandles
	x, y                  int
	clip                  RECT
	visible, tracking     bool
}

var fixedObjectPointer fixedObjectPointerState

func initFixedObjectPointer(instance uintptr) {
	p := objectPointerPixels()
	// The 32x31 artwork is intentionally not resampled or padded with ink.
	dc, _, _ := procCreateCompatibleDC.Call(0)
	if dc == 0 {
		return
	}
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{
		BiSize:  uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
		BiWidth: fixedObjectCursorSide, BiHeight: -fixedObjectCursorSide,
		BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB,
	}}
	var bits unsafe.Pointer
	bitmap, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 || bits == nil {
		if bitmap != 0 {
			procDeleteObject.Call(bitmap)
		}
		procDeleteDC.Call(dc)
		return
	}
	oldBitmap, _, _ := procSelectObject.Call(dc, bitmap)
	data := unsafe.Slice((*byte)(bits), fixedObjectCursorSide*fixedObjectCursorSide*4)
	ink := 0
	for i, v := range p {
		if v == cursorInvert {
			// White source XORs each underlying RGB channel with 0xff.
			data[4*i+0], data[4*i+1], data[4*i+2] = 0xff, 0xff, 0xff
			ink++
		}
	}
	// 32x32 invisible native HCURSOR: Windows may enlarge the *invisible*
	// bitmap, but only our physical-pixel software overlay is visible.
	blank := createMonoCursor(instance, newMonoCursorCanvas(32, 32), 0, 0, "object-pointer-hidden-native")
	if blank == 0 {
		procSelectObject.Call(dc, oldBitmap)
		procDeleteObject.Call(bitmap)
		procDeleteDC.Call(dc)
		return
	}
	fixedObjectPointer = fixedObjectPointerState{
		dc: dc, bitmap: bitmap, oldBitmap: oldBitmap, hiddenCursor: blank,
	}
	logf("CURSOR physical-32 initialized artwork=%dx%d invertPixels=%d", 32, 31, ink)
}

func destroyFixedObjectPointer() {
	if fixedObjectPointer.dc != 0 {
		if fixedObjectPointer.oldBitmap != 0 {
			procSelectObject.Call(fixedObjectPointer.dc, fixedObjectPointer.oldBitmap)
		}
		if fixedObjectPointer.bitmap != 0 {
			procDeleteObject.Call(fixedObjectPointer.bitmap)
		}
		procDeleteDC.Call(fixedObjectPointer.dc)
	}
	// The transparent HCURSOR is freed by destroyToolCursors.
	fixedObjectPointer = fixedObjectPointerState{}
}

func fixedObjectClipRect(a, b RECT) RECT {
	if a.Left < b.Left {
		a.Left = b.Left
	}
	if a.Top < b.Top {
		a.Top = b.Top
	}
	if a.Right > b.Right {
		a.Right = b.Right
	}
	if a.Bottom > b.Bottom {
		a.Bottom = b.Bottom
	}
	return a
}

func fixedObjectPointerArea(x, y int) RECT {
	return RECT{int32(x), int32(y), int32(x + fixedObjectCursorSide), int32(y + fixedObjectCursorSide)}
}

func (a *App) fixedObjectPointerBackBufferValid(client RECT) bool {
	return a != nil && a.backDC != 0 && a.backBitmap != 0 &&
		a.backW == int(client.Right-client.Left) && a.backH == int(client.Bottom-client.Top)
}

func (a *App) fixedObjectPointerRestore(dc uintptr) {
	p := &fixedObjectPointer
	if !p.visible || dc == 0 || a.backDC == 0 {
		return
	}
	clip := fixedObjectClipRect(p.clip, fixedObjectPointerArea(p.x, p.y))
	if rectEmpty(clip) {
		return
	}
	saved, _, _ := procSaveDC.Call(dc)
	procIntersectClipRect.Call(dc, uintptr(clip.Left), uintptr(clip.Top), uintptr(clip.Right), uintptr(clip.Bottom))
	procBitBlt.Call(dc, uintptr(p.x), uintptr(p.y), fixedObjectCursorSide, fixedObjectCursorSide,
		a.backDC, uintptr(p.x), uintptr(p.y), SRCCOPY)
	if saved != 0 {
		procRestoreDC.Call(dc, saved)
	}
}

func (a *App) fixedObjectPointerInvert(dc uintptr, paintClip RECT) {
	p := &fixedObjectPointer
	if dc == 0 || p.dc == 0 || !p.visible {
		return
	}
	clip := fixedObjectClipRect(fixedObjectClipRect(p.clip, paintClip), fixedObjectPointerArea(p.x, p.y))
	if rectEmpty(clip) {
		return
	}
	saved, _, _ := procSaveDC.Call(dc)
	procIntersectClipRect.Call(dc, uintptr(clip.Left), uintptr(clip.Top), uintptr(clip.Right), uintptr(clip.Bottom))
	procBitBlt.Call(dc, uintptr(p.x), uintptr(p.y), fixedObjectCursorSide, fixedObjectCursorSide,
		p.dc, 0, 0, fixedObjectCursorXOR)
	if saved != 0 {
		procRestoreDC.Call(dc, saved)
	}
}

func (a *App) drawFixedObjectPointerOnPaint(screenDC uintptr, update RECT) {
	// Called *after* the backing bitmap was copied to the actual window.
	// Crucially, we NEVER XOR into the persistent backing bitmap itself.
	a.fixedObjectPointerInvert(screenDC, update)
}

func (a *App) clearFixedObjectPointer() {
	p := &fixedObjectPointer
	if !p.visible {
		return
	}
	if a != nil && a.hwnd != 0 {
		dc, _, _ := procGetDC.Call(a.hwnd)
		if dc != 0 {
			if a.backDC != 0 {
				a.fixedObjectPointerRestore(dc)
			} else {
				rc := fixedObjectPointerArea(p.x, p.y)
				invalidateArea(a.hwnd, &rc)
			}
			procReleaseDC.Call(a.hwnd, dc)
		}
	}
	p.visible = false
}

func (a *App) syncFixedObjectPointer(mx, my int, client RECT, native uintptr) uintptr {
	p := &fixedObjectPointer
	canPaint := p.dc != 0 && p.hiddenCursor != 0 && objectCursors[0] != 0 &&
		native == objectCursors[0] && a.fixedObjectPointerBackBufferValid(client)
	if !canPaint {
		a.clearFixedObjectPointer()
		return native
	}
	ws, _, _, _ := a.layout(client)
	if rectEmpty(ws) || mx < int(ws.Left) || mx >= int(ws.Right) || my < int(ws.Top) || my >= int(ws.Bottom) {
		a.clearFixedObjectPointer()
		return native
	}
	if !p.tracking {
		track := objectTrackMouseEvent{Size: uint32(unsafe.Sizeof(objectTrackMouseEvent{})), Flags: fixedObjectTrackLeave, HWND: a.hwnd}
		if success, _, _ := procTrackMouseEventObject.Call(uintptr(unsafe.Pointer(&track))); success != 0 {
			p.tracking = true
		}
	}
	if p.visible && p.x == mx && p.y == my && p.clip == ws {
		return p.hiddenCursor
	}
	dc, _, _ := procGetDC.Call(a.hwnd)
	if dc == 0 {
		return native
	}
	if p.visible {
		a.fixedObjectPointerRestore(dc)
	}
	p.visible, p.x, p.y, p.clip = true, mx, my, ws
	a.fixedObjectPointerInvert(dc, ws)
	procReleaseDC.Call(a.hwnd, dc)
	return p.hiddenCursor
}

// Selection can change a hover pointer into a move/resize cursor without a
// WM_MOUSEMOVE. Reevaluate after mouse-down/up rather than leaving stale ink.
func (a *App) refreshFixedObjectPointerAfterClick(mx, my int) {
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	native := a.cursorForClientPoint(mx, my, client)
	procSetCursor.Call(a.syncFixedObjectPointer(mx, my, client, native))
}

// Wheel/scroll commands can change the visible surface without moving the
// physical mouse. Reestablish the fixed pointer at the current client point.
func (a *App) refreshFixedObjectPointerFromMouse() {
	if a == nil || a.hwnd == 0 || (a.tool != ToolObjectSelect && !fixedObjectPointer.visible) {
		return
	}
	var at POINT
	if ok, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&at))); ok == 0 {
		return
	}
	procScreenToClient.Call(a.hwnd, uintptr(unsafe.Pointer(&at)))
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	if at.X < client.Left || at.Y < client.Top || at.X >= client.Right || at.Y >= client.Bottom {
		a.clearFixedObjectPointer()
		return
	}
	a.refreshFixedObjectPointerAfterClick(int(at.X), int(at.Y))
}
