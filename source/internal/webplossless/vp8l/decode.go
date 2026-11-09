// Derivative of golang.org/x/image/vp8l (BSD-3-Clause, The Go Authors).
package vp8l

import (
	"bufio"
	"errors"
	"image"
	"image/color"
	"io"
)

var (
	errInvalidCodeLengths = errors.New("vp8l: invalid code lengths")
	errInvalidHuffmanTree = errors.New("vp8l: invalid Huffman tree")
)

const colorCacheMultiplier = 0x1e35a7bd

var distanceMapTable = [120]uint8{
	0x18, 0x07, 0x17, 0x19, 0x28, 0x06, 0x27, 0x29, 0x16, 0x1a, 0x26, 0x2a, 0x38, 0x05, 0x37, 0x39, 0x15, 0x1b, 0x36, 0x3a,
	0x25, 0x2b, 0x48, 0x04, 0x47, 0x49, 0x14, 0x1c, 0x35, 0x3b, 0x46, 0x4a, 0x24, 0x2c, 0x58, 0x45, 0x4b, 0x34, 0x3c, 0x03,
	0x57, 0x59, 0x13, 0x1d, 0x56, 0x5a, 0x23, 0x2d, 0x44, 0x4c, 0x55, 0x5b, 0x33, 0x3d, 0x68, 0x02, 0x67, 0x69, 0x12, 0x1e,
	0x66, 0x6a, 0x22, 0x2e, 0x54, 0x5c, 0x43, 0x4d, 0x65, 0x6b, 0x32, 0x3e, 0x78, 0x01, 0x77, 0x79, 0x53, 0x5d, 0x11, 0x1f,
	0x64, 0x6c, 0x42, 0x4e, 0x76, 0x7a, 0x21, 0x2f, 0x75, 0x7b, 0x31, 0x3f, 0x63, 0x6d, 0x52, 0x5e, 0x00, 0x74, 0x7c, 0x41,
	0x4f, 0x10, 0x20, 0x62, 0x6e, 0x30, 0x73, 0x7d, 0x51, 0x5f, 0x40, 0x72, 0x7e, 0x61, 0x6f, 0x50, 0x71, 0x7f, 0x60, 0x70,
}

func distanceMap(w int32, code uint32) int32 {
	if int32(code) > int32(len(distanceMapTable)) {
		return int32(code) - int32(len(distanceMapTable))
	}
	c := int32(distanceMapTable[code-1])
	y := c >> 4
	x := 8 - c&0xf
	if d := y*w + x; d >= 1 {
		return d
	}
	return 1
}

type decoder struct {
	r           io.ByteReader
	bits, nBits uint32
}

func (d *decoder) read(n uint32) (uint32, error) {
	for d.nBits < n {
		c, err := d.r.ReadByte()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return 0, err
		}
		d.bits |= uint32(c) << d.nBits
		d.nBits += 8
	}
	u := d.bits & (1<<n - 1)
	d.bits >>= n
	d.nBits -= n
	return u, nil
}
func (d *decoder) decodeTransform(w, h int32) (t transform, newWidth int32, err error) {
	t.oldWidth = w
	t.transformType, err = d.read(2)
	if err != nil {
		return transform{}, 0, err
	}
	switch t.transformType {
	case transformTypePredictor, transformTypeCrossColor:
		t.bits, err = d.read(3)
		if err != nil {
			return transform{}, 0, err
		}
		t.bits += 2
		t.pix, err = d.decodePix(nTiles(w, t.bits), nTiles(h, t.bits), 0, false)
		if err != nil {
			return transform{}, 0, err
		}
	case transformTypeSubtractGreen:
	case transformTypeColorIndexing:
		nColors, e := d.read(8)
		if e != nil {
			return transform{}, 0, e
		}
		nColors++
		t.bits = 0
		switch {
		case nColors <= 2:
			t.bits = 3
		case nColors <= 4:
			t.bits = 2
		case nColors <= 16:
			t.bits = 1
		}
		w = nTiles(w, t.bits)
		pix, e := d.decodePix(int32(nColors), 1, 4*256, false)
		if e != nil {
			return transform{}, 0, e
		}
		for p := 4; p < len(pix); p += 4 {
			pix[p] += pix[p-4]
			pix[p+1] += pix[p-3]
			pix[p+2] += pix[p-2]
			pix[p+3] += pix[p-1]
		}
		t.pix = pix[:4*256]
	}
	return t, w, nil
}

