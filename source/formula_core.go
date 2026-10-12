package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type formulaKind int

const (
	formulaSeq formulaKind = iota
	formulaText
	formulaSpace
	formulaScript
	formulaFraction
	formulaSum
	formulaIntegral
	formulaVector
	formulaMatrix
	formulaLimit
	formulaStyled
	formulaRoot
)

type formulaTextStyle int

const (
	formulaStyleAuto formulaTextStyle = iota
	formulaStyleRoman
	formulaStyleItalic
	formulaStyleKorean
	formulaStyleBold
	formulaStyleBoldItalic
)

func mergeFormulaTextStyle(inherited, requested formulaTextStyle) formulaTextStyle {
	if requested == formulaStyleAuto {
		return inherited
	}
	if requested == formulaStyleBold && inherited == formulaStyleItalic {
		return formulaStyleBoldItalic
	}
	if requested == formulaStyleItalic && inherited == formulaStyleBold {
		return formulaStyleBoldItalic
	}
	if requested == formulaStyleBoldItalic {
		return formulaStyleBoldItalic
	}
	return requested
}

type formulaNode struct {
	Kind     formulaKind
	Text     string
	Style    formulaTextStyle
	Children []*formulaNode
	Matrix   [][]*formulaNode
}

// formulaExplicitSpaceWidth keeps HWP-style spacing independent of font
// whitespace metrics. A106 defines one backtick as two A105 backticks.
func formulaExplicitSpaceWidth(size int, marker string) int {
	size = max(size, 1)
	// cdot uses a deliberately tiny side bearing: exactly 1 px at the
	// default 20 px size, scaled for other formula sizes.
	if marker == "cdot" {
		return max(1, int((size+10)/20))
	}
	if marker == "~" {
		return max(2, size/3)
	}
	return max(2, size/3)
}

// formulaScriptVerticalLift is shared by ordinary and four-corner scripts so
// x^2/H_2 and isotope notation use exactly the same vertical phase.
func formulaScriptVerticalLift(size int) int {
	size = max(size, 1)
	return max(1, (size+2)/4) + 1
}

type formulaParser struct {
	runes []rune
	pos   int
}

// parseFormula accepts both dotPaint A100's backslash markup and the familiar
// Hancom/HWP script style used by the user: 1 over 2, x^2, H_2, sum_{i=1}^{n},
// int_0^1, vec A, and matrix{a&b#c&d}. Ordinary whitespace separates commands
// but is not painted; use ~ for a visible mathematical space.
func parseFormula(source string) *formulaNode {
	p := &formulaParser{runes: []rune(source)}
	return p.parseSequence(0)
}

