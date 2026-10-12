//go:build windows

package main

import "time"

func fieldRecordVectorArrows(kind, backend string, nx, ny, nz, samples, arrows int, densityScale, reference, minLength, maxLength float64, elapsed time.Duration) {
	logf("FIELD_VECTOR_ARROWS type=%s backend=%s lattice=%dx%dx%d density=%.1fx samples=%d arrows=%d reference=%g length=%g..%g min_fraction=%.2f cap_fraction=%.2f elapsed=%s", kind, backend, nx, ny, nz, densityScale, samples, arrows, reference, minLength, maxLength, fieldVectorArrowMinLengthFraction, fieldVectorArrowMaxLengthFraction, elapsed)
}
