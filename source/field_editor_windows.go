//go:build windows

package main

import (
	"fmt"
	"math"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	fieldEditorClassName    = "dotPaintFieldEditor122"
	fieldPaletteWidth       = 148
	fieldPreviewTimerID     = 0xF128
	fieldPreviewDoneMessage = 0x8000 + 0x121
	fieldInsertDoneMessage  = 0x8000 + 0x122
)

const (
	fieldIDDimension = 8101 + iota
	fieldIDDisplay
	fieldIDSpecialLayout
	fieldIDApplySpecial
	fieldIDObject
	fieldIDMaterial
	fieldIDCharge
	fieldIDShapeSize
	fieldIDSpin
	fieldIDObjectRotX
	fieldIDObjectRotY
	fieldIDObjectRotZ
	fieldIDDensity
	fieldIDViewRotX
	fieldIDViewRotY
	fieldIDViewRotZ
	fieldIDViewDistance
	fieldIDAntialias
	fieldIDBlack
	fieldIDDelete
	fieldIDClear
	fieldIDInsert
	fieldIDCancel
)

type fieldEditorState struct {
	owner  *App
	hwnd   uintptr
	uiFont uintptr

	dimensionCombo uintptr
	displayCombo   uintptr
	specialCombo   uintptr
	specialBtn     uintptr
	objectCombo    uintptr
	materialCombo  uintptr

	chargeLabel uintptr
	charge      uintptr
	chargeValue uintptr
	sizeLabel   uintptr
	sizeSlider  uintptr
	sizeValue   uintptr
	spinLabel   uintptr
	spinSlider  uintptr
	spinValue   uintptr

	objectRotXLabel uintptr
	objectRotX      uintptr
	objectRotXValue uintptr
	objectRotYLabel uintptr
	objectRotY      uintptr
	objectRotYValue uintptr
	objectRotZLabel uintptr
	objectRotZ      uintptr
	objectRotZValue uintptr

	densityLabel      uintptr
	density           uintptr
	densityValue      uintptr
	viewRotXLabel     uintptr
	viewRotX          uintptr
	viewRotXValue     uintptr
	viewRotYLabel     uintptr
	viewRotY          uintptr
	viewRotYValue     uintptr
	viewRotZLabel     uintptr
	viewRotZ          uintptr
	viewRotZValue     uintptr
	viewDistanceLabel uintptr
	viewDistance      uintptr
	viewDistanceValue uintptr

	antialiasCheck uintptr
	blackCheck     uintptr
	deleteBtn      uintptr
	clearBtn       uintptr
	insertBtn      uintptr
	cancelBtn      uintptr
	status         uintptr

	scene           fieldScene
	electricObjects []fieldObject
	magneticObjects []fieldObject
	gravityObjects  []fieldObject
	selected        int
	black           bool
	preview         fieldRaster
	stablePreview   fieldRaster // last completed non-empty frame; never cleared while recomputing
	previewRect     RECT
	paletteRect     RECT
	sceneRect       RECT

	dragPalette     int
	dragObject      int
	dragging        bool
	dragGrabDX      float64 // pointer minus projected object centre, in preview pixels
	dragGrabDY      float64
	dragPlaneY      float64 // preserve volumetric objects' current world depth
	dragAnchorValid bool
	updating        bool

	previewQueued bool
	previewMode   int // 0=none, 1=interactive, 2=final quality
	previewDirty  bool

	asyncMu              sync.Mutex
	previewRenderBusy    bool
	previewSerial        uint64
	previewPending       bool
	previewPendingReason string
	previewPendingFast   bool
	previewResult        *fieldAsyncPreviewResult
	previewCancel        *fieldRenderCancellation
	insertBusy           bool
	insertSerial         uint64
	insertResult         *fieldAsyncInsertResult
	insertCancel         *fieldRenderCancellation
	destroyed            bool

	previewRevision   uint64
	compositeRevision uint64
	compositeBlack    bool
	compositePix      []byte

	paintDC     uintptr
	paintBitmap uintptr
	paintOld    uintptr
	paintW      int
	paintH      int
}

var (
	fieldEditorWndProcCallback uintptr
	fieldEditorClassRegistered bool
	activeFieldEditor          *fieldEditorState
)

type fieldAsyncPreviewResult struct {
	Serial           uint64
	Reason           string
	Interactive      bool
	Raster           fieldRaster
	Scene            fieldScene
	Quality          int
	Visible          int
	FallbackGeometry bool
	Provisional      bool
	Cancelled        bool
	Valid            bool
	Elapsed          time.Duration
}

type fieldAsyncInsertResult struct {
	Serial  uint64
	Raster  fieldRaster
	Elapsed time.Duration
}

var fieldElectricPaletteKinds = []fieldObjectKind{
	fieldObjectSphere,
	fieldObjectRod,
	fieldObjectPlane,
}

var fieldMagneticPaletteKinds = []fieldObjectKind{
	fieldObjectBarMagnet,
	fieldObjectCurrentWire,
	fieldObjectHorseshoeMagnet,
	fieldObjectCurrentLoop,
	fieldObjectFerromagnet,
}

var fieldGravityPaletteKinds = []fieldObjectKind{
	fieldObjectSolidMassSphere,
	fieldObjectHollowMassSphere,
	fieldObjectMassPlane,
	fieldObjectMassRing,
}

func fieldPaletteKindsForScene(scene fieldScene) []fieldObjectKind {
	if scene.Display.gravity() {
		return fieldGravityPaletteKinds
	}
	if scene.Display.magnetic() {
		return fieldMagneticPaletteKinds
	}
	return fieldElectricPaletteKinds
}

func fieldKindName(k fieldObjectKind) string {
	switch k {
	case fieldObjectSphere:
		return "구전하"
	case fieldObjectRod:
		return "막대전하"
	case fieldObjectPlane:
		return "면전하"
	case fieldObjectBarMagnet:
		return "막대자석"
	case fieldObjectCurrentWire:
		return "전류선"
	case fieldObjectHorseshoeMagnet:
		return "말굽자석"
	case fieldObjectCurrentLoop:
		return "원형 고리 전류"
	case fieldObjectFerromagnet:
		return "강자성체"
	case fieldObjectSolidMassSphere:
		return "구 질량 (꽉 찬 구)"
	case fieldObjectHollowMassSphere:
		return "구 질량 (속 빈 구각)"
	case fieldObjectMassPlane:
		return "면 질량"
	case fieldObjectMassRing:
		return "링 질량"
	}
	return "물체"
}

func fieldMaterialName(m fieldMaterial) string {
	if m == fieldMaterialConductor {
		return "도체"
	}
	return "절연체"
}

func registerFieldEditorClass(hinst uintptr) bool {
	if fieldEditorClassRegistered {
		return true
	}
	if fieldEditorWndProcCallback == 0 {
		fieldEditorWndProcCallback = syscall.NewCallback(fieldEditorWndProc)
	}
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{
		CbSize: uint32(unsafe.Sizeof(WNDCLASSEXW{})), Style: CS_HREDRAW | CS_VREDRAW,
		LpfnWndProc: fieldEditorWndProcCallback, HInstance: hinst, HCursor: cursor,
		HbrBackground: uintptr(COLOR_BTNFACE + 1), LpszClassName: utf16Ptr(fieldEditorClassName),
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		logf("FIELD register class failed: %v", err)
		return false
	}
	fieldEditorClassRegistered = true
	return true
}

func (a *App) showFieldEditor() {
	mdD3D11PrewarmField()
	if activeFieldEditor != nil && activeFieldEditor.hwnd != 0 {
		if r, _, _ := procIsWindow.Call(activeFieldEditor.hwnd); r != 0 {
			procShowWindow.Call(activeFieldEditor.hwnd, formulaSWRestore)
			procSetForegroundWindow.Call(activeFieldEditor.hwnd)
			return
		}
	}
	if !registerFieldEditorClass(a.hinst) {
		messageBox(a.hwnd, "장 그리기 창을 만들 수 없습니다.", "삽입 > 이미지 > 장 그리기", MB_OK|MB_ICONERROR)
		return
	}
	icc := INITCOMMONCONTROLSEX{DwSize: uint32(unsafe.Sizeof(INITCOMMONCONTROLSEX{})), DwICC: ICC_BAR_CLASSES}
	procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))
	state := &fieldEditorState{owner: a, uiFont: createCompactEditorUIFont(), scene: fieldDefaultScene(), selected: -1, black: a.darkMode, dragPalette: -1, dragObject: -1}
	activeFieldEditor = state
	hwnd, _, err := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(utf16Ptr(fieldEditorClassName))), uintptr(unsafe.Pointer(utf16Ptr("장 그리기"))),
		WS_OVERLAPPEDWINDOW, uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), 1240, 870,
		a.hwnd, 0, a.hinst, 0,
	)
	if hwnd == 0 {
		if state.uiFont != 0 {
			procDeleteObject.Call(state.uiFont)
			state.uiFont = 0
		}
		activeFieldEditor = nil
		logf("FIELD create window failed: %v", err)
		messageBox(a.hwnd, "장 그리기 창을 만들 수 없습니다.", "삽입 > 이미지 > 장 그리기", MB_OK|MB_ICONERROR)
		return
	}
	state.hwnd = hwnd
	showThemedEditorWindow(hwnd, a.darkMode)
	a.statusText = "장 그리기: 전기·자기·중력 영역에서 퍼텐셜·장선·격자 벡터장을 그립니다."
	logf("FIELD editor opened hwnd=%#x", hwnd)
}

func fieldCreateChild(s *fieldEditorState, class, title string, style uintptr, id int) uintptr {
	style = ownerDrawPushButtonStyle(class, style)
	h, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(utf16Ptr(class))), uintptr(unsafe.Pointer(utf16Ptr(title))),
		style|WS_CHILD|WS_VISIBLE, 0, 0, 10, 10, s.hwnd, uintptr(id), s.owner.hinst, 0,
	)
	if h != 0 {
		applyDarkThemeToWindow(h, darkModeEnabled())
		setCompactEditorControlFont(h, s.uiFont)
	}
	return h
}

func fieldSetText(hwnd uintptr, text string) {
	if hwnd != 0 {
		procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(text))))
	}
}

func fieldAddComboText(hwnd uintptr, text string) {
	sendMessage(hwnd, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(text))))
}

func fieldSliderSetPosSigned(hwnd uintptr, pos int) {
	sendMessage(hwnd, TBM_SETPOS, 1, uintptr(uint32(int32(pos))))
}

