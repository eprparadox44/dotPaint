package main

// textEditOutsideOwnedSpan reports whether a plain EDIT mutation occurred far
// enough from the active insertion-style span that the style must be considered
// stale. A one-unit step to the left is allowed for Backspace replacing the
// character immediately before the caret; real cursor movement is handled by
// the Windows message loop before typing.
func textEditOutsideOwnedSpan(editPrefix, anchor, spanEnd int) bool {
	left := anchor - 1
	if left < 0 {
		left = 0
	}
	return editPrefix < left || editPrefix > spanEnd
}

// TextBoundaryPos is a native-canvas UTF-16 insertion position shared by
// the text layout, hit-testing, selection and Home/End navigation.
type TextBoundaryPos struct {
	Index int
	X     int
	Y     int
}

// textVisualLineEdge returns the insertion index at the *visible* row edge.
// The caller supplies boundary positions calculated by the same layout engine
// that paints the glyphs, not the hidden native EDIT's 1px wrapping geometry.
func textVisualLineEdge(positions []TextBoundaryPos, caret int, toEnd bool) int {
	if len(positions) == 0 {
		return 0
	}
	if caret < 0 {
		caret = 0
	} else if caret >= len(positions) {
		caret = len(positions) - 1
	}
	y := positions[caret].Y
	best := caret
	for i, p := range positions {
		if p.Y != y {
			continue
		}
		b := positions[best]
		if (!toEnd && (p.X < b.X || (p.X == b.X && i < best))) ||
			(toEnd && (p.X > b.X || (p.X == b.X && i > best))) {
			best = i
		}
	}
	return best
}
