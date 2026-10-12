package main

import (
	"math"
	"testing"
)

func finiteVec3(v mdVec3) bool {
	return !(math.IsNaN(v.x) || math.IsNaN(v.y) || math.IsNaN(v.z) || math.IsInf(v.x, 0) || math.IsInf(v.y, 0) || math.IsInf(v.z, 0))
}

func TestPlanarPrimitiveTopology(t *testing.T) {
	disk := mdBuildDiskSurface(32, 4, 1.25)
	if len(disk.verts) != 1+32*4 || len(disk.faces) == 0 || len(disk.paths) == 0 {
		t.Fatalf("unexpected disk topology: verts=%d faces=%d paths=%d", len(disk.verts), len(disk.faces), len(disk.paths))
	}
	rect := mdBuildRectangleSurface(1.2, 0.8)
	if len(rect.verts) != 4 || len(rect.faces) != 1 || len(rect.paths) != 1 {
		t.Fatalf("unexpected rectangle topology")
	}
	for mode := mdCoordinatePlaneXY; mode <= mdCoordinatePlaneYZ; mode++ {
		plane := mdBuildCoordinatePlaneSurface(mode, 1.5)
		if len(plane.verts) != 4 || len(plane.faces) != 1 {
			t.Fatalf("coordinate plane mode=%d malformed", mode)
		}
	}
	triple := mdBuildTripleCoordinatePlaneSurface(1.5)
	if len(triple.verts) != 12 || len(triple.faces) != 3 || len(triple.paths) != 3 {
		t.Fatalf("triple coordinate plane malformed: verts=%d faces=%d paths=%d", len(triple.verts), len(triple.faces), len(triple.paths))
	}
	seenXY, seenXZ, seenYZ := false, false, false
	for _, face := range triple.faces {
		if len(face.indices) != 4 {
			t.Fatalf("triple coordinate face malformed: %+v", face.indices)
		}
		allZ, allY, allX := true, true, true
		for _, idx := range face.indices {
			v := triple.verts[idx]
			allZ = allZ && math.Abs(v.z) < 1e-12
			allY = allY && math.Abs(v.y) < 1e-12
			allX = allX && math.Abs(v.x) < 1e-12
		}
		seenXY = seenXY || allZ
		seenXZ = seenXZ || allY
		seenYZ = seenYZ || allX
	}
	if !seenXY || !seenXZ || !seenYZ {
		t.Fatalf("missing orthogonal plane: XY=%v XZ=%v YZ=%v", seenXY, seenXZ, seenYZ)
	}
}

func TestQuadricMeshesFiniteAndBounded(t *testing.T) {
	cases := []mdSurfaceMesh3{
		mdBuildParaboloidSurface(1.1, 1.2, 12, 32),
		mdBuildHyperboloidSurface(1.6, mdQuadricOneSheet, 1.1, 1.2, 18, 32),
		mdBuildHyperboloidSurface(1.6, mdQuadricTwoSheet, 1.1, 1.2, 18, 32),
	}
	for ci, mesh := range cases {
		if len(mesh.verts) == 0 || len(mesh.faces) == 0 || len(mesh.paths) == 0 {
			t.Fatalf("quadric %d empty", ci)
		}
		for _, v := range mesh.verts {
			if !finiteVec3(v) {
				t.Fatalf("quadric %d non-finite vertex %+v", ci, v)
			}
			if math.Abs(v.x) > 1.100001 || math.Abs(v.y) > 1.100001 || math.Abs(v.z) > 1.200001 {
				t.Fatalf("quadric %d outside requested bounds %+v", ci, v)
			}
		}
	}
}

