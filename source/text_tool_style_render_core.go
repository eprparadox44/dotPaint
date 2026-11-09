package main

import (
	"math"
	"strconv"
	"strings"
)

// textScriptPointSize is used by the actual GDI renderer, not only by the
// hidden Rich Edit layout mirror.  67% is conventional for super/subscripts.
func textScriptPointSize(pt int, mode int8) int {
	pt = max(pt, 1)
	if mode == 0 {
		return pt
	}
	return max(1, int(math.Round(float64(pt)*0.67)))
}

// Both the native-canvas preview and the final raster use the same origin.
func textScriptTopOffset(baseHeight, glyphHeight int, mode int8) int {
	if mode != -1 {
		return 0
	}
	return max(1, (baseHeight-glyphHeight)+max(1, baseHeight/9))
}

// parseTextPointSize accepts every integer 1..100 (as requested) while
// retaining the previous 101..512 expert-size range.
func parseTextPointSize(value string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	return n, err == nil && n >= 1 && n <= 512
}
