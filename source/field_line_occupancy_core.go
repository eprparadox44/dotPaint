package main

import "math"

type fieldScreenPoint struct {
	X, Y float64
}

type fieldScreenLineCandidate struct {
	Points []fieldScreenPoint
	Group  int
	Weight float64
	Closed bool
}

type fieldScreenLineSelectionStats struct {
	Input, Selected int
	Groups          int
	NearRejected    int
	EmptyRejected   int
	CellSize        int
	HaloCells       int
}

// fieldSelectScreenDiverseLines greedily keeps physically computed candidates
// that add new screen coverage. At least one candidate per physical family is
// preserved while capacity remains. A one-cell halo around every chosen line
// makes almost coincident curves compete even when they fall on adjacent cells.
func fieldSelectScreenDiverseLines(candidates []fieldScreenLineCandidate, width, height, cellSize, maxLines int) []int {
	selected, _ := fieldSelectScreenDiverseLinesDetailed(candidates, width, height, cellSize, maxLines)
	return selected
}

func fieldSelectScreenDiverseLinesDetailed(candidates []fieldScreenLineCandidate, width, height, cellSize, maxLines int) ([]int, fieldScreenLineSelectionStats) {
	stats := fieldScreenLineSelectionStats{Input: len(candidates), CellSize: cellSize, HaloCells: 1}
	if len(candidates) == 0 || width <= 0 || height <= 0 || cellSize <= 0 || maxLines <= 0 {
		return nil, stats
	}
	if maxLines > len(candidates) {
		maxLines = len(candidates)
	}
	cols := (width + cellSize - 1) / cellSize
	rows := (height + cellSize - 1) / cellSize
	cores := make([]map[int]struct{}, len(candidates))
	halos := make([]map[int]struct{}, len(candidates))
	for i, candidate := range candidates {
		cores[i] = fieldScreenLineCells(candidate.Points, width, height, cellSize, cols, rows)
		halos[i] = fieldScreenCellHalo(cores[i], cols, rows, stats.HaloCells)
		if len(cores[i]) == 0 {
			stats.EmptyRejected++
		}
	}

	selected := make([]int, 0, maxLines)
	chosen := make([]bool, len(candidates))
	occupiedNear := make(map[int]uint16)

	groups := make([]int, 0)
	seenGroup := make(map[int]bool)
	for i, candidate := range candidates {
		if len(cores[i]) == 0 || seenGroup[candidate.Group] {
			continue
		}
		seenGroup[candidate.Group] = true
		groups = append(groups, candidate.Group)
	}
	stats.Groups = len(groups)
	for _, group := range groups {
		if len(selected) >= maxLines {
			break
		}
		best, bestScore := -1, math.Inf(-1)
		for i, candidate := range candidates {
			if chosen[i] || candidate.Group != group || len(cores[i]) == 0 {
				continue
			}
			score := fieldCoverageScore(cores[i], occupiedNear, candidate.Weight)
			if score > bestScore {
				best, bestScore = i, score
			}
		}
		if best >= 0 {
			fieldSelectCoverageCandidate(best, halos, chosen, &selected, occupiedNear)
		}
	}

	for len(selected) < maxLines {
		best, bestScore := -1, math.Inf(-1)
		for i, candidate := range candidates {
			if chosen[i] || len(cores[i]) == 0 {
				continue
			}
			score := fieldCoverageScore(cores[i], occupiedNear, candidate.Weight)
			if score > bestScore {
				best, bestScore = i, score
			}
		}
		if best < 0 || bestScore <= 0 {
			break
		}
		fieldSelectCoverageCandidate(best, halos, chosen, &selected, occupiedNear)
	}
	stats.Selected = len(selected)
	stats.NearRejected = max(0, len(candidates)-stats.EmptyRejected-stats.Selected)
	return selected, stats
}

func fieldScreenLineCells(points []fieldScreenPoint, width, height, cellSize, cols, rows int) map[int]struct{} {
	out := make(map[int]struct{})
	mark := func(p fieldScreenPoint) {
		if !fieldFinite(p.X) || !fieldFinite(p.Y) || p.X < 0 || p.Y < 0 || p.X >= float64(width) || p.Y >= float64(height) {
			return
		}
		cx, cy := int(p.X)/cellSize, int(p.Y)/cellSize
		if cx >= 0 && cx < cols && cy >= 0 && cy < rows {
			out[cy*cols+cx] = struct{}{}
		}
	}
	if len(points) == 1 {
		mark(points[0])
	}
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		distance := math.Hypot(b.X-a.X, b.Y-a.Y)
		steps := int(math.Ceil(distance / math.Max(1, float64(cellSize)/2)))
		if steps < 1 {
			steps = 1
		}
		for j := 0; j <= steps; j++ {
			t := float64(j) / float64(steps)
			mark(fieldScreenPoint{X: a.X + (b.X-a.X)*t, Y: a.Y + (b.Y-a.Y)*t})
		}
	}
	return out
}

func fieldScreenCellHalo(core map[int]struct{}, cols, rows, radius int) map[int]struct{} {
	if radius <= 0 {
		out := make(map[int]struct{}, len(core))
		for cell := range core {
			out[cell] = struct{}{}
		}
		return out
	}
	out := make(map[int]struct{}, len(core)*4)
	for cell := range core {
		cx, cy := cell%cols, cell/cols
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				x, y := cx+dx, cy+dy
				if x >= 0 && x < cols && y >= 0 && y < rows {
					out[y*cols+x] = struct{}{}
				}
			}
		}
	}
	return out
}

func fieldCoverageScore(core map[int]struct{}, occupiedNear map[int]uint16, weight float64) float64 {
	if weight <= 0 {
		weight = 1
	}
	coverage := len(core)
	if coverage == 0 {
		return math.Inf(-1)
	}
	newCells, overlapPenalty := 0, 0.0
	for cell := range core {
		count := occupiedNear[cell]
		if count == 0 {
			newCells++
		} else {
			overlapPenalty += 1 + math.Min(1.2, float64(count)*.22)
		}
	}
	novelty := float64(newCells) / float64(coverage)
	if len(occupiedNear) > 0 && novelty < .05 {
		return weight * (float64(newCells) - overlapPenalty*1.25)
	}
	return weight * (float64(newCells)*3.2 + float64(coverage)*.035 - overlapPenalty*.68)
}

func fieldSelectCoverageCandidate(index int, halos []map[int]struct{}, chosen []bool, selected *[]int, occupiedNear map[int]uint16) {
	chosen[index] = true
	*selected = append(*selected, index)
	for cell := range halos[index] {
		if occupiedNear[cell] < ^uint16(0) {
			occupiedNear[cell]++
		}
	}
}
