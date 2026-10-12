package main

import (
	"math"
	"testing"
)

func TestA97TanOneThirdPhaseBalancesOneMotif(t *testing.T) {
	px, py := strictTanBalancedPhase(3, 1)
	if math.Abs(px) > 1e-12 || math.Abs(py+1.0/3.0) > 1e-12 {
		t.Fatalf("phase=(%.12f,%.12f), want (0,-1/3)", px, py)
	}
	residuals := make([]float64, 0, 3)
	for k := 0; k < 3; k++ {
		actual := float64((k * 1) / 3)
		ideal := float64(k)/3.0 + py
		residuals = append(residuals, actual-ideal)
	}
	want := []float64{1.0 / 3.0, 0, -1.0 / 3.0}
	for i := range want {
		if math.Abs(residuals[i]-want[i]) > 1e-12 {
			t.Fatalf("residual[%d]=%.12f want %.12f", i, residuals[i], want[i])
		}
	}
}

func TestA97ShadowStartPhaseDoesNotWobbleWithLength(t *testing.T) {
	short, ok := calculateStrictTanShadowSegment(10, 20, 4, 1, 5, 1)
	if !ok {
		t.Fatal("short segment not created")
	}
	long, ok := calculateStrictTanShadowSegment(10, 20, 4, 1, 57, 1)
	if !ok {
		t.Fatal("long segment not created")
	}
	if short.X0 != long.X0 || short.Y0 != long.Y0 {
		t.Fatalf("start phase changed with length: short=(%v,%v) long=(%v,%v)", short.X0, short.Y0, long.X0, long.Y0)
	}
	if math.Abs(short.UnitX-long.UnitX) > 1e-12 || math.Abs(short.UnitY-long.UnitY) > 1e-12 {
		t.Fatalf("direction changed with length")
	}
}

func TestA97ShadowUsesExactSlopeAndDominantLength(t *testing.T) {
	s, ok := calculateStrictTanShadowSegment(0, 0, 3, 1, 17, 1)
	if !ok {
		t.Fatal("segment not created")
	}
	dx, dy := s.X1-s.X0, s.Y1-s.Y0
	if math.Abs(dy/dx-1.0/3.0) > 1e-12 {
		t.Fatalf("slope=%v want 1/3", dy/dx)
	}
	if math.Abs(dx-17) > 1e-12 {
		t.Fatalf("dominant displacement=%v want 17", dx)
	}
}

func TestA97SteepPhaseBalancesOnHorizontalMinorAxis(t *testing.T) {
	px, py := strictTanBalancedPhase(-1, 4)
	if math.Abs(px-3.0/8.0) > 1e-12 || math.Abs(py) > 1e-12 {
		t.Fatalf("phase=(%.12f,%.12f), want (+3/8,0)", px, py)
	}
}

func TestA97PrefixKeepsSameStartAndDirection(t *testing.T) {
	s, ok := calculateStrictTanShadowSegment(3, 7, -3, 2, 30, 4)
	if !ok {
		t.Fatal("segment not created")
	}
	p := s.prefix(s.Length * 0.4)
	if p.X0 != s.X0 || p.Y0 != s.Y0 || p.UnitX != s.UnitX || p.UnitY != s.UnitY {
		t.Fatalf("prefix changed anchor/direction")
	}
	if math.Abs(p.Length-s.Length*0.4) > 1e-12 {
		t.Fatalf("prefix length=%v", p.Length)
	}
}

