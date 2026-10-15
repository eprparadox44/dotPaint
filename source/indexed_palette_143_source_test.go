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
	graphEditorClassName = "dotPaintFunctionGraphEditorA122"

	graphIDInput = 7301 + iota
	graphIDFunctionMode
	graphIDAngle
	graphIDAxisAdd
	graphIDAxisRemove
	graphIDXMin
	graphIDXMax
	graphIDYMin
	graphIDYMax
	graphIDProportional
	graphIDCopyPreview
	graphIDCopyScript
	graphIDInsert
	graphIDCancel
	graphIDPolarSample
	graphIDInvert
	graphIDToolbarBase
)

const (
	graphCBNSelection  = 1
	graphEMScrollCaret = 0x00B7
)

type graphToolbarSpec struct {
	label string
	text  string
	width int
}

var graphToolbarButtons = []graphToolbarSpec{
	{"xⁿ", "x^□", 44},
	{"x!", "(□)!", 44},
	{"sin", "sin(□)", 46},
	{"cos", "cos(□)", 46},
	{"tan", "tan(□)", 46},
	{"cot", "cot(□)", 46},
	{"sec", "sec(□)", 46},
	{"csc", "csc(□)", 46},
	{"π", "π", 38},
	{"e", "e", 38},
	{"asin", "asin(□)", 52},
	{"acos", "acos(□)", 52},
	{"atan", "atan(□)", 52},
	{"acot", "acot(□)", 52},
	{"asec", "asec(□)", 52},
	{"acsc", "acsc(□)", 52},
	{"exp", "exp(□)", 48},
	{"ln", "ln(□)", 42},
	{"log", "log(□)", 48},
}

type functionGraphEditorState struct {
	paintBuffer previewEditorPaintBuffer
	owner       *App
	hwnd        uintptr
	uiFont      uintptr

	input             uintptr
	functionCombo     uintptr
	angleCombo        uintptr
	axisAddBtn        uintptr
	axisRemoveBtn     uintptr
	xMinEdit          uintptr
	xMaxEdit          uintptr
	yMinEdit          uintptr
	yMaxEdit          uintptr
	proportionalCheck uintptr
	invertBtn         uintptr
	invert            bool
	copyPreview       uintptr
	copyScript        uintptr
	insertBtn         uintptr
	cancelBtn         uintptr
	statusLabel       uintptr
	polarSampleLabel  uintptr
	polarSampleCombo  uintptr
	polarSampleDesc   uintptr

	axisLabel  uintptr
	angleLabel uintptr
	xMinLabel  uintptr
	xMaxLabel  uintptr
	yMinLabel  uintptr
	yMaxLabel  uintptr

	toolbar []uintptr

	showAxes          bool
	proportionalScale bool
	updatingRanges    bool
	manualYMin        string
	manualYMax        string
	angleMode         GraphAngleMode
	functionMode      GraphFunctionMode
	preview           ClipboardImage
	previewRect       RECT
	lastError         string
	lastExpr          string
}

var (
	graphEditorWndProcCallback uintptr
	graphEditorClassRegistered bool
	activeFunctionGraphEditor  *functionGraphEditorState
)

func registerFunctionGraphEditorClass(hinst uintptr) bool {
	if graphEditorClassRegistered {
		return true
	}
	if graphEditorWndProcCallback == 0 {
		graphEditorWndProcCallback = syscall.NewCallback(functionGraphEditorWndProc)
	}
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         CS_HREDRAW | CS_VREDRAW,
		LpfnWndProc:   graphEditorWndProcCallback,
		HInstance:     hinst,
		HCursor:       cursor,
		HbrBackground: uintptr(COLOR_BTNFACE + 1),
		LpszClassName: utf16Ptr(graphEditorClassName),
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		logf("GRAPH register class failed: %v", err)
		return false
	}
	graphEditorClassRegistered = true
	return true
}

