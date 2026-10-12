package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// GraphAngleMode controls the unit used by direct trigonometric functions and
// the value returned by inverse trigonometric functions.
type GraphAngleMode int

const (
	GraphRadians GraphAngleMode = iota
	GraphDegrees
)

// GraphFunctionMode selects which coordinate is treated as the independent
// variable.  Cartesian-X draws y=f(x), Cartesian-Y draws x=f(y), and Polar
// draws a radius as a function of one full-turn angular parameter.
type GraphFunctionMode int

const (
	GraphFunctionOfX GraphFunctionMode = iota
	GraphFunctionOfY
	GraphFunctionPolar
)

// graphPolarSample is shared by the Win32 sample menu and portable tests.
// Expressions intentionally use only trigonometric polar forms so the same
// sample works in both radian and degree angle modes.
type graphPolarSample struct {
	name        string
	expression  string
	description string
}

var graphPolarSamples = []graphPolarSample{
	{"원", "3", "반지름 3인 원입니다."},
	{"타원(초점 원점)", "4/(1+0.5cos(r))", "원점이 한 초점인 타원입니다. 이심률 e=0.5입니다."},
	{"포물선(초점 원점)", "3/(1+cos(r))", "원점이 초점인 포물선입니다. 화면 범위 밖으로 이어집니다."},
	{"쌍곡선(초점 원점)", "3/(1+1.4cos(r))", "원점이 한 초점인 쌍곡선입니다. 두 갈래가 화면 범위로 잘릴 수 있습니다."},
	{"심장선", "3(1+cos(r))", "원점에 뾰족점이 있는 심장선(카디오이드)입니다."},
	{"장미곡선 5잎", "4cos(5r)", "홀수 계수의 5잎 장미곡선입니다."},
	{"장미곡선 8잎", "4cos(4r)", "짝수 계수에서 꽃잎 수가 2배가 되는 8잎 장미곡선입니다."},
	{"리마송(내고리)", "2+4cos(r)", "원점 안쪽에 작은 고리가 생기는 리마송입니다."},
	{"렘니스케이트", "sqrt(16cos(2r))", "원점을 중심으로 두 잎이 만나는 베르누이 렘니스케이트입니다."},
}

// FunctionGraphImage is a portable BGRA image used by the graph editor.  It is
// deliberately independent of Win32 so parsing and rasterisation can be tested
// on every build host.
type FunctionGraphImage struct {
	W, H int
	Pix  []byte // straight-alpha BGRA, top-down
	Mask []bool
}

type GraphRenderSpec struct {
	Width, Height     int
	XMin, XMax        float64
	YMin, YMax        float64
	ShowAxes          bool
	ProportionalScale bool
	AngleMode         GraphAngleMode
	FunctionMode      GraphFunctionMode
	Expression        string
	Color             uint32 // COLORREF layout: 0x00BBGGRR
}

// --- Expression parser -----------------------------------------------------

type graphTokenKind int

const (
	graphTokEOF graphTokenKind = iota
	graphTokNumber
	graphTokIdent
	graphTokPlus
	graphTokMinus
	graphTokMul
	graphTokDiv
	graphTokPow
	graphTokLParen
	graphTokRParen
	graphTokComma
	graphTokFact
)

type graphToken struct {
	kind graphTokenKind
	text string
	num  float64
	pos  int
}

type graphLexer struct {
	src   []rune
	pos   int
	token graphToken
}

func newGraphLexer(source string) *graphLexer {
	l := &graphLexer{src: []rune(strings.TrimSpace(source))}
	l.next()
	return l
}

func (l *graphLexer) next() {
	for l.pos < len(l.src) && unicode.IsSpace(l.src[l.pos]) {
		l.pos++
	}
	if l.pos >= len(l.src) {
		l.token = graphToken{kind: graphTokEOF, pos: l.pos}
		return
	}
	start := l.pos
	r := l.src[l.pos]
	l.pos++
	switch r {
	case '+':
		l.token = graphToken{kind: graphTokPlus, text: "+", pos: start}
	case '-', '−':
		l.token = graphToken{kind: graphTokMinus, text: "-", pos: start}
	case '*', '×', '·':
		l.token = graphToken{kind: graphTokMul, text: "*", pos: start}
	case '/', '÷':
		l.token = graphToken{kind: graphTokDiv, text: "/", pos: start}
	case '^':
		l.token = graphToken{kind: graphTokPow, text: "^", pos: start}
	case '(':
		l.token = graphToken{kind: graphTokLParen, text: "(", pos: start}
	case ')':
		l.token = graphToken{kind: graphTokRParen, text: ")", pos: start}
	case ',':
		l.token = graphToken{kind: graphTokComma, text: ",", pos: start}
	case '!':
		l.token = graphToken{kind: graphTokFact, text: "!", pos: start}
	default:
		if unicode.IsDigit(r) || r == '.' {
			for l.pos < len(l.src) && (unicode.IsDigit(l.src[l.pos]) || l.src[l.pos] == '.') {
				l.pos++
			}
			if l.pos < len(l.src) && (l.src[l.pos] == 'e' || l.src[l.pos] == 'E') {
				save := l.pos
				l.pos++
				if l.pos < len(l.src) && (l.src[l.pos] == '+' || l.src[l.pos] == '-' || l.src[l.pos] == '−') {
					l.pos++
				}
				digitStart := l.pos
				for l.pos < len(l.src) && unicode.IsDigit(l.src[l.pos]) {
					l.pos++
				}
				if digitStart == l.pos {
					l.pos = save
				}
			}
			text := strings.ReplaceAll(string(l.src[start:l.pos]), "−", "-")
			v, err := strconv.ParseFloat(text, 64)
			if err != nil {
				l.token = graphToken{kind: graphTokIdent, text: text, pos: start}
			} else {
				l.token = graphToken{kind: graphTokNumber, text: text, num: v, pos: start}
			}
			return
		}
		if unicode.IsLetter(r) || r == 'π' {
			for l.pos < len(l.src) && (unicode.IsLetter(l.src[l.pos]) || unicode.IsDigit(l.src[l.pos]) || l.src[l.pos] == 'π') {
				l.pos++
			}
			l.token = graphToken{kind: graphTokIdent, text: strings.ToLower(string(l.src[start:l.pos])), pos: start}
			return
		}
		l.token = graphToken{kind: graphTokIdent, text: string(r), pos: start}
	}
}

