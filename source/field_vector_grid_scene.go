package main

import (
	"encoding/binary"
	"hash"
	"hash/fnv"
	"math"
	"sync"
	"time"
)

type fieldVectorGridCacheEntry struct {
	Key  uint64
	Grid fieldVectorGrid
}

var fieldVectorGridCache struct {
	sync.Mutex
	Electric fieldVectorGridCacheEntry
	Magnetic fieldVectorGridCacheEntry
	Gravity  fieldVectorGridCacheEntry
}

func fieldVectorGridSteps(scene fieldScene) int {
	switch {
	case scene.Density <= 7:
		return 40
	case scene.Density <= 16:
		return 52
	default:
		return 64
	}
}

func fieldVectorGridSpan(scene fieldScene) float64 {
	// Keep the regular grid concentrated around the useful diagram volume.
	// Far-away portions of unusually wide lines fall back to the exact direct
	// evaluator instead of making every cell too coarse near the sources.
	return math.Min(fieldSceneNumericalInfinityRadius(scene), math.Max(6.5, fieldSceneSpan(scene)*2.2))
}

func fieldVectorGridDirectDistanceCells(grid fieldVectorGrid, cells float64) float64 {
	if !grid.valid() {
		return math.Inf(1)
	}
	dx := (grid.Max.X - grid.Min.X) / float64(grid.NX-1)
	dy := (grid.Max.Y - grid.Min.Y) / float64(grid.NY-1)
	dz := (grid.Max.Z - grid.Min.Z) / float64(grid.NZ-1)
	return math.Max(dx, math.Max(dy, dz)) * math.Max(0, cells)
}

func fieldVectorGridDirectDistance(grid fieldVectorGrid) float64 {
	return fieldVectorGridDirectDistanceCells(grid, 1.75)
}

func fieldMagneticVectorGridDirectDistance(grid fieldVectorGrid) float64 {
	// Magnetic current segments already use finite softening and the 52^3 grid
	// resolves their exterior field well.  Keeping only 0.55 cells on the exact
	// path avoids falling back to hundreds of Biot-Savart segment sums while a
	// line runs near a magnet surface.
	return fieldVectorGridDirectDistanceCells(grid, 0.55)
}

func fieldNearestObjectSurface(scene fieldScene, p fieldVec3, electric bool) float64 {
	nearest := math.Inf(1)
	for _, object := range scene.Objects {
		if electric {
			if !object.electric() {
				continue
			}
		} else if !object.magnetic() {
			continue
		}
		nearest = math.Min(nearest, fieldDistanceToObjectSurface(object, p))
	}
	return nearest
}

func fieldHashFloat64(h hash.Hash64, value float64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(value))
	_, _ = h.Write(b[:])
}

func fieldHashUint64(h hash.Hash64, value uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], value)
	_, _ = h.Write(b[:])
}

func fieldVectorSceneKey(scene fieldScene, samples []fieldChargeSample, magnetic bool, steps int) uint64 {
	hash := fnv.New64a()
	// Camera rotation and display mode are deliberately excluded. The physical
	// vector field is world-space and can be projected repeatedly from cache.
	fieldHashUint64(hash, uint64(steps))
	if magnetic {
		fieldHashUint64(hash, 0x4d41474e45544943)
	} else {
		fieldHashUint64(hash, 0x454c454354524943)
	}
	fieldHashUint64(hash, uint64(len(scene.Objects)))
	for _, object := range scene.Objects {
		fieldHashUint64(hash, uint64(object.Kind))
		fieldHashUint64(hash, uint64(object.Material))
		for _, value := range []float64{object.X, object.Y, object.Z, object.Charge, object.Radius, object.Length, object.Size, object.RotX, object.RotY, object.RotZ, object.Spin} {
			fieldHashFloat64(hash, value)
		}
		if object.Volumetric {
			fieldHashUint64(hash, 0x564f4c554d45)
		}
	}
	if !magnetic {
		fieldHashUint64(hash, uint64(len(samples)))
		for _, sample := range samples {
			for _, value := range []float64{sample.Pos.X, sample.Pos.Y, sample.Pos.Z, sample.Q, sample.Soft, sample.GateOffset, sample.GateNormal.X, sample.GateNormal.Y, sample.GateNormal.Z} {
				fieldHashFloat64(hash, value)
			}
			fieldHashUint64(hash, uint64(sample.Object+1))
			if sample.Gate {
				fieldHashUint64(hash, 1)
			}
			if sample.Conductor {
				fieldHashUint64(hash, 2)
			}
		}
	}
	return hash.Sum64()
}

func fieldCachedVectorGrid(magnetic bool, key uint64) (fieldVectorGrid, bool) {
	fieldVectorGridCache.Lock()
	defer fieldVectorGridCache.Unlock()
	entry := fieldVectorGridCache.Electric
	if magnetic {
		entry = fieldVectorGridCache.Magnetic
	}
	return entry.Grid, entry.Key == key && entry.Grid.valid()
}

