package main

import (
	"math"
	"runtime"
	"sort"
	"sync"
)

// A197 black-hole renderer constants. Geometry is expressed in geometric units
// G=c=M=1. The physical mass is deliberately fixed because only scale-free
// Schwarzschild optics are needed by the drawing tool.
const (
	mdBHMass              = 1.0
	mdBHEventHorizon      = 2.0 * mdBHMass
	mdBHPhotonSphere      = 3.0 * mdBHMass
	mdBHDiskInnerRadius   = 6.0 * mdBHMass
	mdBHDiskOuterRadius   = 15.0 * mdBHMass
	mdBHDiskHalfThickness = 0.22 * mdBHMass
	mdBHCameraRadius      = 36.0 * mdBHMass
	mdBHInnerTemperature  = 10500.0 // display colour temperature, kelvin
	mdBHOuterTemperature  = 2400.0
	mdBHFixedBrightness   = 1.18
)

var mdBHCriticalImpact = 3.0 * math.Sqrt(3.0) * mdBHMass

const (
	mdBHRayEscaped = iota
	mdBHRayCaptured
	mdBHRayDisk
	mdBHRayTrapped
)

const (
	mdBlackHoleModeCurvatureGrid = iota
	mdBlackHoleModeRender
	mdBlackHoleModeRenderWithDiagram
	mdBlackHoleModeGeodesics
)

type mdBHRayResult struct {
	kind       int
	hit        mdVec3
	exitDir    mdVec3
	tangent    mdVec3 // backward-traced photon tangent at the terminal point
	minRadius  float64
	azimuth    float64
	steps      int
	windingAbs float64
}

type mdBHRenderParams struct {
	width          int
	height         int
	fovY           float64
	diskNormal     mdVec3
	mode           int
	quality        int
	rotationX      float64
	rotationY      float64
	rotationZ      float64
	rotationW      float64
	cameraDistance float64
	gridDensity    int
	spin           float64 // Kerr dimensionless spin a*=Jc/(GM^2), 0..0.99
	maskPass       bool
}

func mdBHKerrClampSpin(spin float64) float64 {
	return mdBHClamp(spin, 0, 0.99)
}

// mdBHKerrHorizon returns the Boyer-Lindquist outer horizon r_+/M for M=1.
func mdBHKerrHorizon(spin float64) float64 {
	a := mdBHKerrClampSpin(spin)
	return 1.0 + math.Sqrt(math.Max(0, 1.0-a*a))
}

// mdBHKerrISCO returns the prograde equatorial ISCO radius for M=1.
func mdBHKerrISCO(spin float64) float64 {
	a := mdBHKerrClampSpin(spin)
	if a < 1e-12 {
		return mdBHDiskInnerRadius
	}
	z1 := 1 + math.Cbrt(1-a*a)*(math.Cbrt(1+a)+math.Cbrt(1-a))
	z2 := math.Sqrt(3*a*a + z1*z1)
	return 3 + z2 - math.Sqrt(math.Max(0, (3-z1)*(3+z1+2*z2)))
}

type mdBHOrbitState struct {
	r, vr, phi float64
}

func mdBHAdd(a, b mdVec3) mdVec3 { return mdVec3{a.x + b.x, a.y + b.y, a.z + b.z} }
func mdBHMul(a mdVec3, k float64) mdVec3 {
	return mdVec3{a.x * k, a.y * k, a.z * k}
}
func mdBHLerp(a, b mdVec3, t float64) mdVec3 {
	return mdVec3{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t, a.z + (b.z-a.z)*t}
}
func mdBHLength(v mdVec3) float64 { return math.Sqrt(mdDotCore(v, v)) }
func mdBHClamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func mdBHOrbitDerivative(s mdBHOrbitState, impact float64) mdBHOrbitState {
	r := math.Max(s.r, mdBHEventHorizon*0.98)
	b2 := impact * impact
	return mdBHOrbitState{
		r:   s.vr,
		vr:  b2 * (r - 3.0*mdBHMass) / (r * r * r * r),
		phi: impact / (r * r),
	}
}

func mdBHOrbitAdd(a, b mdBHOrbitState, k float64) mdBHOrbitState {
	return mdBHOrbitState{a.r + b.r*k, a.vr + b.vr*k, a.phi + b.phi*k}
}

func mdBHOrbitRK4(s mdBHOrbitState, impact, h float64) mdBHOrbitState {
	k1 := mdBHOrbitDerivative(s, impact)
	k2 := mdBHOrbitDerivative(mdBHOrbitAdd(s, k1, h*0.5), impact)
	k3 := mdBHOrbitDerivative(mdBHOrbitAdd(s, k2, h*0.5), impact)
	k4 := mdBHOrbitDerivative(mdBHOrbitAdd(s, k3, h), impact)
	return mdBHOrbitState{
		r:   s.r + h*(k1.r+2*k2.r+2*k3.r+k4.r)/6,
		vr:  s.vr + h*(k1.vr+2*k2.vr+2*k3.vr+k4.vr)/6,
		phi: s.phi + h*(k1.phi+2*k2.phi+2*k3.phi+k4.phi)/6,
	}
}

func mdBHAdaptiveStep(r, vr float64, quality int) float64 {
	q := 1.0
	if quality <= 0 {
		q = 1.45
	}
	d := math.Abs(r - mdBHPhotonSphere)
	switch {
	case d < 0.45 && math.Abs(vr) < 0.34:
		return 0.010 * q
	case r < 3.5:
		return 0.016 * q
	case r < 5.0:
		return 0.028 * q
	case r < 8.0:
		return 0.050 * q
	case r < 14.0:
		return 0.110 * q
	default:
		return 0.220 * q
	}
}

func mdBHPointAndTangent(s mdBHOrbitState, impact float64, radialBasis, tangentBasis mdVec3) (mdVec3, mdVec3) {
	c, sn := math.Cos(s.phi), math.Sin(s.phi)
	er := mdBHAdd(mdBHMul(radialBasis, c), mdBHMul(tangentBasis, sn))
	ephi := mdBHAdd(mdBHMul(radialBasis, -sn), mdBHMul(tangentBasis, c))
	p := mdBHMul(er, s.r)
	t := mdBHAdd(mdBHMul(er, s.vr), mdBHMul(ephi, impact/math.Max(s.r, 1e-8)))
	return p, mdNormCore(t)
}

