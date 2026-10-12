package main

const maxColorMismatch = 100

func clampColorTolerance(value int) int {
	if value < 0 {
		return 0
	}
	if value > maxColorMismatch {
		return maxColorMismatch
	}
	return value
}

func colorPixelMatches(pix []byte, pixelIndex int, target [4]byte, tolerance int) bool {
	i := pixelIndex * 4
	if i < 0 || i+3 >= len(pix) {
		return false
	}
	alpha := pix[i+3]
	// Fully transparent pixels remain their own color class. Hidden RGB values
	// under alpha=0 must not make transparent pixels behave like visible black.
	if target[3] == 0 || alpha == 0 {
		return target[3] == 0 && alpha == 0
	}

	// Treat visible colors as points in 8-bit RGB space. Compare the squared
	// Euclidean distance instead of calling sqrt for every pixel:
	//   (R-R0)^2 + (G-G0)^2 + (B-B0)^2 <= tolerance^2
	// The canvas stores BGRA, hence channel offsets 0,1,2 are B,G,R.
	tolerance = clampColorTolerance(tolerance)
	db := int(pix[i+0]) - int(target[0])
	dg := int(pix[i+1]) - int(target[1])
	dr := int(pix[i+2]) - int(target[2])
	distanceSquared := db*db + dg*dg + dr*dr
	return distanceSquared <= tolerance*tolerance
}

// buildColorSelectionMask creates either a 4-neighbour contiguous magic-wand
// selection or a canvas-wide same-color selection. The visible `허용 불일치`
// range is 0..100 and is interpreted as Euclidean distance in RGB space.
func buildColorSelectionMask(pix []byte, canvasW, canvasH, seedX, seedY, tolerance int, selectAll bool) (x, y, w, h int, mask []bool, count int) {
	if canvasW < 1 || canvasH < 1 || len(pix) != canvasW*canvasH*4 || seedX < 0 || seedY < 0 || seedX >= canvasW || seedY >= canvasH {
		return
	}
	total := canvasW * canvasH
	seed := seedY*canvasW + seedX
	si := seed * 4
	target := [4]byte{pix[si], pix[si+1], pix[si+2], pix[si+3]}
	minX, minY := canvasW, canvasH
	maxX, maxY := -1, -1

	// The global selector only needs a match bitmap. The contiguous selector uses
	// one compact state byte per pixel (0=unseen, 1=seen/nonmatch, 2=matched)
	// and a fixed int32 queue. This avoids the repeated queue reallocations and
	// large temporary memory spike of a growing []int queue on flat large images.
	var matched []bool
	var state []byte
	markBounds := func(index int) {
		px, py := index%canvasW, index/canvasW
		if px < minX {
			minX = px
		}
		if px > maxX {
			maxX = px
		}
		if py < minY {
			minY = py
		}
		if py > maxY {
			maxY = py
		}
		count++
	}

	if selectAll {
		matched = make([]bool, total)
		for index := 0; index < total; index++ {
			if colorPixelMatches(pix, index, target, tolerance) {
				matched[index] = true
				markBounds(index)
			}
		}
	} else {
		state = make([]byte, total)
		queue := make([]int32, total)
		head, tail := 0, 0
		state[seed] = 1
		if colorPixelMatches(pix, seed, target, tolerance) {
			state[seed] = 2
			queue[tail] = int32(seed)
			tail++
		}
		for head < tail {
			index := int(queue[head])
			head++
			markBounds(index)
			px, py := index%canvasW, index/canvasW
			neighbors := [4]int{-1, 1, -canvasW, canvasW}
			for n, next := range neighbors {
				ni := index + next
				if n == 0 && px == 0 || n == 1 && px == canvasW-1 || n == 2 && py == 0 || n == 3 && py == canvasH-1 {
					continue
				}
				if ni < 0 || ni >= total || state[ni] != 0 {
					continue
				}
				state[ni] = 1
				if colorPixelMatches(pix, ni, target, tolerance) {
					state[ni] = 2
					queue[tail] = int32(ni)
					tail++
				}
			}
		}
	}

	if count == 0 || maxX < minX || maxY < minY {
		return 0, 0, 0, 0, nil, 0
	}
	x, y = minX, minY
	w, h = maxX-minX+1, maxY-minY+1
	mask = make([]bool, w*h)
	for py := minY; py <= maxY; py++ {
		for px := minX; px <= maxX; px++ {
			index := py*canvasW + px
			isMatch := false
			if selectAll {
				isMatch = matched[index]
			} else {
				isMatch = state[index] == 2
			}
			if isMatch {
				mask[(py-minY)*w+(px-minX)] = true
			}
		}
	}
	if count == w*h {
		mask = nil
	}
	return
}
