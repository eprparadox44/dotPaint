//go:build windows

package main

import (
	"math"
)

func mdRotateCurve3(curve mdCurve3, ax, ay, az float64) mdCurve3 {
	out := mdCurve3{closed: curve.closed, points: make([]mdVec3, len(curve.points))}
	for i, p := range curve.points {
		out.points[i] = mdRotate(p, ax, ay, az)
	}
	return out
}

func mdDrawProjectedCurve3(hdc uintptr, rc RECT, curve mdCurve3, width int, color uint32) {
	if len(curve.points) < 2 {
		return
	}
	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.33
	pts := make([]POINT, 0, len(curve.points)+1)
	for _, p := range curve.points {
		pts = append(pts, mdProject(p, rc, scale))
	}
	if curve.closed && len(pts) > 2 {
		pts = append(pts, pts[0])
	}
	mdDrawPolyline(hdc, pts, width, color, false)
}

func mdTransformVerts3(verts []mdVec3, ax, ay, az float64) []mdVec3 {
	out := make([]mdVec3, len(verts))
	for i, p := range verts {
		out[i] = mdRotate(p, ax, ay, az)
	}
	return out
}

func multidimPaintDisk(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	mode := s.planarGrid
	if mode < mdPlanarGridNone || mode > mdPlanarGridFull {
		mode = mdPlanarGridMajor
	}
	segments, rings := 48, 4
	if mode == mdPlanarGridNone {
		rings = 1
	}
	if mode == mdPlanarGridFull {
		segments, rings = 72, 8
	}
	if multidimRenderSmoothEnabled(s) {
		segments += 24
	}
	radius := float64(multidimSliderPos(s.sizeSlider)) / 200.0
	mesh := mdBuildDiskSurface(segments, rings, radius)
	ax, ay, az, _ := multidimRotation(s)
	world := mdTransformVerts3(mesh.verts, ax, ay, az)
	// A193: every disk patch lies in one plane through the origin. The generic
	// outward-normal repair compares normal·centroid, which is mathematically
	// zero here and can change sign from floating-point noise after rotation.
	// That randomly flips neighbouring rings/sectors and creates the
	// checkerboard “spaceship” pattern. Use the exact rotated plane normal.
	planeNormal := mdRotate(mdVec3{z: 1}, ax, ay, az)
	mdRenderFacesOnlyWithNormal(hdc, rc, world, mesh.faces, multidimRenderOpacityIndex(s), multidimPreviewFillColor(s), func(mdFace) mdVec3 {
		return planeNormal
	})

	lineColor := multidimPreviewLineColor(s)
	gridColor := mdMixTowardWhite(lineColor, 0.58)
	lineW := mdMaxInt(1, 2*ss)
	if mode != mdPlanarGridNone {
		for i, path := range mesh.paths {
			// The final radial ring is the outer contour; all other polar guides are lighter.
			if i == rings-1 {
				continue
			}
			mdDrawProjectedCurve3(hdc, rc, mdRotateCurve3(path, ax, ay, az), mdMaxInt(1, ss), gridColor)
		}
	}
	mdDrawProjectedCurve3(hdc, rc, mdRotateCurve3(mesh.paths[rings-1], ax, ay, az), lineW, lineColor)
}

func multidimPaintRectanglePlane(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	halfWidth := float64(multidimSliderPos(s.widthSlider)) / 250.0
	halfHeight := float64(multidimSliderPos(s.heightSlider)) / 250.0
	mesh := mdBuildRectangleSurface(halfWidth, halfHeight)
	ax, ay, az, _ := multidimRotation(s)
	world := mdTransformVerts3(mesh.verts, ax, ay, az)
	planeNormal := mdRotate(mdVec3{z: 1}, ax, ay, az)
	mdRenderFacesOnlyWithNormal(hdc, rc, world, mesh.faces, multidimRenderOpacityIndex(s), multidimPreviewFillColor(s), func(mdFace) mdVec3 {
		return planeNormal
	})
	mdDrawProjectedCurve3(hdc, rc, mdRotateCurve3(mesh.paths[0], ax, ay, az), mdMaxInt(1, 2*ss), multidimPreviewLineColor(s))
}

