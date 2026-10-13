package main

import (
	"math"
	"strconv"
	"strings"
)

type StretchSkewOptions struct {
	StretchHorizontal float64
	StretchVertical   float64
	SkewHorizontal    float64
	SkewVertical      float64
	Antialias         bool
}

func stretchSkewCompactFloat(v float64) string {
	v = math.Round(v*1000) / 1000
	if math.Abs(v) < 0.0005 {
		v = 0
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func stretchSkewParseDecimal(text string, min, max float64) (float64, bool) {
	text = strings.TrimSpace(strings.ReplaceAll(text, ",", "."))
	if text == "" {
		return 0, false
	}
	if i := strings.IndexByte(text, '.'); i >= 0 && len(text)-i-1 > 3 {
		return 0, false
	}
	v, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < min || v > max {
		return 0, false
	}
	return v, true
}

func stretchSkewScaledPixels(source int, percent float64) int {
	return max(1, int(math.Round(float64(source)*percent/100)))
}

func stretchSkewPercentForPixels(source, pixels int) float64 {
	if source <= 0 {
		return 0
	}
	return math.Round(float64(pixels)*100000/float64(source)) / 1000
}

func stretchSkewParsePixel(text string, source int) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || v < 1 || source < 1 || float64(v) > float64(source)*5 {
		return 0, false
	}
	return v, true
}

func stretchSkewGeometry(sourceW, sourceH int, opts StretchSkewOptions) (scaledW, scaledH, outputW, outputH int, ok bool) {
	if sourceW < 1 || sourceH < 1 || opts.StretchHorizontal < 0.001 || opts.StretchVertical < 0.001 || opts.StretchHorizontal > 500 || opts.StretchVertical > 500 {
		return 0, 0, 0, 0, false
	}
	if math.IsNaN(opts.StretchHorizontal) || math.IsNaN(opts.StretchVertical) || math.IsInf(opts.StretchHorizontal, 0) || math.IsInf(opts.StretchVertical, 0) ||
		math.IsNaN(opts.SkewHorizontal) || math.IsNaN(opts.SkewVertical) || math.IsInf(opts.SkewHorizontal, 0) || math.IsInf(opts.SkewVertical, 0) ||
		math.Abs(opts.SkewHorizontal) > 89 || math.Abs(opts.SkewVertical) > 89 {
		return 0, 0, 0, 0, false
	}
	scaledW = stretchSkewScaledPixels(sourceW, opts.StretchHorizontal)
	scaledH = stretchSkewScaledPixels(sourceH, opts.StretchVertical)
	sh := math.Tan(opts.SkewHorizontal * math.Pi / 180)
	sv := math.Tan(opts.SkewVertical * math.Pi / 180)
	// Horizontal shear first: x1=x+sh*y. Then vertical: y1=y+sv*x1.
	a, b := 1.0, sh
	c, d := sv, 1+sv*sh
	corners := [4][2]float64{{0, 0}, {float64(scaledW), 0}, {0, float64(scaledH)}, {float64(scaledW), float64(scaledH)}}
	minX, maxX := math.Inf(1), math.Inf(-1)
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, p := range corners {
		x := a*p[0] + b*p[1]
		y := c*p[0] + d*p[1]
		minX, maxX = math.Min(minX, x), math.Max(maxX, x)
		minY, maxY = math.Min(minY, y), math.Max(maxY, y)
	}
	left := int(math.Floor(minX + 1e-9))
	right := int(math.Ceil(maxX - 1e-9))
	top := int(math.Floor(minY + 1e-9))
	bottom := int(math.Ceil(maxY - 1e-9))
	outputW, outputH = max(1, right-left), max(1, bottom-top)
	if math.IsNaN(minX) || math.IsNaN(minY) || math.IsInf(minX, 0) || math.IsInf(minY, 0) {
		return 0, 0, 0, 0, false
	}
	return scaledW, scaledH, outputW, outputH, true
}