func (a *App) showFunctionGraphEditor() {
	if activeFunctionGraphEditor != nil && activeFunctionGraphEditor.hwnd != 0 {
		if r, _, _ := procIsWindow.Call(activeFunctionGraphEditor.hwnd); r != 0 {
			procShowWindow.Call(activeFunctionGraphEditor.hwnd, formulaSWRestore)
			procSetForegroundWindow.Call(activeFunctionGraphEditor.hwnd)
			procSetFocus.Call(activeFunctionGraphEditor.input)
			return
		}
	}
	if !registerFunctionGraphEditorClass(a.hinst) {
		messageBox(a.hwnd, "함수/그래프 편집기 창을 만들 수 없습니다.", "삽입 > 함수/그래프", MB_OK|MB_ICONERROR)
		return
	}
	state := &functionGraphEditorState{
		owner: a, showAxes: true, proportionalScale: true,
		manualYMin: "-10", manualYMax: "10",
		angleMode: GraphRadians, functionMode: GraphFunctionOfX, invert: a.darkMode,
		uiFont: createCompactEditorUIFont(),
	}
	activeFunctionGraphEditor = state
	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(graphEditorClassName))),
		uintptr(unsafe.Pointer(utf16Ptr("함수/그래프 편집기"))),
		WS_OVERLAPPEDWINDOW,
		uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), 980, 720,
		a.hwnd, 0, a.hinst, 0,
	)
	if hwnd == 0 {
		if state.uiFont != 0 {
			procDeleteObject.Call(state.uiFont)
			state.uiFont = 0
		}
		activeFunctionGraphEditor = nil
		logf("GRAPH create window failed: %v", err)
		messageBox(a.hwnd, "함수/그래프 편집기 창을 만들 수 없습니다.", "삽입 > 함수/그래프", MB_OK|MB_ICONERROR)
		return
	}
	state.hwnd = hwnd
	showThemedEditorWindow(hwnd, a.darkMode)
	a.statusText = "함수/그래프 편집기를 열었습니다. f(x), f(y), f(r) 방식과 범위·각도 단위를 정하십시오."
	logf("GRAPH editor opened hwnd=%#x", hwnd)
}

func graphCreateChild(s *functionGraphEditorState, class, title string, style uintptr, id int) uintptr {
	style = ownerDrawPushButtonStyle(class, style)
	h, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(class))),
		uintptr(unsafe.Pointer(utf16Ptr(title))),
		style|WS_CHILD|WS_VISIBLE,
		0, 0, 10, 10,
		s.hwnd, uintptr(id), s.owner.hinst, 0,
	)
	if h != 0 {
		applyDarkThemeToWindow(h, darkModeEnabled())
		setCompactEditorControlFont(h, s.uiFont)
	}
	return h
}

func graphSetText(hwnd uintptr, text string) {
	if hwnd != 0 {
		procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(text))))
	}
}

func graphEditorCreateControls(s *functionGraphEditorState) {
	for i, spec := range graphToolbarButtons {
		h := graphCreateChild(s, "BUTTON", spec.label, WS_TABSTOP, graphIDToolbarBase+i)
		s.toolbar = append(s.toolbar, h)
	}

	s.axisAddBtn = graphCreateChild(s, "BUTTON", "축 넣기", WS_TABSTOP, graphIDAxisAdd)
	s.axisRemoveBtn = graphCreateChild(s, "BUTTON", "축 빼기", WS_TABSTOP, graphIDAxisRemove)
	s.axisLabel = graphCreateChild(s, "STATIC", "축: 표시", 0x0000000C, 0)
	s.angleLabel = graphCreateChild(s, "STATIC", "각도", 0, 0)
	s.angleCombo = graphCreateChild(s, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|CBS_HASSTRINGS|CBS_NOINTEGRALHEIGHT, graphIDAngle)
	sendMessage(s.angleCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr("라디안"))))
	sendMessage(s.angleCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr("도"))))
	sendMessage(s.angleCombo, CB_SETCURSEL, 0, 0)

	s.xMinLabel = graphCreateChild(s, "STATIC", "x 최소", 0, 0)
	s.xMaxLabel = graphCreateChild(s, "STATIC", "x 최대", 0, 0)
	s.yMinLabel = graphCreateChild(s, "STATIC", "y 최소", 0, 0)
	s.yMaxLabel = graphCreateChild(s, "STATIC", "y 최대", 0, 0)
	s.xMinEdit = graphCreateChild(s, "EDIT", "-10", WS_TABSTOP|WS_BORDER|ES_CENTER|ES_AUTOHSCROLL, graphIDXMin)
	s.xMaxEdit = graphCreateChild(s, "EDIT", "10", WS_TABSTOP|WS_BORDER|ES_CENTER|ES_AUTOHSCROLL, graphIDXMax)
	s.yMinEdit = graphCreateChild(s, "EDIT", "-10", WS_TABSTOP|WS_BORDER|ES_CENTER|ES_AUTOHSCROLL, graphIDYMin)
	s.yMaxEdit = graphCreateChild(s, "EDIT", "10", WS_TABSTOP|WS_BORDER|ES_CENTER|ES_AUTOHSCROLL, graphIDYMax)
	for _, edit := range []uintptr{s.xMinEdit, s.xMaxEdit, s.yMinEdit, s.yMaxEdit} {
		// Zero equal left/right margins keep ES_CENTER mathematically centered.
		sendMessage(edit, EM_SETMARGINS, EC_LEFTMARGIN|EC_RIGHTMARGIN, 0)
	}
	s.proportionalCheck = graphCreateChild(s, "BUTTON", "x, y 눈금 정비례", WS_TABSTOP|BS_AUTOCHECKBOX, graphIDProportional)
	sendMessage(s.proportionalCheck, BM_SETCHECK, BST_CHECKED, 0)
	s.invertBtn = graphCreateChild(s, "BUTTON", "검은 배경", WS_TABSTOP, graphIDInvert)
	graphUpdateInvertButton(s)
	procEnableWindow.Call(s.yMinEdit, 0)
	procEnableWindow.Call(s.yMaxEdit, 0)

	s.polarSampleLabel = graphCreateChild(s, "STATIC", "극좌표 샘플", 0, 0)
	s.polarSampleCombo = graphCreateChild(s, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|CBS_HASSTRINGS|CBS_NOINTEGRALHEIGHT, graphIDPolarSample)
	sendMessage(s.polarSampleCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr("샘플 선택..."))))
	for _, sample := range graphPolarSamples {
		sendMessage(s.polarSampleCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(sample.name))))
	}
	sendMessage(s.polarSampleCombo, CB_SETCURSEL, 0, 0)
	s.polarSampleDesc = graphCreateChild(s, "STATIC", "f(r) 모드에서 극좌표 샘플을 선택할 수 있습니다.", 0, 0)
	procEnableWindow.Call(s.polarSampleCombo, 0)

	s.functionCombo = graphCreateChild(s, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|CBS_HASSTRINGS|CBS_NOINTEGRALHEIGHT, graphIDFunctionMode)
	sendMessage(s.functionCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr("f(x) ="))))
	sendMessage(s.functionCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr("f(y) ="))))
	sendMessage(s.functionCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr("f(r) ="))))
	sendMessage(s.functionCombo, CB_SETCURSEL, 0, 0)
	s.input = graphCreateChild(s, "EDIT", "x^2", WS_TABSTOP|WS_BORDER|ES_AUTOHSCROLL, graphIDInput)
	s.statusLabel = graphCreateChild(s, "STATIC", "", 0, 0)
	s.copyPreview = graphCreateChild(s, "BUTTON", "미리보기 복사", WS_TABSTOP, graphIDCopyPreview)
	s.copyScript = graphCreateChild(s, "BUTTON", "스크립트 복사", WS_TABSTOP, graphIDCopyScript)
	s.insertBtn = graphCreateChild(s, "BUTTON", "삽입", WS_TABSTOP, graphIDInsert)
	s.cancelBtn = graphCreateChild(s, "BUTTON", "취소", WS_TABSTOP, graphIDCancel)

	graphApplyProportionalRanges(s)
	graphRefreshPreview(s)
}

