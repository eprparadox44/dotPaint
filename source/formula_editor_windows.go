//go:build windows

package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"syscall"
	"unicode"
	"unsafe"
)

const (
	formulaEditorClassName = "dotPaintFormulaEditorA122"

	formulaIDInput = 7101 + iota
	formulaIDSize
	formulaIDInsert
	formulaIDCancel
	formulaIDCopyPreview
	formulaIDCopyScript
	formulaIDBold
	formulaIDItalic
	formulaIDInvert
	formulaIDCategoryScripts
	formulaIDCategoryFraction
	formulaIDCategoryLargeOps
	formulaIDCategoryVector
	formulaIDCategoryMatrix
	formulaIDCategoryTrig
	formulaIDCategorySigns
	formulaIDCategoryRelations
	formulaIDCategorySets
	formulaIDCategoryArrows
	formulaIDCategoryGreek
	formulaIDCategoryExp
	formulaIDCategoryLog
	formulaIDPi
	formulaIDEuler
)

const (
	formulaEMGetSel      = 0x00B0
	formulaEMReplaceSel  = 0x00C2
	formulaCBNSelection  = 1
	formulaSWRestore     = 9
	formulaWMHotKey      = 0x0312
	formulaModShift      = 0x0004
	formulaHotKeyInsert  = 1
	formulaCFUnicodeText = 13
	formulaBSOwnerDraw   = 0x0000000B
	formulaODTButton     = 4
)

var (
	formulaProcGetWindowRect     = modUser32.NewProc("GetWindowRect")
	formulaProcRegisterHotKey    = modUser32.NewProc("RegisterHotKey")
	formulaProcUnregisterHotKey  = modUser32.NewProc("UnregisterHotKey")
	formulaEditorWndProcCallback uintptr
	formulaEditorClassRegistered bool
	activeFormulaEditor          *formulaEditorState
)

type formulaEditorState struct {
	paintBuffer      previewEditorPaintBuffer
	owner            *App
	hwnd             uintptr
	input            uintptr
	sizeCombo        uintptr
	insertBtn        uintptr
	cancelBtn        uintptr
	copyPreviewBtn   uintptr
	copyScriptBtn    uintptr
	boldBtn          uintptr
	italicBtn        uintptr
	invertBtn        uintptr
	invert           bool
	uiFont           uintptr
	compactBtnFont   uintptr
	boldButtonFont   uintptr
	italicButtonFont uintptr
	controls         []uintptr
	categoryButtons  map[int]uintptr
	fontSize         int
	preview          ClipboardImage
	previewRect      RECT
	lastSource       string
}

type formulaCategorySpec struct {
	id         int
	label      string
	directText string
	w          int
}

type formulaPopupEntry struct {
	label     string
	text      string
	children  []formulaPopupEntry
	separator bool
}

// A106: one compact symbolic button per category.  The labels intentionally
// use representative formula glyphs rather than Korean category names.
var formulaSizeChoices = []int{10, 12, 16, 20, 24, 28, 32, 36, 48, 64, 72}

var formulaToolbarCategories = []formulaCategorySpec{
	{formulaIDCategoryScripts, "", "", 68},
	{formulaIDCategoryFraction, "a⁄b", `{□} over {□}`, 48},
	{formulaIDCategoryLargeOps, "lim ∑ ∫", "", 72},
	{formulaIDCategoryVector, "", `vec {□}`, 48},
	{formulaIDCategoryMatrix, "[ ]", `matrix{a&b#c&d}`, 48},
	{formulaIDCategoryTrig, "sin", "", 48},
	{formulaIDCategorySigns, "±", "", 42},
	{formulaIDCategoryRelations, "=", "", 42},
	{formulaIDCategorySets, "∈", "", 42},
	{formulaIDCategoryArrows, "→", "", 42},
	{formulaIDCategoryGreek, "α", "", 42},
	{formulaIDCategoryExp, "eˣ", "", 48},
	{formulaIDCategoryLog, "log", "", 48},
	{formulaIDPi, "π", "π", 42},
	{formulaIDEuler, "e", "e", 42},
}

func formulaPopupEntries(categoryID int) []formulaPopupEntry {
	switch categoryID {
	case formulaIDCategoryScripts:
		return []formulaPopupEntry{
			{label: "ⁿX", text: `script{rm {□}}{□}{}{}{}`},
			{label: "Xⁿ", text: `script{rm {□}}{}{□}{}{}`},
			{label: "ₙX", text: `script{rm {□}}{}{}{□}{}`},
			{label: "Xₙ", text: `script{rm {□}}{}{}{}{□}`},
			{label: "ⁿₘXᵖ_q", text: `script{rm {□}}{□}{□}{□}{□}`},
		}
	case formulaIDCategoryLargeOps:
		return []formulaPopupEntry{
			{label: "lim", text: `lim _{x->0}`},
			{label: "∑", text: `sum _{i=1} ^{n}`},
			{label: "∫", text: `int _{a} ^{b}`},
			{label: "∮", text: `oint _{a} ^{b}`},
		}
	case formulaIDCategoryTrig:
		return []formulaPopupEntry{
			{label: "sin", text: "sin "}, {label: "cos", text: "cos "},
			{label: "tan", text: "tan "}, {label: "sec", text: "sec "},
			{label: "csc", text: "csc "}, {label: "cot", text: "cot "},
		}
	case formulaIDCategoryExp:
		return []formulaPopupEntry{
			{label: "eˣ", text: `script{rm {e}}{}{□}{}{}`},
			{label: "exp(x)", text: `exp {□}`},
		}
	case formulaIDCategoryLog:
		return []formulaPopupEntry{
			{label: "log(x)", text: `log {□}`},
			{label: "ln(x)", text: `ln {□}`},
			{label: "lg(x)", text: `lg {□}`},
			{label: "logₐ(x)", text: `script{rm {log}}{}{}{}{□}~{□}`},
		}
	case formulaIDCategorySigns:
		return []formulaPopupEntry{
			{label: "+", text: "+"}, {label: "−", text: "−"},
			{label: "×", text: "×"}, {label: "÷", text: "div"},
			{label: "·", text: "cdot"}, {label: "±", text: "±"},
			{label: "∓", text: "∓"}, {label: "√", text: `root {□}`}, {label: "ⁿ√", text: `^n root {□}`},
			{label: "∇", text: "Del"}, {label: "∂", text: "round"},
			{label: "∴", text: "therefore"}, {label: "∵", text: "because"},
			{label: "ℏ", text: "hbar"}, {label: "∞", text: "∞"},
		}
	case formulaIDCategoryRelations:
		return []formulaPopupEntry{
			{label: "=", text: "="}, {label: "≠", text: "≠"},
			{label: "<", text: "<"}, {label: ">", text: ">"},
			{label: "≤", text: "≤"}, {label: "≥", text: "≥"},
			{label: "≈", text: "approx"}, {label: "≡", text: "≡"},
			{label: "∝", text: "∝"},
		}
	case formulaIDCategorySets:
		return []formulaPopupEntry{
			{label: "∈", text: "∈"}, {label: "∉", text: "∉"},
			{label: "∋", text: "∋"}, {label: "∌", text: "∌"},
			{label: "⊂", text: "⊂"}, {label: "⊆", text: "⊆"},
			{label: "⊃", text: "⊃"}, {label: "⊇", text: "⊇"},
			{label: "∪", text: "∪"}, {label: "∩", text: "∩"},
			{label: "∅", text: "∅"}, {separator: true},
			{label: "ℕ", text: "ℕ"}, {label: "ℤ", text: "ℤ"},
			{label: "ℚ", text: "ℚ"}, {label: "ℝ", text: "ℝ"},
			{label: "ℂ", text: "ℂ"},
		}
	case formulaIDCategoryArrows:
		return []formulaPopupEntry{
			{label: "→", children: []formulaPopupEntry{
				{label: "←", text: "←"}, {label: "↑", text: "↑"},
				{label: "→", text: "→"}, {label: "↓", text: "↓"},
				{label: "↖", text: "↖"}, {label: "↗", text: "↗"},
				{label: "↘", text: "↘"}, {label: "↙", text: "↙"},
				{separator: true},
				{label: "↔", text: "↔"}, {label: "↕", text: "↕"},
				{label: "⤡", text: "⤡"}, {label: "⤢", text: "⤢"},
			}},
			{label: "⇒", children: []formulaPopupEntry{
				{label: "⇐", text: "⇐"}, {label: "⇑", text: "⇑"},
				{label: "⇒", text: "⇒"}, {label: "⇓", text: "⇓"},
				{label: "⇖", text: "⇖"}, {label: "⇗", text: "⇗"},
				{label: "⇘", text: "⇘"}, {label: "⇙", text: "⇙"},
				{separator: true},
				{label: "⇔", text: "⇔"}, {label: "⇕", text: "⇕"},
			}},
		}
	case formulaIDCategoryGreek:
		lower1 := []formulaPopupEntry{{label: "α", text: "α"}, {label: "β", text: "β"}, {label: "γ", text: "γ"}, {label: "δ", text: "δ"}, {label: "ε", text: "ε"}, {label: "ζ", text: "ζ"}, {label: "η", text: "η"}, {label: "θ", text: "θ"}, {label: "ι", text: "ι"}, {label: "κ", text: "κ"}, {label: "λ", text: "λ"}, {label: "μ", text: "μ"}}
		lower2 := []formulaPopupEntry{{label: "ν", text: "ν"}, {label: "ξ", text: "ξ"}, {label: "ο", text: "ο"}, {label: "π", text: "π"}, {label: "ρ", text: "ρ"}, {label: "σ", text: "σ"}, {label: "τ", text: "τ"}, {label: "υ", text: "υ"}, {label: "φ", text: "φ"}, {label: "χ", text: "χ"}, {label: "ψ", text: "ψ"}, {label: "ω", text: "ω"}}
		upper1 := []formulaPopupEntry{{label: "Α", text: "Α"}, {label: "Β", text: "Β"}, {label: "Γ", text: "Γ"}, {label: "Δ", text: "Δ"}, {label: "Ε", text: "Ε"}, {label: "Ζ", text: "Ζ"}, {label: "Η", text: "Η"}, {label: "Θ", text: "Θ"}, {label: "Ι", text: "Ι"}, {label: "Κ", text: "Κ"}, {label: "Λ", text: "Λ"}, {label: "Μ", text: "Μ"}}
		upper2 := []formulaPopupEntry{{label: "Ν", text: "Ν"}, {label: "Ξ", text: "Ξ"}, {label: "Ο", text: "Ο"}, {label: "Π", text: "Π"}, {label: "Ρ", text: "Ρ"}, {label: "Σ", text: "Σ"}, {label: "Τ", text: "Τ"}, {label: "Υ", text: "Υ"}, {label: "Φ", text: "Φ"}, {label: "Χ", text: "Χ"}, {label: "Ψ", text: "Ψ"}, {label: "Ω", text: "Ω"}}
		return []formulaPopupEntry{
			{label: "α…μ", children: lower1}, {label: "ν…ω", children: lower2},
			{label: "Α…Μ", children: upper1}, {label: "Ν…Ω", children: upper2},
		}
	}
	return nil
}

