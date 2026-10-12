package main

import (
	"math"
	"testing"
)

func TestFieldVectorGridResolutionPolicy(t *testing.T) {
	for _, tc := range []struct {
		density int
		want    int
	}{{4, 40}, {7, 40}, {8, 52}, {16, 52}, {17, 64}, {24, 64}} {
		scene := fieldDefaultScene()
		scene.Density = tc.density
		if got := fieldVectorGridSteps(scene); got != tc.want {
			t.Fatalf("density=%d grid=%d want=%d", tc.density, got, tc.want)
		}
	}
}

func TestFieldVectorSceneKeyIgnoresCameraButTracksPhysics(t *testing.T) {
	scene := fieldDefaultScene()
	scene.Objects = []fieldObject{fieldDefaultObject(fieldObjectBarMagnet)}
	steps := fieldVectorGridSteps(scene)
	base := fieldVectorSceneKey(scene, nil, true, steps)

	camera := scene
	camera.RotX = .7
	camera.RotY = -.4
	camera.RotZ = 1.1
	if got := fieldVectorSceneKey(camera, nil, true, steps); got != base {
		t.Fatalf("camera-only rotation changed physical grid cache key: base=%x got=%x", base, got)
	}

	changed := scene
	changed.Objects[0].Charge *= -1
	if got := fieldVectorSceneKey(changed, nil, true, steps); got == base {
		t.Fatal("magnet polarity change did not invalidate vector grid cache key")
	}

	changed = scene
	changed.Objects[0].RotZ += .25
	if got := fieldVectorSceneKey(changed, nil, true, steps); got == base {
		t.Fatal("object rotation did not invalidate vector grid cache key")
	}
}

func TestFieldVectorGridDirectDistanceCoversOneAndThreeQuarterCells(t *testing.T) {
	grid := fieldVectorGrid{
		NX: 5, NY: 5, NZ: 5,
		Min:    fieldVec3{X: -2, Y: -2, Z: -2},
		Max:    fieldVec3{X: 2, Y: 2, Z: 2},
		Values: make([]fieldVec3, 125),
	}
	if got, want := fieldVectorGridDirectDistance(grid), 1.75; got != want {
		t.Fatalf("direct radius=%g want=%g", got, want)
	}
}

func TestMagneticVectorGridDirectDistanceUsesHalfCellExactShell129(t *testing.T) {
	grid := fieldVectorGrid{
		NX: 5, NY: 5, NZ: 5,
		Min:    fieldVec3{X: -2, Y: -2, Z: -2},
		Max:    fieldVec3{X: 2, Y: 2, Z: 2},
		Values: make([]fieldVec3, 125),
	}
	if got, want := fieldMagneticVectorGridDirectDistance(grid), 0.55; math.Abs(got-want) > 1e-12 {
		t.Fatalf("magnetic direct radius=%g want=%g", got, want)
	}
}