func mdBHSegmentDiskHitRange(a, b, diskNormal mdVec3, innerRadius float64) (mdVec3, bool) {
	n := mdNormCore(diskNormal)
	ha, hb := mdDotCore(a, n), mdDotCore(b, n)
	half := mdBHDiskHalfThickness
	insideA, insideB := math.Abs(ha) <= half, math.Abs(hb) <= half
	var t float64
	switch {
	case insideA:
		t = 0
	case insideB:
		t = 1
	case ha > half && hb < half:
		t = (ha - half) / (ha - hb)
	case ha < -half && hb > -half:
		t = (ha + half) / (ha - hb)
	default:
		return mdVec3{}, false
	}
	t = mdBHClamp(t, 0, 1)
	p := mdBHLerp(a, b, t)
	h := mdDotCore(p, n)
	planar := mdBHAdd(p, mdBHMul(n, -h))
	rho := mdBHLength(planar)
	if innerRadius <= 0 {
		innerRadius = mdBHDiskInnerRadius
	}
	if rho < innerRadius || rho > mdBHDiskOuterRadius {
		return mdVec3{}, false
	}
	return p, true
}

func mdBHSegmentDiskHit(a, b, diskNormal mdVec3) (mdVec3, bool) {
	return mdBHSegmentDiskHitRange(a, b, diskNormal, mdBHDiskInnerRadius)
}

// mdBHTraceSchwarzschildRay integrates an exact Schwarzschild null orbit in the
// unique plane through the black-hole centre, camera and initial ray. The disk
// is then intersected in three-dimensional space, so arbitrary observer/disk
// inclination still produces direct and higher-order lensed images.
func mdBHTraceSchwarzschildRay(camera, initialDir, diskNormal mdVec3, quality int, collectPath bool) (mdBHRayResult, []mdVec3) {
	cameraR := mdBHLength(camera)
	if cameraR <= mdBHEventHorizon {
		return mdBHRayResult{kind: mdBHRayCaptured}, nil
	}
	er0 := mdNormCore(camera)
	d := mdNormCore(initialDir)
	radialLocal := mdDotCore(d, er0)
	tangentVector := mdBHAdd(d, mdBHMul(er0, -radialLocal))
	tangentLen := mdBHLength(tangentVector)
	var e2 mdVec3
	if tangentLen < 1e-10 {
		e2 = mdVec3{1, 0, 0}
		if math.Abs(mdDotCore(e2, er0)) > 0.9 {
			e2 = mdVec3{0, 0, 1}
		}
		e2 = mdNormCore(mdBHAdd(e2, mdBHMul(er0, -mdDotCore(e2, er0))))
	} else {
		e2 = mdBHMul(tangentVector, 1/tangentLen)
	}
	f := 1.0 - 2.0*mdBHMass/cameraR
	impact := cameraR * tangentLen / math.Sqrt(math.Max(f, 1e-9))
	vr2 := 1.0 - f*impact*impact/(cameraR*cameraR)
	vr := -math.Sqrt(math.Max(vr2, 0))
	if radialLocal > 0 {
		vr = -vr // unusual outward-facing camera ray; keep the local sign coherent
	}
	state := mdBHOrbitState{r: cameraR, vr: vr, phi: 0}
	prevP := camera
	minR := cameraR
	maxSteps := 680
	if quality <= 0 {
		maxSteps = 360
	}
	var path []mdVec3
	if collectPath {
		path = make([]mdVec3, 0, maxSteps/3)
		path = append(path, camera)
	}
	for step := 0; step < maxSteps; step++ {
		h := mdBHAdaptiveStep(state.r, state.vr, quality)
		next := mdBHOrbitRK4(state, impact, h)
		if math.IsNaN(next.r) || math.IsInf(next.r, 0) || math.IsNaN(next.phi) || next.r <= 0 {
			return mdBHRayResult{kind: mdBHRayCaptured, minRadius: minR, steps: step}, path
		}
		p, tangent := mdBHPointAndTangent(next, impact, er0, e2)
		if collectPath && (step%2 == 0 || next.r <= mdBHEventHorizon*1.05) {
			path = append(path, p)
		}
		if next.r < minR {
			minR = next.r
		}
		if hit, ok := mdBHSegmentDiskHit(prevP, p, diskNormal); ok {
			return mdBHRayResult{
				kind:       mdBHRayDisk,
				hit:        hit,
				tangent:    tangent,
				minRadius:  minR,
				azimuth:    next.phi,
				steps:      step + 1,
				windingAbs: math.Abs(next.phi) / (2 * math.Pi),
			}, path
		}
		if next.r <= mdBHEventHorizon*1.002 {
			return mdBHRayResult{
				kind:       mdBHRayCaptured,
				tangent:    tangent,
				minRadius:  minR,
				azimuth:    next.phi,
				steps:      step + 1,
				windingAbs: math.Abs(next.phi) / (2 * math.Pi),
			}, path
		}
		if next.r >= cameraR*1.035 && next.vr > 0 && step > 16 {
			return mdBHRayResult{
				kind:       mdBHRayEscaped,
				exitDir:    tangent,
				tangent:    tangent,
				minRadius:  minR,
				azimuth:    next.phi,
				steps:      step + 1,
				windingAbs: math.Abs(next.phi) / (2 * math.Pi),
			}, path
		}
		state = next
		prevP = p
	}
	_, tangent := mdBHPointAndTangent(state, impact, er0, e2)
	return mdBHRayResult{
		kind:       mdBHRayTrapped,
		tangent:    tangent,
		minRadius:  minR,
		azimuth:    state.phi,
		steps:      maxSteps,
		windingAbs: math.Abs(state.phi) / (2 * math.Pi),
	}, path
}

type mdBHKerrState struct {
	r, theta, phi float64
}

