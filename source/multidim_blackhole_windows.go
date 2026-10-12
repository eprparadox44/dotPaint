//go:build windows

package main

import (
	"math"
	"time"
)

func multidimEditorBlackHoleMode(s *multidimShapeEditorState) int {
	if s == nil || s.blackHoleMode < mdBlackHoleModeCurvatureGrid || s.blackHoleMode > mdBlackHoleModeGeodesics {
		return mdBlackHoleModeCurvatureGrid
	}
	return s.blackHoleMode
}

func multidimBlackHoleCacheKey(s *multidimShapeEditorState, w, h, quality int) uint64 {
	if s == nil {
		return 0
	}
	vals := []int{
		w, h, quality,
		multidimEditorBlackHoleMode(s),
		multidimSliderPos(s.sizeSlider),
		multidimSliderPos(s.xSlider),
		multidimSliderPos(s.ySlider),
		multidimSliderPos(s.zSlider),
		multidimSliderPos(s.widthSlider),
		multidimSliderPos(s.heightSlider),
	}
	if s.insertMaskPass {
		vals = append(vals, 1)
	} else {
		vals = append(vals, 0)
	}
	var key uint64 = 1469598103934665603
	for _, v := range vals {
		key ^= uint64(uint32(v))
		key *= 1099511628211
	}
	return key
}

func multidimBlackHoleGridLines(s *multidimShapeEditorState) int {
	pos := 2
	if s != nil {
		pos = multidimSliderPos(s.widthSlider)
	}
	if pos < 1 {
		pos = 1
	}
	if pos > 8 {
		pos = 8
	}
	return 2*pos + 1
}

func multidimBlackHoleSpin(s *multidimShapeEditorState) float64 {
	// The adjustable Kerr parameter is an educational control for the 3-D
	// curvature lattice only. The photorealistic black-hole view already shows
	// orbital Doppler/gravitational redshift and must not silently inherit a spin
	// value selected in another display mode.
	if s == nil || multidimEditorBlackHoleMode(s) != mdBlackHoleModeCurvatureGrid {
		return 0
	}
	pos := multidimSliderPos(s.heightSlider)
	if pos < 0 {
		pos = 0
	}
	if pos > 99 {
		pos = 99
	}
	return float64(pos) / 100.0
}

func multidimBlackHoleRenderDimensions(s *multidimShapeEditorState, targetW, targetH int, gpu bool) (int, int, int) {
	if targetW < 1 || targetH < 1 {
		return 0, 0, 0
	}
	quality := 0
	factor := 1.0
	maxW := 360
	if s != nil && !s.previewInteractive {
		maxW = 720
		quality = 1
		if multidimEditorSmoothEnabled(s) {
			// A206 doubles the established black-hole AA from 2x to 4x when the
			// Direct3D 11 compute path is available. The CPU compatibility path
			// retains A201's 2x ceiling so an unsupported GPU does not turn a
			// fallback render into a multi-minute operation.
			factor = 2.0
			maxW = 1280
			quality = 2
			if gpu {
				factor = 4.0
				maxW = 2560
			}
		}
	}
	w := int(math.Round(float64(targetW) * factor))
	if w > maxW {
		w = maxW
	}
	if w < 1 {
		w = 1
	}
	h := int(math.Round(float64(targetH) * float64(w) / float64(targetW)))
	if h < 1 {
		h = 1
	}
	return w, h, quality
}

func multidimBlackHoleDiskNormal(s *multidimShapeEditorState) mdVec3 {
	// The fixed presentation starts at a 65-degree observer inclination: enough
	// to show both the front disk and its lensed rear image without collapsing
	// to an edge-on line. The ordinary X/Y/Z controls rotate this physical disk
	// orientation, while the Schwarzschild hole itself remains spherical.
	inclination := 65.0 * math.Pi / 180.0
	base := mdVec3{0, math.Cos(inclination), math.Sin(inclination)}
	ax, ay, az, _ := multidimRotation(s)
	return mdNormCore(mdMat3Apply(mdEulerRotationMatrix(ax, ay, az), base))
}