func fieldEditorCreateControls(s *fieldEditorState) {
	s.dimensionCombo = fieldCreateChild(s, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|CBS_HASSTRINGS, fieldIDDimension)
	fieldAddComboText(s.dimensionCombo, "2차원")
	fieldAddComboText(s.dimensionCombo, "3차원")
	sendMessage(s.dimensionCombo, CB_SETCURSEL, 0, 0)

	s.displayCombo = fieldCreateChild(s, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|CBS_HASSTRINGS, fieldIDDisplay)
	fieldAddComboText(s.displayCombo, "등전위선 / 등전위면")
	fieldAddComboText(s.displayCombo, "전기력선")
	fieldAddComboText(s.displayCombo, "전기장 (격자 벡터)")
	fieldAddComboText(s.displayCombo, "자기력선")
	fieldAddComboText(s.displayCombo, "자기장 (격자 벡터)")
	fieldAddComboText(s.displayCombo, "중력 퍼텐셜선 / 퍼텐셜면")
	fieldAddComboText(s.displayCombo, "중력장선")
	fieldAddComboText(s.displayCombo, "중력장 (격자 벡터)")
	sendMessage(s.displayCombo, CB_SETCURSEL, uintptr(s.scene.Display), 0)

	s.specialCombo = fieldCreateChild(s, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|CBS_HASSTRINGS, fieldIDSpecialLayout)
	fieldAddComboText(s.specialCombo, "평면 배치")
	fieldAddComboText(s.specialCombo, "점전하: 정사면체 꼭짓점")
	fieldAddComboText(s.specialCombo, "점전하: 정팔면체 꼭짓점")
	fieldAddComboText(s.specialCombo, "점전하: 정육면체 꼭짓점")
	sendMessage(s.specialCombo, CB_SETCURSEL, 0, 0)
	s.specialBtn = fieldCreateChild(s, "BUTTON", "배치 적용", WS_TABSTOP, fieldIDApplySpecial)

	s.objectCombo = fieldCreateChild(s, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|CBS_HASSTRINGS, fieldIDObject)
	fieldAddComboText(s.objectCombo, "선택 물체 없음")
	sendMessage(s.objectCombo, CB_SETCURSEL, 0, 0)

	s.materialCombo = fieldCreateChild(s, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|CBS_HASSTRINGS, fieldIDMaterial)
	fieldAddComboText(s.materialCombo, "절연체: 고정 균일 전하")
	fieldAddComboText(s.materialCombo, "도체: 정전기 유도 계산")
	sendMessage(s.materialCombo, CB_SETCURSEL, 0, 0)

	s.chargeLabel = fieldCreateChild(s, "STATIC", "전하량", 0, 0)
	s.charge = fieldCreateChild(s, "msctls_trackbar32", "", TBS_HORZ|TBS_AUTOTICKS, fieldIDCharge)
	s.chargeValue = fieldCreateChild(s, "STATIC", "+1.0", ssCenter|ssCenterImage, 0)
	multidimSliderSet(s.charge, -40, 40, 10)
	sendMessage(s.charge, TBM_SETTICFREQ, 10, 0)

	s.sizeLabel = fieldCreateChild(s, "STATIC", "크기", 0, 0)
	s.sizeSlider = fieldCreateChild(s, "msctls_trackbar32", "", TBS_HORZ|TBS_AUTOTICKS, fieldIDShapeSize)
	s.sizeValue = fieldCreateChild(s, "STATIC", "-", ssCenter|ssCenterImage, 0)
	multidimSliderSet(s.sizeSlider, 18, 275, 48)

	s.spinLabel = fieldCreateChild(s, "STATIC", "회전속도", 0, 0)
	s.spinSlider = fieldCreateChild(s, "msctls_trackbar32", "", TBS_HORZ|TBS_AUTOTICKS, fieldIDSpin)
	s.spinValue = fieldCreateChild(s, "STATIC", "0.00", ssCenter|ssCenterImage, 0)
	multidimSliderSet(s.spinSlider, 0, 99, 0)
	sendMessage(s.spinSlider, TBM_SETTICFREQ, 10, 0)

	s.objectRotXLabel = fieldCreateChild(s, "STATIC", "물체 자체 x 자전", 0, 0)
	s.objectRotX = fieldCreateChild(s, "msctls_trackbar32", "", TBS_HORZ|TBS_AUTOTICKS, fieldIDObjectRotX)
	s.objectRotXValue = fieldCreateChild(s, "STATIC", "0°", ssCenter|ssCenterImage, 0)
	s.objectRotYLabel = fieldCreateChild(s, "STATIC", "물체 자체 y 자전", 0, 0)
	s.objectRotY = fieldCreateChild(s, "msctls_trackbar32", "", TBS_HORZ|TBS_AUTOTICKS, fieldIDObjectRotY)
	s.objectRotYValue = fieldCreateChild(s, "STATIC", "0°", ssCenter|ssCenterImage, 0)
	s.objectRotZLabel = fieldCreateChild(s, "STATIC", "물체 자체 z 자전", 0, 0)
	s.objectRotZ = fieldCreateChild(s, "msctls_trackbar32", "", TBS_HORZ|TBS_AUTOTICKS, fieldIDObjectRotZ)
	s.objectRotZValue = fieldCreateChild(s, "STATIC", "0°", ssCenter|ssCenterImage, 0)
	for _, h := range []uintptr{s.objectRotX, s.objectRotY, s.objectRotZ} {
		multidimSliderSet(h, -fieldRotationSliderHalf, fieldRotationSliderHalf, 0)
		sendMessage(h, TBM_SETTICFREQ, 18, 0) // 18 positions × 5° = 90°
	}

	s.densityLabel = fieldCreateChild(s, "STATIC", "전하 1당 선 수", 0, 0)
	s.density = fieldCreateChild(s, "msctls_trackbar32", "", TBS_HORZ|TBS_AUTOTICKS, fieldIDDensity)
	s.densityValue = fieldCreateChild(s, "STATIC", "10", ssCenter|ssCenterImage, 0)
	multidimSliderSet(s.density, 4, 24, s.scene.Density)

	s.viewRotXLabel = fieldCreateChild(s, "STATIC", "시점 x 회전", 0, 0)
	s.viewRotX = fieldCreateChild(s, "msctls_trackbar32", "", TBS_HORZ|TBS_AUTOTICKS, fieldIDViewRotX)
	s.viewRotXValue = fieldCreateChild(s, "STATIC", "0°", ssCenter|ssCenterImage, 0)
	s.viewRotYLabel = fieldCreateChild(s, "STATIC", "시점 y 회전", 0, 0)
	s.viewRotY = fieldCreateChild(s, "msctls_trackbar32", "", TBS_HORZ|TBS_AUTOTICKS, fieldIDViewRotY)
	s.viewRotYValue = fieldCreateChild(s, "STATIC", "0°", ssCenter|ssCenterImage, 0)
	s.viewRotZLabel = fieldCreateChild(s, "STATIC", "시점 z 회전", 0, 0)
	s.viewRotZ = fieldCreateChild(s, "msctls_trackbar32", "", TBS_HORZ|TBS_AUTOTICKS, fieldIDViewRotZ)
	s.viewRotZValue = fieldCreateChild(s, "STATIC", "0°", ssCenter|ssCenterImage, 0)

	s.viewDistanceLabel = fieldCreateChild(s, "STATIC", "관측 거리", 0, 0)
	s.viewDistance = fieldCreateChild(s, "msctls_trackbar32", "", TBS_HORZ|TBS_AUTOTICKS, fieldIDViewDistance)
	s.viewDistanceValue = fieldCreateChild(s, "STATIC", "1.0", ssCenter|ssCenterImage, 0)
	multidimSliderSet(s.viewDistance, 1, 30, 10)
	sendMessage(s.viewDistance, TBM_SETTICFREQ, 5, 0)
	for _, h := range []uintptr{s.viewRotX, s.viewRotY, s.viewRotZ} {
		multidimSliderSet(h, -fieldRotationSliderHalf, fieldRotationSliderHalf, 0)
		sendMessage(h, TBM_SETTICFREQ, 18, 0) // 0° centre, major ticks at ±90°
	}
	fieldSliderSetPosSigned(s.viewRotX, fieldRotationSliderPosition(s.scene.RotX))
	fieldSliderSetPosSigned(s.viewRotY, fieldRotationSliderPosition(s.scene.RotY))
	fieldSliderSetPosSigned(s.viewRotZ, fieldRotationSliderPosition(s.scene.RotZ))
	fieldSliderSetPosSigned(s.viewDistance, int(math.Round(fieldObserverDistance(s.scene.ViewDistance)*10)))

	s.antialiasCheck = fieldCreateChild(s, "BUTTON", "부드럽게 (6×)", WS_TABSTOP|BS_AUTOCHECKBOX, fieldIDAntialias)
	if s.scene.Antialias {
		sendMessage(s.antialiasCheck, BM_SETCHECK, BST_CHECKED, 0)
	}
	s.blackCheck = fieldCreateChild(s, "BUTTON", "검은 배경", WS_TABSTOP|BS_AUTOCHECKBOX, fieldIDBlack)
	if s.black {
		sendMessage(s.blackCheck, BM_SETCHECK, BST_CHECKED, 0)
	}
	s.deleteBtn = fieldCreateChild(s, "BUTTON", "선택 삭제", WS_TABSTOP, fieldIDDelete)
	s.clearBtn = fieldCreateChild(s, "BUTTON", "모두 지우기", WS_TABSTOP, fieldIDClear)
	s.insertBtn = fieldCreateChild(s, "BUTTON", "삽입", WS_TABSTOP, fieldIDInsert)
	s.cancelBtn = fieldCreateChild(s, "BUTTON", "취소", WS_TABSTOP, fieldIDCancel)
	s.status = fieldCreateChild(s, "STATIC", "현재 표시 모드에 맞는 물체만 팔레트에 나타납니다. 특수 배치는 점전하를 정다면체 꼭짓점에 실제 3차원으로 놓습니다.", ssCenterImage, 0)
	fieldSyncControls(s)
	fieldRefreshPreview(s, false)
}

func fieldEditorLayout(s *fieldEditorState, client RECT) {
	w, h := int(client.Right-client.Left), int(client.Bottom-client.Top)
	leftW, margin := 300, 10
	labelW, valueW, rowH := 92, 54, 24
	x, y := margin, margin
	comboW := leftW - margin*2
	move := func(hwnd uintptr, x, y, w, h int) {
		multidimMoveWindow(hwnd, int32(x), int32(y), int32(w), int32(h), true)
	}

	move(s.dimensionCombo, x, y, comboW, 150)
	y += 31
	move(s.displayCombo, x, y, comboW, 150)
	y += 31
	move(s.specialCombo, x, y, comboW-104, 150)
	move(s.specialBtn, x+comboW-100, y, 100, 25)
	y += 31
	move(s.objectCombo, x, y, comboW, 180)
	y += 31
	move(s.materialCombo, x, y, comboW, 150)
	y += 34

	layoutSlider := func(label, slider, value uintptr) {
		move(label, x, y, labelW, rowH)
		move(slider, x+labelW, y, comboW-labelW-valueW, rowH)
		move(value, x+comboW-valueW, y, valueW, rowH)
		y += 33
	}
	layoutSlider(s.chargeLabel, s.charge, s.chargeValue)
	layoutSlider(s.sizeLabel, s.sizeSlider, s.sizeValue)
	layoutSlider(s.spinLabel, s.spinSlider, s.spinValue)
	layoutSlider(s.objectRotXLabel, s.objectRotX, s.objectRotXValue)
	layoutSlider(s.objectRotYLabel, s.objectRotY, s.objectRotYValue)
	layoutSlider(s.objectRotZLabel, s.objectRotZ, s.objectRotZValue)
	layoutSlider(s.densityLabel, s.density, s.densityValue)
	layoutSlider(s.viewRotXLabel, s.viewRotX, s.viewRotXValue)
	layoutSlider(s.viewRotYLabel, s.viewRotY, s.viewRotYValue)
	layoutSlider(s.viewRotZLabel, s.viewRotZ, s.viewRotZValue)
	layoutSlider(s.viewDistanceLabel, s.viewDistance, s.viewDistanceValue)

	move(s.antialiasCheck, x, y, 140, 25)
	move(s.blackCheck, x+145, y, 125, 25)
	y += 34
	move(s.deleteBtn, x, y, 130, 27)
	move(s.clearBtn, x+140, y, 130, 27)
	y += 34
	move(s.insertBtn, x+70, y, 63, 27)
	move(s.cancelBtn, x+137, y, 63, 27)

	previewLeft := leftW + 10
	statusH := 28
	s.previewRect = RECT{int32(previewLeft), 10, int32(max(previewLeft+20, w-10)), int32(max(30, h-statusH-12))}
	s.paletteRect = RECT{s.previewRect.Left + 1, s.previewRect.Top + 1, s.previewRect.Left + fieldPaletteWidth, s.previewRect.Bottom - 1}
	s.sceneRect = RECT{s.paletteRect.Right + 1, s.previewRect.Top + 1, s.previewRect.Right - 1, s.previewRect.Bottom - 1}
	move(s.status, previewLeft, h-statusH, max(20, w-previewLeft-10), statusH-4)
	invalidate(s.hwnd)
}

