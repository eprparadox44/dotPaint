//go:build windows

package main

import "unsafe"

// A203 stages CPU-rendered BGRA frames through a persistent top-down DIB
// section before copying them into the preview backbuffer. Direct
// StretchDIBits calls could return without drawing on the first static frame on
// some Windows/GDI combinations, while a later drag happened to make the same
// cached pixels visible. The DIBSection + BitBlt path is deterministic.

func (s *multidimShapeEditorState) releaseRasterBlitBuffer() {
	if s == nil {
		return
	}
	if s.rasterBlitDC != 0 && s.rasterBlitOldBitmap != 0 {
		procSelectObject.Call(s.rasterBlitDC, s.rasterBlitOldBitmap)
	}
	if s.rasterBlitBitmap != 0 {
		procDeleteObject.Call(s.rasterBlitBitmap)
	}
	if s.rasterBlitDC != 0 {
		procDeleteDC.Call(s.rasterBlitDC)
	}
	s.rasterBlitDC = 0
	s.rasterBlitBitmap = 0
	s.rasterBlitOldBitmap = 0
	s.rasterBlitBits = nil
	s.rasterBlitW = 0
	s.rasterBlitH = 0
}

func (s *multidimShapeEditorState) ensureRasterBlitBuffer(referenceHDC uintptr, width, height int) bool {
	if s == nil || referenceHDC == 0 || width <= 0 || height <= 0 {
		return false
	}
	if s.rasterBlitDC != 0 && s.rasterBlitBitmap != 0 && s.rasterBlitBits != nil && s.rasterBlitW == width && s.rasterBlitH == height {
		return true
	}
	s.releaseRasterBlitBuffer()
	memDC, _, err := procCreateCompatibleDC.Call(referenceHDC)
	if memDC == 0 {
		logf("MULTIDIM_RASTER CreateCompatibleDC failed size=%dx%d err=%v", width, height, err)
		return false
	}
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{
		BiSize:        uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
		BiWidth:       int32(width),
		BiHeight:      -int32(height),
		BiPlanes:      1,
		BiBitCount:    32,
		BiCompression: BI_RGB,
	}}
	var bits unsafe.Pointer
	bitmap, _, err := procCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 || bits == nil {
		logf("MULTIDIM_RASTER CreateDIBSection failed size=%dx%d err=%v", width, height, err)
		procDeleteDC.Call(memDC)
		return false
	}
	oldBitmap, _, _ := procSelectObject.Call(memDC, bitmap)
	s.rasterBlitDC = memDC
	s.rasterBlitBitmap = bitmap
	s.rasterBlitOldBitmap = oldBitmap
	s.rasterBlitBits = bits
	s.rasterBlitW = width
	s.rasterBlitH = height
	logf("MULTIDIM_RASTER dib-ready size=%dx%d dc=%#x bitmap=%#x", width, height, memDC, bitmap)
	return true
}

func multidimBlitBGRA(s *multidimShapeEditorState, hdc uintptr, dst RECT, pixels []byte, srcW, srcH int, label string) bool {
	if s == nil || hdc == 0 || srcW <= 0 || srcH <= 0 || len(pixels) < srcW*srcH*4 {
		return false
	}
	if !s.ensureRasterBlitBuffer(hdc, srcW, srcH) {
		return false
	}
	dstBits := unsafe.Slice((*byte)(s.rasterBlitBits), srcW*srcH*4)
	copy(dstBits, pixels[:srcW*srcH*4])
	dstW := int(dst.Right - dst.Left)
	dstH := int(dst.Bottom - dst.Top)
	if dstW <= 0 || dstH <= 0 {
		return false
	}
	var result uintptr
	if dstW == srcW && dstH == srcH {
		result, _, _ = procBitBlt.Call(hdc, uintptr(dst.Left), uintptr(dst.Top), uintptr(dstW), uintptr(dstH), s.rasterBlitDC, 0, 0, SRCCOPY)
	} else {
		procSetStretchBltMode.Call(hdc, multidimHalftone)
		result, _, _ = procStretchBlt.Call(hdc, uintptr(dst.Left), uintptr(dst.Top), uintptr(dstW), uintptr(dstH), s.rasterBlitDC, 0, 0, uintptr(srcW), uintptr(srcH), SRCCOPY)
	}
	if result == 0 {
		logf("MULTIDIM_RASTER blit-failed label=%q src=%dx%d dst=%dx%d", label, srcW, srcH, dstW, dstH)
		return false
	}
	return true
}