type formulaPlacedLayout struct {
	layout *formulaLayout
	x, y   int
}

type formulaLinePrimitive struct {
	x1, y1, x2, y2 int
	width          int
}

type formulaInkBounds struct {
	left, top, right, bottom int // right/bottom are exclusive
}

func (b formulaInkBounds) width() int  { return max(0, b.right-b.left) }
func (b formulaInkBounds) height() int { return max(0, b.bottom-b.top) }

type formulaMathClass int

const (
	formulaClassOrd formulaMathClass = iota
	formulaClassFunction
	formulaClassBinary
	formulaClassRelation
	formulaClassOpen
	formulaClassClose
	formulaClassPunctuation
	formulaClassSpace
	formulaClassLargeOperator
)

type formulaLayout struct {
	w, h, baseline int
	text           string
	fontSize       int
	fontStyle      formulaTextStyle
	mathClass      formulaMathClass
	complex        bool
	children       []formulaPlacedLayout
	lines          []formulaLinePrimitive
}

type formulaFontKey struct {
	size  int
	style formulaTextStyle
}

type formulaFontCache struct {
	fonts map[formulaFontKey]uintptr
}

func newFormulaFontCache() *formulaFontCache {
	return &formulaFontCache{fonts: make(map[formulaFontKey]uintptr)}
}
func (c *formulaFontCache) close() {
	for _, h := range c.fonts {
		if h != 0 {
			procDeleteObject.Call(h)
		}
	}
}
func (c *formulaFontCache) get(size int, style formulaTextStyle) uintptr {
	size = max(size, 7)
	if style == formulaStyleAuto {
		style = formulaStyleRoman
	}
	key := formulaFontKey{size: size, style: style}
	if h := c.fonts[key]; h != 0 {
		return h
	}
	face := "Cambria Math"
	bold := false
	italic := false
	if style == formulaStyleItalic {
		face = "Cambria"
		italic = true
	}
	if style == formulaStyleBold {
		face = "Cambria"
		bold = true
	}
	if style == formulaStyleBoldItalic {
		face = "Cambria"
		bold = true
		italic = true
	}
	if style == formulaStyleKorean {
		face = "맑은 고딕"
		italic = false
	}
	h := createGDIFont(face, size, bold, italic, false)
	if h == 0 && face != "맑은 고딕" {
		h = createGDIFont("맑은 고딕", size, false, italic, false)
	}
	c.fonts[key] = h
	return h
}

func formulaTextMetrics(hdc uintptr, cache *formulaFontCache, text string, size int, style formulaTextStyle) (w, h, ascent int) {
	if text == "" {
		return 0, 0, 0
	}
	font := cache.get(size, style)
	old, _, _ := procSelectObject.Call(hdc, font)
	defer procSelectObject.Call(hdc, old)
	units := syscall.StringToUTF16(text)
	if len(units) > 0 && units[len(units)-1] == 0 {
		units = units[:len(units)-1]
	}
	var sz SIZE
	if len(units) > 0 {
		procGetTextExtentPoint32W.Call(hdc, uintptr(unsafe.Pointer(&units[0])), uintptr(len(units)), uintptr(unsafe.Pointer(&sz)))
	}
	var tm TEXTMETRICW
	procGetTextMetricsW.Call(hdc, uintptr(unsafe.Pointer(&tm)))
	return max(int(sz.Cx), 1), max(int(tm.TmHeight), max(size, 1)), max(int(tm.TmAscent), 1)
}

func formulaContainsKorean(text string) bool {
	for _, r := range text {
		if unicode.In(r, unicode.Hangul) {
			return true
		}
	}
	return false
}

func formulaLooksVariable(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if unicode.IsLetter(r) && !unicode.In(r, unicode.Hangul) {
			continue
		}
		return false
	}
	return true
}

func resolveFormulaTextStyle(nodeStyle, inherited formulaTextStyle, text string) formulaTextStyle {
	if inherited == formulaStyleRoman || inherited == formulaStyleItalic || inherited == formulaStyleKorean || inherited == formulaStyleBold || inherited == formulaStyleBoldItalic {
		return inherited
	}
	if nodeStyle == formulaStyleRoman || nodeStyle == formulaStyleItalic || nodeStyle == formulaStyleKorean || nodeStyle == formulaStyleBold || nodeStyle == formulaStyleBoldItalic {
		return nodeStyle
	}
	if formulaContainsKorean(text) {
		return formulaStyleKorean
	}
	if formulaLooksVariable(text) {
		return formulaStyleItalic
	}
	return formulaStyleRoman
}

func classifyFormulaText(text string, style formulaTextStyle) formulaMathClass {
	if text == " " || text == " " {
		return formulaClassSpace
	}
	if style == formulaStyleRoman || style == formulaStyleBold || style == formulaStyleBoldItalic {
		switch strings.ToLower(text) {
		case "sin", "cos", "tan", "sec", "csc", "cosec", "cot", "arcsin", "arccos", "arctan", "sinh", "cosh", "tanh", "coth", "log", "ln", "lg", "max", "min", "exp", "det", "mod", "gcd", "lim":
			return formulaClassFunction
		}
	}
	switch text {
	case "+", "−", "-", "×", "÷", "·", "±", "∪", "∩":
		return formulaClassBinary
	case "=", "≠", "≤", "≥", "≈", "<", ">", "∈", "∉", "⊂", "⊆", "→", "↔":
		return formulaClassRelation
	case "(", "[", "{":
		return formulaClassOpen
	case ")", "]", "}":
		return formulaClassClose
	case ",", ";", ":":
		return formulaClassPunctuation
	}
	return formulaClassOrd
}

func formulaInterGap(a, b *formulaLayout, size int) int {
	if a == nil || b == nil || a.mathClass == formulaClassSpace || b.mathClass == formulaClassSpace {
		return 0
	}
	if a.mathClass == formulaClassRelation || b.mathClass == formulaClassRelation {
		return max(3, size/5)
	}
	if a.mathClass == formulaClassBinary || b.mathClass == formulaClassBinary {
		return max(2, size/8)
	}
	if a.mathClass == formulaClassFunction && b.mathClass != formulaClassClose && b.mathClass != formulaClassPunctuation {
		return max(1, size/12)
	}
	if a.mathClass == formulaClassPunctuation {
		return max(1, size/18)
	}
	if a.complex || b.complex {
		return max(1, size/24)
	}
	return 0
}

func formulaEmptyLayout() *formulaLayout { return &formulaLayout{} }

// formulaLayoutVisibleInk returns the exact visible-pixel bounds of an already
// composed subtree. Cambria/Cambria Math GDI text cells carry substantial
// invisible top and bottom leading, so vertical structures must not use l.h
// directly when their parts are meant to touch closely.
func formulaLayoutContainsExplicitSpace(l *formulaLayout) bool {
	if l == nil {
		return false
	}
	if l.mathClass == formulaClassSpace && l.text == "" && l.w > 0 {
		return true
	}
	for _, child := range l.children {
		if formulaLayoutContainsExplicitSpace(child.layout) {
			return true
		}
	}
	return false
}

func formulaLayoutVisibleInk(hdc uintptr, cache *formulaFontCache, l *formulaLayout) formulaInkBounds {
	if l == nil || (l.w == 0 && l.h == 0) {
		return formulaInkBounds{}
	}
	if ink, ok := measureFormulaLayoutInk(hdc, cache, l); ok {
		// Manual ` and ~ spacing is intentional layout, even though it paints
		// no pixels. Preserve the full logical horizontal advance when a subtree
		// contains such spacers, including inside fractions and matrices.
		if formulaLayoutContainsExplicitSpace(l) {
			ink.left = min(ink.left, 0)
			ink.right = max(ink.right, l.w)
		}
		return ink
	}
	return formulaInkBounds{0, 0, max(l.w, 1), max(l.h, 1)}
}

func layoutFormula(hdc uintptr, cache *formulaFontCache, node *formulaNode, size int) *formulaLayout {
	return layoutFormulaStyled(hdc, cache, node, size, formulaStyleAuto)
}