func fieldCloneObjects(objects []fieldObject) []fieldObject {
	return append([]fieldObject(nil), objects...)
}

func fieldFilterObjectsForDisplay(objects []fieldObject, display fieldDisplayMode) []fieldObject {
	out := make([]fieldObject, 0, len(objects))
	for _, o := range objects {
		if display.electric() && o.electric() || display.magnetic() && o.magnetic() || display.gravity() && o.gravity() {
			out = append(out, o)
		}
	}
	return out
}

func fieldSwitchDisplayDomain(s *fieldEditorState, next fieldDisplayMode) {
	if s == nil {
		return
	}
	old := s.scene.Display
	switch old.domain() {
	case 0:
		s.electricObjects = fieldCloneObjects(fieldFilterObjectsForDisplay(s.scene.Objects, old))
	case 1:
		s.magneticObjects = fieldCloneObjects(fieldFilterObjectsForDisplay(s.scene.Objects, old))
	case 2:
		s.gravityObjects = fieldCloneObjects(fieldFilterObjectsForDisplay(s.scene.Objects, old))
	}
	s.scene.Display = next
	if old.domain() != next.domain() {
		switch next.domain() {
		case 0:
			s.scene.Objects = fieldCloneObjects(s.electricObjects)
		case 1:
			s.scene.Objects = fieldCloneObjects(s.magneticObjects)
		case 2:
			s.scene.Objects = fieldCloneObjects(s.gravityObjects)
		}
	} else {
		s.scene.Objects = fieldFilterObjectsForDisplay(s.scene.Objects, next)
	}
	s.selected = -1
	fieldSyncControls(s)
	invalidate(s.hwnd)
}

func fieldApplySpecialPointLayout(s *fieldEditorState) {
	if s == nil || !s.scene.Display.electric() {
		return
	}
	layout := int(sendMessage(s.specialCombo, CB_GETCURSEL, 0, 0))
	if layout <= fieldSpecialLayoutNone {
		fieldSetText(s.status, "정사면체·정팔면체·정육면체 중 하나를 선택하십시오.")
		return
	}
	objects := fieldPointChargePolyhedron(layout)
	if len(objects) == 0 {
		return
	}
	s.scene.Dimension = fieldDimension3D
	sendMessage(s.dimensionCombo, CB_SETCURSEL, uintptr(fieldDimension3D), 0)
	s.scene.Objects = objects
	s.selected = -1
	fieldSyncControls(s)
	fieldSetText(s.status, fmt.Sprintf("점전하 %d개를 %s 꼭짓점에 3차원 배치했습니다. 각 전하를 선택해 부호와 크기를 바꿀 수 있습니다.", len(objects), fieldSpecialLayoutName(layout)))
	logf("FIELD special-layout type=%s vertices=%d charge=+1 radius=0.27", fieldSpecialLayoutName(layout), len(objects))
	fieldRefreshPreview(s, false)
}

func fieldRefreshObjectCombo(s *fieldEditorState) {
	if s == nil || s.objectCombo == 0 {
		return
	}
	wasUpdating := s.updating
	s.updating = true
	defer func() { s.updating = wasUpdating }()
	sendMessage(s.objectCombo, multidimCBResetContent, 0, 0)
	fieldAddComboText(s.objectCombo, "선택 물체 없음")
	for i, o := range s.scene.Objects {
		name := fieldKindName(o.Kind)
		if o.electric() {
			name = fmt.Sprintf("%d. %s / %s / q=%+.1f", i+1, name, fieldMaterialName(o.Material), o.Charge)
		} else if o.Kind == fieldObjectFerromagnet {
			name = fmt.Sprintf("%d. %s / 자성률=%.1f", i+1, name, o.Charge)
		} else if o.gravity() {
			name = fmt.Sprintf("%d. %s / 질량=%.1f", i+1, name, o.Charge)
			if strength := fieldGravityBlackHoleStrength(o); strength > 0 {
				name += fmt.Sprintf(" / 블랙홀 %.0f%%", strength*100)
			}
		} else {
			name = fmt.Sprintf("%d. %s / 세기=%+.1f", i+1, name, o.Charge)
		}
		fieldAddComboText(s.objectCombo, name)
	}
	pos := 0
	if s.selected >= 0 && s.selected < len(s.scene.Objects) {
		pos = s.selected + 1
	}
	sendMessage(s.objectCombo, CB_SETCURSEL, uintptr(pos), 0)
}

func fieldPreviewDensityCap(display fieldDisplayMode) int {
	switch display {
	case fieldDisplayElectricVectors, fieldDisplayMagneticVectors, fieldDisplayGravityVectors:
		return 24
	default:
		return 14
	}
}

func fieldSelectedObject(s *fieldEditorState) *fieldObject {
	if s == nil || s.selected < 0 || s.selected >= len(s.scene.Objects) {
		return nil
	}
	return &s.scene.Objects[s.selected]
}

func fieldSyncControls(s *fieldEditorState) {
	if s == nil {
		return
	}
	wasUpdating := s.updating
	s.updating = true
	defer func() { s.updating = wasUpdating }()
	fieldClampScene(&s.scene)
	switch s.scene.Display {
	case fieldDisplayEquipotential:
		fieldSetText(s.densityLabel, "등전위 밀도")
	case fieldDisplayElectricLines:
		fieldSetText(s.densityLabel, "전하 1당 선 수")
	case fieldDisplayElectricVectors, fieldDisplayMagneticVectors, fieldDisplayGravityVectors:
		fieldSetText(s.densityLabel, "격자 밀도 (최대 2×)")
	case fieldDisplayMagneticLines:
		fieldSetText(s.densityLabel, "자기력선 밀도")
	case fieldDisplayGravityPotential:
		fieldSetText(s.densityLabel, "중력 퍼텐셜 밀도")
	case fieldDisplayGravityLines:
		fieldSetText(s.densityLabel, "중력장선 밀도")
	}
	if s.scene.Display.vectorArrows() {
		multidimSliderSet(s.density, fieldVectorDensityMinTenths, fieldVectorDensityMaxTenths, s.scene.VectorDensity)
		sendMessage(s.density, TBM_SETTICFREQ, 1, 0)
	} else {
		multidimSliderSet(s.density, 4, 24, s.scene.Density)
		sendMessage(s.density, TBM_SETTICFREQ, 2, 0)
	}
	fieldRefreshObjectCombo(s)
	o := fieldSelectedObject(s)
	enable := uintptr(0)
	if o != nil {
		enable = 1
	}
	for _, h := range []uintptr{s.charge, s.sizeSlider, s.objectRotX, s.objectRotY, s.objectRotZ, s.deleteBtn} {
		procEnableWindow.Call(h, enable)
	}
	procEnableWindow.Call(s.materialCombo, enable)
	spinEnable := uintptr(0)
	if o != nil && (o.Kind == fieldObjectSolidMassSphere || o.Kind == fieldObjectHollowMassSphere) {
		spinEnable = 1
	}
	procEnableWindow.Call(s.spinSlider, spinEnable)
	electricMode := s.scene.Display.electric()
	procEnableWindow.Call(s.specialCombo, func() uintptr {
		if electricMode {
			return 1
		}
		return 0
	}())
	procEnableWindow.Call(s.specialBtn, func() uintptr {
		if electricMode {
			return 1
		}
		return 0
	}())
	if o == nil {
		fieldSetText(s.chargeLabel, "전하량")
		fieldSetText(s.chargeValue, "-")
		fieldSetText(s.sizeLabel, "크기")
		fieldSetText(s.sizeValue, "-")
		fieldSetText(s.spinValue, "-")
		return
	}
	if o.electric() {
		fieldSetText(s.chargeLabel, "전하량")
		procEnableWindow.Call(s.materialCombo, 1)
		sendMessage(s.materialCombo, CB_SETCURSEL, uintptr(o.Material), 0)
	} else if o.Kind == fieldObjectFerromagnet {
		fieldSetText(s.chargeLabel, "자성률")
		procEnableWindow.Call(s.materialCombo, 0)
	} else if o.gravity() {
		fieldSetText(s.chargeLabel, "질량")
		procEnableWindow.Call(s.materialCombo, 0)
	} else {
		fieldSetText(s.chargeLabel, "세기")
		procEnableWindow.Call(s.materialCombo, 0)
	}
	if o.Kind == fieldObjectFerromagnet {
		multidimSliderSet(s.charge, 0, 40, int(math.Round(o.Charge*10)))
		fieldSetText(s.chargeValue, fmt.Sprintf("%.1f", o.Charge))
	} else if o.gravity() {
		multidimSliderSet(s.charge, 0, 100, int(math.Round(o.Charge*10)))
		fieldSetText(s.chargeValue, fmt.Sprintf("%.1f", o.Charge))
	} else {
		multidimSliderSet(s.charge, -40, 40, int(math.Round(o.Charge*10)))
		fieldSetText(s.chargeValue, fmt.Sprintf("%+.1f", o.Charge))
	}
	minV, maxV, pos, tickFreq := 18, 275, 48, 10
	switch o.Kind {
	case fieldObjectSphere:
		fieldSetText(s.sizeLabel, "반지름")
		minV, maxV, pos, tickFreq = 18, 115, int(math.Round(o.Radius*100)), 10
	case fieldObjectFerromagnet:
		fieldSetText(s.sizeLabel, "반지름")
		minV, maxV, pos, tickFreq = 25, 125, int(math.Round(o.Radius*100)), 10
	case fieldObjectSolidMassSphere, fieldObjectHollowMassSphere:
		fieldSetText(s.sizeLabel, "반지름")
		minV, maxV, pos, tickFreq = 0, 100, fieldGravityRadiusSliderPosition(o.Radius), 10
	case fieldObjectMassPlane:
		fieldSetText(s.sizeLabel, "면 크기")
		minV, maxV, pos, tickFreq = 35, 400, int(math.Round(o.Size*100)), 25
	case fieldObjectMassRing:
		fieldSetText(s.sizeLabel, "링 반지름")
		minV, maxV, pos, tickFreq = 18, 155, int(math.Round(o.Radius*100)), 10
	case fieldObjectRod, fieldObjectBarMagnet, fieldObjectHorseshoeMagnet:
		fieldSetText(s.sizeLabel, "길이")
		minV, maxV, pos, tickFreq = 35, 275, int(math.Round(o.Length*100)), 25
		if o.Kind == fieldObjectRod {
			minV, maxV = 0, fieldExtendedSizeSliderMax
			pos = fieldExtendedSizeSliderPosition(o.Kind, o.Length)
			tickFreq = 100
		}
	case fieldObjectPlane:
		fieldSetText(s.sizeLabel, "면 크기")
		minV, maxV = 0, fieldExtendedSizeSliderMax
		pos = fieldExtendedSizeSliderPosition(o.Kind, o.Size)
		tickFreq = 100
	case fieldObjectCurrentWire:
		fieldSetText(s.sizeLabel, "길이")
		minV, maxV, pos, tickFreq = 35, 275, int(math.Round(o.Length*100)), 25
	case fieldObjectCurrentLoop:
		fieldSetText(s.sizeLabel, "고리 반지름")
		minV, maxV, pos, tickFreq = 25, 145, int(math.Round(o.Radius*100)), 10
	}
	multidimSliderSet(s.sizeSlider, minV, maxV, pos)
	sendMessage(s.sizeSlider, TBM_SETTICFREQ, uintptr(tickFreq), 0)
	sizeValue := o.Radius
	if o.Kind == fieldObjectRod || o.Kind == fieldObjectBarMagnet || o.Kind == fieldObjectCurrentWire || o.Kind == fieldObjectHorseshoeMagnet {
		sizeValue = o.Length
	} else if o.Kind == fieldObjectCurrentLoop || o.Kind == fieldObjectMassRing {
		sizeValue = o.Radius
	} else if o.Kind == fieldObjectPlane || o.Kind == fieldObjectMassPlane {
		sizeValue = o.Size
	}
	fieldSetText(s.sizeValue, fmt.Sprintf("%.2f", sizeValue))
	if o.Kind == fieldObjectSolidMassSphere || o.Kind == fieldObjectHollowMassSphere {
		multidimSliderSet(s.spinSlider, 0, 99, int(math.Round(o.Spin*100)))
		if o.Kind == fieldObjectSolidMassSphere || o.Kind == fieldObjectHollowMassSphere {
			fieldSetText(s.spinValue, fmt.Sprintf("%.2f", o.Spin))
		} else {
			fieldSetText(s.spinValue, "-")
		}
	} else {
		multidimSliderSet(s.spinSlider, 0, 99, 0)
		fieldSetText(s.spinValue, "-")
	}
	angles := []struct {
		h     uintptr
		v     float64
		label uintptr
	}{{s.objectRotX, o.RotX, s.objectRotXValue}, {s.objectRotY, o.RotY, s.objectRotYValue}, {s.objectRotZ, o.RotZ, s.objectRotZValue}}
	for _, a := range angles {
		position := fieldRotationSliderPosition(a.v)
		fieldSliderSetPosSigned(a.h, position)
		fieldSetText(a.label, fmt.Sprintf("%d°", fieldRotationDegreesFromSlider(position)))
	}
	// Manually placed objects use the XZ plane.  Special point-charge layouts
	// retain true Y coordinates, while every object may rotate independently.
	procEnableWindow.Call(s.objectRotX, enable)
	procEnableWindow.Call(s.objectRotY, enable)
	procEnableWindow.Call(s.objectRotZ, enable)
}

