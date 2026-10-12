package main

import "math"

func fieldPolylineArcLength(points []fieldVec3) float64 {
	total := 0.0
	for i := 1; i < len(points); i++ {
		total += points[i].sub(points[i-1]).len()
	}
	return total
}

// fieldResamplePolylineByArc makes visual spacing independent of RK45's local
// step-size changes.  It never smooths across the physical path: every output
// point is linearly interpolated on an accepted RK45 segment.
func fieldResamplePolylineByArc(points []fieldVec3, spacing float64) []fieldVec3 {
	if len(points) < 3 || spacing <= 1e-8 {
		return append([]fieldVec3(nil), points...)
	}
	total := fieldPolylineArcLength(points)
	if total <= spacing*1.25 {
		return append([]fieldVec3(nil), points...)
	}
	out := make([]fieldVec3, 0, int(total/spacing)+2)
	out = append(out, points[0])
	target := spacing
	travelled := 0.0
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		segment := b.sub(a)
		length := segment.len()
		if length <= 1e-12 {
			continue
		}
		for target <= travelled+length+1e-12 {
			t := fieldClamp((target-travelled)/length, 0, 1)
			p := a.add(segment.mul(t))
			if p.sub(out[len(out)-1]).len2() > 1e-14 {
				out = append(out, p)
			}
			target += spacing
		}
		travelled += length
	}
	last := points[len(points)-1]
	if last.sub(out[len(out)-1]).len() > spacing*.18 {
		out = append(out, last)
	} else {
		out[len(out)-1] = last
	}
	return out
}

func fieldArrowIndexAtArcFraction(points []fieldVec3, fraction float64) int {
	if len(points) < 2 {
		return 0
	}
	fraction = fieldClamp(fraction, .05, .95)
	total := fieldPolylineArcLength(points)
	if total <= 1e-12 {
		return len(points) / 2
	}
	target, travelled := total*fraction, 0.0
	for i := 1; i < len(points); i++ {
		travelled += points[i].sub(points[i-1]).len()
		if travelled >= target {
			return i
		}
	}
	return len(points) - 1
}

func fieldElectricRenderSpacing(scene fieldScene) float64 {
	return fieldClamp(fieldSceneSpan(scene)/75, .028, .060)
}

func fieldRefineElectricLine(scene fieldScene, points []fieldVec3) fieldLine {
	refined := fieldResamplePolylineByArc(points, fieldElectricRenderSpacing(scene))
	return fieldLine{Points: refined, Arrow: fieldArrowIndexAtArcFraction(refined, .58), Sign: 1}
}

func fieldMagneticRenderSpacing(scene fieldScene) float64 {
	return fieldClamp(fieldSceneSpan(scene)/80, .026, .055)
}

func fieldRefineMagneticLineSpacing(points []fieldVec3, closed bool, spacing float64) fieldLine {
	refined := fieldResamplePolylineByArc(points, spacing)
	fraction := .58
	if closed {
		fraction = .61
	}
	return fieldLine{Points: refined, Arrow: fieldArrowIndexAtArcFraction(refined, fraction), Sign: 1, Closed: closed}
}

func fieldEqualFluxPolarAngle(t, minimumDegrees, maximumDegrees float64) float64 {
	t = fieldClamp(t, 0, 1)
	lo, hi := minimumDegrees*math.Pi/180, maximumDegrees*math.Pi/180
	a := math.Sin(lo)
	b := math.Sin(hi)
	return math.Asin(math.Sqrt(a*a + (b*b-a*a)*t))
}

func fieldEqualSolidAnglePolarAngle(t, minimumDegrees, maximumDegrees float64) float64 {
	t = fieldClamp(t, 0, 1)
	lo, hi := minimumDegrees*math.Pi/180, maximumDegrees*math.Pi/180
	return math.Acos(math.Cos(lo) + (math.Cos(hi)-math.Cos(lo))*t)
}