// graphFocusInitialExpression runs only after the input control has received its
// real layout width. Selecting x^2 while the EDIT is still the 10 px creation
// size makes a single-line ES_AUTOHSCROLL control retain a right-shifted
// horizontal origin after it is enlarged, which can hide the leading x^.
func graphFocusInitialExpression(s *functionGraphEditorState) {
	if s == nil || s.input == 0 {
		return
	}
	procSetFocus.Call(s.input)
	// First force the caret to the beginning and scroll that caret into view.
	// The subsequent select-all keeps the convenient replace-on-type behaviour,
	// now that the edit has its final width and the left edge is visible.
	sendMessage(s.input, EM_SETSEL, 0, 0)
	sendMessage(s.input, graphEMScrollCaret, 0, 0)
	sendMessage(s.input, EM_SETSEL, 0, ^uintptr(0))
}

func graphEditorLayoutControls(s *functionGraphEditorState, client RECT) {
	w := int(client.Right - client.Left)
	h := int(client.Bottom - client.Top)

	// Two compact function rows: ordinary trig/constants above, inverse trig
	// plus exponential/logarithmic functions below.
	x, y := 10, 10
	const rowBreak = 10
	for i, spec := range graphToolbarButtons {
		if i == rowBreak {
			x, y = 10, 40
		}
		if i < len(s.toolbar) {
			procMoveWindow.Call(s.toolbar[i], uintptr(x), uintptr(y), uintptr(spec.width), 26, 1)
		}
		x += spec.width + 4
	}

	// Keep colour inversion beside the function buttons, exactly in the open
	// space after the second-row log button. This keeps the range row compact.
	invertX := x + 8
	const invertW = 110
	procMoveWindow.Call(s.invertBtn, uintptr(invertX), 40, invertW, 26, 1)

	// The polar sample menu starts after the moved inversion button so the two
	// controls never overlap, even when the editor is resized narrower.
	samplePanelW := 400
	sampleX := max(w-samplePanelW-10, invertX+invertW+10)
	procMoveWindow.Call(s.polarSampleLabel, uintptr(sampleX), 14, 82, 20, 1)
	procMoveWindow.Call(s.polarSampleCombo, uintptr(sampleX+86), 10, uintptr(max(w-(sampleX+86)-10, 150)), 240, 1)
	procMoveWindow.Call(s.polarSampleDesc, uintptr(sampleX), 43, uintptr(max(w-sampleX-10, 180)), 24, 1)

	rowY := 76
	procMoveWindow.Call(s.axisAddBtn, 10, uintptr(rowY), 68, 26, 1)
	procMoveWindow.Call(s.axisRemoveBtn, 82, uintptr(rowY), 68, 26, 1)
	procMoveWindow.Call(s.axisLabel, 155, uintptr(rowY+5), 86, 20, 1)
	procMoveWindow.Call(s.angleLabel, 246, uintptr(rowY+5), 32, 20, 1)
	procMoveWindow.Call(s.angleCombo, 280, uintptr(rowY), 78, 160, 1)

	rx := 370
	labelW, editW, gap := 42, 58, 8
	labels := []uintptr{s.xMinLabel, s.xMaxLabel, s.yMinLabel, s.yMaxLabel}
	edits := []uintptr{s.xMinEdit, s.xMaxEdit, s.yMinEdit, s.yMaxEdit}
	for i := range labels {
		procMoveWindow.Call(labels[i], uintptr(rx), uintptr(rowY+5), uintptr(labelW), 20, 1)
		procMoveWindow.Call(edits[i], uintptr(rx+labelW+2), uintptr(rowY+3), uintptr(editW), 20, 1)
		rx += labelW + 2 + editW + gap
	}
	proportionalX := min(max(rx, 690), max(w-174, 10))
	procMoveWindow.Call(s.proportionalCheck, uintptr(proportionalX), uintptr(rowY+2), 164, 22, 1)

	previewTop := 112
	reservedBottom := 142
	previewH := clamp(h-previewTop-reservedBottom, 230, 390)
	previewBottom := previewTop + previewH
	s.previewRect = RECT{10, int32(previewTop), int32(max(w-10, 20)), int32(previewBottom)}

	inputTop := previewBottom + 12
	procMoveWindow.Call(s.functionCombo, 10, uintptr(inputTop), 66, 150, 1)
	procMoveWindow.Call(s.input, 80, uintptr(inputTop), uintptr(max(w-90, 80)), 27, 1)
	procMoveWindow.Call(s.statusLabel, 10, uintptr(inputTop+32), uintptr(max(w-20, 80)), 22, 1)

	buttonY := inputTop + 58
	procMoveWindow.Call(s.copyPreview, 10, uintptr(buttonY), 108, 28, 1)
	procMoveWindow.Call(s.copyScript, 124, uintptr(buttonY), 108, 28, 1)
	procMoveWindow.Call(s.insertBtn, uintptr(max(w-180, 10)), uintptr(buttonY), 78, 28, 1)
	procMoveWindow.Call(s.cancelBtn, uintptr(max(w-96, 10)), uintptr(buttonY), 78, 28, 1)
}

