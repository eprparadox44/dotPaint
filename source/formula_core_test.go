package main

import "testing"

func TestFormulaParserFourWayScripts(t *testing.T) {
	n := parseFormula(`\script{C}{12}{q}{6}{n}`)
	if n.Kind != formulaScript || len(n.Children) != 5 {
		t.Fatalf("unexpected script node: %#v", n)
	}
	want := []string{"C", "12", "q", "6", "n"}
	for i, w := range want {
		if got := formulaNodePlainText(n.Children[i]); got != w {
			t.Fatalf("child %d = %q, want %q", i, got, w)
		}
	}
}

func TestFormulaParserFractionAndVector(t *testing.T) {
	n := parseFormula(`\frac{a+b}{\vec{v}}`)
	if n.Kind != formulaFraction || len(n.Children) != 2 {
		t.Fatalf("unexpected fraction node: %#v", n)
	}
	if got := formulaNodePlainText(n); got != "a+b/v" {
		t.Fatalf("plain text = %q", got)
	}
}

func TestFormulaParserMatrixNestedGroups(t *testing.T) {
	n := parseFormula(`\matrix{a,\frac{b}{c};d,e}`)
	if n.Kind != formulaMatrix || len(n.Matrix) != 2 || len(n.Matrix[0]) != 2 || len(n.Matrix[1]) != 2 {
		t.Fatalf("unexpected matrix shape: %#v", n)
	}
	if n.Matrix[0][1].Kind != formulaFraction {
		t.Fatalf("nested fraction lost: %#v", n.Matrix[0][1])
	}
}

func TestSplitFormulaTopLevel(t *testing.T) {
	got := splitFormulaTopLevel(`a,{b,c},d`, ',')
	if len(got) != 3 || got[1] != "{b,c}" {
		t.Fatalf("split = %#v", got)
	}
}

func TestFormulaParserHWPOver(t *testing.T) {
	n := parseFormula(`{a+b} over {c-d}`)
	if n.Kind != formulaFraction || len(n.Children) != 2 {
		t.Fatalf("unexpected HWP over node: %#v", n)
	}
	if got := formulaNodePlainText(n); got != "a+b/c-d" {
		t.Fatalf("plain text = %q", got)
	}
}

func TestFormulaParserHWPPostfixScripts(t *testing.T) {
	n := parseFormula(`E=mc^2`)
	if n.Kind != formulaSeq || len(n.Children) < 4 {
		t.Fatalf("unexpected sequence: %#v", n)
	}
	last := n.Children[len(n.Children)-1]
	if last.Kind != formulaScript || formulaNodePlainText(last.Children[0]) != "c" || formulaNodePlainText(last.Children[2]) != "2" {
		t.Fatalf("superscript lost: %#v", last)
	}

	n2 := parseFormula(`H_2 O`)
	if n2.Kind != formulaSeq || n2.Children[0].Kind != formulaScript || formulaNodePlainText(n2.Children[0].Children[4]) != "2" {
		t.Fatalf("subscript lost: %#v", n2)
	}
}

func TestFormulaParserHWPLargeOperators(t *testing.T) {
	n := parseFormula(`sum_{i=1}^{n}`)
	if n.Kind != formulaSum || formulaNodePlainText(n.Children[0]) != "i=1" || formulaNodePlainText(n.Children[1]) != "n" {
		t.Fatalf("sum limits lost: %#v", n)
	}
	n = parseFormula(`int _0 ^1 x dx`)
	if n.Kind != formulaSeq || n.Children[0].Kind != formulaIntegral {
		t.Fatalf("integral parse = %#v", n)
	}
}

func TestFormulaParserHWPMatrix(t *testing.T) {
	n := parseFormula(`matrix{a&b#c&d}`)
	if n.Kind != formulaMatrix || len(n.Matrix) != 2 || len(n.Matrix[0]) != 2 || len(n.Matrix[1]) != 2 {
		t.Fatalf("matrix shape = %#v", n)
	}
}

func TestFormulaParserRomanItalicCommands(t *testing.T) {
	n := parseFormula(`rm {ABC} it {xyz}`)
	if n.Kind != formulaSeq || len(n.Children) != 2 || n.Children[0].Kind != formulaStyled || n.Children[0].Style != formulaStyleRoman || n.Children[1].Style != formulaStyleItalic {
		t.Fatalf("style commands = %#v", n)
	}
}

