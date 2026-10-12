package main

import "math"

// fieldVectorEvaluator returns the physical vector field at p. The common
// streamline integrator normalizes the value internally, so it follows field
// direction rather than using field magnitude as an artificial speed.
type fieldVectorEvaluator func(p fieldVec3) fieldVec3

func fieldAdd(a, b fieldVec3) fieldVec3         { return a.add(b) }
func fieldSub(a, b fieldVec3) fieldVec3         { return a.sub(b) }
func fieldMul(a fieldVec3, s float64) fieldVec3 { return a.mul(s) }
func fieldDot(a, b fieldVec3) float64           { return a.dot(b) }
func fieldCross(a, b fieldVec3) fieldVec3       { return a.cross(b) }
func fieldLen(a fieldVec3) float64              { return a.len() }

// fieldStreamlineStopFunc can clip an accepted segment to a physical boundary.
// corrected must be finite when stop is true. hitObject is -1 when no specific
// object was hit.
type fieldStreamlineStopFunc func(prev, next fieldVec3) (stop bool, corrected fieldVec3, hitObject int)

type fieldStreamlinePolicy struct {
	InitialStep float64
	MinStep     float64
	MaxStep     float64

	AbsTolerance float64
	RelTolerance float64

	MaxSteps     int
	MaxArcLength float64
	MinField     float64

	// MaxTurnRadians limits the accepted direction change over one step.
	// Set to zero to disable the curvature guard.
	MaxTurnRadians float64

	// Closure checks are disabled when ClosureDistance <= 0.
	ClosureDistance     float64
	ClosureDirectionCos float64
	MinClosureSteps     int
	MinClosureLength    float64

	// Inside is evaluated after a candidate step. Returning false marks the
	// line as escaped without appending the out-of-domain point.
	Inside func(fieldVec3) bool

	// Stop is evaluated on accepted physical segments and may clip the final
	// point to a conductor, opposite charge, wire surface, or another boundary.
	Stop fieldStreamlineStopFunc

	// Cancelled aborts an obsolete editor request at an integration boundary.
	// A nil callback preserves the deterministic standalone/test behaviour.
	Cancelled func() bool
}

type fieldStreamlineResult struct {
	Points    []fieldVec3
	Closed    bool
	HitObject int
	Escaped   bool
	Stagnated bool

	Accepted    int
	Rejected    int
	Evaluations int
	ArcLength   float64
	MaxError    float64
	MinStep     float64
	MaxStep     float64
}

type fieldRK45Trial struct {
	y5          fieldVec3
	error       float64
	startDir    fieldVec3
	endDir      fieldVec3
	evaluations int
	ok          bool
}

func fieldDefaultStreamlinePolicy() fieldStreamlinePolicy {
	return fieldStreamlinePolicy{
		InitialStep:         0.035,
		MinStep:             0.003,
		MaxStep:             0.09,
		AbsTolerance:        2e-4,
		RelTolerance:        2e-3,
		MaxSteps:            1400,
		MaxArcLength:        18,
		MinField:            1e-11,
		MaxTurnRadians:      12 * math.Pi / 180,
		ClosureDistance:     0,
		ClosureDirectionCos: math.Cos(20 * math.Pi / 180),
		MinClosureSteps:     30,
		MinClosureLength:    0.5,
	}
}

func fieldSanitizeStreamlinePolicy(p fieldStreamlinePolicy) fieldStreamlinePolicy {
	defaults := fieldDefaultStreamlinePolicy()
	if !(p.MinStep > 0) {
		p.MinStep = defaults.MinStep
	}
	if !(p.MaxStep >= p.MinStep) {
		p.MaxStep = math.Max(defaults.MaxStep, p.MinStep)
	}
	if !(p.InitialStep > 0) {
		p.InitialStep = defaults.InitialStep
	}
	p.InitialStep = fieldClampFloat(p.InitialStep, p.MinStep, p.MaxStep)
	if !(p.AbsTolerance > 0) {
		p.AbsTolerance = defaults.AbsTolerance
	}
	if !(p.RelTolerance > 0) {
		p.RelTolerance = defaults.RelTolerance
	}
	if p.MaxSteps <= 0 {
		p.MaxSteps = defaults.MaxSteps
	}
	if !(p.MaxArcLength > 0) {
		p.MaxArcLength = defaults.MaxArcLength
	}
	if !(p.MinField > 0) {
		p.MinField = defaults.MinField
	}
	if p.ClosureDirectionCos == 0 {
		p.ClosureDirectionCos = defaults.ClosureDirectionCos
	}
	if p.MinClosureSteps <= 0 {
		p.MinClosureSteps = defaults.MinClosureSteps
	}
	if !(p.MinClosureLength > 0) {
		p.MinClosureLength = defaults.MinClosureLength
	}
	return p
}