func fieldStoreVectorGrid(magnetic bool, key uint64, grid fieldVectorGrid) {
	if !grid.valid() {
		return
	}
	fieldVectorGridCache.Lock()
	if magnetic {
		fieldVectorGridCache.Magnetic = fieldVectorGridCacheEntry{Key: key, Grid: grid}
	} else {
		fieldVectorGridCache.Electric = fieldVectorGridCacheEntry{Key: key, Grid: grid}
	}
	fieldVectorGridCache.Unlock()
}

func fieldCachedGravityVectorGrid(key uint64) (fieldVectorGrid, bool) {
	fieldVectorGridCache.Lock()
	defer fieldVectorGridCache.Unlock()
	entry := fieldVectorGridCache.Gravity
	return entry.Grid, entry.Key == key && entry.Grid.valid()
}

func fieldStoreGravityVectorGrid(key uint64, grid fieldVectorGrid) {
	if !grid.valid() {
		return
	}
	fieldVectorGridCache.Lock()
	fieldVectorGridCache.Gravity = fieldVectorGridCacheEntry{Key: key, Grid: grid}
	fieldVectorGridCache.Unlock()
}

func fieldElectricVectorEvaluator(scene fieldScene, samples []fieldChargeSample) (fieldVectorEvaluator, string) {
	direct := func(p fieldVec3) fieldVec3 { return fieldElectricAt(scene, samples, p) }
	if scene.precise3DLines && scene.Dimension == fieldDimension3D && scene.Display == fieldDisplayElectricLines {
		return direct, "CPU-DIRECT-PRECISE-3D"
	}
	steps := fieldVectorGridSteps(scene)
	key := fieldVectorSceneKey(scene, samples, false, steps)
	if grid, ok := fieldCachedVectorGrid(false, key); ok {
		fieldRecordVectorGridCache("electric", grid, true)
		near := func(p fieldVec3) float64 { return fieldNearestObjectSurface(scene, p, true) }
		return grid.evaluator(direct, near, fieldVectorGridDirectDistance(grid)), "D3D11-GRID-CACHE"
	}
	span := fieldVectorGridSpan(scene)
	started := time.Now()
	values, ok := fieldTryGPUElectricVectorGrid(samples, steps, -span, span)
	if !ok || len(values) != steps*steps*steps {
		fieldRecordVectorGridFallback("electric", steps, len(samples), 0, 0, time.Since(started))
		return direct, "CPU-DIRECT"
	}
	grid := fieldVectorGrid{NX: steps, NY: steps, NZ: steps, Min: fieldVec3{X: -span, Y: -span, Z: -span}, Max: fieldVec3{X: span, Y: span, Z: span}, Values: values, Backend: "D3D11"}
	fieldStoreVectorGrid(false, key, grid)
	fieldRecordVectorGridReady("electric", grid, len(samples), 0, 0, false, time.Since(started))
	near := func(p fieldVec3) float64 { return fieldNearestObjectSurface(scene, p, true) }
	return grid.evaluator(direct, near, fieldVectorGridDirectDistance(grid)), "D3D11-GRID"
}

func fieldMagneticVectorEvaluator(scene fieldScene, model fieldMagneticModel) (fieldVectorEvaluator, string) {
	direct := model.at
	if scene.precise3DLines && scene.Dimension == fieldDimension3D && scene.Display == fieldDisplayMagneticLines {
		return direct, "CPU-DIRECT-PRECISE-3D"
	}
	steps := fieldVectorGridSteps(scene)
	key := fieldVectorSceneKey(scene, nil, true, steps)
	if grid, ok := fieldCachedVectorGrid(true, key); ok {
		fieldRecordVectorGridCache("magnetic", grid, true)
		near := func(p fieldVec3) float64 { return fieldNearestObjectSurface(scene, p, false) }
		return grid.evaluator(direct, near, fieldMagneticVectorGridDirectDistance(grid)), "D3D11-GRID-CACHE"
	}
	span := fieldVectorGridSpan(scene)
	segments := len(model.FiniteCurrents)
	for _, magnet := range model.Magnets {
		segments += len(magnet.Segments)
	}
	started := time.Now()
	values, ok := fieldTryGPUMagneticVectorGrid(model, steps, -span, span)
	if !ok || len(values) != steps*steps*steps {
		fieldRecordVectorGridFallback("magnetic", steps, 0, segments, len(model.Wires), time.Since(started))
		return direct, "CPU-DIRECT"
	}
	grid := fieldVectorGrid{NX: steps, NY: steps, NZ: steps, Min: fieldVec3{X: -span, Y: -span, Z: -span}, Max: fieldVec3{X: span, Y: span, Z: span}, Values: values, Backend: "D3D11"}
	fieldStoreVectorGrid(true, key, grid)
	fieldRecordVectorGridReady("magnetic", grid, 0, segments, len(model.Wires), false, time.Since(started))
	near := func(p fieldVec3) float64 { return fieldNearestObjectSurface(scene, p, false) }
	return grid.evaluator(direct, near, fieldMagneticVectorGridDirectDistance(grid)), "D3D11-GRID"
}
