package main

import (
	"math"
	"testing"
)

func TestBlackHoleFixedPhysicalDefaults(t *testing.T) {
	if mdBHEventHorizon != 2 || mdBHPhotonSphere != 3 {
		t.Fatalf("unexpected Schwarzschild radii: horizon=%v photon=%v", mdBHEventHorizon, mdBHPhotonSphere)
	}
	if mdBHDiskInnerRadius != 6 || mdBHDiskOuterRadius != 15 {
		t.Fatalf("unexpected disk radii: inner=%v outer=%v", mdBHDiskInnerRadius, mdBHDiskOuterRadius)
	}
	if math.Abs(mdBHCriticalImpact-3*math.Sqrt(3)) > 1e-12 {
		t.Fatalf("critical impact mismatch: %v", mdBHCriticalImpact)
	}
	if mdBHDiskHalfThickness <= 0 || mdBHDiskHalfThickness >= 1 {
		t.Fatalf("disk half-thickness out of expected fixed range: %v", mdBHDiskHalfThickness)
	}
}

func TestBlackHolePlanarNullOrbitCaptureAndEscape(t *testing.T) {
	capturedPath, captured := mdBHTracePlanarOrbit(4.8, 1)
	if !captured || len(capturedPath) < 4 {
		t.Fatalf("subcritical ray should be captured: captured=%v points=%d", captured, len(capturedPath))
	}
	escapedPath, escapedCaptured := mdBHTracePlanarOrbit(8.0, 1)
	if escapedCaptured || len(escapedPath) < 4 {
		t.Fatalf("supercritical ray should escape: captured=%v points=%d", escapedCaptured, len(escapedPath))
	}
}

func TestBlackHoleNearCriticalRayWindsMore(t *testing.T) {
	near, capturedNear := mdBHTracePlanarOrbit(mdBHCriticalImpact+0.035, 1)
	far, capturedFar := mdBHTracePlanarOrbit(8.5, 1)
	if capturedNear || capturedFar {
		t.Fatalf("comparison rays should both escape: near=%v far=%v", capturedNear, capturedFar)
	}
	if len(near) <= len(far)+20 {
		t.Fatalf("near-critical ray should spend longer around photon sphere: near=%d far=%d", len(near), len(far))
	}
}

func TestBlackHoleSpatialWarpHasNoPreferredDownDirection(t *testing.T) {
	outer := 15.0
	points := []mdVec3{{6, 0, 0}, {0, 6, 0}, {0, 0, 6}, {-2, 4, -4}}
	wantRadius := mdBHSpatialWarpRadius(6, outer)
	for _, p := range points {
		if math.Abs(mdBHLength(p)-6) > 1e-12 {
			continue
		}
		q := mdBHSpatialWarpPoint(p, outer)
		if math.Abs(mdBHLength(q)-wantRadius) > 1e-12 {
			t.Fatalf("spherical warp radius mismatch: p=%+v q=%+v want=%v", p, q, wantRadius)
		}
		cross := mdCrossCore(p, q)
		if mdBHLength(cross) > 1e-10 {
			t.Fatalf("warp must move only toward the black-hole centre: p=%+v q=%+v cross=%+v", p, q, cross)
		}
	}
}

func TestBlackHoleSpatialWarpPullsNearPointsMoreStrongly(t *testing.T) {
	outer := 15.0
	nearR, farR := 3.0, 11.0
	nearPull := nearR - mdBHSpatialWarpRadius(nearR, outer)
	farPull := farR - mdBHSpatialWarpRadius(farR, outer)
	if !(nearPull > farPull && farPull >= 0) {
		t.Fatalf("central warp should pull near points more strongly: near=%v far=%v", nearPull, farPull)
	}
	if math.Abs(mdBHSpatialWarpRadius(outer, outer)-outer) > 1e-12 {
		t.Fatalf("outer grid boundary must remain fixed")
	}
}

func TestBlackHoleOffsetCartesianLineBowsTowardCentre(t *testing.T) {
	outer := 15.0
	end := mdBHSpatialWarpPoint(mdVec3{x: 10, y: 4, z: 0}, outer)
	mid := mdBHSpatialWarpPoint(mdVec3{x: 0, y: 4, z: 0}, outer)
	if !(mid.y < end.y) {
		t.Fatalf("offset coordinate line should bow toward centre: midpoint y=%v endpoint y=%v", mid.y, end.y)
	}
	if math.Abs(mid.x) > 1e-12 || math.Abs(mid.z) > 1e-12 {
		t.Fatalf("radial warp changed midpoint direction: %+v", mid)
	}
}