func graphFormatRangeValue(v float64) string {
	if math.Abs(v) < 1e-14 {
		v = 0
	}
	return strconv.FormatFloat(v, 'g', 12, 64)
}

func graphApplyProportionalRanges(s *functionGraphEditorState) {
	if s == nil || !s.proportionalScale || s.updatingRanges || s.xMinEdit == 0 || s.xMaxEdit == 0 {
		return
	}
	xMin, errMin := graphParseFloatEdit(s.xMinEdit, "x 최소")
	xMax, errMax := graphParseFloatEdit(s.xMaxEdit, "x 최대")
	if errMin != nil || errMax != nil || xMin >= xMax {
		return
	}
	yMin, yMax := graphProportionalYRange(xMin, xMax)
	s.updatingRanges = true
	graphSetText(s.yMinEdit, graphFormatRangeValue(yMin))
	graphSetText(s.yMaxEdit, graphFormatRangeValue(yMax))
	s.updatingRanges = false
}

func graphSetProportionalScale(s *functionGraphEditorState, enabled bool) {
	if s == nil || s.proportionalScale == enabled {
		return
	}
	if enabled {
		s.manualYMin = getWindowTextString(s.yMinEdit)
		s.manualYMax = getWindowTextString(s.yMaxEdit)
		s.proportionalScale = true
		procEnableWindow.Call(s.yMinEdit, 0)
		procEnableWindow.Call(s.yMaxEdit, 0)
		graphApplyProportionalRanges(s)
	} else {
		s.proportionalScale = false
		procEnableWindow.Call(s.yMinEdit, 1)
		procEnableWindow.Call(s.yMaxEdit, 1)
		s.updatingRanges = true
		if strings.TrimSpace(s.manualYMin) != "" {
			graphSetText(s.yMinEdit, s.manualYMin)
		}
		if strings.TrimSpace(s.manualYMax) != "" {
			graphSetText(s.yMaxEdit, s.manualYMax)
		}
		s.updatingRanges = false
	}
}