func layoutFormulaStyled(hdc uintptr, cache *formulaFontCache, node *formulaNode, size int, inherited formulaTextStyle) *formulaLayout {
	if node == nil {
		return formulaEmptyLayout()
	}
	size = max(size, 8)
	switch node.Kind {
	case formulaText:
		if node.Text == "" {
			return formulaEmptyLayout()
		}
		style := resolveFormulaTextStyle(node.Style, inherited, node.Text)
		w, h, ascent := formulaTextMetrics(hdc, cache, node.Text, size, style)
		return &formulaLayout{w: w, h: h, baseline: ascent, text: node.Text, fontSize: size, fontStyle: style, mathClass: classifyFormulaText(node.Text, style)}
	case formulaSpace:
		// Explicit HWP spacing must not depend on Cambria Math's whitespace
		// glyph metrics. A backtick is a deterministic thin space; ~ is a
		// wider normal mathematical space. Height stays zero so a spacer never
		// changes a row's vertical metrics.
		// A106 doubles the thin-space advance: one ` now equals two A105
		// backticks (about 6 px at the default 20 px size).
		w := formulaExplicitSpaceWidth(size, node.Text)
		return &formulaLayout{w: w, mathClass: formulaClassSpace}
	case formulaSeq:
		parts := make([]*formulaLayout, 0, len(node.Children))
		baseline := 0
		for _, ch := range node.Children {
			l := layoutFormulaStyled(hdc, cache, ch, size, inherited)
			if l.w == 0 && l.h == 0 {
				continue
			}
			parts = append(parts, l)
			baseline = max(baseline, l.baseline)
		}
		if len(parts) == 0 {
			return formulaEmptyLayout()
		}
		out := &formulaLayout{baseline: baseline, mathClass: formulaClassOrd, complex: len(parts) > 1}
		x := 0
		for i, l := range parts {
			if i > 0 {
				x += formulaInterGap(parts[i-1], l, size)
			}
			y := baseline - l.baseline
			out.children = append(out.children, formulaPlacedLayout{l, x, y})
			x += l.w
			out.h = max(out.h, y+l.h)
			out.complex = out.complex || l.complex
		}
		out.w = x
		return out
	case formulaStyled:
		style := mergeFormulaTextStyle(inherited, node.Style)
		l := layoutFormulaStyled(hdc, cache, childAt(node, 0), size, style)
		l.complex = true
		return l
	case formulaScript:
		children := make([]*formulaNode, 5)
		copy(children, node.Children)
		base := layoutFormulaStyled(hdc, cache, children[0], size, inherited)
		scriptSize := max(7, int(math.Round(float64(size)*0.60)))
		tl := layoutFormulaStyled(hdc, cache, children[1], scriptSize, inherited)
		tr := layoutFormulaStyled(hdc, cache, children[2], scriptSize, inherited)
		bl := layoutFormulaStyled(hdc, cache, children[3], scriptSize, inherited)
		br := layoutFormulaStyled(hdc, cache, children[4], scriptSize, inherited)

		// Build all four scripts around the base's *visible* glyph, not around
		// Cambria Math's much taller invisible text cell.  The upper scripts'
		// visible bottoms sit just above the base line; the lower scripts'
		// visible tops sit just below it.  This keeps both ordinary x^2/H_2
		// and four-corner isotope notation compact.
		baseInk := formulaLayoutVisibleInk(hdc, cache, base)
		tlInk := formulaLayoutVisibleInk(hdc, cache, tl)
		trInk := formulaLayoutVisibleInk(hdc, cache, tr)
		blInk := formulaLayoutVisibleInk(hdc, cache, bl)
		brInk := formulaLayoutVisibleInk(hdc, cache, br)
		gapX := max(1, size/20)
		// A106 aligns ordinary right-side x^2/H_2 scripts with the same
		// vertical phase used by four-corner isotope notation.
		scriptLift := formulaScriptVerticalLift(size)
		upperBottom := -max(2, size/8) - scriptLift
		lowerTop := max(2, size/8) - scriptLift

		type scriptPlacement struct {
			layout *formulaLayout
			x, y   int
			ink    formulaInkBounds
		}
		placements := make([]scriptPlacement, 0, 5)
		// Put the base's visible ink at x=0 while keeping its mathematical
		// baseline at y=0 in this temporary coordinate system.
		placements = append(placements, scriptPlacement{base, -baseInk.left, -base.baseline, baseInk})
		baseVisibleRight := baseInk.width()
		if tlInk.width() > 0 && tlInk.height() > 0 {
			placements = append(placements, scriptPlacement{tl, -gapX - tlInk.right, upperBottom - tlInk.bottom, tlInk})
		}
		if blInk.width() > 0 && blInk.height() > 0 {
			placements = append(placements, scriptPlacement{bl, -gapX - blInk.right, lowerTop - blInk.top, blInk})
		}
		if trInk.width() > 0 && trInk.height() > 0 {
			placements = append(placements, scriptPlacement{tr, baseVisibleRight + gapX - trInk.left, upperBottom - trInk.bottom, trInk})
		}
		if brInk.width() > 0 && brInk.height() > 0 {
			placements = append(placements, scriptPlacement{br, baseVisibleRight + gapX - brInk.left, lowerTop - brInk.top, brInk})
		}

		minX, minY := 0, baseInk.top-base.baseline
		maxX, maxY := baseInk.width(), baseInk.bottom-base.baseline
		for _, pl := range placements {
			minX = min(minX, pl.x+pl.ink.left)
			minY = min(minY, pl.y+pl.ink.top)
			maxX = max(maxX, pl.x+pl.ink.right)
			maxY = max(maxY, pl.y+pl.ink.bottom)
		}
		out := &formulaLayout{w: max(1, maxX-minX), h: max(1, maxY-minY), baseline: -minY, mathClass: formulaClassOrd, complex: true}
		for _, pl := range placements {
			out.children = append(out.children, formulaPlacedLayout{pl.layout, pl.x - minX, pl.y - minY})
		}
		return out
	case formulaRoot:
		body := layoutFormulaStyled(hdc, cache, childAt(node, 0), size, inherited)
		index := layoutFormulaStyled(hdc, cache, childAt(node, 1), max(7, int(float64(size)*0.55)), inherited)
		bodyInk := formulaLayoutVisibleInk(hdc, cache, body)
		indexInk := formulaLayoutVisibleInk(hdc, cache, index)
		rad := layoutFormulaStyled(hdc, cache, &formulaNode{Kind: formulaText, Text: "√", Style: formulaStyleRoman}, max(size+2, int(float64(size)*1.15)), formulaStyleRoman)
		radInk := formulaLayoutVisibleInk(hdc, cache, rad)
		pad := max(1, size/20)
		bodyX := radInk.width() + pad
		bodyY := max(1, size/10)
		mainH := max(radInk.height(), bodyY+bodyInk.height())
		minX := 0
		if indexInk.width() > 0 {
			minX = -indexInk.width() / 2
		}
		w := bodyX + bodyInk.width() - minX + pad
		out := &formulaLayout{w: max(1, w), h: max(1, mainH+pad), baseline: bodyY - bodyInk.top + body.baseline, mathClass: formulaClassOrd, complex: true}
		out.children = append(out.children, formulaPlacedLayout{rad, -minX - radInk.left, -radInk.top})
		out.children = append(out.children, formulaPlacedLayout{body, bodyX - minX - bodyInk.left, bodyY - bodyInk.top})
		if indexInk.width() > 0 {
			out.children = append(out.children, formulaPlacedLayout{index, -minX - indexInk.left, -indexInk.top})
		}
		// Match the vinculum to the radical glyph instead of keeping a fixed
		// one-pixel rule.  Cambria Math's radical grows substantially at large
		// sizes, while 10/12 px needs a deliberate overlap so the shoulder does
		// not break away from the bar after rasterisation.
		rootAdjust := func(px int) int {
			return max(1, int(math.Round(float64(px*size)/20.0)))
		}
		lineWidth := max(1, int(math.Round(float64(size)/16.0)))
		lineY := max(0, bodyY-rootAdjust(2))
		// Start from the radical's visible right edge and overlap by about 3 px
		// at size 20 (2 px at 10/12).  This is more reliable than deriving the
		// start from bodyX, whose padding used to leave a tiny white seam.
		radVisibleRight := -minX + radInk.width()
		lineStart := max(0, radVisibleRight-rootAdjust(3))
		lineEnd := bodyX - minX + max(bodyInk.width()-1, 1)
		out.lines = append(out.lines, formulaLinePrimitive{lineStart, lineY, lineEnd, lineY, lineWidth})
		return out
	case formulaFraction:
		// Cambria Math's GDI text cell contains unusually large internal
		// vertical leading.  Using child.h therefore placed the fraction bar
		// tens of pixels below the visible numerator.  Measure the pixels that
		// are actually painted and compose the fraction from those ink bounds.
		num := layoutFormulaStyled(hdc, cache, childAt(node, 0), max(8, int(float64(size)*0.88)), inherited)
		den := layoutFormulaStyled(hdc, cache, childAt(node, 1), max(8, int(float64(size)*0.88)), inherited)
		numInk := formulaLayoutVisibleInk(hdc, cache, num)
		denInk := formulaLayoutVisibleInk(hdc, cache, den)

		padX := max(2, size/10)
		gapAboveBar := max(2, size/10)
		barWidth := max(1, size/20)
		// At the default 20 px size this leaves five clear pixels below the
		// one-pixel bar, matching the compact Hancom-style fraction sample.
		gapBelowBar := max(4, size/4)
		contentW := max(numInk.width(), denInk.width())
		w := contentW + padX*2
		barY := numInk.height() + gapAboveBar
		denInkY := barY + barWidth + gapBelowBar
		h := denInkY + denInk.height()
		baseline := clamp(barY+max(2, size/4), 1, max(h-1, 1))
		out := &formulaLayout{w: w, h: h, baseline: baseline, mathClass: formulaClassOrd, complex: true}

		numInkX := padX + (contentW-numInk.width())/2
		denInkX := padX + (contentW-denInk.width())/2
		out.children = append(out.children,
			formulaPlacedLayout{num, numInkX - numInk.left, -numInk.top},
			formulaPlacedLayout{den, denInkX - denInk.left, denInkY - denInk.top},
		)
		lineInset := max(0, padX/2)
		out.lines = append(out.lines, formulaLinePrimitive{lineInset, barY, w - lineInset - 1, barY, barWidth})
		return out
	case formulaSum, formulaIntegral:
		lower := layoutFormulaStyled(hdc, cache, childAt(node, 0), max(7, int(float64(size)*0.55)), inherited)
		upper := layoutFormulaStyled(hdc, cache, childAt(node, 1), max(7, int(float64(size)*0.55)), inherited)
		symbol := "∑"
		if node.Kind == formulaIntegral {
			symbol = node.Text
			if symbol == "" {
				symbol = "∫"
			}
		}
		main := layoutFormulaStyled(hdc, cache, &formulaNode{Kind: formulaText, Text: symbol, Style: formulaStyleRoman}, max(size+4, int(float64(size)*1.35)), formulaStyleRoman)
		mainInk := formulaLayoutVisibleInk(hdc, cache, main)
		upperInk := formulaLayoutVisibleInk(hdc, cache, upper)
		lowerInk := formulaLayoutVisibleInk(hdc, cache, lower)
		hasUpper := upper.w > 0 || upper.h > 0
		hasLower := lower.w > 0 || lower.h > 0

		// Sigma remains a centered stack with exactly one clear pixel above and
		// below the visible ∑ at the default size (and deliberately stays 1 px
		// rather than inheriting font leading).
		if node.Kind == formulaSum {
			gapY := 1
			contentW := mainInk.width()
			if hasUpper {
				contentW = max(contentW, upperInk.width())
			}
			if hasLower {
				contentW = max(contentW, lowerInk.width())
			}
			contentW = max(contentW, 1)
			mainInkY := 0
			if hasUpper {
				mainInkY = upperInk.height() + gapY
			}
			lowerInkY := mainInkY + mainInk.height()
			if hasLower {
				lowerInkY += gapY
			}
			h := mainInkY + mainInk.height()
			if hasLower {
				h = lowerInkY + lowerInk.height()
			}
			mainInkX := (contentW - mainInk.width()) / 2
			mainX := mainInkX - mainInk.left
			mainY := mainInkY - mainInk.top
			out := &formulaLayout{w: contentW, h: max(h, 1), baseline: mainY + main.baseline, mathClass: formulaClassLargeOperator, complex: true}
			if hasUpper {
				upperInkX := (contentW - upperInk.width()) / 2
				out.children = append(out.children, formulaPlacedLayout{upper, upperInkX - upperInk.left, -upperInk.top})
			}
			out.children = append(out.children, formulaPlacedLayout{main, mainX, mainY})
			if hasLower {
				lowerInkX := (contentW - lowerInk.width()) / 2
				out.children = append(out.children, formulaPlacedLayout{lower, lowerInkX - lowerInk.left, lowerInkY - lowerInk.top})
			}
			return out
		}

		// Definite-integral limits conventionally sit to the right of the ∫.
		// Begin with A104's compact centered stack, then apply the user's exact
		// default-size offsets: lower limit +6 px right / -4 px up, upper limit
		// +11 px right / +6 px down. Scale proportionally away from size 20.
		scaleOffset := func(px int) int {
			return max(1, int(math.Round(float64(px*size)/20.0)))
		}
		gapY := 1
		contentW := max(mainInk.width(), max(upperInk.width(), lowerInk.width()))
		mainInkY := 0
		if hasUpper {
			mainInkY = upperInk.height() + gapY
		}
		lowerInkY := mainInkY + mainInk.height()
		if hasLower {
			lowerInkY += gapY
		}
		mainInkX := (contentW - mainInk.width()) / 2
		type integralPlacement struct {
			layout *formulaLayout
			ink    formulaInkBounds
			x, y   int
		}
		places := []integralPlacement{{main, mainInk, mainInkX - mainInk.left, mainInkY - mainInk.top}}
		if hasUpper {
			upperInkX := (contentW - upperInk.width()) / 2
			places = append(places, integralPlacement{upper, upperInk, upperInkX - upperInk.left + scaleOffset(11), -upperInk.top + scaleOffset(6)})
		}
		if hasLower {
			lowerInkX := (contentW - lowerInk.width()) / 2
			places = append(places, integralPlacement{lower, lowerInk, lowerInkX - lowerInk.left + scaleOffset(6), lowerInkY - lowerInk.top - scaleOffset(4)})
		}
		minX, minY := places[0].x+places[0].ink.left, places[0].y+places[0].ink.top
		maxX, maxY := places[0].x+places[0].ink.right, places[0].y+places[0].ink.bottom
		for _, pl := range places[1:] {
			minX = min(minX, pl.x+pl.ink.left)
			minY = min(minY, pl.y+pl.ink.top)
			maxX = max(maxX, pl.x+pl.ink.right)
			maxY = max(maxY, pl.y+pl.ink.bottom)
		}
		out := &formulaLayout{w: max(1, maxX-minX), h: max(1, maxY-minY), baseline: places[0].y + main.baseline - minY, mathClass: formulaClassLargeOperator, complex: true}
		for _, pl := range places {
			out.children = append(out.children, formulaPlacedLayout{pl.layout, pl.x - minX, pl.y - minY})
		}
		return out
	case formulaVector:
		body := layoutFormulaStyled(hdc, cache, childAt(node, 0), size, inherited)
		bodyInk := formulaLayoutVisibleInk(hdc, cache, body)
		// A113 keeps the 24 px vector accent as the visual reference and scales
		// the shaft thickness and arrowhead only above that size. Smaller sizes
		// remain crisp, while 48/64/72 px no longer carry a tiny 1 px accent.
		scale := math.Max(1.0, float64(size)/24.0)
		lineWidth := max(1, int(math.Round(scale)))
		headLength := max(3, int(math.Round(3.0*scale)))
		headHalfHeight := max(2, int(math.Round(2.0*scale)))
		tipExtra := max(1, int(math.Round(scale)))
		arrowY := max(lineWidth, size/20)
		arrowToInkGap := max(2, size/10) + 2
		bodyInkY := arrowY + arrowToInkGap
		bodyWidth := max(bodyInk.width(), max(5, size/4))
		w := bodyWidth + tipExtra
		bodyInkX := (bodyWidth - bodyInk.width()) / 2
		bodyX := bodyInkX - bodyInk.left
		bodyY := bodyInkY - bodyInk.top
		h := bodyInkY + bodyInk.height()
		out := &formulaLayout{w: w, h: max(h, 1), baseline: bodyY + body.baseline, mathClass: formulaClassOrd, complex: true}
		out.children = append(out.children, formulaPlacedLayout{body, bodyX, bodyY})
		lineStart := bodyInkX
		shoulderX := bodyInkX + max(bodyInk.width()-1, 3)
		tipX := shoulderX + tipExtra
		out.lines = append(out.lines, formulaLinePrimitive{lineStart, arrowY, tipX, arrowY, lineWidth})
		out.lines = append(out.lines,
			formulaLinePrimitive{shoulderX - headLength, arrowY - headHalfHeight, tipX, arrowY, lineWidth},
			formulaLinePrimitive{shoulderX - headLength, arrowY + headHalfHeight, tipX, arrowY, lineWidth},
		)
		return out
	case formulaLimit:
		main := layoutFormulaStyled(hdc, cache, &formulaNode{Kind: formulaText, Text: "lim", Style: formulaStyleRoman}, size, formulaStyleRoman)
		condition := layoutFormulaStyled(hdc, cache, childAt(node, 0), max(7, int(float64(size)*0.55)), inherited)
		mainInk := formulaLayoutVisibleInk(hdc, cache, main)
		conditionInk := formulaLayoutVisibleInk(hdc, cache, condition)
		gapY := max(1, size/20)
		contentW := max(mainInk.width(), conditionInk.width())
		mainInkX := (contentW - mainInk.width()) / 2
		conditionInkX := (contentW - conditionInk.width()) / 2
		mainInkY := 0
		conditionInkY := mainInk.height() + gapY
		mainX := mainInkX - mainInk.left
		mainY := mainInkY - mainInk.top
		conditionX := conditionInkX - conditionInk.left
		conditionY := conditionInkY - conditionInk.top
		out := &formulaLayout{w: max(contentW, 1), h: max(conditionInkY+conditionInk.height(), 1), baseline: mainY + main.baseline, mathClass: formulaClassFunction, complex: true}
		out.children = append(out.children, formulaPlacedLayout{main, mainX, mainY})
		out.children = append(out.children, formulaPlacedLayout{condition, conditionX, conditionY})
		return out
	case formulaMatrix:
		return layoutFormulaMatrixStyled(hdc, cache, node, size, inherited)
	default:
		return formulaEmptyLayout()
	}
}

