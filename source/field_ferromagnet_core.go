package main

import "math"

// fieldBasisFromAxis returns a right-handed basis whose local +X axis is the
// supplied magnetic moment direction.  It is used by induced soft-iron bodies,
// whose magnetisation follows the external field rather than a fixed N/S axis.
func fieldBasisFromAxis(axis fieldVec3) (fieldVec3, fieldVec3, fieldVec3) {
	ex := axis.norm()
	if ex.len2() < 1e-12 {
		ex = fieldVec3{X: 1}
	}
	reference := fieldVec3{Y: 1}
	if math.Abs(ex.dot(reference)) > .86 {
		reference = fieldVec3{Z: 1}
	}
	ez := ex.cross(reference).norm()
	ey := ez.cross(ex).norm()
	return ex, ey, ez
}

// fieldFerromagnetCurrentSegments approximates an isotropic, high-permeability
// sphere by a compact family of bound-current loops.  Its moment is aligned to
// the external B field and saturates in fieldBuildMagneticModel.  It is a
// qualitative magnetostatic soft-iron model: no hysteresis and no remanence.
func fieldFerromagnetCurrentSegments(o fieldObject, axis fieldVec3, strength float64) []fieldMagneticCurrentSegment {
	if strength <= 1e-10 || o.Radius <= 1e-6 {
		return nil
	}
	const slices = 7
	const circum = 24
	ex, ey, ez := fieldBasisFromAxis(axis)
	center := fieldObjectCenter(o)
	radius := fieldClamp(o.Radius, .25, 1.25)

	loopRadii := make([]float64, slices)
	areaSum := 0.0
	for i := 0; i < slices; i++ {
		x := -radius*.78 + 1.56*radius*(float64(i)+.5)/slices
		r := math.Sqrt(math.Max(radius*radius-x*x, radius*radius*.08)) * .82
		loopRadii[i] = r
		areaSum += math.Pi * r * r
	}
	// The scale is intentionally bounded and dimensionless.  It creates the
	// expected field concentration without overwhelming permanent sources.
	targetMoment := strength * radius * radius * radius * 2.8
	loopCurrent := targetMoment / math.Max(areaSum, 1e-9)
	out := make([]fieldMagneticCurrentSegment, 0, slices*circum)
	for i, loopRadius := range loopRadii {
		x := -radius*.78 + 1.56*radius*(float64(i)+.5)/slices
		loopCenter := center.add(ex.mul(x))
		arc := 2 * math.Pi * loopRadius / circum
		soft := math.Max(.006, arc*.30)
		for j := 0; j < circum; j++ {
			a0 := 2 * math.Pi * float64(j) / circum
			a1 := 2 * math.Pi * float64(j+1) / circum
			p0 := loopCenter.add(ey.mul(loopRadius * math.Cos(a0))).add(ez.mul(loopRadius * math.Sin(a0)))
			p1 := loopCenter.add(ey.mul(loopRadius * math.Cos(a1))).add(ez.mul(loopRadius * math.Sin(a1)))
			out = append(out, fieldMagneticCurrentSegment{Mid: p0.add(p1).mul(.5), DL: p1.sub(p0), I: loopCurrent, Soft: soft})
		}
	}
	return out
}
