package main

import "math"

// The volume ray marcher visits the same potential bands millions of times.
// Their logarithmic denominators depend on the scene, not the ray position.
// Keeping the original floating arithmetic order preserves strict thresholds.
type fieldPreparedPotentialBand struct {
	band                    fieldPotentialBand
	logLow, logDen, zeroDen float64
}

func fieldPreparePotentialBands(bands []fieldPotentialBand) []fieldPreparedPotentialBand {
	out := make([]fieldPreparedPotentialBand, len(bands))
	for i, b := range bands {
		out[i].band = b
		if b.Low > 0 && b.High > b.Low {
			if b.ZeroAnchored {
				out[i].zeroDen = math.Log1p(b.High / b.Low)
			} else {
				out[i].logLow = math.Log(b.Low)
				out[i].logDen = math.Log(b.High) - math.Log(b.Low)
			}
		}
	}
	return out
}
func fieldPreparedBandPercent(v fieldPreparedPotentialBand, value float64) (float64, bool) {
	b := v.band
	if b.Low <= 0 || b.High <= b.Low || value*b.Sign < 0 {
		return 0, false
	}
	magnitude := math.Abs(value)
	if b.ZeroAnchored {
		if magnitude <= 1e-14 {
			return 0, true
		}
		p := math.Log1p(magnitude/b.Low) / v.zeroDen
		return fieldClamp(p, 0, 1), true
	}
	if magnitude < b.Low*(1-1e-12) {
		return 0, false
	}
	if magnitude < b.Low {
		magnitude = b.Low
	}
	p := (math.Log(magnitude) - v.logLow) / v.logDen
	return fieldClamp(p, 0, 1), true
}
func fieldPotentialPercentForValuePrepared(bands []fieldPreparedPotentialBand, value float64) (float64, float64, bool) {
	for _, b := range bands {
		if percent, ok := fieldPreparedBandPercent(b, value); ok {
			return percent, b.band.Sign, true
		}
	}
	return 0, 0, false
}
