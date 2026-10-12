package main

// Limit bitmap snapshot memory without removing the newest undo point.
// Large canvases may exceed this limit alone; in that case the newest single
// checkpoint is retained for the immediately preceding edit.
const undoSnapshotByteBudget = 256 << 20

// Given chronological snapshot sizes, return the number of oldest snapshots
// to discard. Keep at least the newest snapshot even if it exceeds budget.
func undoOldestDropCount(sizes []int, budget int) int {
	if len(sizes) < 2 {
		return 0
	}
	var total int64
	for _, size := range sizes {
		total += int64(size)
	}
	drops := 0
	for total > int64(budget) && drops < len(sizes)-1 {
		total -= int64(sizes[drops])
		drops++
	}
	return drops
}