// fieldIntegrateStreamline integrates dr/ds = sign*F/|F| using the
// Dormand-Prince RK5(4) pair. sign should normally be +1 or -1.
func fieldIntegrateStreamline(eval fieldVectorEvaluator, seed fieldVec3, sign float64, policy fieldStreamlinePolicy) fieldStreamlineResult {
	policy = fieldSanitizeStreamlinePolicy(policy)
	result := fieldStreamlineResult{HitObject: -1, MinStep: math.Inf(1)}
	if eval == nil || !fieldVec3Finite(seed) {
		result.Stagnated = true
		result.MinStep = 0
		return result
	}
	if sign < 0 {
		sign = -1
	} else {
		sign = 1
	}

	seedDirection, ok := fieldUnitVector(eval(seed), sign, policy.MinField)
	result.Evaluations++
	if !ok {
		result.Stagnated = true
		result.MinStep = 0
		return result
	}

	result.Points = append(result.Points, seed)
	current := seed
	h := policy.InitialStep
	stagnationRetries := 0

	for result.Accepted < policy.MaxSteps && result.ArcLength < policy.MaxArcLength {
		if policy.Cancelled != nil && policy.Cancelled() {
			break
		}
		trial := fieldDormandPrince45Trial(eval, current, h, sign, policy.MinField)
		result.Evaluations += trial.evaluations
		if !trial.ok {
			result.Rejected++
			stagnationRetries++
			if h <= policy.MinStep*(1+1e-12) || stagnationRetries >= 4 {
				result.Stagnated = true
				break
			}
			h = math.Max(policy.MinStep, h*0.5)
			continue
		}
		stagnationRetries = 0

		scale := math.Max(1, math.Max(fieldLen(current), fieldLen(trial.y5)))
		tolerance := policy.AbsTolerance + policy.RelTolerance*scale
		turnTooSharp := false
		if policy.MaxTurnRadians > 0 {
			turnTooSharp = fieldDot(trial.startDir, trial.endDir) < math.Cos(policy.MaxTurnRadians)
		}
		if trial.error > tolerance || turnTooSharp {
			result.Rejected++
			factor := 0.5
			if !turnTooSharp && trial.error > 0 {
				factor = fieldClampFloat(0.9*math.Pow(tolerance/trial.error, 0.2), 0.25, 0.8)
			}
			if h <= policy.MinStep*(1+1e-12) {
				// At the minimum step, accept a finite trial unless the turn is
				// catastrophically sharp. This prevents infinite rejection near a
				// merely high-curvature but valid field line.
				if turnTooSharp && fieldDot(trial.startDir, trial.endDir) < 0 {
					result.Stagnated = true
					break
				}
			} else {
				h = math.Max(policy.MinStep, h*factor)
				continue
			}
		}

		next := trial.y5
		if !fieldVec3Finite(next) {
			result.Stagnated = true
			break
		}
		if policy.Inside != nil && !policy.Inside(next) {
			result.Escaped = true
			break
		}

		if policy.Stop != nil {
			stop, corrected, hit := policy.Stop(current, next)
			if stop {
				if fieldVec3Finite(corrected) {
					seg := fieldLen(fieldSub(corrected, current))
					if seg > 1e-12 {
						result.Points = append(result.Points, corrected)
						result.ArcLength += seg
						result.Accepted++
						fieldRecordAcceptedStep(&result, seg, trial.error)
					}
				}
				result.HitObject = hit
				break
			}
		}

		seg := fieldLen(fieldSub(next, current))
		if !(seg > 1e-12) || !fieldFinite(seg) {
			result.Stagnated = true
			break
		}
		if result.ArcLength+seg > policy.MaxArcLength {
			break
		}

		result.Points = append(result.Points, next)
		result.ArcLength += seg
		result.Accepted++
		fieldRecordAcceptedStep(&result, seg, trial.error)
		current = next

		if policy.ClosureDistance > 0 && result.Accepted >= policy.MinClosureSteps && result.ArcLength >= policy.MinClosureLength {
			if fieldLen(fieldSub(current, seed)) <= policy.ClosureDistance {
				currentDirection, dirOK := fieldUnitVector(eval(current), sign, policy.MinField)
				result.Evaluations++
				if dirOK && fieldDot(currentDirection, seedDirection) >= policy.ClosureDirectionCos {
					// Keep the numerically integrated endpoint. Appending the seed with a
					// straight chord creates a short segment that is not tangent to B and
					// can appear as a hook near closure. The endpoint is already within
					// ClosureDistance, so the raster gap is sub-pixel at normal zoom.
					result.Closed = true
					break
				}
			}
		}

		factor := 2.0
		if trial.error > 0 {
			factor = fieldClampFloat(0.9*math.Pow(tolerance/trial.error, 0.2), 0.5, 2.0)
		}
		h = fieldClampFloat(h*factor, policy.MinStep, policy.MaxStep)
	}

	if math.IsInf(result.MinStep, 1) {
		result.MinStep = 0
	}
	return result
}