type graphNodeKind int

const (
	graphNodeNumber graphNodeKind = iota
	graphNodeVariable
	graphNodeUnary
	graphNodeBinary
	graphNodeFunction
)

type graphExprNode struct {
	kind        graphNodeKind
	value       float64
	op          graphTokenKind
	name        string
	left, right *graphExprNode
	args        []*graphExprNode
}

type graphParser struct {
	lex *graphLexer
}

func parseGraphExpression(source string) (*graphExprNode, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, fmt.Errorf("함수식을 입력하십시오")
	}
	p := &graphParser{lex: newGraphLexer(source)}
	n, err := p.parseAddSub()
	if err != nil {
		return nil, err
	}
	if p.lex.token.kind != graphTokEOF {
		return nil, fmt.Errorf("%d번째 문자 근처의 %q을(를) 해석할 수 없습니다", p.lex.token.pos+1, p.lex.token.text)
	}
	return n, nil
}

func (p *graphParser) parseAddSub() (*graphExprNode, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return nil, err
	}
	for p.lex.token.kind == graphTokPlus || p.lex.token.kind == graphTokMinus {
		op := p.lex.token.kind
		p.lex.next()
		right, err := p.parseMulDiv()
		if err != nil {
			return nil, err
		}
		left = &graphExprNode{kind: graphNodeBinary, op: op, left: left, right: right}
	}
	return left, nil
}

func graphTokenStartsFactor(k graphTokenKind) bool {
	return k == graphTokNumber || k == graphTokIdent || k == graphTokLParen
}

func (p *graphParser) parseMulDiv() (*graphExprNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		op := p.lex.token.kind
		implicit := false
		if op != graphTokMul && op != graphTokDiv {
			if graphTokenStartsFactor(op) {
				op = graphTokMul
				implicit = true
			} else {
				break
			}
		}
		if !implicit {
			p.lex.next()
		}
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &graphExprNode{kind: graphNodeBinary, op: op, left: left, right: right}
	}
	return left, nil
}

func (p *graphParser) parseUnary() (*graphExprNode, error) {
	if p.lex.token.kind == graphTokPlus || p.lex.token.kind == graphTokMinus {
		op := p.lex.token.kind
		p.lex.next()
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		if op == graphTokPlus {
			return child, nil
		}
		return &graphExprNode{kind: graphNodeUnary, op: op, right: child}, nil
	}
	return p.parsePower()
}

func (p *graphParser) parsePower() (*graphExprNode, error) {
	left, err := p.parsePostfix()
	if err != nil {
		return nil, err
	}
	if p.lex.token.kind == graphTokPow {
		p.lex.next()
		// Unary on the right permits x^-2 while keeping exponentiation right-associative.
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &graphExprNode{kind: graphNodeBinary, op: graphTokPow, left: left, right: right}, nil
	}
	return left, nil
}

func (p *graphParser) parsePostfix() (*graphExprNode, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.lex.token.kind == graphTokFact {
		p.lex.next()
		left = &graphExprNode{kind: graphNodeFunction, name: "fact", args: []*graphExprNode{left}}
	}
	return left, nil
}

func canonicalGraphFunctionName(name string) string {
	switch strings.ToLower(name) {
	case "arcsin":
		return "asin"
	case "arccos":
		return "acos"
	case "arctan":
		return "atan"
	case "arccot":
		return "acot"
	case "arcsec":
		return "asec"
	case "arccsc", "arccosec":
		return "acsc"
	case "cosec":
		return "csc"
	case "loge":
		return "ln"
	case "factorial":
		return "fact"
	}
	return strings.ToLower(name)
}

func isGraphFunction(name string) bool {
	switch canonicalGraphFunctionName(name) {
	case "sin", "cos", "tan", "cot", "sec", "csc",
		"asin", "acos", "atan", "acot", "asec", "acsc",
		"exp", "ln", "log", "log10", "log2", "sqrt", "abs", "fact":
		return true
	}
	return false
}

