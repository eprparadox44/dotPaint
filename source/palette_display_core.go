package main

// The swatch visibility preference is independent of image colour mode,
// indexed-palette capacity, active quantization count and history state.
func paletteDisplayCapacity(mode int) int {
	switch mode {
	case 16:
		return 16
	case 256:
		return 256
	default:
		return 44 // 28 classic + 16 custom slots
	}
}
func paletteDisplayClamp(mode, count int) int {
	capacity := paletteDisplayCapacity(mode)
	if count < 1 || count > capacity {
		return capacity
	}
	return count
}
func paletteDisplayColumns(mode, count int) int {
	count = paletteDisplayClamp(mode, count)
	if mode == 0 {
		if count <= 14 {
			return count
		}
		if count <= 28 {
			return 14
		}
		return 14 + min(8, count-28)
	}
	if count <= 16 {
		return min(count, 8)
	}
	return 16
}
func paletteDisplayRows(mode, count int) int {
	count = paletteDisplayClamp(mode, count)
	if mode == 0 {
		if count <= 14 {
			return 1
		}
		return 2 // Original 14×2 + custom 8×2 ordering
	}
	columns := paletteDisplayColumns(mode, count)
	return (count + columns - 1) / columns
}
func paletteDisplayPosition(mode, index, columns int) (col, row int) {
	if mode == 0 {
		if index < 28 {
			return index % 14, index / 14
		}
		i := index - 28
		return 14 + i%8, i / 8
	}
	return index % columns, index / columns
}