type mdBHKerrDerivativeState struct {
	dr, dtheta, dphi float64
}

func mdBHKerrObserverBasis(spinAxis, camera mdVec3) (er, ex, ey, ez mdVec3, theta float64) {
	er = mdNormCore(camera)
	ez = mdNormCore(spinAxis)
	if mdBHLength(ez) < 1e-9 {
		ez = mdVec3{0, 0, 1}
	}
	cosTheta := mdBHClamp(mdDotCore(er, ez), -1, 1)
	theta = math.Acos(cosTheta)
	ex = mdBHAdd(er, mdBHMul(ez, -cosTheta))
	if mdBHLength(ex) < 1e-8 {
		ex = mdVec3{1, 0, 0}
		if math.Abs(mdDotCore(ex, ez)) > 0.9 {
			ex = mdVec3{0, 1, 0}
		}
		ex = mdNormCore(mdBHAdd(ex, mdBHMul(ez, -mdDotCore(ex, ez))))
	} else {
		ex = mdNormCore(ex)
	}
	ey = mdNormCore(mdCrossCore(ez, ex))
	return
}

func mdBHKerrPoint(state mdBHKerrState, ex, ey, ez mdVec3) mdVec3 {
	st, ct := math.Sin(state.theta), math.Cos(state.theta)
	cp, sp := math.Cos(state.phi), math.Sin(state.phi)
	dir := mdBHAdd(mdBHAdd(mdBHMul(ex, st*cp), mdBHMul(ey, st*sp)), mdBHMul(ez, ct))
	return mdBHMul(dir, state.r)
}

func mdBHKerrPotentials(state mdBHKerrState, a, lambda, q float64) (radial, polar, delta, sigma, pTerm float64) {
	r := state.r
	sinT, cosT := math.Sin(state.theta), math.Cos(state.theta)
	sin2 := math.Max(sinT*sinT, 1e-9)
	delta = r*r - 2*r + a*a
	sigma = r*r + a*a*cosT*cosT
	pTerm = r*r + a*a - a*lambda
	radial = pTerm*pTerm - delta*((lambda-a)*(lambda-a)+q)
	polar = q + a*a*cosT*cosT - lambda*lambda*cosT*cosT/sin2
	return
}

func mdBHKerrDerivative(state mdBHKerrState, a, lambda, q, radialSign, thetaSign float64) mdBHKerrDerivativeState {
	radial, polar, delta, sigma, pTerm := mdBHKerrPotentials(state, a, lambda, q)
	if sigma < 1e-9 {
		sigma = 1e-9
	}
	sinT := math.Sin(state.theta)
	sin2 := math.Max(sinT*sinT, 1e-9)
	if math.Abs(delta) < 1e-8 {
		if delta < 0 {
			delta = -1e-8
		} else {
			delta = 1e-8
		}
	}
	return mdBHKerrDerivativeState{
		dr:     radialSign * math.Sqrt(math.Max(radial, 0)) / sigma,
		dtheta: thetaSign * math.Sqrt(math.Max(polar, 0)) / sigma,
		dphi:   (lambda/sin2 - a + a*pTerm/delta) / sigma,
	}
}

func mdBHKerrStateAdd(s mdBHKerrState, d mdBHKerrDerivativeState, h float64) mdBHKerrState {
	return mdBHKerrState{s.r + d.dr*h, s.theta + d.dtheta*h, s.phi + d.dphi*h}
}

func mdBHKerrNormalizeAngles(s *mdBHKerrState, thetaSign *float64) {
	for s.theta < 0 || s.theta > math.Pi {
		if s.theta < 0 {
			s.theta = -s.theta
			s.phi += math.Pi
			*thetaSign = -*thetaSign
		}
		if s.theta > math.Pi {
			s.theta = 2*math.Pi - s.theta
			s.phi += math.Pi
			*thetaSign = -*thetaSign
		}
	}
}

