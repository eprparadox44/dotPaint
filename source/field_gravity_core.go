package main

import (
	"math"
	"sort"
	"time"
)

// The gravity tool uses normalized educational units.  G is absorbed into the
// mass scale, so only relative field shape and potential are displayed.
const (
	fieldGravityMassMax       = 10.0
	fieldGravityRadiusMin     = 0.08
	fieldGravityRadiusMax     = 1.20
	fieldGravityRingRadiusMin = 0.18
	fieldGravityRingRadiusMax = 1.55
)

type fieldGravityPoint struct {
	Pos  fieldVec3
	Mass float64
	Soft float64
}

type fieldGravityModel struct {
	Scene  fieldScene
	Points []fieldGravityPoint // finite face/ring quadrature points
}

func fieldGravityRadiusSliderPosition(radius float64) int {
	t := (fieldClamp(radius, fieldGravityRadiusMin, fieldGravityRadiusMax) - fieldGravityRadiusMin) / (fieldGravityRadiusMax - fieldGravityRadiusMin)
	return int(math.Round(t * 100))
}

func fieldGravityRadiusFromSlider(pos int) float64 {
	t := fieldClamp(float64(pos)/100.0, 0, 1)
	return fieldGravityRadiusMin + (fieldGravityRadiusMax-fieldGravityRadiusMin)*t
}

// fieldGravitySchwarzschildRadius maps the educational mass scale to a
// physical compactness scale.  The calibration requested by the user is
// The current educational calibration is r_s=M/80.
func fieldGravitySchwarzschildRadius(o fieldObject) float64 {
	if o.Kind != fieldObjectSolidMassSphere && o.Kind != fieldObjectHollowMassSphere {
		return 0
	}
	return math.Max(0, o.Charge) / 80.0
}

// A sphere becomes a black hole only when its actual radius lies at or below
// its Schwarzschild radius.  Strength is zero outside the horizon and grows
// smoothly with excess compactness inside it; there are no slider thresholds.
func fieldGravityBlackHoleStrength(o fieldObject) float64 {
	rs := fieldGravitySchwarzschildRadius(o)
	R := math.Max(o.Radius, 1e-9)
	if rs <= 0 || R > rs {
		return 0
	}
	return fieldClamp(.20+.80*(rs/R-1), .20, 1)
}

// fieldGravityRelativityState returns a weak-field compactness for every
// rotating spherical mass, not only black holes.  u=GM/(rc^2)=r_s/(2r), so a
// neutron-star-like compact object naturally shows frame dragging while an
// ordinary diffuse body receives a negligible correction.
func fieldGravityRelativityState(o fieldObject, p fieldVec3) (rhat, axis fieldVec3, r, u, spin, strength float64, ok bool) {
	if o.Kind != fieldObjectSolidMassSphere && o.Kind != fieldObjectHollowMassSphere || o.Charge <= 0 {
		return fieldVec3{}, fieldVec3{}, 0, 0, 0, 0, false
	}
	d := p.sub(fieldObjectCenter(o))
	r = d.len()
	R := math.Max(o.Radius, fieldGravityRadiusMin)
	if r < R*(1-1e-9) || r < 1e-10 {
		return fieldVec3{}, fieldVec3{}, r, 0, 0, 0, false
	}
	rhat = d.mul(1 / r)
	_, axis, _ = fieldObjectBasis(o)
	axis = axis.norm()
	if axis.len2() < .5 {
		axis = fieldVec3{Y: 1}
	}
	spin = fieldClamp(o.Spin, 0, .99)
	rs := fieldGravitySchwarzschildRadius(o)
	u = fieldClamp(rs/(2*r), 0, .45)
	strength = fieldClamp(rs/R, 0, 2)
	return rhat, axis, r, u, spin, strength, u > 0
}

