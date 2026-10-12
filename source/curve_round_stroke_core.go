package main

import "math"

// curveRoundNibCell is one native-canvas pixel of the curve tool's circular
// nib. DX/DY are relative to the logical curve spine pixel. Coverage is the
// maximum alpha contribution of this nib cell before overlapping stamps are
// unioned in the vector alpha mask.
type curveRoundNibCell struct {
	DX, DY   int
	Coverage float64
}

// curveRoundNibCentreOffset keeps the requested nominal diameter exact for
// both odd and even sizes. Odd nibs are centred on an integer pixel centre;
// even nibs are centred between four pixels, matching the filled-circle brush
// convention already used by dotPaint.
func curveRoundNibCentreOffset(size int) float64 {
	if size < 1 {
		size = 1
	}
	if size%2 == 0 {
		return 0.5
	}
	return 0
}

func curveRoundNibRadius(size int) float64 {
	if size < 1 {
		size = 1
	}
	return float64(size) / 2.0
}

func curveRoundNibContainsSample(sampleX, sampleY float64, size int) bool {
	offset := curveRoundNibCentreOffset(size)
	radius := curveRoundNibRadius(size)
	dx, dy := sampleX-offset, sampleY-offset
	return dx*dx+dy*dy <= radius*radius+1e-12
}

// curveRoundNibCoverageAt returns the area coverage of the pixel whose centre
// is at relative integer coordinates px,py. Non-AA curves use the pixel centre
// as a binary test. AA curves use one stable 4x4 sub-pixel lattice. The nib is
// built once per curve draw and reused at every centreline point, so thick
// curves remain responsive while all overlaps are unioned rather than blended
// repeatedly.
func curveRoundNibCoverageAt(px, py int, size int, antialias bool) float64 {
	if !antialias {
		if curveRoundNibContainsSample(float64(px), float64(py), size) {
			return 1
		}
		return 0
	}
	const samples = 4
	inside := 0
	for sy := 0; sy < samples; sy++ {
		qy := float64(py) - 0.5 + (float64(sy)+0.5)/samples
		for sx := 0; sx < samples; sx++ {
			qx := float64(px) - 0.5 + (float64(sx)+0.5)/samples
			if curveRoundNibContainsSample(qx, qy, size) {
				inside++
			}
		}
	}
	return float64(inside) / float64(samples*samples)
}

// buildCurveRoundNib creates a compact reusable circular stamp. A one-pixel
// fringe is retained for anti-aliasing; cells with zero contribution are not
// stored.
func buildCurveRoundNib(size int, antialias bool) []curveRoundNibCell {
	if size < 1 {
		size = 1
	}
	offset := curveRoundNibCentreOffset(size)
	radius := curveRoundNibRadius(size)
	left := int(math.Floor(offset-radius)) - 1
	right := int(math.Ceil(offset+radius)) + 1
	top, bottom := left, right
	cells := make([]curveRoundNibCell, 0, (right-left+1)*(bottom-top+1))
	for y := top; y <= bottom; y++ {
		for x := left; x <= right; x++ {
			coverage := curveRoundNibCoverageAt(x, y, size, antialias)
			if coverage > 0 {
				cells = append(cells, curveRoundNibCell{DX: x, DY: y, Coverage: coverage})
			}
		}
	}
	return cells
}
