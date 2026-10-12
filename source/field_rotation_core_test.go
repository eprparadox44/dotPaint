package main

import (
	"math"
	"testing"
)

func TestFieldRotationControlsSnapToFiveDegrees130(t *testing.T) {
	cases := []struct {
		degrees float64
		want    int
	}{
		{0, 0}, {2.4, 0}, {2.6, 5}, {87.6, 90}, {92.4, 90},
		{179, 180}, {181, 180}, {183, -175}, {-87.6, -90}, {-181, -180}, {359, 0},
	}
	for _, tc := range cases {
		if got := fieldSnapRotationDegrees(tc.degrees); got != tc.want {
			t.Fatalf("snap(%g)=%d want=%d", tc.degrees, got, tc.want)
		}
	}
}

func TestFieldRotationSliderUsesFiveDegreeUnits130(t *testing.T) {
	for _, degrees := range []int{-180, -90, -5, 0, 5, 90, 180} {
		position := degrees / fieldRotationStepDegrees
		radians := fieldRotationRadiansFromSlider(position)
		if got := fieldRotationSliderPosition(radians); got != position {
			t.Fatalf("roundtrip degrees=%d position=%d got=%d", degrees, position, got)
		}
		if math.Abs(radians-float64(degrees)*math.Pi/180) > 1e-12 {
			t.Fatalf("radians mismatch degrees=%d got=%g", degrees, radians)
		}
	}
}
