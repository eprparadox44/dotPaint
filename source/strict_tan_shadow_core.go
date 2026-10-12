package main

import "math"

// strictTanShadowSegment is the invisible continuous vector used only for
// antialias coverage in Shift+tan n mode.  The visible core remains the exact
// integer staircase.  Unlike A96, this segment is not inferred from the first
// and last staircase pixels: its direction comes from the selected reduced
// ratio and its length comes from the dominant-axis step count.
type strictTanShadowSegment struct {
	X0, Y0 float64
	X1, Y1 float64

	UnitX, UnitY float64
	Length       float64

	ParityOffsetX float64
	ParityOffsetY float64
	PhaseOffsetX  float64
	PhaseOffsetY  float64

	StepX, StepY int
	MajorSteps   int
}

func signFloatFromInt(v int) float64 {
	if v < 0 {
		return -1
	}
	if v > 0 {
		return 1
	}
	return 0
}

// strictTanBalancedPhase returns the fixed sub-pixel phase that centres the
// ideal vector through one complete staircase motif.  For tan 1/3, for example,
// floor(k/3)-k/3 over k=0,1,2 is 0,-1/3,-2/3.  Shifting the ideal line by -1/3
// turns the residuals into +1/3,0,-1/3, so AA contribution is balanced on the
// two sides.  The phase depends only on the reduced ratio and direction, never
// on the current drag length, eliminating the length-dependent up/down wobble.
func strictTanBalancedPhase(stepX, stepY int) (float64, float64) {
	ax, ay := absCoreInt(stepX), absCoreInt(stepY)
	if ax == 0 && ay == 0 {
		return 0, 0
	}
	major := ax
	if ay > major {
		major = ay
	}
	if major <= 1 {
		return 0, 0
	}
	phase := -float64(major-1) / (2.0 * float64(major))
	if ax >= ay {
		if ay == 0 {
			return 0, 0
		}
		return 0, signFloatFromInt(stepY) * phase
	}
	if ax == 0 {
		return 0, 0
	}
	return signFloatFromInt(stepX) * phase, 0
}

// calculateStrictTanShadowSegment builds the length-and-slope vector requested
// by the user.  majorSteps is the exact number of dominant-axis raster advances
// used by the integer staircase.  Therefore extending the line changes only
// Length/X1/Y1; the start phase and direction remain fixed.
func calculateStrictTanShadowSegment(x0, y0, stepX, stepY, majorSteps, lineSize int) (strictTanShadowSegment, bool) {
	ax, ay := absCoreInt(stepX), absCoreInt(stepY)
	if (ax == 0 && ay == 0) || majorSteps <= 0 {
		return strictTanShadowSegment{}, false
	}
	major := ax
	if ay > major {
		major = ay
	}
	if major <= 0 {
		return strictTanShadowSegment{}, false
	}

	norm := math.Hypot(float64(stepX), float64(stepY))
	if norm <= 1e-12 {
		return strictTanShadowSegment{}, false
	}
	ux, uy := float64(stepX)/norm, float64(stepY)/norm
	parityX, parityY := arrowStrokeOffset(x0, y0, x0+stepX, y0+stepY, lineSize)
	phaseX, phaseY := strictTanBalancedPhase(stepX, stepY)

	length := float64(majorSteps) * norm / float64(major)
	sx := float64(x0) + parityX + phaseX
	sy := float64(y0) + parityY + phaseY
	return strictTanShadowSegment{
		X0: sx, Y0: sy,
		X1:    sx + ux*length,
		Y1:    sy + uy*length,
		UnitX: ux, UnitY: uy, Length: length,
		ParityOffsetX: parityX, ParityOffsetY: parityY,
		PhaseOffsetX: phaseX, PhaseOffsetY: phaseY,
		StepX: stepX, StepY: stepY, MajorSteps: majorSteps,
	}, true
}

// containsRasterCentre keeps the opaque staircase inside one pair of global
// butt caps perpendicular to the selected tan direction.  Without this clip,
// each one-pixel microsegment contributes its own square cap and the exposed
// first/last caps appear as several parallel black feathers.
func (s strictTanShadowSegment) containsRasterCentre(x, y float64) bool {
	if s.Length <= 0 {
		return false
	}
	forward := (x-s.X0)*s.UnitX + (y-s.Y0)*s.UnitY

	// The visible raster pixel is a unit square, not a dimensionless centre.
	// Its projection radius on the tan direction is
	// 0.5*(|ux|+|uy|). Keeping the whole square between the global butt-cap
	// planes removes the last isolated endpoint feathers that survived A109's
	// centre-only clip. The logical stroke still spans the familiar half-pixel
	// cap range [-0.5, Length+0.5].
	pixelProjectionRadius := 0.5 * (math.Abs(s.UnitX) + math.Abs(s.UnitY))
	lower := -0.5 + pixelProjectionRadius
	upper := s.Length + 0.5 - pixelProjectionRadius
	if upper < lower {
		mid := s.Length * 0.5
		return math.Abs(forward-mid) <= 0.5+1e-9
	}
	return forward >= lower-1e-9 && forward <= upper+1e-9
}

func (s strictTanShadowSegment) prefix(length float64) strictTanShadowSegment {
	if length < 0 {
		length = 0
	}
	if length > s.Length {
		length = s.Length
	}
	s.X1 = s.X0 + s.UnitX*length
	s.Y1 = s.Y0 + s.UnitY*length
	s.Length = length
	return s
}