func TestDoubleConeSurfaceAndConicSections(t *testing.T) {
	const radius, halfHeight = 1.15, 1.4
	mesh := mdBuildDoubleConeSurface(radius, halfHeight, 48, 5)
	if len(mesh.verts) == 0 || len(mesh.faces) == 0 || len(mesh.paths) == 0 {
		t.Fatalf("double cone empty")
	}
	for mode := mdConeCutHorizontal; mode <= mdConeCutThroughApex; mode++ {
		curves, plane := mdConeCutGeometry(mode, radius, halfHeight, 35, 360)
		if len(curves) == 0 || !finiteVec3(plane.center) || !finiteVec3(plane.u) || !finiteVec3(plane.v) {
			t.Fatalf("cut mode=%d malformed", mode)
		}
		count := 0
		for _, curve := range curves {
			for _, p := range curve.points {
				count++
				if !finiteVec3(p) || math.Abs(p.z) > halfHeight+1e-7 {
					t.Fatalf("cut mode=%d invalid point %+v", mode, p)
				}
				if residual := math.Abs(mdConeSurfaceResidual(p, radius, halfHeight)); residual > 2e-6 {
					t.Fatalf("cut mode=%d point off cone residual=%g point=%+v", mode, residual, p)
				}
			}
		}
		if count < 4 {
			t.Fatalf("cut mode=%d too few points=%d", mode, count)
		}
	}
}

func TestConicPresetCharacter(t *testing.T) {
	circle, _ := mdConeCutGeometry(mdConeCutHorizontal, 1, 1, 35, 360)
	if len(circle) != 1 || !circle[0].closed {
		t.Fatalf("horizontal section should be one closed circle, got paths=%d closed=%v", len(circle), len(circle) == 1 && circle[0].closed)
	}
	ellipse, _ := mdConeCutGeometry(mdConeCutObliqueEllipse, 1, 1, 35, 360)
	if len(ellipse) != 1 || !ellipse[0].closed {
		t.Fatalf("oblique section should be one closed ellipse, got paths=%d", len(ellipse))
	}
	parabola, _ := mdConeCutGeometry(mdConeCutGeneratorParallel, 1, 1, 35, 360)
	if len(parabola) == 0 {
		t.Fatalf("generator-parallel section missing")
	}
	hyperbola, _ := mdConeCutGeometry(mdConeCutVerticalHyperbola, 1, 1, 35, 360)
	if len(hyperbola) < 4 {
		t.Fatalf("vertical section should expose four truncated hyperbola branches, got %d", len(hyperbola))
	}
}

func TestConeCutNegativeOffsetMirrorsPositiveSide(t *testing.T) {
	const radius, halfHeight = 1.2, 1.5
	for _, mode := range []int{mdConeCutHorizontal, mdConeCutObliqueEllipse, mdConeCutGeneratorParallel, mdConeCutVerticalHyperbola} {
		positive, positivePlane := mdConeCutGeometry(mode, radius, halfHeight, 45, 360)
		negative, negativePlane := mdConeCutGeometry(mode, radius, halfHeight, -45, 360)
		if len(positive) == 0 || len(negative) == 0 {
			t.Fatalf("mode=%d missing positive/negative section", mode)
		}
		if mode == mdConeCutVerticalHyperbola {
			if positivePlane.center.x <= 0 || negativePlane.center.x >= 0 {
				t.Fatalf("mode=%d vertical plane did not mirror x: +%v -%v", mode, positivePlane.center, negativePlane.center)
			}
		} else if positivePlane.center.z <= 0 || negativePlane.center.z >= 0 {
			t.Fatalf("mode=%d plane did not mirror z: +%v -%v", mode, positivePlane.center, negativePlane.center)
		}
		for _, curves := range [][]mdCurve3{positive, negative} {
			for _, curve := range curves {
				for _, point := range curve.points {
					if residual := math.Abs(mdConeSurfaceResidual(point, radius, halfHeight)); residual > 2e-6 {
						t.Fatalf("mode=%d mirrored point off cone residual=%g point=%+v", mode, residual, point)
					}
				}
			}
		}
	}
}