func (p *graphParser) parsePrimary() (*graphExprNode, error) {
	tok := p.lex.token
	switch tok.kind {
	case graphTokNumber:
		p.lex.next()
		return &graphExprNode{kind: graphNodeNumber, value: tok.num}, nil
	case graphTokLParen:
		p.lex.next()
		n, err := p.parseAddSub()
		if err != nil {
			return nil, err
		}
		if p.lex.token.kind != graphTokRParen {
			return nil, fmt.Errorf("%d번째 문자 근처에 닫는 괄호가 필요합니다", p.lex.token.pos+1)
		}
		p.lex.next()
		return n, nil
	case graphTokIdent:
		name := canonicalGraphFunctionName(tok.text)
		p.lex.next()
		switch name {
		case "x", "y", "r":
			return &graphExprNode{kind: graphNodeVariable, name: name}, nil
		case "pi", "π":
			return &graphExprNode{kind: graphNodeNumber, value: math.Pi}, nil
		case "e":
			return &graphExprNode{kind: graphNodeNumber, value: math.E}, nil
		}
		if !isGraphFunction(name) {
			return nil, fmt.Errorf("%d번째 문자 근처의 함수 또는 상수 %q을(를) 알 수 없습니다", tok.pos+1, tok.text)
		}
		var args []*graphExprNode
		if p.lex.token.kind == graphTokLParen {
			p.lex.next()
			if p.lex.token.kind == graphTokRParen {
				return nil, fmt.Errorf("함수 %s에는 인수가 필요합니다", name)
			}
			for {
				arg, err := p.parseAddSub()
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
				if p.lex.token.kind != graphTokComma {
					break
				}
				p.lex.next()
			}
			if p.lex.token.kind != graphTokRParen {
				return nil, fmt.Errorf("함수 %s의 닫는 괄호가 필요합니다", name)
			}
			p.lex.next()
		} else {
			// Familiar forms such as sin x and exp x are accepted too.
			arg, err := p.parseUnary()
			if err != nil {
				return nil, fmt.Errorf("함수 %s의 인수가 필요합니다: %w", name, err)
			}
			args = append(args, arg)
		}
		if len(args) != 1 {
			return nil, fmt.Errorf("함수 %s는 인수 하나를 사용합니다", name)
		}
		return &graphExprNode{kind: graphNodeFunction, name: name, args: args}, nil
	default:
		return nil, fmt.Errorf("%d번째 문자 근처에 수, x/y/r, 함수 또는 괄호가 필요합니다", tok.pos+1)
	}
}

func graphAngleIn(v float64, mode GraphAngleMode) float64 {
	if mode == GraphDegrees {
		return v * math.Pi / 180
	}
	return v
}

func graphAngleOut(v float64, mode GraphAngleMode) float64 {
	if mode == GraphDegrees {
		return v * 180 / math.Pi
	}
	return v
}

func evalGraphExpression(n *graphExprNode, variable float64, mode GraphAngleMode) float64 {
	if n == nil {
		return math.NaN()
	}
	switch n.kind {
	case graphNodeNumber:
		return n.value
	case graphNodeVariable:
		return variable
	case graphNodeUnary:
		v := evalGraphExpression(n.right, variable, mode)
		if n.op == graphTokMinus {
			return -v
		}
		return v
	case graphNodeBinary:
		a := evalGraphExpression(n.left, variable, mode)
		b := evalGraphExpression(n.right, variable, mode)
		if math.IsNaN(a) || math.IsNaN(b) {
			return math.NaN()
		}
		switch n.op {
		case graphTokPlus:
			return a + b
		case graphTokMinus:
			return a - b
		case graphTokMul:
			return a * b
		case graphTokDiv:
			if b == 0 {
				return math.NaN()
			}
			return a / b
		case graphTokPow:
			return math.Pow(a, b)
		}
	case graphNodeFunction:
		v := evalGraphExpression(n.args[0], variable, mode)
		if math.IsNaN(v) {
			return math.NaN()
		}
		a := graphAngleIn(v, mode)
		switch n.name {
		case "sin":
			return math.Sin(a)
		case "cos":
			return math.Cos(a)
		case "tan":
			c := math.Cos(a)
			if math.Abs(c) < 1e-12 {
				return math.NaN()
			}
			return math.Tan(a)
		case "cot":
			s := math.Sin(a)
			if math.Abs(s) < 1e-12 {
				return math.NaN()
			}
			return math.Cos(a) / s
		case "sec":
			c := math.Cos(a)
			if math.Abs(c) < 1e-12 {
				return math.NaN()
			}
			return 1 / c
		case "csc":
			s := math.Sin(a)
			if math.Abs(s) < 1e-12 {
				return math.NaN()
			}
			return 1 / s
		case "asin":
			if v < -1 || v > 1 {
				return math.NaN()
			}
			return graphAngleOut(math.Asin(v), mode)
		case "acos":
			if v < -1 || v > 1 {
				return math.NaN()
			}
			return graphAngleOut(math.Acos(v), mode)
		case "atan":
			return graphAngleOut(math.Atan(v), mode)
		case "acot":
			return graphAngleOut(math.Atan2(1, v), mode)
		case "asec":
			if math.Abs(v) < 1 {
				return math.NaN()
			}
			return graphAngleOut(math.Acos(1/v), mode)
		case "acsc":
			if math.Abs(v) < 1 {
				return math.NaN()
			}
			return graphAngleOut(math.Asin(1/v), mode)
		case "exp":
			return math.Exp(v)
		case "ln":
			if v <= 0 {
				return math.NaN()
			}
			return math.Log(v)
		case "log", "log10":
			if v <= 0 {
				return math.NaN()
			}
			return math.Log10(v)
		case "log2":
			if v <= 0 {
				return math.NaN()
			}
			return math.Log2(v)
		case "sqrt":
			if v < 0 {
				return math.NaN()
			}
			return math.Sqrt(v)
		case "abs":
			return math.Abs(v)
		case "fact":
			return graphFactorial(v)
		}
	}
	return math.NaN()
}