func graphParseFloatEdit(hwnd uintptr, label string) (float64, error) {
	raw := strings.TrimSpace(getWindowTextString(hwnd))
	raw = strings.ReplaceAll(raw, "−", "-")
	if raw == "" {
		return 0, fmt.Errorf("%s 값을 입력하십시오", label)
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("%s 값 %q을(를) 숫자로 해석할 수 없습니다", label, raw)
	}
	return v, nil
}

func graphFunctionVariable(mode GraphFunctionMode) string {
	switch mode {
	case GraphFunctionOfY:
		return "y"
	case GraphFunctionPolar:
		return "r"
	default:
		return "x"
	}
}

func graphFunctionModeLabel(mode GraphFunctionMode) string {
	return "f(" + graphFunctionVariable(mode) + ")"
}

func graphFunctionModePrefix(mode GraphFunctionMode) string {
	return graphFunctionModeLabel(mode) + "="
}

func graphReplaceVariableIdentifier(source, oldVariable, newVariable string) string {
	if oldVariable == "" || oldVariable == newVariable {
		return source
	}
	runes := []rune(source)
	var out strings.Builder
	for i := 0; i < len(runes); {
		if unicode.IsLetter(runes[i]) || runes[i] == 'π' {
			start := i
			i++
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == 'π') {
				i++
			}
			token := string(runes[start:i])
			if strings.EqualFold(token, oldVariable) {
				out.WriteString(newVariable)
			} else {
				out.WriteString(token)
			}
			continue
		}
		out.WriteRune(runes[i])
		i++
	}
	return out.String()
}

func graphModeFromExpressionPrefix(source string) (GraphFunctionMode, bool) {
	lower := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(source), " ", ""))
	switch {
	case strings.HasPrefix(lower, "f(y)="):
		return GraphFunctionOfY, true
	case strings.HasPrefix(lower, "f(r)="):
		return GraphFunctionPolar, true
	case strings.HasPrefix(lower, "f(x)="):
		return GraphFunctionOfX, true
	default:
		return GraphFunctionOfX, false
	}
}

func graphNormalizeExpression(source string) string {
	source = strings.TrimSpace(source)
	lower := strings.ToLower(strings.ReplaceAll(source, " ", ""))
	prefixes := []string{"f(x)=", "f(y)=", "f(r)=", "y=", "x=", "r="}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			// Find the first equals sign in the original string so whitespace
			// and Unicode characters after it remain untouched.
			if i := strings.Index(source, "="); i >= 0 {
				return strings.TrimSpace(source[i+1:])
			}
		}
	}
	return source
}

func graphPolarSampleIndex(expression string) int {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(graphNormalizeExpression(expression)), " ", ""))
	for i, sample := range graphPolarSamples {
		candidate := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(sample.expression), " ", ""))
		if normalized == candidate {
			return i + 1
		}
	}
	return 0
}

func graphUpdatePolarSampleUI(s *functionGraphEditorState) {
	if s == nil || s.polarSampleCombo == 0 {
		return
	}
	polar := s.functionMode == GraphFunctionPolar
	if polar {
		procEnableWindow.Call(s.polarSampleCombo, 1)
		idx := graphPolarSampleIndex(getWindowTextString(s.input))
		sendMessage(s.polarSampleCombo, CB_SETCURSEL, uintptr(idx), 0)
		if idx > 0 && idx <= len(graphPolarSamples) {
			sample := graphPolarSamples[idx-1]
			graphSetText(s.polarSampleDesc, sample.description+"  f(r)="+sample.expression)
		} else {
			graphSetText(s.polarSampleDesc, "사용자 정의 극좌표 함수입니다. 샘플을 고르면 입력식이 즉시 바뀝니다.")
		}
	} else {
		procEnableWindow.Call(s.polarSampleCombo, 0)
		sendMessage(s.polarSampleCombo, CB_SETCURSEL, 0, 0)
		graphSetText(s.polarSampleDesc, "f(r) 모드에서 극좌표 샘플을 선택할 수 있습니다.")
	}
}

func graphUpdateInvertButton(s *functionGraphEditorState) {
	if s == nil || s.invertBtn == 0 {
		return
	}
	label := "검은 배경: 끔"
	if s.invert {
		label = "검은 배경: 켬"
	}
	graphSetText(s.invertBtn, label)
	procInvalidateRect.Call(s.invertBtn, 0, 1)
}

