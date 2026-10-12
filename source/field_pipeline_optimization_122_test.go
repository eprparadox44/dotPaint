package main

import "testing"

func TestFieldPhysicalLineKeysReuseCameraButTrackPhysics122(t *testing.T) {
	scene := fieldDefaultScene()
	scene.Dimension = fieldDimension3D
	scene.Density = 14
	scene.Display = fieldDisplayGravityLines
	scene.Objects = []fieldObject{fieldDefaultObject(fieldObjectSolidMassSphere)}
	base := fieldGravityLineSceneKey(scene)

	camera := scene
	camera.RotX, camera.RotY, camera.RotZ = .4, -.7, 1.1
	camera.ViewDistance = 2.3
	camera.Antialias = !scene.Antialias
	if got := fieldGravityLineSceneKey(camera); got != base {
		t.Fatalf("camera/AA invalidated physical gravity lines: %x -> %x", base, got)
	}

	changed := scene
	changed.Objects = append([]fieldObject(nil), scene.Objects...)
	changed.Objects[0].Spin = .6
	if got := fieldGravityLineSceneKey(changed); got == base {
		t.Fatal("spin did not invalidate gravity line cache")
	}
	changed = scene
	changed.Density++
	if got := fieldGravityLineSceneKey(changed); got == base {
		t.Fatal("density did not invalidate gravity line cache")
	}
}

func TestFieldCancellationStopsObsoleteStreamline122(t *testing.T) {
	token := fieldNewRenderCancellation()
	policy := fieldDefaultStreamlinePolicy()
	policy.MaxSteps = 10000
	policy.Cancelled = token.Cancelled
	evaluations := 0
	eval := func(p fieldVec3) fieldVec3 {
		evaluations++
		if evaluations == 2 {
			token.Cancel()
		}
		return fieldVec3{X: 1}
	}
	result := fieldIntegrateStreamline(eval, fieldVec3{}, 1, policy)
	if !token.Cancelled() {
		t.Fatal("test cancellation token was not set")
	}
	if result.Accepted > 1 || evaluations > 8 {
		t.Fatalf("obsolete streamline kept running: accepted=%d evaluations=%d", result.Accepted, evaluations)
	}
}

func TestFieldSupersampleRasterOmitsMaskAndResolvesMask122(t *testing.T) {
	hi := fieldNewRasterAtScale(12, 12, 6)
	if len(hi.Mask) != 0 {
		t.Fatalf("supersample raster allocated redundant mask: %d", len(hi.Mask))
	}
	hi.blend(3, 3, 255, 0, 0, 1)
	resolved := fieldDownsampleRasterScale(hi, 2, 2, 6)
	if len(resolved.Mask) != 4 {
		t.Fatalf("resolved mask length=%d want=4", len(resolved.Mask))
	}
	found := false
	for _, on := range resolved.Mask {
		found = found || on
	}
	if !found {
		t.Fatal("resolved coverage mask lost supersample alpha")
	}
	fieldReleaseSupersampleRaster(&hi)
	if hi.Pix != nil {
		t.Fatal("released supersample pixels remain owned by raster")
	}
}

func TestFieldPotentialGeometryKeyIgnoresViewButTracksQuality122(t *testing.T) {
	scene := fieldDefaultScene()
	scene.Dimension = fieldDimension3D
	scene.Display = fieldDisplayEquipotential
	scene.Objects = []fieldObject{fieldDefaultObject(fieldObjectSphere)}
	samples := fieldElectricalSamples(scene, 0)
	base := fieldPotentialGeometrySceneKey(scene, samples, 0, nil)

	camera := scene
	camera.RotX, camera.RotY, camera.RotZ = .3, -.8, .5
	camera.ViewDistance = 2
	camera.Antialias = false
	if got := fieldPotentialGeometrySceneKey(camera, samples, 0, nil); got != base {
		t.Fatalf("view invalidated world geometry: %x -> %x", base, got)
	}
	if got := fieldPotentialGeometrySceneKey(scene, samples, 1, nil); got == base {
		t.Fatal("quality did not invalidate potential geometry")
	}
}

func TestFieldPotentialGeometryCacheRoundTrip122(t *testing.T) {
	key := uint64(0x122)
	geometry := fieldPotentialGeometry{
		Grid:   fieldPotentialGrid{Steps: 2, MinCoord: -1, MaxCoord: 1, Values: make([]float64, 8)},
		Levels: []float64{1}, VacuumTriangles: 7,
	}
	fieldStorePotentialGeometry(key, geometry)
	got, ok := fieldCachedPotentialGeometry(key)
	if !ok || got.VacuumTriangles != 7 || len(got.Levels) != 1 {
		t.Fatalf("geometry cache round trip failed: ok=%v geometry=%+v", ok, got)
	}
}