func TestA97AllSupportedRatiosHaveZeroMeanBalancedPhase(t *testing.T) {
	ratios := [][2]int{{5, 1}, {4, 1}, {3, 1}, {2, 1}, {3, 2}, {1, 1}, {2, 3}, {1, 2}, {1, 3}, {1, 4}, {1, 5}}
	for _, r := range ratios {
		stepX, stepY := r[0], r[1]
		ax, ay := absCoreInt(stepX), absCoreInt(stepY)
		major, minor := ax, ay
		minorIsY := true
		if ay > ax {
			major, minor = ay, ax
			minorIsY = false
		}
		px, py := strictTanBalancedPhase(stepX, stepY)
		phase := px
		if minorIsY {
			phase = py
		}
		sum := 0.0
		minR, maxR := math.Inf(1), math.Inf(-1)
		for k := 0; k < major; k++ {
			actual := float64((k * minor) / major)
			ideal := float64(k*minor)/float64(major) + phase
			res := actual - ideal
			sum += res
			minR = math.Min(minR, res)
			maxR = math.Max(maxR, res)
		}
		if math.Abs(sum) > 1e-12 {
			t.Fatalf("ratio %d/%d residual sum=%v", stepY, stepX, sum)
		}
		if math.Abs(minR+maxR) > 1e-12 {
			t.Fatalf("ratio %d/%d residual range not symmetric: [%v,%v]", stepY, stepX, minR, maxR)
		}
	}
}

func TestA97DirectedArrowHeadSharesShadowPhaseAndSlope(t *testing.T) {
	s, ok := calculateStrictTanShadowSegment(10, 10, 3, 1, 30, 2)
	if !ok {
		t.Fatal("shadow not created")
	}
	g, ok := calculateArrowHeadDirected(40, 20, s.UnitX, s.UnitY, s.Length, 0, 2, s.PhaseOffsetX, s.PhaseOffsetY)
	if !ok {
		t.Fatal("head not created")
	}
	if math.Abs(g.UnitX-s.UnitX) > 1e-12 || math.Abs(g.UnitY-s.UnitY) > 1e-12 {
		t.Fatalf("head direction differs from shadow")
	}
	wantTipX := 40.0 + g.PixelOffsetX + s.PhaseOffsetX
	wantTipY := 20.0 + g.PixelOffsetY + s.PhaseOffsetY
	if math.Abs(g.Tip.X-wantTipX) > 1e-12 || math.Abs(g.Tip.Y-wantTipY) > 1e-12 {
		t.Fatalf("head tip phase mismatch got=(%v,%v) want=(%v,%v)", g.Tip.X, g.Tip.Y, wantTipX, wantTipY)
	}
}

func TestA110GlobalButtCapsContainWholeRasterSquares(t *testing.T) {
	s, ok := calculateStrictTanShadowSegment(20, 20, 3, -1, 60, 12)
	if !ok {
		t.Fatal("shadow not created")
	}
	radius := 0.5 * (math.Abs(s.UnitX) + math.Abs(s.UnitY))
	lowerPlane := -0.5
	upperPlane := s.Length + 0.5
	accepted := 0
	for y := int(math.Floor(math.Min(s.Y0, s.Y1))) - 20; y <= int(math.Ceil(math.Max(s.Y0, s.Y1)))+20; y++ {
		for x := int(math.Floor(math.Min(s.X0, s.X1))) - 20; x <= int(math.Ceil(math.Max(s.X0, s.X1)))+20; x++ {
			if !s.containsRasterCentre(float64(x), float64(y)) {
				continue
			}
			accepted++
			forward := (float64(x)-s.X0)*s.UnitX + (float64(y)-s.Y0)*s.UnitY
			if forward-radius < lowerPlane-1e-9 || forward+radius > upperPlane+1e-9 {
				t.Fatalf("accepted pixel square crosses a butt cap at %d,%d forward=%v radius=%v", x, y, forward, radius)
			}
		}
	}
	if accepted == 0 {
		t.Fatal("all raster centres were rejected")
	}
	if s.containsRasterCentre(s.X0-s.UnitX*2, s.Y0-s.UnitY*2) {
		t.Fatal("tail feather was not clipped")
	}
	if s.containsRasterCentre(s.X1+s.UnitX*2, s.Y1+s.UnitY*2) {
		t.Fatal("head feather was not clipped")
	}
}