// fieldGravityRelativisticFieldCorrection adds an educational compactness
// expansion through O(u^4): a Schwarzschild-like radial enhancement, a
// spin-squared quadrupole term, and frame-dragging tangent terms through fourth
// order.  This is not an exact 4PN Kerr solver; it is a stable coordinate-field
// visualization whose higher powers make dense rotating stars and black holes
// change continuously without introducing an expensive tensor integration.
func fieldGravityRelativisticFieldCorrection(o fieldObject, p fieldVec3) fieldVec3 {
	rhat, axis, r, u, spin, _, ok := fieldGravityRelativityState(o, p)
	if !ok || o.Charge <= 0 {
		return fieldVec3{}
	}
	g0 := o.Charge / (r * r)
	cosTheta := fieldClamp(rhat.dot(axis), -1, 1)
	p2 := .5 * (3*cosTheta*cosTheta - 1)
	spin2 := spin * spin
	angular := 1 + .5*spin2*p2
	u2 := u * u
	u3 := u2 * u
	u4 := u2 * u2

	// d[-M/r*(1+u+A(u^2+u^3+u^4))]/dr gives the radial
	// 2u + A(3u^2+4u^3+5u^4) enhancement.  Horner-like reuse keeps the
	// additional third/fourth-order work to a handful of multiplies.
	radialExtra := g0 * (2*u + angular*(3*u2+4*u3+5*u4))
	out := rhat.mul(-radialExtra)

	// Spin-squared quadrupole through O(u^4).  axis-cos(theta)rhat is the local
	// poleward direction and therefore rotates with the object's XYZ rotation.
	poleward := axis.sub(rhat.mul(cosTheta))
	quadrupole := 1.5 * spin2 * cosTheta * (u2 + u3 + u4)
	out = out.add(poleward.mul(g0 * quadrupole))

	// Lense-Thirring-inspired azimuthal expansion through O(u^4).
	phi := axis.cross(rhat).norm()
	drag := spin * (1.20*u + 1.80*u2 + 2.40*u3 + 3.00*u4)
	out = out.add(phi.mul(g0 * drag))
	return out
}

func fieldGravityRelativisticCorrection(scene fieldScene, p fieldVec3) fieldVec3 {
	var out fieldVec3
	for _, o := range scene.Objects {
		out = out.add(fieldGravityRelativisticFieldCorrection(o, p))
	}
	return out
}

func fieldGravityRelativisticPotentialCorrection(o fieldObject, p fieldVec3) float64 {
	rhat, axis, r, u, spin, _, ok := fieldGravityRelativityState(o, p)
	if !ok || o.Charge <= 0 {
		return 0
	}
	cosTheta := fieldClamp(rhat.dot(axis), -1, 1)
	p2 := .5 * (3*cosTheta*cosTheta - 1)
	angular := 1 + .5*spin*spin*p2
	u2 := u * u
	return (-o.Charge / r) * (u + angular*(u2+u2*u+u2*u2))
}

func fieldGravityQuadraturePoints(o fieldObject, objectIndex, quality int) []fieldGravityPoint {
	mass := math.Max(0, o.Charge)
	if mass <= 1e-12 {
		return nil
	}
	c := fieldObjectCenter(o)
	ex, ey, ez := fieldObjectBasis(o)
	switch o.Kind {
	case fieldObjectMassPlane:
		n := 10
		if quality > 0 {
			n = 14
		}
		if quality > 1 {
			n = 18
		}
		out := make([]fieldGravityPoint, 0, n*n)
		cell := o.Size / float64(n)
		each := mass / float64(n*n)
		for j := 0; j < n; j++ {
			v := (-.5 + (float64(j)+.5)/float64(n)) * o.Size
			for i := 0; i < n; i++ {
				u := (-.5 + (float64(i)+.5)/float64(n)) * o.Size
				out = append(out, fieldGravityPoint{Pos: c.add(ex.mul(u)).add(ez.mul(v)), Mass: each, Soft: cell * .38})
			}
		}
		return out
	case fieldObjectMassRing:
		n := 64
		if quality > 0 {
			n = 96
		}
		if quality > 1 {
			n = 128
		}
		out := make([]fieldGravityPoint, 0, n)
		each := mass / float64(n)
		arc := 2 * math.Pi * o.Radius / float64(n)
		for i := 0; i < n; i++ {
			a := 2 * math.Pi * (float64(i) + .5) / float64(n)
			out = append(out, fieldGravityPoint{Pos: c.add(ey.mul(o.Radius * math.Cos(a))).add(ez.mul(o.Radius * math.Sin(a))), Mass: each, Soft: arc * .28})
		}
		return out
	}
	_ = objectIndex
	return nil
}

func fieldBuildGravityModel(scene fieldScene, quality int) fieldGravityModel {
	m := fieldGravityModel{Scene: scene}
	for i, o := range scene.Objects {
		if !o.gravity() {
			continue
		}
		m.Points = append(m.Points, fieldGravityQuadraturePoints(o, i, quality)...)
	}
	return m
}

func fieldGravitySphereAt(o fieldObject, p fieldVec3) fieldVec3 {
	d := fieldObjectCenter(o).sub(p) // inward
	r := d.len()
	if r < 1e-12 || o.Charge <= 0 {
		return fieldVec3{}
	}
	R := math.Max(o.Radius, fieldGravityRadiusMin)
	if o.Kind == fieldObjectHollowMassSphere && r < R {
		return fieldVec3{}
	}
	if o.Kind == fieldObjectSolidMassSphere && r < R {
		return d.mul(o.Charge / (R * R * R))
	}
	return d.mul(o.Charge / (r * r * r))
}

