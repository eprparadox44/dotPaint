package main

import (
	"math"
	"testing"
)

func graphEvalForTest(t *testing.T, source string, x float64, mode GraphAngleMode) float64 {
	t.Helper()
	n, err := parseGraphExpression(source)
	if err != nil {
		t.Fatalf("parse %q: %v", source, err)
	}
	return evalGraphExpression(n, x, mode)
}

func TestGraphPolynomialAndImplicitMultiplication(t *testing.T) {
	got := graphEvalForTest(t, "(x+1)(x-1)+2x^5-3", 2, GraphRadians)
	want := (2.0+1)*(2.0-1) + 2*math.Pow(2, 5) - 3
	if math.Abs(got-want) > 1e-10 {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestGraphExponentAssociativityAndUnary(t *testing.T) {
	if got := graphEvalForTest(t, "2^3^2", 0, GraphRadians); got != 512 {
		t.Fatalf("right associative exponent got %v", got)
	}
	if got := graphEvalForTest(t, "-2^2", 0, GraphRadians); got != -4 {
		t.Fatalf("unary precedence got %v", got)
	}
	if got := graphEvalForTest(t, "2^-2", 0, GraphRadians); math.Abs(got-0.25) > 1e-12 {
		t.Fatalf("negative exponent got %v", got)
	}
}

func TestGraphTrigRadianAndDegree(t *testing.T) {
	if got := graphEvalForTest(t, "sin(pi/2)+cos(0)", 0, GraphRadians); math.Abs(got-2) > 1e-12 {
		t.Fatalf("radian got %v", got)
	}
	if got := graphEvalForTest(t, "sin(90)+cos(180)", 0, GraphDegrees); math.Abs(got) > 1e-12 {
		t.Fatalf("degree got %v", got)
	}
}

func TestGraphAllTwelveTrigFunctions(t *testing.T) {
	direct := []string{"sin(0.4)", "cos(0.4)", "tan(0.4)", "cot(0.4)", "sec(0.4)", "csc(0.4)"}
	inverse := []string{"asin(0.4)", "acos(0.4)", "atan(0.4)", "acot(2)", "asec(2)", "acsc(2)"}
	for _, source := range append(direct, inverse...) {
		got := graphEvalForTest(t, source, 0, GraphRadians)
		if !isFiniteGraphValue(got) {
			t.Fatalf("%s returned %v", source, got)
		}
	}
}

func TestGraphExponentialLogConstants(t *testing.T) {
	got := graphEvalForTest(t, "ln(e)+log(100)+exp(0)+pi-pi", 0, GraphRadians)
	if math.Abs(got-4) > 1e-10 {
		t.Fatalf("got %v want 4", got)
	}
}

func TestGraphFactorialPostfixAndFunction(t *testing.T) {
	if got := graphEvalForTest(t, "5!", 0, GraphRadians); math.Abs(got-120) > 1e-10 {
		t.Fatalf("5! got %v want 120", got)
	}
	if got := graphEvalForTest(t, "fact(5)+factorial(0)", 0, GraphRadians); math.Abs(got-121) > 1e-10 {
		t.Fatalf("fact aliases got %v want 121", got)
	}
	if got := graphEvalForTest(t, "2^3!", 0, GraphRadians); math.Abs(got-64) > 1e-10 {
		t.Fatalf("factorial precedence got %v want 64", got)
	}
	if got := graphEvalForTest(t, "3!^2", 0, GraphRadians); math.Abs(got-36) > 1e-10 {
		t.Fatalf("factorial/base precedence got %v want 36", got)
	}
	if got := graphEvalForTest(t, "(-0.5)!", 0, GraphRadians); math.Abs(got-math.Sqrt(math.Pi)) > 1e-10 {
		t.Fatalf("gamma extension got %v want sqrt(pi)", got)
	}
	if got := graphEvalForTest(t, "(-1)!", 0, GraphRadians); !math.IsNaN(got) {
		t.Fatalf("negative integer factorial should be undefined, got %v", got)
	}
}

func TestGraphDomainProducesGap(t *testing.T) {
	if got := graphEvalForTest(t, "ln(x)", -1, GraphRadians); !math.IsNaN(got) {
		t.Fatalf("ln domain got %v", got)
	}
	if got := graphEvalForTest(t, "asin(2)", 0, GraphRadians); !math.IsNaN(got) {
		t.Fatalf("asin domain got %v", got)
	}
}

func TestRenderFunctionGraph640x480AndAxes(t *testing.T) {
	img, err := renderFunctionGraph(GraphRenderSpec{
		Width: 640, Height: 480,
		XMin: -10, XMax: 10, YMin: -10, YMax: 10,
		ShowAxes: true, AngleMode: GraphRadians,
		Expression: "x^2/5-3", Color: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if img.W != 640 || img.H != 480 || len(img.Pix) != 640*480*4 || len(img.Mask) != 640*480 {
		t.Fatalf("bad image dimensions %#v", img)
	}
	count := 0
	for _, on := range img.Mask {
		if on {
			count++
		}
	}
	if count < 1000 {
		t.Fatalf("too little graph ink: %d", count)
	}
}

func TestRenderFunctionGraphWithoutAxes(t *testing.T) {
	withAxes, err := renderFunctionGraph(GraphRenderSpec{Width: 320, Height: 240, XMin: -2, XMax: 2, YMin: -2, YMax: 2, ShowAxes: true, Expression: "x", Color: 0})
	if err != nil {
		t.Fatal(err)
	}
	withoutAxes, err := renderFunctionGraph(GraphRenderSpec{Width: 320, Height: 240, XMin: -2, XMax: 2, YMin: -2, YMax: 2, ShowAxes: false, Expression: "x", Color: 0})
	if err != nil {
		t.Fatal(err)
	}
	count := func(img FunctionGraphImage) int {
		n := 0
		for _, v := range img.Mask {
			if v {
				n++
			}
		}
		return n
	}
	if count(withAxes) <= count(withoutAxes) {
		t.Fatalf("axes should add ink: with=%d without=%d", count(withAxes), count(withoutAxes))
	}
}

func TestGraphParserRejectsUnknownNames(t *testing.T) {
	if _, err := parseGraphExpression("mystery(x)"); err == nil {
		t.Fatal("expected unknown function error")
	}
}

func TestGraphVariablesXYRAreAccepted(t *testing.T) {
	for _, source := range []string{"x^2", "y^2", "r^2"} {
		if got := graphEvalForTest(t, source, 3, GraphRadians); math.Abs(got-9) > 1e-12 {
			t.Fatalf("%s got %v want 9", source, got)
		}
	}
}

func TestGraphMinimumArrowIsOnePixelWithCompactHead(t *testing.T) {
	img := FunctionGraphImage{W: 80, H: 30, Pix: make([]byte, 80*30*4), Mask: make([]bool, 80*30)}
	graphDrawMinimumArrow(&img, 2, 15, 70, 15, graphColorBGRA(0))
	shaftRows := []int{}
	for y := 0; y < img.H; y++ {
		if img.Mask[y*img.W+20] {
			shaftRows = append(shaftRows, y)
		}
	}
	if len(shaftRows) != 1 || shaftRows[0] != 15 {
		t.Fatalf("1 px shaft rows=%v want [15]", shaftRows)
	}
	minX, minY, maxX, maxY := img.W, img.H, -1, -1
	for y := 0; y < img.H; y++ {
		for x := 55; x < img.W; x++ {
			if !img.Mask[y*img.W+x] {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX != 70 || maxY-minY+1 < 8 || maxX-minX+1 < 12 {
		t.Fatalf("compact head/neck bounds=(%d,%d)-(%d,%d)", minX, minY, maxX, maxY)
	}
}

func TestRenderFunctionOfYUsesYAxisAsIndependentVariable(t *testing.T) {
	img, err := renderFunctionGraph(GraphRenderSpec{
		Width: 320, Height: 240,
		XMin: -2, XMax: 2, YMin: -2, YMax: 2,
		FunctionMode: GraphFunctionOfY, Expression: "y^2-1", Color: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	// x=-1 at y=0 lies on the curve x=f(y).
	left, top, right, bottom := graphPlotBounds(320, 240)
	px := int(math.Round(left + ((-1.0)-(-2.0))/4.0*(right-left)))
	py := int(math.Round(bottom - (0.0-(-2.0))/4.0*(bottom-top)))
	found := false
	for yy := py - 3; yy <= py+3; yy++ {
		for xx := px - 3; xx <= px+3; xx++ {
			if xx >= 0 && yy >= 0 && xx < img.W && yy < img.H && img.Mask[yy*img.W+xx] {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("f(y) curve missing near mapped (-1,0)=(%d,%d)", px, py)
	}
}

func TestRenderPolarConstantRadiusDrawsCircle(t *testing.T) {
	img, err := renderFunctionGraph(GraphRenderSpec{
		Width: 320, Height: 240,
		XMin: -3, XMax: 3, YMin: -3, YMax: 3,
		FunctionMode: GraphFunctionPolar, AngleMode: GraphRadians,
		Expression: "2", Color: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	left, top, right, bottom := graphPlotBounds(320, 240)
	mapX := func(x float64) int { return int(math.Round(left + (x+3)/6*(right-left))) }
	mapY := func(y float64) int { return int(math.Round(bottom - (y+3)/6*(bottom-top))) }
	for _, pt := range [][2]float64{{2, 0}, {-2, 0}, {0, 2}, {0, -2}} {
		px, py := mapX(pt[0]), mapY(pt[1])
		found := false
		for yy := py - 3; yy <= py+3; yy++ {
			for xx := px - 3; xx <= px+3; xx++ {
				if xx >= 0 && yy >= 0 && xx < img.W && yy < img.H && img.Mask[yy*img.W+xx] {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("polar radius 2 missing near (%v,%v) mapped (%d,%d)", pt[0], pt[1], px, py)
		}
	}
}

func TestGraphPolarSamplesParseAndRender(t *testing.T) {
	seen := map[string]bool{}
	for _, sample := range graphPolarSamples {
		if sample.name == "" || sample.expression == "" || sample.description == "" {
			t.Fatalf("incomplete polar sample: %#v", sample)
		}
		if seen[sample.name] {
			t.Fatalf("duplicate polar sample name %q", sample.name)
		}
		seen[sample.name] = true
		if _, err := parseGraphExpression(sample.expression); err != nil {
			t.Fatalf("sample %q parse %q: %v", sample.name, sample.expression, err)
		}
		for _, angleMode := range []GraphAngleMode{GraphRadians, GraphDegrees} {
			img, err := renderFunctionGraph(GraphRenderSpec{
				Width: 320, Height: 240,
				XMin: -10, XMax: 10, YMin: -10, YMax: 10,
				ShowAxes: false, AngleMode: angleMode, FunctionMode: GraphFunctionPolar,
				Expression: sample.expression, Color: 0,
			})
			if err != nil {
				t.Fatalf("sample %q mode %d render: %v", sample.name, angleMode, err)
			}
			ink := 0
			for _, on := range img.Mask {
				if on {
					ink++
				}
			}
			if ink < 20 {
				t.Fatalf("sample %q mode %d produced too little ink: %d", sample.name, angleMode, ink)
			}
		}
	}
	if len(graphPolarSamples) < 6 {
		t.Fatalf("expected a useful sample set, got %d", len(graphPolarSamples))
	}
}

func TestGraphUserPolarEllipseSyntaxWithoutFunctionParentheses(t *testing.T) {
	n, err := parseGraphExpression("5/(1+pi/6 cos r)")
	if err != nil {
		t.Fatal(err)
	}
	got := evalGraphExpression(n, 0, GraphRadians)
	want := 5 / (1 + math.Pi/6)
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestGraphNiceTickEndpointExamples(t *testing.T) {
	cases := []struct {
		limit float64
		want  float64
	}{
		{2.5, 2.4},
		{10, 10},
		{119, 100},
		{0.00355, 0.0034},
	}
	for _, tc := range cases {
		got := graphNiceLeadingMultiple(tc.limit, 2)
		if math.Abs(got-tc.want) > math.Max(1e-14, math.Abs(tc.want)*1e-12) {
			t.Fatalf("nice x endpoint(%g)=%g want %g", tc.limit, got, tc.want)
		}
	}
}

func TestGraphTickPlanFourByThreeRules(t *testing.T) {
	plan := graphBuildTickPlan(10, 7.5, true)
	if math.Abs(plan.XStep-2.5) > 1e-12 || math.Abs(plan.YStep-2.5) > 1e-12 || !plan.SharedYStep {
		t.Fatalf("proportional plan=%#v", plan)
	}
	plan = graphBuildTickPlan(10, 11, false)
	if math.Abs(plan.YStep-2.5) > 1e-12 || !plan.SharedYStep {
		t.Fatalf("close y range should share x ticks: %#v", plan)
	}
	plan = graphBuildTickPlan(10, 119, false)
	if math.Abs(plan.YAnchor-99) > 1e-12 || math.Abs(plan.YStep-33) > 1e-12 || plan.SharedYStep {
		t.Fatalf("far y range should use three-part clean endpoint: %#v", plan)
	}
}

func TestGraphProportionalYRange(t *testing.T) {
	yMin, yMax := graphProportionalYRange(-10, 10)
	if yMin != -7.5 || yMax != 7.5 {
		t.Fatalf("got [%g,%g] want [-7.5,7.5]", yMin, yMax)
	}
	yMin, yMax = graphProportionalYRange(-4, 12)
	if yMin != -3 || yMax != 9 {
		t.Fatalf("asymmetric got [%g,%g] want [-3,9]", yMin, yMax)
	}
}

func TestGraphTickFormatting(t *testing.T) {
	cases := []struct {
		value float64
		step  float64
		want  string
	}{
		{0.6, 0.6, "0.6"},
		{2.55, 0.85, "2.55"},
		{0.00085, 0.00085, "8.5e-4"},
		{0.0017, 0.00085, "1.7e-3"},
		{0.00255, 0.00085, "2.55e-3"},
		{0.0034, 0.00085, "3.4e-3"},
		{-25000, 25000, "-2.5e4"},
	}
	for _, tc := range cases {
		if got := graphFormatTickValue(tc.value, tc.step); got != tc.want {
			t.Fatalf("format %g step %g=%q want %q", tc.value, tc.step, got, tc.want)
		}
	}
}

func TestGraphPlotBoundsAreExactFourByThree(t *testing.T) {
	left, top, right, bottom := graphPlotBounds(640, 480)
	if math.Abs((right-left)/(bottom-top)-4.0/3.0) > 1e-12 {
		t.Fatalf("bounds [%g,%g]-[%g,%g] are not 4:3", left, top, right, bottom)
	}
	if left < 10 || top < 8 || right > 630 || bottom > 472 {
		t.Fatalf("arrow safety margins lost: [%g,%g]-[%g,%g]", left, top, right, bottom)
	}
}

func TestRenderAxesIncludeTickLabels(t *testing.T) {
	withTicks, err := renderFunctionGraph(GraphRenderSpec{
		Width: 640, Height: 480,
		XMin: -10, XMax: 10, YMin: -7.5, YMax: 7.5,
		ShowAxes: true, ProportionalScale: true,
		Expression: "0", Color: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The positive x=2.5 label is below the horizontal axis.  Ensure at least
	// one pixel exists in its expected text neighborhood, not just on the axis.
	left, top, right, bottom := graphPlotBounds(640, 480)
	_ = top
	_ = bottom
	px := int(math.Round(left + (2.5+10)/20*(right-left)))
	found := false
	for y := 249; y <= 260; y++ {
		for x := px - 12; x <= px+12; x++ {
			if x >= 0 && y >= 0 && x < withTicks.W && y < withTicks.H && withTicks.Mask[y*withTicks.W+x] {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("tick label missing near x=2.5 mapped px=%d", px)
	}
}

func TestGraphTickFontMatchesUserBitmap(t *testing.T) {
	if graphTickGlyphWidth != 5 || graphTickGlyphHeight != 7 || graphTickGlyphAdvance != 6 {
		t.Fatalf("tick font geometry=%dx%d advance %d, want 5x7 advance 6", graphTickGlyphWidth, graphTickGlyphHeight, graphTickGlyphAdvance)
	}
	if got := graphPixelTextWidth("10"); got != 11 {
		t.Fatalf("two-character tick width=%d want 11", got)
	}
}

func TestGraphAxisArrowExtendsBeyondNumericMaximum(t *testing.T) {
	img, err := renderFunctionGraph(GraphRenderSpec{
		Width: 640, Height: 480,
		XMin: -10, XMax: 10, YMin: -7.5, YMax: 7.5,
		ShowAxes: true, ProportionalScale: true,
		Expression: "0", Color: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	left, top, right, bottom := graphPlotBounds(640, 480)
	axisY := int(math.Round(bottom - (0-(-7.5))/15*(bottom-top)))
	dataEnd := int(math.Round(right))
	// The numeric x=10 endpoint must still be part of the plain 1 px shaft.
	shaftRows := 0
	for y := 0; y < img.H; y++ {
		if img.Mask[y*img.W+dataEnd] {
			shaftRows++
		}
	}
	if shaftRows < 1 {
		t.Fatalf("numeric maximum x=10 is not on the straight axis: rows=%d", shaftRows)
	}
	// Five pixels beyond x=10 is still only the narrow extension shaft; the
	// arrowhead itself begins later, around 2.5%% of the full span (=x=10.5).
	narrowX := dataEnd + 5
	narrowRows := 0
	for y := axisY - 8; y <= axisY+8; y++ {
		if y >= 0 && y < img.H && img.Mask[y*img.W+narrowX] {
			narrowRows++
		}
	}
	if narrowRows != 1 {
		t.Fatalf("early extension at x=%d has %d rows, want the narrow 1 px shaft", narrowX, narrowRows)
	}
	// Past the 2.5%% extension the minimum arrowhead must widen, and all of it
	// must lie outside the numeric right/top data bounds.
	wideFound := false
	maxInkX := -1
	minInkY := img.H
	for y := 0; y < img.H; y++ {
		for x := 0; x < img.W; x++ {
			if !img.Mask[y*img.W+x] {
				continue
			}
			if x > maxInkX {
				maxInkX = x
			}
			if y < minInkY {
				minInkY = y
			}
		}
	}
	for x := dataEnd + 14; x <= min(img.W-1, dataEnd+28); x++ {
		rows := 0
		for y := axisY - 8; y <= axisY+8; y++ {
			if y >= 0 && y < img.H && img.Mask[y*img.W+x] {
				rows++
			}
		}
		if rows > 2 {
			wideFound = true
			break
		}
	}
	if !wideFound {
		t.Fatal("arrowhead did not widen beyond the numeric x maximum")
	}
	if maxInkX <= int(math.Round(right)) {
		t.Fatalf("x arrow does not extend beyond xMax bound: maxInkX=%d right=%g", maxInkX, right)
	}
	if minInkY >= int(math.Round(top)) {
		t.Fatalf("y arrow does not extend beyond yMax bound: minInkY=%d top=%g", minInkY, top)
	}
	_ = left
}

func TestPolarLemniscateBranchesMeetAtOrigin(t *testing.T) {
	img, err := renderFunctionGraph(GraphRenderSpec{
		Width: 640, Height: 480,
		XMin: -1, XMax: 1, YMin: -0.75, YMax: 0.75,
		ShowAxes: false, ProportionalScale: true,
		AngleMode: GraphRadians, FunctionMode: GraphFunctionPolar,
		Expression: "sqrt(16cos(2r))", Color: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	left, top, right, bottom := graphPlotBounds(640, 480)
	originX := int(math.Round(left + (0-(-1.0))/2.0*(right-left)))
	originY := int(math.Round(bottom - (0-(-0.75))/1.5*(bottom-top)))
	found := false
	for y := originY - 2; y <= originY+2; y++ {
		for x := originX - 2; x <= originX+2; x++ {
			if x >= 0 && y >= 0 && x < img.W && y < img.H && img.Mask[y*img.W+x] {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("lemniscate branches do not meet at origin (%d,%d)", originX, originY)
	}
}
