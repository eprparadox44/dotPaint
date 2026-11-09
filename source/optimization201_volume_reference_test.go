package main

import (
	"math"
	"runtime"
	"sync"
)

func fieldRenderPotentialVolumeReference201(r *fieldRaster, scene fieldScene, projector fieldProjector, grid fieldPotentialGrid, bands []fieldPotentialBand, quality int, dark bool) {
	if r == nil || r.W < 1 || r.H < 1 || !grid.valid() || len(bands) == 0 {
		return
	}
	// Volume integration is deliberately performed on a smooth low-resolution
	// buffer and bilinearly expanded.  The scalar grid already carries the
	// physical resolution; tracing one ray for every 2x-AA subpixel would only
	// repeat nearly identical work and make the live editor unusable.
	maxW, maxH := 360, 270
	if quality > 0 {
		maxW, maxH = 440, 330
	}
	scale := math.Min(1, math.Min(float64(maxW)/float64(r.W), float64(maxH)/float64(r.H)))
	vw, vh := max(1, int(math.Ceil(float64(r.W)*scale))), max(1, int(math.Ceil(float64(r.H)*scale)))
	vol := fieldNewRaster(vw, vh)
	vp := projector
	vp.W, vp.H = vw, vh
	raySteps := 28
	if quality > 0 {
		raySteps = 36
	}
	if quality > 1 {
		raySteps = 46
	}
	workers := min(runtime.GOMAXPROCS(0), max(1, vh/20))
	var wg sync.WaitGroup
	rows := (vh + workers - 1) / workers
	for worker := 0; worker < workers; worker++ {
		y0, y1 := worker*rows, min(vh, (worker+1)*rows)
		if y0 >= y1 {
			continue
		}
		wg.Add(1)
		go func(y0, y1 int) {
			defer wg.Done()
			for y := y0; y < y1; y++ {
				if scene.cancelled() {
					return
				}
				for x := 0; x < vw; x++ {
					origin, direction := fieldProjectorScreenRay(vp, float64(x)+.5, float64(y)+.5)
					if direction.len2() < .5 {
						continue
					}
					t0, t1, ok := fieldRayCubeInterval(origin.sub(grid.Center), direction, grid.MinCoord, grid.MaxCoord)
					if !ok {
						continue
					}
					dt := (t1 - t0) / float64(raySteps)
					if dt <= 0 {
						continue
					}
					accA, accR, accG, accB := 0.0, 0.0, 0.0, 0.0
					for step := 0; step < raySteps && accA < .72; step++ {
						t := t0 + (float64(step)+.5)*dt
						point := origin.add(direction.mul(t))
						value, ok := grid.sample(point)
						if !ok {
							continue
						}
						percent, sign, ok := fieldPotentialPercentForValue(bands, value)
						if !ok || percent < .40 || grid.conductorAt(point) {
							continue
						}
						density := fieldPotentialVolumeDensity(percent)
						if density <= 1e-6 {
							continue
						}
						optical := density * 1.15 * dt / math.Max(1e-6, grid.MaxCoord-grid.MinCoord)
						a := 1 - math.Exp(-optical)
						rr, gg, bb := fieldPotentialColor(sign, dark)
						remain := 1 - accA
						accR += float64(rr) * a * remain
						accG += float64(gg) * a * remain
						accB += float64(bb) * a * remain
						accA += a * remain
					}
					if accA > 1e-4 {
						rr := byte(fieldClamp(accR/accA, 0, 255) + .5)
						gg := byte(fieldClamp(accG/accA, 0, 255) + .5)
						bb := byte(fieldClamp(accB/accA, 0, 255) + .5)
						vol.blend(x, y, rr, gg, bb, math.Min(.52, accA))
					}
				}
			}
		}(y0, y1)
	}
	wg.Wait()
	// Bilinear expansion keeps the high-potential cloud smooth without adding
	// artificial isosurface edges.
	for y := 0; y < r.H; y++ {
		if scene.cancelled() {
			return
		}
		fy := (float64(y)+.5)*float64(vh)/float64(r.H) - .5
		y0 := max(0, min(vh-1, int(math.Floor(fy))))
		y1 := max(0, min(vh-1, y0+1))
		ty := fieldClamp(fy-float64(y0), 0, 1)
		for x := 0; x < r.W; x++ {
			fx := (float64(x)+.5)*float64(vw)/float64(r.W) - .5
			x0 := max(0, min(vw-1, int(math.Floor(fx))))
			x1 := max(0, min(vw-1, x0+1))
			tx := fieldClamp(fx-float64(x0), 0, 1)
			weights := [4]float64{(1 - tx) * (1 - ty), tx * (1 - ty), (1 - tx) * ty, tx * ty}
			coords := [4][2]int{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}}
			var sumA, sumR, sumG, sumB float64
			for i, c := range coords {
				o := (c[1]*vw + c[0]) * 4
				a := float64(vol.Pix[o+3]) / 255
				w := weights[i]
				sumA += a * w
				sumR += float64(vol.Pix[o+2]) * a * w
				sumG += float64(vol.Pix[o+1]) * a * w
				sumB += float64(vol.Pix[o+0]) * a * w
			}
			if sumA <= 1e-5 {
				continue
			}
			rr := byte(fieldClamp(sumR/sumA, 0, 255) + .5)
			gg := byte(fieldClamp(sumG/sumA, 0, 255) + .5)
			bb := byte(fieldClamp(sumB/sumA, 0, 255) + .5)
			o := (y*r.W + x) * 4
			if r.Pix[o+3] == 0 {
				// The potential volume is the first layer in the equipotential
				// renderer.  Store its straight-alpha sample directly instead of
				// running the general four-channel compositor against transparent
				// black for millions of 6x subpixels.
				r.Pix[o+0], r.Pix[o+1], r.Pix[o+2] = bb, gg, rr
				r.Pix[o+3] = byte(fieldClamp(sumA, 0, 1)*255 + .5)
				if len(r.Mask) == r.W*r.H {
					r.Mask[y*r.W+x] = true
				}
			} else {
				r.blend(x, y, rr, gg, bb, sumA)
			}
		}
	}
}
