//go:build windows

package main

import "time"

func fieldTryGPUPotentialGrid(samples []fieldChargeSample, steps int, minCoord, maxCoord float64) ([]float64, bool) {
	return mdD3D11RenderFieldPotentialGrid(samples, steps, minCoord, maxCoord)
}

func fieldTryGPUElectricVectorGrid(samples []fieldChargeSample, steps int, minCoord, maxCoord float64) ([]fieldVec3, bool) {
	return mdD3D11RenderFieldVectorGrid(samples, fieldMagneticModel{}, false, steps, minCoord, maxCoord)
}

func fieldTryGPUMagneticVectorGrid(model fieldMagneticModel, steps int, minCoord, maxCoord float64) ([]fieldVec3, bool) {
	return mdD3D11RenderFieldVectorGrid(nil, model, true, steps, minCoord, maxCoord)
}

func fieldTryGPUMembraneRaster(width, height int, vertices []fieldGPUFaceVertex) ([]byte, bool) {
	gpu := make([]mdD3D11FaceVertex, len(vertices))
	for i, v := range vertices {
		gpu[i] = mdD3D11FaceVertex{X: v.X, Y: v.Y, Z: v.Z, R: v.R, G: v.G, B: v.B, A: v.A}
	}
	return mdD3D11RenderFieldMembraneTriangles(width, height, gpu)
}

func fieldRecordVectorGridReady(kind string, grid fieldVectorGrid, charges, segments, wires int, cache bool, elapsed time.Duration) {
	directRadius := fieldVectorGridDirectDistance(grid)
	if kind == "magnetic" {
		directRadius = fieldMagneticVectorGridDirectDistance(grid)
	}
	logf("FIELD_VECTOR_GRID type=%s backend=%s grid=%dx%dx%d points=%d charges=%d segments=%d wires=%d span=%.4g direct_radius=%.4g cache=%v elapsed=%s", kind, grid.Backend, grid.NX, grid.NY, grid.NZ, len(grid.Values), charges, segments, wires, grid.Max.X-grid.Min.X, directRadius, cache, elapsed)
}

func fieldRecordVectorGridCache(kind string, grid fieldVectorGrid, cache bool) {
	logf("FIELD_VECTOR_GRID type=%s backend=%s grid=%dx%dx%d points=%d cache=%v", kind, grid.Backend, grid.NX, grid.NY, grid.NZ, len(grid.Values), cache)
}

func fieldRecordVectorGridFallback(kind string, steps, charges, segments, wires int, elapsed time.Duration) {
	logf("FIELD_VECTOR_GRID type=%s backend=CPU-DIRECT grid=%dx%dx%d charges=%d segments=%d wires=%d fallback=true elapsed=%s reason=%q", kind, steps, steps, steps, charges, segments, wires, elapsed, mdD3D11PipelineReason("field-vector"))
}

func fieldRecordStreamlineTiming(kind, backend string, lines, candidates int, elapsed time.Duration) {
	logf("FIELD_STREAM type=%s backend=%s lines=%d candidates=%d elapsed=%s", kind, backend, lines, candidates, elapsed)
}

func fieldRecordStreamlineParallelTiming(kind, backend string, lines, candidates, workers, prefiltered, points, evaluations int, maxTask, elapsed time.Duration) {
	logf("FIELD_STREAM type=%s backend=%s lines=%d candidates=%d workers=%d prefiltered=%d points=%d evaluations=%d max_task=%s elapsed=%s", kind, backend, lines, candidates, workers, prefiltered, points, evaluations, maxTask, elapsed)
}

func fieldRecordPhysicalLineCache(kind string, lines int, elapsed time.Duration) {
	logf("FIELD_LINE_CACHE type=%s hit=true lines=%d elapsed=%s", kind, lines, elapsed)
}

func fieldRecordPotentialGeometryCache(hit bool, quality, steps, levels, vacuum, conductor int, elapsed time.Duration) {
	logf("FIELD_GEOMETRY_CACHE hit=%v quality=%d grid=%dx%dx%d levels=%d vacuum_triangles=%d conductor_triangles=%d elapsed=%s", hit, quality, steps, steps, steps, levels, vacuum, conductor, elapsed)
}

func fieldRecordLineSelection(kind string, stats fieldScreenLineSelectionStats) {
	logf("FIELD_LINE_SELECTION type=%s input=%d selected=%d groups=%d near_rejected=%d empty=%d cell=%d halo=%d", kind, stats.Input, stats.Selected, stats.Groups, stats.NearRejected, stats.EmptyRejected, stats.CellSize, stats.HaloCells)
}

func fieldRecordEquipotentialTiming(backend string, steps, samples, points, triangles int, grid, mesh time.Duration) {
	logf("FIELD_EQUIPOTENTIAL backend=%s grid=%dx%dx%d points=%d samples=%d triangles=%d potential=%s marching=%s total=%s", backend, steps, steps, steps, points, samples, triangles, grid, mesh, grid+mesh)
}

func fieldRecordEquipotentialLevelStats(level float64, raw, kept int, open bool) {
	logf("FIELD_EQUIPOTENTIAL_LEVEL level=%+.9g raw=%d kept=%d open=%v", level, raw, kept, open)
}

func fieldRecordEquipotentialRefinement(level float64, raw, corrected, projectionIterations, smoothIterations int, elapsed time.Duration) {
	logf("FIELD_EQUIPOTENTIAL_REFINE level=%+.9g raw=%d corrected=%d projection_iterations=%d smooth_iterations=%d elapsed=%s", level, raw, corrected, projectionIterations, smoothIterations, elapsed)
}

func fieldRecordGravityLineDiagnostics(backend string, candidates, outerHits, surfaceHits, directRetries, rejected int, elapsed time.Duration) {
	logf("FIELD_GRAVITY_LINES backend=%s candidates=%d outer_hits=%d surface_hits=%d direct_retries=%d rejected=%d elapsed=%s", backend, candidates, outerHits, surfaceHits, directRetries, rejected, elapsed)
}