func fieldGravitySpherePotential(o fieldObject, p fieldVec3) float64 {
	r := p.sub(fieldObjectCenter(o)).len()
	R := math.Max(o.Radius, fieldGravityRadiusMin)
	M := math.Max(0, o.Charge)
	if M == 0 {
		return 0
	}
	if o.Kind == fieldObjectHollowMassSphere && r < R {
		return -M / R
	}
	if o.Kind == fieldObjectSolidMassSphere && r < R {
		return -M * (3*R*R - r*r) / (2 * R * R * R)
	}
	return -M / math.Max(r, 1e-8)
}

func (m fieldGravityModel) atNewtonian(p fieldVec3) fieldVec3 {
	var g fieldVec3
	for _, o := range m.Scene.Objects {
		switch o.Kind {
		case fieldObjectSolidMassSphere, fieldObjectHollowMassSphere:
			g = g.add(fieldGravitySphereAt(o, p))
		}
	}
	for _, q := range m.Points {
		d := q.Pos.sub(p)
		r2 := d.len2() + q.Soft*q.Soft
		if r2 < 1e-14 {
			continue
		}
		g = g.add(d.mul(q.Mass / (r2 * math.Sqrt(r2))))
	}
	return g
}

func (m fieldGravityModel) at(p fieldVec3) fieldVec3 {
	return m.atNewtonian(p).add(fieldGravityRelativisticCorrection(m.Scene, p))
}

func (m fieldGravityModel) potentialNewtonian(p fieldVec3) float64 {
	v := 0.0
	for _, o := range m.Scene.Objects {
		switch o.Kind {
		case fieldObjectSolidMassSphere, fieldObjectHollowMassSphere:
			v += fieldGravitySpherePotential(o, p)
		}
	}
	for _, q := range m.Points {
		d := p.sub(q.Pos)
		v -= q.Mass / math.Sqrt(d.len2()+q.Soft*q.Soft)
	}
	return v
}

func (m fieldGravityModel) potential(p fieldVec3) float64 {
	v := m.potentialNewtonian(p)
	for _, o := range m.Scene.Objects {
		v += fieldGravityRelativisticPotentialCorrection(o, p)
	}
	return v
}

// Samples are negative electric analogues. They let the existing D3D11 scalar
// and vector kernels accelerate gravity without changing the shader ABI.
func fieldGravityPotentialSamples(scene fieldScene, quality int) []fieldChargeSample {
	out := make([]fieldChargeSample, 0, 512)
	for oi, o := range scene.Objects {
		if !o.gravity() || o.Charge <= 1e-12 {
			continue
		}
		c := fieldObjectCenter(o)
		switch o.Kind {
		case fieldObjectSolidMassSphere, fieldObjectHollowMassSphere:
			// Newton's shell theorem makes every spherical mass exactly a point
			// source outside its radius.  One GPU sample is therefore both faster
			// and more accurate than a finite cloud.  Grid points inside the sphere
			// are corrected to the exact solid/shell formula after readback.
			out = append(out, fieldChargeSample{Pos: c, Q: -o.Charge, Soft: o.Radius * .02, Object: oi})
		case fieldObjectMassPlane, fieldObjectMassRing:
			for _, q := range fieldGravityQuadraturePoints(o, oi, quality) {
				out = append(out, fieldChargeSample{Pos: q.Pos, Q: -q.Mass, Soft: q.Soft, Object: oi})
			}
		}
	}
	return out
}

// fieldCorrectGravityPotentialGrid replaces the singular point-mass extension
// inside spherical sources by the exact Newtonian interior solution.  Other
// objects keep their already-GPU-computed contribution, so the correction is
// only O(number of interior grid points × number of spheres).
func fieldCorrectGravityPotentialGrid(scene fieldScene, points []fieldVec3, values []float64) {
	if !scene.Display.gravity() || len(points) != len(values) {
		return
	}
	for i, p := range points {
		if i&1023 == 0 && scene.cancelled() {
			return
		}
		v := values[i]
		for _, o := range scene.Objects {
			if o.Kind != fieldObjectSolidMassSphere && o.Kind != fieldObjectHollowMassSphere {
				continue
			}
			r := p.sub(fieldObjectCenter(o)).len()
			if r < o.Radius {
				soft := o.Radius * .02
				approx := -o.Charge / math.Sqrt(r*r+soft*soft*.12)
				v += fieldGravitySpherePotential(o, p) - approx
			}
			v += fieldGravityRelativisticPotentialCorrection(o, p)
		}
		values[i] = v
	}
}