func fieldUpdateDimensionControls(s *fieldEditorState) {
	// The observer may orbit freely. Special layouts can contain true 3-D Y coordinates.
	for _, h := range []uintptr{s.viewRotX, s.viewRotY, s.viewRotZ} {
		procEnableWindow.Call(h, 1)
	}
	fieldSyncControls(s)
}

func fieldApplySliders(s *fieldEditorState, interactive bool) {
	if s == nil || s.updating {
		return
	}
	o := fieldSelectedObject(s)
	if o != nil {
		o.Charge = float64(multidimSliderPos(s.charge)) / 10
		if o.Kind == fieldObjectFerromagnet && o.Charge < 0 {
			o.Charge = 0
			fieldSliderSetPosSigned(s.charge, 0)
		}
		sliderPos := multidimSliderPos(s.sizeSlider)
		pos := float64(sliderPos) / 100
		switch o.Kind {
		case fieldObjectSphere, fieldObjectFerromagnet:
			o.Radius = pos
		case fieldObjectSolidMassSphere, fieldObjectHollowMassSphere:
			o.Radius = fieldGravityRadiusFromSlider(sliderPos)
		case fieldObjectMassPlane:
			o.Size = pos
		case fieldObjectMassRing:
			o.Radius = pos
		case fieldObjectRod:
			o.Length = fieldExtendedSizeFromSlider(o.Kind, sliderPos)
		case fieldObjectBarMagnet, fieldObjectCurrentWire, fieldObjectHorseshoeMagnet:
			o.Length = pos
		case fieldObjectCurrentLoop:
			o.Radius = pos
		case fieldObjectPlane:
			o.Size = fieldExtendedSizeFromSlider(o.Kind, sliderPos)
		}
		o.RotX = fieldRotationRadiansFromSlider(multidimSliderPos(s.objectRotX))
		o.RotY = fieldRotationRadiansFromSlider(multidimSliderPos(s.objectRotY))
		o.RotZ = fieldRotationRadiansFromSlider(multidimSliderPos(s.objectRotZ))
		if o.Kind == fieldObjectSolidMassSphere || o.Kind == fieldObjectHollowMassSphere {
			o.Spin = float64(multidimSliderPos(s.spinSlider)) / 100.0
		}
	}
	if s.scene.Display.vectorArrows() {
		s.scene.VectorDensity = multidimSliderPos(s.density)
	} else {
		s.scene.Density = multidimSliderPos(s.density)
	}
	s.scene.RotX = fieldRotationRadiansFromSlider(multidimSliderPos(s.viewRotX))
	s.scene.RotY = fieldRotationRadiansFromSlider(multidimSliderPos(s.viewRotY))
	s.scene.RotZ = fieldRotationRadiansFromSlider(multidimSliderPos(s.viewRotZ))
	s.scene.ViewDistance = float64(multidimSliderPos(s.viewDistance)) / 10.0
	fieldClampScene(&s.scene)
	fieldSyncValueLabels(s)
	fieldRefreshObjectCombo(s)
	fieldSchedulePreview(s, interactive || s.dragging)
}

func fieldSyncValueLabels(s *fieldEditorState) {
	o := fieldSelectedObject(s)
	if o != nil {
		if o.gravity() || o.Kind == fieldObjectFerromagnet {
			fieldSetText(s.chargeValue, fmt.Sprintf("%.1f", o.Charge))
		} else {
			fieldSetText(s.chargeValue, fmt.Sprintf("%+.1f", o.Charge))
		}
		v := o.Radius
		if o.Kind == fieldObjectRod || o.Kind == fieldObjectBarMagnet || o.Kind == fieldObjectCurrentWire || o.Kind == fieldObjectHorseshoeMagnet {
			v = o.Length
		} else if o.Kind == fieldObjectCurrentLoop || o.Kind == fieldObjectMassRing {
			v = o.Radius
		}
		if o.Kind == fieldObjectPlane || o.Kind == fieldObjectMassPlane {
			v = o.Size
		}
		fieldSetText(s.sizeValue, fmt.Sprintf("%.2f", v))
		fieldSetText(s.spinValue, fmt.Sprintf("%.2f", o.Spin))
		fieldSetText(s.objectRotXValue, fmt.Sprintf("%d°", fieldRotationDegreesFromSlider(multidimSliderPos(s.objectRotX))))
		fieldSetText(s.objectRotYValue, fmt.Sprintf("%d°", fieldRotationDegreesFromSlider(multidimSliderPos(s.objectRotY))))
		fieldSetText(s.objectRotZValue, fmt.Sprintf("%d°", fieldRotationDegreesFromSlider(multidimSliderPos(s.objectRotZ))))
	}
	if s.scene.Display.vectorArrows() {
		fieldSetText(s.densityValue, fmt.Sprintf("%.1f×", float64(s.scene.VectorDensity)/10))
	} else {
		fieldSetText(s.densityValue, fmt.Sprintf("%d", s.scene.Density))
	}
	fieldSetText(s.viewRotXValue, fmt.Sprintf("%d°", fieldRotationDegreesFromSlider(multidimSliderPos(s.viewRotX))))
	fieldSetText(s.viewRotYValue, fmt.Sprintf("%d°", fieldRotationDegreesFromSlider(multidimSliderPos(s.viewRotY))))
	fieldSetText(s.viewRotZValue, fmt.Sprintf("%d°", fieldRotationDegreesFromSlider(multidimSliderPos(s.viewRotZ))))
	fieldSetText(s.viewDistanceValue, fmt.Sprintf("%.1f", fieldObserverDistance(s.scene.ViewDistance)))
}

func fieldSchedulePreview(s *fieldEditorState, interactive bool) {
	if s == nil || s.hwnd == 0 {
		return
	}
	if interactive {
		// Render a coalesced 1x physical preview while a thumb or object moves.
		// Invalid/sparse intermediate results still preserve the last completed
		// frame, so the old blink-prevention invariant remains intact.
		s.previewDirty = true
		s.previewMode = 1
		if s.previewQueued {
			procKillTimer.Call(s.hwnd, fieldPreviewTimerID)
			s.previewQueued = false
		}
		fieldQueuePreview(s, "interactive", true)
		invalidateArea(s.hwnd, &s.sceneRect)
		return
	}
	fieldCommitFinalPreview(s, "scheduled-final")
}

func fieldPreviewReasonMustCommit(reason string) bool {
	switch reason {
	case "object-drag-end", "capture-changed", "cancel-mode":
		return true
	}
	return false
}

// fieldInstallReleasePreview makes mouse-up visually atomic.  A heavy 3-D
// render still runs asynchronously, but the scene can no longer fall back to
// the empty editor raster while that worker is pending.  The provisional frame
// contains the actual current objects (not merely the orange selection ring)
// and is replaced only by the matching final serial.
func fieldInstallReleasePreview(s *fieldEditorState, reason string) bool {
	if s == nil || s.hwnd == 0 || !fieldPreviewReasonMustCommit(reason) || len(s.scene.Objects) == 0 {
		return false
	}
	w, h := int(s.sceneRect.Right-s.sceneRect.Left), int(s.sceneRect.Bottom-s.sceneRect.Top)
	if w < 20 || h < 20 {
		return false
	}
	rw, rh := min(w, 560), min(h, 430)
	// Preserve the last completed field image whenever possible, then overlay
	// the objects at their current release positions.  On the first placement
	// there is no completed field yet, so start with a clean raster.
	r := fieldNewRaster(rw, rh)
	base := s.stablePreview
	if base.W != rw || base.H != rh || len(base.Pix) != rw*rh*4 {
		base = s.preview
	}
	if base.W == rw && base.H == rh && len(base.Pix) == rw*rh*4 {
		r = base
		r.Pix = append([]byte(nil), base.Pix...)
		r.Mask = append([]bool(nil), base.Mask...)
	}
	if s.scene.Dimension == fieldDimension3D && s.scene.Display.scalarPotential() {
		r.EquipotentialExpected = true
	}
	if !fieldOverlayPreviewGeometry(&r, s.scene) || fieldRasterVisiblePixelCount(r) == 0 {
		return false
	}
	s.preview = r
	s.previewRevision++
	s.previewDirty = true
	fieldSetText(s.status, "마우스를 놓았습니다. 최종 미리보기를 계산하는 중입니다.")
	invalidateArea(s.hwnd, &s.sceneRect)
	logf("FIELD_PREVIEW release-latch reason=%s visible=%d size=%dx%d", reason, fieldRasterVisiblePixelCount(r), rw, rh)
	return true
}

