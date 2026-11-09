//go:build windows

package main

import "unsafe"

func (a *App) refreshObjectGuidance() {
	if a.objectArmed == objectArmMove {
		a.statusText = localizeUI("객체 이동 대기: 캔버스 작업 영역에서 왼쪽 버튼을 누른 채 끌면 이동합니다. Esc는 취소합니다.")
	} else if a.objectArmed == objectArmResize {
		a.statusText = localizeUI("객체 크기 조절 대기: 다음 왼쪽 드래그로 오른쪽 아래 모서리를 조절합니다. Esc는 취소합니다.")
	} else {
		a.statusText = localizeUI(objectSelectionGuidance)
	}
}

// Thumbnail work is bounded by thumbnail area rather than full canvas size.
func (a *App) paintObjectThumbnail(hdc uintptr, rect RECT, base []byte) {
	w, h := int(rect.Right-rect.Left), int(rect.Bottom-rect.Top)
	if w <= 0 || h <= 0 {
		return
	}
	pix := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx, sy := x*a.canvas.W/w, y*a.canvas.H/h
			si := (sy*a.canvas.W + sx) * 4
			if si+4 > len(base) {
				continue
			}
			di := (y*w + x) * 4
			copy(pix[di:di+4], base[si:si+4])
			for i := range a.objects {
				if i == a.objectEditingIndex {
					continue
				}
				o := &a.objects[i]
				blendObjectPixel(pix, di, o, objectSpriteOffset(o, sx, sy))
			}
		}
	}
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(w), BiHeight: -int32(h), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	procStretchDIBits.Call(hdc, uintptr(rect.Left), uintptr(rect.Top), uintptr(w), uintptr(h), 0, 0, uintptr(w), uintptr(h), uintptr(unsafe.Pointer(&pix[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, SRCCOPY)
}