// fieldIntegrateBidirectionalStreamline reconstructs a complete magnetic line
// from a seed that may lie anywhere on the orbit.
func fieldIntegrateBidirectionalStreamline(eval fieldVectorEvaluator, seed fieldVec3, policy fieldStreamlinePolicy) fieldStreamlineResult {
	// Most magnetic source lines are closed.  Trace the physical +B direction
	// first and stop immediately when it closes; the former implementation
	// always completed a full -B trace first and then discarded it whenever the
	// +B trace formed the same complete loop.
	forward := fieldIntegrateStreamline(eval, seed, 1, policy)
	if forward.Closed {
		return forward
	}
	backward := fieldIntegrateStreamline(eval, seed, -1, policy)
	if backward.Closed {
		backward.Points = fieldReverseVec3Copy(backward.Points)
		return backward
	}
	return fieldMergeBidirectionalStreamlines(backward, forward)
}

type fieldStreamlineValidation struct {
	Points, Segments int
	NonFinite        int
	LongJump         int
	Misaligned       int
	FalseClosure     int
	MaxSegment       float64
	MinAlignment     float64
}

// fieldValidateStreamline samples every segment at 1/4, 1/2, and 3/4. sign is
// +1 for a line oriented with the field and -1 for a raw backward trace.
func fieldValidateStreamline(eval fieldVectorEvaluator, points []fieldVec3, sign, maxSegment, minAlignment float64, closed bool, closureDistance, closureDirectionCos float64) fieldStreamlineValidation {
	report := fieldStreamlineValidation{Points: len(points), MinAlignment: 1}
	if eval == nil || len(points) == 0 {
		return report
	}
	if sign < 0 {
		sign = -1
	} else {
		sign = 1
	}
	for _, p := range points {
		if !fieldVec3Finite(p) {
			report.NonFinite++
		}
	}
	fractions := []float64{0.25, 0.5, 0.75}
	if len(points) > 400 {
		// RK45 already bounds the turn angle of every accepted segment.  On a
		// long line, a midpoint tangent check catches the same numerical failures
		// without tripling thousands of hybrid grid/direct field evaluations.
		fractions = []float64{0.5}
	}
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		segment := fieldSub(b, a)
		length := fieldLen(segment)
		report.Segments++
		report.MaxSegment = math.Max(report.MaxSegment, length)
		if maxSegment > 0 && length > maxSegment {
			report.LongJump++
		}
		tangent, ok := fieldUnitVector(segment, sign, 1e-15)
		if !ok {
			report.Misaligned++
			continue
		}
		for _, fraction := range fractions {
			p := fieldAdd(a, fieldMul(segment, fraction))
			direction, fieldOK := fieldUnitVector(eval(p), 1, 1e-15)
			if !fieldOK {
				report.Misaligned++
				continue
			}
			alignment := fieldDot(tangent, direction)
			report.MinAlignment = math.Min(report.MinAlignment, alignment)
			if alignment < minAlignment {
				report.Misaligned++
			}
		}
	}
	if closed && len(points) >= 3 {
		first, last := points[0], points[len(points)-1]
		if closureDistance > 0 && fieldLen(fieldSub(first, last)) > closureDistance {
			report.FalseClosure++
		}
		firstDir, firstOK := fieldUnitVector(eval(first), 1, 1e-15)
		lastDir, lastOK := fieldUnitVector(eval(last), 1, 1e-15)
		if !firstOK || !lastOK || fieldDot(firstDir, lastDir) < closureDirectionCos {
			report.FalseClosure++
		}
	}
	return report
}

// fieldMergeBidirectionalStreamlines combines a -F integration and a +F
// integration so the returned polyline is oriented along +F. A complete closed
// result is preferred over joining two open halves, avoiding a duplicated loop.
func fieldMergeBidirectionalStreamlines(backward, forward fieldStreamlineResult) fieldStreamlineResult {
	if forward.Closed {
		return forward
	}
	if backward.Closed {
		backward.Points = fieldReverseVec3Copy(backward.Points)
		return backward
	}

	merged := fieldStreamlineResult{HitObject: -1, MinStep: math.Inf(1)}
	for i := len(backward.Points) - 1; i >= 1; i-- {
		merged.Points = append(merged.Points, backward.Points[i])
	}
	if len(forward.Points) > 0 {
		merged.Points = append(merged.Points, forward.Points...)
	} else if len(backward.Points) > 0 {
		merged.Points = append(merged.Points, backward.Points[0])
	}

	merged.Accepted = backward.Accepted + forward.Accepted
	merged.Rejected = backward.Rejected + forward.Rejected
	merged.Evaluations = backward.Evaluations + forward.Evaluations
	merged.ArcLength = backward.ArcLength + forward.ArcLength
	merged.MaxError = math.Max(backward.MaxError, forward.MaxError)
	merged.MinStep = fieldPositiveMin(backward.MinStep, forward.MinStep)
	merged.MaxStep = math.Max(backward.MaxStep, forward.MaxStep)
	merged.Escaped = backward.Escaped || forward.Escaped
	merged.Stagnated = backward.Stagnated && forward.Stagnated
	if forward.HitObject >= 0 {
		merged.HitObject = forward.HitObject
	} else if backward.HitObject >= 0 {
		merged.HitObject = backward.HitObject
	}
	if math.IsInf(merged.MinStep, 1) {
		merged.MinStep = 0
	}
	return merged
}

