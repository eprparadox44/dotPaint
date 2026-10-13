package main

import (
	"math"
	"testing"
)

func TestStretchSkewDecimalThreePlaces(t *testing.T) {
	valid := map[string]float64{"100": 100, "100.123": 100.123, "0.001": 0.001, "500": 500, " 50,125 ": 50.125, "-89.000": -89}
	for text, want := range valid {
		got, ok := stretchSkewParseDecimal(text, -89, 500)
		if !ok || got != want {
			t.Fatalf("%q: got %v,%v want %v", text, got, ok, want)
		}
	}
	for _, text := range []string{"", "0.0001", "1.2345", "nan", "Inf", "Infinity", "1e3", "501", "--3"} {
		_, ok := stretchSkewParseDecimal(text, .001, 500)
		if ok {
			t.Errorf("should reject %q", text)
		}
	}
}

func TestStretchSkewPixelPercentageRoundTrip(t *testing.T) {
	sources := []int{1, 2, 3, 17, 100, 1681, 884, 3276, 8192, 16384}
	for _, src := range sources {
		for _, px := range []int{1, 2, 3, 5, 10, 57, 100, 321, 500, 884, 1681, 4096, 8192, 16384} {
			if px > 16384 || float64(px) > float64(src)*5 {
				continue
			}
			p := stretchSkewPercentForPixels(src, px)
			if p < .001 || p > 500 {
				continue
			}
			got := stretchSkewScaledPixels(src, p)
			if got != px {
				t.Fatalf("%d px from %d at %.3f%% => %d px", px, src, p, got)
			}
		}
	}
	for px := 1; px <= 8405; px++ { // all possible heights for source=1681 at <=500%
		p := stretchSkewPercentForPixels(1681, px)
		if got := stretchSkewScaledPixels(1681, p); got != px {
			t.Fatalf("roundtrip of %d: %f => %d", px, p, got)
		}
	}
}

func TestStretchSkewDimensionsAndFractionalStretch(t *testing.T) {
	cases := []struct {
		w, h         int
		x, y, sh, sv float64
		wantW, wantH int
	}{
		{1681, 884, 100, 100, 0, 0, 1681, 884},
		{1681, 884, 50, 50, 0, 0, 841, 442},
		{1681, 884, 0.5, 0.125, 0, 0, 8, 1},
		{1681, 884, 0.001, 0.001, 0, 0, 1, 1},
		{1681, 884, 200, 100, 0, 0, 3362, 884},
	}
	for _, tc := range cases {
		o := StretchSkewOptions{StretchHorizontal: tc.x, StretchVertical: tc.y, SkewHorizontal: tc.sh, SkewVertical: tc.sv}
		_, _, w, h, ok := stretchSkewGeometry(tc.w, tc.h, o)
		if !ok || w != tc.wantW || h != tc.wantH {
			t.Fatalf("case %+v => %d x %d (ok %t)", tc, w, h, ok)
		}
	}
	if _, _, _, _, ok := stretchSkewGeometry(1681, 884, StretchSkewOptions{StretchHorizontal: .0001, StretchVertical: 100}); ok {
		t.Fatal("invalid stretch should fail")
	}
	if _, _, _, _, ok := stretchSkewGeometry(1681, 884, StretchSkewOptions{StretchHorizontal: 100, StretchVertical: 100, SkewHorizontal: math.NaN()}); ok {
		t.Fatal("NaN skew should fail")
	}
	_, _, w, h, ok := stretchSkewGeometry(1681, 884, StretchSkewOptions{StretchHorizontal: 100, StretchVertical: 100, SkewHorizontal: 15.125})
	if !ok || w <= 1681 || h != 884 {
		t.Fatalf("shear preview failed %d x %d", w, h)
	}
}
