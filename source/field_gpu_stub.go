//go:build !windows

package main

import "time"

func fieldTryGPUPotentialGrid(samples []fieldChargeSample, steps int, minCoord, maxCoord float64) ([]float64, bool) {
	return nil, false
}

func fieldTryGPUElectricVectorGrid(samples []fieldChargeSample, steps int, minCoord, maxCoord float64) ([]fieldVec3, bool) {
	return nil, false
}

func fieldTryGPUMagneticVectorGrid(model fieldMagneticModel, steps int, minCoord, maxCoord float64) ([]fieldVec3, bool) {
	return nil, false
}

func fieldTryGPUMembraneRaster(width, height int, vertices []fieldGPUFaceVertex) ([]byte, bool) {
	return nil, false
}

func fieldRecordVectorGridReady(kind string, grid fieldVectorGrid, charges, segments, wires int, cache bool, elapsed time.Duration) {
}
func fieldRecordVectorGridCache(kind string, grid fieldVectorGrid, cache bool) {}
func fieldRecordVectorGridFallback(kind string, steps, charges, segments, wires int, elapsed time.Duration) {
}
func fieldRecordStreamlineTiming(kind, backend string, lines, candidates int, elapsed time.Duration) {
}

func fieldRecordStreamlineParallelTiming(kind, backend string, lines, candidates, workers, prefiltered, points, evaluations int, maxTask, elapsed time.Duration) {
}

func fieldRecordPhysicalLineCache(kind string, lines int, elapsed time.Duration) {
}

func fieldRecordPotentialGeometryCache(hit bool, quality, steps, levels, vacuum, conductor int, elapsed time.Duration) {
}

func fieldRecordLineSelection(kind string, stats fieldScreenLineSelectionStats) {}

func fieldRecordEquipotentialTiming(backend string, steps, samples, points, triangles int, grid, mesh time.Duration) {
}

func fieldRecordEquipotentialLevelStats(level float64, raw, kept int, open bool) {
}

func fieldRecordEquipotentialRefinement(level float64, raw, corrected, projectionIterations, smoothIterations int, elapsed time.Duration) {
}

func fieldRecordGravityLineDiagnostics(backend string, candidates, outerHits, surfaceHits, directRetries, rejected int, elapsed time.Duration) {
}
