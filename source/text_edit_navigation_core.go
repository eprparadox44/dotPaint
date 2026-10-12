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