func graphRefreshPreview(s *functionGraphEditorState) {
	if s == nil || s.input == 0 {
		return
	}
	if s.proportionalScale && !s.updatingRanges {
		graphApplyProportionalRanges(s)
	}
	xMin, err := graphParseFloatEdit(s.xMinEdit, "x 최소")
	xMax, errXMax := graphParseFloatEdit(s.xMaxEdit, "x 최대")
	yMin, errYMin := graphParseFloatEdit(s.yMinEdit, "y 최소")
	yMax, errYMax := graphParseFloatEdit(s.yMaxEdit, "y 최대")
	if err == nil {
		err = errXMax
	}
	if err == nil {
		err = errYMin
	}
	if err == nil {
		err = errYMax
	}

	rawExpression := getWindowTextString(s.input)
	if prefixedMode, ok := graphModeFromExpressionPrefix(rawExpression); ok && prefixedMode != s.functionMode {
		s.functionMode = prefixedMode
		sendMessage(s.functionCombo, CB_SETCURSEL, uintptr(prefixedMode), 0)
	}
	expr := graphNormalizeExpression(rawExpression)
	s.lastExpr = expr
	s.preview = ClipboardImage{}
	if err == nil {
		var img FunctionGraphImage
		img, err = renderFunctionGraph(GraphRenderSpec{
			Width: 640, Height: 480,
			XMin: xMin, XMax: xMax, YMin: yMin, YMax: yMax,
			ShowAxes: s.showAxes, ProportionalScale: s.proportionalScale,
			AngleMode: s.angleMode, FunctionMode: s.functionMode,
			Expression: expr, Color: s.owner.fg,
		})
		if err == nil {
			s.preview = ClipboardImage{W: img.W, H: img.H, Pix: img.Pix, Mask: img.Mask}
			// "검은 배경" changes only the preview background. Curves, axes
			// and labels retain the exact current foreground colour and alpha.
		}
	}
	if err != nil {
		s.lastError = err.Error()
		graphSetText(s.statusLabel, "오류: "+s.lastError)
	} else {
		s.lastError = ""
		unit := "라디안"
		if s.angleMode == GraphDegrees {
			unit = "도"
		}
		axis := "없음"
		if s.showAxes {
			axis = "표시"
		}
		scaleText := "독립 눈금"
		if s.proportionalScale {
			scaleText = "x, y 정비례"
		}
		graphSetText(s.statusLabel, fmt.Sprintf("640 × 480 px · %s · 축 %s · %s · %s · 검은 배경 %v", graphFunctionModeLabel(s.functionMode), axis, unit, scaleText, s.invert))
	}
	axisText := "축: 없음"
	if s.showAxes {
		axisText = "축: 표시"
	}
	graphSetText(s.axisLabel, axisText)
	graphUpdatePolarSampleUI(s)
	procInvalidateRect.Call(s.hwnd, uintptr(unsafe.Pointer(&s.previewRect)), 0)
}

func graphInsertTemplate(s *functionGraphEditorState, text string) {
	if s == nil || s.input == 0 {
		return
	}
	r := sendMessage(s.input, formulaEMGetSel, 0, 0)
	start := int(uint16(r & 0xFFFF))
	sendMessage(s.input, formulaEMReplaceSel, 1, uintptr(unsafe.Pointer(utf16Ptr(text))))
	idx := strings.Index(text, "□")
	if idx >= 0 {
		before := utf16UnitLen(text[:idx])
		length := utf16UnitLen("□")
		sendMessage(s.input, EM_SETSEL, uintptr(start+before), uintptr(start+before+length))
	}
	procSetFocus.Call(s.input)
}

func graphCopyPreviewImage(s *functionGraphEditorState) error {
	if s == nil || s.owner == nil || !validClipboardImage(s.preview) {
		return fmt.Errorf("복사할 그래프 미리보기가 없습니다")
	}
	// Reuse the formula editor's compatibility path: opaque CF_DIB for legacy
	// programs and registered PNG formats for alpha-aware applications.
	dib, err := buildOpaqueClipboardDIB(s.preview.W, s.preview.H, s.preview.Pix)
	if err != nil {
		return fmt.Errorf("호환 비트맵 생성 실패: %w", err)
	}
	pngData, pngErr := buildTransparentClipboardPNG(s.preview.W, s.preview.H, s.preview.Pix)
	type item struct {
		format uintptr
		data   []byte
		handle uintptr
		must   bool
	}
	items := []item{{format: CF_DIB, data: dib, must: true}}
	if pngErr == nil && len(pngData) > 0 {
		if clipboardPNGFormat != 0 {
			items = append(items, item{format: clipboardPNGFormat, data: pngData})
		}
		if clipboardImagePNGFormat != 0 && clipboardImagePNGFormat != clipboardPNGFormat {
			items = append(items, item{format: clipboardImagePNGFormat, data: pngData})
		}
	}
	for i := range items {
		h, allocErr := formulaAllocClipboardBlock(items[i].data)
		if allocErr != nil {
			if items[i].must {
				for j := range items {
					if items[j].handle != 0 {
						procGlobalFree.Call(items[j].handle)
					}
				}
				return allocErr
			}
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
			if items[i].must {
				return fmt.Errorf("Windows 클립보드에 그래프를 넣지 못했습니다: %v", setErr)
			}
			continue
		}
		items[i].handle = 0
	}
	seq, _, _ := procGetClipboardSequenceNumber.Call()
	s.owner.lastClipboardSeq = uint32(seq)
	return nil
}