// mdBHTraceKerrRay integrates separated Kerr null geodesics in Boyer-Lindquist
// coordinates. The spin axis is identical to the accretion-disk normal. At
// a*=0 callers retain the established Schwarzschild integrator exactly.
func mdBHTraceKerrRay(camera, initialDir, spinAxis mdVec3, spin float64, quality int, collectPath bool) (mdBHRayResult, []mdVec3) {
	a := mdBHKerrClampSpin(spin)
	if a < 1e-6 {
		return mdBHTraceSchwarzschildRay(camera, initialDir, spinAxis, quality, collectPath)
	}
	cameraR := mdBHLength(camera)
	horizon := mdBHKerrHorizon(a)
	if cameraR <= horizon {
		return mdBHRayResult{kind: mdBHRayCaptured}, nil
	}
	er, ex, ey, ez, thetaO := mdBHKerrObserverBasis(spinAxis, camera)
	d := mdNormCore(initialDir)
	inward := -mdDotCore(d, er)
	if inward < 1e-6 {
		inward = 1e-6
	}
	// Bardeen observer-screen coordinates at finite but distant r. The minus
	// sign on alpha preserves the conventional lambda=-alpha*sin(theta_o).
	alpha := -cameraR * mdDotCore(d, ey) / inward
	eTheta := mdNormCore(mdBHAdd(mdBHMul(er, math.Cos(thetaO)), mdBHMul(ez, -1)))
	if mdBHLength(eTheta) < 1e-8 {
		eTheta = ex
	}
	beta := cameraR * mdDotCore(d, eTheta) / inward
	sinO, cosO := math.Sin(thetaO), math.Cos(thetaO)
	lambda := -alpha * sinO
	q := beta*beta + cosO*cosO*(alpha*alpha-a*a)
	state := mdBHKerrState{r: cameraR, theta: thetaO, phi: 0}
	radialSign := -1.0
	thetaSign := 1.0
	if beta < 0 {
		thetaSign = -1
	}
	prevP := camera
	minR := cameraR
	maxSteps := 920
	if quality <= 0 {
		maxSteps = 520
	}
	var path []mdVec3
	if collectPath {
		path = make([]mdVec3, 0, maxSteps/3)
		path = append(path, camera)
	}
	innerRadius := mdBHKerrISCO(a)
	for step := 0; step < maxSteps; step++ {
		radial, polar, _, _, _ := mdBHKerrPotentials(state, a, lambda, q)
		if radial < 1e-8 && radialSign < 0 && state.r > horizon*1.01 {
			radialSign = 1
		}
		if polar < 1e-9 {
			thetaSign = -thetaSign
			state.theta += thetaSign * 1e-7
		}
		h := mdBHAdaptiveStep(state.r, radialSign, quality)
		// Kerr azimuth grows sharply near the horizon, so cap the affine step.
		if state.r < 4 {
			h = math.Min(h, 0.010)
		}
		if state.r < 2.5 {
			h = math.Min(h, 0.004)
		}
		d1 := mdBHKerrDerivative(state, a, lambda, q, radialSign, thetaSign)
		mid := mdBHKerrStateAdd(state, d1, h*0.5)
		mdBHKerrNormalizeAngles(&mid, &thetaSign)
		d2 := mdBHKerrDerivative(mid, a, lambda, q, radialSign, thetaSign)
		next := mdBHKerrStateAdd(state, d2, h)
		mdBHKerrNormalizeAngles(&next, &thetaSign)
		if math.IsNaN(next.r) || math.IsInf(next.r, 0) || math.IsNaN(next.theta) || math.IsNaN(next.phi) || next.r <= 0 {
			return mdBHRayResult{kind: mdBHRayCaptured, minRadius: minR, steps: step}, path
		}
		p := mdBHKerrPoint(next, ex, ey, ez)
		tangent := mdNormCore(mdBHAdd(p, mdBHMul(prevP, -1)))
		if collectPath && (step%2 == 0 || next.r <= horizon*1.05) {
			path = append(path, p)
		}
		if next.r < minR {
			minR = next.r
		}
		if hit, ok := mdBHSegmentDiskHitRange(prevP, p, spinAxis, innerRadius); ok {
			return mdBHRayResult{kind: mdBHRayDisk, hit: hit, tangent: tangent, minRadius: minR, azimuth: next.phi, steps: step + 1, windingAbs: math.Abs(next.phi) / (2 * math.Pi)}, path
		}
		if next.r <= horizon*1.002 {
			return mdBHRayResult{kind: mdBHRayCaptured, tangent: tangent, minRadius: minR, azimuth: next.phi, steps: step + 1, windingAbs: math.Abs(next.phi) / (2 * math.Pi)}, path
		}
		if next.r >= cameraR*1.035 && radialSign > 0 && step > 18 {
			return mdBHRayResult{kind: mdBHRayEscaped, exitDir: tangent, tangent: tangent, minRadius: minR, azimuth: next.phi, steps: step + 1, windingAbs: math.Abs(next.phi) / (2 * math.Pi)}, path
		}
		state, prevP = next, p
	}
	p := mdBHKerrPoint(state, ex, ey, ez)
	tangent := mdNormCore(mdBHAdd(p, mdBHMul(prevP, -1)))
	return mdBHRayResult{kind: mdBHRayTrapped, tangent: tangent, minRadius: minR, azimuth: state.phi, steps: maxSteps, windingAbs: math.Abs(state.phi) / (2 * math.Pi)}, path
}

func mdBHHash2(x, y int) uint32 {
	h := uint32(x)*0x9E3779B1 ^ uint32(y)*0x85EBCA77 ^ 0xC2B2AE3D
	h ^= h >> 16
	h *= 0x7FEB352D
	h ^= h >> 15
	h *= 0x846CA68B
	h ^= h >> 16
	return h
}

func mdBHBackgroundRGB(dir mdVec3) (float64, float64, float64) {
	d := mdNormCore(dir)
	lon := math.Atan2(d.x, d.y)
	lat := math.Asin(mdBHClamp(d.z, -1, 1))
	bandCentre := 0.18*math.Sin(2.0*lon) - 0.05*math.Sin(5.0*lon)
	band := math.Exp(-math.Pow((lat-bandCentre)/0.22, 2))
	r := 2.0 + 7.0*band
	g := 4.0 + 9.0*band
	b := 10.0 + 18.0*band
	ix := int(math.Floor((lon + math.Pi) / (2 * math.Pi) * 2048))
	iy := int(math.Floor((lat + math.Pi/2) / math.Pi * 1024))
	h := mdBHHash2(ix, iy)
	if h&0xFFFF > 65280 {
		v := 135.0 + float64((h>>16)&0x7F)
		warm := float64((h>>24)&0xFF) / 255.0
		r += v
		g += v * (0.80 + 0.20*warm)
		b += v * (0.72 + 0.28*(1-warm))
	}
	return mdBHClamp(r, 0, 255), mdBHClamp(g, 0, 255), mdBHClamp(b, 0, 255)
}

func mdBHTemperatureRGB(kelvin float64) (float64, float64, float64) {
	t := mdBHClamp(kelvin, 1000, 40000) / 100.0
	var r, g, b float64
	if t <= 66 {
		r = 255
		g = 99.4708025861*math.Log(t) - 161.1195681661
		if t <= 19 {
			b = 0
		} else {
			b = 138.5177312231*math.Log(t-10) - 305.0447927307
		}
	} else {
		r = 329.698727446 * math.Pow(t-60, -0.1332047592)
		g = 288.1221695283 * math.Pow(t-60, -0.0755148492)
		b = 255
	}
	return mdBHClamp(r, 0, 255), mdBHClamp(g, 0, 255), mdBHClamp(b, 0, 255)
}

func mdBHDiskBasis(normal mdVec3) (mdVec3, mdVec3) {
	n := mdNormCore(normal)
	ref := mdVec3{0, 0, 1}
	if math.Abs(mdDotCore(n, ref)) > 0.90 {
		ref = mdVec3{1, 0, 0}
	}
	u := mdNormCore(mdCrossCore(ref, n))
	v := mdNormCore(mdCrossCore(n, u))
	return u, v
}

