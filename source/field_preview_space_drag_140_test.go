package main

import (
	"math"
	"testing"
)

func TestFieldPreviewSpaceInverseUsesCurrentObserverDistance2D(t *testing.T) {
	for _, distance := range []float64{.1, .2, .5, 1, 2, 3} {
		scene := fieldDefaultScene()
		scene.Dimension = fieldDimension2D
		scene.ViewDistance = distance
		p := fieldProjectorForScene(scene, 640, 480)
		want := fieldVec3{X: .08, Z: -.052}
		sx, sy, _, ok := p.projectFloat(want)
		if !ok {
			t.Fatalf("distance %.1f point did not project", distance)
		}
		gotX, gotZ := fieldScreenToWorldAtY(scene, sx, sy, p.W, p.H, 0)
		if err := math.Hypot(gotX-want.X, gotZ-want.Z); err > 1e-7 {
			t.Fatalf("distance %.1f inverse mismatch got=(%.8f,%.8f) want=(%.8f,%.8f) err=%g", distance, gotX, gotZ, want.X, want.Z, err)
		}
	}
}

func TestFieldPreviewSpaceInversePreservesDepthAndRotated3DView(t *testing.T) {
	for _, distance := range []float64{.2, .7, 1, 2.4} {
		scene := fieldDefaultScene()
		scene.Dimension = fieldDimension3D
		scene.ViewDistance = distance
		scene.RotX, scene.RotY, scene.RotZ = .31, -.47, .22
		p := fieldProjectorForScene(scene, 640, 480)
		want := fieldVec3{X: .16, Y: .43, Z: -.11}
		sx, sy, _, ok := p.projectFloat(want)
		if !ok {
			t.Fatalf("distance %.1f point did not project", distance)
		}
		gotX, gotZ := fieldScreenToWorldAtY(scene, sx, sy, p.W, p.H, want.Y)
		if err := math.Hypot(gotX-want.X, gotZ-want.Z); err > .0025 {
			t.Fatalf("distance %.1f rotated inverse mismatch got=(%.6f,%.6f) want=(%.6f,%.6f) err=%g", distance, gotX, gotZ, want.X, want.Z, err)
		}
	}
}

func TestFieldPreviewSpaceDragKeepsGrabbedScreenOffset(t *testing.T) {
	scene := fieldDefaultScene()
	scene.Dimension = fieldDimension3D
	scene.ViewDistance = .25
	scene.RotX, scene.RotY, scene.RotZ = .18, -.36, .11
	p := fieldProjectorForScene(scene, 640, 480)
	start := fieldVec3{X: .13, Y: .28, Z: -.09}
	cx, cy, _, ok := p.projectFloat(start)
	if !ok {
		t.Fatal("start point did not project")
	}
	grabDX, grabDY := 23.5, -14.25
	pointerX, pointerY := cx+grabDX, cy+grabDY
	pointerX += 47
	pointerY += 31
	targetCX, targetCY := pointerX-grabDX, pointerY-grabDY
	gotX, gotZ := fieldScreenToWorldAtY(scene, targetCX, targetCY, p.W, p.H, start.Y)
	projectedX, projectedY, _, ok := p.projectFloat(fieldVec3{X: gotX, Y: start.Y, Z: gotZ})
	if !ok {
		t.Fatal("dragged point did not project")
	}
	if err := math.Hypot((projectedX+grabDX)-pointerX, (projectedY+grabDY)-pointerY); err > .12 {
		t.Fatalf("grabbed point drifted by %.4f pixels: pointer=(%.3f,%.3f) projected-grab=(%.3f,%.3f)", err, pointerX, pointerY, projectedX+grabDX, projectedY+grabDY)
	}
}

func TestFieldPreviewSpacePixelDeltaScalesWithDistance(t *testing.T) {
	worldDelta := func(distance float64) float64 {
		scene := fieldDefaultScene()
		scene.Dimension = fieldDimension2D
		scene.ViewDistance = distance
		x0, z0 := fieldScreenToWorldAtY(scene, 320, 240, 640, 480, 0)
		x1, z1 := fieldScreenToWorldAtY(scene, 340, 240, 640, 480, 0)
		return math.Hypot(x1-x0, z1-z0)
	}
	d01 := worldDelta(.1)
	d1 := worldDelta(1)
	d2 := worldDelta(2)
	if math.Abs(d01/d1-.1) > 1e-9 {
		t.Fatalf("distance .1 delta ratio=%g want .1", d01/d1)
	}
	if math.Abs(d2/d1-2) > 1e-9 {
		t.Fatalf("distance 2 delta ratio=%g want 2", d2/d1)
	}
}
