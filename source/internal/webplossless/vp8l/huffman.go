// Derivative of golang.org/x/image/vp8l (BSD-3-Clause, The Go Authors).
package vp8l

import "io"

type hNode struct {
	symbol   uint32
	children int32
}

const leafNode = -1
const lutSize, lutMask = 7, 1<<7 - 1

type hTree struct {
	nodes []hNode
	lut   [1 << lutSize]uint32
}

func reverseByte(v uint8) uint8 {
	v = (v >> 4) | (v << 4)
	v = ((v & 0xcc) >> 2) | ((v & 0x33) << 2)
	return ((v & 0xaa) >> 1) | ((v & 0x55) << 1)
}
func (h *hTree) insert(symbol, code, length uint32) error {
	if symbol > 0xffff || length > 0xfe {
		return errInvalidHuffmanTree
	}
	base := uint32(0)
	if length > lutSize {
		base = uint32(reverseByte(uint8((code>>(length-lutSize))&0xff))) >> (8 - lutSize)
	} else {
		base = uint32(reverseByte(uint8(code&0xff))) >> (8 - length)
		for i := 0; i < 1<<(lutSize-length); i++ {
			h.lut[base|uint32(i)<<length] = symbol<<8 | (length + 1)
		}
	}
	n := uint32(0)
	for jump := lutSize; length > 0; {
		length--
		if int(n) > len(h.nodes) {
			return errInvalidHuffmanTree
		}
		switch h.nodes[n].children {
		case leafNode:
			return errInvalidHuffmanTree
		case 0:
			if len(h.nodes) == cap(h.nodes) {
				return errInvalidHuffmanTree
			}
			h.nodes[n].children = int32(len(h.nodes))
			h.nodes = h.nodes[:len(h.nodes)+2]
		}
		n = uint32(h.nodes[n].children) + 1&(code>>length)
		jump--
		if jump == 0 && h.lut[base] == 0 {
			h.lut[base] = n << 8
		}
	}
	switch h.nodes[n].children {
	case leafNode:
	case 0:
		h.nodes[n].children = leafNode
	default:
		return errInvalidHuffmanTree
	}
	h.nodes[n].symbol = symbol
	return nil
}
func codeLengthsToCodes(lengths []uint32) ([]uint32, error) {
	maxLen := uint32(0)
	for _, c := range lengths {
		if maxLen < c {
			maxLen = c
		}
	}
	const maxAllowed = 15
	if len(lengths) == 0 || maxLen > maxAllowed {
		return nil, errInvalidHuffmanTree
	}
	hist := [maxAllowed + 1]uint32{}
	for _, c := range lengths {
		hist[c]++
	}
	curr := uint32(0)
	next := [maxAllowed + 1]uint32{}
	for i := 1; i < len(next); i++ {
		curr = (curr + hist[i-1]) << 1
		next[i] = curr
	}
	codes := make([]uint32, len(lengths))
	for i, c := range lengths {
		if c > 0 {
			codes[i] = next[c]
			next[c]++
		}
	}
	return codes, nil
}
func (h *hTree) build(lengths []uint32) error {
	var n, last uint32
	for i, c := range lengths {
		if c != 0 {
			n++
			last = uint32(i)
		}
	}
	if n == 0 {
		return errInvalidHuffmanTree
	}
	h.nodes = make([]hNode, 1, 2*n-1)
	if n == 1 {
		if len(lengths) <= int(last) {
			return errInvalidHuffmanTree
		}
		return h.insert(last, 0, 0)
	}
	codes, err := codeLengthsToCodes(lengths)
	if err != nil {
		return err
	}
	for i, c := range lengths {
		if c > 0 {
			if err := h.insert(uint32(i), codes[i], c); err != nil {
				return err
			}
		}
	}
	return nil
}
func (h *hTree) buildSimple(n uint32, symbols [2]uint32, alphabet uint32) error {
	h.nodes = make([]hNode, 1, 2*n-1)
	for i := uint32(0); i < n; i++ {
		if symbols[i] >= alphabet {
			return errInvalidHuffmanTree
		}
		if err := h.insert(symbols[i], i, n-1); err != nil {
			return err
		}
	}
	return nil
}
func (h *hTree) next(d *decoder) (uint32, error) {
	var n uint32
	if d.nBits < lutSize {
		c, err := d.r.ReadByte()
		if err != nil {
			if err == io.EOF {
				goto slowPath
			}
			return 0, err
		}
		d.bits |= uint32(c) << d.nBits
		d.nBits += 8
	}
	n = h.lut[d.bits&lutMask]
	if b := n & 0xff; b != 0 {
		b--
		d.bits >>= b
		d.nBits -= b
		return n >> 8, nil
	}
	n >>= 8
	d.bits >>= lutSize
	d.nBits -= lutSize
slowPath:
	for h.nodes[n].children != leafNode {
		if d.nBits == 0 {
			c, err := d.r.ReadByte()
			if err != nil {
				if err == io.EOF {
					err = io.ErrUnexpectedEOF
				}
				return 0, err
			}
			d.bits = uint32(c)
			d.nBits = 8
		}
		n = uint32(h.nodes[n].children) + 1&d.bits
		d.bits >>= 1
		d.nBits--
	}
	return h.nodes[n].symbol, nil
}