func childAt(n *formulaNode, i int) *formulaNode {
	if n != nil && i >= 0 && i < len(n.Children) {
		return n.Children[i]
	}
	return &formulaNode{Kind: formulaText, Text: ""}
}

func layoutFormulaMatrixStyled(hdc uintptr, cache *formulaFontCache, node *formulaNode, size int, inherited formulaTextStyle) *formulaLayout {
	rows := node.Matrix
	if len(rows) == 0 {
		rows = [][]*formulaNode{{{Kind: formulaText, Text: "□"}}}
	}
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	if cols < 1 {
		cols = 1
	}
	cellLayouts := make([][]*formulaLayout, len(rows))
	cellInks := make([][]formulaInkBounds, len(rows))
	colW := make([]int, cols)
	rowTop := make([]int, len(rows))
	rowBottom := make([]int, len(rows))
	for y, row := range rows {
		cellLayouts[y] = make([]*formulaLayout, cols)
		cellInks[y] = make([]formulaInkBounds, cols)
		first := true
		for x := 0; x < cols; x++ {
			var n *formulaNode
			if x < len(row) {
				n = row[x]
			} else {
				n = &formulaNode{Kind: formulaText, Text: ""}
			}
			l := layoutFormulaStyled(hdc, cache, n, size, inherited)
			ink := formulaLayoutVisibleInk(hdc, cache, l)
			cellLayouts[y][x] = l
			cellInks[y][x] = ink
			colW[x] = max(colW[x], ink.width())
			topRel := ink.top - l.baseline
			bottomRel := ink.bottom - l.baseline
			if first {
				rowTop[y], rowBottom[y], first = topRel, bottomRel, false
			} else {
				rowTop[y] = min(rowTop[y], topRel)
				rowBottom[y] = max(rowBottom[y], bottomRel)
			}
		}
		if first || rowBottom[y] <= rowTop[y] {
			rowTop[y], rowBottom[y] = -size/2, size/2
		}
	}
	colGap := max(4, size/7)
	rowGap := max(2, size/10)
	bracketW := max(7, size/5)
	padY := max(1, size/20)
	contentW := 0
	for x, w := range colW {
		if x > 0 {
			contentW += colGap
		}
		contentW += w
	}
	rowH := make([]int, len(rows))
	contentH := 0
	for y := range rows {
		rowH[y] = max(1, rowBottom[y]-rowTop[y])
		if y > 0 {
			contentH += rowGap
		}
		contentH += rowH[y]
	}
	h := max(contentH+padY*2, size)
	out := &formulaLayout{w: contentW + bracketW*2, h: h, baseline: clamp(h/2+size/4, 1, max(h-1, 1)), mathClass: formulaClassOrd, complex: true}
	yPos := (h - contentH) / 2
	for y := range rows {
		rowBaseline := -rowTop[y]
		xPos := bracketW
		for x := 0; x < cols; x++ {
			l := cellLayouts[y][x]
			ink := cellInks[y][x]
			cx := xPos + (colW[x]-ink.width())/2 - ink.left
			cy := yPos + rowBaseline - l.baseline
			out.children = append(out.children, formulaPlacedLayout{l, cx, cy})
			xPos += colW[x] + colGap
		}
		yPos += rowH[y] + rowGap
	}
	// Keep each vertical bracket five pixels from the nearest matrix ink at
	// size 20, two pixels farther than A104 on both sides.
	leftX := bracketW - 5
	rightX := bracketW + contentW - 1 + 5
	top := 1
	bottom := out.h - 2
	arm := max(4, bracketW-3)
	out.lines = append(out.lines,
		formulaLinePrimitive{leftX, top, leftX, bottom, 1}, formulaLinePrimitive{leftX, top, leftX + arm, top, 1}, formulaLinePrimitive{leftX, bottom, leftX + arm, bottom, 1},
		formulaLinePrimitive{rightX, top, rightX, bottom, 1}, formulaLinePrimitive{rightX - arm, top, rightX, top, 1}, formulaLinePrimitive{rightX - arm, bottom, rightX, bottom, 1},
	)
	return out
}