func fieldPreviewRasterScore(r fieldRaster) int64 {
	visible := int64(fieldRasterVisiblePixelCount(r))
	return int64(r.EquipotentialRenderedLevels)*1_000_000_000 + int64(r.EquipotentialVacuumTriangles)*1_000 + visible
}

func fieldCommitFinalPreview(s *fieldEditorState, reason string) bool {
	if s == nil || s.hwnd == 0 {
		return false
	}
	procKillTimer.Call(s.hwnd, fieldPreviewTimerID)
	s.previewQueued = false
	s.previewMode = 0
	fieldInstallReleasePreview(s, reason)
	ok := fieldQueueFinalPreview(s, reason)
	logf("FIELD_PREVIEW commit reason=%s queued=%v", reason, ok)
	return ok
}

// fieldRefreshPreview remains the common call site used by combo boxes, window
// resizing and object deletion, but final 3-D rendering is now queued instead
// of executed inside the Win32 message callback.  This keeps WM_PAINT and input
// responsive while the scalar grid, marching cubes and CPU membrane compositor
// run on a worker goroutine.
// fieldResetPreviewForEmptyScene treats an empty field scene as a completed
// preview, not as a failed render.  Before 1.1, a zero-visible-pixel result was
// followed by the last stable frame, so deleting the final gravity object left
// the old image on screen and made the editor appear frozen.
func fieldResetPreviewForEmptyScene(s *fieldEditorState, reason string) bool {
	if s == nil || s.hwnd == 0 || len(s.scene.Objects) != 0 {
		return false
	}
	w, h := s.preview.W, s.preview.H
	if w < 1 || h < 1 {
		w = min(max(int(s.sceneRect.Right-s.sceneRect.Left), 1), 560)
		h = min(max(int(s.sceneRect.Bottom-s.sceneRect.Top), 1), 430)
	}

	// Invalidate every in-flight result without falsely marking the worker idle.
	// If a new object is added before the old worker exits, normal coalescing will
	// queue the new scene and launch it as soon as that worker posts completion.
	s.asyncMu.Lock()
	if s.previewCancel != nil {
		s.previewCancel.Cancel()
		s.previewCancel = nil
	}
	s.previewSerial++
	s.previewPending = false
	s.previewPendingReason = ""
	s.previewPendingFast = false
	s.previewResult = nil
	serial := s.previewSerial
	s.asyncMu.Unlock()

	s.preview = fieldNewRaster(w, h)
	s.stablePreview = fieldRaster{}
	s.previewRevision++
	s.previewDirty = false
	s.previewMode = 0
	if s.previewQueued {
		procKillTimer.Call(s.hwnd, fieldPreviewTimerID)
		s.previewQueued = false
	}
	fieldSetText(s.status, "배치한 물체가 없습니다.")
	invalidateArea(s.hwnd, &s.sceneRect)
	logf("FIELD_PREVIEW empty-scene-reset serial=%d reason=%s size=%dx%d", serial, reason, w, h)
	return true
}

func fieldRefreshPreview(s *fieldEditorState, interactive bool) bool {
	if fieldResetPreviewForEmptyScene(s, "refresh") {
		return true
	}
	if interactive {
		fieldSchedulePreview(s, true)
		return true
	}
	return fieldQueueFinalPreview(s, "refresh")
}

func fieldQueueFinalPreview(s *fieldEditorState, reason string) bool {
	return fieldQueuePreview(s, reason, false)
}

func fieldQueuePreview(s *fieldEditorState, reason string, interactive bool) bool {
	if s == nil || s.hwnd == 0 {
		return false
	}
	s.asyncMu.Lock()
	if s.destroyed || s.insertBusy {
		s.asyncMu.Unlock()
		invalidateArea(s.hwnd, &s.sceneRect)
		return false
	}
	s.previewSerial++
	serial := s.previewSerial
	if s.previewCancel != nil {
		s.previewCancel.Cancel()
	}
	if s.previewRenderBusy {
		s.previewPending = true
		s.previewPendingReason = reason
		s.previewPendingFast = interactive
		s.asyncMu.Unlock()
		logf("FIELD_PREVIEW coalesced serial=%d reason=%s phase=%s", serial, reason, fieldPreviewPhaseName(interactive))
		return true
	}
	s.previewRenderBusy = true
	cancel := fieldNewRenderCancellation()
	s.previewCancel = cancel
	s.asyncMu.Unlock()
	return fieldLaunchPreviewWorker(s, serial, reason, interactive, cancel)
}

func fieldPreviewPhaseName(interactive bool) string {
	if interactive {
		return "interactive-1x"
	}
	return "final"
}

func fieldLaunchPreviewWorker(s *fieldEditorState, serial uint64, reason string, interactive bool, cancel *fieldRenderCancellation) bool {
	if s == nil || s.hwnd == 0 {
		return false
	}
	w, h := int(s.sceneRect.Right-s.sceneRect.Left), int(s.sceneRect.Bottom-s.sceneRect.Top)
	if w < 20 || h < 20 {
		s.asyncMu.Lock()
		s.previewRenderBusy = false
		if s.previewCancel == cancel {
			s.previewCancel = nil
		}
		s.asyncMu.Unlock()
		return false
	}
	rw, rh := min(w, 560), min(h, 430)
	previewScene := s.scene
	previewScene.Objects = append([]fieldObject(nil), s.scene.Objects...)
	previewScene.cancel = cancel
	requestedSmooth := previewScene.Antialias
	previewScene = fieldPreviewSceneForInteraction(previewScene, interactive)
	if cap := fieldPreviewDensityCap(previewScene.Display); previewScene.Density > cap {
		previewScene.Density = cap
	}
	// Final previews use the same 6x resolve as insertion. The work stays on the
	// preview worker, so even a three-dimensional equipotential membrane can
	// replace its former jagged 1x silhouette without blocking the UI thread.
	hasLastGood := fieldEquipotentialPreviewUsable(s.preview)
	if previewScene.Dimension == fieldDimension3D && previewScene.Display.scalarPotential() {
		hasLastGood = s.preview.EquipotentialExpected && fieldEquipotentialPreviewUsable(s.preview)
	}
	mustCommit := !interactive && fieldPreviewReasonMustCommit(reason)
	black, hwnd := s.black, s.hwnd
	logf("FIELD_PREVIEW worker-begin serial=%d reason=%s phase=%s display=%d dimension=%d size=%dx%d requestedSmooth=%v actualSS=%d", serial, reason, fieldPreviewPhaseName(interactive), previewScene.Display, previewScene.Dimension, rw, rh, requestedSmooth, fieldRenderSampleScale(previewScene))
	go func(state *fieldEditorState, target uintptr, request uint64, why string, fast bool, snapshot fieldScene, backgroundBlack bool, width, height int, preserve, forceCommit bool) {
		result := fieldComputePreviewResult(request, why, fast, snapshot, backgroundBlack, width, height, preserve, forceCommit)
		state.asyncMu.Lock()
		if state.destroyed {
			state.asyncMu.Unlock()
			return
		}
		state.previewResult = result
		state.asyncMu.Unlock()
		procPostMessageW.Call(target, fieldPreviewDoneMessage, 0, 0)
	}(s, hwnd, serial, reason, interactive, previewScene, black, rw, rh, hasLastGood, mustCommit)
	return true
}

func fieldComputePreviewResult(serial uint64, reason string, interactive bool, previewScene fieldScene, black bool, rw, rh int, hasLastGood, mustCommit bool) *fieldAsyncPreviewResult {
	started := time.Now()
	cancelledResult := func() *fieldAsyncPreviewResult {
		return &fieldAsyncPreviewResult{Serial: serial, Reason: reason, Interactive: interactive, Scene: previewScene, Cancelled: true, Elapsed: time.Since(started)}
	}
	qualityUsed := 0
	if !interactive && fieldSceneNeedsPrecise3DLines(previewScene) {
		// Settled 3-D field-line previews use one higher source-discretisation
		// quality than the drag preview. The final insertion still uses quality 2.
		qualityUsed = 1
	}
	next := fieldRenderForBackground(previewScene, rw, rh, qualityUsed, black)
	if previewScene.cancelled() {
		return cancelledResult()
	}
	best := next
	bestQuality := qualityUsed
	bestScore := fieldPreviewRasterScore(best)
	isEquipotential := previewScene.Dimension == fieldDimension3D && previewScene.Display.scalarPotential()
	if isEquipotential && !fieldEquipotentialPreviewUsable(next) {
		// A release-final request is never allowed to vanish.  It runs outside
		// the UI thread, so it may safely try both higher topology qualities.  A
		// routine refresh keeps the cheaper one-retry policy when no physical
		// frame has ever existed.
		maxQuality := 0
		if mustCommit {
			maxQuality = 2
		} else if !hasLastGood {
			maxQuality = 1
		}
		for q := 1; q <= maxQuality; q++ {
			if previewScene.cancelled() {
				return cancelledResult()
			}
			logf("FIELD_PREVIEW final-retry serial=%d reason=%s quality=%d requested=%d levels=%d triangles=%d", serial, reason, q, best.EquipotentialRequestedLevels, best.EquipotentialRenderedLevels, best.EquipotentialVacuumTriangles)
			retry := fieldRenderForBackground(previewScene, rw, rh, q, black)
			if score := fieldPreviewRasterScore(retry); score > bestScore {
				best, bestQuality, bestScore = retry, q, score
			}
			if fieldEquipotentialPreviewUsable(retry) {
				best, bestQuality = retry, q
				break
			}
		}
		next, qualityUsed = best, bestQuality
	}
	if previewScene.cancelled() {
		return cancelledResult()
	}
	valid := next.W > 0 && next.H > 0 && len(next.Pix) == next.W*next.H*4
	fallbackGeometry := false
	provisional := false
	if valid {
		fallbackGeometry = fieldEnsurePreviewGeometry(&next, previewScene)
	}
	if valid && isEquipotential && !fieldEquipotentialPreviewUsable(next) {
		if mustCommit {
			// Preserve the best scalar-volume result and always overlay the actual
			// objects.  This is a visible, honest provisional physical frame, not
			// the empty background plus selection cursor that caused the blinking.
			fallbackGeometry = fieldOverlayPreviewGeometry(&next, previewScene) || fallbackGeometry
			valid = fieldRasterVisiblePixelCount(next) > 0
			provisional = valid
		} else {
			valid = false
		}
	}
	if !valid {
		next = fieldNewRaster(rw, rh)
		fallbackGeometry = fieldOverlayPreviewGeometry(&next, previewScene)
		valid = next.W > 0 && next.H > 0 && len(next.Pix) == next.W*next.H*4 && fieldRasterVisiblePixelCount(next) > 0
		provisional = valid && mustCommit
		// Ordinary failed refreshes must preserve the previous completed physical
		// frame.  Only a release-final request is permitted to install the
		// geometry latch as a last resort.
		if isEquipotential && !mustCommit {
			valid = false
		}
	}
	visible := 0
	if valid {
		visible = fieldRasterVisiblePixelCount(next)
	}
	return &fieldAsyncPreviewResult{
		Serial: serial, Reason: reason, Interactive: interactive, Raster: next, Scene: previewScene,
		Quality: qualityUsed, Visible: visible, FallbackGeometry: fallbackGeometry,
		Provisional: provisional, Valid: valid, Elapsed: time.Since(started),
	}
}