func mdCoordinateAxisColor(label byte) uint32 {
	switch label {
	case 'x':
		return rgb(205, 45, 45)
	case 'y':
		return rgb(40, 150, 65)
	default:
		return rgb(45, 85, 210)
	}
}

func multidimPaintCoordinatePlane(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	extent := float64(multidimSliderPos(s.widthSlider)) / 180.0
	divisions := multidimSliderPos(s.heightSlider)
	if divisions < 2 {
		divisions = 2
	}
	majorEvery := multidimSliderPos(s.depthSlider)
	if majorEvery < 2 {
		majorEvery = 2
	}

	// A191: one coordinate-plane object is the full Cartesian triad: XY, XZ,
	// and YZ squares sharing the exact same origin and rotation.
	mesh := mdBuildTripleCoordinatePlaneSurface(extent)
	ax, ay, az, _ := multidimRotation(s)
	world := mdTransformVerts3(mesh.verts, ax, ay, az)
	fillColor := mdMixTowardWhite(multidimPreviewFillColor(s), 0.64)
	mdRenderFacesOnlyWithNormal(hdc, rc, world, mesh.faces, multidimRenderOpacityIndex(s), fillColor, func(face mdFace) mdVec3 {
		if len(face.indices) == 0 {
			return mdVec3{}
		}
		var normal mdVec3
		switch face.indices[0] / 4 {
		case mdCoordinatePlaneXZ:
			normal = mdVec3{y: -1}
		case mdCoordinatePlaneYZ:
			normal = mdVec3{x: 1}
		default:
			normal = mdVec3{z: 1}
		}
		return mdRotate(normal, ax, ay, az)
	})

	baseGrid := mdMixTowardWhite(multidimPreviewLineColor(s), 0.82)
	majorGrid := mdMixTowardWhite(multidimPreviewLineColor(s), 0.58)
	outline := mdMixTowardWhite(multidimPreviewLineColor(s), 0.18)
	for mode := mdCoordinatePlaneXY; mode <= mdCoordinatePlaneYZ; mode++ {
		u, v, _, _ := mdCoordinatePlaneBasis(mode)
		for i := -divisions; i <= divisions; i++ {
			if i == 0 {
				continue
			}
			pos := extent * float64(i) / float64(divisions)
			color := baseGrid
			width := mdMaxInt(1, ss)
			if i%majorEvery == 0 {
				color = majorGrid
				width = mdMaxInt(1, 2*ss)
			}
			curveU := mdCurve3{points: []mdVec3{mdPlanePoint(u, v, -extent, pos), mdPlanePoint(u, v, extent, pos)}}
			curveV := mdCurve3{points: []mdVec3{mdPlanePoint(u, v, pos, -extent), mdPlanePoint(u, v, pos, extent)}}
			mdDrawProjectedCurve3(hdc, rc, mdRotateCurve3(curveU, ax, ay, az), width, color)
			mdDrawProjectedCurve3(hdc, rc, mdRotateCurve3(curveV, ax, ay, az), width, color)
		}
		boundary := mdBuildCoordinatePlaneSurface(mode, extent).paths[0]
		mdDrawProjectedCurve3(hdc, rc, mdRotateCurve3(boundary, ax, ay, az), mdMaxInt(1, 2*ss), outline)
	}

	// The normal xyz overlay supplies arrows and honours the axis-size slider.
	// When the user hides that overlay, keep simple in-object axis lines so this
	// still remains a recognizable coordinate-plane object.
	if !multidimEditorAxesEnabled(s) {
		axisWidth := mdMaxInt(1, 2*ss)
		axes := []struct {
			dir   mdVec3
			label byte
		}{{mdVec3{x: 1}, 'x'}, {mdVec3{y: 1}, 'y'}, {mdVec3{z: 1}, 'z'}}
		scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.33
		for _, item := range axes {
			curve := mdCurve3{points: []mdVec3{mdMul(item.dir, -extent), mdMul(item.dir, extent)}}
			mdDrawProjectedCurve3(hdc, rc, mdRotateCurve3(curve, ax, ay, az), axisWidth, mdCoordinateAxisColor(item.label))
			end := mdRotate(mdMul(item.dir, extent), ax, ay, az)
			pt := mdProject(end, rc, scale)
			mdDrawAxisLetter(hdc, POINT{X: pt.X + int32(8*ss), Y: pt.Y - int32(8*ss)}, item.label, 9*ss, ss, mdCoordinateAxisColor(item.label))
		}
	}
}