func mdBHDiskRGB(result mdBHRayResult, diskNormal mdVec3, spin float64) (float64, float64, float64) {
	n := mdNormCore(diskNormal)
	h := mdDotCore(result.hit, n)
	planar := mdBHAdd(result.hit, mdBHMul(n, -h))
	radius := mdBHLength(planar)
	if radius <= 0 {
		return 0, 0, 0
	}
	radial := mdBHMul(planar, 1/radius)
	azimuthal := mdNormCore(mdCrossCore(n, radial))
	a := mdBHKerrClampSpin(spin)
	// Equatorial prograde Kerr orbital angular velocity Ω=1/(r^(3/2)+a).
	// Convert it to a bounded local display speed using the equatorial lapse.
	omega := 1.0 / (math.Pow(radius, 1.5) + a)
	delta := radius*radius - 2*radius + a*a
	area := math.Pow(radius*radius+a*a, 2) - a*a*delta
	lapse := math.Sqrt(math.Max(delta*radius*radius/math.Max(area, 1e-9), 0.02))
	beta := radius * omega / math.Max(lapse, 0.12)
	beta = mdBHClamp(beta, 0, 0.72)
	toObserver := mdBHMul(mdNormCore(result.tangent), -1)
	betaLOS := beta * mdDotCore(azimuthal, toObserver)
	gamma := 1.0 / math.Sqrt(math.Max(1-beta*beta, 1e-6))
	doppler := 1.0 / (gamma * math.Max(1-betaLOS, 0.18))
	grav := lapse

	innerRadius := mdBHKerrISCO(a)
	t := mdBHClamp((radius-innerRadius)/(mdBHDiskOuterRadius-innerRadius), 0, 1)
	baseTemp := mdBHInnerTemperature*math.Pow(1-t, 0.72) + mdBHOuterTemperature*math.Pow(t, 0.82)
	observedTemp := baseTemp * grav * doppler
	r, g, b := mdBHTemperatureRGB(observedTemp)

	uAxis, vAxis := mdBHDiskBasis(n)
	phi := math.Atan2(mdDotCore(radial, vAxis), mdDotCore(radial, uAxis))
	texture := 0.90 + 0.055*math.Sin(9*phi+0.75*radius) + 0.035*math.Sin(23*phi-0.34*radius)
	radialGlow := 0.40 + 0.95*math.Pow(1-t, 1.35)
	redshiftIntensity := math.Pow(grav*doppler, 3)
	higherOrder := math.Pow(0.80, math.Max(0, result.windingAbs-0.20))
	intensity := mdBHFixedBrightness * texture * radialGlow * redshiftIntensity * higherOrder
	if result.minRadius < 3.35 {
		ring := math.Exp(-math.Pow((result.minRadius-mdBHPhotonSphere)/0.20, 2))
		intensity *= 1.0 + 1.10*ring
	}
	return mdBHClamp(r*intensity, 0, 255), mdBHClamp(g*intensity, 0, 255), mdBHClamp(b*intensity, 0, 255)
}

func mdBHRayRGB(result mdBHRayResult, diskNormal mdVec3, spin float64) (byte, byte, byte) {
	var r, g, b float64
	switch result.kind {
	case mdBHRayDisk:
		r, g, b = mdBHDiskRGB(result, diskNormal, spin)
	case mdBHRayEscaped:
		r, g, b = mdBHBackgroundRGB(result.exitDir)
		if result.minRadius < 3.34 {
			ring := math.Exp(-math.Pow((result.minRadius-mdBHPhotonSphere)/0.16, 2))
			gain := 55.0 * ring * mdBHClamp(0.5+result.windingAbs, 0.5, 2.0)
			r += gain
			g += gain * 0.72
			b += gain * 0.35
		}
	case mdBHRayTrapped:
		// Near-critical rays that do not resolve within the step budget stay dark;
		// the visible photon ring is supplied only by escaped or disk-hit rays.
		r, g, b = 0.8, 1.0, 1.8
	default:
		r, g, b = 0.8, 1.0, 1.8
	}
	return byte(mdBHClamp(r, 0, 255) + 0.5), byte(mdBHClamp(g, 0, 255) + 0.5), byte(mdBHClamp(b, 0, 255) + 0.5)
}

func mdBHBlendPixel(pix []byte, w, h, x, y int, r, g, b byte, alpha float64) {
	if x < 0 || x >= w || y < 0 || y >= h || alpha <= 0 {
		return
	}
	alpha = mdBHClamp(alpha, 0, 1)
	i := (y*w + x) * 4
	pix[i+2] = byte(float64(pix[i+2])*(1-alpha) + float64(r)*alpha + 0.5)
	pix[i+1] = byte(float64(pix[i+1])*(1-alpha) + float64(g)*alpha + 0.5)
	pix[i+0] = byte(float64(pix[i+0])*(1-alpha) + float64(b)*alpha + 0.5)
}

func mdBHDrawLine(pix []byte, w, h int, x0, y0, x1, y1 float64, r, g, b byte, alpha float64, thickness int) {
	dx, dy := x1-x0, y1-y0
	steps := int(math.Ceil(math.Max(math.Abs(dx), math.Abs(dy))))
	if steps < 1 {
		steps = 1
	}
	if thickness < 1 {
		thickness = 1
	}
	rad := thickness / 2
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(x0 + dx*t))
		y := int(math.Round(y0 + dy*t))
		for oy := -rad; oy <= rad; oy++ {
			for ox := -rad; ox <= rad; ox++ {
				if ox*ox+oy*oy > rad*rad+1 {
					continue
				}
				mdBHBlendPixel(pix, w, h, x+ox, y+oy, r, g, b, alpha)
			}
		}
	}
}

func mdBHDrawCircle(pix []byte, w, h int, cx, cy, radius float64, r, g, b byte, alpha float64, thickness int, dashed bool) {
	steps := int(math.Max(48, radius*8))
	var px, py float64
	for i := 0; i <= steps; i++ {
		a := 2 * math.Pi * float64(i) / float64(steps)
		x := cx + radius*math.Cos(a)
		y := cy + radius*math.Sin(a)
		if i > 0 && (!dashed || (i/4)%2 == 0) {
			mdBHDrawLine(pix, w, h, px, py, x, y, r, g, b, alpha, thickness)
		}
		px, py = x, y
	}
}

