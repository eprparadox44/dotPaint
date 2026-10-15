//go:build windows

package main

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	multidimEditorClassName  = "dotPaintMultidimShapeEditorA132"
	multidimPreviewClassName = "dotPaintMultidimPreviewA132"
	wmUser                   = 0x0400
	ssCenter                 = 0x00000001
	ssCenterImage            = 0x00000200
	esReadOnly               = 0x0800
	ICC_BAR_CLASSES          = 0x00000004
	TBS_HORZ                 = 0x0000
	TBS_AUTOTICKS            = 0x0001
	TBM_GETPOS               = wmUser + 0
	TBM_SETPOS               = wmUser + 5
	TBM_SETRANGE             = wmUser + 6
	TBM_SETTICFREQ           = wmUser + 20
	CB_SETMINVISIBLE         = 0x1701
	PS_DOT                   = 2
	mdPSGeometric            = 0x00010000
	mdPSEndcapFlat           = 0x00000200
	mdPSJoinMiter            = 0x00002000
	mdBSSolid                = 0
)

const (
	multidimHalftone       = 4
	multidimCBResetContent = 0x014B
)

const (
	multidimIDShapeCombo = 7601 + iota
	multidimIDOpacityCombo
	multidimIDInsert
	multidimIDCancel
	multidimIDPreview
	multidimIDInfo
	multidimIDDescription
	multidimIDXSlider
	multidimIDYSlider
	multidimIDZSlider
	multidimIDWSlider
	multidimIDSizeSlider
	multidimIDWidthSlider
	multidimIDHeightSlider
	multidimIDDepthSlider
	multidimIDSmoothCheck
	multidimIDInvertCheck
	multidimIDAxesCheck
	multidimIDMoveAxesCheck
	multidimIDOptionCombo
	multidimIDAxisScaleSlider
	multidimIDAxisReset
	multidimIDShapeReset
)

type INITCOMMONCONTROLSEX struct {
	DwSize uint32
	DwICC  uint32
}

type multidimShapeSpec struct {
	name          string
	description   string
	is4D          bool
	useWidth      bool
	useHeight     bool
	useDepth      bool
	defaultSize   int
	defaultWidth  int
	defaultHeight int
	defaultDepth  int
}

var multidimShapeSpecs = []multidimShapeSpec{
	{"원판", "두께가 없는 원형 평면입니다. 좌표축은 도형 중심에 붙어 함께 이동하며, 옵션에서 극좌표 격자를 표시할 수 있습니다.", false, false, false, false, 180, 180, 180, 180},
	{"평면", "두께가 없는 사각 평면입니다. 좌표축은 도형 중심에 붙어 함께 이동하고, 가로와 세로를 독립적으로 조절합니다.", false, true, true, false, 170, 220, 160, 120},
	{"좌표평면", "XY·XZ·YZ 세 평면이 원점에서 서로 직교하는 3차원 좌표평면입니다. 세 평면과 xyz축은 한 묶음으로 함께 이동합니다.", false, true, true, true, 170, 220, 10, 5},
	{"포물면/쌍곡면", "이심률 e=1에서는 회전 포물면, e>1에서는 쌍곡면을 표시합니다. 옵션에서 일엽·이엽 계열을 선택합니다.", false, true, true, true, 170, 100, 170, 190},
	{"말안장면", "서로 수직인 두 방향으로 반대 곡률을 갖는 쌍곡포물면입니다. 가로·세로 범위와 굽힘 높이를 독립적으로 조절합니다.", false, true, true, true, 170, 190, 160, 120},
	{"시공간: 블랙홀", "중심방향 3차원 xyz 좌표망과 Kerr/Schwarzschild 광학 렌더링을 표시 메뉴에서 전환합니다. 거리·좌표망 밀도·무차원 회전량 a*를 조절합니다.", false, false, false, false, 200, 2, 0, 170},
	{"쌍원뿔과 원뿔곡선", "꼭짓점이 맞닿은 두 원뿔과 절단 평면, 실제 교선을 함께 보여 줍니다. 가로·세로·모선 평행·비스듬한 절단을 선택합니다.", false, true, true, true, 170, 180, 220, 35},
	{"구", "모든 방향에서 반지름이 같은 기본 3차원 도형입니다. 옵션에서 NASA Blue Marble 지구, LRO 달, Cassini 전구면 토성과 고리를 선택할 수 있습니다.", false, false, false, false, 150, 150, 150, 150},
	{"타원체", "서로 다른 세 축 반지름을 갖는 3차원 이차곡면입니다. 가로·세로·높이를 각각 조절합니다. 실제 미리보기와 면 렌더링을 지원합니다.", false, true, true, true, 150, 180, 140, 120},
	{"원기둥", "두 원형 밑면과 옆면으로 이루어진 도형입니다. 가로는 지름, 세로는 높이로 사용합니다. 실제 미리보기와 면 렌더링을 지원합니다.", false, true, true, false, 150, 180, 220, 150},
	{"원뿔", "원형 밑면과 하나의 꼭짓점을 잇는 회전체입니다. 원기둥과 같은 가로(밑면 지름)·세로(높이) 슬라이드를 사용합니다.", false, true, true, false, 150, 180, 220, 150},
	{"다각뿔", "정다각형 밑면의 중심 위에 꼭짓점이 놓인 각뿔입니다. 가로는 밑면 지름, 세로는 높이이며 우측 상단 메뉴에서 4각~12각을 고릅니다.", false, true, true, false, 150, 180, 220, 4},
	{"원환면(토러스)", "원 둘레를 따라 작은 원을 회전시킨 닫힌 곡면입니다. 가로는 큰 반지름, 세로는 관 반지름, 높이는 곡면 분할 밀도입니다. 정면에서 선 앞을 덮는 곡면 수에 따라 점선 없이 흐려집니다.", false, true, true, true, 150, 190, 65, 48},
	{"뫼비우스 띠", "한 번 반쯤 비틀어 양 끝을 이은 한 면짜리 비가향 곡면입니다. 가로는 중심 반지름, 세로는 띠의 반폭, 높이는 곡면 분할 밀도입니다.", false, true, true, true, 150, 190, 55, 48},
	{"정사면체", "면이 모두 정삼각형인 가장 단순한 정다면체입니다.", false, false, false, false, 170, 170, 170, 170},
	{"정육면체", "정사각형 6개 면으로 이루어진 정다면체입니다. 실제 미리보기와 면 렌더링을 지원합니다.", false, false, false, false, 170, 170, 170, 170},
	{"정팔면체", "정삼각형 8개 면으로 이루어진 정다면체입니다.", false, false, false, false, 170, 170, 170, 170},
	{"정십이면체", "정오각형 12개 면으로 이루어진 정다면체입니다.", false, false, false, false, 165, 165, 165, 165},
	{"정이십면체", "정삼각형 20개 면으로 이루어진 정다면체입니다.", false, false, false, false, 165, 165, 165, 165},
	{"풀러렌(C60)", "12개의 오각형과 20개의 육각형으로 이루어진 축구공형 분자 골격입니다.", false, false, false, false, 165, 165, 165, 165},
	{"2-스타인메츠 입체", "서로 직교하는 두 원기둥의 교집합 입체입니다.", false, false, false, false, 170, 170, 170, 170},
	{"3-스타인메츠 입체", "세 방향 원기둥의 공통 교집합으로 이루어진 입체입니다.", false, false, false, false, 170, 170, 170, 170},
	{"정다포체: 5-셀", "4차원에서 가장 단순한 정다포체입니다.", true, false, false, false, 160, 160, 160, 160},
	{"정다포체: 테서렉트", "4차원 정육포체(tesseract)입니다.", true, false, false, false, 160, 160, 160, 160},
	{"정다포체: 16-셀", "테서렉트의 쌍대 다포체인 4차원 정16포체입니다.", true, false, false, false, 160, 160, 160, 160},
	{"정다포체: 24-셀", "4차원에서만 존재하는 특수한 정24포체입니다.", true, false, false, false, 155, 155, 155, 155},
	{"정다포체: 120-셀", "정십이면체 셀 120개로 이루어진 정다포체입니다.", true, false, false, false, 145, 145, 145, 145},
	{"정다포체: 600-셀", "정사면체 셀 600개로 이루어진 정다포체입니다.", true, false, false, false, 145, 145, 145, 145},
	{"4차원 곡면: 클라인 병", "고전적인 병 모양의 3차원 투영을 유지하면서, 관통해 보이는 두 면을 W 좌표로 분리한 4차원 비가향 곡면입니다. 가로는 몸통 곡선 크기, 세로는 관 굵기, 높이는 분할 밀도입니다.", true, true, true, true, 150, 180, 65, 24},
}

var multidimOpacityModes = []string{"불투명", "반투명", "투명"}

const (
	mdKleinGridNone = iota
	mdKleinGridMajor
	mdKleinGridFull
)

var (
	modComctl32                    = syscall.NewLazyDLL("comctl32.dll")
	procInitCommonControlsEx       = modComctl32.NewProc("InitCommonControlsEx")
	procPolygon                    = modGdi32.NewProc("Polygon")
	procStretchBlt                 = modGdi32.NewProc("StretchBlt")
	procExtCreatePenA194           = modGdi32.NewProc("ExtCreatePen")
	multidimEditorWndProcCallback  uintptr
	multidimPreviewWndProcCallback uintptr
	multidimEditorClassRegistered  bool
	multidimPreviewClassRegistered bool
	activeMultidimShapeEditor      *multidimShapeEditorState
)

type mdLogBrushA194 struct {
	Style uint32
	Color uint32
	Hatch uintptr
}

type multidimShapeEditorState struct {
	owner  uintptr
	app    *App
	hwnd   uintptr
	uiFont uintptr

	shapeLabel      uintptr
	shapeCombo      uintptr
	opacityLabel    uintptr
	opacityCombo    uintptr
	smoothCheck     uintptr
	invertCheck     uintptr
	axesCheck       uintptr
	moveAxesCheck   uintptr
	axisScaleLabel  uintptr
	axisScaleSlider uintptr
	axisScaleValue  uintptr
	axisResetBtn    uintptr
	shapeResetBtn   uintptr
	optionLabel     uintptr
	optionCombo     uintptr
	polygonSides    int
	sphereGuides    int
	sphereSkin      int
	kleinGrid       int
	torusGrid       int
	mobiusGrid      int
	planarGrid      int
	saddleGrid      int
	coordinatePlane int
	quadricFamily   int
	coneCutMode     int
	blackHoleMode   int
	previewBox      uintptr
	statusLabel     uintptr
	insertBtn       uintptr
	cancelBtn       uintptr

	xLabel      uintptr
	xSlider     uintptr
	xValueLabel uintptr
	yLabel      uintptr
	ySlider     uintptr
	yValueLabel uintptr
	zLabel      uintptr
	zSlider     uintptr
	zValueLabel uintptr
	wLabel      uintptr
	wSlider     uintptr
	wValueLabel uintptr

	sizeLabel        uintptr
	sizeSlider       uintptr
	sizeValueLabel   uintptr
	widthLabel       uintptr
	widthSlider      uintptr
	widthValueLabel  uintptr
	heightLabel      uintptr
	heightSlider     uintptr
	heightValueLabel uintptr
	depthLabel       uintptr
	depthSlider      uintptr
	depthValueLabel  uintptr

	// Persistent preview buffers prevent the child window from flashing its
	// background between erase and paint. The smooth buffer is reused too, so
	// slider movement does not allocate a large bitmap for every frame.
	previewBackDC          uintptr
	previewBackBitmap      uintptr
	previewBackOldBitmap   uintptr
	previewBackW           int
	previewBackH           int
	previewSmoothDC        uintptr
	previewSmoothBitmap    uintptr
	previewSmoothOldBitmap uintptr
	previewSmoothW         int
	previewSmoothH         int
	previewSmoothBits      unsafe.Pointer
	previewDownsample      []byte
	smoothEnabled          bool

	// A203 persistent top-down DIB used by CPU raster renderers. This makes
	// the first black-hole/planet frame visible without requiring a drag or
	// slider event to perturb the GDI state.
	rasterBlitDC        uintptr
	rasterBlitBitmap    uintptr
	rasterBlitOldBitmap uintptr
	rasterBlitBits      unsafe.Pointer
	rasterBlitW         int
	rasterBlitH         int

	// A161 opaque 4D face raster scratch. The slices and raster bounds are
	// retained between frames. Opaque edge segments query this exact face
	// depth buffer instead of relying on polygon-level occlusion estimates.
	opaqueFacePixels []byte
	opaqueFaceDepth  []float32
	opaqueFaceLeft   int
	opaqueFaceTop    int
	opaqueFaceW      int
	opaqueFaceH      int
	opaqueFaceValid  bool

	// A197 fixed-parameter Schwarzschild ray-trace cache. The expensive optical
	// calculation is reused across ordinary WM_PAINT and the insertion mask pass.
	blackHoleCachePixels []byte
	blackHoleCacheW      int
	blackHoleCacheH      int
	blackHoleCacheKey    uint64

	// A205 embedded NASA/JPL source imagery. Earth and Moon are wrapped from
	// cylindrical maps; Saturn uses a Cassini-derived full cylindrical globe
	// texture plus a separately reconstructed ring profile. The cache stores the
	// current offline-rendered frame.
	celestialCachePixels []byte
	celestialCacheW      int
	celestialCacheH      int
	celestialCacheKey    uint64

	// Set only while producing the black-geometry insertion pass. Dedicated
	// raster renderers use it to output a correct white-key coverage mask.
	insertMaskPass bool

	previewInteractive bool
	previewDirty       bool

	// A171 virtual-trackball state. Dragging directly in the preview updates
	// the existing x/y/z trackbars, so the numeric rotation fields remain the
	// single source of truth for rendering and insertion.
	previewRotationDragging bool
	previewDragStartVector  mdVec3
	previewDragStartMatrix  mdMat3

	// A176 movable rotation pivot. axisOrigin is the screen-normalized XYZ
	// location shown by the overlay. axisPivotLocal3/4 store the corresponding
	// point in the object's unrotated coordinates, so the renderer can always
	// translate that point back onto axisOrigin after applying the current
	// absolute sliders. Moving the pivot recomputes only this local point and
	// therefore leaves the current object pose untouched.
	axisOrigin      mdVec3
	axisPivotLocal3 mdVec3
	axisPivotLocal4 mdVec4
	// A192: every shape starts with the visible XYZ frame attached to its
	// local object frame. Explicit axis movement detaches the frame and turns
	// it into the independent rotation pivot used by the X/Y/Z/W controls.
	axisDetached                     bool
	previewAxisDragging              bool
	previewAxisDragStartX            int
	previewAxisDragStartY            int
	previewAxisDragStartOrigin       mdVec3
	previewAxisDragStartObjectOffset mdVec3
}

type mdEdge struct{ a, b int }
type mdFaceRender struct {
	pts    []POINT
	depth  float64
	bright float64
}

type mdSegment struct {
	a, b  POINT
	depth float64
}

type mdEdgePathSpec struct {
	indices       []int
	depthCenter   float64
	alwaysVisible bool
}

type mdRegular4DGeometry struct {
	verts []mdVec4
	edges []mdEdge4
	faces []mdFace4
}

var (
	mdCachedTesseractGeometry = func() mdRegular4DGeometry {
		verts, edges, quads, _ := mdBuildTesseract()
		return mdRegular4DGeometry{verts: verts, edges: edges, faces: mdTesseractFacesAsPolygons(quads)}
	}()
	mdCached5CellGeometry = func() mdRegular4DGeometry {
		verts, edges, faces, _ := mdBuildRegular5Cell()
		return mdRegular4DGeometry{verts: verts, edges: edges, faces: faces}
	}()
	mdCached16CellGeometry = func() mdRegular4DGeometry {
		verts, edges, faces, _ := mdBuildRegular16Cell()
		return mdRegular4DGeometry{verts: verts, edges: edges, faces: faces}
	}()
	mdCached24CellGeometry = func() mdRegular4DGeometry {
		verts, edges, faces, _ := mdBuildRegular24Cell()
		return mdRegular4DGeometry{verts: verts, edges: edges, faces: faces}
	}()
	mdCached120CellOnce     sync.Once
	mdCached120CellGeometry mdRegular4DGeometry
	mdCached600CellOnce     sync.Once
	mdCached600CellGeometry mdRegular4DGeometry
)

func mdRegular120CellGeometry() mdRegular4DGeometry {
	mdCached120CellOnce.Do(func() {
		verts, edges, faces, _ := mdBuildRegular120Cell()
		mdCached120CellGeometry = mdRegular4DGeometry{verts: verts, edges: edges, faces: faces}
	})
	return mdCached120CellGeometry
}

func mdRegular600CellGeometry() mdRegular4DGeometry {
	mdCached600CellOnce.Do(func() {
		verts, edges, faces, _ := mdBuildRegular600Cell()
		mdCached600CellGeometry = mdRegular4DGeometry{verts: verts, edges: edges, faces: faces}
	})
	return mdCached600CellGeometry
}

func initMultidimCommonControls() bool {
	icc := INITCOMMONCONTROLSEX{DwSize: uint32(unsafe.Sizeof(INITCOMMONCONTROLSEX{})), DwICC: ICC_BAR_CLASSES}
	r, _, _ := procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))
	return r != 0
}

func registerMultidimShapeEditorClass(hinst uintptr) bool {
	if multidimEditorClassRegistered {
		return true
	}
	if multidimEditorWndProcCallback == 0 {
		multidimEditorWndProcCallback = syscall.NewCallback(multidimShapeEditorWndProc)
	}
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         CS_HREDRAW | CS_VREDRAW,
		LpfnWndProc:   multidimEditorWndProcCallback,
		HInstance:     hinst,
		HCursor:       cursor,
		HbrBackground: uintptr(COLOR_BTNFACE + 1),
		LpszClassName: utf16Ptr(multidimEditorClassName),
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		logf("MULTIDIM register editor class failed: %v", err)
		return false
	}
	multidimEditorClassRegistered = true
	return true
}

func registerMultidimPreviewClass(hinst uintptr) bool {
	if multidimPreviewClassRegistered {
		return true
	}
	if multidimPreviewWndProcCallback == 0 {
		multidimPreviewWndProcCallback = syscall.NewCallback(multidimPreviewWndProc)
	}
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         CS_HREDRAW | CS_VREDRAW,
		LpfnWndProc:   multidimPreviewWndProcCallback,
		HInstance:     hinst,
		HCursor:       cursor,
		HbrBackground: 0,
		LpszClassName: utf16Ptr(multidimPreviewClassName),
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		logf("MULTIDIM register preview class failed: %v", err)
		return false
	}
	multidimPreviewClassRegistered = true
	return true
}

func (a *App) showMultidimShapeEditor() {
	if activeMultidimShapeEditor != nil && activeMultidimShapeEditor.hwnd != 0 {
		if r, _, _ := procIsWindow.Call(activeMultidimShapeEditor.hwnd); r != 0 {
			procShowWindow.Call(activeMultidimShapeEditor.hwnd, formulaSWRestore)
			procSetForegroundWindow.Call(activeMultidimShapeEditor.hwnd)
			procSetFocus.Call(activeMultidimShapeEditor.shapeCombo)
			return
		}
	}
	if !initMultidimCommonControls() || !registerMultidimShapeEditorClass(a.hinst) || !registerMultidimPreviewClass(a.hinst) {
		messageBox(a.hwnd, "다차원 도형 그리기 창을 만들 수 없습니다.", "삽입 > 이미지 > 다차원 도형", MB_OK|MB_ICONERROR)
		return
	}
	state := &multidimShapeEditorState{owner: a.hinst, app: a, uiFont: createCompactEditorUIFont(), previewDirty: true, polygonSides: 4, sphereGuides: mdSphereGuidesThreeAxes, sphereSkin: mdSphereSkinPlain, kleinGrid: mdKleinGridMajor, torusGrid: mdSurfaceGridMajor, mobiusGrid: mdSurfaceGridMajor, planarGrid: mdPlanarGridMajor, coordinatePlane: mdCoordinatePlaneXY, quadricFamily: mdQuadricOneSheet, coneCutMode: mdConeCutHorizontal}
	activeMultidimShapeEditor = state
	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(multidimEditorClassName))),
		uintptr(unsafe.Pointer(utf16Ptr("다차원 도형 그리기"))),
		WS_OVERLAPPEDWINDOW,
		uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), 1220, 800,
		a.hwnd, 0, a.hinst, 0,
	)
	if hwnd == 0 {
		if state.uiFont != 0 {
			procDeleteObject.Call(state.uiFont)
			state.uiFont = 0
		}
		activeMultidimShapeEditor = nil
		logf("MULTIDIM create window failed: %v", err)
		messageBox(a.hwnd, "다차원 도형 그리기 창을 만들 수 없습니다.", "삽입 > 이미지 > 다차원 도형", MB_OK|MB_ICONERROR)
		return
	}
	state.hwnd = hwnd
	showThemedEditorWindow(hwnd, a.darkMode)
	a.statusText = "다차원 도형 그리기 창을 열었습니다. 기본 축은 도형에 붙고, 축 옮기기를 사용하면 독립 회전 피벗으로 분리됩니다."
	logf("MULTIDIM editor opened hwnd=%#x", hwnd)
	// Warm Direct3D and upload the embedded celestial textures in the background.
	// This hides most of the one-time shader compilation / texture upload pause
	// before the user selects a black hole, Earth, Moon, Saturn, or Klein bottle.
	mdD3D11Prewarm()
}

func multidimCreateChild(s *multidimShapeEditorState, class, title string, style uintptr, id int) uintptr {
	style = ownerDrawPushButtonStyle(class, style)
	h, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(class))),
		uintptr(unsafe.Pointer(utf16Ptr(title))),
		style|WS_CHILD|WS_VISIBLE,
		0, 0, 10, 10,
		s.hwnd, uintptr(id), s.owner, 0,
	)
	if h != 0 {
		if s.app != nil {
			applyDarkThemeToWindow(h, s.app.darkMode)
		}
		setCompactEditorControlFont(h, s.uiFont)
	}
	return h
}

func multidimSetText(hwnd uintptr, text string) {
	if hwnd != 0 {
		procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(text))))
	}
}

func multidimMoveWindow(hwnd uintptr, x, y, w, h int32, repaint bool) {
	var rp uintptr
	if repaint {
		rp = 1
	}
	procMoveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(w), uintptr(h), rp)
}

func multidimMakeLong(lo, hi int) uintptr {
	return uintptr(uint32(uint16(lo)) | uint32(uint16(hi))<<16)
}

func multidimSliderSet(hwnd uintptr, min, max, pos int) {
	sendMessage(hwnd, TBM_SETRANGE, 1, multidimMakeLong(min, max))
	sendMessage(hwnd, TBM_SETTICFREQ, 100, 0)
	sendMessage(hwnd, TBM_SETPOS, 1, uintptr(pos))
}

func multidimSliderPos(hwnd uintptr) int { return int(sendMessage(hwnd, TBM_GETPOS, 0, 0)) }
func multidimRadiansText(hwnd uintptr) string {
	return fmt.Sprintf("%.2f rad", float64(multidimSliderPos(hwnd))/100.0)
}
func multidimLengthText(hwnd uintptr) string    { return fmt.Sprintf("%d", multidimSliderPos(hwnd)) }
func multidimAxisScaleText(hwnd uintptr) string { return fmt.Sprintf("%d%%", multidimSliderPos(hwnd)) }

func multidimRotationSliderPos(angle float64) int {
	angle = mdNormalizeAnglePositive(angle)
	pos := int(math.Round(angle * 100))
	if pos < 0 {
		return 0
	}
	if pos > 628 {
		return 628
	}
	return pos
}

func multidimSetRotationSliders(s *multidimShapeEditorState, ax, ay, az float64) {
	if s == nil {
		return
	}
	sendMessage(s.xSlider, TBM_SETPOS, 1, uintptr(multidimRotationSliderPos(ax)))
	sendMessage(s.ySlider, TBM_SETPOS, 1, uintptr(multidimRotationSliderPos(ay)))
	sendMessage(s.zSlider, TBM_SETPOS, 1, uintptr(multidimRotationSliderPos(az)))
	multidimEditorRefreshValueLabels(s)
}

func multidimPreviewMousePoint(lparam uintptr) (int, int) {
	return int(int16(loWord(lparam))), int(int16(hiWord(lparam)))
}

func multidimTrackballVectorAtAxisOrigin(s *multidimShapeEditorState, x, y, w, h int) mdVec3 {
	if s == nil {
		return mdTrackballVector(x, y, w, h)
	}
	extent := math.Min(float64(w), float64(h))
	origin := s.axisOrigin
	if multidimAxesFollowShape(s) {
		origin = multidimAxisObjectOffset(s)
	}
	offsetX := int(math.Round(origin.x * extent))
	offsetY := int(math.Round(-origin.z * extent))
	return mdTrackballVector(x-offsetX, y-offsetY, w, h)
}

func multidimBeginPreviewRotation(s *multidimShapeEditorState, hwnd uintptr, x, y int) {
	if s == nil || s.previewBox != hwnd {
		return
	}
	var rc RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := int(rc.Right-rc.Left), int(rc.Bottom-rc.Top)
	ax, ay, az, _ := multidimRotation(s)
	s.previewDragStartVector = multidimTrackballVectorAtAxisOrigin(s, x, y, w, h)
	s.previewDragStartMatrix = mdEulerRotationMatrix(ax, ay, az)
	s.previewRotationDragging = true
	s.previewInteractive = true
	procSetCapture.Call(hwnd)
	multidimSetText(s.statusLabel, "미리보기 드래그 회전 중 · x/y/z 회전량 자동 계산 · 놓으면 고품질 갱신")
}

func multidimUpdatePreviewRotation(s *multidimShapeEditorState, hwnd uintptr, x, y int) {
	if s == nil || !s.previewRotationDragging || s.previewBox != hwnd {
		return
	}
	var rc RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := int(rc.Right-rc.Left), int(rc.Bottom-rc.Top)
	current := multidimTrackballVectorAtAxisOrigin(s, x, y, w, h)
	delta := mdTrackballDeltaMatrix(s.previewDragStartVector, current)
	rotation := mdMat3Mul(delta, s.previewDragStartMatrix)
	ax, ay, az := mdEulerFromRotationMatrix(rotation)
	multidimSetRotationSliders(s, ax, ay, az)
	multidimInvalidatePreview(s)
}

func multidimEndPreviewRotation(s *multidimShapeEditorState, hwnd uintptr, releaseCapture bool) {
	if s == nil || !s.previewRotationDragging {
		return
	}
	// Clear state before ReleaseCapture emits WM_CAPTURECHANGED.
	s.previewRotationDragging = false
	s.previewInteractive = false
	if releaseCapture {
		owner, _, _ := procGetCapture.Call()
		if owner == hwnd {
			procReleaseCapture.Call()
		}
	}
	multidimEditorRefreshSelection(s)
	multidimSetText(s.statusLabel, "드래그 회전 완료 · 아래 x/y/z 회전량에 자동 반영됨")
}

// multidimAxisProjectedLocalPivot projects the stored local pivot through the
// current absolute sliders. The object rectangle is then translated so this
// point lands exactly on axisOrigin.
func multidimAxisProjectedLocalPivot(s *multidimShapeEditorState) mdVec3 {
	if s == nil {
		return mdVec3{}
	}
	ax, ay, az, aw := multidimRotation(s)
	spec := multidimEditorSelectedShape(s)
	if !spec.is4D {
		return mdRotate(s.axisPivotLocal3, ax, ay, az)
	}
	if spec.name == "4차원 곡면: 클라인 병" {
		p := mdRot4KleinW(s.axisPivotLocal4, aw)
		return mdRotate(mdProjectKlein4To3(p, aw), ax, ay, az)
	}
	p := mdRot4XW(s.axisPivotLocal4, aw)
	return mdRotate(mdProject4To3(p, 4.0), ax, ay, az)
}

func multidimAxisObjectOffset(s *multidimShapeEditorState) mdVec3 {
	if s == nil {
		return mdVec3{}
	}
	return mdSub(s.axisOrigin, multidimAxisProjectedLocalPivot(s))
}

// multidimRebaseAxisPivot changes the visible rotation origin without moving
// the currently rendered object. currentOffset is the object's translation at
// the instant of the rebase. Solving the local synthetic pivot for
// newOrigin-currentOffset makes the current frame retain that translation,
// while the synthetic pivot itself continues to map exactly onto newOrigin.
// Later X/Y/Z and XW/W changes therefore all orbit around the visible origin.
func multidimRebaseAxisPivot(s *multidimShapeEditorState, newOrigin, currentOffset mdVec3) {
	if s == nil {
		return
	}
	s.axisOrigin = newOrigin
	multidimAxisSetLocalPivotForTarget(s, mdSub(newOrigin, currentOffset))
}

// multidimAxisSetLocalPivotForTarget finds the unrotated object-space point
// that currently projects to target. target is axisOrigin minus the object's
// existing screen-normalized offset, so changing this local point alone leaves
// the current object pose unchanged.
func multidimAxisSetLocalPivotForTarget(s *multidimShapeEditorState, target mdVec3) {
	if s == nil {
		return
	}
	ax, ay, az, aw := multidimRotation(s)
	invXYZ := mdMat3ApplyTranspose(mdEulerRotationMatrix(ax, ay, az), target)
	spec := multidimEditorSelectedShape(s)
	if !spec.is4D {
		s.axisPivotLocal3 = invXYZ
		return
	}

	if spec.name == "4차원 곡면: 클라인 병" {
		// Choose the current post-rotation pivot at visible XYZ with W=0. The
		// dedicated Klein projection is the identity on that hyperplane, and
		// the inverse isoclinic turn is exact even at quarter turns where the
		// former local-W=0 inverse became singular.
		postTurn := mdVec4{x: invXYZ.x, y: invXYZ.y, z: invXYZ.z, w: 0}
		s.axisPivotLocal4 = mdRot4KleinW(postTurn, -aw)
		return
	}

	// The regular 4D shapes retain the local-W=0 perspective inverse.
	c, sn := math.Cos(aw), math.Sin(aw)
	// Perspective inverse for local w=0:
	// qx = 4*x*cos(a)/(4-x*sin(a)).
	denom := 4*c + invXYZ.x*sn
	if math.Abs(denom) < 1e-5 {
		denom = math.Copysign(1e-5, denom)
	}
	lx := 4 * invXYZ.x / denom
	k := 4 / (4 - lx*sn)
	if math.Abs(k) < 1e-5 {
		k = math.Copysign(1e-5, k)
	}
	s.axisPivotLocal4 = mdVec4{x: lx, y: invXYZ.y / k, z: invXYZ.z / k}
}

