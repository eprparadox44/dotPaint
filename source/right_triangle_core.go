package main

// rightTriangleLegCorner returns the axis-aligned corner added to a straight
// hypotenuse. The drawing order is start -> horizontal corner -> end, so mouse
// down and mouse up remain the two hypotenuse endpoints and neither is the
// right-angle vertex except in a degenerate horizontal/vertical drag.
func rightTriangleLegCorner(x0, y0, x1, y1 int) (x, y int) {
	return x1, y0
}