func mdBHTracePlanarOrbit(impact float64, quality int, spins ...float64) ([][2]float64, bool) {
	spin := 0.0
	if len(spins) > 0 {
		spin = spins[0]
	}
	r := mdBHCameraRadius
	f := 1 - 2*mdBHMass/r
	vr := -math.Sqrt(math.Max(1-f*impact*impact/(r*r), 0))
	s := mdBHOrbitState{r: r, vr: vr}
	maxSteps := 980
	if quality <= 0 {
		maxSteps = 520
	}
	pts := make([][2]float64, 0, maxSteps/2)
	captured := false
	for i := 0; i < maxSteps; i++ {
		if i%2 == 0 {
			pts = append(pts, [2]float64{s.r * math.Cos(s.phi), s.r * math.Sin(s.phi)})
		}
		h := mdBHAdaptiveStep(s.r, s.vr, quality)
		s = mdBHOrbitRK4(s, impact, h)
		a := mdBHKerrClampSpin(spin)
		if a > 0 {
			s.phi += h * (2 * a / math.Max(s.r*s.r*s.r, 1e-6))
		}
		horizon := mdBHKerrHorizon(a)
		if s.r <= horizon*1.002 {
			captured = true
			pts = append(pts, [2]float64{s.r * math.Cos(s.phi), s.r * math.Sin(s.phi)})
			break
		}
		if s.r >= mdBHCameraRadius*1.03 && s.vr > 0 && i > 16 {
			pts = append(pts, [2]float64{s.r * math.Cos(s.phi), s.r * math.Sin(s.phi)})
			break
		}
	}
	return pts, captured
}

func mdBHDrawGeodesicDiagram(pix []byte, w, h int, inset bool, quality int, spin float64) {
	left, top, right, bottom := 0, 0, w, h
	if inset {
		boxW := int(float64(w) * 0.43)
		boxH := int(float64(h) * 0.37)
		left = w - boxW - max(4, w/80)
		top = h - boxH - max(4, h/80)
		right, bottom = left+boxW, top+boxH
		for y := top; y < bottom; y++ {
			for x := left; x < right; x++ {
				mdBHBlendPixel(pix, w, h, x, y, 2, 4, 10, 0.88)
			}
		}
		mdBHDrawLine(pix, w, h, float64(left), float64(top), float64(right-1), float64(top), 80, 104, 138, 0.8, 1)
		mdBHDrawLine(pix, w, h, float64(right-1), float64(top), float64(right-1), float64(bottom-1), 80, 104, 138, 0.8, 1)
		mdBHDrawLine(pix, w, h, float64(right-1), float64(bottom-1), float64(left), float64(bottom-1), 80, 104, 138, 0.8, 1)
		mdBHDrawLine(pix, w, h, float64(left), float64(bottom-1), float64(left), float64(top), 80, 104, 138, 0.8, 1)
	} else {
		for i := 0; i < w*h; i++ {
			pix[i*4+0], pix[i*4+1], pix[i*4+2], pix[i*4+3] = 8, 4, 2, 0
		}
	}
	cw, ch := float64(right-left), float64(bottom-top)
	cx, cy := float64(left)+cw*0.53, float64(top)+ch*0.50
	scale := math.Min(cw, ch) / 68.0
	worldToScreen := func(p [2]float64) (float64, float64) {
		return cx + p[0]*scale, cy - p[1]*scale
	}
	mdBHDrawCircle(pix, w, h, cx, cy, mdBHDiskOuterRadius*scale, 104, 55, 20, 0.32, 1, false)
	mdBHDrawCircle(pix, w, h, cx, cy, mdBHKerrISCO(spin)*scale, 232, 116, 28, 0.58, 1, false)
	mdBHDrawCircle(pix, w, h, cx, cy, mdBHPhotonSphere*scale, 255, 193, 72, 0.92, max(1, int(scale*0.35)), true)
	// Fill the event horizon.
	horizon := mdBHKerrHorizon(spin)
	hr := int(math.Ceil(horizon * scale))
	for yy := -hr; yy <= hr; yy++ {
		for xx := -hr; xx <= hr; xx++ {
			if float64(xx*xx+yy*yy) <= math.Pow(horizon*scale, 2) {
				mdBHBlendPixel(pix, w, h, int(math.Round(cx))+xx, int(math.Round(cy))+yy, 0, 0, 0, 1)
			}
		}
	}
	mdBHDrawCircle(pix, w, h, cx, cy, horizon*scale, 220, 150, 56, 0.9, 1, false)

	impacts := []float64{-8.5, -6.6, -5.45, -5.22, -4.85, 4.85, 5.22, 5.45, 6.6, 8.5}
	for _, impact := range impacts {
		pts, captured := mdBHTracePlanarOrbit(impact, quality, spin)
		if len(pts) < 2 {
			continue
		}
		r, g, b := byte(94), byte(211), byte(255)
		if captured {
			r, g, b = 255, 102, 70
		} else if math.Abs(impact-mdBHCriticalImpact) < 0.30 || math.Abs(impact+mdBHCriticalImpact) < 0.30 {
			r, g, b = 255, 222, 86
		}
		for i := 1; i < len(pts); i++ {
			x0, y0 := worldToScreen(pts[i-1])
			x1, y1 := worldToScreen(pts[i])
			mdBHDrawLine(pix, w, h, x0, y0, x1, y1, r, g, b, 0.90, max(1, int(scale*0.30)))
		}
	}
}