func fieldDormandPrince45Trial(eval fieldVectorEvaluator, y fieldVec3, h, sign, minField float64) fieldRK45Trial {
	var out fieldRK45Trial
	f := func(p fieldVec3) (fieldVec3, bool) {
		out.evaluations++
		return fieldUnitVector(eval(p), sign, minField)
	}

	k1, ok := f(y)
	if !ok {
		return out
	}
	out.startDir = k1
	k2, ok := f(fieldRKPoint(y, h, []fieldVec3{k1}, []float64{1.0 / 5.0}))
	if !ok {
		return out
	}
	k3, ok := f(fieldRKPoint(y, h, []fieldVec3{k1, k2}, []float64{3.0 / 40.0, 9.0 / 40.0}))
	if !ok {
		return out
	}
	k4, ok := f(fieldRKPoint(y, h, []fieldVec3{k1, k2, k3}, []float64{44.0 / 45.0, -56.0 / 15.0, 32.0 / 9.0}))
	if !ok {
		return out
	}
	k5, ok := f(fieldRKPoint(y, h, []fieldVec3{k1, k2, k3, k4}, []float64{19372.0 / 6561.0, -25360.0 / 2187.0, 64448.0 / 6561.0, -212.0 / 729.0}))
	if !ok {
		return out
	}
	k6, ok := f(fieldRKPoint(y, h, []fieldVec3{k1, k2, k3, k4, k5}, []float64{9017.0 / 3168.0, -355.0 / 33.0, 46732.0 / 5247.0, 49.0 / 176.0, -5103.0 / 18656.0}))
	if !ok {
		return out
	}

	y5 := fieldRKPoint(y, h, []fieldVec3{k1, k3, k4, k5, k6}, []float64{35.0 / 384.0, 500.0 / 1113.0, 125.0 / 192.0, -2187.0 / 6784.0, 11.0 / 84.0})
	k7, ok := f(y5)
	if !ok {
		return out
	}
	y4 := fieldRKPoint(y, h, []fieldVec3{k1, k3, k4, k5, k6, k7}, []float64{5179.0 / 57600.0, 7571.0 / 16695.0, 393.0 / 640.0, -92097.0 / 339200.0, 187.0 / 2100.0, 1.0 / 40.0})

	out.y5 = y5
	out.error = fieldLen(fieldSub(y5, y4))
	out.endDir = k7
	out.ok = fieldVec3Finite(y5) && fieldFinite(out.error)
	return out
}

func fieldRKPoint(base fieldVec3, h float64, ks []fieldVec3, coefficients []float64) fieldVec3 {
	result := base
	for i := range ks {
		result = fieldAdd(result, fieldMul(ks[i], h*coefficients[i]))
	}
	return result
}

func fieldUnitVector(v fieldVec3, sign, minField float64) (fieldVec3, bool) {
	if !fieldVec3Finite(v) {
		return fieldVec3{}, false
	}
	length := fieldLen(v)
	if !(length >= minField) || !fieldFinite(length) {
		return fieldVec3{}, false
	}
	return fieldMul(v, sign/length), true
}

func fieldRecordAcceptedStep(result *fieldStreamlineResult, step, err float64) {
	result.MinStep = math.Min(result.MinStep, step)
	result.MaxStep = math.Max(result.MaxStep, step)
	result.MaxError = math.Max(result.MaxError, err)
}

func fieldReverseVec3Copy(points []fieldVec3) []fieldVec3 {
	out := make([]fieldVec3, len(points))
	for i := range points {
		out[len(points)-1-i] = points[i]
	}
	return out
}

func fieldPositiveMin(a, b float64) float64 {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	return math.Min(a, b)
}

func fieldVec3Finite(v fieldVec3) bool {
	return fieldFinite(v.X) && fieldFinite(v.Y) && fieldFinite(v.Z)
}

func fieldFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func fieldClampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
