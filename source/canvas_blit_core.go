package main

// CanvasBlitTile describes one top-down strip of the source canvas that should
// be transferred to the screen. Source rows are half-open [Y0,Y1).
type CanvasBlitTile struct {
	X0, X1 int
	Y0, Y1 int
}

type CanvasBlitClip struct {
	Left, Top, Right, Bottom int
}

const canvasBlitTargetBytes = 512 * 1024
const canvasBlitMaxRows = 128

func clampBlit(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func floorDivInt(a, b int) int {
	if b <= 0 {
		return 0
	}
	q := a / b
	r := a % b
	if r != 0 && ((r < 0) != (b < 0)) {
		q--
	}
	return q
}

func ceilDivInt(a, b int) int {
	if b <= 0 {
		return 0
	}
	q := a / b
	r := a % b
	if r != 0 && ((r > 0) == (b > 0)) {
		q++
	}
	return q
}

// canvasVisibleBlitTiles returns only source rows/columns that can appear in
// the clipped workspace. Each transfer stays around 512 KiB even for a very
// wide canvas, avoiding a single multi-megabyte StretchDIBits call.
func canvasVisibleBlitTiles(canvasW, canvasH, zoom, originX, originY int, clip CanvasBlitClip) []CanvasBlitTile {
	if canvasW <= 0 || canvasH <= 0 || zoom <= 0 || clip.Right <= clip.Left || clip.Bottom <= clip.Top {
		return nil
	}
	x0 := floorDivInt(clip.Left-originX, zoom)
	y0 := floorDivInt(clip.Top-originY, zoom)
	x1 := ceilDivInt(clip.Right-originX, zoom)
	y1 := ceilDivInt(clip.Bottom-originY, zoom)
	x0 = clampBlit(x0, 0, canvasW)
	y0 = clampBlit(y0, 0, canvasH)
	x1 = clampBlit(x1, 0, canvasW)
	y1 = clampBlit(y1, 0, canvasH)
	if x1 <= x0 || y1 <= y0 {
		return nil
	}
	rowBytes := canvasW * 4
	rows := canvasBlitMaxRows
	if rowBytes > 0 {
		rows = canvasBlitTargetBytes / rowBytes
	}
	if rows < 1 {
		rows = 1
	}
	if rows > canvasBlitMaxRows {
		rows = canvasBlitMaxRows
	}
	out := make([]CanvasBlitTile, 0, (y1-y0+rows-1)/rows)
	for y := y0; y < y1; y += rows {
		next := y + rows
		if next > y1 {
			next = y1
		}
		out = append(out, CanvasBlitTile{X0: x0, X1: x1, Y0: y, Y1: next})
	}
	return out
}