func TestDoubleConeAnalyticNormalsStable(t *testing.T) {
	const radius, halfHeight = 1.15, 1.4
	mesh := mdBuildDoubleConeSurface(radius, halfHeight, 48, 5)
	for fi, face := range mesh.faces {
		n := mdDoubleConeFaceNormal(mesh.verts, face, radius, halfHeight)
		length := math.Sqrt(n.x*n.x + n.y*n.y + n.z*n.z)
		if math.IsNaN(length) || math.IsInf(length, 0) || math.Abs(length-1) > 1e-8 {
			t.Fatalf("face=%d invalid analytic normal %+v length=%g", fi, n, length)
		}
		minZ, maxZ := math.Inf(1), math.Inf(-1)
		centroid := mdVec3{}
		for _, idx := range face.indices {
			point := mesh.verts[idx]
			centroid.x += point.x
			centroid.y += point.y
			centroid.z += point.z
			minZ = math.Min(minZ, point.z)
			maxZ = math.Max(maxZ, point.z)
		}
		inv := 1 / float64(len(face.indices))
		centroid.x *= inv
		centroid.y *= inv
		centroid.z *= inv
		isCap := maxZ-minZ < 1e-8 && math.Abs(math.Abs(centroid.z)-halfHeight) < 1e-7
		if !isCap {
			radialOut := n.x*centroid.x + n.y*centroid.y
			if radialOut <= 1e-10 {
				t.Fatalf("face=%d side normal not radially outward normal=%+v centroid=%+v", fi, n, centroid)
			}
		}
	}
}

func TestDiskFacesShareOneObjectSpaceNormal(t *testing.T) {
	mesh := mdBuildDiskSurface(72, 8, 1)
	if len(mesh.faces) == 0 {
		t.Fatal("disk has no faces")
	}
	for i, face := range mesh.faces {
		n := mdFaceOutwardNormal(mesh.verts, face)
		if math.Abs(n.x) > 1e-12 || math.Abs(n.y) > 1e-12 || n.z < 1-1e-12 {
			t.Fatalf("face %d normal=(%g,%g,%g), want +Z", i, n.x, n.y, n.z)
		}
	}
}

func TestSaddleSurfaceTopologyBoundsAndNormals(t *testing.T) {
	const halfW, halfH, halfD = 1.4, 1.1, 0.9
	const xs, ys = 20, 16
	mesh := mdBuildSaddleSurface(halfW, halfH, halfD, xs, ys)
	wantVerts := (xs + 1) * (ys + 1)
	wantFaces := xs * ys
	wantPaths := (ys + 1) + (xs + 1)
	if len(mesh.verts) != wantVerts || len(mesh.faces) != wantFaces || len(mesh.paths) != wantPaths {
		t.Fatalf("saddle topology verts=%d/%d faces=%d/%d paths=%d/%d", len(mesh.verts), wantVerts, len(mesh.faces), wantFaces, len(mesh.paths), wantPaths)
	}
	seenPositive, seenNegative := false, false
	for _, v := range mesh.verts {
		if !finiteVec3(v) {
			t.Fatalf("non-finite saddle vertex %+v", v)
		}
		if math.Abs(v.x) > halfW+1e-9 || math.Abs(v.y) > halfH+1e-9 || math.Abs(v.z) > halfD+1e-9 {
			t.Fatalf("saddle vertex outside bounds %+v", v)
		}
		seenPositive = seenPositive || v.z > halfD*0.9
		seenNegative = seenNegative || v.z < -halfD*0.9
	}
	if !seenPositive || !seenNegative {
		t.Fatalf("saddle did not span both curvatures positive=%v negative=%v", seenPositive, seenNegative)
	}
	for i, face := range mesh.faces {
		n := mdSaddleFaceNormal(mesh.verts, face, halfW, halfH, halfD)
		length := math.Sqrt(n.x*n.x + n.y*n.y + n.z*n.z)
		if math.Abs(length-1) > 1e-9 || n.z <= 0 {
			t.Fatalf("face=%d invalid saddle normal %+v length=%g", i, n, length)
		}
	}
}