func TestBlackHoleSpatialProjectionUsesOnlyXYZRotation(t *testing.T) {
	p := mdBHSpatialWarpPoint(mdVec3{x: 5, y: 4, z: 3}, 15)
	aX, aY, _, _ := mdBHSpatialProject3D(p, mdBHRenderParams{}, 320, 240, 5)
	bX, bY, _, _ := mdBHSpatialProject3D(p, mdBHRenderParams{rotationZ: 0.8, rotationW: 2.0}, 320, 240, 5)
	cX, cY, _, _ := mdBHSpatialProject3D(p, mdBHRenderParams{rotationZ: 0.8, rotationW: -2.0}, 320, 240, 5)
	if math.Hypot(aX-bX, aY-bY) < 0.5 {
		t.Fatalf("ordinary 3-D rotation should change projection")
	}
	if math.Hypot(bX-cX, bY-cY) > 1e-12 {
		t.Fatalf("W must not affect the central 3-D coordinate-grid view")
	}
}

func TestBlackHoleRenderContainsShadowAndLuminousDisk(t *testing.T) {
	n := mdNormCore(mdVec3{0, math.Cos(65 * math.Pi / 180), math.Sin(65 * math.Pi / 180)})
	w, h := 96, 72
	pix := mdBHRender(mdBHRenderParams{
		width: w, height: h, fovY: 50 * math.Pi / 180,
		diskNormal: n, mode: mdBlackHoleModeRender, quality: 0,
	})
	if len(pix) != w*h*4 {
		t.Fatalf("unexpected pixel buffer size: %d", len(pix))
	}
	centre := ((h/2)*w + w/2) * 4
	centreLuma := int(pix[centre]) + int(pix[centre+1]) + int(pix[centre+2])
	if centreLuma > 35 {
		t.Fatalf("black-hole centre should remain dark, luma sum=%d", centreLuma)
	}
	bright := 0
	for i := 0; i < len(pix); i += 4 {
		if int(pix[i])+int(pix[i+1])+int(pix[i+2]) > 420 {
			bright++
		}
	}
	if bright < 20 {
		t.Fatalf("expected a visible hot accretion disk, bright pixels=%d", bright)
	}
}

func TestBlackHoleGeodesicDiagramProducesColoredPaths(t *testing.T) {
	w, h := 120, 90
	pix := mdBHRender(mdBHRenderParams{width: w, height: h, mode: mdBlackHoleModeGeodesics, quality: 0})
	cyan, orange := 0, 0
	for i := 0; i < len(pix); i += 4 {
		b, g, r := pix[i], pix[i+1], pix[i+2]
		if b > 160 && g > 120 && r < 170 {
			cyan++
		}
		if r > 180 && g > 60 && b < 130 {
			orange++
		}
	}
	if cyan == 0 || orange == 0 {
		t.Fatalf("geodesic diagram missing path classes: cyan=%d orange=%d", cyan, orange)
	}
}

func TestBlackHoleDisplayedHorizonRemainsCentralAndFinite(t *testing.T) {
	outer := 15.0
	h := mdBHSpatialWarpRadius(mdBHEventHorizon, outer)
	if !(h > 0 && h < mdBHEventHorizon) {
		t.Fatalf("displayed horizon radius should remain finite and centrally compressed: %v", h)
	}
}

func TestBlackHoleCurvatureGridContainsMeshCoreAndAccretionColour(t *testing.T) {
	w, h := 180, 140
	pix := mdBHRender(mdBHRenderParams{
		width: w, height: h, mode: mdBlackHoleModeCurvatureGrid, quality: 1,
		rotationX: 1.1, rotationY: 0.25, rotationZ: 0.35,
	})
	if len(pix) != w*h*4 {
		t.Fatalf("unexpected pixel buffer size: %d", len(pix))
	}
	cyan, orange, black := 0, 0, 0
	redAxis, greenAxis, blueAxis := 0, 0, 0
	for i := 0; i < len(pix); i += 4 {
		b, g, r := pix[i], pix[i+1], pix[i+2]
		if b > 120 && g > 100 && r < 150 {
			cyan++
		}
		if r > 170 && g > 55 && b < 120 {
			orange++
		}
		if r > 150 && g < 145 && b < 155 {
			redAxis++
		}
		if g > 150 && r < 145 && b < 180 {
			greenAxis++
		}
		if b > 155 && r < 145 && g < 195 {
			blueAxis++
		}
		if int(r)+int(g)+int(b) < 10 {
			black++
		}
	}
	if cyan < 80 || orange < 10 || black < 20 || redAxis == 0 || greenAxis == 0 || blueAxis == 0 {
		t.Fatalf("3-D curvature lattice missing expected layers: cyan=%d orange=%d black=%d xyz=%d/%d/%d", cyan, orange, black, redAxis, greenAxis, blueAxis)
	}
}