func multidimBlackHoleDistance(s *multidimShapeEditorState) float64 {
	pos := 170
	if s != nil {
		pos = multidimSliderPos(s.sizeSlider)
	}
	if pos < 5 {
		pos = 5
	}
	if pos > 500 {
		pos = 500
	}
	return 20.0 + float64(pos-5)*(70.0/495.0)
}

func multidimBlackHoleFOV(s *multidimShapeEditorState) float64 {
	pos := 170
	if s != nil {
		pos = multidimSliderPos(s.sizeSlider)
	}
	if pos < 5 {
		pos = 5
	}
	// Larger size values mean a tighter camera field and therefore a larger
	// apparent black hole, matching every other shape's size control.
	deg := 50.0 * 170.0 / float64(pos)
	deg = mdBHClamp(deg, 22, 78)
	return deg * math.Pi / 180.0
}

func multidimPaintBlackHole(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	if s == nil || hdc == 0 {
		return
	}
	targetW := int(rc.Right - rc.Left)
	targetH := int(rc.Bottom - rc.Top)
	if targetW <= 0 || targetH <= 0 {
		return
	}
	mode := multidimEditorBlackHoleMode(s)
	gpu := (mode == mdBlackHoleModeRender || mode == mdBlackHoleModeRenderWithDiagram) && mdD3D11BlackHoleAvailable()
	rw, rh, quality := multidimBlackHoleRenderDimensions(s, targetW, targetH, gpu)
	if rw <= 0 || rh <= 0 {
		return
	}
	key := multidimBlackHoleCacheKey(s, rw, rh, quality)
	if key != s.blackHoleCacheKey || s.blackHoleCacheW != rw || s.blackHoleCacheH != rh || len(s.blackHoleCachePixels) != rw*rh*4 {
		ax, ay, az, _ := multidimRotation(s)
		params := mdBHRenderParams{
			width:          rw,
			height:         rh,
			fovY:           multidimBlackHoleFOV(s),
			diskNormal:     multidimBlackHoleDiskNormal(s),
			mode:           mode,
			quality:        quality,
			rotationX:      ax,
			rotationY:      ay,
			rotationZ:      az,
			cameraDistance: multidimBlackHoleDistance(s),
			gridDensity:    multidimBlackHoleGridLines(s),
			spin:           multidimBlackHoleSpin(s),
			maskPass:       s.insertMaskPass,
		}
		renderStarted := time.Now()
		if gpu {
			if pixels, ok := mdD3D11RenderBlackHole(params); ok {
				s.blackHoleCachePixels = pixels
			} else {
				reason := mdD3D11PipelineReason("blackhole")
				if reason == "" {
					reason = "D3D11 render or readback failed"
				}
				s.blackHoleCachePixels = mdBHRender(params)
				logf("MULTIDIM_RENDER shape=blackhole backend=CPU size=%dx%d quality=%d spin=%.2f grid_lines=%d reason=%q total=%s fallback=true", rw, rh, quality, params.spin, params.gridDensity, reason, time.Since(renderStarted))
			}
		} else {
			s.blackHoleCachePixels = mdBHRender(params)
			if mode == mdBlackHoleModeRender || mode == mdBlackHoleModeRenderWithDiagram {
				reason := mdD3D11PipelineReason("blackhole")
				mdD3D11LogFallbackOnce("blackhole", reason)
				logf("MULTIDIM_RENDER shape=blackhole backend=CPU size=%dx%d quality=%d spin=%.2f grid_lines=%d reason=%q total=%s fallback=true", rw, rh, quality, params.spin, params.gridDensity, reason, time.Since(renderStarted))
			}
		}
		s.blackHoleCacheW = rw
		s.blackHoleCacheH = rh
		s.blackHoleCacheKey = key
	}
	if len(s.blackHoleCachePixels) == 0 {
		return
	}
	if !multidimBlitBGRA(s, hdc, rc, s.blackHoleCachePixels, rw, rh, "black-hole") {
		logf("MULTIDIM black-hole frame not drawn mode=%d src=%dx%d dst=%dx%d", multidimEditorBlackHoleMode(s), rw, rh, targetW, targetH)
	}
}