func fieldGravityNearestSurface(scene fieldScene, p fieldVec3) float64 {
	best := math.Inf(1)
	for _, o := range scene.Objects {
		if !o.gravity() {
			continue
		}
		c := fieldObjectCenter(o)
		switch o.Kind {
		case fieldObjectSolidMassSphere, fieldObjectHollowMassSphere:
			best = math.Min(best, math.Abs(p.sub(c).len()-o.Radius))
		case fieldObjectMassRing:
			q := p.sub(c)
			ex, ey, ez := fieldObjectBasis(o)
			lx, ly, lz := q.dot(ex), q.dot(ey), q.dot(ez)
			best = math.Min(best, math.Hypot(lx, math.Hypot(ly, lz)-o.Radius))
		case fieldObjectMassPlane:
			q := p.sub(c)
			ex, normal, ez := fieldObjectBasis(o)
			lx, ly, lz := q.dot(ex), q.dot(normal), q.dot(ez)
			dx := math.Max(0, math.Abs(lx)-o.Size*.5)
			dz := math.Max(0, math.Abs(lz)-o.Size*.5)
			best = math.Min(best, math.Sqrt(dx*dx+dz*dz+ly*ly))
		}
	}
	return best
}

func fieldGravityVectorEvaluator(scene fieldScene, model fieldGravityModel, samples []fieldChargeSample) (fieldVectorEvaluator, string) {
	directNewtonian := model.atNewtonian
	directFull := model.at
	if scene.precise3DLines && scene.Dimension == fieldDimension3D && scene.Display == fieldDisplayGravityLines {
		return directFull, "CPU-DIRECT-PRECISE-3D-GR4"
	}
	steps := fieldVectorGridSteps(scene)
	key := fieldVectorSceneKey(scene, samples, false, steps) ^ 0x4752415649545921
	if grid, ok := fieldCachedGravityVectorGrid(key); ok {
		fieldRecordVectorGridCache("gravity", grid, true)
		near := func(p fieldVec3) float64 { return fieldGravityNearestSurface(scene, p) }
		newtonian := grid.evaluator(directNewtonian, near, fieldVectorGridDirectDistance(grid))
		return func(p fieldVec3) fieldVec3 {
			return newtonian(p).add(fieldGravityRelativisticCorrection(scene, p))
		}, "D3D11-GRID-CACHE-GR4"
	}
	span := fieldVectorGridSpan(scene)
	started := time.Now()
	values, ok := fieldTryGPUElectricVectorGrid(samples, steps, -span, span)
	if !ok || len(values) != steps*steps*steps {
		fieldRecordVectorGridFallback("gravity", steps, len(samples), 0, 0, time.Since(started))
		return directFull, "CPU-DIRECT-GR4"
	}
	grid := fieldVectorGrid{NX: steps, NY: steps, NZ: steps, Min: fieldVec3{X: -span, Y: -span, Z: -span}, Max: fieldVec3{X: span, Y: span, Z: span}, Values: values, Backend: "D3D11"}
	fieldStoreGravityVectorGrid(key, grid)
	fieldRecordVectorGridReady("gravity", grid, len(samples), 0, 0, false, time.Since(started))
	near := func(p fieldVec3) float64 { return fieldGravityNearestSurface(scene, p) }
	newtonian := grid.evaluator(directNewtonian, near, fieldVectorGridDirectDistance(grid))
	return func(p fieldVec3) fieldVec3 {
		return newtonian(p).add(fieldGravityRelativisticCorrection(scene, p))
	}, "D3D11-GRID-GR4"
}

func fieldGravityBoundaryHit(scene fieldScene, a, b fieldVec3) (bool, fieldVec3, int) {
	// Thin rings and sheets can be crossed by one accepted RK step without its
	// endpoint landing inside the object.  Search along the segment and select
	// the earliest physical hit, then bisect it to the surface.
	bestT := math.Inf(1)
	bestObject := -1
	const probes = 12
	for i, o := range scene.Objects {
		if !o.gravity() {
			continue
		}
		previousT := 0.0
		previousInside := fieldPointInsideObject(a, o, 0)
		if previousInside {
			if 0 < bestT {
				bestT, bestObject = 0, i
			}
			continue
		}
		for probe := 1; probe <= probes; probe++ {
			t := float64(probe) / probes
			q := a.add(b.sub(a).mul(t))
			inside := fieldPointInsideObject(q, o, 0)
			if inside && !previousInside {
				lo, hi := previousT, t
				for n := 0; n < 20; n++ {
					mid := (lo + hi) * .5
					m := a.add(b.sub(a).mul(mid))
					if fieldPointInsideObject(m, o, 0) {
						hi = mid
					} else {
						lo = mid
					}
				}
				if hi < bestT {
					bestT, bestObject = hi, i
				}
				break
			}
			previousT, previousInside = t, inside
		}
	}
	if bestObject >= 0 && fieldFinite(bestT) {
		return true, a.add(b.sub(a).mul(bestT)), bestObject
	}
	return false, fieldVec3{}, -1
}

