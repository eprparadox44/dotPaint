//go:build windows

package main

import "time"

func multidimCelestialCacheKey(s *multidimShapeEditorState, w, h, skin int) uint64 {
	if s == nil {
		return 0
	}
	vals := []int{
		w, h, skin,
		multidimSliderPos(s.sizeSlider),
		multidimSliderPos(s.xSlider),
		multidimSliderPos(s.ySlider),
		multidimSliderPos(s.zSlider),
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

func multidimPaintTexturedSphere(s *multidimShapeEditorState, hdc uintptr, rc RECT, skin int) {
	if s == nil || hdc == 0 || skin <= mdSphereSkinPlain || skin > mdSphereSkinSaturn {
		return
	}
	w := int(rc.Right - rc.Left)
	h := int(rc.Bottom - rc.Top)
	if w <= 0 || h <= 0 {
		return
	}
	key := multidimCelestialCacheKey(s, w, h, skin)
	if key != s.celestialCacheKey || s.celestialCacheW != w || s.celestialCacheH != h || len(s.celestialCachePixels) != w*h*4 {
		ax, ay, az, _ := multidimRotation(s)
		// Earth and Moon rotate as wrapped maps. Saturn now uses a true full
		// cylindrical Cassini-derived texture plus a radial ring profile, so
		// different longitudes and the reconstructed back hemisphere rotate into view.
		r := float64(multidimSliderPos(s.sizeSlider)) / 200.0
		radiusPx := float64(min(w, h)) * 0.33 * r
		// Saturn needs room for an annulus more than twice the globe radius.
		// Shrink only when the selected size would clip the ring from the preview.
		if skin == mdSphereSkinSaturn {
			limit := float64(min(w, h)) * 0.47 / 2.50
			if radiusPx > limit {
				radiusPx = limit
			}
		}
		params := mdCelestialRenderParams{
			width:        w,
			height:       h,
			radiusPixels: radiusPx,
			rotation:     mdEulerRotationMatrix(ax, ay, az),
			skin:         skin,
			maskPass:     s.insertMaskPass,
		}
		renderStarted := time.Now()
		if gpuPixels, ok := mdD3D11RenderCelestial(params); ok {
			s.celestialCachePixels = gpuPixels
		} else {
			reason := mdD3D11PipelineReason("celestial")
			if reason == "" {
				reason = "D3D11 render or readback failed"
			}
			mdD3D11LogFallbackOnce("celestial", reason)
			s.celestialCachePixels = mdCelestialRender(params)
			logf("MULTIDIM_RENDER shape=%s backend=CPU size=%dx%d reason=%q total=%s fallback=true", mdD3D11CelestialShapeName(skin), w, h, reason, time.Since(renderStarted))
		}
		s.celestialCacheW = w
		s.celestialCacheH = h
		s.celestialCacheKey = key
	}
	if len(s.celestialCachePixels) == 0 {
		return
	}
	if !multidimBlitBGRA(s, hdc, rc, s.celestialCachePixels, w, h, "celestial") {
		logf("MULTIDIM celestial frame not drawn skin=%d size=%dx%d", skin, w, h)
	}
}
