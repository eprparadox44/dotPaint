package main

import (
	"math"
	"sync"
	"time"
)

// fieldPotentialGeometry is the camera-independent half of a 3-D
// equipotential render.  The scalar grid and world-space triangles are
// expensive to rebuild but do not change when only the observer, background or
// antialias setting changes.
type fieldPotentialGeometry struct {
	Grid               fieldPotentialGrid
	Bands              []fieldPotentialBand
	Levels             []float64
	Vacuum             []fieldTriangle3
	Conductor          []fieldTriangle3
	RenderedLevels     int
	VacuumTriangles    int
	ConductorTriangles int
}

type fieldPotentialGeometryCacheEntry struct {
	Key      uint64
	Valid    bool
	Geometry fieldPotentialGeometry
}

var fieldPotentialGeometryCache struct {
	sync.Mutex
	Entry fieldPotentialGeometryCacheEntry
}

func fieldPotentialGeometrySceneKey(scene fieldScene, samples []fieldChargeSample, quality int, levels []float64) uint64 {
	steps := fieldEquipotentialGridSteps(scene, quality)
	key := fieldVectorSceneKey(scene, samples, false, steps)
	key ^= uint64(uint32(quality)+1) * 0x9e3779b185ebca87
	key ^= uint64(uint32(scene.Display.domain())+2) * 0xc2b2ae3d27d4eb4f
	key ^= uint64(uint32(scene.Density)+1) * 0x165667b19e3779f9
	for _, level := range levels {
		bits := math.Float64bits(level)
		key ^= bits + 0x9e3779b97f4a7c15 + (key << 6) + (key >> 2)
	}
	return key
}

func fieldCachedPotentialGeometry(key uint64) (fieldPotentialGeometry, bool) {
	fieldPotentialGeometryCache.Lock()
	defer fieldPotentialGeometryCache.Unlock()
	entry := fieldPotentialGeometryCache.Entry
	return entry.Geometry, entry.Valid && entry.Key == key
}

func fieldDiscardPotentialGeometryIfDifferent(key uint64) {
	fieldPotentialGeometryCache.Lock()
	if fieldPotentialGeometryCache.Entry.Valid && fieldPotentialGeometryCache.Entry.Key != key {
		// Drop the previous large triangle backing arrays before building a new
		// physical scene, so cache replacement does not double peak memory.
		fieldPotentialGeometryCache.Entry = fieldPotentialGeometryCacheEntry{}
	}
	fieldPotentialGeometryCache.Unlock()
}

func fieldStorePotentialGeometry(key uint64, geometry fieldPotentialGeometry) {
	if !geometry.Grid.valid() {
		return
	}
	fieldPotentialGeometryCache.Lock()
	fieldPotentialGeometryCache.Entry = fieldPotentialGeometryCacheEntry{Key: key, Valid: true, Geometry: geometry}
	fieldPotentialGeometryCache.Unlock()
}

func fieldPotentialGeometryForScene(scene fieldScene, samples []fieldChargeSample, levels []float64, quality int) (fieldPotentialGeometry, bool) {
	started := time.Now()
	key := fieldPotentialGeometrySceneKey(scene, samples, quality, levels)
	if geometry, ok := fieldCachedPotentialGeometry(key); ok {
		fieldRecordPotentialGeometryCache(true, quality, geometry.Grid.Steps, len(geometry.Levels), geometry.VacuumTriangles, geometry.ConductorTriangles, time.Since(started))
		return geometry, true
	}
	fieldDiscardPotentialGeometryIfDifferent(key)
	steps := fieldEquipotentialGridSteps(scene, quality)
	grid := fieldBuildPotentialGrid(scene, samples, steps)
	if !grid.valid() || scene.cancelled() {
		return fieldPotentialGeometry{}, false
	}
	bands := fieldPotentialBandsFromGrid(grid)
	if normalized := fieldPotentialSurfaceLevels(bands); len(normalized) > 0 {
		levels = normalized
	} else {
		levels = append([]float64(nil), levels...)
	}
	triMap := fieldIsoSurfaceTrianglesFromGridQuality(scene, samples, grid, levels, quality)
	if scene.cancelled() {
		return fieldPotentialGeometry{}, false
	}
	geometry := fieldPotentialGeometry{Grid: grid, Bands: bands, Levels: levels}
	for _, level := range levels {
		tris := triMap[level]
		if len(tris) == 0 {
			continue
		}
		geometry.RenderedLevels++
		geometry.VacuumTriangles += len(tris)
		geometry.Vacuum = append(geometry.Vacuum, tris...)
	}
	geometry.Conductor = fieldConductorEquipotentialTriangles(scene, samples, quality)
	geometry.ConductorTriangles = len(geometry.Conductor)
	if scene.cancelled() {
		return fieldPotentialGeometry{}, false
	}
	fieldStorePotentialGeometry(key, geometry)
	fieldRecordPotentialGeometryCache(false, quality, grid.Steps, len(levels), geometry.VacuumTriangles, geometry.ConductorTriangles, time.Since(started))
	return geometry, true
}