func measureFormulaLayoutInk(referenceDC uintptr, cache *formulaFontCache, l *formulaLayout) (formulaInkBounds, bool) {
	if l == nil || l.w < 1 || l.h < 1 {
		return formulaInkBounds{}, false
	}
	margin := 3
	w := max(l.w+margin*2, 8)
	h := max(l.h+margin*2, 8)
	dc, _, _ := procCreateCompatibleDC.Call(referenceDC)
	if dc == 0 {
		return formulaInkBounds{}, false
	}
	defer procDeleteDC.Call(dc)
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(w), BiHeight: -int32(h), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	var bits uintptr
	hbm, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbm == 0 || bits == 0 {
		return formulaInkBounds{}, false
	}
	defer procDeleteObject.Call(hbm)
	old, _, _ := procSelectObject.Call(dc, hbm)
	defer procSelectObject.Call(dc, old)
	buf := unsafe.Slice((*byte)(unsafe.Pointer(bits)), w*h*4)
	for i := 0; i < len(buf); i += 4 {
		buf[i], buf[i+1], buf[i+2], buf[i+3] = 255, 255, 255, 255
	}
	drawFormulaLayout(dc, cache, l, margin, margin, rgb(0, 0, 0))
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			// Include antialiased edge pixels while ignoring tiny white noise.
			if int(buf[i])+int(buf[i+1])+int(buf[i+2]) >= 3*251 {
				continue
			}
			minX, minY = min(minX, x), min(minY, y)
			maxX, maxY = max(maxX, x), max(maxY, y)
		}
	}
	if maxX < minX || maxY < minY {
		return formulaInkBounds{}, false
	}
	return formulaInkBounds{minX - margin, minY - margin, maxX - margin + 1, maxY - margin + 1}, true
}

func drawFormulaLayout(hdc uintptr, cache *formulaFontCache, l *formulaLayout, x, y int, color uint32) {
	if l == nil {
		return
	}
	if l.text != "" {
		font := cache.get(l.fontSize, l.fontStyle)
		old, _, _ := procSelectObject.Call(hdc, font)
		procSetBkMode.Call(hdc, TRANSPARENT)
		procSetTextColor.Call(hdc, uintptr(color))
		units := syscall.StringToUTF16(l.text)
		if len(units) > 0 && units[len(units)-1] == 0 {
			units = units[:len(units)-1]
		}
		if len(units) > 0 {
			procTextOutW.Call(hdc, uintptr(x), uintptr(y), uintptr(unsafe.Pointer(&units[0])), uintptr(len(units)))
		}
		procSelectObject.Call(hdc, old)
	}
	for _, c := range l.children {
		drawFormulaLayout(hdc, cache, c.layout, x+c.x, y+c.y, color)
	}
	for _, ln := range l.lines {
		pen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(max(ln.width, 1)), uintptr(color))
		old, _, _ := procSelectObject.Call(hdc, pen)
		procMoveToEx.Call(hdc, uintptr(x+ln.x1), uintptr(y+ln.y1), 0)
		procLineTo.Call(hdc, uintptr(x+ln.x2), uintptr(y+ln.y2))
		procSelectObject.Call(hdc, old)
		procDeleteObject.Call(pen)
	}
}

func renderFormulaImage(owner uintptr, source string, fontSize int, color uint32) ClipboardImage {
	node := parseFormula(source)
	windowDC, _, _ := procGetDC.Call(owner)
	if windowDC == 0 {
		return ClipboardImage{}
	}
	defer procReleaseDC.Call(owner, windowDC)
	memDC, _, _ := procCreateCompatibleDC.Call(windowDC)
	if memDC == 0 {
		return ClipboardImage{}
	}
	defer procDeleteDC.Call(memDC)
	cache := newFormulaFontCache()
	defer cache.close()
	layout := layoutFormula(memDC, cache, node, clamp(fontSize, 10, 96))
	if layout.w < 1 || layout.h < 1 {
		return ClipboardImage{}
	}
	margin := 6
	w, h := layout.w+margin*2, layout.h+margin*2
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(w), BiHeight: -int32(h), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	var bits uintptr
	hbm, _, _ := procCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbm == 0 || bits == 0 {
		return ClipboardImage{}
	}
	defer procDeleteObject.Call(hbm)
	old, _, _ := procSelectObject.Call(memDC, hbm)
	defer procSelectObject.Call(memDC, old)
	buf := unsafe.Slice((*byte)(unsafe.Pointer(bits)), w*h*4)
	for i := 0; i < len(buf); i += 4 {
		buf[i], buf[i+1], buf[i+2], buf[i+3] = 255, 255, 255, 255
	}
	drawFormulaLayout(memDC, cache, layout, margin, margin, rgb(0, 0, 0))

	if color == transparentColor {
		color = rgb(0, 0, 0)
	}
	cb := colorrefToBGRA(color)
	alpha := make([]byte, w*h)
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			brightness := (int(buf[i]) + int(buf[i+1]) + int(buf[i+2])) / 3
			a := clamp(255-brightness, 0, 255)
			if a < 4 {
				a = 0
			}
			alpha[y*w+x] = byte(a)
			if a > 0 {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x), max(maxY, y)
			}
		}
	}
	if maxX < minX || maxY < minY {
		return ClipboardImage{}
	}
	pad := 2
	minX = max(0, minX-pad)
	minY = max(0, minY-pad)
	maxX = min(w-1, maxX+pad)
	maxY = min(h-1, maxY+pad)
	outW, outH := maxX-minX+1, maxY-minY+1
	out := ClipboardImage{W: outW, H: outH, Pix: make([]byte, outW*outH*4), Mask: make([]bool, outW*outH)}
	for y := 0; y < outH; y++ {
		for x := 0; x < outW; x++ {
			a := alpha[(minY+y)*w+(minX+x)]
			di := (y*outW + x) * 4
			out.Pix[di], out.Pix[di+1], out.Pix[di+2], out.Pix[di+3] = cb[0], cb[1], cb[2], a
			out.Mask[y*outW+x] = a > 0
		}
	}
	return out
}

func formulaCompositePreviewOn(img ClipboardImage, bg byte) []byte {
	if !validClipboardImage(img) {
		return nil
	}
	out := make([]byte, len(img.Pix))
	for i := 0; i < len(img.Pix); i += 4 {
		a := int(img.Pix[i+3])
		out[i+0] = byte((int(img.Pix[i+0])*a + int(bg)*(255-a)) / 255)
		out[i+1] = byte((int(img.Pix[i+1])*a + int(bg)*(255-a)) / 255)
		out[i+2] = byte((int(img.Pix[i+2])*a + int(bg)*(255-a)) / 255)
		out[i+3] = 255
	}
	return out
}

func formulaCompositePreview(img ClipboardImage) []byte {
	return formulaCompositePreviewOn(img, 255)
}

func registerFormulaEditorClass(hinst uintptr) bool {
	if formulaEditorClassRegistered {
		return true
	}
	if formulaEditorWndProcCallback == 0 {
		formulaEditorWndProcCallback = syscall.NewCallback(formulaEditorWndProc)
	}
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{CbSize: uint32(unsafe.Sizeof(WNDCLASSEXW{})), Style: CS_HREDRAW | CS_VREDRAW, LpfnWndProc: formulaEditorWndProcCallback, HInstance: hinst, HCursor: cursor, HbrBackground: uintptr(COLOR_BTNFACE + 1), LpszClassName: utf16Ptr(formulaEditorClassName)}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		logf("FORMULA register class failed: %v", err)
		return false
	}
	formulaEditorClassRegistered = true
	return true
}

func (a *App) showFormulaEditor() {
	if activeFormulaEditor != nil && activeFormulaEditor.hwnd != 0 {
		if r, _, _ := procIsWindow.Call(activeFormulaEditor.hwnd); r != 0 {
			procShowWindow.Call(activeFormulaEditor.hwnd, formulaSWRestore)
			procSetForegroundWindow.Call(activeFormulaEditor.hwnd)
			procSetFocus.Call(activeFormulaEditor.input)
			return
		}
	}
	if !registerFormulaEditorClass(a.hinst) {
		messageBox(a.hwnd, "수식 편집기 창을 만들 수 없습니다.", "삽입 > 수식", MB_OK|MB_ICONERROR)
		return
	}
	state := &formulaEditorState{owner: a, fontSize: 20, categoryButtons: make(map[int]uintptr), invert: a.darkMode, uiFont: createCompactEditorUIFont()}
	activeFormulaEditor = state
	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16Ptr(formulaEditorClassName))), uintptr(unsafe.Pointer(utf16Ptr("수식 편집기"))), WS_OVERLAPPEDWINDOW, uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), 920, 650, a.hwnd, 0, a.hinst, 0)
	if hwnd == 0 {
		if state.uiFont != 0 {
			procDeleteObject.Call(state.uiFont)
			state.uiFont = 0
		}
		activeFormulaEditor = nil
		logf("FORMULA create window failed: %v", err)
		messageBox(a.hwnd, "수식 편집기 창을 만들 수 없습니다.", "삽입 > 수식", MB_OK|MB_ICONERROR)
		return
	}
	state.hwnd = hwnd
	showThemedEditorWindow(hwnd, a.darkMode)
	a.statusText = "수식 편집기를 열었습니다. 기호·템플릿을 누르고 입력식을 다듬은 뒤 삽입을 누르십시오."
	logf("FORMULA editor opened hwnd=%#x", hwnd)
}