const fieldGravityInfinityHitObject = -2

type fieldGravityLineSeed struct {
	Near, Surface fieldVec3
	Group         int
}

func fieldGravityLineOuterRadius(scene fieldScene) float64 {
	return fieldSceneNumericalInfinityRadius(scene) * .92
}

func fieldGravityOuterBoundaryHit(scene fieldScene, a, b fieldVec3) (bool, fieldVec3, int) {
	radius := fieldGravityLineOuterRadius(scene)
	if a.len() >= radius {
		return true, a, fieldGravityInfinityHitObject
	}
	if b.len() < radius {
		return false, fieldVec3{}, -1
	}
	d := b.sub(a)
	aa := d.dot(d)
	if aa < 1e-18 {
		return false, fieldVec3{}, -1
	}
	bb := 2 * a.dot(d)
	cc := a.dot(a) - radius*radius
	disc := bb*bb - 4*aa*cc
	if disc < 0 {
		return false, fieldVec3{}, -1
	}
	sqrtDisc := math.Sqrt(disc)
	t0 := (-bb - sqrtDisc) / (2 * aa)
	t1 := (-bb + sqrtDisc) / (2 * aa)
	t := math.Inf(1)
	for _, candidate := range []float64{t0, t1} {
		if candidate >= -1e-9 && candidate <= 1+1e-9 && candidate < t {
			t = fieldClamp(candidate, 0, 1)
		}
	}
	if !fieldFinite(t) {
		return false, fieldVec3{}, -1
	}
	return true, a.add(d.mul(t)), fieldGravityInfinityHitObject
}

func fieldGravityFibonacciDirection(dimension fieldDimension, i, count int) fieldVec3 {
	if dimension == fieldDimension2D {
		a := 2 * math.Pi * float64(i) / float64(max(1, count))
		return fieldVec3{X: math.Cos(a), Z: math.Sin(a)}
	}
	golden := math.Pi * (3 - math.Sqrt(5))
	z := 1 - 2*(float64(i)+.5)/float64(max(1, count))
	r := math.Sqrt(math.Max(0, 1-z*z))
	a := golden * float64(i)
	return fieldVec3{X: r * math.Cos(a), Y: z, Z: r * math.Sin(a)}
}

func fieldGravitySurfaceSeeds(scene fieldScene, perObject int) []fieldGravityLineSeed {
	if perObject < 1 {
		return nil
	}
	margin := math.Max(.055, fieldSceneSpan(scene)/125)
	seeds := make([]fieldGravityLineSeed, 0, perObject*len(scene.Objects))
	golden := math.Pi * (3 - math.Sqrt(5))
	for objectIndex, o := range scene.Objects {
		if !o.gravity() || o.Charge <= 1e-12 {
			continue
		}
		center := fieldObjectCenter(o)
		ex, normal, ez := fieldObjectBasis(o)
		switch o.Kind {
		case fieldObjectSolidMassSphere, fieldObjectHollowMassSphere:
			for i := 0; i < perObject; i++ {
				direction := fieldGravityFibonacciDirection(scene.Dimension, i, perObject).norm()
				surface := center.add(direction.mul(o.Radius))
				seeds = append(seeds, fieldGravityLineSeed{Near: surface.add(direction.mul(margin)), Surface: surface, Group: objectIndex})
			}
		case fieldObjectMassPlane:
			// Seed both faces of the finite sheet. Keep samples away from the exact
			// corners, where the finite-sheet field changes direction most abruptly.
			halfCount := max(1, (perObject+1)/2)
			gridN := max(1, int(math.Ceil(math.Sqrt(float64(halfCount)))))
			for i := 0; i < perObject; i++ {
				side := 1.0
				if i&1 != 0 {
					side = -1
				}
				k := i / 2
				iu, iv := k%gridN, (k/gridN)%gridN
				u := ((float64(iu)+.5)/float64(gridN) - .5) * o.Size * .82
				v := ((float64(iv)+.5)/float64(gridN) - .5) * o.Size * .82
				n := normal.mul(side)
				surface := center.add(ex.mul(u)).add(ez.mul(v)).add(n.mul(.07))
				seeds = append(seeds, fieldGravityLineSeed{Near: surface.add(n.mul(margin)), Surface: surface, Group: objectIndex})
			}
		case fieldObjectMassRing:
			tube := math.Max(.045, o.Radius*.055)
			for i := 0; i < perObject; i++ {
				a := 2 * math.Pi * float64(i) / float64(perObject)
				radial := normal.mul(math.Cos(a)).add(ez.mul(math.Sin(a))).norm()
				crossAngle := 0.0
				if scene.Dimension == fieldDimension3D {
					crossAngle = math.Mod(golden*float64(i), 2*math.Pi)
				}
				cross := radial.mul(math.Cos(crossAngle)).add(ex.mul(math.Sin(crossAngle))).norm()
				centerline := center.add(normal.mul(o.Radius * math.Cos(a))).add(ez.mul(o.Radius * math.Sin(a)))
				surface := centerline.add(cross.mul(tube))
				seeds = append(seeds, fieldGravityLineSeed{Near: surface.add(cross.mul(margin)), Surface: surface, Group: objectIndex})
			}
		}
	}
	return seeds
}

