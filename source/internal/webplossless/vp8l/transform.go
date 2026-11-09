// Derivative of golang.org/x/image/vp8l (BSD-3-Clause, The Go Authors).
package vp8l

func nTiles(size int32, bits uint32) int32 { return (size + 1<<bits - 1) >> bits }

const (
	transformTypePredictor     = 0
	transformTypeCrossColor    = 1
	transformTypeSubtractGreen = 2
	transformTypeColorIndexing = 3
	nTransformTypes            = 4
)

type transform struct {
	transformType uint32
	oldWidth      int32
	bits          uint32
	pix           []byte
}

var inverseTransforms = [nTransformTypes]func(*transform, []byte, int32) []byte{inversePredictor, inverseCrossColor, inverseSubtractGreen, inverseColorIndexing}

func inversePredictor(t *transform, pix []byte, h int32) []byte {
	if t.oldWidth == 0 || h == 0 {
		return pix
	}
	pix[3] += 0xff
	p, mask := int32(4), int32(1)<<t.bits-1
	for x := int32(1); x < t.oldWidth; x++ {
		for c := 0; c < 4; c++ {
			pix[p+int32(c)] += pix[p-4+int32(c)]
		}
		p += 4
	}
	top, tilesPerRow := 0, nTiles(t.oldWidth, t.bits)
	for y := int32(1); y < h; y++ {
		for c := 0; c < 4; c++ {
			pix[p+int32(c)] += pix[int32(top+c)]
		}
		p, top = p+4, top+4
		q := 4 * (y >> t.bits) * tilesPerRow
		mode := t.pix[q+1] & 0x0f
		q += 4
		for x := int32(1); x < t.oldWidth; x++ {
			if x&mask == 0 {
				mode = t.pix[q+1] & 0x0f
				q += 4
			}
			var leftChoice bool
			if mode == 11 {
				var ldist, tdist int32
				for k := int32(0); k < 4; k++ {
					ldist += abs(int32(pix[int32(top)-4+k]) - int32(pix[int32(top)+k]))
					tdist += abs(int32(pix[int32(top)-4+k]) - int32(pix[p-4+k]))
				}
				leftChoice = ldist < tdist
			}
			for c := int32(0); c < 4; c++ {
				var pred uint8
				l, tt, tl, tr := pix[p-4+c], pix[int32(top)+c], pix[int32(top)-4+c], pix[int32(top)+4+c]
				switch mode {
				case 0:
					if c == 3 {
						pred = 255
					}
				case 1:
					pred = l
				case 2:
					pred = tt
				case 3:
					pred = tr
				case 4:
					pred = tl
				case 5:
					pred = avg2(avg2(l, tr), tt)
				case 6:
					pred = avg2(l, tl)
				case 7:
					pred = avg2(l, tt)
				case 8:
					pred = avg2(tl, tt)
				case 9:
					pred = avg2(tt, tr)
				case 10:
					pred = avg2(avg2(l, tl), avg2(tt, tr))
				case 11:
					if leftChoice {
						pred = l
					} else {
						pred = tt
					}
				case 12:
					pred = clampAddSubtractFull(l, tt, tl)
				case 13:
					pred = clampAddSubtractHalf(avg2(l, tt), tl)
				}
				pix[p+c] += pred
			}
			p += 4
			top += 4
		}
	}
	return pix
}
func inverseCrossColor(t *transform, pix []byte, h int32) []byte {
	var greenToRed, greenToBlue, redToBlue int32
	p, mask, tilesPerRow := int32(0), int32(1)<<t.bits-1, nTiles(t.oldWidth, t.bits)
	for y := int32(0); y < h; y++ {
		q := 4 * (y >> t.bits) * tilesPerRow
		for x := int32(0); x < t.oldWidth; x++ {
			if x&mask == 0 {
				redToBlue = int32(int8(t.pix[q]))
				greenToBlue = int32(int8(t.pix[q+1]))
				greenToRed = int32(int8(t.pix[q+2]))
				q += 4
			}
			red, green, blue := pix[p], pix[p+1], pix[p+2]
			red += uint8(uint32(greenToRed*int32(int8(green))) >> 5)
			blue += uint8(uint32(greenToBlue*int32(int8(green))) >> 5)
			blue += uint8(uint32(redToBlue*int32(int8(red))) >> 5)
			pix[p] = red
			pix[p+2] = blue
			p += 4
		}
	}
	return pix
}
func inverseSubtractGreen(t *transform, pix []byte, h int32) []byte {
	for p := 0; p < len(pix); p += 4 {
		g := pix[p+1]
		pix[p] += g
		pix[p+2] += g
	}
	return pix
}
func inverseColorIndexing(t *transform, pix []byte, h int32) []byte {
	if t.bits == 0 {
		for p := 0; p < len(pix); p += 4 {
			i := 4 * uint32(pix[p+1])
			copy(pix[p:p+4], t.pix[i:i+4])
		}
		return pix
	}
	var vMask uint32
	var xMask int32
	bitsPerPixel := uint32(8 >> t.bits)
	switch t.bits {
	case 1:
		vMask, xMask = 0x0f, 1
	case 2:
		vMask, xMask = 0x03, 3
	case 3:
		vMask, xMask = 0x01, 7
	}
	d, p, v, dst := 0, 0, uint32(0), make([]byte, 4*t.oldWidth*h)
	for y := int32(0); y < h; y++ {
		for x := int32(0); x < t.oldWidth; x++ {
			if x&xMask == 0 {
				v = uint32(pix[p+1])
				p += 4
			}
			i := 4 * (v & vMask)
			copy(dst[d:d+4], t.pix[i:i+4])
			d += 4
			v >>= bitsPerPixel
		}
	}
	return dst
}
func abs(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}
func avg2(a, b uint8) uint8 { return uint8((int32(a) + int32(b)) / 2) }
func clampAddSubtractFull(a, b, c uint8) uint8 {
	x := int32(a) + int32(b) - int32(c)
	if x < 0 {
		return 0
	}
	if x > 255 {
		return 255
	}
	return uint8(x)
}
func clampAddSubtractHalf(a, b uint8) uint8 {
	x := int32(a) + (int32(a)-int32(b))/2
	if x < 0 {
		return 0
	}
	if x > 255 {
		return 255
	}
	return uint8(x)
}