func TestFormulaParserHWPOverEndsAtSpace(t *testing.T) {
	n := parseFormula(`-ih over 2m script{U}{235}{2+}{92}{2}`)
	if n.Kind != formulaSeq || len(n.Children) != 2 {
		t.Fatalf("unexpected sequence after OVER termination: %#v", n)
	}
	frac := n.Children[0]
	if frac.Kind != formulaFraction || formulaNodePlainText(frac.Children[0]) != "-ih" || formulaNodePlainText(frac.Children[1]) != "2m" {
		t.Fatalf("fraction did not end at whitespace: %#v", frac)
	}
	if n.Children[1].Kind != formulaScript || formulaNodePlainText(n.Children[1].Children[0]) != "U" {
		t.Fatalf("following script was swallowed by denominator: %#v", n.Children[1])
	}
}

func TestFormulaParserHWPOverBracedDenominatorCanContainSpaces(t *testing.T) {
	n := parseFormula(`a over {2 m + 3} x`)
	if n.Kind != formulaSeq || len(n.Children) != 2 {
		t.Fatalf("unexpected braced OVER sequence: %#v", n)
	}
	frac := n.Children[0]
	if frac.Kind != formulaFraction || formulaNodePlainText(frac.Children[1]) != "2m+3" {
		t.Fatalf("braced denominator was not kept together: %#v", frac)
	}
	if formulaNodePlainText(n.Children[1]) != "x" {
		t.Fatalf("trailing expression lost: %#v", n.Children[1])
	}
}

func TestFormulaParserHWPWhitespaceStartsNewOverNumerator(t *testing.T) {
	n := parseFormula(`{script{rm {H}}{1}{2}{3}{4} } over 2m 1_2 over 2^1_2`)
	if n.Kind != formulaSeq || len(n.Children) != 2 {
		t.Fatalf("expected two side-by-side fractions, got %#v", n)
	}
	first := n.Children[0]
	second := n.Children[1]
	if first.Kind != formulaFraction || second.Kind != formulaFraction {
		t.Fatalf("expected fraction/fraction, got %#v", n.Children)
	}
	if got := formulaNodePlainText(first.Children[1]); got != "2m" {
		t.Fatalf("first denominator = %q, want 2m", got)
	}
	if second.Children[0].Kind != formulaScript || formulaNodePlainText(second.Children[0].Children[0]) != "1" || formulaNodePlainText(second.Children[0].Children[4]) != "2" {
		t.Fatalf("second numerator must be only 1_2: %#v", second.Children[0])
	}
	if second.Children[1].Kind != formulaScript || formulaNodePlainText(second.Children[1].Children[0]) != "2" || formulaNodePlainText(second.Children[1].Children[2]) != "1" || formulaNodePlainText(second.Children[1].Children[4]) != "2" {
		t.Fatalf("second denominator must be 2^1_2: %#v", second.Children[1])
	}
}

func TestFormulaParserOrdinarySpaceEndsImplicitGroup(t *testing.T) {
	n := parseFormula(`a b over c`)
	if n.Kind != formulaSeq || len(n.Children) != 2 || n.Children[1].Kind != formulaFraction {
		t.Fatalf("expected a followed by b/c, got %#v", n)
	}
	if got := formulaNodePlainText(n.Children[0]); got != "a" {
		t.Fatalf("prefix = %q, want a", got)
	}
	if got := formulaNodePlainText(n.Children[1].Children[0]); got != "b" {
		t.Fatalf("fraction numerator = %q, want b", got)
	}
}

func TestFormulaParserBackticksBecomeExplicitSpaces(t *testing.T) {
	n := parseFormula("{````a}")
	if n.Kind != formulaSeq || len(n.Children) != 5 {
		t.Fatalf("expected four spacer nodes plus a, got %#v", n)
	}
	for i := 0; i < 4; i++ {
		if n.Children[i].Kind != formulaSpace || n.Children[i].Text != "`" {
			t.Fatalf("child %d is not a thin spacer: %#v", i, n.Children[i])
		}
	}
	if formulaNodePlainText(n.Children[4]) != "a" {
		t.Fatalf("trailing symbol lost: %#v", n.Children[4])
	}
}

func TestFormulaParserGreekNamesRespectInitialCase(t *testing.T) {
	n := parseFormula(`alpha Alpha beta Beta omega Omega`)
	if got := formulaNodePlainText(n); got != "αΑβΒωΩ" {
		t.Fatalf("Greek expansion = %q", got)
	}
}