func fieldTraceCompleteGravityLine(scene fieldScene, eval fieldVectorEvaluator, seed fieldGravityLineSeed) ([]fieldVec3, bool, bool) {
	if eval == nil {
		return nil, false, false
	}
	outerPolicy := fieldDefaultStreamlinePolicy()
	outerPolicy.InitialStep = .055
	outerPolicy.MinStep = .0025
	outerPolicy.MaxStep = .14
	outerPolicy.MaxSteps = 3600
	outerPolicy.MaxArcLength = fieldGravityLineOuterRadius(scene)*3.2 + fieldSceneSpan(scene)
	outerPolicy.MinField = 1e-14
	outerPolicy.MaxTurnRadians = 10 * math.Pi / 180
	if scene.precise3DLines {
		outerPolicy.InitialStep = .030
		outerPolicy.MinStep = .0015
		outerPolicy.MaxStep = .065
		outerPolicy.AbsTolerance = 6e-5
		outerPolicy.RelTolerance = 6e-4
		outerPolicy.MaxTurnRadians = 6 * math.Pi / 180
		outerPolicy.MaxSteps = 5200
	}
	outerPolicy.ClosureDistance = 0
	if scene.cancel != nil {
		outerPolicy.Cancelled = scene.cancel.Cancelled
	}
	outerPolicy.Inside = nil
	outerPolicy.Stop = func(a, b fieldVec3) (bool, fieldVec3, int) {
		if hit, corrected, object := fieldGravityBoundaryHit(scene, a, b); hit {
			return true, corrected, object
		}
		return fieldGravityOuterBoundaryHit(scene, a, b)
	}
	// Trace against the physical field so reverse integration walks from the
	// source neighbourhood toward numerical infinity. Reversing the result then
	// restores the conventional infinity -> mass field-line direction.
	outerResult := fieldIntegrateStreamline(eval, seed.Near, -1, outerPolicy)
	outerHit := outerResult.HitObject == fieldGravityInfinityHitObject && len(outerResult.Points) >= 4
	if !outerHit {
		return nil, false, false
	}

	innerPolicy := fieldDefaultStreamlinePolicy()
	innerPolicy.InitialStep = .018
	innerPolicy.MinStep = .0015
	innerPolicy.MaxStep = .05
	innerPolicy.MaxSteps = 600
	innerPolicy.MaxArcLength = math.Max(.8, fieldSceneSpan(scene)*.45)
	innerPolicy.MinField = 1e-14
	innerPolicy.MaxTurnRadians = 10 * math.Pi / 180
	if scene.precise3DLines {
		innerPolicy.InitialStep = .012
		innerPolicy.MinStep = .0008
		innerPolicy.MaxStep = .030
		innerPolicy.AbsTolerance = 5e-5
		innerPolicy.RelTolerance = 5e-4
		innerPolicy.MaxTurnRadians = 6 * math.Pi / 180
		innerPolicy.MaxSteps = 1200
	}
	innerPolicy.ClosureDistance = 0
	if scene.cancel != nil {
		innerPolicy.Cancelled = scene.cancel.Cancelled
	}
	innerPolicy.Inside = func(p fieldVec3) bool { return !fieldOutsideNumericalInfinity(scene, p) }
	innerPolicy.Stop = func(a, b fieldVec3) (bool, fieldVec3, int) { return fieldGravityBoundaryHit(scene, a, b) }
	innerResult := fieldIntegrateStreamline(eval, seed.Near, 1, innerPolicy)
	surfaceHit := innerResult.HitObject >= 0 && len(innerResult.Points) >= 2

	points := fieldReverseVec3Copy(outerResult.Points)
	if surfaceHit {
		points = append(points, innerResult.Points[1:]...)
	} else {
		// The seed is deliberately only a few hundredths outside the known
		// surface. A direct final segment is safer than dropping an otherwise
		// complete line when a coarse GPU cell makes the short inward trace miss.
		last := points[len(points)-1]
		if seed.Surface.sub(last).len() <= .22 {
			points = append(points, seed.Surface)
			surfaceHit = true
		}
	}
	return points, outerHit, surfaceHit
}

