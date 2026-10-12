package main

import "math"

const (
	fieldRotationStepDegrees = 5
	fieldRotationSliderHalf  = 180 / fieldRotationStepDegrees
)

// fieldNormaliseSignedDegrees maps any angle to the control's canonical
// -180..+180 range.  Both endpoints are retained so the user can deliberately
// land on either signed half-turn while every other orientation has one label.
func fieldNormaliseSignedDegrees(degrees int) int {
	for degrees > 180 {
		degrees -= 360
	}
	for degrees < -180 {
		degrees += 360
	}
	return degrees
}

func fieldSnapRotationDegrees(degrees float64) int {
	if math.IsNaN(degrees) || math.IsInf(degrees, 0) {
		return 0
	}
	steps := int(math.Round(degrees / fieldRotationStepDegrees))
	return fieldNormaliseSignedDegrees(steps * fieldRotationStepDegrees)
}

func fieldRotationSliderPosition(radians float64) int {
	degrees := fieldSnapRotationDegrees(radians * 180 / math.Pi)
	return degrees / fieldRotationStepDegrees
}

func fieldRotationDegreesFromSlider(position int) int {
	position = max(-fieldRotationSliderHalf, min(fieldRotationSliderHalf, position))
	return position * fieldRotationStepDegrees
}

func fieldRotationRadiansFromSlider(position int) float64 {
	return float64(fieldRotationDegreesFromSlider(position)) * math.Pi / 180
}

func fieldSnapRotationRadians(radians float64) float64 {
	return fieldRotationRadiansFromSlider(fieldRotationSliderPosition(radians))
}