// mdBHSpatialWarpRadius maps a regular Euclidean reference radius to the
// displayed radius of the educational 3-D coordinate lattice.  Unlike the
// The former one-sided 4-D height embedding had a preferred "down" direction; this map does not.
// Every point is displaced only along the direction toward the black-hole
// centre, so off-axis Cartesian coordinate lines visibly bow inward while the
// black hole remains at the centre of the 3-D lattice.
//
// This is a pedagogical coordinate deformation, not an isometric embedding of
// the full Schwarzschild spatial metric.  The exact relativistic calculation
// remains available in the ray-traced and geodesic modes.
func mdBHSpatialWarpRadiusSpin(r, outerRadius, spin float64) float64 {
	rs := mdBHKerrHorizon(spin)
	if outerRadius <= rs {
		outerRadius = rs + 1
	}
	if r <= rs {
		return 0.72 * rs
	}
	if r >= outerRadius {
		return r
	}
	u := mdBHClamp((r-rs)/(outerRadius-rs), 0, 1)
	// Strong local pull near the horizon, smoothly vanishing at the outer
	// lattice boundary.  The exponential keeps distant grid lines almost
	// Euclidean while the squared edge term makes the boundary exact.
	falloff := math.Exp(-(r-rs)/(6.0*mdBHMass)) * (1 - u) * (1 - u)
	pull := 5.00 * mdBHMass * falloff
	warped := r - pull
	minimum := 0.72 * rs
	if warped < minimum {
		warped = minimum
	}
	return warped
}

func mdBHSpatialWarpRadius(r, outerRadius float64) float64 {
	return mdBHSpatialWarpRadiusSpin(r, outerRadius, 0)
}

func mdBHSpatialWarpPointSpin(p mdVec3, outerRadius, spin float64) mdVec3 {
	r := mdBHLength(p)
	if r <= 1e-12 {
		return mdVec3{}
	}
	a := mdBHKerrClampSpin(spin)
	wr := mdBHSpatialWarpRadiusSpin(r, outerRadius, a)
	q := mdBHMul(p, wr/r)
	if a > 1e-8 {
		horizon := mdBHKerrHorizon(a)
		u := mdBHClamp((r-horizon)/math.Max(outerRadius-horizon, 1e-6), 0, 1)
		// Integral-like display twist from Ω_drag≈2a/r^3. It vanishes exactly
		// at the lattice boundary and grows smoothly toward the ergoregion.
		twist := 2.35 * a * math.Exp(-(r-horizon)/4.2) * (1 - u) * (1 - u)
		c, sn := math.Cos(twist), math.Sin(twist)
		q.x, q.y = q.x*c-q.y*sn, q.x*sn+q.y*c
	}
	return q
}

func mdBHSpatialWarpPoint(p mdVec3, outerRadius float64) mdVec3 {
	return mdBHSpatialWarpPointSpin(p, outerRadius, 0)
}

type mdBHCurvatureSegment struct {
	x0, y0, x1, y1 float64
	depth          float64
	axis           int // 0=x family, 1=y family, 2=z family
	level          int // 0=minor, 1=coordinate plane, 2=principal axis
	alpha          float64
}

func mdBHSpatialProject3D(p mdVec3, params mdBHRenderParams, w, h int, scale float64) (float64, float64, float64, float64) {
	q := mdMat3Apply(mdEulerRotationMatrix(params.rotationX, params.rotationY, params.rotationZ), p)
	cameraDistance := params.cameraDistance
	if cameraDistance <= 0 {
		cameraDistance = 48.0
	}
	denom := cameraDistance - q.y
	if denom < 4 {
		denom = 4
	}
	// Keep the virtual lens fixed at the old 48M reference. Moving the camera
	// closer now enlarges the central black hole and strengthens perspective,
	// instead of merely changing the perspective distortion at constant size.
	perspective := 48.0 / denom
	sx := float64(w)*0.5 + q.x*scale*perspective
	sy := float64(h)*0.49 - q.z*scale*perspective
	return sx, sy, q.y, perspective
}

func mdBHSpatialRingSegments(radius, physicalZ, outerRadius float64, params mdBHRenderParams, w, h int, scale float64) []mdBHCurvatureSegment {
	steps := 240
	if params.quality <= 0 {
		steps = 136
	}
	out := make([]mdBHCurvatureSegment, 0, steps)
	var prevX, prevY, prevDepth float64
	for i := 0; i <= steps; i++ {
		a := 2 * math.Pi * float64(i) / float64(steps)
		p := mdVec3{x: radius * math.Cos(a), y: radius * math.Sin(a), z: physicalZ}
		p = mdBHSpatialWarpPointSpin(p, outerRadius, params.spin)
		x, y, depth, _ := mdBHSpatialProject3D(p, params, w, h, scale)
		if i > 0 {
			out = append(out, mdBHCurvatureSegment{x0: prevX, y0: prevY, x1: x, y1: y, depth: (prevDepth + depth) * 0.5, alpha: 1})
		}
		prevX, prevY, prevDepth = x, y, depth
	}
	return out
}