func fieldGravityLines(scene fieldScene) []fieldLine {
	started := time.Now()
	lineCacheKey := fieldGravityLineSceneKey(scene)
	if cached, ok := fieldCachedPhysicalLines("gravity", lineCacheKey); ok {
		fieldRecordPhysicalLineCache("gravity", len(cached), time.Since(started))
		return cached
	}
	model := fieldBuildGravityModel(scene, 1)
	samples := fieldGravityPotentialSamples(scene, 1)
	eval, backend := fieldGravityVectorEvaluator(scene, model, samples)

	active := 0
	for _, o := range scene.Objects {
		if o.gravity() && o.Charge > 1e-12 {
			active++
		}
	}
	if active == 0 || eval == nil {
		fieldRecordStreamlineTiming("gravity", backend, 0, 0, time.Since(started))
		fieldRecordGravityLineDiagnostics(backend, 0, 0, 0, 0, 0, time.Since(started))
		return nil
	}

	perObject := max(8, scene.Density)
	if scene.Dimension == fieldDimension2D {
		perObject = max(12, perObject)
	}
	perObject = min(perObject, max(1, 192/active))
	seeds := fieldGravitySurfaceSeeds(scene, perObject)
	lines := make([]fieldLine, 0, len(seeds))
	outerHits, surfaceHits, directRetries, rejected := 0, 0, 0, 0
	for _, seed := range seeds {
		if scene.cancelled() {
			break
		}
		points, outerHit, surfaceHit := fieldTraceCompleteGravityLine(scene, eval, seed)
		if outerHit {
			outerHits++
		}
		if surfaceHit {
			surfaceHits++
		}
		// A GPU/interpolation failure must not erase the physical line. Retry the
		// individual candidate with the exact CPU field; simple spheres make this
		// virtually free, while rings/sheets pay only for failed candidates.
		if !outerHit || !surfaceHit || len(points) < 8 {
			directRetries++
			points, outerHit, surfaceHit = fieldTraceCompleteGravityLine(scene, model.at, seed)
		}
		if !outerHit || !surfaceHit || len(points) < 8 {
			rejected++
			continue
		}
		refined := fieldResamplePolylineByArc(points, .055)
		if len(refined) < 8 {
			rejected++
			continue
		}
		lines = append(lines, fieldLine{
			Points: refined,
			Arrow:  fieldArrowIndexAtArcFraction(refined, .58),
			Sign:   -1,
			Group:  seed.Group,
		})
	}
	elapsed := time.Since(started)
	if !scene.cancelled() {
		fieldStorePhysicalLines("gravity", lineCacheKey, lines)
	}
	fieldRecordStreamlineTiming("gravity", backend, len(lines), len(seeds), elapsed)
	fieldRecordGravityLineDiagnostics(backend, len(seeds), outerHits, surfaceHits, directRetries, rejected, elapsed)
	return lines
}

