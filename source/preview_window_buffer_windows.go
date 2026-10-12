//go:build windows

package main

import "unsafe"

// previewEditorPaintBuffer atomically presents formula/graph editor previews.
// Both editors already suppress WM_ERASEBKGND; this closes the remaining
// direct-to-screen exposure (background first, raster second).
type previewEditorPaintBuffer struct {
	dc, bitmap, oldBitmap uintptr
	w, h                  int
}

func (b *previewEditorPaintBuffer) release() {
	multidimReleaseBitmapBuffer(&b.dc, &b.bitmap, &b.oldBitmap, &b.w, &b.h)
}

func (b *previewEditorPaintBuffer) paint(hwnd, screenDC uintptr, update RECT, draw func(uintptr)) {
	if hwnd == 0 || screenDC == 0 {
		return
	}
	var client RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
	w, h := int(client.Right-client.Left), int(client.Bottom-client.Top)
	if w < 1 || h < 1 {
		return
	}
	if !multidimEnsureBitmapBuffer(screenDC, w, h, &b.dc, &b.bitmap, &b.oldBitmap, &b.w, &b.h) {
		// Keep the old direct path only as a last-resort GDI allocation fallback.
		draw(screenDC)
		return
	}
	draw(b.dc)
	rc := update
	if rc.Right <= rc.Left || rc.Bottom <= rc.Top {
		rc = client
	}
	rc.Left = int32(clamp(int(rc.Left), 0, w))
	rc.Top = int32(clamp(int(rc.Top), 0, h))
	rc.Right = int32(clamp(int(rc.Right), int(rc.Left), w))
	rc.Bottom = int32(clamp(int(rc.Bottom), int(rc.Top), h))
	if rc.Right > rc.Left && rc.Bottom > rc.Top {
		procBitBlt.Call(screenDC, uintptr(rc.Left), uintptr(rc.Top), uintptr(rc.Right-rc.Left), uintptr(rc.Bottom-rc.Top), b.dc, uintptr(rc.Left), uintptr(rc.Top), SRCCOPY)
	}
}