func mdBHDrawCurvatureGrid(pix []byte, w, h int, params mdBHRenderParams) {
	if w < 1 || h < 1 {
		return
	}
	// The curvature-grid mode contains only the Cartesian coordinate lattice and
	// the central black hole. Other modes keep the accretion disk, photon ring,
	// star field and geodesic overlays. A white key background lets insertion
	// preserve transparency around the grid subject.
	for i := 0; i < w*h; i++ {
		pix[i*4+0], pix[i*4+1], pix[i*4+2], pix[i*4+3] = 255, 255, 255, 0
	}

	extent := 11.0
	outerRadius := 13.0
	scale := 0.80 * math.Min(float64(w), float64(h)) / (2 * extent)
	if scale < 1 {
		scale = 1
	}

	density := params.gridDensity
	if density < 3 {
		density = 5
	}
	if density > 17 {
		density = 17
	}
	if density%2 == 0 {
		density++
	}
	samples := 112
	switch {
	case params.quality >= 2:
		samples = 288
	case params.quality == 1:
		samples = 192
	}
	half := density / 2
	values := make([]float64, 0, density)
	for i := -half; i <= half; i++ {
		values = append(values, 8.0*float64(i)/float64(half))
	}
	segments := make([]mdBHCurvatureSegment, 0, 70000)
	appendLine := func(axis int, fixedA, fixedB float64) {
		level := 0
		if math.Abs(fixedA) < 1e-8 && math.Abs(fixedB) < 1e-8 {
			level = 2
		} else if math.Abs(fixedA) < 1e-8 || math.Abs(fixedB) < 1e-8 {
			level = 1
		}
		var lastX, lastY, lastDepth float64
		haveLast := false
		for i := 0; i <= samples; i++ {
			t := -extent + 2*extent*float64(i)/float64(samples)
			p := mdVec3{}
			switch axis {
			case 0:
				p = mdVec3{x: t, y: fixedA, z: fixedB}
			case 1:
				p = mdVec3{x: fixedA, y: t, z: fixedB}
			default:
				p = mdVec3{x: fixedA, y: fixedB, z: t}
			}
			r := mdBHLength(p)
			if r < mdBHKerrHorizon(params.spin)*1.045 {
				haveLast = false
				continue
			}
			p = mdBHSpatialWarpPointSpin(p, outerRadius, params.spin)
			sx, sy, depth, _ := mdBHSpatialProject3D(p, params, w, h, scale)
			if haveLast {
				fade := mdBHClamp(0.70+0.018*(depth+extent), 0.48, 0.98)
				segments = append(segments, mdBHCurvatureSegment{
					x0: lastX, y0: lastY, x1: sx, y1: sy,
					depth: (lastDepth + depth) * 0.5,
					axis:  axis, level: level, alpha: fade,
				})
			}
			lastX, lastY, lastDepth = sx, sy, depth
			haveLast = true
		}
	}
	for _, a := range values {
		for _, b := range values {
			appendLine(0, a, b)
			appendLine(1, a, b)
			appendLine(2, a, b)
		}
	}
	sort.SliceStable(segments, func(i, j int) bool { return segments[i].depth < segments[j].depth })

	centreX, centreY, centreDepth, centrePerspective := mdBHSpatialProject3D(mdVec3{}, params, w, h, scale)
	drawGridSegment := func(seg mdBHCurvatureSegment) {
		thickness := max(1, int(math.Round(float64(w)/720.0)))
		alpha := 0.35 * seg.alpha
		r, g, b := byte(39), byte(112), byte(164)
		if seg.level == 1 {
			alpha = 0.58 * seg.alpha
			r, g, b = 48, 147, 198
		}
		if seg.level == 2 {
			thickness = max(1, int(math.Round(scale*0.26)))
			alpha = 0.96 * seg.alpha
			switch seg.axis {
			case 0:
				r, g, b = 230, 56, 54
			case 1:
				r, g, b = 45, 174, 91
			default:
				r, g, b = 53, 100, 225
			}
		}
		if params.maskPass {
			r, g, b = 0, 0, 0
		}
		mdBHDrawLine(pix, w, h, seg.x0, seg.y0, seg.x1, seg.y1, r, g, b, alpha, thickness)
	}
	for _, seg := range segments {
		if seg.depth < centreDepth {
			drawGridSegment(seg)
		}
	}

	// The black hole itself is the only non-grid object. No accretion ring,
	// photon-sphere guide or glow is drawn in this final simplified mode.
	holeWorldRadius := mdBHSpatialWarpRadiusSpin(mdBHKerrHorizon(params.spin), outerRadius, params.spin)
	holeRadiusPx := holeWorldRadius * scale * centrePerspective
	hr := int(math.Ceil(holeRadiusPx))
	for yy := -hr; yy <= hr; yy++ {
		for xx := -hr; xx <= hr; xx++ {
			if float64(xx*xx+yy*yy) <= holeRadiusPx*holeRadiusPx {
				mdBHBlendPixel(pix, w, h, int(math.Round(centreX))+xx, int(math.Round(centreY))+yy, 0, 0, 0, 1)
			}
		}
	}
	mdBHDrawCircle(pix, w, h, centreX, centreY, holeRadiusPx, 0, 0, 0, 1, max(1, int(scale*0.18)), false)

	for _, seg := range segments {
		if seg.depth >= centreDepth {
			drawGridSegment(seg)
		}
	}
}

// mdBHRender produces a top-down BGRA bitmap ready for StretchDIBits.
func mdBHRender(params mdBHRenderParams) []byte {
	w, h := params.width, params.height
	if w < 1 || h < 1 {
		return nil
	}
	pix := make([]byte, w*h*4)
	if params.mode == mdBlackHoleModeCurvatureGrid {
		mdBHDrawCurvatureGrid(pix, w, h, params)
		return pix
	}
	if params.mode == mdBlackHoleModeGeodesics {
		mdBHDrawGeodesicDiagram(pix, w, h, false, params.quality, params.spin)
		return pix
	}
	camera := mdVec3{0, mdBHCameraRadius, 0}
	forward := mdVec3{0, -1, 0}
	right := mdVec3{1, 0, 0}
	up := mdVec3{0, 0, 1}
	fovY := mdBHClamp(params.fovY, 10*math.Pi/180, 85*math.Pi/180)
	tanY := math.Tan(fovY * 0.5)
	aspect := float64(w) / float64(h)
	tanX := tanY * aspect
	normal := mdNormCore(params.diskNormal)
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > h {
		workers = h
	}
	rows := make(chan int, h)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for y := range rows {
				ny := 1.0 - 2.0*(float64(y)+0.5)/float64(h)
				for x := 0; x < w; x++ {
					nx := 2.0*(float64(x)+0.5)/float64(w) - 1.0
					dir := mdNormCore(mdBHAdd(forward, mdBHAdd(mdBHMul(right, nx*tanX), mdBHMul(up, ny*tanY))))
					res, _ := mdBHTraceKerrRay(camera, dir, normal, params.spin, params.quality, false)
					r, g, b := mdBHRayRGB(res, normal, params.spin)
					i := (y*w + x) * 4
					pix[i+0], pix[i+1], pix[i+2], pix[i+3] = b, g, r, 0
				}
			}
		}()
	}
	for y := 0; y < h; y++ {
		rows <- y
	}
	close(rows)
	wg.Wait()
	if params.mode == mdBlackHoleModeRenderWithDiagram {
		mdBHDrawGeodesicDiagram(pix, w, h, true, params.quality, params.spin)
	}
	return pix
}