func fieldFinishAsyncPreview(s *fieldEditorState) {
	if s == nil {
		return
	}
	s.asyncMu.Lock()
	result := s.previewResult
	s.previewResult = nil
	s.previewRenderBusy = false
	latest := s.previewSerial
	pending := s.previewPending
	pendingReason := s.previewPendingReason
	pendingFast := s.previewPendingFast
	s.previewPending = false
	s.previewPendingReason = ""
	s.previewPendingFast = false
	destroyed, inserting := s.destroyed, s.insertBusy
	launchNext := pending && !destroyed && !inserting
	var nextCancel *fieldRenderCancellation
	if launchNext {
		s.previewRenderBusy = true
		nextCancel = fieldNewRenderCancellation()
		s.previewCancel = nextCancel
	} else {
		s.previewCancel = nil
	}
	s.asyncMu.Unlock()

	currentEnough := result != nil && fieldPreviewResultCurrentEnough(result.Serial, latest, result.Interactive, pending, pendingFast)
	if result != nil && !result.Cancelled && currentEnough && result.Valid && !destroyed && !inserting {
		s.preview = result.Raster
		s.previewRevision++
		if result.Visible > 0 {
			s.stablePreview = result.Raster
		}
		s.previewDirty = false
		if result.Interactive {
			fieldSetText(s.status, "조작 중 1배 빠른 미리보기입니다. 놓으면 6배로 갱신합니다.")
		} else if result.Provisional {
			fieldSetText(s.status, "정밀 등전위막이 불완전하여 계산된 부피와 물체 미리보기를 유지합니다.")
		} else {
			fieldSetText(s.status, "최종 미리보기를 갱신했습니다.")
		}
		logf("FIELD_PREVIEW ready serial=%d latest=%d reason=%s phase=%s display=%d density=%d vector_density=%.1fx quality=%d size=%dx%d visible=%d levels=%d/%d triangles=%d conductor=%d smooth=%v actualSS=%d geometry_fallback=%v provisional=%v elapsed=%s", result.Serial, latest, result.Reason, fieldPreviewPhaseName(result.Interactive), result.Scene.Display, result.Scene.Density, fieldVectorArrowDensityScale(result.Scene), result.Quality, result.Raster.W, result.Raster.H, result.Visible, result.Raster.EquipotentialRenderedLevels, result.Raster.EquipotentialRequestedLevels, result.Raster.EquipotentialVacuumTriangles, result.Raster.EquipotentialConductorTriangles, result.Scene.Antialias, fieldRenderSampleScale(result.Scene), result.FallbackGeometry, result.Provisional, result.Elapsed)
		invalidateArea(s.hwnd, &s.sceneRect)
	} else if result != nil {
		logf("FIELD_PREVIEW discarded serial=%d latest=%d reason=%s valid=%v cancelled=%v elapsed=%s", result.Serial, latest, result.Reason, result.Valid, result.Cancelled, result.Elapsed)
	}
	if launchNext {
		fieldLaunchPreviewWorker(s, latest, pendingReason, pendingFast, nextCancel)
	}
}

func fieldEquipotentialPreviewUsable(r fieldRaster) bool {
	if !r.EquipotentialExpected {
		return r.W > 0 && r.H > 0 && len(r.Pix) == r.W*r.H*4
	}
	minimumLevels := min(3, r.EquipotentialRequestedLevels)
	return r.EquipotentialVacuumTriangles > 0 && r.EquipotentialRenderedLevels >= minimumLevels
}

func fieldCompositeRaster(r fieldRaster, black bool) []byte {
	return fieldCompositeRasterInto(r, black, nil)
}

func fieldCompositeRasterInto(r fieldRaster, black bool, out []byte) []byte {
	if r.W < 1 || r.H < 1 || len(r.Pix) != r.W*r.H*4 {
		return nil
	}
	bg := byte(255)
	if black {
		bg = 18
	}
	if cap(out) < len(r.Pix) {
		out = make([]byte, len(r.Pix))
	} else {
		out = out[:len(r.Pix)]
	}
	for i := 0; i < r.W*r.H; i++ {
		o := i * 4
		a := int(r.Pix[o+3])
		out[o+0] = byte((int(r.Pix[o+0])*a + int(bg)*(255-a)) / 255)
		out[o+1] = byte((int(r.Pix[o+1])*a + int(bg)*(255-a)) / 255)
		out[o+2] = byte((int(r.Pix[o+2])*a + int(bg)*(255-a)) / 255)
		out[o+3] = 255
	}
	return out
}

func fieldCompositePreviewCached(s *fieldEditorState, r fieldRaster) []byte {
	if s == nil {
		return fieldCompositeRaster(r, false)
	}
	if s.compositeRevision != s.previewRevision || s.compositeBlack != s.black || len(s.compositePix) != len(r.Pix) {
		s.compositePix = fieldCompositeRasterInto(r, s.black, s.compositePix)
		s.compositeRevision = s.previewRevision
		s.compositeBlack = s.black
	}
	return s.compositePix
}

func fieldPaletteTileRects(s *fieldEditorState) []RECT {
	kinds := fieldPaletteKindsForScene(s.scene)
	out := make([]RECT, len(kinds))
	left, top := int(s.paletteRect.Left)+7, int(s.paletteRect.Top)+28
	w, h, gap := 62, 68, 6
	for i := range out {
		col, row := i%2, i/2
		x := left + col*(w+gap)
		y := top + row*(h+gap)
		out[i] = RECT{int32(x), int32(y), int32(x + w), int32(y + h)}
	}
	return out
}

func fieldPointInRect(x, y int, r RECT) bool {
	return x >= int(r.Left) && x < int(r.Right) && y >= int(r.Top) && y < int(r.Bottom)
}

func fieldPaletteHit(s *fieldEditorState, x, y int) int {
	for i, r := range fieldPaletteTileRects(s) {
		if fieldPointInRect(x, y, r) {
			return i
		}
	}
	return -1
}

