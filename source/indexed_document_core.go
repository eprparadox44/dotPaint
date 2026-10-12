package main

// An indexed document has a 16- or 256-entry colour table but can use fewer
// entries. A separate alpha channel provides transparency; it does NOT consume
// an index. In particular, a 256-colour document never silently becomes 16+1.
func indexedUsableCount(tableSize, requested int) int {
	if tableSize != 16 && tableSize != 256 {
		return 0
	}
	if requested < 1 || requested > tableSize {
		return tableSize
	}
	return requested
}

// Convert the current BGRA canvas to the nearest visible palette entries.
// The result retains alpha, including fully-transparent pixels. Cache exact
// source RGB values; a large photo need not re-scan all palette entries for
// each repeated colour. The returned index map records index identity for
// history and palette-recolour operations (alpha lives in the BGRA pixels).
func quantizeIndexedBGRA(pix []byte, pal []uint32, count int) []byte {
	count = indexedUsableCount(len(pal), count)
	if count == 0 {
		return nil
	}
	indices := make([]byte, len(pix)/4)
	cache := make(map[uint32]byte, min(len(pix)/4, 4096))
	for i := range indices {
		off := i * 4
		if pix[off+3] == 0 {
			continue
		}
		key := uint32(pix[off]) | uint32(pix[off+1])<<8 | uint32(pix[off+2])<<16
		idx, ok := cache[key]
		if !ok {
			idx = byte(nearestIndexedColor(pix[off+2], pix[off+1], pix[off], pal[:count]))
			cache[key] = idx
		}
		indices[i] = idx
		c := pal[int(idx)]
		pix[off], pix[off+1], pix[off+2] = byte(c>>16), byte(c>>8), byte(c)
	}
	return indices
}

// Recolor old indexed image by its existing indices. This is different from
// re-quantizing: a user who edits palette entry #37 expects pixels using #37
// to change with it, and history must still be able to recover old indices.
func recolorIndexedBGRA(pix []byte, indices []byte, pal []uint32, count int) {
	count = indexedUsableCount(len(pal), count)
	if count == 0 || len(indices)*4 != len(pix) {
		return
	}
	for i, index := range indices {
		off := i * 4
		if pix[off+3] == 0 {
			continue
		}
		idx := int(index)
		if idx >= count { // reducing the active count must not leave an illegal index
			idx = nearestIndexedColor(pix[off+2], pix[off+1], pix[off], pal[:count])
			indices[i] = byte(idx)
		}
		c := pal[idx]
		pix[off], pix[off+1], pix[off+2] = byte(c>>16), byte(c>>8), byte(c)
	}
}

// Recover the visual image and per-pixel index map of a saved history entry.
func restoreIndexedSnapshot(pix []byte, indices []byte, pal []uint32) {
	if len(pix)/4 != len(indices) || !validIndexedSize(len(pal)) {
		return
	}
	for i, ix := range indices {
		off := i * 4
		if pix[off+3] == 0 {
			continue
		}
		c := pal[int(ix)]
		pix[off], pix[off+1], pix[off+2] = byte(c>>16), byte(c>>8), byte(c)
	}
}

// Maintain distinct index identities across snapshots, even if two palette
// slots have equal RGB. Only remap a pixel when its displayed colour no longer
// matches its remembered index. This is a best-effort bridge for generic RGB
// tools that do not write indexed pixel IDs directly.
func indexedIndicesForSnapshot(pix []byte, pal []uint32, count int, previous []byte) []byte {
	count = indexedUsableCount(len(pal), count)
	if count == 0 {
		return nil
	}
	out := make([]byte, len(pix)/4)
	cache := make(map[uint32]byte, 256)
	for i := range out {
		off := i * 4
		if pix[off+3] == 0 {
			continue
		}
		rgb := uint32(pix[off+2]) | uint32(pix[off+1])<<8 | uint32(pix[off])<<16
		if len(previous) == len(out) && int(previous[i]) < count && pal[previous[i]] == rgb {
			out[i] = previous[i]
			continue
		}
		idx, exists := cache[rgb]
		if !exists {
			idx = byte(nearestIndexedColor(pix[off+2], pix[off+1], pix[off], pal[:count]))
			cache[rgb] = idx
		}
		out[i] = idx
	}
	return out
}