// graphFactorial implements the standard continuous extension x! = Γ(x+1).
// It therefore agrees with ordinary factorials at every non-negative integer,
// while negative integers remain undefined.  This is much more useful for a
// graph editor than an integer-only operation, because the curve can be drawn
// continuously between its poles.
func graphFactorial(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return math.NaN()
	}
	if v <= -1 {
		r := math.Round(v)
		if math.Abs(v-r) < 1e-12 {
			return math.NaN()
		}
	}
	result := math.Gamma(v + 1)
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return math.NaN()
	}
	return result
}

// graphNiceLeadingMultiple chooses the largest "clean" endpoint not greater
// than limit.  The normalized leading coefficient is an integer with two
// digits and is a multiple of the requested divisor.  This matches the graph
// editor rules: 2.5 -> 2.4, 10 -> 10, 119 -> 100 and 0.00355 -> 0.0034 for
// the four-part x axis (divisor 2).
func graphNiceLeadingMultiple(limit float64, divisor int) float64 {
	if !isFiniteGraphValue(limit) || limit <= 0 || divisor <= 0 {
		return 0
	}
	exponent := math.Floor(math.Log10(limit)) - 1
	scale := math.Pow(10, exponent)
	coefficient := int(math.Floor(limit/scale + 1e-12))
	candidate := coefficient - coefficient%divisor
	minimumTwoDigit := ((10 + divisor - 1) / divisor) * divisor
	if candidate < minimumTwoDigit {
		// A divisor of three has no two-digit candidate below 12.  Move to the
		// next finer decade and cap the coefficient at 99, preserving the
		// two-significant-digit rule instead of producing a three-digit leader.
		scale /= 10
		coefficient = int(math.Floor(limit/scale + 1e-12))
		if coefficient > 99 {
			coefficient = 99
		}
		candidate = coefficient - coefficient%divisor
	}
	if candidate <= 0 {
		return 0
	}
	return float64(candidate) * scale
}

type graphTickPlan struct {
	XAnchor, XStep float64
	YAnchor, YStep float64
	SharedYStep    bool
}

func graphBuildTickPlan(xMax, yMax float64, proportional bool) graphTickPlan {
	plan := graphTickPlan{}
	plan.XAnchor = graphNiceLeadingMultiple(xMax, 2)
	if plan.XAnchor > 0 {
		plan.XStep = plan.XAnchor / 4
	}
	expectedY := plan.XAnchor * 0.75
	closeEnough := false
	if expectedY > 0 && yMax > 0 {
		ratio := math.Max(expectedY, yMax) / math.Min(expectedY, yMax)
		closeEnough = ratio < 1.5
	}
	if proportional || closeEnough {
		plan.YStep = plan.XStep
		plan.YAnchor = plan.YStep * 3
		plan.SharedYStep = true
	} else {
		plan.YAnchor = graphNiceLeadingMultiple(yMax, 3)
		if plan.YAnchor > 0 {
			plan.YStep = plan.YAnchor / 3
		}
	}
	return plan
}

func graphProportionalYRange(xMin, xMax float64) (float64, float64) {
	return xMin * 0.75, xMax * 0.75
}

func graphFormatTickValue(value, step float64) string {
	if math.Abs(value) < math.Max(math.Abs(step)*1e-10, 1e-15) {
		value = 0
	}
	abs := math.Abs(value)
	stepAbs := math.Abs(step)
	if abs != 0 && (abs < 0.001 || stepAbs < 0.001 || abs >= 10000 || stepAbs >= 10000) {
		text := strconv.FormatFloat(value, 'e', 6, 64)
		parts := strings.Split(text, "e")
		mantissa := strings.TrimRight(strings.TrimRight(parts[0], "0"), ".")
		exp := strings.TrimLeft(parts[1], "+")
		sign := ""
		if strings.HasPrefix(exp, "-") {
			sign = "-"
			exp = strings.TrimPrefix(exp, "-")
		}
		exp = strings.TrimLeft(exp, "0")
		if exp == "" {
			exp = "0"
		}
		return mantissa + "e" + sign + exp
	}
	text := strconv.FormatFloat(value, 'f', 10, 64)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "-0" || text == "" {
		return "0"
	}
	return text
}

// graphPlotBounds returns the largest exact 4:3 data rectangle that fits
// inside the transparent image while reserving space beyond the positive x/y
// maxima.  The numeric range ends at right/top; the minimum arrowhead is drawn
// as an additional extension outside those limits instead of consuming them.
func graphPlotBounds(width, height int) (left, top, right, bottom float64) {
	// On a 640x480 output this produces a 576x432 data rectangle with 32 px
	// horizontal and 24 px vertical margins.  Those margins hold a 2.5%-of-span
	// extension plus dotPaint's minimum arrowhead without changing the 4:3 scale.
	availW := math.Max(float64(width)-64, 10)
	availH := math.Max(float64(height)-48, 10)
	plotW := math.Min(availW, availH*4.0/3.0)
	plotH := plotW * 3.0 / 4.0
	if plotH > availH {
		plotH = availH
		plotW = plotH * 4.0 / 3.0
	}
	left = (float64(width) - plotW) / 2
	top = (float64(height) - plotH) / 2
	right = left + plotW
	bottom = top + plotH
	return
}