func multidimBeginPreviewAxisMove(s *multidimShapeEditorState, hwnd uintptr, x, y int) {
	if s == nil || s.previewBox != hwnd || !multidimEditorAxesEnabled(s) || !multidimEditorMoveAxesEnabled(s) {
		return
	}
	s.previewAxisDragStartX = x
	s.previewAxisDragStartY = y
	s.previewAxisDragStartOrigin = s.axisOrigin
	s.previewAxisDragStartObjectOffset = multidimAxisObjectOffset(s)
	if !multidimShapeAxesAttached(multidimEditorSelectedShape(s)) {
		s.axisDetached = true
	}
	s.previewAxisDragging = true
	s.previewInteractive = true
	procSetCapture.Call(hwnd)
	if multidimShapeAxesAttached(multidimEditorSelectedShape(s)) {
		multidimSetText(s.statusLabel, "평면류 이동 중 · 도형과 붙은 xyz축을 화면에서 함께 옮깁니다.")
	} else {
		multidimSetText(s.statusLabel, "축 옮기기 중 · 도형에 붙은 기본 축을 분리해 독립 회전 피벗으로 옮깁니다.")
	}
}

func multidimUpdatePreviewAxisMove(s *multidimShapeEditorState, hwnd uintptr, x, y int) {
	if s == nil || !s.previewAxisDragging || s.previewBox != hwnd {
		return
	}
	var rc RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	extent := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top))
	newOrigin := mdAxisOriginAfterScreenDrag(
		s.previewAxisDragStartOrigin,
		float64(x-s.previewAxisDragStartX),
		float64(y-s.previewAxisDragStartY),
		extent,
	)
	if multidimShapeAxesAttached(multidimEditorSelectedShape(s)) {
		// 원판·평면·3면 좌표평면은 축과 도형 중심을 붙인다. 화면상
		// 드래그 변위만큼 둘이 함께 이동하므로 고정할 제3축을 고를 필요가 없다.
		s.axisOrigin = newOrigin
		s.axisPivotLocal3 = mdVec3{}
		s.axisPivotLocal4 = mdVec4{}
	} else {
		s.axisDetached = true
		multidimRebaseAxisPivot(s, newOrigin, s.previewAxisDragStartObjectOffset)
	}
	multidimEditorRefreshValueLabels(s)
	multidimInvalidatePreview(s)
}

func multidimEndPreviewAxisMove(s *multidimShapeEditorState, hwnd uintptr, releaseCapture bool) {
	if s == nil || !s.previewAxisDragging {
		return
	}
	// Clear state before ReleaseCapture emits WM_CAPTURECHANGED.
	s.previewAxisDragging = false
	s.previewInteractive = false
	if releaseCapture {
		owner, _, _ := procGetCapture.Call()
		if owner == hwnd {
			procReleaseCapture.Call()
		}
	}
	if !multidimShapeAxesAttached(multidimEditorSelectedShape(s)) {
		multidimRefreshAxisDetachedState(s)
	}
	multidimEditorRefreshSelection(s)
	if multidimShapeAxesAttached(multidimEditorSelectedShape(s)) {
		multidimSetText(s.statusLabel, "평면류 이동 완료 · 도형과 xyz축이 붙은 상태로 함께 이동했습니다.")
	} else if s.axisDetached {
		multidimSetText(s.statusLabel, "축 이동 완료 · 축이 도형에서 분리되었으며 이후 X/Y/Z/W 회전은 새 피벗을 기준으로 합니다.")
	} else {
		multidimSetText(s.statusLabel, "축 이동 완료 · 축이 도형 중심과 다시 일치해 기본 결합 상태로 돌아왔습니다.")
	}
}

func multidimBeginPreviewDrag(s *multidimShapeEditorState, hwnd uintptr, x, y int) {
	if multidimEditorMoveAxesEnabled(s) {
		multidimBeginPreviewAxisMove(s, hwnd, x, y)
		return
	}
	multidimBeginPreviewRotation(s, hwnd, x, y)
}

func multidimPreviewDragging(s *multidimShapeEditorState) bool {
	return s != nil && (s.previewRotationDragging || s.previewAxisDragging)
}

func multidimUpdatePreviewDrag(s *multidimShapeEditorState, hwnd uintptr, x, y int) {
	if s == nil {
		return
	}
	if s.previewAxisDragging {
		multidimUpdatePreviewAxisMove(s, hwnd, x, y)
		return
	}
	if s.previewRotationDragging {
		multidimUpdatePreviewRotation(s, hwnd, x, y)
	}
}

func multidimEndPreviewDrag(s *multidimShapeEditorState, hwnd uintptr, releaseCapture bool) {
	if s == nil {
		return
	}
	if s.previewAxisDragging {
		multidimEndPreviewAxisMove(s, hwnd, releaseCapture)
		return
	}
	if s.previewRotationDragging {
		multidimEndPreviewRotation(s, hwnd, releaseCapture)
	}
}

func multidimEditorSmoothEnabled(s *multidimShapeEditorState) bool {
	return s != nil && s.smoothEnabled
}

// A210 keeps the selected face mode and AA quality during live rotation.
// Direct3D 11 now carries face filling and batched visibility tests, so the
// preview no longer changes into a simplified wireframe while dragging.
func multidimRenderSmoothEnabled(s *multidimShapeEditorState) bool {
	return multidimEditorSmoothEnabled(s)
}

func multidimRenderOpacityIndex(s *multidimShapeEditorState) int {
	return multidimOpacityIndex(s)
}

func multidimEditorInvertEnabled(s *multidimShapeEditorState) bool {
	return s != nil && sendMessage(s.invertCheck, BM_GETCHECK, 0, 0) == BST_CHECKED
}

func multidimEditorAxesEnabled(s *multidimShapeEditorState) bool {
	return s != nil && sendMessage(s.axesCheck, BM_GETCHECK, 0, 0) == BST_CHECKED
}

func multidimEditorMoveAxesEnabled(s *multidimShapeEditorState) bool {
	return s != nil && multidimEditorAxesEnabled(s) && sendMessage(s.moveAxesCheck, BM_GETCHECK, 0, 0) == BST_CHECKED
}

func multidimEditorCreateControls(s *multidimShapeEditorState) {
	s.shapeLabel = multidimCreateChild(s, "STATIC", "도형", 0, 0)
	s.shapeCombo = multidimCreateChild(s, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|CBS_HASSTRINGS|CBS_NOINTEGRALHEIGHT, multidimIDShapeCombo)
	for _, spec := range multidimShapeSpecs {
		sendMessage(s.shapeCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(spec.name))))
	}
	// A190: The planar entries are the first group and must be visible without
	// scrolling upward from the former torus default. Ask the native combo to
	// expose the whole list when screen space permits, and start on 원판.
	sendMessage(s.shapeCombo, CB_SETMINVISIBLE, uintptr(len(multidimShapeSpecs)), 0)
	sendMessage(s.shapeCombo, CB_SETDROPPEDWIDTH, 360, 0)
	sendMessage(s.shapeCombo, CB_SETCURSEL, uintptr(multidimShapeIndexByName("원판")), 0)

	s.opacityLabel = multidimCreateChild(s, "STATIC", "면", 0, 0)
	s.opacityCombo = multidimCreateChild(s, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|CBS_HASSTRINGS|CBS_NOINTEGRALHEIGHT, multidimIDOpacityCombo)
	for _, name := range multidimOpacityModes {
		sendMessage(s.opacityCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(name))))
	}
	sendMessage(s.opacityCombo, CB_SETCURSEL, 1, 0) // A162: semi-transparent surface-layer fade default
	s.smoothCheck = multidimCreateChild(s, "BUTTON", "부드럽게", WS_TABSTOP|BS_AUTOCHECKBOX|BS_PUSHLIKE, multidimIDSmoothCheck)
	s.smoothEnabled = true
	sendMessage(s.smoothCheck, BM_SETCHECK, BST_CHECKED, 0)
	procSetWindowTextW.Call(s.smoothCheck, uintptr(unsafe.Pointer(utf16Ptr("부드럽게: 켬"))))
	s.invertCheck = multidimCreateChild(s, "BUTTON", "검은 배경", WS_TABSTOP|BS_AUTOCHECKBOX|BS_PUSHLIKE, multidimIDInvertCheck)
	// A231: dark mode starts with final preview RGB inversion enabled so the
	// rendered background and line contrast match the rest of the dark UI.
	invertState := uintptr(BST_UNCHECKED)
	if s.app != nil && s.app.darkMode {
		invertState = BST_CHECKED
	}
	sendMessage(s.invertCheck, BM_SETCHECK, invertState, 0)
	s.axesCheck = multidimCreateChild(s, "BUTTON", "xyz축 보이기", WS_TABSTOP|BS_AUTOCHECKBOX|BS_PUSHLIKE, multidimIDAxesCheck)
	sendMessage(s.axesCheck, BM_SETCHECK, BST_UNCHECKED, 0)
	s.moveAxesCheck = multidimCreateChild(s, "BUTTON", "축 옮기기", WS_TABSTOP|BS_AUTOCHECKBOX|BS_PUSHLIKE, multidimIDMoveAxesCheck)
	sendMessage(s.moveAxesCheck, BM_SETCHECK, BST_UNCHECKED, 0)
	s.axisScaleLabel = multidimCreateChild(s, "STATIC", "축 크기", 0, 0)
	s.axisScaleSlider = multidimCreateChild(s, "msctls_trackbar32", "", WS_TABSTOP|TBS_HORZ|TBS_AUTOTICKS, multidimIDAxisScaleSlider)
	s.axisScaleValue = multidimCreateChild(s, "STATIC", "", 0, 0)
	s.axisResetBtn = multidimCreateChild(s, "BUTTON", "축 제자리", WS_TABSTOP, multidimIDAxisReset)
	s.shapeResetBtn = multidimCreateChild(s, "BUTTON", "도형 제자리", WS_TABSTOP, multidimIDShapeReset)
	s.optionLabel = multidimCreateChild(s, "STATIC", "옵션:", 0, 0)
	s.optionCombo = multidimCreateChild(s, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|CBS_HASSTRINGS|CBS_NOINTEGRALHEIGHT, multidimIDOptionCombo)
	multidimEditorConfigureOption(s, multidimEditorSelectedShape(s))

	s.previewBox = multidimCreateChild(s, multidimPreviewClassName, "", WS_BORDER, multidimIDPreview)
	s.statusLabel = multidimCreateChild(s, "STATIC", "", 0, 0)
	s.insertBtn = multidimCreateChild(s, "BUTTON", "삽입", WS_TABSTOP, multidimIDInsert)
	s.cancelBtn = multidimCreateChild(s, "BUTTON", "취소", WS_TABSTOP, multidimIDCancel)

	s.xLabel = multidimCreateChild(s, "STATIC", "x 회전", 0, 0)
	s.xSlider = multidimCreateChild(s, "msctls_trackbar32", "", WS_TABSTOP|TBS_HORZ|TBS_AUTOTICKS, multidimIDXSlider)
	s.xValueLabel = multidimCreateChild(s, "STATIC", "", 0, 0)
	s.yLabel = multidimCreateChild(s, "STATIC", "y 회전", 0, 0)
	s.ySlider = multidimCreateChild(s, "msctls_trackbar32", "", WS_TABSTOP|TBS_HORZ|TBS_AUTOTICKS, multidimIDYSlider)
	s.yValueLabel = multidimCreateChild(s, "STATIC", "", 0, 0)
	s.zLabel = multidimCreateChild(s, "STATIC", "z 회전", 0, 0)
	s.zSlider = multidimCreateChild(s, "msctls_trackbar32", "", WS_TABSTOP|TBS_HORZ|TBS_AUTOTICKS, multidimIDZSlider)
	s.zValueLabel = multidimCreateChild(s, "STATIC", "", 0, 0)
	s.wLabel = multidimCreateChild(s, "STATIC", "w 회전", 0, 0)
	s.wSlider = multidimCreateChild(s, "msctls_trackbar32", "", WS_TABSTOP|TBS_HORZ|TBS_AUTOTICKS, multidimIDWSlider)
	s.wValueLabel = multidimCreateChild(s, "STATIC", "", 0, 0)

	s.sizeLabel = multidimCreateChild(s, "STATIC", "크기", 0, 0)
	s.sizeSlider = multidimCreateChild(s, "msctls_trackbar32", "", WS_TABSTOP|TBS_HORZ|TBS_AUTOTICKS, multidimIDSizeSlider)
	s.sizeValueLabel = multidimCreateChild(s, "STATIC", "", 0, 0)
	s.widthLabel = multidimCreateChild(s, "STATIC", "가로", 0, 0)
	s.widthSlider = multidimCreateChild(s, "msctls_trackbar32", "", WS_TABSTOP|TBS_HORZ|TBS_AUTOTICKS, multidimIDWidthSlider)
	s.widthValueLabel = multidimCreateChild(s, "STATIC", "", 0, 0)
	s.heightLabel = multidimCreateChild(s, "STATIC", "세로", 0, 0)
	s.heightSlider = multidimCreateChild(s, "msctls_trackbar32", "", WS_TABSTOP|TBS_HORZ|TBS_AUTOTICKS, multidimIDHeightSlider)
	s.heightValueLabel = multidimCreateChild(s, "STATIC", "", 0, 0)
	s.depthLabel = multidimCreateChild(s, "STATIC", "높이", 0, 0)
	s.depthSlider = multidimCreateChild(s, "msctls_trackbar32", "", WS_TABSTOP|TBS_HORZ|TBS_AUTOTICKS, multidimIDDepthSlider)
	s.depthValueLabel = multidimCreateChild(s, "STATIC", "", 0, 0)

	for _, h := range []uintptr{s.xSlider, s.ySlider, s.zSlider, s.wSlider} {
		multidimSliderSet(h, 0, 628, 0)
	}
	multidimSliderSet(s.sizeSlider, 5, 500, 170)
	multidimSliderSet(s.widthSlider, 5, 500, 180)
	multidimSliderSet(s.heightSlider, 5, 500, 140)
	multidimSliderSet(s.depthSlider, 5, 500, 120)
	multidimSliderSet(s.axisScaleSlider, 10, 200, 100)
	multidimEditorApplyShapeDefaults(s)
	sendMessage(s.xSlider, TBM_SETPOS, 1, 61)
	sendMessage(s.ySlider, TBM_SETPOS, 1, 52)
	sendMessage(s.wSlider, TBM_SETPOS, 1, 0) // show the recognizable classical bottle first; W rotation reveals the lift
	multidimEditorRefreshSelection(s)
}

func multidimEditorSelectedIndex(s *multidimShapeEditorState) int {
	idx := int(sendMessage(s.shapeCombo, CB_GETCURSEL, 0, 0))
	if idx < 0 || idx >= len(multidimShapeSpecs) {
		idx = 0
	}
	return idx
}
func multidimEditorSelectedShape(s *multidimShapeEditorState) multidimShapeSpec {
	return multidimShapeSpecs[multidimEditorSelectedIndex(s)]
}

func multidimShapeIndexByName(name string) int {
	for i, spec := range multidimShapeSpecs {
		if spec.name == name {
			return i
		}
	}
	return 0
}

func multidimShapeUsesSides(spec multidimShapeSpec) bool { return spec.name == "다각뿔" }
func multidimShapeUsesSphereGuides(spec multidimShapeSpec) bool {
	return spec.name == "타원체"
}
func multidimShapeUsesSphereSkins(spec multidimShapeSpec) bool {
	return spec.name == "구"
}
func multidimShapeUsesKleinGrid(spec multidimShapeSpec) bool {
	return spec.name == "4차원 곡면: 클라인 병"
}
func multidimShapeUsesTorusGrid(spec multidimShapeSpec) bool {
	return spec.name == "원환면(토러스)"
}
func multidimShapeUsesMobiusGrid(spec multidimShapeSpec) bool {
	return spec.name == "뫼비우스 띠"
}
func multidimShapeUsesPlanarGrid(spec multidimShapeSpec) bool {
	return spec.name == "원판"
}
func multidimShapeUsesCoordinatePlane(spec multidimShapeSpec) bool {
	return spec.name == "좌표평면"
}
func multidimShapeAxesAttached(spec multidimShapeSpec) bool {
	switch spec.name {
	case "원판", "평면", "좌표평면":
		return true
	default:
		return false
	}
}

// multidimAxesFollowShape reports whether the displayed XYZ frame is the
// object's fully rotated local basis. Plane primitives are permanently bound
// to their frame. Every other shape also starts bound, but an explicit axis
// move detaches the frame so the overlay can continue to show the real Euler
// operation axes at the moved pivot.
func multidimAxesFollowShape(s *multidimShapeEditorState) bool {
	if s == nil {
		return false
	}
	if multidimShapeAxesAttached(multidimEditorSelectedShape(s)) {
		return true
	}
	return !s.axisDetached && !multidimEditorMoveAxesEnabled(s)
}

func multidimAxisCoincidesWithShapeOrigin(s *multidimShapeEditorState) bool {
	if s == nil {
		return false
	}
	shapeOrigin := multidimAxisObjectOffset(s)
	d := mdSub(s.axisOrigin, shapeOrigin)
	return math.Abs(d.x) <= 1e-7 && math.Abs(d.y) <= 1e-7 && math.Abs(d.z) <= 1e-7
}

func multidimRefreshAxisDetachedState(s *multidimShapeEditorState) {
	if s == nil {
		return
	}
	if multidimShapeAxesAttached(multidimEditorSelectedShape(s)) {
		s.axisDetached = false
		return
	}
	s.axisDetached = !multidimAxisCoincidesWithShapeOrigin(s)
}
func multidimShapeUsesQuadricFamily(spec multidimShapeSpec) bool {
	return spec.name == "포물면/쌍곡면"
}
func multidimShapeUsesSaddleGrid(spec multidimShapeSpec) bool {
	return spec.name == "말안장면"
}
func multidimShapeUsesConeCut(spec multidimShapeSpec) bool {
	return spec.name == "쌍원뿔과 원뿔곡선"
}
func multidimShapeUsesBlackHole(spec multidimShapeSpec) bool {
	return spec.name == "시공간: 블랙홀"
}
func multidimShapeUsesParametricSurfaceGrid(spec multidimShapeSpec) bool {
	return multidimShapeUsesTorusGrid(spec) || multidimShapeUsesMobiusGrid(spec)
}

func multidimShapeIsImplemented4D(spec multidimShapeSpec) bool {
	switch spec.name {
	case "정다포체: 5-셀", "정다포체: 테서렉트", "정다포체: 16-셀", "정다포체: 24-셀", "정다포체: 120-셀", "정다포체: 600-셀", "4차원 곡면: 클라인 병":
		return true
	default:
		return false
	}
}

func multidimEditorResetOptionItems(s *multidimShapeEditorState, items []string, selected int) {
	if s == nil || s.optionCombo == 0 {
		return
	}
	sendMessage(s.optionCombo, multidimCBResetContent, 0, 0)
	for _, item := range items {
		sendMessage(s.optionCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(item))))
	}
	if selected < 0 || selected >= len(items) {
		selected = 0
	}
	if len(items) > 0 {
		sendMessage(s.optionCombo, CB_SETCURSEL, uintptr(selected), 0)
	}
}

func multidimEditorConfigureOption(s *multidimShapeEditorState, spec multidimShapeSpec) {
	if s == nil {
		return
	}
	multidimSetText(s.optionLabel, "옵션:")
	switch {
	case multidimShapeUsesSides(spec):
		items := make([]string, 0, 9)
		for sides := 4; sides <= 12; sides++ {
			items = append(items, fmt.Sprintf("%d각", sides))
		}
		if s.polygonSides < 4 || s.polygonSides > 12 {
			s.polygonSides = 4
		}
		multidimEditorResetOptionItems(s, items, s.polygonSides-4)
		multidimEditorSetOptionVisible(s, true)
	case multidimShapeUsesSphereSkins(spec):
		if s.sphereSkin < mdSphereSkinPlain || s.sphereSkin > mdSphereSkinSaturn {
			s.sphereSkin = mdSphereSkinPlain
		}
		if s.sphereGuides < mdSphereGuidesEquator || s.sphereGuides > mdSphereGuidesNone {
			s.sphereGuides = mdSphereGuidesThreeAxes
		}
		selected := s.sphereGuides
		if s.sphereSkin > mdSphereSkinPlain {
			selected = 2 + s.sphereSkin
		}
		multidimSetText(s.optionLabel, "표면:")
		multidimEditorResetOptionItems(s, []string{"기본 구 · 적도만", "기본 구 · 3축", "기본 구 · 안내선 없음", "지구 · NASA Blue Marble", "달 · NASA LRO", "토성 · Cassini 전구면 + 고리"}, selected)
		multidimEditorSetOptionVisible(s, true)
	case multidimShapeUsesSphereGuides(spec):
		if s.sphereGuides < mdSphereGuidesEquator || s.sphereGuides > mdSphereGuidesNone {
			s.sphereGuides = mdSphereGuidesThreeAxes
		}
		multidimEditorResetOptionItems(s, []string{"적도만", "3축", "없음"}, s.sphereGuides)
		multidimEditorSetOptionVisible(s, true)
	case multidimShapeUsesTorusGrid(spec):
		if s.torusGrid < mdSurfaceGridNone || s.torusGrid > mdSurfaceGridFull {
			s.torusGrid = mdSurfaceGridMajor
		}
		multidimEditorResetOptionItems(s, []string{"격자선 없음", "주요 격자선", "전체 격자선"}, s.torusGrid)
		multidimEditorSetOptionVisible(s, true)
	case multidimShapeUsesMobiusGrid(spec):
		if s.mobiusGrid < mdSurfaceGridNone || s.mobiusGrid > mdSurfaceGridFull {
			s.mobiusGrid = mdSurfaceGridMajor
		}
		multidimEditorResetOptionItems(s, []string{"격자선 없음", "주요 격자선", "전체 격자선"}, s.mobiusGrid)
		multidimEditorSetOptionVisible(s, true)
	case multidimShapeUsesKleinGrid(spec):
		if s.kleinGrid < mdKleinGridNone || s.kleinGrid > mdKleinGridFull {
			s.kleinGrid = mdKleinGridMajor
		}
		multidimEditorResetOptionItems(s, []string{"격자선 없음", "주요 격자선", "전체 격자선"}, s.kleinGrid)
		multidimEditorSetOptionVisible(s, true)
	case multidimShapeUsesPlanarGrid(spec):
		if s.planarGrid < mdPlanarGridNone || s.planarGrid > mdPlanarGridFull {
			s.planarGrid = mdPlanarGridMajor
		}
		multidimSetText(s.optionLabel, "격자:")
		multidimEditorResetOptionItems(s, []string{"격자 없음", "주요 극좌표 격자", "전체 극좌표 격자"}, s.planarGrid)
		multidimEditorSetOptionVisible(s, true)
	case multidimShapeUsesCoordinatePlane(spec):
		// A191: 좌표평면은 한 장을 고르는 옵션이 아니라 XY·XZ·YZ 세 장을
		// 동시에 표시하는 고정 도형이다.
		multidimEditorSetOptionVisible(s, false)
	case multidimShapeUsesSaddleGrid(spec):
		if s.saddleGrid < mdPlanarGridNone || s.saddleGrid > mdPlanarGridFull {
			s.saddleGrid = mdPlanarGridMajor
		}
		multidimSetText(s.optionLabel, "격자:")
		multidimEditorResetOptionItems(s, []string{"격자 없음", "주요 곡선 격자", "전체 곡선 격자"}, s.saddleGrid)
		multidimEditorSetOptionVisible(s, true)
	case multidimShapeUsesQuadricFamily(spec):
		if s.quadricFamily < mdQuadricOneSheet || s.quadricFamily > mdQuadricTwoSheet {
			s.quadricFamily = mdQuadricOneSheet
		}
		multidimSetText(s.optionLabel, "계열:")
		multidimEditorResetOptionItems(s, []string{"e=1 포물면 · e>1 일엽 쌍곡면", "e=1 포물면 · e>1 이엽 쌍곡면"}, s.quadricFamily)
		multidimEditorSetOptionVisible(s, true)
	case multidimShapeUsesBlackHole(spec):
		if s.blackHoleMode < mdBlackHoleModeCurvatureGrid || s.blackHoleMode > mdBlackHoleModeGeodesics {
			s.blackHoleMode = mdBlackHoleModeCurvatureGrid
		}
		multidimSetText(s.optionLabel, "표시:")
		multidimEditorResetOptionItems(s, []string{
			"3차원 곡률 좌표망",
			"실제 블랙홀 · 강착원반+별",
			"실제 모습 + 측지선",
			"측지선 도해",
		}, s.blackHoleMode)
		multidimEditorSetOptionVisible(s, true)
	case multidimShapeUsesConeCut(spec):
		if s.coneCutMode < mdConeCutHorizontal || s.coneCutMode > mdConeCutThroughApex {
			s.coneCutMode = mdConeCutHorizontal
		}
		multidimSetText(s.optionLabel, "절단:")
		multidimEditorResetOptionItems(s, []string{"가로 자르기 · 원", "그냥 자르기 · 타원", "빗면 평행 · 포물선", "세로 자르기 · 쌍곡선", "꼭짓점 통과 · 두 직선"}, s.coneCutMode)
		multidimEditorSetOptionVisible(s, true)
	default:
		multidimEditorSetOptionVisible(s, false)
	}
}

func multidimEditorCommitOption(s *multidimShapeEditorState) {
	if s == nil || s.optionCombo == 0 {
		return
	}
	idx := int(sendMessage(s.optionCombo, CB_GETCURSEL, 0, 0))
	spec := multidimEditorSelectedShape(s)
	switch {
	case multidimShapeUsesSides(spec):
		if idx < 0 || idx > 8 {
			idx = 0
		}
		s.polygonSides = idx + 4
	case multidimShapeUsesSphereSkins(spec):
		if idx >= 0 && idx <= 2 {
			s.sphereSkin = mdSphereSkinPlain
			s.sphereGuides = idx
		} else if idx >= 3 && idx <= 5 {
			s.sphereSkin = idx - 2
			s.sphereGuides = mdSphereGuidesNone
		} else {
			s.sphereSkin = mdSphereSkinPlain
			s.sphereGuides = mdSphereGuidesThreeAxes
		}
	case multidimShapeUsesSphereGuides(spec):
		if idx < mdSphereGuidesEquator || idx > mdSphereGuidesNone {
			idx = mdSphereGuidesThreeAxes
		}
		s.sphereGuides = idx
	case multidimShapeUsesTorusGrid(spec):
		if idx < mdSurfaceGridNone || idx > mdSurfaceGridFull {
			idx = mdSurfaceGridMajor
		}
		s.torusGrid = idx
	case multidimShapeUsesMobiusGrid(spec):
		if idx < mdSurfaceGridNone || idx > mdSurfaceGridFull {
			idx = mdSurfaceGridMajor
		}
		s.mobiusGrid = idx
	case multidimShapeUsesKleinGrid(spec):
		if idx < mdKleinGridNone || idx > mdKleinGridFull {
			idx = mdKleinGridMajor
		}
		s.kleinGrid = idx
	case multidimShapeUsesPlanarGrid(spec):
		if idx < mdPlanarGridNone || idx > mdPlanarGridFull {
			idx = mdPlanarGridMajor
		}
		s.planarGrid = idx
	case multidimShapeUsesCoordinatePlane(spec):
		// 고정된 3면 좌표평면이므로 선택 옵션이 없다.
	case multidimShapeUsesSaddleGrid(spec):
		if idx < mdPlanarGridNone || idx > mdPlanarGridFull {
			idx = mdPlanarGridMajor
		}
		s.saddleGrid = idx
	case multidimShapeUsesQuadricFamily(spec):
		if idx < mdQuadricOneSheet || idx > mdQuadricTwoSheet {
			idx = mdQuadricOneSheet
		}
		s.quadricFamily = idx
	case multidimShapeUsesBlackHole(spec):
		if idx < mdBlackHoleModeCurvatureGrid || idx > mdBlackHoleModeGeodesics {
			idx = mdBlackHoleModeCurvatureGrid
		}
		s.blackHoleMode = idx
		s.blackHoleCacheKey = 0
	case multidimShapeUsesConeCut(spec):
		if idx < mdConeCutHorizontal || idx > mdConeCutThroughApex {
			idx = mdConeCutHorizontal
		}
		s.coneCutMode = idx
	}
}

func multidimEditorPolygonSides(s *multidimShapeEditorState) int {
	if s == nil || s.polygonSides < 4 || s.polygonSides > 12 {
		return 4
	}
	return s.polygonSides
}

func multidimEditorSphereGuideMode(s *multidimShapeEditorState) int {
	if s == nil || s.sphereGuides < mdSphereGuidesEquator || s.sphereGuides > mdSphereGuidesNone {
		return mdSphereGuidesThreeAxes
	}
	return s.sphereGuides
}

func multidimEditorSphereSkinMode(s *multidimShapeEditorState) int {
	if s == nil || s.sphereSkin < mdSphereSkinPlain || s.sphereSkin > mdSphereSkinSaturn {
		return mdSphereSkinPlain
	}
	return s.sphereSkin
}

func multidimEditorKleinGridMode(s *multidimShapeEditorState) int {
	if s == nil || s.kleinGrid < mdKleinGridNone || s.kleinGrid > mdKleinGridFull {
		return mdKleinGridMajor
	}
	return s.kleinGrid
}

func multidimEditorTorusGridMode(s *multidimShapeEditorState) int {
	if s == nil || s.torusGrid < mdSurfaceGridNone || s.torusGrid > mdSurfaceGridFull {
		return mdSurfaceGridMajor
	}
	return s.torusGrid
}

func multidimEditorMobiusGridMode(s *multidimShapeEditorState) int {
	if s == nil || s.mobiusGrid < mdSurfaceGridNone || s.mobiusGrid > mdSurfaceGridFull {
		return mdSurfaceGridMajor
	}
	return s.mobiusGrid
}

func multidimEditorSetOptionVisible(s *multidimShapeEditorState, visible bool) {
	if s == nil {
		return
	}
	show := uintptr(SW_HIDE)
	enable := uintptr(0)
	if visible {
		show = SW_SHOW
		enable = 1
	}
	procShowWindow.Call(s.optionLabel, show)
	procShowWindow.Call(s.optionCombo, show)
	procEnableWindow.Call(s.optionLabel, enable)
	procEnableWindow.Call(s.optionCombo, enable)
}