func TestKerrHorizonAndProgradeISCOA215(t *testing.T) {
	if got := mdBHKerrHorizon(0); math.Abs(got-2) > 1e-12 {
		t.Fatalf("a*=0 horizon must equal Schwarzschild: %v", got)
	}
	if got := mdBHKerrISCO(0); math.Abs(got-6) > 1e-12 {
		t.Fatalf("a*=0 ISCO must equal 6M: %v", got)
	}
	h99 := mdBHKerrHorizon(0.99)
	i99 := mdBHKerrISCO(0.99)
	if !(h99 > 1 && h99 < 1.2) {
		t.Fatalf("near-extreme Kerr horizon out of range: %v", h99)
	}
	if !(i99 > h99 && i99 < 2) {
		t.Fatalf("prograde near-extreme ISCO should approach the horizon: horizon=%v isco=%v", h99, i99)
	}
}

func TestKerrSpatialGridTwistsWithoutMovingOuterBoundaryA215(t *testing.T) {
	outer := 13.0
	p := mdVec3{x: 4, y: 1, z: 2}
	zero := mdBHSpatialWarpPointSpin(p, outer, 0)
	spin := mdBHSpatialWarpPointSpin(p, outer, 0.9)
	if math.Hypot(zero.x-spin.x, zero.y-spin.y) < 0.05 {
		t.Fatalf("Kerr spin should azimuthally drag the inner lattice: zero=%+v spin=%+v", zero, spin)
	}
	boundary := mdVec3{x: outer, y: 0, z: 0}
	q := mdBHSpatialWarpPointSpin(boundary, outer, 0.99)
	if math.Hypot(q.x-boundary.x, q.y-boundary.y) > 1e-9 || math.Abs(q.z-boundary.z) > 1e-9 {
		t.Fatalf("outer lattice boundary must remain fixed: got=%+v want=%+v", q, boundary)
	}
}

func TestBlackHoleGridDensityAddsCoordinateLinesA215(t *testing.T) {
	countInk := func(density int) int {
		pix := mdBHRender(mdBHRenderParams{width: 220, height: 170, mode: mdBlackHoleModeCurvatureGrid, quality: 0, gridDensity: density, spin: 0.7, rotationX: 0.8, rotationY: 0.25, rotationZ: 0.2})
		ink := 0
		for i := 0; i < len(pix); i += 4 {
			if int(pix[i])+int(pix[i+1])+int(pix[i+2]) < 735 {
				ink++
			}
		}
		return ink
	}
	low, high := countInk(5), countInk(17)
	if high <= low*13/10 {
		t.Fatalf("higher grid density should add substantially more coordinate-line pixels: low=%d high=%d", low, high)
	}
}

func TestKerrRayTraceProducesFiniteFrameDraggedResultA215(t *testing.T) {
	n := mdNormCore(mdVec3{0, math.Cos(65 * math.Pi / 180), math.Sin(65 * math.Pi / 180)})
	camera := mdVec3{0, mdBHCameraRadius, 0}
	left := mdNormCore(mdVec3{-0.16, -1, 0.03})
	right := mdNormCore(mdVec3{0.16, -1, 0.03})
	a, _ := mdBHTraceKerrRay(camera, left, n, 0.9, 0, false)
	b, _ := mdBHTraceKerrRay(camera, right, n, 0.9, 0, false)
	if a.steps == 0 || b.steps == 0 || math.IsNaN(a.minRadius) || math.IsNaN(b.minRadius) {
		t.Fatalf("Kerr rays must integrate to finite terminal states: left=%+v right=%+v", a, b)
	}
	if math.Abs(a.minRadius-b.minRadius) < 1e-4 && a.kind == b.kind {
		t.Fatalf("high-spin Kerr lens should break left/right Schwarzschild symmetry: left=%+v right=%+v", a, b)
	}
}

func TestKerrBlackHoleRenderContainsShadowDiskAndAsymmetryA215(t *testing.T) {
	n := mdNormCore(mdVec3{0, math.Cos(65 * math.Pi / 180), math.Sin(65 * math.Pi / 180)})
	w, h := 72, 54
	pix := mdBHRender(mdBHRenderParams{width: w, height: h, fovY: 50 * math.Pi / 180, diskNormal: n, mode: mdBlackHoleModeRender, quality: 0, spin: 0.9})
	if len(pix) != w*h*4 {
		t.Fatalf("unexpected Kerr pixel buffer: %d", len(pix))
	}
	bright, dark := 0, 0
	leftLuma, rightLuma := 0, 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			l := int(pix[i]) + int(pix[i+1]) + int(pix[i+2])
			if l > 390 {
				bright++
			}
			if l < 20 {
				dark++
			}
			if x < w/2 {
				leftLuma += l
			} else {
				rightLuma += l
			}
		}
	}
	if bright < 8 || dark < 20 {
		t.Fatalf("Kerr render missing disk/shadow: bright=%d dark=%d", bright, dark)
	}
	if absIntA215(leftLuma-rightLuma) < (leftLuma+rightLuma)/200 {
		t.Fatalf("high-spin Kerr render should have measurable left/right asymmetry: left=%d right=%d", leftLuma, rightLuma)
	}
}

func absIntA215(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