func graphCommitInsert(s *functionGraphEditorState) {
	if s == nil {
		return
	}
	graphRefreshPreview(s)
	if s.lastError != "" || !validClipboardImage(s.preview) {
		message := s.lastError
		if message == "" {
			message = "삽입할 그래프가 없습니다"
		}
		messageBox(s.hwnd, message, "함수/그래프 편집기", MB_OK|MB_ICONWARNING)
		return
	}
	img := s.preview
	expr := s.lastExpr
	owner := s.owner
	procDestroyWindow.Call(s.hwnd)
	if owner.tool == ToolText && owner.textTool.Active {
		owner.finishTextEditing(true)
	}
	if owner.curveUndoPending {
		owner.finishCurveEarlyOrCancel("function graph insert")
	}
	owner.pasteClipboardImage(img, "함수/그래프 편집기")
	owner.statusText = fmt.Sprintf("%s%s 그래프를 640 × 480 투명 이미지로 삽입했습니다.", graphFunctionModePrefix(s.functionMode), expr)
	logf("GRAPH inserted mode=%d expr=%q axes=%v proportional=%v angle=%d black-background=%v", s.functionMode, expr, s.showAxes, s.proportionalScale, s.angleMode, s.invert)
}

func graphHandleCommand(s *functionGraphEditorState, id, code int) {
	if s == nil {
		return
	}
	if code == EN_CHANGE {
		if s.updatingRanges {
			return
		}
		switch id {
		case graphIDXMin, graphIDXMax:
			if s.proportionalScale {
				graphApplyProportionalRanges(s)
			}
			graphRefreshPreview(s)
			return
		case graphIDYMin, graphIDYMax:
			if !s.proportionalScale {
				s.manualYMin = getWindowTextString(s.yMinEdit)
				s.manualYMax = getWindowTextString(s.yMaxEdit)
			}
			graphRefreshPreview(s)
			return
		case graphIDInput:
			graphRefreshPreview(s)
			return
		}
	}
	if id == graphIDFunctionMode && code == graphCBNSelection {
		idx := int(sendMessage(s.functionCombo, CB_GETCURSEL, 0, 0))
		newMode := GraphFunctionOfX
		if idx == 1 {
			newMode = GraphFunctionOfY
		} else if idx == 2 {
			newMode = GraphFunctionPolar
		}
		if newMode != s.functionMode {
			oldVariable := graphFunctionVariable(s.functionMode)
			newVariable := graphFunctionVariable(newMode)
			current := getWindowTextString(s.input)
			graphSetText(s.input, graphReplaceVariableIdentifier(current, oldVariable, newVariable))
			s.functionMode = newMode
		}
		graphRefreshPreview(s)
		return
	}
	if id == graphIDPolarSample && code == graphCBNSelection {
		if s.functionMode != GraphFunctionPolar {
			return
		}
		idx := int(sendMessage(s.polarSampleCombo, CB_GETCURSEL, 0, 0))
		if idx > 0 && idx <= len(graphPolarSamples) {
			sample := graphPolarSamples[idx-1]
			graphSetText(s.input, sample.expression)
			graphSetText(s.polarSampleDesc, sample.description+"  f(r)="+sample.expression)
			procSetFocus.Call(s.input)
			end := utf16UnitLen(sample.expression)
			sendMessage(s.input, EM_SETSEL, uintptr(end), uintptr(end))
		}
		graphRefreshPreview(s)
		return
	}
	if id == graphIDAngle && code == graphCBNSelection {
		idx := int(sendMessage(s.angleCombo, CB_GETCURSEL, 0, 0))
		if idx == 1 {
			s.angleMode = GraphDegrees
		} else {
			s.angleMode = GraphRadians
		}
		graphRefreshPreview(s)
		return
	}
	if code != BN_CLICKED {
		return
	}
	switch id {
	case graphIDProportional:
		enabled := sendMessage(s.proportionalCheck, BM_GETCHECK, 0, 0) == BST_CHECKED
		graphSetProportionalScale(s, enabled)
		graphRefreshPreview(s)
		return
	case graphIDAxisAdd:
		s.showAxes = true
		graphRefreshPreview(s)
		return
	case graphIDAxisRemove:
		s.showAxes = false
		graphRefreshPreview(s)
		return
	case graphIDInvert:
		s.invert = !s.invert
		graphUpdateInvertButton(s)
		graphRefreshPreview(s)
		logf("GRAPH black-background=%v", s.invert)
		return
	case graphIDCopyPreview:
		if err := graphCopyPreviewImage(s); err != nil {
			messageBox(s.hwnd, err.Error(), "미리보기 복사", MB_OK|MB_ICONERROR)
		} else {
			s.owner.statusText = "그래프 미리보기를 이미지로 복사했습니다."
		}
		return
	case graphIDCopyScript:
		script := graphFunctionModePrefix(s.functionMode) + graphNormalizeExpression(getWindowTextString(s.input))
		if err := formulaCopyUnicodeText(s.hwnd, script); err != nil {
			messageBox(s.hwnd, err.Error(), "스크립트 복사", MB_OK|MB_ICONERROR)
		} else {
			s.owner.statusText = "함수 스크립트를 텍스트로 복사했습니다."
		}
		return
	case graphIDInsert:
		graphCommitInsert(s)
		return
	case graphIDCancel:
		procDestroyWindow.Call(s.hwnd)
		return
	}
	idx := id - graphIDToolbarBase
	if idx >= 0 && idx < len(graphToolbarButtons) {
		graphInsertTemplate(s, graphToolbarButtons[idx].text)
	}
}