func multidimEditorApplyShapeDefaults(s *multidimShapeEditorState) {
	spec := multidimEditorSelectedShape(s)
	multidimSetText(s.sizeLabel, "크기")
	multidimSetText(s.widthLabel, "가로")
	multidimSetText(s.heightLabel, "세로")
	multidimSetText(s.depthLabel, "높이")

	sizeMin, sizeMax, sizePos := 5, 500, spec.defaultSize
	widthMin, widthMax, widthPos := 5, 500, spec.defaultWidth
	heightMin, heightMax, heightPos := 5, 500, spec.defaultHeight
	depthMin, depthMax, depthPos := 5, 500, spec.defaultDepth

	switch {
	case multidimShapeUsesBlackHole(spec):
		multidimSetText(s.sizeLabel, "블랙홀과 거리")
		multidimSetText(s.widthLabel, "좌표망 밀도")
		multidimSetText(s.heightLabel, "커 회전량 a*")
		sizeMin, sizeMax = 5, 500
		widthMin, widthMax, widthPos = 1, 8, 2 // 3,5,...17 lines per axis; 2 => legacy 5 lines
		heightMin, heightMax, heightPos = 0, 99, 0
		if s.blackHoleMode < mdBlackHoleModeCurvatureGrid || s.blackHoleMode > mdBlackHoleModeGeodesics {
			s.blackHoleMode = mdBlackHoleModeCurvatureGrid
		}
	case multidimShapeUsesPlanarGrid(spec):
		sizeMin, sizeMax = 20, 500
		if s.planarGrid < mdPlanarGridNone || s.planarGrid > mdPlanarGridFull {
			s.planarGrid = mdPlanarGridMajor
		}
	case spec.name == "평면":
		widthMin, widthMax = 20, 500
		heightMin, heightMax = 20, 500
	case multidimShapeUsesCoordinatePlane(spec):
		multidimSetText(s.widthLabel, "범위")
		multidimSetText(s.heightLabel, "분할")
		multidimSetText(s.depthLabel, "굵은 간격")
		widthMin, widthMax = 80, 400
		heightMin, heightMax = 2, 20
		depthMin, depthMax = 2, 10
	case multidimShapeUsesSaddleGrid(spec):
		multidimSetText(s.widthLabel, "x 반폭")
		multidimSetText(s.heightLabel, "y 반폭")
		multidimSetText(s.depthLabel, "굽힘 높이")
		widthMin, widthMax = 50, 320
		heightMin, heightMax = 50, 320
		depthMin, depthMax = 20, 260
		if s.saddleGrid < mdPlanarGridNone || s.saddleGrid > mdPlanarGridFull {
			s.saddleGrid = mdPlanarGridMajor
		}
	case multidimShapeUsesQuadricFamily(spec):
		multidimSetText(s.widthLabel, "이심률")
		multidimSetText(s.heightLabel, "반지름")
		multidimSetText(s.depthLabel, "높이")
		widthMin, widthMax = 100, 300
		heightMin, heightMax = 60, 300
		depthMin, depthMax = 60, 300
		if s.quadricFamily < mdQuadricOneSheet || s.quadricFamily > mdQuadricTwoSheet {
			s.quadricFamily = mdQuadricOneSheet
		}
	case multidimShapeUsesConeCut(spec):
		multidimSetText(s.widthLabel, "반지름")
		multidimSetText(s.heightLabel, "반높이")
		multidimSetText(s.depthLabel, "절단 위치")
		widthMin, widthMax = 60, 300
		heightMin, heightMax = 60, 300
		depthMin, depthMax = -90, 90
		if s.coneCutMode < mdConeCutHorizontal || s.coneCutMode > mdConeCutThroughApex {
			s.coneCutMode = mdConeCutHorizontal
		}
	case multidimShapeUsesSides(spec):
		depthPos = 120
	case multidimShapeUsesTorusGrid(spec):
		multidimSetText(s.widthLabel, "큰 반지름")
		multidimSetText(s.heightLabel, "관 반지름")
		multidimSetText(s.depthLabel, "분할")
		depthMin, depthMax = 16, 96
		if s.torusGrid < mdSurfaceGridNone || s.torusGrid > mdSurfaceGridFull {
			s.torusGrid = mdSurfaceGridMajor
		}
	case multidimShapeUsesMobiusGrid(spec):
		multidimSetText(s.widthLabel, "중심 반지름")
		multidimSetText(s.heightLabel, "띠 반폭")
		multidimSetText(s.depthLabel, "분할")
		depthMin, depthMax = 16, 96
		if s.mobiusGrid < mdSurfaceGridNone || s.mobiusGrid > mdSurfaceGridFull {
			s.mobiusGrid = mdSurfaceGridMajor
		}
	case multidimShapeUsesKleinGrid(spec):
		multidimSetText(s.widthLabel, "몸통")
		multidimSetText(s.heightLabel, "관")
		multidimSetText(s.depthLabel, "분할")
		depthMin, depthMax = mdKleinSubdivisionMin, mdKleinSubdivisionMax
		if s.kleinGrid < mdKleinGridNone || s.kleinGrid > mdKleinGridFull {
			s.kleinGrid = mdKleinGridMajor
		}
	}

	multidimSliderSet(s.sizeSlider, sizeMin, sizeMax, sizePos)
	multidimSliderSet(s.widthSlider, widthMin, widthMax, widthPos)
	multidimSliderSet(s.heightSlider, heightMin, heightMax, heightPos)
	multidimSliderSet(s.depthSlider, depthMin, depthMax, depthPos)
	if multidimShapeUsesSides(spec) && (s.polygonSides < 4 || s.polygonSides > 12) {
		s.polygonSides = 4
	}
	multidimEditorConfigureOption(s, spec)
}

func multidimEditorSetDimensionEnabled(label, slider, value uintptr, enabled bool) {
	var flag uintptr
	if enabled {
		flag = 1
	}
	procEnableWindow.Call(label, flag)
	procEnableWindow.Call(slider, flag)
	procEnableWindow.Call(value, flag)
}

func multidimEditorSetAxisControlsVisible(s *multidimShapeEditorState, visible bool) {
	if s == nil {
		return
	}
	show := uintptr(SW_HIDE)
	enable := uintptr(0)
	if visible {
		show = SW_SHOW
		enable = 1
	}
	for _, h := range []uintptr{s.axisScaleLabel, s.axisScaleSlider, s.axisScaleValue, s.axisResetBtn, s.shapeResetBtn} {
		procShowWindow.Call(h, show)
		procEnableWindow.Call(h, enable)
	}
}

func multidimResetAxisOrigin(s *multidimShapeEditorState) {
	if s == nil {
		return
	}
	spec := multidimEditorSelectedShape(s)
	if multidimShapeAxesAttached(spec) {
		// 평면류는 축과 도형 중심이 한 묶음이다. 축을 중앙으로 돌리면
		// 도형도 함께 중앙으로 돌아온다.
		s.axisOrigin = mdVec3{}
		s.axisPivotLocal3 = mdVec3{}
		s.axisPivotLocal4 = mdVec4{}
		multidimEditorRefreshValueLabels(s)
		multidimInvalidatePreview(s)
		multidimSetText(s.statusLabel, "축 제자리 완료 · 평면 도형과 붙은 xyz축을 함께 중앙으로 되돌렸습니다.")
		return
	}
	currentOffset := multidimAxisObjectOffset(s)
	multidimRebaseAxisPivot(s, mdVec3{}, currentOffset)
	multidimRefreshAxisDetachedState(s)
	multidimEditorRefreshValueLabels(s)
	multidimInvalidatePreview(s)
	multidimSetText(s.statusLabel, "축 제자리 완료 · 도형은 그대로 두고 X/Y/Z/W 회전 원점을 중앙으로 되돌렸습니다.")
}

func multidimResetShapeOrigin(s *multidimShapeEditorState) {
	if s == nil {
		return
	}
	spec := multidimEditorSelectedShape(s)
	if multidimShapeAxesAttached(spec) {
		// 붙어 있는 평면류는 도형과 축을 서로 분리할 수 없다.
		s.axisOrigin = mdVec3{}
		s.axisPivotLocal3 = mdVec3{}
		s.axisPivotLocal4 = mdVec4{}
		multidimEditorRefreshValueLabels(s)
		multidimInvalidatePreview(s)
		multidimSetText(s.statusLabel, "도형 제자리 완료 · 평면 도형과 붙은 xyz축을 함께 중앙으로 되돌렸습니다.")
		return
	}
	// Zero object translation while preserving the moved rotation axis. The
	// local pivot is solved so its current projection equals axisOrigin.
	multidimAxisSetLocalPivotForTarget(s, s.axisOrigin)
	multidimRefreshAxisDetachedState(s)
	multidimEditorRefreshValueLabels(s)
	multidimInvalidatePreview(s)
	multidimSetText(s.statusLabel, "도형 제자리 완료 · 축 위치와 회전값은 유지하고 도형만 미리보기 중앙으로 되돌렸습니다.")
}

func multidimEditorRefreshSelection(s *multidimShapeEditorState) {
	if s == nil {
		return
	}
	spec := multidimEditorSelectedShape(s)
	if multidimShapeAxesAttached(spec) {
		multidimSetText(s.moveAxesCheck, "도형+축 옮기기")
	} else {
		multidimSetText(s.moveAxesCheck, "축 옮기기")
	}
	isBlackHole := multidimShapeUsesBlackHole(spec)
	var opacityEnable uintptr = 1
	if isBlackHole {
		opacityEnable = 0
	}
	procEnableWindow.Call(s.opacityLabel, opacityEnable)
	procEnableWindow.Call(s.opacityCombo, opacityEnable)
	axesVisible := multidimEditorAxesEnabled(s)
	multidimEditorSetAxisControlsVisible(s, axesVisible)
	if axesVisible {
		procEnableWindow.Call(s.moveAxesCheck, 1)
	} else {
		procEnableWindow.Call(s.moveAxesCheck, 0)
		if sendMessage(s.moveAxesCheck, BM_GETCHECK, 0, 0) == BST_CHECKED {
			sendMessage(s.moveAxesCheck, BM_SETCHECK, BST_UNCHECKED, 0)
		}
		if s.previewAxisDragging {
			multidimEndPreviewAxisMove(s, s.previewBox, true)
		}
	}
	multidimEditorRefreshValueLabels(s)

	wEnabled := spec.is4D
	if multidimShapeUsesBlackHole(spec) {
		wEnabled = false
	}
	multidimEditorSetDimensionEnabled(s.wLabel, s.wSlider, s.wValueLabel, wEnabled)
	if isBlackHole {
		// Density and Kerr spin are educational controls for the 3-D curvature
		// lattice only. The photorealistic view keeps its established redshift
		// rendering and does not inherit a hidden spin value from another mode.
		curvatureControls := multidimEditorBlackHoleMode(s) == mdBlackHoleModeCurvatureGrid
		multidimEditorSetDimensionEnabled(s.widthLabel, s.widthSlider, s.widthValueLabel, curvatureControls)
		multidimEditorSetDimensionEnabled(s.heightLabel, s.heightSlider, s.heightValueLabel, curvatureControls)
		multidimEditorSetDimensionEnabled(s.depthLabel, s.depthSlider, s.depthValueLabel, false)
	} else {
		multidimEditorSetDimensionEnabled(s.widthLabel, s.widthSlider, s.widthValueLabel, spec.useWidth)
		multidimEditorSetDimensionEnabled(s.heightLabel, s.heightSlider, s.heightValueLabel, spec.useHeight)
		multidimEditorSetDimensionEnabled(s.depthLabel, s.depthSlider, s.depthValueLabel, spec.useDepth)
	}
	multidimEditorConfigureOption(s, spec)
	useDim := spec.useWidth || spec.useHeight || spec.useDepth
	multidimEditorSetDimensionEnabled(s.sizeLabel, s.sizeSlider, s.sizeValueLabel, !useDim)

	// The former read-only status/description panes are intentionally removed.
	// Keep only the concise bottom status and the functional preview controls.
	if multidimShapeUsesParametricSurfaceGrid(spec) {
		multidimSetText(s.statusLabel, "A170: 3D는 면당 70% 감쇠(30% 잔존) · 보이는 실루엣은 진하게 · 가려진 접선은 불투명 생략")
	}
	if multidimShapeIsImplemented4D(spec) {
		multidimSetText(s.statusLabel, "A161: 외곽선 우선 · 정면 가림 면 수 n · 불투명 n=0 · max(8%, 0.70^n).")
	} else if multidimShapeUsesParametricSurfaceGrid(spec) {
		// Status was set above with the A170 surface-layer rule.
	} else if spec.name == "원뿔" || spec.name == "다각뿔" {
		multidimSetText(s.statusLabel, "A161: 외곽선 우선 · 정면 가림 면 수 기반 모서리 판정")
	} else if multidimShapeUsesSaddleGrid(spec) {
		multidimSetText(s.statusLabel, "A196: 말안장면 · 반대 곡률 쌍곡포물면 · 실제 캔버스 삽입 지원")
	} else if multidimShapeUsesBlackHole(spec) {
		multidimSetText(s.statusLabel, "A203: 지구 좌우 방향 수정 · 한반도 기본 시점 · 울릉도/독도 최소 픽셀 보존")
	} else if multidimShapeUsesConeCut(spec) {
		multidimSetText(s.statusLabel, "A190: 절단 위치 -90%~+90% · 쌍원뿔 면 조명 안정화 유지")
	} else if multidimShapeUsesQuadricFamily(spec) {
		multidimSetText(s.statusLabel, "A190: 포물면/쌍곡면 메뉴 표시 · e=1 포물면 · e>1 쌍곡면")
	} else if spec.name == "원판" || spec.name == "평면" || spec.name == "좌표평면" {
		multidimSetText(s.statusLabel, "A192: 모든 도형의 기본 xyz축은 로컬 좌표계에 결합 · 축 옮기기 시 독립 피벗")
	} else {
		multidimSetText(s.statusLabel, "도형, 면 모드, 옵션, 회전과 크기를 조절하십시오.")
	}
	multidimInvalidatePreview(s)
}

func multidimEditorLayoutControls(s *multidimShapeEditorState, client RECT) {
	if s == nil {
		return
	}
	width := client.Right - client.Left
	height := client.Bottom - client.Top
	if width < 720 {
		width = 720
	}
	if height < 540 {
		height = 540
	}
	margin := int32(10)
	topRowY := int32(10)
	shapeLabelW := int32(32)
	shapeComboW := int32(300)
	opacityX := margin + shapeLabelW + shapeComboW + 12
	opacityLabelW := int32(24)
	opacityComboW := int32(100)
	smoothX := opacityX + opacityLabelW + opacityComboW + 14
	smoothW := int32(82)
	invertX := smoothX + smoothW + 8
	invertW := int32(82)
	axesX := invertX + invertW + 8
	axesW := int32(104)
	moveAxesX := axesX + axesW + 8
	moveAxesW := int32(90)
	optionLabelW := int32(48)
	optionComboW := int32(220)
	sidesX := width - margin - optionLabelW - optionComboW
	minSidesX := moveAxesX + moveAxesW + 18
	if sidesX < minSidesX {
		sidesX = minSidesX
	}
	contentTop := topRowY + 36
	contentBottomMargin := int32(182)
	if multidimEditorAxesEnabled(s) {
		contentBottomMargin = 234
	}
	contentH := height - contentTop - contentBottomMargin
	if contentH < 260 {
		contentH = 260
	}
	// Use the full content width for the preview after removing the two
	// redundant explanatory text panes.  No blank sidebar is reserved.
	previewX := margin
	previewW := width - 2*margin
	if previewW < 300 {
		previewW = 300
	}
	multidimMoveWindow(s.shapeLabel, margin, topRowY+4, shapeLabelW, 20, true)
	multidimMoveWindow(s.shapeCombo, margin+shapeLabelW, topRowY, shapeComboW, 620, true)
	multidimMoveWindow(s.opacityLabel, opacityX, topRowY+4, opacityLabelW, 20, true)
	multidimMoveWindow(s.opacityCombo, opacityX+opacityLabelW, topRowY, opacityComboW, 180, true)
	multidimMoveWindow(s.smoothCheck, smoothX, topRowY+1, smoothW, 24, true)
	multidimMoveWindow(s.invertCheck, invertX, topRowY+1, invertW, 24, true)
	multidimMoveWindow(s.axesCheck, axesX, topRowY+1, axesW, 24, true)
	multidimMoveWindow(s.moveAxesCheck, moveAxesX, topRowY+1, moveAxesW, 24, true)
	multidimMoveWindow(s.optionLabel, sidesX, topRowY+4, optionLabelW, 20, true)
	multidimMoveWindow(s.optionCombo, sidesX+optionLabelW, topRowY, optionComboW, 220, true)
	multidimMoveWindow(s.previewBox, previewX, contentTop, previewW, contentH, true)

	bottomTop := contentTop + contentH + 16
	// Four complete slider groups must fit the client width.  In particular,
	// the fourth label is "w 회전" (the multidimensional W rotation), not a
	// one-letter axis caption, so reserve enough width for the full Korean text.
	labelW := int32(44)
	valueW := int32(60)
	gap := int32(6)
	columnGap := int32(12)
	columnW := (width - margin*2 - columnGap*3) / 4
	minColumnW := labelW + gap + 100 + gap + valueW
	if columnW < minColumnW {
		columnW = minColumnW
	}
	sliderW := columnW - labelW - gap - gap - valueW
	col1X := margin
	col2X := col1X + columnW + columnGap
	col3X := col2X + columnW + columnGap
	col4X := col3X + columnW + columnGap
	row1Y := bottomTop
	row2Y := bottomTop + 52
	row3Y := bottomTop + 104
	layoutSlider := func(label, slider, value uintptr, x, rowY int32) {
		multidimMoveWindow(label, x, rowY+8, labelW, 20, true)
		multidimMoveWindow(slider, x+labelW+gap, rowY+1, sliderW, 30, true)
		multidimMoveWindow(value, x+labelW+gap+sliderW+gap, rowY+8, valueW, 20, true)
	}
	layoutSlider(s.xLabel, s.xSlider, s.xValueLabel, col1X, row1Y)
	layoutSlider(s.yLabel, s.ySlider, s.yValueLabel, col2X, row1Y)
	layoutSlider(s.zLabel, s.zSlider, s.zValueLabel, col3X, row1Y)
	layoutSlider(s.wLabel, s.wSlider, s.wValueLabel, col4X, row1Y)
	layoutSlider(s.sizeLabel, s.sizeSlider, s.sizeValueLabel, col1X, row2Y)
	layoutSlider(s.widthLabel, s.widthSlider, s.widthValueLabel, col2X, row2Y)
	layoutSlider(s.heightLabel, s.heightSlider, s.heightValueLabel, col3X, row2Y)
	layoutSlider(s.depthLabel, s.depthSlider, s.depthValueLabel, col4X, row2Y)
	axisLabelW := int32(54)
	axisSliderW := int32(260)
	axisValueW := int32(56)
	multidimMoveWindow(s.axisScaleLabel, col1X, row3Y+8, axisLabelW, 20, true)
	multidimMoveWindow(s.axisScaleSlider, col1X+axisLabelW+gap, row3Y+1, axisSliderW, 30, true)
	multidimMoveWindow(s.axisScaleValue, col1X+axisLabelW+gap+axisSliderW+gap, row3Y+8, axisValueW, 20, true)
	axisResetX := col1X + axisLabelW + gap + axisSliderW + gap + axisValueW + 14
	multidimMoveWindow(s.axisResetBtn, axisResetX, row3Y+3, 88, 26, true)
	multidimMoveWindow(s.shapeResetBtn, axisResetX+96, row3Y+3, 96, 26, true)
	buttonY := height - 42
	buttonW := int32(70)
	buttonH := int32(26)
	buttonGap := int32(8)
	multidimMoveWindow(s.cancelBtn, width-margin-buttonW, buttonY, buttonW, buttonH, true)
	multidimMoveWindow(s.insertBtn, width-margin-buttonW*2-buttonGap, buttonY, buttonW, buttonH, true)
	statusW := width - margin - (buttonW*2 + buttonGap + 24)
	if statusW < 200 {
		statusW = 200
	}
	multidimMoveWindow(s.statusLabel, margin, buttonY+5, statusW, 20, true)
}

func multidimEditorRefreshValueLabels(s *multidimShapeEditorState) {
	if s == nil {
		return
	}
	multidimSetText(s.xValueLabel, multidimRadiansText(s.xSlider))
	multidimSetText(s.yValueLabel, multidimRadiansText(s.ySlider))
	multidimSetText(s.zValueLabel, multidimRadiansText(s.zSlider))
	multidimSetText(s.wValueLabel, multidimRadiansText(s.wSlider))
	multidimSetText(s.sizeValueLabel, multidimLengthText(s.sizeSlider))
	multidimSetText(s.widthValueLabel, multidimLengthText(s.widthSlider))
	multidimSetText(s.heightValueLabel, multidimLengthText(s.heightSlider))
	multidimSetText(s.depthValueLabel, multidimLengthText(s.depthSlider))
	multidimSetText(s.axisScaleValue, multidimAxisScaleText(s.axisScaleSlider))

	spec := multidimEditorSelectedShape(s)
	if multidimShapeUsesBlackHole(spec) {
		multidimSetText(s.sizeValueLabel, fmt.Sprintf("%.1fM", multidimBlackHoleDistance(s)))
		if multidimEditorBlackHoleMode(s) == mdBlackHoleModeCurvatureGrid {
			multidimSetText(s.widthValueLabel, fmt.Sprintf("%d줄/축", multidimBlackHoleGridLines(s)))
			multidimSetText(s.heightValueLabel, fmt.Sprintf("a*=%.2f", multidimBlackHoleSpin(s)))
		} else {
			multidimSetText(s.widthValueLabel, "사용 안 함")
			multidimSetText(s.heightValueLabel, "고정")
		}
	}
	if multidimShapeUsesQuadricFamily(spec) {
		multidimSetText(s.widthValueLabel, fmt.Sprintf("e=%.2f", float64(multidimSliderPos(s.widthSlider))/100.0))
	}
	if multidimShapeUsesConeCut(spec) {
		multidimSetText(s.depthValueLabel, fmt.Sprintf("%d%%", multidimSliderPos(s.depthSlider)))
	}
	if multidimShapeUsesCoordinatePlane(spec) {
		multidimSetText(s.widthValueLabel, fmt.Sprintf("±%d", multidimSliderPos(s.widthSlider)))
		multidimSetText(s.heightValueLabel, fmt.Sprintf("%d칸", multidimSliderPos(s.heightSlider)))
		multidimSetText(s.depthValueLabel, fmt.Sprintf("%d칸", multidimSliderPos(s.depthSlider)))
	}
}

func multidimInvalidatePreview(s *multidimShapeEditorState) {
	if s == nil || s.previewBox == 0 {
		return
	}
	s.previewDirty = true
	// Do not ask Windows to erase the child background. The completed frame is
	// copied from the persistent back buffer in one operation.
	procInvalidateRect.Call(s.previewBox, 0, 0)
}

func multidimReleaseBitmapBuffer(dc, bitmap, oldBitmap *uintptr, width, height *int) {
	if *dc != 0 && *oldBitmap != 0 {
		procSelectObject.Call(*dc, *oldBitmap)
	}
	if *bitmap != 0 {
		procDeleteObject.Call(*bitmap)
	}
	if *dc != 0 {
		procDeleteDC.Call(*dc)
	}
	*dc, *bitmap, *oldBitmap = 0, 0, 0
	*width, *height = 0, 0
}

func multidimEnsureBitmapBuffer(referenceHDC uintptr, width, height int, dc, bitmap, oldBitmap *uintptr, bufferW, bufferH *int) bool {
	if width <= 0 || height <= 0 || referenceHDC == 0 {
		return false
	}
	if *dc != 0 && *bitmap != 0 && *bufferW == width && *bufferH == height {
		return true
	}
	multidimReleaseBitmapBuffer(dc, bitmap, oldBitmap, bufferW, bufferH)
	newDC, _, _ := procCreateCompatibleDC.Call(referenceHDC)
	if newDC == 0 {
		return false
	}
	newBitmap, _, _ := procCreateCompatibleBitmap.Call(referenceHDC, uintptr(width), uintptr(height))
	if newBitmap == 0 {
		procDeleteDC.Call(newDC)
		return false
	}
	newOld, _, _ := procSelectObject.Call(newDC, newBitmap)
	*dc, *bitmap, *oldBitmap = newDC, newBitmap, newOld
	*bufferW, *bufferH = width, height
	return true
}

func (s *multidimShapeEditorState) releasePreviewBuffers() {
	if s == nil {
		return
	}
	multidimReleaseBitmapBuffer(&s.previewBackDC, &s.previewBackBitmap, &s.previewBackOldBitmap, &s.previewBackW, &s.previewBackH)
	multidimReleaseBitmapBuffer(&s.previewSmoothDC, &s.previewSmoothBitmap, &s.previewSmoothOldBitmap, &s.previewSmoothW, &s.previewSmoothH)
	s.previewSmoothBits = nil
	s.previewDownsample = nil
	s.releaseRasterBlitBuffer()
	s.opaqueFacePixels = nil
	s.opaqueFaceDepth = nil
	s.opaqueFaceLeft, s.opaqueFaceTop = 0, 0
	s.opaqueFaceW, s.opaqueFaceH = 0, 0
	s.opaqueFaceValid = false
	s.blackHoleCachePixels = nil
	s.blackHoleCacheW, s.blackHoleCacheH = 0, 0
	s.blackHoleCacheKey = 0
	s.celestialCachePixels = nil
	s.celestialCacheW, s.celestialCacheH = 0, 0
	s.celestialCacheKey = 0
}

func multidimRenderInsertPass(s *multidimShapeEditorState, width, height, requestedSS int, maskPass bool) (ClipboardImage, error) {
	if s == nil || s.previewBox == 0 || s.app == nil || width <= 0 || height <= 0 {
		return ClipboardImage{}, fmt.Errorf("삽입 렌더링 상태가 올바르지 않습니다")
	}
	refHDC, _, _ := procGetDC.Call(s.previewBox)
	if refHDC == 0 {
		return ClipboardImage{}, fmt.Errorf("미리보기 화면을 읽을 수 없습니다")
	}
	defer procReleaseDC.Call(s.previewBox, refHDC)

	if requestedSS < 1 {
		requestedSS = 1
	}
	actualSS := requestedSS
	var highDC, highBitmap, highOld uintptr
	var highW, highH int
	for actualSS > 1 {
		if multidimEnsureBitmapBuffer(refHDC, width*actualSS, height*actualSS, &highDC, &highBitmap, &highOld, &highW, &highH) {
			break
		}
		actualSS--
	}
	if highDC == 0 {
		actualSS = 1
		if !multidimEnsureBitmapBuffer(refHDC, width, height, &highDC, &highBitmap, &highOld, &highW, &highH) {
			return ClipboardImage{}, fmt.Errorf("삽입용 렌더링 버퍼를 만들 수 없습니다")
		}
	}
	defer multidimReleaseBitmapBuffer(&highDC, &highBitmap, &highOld, &highW, &highH)

	var finalDC, finalBitmap, finalOld uintptr
	var finalW, finalH int
	if !multidimEnsureBitmapBuffer(refHDC, width, height, &finalDC, &finalBitmap, &finalOld, &finalW, &finalH) {
		return ClipboardImage{}, fmt.Errorf("삽입용 출력 버퍼를 만들 수 없습니다")
	}
	defer multidimReleaseBitmapBuffer(&finalDC, &finalBitmap, &finalOld, &finalW, &finalH)

	oldFG, oldBG := s.app.fg, s.app.bg
	oldInteractive := s.previewInteractive
	oldMaskPass := s.insertMaskPass
	oldInvert := sendMessage(s.invertCheck, BM_GETCHECK, 0, 0)
	defer func() {
		s.app.fg, s.app.bg = oldFG, oldBG
		s.previewInteractive = oldInteractive
		s.insertMaskPass = oldMaskPass
		sendMessage(s.invertCheck, BM_SETCHECK, oldInvert, 0)
	}()
	// The insertion image always renders against a white key background so alpha
	// extraction remains stable. The black-background option is preview-only and
	// never changes the extracted foreground/face colours.
	sendMessage(s.invertCheck, BM_SETCHECK, BST_UNCHECKED, 0)
	s.previewInteractive = false
	s.insertMaskPass = maskPass
	if maskPass {
		s.app.fg = rgb(0, 0, 0)
		s.app.bg = rgb(0, 0, 0)
	}
	multidimPaintScene(s, highDC, RECT{0, 0, int32(width * actualSS), int32(height * actualSS)}, actualSS, false)
	if actualSS == 1 {
		procBitBlt.Call(finalDC, 0, 0, uintptr(width), uintptr(height), highDC, 0, 0, SRCCOPY)
	} else {
		procSetStretchBltMode.Call(finalDC, multidimHalftone)
		procStretchBlt.Call(finalDC, 0, 0, uintptr(width), uintptr(height), highDC, 0, 0, uintptr(width*actualSS), uintptr(height*actualSS), SRCCOPY)
	}

	// GetDIBits is most reliable when the bitmap is not selected into a DC.
	procSelectObject.Call(finalDC, finalOld)
	finalOld = 0
	img, err := bitmapHandleToClipboardImage(finalBitmap)
	if err != nil {
		return ClipboardImage{}, err
	}
	return img, nil
}