func formulaCreateChild(s *formulaEditorState, class, title string, style uintptr, id int) uintptr {
	style = ownerDrawPushButtonStyle(class, style)
	h, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16Ptr(class))), uintptr(unsafe.Pointer(utf16Ptr(title))), style|WS_CHILD|WS_VISIBLE, 0, 0, 10, 10, s.hwnd, uintptr(id), app.hinst, 0)
	if h != 0 {
		applyDarkThemeToWindow(h, darkModeEnabled())
		setCompactEditorControlFont(h, s.uiFont)
	}
	return h
}

func formulaEditorCreateControls(s *formulaEditorState) {
	for _, spec := range formulaToolbarCategories {
		style := uintptr(WS_TABSTOP)
		if spec.id == formulaIDCategoryScripts || spec.id == formulaIDCategoryVector {
			style |= formulaBSOwnerDraw
		}
		h := formulaCreateChild(s, "BUTTON", spec.label, style, spec.id)
		s.controls = append(s.controls, h)
		s.categoryButtons[spec.id] = h
	}
	s.sizeCombo = formulaCreateChild(s, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|CBS_HASSTRINGS|CBS_NOINTEGRALHEIGHT|WS_VSCROLL, formulaIDSize)
	for _, sz := range formulaSizeChoices {
		sendMessage(s.sizeCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(strconv.Itoa(sz)))))
	}
	sendMessage(s.sizeCombo, CB_SETCURSEL, 3, 0)
	s.boldBtn = formulaCreateChild(s, "BUTTON", "B", WS_TABSTOP, formulaIDBold)
	s.italicBtn = formulaCreateChild(s, "BUTTON", "I", WS_TABSTOP, formulaIDItalic)
	s.invertBtn = formulaCreateChild(s, "BUTTON", "검은 배경", WS_TABSTOP, formulaIDInvert)
	s.controls = append(s.controls, s.sizeCombo, s.boldBtn, s.italicBtn, s.invertBtn)
	formulaUpdateInvertButton(s)
	s.copyPreviewBtn = formulaCreateChild(s, "BUTTON", "미리보기 복사", WS_TABSTOP, formulaIDCopyPreview)
	s.copyScriptBtn = formulaCreateChild(s, "BUTTON", "스크립트 복사", WS_TABSTOP, formulaIDCopyScript)
	// A115: copy actions move to the lower-left command row; their compact
	// font is kept so the labels fit beside Insert/Cancel without crowding.
	s.compactBtnFont = createGDIFont("맑은 고딕", 11, false, false, false)
	if s.compactBtnFont != 0 {
		sendMessage(s.copyPreviewBtn, WM_SETFONT, s.compactBtnFont, 1)
		sendMessage(s.copyScriptBtn, WM_SETFONT, s.compactBtnFont, 1)
	}
	boldFont := createGDIFont("Cambria", 15, true, false, false)
	if boldFont != 0 {
		sendMessage(s.boldBtn, WM_SETFONT, boldFont, 1)
		// The button owns no font, so keep it with the state-owned font list by
		// using compactBtnFont only for copy buttons and intentionally leak-free
		// cleanup through the extra handle field below.
		s.boldButtonFont = boldFont
	}
	italicFont := createGDIFont("Cambria", 15, false, true, false)
	if italicFont != 0 {
		sendMessage(s.italicBtn, WM_SETFONT, italicFont, 1)
		s.italicButtonFont = italicFont
	}
	s.input = formulaCreateChild(s, "EDIT", "", WS_TABSTOP|WS_BORDER|WS_VSCROLL|ES_MULTILINE|ES_AUTOVSCROLL|ES_AUTOHSCROLL, formulaIDInput)
	s.insertBtn = formulaCreateChild(s, "BUTTON", "삽입", WS_TABSTOP, formulaIDInsert)
	s.cancelBtn = formulaCreateChild(s, "BUTTON", "취소", WS_TABSTOP, formulaIDCancel)
	s.controls = append(s.controls, s.input, s.copyPreviewBtn, s.copyScriptBtn, s.insertBtn, s.cancelBtn)
	sendMessage(s.input, EM_SETSEL, 0, ^uintptr(0))
	formulaRefreshPreview(s)
	procSetFocus.Call(s.input)
}

func formulaEditorLayoutControls(s *formulaEditorState, client RECT) {
	w := int(client.Right - client.Left)
	// A116: the formula palette is deliberately split into two compact rows.
	// The first row keeps the structural tools; the second row carries sets,
	// arrows, constants and the new exponential/logarithmic templates.
	const rowBreak = 8
	x, y := 10, 10
	for i, spec := range formulaToolbarCategories {
		if i == rowBreak {
			x, y = 10, 40
		}
		if i < len(s.controls) {
			procMoveWindow.Call(s.controls[i], uintptr(x), uintptr(y), uintptr(spec.w), 26, 1)
		}
		x += spec.w + 4
	}
	// A117: text-style buttons come first, followed by the size selector.
	// This mirrors the user's marked-up layout and keeps B/I visually grouped
	// with the formula palette instead of stranded at the far-right edge.
	topRight := max(w-262, 10)
	procMoveWindow.Call(s.boldBtn, uintptr(topRight), 40, 42, 26, 1)
	procMoveWindow.Call(s.italicBtn, uintptr(topRight+48), 40, 42, 26, 1)
	procMoveWindow.Call(s.sizeCombo, uintptr(topRight+96), 40, 70, 240, 1)
	procMoveWindow.Call(s.invertBtn, uintptr(topRight+172), 40, 82, 26, 1)
	previewTop := 76
	previewBottom := max(previewTop+120, int(client.Bottom)-155)
	s.previewRect = RECT{10, int32(previewTop), int32(max(w-10, 20)), int32(previewBottom)}
	inputTop := previewBottom + 10
	procMoveWindow.Call(s.input, 10, uintptr(inputTop), uintptr(max(w-20, 40)), 78, 1)
	buttonY := inputTop + 88
	procMoveWindow.Call(s.copyPreviewBtn, 10, uintptr(buttonY), 108, 28, 1)
	procMoveWindow.Call(s.copyScriptBtn, 124, uintptr(buttonY), 108, 28, 1)
	procMoveWindow.Call(s.insertBtn, uintptr(max(w-180, 10)), uintptr(buttonY), 78, 28, 1)
	procMoveWindow.Call(s.cancelBtn, uintptr(max(w-96, 10)), uintptr(buttonY), 78, 28, 1)
}

func formulaUpdateInvertButton(s *formulaEditorState) {
	if s == nil || s.invertBtn == 0 {
		return
	}
	label := "검은 배경: 끔"
	if s.invert {
		label = "검은 배경: 켬"
	}
	procSetWindowTextW.Call(s.invertBtn, uintptr(unsafe.Pointer(utf16Ptr(label))))
	procInvalidateRect.Call(s.invertBtn, 0, 1)
}

func formulaRefreshPreview(s *formulaEditorState) {
	if s == nil || s.input == 0 {
		return
	}
	source := getWindowTextString(s.input)
	s.lastSource = source
	s.preview = renderFormulaImage(s.hwnd, source, s.fontSize, s.owner.fg)
	// "검은 배경" changes only the editor preview background. The formula
	// pixels keep the exact current foreground colour and alpha.
	procInvalidateRect.Call(s.hwnd, uintptr(unsafe.Pointer(&s.previewRect)), 0)
}

func formulaInsertMarkup(s *formulaEditorState, text string) {
	if s == nil || s.input == 0 {
		return
	}
	r := sendMessage(s.input, formulaEMGetSel, 0, 0)
	start := int(uint16(r & 0xFFFF))
	p := utf16Ptr(text)
	sendMessage(s.input, formulaEMReplaceSel, 1, uintptr(unsafe.Pointer(p)))
	idx := strings.Index(text, "□")
	if idx >= 0 {
		before := utf16UnitLen(text[:idx])
		placeholderLen := utf16UnitLen("□")
		sendMessage(s.input, EM_SETSEL, uintptr(start+before), uintptr(start+before+placeholderLen))
	}
	procSetFocus.Call(s.input)
}

func formulaWrapSelection(s *formulaEditorState, command string) {
	if s == nil || s.input == 0 || strings.TrimSpace(command) == "" {
		return
	}
	r := sendMessage(s.input, formulaEMGetSel, 0, 0)
	start := int(uint16(r & 0xFFFF))
	end := int(uint16((r >> 16) & 0xFFFF))
	if end < start {
		start, end = end, start
	}
	units := syscall.StringToUTF16(getWindowTextString(s.input))
	if len(units) > 0 && units[len(units)-1] == 0 {
		units = units[:len(units)-1]
	}
	start = min(max(start, 0), len(units))
	end = min(max(end, start), len(units))
	selected := ""
	if end > start {
		selected = syscall.UTF16ToString(units[start:end])
	}
	body := selected
	if body == "" {
		body = "□"
	}
	prefix := command + " {"
	replacement := prefix + body + "}"
	sendMessage(s.input, EM_SETSEL, uintptr(start), uintptr(end))
	sendMessage(s.input, formulaEMReplaceSel, 1, uintptr(unsafe.Pointer(utf16Ptr(replacement))))
	bodyStart := start + utf16UnitLen(prefix)
	bodyEnd := bodyStart + utf16UnitLen(body)
	sendMessage(s.input, EM_SETSEL, uintptr(bodyStart), uintptr(bodyEnd))
	procSetFocus.Call(s.input)
}

func formulaAppendPopupEntries(menu uintptr, entries []formulaPopupEntry, nextID *int, values map[int]string) {
	for _, entry := range entries {
		if entry.separator {
			appendSep(menu)
			continue
		}
		if len(entry.children) > 0 {
			sub, _, _ := procCreatePopupMenu.Call()
			if app != nil {
				app.applyDarkMenuBackground(sub)
			}
			formulaAppendPopupEntries(sub, entry.children, nextID, values)
			appendPopup(menu, sub, entry.label)
			continue
		}
		id := *nextID
		*nextID = *nextID + 1
		values[id] = entry.text
		appendItem(menu, MF_STRING, id, entry.label)
	}
}

