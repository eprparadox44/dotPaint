package main

import "testing"

func TestMagneticPhysicalLineCacheKeyIgnoresCamera129(t *testing.T) {
	scene := fieldDefaultScene()
	scene.Dimension = fieldDimension3D
	scene.Density = 14
	scene.Objects = []fieldObject{fieldDefaultObject(fieldObjectBarMagnet)}
	base := fieldMagneticLineSceneKey(scene)
	camera := scene
	camera.RotX, camera.RotY, camera.RotZ = .4, -.9, 1.2
	if got := fieldMagneticLineSceneKey(camera); got != base {
		t.Fatalf("camera changed physical line key: %x -> %x", base, got)
	}
	changed := scene
	changed.Density++
	if got := fieldMagneticLineSceneKey(changed); got == base {
		t.Fatal("density did not invalidate line cache")
	}
	changed = scene
	changed.Dimension = fieldDimension2D
	if got := fieldMagneticLineSceneKey(changed); got == base {
		t.Fatal("dimension did not invalidate line cache")
	}
	changed = scene
	changed.Objects = append([]fieldObject(nil), scene.Objects...)
	changed.Objects[0].X += .1
	if got := fieldMagneticLineSceneKey(changed); got == base {
		t.Fatal("object movement did not invalidate line cache")
	}
}

func TestMagneticPhysicalLineCacheReturnsIndependentSlice129(t *testing.T) {
	key := uint64(0x129)
	original := []fieldLine{{Group: 7, Points: []fieldVec3{{X: 1}, {X: 2}}}}
	fieldStoreMagneticLines(key, original)
	got, ok := fieldCachedMagneticLines(key)
	if !ok || len(got) != 1 || got[0].Group != 7 {
		t.Fatalf("cache miss or wrong data: ok=%v lines=%v", ok, got)
	}
	got[0].Group = 99
	again, ok := fieldCachedMagneticLines(key)
	if !ok || again[0].Group != 7 {
		t.Fatal("caller mutated cached line slice")
	}
}