func multidimBuildInsertImage(s *multidimShapeEditorState) (ClipboardImage, error) {
	if s == nil || s.app == nil {
		return ClipboardImage{}, fmt.Errorf("다차원 도형 편집기가 닫혀 있습니다")
	}
	const targetW, targetH = 640, 480
	ss := multidimPreviewSupersample(s)
	colorImage, err := multidimRenderInsertPass(s, targetW, targetH, ss, false)
	if err != nil {
		return ClipboardImage{}, err
	}
	maskImage, err := multidimRenderInsertPass(s, targetW, targetH, ss, true)
	if err != nil {
		return ClipboardImage{}, err
	}
	if !validClipboardImage(colorImage) || !validClipboardImage(maskImage) || colorImage.W != maskImage.W || colorImage.H != maskImage.H {
		return ClipboardImage{}, fmt.Errorf("삽입 이미지 렌더링 결과가 올바르지 않습니다")
	}
	alpha := make([]byte, targetW*targetH)
	minX, minY, maxX, maxY := targetW, targetH, -1, -1
	for y := 0; y < targetH; y++ {
		for x := 0; x < targetW; x++ {
			i := y*targetW + x
			pi := i * 4
			b := int(maskImage.Pix[pi+0])
			g := int(maskImage.Pix[pi+1])
			r := int(maskImage.Pix[pi+2])
			maxChannel := max(r, max(g, b))
			minChannel := min(r, min(g, b))
			diff := 255 - minChannel
			if diff <= 0 {
				continue
			}
			a := 255
			// The forced-black geometry pass produces neutral gray wherever GDI
			// has blended coverage with the white key background. In that case
			// 255-gray is the real coverage/opacity, including 20% faces and AA.
			// Hard-coded coloured helpers (xyz axes and conic-section guides) are
			// treated as opaque because their source colour is intentionally not
			// controlled by the foreground/background override.
			if maxChannel-minChannel <= 2 {
				a = 255 - (r+g+b)/3
			}
			if a < 1 {
				continue
			}
			alpha[i] = byte(a)
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < minX || maxY < minY {
		return ClipboardImage{}, fmt.Errorf("삽입할 도형이 화면에 보이지 않습니다")
	}
	margin := 3
	minX = max(0, minX-margin)
	minY = max(0, minY-margin)
	maxX = min(targetW-1, maxX+margin)
	maxY = min(targetH-1, maxY+margin)
	outW, outH := maxX-minX+1, maxY-minY+1
	out := ClipboardImage{W: outW, H: outH, Pix: make([]byte, outW*outH*4), Mask: make([]bool, outW*outH)}
	for y := 0; y < outH; y++ {
		for x := 0; x < outW; x++ {
			si := (y+minY)*targetW + (x + minX)
			di := y*outW + x
			a := alpha[si]
			if a == 0 {
				continue
			}
			spi, dpi := si*4, di*4
			b, g, r := colorImage.Pix[spi+0], colorImage.Pix[spi+1], colorImage.Pix[spi+2]
			if a < 255 {
				recoverChannel := func(c byte) byte {
					v := 255 - ((255-int(c))*255+int(a)/2)/int(a)
					return byte(clamp(v, 0, 255))
				}
				b, g, r = recoverChannel(b), recoverChannel(g), recoverChannel(r)
			}
			out.Pix[dpi+0], out.Pix[dpi+1], out.Pix[dpi+2], out.Pix[dpi+3] = b, g, r, a
			out.Mask[di] = true
		}
	}
	return out, nil
}

func multidimCommitInsert(s *multidimShapeEditorState) {
	if s == nil || s.app == nil {
		return
	}
	spec := multidimEditorSelectedShape(s)
	img, err := multidimBuildInsertImage(s)
	if err != nil || !validClipboardImage(img) {
		message := "삽입 이미지를 만들 수 없습니다."
		if err != nil {
			message += "\n\n" + err.Error()
		}
		messageBox(s.hwnd, message, "다차원 도형 그리기", MB_OK|MB_ICONWARNING)
		return
	}
	owner := s.app
	procDestroyWindow.Call(s.hwnd)
	if owner.tool == ToolText && owner.textTool.Active {
		owner.finishTextEditing(true)
	}
	if owner.curveUndoPending {
		owner.finishCurveEarlyOrCancel("multidimensional shape insert")
	}
	if !owner.pasteClipboardImage(img, "다차원 도형: "+spec.name) {
		messageBox(owner.hwnd, "다차원 도형 이미지를 캔버스에 삽입하지 못했습니다.", "다차원 도형 그리기", MB_OK|MB_ICONWARNING)
		return
	}
	owner.statusText = fmt.Sprintf("%s을(를) %d × %d 투명 선택 이미지로 삽입했습니다.", spec.name, img.W, img.H)
	logf("MULTIDIM inserted shape=%q image=%dx%d mask=%d", spec.name, img.W, img.H, len(img.Mask))
}

func multidimHandleCommand(s *multidimShapeEditorState, id, code int) {
	switch id {
	case multidimIDShapeCombo:
		if code == graphCBNSelection {
			multidimEditorApplyShapeDefaults(s)
			s.blackHoleCacheKey = 0
			s.celestialCacheKey = 0
			spec := multidimEditorSelectedShape(s)
			// A192: every newly selected shape starts in the intuitive bound state:
			// object centre, visible axis origin, and local frame all coincide.
			s.axisOrigin = mdVec3{}
			s.axisPivotLocal3 = mdVec3{}
			s.axisPivotLocal4 = mdVec4{}
			s.axisDetached = false
			if multidimShapeUsesCoordinatePlane(spec) {
				// 좌표평면은 축을 포함하는 도형이므로 처음 선택할 때 축 표시를 켠다.
				sendMessage(s.axesCheck, BM_SETCHECK, BST_CHECKED, 0)
			} else if multidimShapeUsesBlackHole(spec) {
				// 블랙홀 전용 장면은 자체 좌표망/광학 화면을 가지므로 일반 축 오버레이는 처음에는 숨긴다.
				sendMessage(s.axesCheck, BM_SETCHECK, BST_UNCHECKED, 0)
				sendMessage(s.moveAxesCheck, BM_SETCHECK, BST_UNCHECKED, 0)
			}
			multidimEditorRefreshSelection(s)
		}
	case multidimIDOpacityCombo:
		if code == graphCBNSelection {
			multidimEditorRefreshSelection(s)
		}
	case multidimIDOptionCombo:
		if code == graphCBNSelection {
			multidimEditorCommitOption(s)
			s.blackHoleCacheKey = 0
			s.celestialCacheKey = 0
			multidimEditorRefreshSelection(s)
		}
	case multidimIDSmoothCheck:
		if code == BN_CLICKED {
			s.smoothEnabled = !s.smoothEnabled
			state, label := uintptr(BST_UNCHECKED), "부드럽게: 끔"
			if s.smoothEnabled {
				state, label = BST_CHECKED, "부드럽게: 켬"
			}
			sendMessage(s.smoothCheck, BM_SETCHECK, state, 0)
			procSetWindowTextW.Call(s.smoothCheck, uintptr(unsafe.Pointer(utf16Ptr(label))))
			logf("MDSMOOTH enabled=%v", s.smoothEnabled)
			multidimEditorRefreshSelection(s)
		}
	case multidimIDInvertCheck, multidimIDMoveAxesCheck:
		if code == BN_CLICKED {
			multidimEditorRefreshSelection(s)
		}
	case multidimIDAxesCheck:
		if code == BN_CLICKED {
			multidimEditorRefreshSelection(s)
			var client RECT
			procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
			multidimEditorLayoutControls(s, client)
		}
	case multidimIDAxisReset:
		if code == BN_CLICKED {
			multidimResetAxisOrigin(s)
			multidimEditorRefreshSelection(s)
		}
	case multidimIDShapeReset:
		if code == BN_CLICKED {
			multidimResetShapeOrigin(s)
			multidimEditorRefreshSelection(s)
		}
	case multidimIDInsert:
		if code == BN_CLICKED {
			multidimCommitInsert(s)
		}
	case multidimIDCancel:
		procDestroyWindow.Call(s.hwnd)
	}
}

func multidimShapeEditorWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			logPanic("multidimShapeEditorWndProc", r)
		}
	}()
	s := activeMultidimShapeEditor
	if r, handled := handleDarkEditorNonClient(hwnd, msg, wparam, lparam); handled {
		return r
	}
	switch msg {
	case WM_CREATE:
		if s == nil {
			return ^uintptr(0)
		}
		s.hwnd = hwnd
		multidimEditorCreateControls(s)
		var client RECT
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
		multidimEditorLayoutControls(s, client)
		// Apply caption theming during WM_CREATE, as the help centre does.
		applyDarkThemeToWindow(hwnd, darkModeEnabled())
		drawDarkEditorCaption(hwnd)
		scheduleDarkEditorChromeRefresh(hwnd)
		return 0
	case WM_ERASEBKGND:
		if s != nil && s.app != nil && s.app.darkMode && wparam != 0 {
			var client RECT
			procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
			fillRect(wparam, client, rgb(30, 30, 30))
			return 1
		}
	case WM_CTLCOLOREDIT, WM_CTLCOLORSTATIC, WM_CTLCOLORBTN, WM_CTLCOLORLISTBOX, WM_CTLCOLORSCROLLBAR, WM_CTLCOLORDLG:
		if s != nil && s.app != nil && s.app.darkMode && wparam != 0 {
			procSetBkMode.Call(wparam, OPAQUE)
			procSetTextColor.Call(wparam, uintptr(rgb(235, 235, 235)))
			procSetBkColor.Call(wparam, uintptr(rgb(30, 30, 30)))
			if s.app.darkBrush == 0 {
				s.app.darkBrush, _, _ = procCreateSolidBrush.Call(uintptr(rgb(30, 30, 30)))
			}
			return s.app.darkBrush
		}
	case WM_SIZE:
		if s != nil && s.hwnd == hwnd {
			var client RECT
			procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
			multidimEditorLayoutControls(s, client)
		}
		return 0
	case WM_HSCROLL:
		if s != nil && s.hwnd == hwnd {
			scrollCode := int(wparam & 0xFFFF)
			scrollHwnd := lparam
			if scrollCode == SB_THUMBTRACK {
				s.previewInteractive = true
				multidimEditorRefreshValueLabels(s)
				if scrollHwnd == s.axisScaleSlider {
					multidimSetText(s.statusLabel, "축 크기 조절 중 · 축 표시 길이만 바뀌며 도형과 회전 원점은 유지됩니다.")
				} else {
					multidimSetText(s.statusLabel, "회전 중: 선택한 면 모드·격자·부드러움 품질을 그대로 표시합니다.")
				}
				multidimInvalidatePreview(s)
			} else {
				s.previewInteractive = false
				multidimEditorRefreshSelection(s)
			}
		}
		return 0
	case WM_COMMAND:
		if s != nil && s.hwnd == hwnd {
			id := int(wparam & 0xFFFF)
			code := int((wparam >> 16) & 0xFFFF)
			multidimHandleCommand(s, id, code)
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
	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		if s != nil && s.hwnd == hwnd {
			s.releasePreviewBuffers()
			if s.uiFont != 0 {
				procDeleteObject.Call(s.uiFont)
				s.uiFont = 0
			}
			activeMultidimShapeEditor = nil
			logf("MULTIDIM editor closed")
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func multidimPreviewWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			logPanic("multidimPreviewWndProc", r)
		}
	}()
	s := activeMultidimShapeEditor
	if r, handled := handleDarkEditorNonClient(hwnd, msg, wparam, lparam); handled {
		return r
	}
	switch msg {
	case WM_LBUTTONDOWN:
		if s != nil && s.previewBox == hwnd {
			x, y := multidimPreviewMousePoint(lparam)
			multidimBeginPreviewDrag(s, hwnd, x, y)
			return 0
		}
	case WM_MOUSEMOVE:
		if s != nil && s.previewBox == hwnd && multidimPreviewDragging(s) {
			if wparam&MK_LBUTTON == 0 {
				multidimEndPreviewDrag(s, hwnd, true)
				return 0
			}
			x, y := multidimPreviewMousePoint(lparam)
			multidimUpdatePreviewDrag(s, hwnd, x, y)
			return 0
		}
	case WM_LBUTTONUP:
		if s != nil && s.previewBox == hwnd && multidimPreviewDragging(s) {
			multidimEndPreviewDrag(s, hwnd, true)
			return 0
		}
	case WM_CAPTURECHANGED:
		if s != nil && s.previewBox == hwnd && multidimPreviewDragging(s) {
			newOwner := lparam
			if newOwner != hwnd {
				multidimEndPreviewDrag(s, hwnd, false)
			}
		}
		return 0
	case WM_ERASEBKGND:
		// The preview is committed atomically from a persistent back buffer.
		// Claim the erase so the class/background brush can never flash first.
		return 1
	case WM_SIZE:
		if s != nil {
			s.previewDirty = true
		}
		return 0
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if hdc != 0 {
			var client RECT
			procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
			w := int(client.Right - client.Left)
			h := int(client.Bottom - client.Top)
			if s != nil {
				oldW, oldH := s.previewBackW, s.previewBackH
				if multidimEnsureBitmapBuffer(hdc, w, h,
					&s.previewBackDC, &s.previewBackBitmap, &s.previewBackOldBitmap,
					&s.previewBackW, &s.previewBackH) {
					if oldW != w || oldH != h {
						s.previewDirty = true
					}
					if s.previewDirty {
						multidimPaintPreview(s, hwnd, s.previewBackDC)
						s.previewDirty = false
					}
					rc := ps.RcPaint
					if rc.Right <= rc.Left || rc.Bottom <= rc.Top {
						rc = client
					}
					procBitBlt.Call(hdc,
						uintptr(rc.Left), uintptr(rc.Top), uintptr(rc.Right-rc.Left), uintptr(rc.Bottom-rc.Top),
						s.previewBackDC, uintptr(rc.Left), uintptr(rc.Top), SRCCOPY)
				} else {
					multidimPaintPreview(s, hwnd, hdc)
				}
			} else {
				multidimPaintPreview(nil, hwnd, hdc)
			}
			procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		}
		return 0
	case WM_DESTROY:
		if s != nil && s.previewBox == hwnd {
			s.previewRotationDragging = false
			s.previewAxisDragging = false
			s.previewInteractive = false
			s.releasePreviewBuffers()
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func multidimPreviewSupersample(s *multidimShapeEditorState) int {
	if s == nil {
		return 1
	}
	spec := multidimEditorSelectedShape(s)
	return mdPreviewSupersamplePolicy(spec.name, multidimEditorSmoothEnabled(s), s.previewInteractive)
}

func multidimPaintPreview(s *multidimShapeEditorState, hwnd, hdc uintptr) {
	var rc RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	ss := multidimPreviewSupersample(s)
	if ss <= 1 {
		multidimPaintPreviewCore(s, hdc, rc, 1)
		return
	}
	w := int(rc.Right - rc.Left)
	h := int(rc.Bottom - rc.Top)
	if w <= 0 || h <= 0 {
		return
	}
	// A maximized 6x preview can be large. If the requested DIB cannot be
	// allocated, step down one level at a time instead of falling directly back
	// to a jagged 1x preview.
	actualSS := ss
	for actualSS > 1 {
		if s.ensureSmoothDIB(hdc, w*actualSS, h*actualSS) {
			break
		}
		actualSS--
	}
	if actualSS <= 1 {
		multidimPaintPreviewCore(s, hdc, rc, 1)
		return
	}
	multidimPaintPreviewCore(s, s.previewSmoothDC, RECT{0, 0, int32(w * actualSS), int32(h * actualSS)}, actualSS)
	need := w * h * 4
	if cap(s.previewDownsample) < need {
		s.previewDownsample = make([]byte, need)
	} else {
		s.previewDownsample = s.previewDownsample[:need]
	}
	src := unsafe.Slice((*byte)(s.previewSmoothBits), w*actualSS*h*actualSS*4)
	if mdAreaDownsampleBGRA(s.previewDownsample, src, w, h, actualSS) && multidimBlitBGRA(s, hdc, rc, s.previewDownsample, w, h, "deterministic-aa") {
		return
	}
	procSetStretchBltMode.Call(hdc, multidimHalftone)
	procStretchBlt.Call(hdc, uintptr(rc.Left), uintptr(rc.Top), uintptr(w), uintptr(h), s.previewSmoothDC, 0, 0, uintptr(w*actualSS), uintptr(h*actualSS), SRCCOPY)
}

func (s *multidimShapeEditorState) ensureSmoothDIB(referenceHDC uintptr, width, height int) bool {
	if s != nil && s.previewSmoothDC != 0 && s.previewSmoothBitmap != 0 && s.previewSmoothBits != nil && s.previewSmoothW == width && s.previewSmoothH == height {
		return true
	}
	if s == nil || referenceHDC == 0 || width <= 0 || height <= 0 {
		return false
	}
	multidimReleaseBitmapBuffer(&s.previewSmoothDC, &s.previewSmoothBitmap, &s.previewSmoothOldBitmap, &s.previewSmoothW, &s.previewSmoothH)
	s.previewSmoothBits = nil
	dc, _, _ := procCreateCompatibleDC.Call(referenceHDC)
	if dc == 0 {
		return false
	}
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(width), BiHeight: -int32(height), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	var bits unsafe.Pointer
	bm, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bm == 0 || bits == nil {
		procDeleteDC.Call(dc)
		return false
	}
	old, _, _ := procSelectObject.Call(dc, bm)
	s.previewSmoothDC, s.previewSmoothBitmap, s.previewSmoothOldBitmap = dc, bm, old
	s.previewSmoothW, s.previewSmoothH, s.previewSmoothBits = width, height, bits
	return true
}

// multidimPreviewObjectRect applies only the translation induced by rotating
// around the movable pivot. Moving the pivot itself is compensated so this
// rectangle does not change during an axis drag; the object therefore stays
// visually fixed until the user changes a rotation value.
func multidimPreviewObjectRect(s *multidimShapeEditorState, rc RECT) RECT {
	if s == nil {
		return rc
	}
	extent := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top))
	if extent <= 0 {
		return rc
	}
	offset := multidimAxisObjectOffset(s)
	dx := int32(math.Round(offset.x * extent))
	dy := int32(math.Round(-offset.z * extent))
	return RECT{Left: rc.Left + dx, Top: rc.Top + dy, Right: rc.Right + dx, Bottom: rc.Bottom + dy}
}

func multidimPaintPreviewCore(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	multidimPaintScene(s, hdc, rc, ss, true)
}

func multidimPreviewBackgroundColor(s *multidimShapeEditorState) uint32 {
	if s != nil && multidimEditorInvertEnabled(s) && !s.insertMaskPass {
		return rgb(18, 18, 18)
	}
	return rgb(255, 255, 255)
}

func multidimPaintScene(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int, drawFrame bool) {
	background := multidimPreviewBackgroundColor(s)
	fillRect(hdc, rc, background)
	if s == nil {
		return
	}
	if ss < 1 {
		ss = 1
	}
	if drawFrame {
		multidimStrokeRect(hdc, rc, ss, rgb(0, 0, 0))
	}
	renderRC := multidimPreviewObjectRect(s, rc)
	spec := multidimEditorSelectedShape(s)
	switch spec.name {
	case "원판":
		multidimPaintDisk(s, hdc, renderRC, ss)
	case "평면":
		multidimPaintRectanglePlane(s, hdc, renderRC, ss)
	case "좌표평면":
		multidimPaintCoordinatePlane(s, hdc, renderRC, ss)
	case "포물면/쌍곡면":
		multidimPaintQuadricSurface(s, hdc, renderRC, ss)
	case "말안장면":
		multidimPaintSaddleSurface(s, hdc, renderRC, ss)
	case "시공간: 블랙홀":
		multidimPaintBlackHole(s, hdc, renderRC, ss)
	case "쌍원뿔과 원뿔곡선":
		multidimPaintDoubleConeSection(s, hdc, renderRC, ss)
	case "정사면체":
		multidimPaintTetrahedron(s, hdc, renderRC, ss)
	case "정육면체":
		multidimPaintCube(s, hdc, renderRC, ss)
	case "정팔면체":
		multidimPaintOctahedron(s, hdc, renderRC, ss)
	case "정십이면체":
		multidimPaintDodecahedron(s, hdc, renderRC, ss)
	case "정이십면체":
		multidimPaintIcosahedron(s, hdc, renderRC, ss)
	case "풀러렌(C60)":
		multidimPaintFullerene(s, hdc, renderRC, ss)
	case "2-스타인메츠 입체":
		multidimPaintSteinmetz2(s, hdc, renderRC, ss)
	case "3-스타인메츠 입체":
		multidimPaintSteinmetz3(s, hdc, renderRC, ss)
	case "원기둥":
		multidimPaintCylinder(s, hdc, renderRC, ss)
	case "원뿔":
		multidimPaintCone(s, hdc, renderRC, ss)
	case "다각뿔":
		multidimPaintPolygonalPyramid(s, hdc, renderRC, ss)
	case "원환면(토러스)":
		multidimPaintTorus(s, hdc, renderRC, ss)
	case "뫼비우스 띠":
		multidimPaintMobiusStrip(s, hdc, renderRC, ss)
	case "타원체":
		multidimPaintEllipsoid(s, hdc, renderRC, ss, false)
	case "구":
		multidimPaintEllipsoid(s, hdc, renderRC, ss, true)
	case "정다포체: 5-셀":
		multidimPaintRegular5Cell(s, hdc, renderRC, ss)
	case "정다포체: 테서렉트":
		multidimPaintTesseract(s, hdc, renderRC, ss)
	case "정다포체: 16-셀":
		multidimPaintRegular16Cell(s, hdc, renderRC, ss)
	case "정다포체: 24-셀":
		multidimPaintRegular24Cell(s, hdc, renderRC, ss)
	case "정다포체: 120-셀":
		multidimPaintRegular120Cell(s, hdc, renderRC, ss)
	case "정다포체: 600-셀":
		multidimPaintRegular600Cell(s, hdc, renderRC, ss)
	case "4차원 곡면: 클라인 병":
		multidimPaintKleinBottle(s, hdc, renderRC, ss)
	default:
		multidimPaintPreviewPlaceholder(hdc, rc, spec.name)
	}
	if multidimEditorAxesEnabled(s) {
		multidimPaintXYZAxes(s, hdc, rc, ss)
	}
}

func mdDrawAxisLetter(hdc uintptr, center POINT, label byte, size, width int, color uint32) {
	if size < 4 {
		size = 4
	}
	h := int32(size / 2)
	w := int32(width)
	if w < 1 {
		w = 1
	}
	line := func(x0, y0, x1, y1 int32) {
		mdDrawPolyline(hdc, []POINT{{X: center.X + x0, Y: center.Y + y0}, {X: center.X + x1, Y: center.Y + y1}}, int(w), color, false)
	}
	switch label {
	case 'x':
		line(-h, -h, h, h)
		line(-h, h, h, -h)
	case 'y':
		line(-h, -h, 0, 0)
		line(h, -h, 0, 0)
		line(0, 0, 0, h)
	case 'z':
		line(-h, -h, h, -h)
		line(h, -h, -h, h)
		line(-h, h, h, h)
	}
}

func mdDrawAxisDepthMarker(hdc uintptr, center POINT, towardCamera bool, radius, width int, color uint32) {
	if radius < 3 {
		radius = 3
	}
	if width < 1 {
		width = 1
	}
	pen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(width), uintptr(color))
	nullBrush, _, _ := procGetStockObject.Call(NULL_BRUSH)
	oldPen, _, _ := procSelectObject.Call(hdc, pen)
	oldBrush, _, _ := procSelectObject.Call(hdc, nullBrush)
	procEllipse.Call(hdc, uintptr(center.X-int32(radius)), uintptr(center.Y-int32(radius)), uintptr(center.X+int32(radius)+1), uintptr(center.Y+int32(radius)+1))
	procSelectObject.Call(hdc, oldBrush)
	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(pen)
	if towardCamera {
		brush, _, _ := procCreateSolidBrush.Call(uintptr(color))
		oldBrush, _, _ = procSelectObject.Call(hdc, brush)
		r := int32(mdMaxInt(1, radius/3))
		procEllipse.Call(hdc, uintptr(center.X-r), uintptr(center.Y-r), uintptr(center.X+r+1), uintptr(center.Y+r+1))
		procSelectObject.Call(hdc, oldBrush)
		procDeleteObject.Call(brush)
	} else {
		r := int32(radius - 2)
		mdDrawPolyline(hdc, []POINT{{X: center.X - r, Y: center.Y - r}, {X: center.X + r, Y: center.Y + r}}, width, color, false)
		mdDrawPolyline(hdc, []POINT{{X: center.X - r, Y: center.Y + r}, {X: center.X + r, Y: center.Y - r}}, width, color, false)
	}
}

func multidimPaintXYZAxes(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	if s == nil || hdc == 0 {
		return
	}
	if ss < 1 {
		ss = 1
	}
	ax, ay, az, _ := multidimRotation(s)
	dirs := mdCoordinateAxisDirections(ax, ay, az)
	if multidimAxesFollowShape(s) {
		// In the normal state every shape carries its fully rotated local frame.
		// After an explicit pivot move we switch to the real Euler operation axes
		// so moved X/Y/Z lines continue to match their sliders exactly.
		dirs = mdObjectCoordinateAxisDirections(ax, ay, az)
	}
	colors := [3]uint32{rgb(205, 45, 45), rgb(40, 150, 65), rgb(45, 85, 210)}
	labels := [3]byte{'x', 'y', 'z'}
	extent := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top))
	baseCenter := mdProject(mdVec3{}, rc, 1)
	visibleOrigin := s.axisOrigin
	if multidimAxesFollowShape(s) {
		visibleOrigin = multidimAxisObjectOffset(s)
	}
	center := POINT{
		X: baseCenter.X + int32(math.Round(visibleOrigin.x*extent)),
		Y: baseCenter.Y - int32(math.Round(visibleOrigin.z*extent)),
	}
	axisScaleMultiplier := 1.0
	if s.axisScaleSlider != 0 {
		axisScaleMultiplier = float64(multidimSliderPos(s.axisScaleSlider)) / 100.0
	}
	axisScale := 0.30 * axisScaleMultiplier * extent
	projectFromCenter := func(v mdVec3, scale float64) POINT {
		return POINT{
			X: center.X + int32(math.Round(v.x*scale)),
			Y: center.Y - int32(math.Round(v.z*scale)),
		}
	}
	positiveWidth := mdMaxInt(1, 2*ss)
	negativeWidth := mdMaxInt(1, ss)
	arrowLength := float64(9 * ss)
	arrowHalfWidth := float64(4 * ss)
	for i, dir := range dirs {
		positive := projectFromCenter(dir, axisScale)
		negative := projectFromCenter(mdMul(dir, -1), axisScale)
		negativeColor := mdMixColorRefTowardWhite(colors[i], 0.58)
		mdDrawDashedLine(hdc, negative, center, negativeWidth, negativeColor)
		dx := float64(positive.X - center.X)
		dy := float64(positive.Y - center.Y)
		projectedLength := math.Hypot(dx, dy)
		if projectedLength < float64(10*ss) {
			mdDrawAxisDepthMarker(hdc, center, dir.y >= 0, 6*ss, positiveWidth, colors[i])
			offsets := [3]POINT{{X: int32(12 * ss), Y: int32(-10 * ss)}, {X: int32(12 * ss), Y: int32(4 * ss)}, {X: int32(-12 * ss), Y: int32(-10 * ss)}}
			labelAt := POINT{X: center.X + offsets[i].X, Y: center.Y + offsets[i].Y}
			mdDrawAxisLetter(hdc, labelAt, labels[i], 9*ss, ss, colors[i])
			continue
		}
		mdDrawPolyline(hdc, []POINT{center, positive}, positiveWidth, colors[i], false)
		ux, uy := dx/projectedLength, dy/projectedLength
		px, py := -uy, ux
		left := POINT{
			X: int32(math.Round(float64(positive.X) - ux*arrowLength + px*arrowHalfWidth)),
			Y: int32(math.Round(float64(positive.Y) - uy*arrowLength + py*arrowHalfWidth)),
		}
		right := POINT{
			X: int32(math.Round(float64(positive.X) - ux*arrowLength - px*arrowHalfWidth)),
			Y: int32(math.Round(float64(positive.Y) - uy*arrowLength - py*arrowHalfWidth)),
		}
		mdDrawPolyline(hdc, []POINT{left, positive, right}, positiveWidth, colors[i], false)
		labelAt := POINT{
			X: int32(math.Round(float64(positive.X) + ux*float64(12*ss))),
			Y: int32(math.Round(float64(positive.Y) + uy*float64(12*ss))),
		}
		mdDrawAxisLetter(hdc, labelAt, labels[i], 9*ss, ss, colors[i])
	}
}

func multidimPaintPreviewPlaceholder(hdc uintptr, rc RECT, name string) {
	msg := fmt.Sprintf("%s\r\n\r\n미리보기 렌더링 준비 중", name)
	drawText(hdc, msg, rc, DT_CENTER|DT_VCENTER|DT_WORDBREAK, rgb(0, 0, 0))
}

func multidimRotation(s *multidimShapeEditorState) (float64, float64, float64, float64) {
	return float64(multidimSliderPos(s.xSlider)) / 100.0,
		float64(multidimSliderPos(s.ySlider)) / 100.0,
		float64(multidimSliderPos(s.zSlider)) / 100.0,
		float64(multidimSliderPos(s.wSlider)) / 100.0
}

func multidimOpacityIndex(s *multidimShapeEditorState) int {
	idx := int(sendMessage(s.opacityCombo, CB_GETCURSEL, 0, 0))
	if idx < 0 || idx >= len(multidimOpacityModes) {
		idx = 0
	}
	return idx
}

func mdRotX(v mdVec3, a float64) mdVec3 {
	c, s := math.Cos(a), math.Sin(a)
	return mdVec3{v.x, v.y*c - v.z*s, v.y*s + v.z*c}
}
func mdRotY(v mdVec3, a float64) mdVec3 {
	c, s := math.Cos(a), math.Sin(a)
	return mdVec3{v.x*c + v.z*s, v.y, -v.x*s + v.z*c}
}
func mdRotZ(v mdVec3, a float64) mdVec3 {
	c, s := math.Cos(a), math.Sin(a)
	return mdVec3{v.x*c - v.y*s, v.x*s + v.y*c, v.z}
}
func mdRotate(v mdVec3, ax, ay, az float64) mdVec3 { return mdRotZ(mdRotY(mdRotX(v, ax), ay), az) }
func mdAdd(a, b mdVec3) mdVec3                     { return mdVec3{a.x + b.x, a.y + b.y, a.z + b.z} }
func mdSub(a, b mdVec3) mdVec3                     { return mdVec3{a.x - b.x, a.y - b.y, a.z - b.z} }
func mdMul(a mdVec3, k float64) mdVec3             { return mdVec3{a.x * k, a.y * k, a.z * k} }
func mdCross(a, b mdVec3) mdVec3 {
	return mdVec3{a.y*b.z - a.z*b.y, a.z*b.x - a.x*b.z, a.x*b.y - a.y*b.x}
}
func mdDot(a, b mdVec3) float64 { return a.x*b.x + a.y*b.y + a.z*b.z }
func mdNorm(v mdVec3) mdVec3 {
	l := math.Sqrt(mdDot(v, v))
	if l == 0 {
		return v
	}
	return mdMul(v, 1/l)
}

func mdProject(v mdVec3, rc RECT, scale float64) POINT {
	cx := float64(rc.Left+rc.Right) / 2.0
	cy := float64(rc.Top+rc.Bottom) / 2.0
	return POINT{X: int32(math.Round(cx + v.x*scale)), Y: int32(math.Round(cy - v.z*scale))}
}

func mdProject4To3(v mdVec4, distance float64) mdVec3 {
	x, y, z := mdProject4To3Components(v, distance)
	return mdVec3{x: x, y: y, z: z}
}

func mdBlendedColorOnBackground(base, background uint32, brightness, alpha float64) uint32 {
	if brightness < 0.15 {
		brightness = 0.15
	}
	if brightness > 1 {
		brightness = 1
	}
	r := float64((base>>16)&0xFF) * brightness
	g := float64((base>>8)&0xFF) * brightness
	b := float64(base&0xFF) * brightness
	br := float64((background >> 16) & 0xFF)
	bg := float64((background >> 8) & 0xFF)
	bb := float64(background & 0xFF)
	r = br*(1-alpha) + r*alpha
	g = bg*(1-alpha) + g*alpha
	b = bb*(1-alpha) + b*alpha
	return rgb(byte(r+0.5), byte(g+0.5), byte(b+0.5))
}

func mdBlendedColor(base uint32, brightness, alpha float64) uint32 {
	return mdBlendedColorOnBackground(base, rgb(255, 255, 255), brightness, alpha)
}

func mdMixTowardWhite(base uint32, amount float64) uint32 {
	if amount < 0 {
		amount = 0
	}
	if amount > 1 {
		amount = 1
	}
	r := float64((base >> 16) & 0xFF)
	g := float64((base >> 8) & 0xFF)
	b := float64(base & 0xFF)
	r = r + (255-r)*amount
	g = g + (255-g)*amount
	b = b + (255-b)*amount
	return rgb(byte(r+0.5), byte(g+0.5), byte(b+0.5))
}

func multidimPreviewLineColor(s *multidimShapeEditorState) uint32 {
	if s == nil || s.app == nil || isTransparentColor(s.app.fg) {
		return rgb(255, 255, 255)
	}
	return s.app.fg
}

func multidimPreviewFillColor(s *multidimShapeEditorState) uint32 {
	if s == nil || s.app == nil || isTransparentColor(s.app.bg) {
		return rgb(255, 255, 255)
	}
	return s.app.bg
}