func (p *formulaParser) parseSequence(stop rune) *formulaNode {
	children := make([]*formulaNode, 0, 8)
	// ordinary spaces are invisible in HWP equations, but they are important
	// syntax: one space closes the current implicit group.  segmentStart marks
	// the first AST item since that boundary.  OVER therefore uses only the
	// most recent implicit group as its numerator instead of swallowing every
	// item to the left.  Explicit {...} still returns one atom and can contain
	// spaces freely.
	segmentStart := 0
	for p.pos < len(p.runes) {
		hadSpace := p.consumeWhitespace()
		if p.pos >= len(p.runes) || (stop != 0 && p.runes[p.pos] == stop) {
			break
		}

		// Spaces around the infix keyword itself do not start a new numerator;
		// they only separate the preceding implicit group from normal following
		// atoms.  Thus both "a over b" and "... b  c over d" behave naturally.
		isOver := len(children) > 0 && p.peekDelimitedWord("over")
		if hadSpace && !isOver {
			segmentStart = len(children)
		}

		if isOver {
			p.consumeWord()
			right := p.parseOverDenominator(stop)
			if segmentStart < 0 || segmentStart >= len(children) {
				segmentStart = len(children) - 1
			}
			left := formulaSequence(append([]*formulaNode(nil), children[segmentStart:]...))
			prefix := append([]*formulaNode(nil), children[:segmentStart]...)
			frac := &formulaNode{Kind: formulaFraction, Children: []*formulaNode{left, right}}
			children = append(prefix, frac)
			segmentStart = len(prefix)
			continue
		}

		// HWP-style leading scripts: ^x or _y may stand alone, and when a
		// following atom exists they become that atom's left upper/lower scripts.
		// The special form ^n root {body} creates an n-th root.
		if p.runes[p.pos] == '^' || p.runes[p.pos] == '_' {
			empty := func() *formulaNode { return &formulaNode{Kind: formulaText, Text: ""} }
			var leftUpper, leftLower *formulaNode
			for p.pos < len(p.runes) && (p.runes[p.pos] == '^' || p.runes[p.pos] == '_') {
				op := p.runes[p.pos]
				p.pos++
				v := p.parseScriptOperand()
				if op == '^' {
					leftUpper = v
				} else {
					leftLower = v
				}
				p.skipWhitespace()
			}
			if p.peekDelimitedWord("root") {
				p.consumeWord()
				body := p.parseRequiredGroup()
				if leftUpper == nil {
					leftUpper = empty()
				}
				children = append(children, &formulaNode{Kind: formulaRoot, Children: []*formulaNode{body, leftUpper}})
				continue
			}
			base := empty()
			if p.pos < len(p.runes) && (stop == 0 || p.runes[p.pos] != stop) {
				base = p.parseAtom()
				if base != nil {
					base = p.parsePostfix(base)
				} else {
					base = empty()
				}
			}
			if leftUpper == nil {
				leftUpper = empty()
			}
			if leftLower == nil {
				leftLower = empty()
			}
			children = append(children, &formulaNode{Kind: formulaScript, Children: []*formulaNode{base, leftUpper, empty(), leftLower, empty()}})
			continue
		}

		n := p.parseAtom()
		if n == nil {
			p.pos++
			continue
		}
		n = p.parsePostfix(n)
		children = append(children, n)
	}
	return formulaSequence(children)
}

