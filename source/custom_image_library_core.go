package main

import "fmt"

const (
	customImageMaxSlots     = 20
	customImageMaxDimension = 300
)

func customImageSlotFilename(index int) string {
	if index < 0 || index >= customImageMaxSlots {
		return ""
	}
	return fmt.Sprintf("image_%02d.png", index+1)
}

func customImageDimensionsAllowed(w, h int) bool {
	return w >= 1 && h >= 1 && w <= customImageMaxDimension && h <= customImageMaxDimension
}

func customImageChooseSaveSlot(occupied []bool, selected int) int {
	if selected >= 0 && selected < len(occupied) && selected < customImageMaxSlots && !occupied[selected] {
		return selected
	}
	limit := len(occupied)
	if limit > customImageMaxSlots {
		limit = customImageMaxSlots
	}
	for i := 0; i < limit; i++ {
		if !occupied[i] {
			return i
		}
	}
	return -1
}