func multidimStrokeRect(hdc uintptr, rc RECT, width int, color uint32) {
	pen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(width), uintptr(color))
	nullBrush, _, _ := procGetStockObject.Call(NULL_BRUSH)
	oldPen, _, _ := procSelectObject.Call(hdc, pen)
	oldBrush, _, _ := procSelectObject.Call(hdc, nullBrush)
	procRectangle.Call(hdc, uintptr(rc.Left), uintptr(rc.Top), uintptr(rc.Right), uintptr(rc.Bottom))
	procSelectObject.Call(hdc, oldBrush)
	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(pen)
}

func mdDrawPolyline(hdc uintptr, pts []POINT, width int, color uint32, closePath bool) {
	if len(pts) < 2 {
		return
	}
	pen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(width), uintptr(color))
	old, _, _ := procSelectObject.Call(hdc, pen)
	defer func() { procSelectObject.Call(hdc, old); procDeleteObject.Call(pen) }()
	procMoveToEx.Call(hdc, uintptr(pts[0].X), uintptr(pts[0].Y), 0)
	for i := 1; i < len(pts); i++ {
		procLineTo.Call(hdc, uintptr(pts[i].X), uintptr(pts[i].Y))
	}
	if closePath {
		procLineTo.Call(hdc, uintptr(pts[0].X), uintptr(pts[0].Y))
	}
}

func mdDrawDashedLine(hdc uintptr, a, b POINT, width int, color uint32) {
	dx := float64(b.X - a.X)
	dy := float64(b.Y - a.Y)
	length := math.Hypot(dx, dy)
	if length < 1 {
		return
	}
	ux, uy := dx/length, dy/length
	dash, gap := float64(3*width), float64(6*width)
	pen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(width), uintptr(color))
	old, _, _ := procSelectObject.Call(hdc, pen)
	defer func() { procSelectObject.Call(hdc, old); procDeleteObject.Call(pen) }()
	for pos := 0.0; pos < length; pos += dash + gap {
		end := math.Min(length, pos+dash)
		x0 := float64(a.X) + ux*pos
		y0 := float64(a.Y) + uy*pos
		x1 := float64(a.X) + ux*end
		y1 := float64(a.Y) + uy*end
		procMoveToEx.Call(hdc, uintptr(int32(math.Round(x0))), uintptr(int32(math.Round(y0))), 0)
		procLineTo.Call(hdc, uintptr(int32(math.Round(x1))), uintptr(int32(math.Round(y1))))
	}
}

func mdFillPolygon(hdc uintptr, pts []POINT, fill, outline uint32) {
	if len(pts) < 3 {
		return
	}
	brush, _, _ := procCreateSolidBrush.Call(uintptr(fill))
	pen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(outline))
	oldBrush, _, _ := procSelectObject.Call(hdc, brush)
	oldPen, _, _ := procSelectObject.Call(hdc, pen)
	procPolygon.Call(hdc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	procSelectObject.Call(hdc, oldPen)
	procSelectObject.Call(hdc, oldBrush)
	procDeleteObject.Call(pen)
	procDeleteObject.Call(brush)
}

func mdHiddenLineColor(opacity int, lineColor uint32) (uint32, bool) {
	if opacity == 0 {
		return 0, false
	}
	if opacity == 1 {
		return mdMixTowardWhite(lineColor, 0.65), true
	}
	return lineColor, true
}

func mdSceneLightRayDirection() mdVec3      { return mdDefaultLightRayDirection() }
func mdSceneIncomingLightDirection() mdVec3 { return mdDefaultIncomingLightDirection() }

func mdRenderFacesOnlyWithNormal(hdc uintptr, rc RECT, verts []mdVec3, faces []mdFace, opacity int, fillColor uint32, normalForFace func(mdFace) mdVec3) {
	incomingLight := mdSceneIncomingLightDirection()
	alpha := 1.0
	if opacity == 1 {
		alpha = 0.20
	}
	fillFaces := opacity != 2
	if !fillFaces {
		return
	}
	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.33
	baseColor := fillColor
	faceRenders := []mdFaceRender{}
	for _, f := range faces {
		if len(f.indices) < 3 {
			continue
		}
		pts3 := make([]mdVec3, len(f.indices))
		avg := mdVec3{}
		valid := true
		for i, idx := range f.indices {
			if idx < 0 || idx >= len(verts) {
				valid = false
				break
			}
			pts3[i] = verts[idx]
			avg = mdAdd(avg, verts[idx])
		}
		if !valid {
			continue
		}
		avg = mdMul(avg, 1.0/float64(len(f.indices)))
		n := mdFaceOutwardNormal(verts, f)
		if normalForFace != nil {
			if custom := normalForFace(f); mdDot(custom, custom) > 1e-16 {
				n = mdNorm(custom)
			}
		}
		b := 0.30 + 0.70*math.Max(0, mdDot(n, incomingLight))
		pts2 := make([]POINT, len(pts3))
		for i, pt := range pts3 {
			pts2[i] = mdProject(pt, rc, scale)
		}
		faceRenders = append(faceRenders, mdFaceRender{pts: pts2, depth: avg.y, bright: b})
	}
	sort.Slice(faceRenders, func(i, j int) bool { return faceRenders[i].depth < faceRenders[j].depth })
	for _, fr := range faceRenders {
		col := mdBlendedColor(baseColor, fr.bright, alpha)
		mdFillPolygon(hdc, fr.pts, col, col)
	}
}

func mdRenderFacesOnly(hdc uintptr, rc RECT, verts []mdVec3, faces []mdFace, opacity int, fillColor uint32) {
	mdRenderFacesOnlyWithNormal(hdc, rc, verts, faces, opacity, fillColor, nil)
}

type mdPolyEdgeKey struct {
	a int
	b int
}

type mdPolyEdgeAdjacency struct {
	a          int
	b          int
	frontFaces int
	faceCount  int
	depth      float64
}

// mdRenderConvexPolyhedron classifies an edge by its adjacent faces, not by
// the edge midpoint's depth.  For a closed convex solid an edge is visible
// whenever at least one adjacent face points toward the viewer.  This makes
// silhouette edges solid even when their midpoint lies on the far half of the
// object, while the three edges meeting at a genuinely hidden cube vertex are
// dashed (or omitted in opaque mode).
func mdRenderConvexPolyhedron(hdc uintptr, rc RECT, verts []mdVec3, faces []mdFace, opacity int, ss int, lineColor uint32, fillColor uint32) {
	mdRenderFacesOnly(hdc, rc, verts, faces, opacity, fillColor)
	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.33
	lineW := mdMaxInt(1, 2*ss)

	frontFace := make([]bool, len(faces))
	for i, face := range faces {
		n := mdFaceOutwardNormal(verts, face)
		// Orthographic camera looks from +y toward the origin.
		frontFace[i] = n.y > 1e-7
	}

	edges := map[mdPolyEdgeKey]*mdPolyEdgeAdjacency{}
	for fi, face := range faces {
		count := len(face.indices)
		for i := 0; i < count; i++ {
			a := face.indices[i]
			b := face.indices[(i+1)%count]
			ka, kb := a, b
			if ka > kb {
				ka, kb = kb, ka
			}
			key := mdPolyEdgeKey{a: ka, b: kb}
			edge := edges[key]
			if edge == nil {
				edge = &mdPolyEdgeAdjacency{a: ka, b: kb, depth: (verts[ka].y + verts[kb].y) * 0.5}
				edges[key] = edge
			}
			edge.faceCount++
			if frontFace[fi] {
				edge.frontFaces++
			}
		}
	}

	projectedPts := make([]POINT, len(verts))
	for i, v := range verts {
		projectedPts[i] = mdProject(v, rc, scale)
	}
	hull := mdConvexHull(projectedPts)

	visible := make([]mdSegment, 0, len(edges))
	hidden := make([]mdSegment, 0, len(edges))
	for _, edge := range edges {
		seg := mdSegment{
			a:     projectedPts[edge.a],
			b:     projectedPts[edge.b],
			depth: edge.depth,
		}
		onHull := mdSegmentOnHull(seg.a, seg.b, hull)
		// For convex polyhedra, an edge is visible if at least one adjacent face
		// faces the viewer OR the projected edge lies on the outer silhouette hull.
		if edge.frontFaces > 0 || onHull {
			visible = append(visible, seg)
		} else {
			hidden = append(hidden, seg)
		}
	}

	sort.Slice(hidden, func(i, j int) bool { return hidden[i].depth < hidden[j].depth })
	if hiddenColor, ok := mdHiddenLineColor(opacity, lineColor); ok {
		for _, seg := range hidden {
			mdDrawDashedLine(hdc, seg.a, seg.b, lineW, hiddenColor)
		}
	}
	for _, seg := range visible {
		mdDrawPolyline(hdc, []POINT{seg.a, seg.b}, lineW, lineColor, false)
	}
}

func mdRenderMesh(hdc uintptr, rc RECT, verts []mdVec3, faces []mdFace, edgePaths []mdEdgePathSpec, opacity int, ss int, lineColor uint32, fillColor uint32) {
	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.33
	mdRenderFacesOnly(hdc, rc, verts, faces, opacity, fillColor)
	lineW := 2 * ss
	if lineW < 1 {
		lineW = 1
	}
	frontSegs := []mdSegment{}
	backSegs := []mdSegment{}
	for _, path := range edgePaths {
		for i := 0; i+1 < len(path.indices); i++ {
			a := verts[path.indices[i]]
			b := verts[path.indices[i+1]]
			depth := (a.y+b.y)*0.5 - path.depthCenter
			seg := mdSegment{a: mdProject(a, rc, scale), b: mdProject(b, rc, scale), depth: depth}
			if path.alwaysVisible || depth >= 0 {
				frontSegs = append(frontSegs, seg)
			} else {
				backSegs = append(backSegs, seg)
			}
		}
	}
	sort.Slice(backSegs, func(i, j int) bool { return backSegs[i].depth < backSegs[j].depth })
	if hiddenColor, ok := mdHiddenLineColor(opacity, lineColor); ok {
		for _, seg := range backSegs {
			mdDrawDashedLine(hdc, seg.a, seg.b, lineW, hiddenColor)
		}
	}
	for _, seg := range frontSegs {
		mdDrawPolyline(hdc, []POINT{seg.a, seg.b}, lineW, lineColor, false)
	}
}

func mdCross2D(o, a, b POINT) int64 {
	return int64(a.X-o.X)*int64(b.Y-o.Y) - int64(a.Y-o.Y)*int64(b.X-o.X)
}

func mdConvexHull(points []POINT) []POINT {
	if len(points) <= 1 {
		out := make([]POINT, len(points))
		copy(out, points)
		return out
	}
	pts := make([]POINT, len(points))
	copy(pts, points)
	sort.Slice(pts, func(i, j int) bool {
		if pts[i].X == pts[j].X {
			return pts[i].Y < pts[j].Y
		}
		return pts[i].X < pts[j].X
	})
	uniq := pts[:0]
	for _, p := range pts {
		if len(uniq) == 0 || uniq[len(uniq)-1].X != p.X || uniq[len(uniq)-1].Y != p.Y {
			uniq = append(uniq, p)
		}
	}
	if len(uniq) <= 1 {
		out := make([]POINT, len(uniq))
		copy(out, uniq)
		return out
	}
	lower := make([]POINT, 0, len(uniq))
	for _, p := range uniq {
		for len(lower) >= 2 && mdCross2D(lower[len(lower)-2], lower[len(lower)-1], p) <= 0 {
			lower = lower[:len(lower)-1]
		}
		lower = append(lower, p)
	}
	upper := make([]POINT, 0, len(uniq))
	for i := len(uniq) - 1; i >= 0; i-- {
		p := uniq[i]
		for len(upper) >= 2 && mdCross2D(upper[len(upper)-2], upper[len(upper)-1], p) <= 0 {
			upper = upper[:len(upper)-1]
		}
		upper = append(upper, p)
	}
	hull := append(lower[:len(lower)-1], upper[:len(upper)-1]...)
	return hull
}

func mdPointEqual(a, b POINT) bool {
	return a.X == b.X && a.Y == b.Y
}

func mdSegmentOnHull(a, b POINT, hull []POINT) bool {
	if len(hull) < 2 {
		return false
	}
	for i := 0; i < len(hull); i++ {
		j := (i + 1) % len(hull)
		if (mdPointEqual(hull[i], a) && mdPointEqual(hull[j], b)) || (mdPointEqual(hull[i], b) && mdPointEqual(hull[j], a)) {
			return true
		}
	}
	return false
}

func mdDrawProjectedHull(hdc uintptr, rc RECT, verts []mdVec3, ss int, lineColor uint32) {
	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.33
	pts := make([]POINT, len(verts))
	for i, v := range verts {
		pts[i] = mdProject(v, rc, scale)
	}
	hull := mdConvexHull(pts)
	if len(hull) >= 2 {
		mdDrawPolyline(hdc, hull, mdMaxInt(1, 2*ss), lineColor, true)
	}
}

func mdMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func mdMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func mdScaleVerts(verts []mdVec3, s float64) []mdVec3 {
	out := make([]mdVec3, len(verts))
	for i, v := range verts {
		out[i] = mdMul(v, s)
	}
	return out
}

func mdRotateVerts(verts []mdVec3, ax, ay, az float64) []mdVec3 {
	out := make([]mdVec3, len(verts))
	for i, v := range verts {
		out[i] = mdRotate(v, ax, ay, az)
	}
	return out
}

func mdNormalizedVerts(verts []mdVec3) []mdVec3 {
	maxLen := 0.0
	for _, v := range verts {
		l := math.Sqrt(v.x*v.x + v.y*v.y + v.z*v.z)
		if l > maxLen {
			maxLen = l
		}
	}
	if maxLen <= 0 {
		return verts
	}
	out := make([]mdVec3, len(verts))
	for i, v := range verts {
		out[i] = mdMul(v, 1.0/maxLen)
	}
	return out
}

func mdSortFaceAroundCenter(verts []mdVec3, indices []int, normalHint mdVec3) []int {
	if len(indices) <= 2 {
		out := make([]int, len(indices))
		copy(out, indices)
		return out
	}
	center := mdVec3{}
	for _, idx := range indices {
		center = mdAdd(center, verts[idx])
	}
	center = mdMul(center, 1.0/float64(len(indices)))
	n := mdNorm(normalHint)
	if math.Abs(n.x)+math.Abs(n.y)+math.Abs(n.z) < 1e-9 {
		n = mdNorm(center)
	}
	u := mdSub(verts[indices[0]], center)
	u = mdNorm(u)
	v := mdNorm(mdCross(n, u))
	type pair struct {
		ang float64
		idx int
	}
	arr := make([]pair, len(indices))
	for i, idx := range indices {
		d := mdSub(verts[idx], center)
		arr[i] = pair{math.Atan2(mdDot(d, v), mdDot(d, u)), idx}
	}
	sort.Slice(arr, func(i, j int) bool { return arr[i].ang < arr[j].ang })
	out := make([]int, len(indices))
	for i, p := range arr {
		out[i] = p.idx
	}
	return out
}

func mdBuildIcosahedron() ([]mdVec3, []mdFace) {
	phi := (1 + math.Sqrt(5)) / 2
	verts := []mdVec3{
		{-1, phi, 0}, {1, phi, 0}, {-1, -phi, 0}, {1, -phi, 0},
		{0, -1, phi}, {0, 1, phi}, {0, -1, -phi}, {0, 1, -phi},
		{phi, 0, -1}, {phi, 0, 1}, {-phi, 0, -1}, {-phi, 0, 1},
	}
	faces := []mdFace{
		{[]int{0, 11, 5}}, {[]int{0, 5, 1}}, {[]int{0, 1, 7}}, {[]int{0, 7, 10}}, {[]int{0, 10, 11}},
		{[]int{1, 5, 9}}, {[]int{5, 11, 4}}, {[]int{11, 10, 2}}, {[]int{10, 7, 6}}, {[]int{7, 1, 8}},
		{[]int{3, 9, 4}}, {[]int{3, 4, 2}}, {[]int{3, 2, 6}}, {[]int{3, 6, 8}}, {[]int{3, 8, 9}},
		{[]int{4, 9, 5}}, {[]int{2, 4, 11}}, {[]int{6, 2, 10}}, {[]int{8, 6, 7}}, {[]int{9, 8, 1}},
	}
	return mdNormalizedVerts(verts), faces
}

func mdBuildDodecahedron() ([]mdVec3, []mdFace) {
	iverts, ifaces := mdBuildIcosahedron()
	verts := make([]mdVec3, len(ifaces))
	for i, f := range ifaces {
		c := mdVec3{}
		for _, idx := range f.indices {
			c = mdAdd(c, iverts[idx])
		}
		verts[i] = mdMul(c, 1.0/float64(len(f.indices)))
	}
	verts = mdNormalizedVerts(verts)
	incident := make([][]int, len(iverts))
	for fi, f := range ifaces {
		for _, vidx := range f.indices {
			incident[vidx] = append(incident[vidx], fi)
		}
	}
	faces := make([]mdFace, 0, len(iverts))
	for vi, list := range incident {
		ordered := mdSortFaceAroundCenter(verts, list, iverts[vi])
		faces = append(faces, mdFace{ordered})
	}
	return verts, faces
}

func mdBuildFullerene() ([]mdVec3, []mdFace) {
	iverts, ifaces := mdBuildIcosahedron()
	type okey struct{ a, b int }
	orient := map[okey]int{}
	neighbors := make([]map[int]bool, len(iverts))
	for i := range neighbors {
		neighbors[i] = map[int]bool{}
	}
	verts := []mdVec3{}
	for _, f := range ifaces {
		for i := 0; i < len(f.indices); i++ {
			a := f.indices[i]
			b := f.indices[(i+1)%len(f.indices)]
			neighbors[a][b] = true
			neighbors[b][a] = true
			if _, ok := orient[okey{a, b}]; !ok {
				orient[okey{a, b}] = len(verts)
				verts = append(verts, mdAdd(mdMul(iverts[a], 2.0/3.0), mdMul(iverts[b], 1.0/3.0)))
			}
			if _, ok := orient[okey{b, a}]; !ok {
				orient[okey{b, a}] = len(verts)
				verts = append(verts, mdAdd(mdMul(iverts[b], 2.0/3.0), mdMul(iverts[a], 1.0/3.0)))
			}
		}
	}
	verts = mdNormalizedVerts(verts)
	faces := []mdFace{}
	// 12 pentagons around original vertices
	for vi := range iverts {
		list := []int{}
		for nb := range neighbors[vi] {
			list = append(list, orient[okey{vi, nb}])
		}
		ordered := mdSortFaceAroundCenter(verts, list, iverts[vi])
		faces = append(faces, mdFace{ordered})
	}
	// 20 hexagons from original triangular faces
	for _, f := range ifaces {
		a, b, c := f.indices[0], f.indices[1], f.indices[2]
		indices := []int{orient[okey{a, b}], orient[okey{b, a}], orient[okey{b, c}], orient[okey{c, b}], orient[okey{c, a}], orient[okey{a, c}]}
		faces = append(faces, mdFace{indices})
	}
	return verts, faces
}

func mdRenderPolyShape(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int, verts []mdVec3, faces []mdFace) {
	lineColor := multidimPreviewLineColor(s)
	fillColor := multidimPreviewFillColor(s)
	sz := float64(multidimSliderPos(s.sizeSlider)) / 200.0
	ax, ay, az, _ := multidimRotation(s)
	verts = mdScaleVerts(verts, sz)
	verts = mdRotateVerts(verts, ax, ay, az)
	mdRenderConvexPolyhedron(hdc, rc, verts, faces, multidimRenderOpacityIndex(s), ss, lineColor, fillColor)
}

// md4FaceContainsEdge reports whether a polygonal 2-face is incident to the
// tested edge.  Incident faces meet the line rather than covering it, so they
// must not reduce the line contrast.
func md4FaceContainsEdge(face mdFace4, edge mdEdge4) bool {
	hasA, hasB := false, false
	for _, idx := range face.indices {
		if idx == edge.a {
			hasA = true
		}
		if idx == edge.b {
			hasB = true
		}
	}
	return hasA && hasB
}

// md4FaceDepthAtXZ triangulates one projected polygon face and returns the Y
// depth at the supplied XZ camera ray.  Each polygon is counted at most once,
// even though a quad is internally tested as two triangles.
func md4FaceDepthAtXZ(point mdVec3, verts []mdVec3, face mdFace4) (float64, bool) {
	if len(face.indices) < 3 {
		return 0, false
	}
	a := verts[face.indices[0]]
	found := false
	frontY := math.Inf(-1)
	for i := 1; i+1 < len(face.indices); i++ {
		b := verts[face.indices[i]]
		c := verts[face.indices[i+1]]
		l1, l2, l3, inside := mdPointInsideProjectedTriangleXZ(point, a, b, c)
		if !inside {
			continue
		}
		y := l1*a.y + l2*b.y + l3*c.y
		if !found || y > frontY {
			frontY = y
			found = true
		}
	}
	return frontY, found
}

type md4FaceAccel struct {
	minX, maxX float64
	minZ, maxZ float64
	cols, rows int
	bins       [][]int
}

func md4ClampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// mdBuild4DFaceAccel bins projected 2-faces in the XZ camera plane.  Dense
// regular polytopes otherwise test every edge sample against every face; the
// grid reduces the 120-cell final pass from hundreds of millions of triangle
// tests to a small local candidate set while preserving the same layer rule.
func mdBuild4DFaceAccel(verts []mdVec3, faces []mdFace4, grid int) *md4FaceAccel {
	if len(verts) == 0 || len(faces) == 0 || grid < 2 {
		return nil
	}
	minX, maxX := math.Inf(1), math.Inf(-1)
	minZ, maxZ := math.Inf(1), math.Inf(-1)
	for _, v := range verts {
		if v.x < minX {
			minX = v.x
		}
		if v.x > maxX {
			maxX = v.x
		}
		if v.z < minZ {
			minZ = v.z
		}
		if v.z > maxZ {
			maxZ = v.z
		}
	}
	if !(maxX > minX) || !(maxZ > minZ) {
		return nil
	}
	padX := (maxX - minX) * 1e-6
	padZ := (maxZ - minZ) * 1e-6
	minX, maxX = minX-padX, maxX+padX
	minZ, maxZ = minZ-padZ, maxZ+padZ
	a := &md4FaceAccel{minX: minX, maxX: maxX, minZ: minZ, maxZ: maxZ, cols: grid, rows: grid, bins: make([][]int, grid*grid)}
	toX := func(x float64) int {
		return md4ClampInt(int((x-minX)/(maxX-minX)*float64(grid)), 0, grid-1)
	}
	toZ := func(z float64) int {
		return md4ClampInt(int((z-minZ)/(maxZ-minZ)*float64(grid)), 0, grid-1)
	}
	for fi, face := range faces {
		if len(face.indices) < 3 {
			continue
		}
		fminX, fmaxX := math.Inf(1), math.Inf(-1)
		fminZ, fmaxZ := math.Inf(1), math.Inf(-1)
		for _, vi := range face.indices {
			if vi < 0 || vi >= len(verts) {
				continue
			}
			v := verts[vi]
			if v.x < fminX {
				fminX = v.x
			}
			if v.x > fmaxX {
				fmaxX = v.x
			}
			if v.z < fminZ {
				fminZ = v.z
			}
			if v.z > fmaxZ {
				fmaxZ = v.z
			}
		}
		if math.IsInf(fminX, 0) || math.IsInf(fminZ, 0) {
			continue
		}
		x0, x1 := toX(fminX), toX(fmaxX)
		z0, z1 := toZ(fminZ), toZ(fmaxZ)
		for z := z0; z <= z1; z++ {
			for x := x0; x <= x1; x++ {
				idx := z*grid + x
				a.bins[idx] = append(a.bins[idx], fi)
			}
		}
	}
	return a
}

func (a *md4FaceAccel) candidates(point mdVec3) []int {
	if a == nil || point.x < a.minX || point.x > a.maxX || point.z < a.minZ || point.z > a.maxZ {
		return nil
	}
	x := md4ClampInt(int((point.x-a.minX)/(a.maxX-a.minX)*float64(a.cols)), 0, a.cols-1)
	z := md4ClampInt(int((point.z-a.minZ)/(a.maxZ-a.minZ)*float64(a.rows)), 0, a.rows-1)
	return a.bins[z*a.cols+x]
}

func md4OcclusionLayerCount(point mdVec3, verts []mdVec3, faces []mdFace4, edge mdEdge4, accel *md4FaceAccel, depthEps float64) int {
	if depthEps < 1e-7 {
		depthEps = 1e-7
	}
	layers := 0
	if accel != nil {
		for _, fi := range accel.candidates(point) {
			face := faces[fi]
			if md4FaceContainsEdge(face, edge) {
				continue
			}
			faceY, hit := md4FaceDepthAtXZ(point, verts, face)
			if hit && faceY > point.y+depthEps {
				layers++
			}
		}
		return layers
	}
	for _, face := range faces {
		if md4FaceContainsEdge(face, edge) {
			continue
		}
		faceY, hit := md4FaceDepthAtXZ(point, verts, face)
		if hit && faceY > point.y+depthEps {
			layers++
		}
	}
	return layers
}

type mdGPUOcclusionBatchQuery struct {
	point         mdVec3
	excludedFaces []int
}

func mdFaceIndexExcluded(faceIndex int, excluded []int) bool {
	for _, v := range excluded {
		if v == faceIndex {
			return true
		}
	}
	return false
}

func mdD3D11OcclusionLayerCounts(label string, verts []mdVec3, faces []mdFace4, queries []mdGPUOcclusionBatchQuery, accel *md4FaceAccel, depthEps float64) ([]int, bool) {
	if !mdD3D11OcclusionAvailable() || len(verts) == 0 || len(faces) == 0 || len(queries) == 0 {
		return nil, false
	}
	if depthEps < 1e-7 {
		depthEps = 1e-7
	}
	gpuFaces := make([]mdD3D11OcclusionFace, len(faces))
	for fi, face := range faces {
		if len(face.indices) < 3 || len(face.indices) > mdD3D11OcclusionMaxFaceVerts {
			return nil, false
		}
		gpuFaces[fi].Count = uint32(len(face.indices))
		for vi, index := range face.indices {
			if index < 0 || index >= len(verts) {
				return nil, false
			}
			v := verts[index]
			gpuFaces[fi].V[vi] = [4]float32{float32(v.x), float32(v.z), float32(v.y), 0}
		}
	}
	gpuQueries := make([]mdD3D11OcclusionQuery, len(queries))
	pairs := make([]mdD3D11OcclusionPair, 0, len(queries)*12)
	const maxPairs = 12_000_000
	for qi, query := range queries {
		gpuQueries[qi] = mdD3D11OcclusionQuery{X: float32(query.point.x), Z: float32(query.point.z), Y: float32(query.point.y), DepthEpsilon: float32(depthEps)}
		var candidates []int
		if accel != nil {
			candidates = accel.candidates(query.point)
		}
		if accel == nil {
			for fi := range faces {
				if mdFaceIndexExcluded(fi, query.excludedFaces) {
					continue
				}
				pairs = append(pairs, mdD3D11OcclusionPair{QueryIndex: uint32(qi), FaceIndex: uint32(fi)})
			}
		} else {
			for _, fi := range candidates {
				if fi < 0 || fi >= len(faces) || mdFaceIndexExcluded(fi, query.excludedFaces) {
					continue
				}
				pairs = append(pairs, mdD3D11OcclusionPair{QueryIndex: uint32(qi), FaceIndex: uint32(fi)})
			}
		}
		if len(pairs) > maxPairs {
			mdD3D11LogFallbackOnce("occlusion-pairs", fmt.Sprintf("pair budget exceeded label=%s pairs=%d", label, len(pairs)))
			return nil, false
		}
	}
	if len(pairs) == 0 {
		return make([]int, len(queries)), true
	}
	return mdD3D11RenderOcclusionCounts(label, gpuFaces, gpuQueries, pairs)
}

func md4IncidentFaceIndices(faces []mdFace4, edge mdEdge4) []int {
	out := make([]int, 0, 4)
	for fi, face := range faces {
		if md4FaceContainsEdge(face, edge) {
			out = append(out, fi)
		}
	}
	return out
}

func mdCollect4DEdgesWithFaceFadeGPU(state *multidimShapeEditorState, out *[]md4LineRenderSegment, rc RECT, ss int, scale float64, lineColor uint32, opacity int, verts []mdVec3, faces []mdFace4, edges []mdEdge4, exteriorEdges []bool, dense bool, preciseCrossings bool, accel *md4FaceAccel, label string) bool {
	if len(edges) == 0 || len(faces) == 0 || len(verts) == 0 {
		return false
	}
	type plan struct {
		a, b       mdVec3
		exterior   bool
		steps      int
		queryStart int
	}
	plans := make([]plan, 0, len(edges))
	queries := make([]mdGPUOcclusionBatchQuery, 0, len(edges)*16)
	depthEps := math.Max(1e-7, 0.40*float64(mdMaxInt(1, ss))/math.Max(scale, 1e-6))
	for edgeIndex, edge := range edges {
		a := verts[edge.a]
		b := verts[edge.b]
		pa := mdProject(a, rc, scale)
		pb := mdProject(b, rc, scale)
		pixelLength := math.Hypot(float64(pb.X-pa.X), float64(pb.Y-pa.Y))
		stepPixels := mdMaxInt(4, 4*ss)
		minSteps, maxSteps := 12, 192
		if dense {
			stepPixels = mdMaxInt(3*ss, 3)
			minSteps, maxSteps = 6, 160
			if preciseCrossings {
				stepPixels = mdMaxInt(2*ss, 2)
				minSteps, maxSteps = 10, 256
			}
		}
		if opacity == 0 {
			stepPixels = mdMaxInt(1, ss)
			minSteps = mdMaxInt(minSteps, 16)
			maxSteps = mdMaxInt(maxSteps, 512)
		}
		steps := int(math.Ceil(pixelLength / float64(stepPixels)))
		if steps < minSteps {
			steps = minSteps
		}
		if steps > maxSteps {
			steps = maxSteps
		}
		if steps < 1 {
			steps = 1
		}
		exterior := edgeIndex < len(exteriorEdges) && exteriorEdges[edgeIndex]
		queryStart := len(queries)
		if !exterior {
			excluded := md4IncidentFaceIndices(faces, edge)
			for i := 0; i < steps; i++ {
				midT := (float64(i) + 0.5) / float64(steps)
				queries = append(queries, mdGPUOcclusionBatchQuery{point: mdLerp3(a, b, midT), excludedFaces: excluded})
			}
		}
		plans = append(plans, plan{a: a, b: b, exterior: exterior, steps: steps, queryStart: queryStart})
	}
	counts := make([]int, len(queries))
	if len(queries) > 0 {
		var ok bool
		counts, ok = mdD3D11OcclusionLayerCounts(label, verts, faces, queries, accel, depthEps)
		if !ok {
			return false
		}
	}
	for _, p := range plans {
		layersBySegment := make([]int, p.steps)
		visible := make([]bool, p.steps)
		for i := 0; i < p.steps; i++ {
			layers := 0
			if !p.exterior {
				layers = counts[p.queryStart+i]
			}
			layersBySegment[i] = layers
			visible[i] = md4EdgeVisibleByCoverCount(layers, p.exterior)
		}
		if opacity == 0 && !p.exterior && p.steps >= 3 {
			filtered := append([]bool(nil), visible...)
			for i := 1; i+1 < p.steps; i++ {
				if !visible[i] && visible[i-1] && visible[i+1] {
					filtered[i] = true
				} else if visible[i] && !visible[i-1] && !visible[i+1] {
					filtered[i] = false
				}
			}
			visible = filtered
		}
		for i := 0; i < p.steps; i++ {
			if opacity == 0 && !visible[i] {
				continue
			}
			t0 := float64(i) / float64(p.steps)
			t1 := float64(i+1) / float64(p.steps)
			midT := (t0 + t1) * 0.5
			col := lineColor
			if opacity == 1 || opacity == 2 {
				col = md4DepthFadeColor(lineColor, layersBySegment[i])
			}
			*out = append(*out, md4LineRenderSegment{
				a:     mdProject(mdLerp3(p.a, p.b, t0), rc, scale),
				b:     mdProject(mdLerp3(p.a, p.b, t1), rc, scale),
				depth: mdLerp3(p.a, p.b, midT).y,
				color: col,
			})
		}
	}
	return true
}

