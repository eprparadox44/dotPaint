package main

import "math"

func fieldMagneticStreamlinePolicy(scene fieldScene) fieldStreamlinePolicy {
	policy := fieldDefaultStreamlinePolicy()
	policy.InitialStep = .028
	policy.MinStep = .0021
	policy.MaxStep = .070
	policy.AbsTolerance = 1.5e-4
	policy.RelTolerance = 1.5e-3
	policy.MaxTurnRadians = 9 * math.Pi / 180
	policy.MaxSteps = 2100
	if scene.precise3DLines {
		policy.InitialStep = .018
		policy.MinStep = .0012
		policy.MaxStep = .040
		policy.AbsTolerance = 5e-5
		policy.RelTolerance = 5e-4
		policy.MaxTurnRadians = 5 * math.Pi / 180
		policy.MaxSteps = 3600
	}
	policy.MaxArcLength = math.Max(36, fieldSceneNumericalInfinityRadius(scene)*4)
	policy.ClosureDistance = .030
	policy.ClosureDirectionCos = math.Cos(16 * math.Pi / 180)
	policy.MinClosureSteps = 36
	policy.MinClosureLength = .55
	if scene.cancel != nil {
		policy.Cancelled = scene.cancel.Cancelled
	}
	policy.Inside = func(p fieldVec3) bool { return !fieldOutsideNumericalInfinity(scene, p) }
	return policy
}

// fieldFirstElectricBoundaryHit finds the first actual object crossing on an
// accepted RK45 segment. It preserves the old anti-tunnelling rule but clips
// the final point to a bisection-refined surface instead of an arbitrary 1/8
// sample position.
func fieldFirstElectricBoundaryHit(scene fieldScene, a, b fieldVec3) (bool, fieldVec3, int) {
	bestT := math.Inf(1)
	bestObject := -1
	for objectIndex, object := range scene.Objects {
		if !object.electric() || fieldPointInsideObject(a, object, .002) {
			continue
		}
		previousT := 0.0
		previousInside := false
		for sample := 1; sample <= 12; sample++ {
			t := float64(sample) / 12
			p := a.add(b.sub(a).mul(t))
			inside := fieldPointInsideObject(p, object, .004)
			if inside && !previousInside {
				lo, hi := previousT, t
				for iteration := 0; iteration < 22; iteration++ {
					mid := (lo + hi) * .5
					q := a.add(b.sub(a).mul(mid))
					if fieldPointInsideObject(q, object, .004) {
						hi = mid
					} else {
						lo = mid
					}
				}
				if hi < bestT {
					bestT, bestObject = hi, objectIndex
				}
				break
			}
			previousT, previousInside = t, inside
		}
	}
	if bestObject < 0 {
		return false, fieldVec3{}, -1
	}
	return true, a.add(b.sub(a).mul(bestT)), bestObject
}

func fieldSelectScreenDiverseLineSet(kind string, projector fieldProjector, lines []fieldLine) []fieldLine {
	if len(lines) < 2 || projector.W <= 0 || projector.H <= 0 {
		return lines
	}
	candidates := make([]fieldScreenLineCandidate, len(lines))
	diagonal := math.Hypot(float64(projector.W), float64(projector.H))
	for i, line := range lines {
		points := make([]fieldScreenPoint, 0, len(line.Points))
		arc := 0.0
		var previous fieldScreenPoint
		havePrevious := false
		for _, point := range line.Points {
			x, y, _, ok := projector.projectFloat(point)
			if !ok {
				continue
			}
			current := fieldScreenPoint{X: x, Y: y}
			if havePrevious {
				arc += math.Hypot(current.X-previous.X, current.Y-previous.Y)
			}
			points = append(points, current)
			previous, havePrevious = current, true
		}
		weight := 1 + math.Min(.42, arc/math.Max(diagonal*3.5, 1))
		if line.Closed {
			weight += .08
		}
		candidates[i] = fieldScreenLineCandidate{Points: points, Group: line.Group, Weight: weight, Closed: line.Closed}
	}
	cellSize := max(5, min(projector.W, projector.H)/90)
	indices, stats := fieldSelectScreenDiverseLinesDetailed(candidates, projector.W, projector.H, cellSize, len(lines))
	fieldRecordLineSelection(kind, stats)
	if len(indices) == 0 {
		return lines
	}
	selected := make([]fieldLine, 0, len(indices))
	for _, index := range indices {
		if index >= 0 && index < len(lines) {
			selected = append(selected, lines[index])
		}
	}
	return selected
}
