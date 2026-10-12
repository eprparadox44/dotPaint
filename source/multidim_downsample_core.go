package main

import (
	"runtime"
	"sync"
)

// mdAreaDownsampleBGRA performs an exact integer box average. Each output row
// is independent, so parallel execution is deterministic and byte-identical
// to the serial formula.
func mdAreaDownsampleBGRA(dst, src []byte, width, height, scale int) bool {
	if width <= 0 || height <= 0 || scale <= 1 || len(dst) < width*height*4 || len(src) < width*scale*height*scale*4 {
		return false
	}
	workers := runtime.GOMAXPROCS(0)
	if workers > height {
		workers = height
	}
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		y0, y1 := height*worker/workers, height*(worker+1)/workers
		wg.Add(1)
		go func(y0, y1 int) {
			defer wg.Done()
			srcW := width * scale
			div := uint32(scale * scale)
			for y := y0; y < y1; y++ {
				for x := 0; x < width; x++ {
					var b, g, r uint32
					for sy := 0; sy < scale; sy++ {
						i := ((y*scale+sy)*srcW + x*scale) * 4
						for sx := 0; sx < scale; sx++ {
							b += uint32(src[i])
							g += uint32(src[i+1])
							r += uint32(src[i+2])
							i += 4
						}
					}
					d := (y*width + x) * 4
					dst[d], dst[d+1], dst[d+2], dst[d+3] = byte((b+div/2)/div), byte((g+div/2)/div), byte((r+div/2)/div), 255
				}
			}
		}(y0, y1)
	}
	wg.Wait()
	return true
}
