package main

import (
	"math"
	"sort"
	"time"
)

type fieldVectorArrowSample struct {
	P, V  fieldVec3
	Mag   float64
	Depth float64
}

const (
	fieldVectorDensityMinTenths = 10
	fieldVectorDensityMaxTenths = 20

	fieldVectorArrowMinLengthFraction = 0.0
	fieldVectorArrowMaxLengthFraction = .78
	fieldVectorArrowStrengthCap       = 3.0
)

func fieldVectorArrowDensityScale(scene fieldScene) float64 {
	value := scene.VectorDensity
	if value < fieldVectorDensityMinTenths {
		value = fieldVectorDensityMinTenths
	}
	if value > fieldVectorDensityMaxTenths {
		value = fieldVectorDensityMaxTenths
	}
	return float64(value) / 10
}

func fieldVectorArrowGridCounts(scene fieldScene) (nx, ny, nz int) {
	// 1.0x preserves the 1.2.31 default lattice. 2.0x doubles each spatial
	// axis exactly. In 3-D that deliberately permits up to eight times as many
	// samples, but all samples reuse the already-computed D3D11 vector grid.
	scale := fieldVectorArrowDensityScale(scene)
	nx = max(5, int(math.Round(7*scale)))
	nz = max(5, int(math.Round(7*scale)))
	if scene.Dimension == fieldDimension2D {
		return nx, 1, nz
	}
	ny = max(3, int(math.Round(5*scale)))
	return nx, ny, nz
}

// fieldVectorArrowLengthFraction maps field magnitude to a readable arrow
// length relative to the lattice spacing. Zero field maps to zero length; weak,
// medium and strong fields remain distinguishable through logarithmic shaping,
// while the explicit cap prevents arrows near singularities from crossing
// neighbouring lattice points.
func fieldVectorArrowLengthFraction(magnitude, reference float64) float64 {
	if math.IsNaN(magnitude) || math.IsInf(magnitude, 0) || magnitude <= 0 ||
		math.IsNaN(reference) || math.IsInf(reference, 0) || reference <= 0 {
		return 0
	}
	relative := fieldClamp(magnitude/reference, 0, fieldVectorArrowStrengthCap)
	shaped := math.Log1p(4*relative) / math.Log1p(4*fieldVectorArrowStrengthCap)
	return fieldVectorArrowMinLengthFraction +
		(fieldVectorArrowMaxLengthFraction-fieldVectorArrowMinLengthFraction)*shaped
}

func fieldVectorArrowInsideSource(scene fieldScene, p fieldVec3, magnetic, gravity bool) bool {
	for _, o := range scene.Objects {
		if gravity {
			if !o.gravity() {
				continue
			}
		} else if magnetic {
			if !o.magnetic() {
				continue
			}
		} else if !o.electric() {
			continue
		}
		if fieldPointInsideObject(p, o, .035) {
			return true
		}
	}
	return false
}

func fieldDrawVectorArrowHead(r *fieldRaster, p fieldProjector, a, b fieldVec3, rr, gg, bb byte) {
	x0, y0, _, ok0 := p.project(a)
	x1, y1, _, ok1 := p.project(b)
	if !ok0 || !ok1 {
		return
	}
	dx, dy := float64(x1-x0), float64(y1-y0)
	pixelLength := math.Hypot(dx, dy)
	sampleScale := float64(r.sampleScale())
	if pixelLength < 3*sampleScale {
		return
	}
	dx, dy = dx/pixelLength, dy/pixelLength
	px, py := -dy, dx
	head := fieldClamp(pixelLength*.28, 3.0*sampleScale, 6.0*sampleScale)
	leftX := int(math.Round(float64(x1) - dx*head + px*head*.55))
	leftY := int(math.Round(float64(y1) - dy*head + py*head*.55))
	rightX := int(math.Round(float64(x1) - dx*head - px*head*.55))
	rightY := int(math.Round(float64(y1) - dy*head - py*head*.55))
	r.line(x1, y1, leftX, leftY, rr, gg, bb, .95, 2)
	r.line(x1, y1, rightX, rightY, rr, gg, bb, .95, 2)
}

