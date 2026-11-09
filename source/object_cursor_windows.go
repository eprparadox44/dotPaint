//go:build windows

package main

var objectCursors [6]uintptr // pointer, move, horizontal, vertical, NWSE, NESW

func initObjectCursors(instance uintptr) {
	p := objectPointerPixels()
	c := newMonoCursorCanvas(32, 32)
	copy(c.pix, p[:])
	objectCursors[0] = createMonoCursor(instance, c, 0, 0, "object-pointer-invert-user")
	for i := 0; i < 5; i++ {
		p = objectArrowCursorPixels(i)
		copy(c.pix, p[:])
		objectCursors[i+1] = createMonoCursor(instance, c, 15, 15, "object-manipulation")
	}
}

func (a *App) objectInteractionCursor(h ObjectHandle) uintptr {
	index := 1
	if a.objectArmed == objectArmResize {
		h = ObjectSE
	}
	switch h {
	case ObjectW, ObjectE:
		index = 2
	case ObjectN, ObjectS:
		index = 3
	case ObjectNW, ObjectSE:
		index = 4
	case ObjectNE, ObjectSW:
		index = 5
	}
	if objectCursors[index] != 0 {
		return objectCursors[index]
	}
	return loadSystemCursorA181(IDC_SIZEALL)
}

func (a *App) drawObjectSelectionToolIcon(hdc uintptr, cx, cy int) {
	theme := 0
	if a.darkMode {
		theme = 1
	}
	pixels := &objectToolIcons[theme]
	left, top := cx-objectIconW/2, cy-objectIconH/2
	for y := 0; y < objectIconH; y++ {
		for x := 0; x < objectIconW; {
			p := pixels[y*objectIconW+x]
			if p == objectArtTransparent {
				x++
				continue
			}
			start := x
			for x < objectIconW && pixels[y*objectIconW+x] == p {
				x++
			}
			fillRect(hdc, RECT{int32(left + start), int32(top + y), int32(left + x), int32(top + y + 1)}, p)
		}
	}
}
