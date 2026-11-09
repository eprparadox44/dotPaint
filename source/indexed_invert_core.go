package main

// Indexed inversion is a palette operation, not a 24-bit temporary colour.
// Build the complement lookup once at the start of a stroke or shape operation.
// The active palette never changes while an active mouse drag is captured.
func indexedInvertLookup(palette []uint32) []byte {
	if len(palette) == 0 || len(palette) > 256 {
		return nil
	}
	out := make([]byte, len(palette))
	for i, c := range palette {
		out[i] = byte(nearestIndexedColor(^byte(c), ^byte(c>>8), ^byte(c>>16), palette))
	}
	return out
}

// No per-pixel palette search when an exact palette index is known. Keep the
// indexed identity map synchronized with the actual stored BGRA pixels.
func applyIndexedInvertPixel(pix []byte, pixelIndex int, ids []byte, palette []uint32, lookup []byte) {
	if pixelIndex < 0 || pixelIndex*4+3 >= len(pix) || len(lookup) != len(palette) || len(palette) == 0 {
		return
	}
	i := pixelIndex * 4
	id := 0
	if len(ids) == len(pix)/4 && int(ids[pixelIndex]) < len(lookup) &&
		palette[ids[pixelIndex]]&0xFFFFFF == (uint32(pix[i+2])|uint32(pix[i+1])<<8|uint32(pix[i])<<16) {
		id = int(ids[pixelIndex])
	} else {
		id = nearestIndexedColor(pix[i+2], pix[i+1], pix[i], palette)
	}
	newID := lookup[id]
	color := palette[int(newID)]
	pix[i+0], pix[i+1], pix[i+2] = byte(color>>16), byte(color>>8), byte(color)
	if len(ids) == len(pix)/4 {
		ids[pixelIndex] = newID
	}
}

// For clipped preview sprites, existing colors are looked up with an exact
// RGB -> index map, then with nearest-color search only as a fallback. This
// avoids an O(256) search for every pixel in the usual indexed canvas case.
func invertIndexedMask(dst, mask []byte, palette []uint32, lookup []byte) {
	if len(dst) != len(mask) || len(dst)%4 != 0 || len(palette) == 0 || len(lookup) != len(palette) {
		return
	}
	idsByColor := make(map[uint32]byte, len(palette))
	for i, color := range palette {
		if _, exists := idsByColor[color&0xFFFFFF]; !exists {
			idsByColor[color&0xFFFFFF] = byte(i)
		}
	}
	for i := 0; i < len(dst); i += 4 {
		if mask[i+3] < 128 {
			continue
		}
		key := uint32(dst[i+2]) | uint32(dst[i+1])<<8 | uint32(dst[i])<<16
		id, found := idsByColor[key]
		if !found {
			id = byte(nearestIndexedColor(dst[i+2], dst[i+1], dst[i], palette))
			idsByColor[key] = id
		}
		color := palette[lookup[id]]
		dst[i], dst[i+1], dst[i+2] = byte(color>>16), byte(color>>8), byte(color)
	}
}