func TestA106ExplicitSpaceWidth(t *testing.T) {
	if got := formulaExplicitSpaceWidth(20, "`"); got != 6 {
		t.Fatalf("backtick width = %d, want 6", got)
	}
	if got := formulaExplicitSpaceWidth(20, "~"); got != 6 {
		t.Fatalf("tilde width = %d, want 6", got)
	}
}

func TestA106OrdinaryAndFourWayScriptsShareLift(t *testing.T) {
	if got := formulaScriptVerticalLift(20); got != 6 {
		t.Fatalf("script lift = %d, want 6", got)
	}
}

func TestA107ArrowCommandAliases(t *testing.T) {
	n := parseFormula(`approx arrow rarrow uarrow larrow darrow Arrow Rarrow Uarrow Larrow Darrow lrarrow LRarrow`)
	if got := formulaNodePlainText(n); got != "≈→→↑←↓⇒⇒⇑⇐⇓↔⇔" {
		t.Fatalf("arrow expansion = %q", got)
	}
}

func TestA107RootAndNthRoot(t *testing.T) {
	sq := parseFormula(`root {x+1}`)
	if sq.Kind != formulaRoot || formulaNodePlainText(sq.Children[0]) != "x+1" {
		t.Fatalf("square root = %#v", sq)
	}
	nth := parseFormula(`^3 root {y}`)
	if nth.Kind != formulaRoot || formulaNodePlainText(nth.Children[1]) != "3" {
		t.Fatalf("nth root = %#v", nth)
	}
}

func TestA107LeadingScriptsAttachToFollowingBase(t *testing.T) {
	n := parseFormula(`^x_y A`)
	if n.Kind != formulaScript || len(n.Children) != 5 {
		t.Fatalf("leading script = %#v", n)
	}
	if formulaNodePlainText(n.Children[0]) != "A" || formulaNodePlainText(n.Children[1]) != "x" || formulaNodePlainText(n.Children[3]) != "y" {
		t.Fatalf("leading script placement = %#v", n.Children)
	}
}

func TestA109AdditionalHWPCommandAliases(t *testing.T) {
	n := parseFormula(`Del round div cdot therefore because oint`)
	if got := formulaNodePlainText(n); got != "∇∂÷ · ∴∵∮" {
		t.Fatalf("additional command expansion = %q", got)
	}
	if n.Kind != formulaSeq {
		t.Fatalf("expected command sequence, got %#v", n)
	}
	last := n.Children[len(n.Children)-1]
	if last.Kind != formulaIntegral || last.Text != "∮" {
		t.Fatalf("oint did not create contour integral: %#v", last)
	}
}

func TestA109CdotUsesOnePixelSideBearingsAt20(t *testing.T) {
	if got := formulaExplicitSpaceWidth(20, "cdot"); got != 1 {
		t.Fatalf("cdot side bearing=%d want 1", got)
	}
	n := parseFormula(`a cdot b`)
	if n.Kind != formulaSeq {
		t.Fatalf("cdot sequence = %#v", n)
	}
	spaces := 0
	for _, child := range n.Children {
		if child.Kind == formulaSpace && child.Text == "cdot" {
			spaces++
		}
	}
	if spaces != 2 {
		t.Fatalf("cdot spacer count=%d want 2; %#v", spaces, n.Children)
	}
}

func TestA110HbarCommand(t *testing.T) {
	n := parseFormula(`hbar over 2m`)
	if got := formulaNodePlainText(n); got != "ℏ/2m" {
		t.Fatalf("hbar expansion = %q", got)
	}
}

func TestA115FormulaParserBoldCommand(t *testing.T) {
	n := parseFormula(`bold {ABC}`)
	if n.Kind != formulaStyled || n.Style != formulaStyleBold || formulaNodePlainText(n) != "ABC" {
		t.Fatalf("bold command = %#v", n)
	}
}

func TestA115MergeBoldItalicStyle(t *testing.T) {
	if got := mergeFormulaTextStyle(formulaStyleItalic, formulaStyleBold); got != formulaStyleBoldItalic {
		t.Fatalf("italic + bold = %v", got)
	}
	if got := mergeFormulaTextStyle(formulaStyleBold, formulaStyleItalic); got != formulaStyleBoldItalic {
		t.Fatalf("bold + italic = %v", got)
	}
}