func multidimPaintQuadricSurface(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	e := float64(multidimSliderPos(s.widthSlider)) / 100.0
	radius := float64(multidimSliderPos(s.heightSlider)) / 180.0
	halfHeight := float64(multidimSliderPos(s.depthSlider)) / 180.0
	axial, angular := 18, 40
	if multidimRenderSmoothEnabled(s) {
		axial, angular = 30, 64
	}
	var mesh mdSurfaceMesh3
	if e <= 1.005 {
		mesh = mdBuildParaboloidSurface(radius, halfHeight, axial, angular)
	} else {
		family := s.quadricFamily
		if family < mdQuadricOneSheet || family > mdQuadricTwoSheet {
			family = mdQuadricOneSheet
		}
		mesh = mdBuildHyperboloidSurface(e, family, radius, halfHeight, axial, angular)
	}
	ax, ay, az, _ := multidimRotation(s)
	world := mdTransformVerts3(mesh.verts, ax, ay, az)
	opacity := multidimRenderOpacityIndex(s)
	mdRenderFacesOnly(hdc, rc, world, mesh.faces, opacity, multidimPreviewFillColor(s))
	lineColor := multidimPreviewLineColor(s)
	for _, curve := range mesh.paths {
		points := curve.points
		if curve.closed && len(points) >= 3 {
			points = append(append([]mdVec3{}, points...), points[0])
		}
		mdDrawOcclusionCurve(hdc, rc, points, opacity, ss, lineColor, 1, ax, ay, az, world, mesh.faces)
	}
}

func mdConeCutPlaneCorners(plane mdConeCutPlane) []mdVec3 {
	return []mdVec3{
		mdAdd(plane.center, mdAdd(mdMul(plane.u, -plane.half), mdMul(plane.v, -plane.half))),
		mdAdd(plane.center, mdAdd(mdMul(plane.u, plane.half), mdMul(plane.v, -plane.half))),
		mdAdd(plane.center, mdAdd(mdMul(plane.u, plane.half), mdMul(plane.v, plane.half))),
		mdAdd(plane.center, mdAdd(mdMul(plane.u, -plane.half), mdMul(plane.v, plane.half))),
	}
}