func formulaShowCategoryPopup(s *formulaEditorState, categoryID int) {
	entries := formulaPopupEntries(categoryID)
	if len(entries) == 0 {
		return
	}
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	if app != nil {
		app.applyDarkMenuBackground(menu)
	}
	values := make(map[int]string)
	nextID := 7600
	formulaAppendPopupEntries(menu, entries, &nextID, values)

	var pt POINT
	if button := s.categoryButtons[categoryID]; button != 0 {
		var rc RECT
		formulaProcGetWindowRect.Call(button, uintptr(unsafe.Pointer(&rc)))
		pt.X, pt.Y = rc.Left, rc.Bottom
	} else {
		procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	}
	cmd, _, _ := procTrackPopupMenu.Call(menu, TPM_LEFTALIGN|TPM_TOPALIGN|TPM_RETURNCMD|TPM_NONOTIFY, uintptr(pt.X), uintptr(pt.Y), 0, s.hwnd, 0)
	if text := values[int(cmd)]; text != "" {
		formulaInsertMarkup(s, text)
	}
}

func formulaCopyUnicodeText(hwnd uintptr, text string) error {
	units := syscall.StringToUTF16(text)
	total := len(units) * 2
	hMem, _, err := procGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(total))
	if hMem == 0 {
		return fmt.Errorf("클립보드 메모리를 할당하지 못했습니다: %v", err)
	}
	owned := true
	defer func() {
		if owned {
			procGlobalFree.Call(hMem)
		}
	}()
	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return fmt.Errorf("클립보드 메모리를 잠글 수 없습니다")
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(units)), units)
	procGlobalUnlock.Call(hMem)
	if !openClipboardRetry(hwnd) {
		return fmt.Errorf("Windows 클립보드를 열 수 없습니다")
	}
	defer procCloseClipboard.Call()
	if r, _, _ := procEmptyClipboard.Call(); r == 0 {
		return fmt.Errorf("Windows 클립보드를 비울 수 없습니다")
	}
	if r, _, e := procSetClipboardData.Call(formulaCFUnicodeText, hMem); r == 0 {
		return fmt.Errorf("스크립트를 복사하지 못했습니다: %v", e)
	}
	owned = false
	return nil
}

func formulaAllocClipboardBlock(data []byte) (uintptr, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("빈 클립보드 데이터입니다")
	}
	hMem, _, err := procGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(len(data)))
	if hMem == 0 {
		return 0, fmt.Errorf("클립보드 메모리를 할당하지 못했습니다: %v", err)
	}
	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		procGlobalFree.Call(hMem)
		return 0, fmt.Errorf("클립보드 메모리를 잠글 수 없습니다")
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), len(data)), data)
	procGlobalUnlock.Call(hMem)
	return hMem, nil
}

// formulaCopyPreviewImage publishes two complementary clipboard forms:
//  1. CF_DIB composited on white for HWP, classic Paint and other programs
//     that ignore alpha in 32-bit BI_RGB data.
//  2. Registered PNG formats preserving transparency for applications that
//     explicitly support PNG clipboard payloads.
//
// This prevents transparent BGRA pixels (RGB=0, A=0) from becoming a solid
// black rectangle when pasted into legacy Windows applications.
func formulaCopyPreviewImage(s *formulaEditorState) error {
	if s == nil || s.owner == nil || !validClipboardImage(s.preview) {
		return fmt.Errorf("복사할 수식 미리보기가 없습니다")
	}
	dib, err := buildOpaqueClipboardDIB(s.preview.W, s.preview.H, s.preview.Pix)
	if err != nil {
		return fmt.Errorf("호환 비트맵 생성 실패: %w", err)
	}
	pngData, pngErr := buildTransparentClipboardPNG(s.preview.W, s.preview.H, s.preview.Pix)

	type clipboardPayload struct {
		format   uintptr
		data     []byte
		required bool
		handle   uintptr
	}
	items := []clipboardPayload{{format: CF_DIB, data: dib, required: true}}
	if pngErr == nil && len(pngData) > 0 {
		if clipboardPNGFormat != 0 {
			items = append(items, clipboardPayload{format: clipboardPNGFormat, data: pngData})
		}
		if clipboardImagePNGFormat != 0 && clipboardImagePNGFormat != clipboardPNGFormat {
			items = append(items, clipboardPayload{format: clipboardImagePNGFormat, data: pngData})
		}
	}

	for i := range items {
		h, allocErr := formulaAllocClipboardBlock(items[i].data)
		if allocErr != nil {
			if items[i].required {
				for j := 0; j < i; j++ {
					if items[j].handle != 0 {
						procGlobalFree.Call(items[j].handle)
						items[j].handle = 0
					}
				}
				return allocErr
			}
			logf("FORMULA clipboard optional allocation failed format=%d err=%v", items[i].format, allocErr)
			continue
		}
		items[i].handle = h
	}
	defer func() {
		for i := range items {
			if items[i].handle != 0 {
				procGlobalFree.Call(items[i].handle)
			}
		}
	}()

	if !openClipboardRetry(s.hwnd) {
		return fmt.Errorf("Windows 클립보드를 열 수 없습니다")
	}
	defer procCloseClipboard.Call()
	if r, _, _ := procEmptyClipboard.Call(); r == 0 {
		return fmt.Errorf("Windows 클립보드를 비울 수 없습니다")
	}
	for i := range items {
		if items[i].handle == 0 {
			continue
		}
		r, _, setErr := procSetClipboardData.Call(items[i].format, items[i].handle)
		if r == 0 {
			if items[i].required {
				return fmt.Errorf("Windows 클립보드에 이미지를 넣지 못했습니다: %v", setErr)
			}
			logf("FORMULA clipboard optional format failed format=%d err=%v", items[i].format, setErr)
			continue
		}
		items[i].handle = 0 // clipboard owns the block after success
	}
	seq, _, _ := procGetClipboardSequenceNumber.Call()
	s.owner.lastClipboardSeq = uint32(seq)
	logf("FORMULA clipboard copied w=%d h=%d png=%v seq=%d", s.preview.W, s.preview.H, pngErr == nil, seq)
	return nil
}

func formulaCommitInsert(s *formulaEditorState) {
	if s == nil {
		return
	}
	if !validClipboardImage(s.preview) {
		messageBox(s.hwnd, "삽입할 수식이 없습니다.", "수식 편집기", MB_OK|MB_ICONWARNING)
		return
	}
	source := strings.TrimSpace(getWindowTextString(s.input))
	if source == "" {
		messageBox(s.hwnd, "삽입할 수식을 입력하십시오.", "수식 편집기", MB_OK|MB_ICONWARNING)
		return
	}
	img := s.preview
	owner := s.owner
	procDestroyWindow.Call(s.hwnd)
	if owner.tool == ToolText && owner.textTool.Active {
		owner.finishTextEditing(true)
	}
	if owner.curveUndoPending {
		owner.finishCurveEarlyOrCancel("formula insert")
	}
	owner.pasteClipboardImage(img, "수식 편집기")
	owner.statusText = fmt.Sprintf("수식 %d × %d 이미지를 투명 선택으로 삽입했습니다.", img.W, img.H)
	logf("FORMULA inserted source=%q size=%d image=%dx%d black-background=%v", source, s.fontSize, img.W, img.H, s.invert)
}

func formulaHandleCommand(s *formulaEditorState, id, code int) {
	if s == nil {
		return
	}
	if id == formulaIDInput && code == EN_CHANGE {
		formulaRefreshPreview(s)
		return
	}
	if id == formulaIDSize && code == formulaCBNSelection {
		idx := int(sendMessage(s.sizeCombo, CB_GETCURSEL, 0, 0))
		if idx >= 0 && idx < len(formulaSizeChoices) {
			s.fontSize = formulaSizeChoices[idx]
			formulaRefreshPreview(s)
		}
		return
	}
	if code != BN_CLICKED {
		return
	}
	if id == formulaIDInsert {
		formulaCommitInsert(s)
		return
	}
	if id == formulaIDBold {
		formulaWrapSelection(s, "bold")
		return
	}
	if id == formulaIDItalic {
		formulaWrapSelection(s, "italic")
		return
	}
	if id == formulaIDInvert {
		s.invert = !s.invert
		formulaUpdateInvertButton(s)
		formulaRefreshPreview(s)
		logf("FORMULA black-background=%v", s.invert)
		return
	}
	if id == formulaIDCopyPreview {
		if err := formulaCopyPreviewImage(s); err != nil {
			messageBox(s.hwnd, err.Error(), "미리보기 복사", MB_OK|MB_ICONERROR)
		} else {
			s.owner.statusText = "수식 미리보기를 이미지로 복사했습니다."
		}
		return
	}
	if id == formulaIDCopyScript {
		if err := formulaCopyUnicodeText(s.hwnd, getWindowTextString(s.input)); err != nil {
			messageBox(s.hwnd, err.Error(), "스크립트 복사", MB_OK|MB_ICONERROR)
		} else {
			s.owner.statusText = "수식 스크립트를 텍스트로 복사했습니다."
		}
		return
	}
	if id == formulaIDCancel {
		procDestroyWindow.Call(s.hwnd)
		return
	}
	for _, spec := range formulaToolbarCategories {
		if spec.id != id {
			continue
		}
		if spec.directText != "" {
			formulaInsertMarkup(s, spec.directText)
			return
		}
		formulaShowCategoryPopup(s, spec.id)
		return
	}
}