func fieldDrawPaletteIcon(hdc uintptr, rc RECT, kind fieldObjectKind, dark bool) {
	cx, cy := (int(rc.Left)+int(rc.Right))/2, int(rc.Top)+25
	fg := rgb(45, 45, 45)
	if dark {
		fg = rgb(235, 235, 235)
	}
	pen, _, _ := procCreatePen.Call(PS_SOLID, 2, uintptr(fg))
	old, _, _ := procSelectObject.Call(hdc, pen)
	switch kind {
	case fieldObjectSphere, fieldObjectSolidMassSphere:
		procEllipse.Call(hdc, uintptr(cx-14), uintptr(cy-14), uintptr(cx+15), uintptr(cy+15))
	case fieldObjectHollowMassSphere:
		procEllipse.Call(hdc, uintptr(cx-14), uintptr(cy-14), uintptr(cx+15), uintptr(cy+15))
		procEllipse.Call(hdc, uintptr(cx-9), uintptr(cy-9), uintptr(cx+10), uintptr(cy+10))
	case fieldObjectRod, fieldObjectBarMagnet:
		procMoveToEx.Call(hdc, uintptr(cx-22), uintptr(cy), 0)
		procLineTo.Call(hdc, uintptr(cx+22), uintptr(cy))
		procEllipse.Call(hdc, uintptr(cx-24), uintptr(cy-4), uintptr(cx-17), uintptr(cy+4))
		procEllipse.Call(hdc, uintptr(cx+17), uintptr(cy-4), uintptr(cx+24), uintptr(cy+4))
	case fieldObjectPlane, fieldObjectMassPlane:
		pts := []POINT{{int32(cx - 19), int32(cy - 11)}, {int32(cx + 16), int32(cy - 15)}, {int32(cx + 20), int32(cy + 10)}, {int32(cx - 15), int32(cy + 14)}}
		procPolygon.Call(hdc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	case fieldObjectCurrentWire:
		procMoveToEx.Call(hdc, uintptr(cx), uintptr(cy-17), 0)
		procLineTo.Call(hdc, uintptr(cx), uintptr(cy+17))
	case fieldObjectHorseshoeMagnet:
		procMoveToEx.Call(hdc, uintptr(cx-15), uintptr(cy-15), 0)
		for _, p := range [][2]int{{cx - 15, cy + 4}, {cx - 12, cy + 11}, {cx - 7, cy + 15}, {cx, cy + 17}, {cx + 7, cy + 15}, {cx + 12, cy + 11}, {cx + 15, cy + 4}} {
			procLineTo.Call(hdc, uintptr(p[0]), uintptr(p[1]))
		}
		procLineTo.Call(hdc, uintptr(cx+15), uintptr(cy-15))
	case fieldObjectCurrentLoop, fieldObjectMassRing:
		procEllipse.Call(hdc, uintptr(cx-18), uintptr(cy-15), uintptr(cx+19), uintptr(cy+16))
	case fieldObjectFerromagnet:
		procEllipse.Call(hdc, uintptr(cx-15), uintptr(cy-15), uintptr(cx+16), uintptr(cy+16))
		for off := -10; off <= 10; off += 7 {
			procMoveToEx.Call(hdc, uintptr(cx+off-7), uintptr(cy+12), 0)
			procLineTo.Call(hdc, uintptr(cx+off+7), uintptr(cy-12))
		}
	}
	procSelectObject.Call(hdc, old)
	procDeleteObject.Call(pen)
}

func fieldDestroyPaintBuffer(s *fieldEditorState) {
	if s == nil {
		return
	}
	if s.paintDC != 0 && s.paintOld != 0 {
		procSelectObject.Call(s.paintDC, s.paintOld)
	}
	if s.paintBitmap != 0 {
		procDeleteObject.Call(s.paintBitmap)
	}
	if s.paintDC != 0 {
		procDeleteDC.Call(s.paintDC)
	}
	s.paintDC, s.paintBitmap, s.paintOld = 0, 0, 0
	s.paintW, s.paintH = 0, 0
}

func fieldEnsurePaintBuffer(s *fieldEditorState, referenceDC uintptr, w, h int) bool {
	if s == nil || referenceDC == 0 || w < 1 || h < 1 {
		return false
	}
	if s.paintDC != 0 && s.paintBitmap != 0 && s.paintW == w && s.paintH == h {
		return true
	}
	fieldDestroyPaintBuffer(s)
	dc, _, _ := procCreateCompatibleDC.Call(referenceDC)
	if dc == 0 {
		return false
	}
	bmp, _, _ := procCreateCompatibleBitmap.Call(referenceDC, uintptr(w), uintptr(h))
	if bmp == 0 {
		procDeleteDC.Call(dc)
		return false
	}
	old, _, _ := procSelectObject.Call(dc, bmp)
	s.paintDC, s.paintBitmap, s.paintOld = dc, bmp, old
	s.paintW, s.paintH = w, h
	return true
}

func fieldPaintEditor(s *fieldEditorState, hdc uintptr) {
	var client RECT
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	fillUIRect(hdc, client, rgb(240, 240, 240))
	frameUIRect(hdc, s.previewRect, rgb(130, 130, 130))
	fillUIRect(hdc, s.paletteRect, rgb(228, 228, 228))
	frameUIRect(hdc, s.paletteRect, rgb(165, 165, 165))
	paletteTitle := "전기 물체 팔레트"
	if s.scene.Display.magnetic() {
		paletteTitle = "자기 물체 팔레트"
	} else if s.scene.Display.gravity() {
		paletteTitle = "중력 물체 팔레트"
	}
	drawUIText(hdc, paletteTitle, RECT{s.paletteRect.Left + 6, s.paletteRect.Top + 3, s.paletteRect.Right - 5, s.paletteRect.Top + 25}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(35, 35, 35))
	kinds := fieldPaletteKindsForScene(s.scene)
	for i, rc := range fieldPaletteTileRects(s) {
		fillUIRect(hdc, rc, rgb(247, 247, 247))
		frameUIRect(hdc, rc, rgb(155, 155, 155))
		fieldDrawPaletteIcon(hdc, rc, kinds[i], darkModeEnabled())
		drawUIText(hdc, fieldKindName(kinds[i]), RECT{rc.Left + 2, rc.Bottom - 22, rc.Right - 2, rc.Bottom - 2}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(30, 30, 30))
	}
	bg := rgb(255, 255, 255)
	if s.black {
		bg = rgb(18, 18, 18)
	}
	fillRect(hdc, s.sceneRect, bg)
	frameUIRect(hdc, s.sceneRect, rgb(110, 110, 110))
	paintPreview := s.preview
	if fieldRasterVisiblePixelCount(paintPreview) == 0 && fieldRasterVisiblePixelCount(s.stablePreview) > 0 {
		paintPreview = s.stablePreview
	}
	if paintPreview.W > 0 && paintPreview.H > 0 {
		display := fieldCompositePreviewCached(s, paintPreview)
		if len(display) > 0 {
			bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(paintPreview.W), BiHeight: -int32(paintPreview.H), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
			procSetStretchBltMode.Call(hdc, COLORONCOLOR)
			procStretchDIBits.Call(hdc, uintptr(s.sceneRect.Left), uintptr(s.sceneRect.Top), uintptr(s.sceneRect.Right-s.sceneRect.Left), uintptr(s.sceneRect.Bottom-s.sceneRect.Top), 0, 0, uintptr(paintPreview.W), uintptr(paintPreview.H), uintptr(unsafe.Pointer(&display[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, SRCCOPY)
		}
	}
	if s.selected >= 0 && s.selected < len(s.scene.Objects) && s.preview.W > 0 {
		p := fieldProjectorForScene(s.scene, s.preview.W, s.preview.H)
		x, y, _, ok := p.project(fieldObjectCenter(s.scene.Objects[s.selected]))
		if ok {
			dx := int(s.sceneRect.Left) + x*int(s.sceneRect.Right-s.sceneRect.Left)/s.preview.W
			dy := int(s.sceneRect.Top) + y*int(s.sceneRect.Bottom-s.sceneRect.Top)/s.preview.H
			pen, _, _ := procCreatePen.Call(PS_DOT, 1, uintptr(rgb(255, 190, 0)))
			old, _, _ := procSelectObject.Call(hdc, pen)
			procEllipse.Call(hdc, uintptr(dx-18), uintptr(dy-18), uintptr(dx+19), uintptr(dy+19))
			procSelectObject.Call(hdc, old)
			procDeleteObject.Call(pen)
		}
	}
}

func fieldScenePixel(s *fieldEditorState, x, y int) (int, int, bool) {
	if !fieldPointInRect(x, y, s.sceneRect) || s.preview.W < 1 || s.preview.H < 1 {
		return 0, 0, false
	}
	px := (x - int(s.sceneRect.Left)) * s.preview.W / max(int(s.sceneRect.Right-s.sceneRect.Left), 1)
	py := (y - int(s.sceneRect.Top)) * s.preview.H / max(int(s.sceneRect.Bottom-s.sceneRect.Top), 1)
	return px, py, true
}

func fieldSelectAt(s *fieldEditorState, x, y int) int {
	px, py, ok := fieldScenePixel(s, x, y)
	if !ok {
		return -1
	}
	return fieldObjectScreenHit(s.scene, px, py, s.preview.W, s.preview.H)
}

func fieldAddObjectAt(s *fieldEditorState, kind fieldObjectKind, x, y int) {
	oDefault := fieldDefaultObject(kind)
	if s == nil || s.scene.Display.electric() && !oDefault.electric() || s.scene.Display.magnetic() && !oDefault.magnetic() || s.scene.Display.gravity() && !oDefault.gravity() {
		return
	}
	if len(s.scene.Objects) >= fieldMaxObjects {
		fieldSetText(s.status, "물체는 최대 8개까지 배치할 수 있습니다.")
		return
	}
	px, py, ok := fieldScenePixel(s, x, y)
	if !ok {
		return
	}
	wx, wz := fieldScreenToWorldPlanar(s.scene, px, py, s.preview.W, s.preview.H)
	o := fieldDefaultObject(kind)
	o.X, o.Y, o.Z = wx, 0, wz
	s.scene.Objects = append(s.scene.Objects, o)
	s.selected = len(s.scene.Objects) - 1
	guide := "전하량·재질·크기·회전"
	if kind == fieldObjectFerromagnet {
		guide = "자성률·반지름"
	} else if o.magnetic() {
		guide = "세기·크기·회전"
	} else if o.gravity() {
		guide = "질량·크기·회전"
		if o.Kind == fieldObjectSolidMassSphere || o.Kind == fieldObjectHollowMassSphere {
			guide = "질량·반지름·회전속도"
		}
	}
	fieldSetText(s.status, fmt.Sprintf("%s을 배치했습니다. %s을 조절하십시오. (%d/8)", fieldKindName(kind), guide, len(s.scene.Objects)))
	fieldSyncControls(s)
}

func fieldDeleteSelected(s *fieldEditorState) {
	if s == nil || s.selected < 0 || s.selected >= len(s.scene.Objects) {
		return
	}
	name := fieldKindName(s.scene.Objects[s.selected].Kind)
	objects, selected, deleted := fieldDeleteObjectAt(s.scene.Objects, s.selected)
	if !deleted {
		return
	}
	s.scene.Objects, s.selected = objects, selected
	// A delete command may arrive from the right mouse button while a prior
	// capture transition is still settling.  Never leave a stale object index in
	// the drag state after shortening the slice.
	s.dragPalette, s.dragObject, s.dragging = -1, -1, false
	s.dragAnchorValid = false
	procReleaseCapture.Call()
	fieldSetText(s.status, name+"을 삭제했습니다.")
	fieldSyncControls(s)
	fieldRefreshPreview(s, false)
	logf("FIELD_OBJECT delete kind=%d remaining=%d selected=%d", int(s.scene.Display), len(s.scene.Objects), s.selected)
}

func fieldInsert(s *fieldEditorState) {
	if s == nil || s.owner == nil || len(s.scene.Objects) == 0 {
		if s != nil {
			messageBox(s.hwnd, "배치한 물체가 없습니다.", "장 그리기", MB_OK|MB_ICONINFORMATION)
		}
		return
	}
	s.asyncMu.Lock()
	if s.insertBusy || s.destroyed {
		s.asyncMu.Unlock()
		return
	}
	s.insertBusy = true
	s.insertSerial++
	serial := s.insertSerial
	s.insertResult = nil
	insertCancel := fieldNewRenderCancellation()
	s.insertCancel = insertCancel
	// Any preview worker already in flight may finish, but its result belongs to
	// the pre-insertion state and must not be applied while the quality-2 export
	// owns the shared field backend.
	s.previewSerial++
	if s.previewCancel != nil {
		s.previewCancel.Cancel()
	}
	s.previewPending = false
	s.previewPendingReason = ""
	s.previewPendingFast = false
	s.asyncMu.Unlock()

	w, h := 720, 520
	if s.scene.Dimension == fieldDimension3D {
		h = 560
	}
	// Render a deep copy on a worker goroutine.  Quality-2 3-D membranes can
	// involve hundreds of thousands of triangles; doing that inside WM_COMMAND
	// blocks painting and input long enough for Windows to mark the editor as
	// unresponsive.  D3D11 access is already serialized by the shared backend
	// mutex, and the completed raster is posted back to the UI thread for the
	// actual canvas paste.
	scene := s.scene
	scene.Objects = append([]fieldObject(nil), s.scene.Objects...)
	scene.precise3DLines = fieldSceneNeedsPrecise3DLines(scene)
	scene.cancel = insertCancel
	black, hwnd := s.black, s.hwnd
	procEnableWindow.Call(s.insertBtn, 0)
	fieldSetText(s.status, "3차원 장을 계산하는 중입니다. 창은 계속 움직일 수 있으며 취소할 수도 있습니다.")
	logf("FIELD insert begin serial=%d dimension=%d display=%d objects=%d density=%d vector_density=%.1fx smooth=%v actualSS=%d", serial, scene.Dimension, scene.Display, len(scene.Objects), scene.Density, fieldVectorArrowDensityScale(scene), scene.Antialias, fieldRenderSampleScale(scene))
	go func(state *fieldEditorState, target uintptr, request uint64, snapshot fieldScene, backgroundBlack bool, width, height int) {
		started := time.Now()
		r := fieldRenderForBackground(snapshot, width, height, 2, backgroundBlack)
		result := &fieldAsyncInsertResult{Serial: request, Raster: r, Elapsed: time.Since(started)}
		state.asyncMu.Lock()
		if state.destroyed || state.insertSerial != request {
			state.asyncMu.Unlock()
			return
		}
		state.insertResult = result
		state.asyncMu.Unlock()
		procPostMessageW.Call(target, fieldInsertDoneMessage, 0, 0)
	}(s, hwnd, serial, scene, black, w, h)
}

func fieldFinishAsyncInsert(s *fieldEditorState) {
	if s == nil {
		return
	}
	s.asyncMu.Lock()
	result := s.insertResult
	s.insertResult = nil
	if result == nil || result.Serial != s.insertSerial || s.destroyed {
		s.asyncMu.Unlock()
		return
	}
	s.insertBusy = false
	s.insertCancel = nil
	s.asyncMu.Unlock()
	procEnableWindow.Call(s.insertBtn, 1)

	r := result.Raster
	img := ClipboardImage{W: r.W, H: r.H, Pix: append([]byte(nil), r.Pix...), Mask: append([]bool(nil), r.Mask...)}
	if !validClipboardImage(img) || !s.owner.pasteClipboardImage(img, "장 그리기") {
		fieldSetText(s.status, "계산은 끝났지만 캔버스 삽입에 실패했습니다.")
		messageBox(s.hwnd, "계산한 장 이미지를 캔버스에 삽입하지 못했습니다.", "장 그리기", MB_OK|MB_ICONERROR)
		logf("FIELD insert failed serial=%d elapsed=%s size=%dx%d", result.Serial, result.Elapsed, r.W, r.H)
		return
	}
	logf("FIELD insert complete serial=%d dimension=%d display=%d objects=%d density=%d vector_density=%.1fx levels=%d/%d triangles=%d smooth=%v actualSS=%d elapsed=%s", result.Serial, s.scene.Dimension, s.scene.Display, len(s.scene.Objects), s.scene.Density, fieldVectorArrowDensityScale(s.scene), r.EquipotentialRenderedLevels, r.EquipotentialRequestedLevels, r.EquipotentialVacuumTriangles, s.scene.Antialias, fieldRenderSampleScale(s.scene), result.Elapsed)
	procDestroyWindow.Call(s.hwnd)
}

func fieldHandleCommand(s *fieldEditorState, id, code int) {
	if s == nil || s.updating {
		return
	}
	s.asyncMu.Lock()
	busy := s.insertBusy
	s.asyncMu.Unlock()
	if busy && id != fieldIDCancel {
		return
	}
	switch id {
	case fieldIDDimension:
		if code == 1 {
			sel := int(sendMessage(s.dimensionCombo, CB_GETCURSEL, 0, 0))
			if sel < 0 {
				sel = 0
			}
			s.scene.Dimension = fieldDimension(sel)
			fieldUpdateDimensionControls(s)
			fieldRefreshPreview(s, false)
		}
	case fieldIDDisplay:
		if code == 1 {
			sel := int(sendMessage(s.displayCombo, CB_GETCURSEL, 0, 0))
			if sel < 0 {
				sel = 0
			}
			fieldSwitchDisplayDomain(s, fieldDisplayMode(sel))
			domain := "전기"
			if s.scene.Display.magnetic() {
				domain = "자기"
			}
			fieldSetText(s.status, domain+" 물체 팔레트로 전환했습니다. 다른 계의 배치는 별도로 보존됩니다.")
			fieldRefreshPreview(s, false)
		}
	case fieldIDApplySpecial:
		fieldApplySpecialPointLayout(s)
	case fieldIDObject:
		if code == 1 {
			sel := int(sendMessage(s.objectCombo, CB_GETCURSEL, 0, 0)) - 1
			if sel < -1 || sel >= len(s.scene.Objects) {
				sel = -1
			}
			s.selected = sel
			fieldSyncControls(s)
			invalidate(s.hwnd)
		}
	case fieldIDMaterial:
		if code == 1 {
			if o := fieldSelectedObject(s); o != nil && o.electric() {
				sel := int(sendMessage(s.materialCombo, CB_GETCURSEL, 0, 0))
				if sel < 0 {
					sel = 0
				}
				o.Material = fieldMaterial(sel)
				fieldRefreshObjectCombo(s)
				fieldRefreshPreview(s, false)
			}
		}
	case fieldIDAntialias:
		s.scene.Antialias = sendMessage(s.antialiasCheck, BM_GETCHECK, 0, 0) == BST_CHECKED
		logf("FIELD_AA changed enabled=%v actualSS=%d display=%d dimension=%d", s.scene.Antialias, fieldRenderSampleScale(s.scene), s.scene.Display, s.scene.Dimension)
		fieldRefreshPreview(s, false)
	case fieldIDBlack:
		s.black = sendMessage(s.blackCheck, BM_GETCHECK, 0, 0) == BST_CHECKED
		fieldRefreshPreview(s, false)
	case fieldIDDelete:
		fieldDeleteSelected(s)
	case fieldIDClear:
		s.scene.Objects = nil
		s.selected = -1
		s.dragPalette, s.dragObject, s.dragging = -1, -1, false
		s.dragAnchorValid = false
		procReleaseCapture.Call()
		domain := "전기"
		if s.scene.Display.magnetic() {
			domain = "자기"
		} else if s.scene.Display.gravity() {
			domain = "중력"
		}
		fieldSetText(s.status, domain+" 물체를 모두 지웠습니다.")
		fieldSyncControls(s)
		fieldRefreshPreview(s, false)
		logf("FIELD_OBJECT clear domain=%d", int(s.scene.Display))
	case fieldIDInsert:
		fieldInsert(s)
	case fieldIDCancel:
		procDestroyWindow.Call(s.hwnd)
	}
}

func fieldEditorWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			logPanic("fieldEditorWndProc", r)
		}
	}()
	s := activeFieldEditor
	if r, handled := handleDarkEditorNonClient(hwnd, msg, wparam, lparam); handled {
		return r
	}
	switch msg {
	case WM_CREATE:
		if s == nil {
			return ^uintptr(0)
		}
		s.hwnd = hwnd
		fieldEditorCreateControls(s)
		var client RECT
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
		fieldEditorLayout(s, client)
		fieldUpdateDimensionControls(s)
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
			fieldEditorLayout(s, client)
			fieldRefreshPreview(s, false)
		}
		return 0
	case WM_COMMAND:
		if s != nil && s.hwnd == hwnd {
			fieldHandleCommand(s, int(wparam&0xffff), int((wparam>>16)&0xffff))
		}
		return 0
	case WM_HSCROLL:
		if s != nil && s.hwnd == hwnd {
			s.asyncMu.Lock()
			busy := s.insertBusy
			s.asyncMu.Unlock()
			if !busy {
				code := int(wparam & 0xffff)
				fieldApplySliders(s, code != SB_ENDSCROLL)
			}
		}
		return 0
	case WM_LBUTTONDOWN:
		if s != nil && s.hwnd == hwnd {
			s.asyncMu.Lock()
			busy := s.insertBusy
			s.asyncMu.Unlock()
			if busy {
				return 0
			}
			x, y := int(int16(loword(lparam))), int(int16(hiword(lparam)))
			if pi := fieldPaletteHit(s, x, y); pi >= 0 {
				s.dragPalette, s.dragObject, s.dragging = pi, -1, true
				procSetCapture.Call(hwnd)
				kinds := fieldPaletteKindsForScene(s.scene)
				fieldSetText(s.status, fieldKindName(kinds[pi])+"을 장면으로 끌어 놓으십시오.")
				return 0
			}
			if hit := fieldSelectAt(s, x, y); hit >= 0 {
				s.selected, s.dragObject, s.dragPalette, s.dragging = hit, hit, -1, true
				s.dragAnchorValid = false
				if px, py, ok := fieldScenePixel(s, x, y); ok && hit < len(s.scene.Objects) {
					p := fieldProjectorForScene(s.scene, s.preview.W, s.preview.H)
					centre := fieldObjectCenter(s.scene.Objects[hit])
					if cx, cy, _, visible := p.projectFloat(centre); visible {
						s.dragGrabDX = float64(px) - cx
						s.dragGrabDY = float64(py) - cy
						s.dragPlaneY = centre.Y
						s.dragAnchorValid = true
					}
				}
				fieldSyncControls(s)
				procSetCapture.Call(hwnd)
				invalidate(hwnd)
				return 0
			}
			s.selected = -1
			fieldSyncControls(s)
			invalidate(hwnd)
		}
	case WM_MOUSEMOVE:
		if s != nil && s.hwnd == hwnd && s.dragging && s.dragObject >= 0 && wparam&MK_LBUTTON != 0 {
			x, y := int(int16(loword(lparam))), int(int16(hiword(lparam)))
			px, py, ok := fieldScenePixel(s, x, y)
			if ok && s.dragObject < len(s.scene.Objects) {
				targetPX, targetPY := float64(px), float64(py)
				targetY := s.scene.Objects[s.dragObject].Y
				if s.dragAnchorValid {
					// Keep the exact point grabbed in the current preview under the
					// mouse instead of snapping the object's centre to the pointer.
					targetPX -= s.dragGrabDX
					targetPY -= s.dragGrabDY
					targetY = s.dragPlaneY
				}
				wx, wz := fieldScreenToWorldAtY(s.scene, targetPX, targetPY, s.preview.W, s.preview.H, targetY)
				s.scene.Objects[s.dragObject].X, s.scene.Objects[s.dragObject].Z = wx, wz
				fieldSchedulePreview(s, true)
			}
			return 0
		}
	case WM_LBUTTONUP:
		if s != nil && s.hwnd == hwnd && s.dragging {
			x, y := int(int16(loword(lparam))), int(int16(hiword(lparam)))
			kinds := fieldPaletteKindsForScene(s.scene)
			if s.dragPalette >= 0 && s.dragPalette < len(kinds) && fieldPointInRect(x, y, s.sceneRect) {
				fieldAddObjectAt(s, kinds[s.dragPalette], x, y)
			}
			s.dragPalette, s.dragObject, s.dragging = -1, -1, false
			s.dragAnchorValid = false
			procReleaseCapture.Call()
			fieldCommitFinalPreview(s, "object-drag-end")
			return 0
		}
	case WM_CANCELMODE:
		if s != nil && s.hwnd == hwnd && s.dragging {
			s.dragPalette, s.dragObject, s.dragging = -1, -1, false
			s.dragAnchorValid = false
			fieldCommitFinalPreview(s, "cancel-mode")
			return 0
		}
	case WM_CAPTURECHANGED:
		if s != nil && s.hwnd == hwnd && s.dragging {
			newOwner := uintptr(lparam)
			if newOwner != hwnd {
				s.dragPalette, s.dragObject, s.dragging = -1, -1, false
				s.dragAnchorValid = false
				fieldCommitFinalPreview(s, "capture-changed")
			}
			return 0
		}
	case WM_RBUTTONDOWN:
		if s != nil && s.hwnd == hwnd {
			x, y := int(int16(loword(lparam))), int(int16(hiword(lparam)))
			if hit := fieldSelectAt(s, x, y); hit >= 0 {
				s.selected = hit
				fieldDeleteSelected(s)
			}
			return 0
		}
	case WM_DRAWITEM:
		if lparam != 0 && drawThemedOwnerDrawPushButton((*DRAWITEMSTRUCT)(unsafe.Pointer(lparam))) {
			return 1
		}
	case WM_KEYDOWN:
		if wparam == VK_ESCAPE {
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case WM_TIMER:
		if s != nil && s.hwnd == hwnd && wparam == fieldPreviewTimerID {
			procKillTimer.Call(hwnd, fieldPreviewTimerID)
			mode := s.previewMode
			s.previewMode, s.previewQueued = 0, false
			if mode == 2 {
				fieldCommitFinalPreview(s, "timer-final")
			}
			return 0
		}
	case fieldPreviewDoneMessage:
		if s != nil && s.hwnd == hwnd {
			fieldFinishAsyncPreview(s)
		}
		return 0
	case fieldInsertDoneMessage:
		if s != nil && s.hwnd == hwnd {
			fieldFinishAsyncInsert(s)
		}
		return 0
	case WM_PAINT:
		if s != nil && s.hwnd == hwnd {
			var ps PAINTSTRUCT
			hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			var client RECT
			procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
			w, h := int(client.Right-client.Left), int(client.Bottom-client.Top)
			if fieldEnsurePaintBuffer(s, hdc, max(w, 1), max(h, 1)) {
				fieldPaintEditor(s, s.paintDC)
				procBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), s.paintDC, 0, 0, SRCCOPY)
			} else {
				fieldPaintEditor(s, hdc)
			}
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
			procKillTimer.Call(hwnd, fieldPreviewTimerID)
			s.asyncMu.Lock()
			s.destroyed = true
			if s.previewCancel != nil {
				s.previewCancel.Cancel()
				s.previewCancel = nil
			}
			if s.insertCancel != nil {
				s.insertCancel.Cancel()
				s.insertCancel = nil
			}
			s.previewSerial++
			s.previewRenderBusy = false
			s.previewPending = false
			s.previewPendingReason = ""
			s.previewPendingFast = false
			s.previewResult = nil
			s.insertSerial++
			s.insertBusy = false
			s.insertResult = nil
			s.asyncMu.Unlock()
			fieldDestroyPaintBuffer(s)
			if s.uiFont != 0 {
				procDeleteObject.Call(s.uiFont)
				s.uiFont = 0
			}
			activeFieldEditor = nil
			logf("FIELD editor closed")
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}
