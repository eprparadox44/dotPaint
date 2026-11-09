package main

import (
	"bytes"
	"math/rand"
	"testing"
)

func fieldMakeAAFixture201(w, h, scale, alphaPercent int) fieldRaster {
	r := fieldNewRaster(w*scale, h*scale)
	rng := rand.New(rand.NewSource(20261108))
	for i := 0; i < len(r.Pix); i += 4 {
		r.Pix[i+0] = byte(rng.Intn(256))
		r.Pix[i+1] = byte(rng.Intn(256))
		r.Pix[i+2] = byte(rng.Intn(256))
		if rng.Intn(100) < alphaPercent {
			r.Pix[i+3] = byte(1 + rng.Intn(255))
		}
	}
	return r
}

// Independent old algorithm, so optimizing transparent samples cannot alter
// straight-alpha RGB/alpha rounding or binary selection masks.
func fieldAAReference201(src fieldRaster, dstW, dstH, scale int) fieldRaster {
	dst := fieldNewRaster(dstW, dstH)
	for y := 0; y < dstH; y++ {
		for x := 0; x < dstW; x++ {
			var sumA, sumB, sumG, sumR uint64
			for oy := 0; oy < scale; oy++ {
				si := ((y*scale+oy)*src.W + x*scale) * 4
				for ox := 0; ox < scale; ox++ {
					a := uint64(src.Pix[si+3])
					sumA += a
					sumB += uint64(src.Pix[si+0]) * a
					sumG += uint64(src.Pix[si+1]) * a
					sumR += uint64(src.Pix[si+2]) * a
					si += 4
				}
			}
			if sumA == 0 {
				continue
			}
			di := (y*dstW + x) * 4
			dst.Pix[di+0] = byte((sumB + sumA/2) / sumA)
			dst.Pix[di+1] = byte((sumG + sumA/2) / sumA)
			dst.Pix[di+2] = byte((sumR + sumA/2) / sumA)
			dst.Pix[di+3] = byte((sumA + uint64(scale*scale)/2) / uint64(scale*scale))
			dst.Mask[y*dstW+x] = true
		}
	}
	return dst
}
func TestDotPaint201FieldDownsampleAAExactPixelIdentity(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {7, 9}, {44, 35}, {95, 67}} {
		for _, scale := range []int{2, 3, 6} {
			for _, pct := range []int{0, 3, 25, 100} {
				src := fieldMakeAAFixture201(size[0], size[1], scale, pct)
				want, got := fieldAAReference201(src, size[0], size[1], scale), fieldDownsampleRasterScale(src, size[0], size[1], scale)
				if !bytes.Equal(want.Pix, got.Pix) {
					t.Fatalf("pixels differ size=%v scale=%d fill=%d", size, scale, pct)
				}
				for i := range want.Mask {
					if want.Mask[i] != got.Mask[i] {
						t.Fatalf("mask differs")
					}
				}
			}
		}
	}
}
func BenchmarkDotPaint201FieldAA(b *testing.B) {
	for _, pct := range []int{3, 25, 100} {
		name := "Sparse3"
		if pct == 25 {
			name = "Sparse25"
		}
		if pct == 100 {
			name = "Dense100"
		}
		b.Run(name, func(b *testing.B) {
			src := fieldMakeAAFixture201(720, 540, 6, pct)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				out := fieldDownsampleRasterScale(src, 720, 540, 6)
				if out.W != 720 {
					b.Fatal(out.W)
				}
			}
		})
	}
}
func BenchmarkDotPaint201FieldAAReference(b *testing.B) {
	for _, pct := range []int{3, 25, 100} {
		name := "Sparse3"
		if pct == 25 {
			name = "Sparse25"
		}
		if pct == 100 {
			name = "Dense100"
		}
		b.Run(name, func(b *testing.B) {
			src := fieldMakeAAFixture201(720, 540, 6, pct)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r := fieldAAReference201(src, 720, 540, 6)
				if r.W != 720 {
					b.Fatal(r.W)
				}
			}
		})
	}
}