var graphPixelGlyphs = map[rune][7]string{
	'0': {"01110", "10001", "10001", "10001", "10001", "10001", "01110"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3': {"01110", "10001", "00001", "00110", "00001", "10001", "01110"},
	'4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5': {"11111", "10000", "10000", "11110", "00001", "00001", "11110"},
	'6': {"00111", "01000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "10001", "00001", "00010", "00100", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00001", "01110"},
	'-': {"00000", "00000", "00000", "11111", "00000", "00000", "00000"},
	'+': {"00000", "00100", "00100", "11111", "00100", "00100", "00000"},
	'.': {"00000", "00000", "00000", "00000", "00000", "00110", "00110"},
	'e': {"00000", "01110", "10001", "11111", "10000", "10001", "01110"},
}

const (
	graphTickGlyphWidth   = 5
	graphTickGlyphHeight  = 7
	graphTickGlyphAdvance = 6
)

func graphPixelTextWidth(text string) int {
	if text == "" {
		return 0
	}
	return len([]rune(text))*graphTickGlyphAdvance - 1
}

// graphDrawPixelText copies the user-defined 5x7 bitmap glyphs directly
// without any scaling so the labels remain exact dot text.
func graphDrawPixelText(img *FunctionGraphImage, x, y int, text string, col [4]byte) {
	cursor := x
	for _, r := range text {
		glyph, ok := graphPixelGlyphs[r]
		if !ok {
			cursor += graphTickGlyphAdvance
			continue
		}
		for gy := 0; gy < graphTickGlyphHeight; gy++ {
			row := glyph[gy]
			for gx := 0; gx < graphTickGlyphWidth; gx++ {
				if row[gx] == '1' {
					graphBlendPixel(img, cursor+gx, y+gy, 1, col)
				}
			}
		}
		cursor += graphTickGlyphAdvance
	}
}

func graphDrawCrispHVLine(img *FunctionGraphImage, x0, y0, x1, y1 float64, col [4]byte) {
	ix0, iy0 := int(math.Round(x0)), int(math.Round(y0))
	ix1, iy1 := int(math.Round(x1)), int(math.Round(y1))
	if ix0 == ix1 {
		if iy0 > iy1 {
			iy0, iy1 = iy1, iy0
		}
		for y := iy0; y <= iy1; y++ {
			graphBlendPixel(img, ix0, y, 1, col)
		}
		return
	}
	if iy0 == iy1 {
		if ix0 > ix1 {
			ix0, ix1 = ix1, ix0
		}
		for x := ix0; x <= ix1; x++ {
			graphBlendPixel(img, x, iy0, 1, col)
		}
		return
	}
	graphDrawLine(img, x0, y0, x1, y1, 1.0, col)
}

func graphDrawAxisTicks(img *FunctionGraphImage, spec GraphRenderSpec, left, top, right, bottom float64, mapX, mapY func(float64) float64, col [4]byte) {
	plan := graphBuildTickPlan(spec.XMax, spec.YMax, spec.ProportionalScale)
	if spec.YMin <= 0 && spec.YMax >= 0 && plan.XStep > 0 {
		axisY := mapY(0)
		labelY := int(math.Round(axisY)) + 8
		if labelY+graphTickGlyphHeight > img.H-1 {
			labelY = int(math.Round(axisY)) - graphTickGlyphHeight - 8
		}
		for _, sign := range []float64{-1, 1} {
			for n := 1; n <= 4; n++ {
				value := sign * float64(n) * plan.XStep
				if value < spec.XMin-1e-12 || value > spec.XMax+1e-12 {
					continue
				}
				x := mapX(value)
				graphDrawCrispHVLine(img, x, axisY-3, x, axisY+3, col)
				label := graphFormatTickValue(value, plan.XStep)
				labelX := int(math.Round(x)) - graphPixelTextWidth(label)/2
				graphDrawPixelText(img, labelX, labelY, label, col)
			}
		}
	}
	if spec.XMin <= 0 && spec.XMax >= 0 && plan.YStep > 0 {
		axisX := mapX(0)
		for _, sign := range []float64{-1, 1} {
			for n := 1; n <= 3; n++ {
				value := sign * float64(n) * plan.YStep
				if value < spec.YMin-1e-12 || value > spec.YMax+1e-12 {
					continue
				}
				y := mapY(value)
				graphDrawCrispHVLine(img, axisX-3, y, axisX+3, y, col)
				label := graphFormatTickValue(value, plan.YStep)
				labelW := graphPixelTextWidth(label)
				labelX := int(math.Round(axisX)) - 7 - labelW
				if labelX < int(math.Floor(left)) {
					labelX = int(math.Round(axisX)) + 7
				}
				labelY := int(math.Round(y)) - graphTickGlyphHeight/2
				graphDrawPixelText(img, labelX, labelY, label, col)
			}
		}
	}
}

// --- Rasterisation ---------------------------------------------------------

func graphColorBGRA(c uint32) [4]byte {
	return [4]byte{byte((c >> 16) & 0xFF), byte((c >> 8) & 0xFF), byte(c & 0xFF), 255}
}

func graphBlendPixel(img *FunctionGraphImage, x, y int, alpha float64, col [4]byte) {
	if img == nil || x < 0 || y < 0 || x >= img.W || y >= img.H || alpha <= 0 {
		return
	}
	if alpha > 1 {
		alpha = 1
	}
	i := (y*img.W + x) * 4
	a := byte(math.Round(alpha * 255))
	if a <= img.Pix[i+3] {
		return
	}
	img.Pix[i+0], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = col[0], col[1], col[2], a
	if len(img.Mask) == img.W*img.H {
		img.Mask[y*img.W+x] = a > 0
	}
}

func graphDistanceToSegment(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	den := dx*dx + dy*dy
	if den <= 1e-12 {
		return math.Hypot(px-x0, py-y0)
	}
	t := ((px-x0)*dx + (py-y0)*dy) / den
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return math.Hypot(px-(x0+t*dx), py-(y0+t*dy))
}

func graphDrawLine(img *FunctionGraphImage, x0, y0, x1, y1, width float64, col [4]byte) {
	half := width/2 + 1
	minX := max(0, int(math.Floor(math.Min(x0, x1)-half)))
	maxX := min(img.W-1, int(math.Ceil(math.Max(x0, x1)+half)))
	minY := max(0, int(math.Floor(math.Min(y0, y1)-half)))
	maxY := min(img.H-1, int(math.Ceil(math.Max(y0, y1)+half)))
	radius := width / 2
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			d := graphDistanceToSegment(float64(x)+0.5, float64(y)+0.5, x0, y0, x1, y1)
			coverage := radius + 0.75 - d
			if coverage > 0 {
				graphBlendPixel(img, x, y, math.Min(1, coverage), col)
			}
		}
	}
}

