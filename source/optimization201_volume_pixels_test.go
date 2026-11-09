package main

import (
	"bytes"
	"math"
	"testing"
)

func fieldVolumeRenderFixture201(w, h int) (fieldScene, fieldProjector, fieldPotentialGrid, []fieldPotentialBand) {
	scene := fieldDefaultScene()
	scene.Dimension = fieldDimension3D
	scene.Antialias = false
	steps := 26
	grid := fieldPotentialGrid{Steps: steps, MinCoord: -2, MaxCoord: 2, Values: make([]float64, steps*steps*steps), Conductor: make([]bool, steps*steps*steps)}
	for z := 0; z < steps; z++ {
		for y := 0; y < steps; y++ {
			for x := 0; x < steps; x++ {
				xx := 4*float64(x)/float64(steps-1) - 2
				yy := 4*float64(y)/float64(steps-1) - 2
				zz := 4*float64(z)/float64(steps-1) - 2
				grid.Values[(z*steps+y)*steps+x] = 1 / (math.Sqrt(xx*xx+yy*yy+zz*zz) + .25)
			}
		}
	}
	projector := fieldProjectorForScene(scene, w, h)
	bands := []fieldPotentialBand{{Sign: 1, Low: .005, High: 3.5, ZeroAnchored: true}}
	return scene, projector, grid, bands
}
func TestDotPaint201PotentialVolumeRendersIdenticalBGRA(t *testing.T) {
	for _, dims := range [][2]int{{52, 39}, {180, 120}, {240, 160}} {
		scene, p, grid, bands := fieldVolumeRenderFixture201(dims[0], dims[1])
		old, newr := fieldNewRaster(dims[0], dims[1]), fieldNewRaster(dims[0], dims[1])
		fieldRenderPotentialVolumeReference201(&old, scene, p, grid, bands, 0, false)
		fieldRenderPotentialVolume(&newr, scene, p, grid, bands, 0, false)
		if !bytes.Equal(old.Pix, newr.Pix) {
			for i := range old.Pix {
				if old.Pix[i] != newr.Pix[i] {
					t.Fatalf("volume pixels mismatch dims=%v byte=%d", dims, i)
				}
			}
		}
		for i := range old.Mask {
			if old.Mask[i] != newr.Mask[i] {
				t.Fatalf("volume mask mismatch dims=%v pixel=%d", dims, i)
			}
		}
	}
}
func BenchmarkDotPaint201PotentialVolumeReference(b *testing.B) {
	s, p, g, bands := fieldVolumeRenderFixture201(160, 120)
	for i := 0; i < b.N; i++ {
		r := fieldNewRaster(160, 120)
		fieldRenderPotentialVolumeReference201(&r, s, p, g, bands, 0, false)
	}
}
func BenchmarkDotPaint201PotentialVolumeOptimized(b *testing.B) {
	s, p, g, bands := fieldVolumeRenderFixture201(160, 120)
	for i := 0; i < b.N; i++ {
		r := fieldNewRaster(160, 120)
		fieldRenderPotentialVolume(&r, s, p, g, bands, 0, false)
	}
}
