package main

// toleranceFloodFillBGRA performs a four-neighbour contiguous fill using the
// same Euclidean RGB mismatch rule as the magic wand. It marks each pixel once,
// so the replacement color may safely fall inside the tolerance radius.
func toleranceFloodFillBGRA(pix []byte, width, height, seedX, seedY int, replacement [4]byte, tolerance int) int {
	if width < 1 || height < 1 || len(pix) != width*height*4 ||
		seedX < 0 || seedY < 0 || seedX >= width || seedY >= height {
		return 0
	}
	tolerance = clampColorTolerance(tolerance)
	seedIndex := seedY*width + seedX
	si := seedIndex * 4
	target := [4]byte{pix[si], pix[si+1], pix[si+2], pix[si+3]}
	if tolerance == 0 && target == replacement {
		return 0
	}

	total := width * height
	seen := make([]byte, total)
	stack := make([]int32, 0, min(total, max(width+height, 1024)))
	stack = append(stack, int32(seedIndex))
	seen[seedIndex] = 1
	filled := 0

	for len(stack) > 0 {
		index := int(stack[len(stack)-1])
		stack = stack[:len(stack)-1]
		if !colorPixelMatches(pix, index, target, tolerance) {
			continue
		}
		i := index * 4
		pix[i], pix[i+1], pix[i+2], pix[i+3] = replacement[0], replacement[1], replacement[2], replacement[3]
		filled++

		px, py := index%width, index/width
		push := func(next int) {
			if next < 0 || next >= total || seen[next] != 0 {
				return
			}
			seen[next] = 1
			stack = append(stack, int32(next))
		}
		if px > 0 {
			push(index - 1)
		}
		if px+1 < width {
			push(index + 1)
		}
		if py > 0 {
			push(index - width)
		}
		if py+1 < height {
			push(index + width)
		}
	}
	return filled
}