func fieldDrawGravityBlackHole(r *fieldRaster, p fieldProjector, o fieldObject, strength float64) bool {
	if r == nil || strength <= 0 {
		return false
	}
	center := fieldObjectCenter(o)
	cx, cy, _, ok := p.project(center)
	if !ok {
		return false
	}
	scale := .44 * float64(min(p.W, p.H)) / p.effectiveSpan()
	sampleScale := r.sampleScale()
	base := math.Max(float64(4*sampleScale), o.Radius*scale)
	horizon := int(math.Round(base * (.72 + .42*strength)))
	if horizon < 4*sampleScale {
		horizon = 4 * sampleScale
	}
	// Soft amber lensing glow and a dark event horizon.  The orbit guides twist
	// with Spin, so the same slider that represents Kerr-like rotation also
	// changes the visible frame-dragging cue without introducing animation.
	for i := 4; i >= 1; i-- {
		rr := horizon + int(math.Round(float64(i)*base*.34))
		a := (.025 + .035*float64(5-i)) * strength
		r.circle(cx, cy, rr, 235, 145, 45, a, false)
	}
	spin := fieldClamp(o.Spin, 0, .99)
	ex, ey, ez := fieldObjectBasis(o)
	for ring := 0; ring < 4; ring++ {
		rad := o.Radius * (1.45 + .33*float64(ring))
		twist := spin * (.35 + .22*float64(ring)) * math.Pi
		var prev fieldVec3
		for i := 0; i <= 72; i++ {
			a := 2 * math.Pi * float64(i) / 72
			squash := .38 + .10*float64(ring)
			v := center.add(ex.mul(rad * math.Cos(a+twist))).add(ez.mul(rad * squash * math.Sin(a))).add(ey.mul(rad * .08 * spin * math.Sin(2*a)))
			if i > 0 {
				x0, y0, _, ok0 := p.project(prev)
				x1, y1, _, ok1 := p.project(v)
				if ok0 && ok1 {
					r.line(x0, y0, x1, y1, 245, 165, 65, .22+.13*strength, 1)
				}
			}
			prev = v
		}
	}
	r.circle(cx, cy, horizon, 2, 2, 3, .99, true)
	r.circle(cx, cy, horizon, 245, 175, 75, .75*strength, false)
	return true
}

func fieldGravityPotentialLevels(scene fieldScene, model fieldGravityModel, slice fieldPlaneSlice, steps int) []float64 {
	if steps < 16 {
		steps = 16
	}
	span := fieldEquipotentialSamplingSpan(scene)
	magnitudes := make([]float64, 0, steps*steps)
	for j := 0; j < steps; j++ {
		v := -span + 2*span*float64(j)/float64(steps-1)
		for i := 0; i < steps; i++ {
			u := -span + 2*span*float64(i)/float64(steps-1)
			p := slice.Origin.add(slice.U.mul(u)).add(slice.V.mul(v))
			x := model.potential(p)
			if !math.IsNaN(x) && !math.IsInf(x, 0) && x < -1e-6 {
				magnitudes = append(magnitudes, -x)
			}
		}
	}
	if len(magnitudes) < 8 {
		return nil
	}
	sort.Float64s(magnitudes)
	base := magnitudes[int(float64(len(magnitudes)-1)*.96)]
	if base < 1e-6 {
		return nil
	}
	levels := make([]float64, 0, 6)
	for _, ratio := range []float64{.25, .36, .50, .68, .90, 1.18} {
		levels = append(levels, -base*ratio)
	}
	sort.Float64s(levels)
	return levels
}

func fieldGravityContourSlice(scene fieldScene, model fieldGravityModel, slice fieldPlaneSlice, steps int, levels []float64) map[float64][]fieldSegment3 {
	if steps < 12 {
		steps = 12
	}
	points := make([]fieldVec3, steps*steps)
	values := make([]float64, steps*steps)
	span := fieldSceneSpan(scene)
	for j := 0; j < steps; j++ {
		v := -span + 2*span*float64(j)/float64(steps-1)
		for i := 0; i < steps; i++ {
			u := -span + 2*span*float64(i)/float64(steps-1)
			idx := j*steps + i
			points[idx] = slice.Origin.add(slice.U.mul(u)).add(slice.V.mul(v))
			values[idx] = model.potential(points[idx])
		}
	}
	out := make(map[float64][]fieldSegment3)
	for _, level := range levels {
		for j := 0; j < steps-1; j++ {
			for i := 0; i < steps-1; i++ {
				i00, i10 := j*steps+i, j*steps+i+1
				i01, i11 := (j+1)*steps+i, (j+1)*steps+i+1
				ps := [4]fieldVec3{points[i00], points[i10], points[i11], points[i01]}
				vs := [4]float64{values[i00], values[i10], values[i11], values[i01]}
				edges := make([]fieldVec3, 0, 4)
				for e := 0; e < 4; e++ {
					n := (e + 1) % 4
					a, b := vs[e]-level, vs[n]-level
					if (a <= 0 && b > 0) || (a > 0 && b <= 0) {
						edges = append(edges, fieldInterpolateLevel(ps[e], ps[n], vs[e], vs[n], level))
					}
				}
				if len(edges) == 2 {
					out[level] = append(out[level], fieldSegment3{edges[0], edges[1]})
				} else if len(edges) == 4 {
					center := (vs[0] + vs[1] + vs[2] + vs[3]) * .25
					if center > level {
						out[level] = append(out[level], fieldSegment3{edges[0], edges[3]}, fieldSegment3{edges[1], edges[2]})
					} else {
						out[level] = append(out[level], fieldSegment3{edges[0], edges[1]}, fieldSegment3{edges[2], edges[3]})
					}
				}
			}
		}
	}
	return out
}