func formulaPaintEditor(s *formulaEditorState, hdc uintptr) {
	var client RECT
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	fillUIRect(hdc, client, rgb(240, 240, 240))
	pr := s.previewRect
	previewBG := rgb(255, 255, 255)
	bgByte := byte(255)
	if s.invert {
		previewBG = rgb(18, 18, 18)
		bgByte = 18
	}
	fillRect(hdc, pr, previewBG)
	frameRect(hdc, pr, uiColor(rgb(150, 150, 150)))
	if validClipboardImage(s.preview) {
		display := formulaCompositePreviewOn(s.preview, bgByte)
		availW := max(int(pr.Right-pr.Left)-20, 1)
		availH := max(int(pr.Bottom-pr.Top)-20, 1)
		dw, dh := s.preview.W, s.preview.H
		if dw > availW || dh > availH {
			scale := math.Min(float64(availW)/float64(dw), float64(availH)/float64(dh))
			dw = max(1, int(math.Round(float64(dw)*scale)))
			dh = max(1, int(math.Round(float64(dh)*scale)))
		}
		dx := int(pr.Left) + (int(pr.Right-pr.Left)-dw)/2
		dy := int(pr.Top) + 10 + (availH-dh)/2
		bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(s.preview.W), BiHeight: -int32(s.preview.H), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
		procSetStretchBltMode.Call(hdc, COLORONCOLOR)
		procStretchDIBits.Call(hdc, uintptr(dx), uintptr(dy), uintptr(dw), uintptr(dh), 0, 0, uintptr(s.preview.W), uintptr(s.preview.H), uintptr(unsafe.Pointer(&display[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, SRCCOPY)
	}
}

func formulaDrawCategoryButtonBase(dis *DRAWITEMSTRUCT) (RECT, uint32, int, int) {
	rc := dis.RcItem
	fg, _, dx, dy, _ := paintThemedOwnerDrawPushButtonBase(dis)
	procSetBkMode.Call(dis.HDC, TRANSPARENT)
	procSetTextColor.Call(dis.HDC, uintptr(fg))
	return rc, fg, dx, dy
}

func formulaDrawScriptsCategoryButton(dis *DRAWITEMSTRUCT) bool {
	if dis == nil || dis.CtlType != formulaODTButton || int(dis.CtlID) != formulaIDCategoryScripts || dis.HDC == 0 {
		return false
	}
	rc, fg, dx, dy := formulaDrawCategoryButtonBase(dis)

	// Draw a real isotope/ion template instead of a vague x² label:
	// ²³⁵₉₂U³⁺.  Separate text cells preserve the conventional left isotope
	// scripts and right ionic charge even when Unicode script glyph fallback
	// differs between Windows versions.
	baseFont := createGDIFont("Cambria Math", 16, false, false, false)
	if baseFont == 0 {
		baseFont = createGDIFont("Cambria", 16, false, false, false)
	}
	scriptFont := createGDIFont("Cambria Math", 8, false, false, false)
	if scriptFont == 0 {
		scriptFont = createGDIFont("Cambria", 8, false, false, false)
	}
	centerX := int(rc.Left+rc.Right)/2 + dx
	centerY := int(rc.Top+rc.Bottom)/2 + dy

	if baseFont != 0 {
		old, _, _ := procSelectObject.Call(dis.HDC, baseFont)
		baseRC := RECT{int32(centerX - 8), int32(centerY - 10), int32(centerX + 9), int32(centerY + 11)}
		drawTextUsingSelectedFont(dis.HDC, "U", baseRC, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX, uint32(fg))
		procSelectObject.Call(dis.HDC, old)
		procDeleteObject.Call(baseFont)
	}
	if scriptFont != 0 {
		old, _, _ := procSelectObject.Call(dis.HDC, scriptFont)
		leftX := centerX - 24
		rightX := centerX + 9
		topY := centerY - 11
		bottomY := centerY + 1
		topLeft := RECT{int32(leftX), int32(topY), int32(centerX - 7), int32(centerY)}
		bottomLeft := RECT{int32(leftX + 5), int32(bottomY), int32(centerX - 7), int32(centerY + 11)}
		topRight := RECT{int32(rightX), int32(topY), int32(centerX + 27), int32(centerY)}
		drawTextUsingSelectedFont(dis.HDC, "235", topLeft, DT_RIGHT|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX, uint32(fg))
		drawTextUsingSelectedFont(dis.HDC, "92", bottomLeft, DT_RIGHT|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX, uint32(fg))
		drawTextUsingSelectedFont(dis.HDC, "3+", topRight, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX, uint32(fg))
		procSelectObject.Call(dis.HDC, old)
		procDeleteObject.Call(scriptFont)
	}

	_, _, _, focusColor := themedOwnerDrawPushButtonColors(dis)
	paintThemedOwnerDrawPushButtonFocus(dis, focusColor)
	return true
}

func formulaDrawVectorCategoryButton(dis *DRAWITEMSTRUCT) bool {
	if dis == nil || dis.CtlType != formulaODTButton || int(dis.CtlID) != formulaIDCategoryVector || dis.HDC == 0 {
		return false
	}
	rc, fg, dx, dy := formulaDrawCategoryButtonBase(dis)
	vRect := rc
	vRect.Left += int32(12 + dx)
	vRect.Right -= int32(10 - dx)
	vRect.Top += int32(7 + dy)
	vRect.Bottom += int32(dy)
	drawTextUsingSelectedFont(dis.HDC, "v", vRect, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX, uint32(fg))

	left := int(rc.Left) + 12 + dx
	right := int(rc.Right) - 11 + dx
	y := int(rc.Top) + 7 + dy
	pen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(fg))
	oldPen, _, _ := procSelectObject.Call(dis.HDC, pen)
	procMoveToEx.Call(dis.HDC, uintptr(left), uintptr(y), 0)
	procLineTo.Call(dis.HDC, uintptr(right), uintptr(y))
	procMoveToEx.Call(dis.HDC, uintptr(right-3), uintptr(y-2), 0)
	procLineTo.Call(dis.HDC, uintptr(right), uintptr(y))
	procMoveToEx.Call(dis.HDC, uintptr(right-3), uintptr(y+2), 0)
	procLineTo.Call(dis.HDC, uintptr(right), uintptr(y))
	procSelectObject.Call(dis.HDC, oldPen)
	procDeleteObject.Call(pen)

	_, _, _, focusColor := themedOwnerDrawPushButtonColors(dis)
	paintThemedOwnerDrawPushButtonFocus(dis, focusColor)
	return true
}

func formulaEditorWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			logPanic("formulaEditorWndProc", r)
		}
	}()
	s := activeFormulaEditor
	if r, handled := handleDarkEditorNonClient(hwnd, msg, wparam, lparam); handled {
		return r
	}
	switch msg {
	case WM_MENUCHAR:
		if result, ok := handleThemedMenuChar(wparam, lparam); ok {
			return result
		}
	case WM_MEASUREITEM:
		if lparam != 0 && app != nil {
			if app.measureThemedMenuItem((*MEASUREITEMSTRUCT)(unsafe.Pointer(lparam))) {
				return 1
			}
		}
	case WM_CREATE:
		if s == nil {
			return ^uintptr(0)
		}
		s.hwnd = hwnd
		formulaEditorCreateControls(s)
		var client RECT
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
		formulaEditorLayoutControls(s, client)
		// Match the help centre: apply the non-client dark theme inside
		// WM_CREATE, before Windows presents the first visible caption frame.
		applyDarkThemeToWindow(hwnd, darkModeEnabled())
		drawDarkEditorCaption(hwnd)
		scheduleDarkEditorChromeRefresh(hwnd)
		formulaProcRegisterHotKey.Call(hwnd, formulaHotKeyInsert, formulaModShift, VK_ESCAPE)
		return 0
	case WM_CTLCOLORSTATIC, WM_CTLCOLORBTN, WM_CTLCOLORLISTBOX, WM_CTLCOLOREDIT, WM_CTLCOLORSCROLLBAR, WM_CTLCOLORDLG:
		if darkModeEnabled() && wparam != 0 && app != nil {
			procSetBkMode.Call(wparam, OPAQUE)
			procSetTextColor.Call(wparam, uintptr(rgb(235, 235, 235)))
			procSetBkColor.Call(wparam, uintptr(rgb(30, 30, 30)))
			if app.darkBrush == 0 {
				app.darkBrush, _, _ = procCreateSolidBrush.Call(uintptr(rgb(30, 30, 30)))
			}
			return app.darkBrush
		}
	case WM_SIZE:
		if s != nil && s.hwnd == hwnd {
			var client RECT
			procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
			formulaEditorLayoutControls(s, client)
			formulaRefreshPreview(s)
		}
		return 0
	case WM_COMMAND:
		if s != nil && s.hwnd == hwnd {
			id := int(wparam & 0xFFFF)
			code := int((wparam >> 16) & 0xFFFF)
			formulaHandleCommand(s, id, code)
		}
		return 0
	case WM_DRAWITEM:
		if lparam != 0 {
			dis := (*DRAWITEMSTRUCT)(unsafe.Pointer(lparam))
			if (app != nil && app.drawThemedMenuItem(dis)) || formulaDrawScriptsCategoryButton(dis) || formulaDrawVectorCategoryButton(dis) || drawThemedOwnerDrawPushButton(dis) {
				return 1
			}
		}
	case formulaWMHotKey:
		if wparam == formulaHotKeyInsert && s != nil {
			formulaCommitInsert(s)
			return 0
		}
	case WM_KEYDOWN:
		if wparam == VK_ESCAPE {
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case WM_PAINT:
		if s != nil && s.hwnd == hwnd {
			var ps PAINTSTRUCT
			hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			s.paintBuffer.paint(hwnd, hdc, ps.RcPaint, func(targetDC uintptr) { formulaPaintEditor(s, targetDC) })
			procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			return 0
		}
	case WM_ERASEBKGND:
		return 1
	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		formulaProcUnregisterHotKey.Call(hwnd, formulaHotKeyInsert)
		if s != nil && s.hwnd == hwnd {
			s.paintBuffer.release()
			if s.uiFont != 0 {
				procDeleteObject.Call(s.uiFont)
				s.uiFont = 0
			}
			if s.compactBtnFont != 0 {
				procDeleteObject.Call(s.compactBtnFont)
				s.compactBtnFont = 0
			}
			if s.boldButtonFont != 0 {
				procDeleteObject.Call(s.boldButtonFont)
				s.boldButtonFont = 0
			}
			if s.italicButtonFont != 0 {
				procDeleteObject.Call(s.italicButtonFont)
				s.italicButtonFont = 0
			}
			logf("FORMULA editor closed")
			activeFormulaEditor = nil
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}
