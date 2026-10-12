package main

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func rasterRowsAtX(g ArrowHeadGeometry, x, yMin, yMax int) []int {
	rows := make([]int, 0)
	for y := yMin; y <= yMax; y++ {
		if pointInsideArrowHead(ArrowPoint{X: float64(x), Y: float64(y)}, g) {
			rows = append(rows, y)
		}
	}
	return rows
}

func TestArrowHeadSizesIncludeUploadedCompactTwelveByNine(t *testing.T) {
	wantWings := []float64{4, 5, 7, 9}
	if len(arrowHeadWingSteps) != len(wantWings) {
		t.Fatalf("choices=%d want %d", len(arrowHeadWingSteps), len(wantWings))
	}
	for i, want := range wantWings {
		if !near(arrowHeadWingSteps[i], want) {
			t.Fatalf("choice %d wing=%v want %v", i, arrowHeadWingSteps[i], want)
		}
	}
	g, ok := calculateArrowHead(0, 10, 100, 10, 0, 3)
	if !ok {
		t.Fatal("expected compact geometry")
	}
	if !near(g.Length, 12) {
		t.Fatalf("compact length=%v want 12", g.Length)
	}
	rows := rasterRowsAtX(g, int(math.Round(g.Base.X)), 0, 20)
	if len(rows) != 9 || rows[0] != 6 || rows[len(rows)-1] != 14 {
		t.Fatalf("compact base rows=%v want 6..14 (9 pixels)", rows)
	}
}

func TestArrowHeadUsesOneToThreeSides(t *testing.T) {
	g, ok := calculateArrowHead(0, 0, 100, 0, 2, 3)
	if !ok {
		t.Fatal("expected geometry")
	}
	if !near(g.Length, 21) {
		t.Fatalf("length=%v want 21", g.Length)
	}
	if !near(g.BaseHalfWidth-g.TipHalfWidth, 7) {
		t.Fatalf("wing growth=%v want 7", g.BaseHalfWidth-g.TipHalfWidth)
	}
	if !near(g.Length/(g.BaseHalfWidth-g.TipHalfWidth), 3) {
		t.Fatalf("run:rise=%v want 3", g.Length/(g.BaseHalfWidth-g.TipHalfWidth))
	}
}

