package main

// mdFaceBinRange is an inclusive projected face footprint in grid cells.
// Invalid/degenerate faces remain absent, matching the previous append bins.
type mdFaceBinRange struct {
	x0, x1, z0, z1 int
	valid          bool
}

// mdPackProjectedFaceBins makes exactly one integer backing allocation for all
// projected face indices instead of growing hundreds of small cell slices.
// Face iteration order in each cell is bit-for-bit unchanged.
func mdPackProjectedFaceBins(bounds []mdFaceBinRange, grid int) [][]int {
	if grid < 1 {
		return nil
	}
	counts := make([]int, grid*grid)
	for _, b := range bounds {
		if !b.valid {
			continue
		}
		for z := b.z0; z <= b.z1; z++ {
			for x := b.x0; x <= b.x1; x++ {
				counts[z*grid+x]++
			}
		}
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	data := make([]int, total)
	bins := make([][]int, grid*grid)
	offset := 0
	for i, n := range counts {
		bins[i] = data[offset : offset : offset+n]
		offset += n
	}
	for fi, b := range bounds {
		if !b.valid {
			continue
		}
		for z := b.z0; z <= b.z1; z++ {
			for x := b.x0; x <= b.x1; x++ {
				idx := z*grid + x
				bins[idx] = append(bins[idx], fi)
			}
		}
	}
	return bins
}