func graphPointInTriangle(px, py, ax, ay, bx, by, cx, cy float64) bool {
	sign := func(px, py, x1, y1, x2, y2 float64) float64 {
		return (px-x2)*(y1-y2) - (x1-x2)*(py-y2)
	}
	d1 := sign(px, py, ax, ay, bx, by)
	d2 := sign(px, py, bx, by, cx, cy)
	d3 := sign(px, py, cx, cy, ax, ay)
	hasNeg := d1 < 0 || d2 < 0 || d3 < 0
	hasPos := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNeg && hasPos)
}

func graphFillTriangle(img *FunctionGraphImage, ax, ay, bx, by, cx, cy float64, col [4]byte) {
	minX := max(0, int(math.Floor(math.Min(ax, math.Min(bx, cx))))-1)
	maxX := min(img.W-1, int(math.Ceil(math.Max(ax, math.Max(bx, cx))))+1)
	minY := max(0, int(math.Floor(math.Min(ay, math.Min(by, cy))))-1)
	maxY := min(img.H-1, int(math.Ceil(math.Max(ay, math.Max(by, cy))))+1)
	const samples = 4
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			inside := 0
			for sy := 0; sy < samples; sy++ {
				py := float64(y) + (float64(sy)+0.5)/samples
				for sx := 0; sx < samples; sx++ {
					px := float64(x) + (float64(sx)+0.5)/samples
					if graphPointInTriangle(px, py, ax, ay, bx, by, cx, cy) {
						inside++
					}
				}
			}
			if inside > 0 {
				graphBlendPixel(img, x, y, float64(inside)/(samples*samples), col)
			}
		}
	}
}

func graphClipLine(x0, y0, x1, y1, left, top, right, bottom float64) (float64, float64, float64, float64, bool) {
	dx, dy := x1-x0, y1-y0
	p := [4]float64{-dx, dx, -dy, dy}
	q := [4]float64{x0 - left, right - x0, y0 - top, bottom - y0}
	u1, u2 := 0.0, 1.0
	for i := 0; i < 4; i++ {
		if math.Abs(p[i]) < 1e-12 {
			if q[i] < 0 {
				return 0, 0, 0, 0, false
			}
			continue
		}
		t := q[i] / p[i]
		if p[i] < 0 {
			if t > u2 {
				return 0, 0, 0, 0, false
			}
			if t > u1 {
				u1 = t
			}
		} else {
			if t < u1 {
				return 0, 0, 0, 0, false
			}
			if t < u2 {
				u2 = t
			}
		}
	}
	return x0 + u1*dx, y0 + u1*dy, x0 + u2*dx, y0 + u2*dy, true
}

func graphDrawMinimumArrow(img *FunctionGraphImage, x0, y0, x1, y1 float64, col [4]byte) {
	if img == nil {
		return
	}
	const lineSize = 1
	ix0, iy0 := int(math.Round(x0)), int(math.Round(y0))
	ix1, iy1 := int(math.Round(x1)), int(math.Round(y1))
	head, ok := calculateArrowHead(ix0, iy0, ix1, iy1, 0, lineSize)
	if !ok {
		return
	}
	joinedX, joinedY := arrowShaftJoinedLogicalEnd(head, lineSize)
	fx0, fy0 := float64(ix0)+head.PixelOffsetX, float64(iy0)+head.PixelOffsetY
	fx1, fy1 := float64(joinedX)+head.PixelOffsetX, float64(joinedY)+head.PixelOffsetY
	half := float64(lineSize) / 2
	minX := math.Min(math.Min(fx0, fx1)-half-1, math.Min(head.Tip.X-head.TipHalfWidth, math.Min(head.SideA.X, head.SideB.X))-1)
	maxX := math.Max(math.Max(fx0, fx1)+half+1, math.Max(head.Tip.X+head.TipHalfWidth, math.Max(head.SideA.X, head.SideB.X))+1)
	minY := math.Min(math.Min(fy0, fy1)-half-1, math.Min(head.Tip.Y-head.TipHalfWidth, math.Min(head.SideA.Y, head.SideB.Y))-1)
	maxY := math.Max(math.Max(fy0, fy1)+half+1, math.Max(head.Tip.Y+head.TipHalfWidth, math.Max(head.SideA.Y, head.SideB.Y))+1)
	left := max(0, int(math.Floor(minX)))
	right := min(img.W-1, int(math.Ceil(maxX)))
	top := max(0, int(math.Floor(minY)))
	bottom := min(img.H-1, int(math.Ceil(maxY)))
	for y := top; y <= bottom; y++ {
		for x := left; x <= right; x++ {
			coverage := arrowCompositeCoverageAt(float64(x), float64(y), true, fx0, fy0, fx1, fy1, lineSize, false, head, false)
			if coverage > 0 {
				graphBlendPixel(img, x, y, coverage, col)
			}
		}
	}
}