func mdMixColorRefTowardWhite(base uint32, amount float64) uint32 {
	if amount < 0 {
		amount = 0
	}
	if amount > 1 {
		amount = 1
	}
	// Win32 COLORREF stores red in the low byte (0x00BBGGRR).
	r := float64(base & 0xFF)
	g := float64((base >> 8) & 0xFF)
	b := float64((base >> 16) & 0xFF)
	r += (255 - r) * amount
	g += (255 - g) * amount
	b += (255 - b) * amount
	return rgb(byte(r+0.5), byte(g+0.5), byte(b+0.5))
}

func md4DepthFadeColor(base uint32, layers int) uint32 {
	if layers <= 0 {
		return base
	}
	remainingContrast := mdDepthFadeFactor(layers)
	return mdMixColorRefTowardWhite(base, 1.0-remainingContrast)
}

func mdSurfaceDepthFadeColor(base uint32, layers int) uint32 {
	if layers <= 0 {
		return base
	}
	remainingContrast := mdSurfaceDepthFadeFactor(layers)
	return mdMixColorRefTowardWhite(base, 1.0-remainingContrast)
}

func mdLerp3(a, b mdVec3, t float64) mdVec3 {
	return mdVec3{x: a.x + (b.x-a.x)*t, y: a.y + (b.y-a.y)*t, z: a.z + (b.z-a.z)*t}
}

func md4CachedPen(cache map[uint32]uintptr, width int, color uint32) uintptr {
	if pen := cache[color]; pen != 0 {
		return pen
	}
	// A194: use a flat-ended geometric pen for depth-subdivided 4D edges.
	// CreatePen's wide default caps extend beyond every short segment endpoint;
	// many segments meeting at a projected polytope vertex therefore formed the
	// visible star/spur pattern reported in A193.  Flat caps end exactly at the
	// classified segment boundary and preserve the true edge length.
	brush := mdLogBrushA194{Style: mdBSSolid, Color: color}
	style := uintptr(mdPSGeometric | PS_SOLID | mdPSEndcapFlat | mdPSJoinMiter)
	pen, _, _ := procExtCreatePenA194.Call(style, uintptr(width), uintptr(unsafe.Pointer(&brush)), 0, 0)
	if pen == 0 {
		pen, _, _ = procCreatePen.Call(PS_SOLID, uintptr(width), uintptr(color))
	}
	if pen != 0 {
		cache[color] = pen
	}
	return pen
}

func mdDrawLineWithPen(hdc uintptr, a, b POINT, pen uintptr) {
	if pen == 0 {
		return
	}
	old, _, _ := procSelectObject.Call(hdc, pen)
	procMoveToEx.Call(hdc, uintptr(a.X), uintptr(a.Y), 0)
	procLineTo.Call(hdc, uintptr(b.X), uintptr(b.Y))
	procSelectObject.Call(hdc, old)
}

// md4LineRenderSegment is a short projected line piece with its local camera
// depth.  Sorting these pieces globally fixes dense-polytope crossings where
// drawing whole edges by their average depth could put the rear line on top.
type md4LineRenderSegment struct {
	a, b  POINT
	depth float64
	color uint32
}

// mdCollect4DEdgeWithFaceFade applies the same front-view rule used by the
// simpler 3D solids. At each short edge interval it asks only two questions:
// is this an outer-contour edge, and how many projected 2-faces lie in front?
// Opaque mode keeps exterior or zero-cover intervals. Semi-transparent and
// transparent modes retain max(8%, 0.70^n) contrast for n covering faces.
func mdCollect4DEdgeWithFaceFade(state *multidimShapeEditorState, out *[]md4LineRenderSegment, rc RECT, ss int, scale float64, lineColor uint32, opacity int, verts []mdVec3, faces []mdFace4, edge mdEdge4, exterior bool, interactive bool, dense bool, preciseCrossings bool, accel *md4FaceAccel) {
	a := verts[edge.a]
	b := verts[edge.b]
	pa := mdProject(a, rc, scale)
	pb := mdProject(b, rc, scale)
	pixelLength := math.Hypot(float64(pb.X-pa.X), float64(pb.Y-pa.Y))

	stepPixels := mdMaxInt(4, 4*ss)
	minSteps, maxSteps := 12, 192
	if interactive {
		stepPixels = mdMaxInt(8, 8*ss)
		minSteps, maxSteps = 6, 64
	}
	if dense {
		stepPixels = mdMaxInt(3*ss, 3)
		minSteps, maxSteps = 6, 160
		if preciseCrossings && !interactive {
			stepPixels = mdMaxInt(2*ss, 2)
			minSteps, maxSteps = 10, 256
		}
		if interactive {
			stepPixels = mdMaxInt(12, 12*ss)
			minSteps, maxSteps = 3, 20
		}
	}
	if opacity == 0 && !interactive {
		// One final-display pixel per decision interval. This makes a cover-count
		// transition local instead of clipping a long edge chunk at once.
		stepPixels = mdMaxInt(1, ss)
		minSteps = mdMaxInt(minSteps, 16)
		maxSteps = mdMaxInt(maxSteps, 512)
	}
	steps := int(math.Ceil(pixelLength / float64(stepPixels)))
	if steps < minSteps {
		steps = minSteps
	}
	if steps > maxSteps {
		steps = maxSteps
	}
	if steps < 1 {
		steps = 1
	}

	layersBySegment := make([]int, steps)
	visible := make([]bool, steps)
	depthEps := math.Max(1e-7, 0.40*float64(mdMaxInt(1, ss))/math.Max(scale, 1e-6))
	for i := 0; i < steps; i++ {
		midT := (float64(i) + 0.5) / float64(steps)
		mid := mdLerp3(a, b, midT)
		layers := 0
		if !exterior {
			layers = md4OcclusionLayerCount(mid, verts, faces, edge, accel, depthEps)
		}
		layersBySegment[i] = layers
		visible[i] = md4EdgeVisibleByCoverCount(layers, exterior)
	}

	if opacity == 0 && !exterior && steps >= 3 {
		// Numerical boundaries can create a one-pixel hole or inward stub when a
		// sampled interval lands exactly on a projected face edge. Remove only a
		// single isolated interval; genuine multi-pixel visibility changes remain.
		filtered := append([]bool(nil), visible...)
		for i := 1; i+1 < steps; i++ {
			if !visible[i] && visible[i-1] && visible[i+1] {
				filtered[i] = true
			} else if visible[i] && !visible[i-1] && !visible[i+1] {
				filtered[i] = false
			}
		}
		visible = filtered
	}

	for i := 0; i < steps; i++ {
		if opacity == 0 && !visible[i] {
			continue
		}
		t0 := float64(i) / float64(steps)
		t1 := float64(i+1) / float64(steps)
		midT := (t0 + t1) * 0.5
		mid := mdLerp3(a, b, midT)
		col := lineColor
		if opacity == 1 || opacity == 2 {
			col = md4DepthFadeColor(lineColor, layersBySegment[i])
		}
		*out = append(*out, md4LineRenderSegment{
			a:     mdProject(mdLerp3(a, b, t0), rc, scale),
			b:     mdProject(mdLerp3(a, b, t1), rc, scale),
			depth: mid.y,
			color: col,
		})
	}
}

func mdDrawSorted4DLineSegments(hdc uintptr, ss int, segments []md4LineRenderSegment) {
	if len(segments) == 0 {
		return
	}
	// Camera looks from +Y. Draw far local pieces first, then near pieces.
	sort.SliceStable(segments, func(i, j int) bool { return segments[i].depth < segments[j].depth })
	lineW := mdMaxInt(1, 2*ss)
	penCache := make(map[uint32]uintptr, 16)
	var currentPen, oldPen uintptr
	for _, seg := range segments {
		pen := md4CachedPen(penCache, lineW, seg.color)
		if pen == 0 {
			continue
		}
		if pen != currentPen {
			prev, _, _ := procSelectObject.Call(hdc, pen)
			if currentPen == 0 {
				oldPen = prev
			}
			currentPen = pen
		}
		procMoveToEx.Call(hdc, uintptr(seg.a.X), uintptr(seg.a.Y), 0)
		procLineTo.Call(hdc, uintptr(seg.b.X), uintptr(seg.b.Y))
	}
	if currentPen != 0 && oldPen != 0 {
		procSelectObject.Call(hdc, oldPen)
	}
	for _, pen := range penCache {
		if pen != 0 {
			procDeleteObject.Call(pen)
		}
	}
}

type mdDepthRasterVertex struct {
	x, y  float64
	depth float64
}

func mdDepthRasterEdge(a, b, p mdDepthRasterVertex) float64 {
	return (p.x-a.x)*(b.y-a.y) - (p.y-a.y)*(b.x-a.x)
}

// mdRenderOpaque4DFacesZBuffer rasterizes projected 2-faces with a local
// per-pixel depth buffer. A single painter order cannot correctly sort 4D
// faces after projection because their depth order can reverse inside one
// polygon. The z-buffer chooses the front face independently at every pixel.
func mdRenderOpaque4DFacesZBuffer(s *multidimShapeEditorState, hdc uintptr, rc RECT, verts []mdVec3, faces []mdFace4, scale float64, fillColor uint32) bool {
	return mdRenderFaceZBufferWithCapAlpha(s, hdc, rc, verts, faces, scale, fillColor, 1.0, 1.0, "opaque-4d-faces")
}

func mdRenderOpaque4DFacesZBufferWithCap(s *multidimShapeEditorState, hdc uintptr, rc RECT, verts []mdVec3, faces []mdFace4, scale float64, fillColor uint32, maxBrightness float64) bool {
	return mdRenderFaceZBufferWithCapAlpha(s, hdc, rc, verts, faces, scale, fillColor, maxBrightness, 1.0, "opaque-4d-faces")
}

func mdRenderTranslucent4DFacesZBuffer(s *multidimShapeEditorState, hdc uintptr, rc RECT, verts []mdVec3, faces []mdFace4, scale float64, fillColor uint32, maxBrightness float64, label string) bool {
	return mdRenderFaceZBufferWithCapAlpha(s, hdc, rc, verts, faces, scale, fillColor, maxBrightness, 0.20, label)
}

func mdRenderFaceZBufferWithCapAlpha(s *multidimShapeEditorState, hdc uintptr, rc RECT, verts []mdVec3, faces []mdFace4, scale float64, fillColor uint32, maxBrightness, alpha float64, label string) bool {
	if s != nil {
		s.opaqueFaceValid = false
	}
	if s == nil || hdc == 0 || len(verts) == 0 || len(faces) == 0 {
		return false
	}
	if maxBrightness <= 0 || maxBrightness > 1 {
		maxBrightness = 1
	}
	if alpha <= 0 || alpha > 1 {
		alpha = 1
	}
	if label == "" {
		label = "faces"
	}
	cx := float64(rc.Left+rc.Right) * 0.5
	cy := float64(rc.Top+rc.Bottom) * 0.5
	projected := make([]mdDepthRasterVertex, len(verts))
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for i, v := range verts {
		p := mdDepthRasterVertex{x: cx + v.x*scale, y: cy - v.z*scale, depth: v.y}
		projected[i] = p
		minX = math.Min(minX, p.x)
		maxX = math.Max(maxX, p.x)
		minY = math.Min(minY, p.y)
		maxY = math.Max(maxY, p.y)
	}
	left := mdMaxInt(int(rc.Left), int(math.Floor(minX))-2)
	top := mdMaxInt(int(rc.Top), int(math.Floor(minY))-2)
	right := mdMinInt(int(rc.Right), int(math.Ceil(maxX))+3)
	bottom := mdMinInt(int(rc.Bottom), int(math.Ceil(maxY))+3)
	w, h := right-left, bottom-top
	if w <= 0 || h <= 0 {
		return false
	}
	// A206: offload the expensive opaque 4-D/Klein face fill and depth test to
	// Direct3D 11. The exact CPU edge-occlusion pass is intentionally left
	// untouched and is drawn after this bitmap. If GPU initialization, shader
	// compilation, rendering or readback fails, execution falls through to the
	// byte-for-byte compatible CPU rasterizer below.
	faceRenderStarted := time.Now()
	if mdD3D11FaceAvailable() {
		gpuVerts := mdBuildD3D11FaceVertices(verts, faces, projected, left, top, w, h, fillColor, multidimPreviewBackgroundColor(s), maxBrightness, alpha)
		if gpuPixels, ok := mdD3D11RenderFaceTriangles(label, w, h, gpuVerts, multidimPreviewBackgroundColor(s)); ok {
			dst := RECT{Left: int32(left), Top: int32(top), Right: int32(right), Bottom: int32(bottom)}
			if multidimBlitBGRA(s, hdc, dst, gpuPixels, w, h, "d3d11-"+label) {
				s.opaqueFaceLeft, s.opaqueFaceTop = left, top
				s.opaqueFaceW, s.opaqueFaceH = w, h
				s.opaqueFaceValid = true
				return true
			}
		}
	}
	pixelCount := w * h
	needBytes := pixelCount * 4
	if cap(s.opaqueFacePixels) < needBytes {
		s.opaqueFacePixels = make([]byte, needBytes)
	} else {
		s.opaqueFacePixels = s.opaqueFacePixels[:needBytes]
	}
	if cap(s.opaqueFaceDepth) < pixelCount {
		s.opaqueFaceDepth = make([]float32, pixelCount)
	} else {
		s.opaqueFaceDepth = s.opaqueFaceDepth[:pixelCount]
	}
	pixels := s.opaqueFacePixels
	depth := s.opaqueFaceDepth
	rasterBackground := multidimPreviewBackgroundColor(s)
	bgR := byte(rasterBackground & 0xFF)
	bgG := byte((rasterBackground >> 8) & 0xFF)
	bgB := byte((rasterBackground >> 16) & 0xFF)
	for i := 0; i < pixelCount; i++ {
		base := i * 4
		pixels[base+0] = bgB
		pixels[base+1] = bgG
		pixels[base+2] = bgR
		pixels[base+3] = 0
		depth[i] = float32(-math.MaxFloat32)
	}

	incomingLight := mdSceneIncomingLightDirection()
	coveredPixels := 0
	for _, face := range faces {
		if len(face.indices) < 3 {
			continue
		}
		a3 := verts[face.indices[0]]
		b3 := verts[face.indices[1]]
		c3 := verts[face.indices[2]]
		n := mdNorm(mdCross(mdSub(b3, a3), mdSub(c3, a3)))
		if n.y < 0 {
			n = mdMul(n, -1)
		}
		brightness := 0.40 + 0.60*math.Max(0, mdDot(n, incomingLight))
		if brightness > maxBrightness {
			brightness = maxBrightness
		}
		col := mdBlendedColorOnBackground(fillColor, rasterBackground, brightness, alpha)
		r := byte(col & 0xFF)
		g := byte((col >> 8) & 0xFF)
		b := byte((col >> 16) & 0xFF)

		root := projected[face.indices[0]]
		for tri := 1; tri+1 < len(face.indices); tri++ {
			v0 := root
			v1 := projected[face.indices[tri]]
			v2 := projected[face.indices[tri+1]]
			area := mdDepthRasterEdge(v0, v1, v2)
			if math.Abs(area) < 1e-9 {
				continue
			}
			tMinX := mdMaxInt(left, int(math.Floor(math.Min(v0.x, math.Min(v1.x, v2.x)))))
			tMaxX := mdMinInt(right-1, int(math.Ceil(math.Max(v0.x, math.Max(v1.x, v2.x)))))
			tMinY := mdMaxInt(top, int(math.Floor(math.Min(v0.y, math.Min(v1.y, v2.y)))))
			tMaxY := mdMinInt(bottom-1, int(math.Ceil(math.Max(v0.y, math.Max(v1.y, v2.y)))))
			positive := area > 0
			invArea := 1.0 / area
			for py := tMinY; py <= tMaxY; py++ {
				for px := tMinX; px <= tMaxX; px++ {
					p := mdDepthRasterVertex{x: float64(px) + 0.5, y: float64(py) + 0.5}
					w0 := mdDepthRasterEdge(v1, v2, p)
					w1 := mdDepthRasterEdge(v2, v0, p)
					w2 := mdDepthRasterEdge(v0, v1, p)
					if positive {
						if w0 < -1e-8 || w1 < -1e-8 || w2 < -1e-8 {
							continue
						}
					} else if w0 > 1e-8 || w1 > 1e-8 || w2 > 1e-8 {
						continue
					}
					d := (w0*v0.depth + w1*v1.depth + w2*v2.depth) * invArea
					idx := (py-top)*w + (px - left)
					if d <= float64(depth[idx])+1e-7 {
						continue
					}
					if depth[idx] < float32(-math.MaxFloat32*0.5) {
						coveredPixels++
					}
					depth[idx] = float32(d)
					base := idx * 4
					pixels[base+0] = b
					pixels[base+1] = g
					pixels[base+2] = r
					pixels[base+3] = 0
				}
			}
		}
	}
	if coveredPixels == 0 {
		s.opaqueFaceValid = false
		return false
	}
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{
		BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(w), BiHeight: -int32(h),
		BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB,
	}}
	blitResult, _, _ := procStretchDIBits.Call(hdc, uintptr(left), uintptr(top), uintptr(w), uintptr(h), 0, 0, uintptr(w), uintptr(h), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, SRCCOPY)
	if blitResult == 0 || blitResult == ^uintptr(0) {
		s.opaqueFaceValid = false
		return false
	}
	s.opaqueFaceLeft, s.opaqueFaceTop = left, top
	s.opaqueFaceW, s.opaqueFaceH = w, h
	s.opaqueFaceValid = true
	reason := mdD3D11PipelineReason("faces")
	if reason == "" {
		reason = "D3D11 render or readback failed"
	}
	mdD3D11LogFallbackOnce("faces", reason)
	logf("MULTIDIM_RENDER shape=%s backend=CPU size=%dx%d faces=%d reason=%q total=%s fallback=true", label, w, h, len(faces), reason, time.Since(faceRenderStarted))
	return true
}

func multidimPaintRegular4D(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int, verts4 []mdVec4, edges []mdEdge4, faces []mdFace4, scaleBase float64) {
	ax, ay, az, aw := multidimRotation(s)
	verts3 := make([]mdVec3, len(verts4))
	for i, v := range verts4 {
		v = mdRot4XW(v, aw)
		verts3[i] = mdRotate(mdProject4To3(v, 4.0), ax, ay, az)
	}

	sizeFactor := float64(multidimSliderPos(s.sizeSlider)) / 160.0
	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * scaleBase * sizeFactor
	opacity := multidimRenderOpacityIndex(s)
	fillColor := multidimPreviewFillColor(s)
	lineColor := multidimPreviewLineColor(s)
	if s != nil {
		s.opaqueFaceValid = false
	}

	renderedFaces := false
	if opacity == 0 {
		renderedFaces = mdRenderOpaque4DFacesZBuffer(s, hdc, rc, verts3, faces, scale, fillColor)
	} else if opacity == 1 {
		renderedFaces = mdRenderTranslucent4DFacesZBuffer(s, hdc, rc, verts3, faces, scale, fillColor, 1.0, "translucent-4d-faces")
	}
	if opacity != 2 && !renderedFaces {
		alpha := 1.0
		if opacity == 1 {
			alpha = 0.20
		}
		incomingLight := mdSceneIncomingLightDirection()
		renders := make([]mdFaceRender, 0, len(faces))
		for _, face := range faces {
			if len(face.indices) < 3 {
				continue
			}
			pts3 := make([]mdVec3, len(face.indices))
			avg := mdVec3{}
			for i, idx := range face.indices {
				pts3[i] = verts3[idx]
				avg = mdAdd(avg, verts3[idx])
			}
			avg = mdMul(avg, 1.0/float64(len(pts3)))
			n := mdNorm(mdCross(mdSub(pts3[1], pts3[0]), mdSub(pts3[2], pts3[0])))
			if n.y < 0 {
				n = mdMul(n, -1)
			}
			brightness := 0.40 + 0.60*math.Max(0, mdDot(n, incomingLight))
			pts2 := make([]POINT, len(pts3))
			for i := range pts3 {
				pts2[i] = mdProject(pts3[i], rc, scale)
			}
			renders = append(renders, mdFaceRender{pts: pts2, depth: avg.y, bright: brightness})
		}
		sort.Slice(renders, func(i, j int) bool { return renders[i].depth < renders[j].depth })
		for _, face := range renders {
			col := mdBlendedColor(fillColor, face.bright, alpha)
			mdFillPolygon(hdc, face.pts, col, col)
		}
	}

	dense := len(edges) > 300
	preciseCrossings := len(faces) >= 1000 && len(edges) <= 800
	var accel *md4FaceAccel
	if dense {
		accel = mdBuild4DFaceAccel(verts3, faces, 28)
	}
	exteriorEdges := mdProjectedExteriorEdgeFlagsXZ(verts3, edges)
	segments := make([]md4LineRenderSegment, 0, len(edges)*8)
	if !mdCollect4DEdgesWithFaceFadeGPU(s, &segments, rc, ss, scale, lineColor, opacity, verts3, faces, edges, exteriorEdges, dense, preciseCrossings, accel, "edge-occlusion-4d") {
		for edgeIndex, edge := range edges {
			exterior := edgeIndex < len(exteriorEdges) && exteriorEdges[edgeIndex]
			mdCollect4DEdgeWithFaceFade(s, &segments, rc, ss, scale, lineColor, opacity, verts3, faces, edge, exterior, false, dense, preciseCrossings, accel)
		}
	}
	mdDrawSorted4DLineSegments(hdc, ss, segments)
}

func multidimPaintTesseract(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	g := mdCachedTesseractGeometry
	multidimPaintRegular4D(s, hdc, rc, ss, g.verts, g.edges, g.faces, 0.15)
}

func multidimPaintRegular5Cell(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	g := mdCached5CellGeometry
	multidimPaintRegular4D(s, hdc, rc, ss, g.verts, g.edges, g.faces, 0.24)
}

func multidimPaintRegular16Cell(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	g := mdCached16CellGeometry
	multidimPaintRegular4D(s, hdc, rc, ss, g.verts, g.edges, g.faces, 0.24)
}

func multidimPaintRegular24Cell(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	g := mdCached24CellGeometry
	multidimPaintRegular4D(s, hdc, rc, ss, g.verts, g.edges, g.faces, 0.20)
}

func multidimPaintRegular120Cell(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	g := mdRegular120CellGeometry()
	multidimPaintRegular4D(s, hdc, rc, ss, g.verts, g.edges, g.faces, 0.205)
}

func multidimPaintRegular600Cell(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	g := mdRegular600CellGeometry()
	multidimPaintRegular4D(s, hdc, rc, ss, g.verts, g.edges, g.faces, 0.215)
}

func mdKleinSelectedGridEdges(mode, uSegments, vSegments int, uEdges, vEdges []mdEdge4) []mdEdge4 {
	switch mode {
	case mdKleinGridNone:
		return nil
	case mdKleinGridFull:
		out := make([]mdEdge4, 0, len(uEdges)+len(vEdges))
		out = append(out, uEdges...)
		out = append(out, vEdges...)
		return out
	default:
		out := make([]mdEdge4, 0, 4*uSegments+8*vSegments)
		vStride := mdMaxInt(1, vSegments/4)
		uStride := mdMaxInt(1, uSegments/8)
		for iu := 0; iu < uSegments; iu++ {
			for iv := 0; iv < vSegments; iv++ {
				idx := iu*vSegments + iv
				if iv%vStride == 0 {
					out = append(out, uEdges[idx])
				}
				if iu%uStride == 0 {
					out = append(out, vEdges[idx])
				}
			}
		}
		return out
	}
}

func multidimPaintKleinBottle(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	if s == nil {
		return
	}
	density := mdClampKleinSubdivision(multidimSliderPos(s.depthSlider))
	uSegments, vSegments := density*2, density
	if multidimRenderSmoothEnabled(s) {
		uSegments = density * 3
		vSegments = density * 3 / 2
	}
	// Even V counts make the v -> -v seam exact on the discrete grid.
	if vSegments%2 != 0 {
		vSegments++
	}

	ringRadius := float64(multidimSliderPos(s.widthSlider)) / 100.0
	tubeRadius := float64(multidimSliderPos(s.heightSlider)) / 100.0
	if ringRadius < tubeRadius*1.15 {
		ringRadius = tubeRadius * 1.15
	}
	verts4, faces, uEdges, vEdges := mdBuildKleinBottle4D(uSegments, vSegments, ringRadius, tubeRadius)
	// A159's one uniform 4D normalization made the fourth-dimensional lift
	// only about one tenth as large as the visible bottle. It prevented the
	// projection singularity, but also hid the sheet exchange the user wanted
	// to see. A187 keeps independent XYZ/W scaling, applies an XW+YZ
	// isoclinic double rotation, and reveals W with bounded perspective plus a
	// small oblique lift. At aw=0 the classical bottle remains exact; away from
	// zero there is no single fixed 3D axis, so the turn visibly changes the
	// neck/body relationship instead of resembling a Y-axis rotation.
	verts4 = mdPrepareKleinTurn4D(verts4, 2.35, 1.60)
	ax, ay, az, aw := multidimRotation(s)
	verts3 := make([]mdVec3, len(verts4))
	for i, v := range verts4 {
		v = mdRot4KleinW(v, aw)
		verts3[i] = mdRotate(mdProjectKlein4To3(v, aw), ax, ay, az)
	}

	// Recenter the projected surface in XZ, then fit it to the preview.
	minX, maxX := math.Inf(1), math.Inf(-1)
	minZ, maxZ := math.Inf(1), math.Inf(-1)
	for _, v := range verts3 {
		minX = math.Min(minX, v.x)
		maxX = math.Max(maxX, v.x)
		minZ = math.Min(minZ, v.z)
		maxZ = math.Max(maxZ, v.z)
	}
	centerX, centerZ := (minX+maxX)*0.5, (minZ+maxZ)*0.5
	for i := range verts3 {
		verts3[i].x -= centerX
		verts3[i].z -= centerZ
	}
	rangeX, rangeZ := math.Max(1e-6, maxX-minX), math.Max(1e-6, maxZ-minZ)
	scale := 0.84 * math.Min(float64(rc.Right-rc.Left)/rangeX, float64(rc.Bottom-rc.Top)/rangeZ)

	opacity := multidimRenderOpacityIndex(s)
	fillColor := multidimPreviewFillColor(s)
	lineColor := multidimPreviewLineColor(s)
	s.opaqueFaceValid = false
	renderedOpaque := false
	renderedTranslucent := false
	if opacity == 0 {
		// The Klein bottle is non-orientable and can present a nearly uniformly
		// lit visible envelope.  Cap the opaque highlight so a white fill never
		// becomes indistinguishable from the white preview background.  The
		// z-buffer now reports an empty/failed raster, allowing the two-sided
		// painter below to act as a guaranteed fallback rather than leaving only
		// grid lines at a special angle.
		renderedOpaque = mdRenderOpaque4DFacesZBufferWithCap(s, hdc, rc, verts3, faces, scale, fillColor, 0.92)
	} else if opacity == 1 {
		renderedTranslucent = mdRenderTranslucent4DFacesZBuffer(s, hdc, rc, verts3, faces, scale, fillColor, 0.92, "translucent-klein-faces")
	}
	if (opacity == 0 && !renderedOpaque) || (opacity == 1 && !renderedTranslucent) {
		alpha := 1.0
		if opacity == 1 {
			alpha = 0.20
		}
		incomingLight := mdSceneIncomingLightDirection()
		renders := make([]mdFaceRender, 0, len(faces))
		for _, face := range faces {
			if len(face.indices) < 3 {
				continue
			}
			pts3 := make([]mdVec3, len(face.indices))
			avg := mdVec3{}
			for i, idx := range face.indices {
				pts3[i] = verts3[idx]
				avg = mdAdd(avg, pts3[i])
			}
			avg = mdMul(avg, 1.0/float64(len(pts3)))
			n := mdNorm(mdCross(mdSub(pts3[1], pts3[0]), mdSub(pts3[2], pts3[0])))
			// A Klein bottle has no global outside. Choose the camera-facing
			// representative independently for stable two-sided lighting.
			if n.y < 0 {
				n = mdMul(n, -1)
			}
			brightness := 0.40 + 0.60*math.Max(0, mdDot(n, incomingLight))
			if opacity == 0 && brightness > 0.92 {
				brightness = 0.92
			}
			pts2 := make([]POINT, len(pts3))
			for i := range pts3 {
				pts2[i] = mdProject(pts3[i], rc, scale)
			}
			renders = append(renders, mdFaceRender{pts: pts2, depth: avg.y, bright: brightness})
		}
		sort.Slice(renders, func(i, j int) bool { return renders[i].depth < renders[j].depth })
		for _, face := range renders {
			col := mdBlendedColor(fillColor, face.bright, alpha)
			mdFillPolygon(hdc, face.pts, col, col)
		}
	}

	edges := mdKleinSelectedGridEdges(multidimEditorKleinGridMode(s), uSegments, vSegments, uEdges, vEdges)
	if len(edges) == 0 {
		return
	}
	accel := mdBuild4DFaceAccel(verts3, faces, 28)
	segments := make([]md4LineRenderSegment, 0, len(edges)*6)
	exterior := make([]bool, len(edges))
	if !mdCollect4DEdgesWithFaceFadeGPU(s, &segments, rc, ss, scale, lineColor, opacity, verts3, faces, edges, exterior, true, false, accel, "edge-occlusion-klein") {
		for _, edge := range edges {
			mdCollect4DEdgeWithFaceFade(s, &segments, rc, ss, scale, lineColor, opacity, verts3, faces, edge, false, false, true, false, accel)
		}
	}
	mdDrawSorted4DLineSegments(hdc, ss, segments)
}

func multidimPaintTetrahedron(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	verts, faces := mdBuildTetrahedronGeometry()
	mdRenderPolyShape(s, hdc, rc, ss, mdNormalizedVerts(verts), faces)
}

func multidimPaintOctahedron(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	verts, faces := mdBuildOctahedronGeometry()
	mdRenderPolyShape(s, hdc, rc, ss, verts, faces)
}