const repeatsCodeLength = 16

var (
	codeLengthCodeOrder = [19]uint8{17, 18, 0, 1, 2, 3, 4, 5, 16, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	repeatBits          = [3]uint8{2, 3, 7}
	repeatOffsets       = [3]uint8{3, 3, 11}
)

func (d *decoder) decodeCodeLengths(dst []uint32, lengths []uint32) error {
	h := hTree{}
	if err := h.build(lengths); err != nil {
		return err
	}
	maxSymbol := len(dst)
	use, e := d.read(1)
	if e != nil {
		return e
	}
	if use != 0 {
		n, e := d.read(3)
		if e != nil {
			return e
		}
		n = 2 + 2*n
		ms, e := d.read(n)
		if e != nil {
			return e
		}
		maxSymbol = int(ms) + 2
		if maxSymbol > len(dst) {
			return errInvalidCodeLengths
		}
	}
	prev := uint32(8)
	for symbol := 0; symbol < len(dst); {
		if maxSymbol == 0 {
			break
		}
		maxSymbol--
		length, e := h.next(d)
		if e != nil {
			return e
		}
		if length < repeatsCodeLength {
			dst[symbol] = length
			symbol++
			if length != 0 {
				prev = length
			}
			continue
		}
		repeat, e := d.read(uint32(repeatBits[length-repeatsCodeLength]))
		if e != nil {
			return e
		}
		repeat += uint32(repeatOffsets[length-repeatsCodeLength])
		if symbol+int(repeat) > len(dst) {
			return errInvalidCodeLengths
		}
		cl := uint32(0)
		if length == 16 {
			cl = prev
		}
		for ; repeat > 0; repeat-- {
			dst[symbol] = cl
			symbol++
		}
	}
	return nil
}
func (d *decoder) decodeHuffmanTree(h *hTree, alphabet uint32) error {
	simple, e := d.read(1)
	if e != nil {
		return e
	}
	if simple != 0 {
		n, e := d.read(1)
		if e != nil {
			return e
		}
		n++
		first, e := d.read(1)
		if e != nil {
			return e
		}
		first = 7*first + 1
		var symbols [2]uint32
		symbols[0], e = d.read(first)
		if e != nil {
			return e
		}
		if n == 2 {
			symbols[1], e = d.read(8)
			if e != nil {
				return e
			}
		}
		return h.buildSimple(n, symbols, alphabet)
	}
	n, e := d.read(4)
	if e != nil {
		return e
	}
	n += 4
	if int(n) > len(codeLengthCodeOrder) {
		return errInvalidHuffmanTree
	}
	lengths := [len(codeLengthCodeOrder)]uint32{}
	for i := uint32(0); i < n; i++ {
		lengths[codeLengthCodeOrder[i]], e = d.read(3)
		if e != nil {
			return e
		}
	}
	codes := make([]uint32, alphabet)
	if e = d.decodeCodeLengths(codes, lengths[:]); e != nil {
		return e
	}
	return h.build(codes)
}

const (
	huffGreen    = 0
	huffRed      = 1
	huffBlue     = 2
	huffAlpha    = 3
	huffDistance = 4
	nHuff        = 5
)

type hGroup [nHuff]hTree

func (d *decoder) decodeHuffmanGroups(w, h int32, top bool, ccBits uint32) (hGroups []hGroup, hPix []byte, hBits uint32, err error) {
	maxGroup := 0
	if top {
		meta, e := d.read(1)
		if e != nil {
			return nil, nil, 0, e
		}
		if meta != 0 {
			hBits, e = d.read(3)
			if e != nil {
				return nil, nil, 0, e
			}
			hBits += 2
			hPix, e = d.decodePix(nTiles(w, hBits), nTiles(h, hBits), 0, false)
			if e != nil {
				return nil, nil, 0, e
			}
			for p := 0; p < len(hPix); p += 4 {
				i := int(hPix[p])<<8 | int(hPix[p+1])
				if maxGroup < i {
					maxGroup = i
				}
			}
		}
	}
	hGroups = make([]hGroup, maxGroup+1)
	for i := range hGroups {
		for j, sz := range alphabetSizes {
			if j == 0 && ccBits > 0 {
				sz += 1 << ccBits
			}
			if e := d.decodeHuffmanTree(&hGroups[i][j], sz); e != nil {
				return nil, nil, 0, e
			}
		}
	}
	return hGroups, hPix, hBits, nil
}

const (
	nLiteralCodes  = 256
	nLengthCodes   = 24
	nDistanceCodes = 40
)

var alphabetSizes = [nHuff]uint32{nLiteralCodes + nLengthCodes, nLiteralCodes, nLiteralCodes, nLiteralCodes, nDistanceCodes}

func (d *decoder) decodePix(w, h, minCap int32, top bool) ([]byte, error) {
	ccBits, ccShift := uint32(0), uint32(0)
	var ccEntries []uint32
	use, e := d.read(1)
	if e != nil {
		return nil, e
	}
	if use != 0 {
		ccBits, e = d.read(4)
		if e != nil {
			return nil, e
		}
		if ccBits < 1 || ccBits > 11 {
			return nil, errors.New("vp8l: invalid color cache parameters")
		}
		ccShift = 32 - ccBits
		ccEntries = make([]uint32, 1<<ccBits)
	}
	groups, hPix, hBits, e := d.decodeHuffmanGroups(w, h, top, ccBits)
	if e != nil {
		return nil, e
	}
	hMask, tilesPerRow := int32(0), int32(0)
	if hBits != 0 {
		hMask, tilesPerRow = 1<<hBits-1, nTiles(w, hBits)
	}
	if minCap < 4*w*h {
		minCap = 4 * w * h
	}
	pix := make([]byte, 4*w*h, minCap)
	p, cachedP := 0, 0
	x, y := int32(0), int32(0)
	hg, lookup := &groups[0], hMask != 0
	for p < len(pix) {
		if lookup {
			i := 4 * (tilesPerRow*(y>>hBits) + (x >> hBits))
			hg = &groups[uint32(hPix[i])<<8|uint32(hPix[i+1])]
		}
		green, e := hg[huffGreen].next(d)
		if e != nil {
			return nil, e
		}
		switch {
		case green < nLiteralCodes:
			red, e := hg[huffRed].next(d)
			if e != nil {
				return nil, e
			}
			blue, e := hg[huffBlue].next(d)
			if e != nil {
				return nil, e
			}
			alpha, e := hg[huffAlpha].next(d)
			if e != nil {
				return nil, e
			}
			pix[p], pix[p+1], pix[p+2], pix[p+3] = byte(red), byte(green), byte(blue), byte(alpha)
			p += 4
			x++
			if x == w {
				x, y = 0, y+1
			}
			lookup = hMask != 0 && x&hMask == 0
		case green < nLiteralCodes+nLengthCodes:
			length, e := d.lz77Param(green - nLiteralCodes)
			if e != nil {
				return nil, e
			}
			distSym, e := hg[huffDistance].next(d)
			if e != nil {
				return nil, e
			}
			distCode, e := d.lz77Param(distSym)
			if e != nil {
				return nil, e
			}
			dist := distanceMap(w, distCode)
			pEnd := p + 4*int(length)
			q := p - 4*int(dist)
			qEnd := pEnd - 4*int(dist)
			if p < 0 || len(pix) < pEnd || q < 0 || len(pix) < qEnd {
				return nil, errors.New("vp8l: invalid LZ77 parameters")
			}
			for ; p < pEnd; p, q = p+1, q+1 {
				pix[p] = pix[q]
			}
			x += int32(length)
			for x >= w {
				x, y = x-w, y+1
			}
			lookup = hMask != 0
		default:
			for ; cachedP < p; cachedP += 4 {
				argb := uint32(pix[cachedP])<<16 | uint32(pix[cachedP+1])<<8 | uint32(pix[cachedP+2]) | uint32(pix[cachedP+3])<<24
				ccEntries[(argb*colorCacheMultiplier)>>ccShift] = argb
			}
			green -= nLiteralCodes + nLengthCodes
			if int(green) >= len(ccEntries) {
				return nil, errors.New("vp8l: invalid color cache index")
			}
			argb := ccEntries[green]
			pix[p], pix[p+1], pix[p+2], pix[p+3] = byte(argb>>16), byte(argb>>8), byte(argb), byte(argb>>24)
			p += 4
			x++
			if x == w {
				x, y = 0, y+1
			}
			lookup = hMask != 0 && x&hMask == 0
		}
	}
	return pix, nil
}
func (d *decoder) lz77Param(symbol uint32) (uint32, error) {
	if symbol < 4 {
		return symbol + 1, nil
	}
	bits := (symbol - 2) >> 1
	offset := (2 + symbol&1) << bits
	n, e := d.read(bits)
	if e != nil {
		return 0, e
	}
	return offset + n + 1, nil
}
func decodeHeader(r io.Reader) (d *decoder, w, h int32, err error) {
	rr, ok := r.(io.ByteReader)
	if !ok {
		rr = bufio.NewReader(r)
	}
	d = &decoder{r: rr}
	magic, e := d.read(8)
	if e != nil {
		return nil, 0, 0, e
	}
	if magic != 0x2f {
		return nil, 0, 0, errors.New("vp8l: invalid header")
	}
	width, e := d.read(14)
	if e != nil {
		return nil, 0, 0, e
	}
	width++
	height, e := d.read(14)
	if e != nil {
		return nil, 0, 0, e
	}
	height++
	_, e = d.read(1)
	if e != nil {
		return nil, 0, 0, e
	}
	version, e := d.read(3)
	if e != nil {
		return nil, 0, 0, e
	}
	if version != 0 {
		return nil, 0, 0, errors.New("vp8l: invalid version")
	}
	return d, int32(width), int32(height), nil
}
func DecodeConfig(r io.Reader) (image.Config, error) {
	_, w, h, e := decodeHeader(r)
	if e != nil {
		return image.Config{}, e
	}
	return image.Config{ColorModel: color.NRGBAModel, Width: int(w), Height: int(h)}, nil
}
func Decode(r io.Reader) (image.Image, error) {
	d, w, h, e := decodeHeader(r)
	if e != nil {
		return nil, e
	}
	var n int
	var transforms [nTransformTypes]transform
	var seen [nTransformTypes]bool
	originalW := w
	for {
		more, e := d.read(1)
		if e != nil {
			return nil, e
		}
		if more == 0 {
			break
		}
		t, nw, e := d.decodeTransform(w, h)
		if e != nil {
			return nil, e
		}
		w = nw
		if seen[t.transformType] {
			return nil, errors.New("vp8l: repeated transform")
		}
		seen[t.transformType] = true
		transforms[n] = t
		n++
	}
	pix, e := d.decodePix(w, h, 0, true)
	if e != nil {
		return nil, e
	}
	for i := n - 1; i >= 0; i-- {
		t := &transforms[i]
		pix = inverseTransforms[t.transformType](t, pix, h)
	}
	return &image.NRGBA{Pix: pix, Stride: 4 * int(originalW), Rect: image.Rect(0, 0, int(originalW), int(h))}, nil
}
