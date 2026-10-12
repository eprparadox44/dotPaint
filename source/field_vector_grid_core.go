package main

import "math"

// fieldVectorGrid stores a regular world-space E or B grid. GPU and CPU
// producers share this representation; CPU streamline integration consumes it
// through trilinear interpolation.
type fieldVectorGrid struct {
	NX, NY, NZ int
	Min, Max   fieldVec3
	Values     []fieldVec3
	Backend    string
}

func (g fieldVectorGrid) valid() bool {
	return g.NX >= 2 && g.NY >= 2 && g.NZ >= 2 &&
		len(g.Values) == g.NX*g.NY*g.NZ &&
		g.Max.X > g.Min.X && g.Max.Y > g.Min.Y && g.Max.Z > g.Min.Z
}

func (g fieldVectorGrid) index(ix, iy, iz int) int {
	return (iz*g.NY+iy)*g.NX + ix
}

// sample returns a trilinearly interpolated vector and false outside the grid.
func (g fieldVectorGrid) sample(p fieldVec3) (fieldVec3, bool) {
	if !g.valid() || !fieldVec3Finite(p) {
		return fieldVec3{}, false
	}
	epsilon := 1e-12
	if p.X < g.Min.X-epsilon || p.X > g.Max.X+epsilon ||
		p.Y < g.Min.Y-epsilon || p.Y > g.Max.Y+epsilon ||
		p.Z < g.Min.Z-epsilon || p.Z > g.Max.Z+epsilon {
		return fieldVec3{}, false
	}

	fx := fieldClampFloat((p.X-g.Min.X)/(g.Max.X-g.Min.X)*float64(g.NX-1), 0, float64(g.NX-1))
	fy := fieldClampFloat((p.Y-g.Min.Y)/(g.Max.Y-g.Min.Y)*float64(g.NY-1), 0, float64(g.NY-1))
	fz := fieldClampFloat((p.Z-g.Min.Z)/(g.Max.Z-g.Min.Z)*float64(g.NZ-1), 0, float64(g.NZ-1))

	x0, y0, z0 := int(math.Floor(fx)), int(math.Floor(fy)), int(math.Floor(fz))
	x1, y1, z1 := minInt(x0+1, g.NX-1), minInt(y0+1, g.NY-1), minInt(z0+1, g.NZ-1)
	tx, ty, tz := fx-float64(x0), fy-float64(y0), fz-float64(z0)

	c000 := g.Values[g.index(x0, y0, z0)]
	c100 := g.Values[g.index(x1, y0, z0)]
	c010 := g.Values[g.index(x0, y1, z0)]
	c110 := g.Values[g.index(x1, y1, z0)]
	c001 := g.Values[g.index(x0, y0, z1)]
	c101 := g.Values[g.index(x1, y0, z1)]
	c011 := g.Values[g.index(x0, y1, z1)]
	c111 := g.Values[g.index(x1, y1, z1)]

	c00 := fieldLerpVec3(c000, c100, tx)
	c10 := fieldLerpVec3(c010, c110, tx)
	c01 := fieldLerpVec3(c001, c101, tx)
	c11 := fieldLerpVec3(c011, c111, tx)
	c0 := fieldLerpVec3(c00, c10, ty)
	c1 := fieldLerpVec3(c01, c11, ty)
	out := fieldLerpVec3(c0, c1, tz)
	return out, fieldVec3Finite(out)
}

// evaluator combines the fast grid with a direct evaluator near physical
// surfaces or outside the grid. nearSurface returns the nearest physical
// boundary distance; directDistance should normally be about 1.5 grid cells.
func (g fieldVectorGrid) evaluator(direct fieldVectorEvaluator, nearSurface func(fieldVec3) float64, directDistance float64) fieldVectorEvaluator {
	return func(p fieldVec3) fieldVec3 {
		if nearSurface != nil && nearSurface(p) < directDistance && direct != nil {
			return direct(p)
		}
		if v, ok := g.sample(p); ok {
			return v
		}
		if direct != nil {
			return direct(p)
		}
		return fieldVec3{}
	}
}

func fieldLerpVec3(a, b fieldVec3, t float64) fieldVec3 {
	return fieldAdd(a, fieldMul(fieldSub(b, a), t))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