// graphDrawExtendedPositiveAxis draws the numeric range as a plain 1 px line,
// then continues beyond the maximum before attaching the minimum arrowhead.
// The head base begins approximately 2.5% of the visible span past the maximum
// (for -10..10, around x=10.5), so the arrow no longer consumes graph range.
func graphDrawExtendedPositiveAxis(img *FunctionGraphImage, x0, y0, xMax, yMax float64, horizontal bool, col [4]byte) {
	const (
		axisWidth       = 1.0
		extensionRatio  = 0.025
		minimumHeadSpan = 12.0
	)
	graphDrawCrispHVLine(img, x0, y0, xMax, yMax, col)
	if horizontal {
		extension := math.Abs(xMax-x0) * extensionRatio
		direction := 1.0
		if xMax < x0 {
			direction = -1
		}
		tipX := xMax + direction*(extension+minimumHeadSpan)
		graphDrawMinimumArrow(img, xMax, yMax, tipX, yMax, col)
		return
	}
	extension := math.Abs(yMax-y0) * extensionRatio
	direction := 1.0
	if yMax < y0 {
		direction = -1
	}
	tipY := yMax + direction*(extension+minimumHeadSpan)
	graphDrawMinimumArrow(img, xMax, yMax, xMax, tipY, col)
}