func graphPaintEditor(s *functionGraphEditorState, hdc uintptr) {
	var client RECT
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	fillUIRect(hdc, client, rgb(240, 240, 240))
	pr := s.previewRect
	previewBG := rgb(255, 255, 255)
	if s.invert {
		previewBG = rgb(18, 18, 18)
	}
	fillRect(hdc, pr, previewBG)
	frameRect(hdc, pr, uiColor(rgb(150, 150, 150)))
	if !validClipboardImage(s.preview) {
		return
	}
	bgByte := byte(255)
	if s.invert {
		bgByte = 18
	}
	display := formulaCompositePreviewOn(s.preview, bgByte)
	availW := max(int(pr.Right-pr.Left)-20, 1)
	availH := max(int(pr.Bottom-pr.Top)-20, 1)
	dw, dh := s.preview.W, s.preview.H
	scale := math.Min(float64(availW)/float64(dw), float64(availH)/float64(dh))
	if scale > 1 {
		scale = 1
	}
	dw = max(1, int(math.Round(float64(dw)*scale)))
	dh = max(1, int(math.Round(float64(dh)*scale)))
	dx := int(pr.Left) + (int(pr.Right-pr.Left)-dw)/2
	dy := int(pr.Top) + (int(pr.Bottom-pr.Top)-dh)/2
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{
		BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(s.preview.W), BiHeight: -int32(s.preview.H),
		BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB,
	}}
	procSetStretchBltMode.Call(hdc, COLORONCOLOR)
	procStretchDIBits.Call(
		hdc, uintptr(dx), uintptr(dy), uintptr(dw), uintptr(dh),
		0, 0, uintptr(s.preview.W), uintptr(s.preview.H),
		uintptr(unsafe.Pointer(&display[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, SRCCOPY,
	)
}

func functionGraphEditorWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			logPanic("functionGraphEditorWndProc", r)
		}
	}()
	s := activeFunctionGraphEditor
	if r, handled := handleDarkEditorNonClient(hwnd, msg, wparam, lparam); handled {
		return r
	}
	switch msg {
	case WM_CREATE:
		if s == nil {
			return ^uintptr(0)
		}
		s.hwnd = hwnd
		graphEditorCreateControls(s)
		var client RECT
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
		graphEditorLayoutControls(s, client)
		graphFocusInitialExpression(s)
		// Apply caption theming during WM_CREATE, as the help centre does.
		applyDarkThemeToWindow(hwnd, darkModeEnabled())
		drawDarkEditorCaption(hwnd)
		scheduleDarkEditorChromeRefresh(hwnd)
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
			graphEditorLayoutControls(s, client)
			graphRefreshPreview(s)
		}
		return 0
	case WM_COMMAND:
		if s != nil && s.hwnd == hwnd {
			id := int(wparam & 0xFFFF)
			code := int((wparam >> 16) & 0xFFFF)
			graphHandleCommand(s, id, code)
		}
		return 0
	case WM_DRAWITEM:
		if lparam != 0 && drawThemedOwnerDrawPushButton((*DRAWITEMSTRUCT)(unsafe.Pointer(lparam))) {
			return 1
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
			s.paintBuffer.paint(hwnd, hdc, ps.RcPaint, func(targetDC uintptr) { graphPaintEditor(s, targetDC) })
			procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			return 0
		}
	case WM_ERASEBKGND:
		return 1
	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		if s != nil && s.hwnd == hwnd {
			s.paintBuffer.release()
			if s.uiFont != 0 {
				procDeleteObject.Call(s.uiFont)
				s.uiFont = 0
			}
			logf("GRAPH editor closed")
			activeFunctionGraphEditor = nil
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}