func fieldDrawVectorField(r *fieldRaster, projector fieldProjector, scene fieldScene, samples []fieldChargeSample, magnetic, gravity, dark bool) {
	started := time.Now()
	var eval fieldVectorEvaluator
	backend := "CPU-DIRECT"
	kind := "electric"
	if gravity {
		kind = "gravity"
		model := fieldBuildGravityModel(scene, 1)
		eval, backend = fieldGravityVectorEvaluator(scene, model, samples)
	} else if magnetic {
		kind = "magnetic"
		model := fieldBuildMagneticModel(scene)
		eval, backend = fieldMagneticVectorEvaluator(scene, model)
	} else {
		eval, backend = fieldElectricVectorEvaluator(scene, samples)
	}
	if eval == nil {
		return
	}

	nx, ny, nz := fieldVectorArrowGridCounts(scene)
	densityScale := fieldVectorArrowDensityScale(scene)
	span := math.Max(2.3, fieldSceneSpan(scene)*.86)
	if scene.Dimension == fieldDimension2D {
		span = math.Max(2.15, fieldSceneSpan(scene)*.84)
	}
	spacingX := 2 * span / float64(max(1, nx-1))
	spacingY := 2 * span / float64(max(1, ny-1))
	spacingZ := 2 * span / float64(max(1, nz-1))
	spacing := math.Min(spacingX, spacingZ)
	if ny > 1 {
		spacing = math.Min(spacing, spacingY)
	}

	arrows := make([]fieldVectorArrowSample, 0, nx*ny*nz)
	magnitudes := make([]float64, 0, nx*ny*nz)
	for iy := 0; iy < ny; iy++ {
		y := 0.0
		if ny > 1 {
			y = -span + 2*span*float64(iy)/float64(ny-1)
		}
		for iz := 0; iz < nz; iz++ {
			z := -span + 2*span*float64(iz)/float64(max(1, nz-1))
			for ix := 0; ix < nx; ix++ {
				x := -span + 2*span*float64(ix)/float64(max(1, nx-1))
				p := fieldVec3{X: x, Y: y, Z: z}
				if fieldVectorArrowInsideSource(scene, p, magnetic, gravity) {
					continue
				}
				v := eval(p)
				mag := v.len()
				if !fieldFiniteVec(v) || mag < 1e-9 || math.IsNaN(mag) || math.IsInf(mag, 0) {
					continue
				}
				_, _, depth, ok := projector.project(p)
				if !ok {
					continue
				}
				arrows = append(arrows, fieldVectorArrowSample{P: p, V: v, Mag: mag, Depth: depth})
				magnitudes = append(magnitudes, mag)
			}
		}
	}
	if len(arrows) == 0 {
		fieldRecordVectorArrows(kind, backend, nx, ny, nz, nx*ny*nz, 0, densityScale, 0, 0, 0, time.Since(started))
		return
	}
	sort.Float64s(magnitudes)
	ref := magnitudes[min(len(magnitudes)-1, int(float64(len(magnitudes)-1)*.88))]
	if ref < 1e-9 {
		ref = magnitudes[len(magnitudes)-1]
	}
	// Far arrows first, near arrows last.
	sort.SliceStable(arrows, func(i, j int) bool { return arrows[i].Depth < arrows[j].Depth })

	var rr, gg, bb byte
	if gravity {
		if dark {
			rr, gg, bb = 235, 190, 95
		} else {
			rr, gg, bb = 150, 95, 25
		}
	} else if magnetic {
		if dark {
			rr, gg, bb = 105, 225, 135
		} else {
			rr, gg, bb = 35, 140, 70
		}
	} else if dark {
		rr, gg, bb = 235, 235, 235
	} else {
		rr, gg, bb = 35, 35, 35
	}
	rendered := 0
	minLength, maxLength := math.Inf(1), 0.0
	for _, a := range arrows {
		lengthFraction := fieldVectorArrowLengthFraction(a.Mag, ref)
		length := spacing * lengthFraction
		direction := a.V.norm()
		start := a.P.sub(direction.mul(length * .46))
		end := a.P.add(direction.mul(length * .54))
		x0, y0, d0, ok0 := projector.project(start)
		x1, y1, d1, ok1 := projector.project(end)
		if !ok0 || !ok1 || math.Hypot(float64(x1-x0), float64(y1-y0)) < 3.0*float64(r.sampleScale()) {
			continue
		}
		alpha := .84
		if scene.Dimension == fieldDimension3D {
			alpha = fieldClamp(.56+(d0+d1)*.045, .32, .92)
		}
		r.line(x0, y0, x1, y1, rr, gg, bb, alpha, 1)
		fieldDrawVectorArrowHead(r, projector, start, end, rr, gg, bb)
		rendered++
		if length < minLength {
			minLength = length
		}
		if length > maxLength {
			maxLength = length
		}
	}
	if rendered == 0 {
		minLength = 0
	}
	fieldRecordVectorArrows(kind, backend, nx, ny, nz, nx*ny*nz, rendered, densityScale, ref, minLength, maxLength, time.Since(started))
}