func multidimPaintIcosahedron(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	verts, faces := mdBuildIcosahedron()
	mdRenderPolyShape(s, hdc, rc, ss, verts, faces)
}

func multidimPaintDodecahedron(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	verts, faces := mdBuildDodecahedron()
	mdRenderPolyShape(s, hdc, rc, ss, verts, faces)
}

func multidimPaintFullerene(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	verts, faces := mdBuildFullerene()
	mdRenderPolyShape(s, hdc, rc, ss, verts, faces)
}

func mdAppendSteinmetzStripFaces(faces *[]mdFace, a, b []int) {
	// Rows whose curve radius becomes zero are represented by one real vertex,
	// not segments+1 coincident copies.  Connecting a collapsed row to the next
	// row with a triangle fan prevents the degenerate quads that used to leave an
	// unpainted pinhole at an opaque Steinmetz vertex.
	if len(a) == 0 || len(b) == 0 {
		return
	}
	switch {
	case len(a) == 1 && len(b) > 1:
		for j := 0; j+1 < len(b); j++ {
			*faces = append(*faces, mdFace{[]int{a[0], b[j], b[j+1]}})
		}
	case len(b) == 1 && len(a) > 1:
		for j := 0; j+1 < len(a); j++ {
			*faces = append(*faces, mdFace{[]int{a[j], b[0], a[j+1]}})
		}
	case len(a) > 1 && len(b) > 1:
		n := mdMinInt(len(a), len(b)) - 1
		for j := 0; j < n; j++ {
			*faces = append(*faces, mdFace{[]int{a[j], b[j], b[j+1], a[j+1]}})
		}
	}
}

func mdBuildSteinmetz2Surface(segments, strips int) ([]mdVec3, []mdFace) {
	// Intersection of two equal perpendicular cylinders:
	//   x²+z² <= 1  (axis y)
	//   y²+z² <= 1  (axis x)
	// Its boundary consists of exactly four curved patches.  The z=±1 endpoints
	// are genuine common vertices, so they are built as triangle fans rather
	// than zero-area quads with many coincident indices.
	verts := []mdVec3{}
	faces := []mdFace{}
	addPatch := func(family int, sign float64) {
		rows := make([][]int, strips+1)
		for i := 0; i <= strips; i++ {
			z := -1.0 + 2.0*float64(i)/float64(strips)
			q := math.Sqrt(math.Max(0, 1-z*z))
			if q < 1e-10 {
				rows[i] = []int{len(verts)}
				verts = append(verts, mdVec3{0, 0, z})
				continue
			}
			rows[i] = make([]int, segments+1)
			for j := 0; j <= segments; j++ {
				t := -q + 2*q*float64(j)/float64(segments)
				var pt mdVec3
				if family == 0 {
					pt = mdVec3{t, sign * q, z}
				} else {
					pt = mdVec3{sign * q, t, z}
				}
				rows[i][j] = len(verts)
				verts = append(verts, pt)
			}
		}
		for i := 0; i < strips; i++ {
			mdAppendSteinmetzStripFaces(&faces, rows[i], rows[i+1])
		}
	}
	addPatch(0, 1)
	addPatch(0, -1)
	addPatch(1, 1)
	addPatch(1, -1)
	return verts, faces
}

func mdBuildSteinmetz3Surface(segments, strips int) ([]mdVec3, []mdFace) {
	verts := []mdVec3{}
	faces := []mdFace{}
	addPatch := func(axis int, sign float64) {
		rows := make([][]int, strips+1)
		for i := 0; i <= strips; i++ {
			theta := 2 * math.Pi * float64(i) / float64(strips)
			a := math.Cos(theta)
			b := math.Sin(theta)
			limit := math.Min(math.Abs(a), math.Abs(b))
			if limit < 1e-10 {
				var p mdVec3
				switch axis {
				case 0:
					p = mdVec3{0, sign * a, sign * b}
				case 1:
					p = mdVec3{sign * a, 0, sign * b}
				default:
					p = mdVec3{sign * a, sign * b, 0}
				}
				rows[i] = []int{len(verts)}
				verts = append(verts, p)
				continue
			}
			rows[i] = make([]int, segments+1)
			for j := 0; j <= segments; j++ {
				t := -limit + 2*limit*float64(j)/float64(segments)
				var p mdVec3
				switch axis {
				case 0: // x-cylinder: y²+z²=1
					p = mdVec3{t, sign * a, sign * b}
				case 1: // y-cylinder: x²+z²=1
					p = mdVec3{sign * a, t, sign * b}
				default: // z-cylinder: x²+y²=1
					p = mdVec3{sign * a, sign * b, t}
				}
				rows[i][j] = len(verts)
				verts = append(verts, p)
			}
		}
		for i := 0; i < strips; i++ {
			mdAppendSteinmetzStripFaces(&faces, rows[i], rows[i+1])
		}
	}
	for axis := 0; axis < 3; axis++ {
		addPatch(axis, 1)
		addPatch(axis, -1)
	}
	return verts, faces
}

func mdRenderSmoothSurface(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int, verts []mdVec3, faces []mdFace, drawHull bool) {
	lineColor := multidimPreviewLineColor(s)
	fillColor := multidimPreviewFillColor(s)
	sz := float64(multidimSliderPos(s.sizeSlider)) / 200.0
	ax, ay, az, _ := multidimRotation(s)
	verts = mdScaleVerts(verts, sz)
	verts = mdRotateVerts(verts, ax, ay, az)
	mdRenderFacesOnly(hdc, rc, verts, faces, multidimRenderOpacityIndex(s), fillColor)
	if drawHull {
		mdDrawProjectedHull(hdc, rc, verts, ss, lineColor)
	}
}

func mdTransformSteinmetzVerts(verts []mdVec3, size, ax, ay, az float64) []mdVec3 {
	out := make([]mdVec3, len(verts))
	for i, pt := range verts {
		out[i] = mdRotate(mdMul(pt, size), ax, ay, az)
	}
	return out
}

func mdPointInsideProjectedTriangleXZ(p, a, b, c mdVec3) (float64, float64, float64, bool) {
	den := (b.z-c.z)*(a.x-c.x) + (c.x-b.x)*(a.z-c.z)
	if math.Abs(den) < 1e-10 {
		return 0, 0, 0, false
	}
	l1 := ((b.z-c.z)*(p.x-c.x) + (c.x-b.x)*(p.z-c.z)) / den
	l2 := ((c.z-a.z)*(p.x-c.x) + (a.x-c.x)*(p.z-c.z)) / den
	l3 := 1 - l1 - l2
	eps := -1e-7
	return l1, l2, l3, l1 >= eps && l2 >= eps && l3 >= eps
}

func mdPointOccludedBySurface(point mdVec3, verts []mdVec3, faces []mdFace) bool {
	// Orthographic camera looks from +y toward the origin.  A curve point is
	// hidden exactly when some surface triangle projects to the same (x,z)
	// position and has a greater y value.  This matches the user's visual rule:
	// in semi-transparent mode, any line whose color would be changed by a face
	// in front must become a dashed hidden line.
	const depthEps = 0.012
	for _, face := range faces {
		if len(face.indices) < 3 {
			continue
		}
		a := verts[face.indices[0]]
		for i := 1; i+1 < len(face.indices); i++ {
			b := verts[face.indices[i]]
			c := verts[face.indices[i+1]]
			l1, l2, l3, inside := mdPointInsideProjectedTriangleXZ(point, a, b, c)
			if !inside {
				continue
			}
			y := l1*a.y + l2*b.y + l3*c.y
			if y > point.y+depthEps {
				return true
			}
		}
	}
	return false
}

func mdDrawDashedLinePhase(hdc uintptr, a, b POINT, width int, color uint32, phase *float64) {
	dx := float64(b.X - a.X)
	dy := float64(b.Y - a.Y)
	length := math.Hypot(dx, dy)
	if length < 1e-6 {
		return
	}
	ux, uy := dx/length, dy/length
	dash := float64(3 * width)
	gap := float64(6 * width)
	period := dash + gap
	pos := 0.0
	currentPhase := math.Mod(*phase, period)
	pen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(width), uintptr(color))
	old, _, _ := procSelectObject.Call(hdc, pen)
	defer func() { procSelectObject.Call(hdc, old); procDeleteObject.Call(pen) }()
	for pos < length-1e-6 {
		inDash := currentPhase < dash
		remaining := dash - currentPhase
		if !inDash {
			remaining = period - currentPhase
		}
		step := math.Min(length-pos, remaining)
		if inDash && step > 0 {
			x0 := float64(a.X) + ux*pos
			y0 := float64(a.Y) + uy*pos
			x1 := float64(a.X) + ux*(pos+step)
			y1 := float64(a.Y) + uy*(pos+step)
			procMoveToEx.Call(hdc, uintptr(int32(math.Round(x0))), uintptr(int32(math.Round(y0))), 0)
			procLineTo.Call(hdc, uintptr(int32(math.Round(x1))), uintptr(int32(math.Round(y1))))
		}
		pos += step
		currentPhase = math.Mod(currentPhase+step, period)
	}
	*phase = math.Mod(*phase+length, period)
}

func mdDrawOcclusionCurve(hdc uintptr, rc RECT, curve []mdVec3, opacity, ss int, lineColor uint32, size, ax, ay, az float64, surfaceVerts []mdVec3, faces []mdFace) {
	if len(curve) < 2 {
		return
	}
	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.33
	lineW := mdMaxInt(1, 2*ss)
	hiddenColor, showHidden := mdHiddenLineColor(opacity, lineColor)
	dashPhase := 0.0
	wasHidden := false
	for i := 0; i+1 < len(curve); i++ {
		a3 := mdRotate(mdMul(curve[i], size), ax, ay, az)
		b3 := mdRotate(mdMul(curve[i+1], size), ax, ay, az)
		mid := mdMul(mdAdd(a3, b3), 0.5)
		hidden := mdPointOccludedBySurface(mid, surfaceVerts, faces)
		a := mdProject(a3, rc, scale)
		b := mdProject(b3, rc, scale)
		if !hidden {
			mdDrawPolyline(hdc, []POINT{a, b}, lineW, lineColor, false)
			wasHidden = false
			continue
		}
		if !showHidden {
			wasHidden = false
			continue
		}
		if !wasHidden {
			dashPhase = 0
		}
		mdDrawDashedLinePhase(hdc, a, b, lineW, hiddenColor, &dashPhase)
		wasHidden = true
	}
}

func mdDrawObjectCurveSegments(hdc uintptr, rc RECT, curve []mdVec3, opacity int, ss int, lineColor uint32, size, ax, ay, az float64, visibleFn func(mdVec3) bool) {
	if len(curve) < 2 {
		return
	}
	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.33
	lineW := mdMaxInt(1, 2*ss)
	hiddenColor, showHidden := mdHiddenLineColor(opacity, lineColor)
	for i := 0; i+1 < len(curve); i++ {
		aObj := curve[i]
		bObj := curve[i+1]
		midObj := mdMul(mdAdd(aObj, bObj), 0.5)
		a3 := mdRotate(mdMul(aObj, size), ax, ay, az)
		b3 := mdRotate(mdMul(bObj, size), ax, ay, az)
		a := mdProject(a3, rc, scale)
		b := mdProject(b3, rc, scale)
		if visibleFn(midObj) {
			mdDrawPolyline(hdc, []POINT{a, b}, lineW, lineColor, false)
		} else if showHidden {
			mdDrawDashedLine(hdc, a, b, lineW, hiddenColor)
		}
	}
}

func mdInverseRotate(v mdVec3, ax, ay, az float64) mdVec3 {
	// mdRotate applies X, then Y, then Z.  Apply the inverse in reverse order.
	return mdRotX(mdRotY(mdRotZ(v, -az), -ay), -ax)
}

func mdSampleLine(a, b mdVec3, samples int) []mdVec3 {
	if samples < 1 {
		samples = 1
	}
	pts := make([]mdVec3, 0, samples+1)
	for i := 0; i <= samples; i++ {
		t := float64(i) / float64(samples)
		pts = append(pts, mdAdd(mdMul(a, 1-t), mdMul(b, t)))
	}
	return pts
}

func mdSampleSteinmetz2Silhouettes(ax, ay, az float64, samples int) [][]mdVec3 {
	// Camera looks along +world-y. Convert that view direction into object
	// coordinates so the tangent generators can be solved analytically.
	viewObj := mdInverseRotate(mdVec3{0, 1, 0}, ax, ay, az)
	curves := [][]mdVec3{}
	const eps = 1e-8

	// Cylinder y²+z²=1 (axis x): tangent condition y*vy + z*vz = 0.
	if d := math.Hypot(viewObj.y, viewObj.z); d > eps {
		y0 := -viewObj.z / d
		z0 := viewObj.y / d
		for _, sign := range []float64{-1, 1} {
			y := sign * y0
			z := sign * z0
			xLimit := math.Sqrt(math.Max(0, 1-z*z))
			curves = append(curves, mdSampleLine(mdVec3{-xLimit, y, z}, mdVec3{xLimit, y, z}, samples))
		}
	}

	// Cylinder x²+z²=1 (axis y): tangent condition x*vx + z*vz = 0.
	if d := math.Hypot(viewObj.x, viewObj.z); d > eps {
		x0 := -viewObj.z / d
		z0 := viewObj.x / d
		for _, sign := range []float64{-1, 1} {
			x := sign * x0
			z := sign * z0
			yLimit := math.Sqrt(math.Max(0, 1-z*z))
			curves = append(curves, mdSampleLine(mdVec3{x, -yLimit, z}, mdVec3{x, yLimit, z}, samples))
		}
	}
	return curves
}

func mdSampleSteinmetz2Edge(signX, signY float64, samples int) []mdVec3 {
	pts := make([]mdVec3, 0, samples+1)
	for i := 0; i <= samples; i++ {
		t := -1.0 + 2.0*float64(i)/float64(samples)
		s := math.Sqrt(math.Max(0, 1-t*t))
		pts = append(pts, mdVec3{signX * s, signY * s, t})
	}
	return pts
}

type mdSteinmetz3Curve struct {
	points []mdVec3
	family int
}

func mdSampleSteinmetz3Edges(samples int) []mdSteinmetz3Curve {
	curves := []mdSteinmetz3Curve{}
	limit := 1 / math.Sqrt2
	// family 0: intersection of x-cylinder (y²+z²=1) and z-cylinder (x²+y²=1)
	for _, sx := range []float64{-1, 1} {
		for _, sy := range []float64{-1, 1} {
			curve := make([]mdVec3, 0, samples+1)
			for i := 0; i <= samples; i++ {
				t := -limit + 2*limit*float64(i)/float64(samples)
				s := math.Sqrt(math.Max(0, 1-t*t))
				curve = append(curve, mdVec3{sx * t, sy * s, t})
			}
			curves = append(curves, mdSteinmetz3Curve{points: curve, family: 0})
		}
	}
	// family 1: intersection of x-cylinder (y²+z²=1) and y-cylinder (x²+z²=1)
	for _, sx := range []float64{-1, 1} {
		for _, sz := range []float64{-1, 1} {
			curve := make([]mdVec3, 0, samples+1)
			for i := 0; i <= samples; i++ {
				t := -limit + 2*limit*float64(i)/float64(samples)
				s := math.Sqrt(math.Max(0, 1-t*t))
				curve = append(curve, mdVec3{sx * t, t, sz * s})
			}
			curves = append(curves, mdSteinmetz3Curve{points: curve, family: 1})
		}
	}
	// family 2: intersection of y-cylinder (x²+z²=1) and z-cylinder (x²+y²=1)
	for _, sx := range []float64{-1, 1} {
		for _, sy := range []float64{-1, 1} {
			curve := make([]mdVec3, 0, samples+1)
			for i := 0; i <= samples; i++ {
				t := -limit + 2*limit*float64(i)/float64(samples)
				s := math.Sqrt(math.Max(0, 1-t*t))
				curve = append(curve, mdVec3{sx * s, sy * t, t})
			}
			curves = append(curves, mdSteinmetz3Curve{points: curve, family: 2})
		}
	}
	return curves
}

func mdSteinmetz2EdgeVisible(p mdVec3, ax, ay, az float64) bool {
	// The edge is shared by x²+z²=1 and y²+z²=1.
	n1 := mdRotate(mdNorm(mdVec3{p.x, 0, p.z}), ax, ay, az)
	n2 := mdRotate(mdNorm(mdVec3{0, p.y, p.z}), ax, ay, az)
	// An edge of a convex solid is visible if at least one adjacent surface faces
	// the camera. A tangent (zero dot) is also a visible silhouette.
	return n1.y >= -1e-6 || n2.y >= -1e-6
}

func mdSteinmetz3EdgeVisible(p mdVec3, family int, ax, ay, az float64) bool {
	var n1, n2 mdVec3
	switch family {
	case 0:
		// x-cylinder and z-cylinder
		n1 = mdVec3{0, p.y, p.z}
		n2 = mdVec3{p.x, p.y, 0}
	case 1:
		// x-cylinder and y-cylinder
		n1 = mdVec3{0, p.y, p.z}
		n2 = mdVec3{p.x, 0, p.z}
	default:
		// y-cylinder and z-cylinder
		n1 = mdVec3{p.x, 0, p.z}
		n2 = mdVec3{p.x, p.y, 0}
	}
	n1 = mdRotate(mdNorm(n1), ax, ay, az)
	n2 = mdRotate(mdNorm(n2), ax, ay, az)
	return n1.y >= -1e-6 || n2.y >= -1e-6
}

func multidimPaintSteinmetzEdges(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int, three bool, surfaceVerts []mdVec3, faces []mdFace) {
	lineColor := multidimPreviewLineColor(s)
	opacity := multidimRenderOpacityIndex(s)
	sz := float64(multidimSliderPos(s.sizeSlider)) / 200.0
	ax, ay, az, _ := multidimRotation(s)
	samples := 80
	if multidimRenderSmoothEnabled(s) {
		samples = 160
	}
	if !three {
		// True cylinder-intersection curves.
		for _, sx := range []float64{-1, 1} {
			for _, sy := range []float64{-1, 1} {
				curve := mdSampleSteinmetz2Edge(sx, sy, samples)
				mdDrawOcclusionCurve(hdc, rc, curve, opacity, ss, lineColor, sz, ax, ay, az, surfaceVerts, faces)
			}
		}
		// Apparent outer contour.  The intersection curves alone do not always
		// form the projected silhouette; each cylindrical patch contributes up
		// to two tangent generator segments.  Draw only the unoccluded portions
		// as solid lines, with hidden portions following the selected face mode.
		for _, curve := range mdSampleSteinmetz2Silhouettes(ax, ay, az, samples/2) {
			mdDrawOcclusionCurve(hdc, rc, curve, opacity, ss, lineColor, sz, ax, ay, az, surfaceVerts, faces)
		}
		return
	}
	for _, curve := range mdSampleSteinmetz3Edges(samples) {
		mdDrawOcclusionCurve(hdc, rc, curve.points, opacity, ss, lineColor, sz, ax, ay, az, surfaceVerts, faces)
	}
}

func multidimPaintSteinmetz2(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	seg, strips := 18, 28
	if multidimRenderSmoothEnabled(s) {
		seg, strips = 44, 72
	}
	objectVerts, faces := mdBuildSteinmetz2Surface(seg, strips)
	sz := float64(multidimSliderPos(s.sizeSlider)) / 200.0
	ax, ay, az, _ := multidimRotation(s)
	worldVerts := mdTransformSteinmetzVerts(objectVerts, sz, ax, ay, az)
	mdRenderFacesOnly(hdc, rc, worldVerts, faces, multidimRenderOpacityIndex(s), multidimPreviewFillColor(s))
	// No projected convex hull here: it created the false diamond/extra-cylinder
	// corners.  The four true cylinder-intersection curves provide the outline.
	multidimPaintSteinmetzEdges(s, hdc, rc, ss, false, worldVerts, faces)
}

func multidimPaintSteinmetz3(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	seg, strips := 16, 28
	if multidimRenderSmoothEnabled(s) {
		seg, strips = 36, 64
	}
	objectVerts, faces := mdBuildSteinmetz3Surface(seg, strips)
	sz := float64(multidimSliderPos(s.sizeSlider)) / 200.0
	ax, ay, az, _ := multidimRotation(s)
	worldVerts := mdTransformSteinmetzVerts(objectVerts, sz, ax, ay, az)
	mdRenderFacesOnly(hdc, rc, worldVerts, faces, multidimRenderOpacityIndex(s), multidimPreviewFillColor(s))
	mdDrawProjectedHull(hdc, rc, worldVerts, ss, multidimPreviewLineColor(s))
	multidimPaintSteinmetzEdges(s, hdc, rc, ss, true, worldVerts, faces)
}

func multidimPaintCube(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	lineColor := multidimPreviewLineColor(s)
	fillColor := multidimPreviewFillColor(s)
	sz := float64(multidimSliderPos(s.sizeSlider)) / 200.0
	ax, ay, az, _ := multidimRotation(s)
	base, faces := mdBuildCubeGeometry()
	verts := make([]mdVec3, len(base))
	for i, v := range base {
		verts[i] = mdRotate(mdMul(v, sz), ax, ay, az)
	}
	mdRenderConvexPolyhedron(hdc, rc, verts, faces, multidimRenderOpacityIndex(s), ss, lineColor, fillColor)
}

func multidimPaintCylinder(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	lineColor := multidimPreviewLineColor(s)
	fillColor := multidimPreviewFillColor(s)
	segments := 32
	if multidimRenderSmoothEnabled(s) {
		segments = 56
	}
	ax, ay, az, _ := multidimRotation(s)
	r := float64(multidimSliderPos(s.widthSlider)) / 250.0
	h := float64(multidimSliderPos(s.heightSlider)) / 180.0
	verts := []mdVec3{}
	for _, z := range []float64{-h / 2, h / 2} {
		for i := 0; i < segments; i++ {
			t := 2 * math.Pi * float64(i) / float64(segments)
			v := mdVec3{r * math.Cos(t), r * math.Sin(t), z}
			verts = append(verts, mdRotate(v, ax, ay, az))
		}
	}
	bottomCenter := len(verts)
	verts = append(verts, mdRotate(mdVec3{0, 0, -h / 2}, ax, ay, az))
	topCenter := len(verts)
	verts = append(verts, mdRotate(mdVec3{0, 0, h / 2}, ax, ay, az))
	faces := []mdFace{}
	for i := 0; i < segments; i++ {
		n := (i + 1) % segments
		faces = append(faces, mdFace{[]int{i, n, segments + n, segments + i}})
		faces = append(faces, mdFace{[]int{bottomCenter, n, i}})
		faces = append(faces, mdFace{[]int{topCenter, segments + i, segments + n}})
	}
	mdRenderFacesOnly(hdc, rc, verts, faces, multidimRenderOpacityIndex(s), fillColor)

	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.33
	lineW := mdMaxInt(1, 2*ss)
	hiddenColor, showHidden := mdHiddenLineColor(multidimRenderOpacityIndex(s), lineColor)

	topNormal := mdRotate(mdVec3{0, 0, 1}, ax, ay, az)
	bottomNormal := mdRotate(mdVec3{0, 0, -1}, ax, ay, az)
	drawRing := func(offset int, capNormal mdVec3) {
		for i := 0; i < segments; i++ {
			n := (i + 1) % segments
			tMid := 2 * math.Pi * (float64(i) + 0.5) / float64(segments)
			sideNormal := mdRotate(mdVec3{math.Cos(tMid), math.Sin(tMid), 0}, ax, ay, az)
			visible := capNormal.y > 1e-6 || sideNormal.y > 1e-6
			a := mdProject(verts[offset+i], rc, scale)
			b := mdProject(verts[offset+n], rc, scale)
			if visible {
				mdDrawPolyline(hdc, []POINT{a, b}, lineW, lineColor, false)
			} else if showHidden {
				mdDrawDashedLine(hdc, a, b, lineW, hiddenColor)
			}
		}
	}
	drawRing(segments, topNormal)
	drawRing(0, bottomNormal)

	// Draw the projected silhouette generators as solid lines. These are always visible contour edges,
	// so visibility is determined by occlusion geometry, not by center depth.
	silhouetteA := 0
	best := math.Inf(1)
	for i := 0; i < segments/2; i++ {
		t := 2 * math.Pi * float64(i) / float64(segments)
		normal := mdRotate(mdVec3{math.Cos(t), math.Sin(t), 0}, ax, ay, az)
		if v := math.Abs(normal.y); v < best {
			best = v
			silhouetteA = i
		}
	}
	silhouetteB := (silhouetteA + segments/2) % segments
	for _, idx := range []int{silhouetteA, silhouetteB} {
		a := mdProject(verts[idx], rc, scale)
		b := mdProject(verts[segments+idx], rc, scale)
		mdDrawPolyline(hdc, []POINT{a, b}, lineW, lineColor, false)
	}
}

func multidimPaintCone(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	lineColor := multidimPreviewLineColor(s)
	fillColor := multidimPreviewFillColor(s)
	segments := 32
	if multidimRenderSmoothEnabled(s) {
		segments = 56
	}
	ax, ay, az, _ := multidimRotation(s)
	r := float64(multidimSliderPos(s.widthSlider)) / 250.0
	h := float64(multidimSliderPos(s.heightSlider)) / 180.0
	objectVerts, faces, apexIndex, _ := mdBuildConeGeometry(segments, r, h)
	verts := mdRotateVerts(objectVerts, ax, ay, az)
	mdRenderFacesOnly(hdc, rc, verts, faces, multidimRenderOpacityIndex(s), fillColor)

	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.33
	lineW := mdMaxInt(1, 2*ss)
	hiddenColor, showHidden := mdHiddenLineColor(multidimRenderOpacityIndex(s), lineColor)

	// Each base-ring edge belongs to the base cap and one side patch. It is
	// visible when either adjacent surface faces the camera. This preserves the
	// correct front arc while allowing the rear arc to be dashed in transparent
	// modes, just like the existing cylinder renderer.
	baseFront := mdRotate(mdVec3{0, 0, -1}, ax, ay, az).y > 1e-7
	sideFront := make([]bool, segments)
	for i := 0; i < segments; i++ {
		tMid := 2 * math.Pi * (float64(i) + 0.5) / float64(segments)
		// Outward normal of x²+y²=(r/h)²(h/2-z)² is proportional
		// to (h cos t, h sin t, r).
		normal := mdRotate(mdNorm(mdVec3{h * math.Cos(tMid), h * math.Sin(tMid), r}), ax, ay, az)
		sideFront[i] = normal.y > 1e-7
		n := (i + 1) % segments
		a := mdProject(verts[i], rc, scale)
		b := mdProject(verts[n], rc, scale)
		if baseFront || sideFront[i] {
			mdDrawPolyline(hdc, []POINT{a, b}, lineW, lineColor, false)
		} else if showHidden {
			mdDrawDashedLine(hdc, a, b, lineW, hiddenColor)
		}
	}

	// A smooth cone has no meridian edges. Draw only the two projected contour
	// generators: the edges separating front-facing and rear-facing side
	// patches in the polygonal approximation.
	apex := mdProject(verts[apexIndex], rc, scale)
	for i := 0; i < segments; i++ {
		prev := (i + segments - 1) % segments
		if sideFront[prev] != sideFront[i] {
			base := mdProject(verts[i], rc, scale)
			mdDrawPolyline(hdc, []POINT{apex, base}, lineW, lineColor, false)
		}
	}
}

func multidimPaintPolygonalPyramid(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	lineColor := multidimPreviewLineColor(s)
	fillColor := multidimPreviewFillColor(s)
	sides := multidimEditorPolygonSides(s)
	ax, ay, az, _ := multidimRotation(s)
	r := float64(multidimSliderPos(s.widthSlider)) / 250.0
	h := float64(multidimSliderPos(s.heightSlider)) / 180.0
	objectVerts, faces, _ := mdBuildPolygonalPyramidGeometry(sides, r, h)
	verts := mdRotateVerts(objectVerts, ax, ay, az)
	mdRenderConvexPolyhedron(hdc, rc, verts, faces, multidimRenderOpacityIndex(s), ss, lineColor, fillColor)
}

func mdRenderEllipsoidFacesOnly(hdc uintptr, rc RECT, objectVerts, rotatedVerts []mdVec3, faces []mdFace, opacity int, fillColor uint32, rx, ry, rz, ax, ay, az float64, southPole, northPole int) {
	if opacity == 2 {
		return
	}
	alpha := 1.0
	if opacity == 1 {
		alpha = 0.20
	}
	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.33
	incomingLight := mdSceneIncomingLightDirection()
	renders := make([]mdFaceRender, 0, len(faces))
	for _, face := range faces {
		if len(face.indices) < 3 {
			continue
		}
		avgObject := mdVec3{}
		avgWorld := mdVec3{}
		pts := make([]POINT, len(face.indices))
		pole := -1
		for i, idx := range face.indices {
			avgObject = mdAdd(avgObject, objectVerts[idx])
			avgWorld = mdAdd(avgWorld, rotatedVerts[idx])
			pts[i] = mdProject(rotatedVerts[idx], rc, scale)
			if idx == southPole || idx == northPole {
				pole = idx
			}
		}
		avgObject = mdMul(avgObject, 1.0/float64(len(face.indices)))
		avgWorld = mdMul(avgWorld, 1.0/float64(len(face.indices)))
		// All triangles in one polar cap use the exact pole normal. This keeps
		// their fill identical and removes the radial dark pinwheel seen when
		// many independently lit triangles converge at a duplicated pole.
		normalPoint := avgObject
		if pole >= 0 {
			normalPoint = objectVerts[pole]
		}
		n := mdRotate(mdEllipsoidNormalAt(normalPoint, rx, ry, rz), ax, ay, az)
		brightness := 0.30 + 0.70*math.Max(0, mdDot(n, incomingLight))
		renders = append(renders, mdFaceRender{pts: pts, depth: avgWorld.y, bright: brightness})
	}
	sort.Slice(renders, func(i, j int) bool { return renders[i].depth < renders[j].depth })
	for _, face := range renders {
		col := mdBlendedColor(fillColor, face.bright, alpha)
		mdFillPolygon(hdc, face.pts, col, col)
	}
}

func mdDrawDashedLineSelected(hdc uintptr, a, b POINT, width int) {
	dx := float64(b.X - a.X)
	dy := float64(b.Y - a.Y)
	length := math.Hypot(dx, dy)
	if length < 1 {
		return
	}
	ux, uy := dx/length, dy/length
	dash, gap := float64(3*width), float64(6*width)
	for pos := 0.0; pos < length; pos += dash + gap {
		end := math.Min(length, pos+dash)
		x0 := float64(a.X) + ux*pos
		y0 := float64(a.Y) + uy*pos
		x1 := float64(a.X) + ux*end
		y1 := float64(a.Y) + uy*end
		procMoveToEx.Call(hdc, uintptr(int32(math.Round(x0))), uintptr(int32(math.Round(y0))), 0)
		procLineTo.Call(hdc, uintptr(int32(math.Round(x1))), uintptr(int32(math.Round(y1))))
	}
}