func TestArrowHeadOddWidthHasOnePixelTipAndExactSymmetry(t *testing.T) {
	g, ok := calculateArrowHead(0, 10, 100, 10, 0, 3)
	if !ok {
		t.Fatal("expected geometry")
	}
	rows := rasterRowsAtX(g, 100, 0, 20)
	if len(rows) != 1 || rows[0] != 10 {
		t.Fatalf("odd tip rows=%v want [10]", rows)
	}
	baseX := int(math.Round(g.Base.X))
	baseRows := rasterRowsAtX(g, baseX, 0, 20)
	if len(baseRows) != 9 || baseRows[0] != 6 || baseRows[len(baseRows)-1] != 14 {
		t.Fatalf("odd base rows=%v want 6..14", baseRows)
	}
	for _, y := range baseRows {
		mirror := 20 - y
		found := false
		for _, yy := range baseRows {
			if yy == mirror {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("row %d lacks mirror %d in %v", y, mirror, baseRows)
		}
	}
}

func TestArrowHeadEvenWidthUsesUpperTwoPixelTipAndExactSymmetry(t *testing.T) {
	g, ok := calculateArrowHead(0, 10, 100, 10, 0, 2)
	if !ok {
		t.Fatal("expected geometry")
	}
	if !near(g.PixelOffsetX, 0) || !near(g.PixelOffsetY, -0.5) {
		t.Fatalf("even horizontal offset=(%v,%v) want (0,-0.5)", g.PixelOffsetX, g.PixelOffsetY)
	}
	rows := rasterRowsAtX(g, 100, 0, 22)
	if len(rows) != 2 || rows[0] != 9 || rows[1] != 10 {
		t.Fatalf("even tip rows=%v want [9 10]", rows)
	}
	baseX := int(math.Ceil(g.Base.X))
	baseRows := rasterRowsAtX(g, baseX, 0, 22)
	if len(baseRows) != 10 || baseRows[0] != 5 || baseRows[len(baseRows)-1] != 14 {
		t.Fatalf("even base rows=%v want 5..14", baseRows)
	}
	for _, y := range baseRows {
		mirror := 19 - y // exact symmetry around y=9.5
		found := false
		for _, yy := range baseRows {
			if yy == mirror {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("row %d lacks mirror %d in %v", y, mirror, baseRows)
		}
	}
}

func TestArrowEvenOffsetIsDirectionIndependentAndUpperOrLeft(t *testing.T) {
	cases := []struct {
		x0, y0, x1, y1 int
		wantX, wantY   float64
	}{
		{0, 10, 100, 10, 0, -0.5},
		{100, 10, 0, 10, 0, -0.5},
		{10, 0, 10, 100, -0.5, 0},
		{10, 100, 10, 0, -0.5, 0},
	}
	for _, tc := range cases {
		ox, oy := arrowStrokeOffset(tc.x0, tc.y0, tc.x1, tc.y1, 2)
		if !near(ox, tc.wantX) || !near(oy, tc.wantY) {
			t.Fatalf("(%d,%d)->(%d,%d) offset=(%v,%v) want (%v,%v)", tc.x0, tc.y0, tc.x1, tc.y1, ox, oy, tc.wantX, tc.wantY)
		}
	}
}

func TestArrowEvenAAHorizontalStrokeKeepsExactIntegerThickness(t *testing.T) {
	for _, size := range []int{2, 4, 6, 8} {
		ox, oy := arrowStrokeOffset(0, 10, 100, 10, size)
		full := 0
		partial := 0
		for y := 0; y <= 20; y++ {
			cov := arrowStrokeCoverageAt(50, float64(y), ox, 10+oy, 100+ox, 10+oy, size)
			if near(cov, 1) {
				full++
			} else if cov > 0 {
				partial++
			}
		}
		if full != size || partial != 0 {
			t.Fatalf("size=%d full=%d partial=%d want full=%d partial=0", size, full, partial, size)
		}
	}
}

func TestArrowOddAAHorizontalStrokeKeepsExactIntegerThickness(t *testing.T) {
	for _, size := range []int{1, 3, 5, 7} {
		ox, oy := arrowStrokeOffset(0, 10, 100, 10, size)
		full := 0
		partial := 0
		for y := 0; y <= 20; y++ {
			cov := arrowStrokeCoverageAt(50, float64(y), ox, 10+oy, 100+ox, 10+oy, size)
			if near(cov, 1) {
				full++
			} else if cov > 0 {
				partial++
			}
		}
		if full != size || partial != 0 {
			t.Fatalf("size=%d full=%d partial=%d want full=%d partial=0", size, full, partial, size)
		}
	}
}

func TestArrowHeadEvenTipKeepsEndpointInReverseDirections(t *testing.T) {
	left, ok := calculateArrowHead(100, 10, 20, 10, 2, 4)
	if !ok {
		t.Fatal("expected left geometry")
	}
	leftRows := rasterRowsAtX(left, 20, 0, 22)
	if len(leftRows) != 2 || leftRows[0] != 9 || leftRows[1] != 10 {
		t.Fatalf("left even tip rows=%v want [9 10]", leftRows)
	}

	up, ok := calculateArrowHead(10, 100, 10, 20, 2, 4)
	if !ok {
		t.Fatal("expected up geometry")
	}
	cols := make([]int, 0)
	for x := 0; x <= 22; x++ {
		if pointInsideArrowHead(ArrowPoint{X: float64(x), Y: 20}, up) {
			cols = append(cols, x)
		}
	}
	if len(cols) != 2 || cols[0] != 9 || cols[1] != 10 {
		t.Fatalf("up even tip cols=%v want [9 10]", cols)
	}
}

func TestArrowShaftStopsAtRearEdge(t *testing.T) {
	g, ok := calculateArrowHead(0, 0, 100, 0, 1, 2)
	if !ok {
		t.Fatal("expected geometry")
	}
	x, y := arrowShaftLogicalEnd(g)
	if x != 85 || y != 0 {
		t.Fatalf("shaft end=(%d,%d) want (85,0)", x, y)
	}
}

func TestArrowHeadRotatesWithDirection(t *testing.T) {
	g, ok := calculateArrowHead(10, 20, 10, 0, 0, 3)
	if !ok {
		t.Fatal("expected geometry")
	}
	if !near(g.Tip.X, 10) || !near(g.Tip.Y, 0) {
		t.Fatalf("tip=%+v", g.Tip)
	}
	if !near(g.SideA.Y, g.SideB.Y) {
		t.Fatalf("rear edge not horizontal: %+v", g)
	}
}

func TestArrowHeadScalesForShortSegmentWithoutLosingTipParity(t *testing.T) {
	g, ok := calculateArrowHead(0, 0, 4, 0, 3, 8)
	if !ok {
		t.Fatal("expected geometry")
	}
	if g.Base.X < -1e-9 {
		t.Fatalf("head extended behind start: %+v", g)
	}
	rows := rasterRowsAtX(g, 4, -5, 8)
	if len(rows) != 2 || rows[0] != -1 || rows[1] != 0 {
		t.Fatalf("short even tip rows=%v want [-1 0]", rows)
	}
}

func TestArrowDirectionAngles(t *testing.T) {
	cases := []struct {
		x1, y1 int
		want   float64
	}{{10, 0, 0}, {0, -10, 90}, {-10, 0, 180}, {0, 10, -90}}
	for _, tc := range cases {
		got := arrowDirectionAngleDegrees(0, 0, tc.x1, tc.y1)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Fatalf("(%d,%d): got %v want %v", tc.x1, tc.y1, got, tc.want)
		}
	}
}

func TestArrowSquareShaftHasFlatTailAndRearCaps(t *testing.T) {
	// A former round distance-to-segment cap included the diagonal corner
	// immediately behind a 3 px horizontal tail. The square/butt cap must not.
	if pointInsideArrowStroke(9, 9, 10, 10, 20, 10, 3) {
		t.Fatal("square tail leaked into the diagonal pixel behind the start")
	}
	if !pointInsideArrowStroke(10, 9, 10, 10, 20, 10, 3) {
		t.Fatal("first tail column should be fully present")
	}
	if !pointInsideArrowStroke(20, 11, 10, 10, 20, 10, 3) {
		t.Fatal("last rear column should be fully present")
	}
	if pointInsideArrowStroke(21, 10, 10, 10, 20, 10, 3) {
		t.Fatal("shaft extended beyond its flat rear cap")
	}
}

func TestArrowSquareShaftAACapsAreFullThenZero(t *testing.T) {
	if got := arrowStrokeCoverageAt(10, 10, 10, 10, 20, 10, 2); !near(got, 1) {
		t.Fatalf("tail coverage=%v want 1", got)
	}
	if got := arrowStrokeCoverageAt(9, 10, 10, 10, 20, 10, 2); !near(got, 0) {
		t.Fatalf("pixel behind tail coverage=%v want 0", got)
	}
	if got := arrowStrokeCoverageAt(20, 10, 10, 10, 20, 10, 2); !near(got, 1) {
		t.Fatalf("rear coverage=%v want 1", got)
	}
	if got := arrowStrokeCoverageAt(21, 10, 10, 10, 20, 10, 2); !near(got, 0) {
		t.Fatalf("pixel beyond rear coverage=%v want 0", got)
	}
}

func TestArrowEightWayDirectionsForceCrispRaster(t *testing.T) {
	crisp := [][4]int{
		{0, 0, 10, 0}, {0, 0, -10, 0}, {0, 0, 0, 10}, {0, 0, 0, -10},
		{0, 0, 10, 10}, {0, 0, 10, -10}, {0, 0, -10, 10}, {0, 0, -10, -10},
		// One-pixel endpoint rounding is accepted for constrained 45-degree drags.
		{0, 0, 11, 10},
	}
	for _, c := range crisp {
		if !arrowDirectionIsCrispEightWay(c[0], c[1], c[2], c[3]) {
			t.Fatalf("direction %v should be crisp", c)
		}
		if arrowEffectiveAntialias(true, c[0], c[1], c[2], c[3]) {
			t.Fatalf("direction %v should bypass AA", c)
		}
	}
	if arrowDirectionIsCrispEightWay(0, 0, 10, 5) {
		t.Fatal("2:1 direction incorrectly classified as eight-way")
	}
	if !arrowEffectiveAntialias(true, 0, 0, 10, 5) {
		t.Fatal("arbitrary slope should retain AA")
	}
	if arrowEffectiveAntialias(false, 0, 0, 10, 5) {
		t.Fatal("disabled AA should remain disabled")
	}
}

func TestArrowHeadHorizontalArrowKeepsCapsCrispButAntialiasesSlopes(t *testing.T) {
	g, ok := calculateArrowHead(0, 20, 100, 20, 0, 3)
	if !ok {
		t.Fatal("expected geometry")
	}
	edges := arrowHeadEdgeAntialias(g, true)
	if edges.TipCap || edges.RearBase {
		t.Fatalf("horizontal arrow caps must stay crisp: %+v", edges)
	}
	if !edges.SideA || !edges.SideB {
		t.Fatalf("horizontal arrow 1:3 sides must use AA: %+v", edges)
	}
}

func TestArrowHeadTanOneThirdHasOneCrispAndOneAntialiasedSide(t *testing.T) {
	// Shaft direction is tan(theta)=1/3 and the head half-angle is also
	// atan(1/3). Therefore the two side directions are theta-alpha=0 degrees
	// and theta+alpha=2theta, whose tangent is 3/4.
	g, ok := calculateArrowHead(0, 0, 30, 10, 0, 3)
	if !ok {
		t.Fatal("expected geometry")
	}
	edges := arrowHeadEdgeAntialias(g, true)
	if edges.SideA {
		t.Fatalf("SideA should be the horizontal crisp side: %+v", edges)
	}
	if !edges.SideB {
		t.Fatalf("SideB should be the tan(3/4) antialiased side: %+v", edges)
	}
	if !edges.TipCap || !edges.RearBase {
		t.Fatalf("tan(1/3) perpendicular caps have slope -3 and need AA: %+v", edges)
	}

	wing := g.BaseHalfWidth - g.TipHalfWidth
	dxA := -g.UnitX*g.Length + g.PerpX*wing
	dyA := -g.UnitY*g.Length + g.PerpY*wing
	dxB := -g.UnitX*g.Length - g.PerpX*wing
	dyB := -g.UnitY*g.Length - g.PerpY*wing
	if math.Abs(dyA) > 1e-8 {
		t.Fatalf("crisp side is not horizontal: vector=(%v,%v)", dxA, dyA)
	}
	if math.Abs(math.Abs(dyB/dxB)-0.75) > 1e-8 {
		t.Fatalf("AA side slope=%v want |3/4|", dyB/dxB)
	}
}

func TestArrowHeadFortyFiveShaftStillAntialiasesItsNonGridSideEdges(t *testing.T) {
	g, ok := calculateArrowHead(0, 0, 40, 40, 1, 4)
	if !ok {
		t.Fatal("expected geometry")
	}
	edges := arrowHeadEdgeAntialias(g, true)
	if edges.TipCap || edges.RearBase {
		t.Fatalf("45-degree shaft has 45-degree caps, which must stay crisp: %+v", edges)
	}
	if !edges.SideA || !edges.SideB {
		t.Fatalf("45-degree shaft does not make its 1:3 head sides 45 degrees: %+v", edges)
	}
}

func TestArrowHeadAAOffLeavesEveryEdgeBinary(t *testing.T) {
	g, ok := calculateArrowHead(0, 0, 30, 10, 0, 3)
	if !ok {
		t.Fatal("expected geometry")
	}
	if edges := arrowHeadEdgeAntialias(g, false); edges.any() {
		t.Fatalf("AA off returned active edges: %+v", edges)
	}
	for y := -20; y <= 20; y++ {
		for x := 0; x <= 40; x++ {
			cov := arrowHeadCoverageAt(float64(x), float64(y), g, false)
			if cov != 0 && cov != 1 {
				t.Fatalf("AA-off partial coverage at %d,%d: %v", x, y, cov)
			}
		}
	}
}

func TestArrowShaftJoinAdvancesInsideHeadAtFortyFiveDegrees(t *testing.T) {
	g, ok := calculateArrowHead(0, 0, 30, 30, 0, 1)
	if !ok {
		t.Fatal("expected geometry")
	}
	baseX, baseY := arrowShaftLogicalEnd(g)
	joinX, joinY := arrowShaftJoinedLogicalEnd(g, 1)
	baseForward := float64(baseX)*g.UnitX + float64(baseY)*g.UnitY
	joinForward := float64(joinX)*g.UnitX + float64(joinY)*g.UnitY
	if joinForward <= baseForward+0.5 {
		t.Fatalf("45-degree join did not advance enough: base=(%d,%d) join=(%d,%d) projections=%v,%v", baseX, baseY, joinX, joinY, baseForward, joinForward)
	}
	if !pointInsideArrowHead(ArrowPoint{X: float64(joinX) + g.PixelOffsetX, Y: float64(joinY) + g.PixelOffsetY}, g) {
		t.Fatalf("joined shaft endpoint is not covered by the head: join=(%d,%d) geometry=%+v", joinX, joinY, g)
	}
}

func TestSharedSquareLineCoreKeepsExactThicknessThroughFortyPixels(t *testing.T) {
	for _, size := range []int{1, 2, 3, 8, 20, 40} {
		ox, oy := arrowStrokeOffset(0, 50, 100, 50, size)
		full, partial := 0, 0
		for y := 20; y <= 80; y++ {
			cov := arrowStrokeCoverageAt(50, float64(y), ox, 50+oy, 100+ox, 50+oy, size)
			if near(cov, 1) {
				full++
			} else if cov > 0 {
				partial++
			}
		}
		if full != size || partial != 0 {
			t.Fatalf("size=%d full=%d partial=%d want full=%d partial=0", size, full, partial, size)
		}
	}
}

func TestSharedLineAABehaviourMatchesArrowEightWayPolicy(t *testing.T) {
	for _, c := range [][4]int{
		{0, 0, 30, 0}, {0, 0, 0, 30}, {0, 0, 30, 30}, {0, 0, -30, 30},
	} {
		if arrowEffectiveAntialias(true, c[0], c[1], c[2], c[3]) {
			t.Fatalf("eight-way line %v should stay crisp", c)
		}
	}
	if !arrowEffectiveAntialias(true, 0, 0, 30, 10) {
		t.Fatal("tan(1/3) line should retain AA")
	}
}

func TestDirectionalAAForDescendingShallowLineStaysBetweenAdjacentRows(t *testing.T) {
	// Ideal line: (1,4) -> (11,6), a shallow down-right slope.  At x=5 the
	// continuous centre lies between rows 4 and 5.  Correct AA may distribute
	// coverage across those two rows, but it must not create the former grey
	// pixel one full row above at y=3.
	if got := arrowStrokeCoverageAt(5, 3, 1, 4, 11, 6, 1); got != 0 {
		t.Fatalf("spurious upper-side coverage=%v want 0", got)
	}
	upper := arrowStrokeCoverageAt(5, 4, 1, 4, 11, 6, 1)
	lower := arrowStrokeCoverageAt(5, 5, 1, 4, 11, 6, 1)
	if !(upper > 0 && upper < 1 && lower > 0 && lower < 1) {
		t.Fatalf("adjacent-row coverage upper=%v lower=%v; want two partial samples", upper, lower)
	}
	if got := arrowStrokeCoverageAt(5, 6, 1, 4, 11, 6, 1); got != 0 {
		t.Fatalf("coverage leaked beyond adjacent rows: %v", got)
	}
}

func TestA96VectorCapsUseSmallSymmetricAreaContribution(t *testing.T) {
	// A96 no longer treats the oblique caps as an unrelated binary centre test.
	// Both ends belong to the same supersampled vector rectangle, so a tiny
	// sub-pixel contribution is allowed but must be equal after a half turn.
	tail := arrowStrokeCoverageAt(-1, 0, 0, 0, 10, 2, 1)
	head := arrowStrokeCoverageAt(11, 2, 0, 0, 10, 2, 1)
	if !near(tail, head) {
		t.Fatalf("cap contribution differs tail=%v head=%v", tail, head)
	}
	if !(tail > 0 && tail < 0.1) {
		t.Fatalf("cap contribution=%v want a small vector-area fraction", tail)
	}
}

func TestA95EvenStrokeOffsetUsesGridAlignedUpperOrLeftAxis(t *testing.T) {
	cases := []struct {
		name   string
		x0, y0 int
		x1, y1 int
		wantX  float64
		wantY  float64
	}{
		{"horizontal", 0, 10, 100, 10, 0, -0.5},
		{"shallow", 0, 10, 100, 30, 0, -0.5},
		{"diagonal", 0, 10, 100, 110, 0, -0.5},
		{"vertical", 10, 0, 10, 100, -0.5, 0},
		{"steep", 10, 0, 30, 100, -0.5, 0},
	}
	for _, tc := range cases {
		ox, oy := arrowStrokeOffset(tc.x0, tc.y0, tc.x1, tc.y1, 4)
		if !near(ox, tc.wantX) || !near(oy, tc.wantY) {
			t.Fatalf("%s offset=(%v,%v) want=(%v,%v)", tc.name, ox, oy, tc.wantX, tc.wantY)
		}
		rx, ry := arrowStrokeOffset(tc.x1, tc.y1, tc.x0, tc.y0, 4)
		if !near(rx, ox) || !near(ry, oy) {
			t.Fatalf("%s reverse offset=(%v,%v) differs from forward=(%v,%v)", tc.name, rx, ry, ox, oy)
		}
	}
}

func TestA95StrokeCoverageIsEndpointOrderInvariant(t *testing.T) {
	for _, size := range []int{1, 2, 4, 7} {
		ox, oy := arrowStrokeOffset(4, 5, 84, 31, size)
		fx0, fy0 := 4.0+ox, 5.0+oy
		fx1, fy1 := 84.0+ox, 31.0+oy
		for y := 0; y <= 38; y++ {
			for x := 0; x <= 90; x++ {
				forward := arrowStrokeCoverageAt(float64(x), float64(y), fx0, fy0, fx1, fy1, size)
				reverse := arrowStrokeCoverageAt(float64(x), float64(y), fx1, fy1, fx0, fy0, size)
				if !near(forward, reverse) {
					t.Fatalf("size=%d pixel=%d,%d forward=%v reverse=%v", size, x, y, forward, reverse)
				}
			}
		}
	}
}

func TestA95ObliqueEvenStrokeHasExactHalfTurnRasterSymmetry(t *testing.T) {
	for _, size := range []int{2, 4, 6, 8} {
		x0, y0, x1, y1 := 4, 5, 84, 31
		ox, oy := arrowStrokeOffset(x0, y0, x1, y1, size)
		fx0, fy0 := float64(x0)+ox, float64(y0)+oy
		fx1, fy1 := float64(x1)+ox, float64(y1)+oy
		// With A95's grid-aligned parity offset, this half-turn maps integer
		// pixel centres to integer pixel centres exactly.
		sumX := x0 + x1 + int(math.Round(2*ox))
		sumY := y0 + y1 + int(math.Round(2*oy))
		for y := -2; y <= 38; y++ {
			for x := -2; x <= 90; x++ {
				mx, my := sumX-x, sumY-y
				a := arrowStrokeCoverageAt(float64(x), float64(y), fx0, fy0, fx1, fy1, size)
				b := arrowStrokeCoverageAt(float64(mx), float64(my), fx0, fy0, fx1, fy1, size)
				if !near(a, b) {
					t.Fatalf("size=%d p=(%d,%d) mirror=(%d,%d) coverage=%v/%v", size, x, y, mx, my, a, b)
				}
			}
		}
	}
}

func TestA96HiddenVectorMaskUsesMaxUnionAndStableAlpha(t *testing.T) {
	m := newVectorAlphaMask(10, 20, 12, 22)
	m.setMaxCoverage(11, 21, 0.25)
	first := m.alphaAt(11, 21)
	m.setMaxCoverage(11, 21, 0.10)
	if got := m.alphaAt(11, 21); got != first {
		t.Fatalf("smaller union contribution changed alpha %d -> %d", first, got)
	}
	m.setMaxCoverage(11, 21, 0.75)
	if got := m.alphaAt(11, 21); got <= first {
		t.Fatalf("larger contribution did not win: first=%d got=%d", first, got)
	}
	m.setOpaque(11, 21)
	if got := m.alphaAt(11, 21); got != 255 {
		t.Fatalf("opaque union alpha=%d want 255", got)
	}
}

func TestA96CompositeArrowUnionCoversFortyFiveDegreeNeck(t *testing.T) {
	g, ok := calculateArrowHead(0, 0, 40, 40, 0, 2)
	if !ok {
		t.Fatal("expected geometry")
	}
	sx, sy := arrowShaftJoinedLogicalEnd(g, 2)
	ox, oy := g.PixelOffsetX, g.PixelOffsetY
	fx0, fy0 := ox, oy
	fx1, fy1 := float64(sx)+ox, float64(sy)+oy
	// Sample native pixels around the rear-centre. At least one full or partial
	// contribution must bridge every local forward slice through the neck.
	for step := -1.0; step <= 1.0; step += 0.5 {
		cx := g.Base.X + g.UnitX*step
		cy := g.Base.Y + g.UnitY*step
		found := false
		for y := int(math.Floor(cy)) - 2; y <= int(math.Ceil(cy))+2; y++ {
			for x := int(math.Floor(cx)) - 2; x <= int(math.Ceil(cx))+2; x++ {
				cov := arrowCompositeCoverageAt(float64(x), float64(y), true, fx0, fy0, fx1, fy1, 2, false, g, true)
				if cov > 0 {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("no union coverage near neck step=%v", step)
		}
	}
}

func TestA96CompositeCoverageIsEndpointDirectionIndependent(t *testing.T) {
	g, ok := calculateArrowHead(4, 5, 84, 31, 0, 3)
	if !ok {
		t.Fatal("expected geometry")
	}
	sx, sy := arrowShaftJoinedLogicalEnd(g, 3)
	fx0, fy0 := 4.0+g.PixelOffsetX, 5.0+g.PixelOffsetY
	fx1, fy1 := float64(sx)+g.PixelOffsetX, float64(sy)+g.PixelOffsetY
	for y := 0; y <= 40; y++ {
		for x := 0; x <= 90; x++ {
			a := arrowCompositeCoverageAt(float64(x), float64(y), true, fx0, fy0, fx1, fy1, 3, true, g, true)
			b := arrowCompositeCoverageAt(float64(x), float64(y), true, fx1, fy1, fx0, fy0, 3, true, g, true)
			if !near(a, b) {
				t.Fatalf("pixel=%d,%d forward=%v reverse=%v", x, y, a, b)
			}
		}
	}
}

func TestA109StrictTanLongEdgeAACutsOffCapFeathers(t *testing.T) {
	// Coverage is allowed beside the middle of the stroke, but not in either
	// cap zone. The opaque staircase itself owns those flat ends.
	mid := arrowStrokeLongEdgesCoverageAt(10, 4, 0, 0, 20, 20.0/3.0, 6)
	if mid <= 0 {
		t.Fatalf("expected long-edge coverage in the middle, got %v", mid)
	}
	if got := arrowStrokeLongEdgesCoverageAt(0, 0, 0, 0, 20, 20.0/3.0, 6); got != 0 {
		t.Fatalf("start cap coverage=%v, want 0", got)
	}
	if got := arrowStrokeLongEdgesCoverageAt(20, 20.0/3.0, 0, 0, 20, 20.0/3.0, 6); got != 0 {
		t.Fatalf("end cap coverage=%v, want 0", got)
	}
}
