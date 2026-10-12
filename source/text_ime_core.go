package main

// textIMEAnchoredRange finds the replacement range in newUnits for an IME
// composition that began by replacing baseUnits[baseStart:baseEnd].  The text
// before baseStart and the suffix after baseEnd are treated as immutable.  This
// avoids ambiguous longest-prefix diffs when the composing syllable is equal to
// a neighbouring character.
func textIMEAnchoredRange(baseUnits, newUnits []uint16, baseStart, baseEnd int) (start, end int, ok bool) {
	if baseStart < 0 || baseEnd < baseStart || baseEnd > len(baseUnits) || baseStart > len(newUnits) {
		return 0, 0, false
	}
	if !uint16SliceEqual(baseUnits[:baseStart], newUnits[:baseStart]) {
		return 0, 0, false
	}
	baseSuffix := baseUnits[baseEnd:]
	newSuffixStart := len(newUnits) - len(baseSuffix)
	if newSuffixStart < baseStart || !uint16SliceEqual(baseSuffix, newUnits[newSuffixStart:]) {
		return 0, 0, false
	}
	return baseStart, newSuffixStart, true
}

func uint16SliceEqual(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