func mdDrawExactEllipsoidGuide(hdc uintptr, rc RECT, ss, steps, opacity int, lineColor uint32, rx, ry, rz, ax, ay, az float64, pointAt func(float64) mdVec3) {
	if steps < 24 {
		steps = 24
	}
	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.33
	lineW := mdMaxInt(1, 2*ss)
	// Less than one final pixel keeps the mathematical guide on the surface but
	// prevents a straight projected chord from appearing to cut into a strongly
	// compressed ellipsoid.
	offset := 0.65 * float64(ss) / math.Max(scale, 1)
	front := make([]mdSegment, 0, steps)
	back := make([]mdSegment, 0, steps)
	projectAt := func(t float64) POINT {
		p := pointAt(t)
		n := mdRotate(mdEllipsoidNormalAt(p, rx, ry, rz), ax, ay, az)
		w := mdAdd(mdRotate(p, ax, ay, az), mdMul(n, offset))
		return mdProject(w, rc, scale)
	}
	for i := 0; i < steps; i++ {
		t0 := 2 * math.Pi * float64(i) / float64(steps)
		t1 := 2 * math.Pi * float64(i+1) / float64(steps)
		tm := (t0 + t1) * 0.5
		p0, p1 := projectAt(t0), projectAt(t1)
		midNormal := mdRotate(mdEllipsoidNormalAt(pointAt(tm), rx, ry, rz), ax, ay, az)
		seg := mdSegment{a: p0, b: p1}
		// For a convex ellipsoid viewed orthographically from +Y, the exact
		// visibility condition is n·view > 0, not point.y > 0. The latter fails
		// badly after a nonuniform ellipsoid is rotated.
		if midNormal.y >= -1e-9 {
			front = append(front, seg)
		} else {
			back = append(back, seg)
		}
	}
	if hiddenColor, ok := mdHiddenLineColor(opacity, lineColor); ok && len(back) > 0 {
		pen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(lineW), uintptr(hiddenColor))
		old, _, _ := procSelectObject.Call(hdc, pen)
		for _, seg := range back {
			mdDrawDashedLineSelected(hdc, seg.a, seg.b, lineW)
		}
		procSelectObject.Call(hdc, old)
		procDeleteObject.Call(pen)
	}
	if len(front) > 0 {
		pen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(lineW), uintptr(lineColor))
		old, _, _ := procSelectObject.Call(hdc, pen)
		for _, seg := range front {
			procMoveToEx.Call(hdc, uintptr(seg.a.X), uintptr(seg.a.Y), 0)
			procLineTo.Call(hdc, uintptr(seg.b.X), uintptr(seg.b.Y))
		}
		procSelectObject.Call(hdc, old)
		procDeleteObject.Call(pen)
	}
}

func mdProjectedEllipsoidOutline(rc RECT, scale, rx, ry, rz, ax, ay, az float64, steps int) []POINT {
	if steps < 48 {
		steps = 48
	}
	// The orthographic projection of R·diag(rx,ry,rz)·S² onto the XZ plane is
	// exactly an ellipse. C=B·Bᵀ gives its 2x2 quadratic extent; diagonalizing C
	// yields the two screen-plane semiaxes without relying on a polygon hull.
	ex := mdRotate(mdVec3{x: rx}, ax, ay, az)
	ey := mdRotate(mdVec3{y: ry}, ax, ay, az)
	ez := mdRotate(mdVec3{z: rz}, ax, ay, az)
	c00 := ex.x*ex.x + ey.x*ey.x + ez.x*ez.x
	c01 := ex.x*ex.z + ey.x*ey.z + ez.x*ez.z
	c11 := ex.z*ex.z + ey.z*ey.z + ez.z*ez.z
	trace := c00 + c11
	disc := math.Sqrt(math.Max(0, (c00-c11)*(c00-c11)+4*c01*c01))
	lambdaA := math.Max(0, (trace+disc)*0.5)
	lambdaB := math.Max(0, (trace-disc)*0.5)
	a, b := math.Sqrt(lambdaA), math.Sqrt(lambdaB)
	angle := 0.5 * math.Atan2(2*c01, c00-c11)
	ca, sa := math.Cos(angle), math.Sin(angle)
	cx := float64(rc.Left+rc.Right) * 0.5
	cy := float64(rc.Top+rc.Bottom) * 0.5
	pts := make([]POINT, steps)
	for i := 0; i < steps; i++ {
		t := 2 * math.Pi * float64(i) / float64(steps)
		u, v := a*math.Cos(t), b*math.Sin(t)
		x := u*ca - v*sa
		z := u*sa + v*ca
		pts[i] = POINT{X: int32(math.Round(cx + x*scale)), Y: int32(math.Round(cy - z*scale))}
	}
	return pts
}

func multidimPaintEllipsoid(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int, sphere bool) {
	if sphere {
		skin := multidimEditorSphereSkinMode(s)
		if skin != mdSphereSkinPlain {
			multidimPaintTexturedSphere(s, hdc, rc, skin)
			return
		}
	}
	lineColor := multidimPreviewLineColor(s)
	fillColor := multidimPreviewFillColor(s)
	guideMode := multidimEditorSphereGuideMode(s)
	smooth := multidimEditorSmoothEnabled(s)
	latSteps, lonSteps := mdSphereMeshSteps(smooth, false, guideMode)
	curveSteps := mdSphereCurveSteps(smooth, false, guideMode)
	ax, ay, az, _ := multidimRotation(s)
	rx, ry, rz := 1.0, 1.0, 1.0
	if sphere {
		r := float64(multidimSliderPos(s.sizeSlider)) / 200.0
		rx, ry, rz = r, r, r
	} else {
		rx = float64(multidimSliderPos(s.widthSlider)) / 220.0
		ry = float64(multidimSliderPos(s.depthSlider)) / 220.0
		rz = float64(multidimSliderPos(s.heightSlider)) / 220.0
	}
	objectVerts, faces, southPole, northPole := mdBuildEllipsoidMesh(rx, ry, rz, latSteps, lonSteps)
	rotatedVerts := mdRotateVerts(objectVerts, ax, ay, az)
	opacity := multidimRenderOpacityIndex(s)
	mdRenderEllipsoidFacesOnly(hdc, rc, objectVerts, rotatedVerts, faces, opacity, fillColor, rx, ry, rz, ax, ay, az, southPole, northPole)

	if guideMode == mdSphereGuidesEquator || guideMode == mdSphereGuidesThreeAxes {
		mdDrawExactEllipsoidGuide(hdc, rc, ss, curveSteps, opacity, lineColor, rx, ry, rz, ax, ay, az,
			func(t float64) mdVec3 { return mdVec3{x: rx * math.Cos(t), y: ry * math.Sin(t)} })
	}
	if guideMode == mdSphereGuidesThreeAxes {
		mdDrawExactEllipsoidGuide(hdc, rc, ss, curveSteps, opacity, lineColor, rx, ry, rz, ax, ay, az,
			func(t float64) mdVec3 { return mdVec3{x: rx * math.Cos(t), z: rz * math.Sin(t)} })
		mdDrawExactEllipsoidGuide(hdc, rc, ss, curveSteps, opacity, lineColor, rx, ry, rz, ax, ay, az,
			func(t float64) mdVec3 { return mdVec3{y: ry * math.Cos(t), z: rz * math.Sin(t)} })
	}

	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.33
	outline := mdProjectedEllipsoidOutline(rc, scale, rx, ry, rz, ax, ay, az, curveSteps)
	mdDrawPolyline(hdc, outline, mdMaxInt(1, 2*ss), lineColor, true)
}

type mdSurfaceCurve struct {
	points []mdVec3
	closed bool
}

type mdSurfaceEdgeKey struct{ a, b int }

type mdSurfaceEdgeNormals struct {
	a, b        int
	normals     []mdVec3
	faceIndices []int
}

type mdSurfaceSilhouetteEdge struct {
	a, b          int
	incidentFaces []int
}

func mdFaces3To4(faces []mdFace) []mdFace4 {
	out := make([]mdFace4, len(faces))
	for i, face := range faces {
		out[i] = mdFace4{indices: face.indices}
	}
	return out
}

// mdRenderParametricSurfaceFaces uses the exact opaque z-buffer for a
// non-convex surface and a depth-sorted two-sided painter for the 20-percent
// translucent mode. Torus inner walls and the non-orientable Mobius strip
// therefore do not depend on a global "outward" normal.
func mdRenderParametricSurfaceFaces(s *multidimShapeEditorState, hdc uintptr, rc RECT, verts []mdVec3, faces []mdFace, opacity int, scale float64, fillColor uint32) {
	if opacity == 2 || len(verts) == 0 || len(faces) == 0 {
		return
	}
	faces4 := mdFaces3To4(faces)
	if opacity == 0 {
		mdRenderOpaque4DFacesZBuffer(s, hdc, rc, verts, faces4, scale, fillColor)
		return
	}
	if opacity == 1 && mdRenderTranslucent4DFacesZBuffer(s, hdc, rc, verts, faces4, scale, fillColor, 1.0, "translucent-parametric-faces") {
		return
	}
	incoming := mdSceneIncomingLightDirection()
	renders := make([]mdFaceRender, 0, len(faces))
	for _, face := range faces {
		if len(face.indices) < 3 {
			continue
		}
		a := verts[face.indices[0]]
		b := verts[face.indices[1]]
		c := verts[face.indices[2]]
		n := mdNorm(mdCross(mdSub(b, a), mdSub(c, a)))
		// Both torus sides can be seen at the inner rim, and a Mobius strip has
		// no globally consistent side. Use the locally camera-facing normal.
		if n.y < 0 {
			n = mdMul(n, -1)
		}
		bright := 0.34 + 0.66*math.Max(0, mdDot(n, incoming))
		avgY := 0.0
		pts := make([]POINT, len(face.indices))
		for i, idx := range face.indices {
			avgY += verts[idx].y
			pts[i] = mdProject(verts[idx], rc, scale)
		}
		avgY /= float64(len(face.indices))
		renders = append(renders, mdFaceRender{pts: pts, depth: avgY, bright: bright})
	}
	sort.SliceStable(renders, func(i, j int) bool { return renders[i].depth < renders[j].depth })
	for _, fr := range renders {
		col := mdBlendedColor(fillColor, fr.bright, 0.20)
		mdFillPolygon(hdc, fr.pts, col, col)
	}
}

func mdSurfaceUniqueLayerCount(point mdVec3, verts []mdVec3, faces []mdFace, accel *md4FaceAccel, depthEps float64) int {
	var candidates []int
	if accel != nil {
		candidates = accel.candidates(point)
	}
	return mdUniqueSurfaceCoverCountCore(point, verts, faces, candidates, depthEps)
}

func mdCollectSurfaceCurveFade(out *[]md4LineRenderSegment, rc RECT, scale float64, ss int, curve mdSurfaceCurve, opacity int, lineColor uint32, verts []mdVec3, faces []mdFace, accel *md4FaceAccel) {
	if len(curve.points) < 2 {
		return
	}
	type piece struct {
		a, b   mdVec3
		mid    mdVec3
		layers int
		show   bool
	}
	pieces := make([]piece, 0, len(curve.points)*2)
	count := len(curve.points) - 1
	if curve.closed {
		count = len(curve.points)
	}
	depthEps := math.Max(1e-7, 0.34*float64(mdMaxInt(1, ss))/math.Max(scale, 1e-6))
	for i := 0; i < count; i++ {
		a := curve.points[i]
		b := curve.points[(i+1)%len(curve.points)]
		pa, pb := mdProject(a, rc, scale), mdProject(b, rc, scale)
		length := math.Hypot(float64(pb.X-pa.X), float64(pb.Y-pa.Y))
		steps := mdMaxInt(1, int(math.Ceil(length/float64(mdMaxInt(1, ss)))))
		for j := 0; j < steps; j++ {
			t0 := float64(j) / float64(steps)
			t1 := float64(j+1) / float64(steps)
			a0 := mdLerp3(a, b, t0)
			b0 := mdLerp3(a, b, t1)
			mid := mdLerp3(a0, b0, 0.5)
			layers := mdSurfaceUniqueLayerCount(mid, verts, faces, accel, depthEps)
			pieces = append(pieces, piece{a: a0, b: b0, mid: mid, layers: layers, show: opacity != 0 || layers == 0})
		}
	}
	if opacity == 0 && len(pieces) >= 3 {
		filtered := make([]bool, len(pieces))
		for i := range pieces {
			filtered[i] = pieces[i].show
		}
		limitStart, limitEnd := 1, len(pieces)-1
		if curve.closed {
			limitStart, limitEnd = 0, len(pieces)
		}
		for i := limitStart; i < limitEnd; i++ {
			prev := i - 1
			next := i + 1
			if curve.closed {
				prev = (i - 1 + len(pieces)) % len(pieces)
				next = (i + 1) % len(pieces)
			}
			if !pieces[i].show && pieces[prev].show && pieces[next].show {
				filtered[i] = true
			} else if pieces[i].show && !pieces[prev].show && !pieces[next].show {
				filtered[i] = false
			}
		}
		for i := range pieces {
			pieces[i].show = filtered[i]
		}
	}
	for _, p := range pieces {
		if !p.show {
			continue
		}
		color := lineColor
		if opacity == 1 || opacity == 2 {
			color = mdSurfaceDepthFadeColor(lineColor, p.layers)
		}
		*out = append(*out, md4LineRenderSegment{
			a: mdProject(p.a, rc, scale), b: mdProject(p.b, rc, scale), depth: p.mid.y, color: color,
		})
	}
}

// mdSurfaceSilhouetteEdges approximates the true view silhouette by mesh
// edges whose adjacent face normals straddle the camera tangent condition.
// Normals are locally aligned before testing, which also works across the
// non-orientable Mobius seam. The incident faces are retained so visibility
// tests can ignore the two faces that generated the tangent candidate itself.
func mdSurfaceSilhouetteEdges(verts []mdVec3, faces []mdFace) []mdSurfaceSilhouetteEdge {
	edges := make(map[mdSurfaceEdgeKey]*mdSurfaceEdgeNormals)
	for faceIndex, face := range faces {
		if len(face.indices) < 3 {
			continue
		}
		a := verts[face.indices[0]]
		b := verts[face.indices[1]]
		c := verts[face.indices[2]]
		n := mdNorm(mdCross(mdSub(b, a), mdSub(c, a)))
		for i := 0; i < len(face.indices); i++ {
			va := face.indices[i]
			vb := face.indices[(i+1)%len(face.indices)]
			ka, kb := va, vb
			if ka > kb {
				ka, kb = kb, ka
			}
			key := mdSurfaceEdgeKey{ka, kb}
			e := edges[key]
			if e == nil {
				e = &mdSurfaceEdgeNormals{a: ka, b: kb}
				edges[key] = e
			}
			e.normals = append(e.normals, n)
			e.faceIndices = append(e.faceIndices, faceIndex)
		}
	}
	out := make([]mdSurfaceSilhouetteEdge, 0, len(edges)/4)
	for _, edge := range edges {
		draw := false
		if len(edge.normals) == 1 {
			// Open surfaces such as the Mobius strip keep their physical boundary.
			draw = true
		} else if len(edge.normals) >= 2 {
			n0, n1 := edge.normals[0], edge.normals[1]
			if mdDot(n0, n1) < 0 {
				n1 = mdMul(n1, -1)
			}
			draw = n0.y*n1.y <= 1e-5
		}
		if !draw {
			continue
		}
		out = append(out, mdSurfaceSilhouetteEdge{
			a: edge.a, b: edge.b,
			incidentFaces: append([]int(nil), edge.faceIndices...),
		})
	}
	return out
}

// mdDrawSurfaceSilhouette applies the same physical visibility rule used by
// surface grid curves. A tangent candidate is not automatically a visible
// outline: a rear tangent can still be covered by another front sheet of the
// torus. Opaque mode omits those intervals, while semi-transparent and
// transparent modes retain max(8%, 0.30^n) contrast for 3D surfaces. The candidate's own
// incident faces are ignored so coarse mesh approximation does not falsely
// classify a true outer boundary as hidden by itself.
func mdDrawSurfaceSilhouette(hdc uintptr, rc RECT, scale float64, ss int, opacity int, lineColor uint32, verts []mdVec3, faces []mdFace, accel *md4FaceAccel) {
	edges := mdSurfaceSilhouetteEdges(verts, faces)
	if len(edges) == 0 {
		return
	}
	segs := make([]md4LineRenderSegment, 0, len(edges)*3)
	depthEps := math.Max(1e-7, 0.34*float64(mdMaxInt(1, ss))/math.Max(scale, 1e-6))
	for _, edge := range edges {
		aidx, bidx := edge.a, edge.b
		if aidx < 0 || aidx >= len(verts) || bidx < 0 || bidx >= len(verts) {
			continue
		}
		a, b := verts[aidx], verts[bidx]
		pa, pb := mdProject(a, rc, scale), mdProject(b, rc, scale)
		length := math.Hypot(float64(pb.X-pa.X), float64(pb.Y-pa.Y))
		steps := mdMaxInt(1, int(math.Ceil(length/float64(mdMaxInt(1, ss)))))
		for j := 0; j < steps; j++ {
			t0 := float64(j) / float64(steps)
			t1 := float64(j+1) / float64(steps)
			a0 := mdLerp3(a, b, t0)
			b0 := mdLerp3(a, b, t1)
			mid := mdLerp3(a0, b0, 0.5)
			var candidates []int
			if accel != nil {
				candidates = accel.candidates(mid)
			}
			layers := mdUniqueSurfaceCoverCountIgnoringCore(mid, verts, faces, candidates, depthEps, edge.incidentFaces)
			if opacity == 0 && layers > 0 {
				continue
			}
			color := lineColor
			if opacity == 1 || opacity == 2 {
				color = mdSurfaceDepthFadeColor(lineColor, layers)
			}
			segs = append(segs, md4LineRenderSegment{
				a: mdProject(a0, rc, scale), b: mdProject(b0, rc, scale), depth: mid.y, color: color,
			})
		}
	}
	mdDrawSorted4DLineSegments(hdc, ss, segs)
}

func mdCollectParametricSurfaceLinesGPU(rc RECT, scale float64, ss int, opacity int, lineColor uint32, verts []mdVec3, faces []mdFace, curves []mdSurfaceCurve, accel *md4FaceAccel) ([]md4LineRenderSegment, bool) {
	if len(verts) == 0 || len(faces) == 0 {
		return nil, false
	}
	type plan struct {
		a, b, mid  mdVec3
		query      int
		group      int
		silhouette bool
	}
	plans := make([]plan, 0, 4096)
	queries := make([]mdGPUOcclusionBatchQuery, 0, 4096)
	groupRanges := make([][2]int, 0, len(curves))
	depthEps := math.Max(1e-7, 0.34*float64(mdMaxInt(1, ss))/math.Max(scale, 1e-6))
	for group, curve := range curves {
		start := len(plans)
		if len(curve.points) >= 2 {
			count := len(curve.points) - 1
			if curve.closed {
				count = len(curve.points)
			}
			for i := 0; i < count; i++ {
				a := curve.points[i]
				b := curve.points[(i+1)%len(curve.points)]
				pa, pb := mdProject(a, rc, scale), mdProject(b, rc, scale)
				length := math.Hypot(float64(pb.X-pa.X), float64(pb.Y-pa.Y))
				steps := mdMaxInt(1, int(math.Ceil(length/float64(mdMaxInt(1, ss)))))
				for j := 0; j < steps; j++ {
					t0 := float64(j) / float64(steps)
					t1 := float64(j+1) / float64(steps)
					a0 := mdLerp3(a, b, t0)
					b0 := mdLerp3(a, b, t1)
					mid := mdLerp3(a0, b0, 0.5)
					qi := len(queries)
					queries = append(queries, mdGPUOcclusionBatchQuery{point: mid})
					plans = append(plans, plan{a: a0, b: b0, mid: mid, query: qi, group: group})
				}
			}
		}
		groupRanges = append(groupRanges, [2]int{start, len(plans)})
	}
	curvePlanEnd := len(plans)
	for _, edge := range mdSurfaceSilhouetteEdges(verts, faces) {
		if edge.a < 0 || edge.a >= len(verts) || edge.b < 0 || edge.b >= len(verts) {
			continue
		}
		a, b := verts[edge.a], verts[edge.b]
		pa, pb := mdProject(a, rc, scale), mdProject(b, rc, scale)
		length := math.Hypot(float64(pb.X-pa.X), float64(pb.Y-pa.Y))
		steps := mdMaxInt(1, int(math.Ceil(length/float64(mdMaxInt(1, ss)))))
		for j := 0; j < steps; j++ {
			t0 := float64(j) / float64(steps)
			t1 := float64(j+1) / float64(steps)
			a0 := mdLerp3(a, b, t0)
			b0 := mdLerp3(a, b, t1)
			mid := mdLerp3(a0, b0, 0.5)
			qi := len(queries)
			queries = append(queries, mdGPUOcclusionBatchQuery{point: mid, excludedFaces: edge.incidentFaces})
			plans = append(plans, plan{a: a0, b: b0, mid: mid, query: qi, silhouette: true})
		}
	}
	if len(queries) == 0 {
		return nil, false
	}
	counts, ok := mdD3D11OcclusionLayerCounts("surface-occlusion", verts, mdFaces3To4(faces), queries, accel, depthEps)
	if !ok {
		return nil, false
	}
	show := make([]bool, curvePlanEnd)
	for i := 0; i < curvePlanEnd; i++ {
		show[i] = opacity != 0 || counts[plans[i].query] == 0
	}
	if opacity == 0 {
		for group, r := range groupRanges {
			if r[1]-r[0] < 3 {
				continue
			}
			curve := curves[group]
			filtered := append([]bool(nil), show[r[0]:r[1]]...)
			start, end := 1, len(filtered)-1
			if curve.closed {
				start, end = 0, len(filtered)
			}
			for i := start; i < end; i++ {
				prev, next := i-1, i+1
				if curve.closed {
					prev = (i - 1 + len(filtered)) % len(filtered)
					next = (i + 1) % len(filtered)
				}
				if !show[r[0]+i] && show[r[0]+prev] && show[r[0]+next] {
					filtered[i] = true
				} else if show[r[0]+i] && !show[r[0]+prev] && !show[r[0]+next] {
					filtered[i] = false
				}
			}
			copy(show[r[0]:r[1]], filtered)
		}
	}
	segments := make([]md4LineRenderSegment, 0, len(plans))
	for i, p := range plans {
		layers := counts[p.query]
		if !p.silhouette {
			if opacity == 0 && !show[i] {
				continue
			}
		} else if opacity == 0 && layers > 0 {
			continue
		}
		color := lineColor
		if opacity == 1 || opacity == 2 {
			color = mdSurfaceDepthFadeColor(lineColor, layers)
		}
		segments = append(segments, md4LineRenderSegment{
			a: mdProject(p.a, rc, scale), b: mdProject(p.b, rc, scale), depth: p.mid.y, color: color,
		})
	}
	return segments, true
}

func mdTorusCurves(majorRadius, minorRadius float64, majorSteps, minorSteps, mode int) []mdSurfaceCurve {
	curves := []mdSurfaceCurve{}
	addU := func(v float64) {
		pts := make([]mdVec3, majorSteps)
		cv, sv := math.Cos(v), math.Sin(v)
		for i := 0; i < majorSteps; i++ {
			u := 2 * math.Pi * float64(i) / float64(majorSteps)
			rho := majorRadius + minorRadius*cv
			pts[i] = mdVec3{x: rho * math.Cos(u), y: rho * math.Sin(u), z: minorRadius * sv}
		}
		curves = append(curves, mdSurfaceCurve{points: pts, closed: true})
	}
	addV := func(u float64) {
		pts := make([]mdVec3, minorSteps)
		cu, su := math.Cos(u), math.Sin(u)
		for j := 0; j < minorSteps; j++ {
			v := 2 * math.Pi * float64(j) / float64(minorSteps)
			rho := majorRadius + minorRadius*math.Cos(v)
			pts[j] = mdVec3{x: rho * cu, y: rho * su, z: minorRadius * math.Sin(v)}
		}
		curves = append(curves, mdSurfaceCurve{points: pts, closed: true})
	}
	if mode == mdSurfaceGridNone {
		return curves
	}
	if mode == mdSurfaceGridMajor {
		addU(0)
		addU(math.Pi)
		for i := 0; i < 4; i++ {
			addV(2 * math.Pi * float64(i) / 4)
		}
		return curves
	}
	for j := 0; j < 8; j++ {
		addU(2 * math.Pi * float64(j) / 8)
	}
	for i := 0; i < 12; i++ {
		addV(2 * math.Pi * float64(i) / 12)
	}
	return curves
}

func mdMobiusPairedCurve(radius, v float64, aroundSteps int) mdSurfaceCurve {
	pts := make([]mdVec3, 0, aroundSteps*2)
	for i := 0; i < aroundSteps; i++ {
		u := 2 * math.Pi * float64(i) / float64(aroundSteps)
		pts = append(pts, mdMobiusPoint(radius, u, v))
	}
	for i := 0; i < aroundSteps; i++ {
		u := 2 * math.Pi * float64(i) / float64(aroundSteps)
		pts = append(pts, mdMobiusPoint(radius, u, -v))
	}
	return mdSurfaceCurve{points: pts, closed: true}
}

func mdMobiusCurves(radius, halfWidth float64, aroundSteps, acrossSteps, mode int) []mdSurfaceCurve {
	curves := []mdSurfaceCurve{}
	center := make([]mdVec3, aroundSteps)
	for i := 0; i < aroundSteps; i++ {
		u := 2 * math.Pi * float64(i) / float64(aroundSteps)
		center[i] = mdMobiusPoint(radius, u, 0)
	}
	if mode != mdSurfaceGridNone {
		curves = append(curves, mdSurfaceCurve{points: center, closed: true})
	}
	// The two parameter boundaries are one physical closed boundary component.
	curves = append(curves, mdMobiusPairedCurve(radius, halfWidth, aroundSteps))
	addCross := func(u float64) {
		pts := make([]mdVec3, acrossSteps+1)
		for j := 0; j <= acrossSteps; j++ {
			v := -halfWidth + 2*halfWidth*float64(j)/float64(acrossSteps)
			pts[j] = mdMobiusPoint(radius, u, v)
		}
		curves = append(curves, mdSurfaceCurve{points: pts})
	}
	if mode == mdSurfaceGridMajor {
		for i := 0; i < 4; i++ {
			addCross(2 * math.Pi * float64(i) / 4)
		}
		return curves
	}
	if mode == mdSurfaceGridFull {
		for _, fraction := range []float64{0.25, 0.5, 0.75} {
			curves = append(curves, mdMobiusPairedCurve(radius, halfWidth*fraction, aroundSteps))
		}
		for i := 0; i < 12; i++ {
			addCross(2 * math.Pi * float64(i) / 12)
		}
	}
	return curves
}

func multidimPaintParametricSurface(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int, verts, objectVerts []mdVec3, faces []mdFace, curves []mdSurfaceCurve) {
	opacity := multidimRenderOpacityIndex(s)
	lineColor := multidimPreviewLineColor(s)
	fillColor := multidimPreviewFillColor(s)
	scale := math.Min(float64(rc.Right-rc.Left), float64(rc.Bottom-rc.Top)) * 0.30
	mdRenderParametricSurfaceFaces(s, hdc, rc, verts, faces, opacity, scale, fillColor)
	faces4 := mdFaces3To4(faces)
	grid := 18
	if len(faces) > 4000 {
		grid = 28
	}
	accel := mdBuild4DFaceAccel(verts, faces4, grid)
	rotatedCurves := make([]mdSurfaceCurve, 0, len(curves))
	ax, ay, az, _ := multidimRotation(s)
	for _, curve := range curves {
		rotated := make([]mdVec3, len(curve.points))
		for i, p := range curve.points {
			rotated[i] = mdRotate(p, ax, ay, az)
		}
		rotatedCurves = append(rotatedCurves, mdSurfaceCurve{points: rotated, closed: curve.closed})
	}
	if segments, ok := mdCollectParametricSurfaceLinesGPU(rc, scale, ss, opacity, lineColor, verts, faces, rotatedCurves, accel); ok {
		mdDrawSorted4DLineSegments(hdc, ss, segments)
	} else {
		segments := make([]md4LineRenderSegment, 0, 4096)
		for _, curve := range rotatedCurves {
			mdCollectSurfaceCurveFade(&segments, rc, scale, ss, curve, opacity, lineColor, verts, faces, accel)
		}
		mdDrawSorted4DLineSegments(hdc, ss, segments)
		mdDrawSurfaceSilhouette(hdc, rc, scale, ss, opacity, lineColor, verts, faces, accel)
	}
	_ = objectVerts
}

func multidimPaintTorus(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	smooth := multidimRenderSmoothEnabled(s)
	majorSteps, minorSteps := mdTorusMeshSteps(smooth, false)
	density := float64(multidimSliderPos(s.depthSlider)) / 48.0
	majorSteps = mdMaxInt(16, int(math.Round(float64(majorSteps)*density)))
	minorSteps = mdMaxInt(8, int(math.Round(float64(minorSteps)*density)))
	majorRadius := float64(multidimSliderPos(s.widthSlider)) / 135.0
	minorRadius := float64(multidimSliderPos(s.heightSlider)) / 145.0
	if minorRadius > majorRadius*0.92 {
		minorRadius = majorRadius * 0.92
	}
	objectVerts, faces := mdBuildTorusMesh(majorRadius, minorRadius, majorSteps, minorSteps)
	ax, ay, az, _ := multidimRotation(s)
	verts := mdRotateVerts(objectVerts, ax, ay, az)
	curves := mdTorusCurves(majorRadius, minorRadius, majorSteps, minorSteps, multidimEditorTorusGridMode(s))
	multidimPaintParametricSurface(s, hdc, rc, ss, verts, objectVerts, faces, curves)
}

func multidimPaintMobiusStrip(s *multidimShapeEditorState, hdc uintptr, rc RECT, ss int) {
	smooth := multidimRenderSmoothEnabled(s)
	aroundSteps, acrossSteps := mdMobiusMeshSteps(smooth, false)
	density := float64(multidimSliderPos(s.depthSlider)) / 48.0
	aroundSteps = mdMaxInt(24, int(math.Round(float64(aroundSteps)*density)))
	acrossSteps = mdMaxInt(4, int(math.Round(float64(acrossSteps)*math.Sqrt(density))))
	radius := float64(multidimSliderPos(s.widthSlider)) / 140.0
	halfWidth := float64(multidimSliderPos(s.heightSlider)) / 160.0
	if halfWidth > radius*0.72 {
		halfWidth = radius * 0.72
	}
	objectVerts, faces := mdBuildMobiusMesh(radius, halfWidth, aroundSteps, acrossSteps)
	ax, ay, az, _ := multidimRotation(s)
	verts := mdRotateVerts(objectVerts, ax, ay, az)
	curves := mdMobiusCurves(radius, halfWidth, aroundSteps, acrossSteps, multidimEditorMobiusGridMode(s))
	multidimPaintParametricSurface(s, hdc, rc, ss, verts, objectVerts, faces, curves)
}