func formulaSequence(children []*formulaNode) *formulaNode {
	filtered := make([]*formulaNode, 0, len(children))
	for _, n := range children {
		if n == nil {
			continue
		}
		if n.Kind == formulaSeq {
			filtered = append(filtered, n.Children...)
		} else {
			filtered = append(filtered, n)
		}
	}
	if len(filtered) == 0 {
		return &formulaNode{Kind: formulaText, Text: ""}
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return &formulaNode{Kind: formulaSeq, Children: filtered}
}

func (p *formulaParser) consumeWhitespace() bool {
	start := p.pos
	for p.pos < len(p.runes) && unicode.IsSpace(p.runes[p.pos]) {
		p.pos++
	}
	return p.pos > start
}

func (p *formulaParser) skipWhitespace() {
	p.consumeWhitespace()
}

// parseOverDenominator implements Hancom-style unbraced OVER termination.
// Leading whitespace after OVER is ignored.  If the denominator begins with
// a brace, the whole balanced group is parsed.  Otherwise only the next
// contiguous token is consumed; the first top-level whitespace ends it.
func (p *formulaParser) parseOverDenominator(stop rune) *formulaNode {
	p.skipWhitespace()
	if p.pos >= len(p.runes) || (stop != 0 && p.runes[p.pos] == stop) {
		return &formulaNode{Kind: formulaText, Text: "□", Style: formulaStyleRoman}
	}
	if p.runes[p.pos] == '{' {
		p.pos++
		n := p.parseSequence('}')
		if p.pos < len(p.runes) && p.runes[p.pos] == '}' {
			p.pos++
		}
		return n
	}

	start := p.pos
	depth := 0
	for p.pos < len(p.runes) {
		r := p.runes[p.pos]
		if depth == 0 {
			if unicode.IsSpace(r) || (stop != 0 && r == stop) {
				break
			}
		}
		switch r {
		case '{':
			depth++
		case '}':
			if depth == 0 {
				goto done
			}
			depth--
		}
		p.pos++
	}

done:
	raw := strings.TrimSpace(string(p.runes[start:p.pos]))
	if raw == "" {
		return &formulaNode{Kind: formulaText, Text: "□", Style: formulaStyleRoman}
	}
	return parseFormula(raw)
}

func (p *formulaParser) parseAtom() *formulaNode {
	p.skipWhitespace()
	if p.pos >= len(p.runes) {
		return nil
	}
	r := p.runes[p.pos]
	if r == '{' {
		p.pos++
		n := p.parseSequence('}')
		if p.pos < len(p.runes) && p.runes[p.pos] == '}' {
			p.pos++
		}
		return n
	}
	if r == '\\' {
		p.pos++
		name := p.consumeWord()
		if name == "" {
			if p.pos < len(p.runes) {
				r := p.runes[p.pos]
				p.pos++
				return &formulaNode{Kind: formulaText, Text: string(r), Style: formulaStyleRoman}
			}
			return &formulaNode{Kind: formulaText, Text: "\\", Style: formulaStyleRoman}
		}
		return p.parseCommand(name, true)
	}
	if unicode.IsLetter(r) {
		start := p.pos
		name := p.consumeWord()
		if isFormulaCommandWord(name) {
			return p.parseCommand(name, false)
		}
		// Mathematical Latin/Greek letters are separate variables, so mc^2
		// attaches the exponent to c rather than to the whole word mc. Korean
		// prose remains a single run for correct shaping and fallback.
		if unicode.In(r, unicode.Hangul) {
			return &formulaNode{Kind: formulaText, Text: name, Style: formulaStyleAuto}
		}
		p.pos = start + 1
		return &formulaNode{Kind: formulaText, Text: string(r), Style: formulaStyleAuto}
	}

	// HWP uses ~ and ` for explicit deterministic spacing. A106 makes one `  Keep
	// equal to two A105 backticks (about 6 px at size 20). Keep these as structural spacer nodes rather than font whitespace glyphs:
	// Cambria Math/GDI may report a near-zero advance for U+2009, which made
	// {````a} look identical to {a}.  Spacer nodes have deterministic widths.
	if r == '~' {
		p.pos++
		return &formulaNode{Kind: formulaSpace, Text: "~", Style: formulaStyleRoman}
	}
	if r == '`' || r == '\'' {
		p.pos++
		return &formulaNode{Kind: formulaSpace, Text: "`", Style: formulaStyleRoman}
	}

	// Common multi-character relations and arrows.
	for _, pair := range []struct{ in, out string }{
		{"<->", "↔"}, {"->", "→"}, {"<=", "≤"}, {">=", "≥"}, {"!=", "≠"}, {"+-", "±"},
	} {
		if p.hasPrefix(pair.in) {
			p.pos += len([]rune(pair.in))
			return &formulaNode{Kind: formulaText, Text: pair.out, Style: formulaStyleRoman}
		}
	}

	p.pos++
	return &formulaNode{Kind: formulaText, Text: string(r), Style: formulaStyleAuto}
}

func (p *formulaParser) parsePostfix(base *formulaNode) *formulaNode {
	var upper, lower *formulaNode
	for {
		// Spaces before ^/_ are accepted for familiar HWP input such as
		// "int _0 ^1", but a space before any other token must remain visible
		// to parseSequence as an implicit-group boundary.
		save := p.pos
		p.skipWhitespace()
		if p.pos >= len(p.runes) || (p.runes[p.pos] != '^' && p.runes[p.pos] != '_') {
			p.pos = save
			break
		}
		op := p.runes[p.pos]
		p.pos++
		value := p.parseScriptOperand()
		if op == '^' {
			upper = value
		} else {
			lower = value
		}
	}
	if upper == nil && lower == nil {
		return base
	}

	empty := func() *formulaNode { return &formulaNode{Kind: formulaText, Text: ""} }
	switch base.Kind {
	case formulaSum, formulaIntegral:
		if len(base.Children) < 2 {
			base.Children = []*formulaNode{empty(), empty()}
		}
		if lower != nil {
			base.Children[0] = lower
		}
		if upper != nil {
			base.Children[1] = upper
		}
		return base
	case formulaLimit:
		if lower != nil {
			base.Children = []*formulaNode{lower}
		} else if upper != nil {
			base.Children = []*formulaNode{upper}
		}
		return base
	case formulaScript:
		for len(base.Children) < 5 {
			base.Children = append(base.Children, empty())
		}
		if upper != nil {
			base.Children[2] = upper
		}
		if lower != nil {
			base.Children[4] = lower
		}
		return base
	default:
		if upper == nil {
			upper = empty()
		}
		if lower == nil {
			lower = empty()
		}
		return &formulaNode{Kind: formulaScript, Children: []*formulaNode{base, empty(), upper, empty(), lower}}
	}
}

func (p *formulaParser) parseScriptOperand() *formulaNode {
	p.skipWhitespace()
	if p.pos >= len(p.runes) {
		return &formulaNode{Kind: formulaText, Text: "□", Style: formulaStyleRoman}
	}
	if p.runes[p.pos] == '{' {
		p.pos++
		n := p.parseSequence('}')
		if p.pos < len(p.runes) && p.runes[p.pos] == '}' {
			p.pos++
		}
		return n
	}
	return p.parseAtom()
}

func (p *formulaParser) parseRequiredGroup() *formulaNode {
	p.skipWhitespace()
	if p.pos >= len(p.runes) || p.runes[p.pos] != '{' {
		return &formulaNode{Kind: formulaText, Text: "□", Style: formulaStyleRoman}
	}
	p.pos++
	n := p.parseSequence('}')
	if p.pos < len(p.runes) && p.runes[p.pos] == '}' {
		p.pos++
	}
	return n
}

func (p *formulaParser) parseRawRequiredGroup() string {
	p.skipWhitespace()
	if p.pos >= len(p.runes) || p.runes[p.pos] != '{' {
		return "□"
	}
	p.pos++
	start := p.pos
	depth := 0
	for p.pos < len(p.runes) {
		r := p.runes[p.pos]
		if r == '{' {
			depth++
		} else if r == '}' {
			if depth == 0 {
				raw := string(p.runes[start:p.pos])
				p.pos++
				return raw
			}
			depth--
		}
		p.pos++
	}
	return string(p.runes[start:])
}

func (p *formulaParser) parseCommand(name string, backslash bool) *formulaNode {
	if symbol, ok := formulaGreekSymbol(name); ok {
		return &formulaNode{Kind: formulaText, Text: symbol, Style: formulaStyleAuto}
	}
	// Arrow command names follow the familiar HWP convention. The first
	// letter's case selects single-line versus double-line arrows.
	if symbol, ok := formulaArrowSymbol(name); ok {
		return &formulaNode{Kind: formulaText, Text: symbol, Style: formulaStyleRoman}
	}
	lower := strings.ToLower(name)
	empty := func() *formulaNode { return &formulaNode{Kind: formulaText, Text: ""} }
	switch lower {
	case "script", "scripts":
		children := make([]*formulaNode, 5)
		for i := range children {
			children[i] = p.parseRequiredGroup()
		}
		return &formulaNode{Kind: formulaScript, Children: children}
	case "frac", "fraction":
		return &formulaNode{Kind: formulaFraction, Children: []*formulaNode{p.parseRequiredGroup(), p.parseRequiredGroup()}}
	case "root", "sqrt":
		return &formulaNode{Kind: formulaRoot, Children: []*formulaNode{p.parseRequiredGroup(), empty()}}
	case "sum":
		if backslash && p.nextIsGroup() {
			return &formulaNode{Kind: formulaSum, Children: []*formulaNode{p.parseRequiredGroup(), p.parseRequiredGroup()}}
		}
		return &formulaNode{Kind: formulaSum, Children: []*formulaNode{empty(), empty()}}
	case "int", "integral", "oint":
		operator := "∫"
		if lower == "oint" {
			operator = "∮"
		}
		if backslash && p.nextIsGroup() {
			return &formulaNode{Kind: formulaIntegral, Text: operator, Children: []*formulaNode{p.parseRequiredGroup(), p.parseRequiredGroup()}}
		}
		return &formulaNode{Kind: formulaIntegral, Text: operator, Children: []*formulaNode{empty(), empty()}}
	case "lim", "limit":
		if backslash && p.nextIsGroup() {
			return &formulaNode{Kind: formulaLimit, Children: []*formulaNode{p.parseRequiredGroup()}}
		}
		return &formulaNode{Kind: formulaLimit, Children: []*formulaNode{empty()}}
	case "vec", "vector":
		var body *formulaNode
		if backslash && p.nextIsGroup() {
			body = p.parseRequiredGroup()
		} else {
			body = p.parseScriptOperand()
		}
		return &formulaNode{Kind: formulaVector, Children: []*formulaNode{body}}
	case "matrix", "mat", "pmatrix", "bmatrix", "dmatrix":
		raw := p.parseRawRequiredGroup()
		return parseFormulaMatrixRaw(raw)
	case "rm", "roman":
		return &formulaNode{Kind: formulaStyled, Style: formulaStyleRoman, Children: []*formulaNode{p.parseScriptOperand()}}
	case "it", "italic":
		return &formulaNode{Kind: formulaStyled, Style: formulaStyleItalic, Children: []*formulaNode{p.parseScriptOperand()}}
	case "bf", "bold":
		return &formulaNode{Kind: formulaStyled, Style: formulaStyleBold, Children: []*formulaNode{p.parseScriptOperand()}}
	case "sin", "cos", "tan", "sec", "csc", "cosec", "cot", "arcsin", "arccos", "arctan", "sinh", "cosh", "tanh", "coth", "log", "ln", "lg", "max", "min", "exp", "det", "mod", "gcd":
		return &formulaNode{Kind: formulaText, Text: lower, Style: formulaStyleRoman}
	case "times":
		return &formulaNode{Kind: formulaText, Text: "×", Style: formulaStyleRoman}
	case "div":
		return &formulaNode{Kind: formulaText, Text: "÷", Style: formulaStyleRoman}
	case "cdot":
		// One-pixel side bearings at 20 px keep the centred dot readable without
		// giving it the much wider spacing of a normal HWP backtick.
		return formulaSequence([]*formulaNode{
			{Kind: formulaSpace, Text: "cdot", Style: formulaStyleRoman},
			{Kind: formulaText, Text: "·", Style: formulaStyleRoman},
			{Kind: formulaSpace, Text: "cdot", Style: formulaStyleRoman},
		})
	case "approx":
		return &formulaNode{Kind: formulaText, Text: "≈", Style: formulaStyleRoman}
	case "del":
		return &formulaNode{Kind: formulaText, Text: "∇", Style: formulaStyleRoman}
	case "round":
		return &formulaNode{Kind: formulaText, Text: "∂", Style: formulaStyleRoman}
	case "therefore":
		return &formulaNode{Kind: formulaText, Text: "∴", Style: formulaStyleRoman}
	case "because":
		return &formulaNode{Kind: formulaText, Text: "∵", Style: formulaStyleRoman}
	case "hbar":
		return &formulaNode{Kind: formulaText, Text: "ℏ", Style: formulaStyleRoman}
	case "union":
		return &formulaNode{Kind: formulaText, Text: "∪", Style: formulaStyleRoman}
	case "inter":
		return &formulaNode{Kind: formulaText, Text: "∩", Style: formulaStyleRoman}
	case "inf", "infty", "infinity":
		return &formulaNode{Kind: formulaText, Text: "∞", Style: formulaStyleRoman}
	default:
		return &formulaNode{Kind: formulaText, Text: name, Style: formulaStyleAuto}
	}
}

func (p *formulaParser) nextIsGroup() bool {
	save := p.pos
	p.skipWhitespace()
	ok := p.pos < len(p.runes) && p.runes[p.pos] == '{'
	p.pos = save
	return ok
}

func parseFormulaMatrixRaw(raw string) *formulaNode {
	rows := splitFormulaTopLevelAny(raw, "#;")
	matrix := make([][]*formulaNode, 0, len(rows))
	for _, row := range rows {
		cells := splitFormulaTopLevelAny(row, "&,")
		parsed := make([]*formulaNode, 0, len(cells))
		for _, cell := range cells {
			parsed = append(parsed, parseFormula(strings.TrimSpace(cell)))
		}
		if len(parsed) == 0 {
			parsed = append(parsed, &formulaNode{Kind: formulaText, Text: "□", Style: formulaStyleRoman})
		}
		matrix = append(matrix, parsed)
	}
	if len(matrix) == 0 {
		matrix = [][]*formulaNode{{{Kind: formulaText, Text: "□", Style: formulaStyleRoman}}}
	}
	return &formulaNode{Kind: formulaMatrix, Matrix: matrix}
}

func formulaGreekSymbol(name string) (string, bool) {
	lower := strings.ToLower(name)
	pair, ok := map[string][2]string{
		"alpha": {"α", "Α"}, "beta": {"β", "Β"}, "gamma": {"γ", "Γ"},
		"delta": {"δ", "Δ"}, "epsilon": {"ε", "Ε"}, "zeta": {"ζ", "Ζ"},
		"eta": {"η", "Η"}, "theta": {"θ", "Θ"}, "iota": {"ι", "Ι"},
		"kappa": {"κ", "Κ"}, "lambda": {"λ", "Λ"}, "mu": {"μ", "Μ"},
		"nu": {"ν", "Ν"}, "xi": {"ξ", "Ξ"}, "omicron": {"ο", "Ο"},
		"pi": {"π", "Π"}, "rho": {"ρ", "Ρ"}, "sigma": {"σ", "Σ"},
		"tau": {"τ", "Τ"}, "upsilon": {"υ", "Υ"}, "phi": {"φ", "Φ"},
		"chi": {"χ", "Χ"}, "psi": {"ψ", "Ψ"}, "omega": {"ω", "Ω"},
	}[lower]
	if !ok || name == "" {
		return "", false
	}
	first, _ := utf8.DecodeRuneInString(name)
	if unicode.IsUpper(first) {
		return pair[1], true
	}
	return pair[0], true
}

func formulaArrowSymbol(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	double := unicode.IsUpper([]rune(name)[0])
	key := strings.ToLower(name)
	single := map[string]string{
		"arrow": "→", "rarrow": "→", "rightarrow": "→",
		"larrow": "←", "leftarrow": "←", "uarrow": "↑", "uparrow": "↑",
		"darrow": "↓", "downarrow": "↓", "lrarrow": "↔", "leftrightarrow": "↔",
		"udarrow": "↕", "updownarrow": "↕", "nwarrow": "↖", "nearrow": "↗",
		"searrow": "↘", "swarrow": "↙",
	}
	dbl := map[string]string{
		"arrow": "⇒", "rarrow": "⇒", "rightarrow": "⇒",
		"larrow": "⇐", "leftarrow": "⇐", "uarrow": "⇑", "uparrow": "⇑",
		"darrow": "⇓", "downarrow": "⇓", "lrarrow": "⇔", "leftrightarrow": "⇔",
		"udarrow": "⇕", "updownarrow": "⇕", "nwarrow": "⇖", "nearrow": "⇗",
		"searrow": "⇘", "swarrow": "⇙",
	}
	if double {
		v, ok := dbl[key]
		return v, ok
	}
	v, ok := single[key]
	return v, ok
}

func isFormulaCommandWord(s string) bool {
	if _, ok := formulaArrowSymbol(s); ok {
		return true
	}
	if _, ok := formulaGreekSymbol(s); ok {
		return true
	}
	switch strings.ToLower(s) {
	case "script", "scripts", "frac", "fraction", "root", "sqrt", "sum", "int", "integral", "oint", "lim", "limit", "vec", "vector", "matrix", "mat", "pmatrix", "bmatrix", "dmatrix", "rm", "roman", "it", "italic", "bf", "bold", "sin", "cos", "tan", "sec", "csc", "cosec", "cot", "arcsin", "arccos", "arctan", "sinh", "cosh", "tanh", "coth", "log", "ln", "lg", "max", "min", "exp", "det", "mod", "gcd", "times", "div", "cdot", "approx", "del", "round", "therefore", "because", "hbar", "union", "inter", "inf", "infty", "infinity":
		return true
	}
	return false
}

func (p *formulaParser) peekDelimitedWord(want string) bool {
	if p.pos >= len(p.runes) || !unicode.IsLetter(p.runes[p.pos]) {
		return false
	}
	i := p.pos
	for i < len(p.runes) && unicode.IsLetter(p.runes[i]) {
		i++
	}
	return strings.EqualFold(string(p.runes[p.pos:i]), want)
}

func (p *formulaParser) consumeWord() string {
	start := p.pos
	for p.pos < len(p.runes) && unicode.IsLetter(p.runes[p.pos]) {
		p.pos++
	}
	return string(p.runes[start:p.pos])
}

func (p *formulaParser) hasPrefix(s string) bool {
	r := []rune(s)
	if p.pos+len(r) > len(p.runes) {
		return false
	}
	for i := range r {
		if p.runes[p.pos+i] != r[i] {
			return false
		}
	}
	return true
}

func splitFormulaTopLevel(source string, sep rune) []string {
	return splitFormulaTopLevelAny(source, string(sep))
}

func splitFormulaTopLevelAny(source, separators string) []string {
	runes := []rune(source)
	depth := 0
	start := 0
	out := make([]string, 0, 4)
	isSep := func(r rune) bool { return strings.ContainsRune(separators, r) }
	for i, r := range runes {
		switch r {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		default:
			if isSep(r) && depth == 0 {
				out = append(out, string(runes[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, string(runes[start:]))
	return out
}

func formulaNodePlainText(n *formulaNode) string {
	if n == nil {
		return ""
	}
	switch n.Kind {
	case formulaText:
		return n.Text
	case formulaSpace:
		return " "
	case formulaSeq:
		var b strings.Builder
		for _, c := range n.Children {
			b.WriteString(formulaNodePlainText(c))
		}
		return b.String()
	case formulaScript:
		if len(n.Children) > 0 {
			return formulaNodePlainText(n.Children[0])
		}
	case formulaFraction:
		if len(n.Children) >= 2 {
			return formulaNodePlainText(n.Children[0]) + "/" + formulaNodePlainText(n.Children[1])
		}
	case formulaSum:
		return "∑"
	case formulaIntegral:
		if n.Text != "" {
			return n.Text
		}
		return "∫"
	case formulaVector:
		if len(n.Children) > 0 {
			return formulaNodePlainText(n.Children[0])
		}
	case formulaLimit:
		return "lim"
	case formulaMatrix:
		return "matrix"
	case formulaRoot:
		if len(n.Children) > 0 {
			return "√" + formulaNodePlainText(n.Children[0])
		}
		return "√"
	case formulaStyled:
		if len(n.Children) > 0 {
			return formulaNodePlainText(n.Children[0])
		}
	}
	return ""
}