func renderFunctionGraph(spec GraphRenderSpec) (FunctionGraphImage, error) {
	if spec.Width <= 0 {
		spec.Width = 640
	}
	if spec.Height <= 0 {
		spec.Height = 480
	}
	if spec.Width > 4096 || spec.Height > 4096 {
		return FunctionGraphImage{}, fmt.Errorf("그래프 크기가 너무 큽니다")
	}
	if !isFiniteGraphValue(spec.XMin) || !isFiniteGraphValue(spec.XMax) || spec.XMin >= spec.XMax {
		return FunctionGraphImage{}, fmt.Errorf("x 최솟값은 x 최댓값보다 작아야 합니다")
	}
	if !isFiniteGraphValue(spec.YMin) || !isFiniteGraphValue(spec.YMax) || spec.YMin >= spec.YMax {
		return FunctionGraphImage{}, fmt.Errorf("y 최솟값은 y 최댓값보다 작아야 합니다")
	}
	if spec.FunctionMode < GraphFunctionOfX || spec.FunctionMode > GraphFunctionPolar {
		return FunctionGraphImage{}, fmt.Errorf("알 수 없는 그래프 방식입니다")
	}
	expr, err := parseGraphExpression(spec.Expression)
	if err != nil {
		return FunctionGraphImage{}, err
	}
	if spec.Color == 0x01000000 {
		spec.Color = 0
	}
	img := FunctionGraphImage{W: spec.Width, H: spec.Height, Pix: make([]byte, spec.Width*spec.Height*4), Mask: make([]bool, spec.Width*spec.Height)}
	col := graphColorBGRA(spec.Color)

	// The plotting rectangle is an exact 4:3 region.  Combined with a
	// proportional y range this gives identical pixels-per-unit on both axes.
	left, top, right, bottom := graphPlotBounds(spec.Width, spec.Height)
	if right-left < 10 || bottom-top < 10 {
		return FunctionGraphImage{}, fmt.Errorf("그래프 영역이 너무 작습니다")
	}
	xRange, yRange := spec.XMax-spec.XMin, spec.YMax-spec.YMin
	mapX := func(x float64) float64 {
		return left + (x-spec.XMin)/xRange*(right-left)
	}
	mapY := func(y float64) float64 {
		return bottom - (y-spec.YMin)/yRange*(bottom-top)
	}

	if spec.ShowAxes {
		graphDrawAxisTicks(&img, spec, left, top, right, bottom, mapX, mapY, col)
		// The data range itself is a plain 1 px axis.  The minimum arrowhead is
		// attached only after an extra positive-direction extension, so xMax/yMax
		// remain visibly on the straight part of the coordinate axis.
		if spec.YMin <= 0 && spec.YMax >= 0 {
			py := mapY(0)
			graphDrawExtendedPositiveAxis(&img, left, py, right, py, true, col)
		}
		if spec.XMin <= 0 && spec.XMax >= 0 {
			px := mapX(0)
			graphDrawExtendedPositiveAxis(&img, px, bottom, px, top, false, col)
		}
	}

	drawSegment := func(x0, y0, x1, y1 float64) {
		cx0, cy0, cx1, cy1, visible := graphClipLine(x0, y0, x1, y1, left, top, right, bottom)
		if visible {
			graphDrawLine(&img, cx0, cy0, cx1, cy1, 2.0, col)
		}
	}

	switch spec.FunctionMode {
	case GraphFunctionOfX, GraphFunctionOfY:
		// Two samples per output pixel keep high-degree polynomials and
		// trigonometric curves smooth without a degree or term-count limit.
		samplePixels := spec.Width
		independentMin, independentMax := spec.XMin, spec.XMax
		dependentRange := yRange
		if spec.FunctionMode == GraphFunctionOfY {
			samplePixels = spec.Height
			independentMin, independentMax = spec.YMin, spec.YMax
			dependentRange = xRange
		}
		samples := max(samplePixels*2, 512)
		var prevIndependent, prevDependent, prevPX, prevPY float64
		prevOK := false
		for i := 0; i < samples; i++ {
			t := float64(i) / float64(samples-1)
			independent := independentMin + t*(independentMax-independentMin)
			dependent := evalGraphExpression(expr, independent, spec.AngleMode)
			mathX, mathY := independent, dependent
			if spec.FunctionMode == GraphFunctionOfY {
				mathX, mathY = dependent, independent
			}
			px, py := mapX(mathX), mapY(mathY)
			ok := isFiniteGraphValue(dependent) && isFiniteGraphValue(px) && isFiniteGraphValue(py)
			if ok && prevOK {
				midIndependent := (independent + prevIndependent) / 2
				midDependent := evalGraphExpression(expr, midIndependent, spec.AngleMode)
				discontinuous := !isFiniteGraphValue(midDependent)
				if !discontinuous {
					linearMid := (dependent + prevDependent) / 2
					if math.Abs(midDependent-linearMid) > dependentRange*0.35 || math.Abs(dependent-prevDependent) > dependentRange*1.25 {
						discontinuous = true
					}
				}
				if !discontinuous {
					drawSegment(prevPX, prevPY, px, py)
				}
			}
			if ok {
				prevIndependent, prevDependent, prevPX, prevPY, prevOK = independent, dependent, px, py, true
			} else {
				prevOK = false
			}
		}

	case GraphFunctionPolar:
		// f(r) mode treats r as the full-turn angular parameter and the entered
		// expression as the polar radius. Radian mode samples r=0..2pi; degree
		// mode samples r=0..360. Negative radii follow the ordinary polar rule.
		turn := 2 * math.Pi
		if spec.AngleMode == GraphDegrees {
			turn = 360
		}
		samples := max(max(spec.Width, spec.Height)*4, 1440)
		viewScale := math.Max(xRange, yRange)

		// When a polar expression such as sqrt(cos(2r)) reaches a domain
		// boundary, the mathematical branch can end at radius zero. Ordinary
		// sampling usually misses that exact parameter and leaves a false hole
		// around the origin. Refine finite/non-finite transitions from the finite
		// side so zero-radius branches meet at the correct point, while leaving
		// genuine asymptotes disconnected.
		boundaryFromFinite := func(finiteParam, invalidParam float64) (float64, float64, bool) {
			finite := finiteParam
			invalid := invalidParam
			radius := evalGraphExpression(expr, finite, spec.AngleMode)
			if !isFiniteGraphValue(radius) {
				return 0, 0, false
			}
			for j := 0; j < 36; j++ {
				mid := (finite + invalid) / 2
				midRadius := evalGraphExpression(expr, mid, spec.AngleMode)
				if isFiniteGraphValue(midRadius) {
					finite, radius = mid, midRadius
				} else {
					invalid = mid
				}
			}
			return finite, radius, true
		}

		var prevParam, prevRadius, prevPX, prevPY float64
		prevOK := false
		var lastSampleParam float64
		lastSampleOK := false
		for i := 0; i < samples; i++ {
			t := float64(i) / float64(samples-1)
			param := t * turn
			radius := evalGraphExpression(expr, param, spec.AngleMode)
			theta := graphAngleIn(param, spec.AngleMode)
			mathX, mathY := radius*math.Cos(theta), radius*math.Sin(theta)
			px, py := mapX(mathX), mapY(mathY)
			ok := isFiniteGraphValue(radius) && isFiniteGraphValue(px) && isFiniteGraphValue(py)
			if ok && prevOK {
				midParam := (param + prevParam) / 2
				midRadius := evalGraphExpression(expr, midParam, spec.AngleMode)
				discontinuous := !isFiniteGraphValue(midRadius)
				if !discontinuous {
					linearMid := (radius + prevRadius) / 2
					if math.Abs(midRadius-linearMid) > viewScale*0.35 || math.Abs(radius-prevRadius) > viewScale*1.25 {
						discontinuous = true
					}
				}
				if !discontinuous {
					drawSegment(prevPX, prevPY, px, py)
				}
			} else if ok && i > 0 && !lastSampleOK {
				if boundaryParam, boundaryRadius, found := boundaryFromFinite(param, lastSampleParam); found && math.Abs(boundaryRadius) <= viewScale*0.02 {
					boundaryTheta := graphAngleIn(boundaryParam, spec.AngleMode)
					boundaryPX := mapX(boundaryRadius * math.Cos(boundaryTheta))
					boundaryPY := mapY(boundaryRadius * math.Sin(boundaryTheta))
					drawSegment(boundaryPX, boundaryPY, px, py)
				}
			} else if !ok && prevOK {
				if boundaryParam, boundaryRadius, found := boundaryFromFinite(prevParam, param); found && math.Abs(boundaryRadius) <= viewScale*0.02 {
					boundaryTheta := graphAngleIn(boundaryParam, spec.AngleMode)
					boundaryPX := mapX(boundaryRadius * math.Cos(boundaryTheta))
					boundaryPY := mapY(boundaryRadius * math.Sin(boundaryTheta))
					drawSegment(prevPX, prevPY, boundaryPX, boundaryPY)
				}
			}
			if ok {
				prevParam, prevRadius, prevPX, prevPY, prevOK = param, radius, px, py, true
			} else {
				prevOK = false
			}
			lastSampleParam, lastSampleOK = param, ok
		}
	}
	return img, nil
}

func isFiniteGraphValue(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