func multidimPaintDoubleConeSection(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	radius := float64(multidimSliderPos(s.widthSlider)) / 220.0
	halfHeight := float64(multidimSliderPos(s.heightSlider)) / 180.0
	offset := multidimSliderPos(s.depthSlider)
	segments, radial := 40, 5
	curveSamples := 240
	if multidimRenderSmoothEnabled(s) {
		segments, radial, curveSamples = 64, 8, 420
	}
	mesh := mdBuildDoubleConeSurface(radius, halfHeight, segments, radial)
	mode := s.coneCutMode
	if mode < mdConeCutHorizontal || mode > mdConeCutThroughApex {
		mode = mdConeCutHorizontal
	}
	curves, plane := mdConeCutGeometry(mode, radius, halfHeight, offset, curveSamples)
	ax, ay, az, _ := multidimRotation(s)

	// The pale cutting plane is painted before the cone, so front cone patches
	// naturally cover it while its exposed corners remain visible.
	planeObject := mdConeCutPlaneCorners(plane)
	planeWorld := mdTransformVerts3(planeObject, ax, ay, az)
	planePts := make([]POINT, len(planeWorld))
	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.33
	for i, p := range planeWorld {
		planePts[i] = mdProject(p, rc, scale)
	}
	planeFill := rgb(218, 239, 250)
	planeLine := rgb(55, 125, 185)
	mdFillPolygon(hdc, planePts, planeFill, planeFill)

	world := mdTransformVerts3(mesh.verts, ax, ay, az)
	opacity := multidimRenderOpacityIndex(s)
	mdRenderFacesOnlyWithNormal(hdc, rc, world, mesh.faces, opacity, multidimPreviewFillColor(s), func(face mdFace) mdVec3 {
		// A cone's side tangent plane passes through the apex, so the generic
		// centroid-dot-normal winding repair is mathematically zero and can flip
		// randomly from patch to patch. Use the analytic cone normal in object
		// space, then rotate it with the object for stable continuous shading.
		return mdRotate(mdDoubleConeFaceNormal(mesh.verts, face, radius, halfHeight), ax, ay, az)
	})
	gridColor := mdMixTowardWhite(multidimPreviewLineColor(s), 0.38)
	for _, curve := range mesh.paths {
		points := curve.points
		if curve.closed && len(points) >= 3 {
			points = append(append([]mdVec3{}, points...), points[0])
		}
		mdDrawOcclusionCurve(hdc, rc, points, opacity, ss, gridColor, 1, ax, ay, az, world, mesh.faces)
	}
	mdDrawPolyline(hdc, append(planePts, planePts[0]), mdMaxInt(1, ss), planeLine, false)

	sectionColor := rgb(225, 70, 35)
	for _, curve := range curves {
		worldCurve := mdRotateCurve3(curve, ax, ay, az)
		mdDrawProjectedCurve3(hdc, rc, worldCurve, mdMaxInt(2, 3*ss), sectionColor)
	}
}

func multidimPaintSaddleSurface(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	if s == nil {
		return
	}
	mode := s.saddleGrid
	if mode < mdPlanarGridNone || mode > mdPlanarGridFull {
		mode = mdPlanarGridMajor
	}
	xSegments, ySegments := 20, 18
	if multidimRenderSmoothEnabled(s) {
		xSegments, ySegments = 36, 32
	}
	halfWidth := float64(multidimSliderPos(s.widthSlider)) / 250.0
	halfHeight := float64(multidimSliderPos(s.heightSlider)) / 250.0
	halfDepth := float64(multidimSliderPos(s.depthSlider)) / 200.0
	mesh := mdBuildSaddleSurface(halfWidth, halfHeight, halfDepth, xSegments, ySegments)
	ax, ay, az, _ := multidimRotation(s)
	world := mdTransformVerts3(mesh.verts, ax, ay, az)
	opacity := multidimRenderOpacityIndex(s)
	mdRenderFacesOnlyWithNormal(hdc, rc, world, mesh.faces, opacity, multidimPreviewFillColor(s), func(face mdFace) mdVec3 {
		return mdRotate(mdSaddleFaceNormal(mesh.verts, face, halfWidth, halfHeight, halfDepth), ax, ay, az)
	})

	lineColor := multidimPreviewLineColor(s)
	gridColor := mdMixTowardWhite(lineColor, 0.55)
	rows := ySegments + 1
	majorX := mdMaxInt(1, xSegments/6)
	majorY := mdMaxInt(1, ySegments/6)
	for i, curve := range mesh.paths {
		isBoundary := false
		draw := false
		if i < rows {
			row := i
			isBoundary = row == 0 || row == ySegments
			switch mode {
			case mdPlanarGridFull:
				draw = true
			case mdPlanarGridMajor:
				draw = isBoundary || row%majorY == 0 || row == ySegments/2
			default:
				draw = isBoundary
			}
		} else {
			col := i - rows
			isBoundary = col == 0 || col == xSegments
			switch mode {
			case mdPlanarGridFull:
				draw = true
			case mdPlanarGridMajor:
				draw = isBoundary || col%majorX == 0 || col == xSegments/2
			default:
				draw = isBoundary
			}
		}
		if !draw {
			continue
		}
		color := gridColor
		if isBoundary {
			color = lineColor
		}
		mdDrawOcclusionCurve(hdc, rc, curve.points, opacity, ss, color, 1, ax, ay, az, world, mesh.faces)
	}
}
