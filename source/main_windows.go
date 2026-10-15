//go:build windows

package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"
	"unicode/utf16"
	"unsafe"
)

// dotPaint A196-SADDLE-AND-MULTIDIM-INSERT
// Independent Win32 paint program for Korean Windows 11, inspired by the classic Paint layout.

// A237 regression-test baseline: const appVersion = "A237-DARK-LIBRARIES-HELP-ABOUT-BACKGROUND"
// Historical 1.2.38 regression marker retained for inherited source tests:
// const appVersion = "1.2.38"
// Release 1.2.39 regression marker: const appVersion = "1.2.40"
const appVersion = "1.3.7"

// A1250: one summary per Shift hold, never one synchronous log write per repeat.
var shiftPreviewAutorepeatsIgnored uint64

// --- Diagnostic logging ----------------------------------------------------
// The log is intentionally written next to dotPaint.exe whenever possible so
// the user can upload it without searching through AppData. Every line is
// flushed immediately; this preserves the last successful operation even when
// the process freezes or is forcibly terminated.
type diagnosticLogger struct {
	mu   sync.Mutex
	file *os.File
	path string
	seq  uint64
}

var diag diagnosticLogger

func initDiagnosticLog() {
	if !diagnosticLoggingEnabled {
		return
	}
	exe, err := os.Executable()
	base := "."
	if err == nil && exe != "" {
		base = filepath.Dir(exe)
	}
	path := filepath.Join(base, "dotPaint.log")
	if st, statErr := os.Stat(path); statErr == nil && st.Size() > 5*1024*1024 {
		_ = os.Remove(path + ".old")
		_ = os.Rename(path, path+".old")
	}
	f, openErr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if openErr != nil {
		fallback := filepath.Join(os.TempDir(), "dotPaint.log")
		f, openErr = os.OpenFile(fallback, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if openErr == nil {
			path = fallback
		}
	}
	diag.file = f
	diag.path = path
	logf("================ dotPaint %s start ================", appVersion)
	logf("go=%s os=%s arch=%s cpus=%d", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
	logf("executable=%q log=%q", exe, path)
}

func logf(format string, args ...any) {
	if !diagnosticLoggingEnabled {
		return
	}
	diag.mu.Lock()
	defer diag.mu.Unlock()
	if diag.file == nil {
		return
	}
	diag.seq++
	line := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(diag.file, "%s #%06d %s\r\n", time.Now().Format("2006-01-02 15:04:05.000"), diag.seq, line)
	_ = diag.file.Sync()
}

func logPanic(where string, recovered any) {
	logf("PANIC at %s: %v", where, recovered)
	for _, line := range strings.Split(string(debug.Stack()), "\n") {
		if line != "" {
			logf("STACK %s", line)
		}
	}
}

func closeDiagnosticLog() {
	logf("================ dotPaint normal end ================")
	diag.mu.Lock()
	defer diag.mu.Unlock()
	if diag.file != nil {
		_ = diag.file.Sync()
		_ = diag.file.Close()
		diag.file = nil
	}
}

// --- Win32 constants -------------------------------------------------------
const (
	WS_OVERLAPPED       = 0x00000000
	WS_CAPTION          = 0x00C00000
	WS_SYSMENU          = 0x00080000
	WS_THICKFRAME       = 0x00040000
	WS_MINIMIZEBOX      = 0x00020000
	WS_MAXIMIZEBOX      = 0x00010000
	WS_HSCROLL          = 0x00100000
	WS_VSCROLL          = 0x00200000
	WS_CHILD            = 0x40000000
	WS_VISIBLE          = 0x10000000
	WS_BORDER           = 0x00800000
	WS_TABSTOP          = 0x00010000
	WS_OVERLAPPEDWINDOW = WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU | WS_THICKFRAME | WS_MINIMIZEBOX | WS_MAXIMIZEBOX

	CW_USEDEFAULT = ^uint32(0x7fffffff)

	SW_HIDE = 0
	SW_SHOW = 5

	IMAGE_ICON     = 1
	LR_DEFAULTSIZE = 0x0040
	LR_SHARED      = 0x8000
	ICON_SMALL     = 0
	ICON_BIG       = 1
	IDI_DOTPAINT   = 1

	CS_HREDRAW = 0x0002
	CS_VREDRAW = 0x0001
	CS_DBLCLKS = 0x0008

	WM_CREATE                   = 0x0001
	WM_DRAWITEM                 = 0x002B
	WM_MEASUREITEM              = 0x002C
	WM_DESTROY                  = 0x0002
	WM_SIZE                     = 0x0005
	WM_PAINT                    = 0x000F
	WM_CLOSE                    = 0x0010
	WM_GETICON                  = 0x007F
	WM_SETICON                  = 0x0080
	WM_NCPAINT                  = 0x0085
	WM_NCACTIVATE               = 0x0086
	WM_CANCELMODE               = 0x001F
	WM_ERASEBKGND               = 0x0014
	WM_SETCURSOR                = 0x0020
	WM_COMMAND                  = 0x0111
	WM_INITDIALOG               = 0x0110
	WM_SETTEXT                  = 0x000C
	WM_GETTEXT                  = 0x000D
	WM_GETTEXTLENGTH            = 0x000E
	WM_SETFONT                  = 0x0030
	WM_GETFONT                  = 0x0031
	WM_HSCROLL                  = 0x0114
	WM_VSCROLL                  = 0x0115
	WM_KEYDOWN                  = 0x0100
	WM_KEYUP                    = 0x0101
	WM_IME_STARTCOMPOSITION     = 0x010D
	WM_IME_ENDCOMPOSITION       = 0x010E
	WM_IME_COMPOSITION          = 0x010F
	WM_CUT                      = 0x0300
	WM_COPY                     = 0x0301
	WM_PASTE                    = 0x0302
	WM_LBUTTONDOWN              = 0x0201
	WM_LBUTTONUP                = 0x0202
	WM_LBUTTONDBLCLK            = 0x0203
	WM_RBUTTONDOWN              = 0x0204
	WM_RBUTTONUP                = 0x0205
	WM_MOUSEMOVE                = 0x0200
	WM_MOUSEWHEEL               = 0x020A
	WM_CAPTURECHANGED           = 0x0215
	WM_DROPFILES                = 0x0233
	WM_MENUSELECT               = 0x011F
	WM_MENUCHAR                 = 0x0120
	WM_CLIPBOARDUPDATE          = 0x031D
	WM_TIMER                    = 0x0113
	darkEditorChromeTimerID     = 0xA231
	darkEditorChromeLateTimerID = 0xA232
	WM_CTLCOLORMSGBOX           = 0x0132
	WM_CTLCOLOREDIT             = 0x0133
	WM_CTLCOLORLISTBOX          = 0x0134
	WM_CTLCOLORBTN              = 0x0135
	WM_CTLCOLORDLG              = 0x0136
	WM_CTLCOLORSCROLLBAR        = 0x0137
	WM_CTLCOLORSTATIC           = 0x0138
	GWL_STYLE                   = ^uintptr(15)

	VK_LBUTTON = 0x01
	VK_RBUTTON = 0x02
	VK_TAB     = 0x09
	VK_CONTROL = 0x11
	VK_SHIFT   = 0x10
	VK_LEFT    = 0x25
	VK_UP      = 0x26
	VK_RIGHT   = 0x27
	VK_DOWN    = 0x28
	VK_HOME    = 0x24
	VK_END     = 0x23
	VK_DELETE  = 0x2E
	VK_ESCAPE  = 0x1B
	VK_F1      = 0x70
	VK_F10     = 0x79
	VK_PRIOR   = 0x21 // Page Up
	VK_NEXT    = 0x22 // Page Down

	MK_LBUTTON = 0x0001
	MK_RBUTTON = 0x0002

	MB_OK              = 0x00000000
	MB_OKCANCEL        = 0x00000001
	MB_YESNOCANCEL     = 0x00000003
	MB_YESNO           = 0x00000004
	MB_ICONINFORMATION = 0x00000040
	MB_ICONQUESTION    = 0x00000020
	MB_ICONERROR       = 0x00000010
	MB_ICONWARNING     = 0x00000030

	IDOK     = 1
	IDCANCEL = 2
	IDYES    = 6
	IDNO     = 7

	COLOR_WINDOW        = 5
	COLOR_WINDOWTEXT    = 8
	COLOR_BTNFACE       = 15
	COLOR_BTNTEXT       = 18
	COLOR_HIGHLIGHT     = 13
	COLOR_HIGHLIGHTTEXT = 14

	IDC_ARROW    = 32512
	IDC_IBEAM    = 32513
	IDC_CROSS    = 32515
	IDC_HAND     = 32649
	IDC_SIZEALL  = 32646
	IDC_SIZENWSE = 32642
	IDC_SIZENESW = 32643
	IDC_SIZEWE   = 32644
	IDC_SIZENS   = 32645

	DEFAULT_GUI_FONT = 17
	NULL_BRUSH       = 5
	BLACK_PEN        = 7

	PS_SOLID   = 0
	LOGPIXELSY = 90

	TRANSPARENT = 1
	OPAQUE      = 2

	SRCCOPY        = 0x00CC0020
	DIB_RGB_COLORS = 0
	BI_RGB         = 0
	BI_BITFIELDS   = 3

	CF_BITMAP = 2
	CF_DIB    = 8
	CF_DIBV5  = 17
	CF_HDROP  = 15

	GMEM_MOVEABLE = 0x0002
	COLORONCOLOR  = 3

	MF_STRING     = 0x00000000
	MF_GRAYED     = 0x00000001
	MF_DISABLED   = 0x00000002
	MF_CHECKED    = 0x00000008
	MF_POPUP      = 0x00000010
	MF_OWNERDRAW  = 0x00000100
	MF_SEPARATOR  = 0x00000800
	MF_BYCOMMAND  = 0x00000000
	MF_BYPOSITION = 0x00000400

	MNC_EXECUTE = 2

	TPM_LEFTALIGN = 0x0000
	TPM_TOPALIGN  = 0x0000
	TPM_RETURNCMD = 0x0100
	TPM_NONOTIFY  = 0x0080

	SB_HORZ = 0
	SB_VERT = 1

	SIF_RANGE           = 0x0001
	SIF_PAGE            = 0x0002
	SIF_POS             = 0x0004
	SIF_DISABLENOSCROLL = 0x0008
	SIF_TRACKPOS        = 0x0010
	SIF_ALL             = SIF_RANGE | SIF_PAGE | SIF_POS | SIF_TRACKPOS

	SB_LINEUP        = 0
	SB_LINEDOWN      = 1
	SB_PAGEUP        = 2
	SB_PAGEDOWN      = 3
	SB_THUMBPOSITION = 4
	SB_THUMBTRACK    = 5
	SB_TOP           = 6
	SB_BOTTOM        = 7
	SB_ENDSCROLL     = 8

	OFN_PATHMUSTEXIST   = 0x00000800
	OFN_FILEMUSTEXIST   = 0x00001000
	OFN_HIDEREADONLY    = 0x00000004
	OFN_OVERWRITEPROMPT = 0x00000002

	CC_RGBINIT    = 0x00000001
	CC_FULLOPEN   = 0x00000002
	CC_ENABLEHOOK = 0x00000010

	DT_LEFT         = 0x00000000
	DT_CENTER       = 0x00000001
	DT_RIGHT        = 0x00000002
	DT_VCENTER      = 0x00000004
	DT_SINGLELINE   = 0x00000020
	DT_END_ELLIPSIS = 0x00008000
	DT_TOP          = 0x00000000
	DT_WORDBREAK    = 0x00000010
	DT_EDITCONTROL  = 0x00002000
	DT_NOPREFIX     = 0x00000800
	DT_CALCRECT     = 0x00000400

	WHEEL_DELTA          = 120
	ES_MULTILINE         = 0x0004
	ES_NOHIDESEL         = 0x0100
	ES_AUTOVSCROLL       = 0x0040
	ES_AUTOHSCROLL       = 0x0080
	CBS_DROPDOWNLIST     = 0x0003
	CBS_DROPDOWN         = 0x0002
	CBS_HASSTRINGS       = 0x0200
	CBS_NOINTEGRALHEIGHT = 0x0400
	CBS_OWNERDRAWFIXED   = 0x0010
	BS_AUTOCHECKBOX      = 0x00000003
	BS_OWNERDRAW         = 0x0000000B
	BS_PUSHLIKE          = 0x00001000
	BN_CLICKED           = 0
	CBN_SELCHANGE        = 1
	CBN_KILLFOCUS        = 4
	CBN_EDITCHANGE       = 5
	CBN_CLOSEUP          = 8
	EN_CHANGE            = 0x0300
	EM_GETSEL            = 0x00B0
	EM_SETSEL            = 0x00B1
	EM_GETRECT           = 0x00B2
	EM_SETRECTNP         = 0x00B4
	EM_SETMARGINS        = 0x00D3
	EM_POSFROMCHAR       = 0x00D6
	EM_CHARFROMPOS       = 0x00D7
	EM_FORMATRANGE       = 0x0439
	EM_SETBKGNDCOLOR     = 0x0443
	EM_SETCHARFORMAT     = 0x0444
	EM_GETZOOM           = 0x04E0
	EM_SETZOOM           = 0x04E1
	SCF_SELECTION        = 0x0001
	SCF_ALL              = 0x0004
	CFM_BOLD             = 0x00000001
	CFM_ITALIC           = 0x00000002
	CFM_UNDERLINE        = 0x00000004
	CFM_SIZE             = 0x80000000
	CFM_COLOR            = 0x40000000
	CFM_FACE             = 0x20000000
	CFE_BOLD             = 0x00000001
	CFE_ITALIC           = 0x00000002
	CFE_UNDERLINE        = 0x00000004
	EC_LEFTMARGIN        = 0x0001
	EC_RIGHTMARGIN       = 0x0002
	CB_ADDSTRING         = 0x0143
	CB_GETCURSEL         = 0x0147
	CB_GETLBTEXT         = 0x0148
	CB_GETLBTEXTLEN      = 0x0149
	CB_SETCURSEL         = 0x014E
	CB_SETITEMHEIGHT     = 0x0153
	CB_SETDROPPEDWIDTH   = 0x0160
	BM_GETCHECK          = 0x00F0
	BM_SETCHECK          = 0x00F1
	BST_UNCHECKED        = 0
	BST_CHECKED          = 1
	ODT_MENU             = 1
	ODT_BUTTON           = 4
	ODT_COMBOBOX         = 3
	ODA_DRAWENTIRE       = 0x0001
	ODA_SELECT           = 0x0002
	ODA_FOCUS            = 0x0004
	ODS_SELECTED         = 0x0001
	ODS_GRAYED           = 0x0002
	ODS_DISABLED         = 0x0004
	ODS_CHECKED          = 0x0008
	ODS_FOCUS            = 0x0010
	ODS_NOACCEL          = 0x0100
	FW_NORMAL            = 400
	FW_BOLD              = 700
	DEFAULT_CHARSET      = 1
	OUT_DEFAULT_PRECIS   = 0
	CLIP_DEFAULT_PRECIS  = 0
	DEFAULT_QUALITY      = 0
	DEFAULT_PITCH        = 0
	FF_DONTCARE          = 0
	DSTINVERT            = 0x00550009

	MIM_BACKGROUND                           = 0x00000002
	MIM_APPLYTOSUBMENUS                      = 0x80000000
	DWMWA_USE_IMMERSIVE_DARK_MODE_OLD        = 19
	DWMWA_USE_IMMERSIVE_DARK_MODE            = 20
	DWMWA_BORDER_COLOR                       = 34
	DWMWA_CAPTION_COLOR                      = 35
	DWMWA_TEXT_COLOR                         = 36
	DWMWA_COLOR_DEFAULT               uint32 = 0xFFFFFFFF
	SWP_NOSIZE                               = 0x0001
	SWP_NOMOVE                               = 0x0002
	SWP_FRAMECHANGED                         = 0x0020
	SM_CYCAPTION                             = 4
	SM_CXSIZE                                = 30
	SM_CYSIZE                                = 31
	SM_CXFRAME                               = 32
	SM_CYFRAME                               = 33
	SM_CXPADDEDBORDER                        = 92
	DI_NORMAL                                = 0x0003
)

// Menu command IDs.
const (
	ID_FILE_NEW = 1001 + iota
	ID_FILE_OPEN
	ID_FILE_SAVE
	ID_FILE_SAVEAS
	ID_FILE_PRINTPREVIEW
	ID_FILE_PAGESETUP
	ID_FILE_PRINT
	ID_FILE_SEND
	ID_FILE_WALLTILE
	ID_FILE_WALLCENTER
	ID_FILE_EXIT
)

const (
	ID_EDIT_UNDO = 1101 + iota
	ID_EDIT_REDO
	ID_EDIT_CUT
	ID_EDIT_COPY
	ID_EDIT_PASTE
	ID_EDIT_DELETE
	ID_EDIT_DELETE_TRANSPARENT
	ID_EDIT_SELECTALL
	ID_EDIT_SAVESELECTION
	ID_EDIT_PASTEFROM
)

const (
	ID_VIEW_TOOLBOX = 1201 + iota
	ID_VIEW_PALETTE
	ID_VIEW_STATUS
	ID_VIEW_TEXTBAR
	ID_VIEW_ZOOM_NORMAL
	ID_VIEW_ZOOM_LARGE
	ID_VIEW_ZOOM_CUSTOM
	ID_VIEW_BITMAP
	ID_VIEW_GRID
	ID_VIEW_THUMBNAIL
	ID_VIEW_CLIPBOARD
	ID_VIEW_HISTORY
	ID_VIEW_DARKMODE
	ID_VIEW_LANGUAGE_KO
	ID_VIEW_LANGUAGE_EN
)

const (
	ID_IMAGE_FLIP = 1301 + iota
	ID_IMAGE_STRETCH
	ID_IMAGE_INVERT
	ID_IMAGE_ATTRIBUTES
	ID_IMAGE_CLEAR
	ID_IMAGE_OPAQUE
)

const (
	ID_COLOR_EDIT           = 1401
	ID_COLOR_INDEXED_TOGGLE = 1402
	ID_COLOR_INDEXED_LOAD   = 1403
	ID_COLOR_INDEXED_SAVE   = 1404
	ID_COLOR_INDEXED_256    = 1405
)

const (
	ID_INSERT_FORMULA = 1601 + iota
	ID_INSERT_FUNCTION_GRAPH
	ID_INSERT_IMAGE_CIRCUIT
	ID_INSERT_IMAGE_ANIMAL
	ID_INSERT_IMAGE_CUSTOM
	ID_INSERT_IMAGE_MULTIDIM
	ID_INSERT_IMAGE_FIELD
)

const (
	ID_HELP_TOPICS = 1501 + iota
	ID_HELP_LOG
	ID_HELP_ABOUT
)

const (
	ID_DEJAVU_TILES = 1701 + iota
	ID_DEJAVU_EXPORT_SHEET
	ID_DEJAVU_IMPORT_SHEET
	ID_DEJAVU_EXPORT_TILE
	ID_DEJAVU_IMPORT_TILE
	ID_DEJAVU_RELOAD_PALETTE
)

const ID_DEJAVU_TRANSPARENT_BASE = 1740

const (
	ID_RECENT_BASE = 2000
	ID_ZOOM_BASE   = 2100
)

const (
	ID_TEXT_EDITBOX = 3001 + iota
	ID_TEXT_FONTCOMBO
	ID_TEXT_SIZECOMBO
	ID_TEXT_SCRIPTCOMBO
	ID_TEXT_BOLD
	ID_TEXT_ITALIC
	ID_TEXT_UNDERLINE
	ID_TEXT_LAYOUTBOX
)

// --- Win32 structures ------------------------------------------------------
type POINT struct{ X, Y int32 }
type SIZE struct{ Cx, Cy int32 }
type TEXTMETRICW struct {
	TmHeight           int32
	TmAscent           int32
	TmDescent          int32
	TmInternalLeading  int32
	TmExternalLeading  int32
	TmAveCharWidth     int32
	TmMaxCharWidth     int32
	TmWeight           int32
	TmOverhang         int32
	TmDigitizedAspectX int32
	TmDigitizedAspectY int32
	TmFirstChar        uint16
	TmLastChar         uint16
	TmDefaultChar      uint16
	TmBreakChar        uint16
	TmItalic           byte
	TmUnderlined       byte
	TmStruckOut        byte
	TmPitchAndFamily   byte
	TmCharSet          byte
}
type RECT struct{ Left, Top, Right, Bottom int32 }
type CHARRANGE struct{ CpMin, CpMax int32 }
type FORMATRANGE struct {
	Hdc       uintptr
	HdcTarget uintptr
	Rc        RECT
	RcPage    RECT
	Chrg      CHARRANGE
}
type CHARFORMATW struct {
	CbSize          uint32
	DwMask          uint32
	DwEffects       uint32
	YHeight         int32
	YOffset         int32
	CrTextColor     uint32
	BCharSet        byte
	BPitchAndFamily byte
	SzFaceName      [32]uint16
}
type MSG struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       POINT
	LPrivate uint32
}
type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}
type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}
type BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}
type RGBQUAD struct{ Blue, Green, Red, Reserved byte }
type BITMAPINFO struct {
	BmiHeader BITMAPINFOHEADER
	BmiColors [1]RGBQUAD
}
type BITMAP struct {
	BmType       int32
	BmWidth      int32
	BmHeight     int32
	BmWidthBytes int32
	BmPlanes     uint16
	BmBitsPixel  uint16
	BmBits       uintptr
}
type SCROLLINFO struct {
	CbSize    uint32
	FMask     uint32
	NMin      int32
	NMax      int32
	NPage     uint32
	NPos      int32
	NTrackPos int32
}
type DRAWITEMSTRUCT struct {
	CtlType    uint32
	CtlID      uint32
	ItemID     uint32
	ItemAction uint32
	ItemState  uint32
	HwndItem   uintptr
	HDC        uintptr
	RcItem     RECT
	ItemData   uintptr
}
type MEASUREITEMSTRUCT struct {
	CtlType    uint32
	CtlID      uint32
	ItemID     uint32
	ItemWidth  uint32
	ItemHeight uint32
	ItemData   uintptr
}
type MENUINFO struct {
	CbSize          uint32
	FMask           uint32
	DwStyle         uint32
	CyMax           uint32
	HbrBack         uintptr
	DwContextHelpID uint32
	DwMenuData      uintptr
}
type LOGFONTW struct {
	LfHeight         int32
	LfWidth          int32
	LfEscapement     int32
	LfOrientation    int32
	LfWeight         int32
	LfItalic         byte
	LfUnderline      byte
	LfStrikeOut      byte
	LfCharSet        byte
	LfOutPrecision   byte
	LfClipPrecision  byte
	LfQuality        byte
	LfPitchAndFamily byte
	LfFaceName       [32]uint16
}
type OPENFILENAMEW struct {
	LStructSize       uint32
	HwndOwner         uintptr
	HInstance         uintptr
	LpstrFilter       *uint16
	LpstrCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilterIndex      uint32
	LpstrFile         *uint16
	NMaxFile          uint32
	LpstrFileTitle    *uint16
	NMaxFileTitle     uint32
	LpstrInitialDir   *uint16
	LpstrTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LpstrDefExt       *uint16
	LCustData         uintptr
	LpfnHook          uintptr
	LpTemplateName    *uint16
	PvReserved        uintptr
	DwReserved        uint32
	FlagsEx           uint32
}
type CHOOSECOLORW struct {
	LStructSize    uint32
	HwndOwner      uintptr
	HInstance      uintptr
	RgbResult      uint32
	LpCustColors   *uint32
	Flags          uint32
	LCustData      uintptr
	LpfnHook       uintptr
	LpTemplateName *uint16
}

// --- DLL procedures --------------------------------------------------------
var (
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modGdi32    = syscall.NewLazyDLL("gdi32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")
	modComdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	modShell32  = syscall.NewLazyDLL("shell32.dll")
	modMsftedit = syscall.NewLazyDLL("msftedit.dll")
	modDwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	modUxtheme  = syscall.NewLazyDLL("uxtheme.dll")

	procRegisterClassExW              = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW               = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW                = modUser32.NewProc("DefWindowProcW")
	procShowWindow                    = modUser32.NewProc("ShowWindow")
	procUpdateWindow                  = modUser32.NewProc("UpdateWindow")
	procGetMessageW                   = modUser32.NewProc("GetMessageW")
	procTranslateMessage              = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW              = modUser32.NewProc("DispatchMessageW")
	procPostQuitMessage               = modUser32.NewProc("PostQuitMessage")
	procPostMessageW                  = modUser32.NewProc("PostMessageW")
	procBeginPaint                    = modUser32.NewProc("BeginPaint")
	procEndPaint                      = modUser32.NewProc("EndPaint")
	procGetClientRect                 = modUser32.NewProc("GetClientRect")
	procInvalidateRect                = modUser32.NewProc("InvalidateRect")
	procCreateMenu                    = modUser32.NewProc("CreateMenu")
	procCreatePopupMenu               = modUser32.NewProc("CreatePopupMenu")
	procAppendMenuW                   = modUser32.NewProc("AppendMenuW")
	procSetMenu                       = modUser32.NewProc("SetMenu")
	procDrawMenuBar                   = modUser32.NewProc("DrawMenuBar")
	procDestroyMenu                   = modUser32.NewProc("DestroyMenu")
	procSetMenuInfo                   = modUser32.NewProc("SetMenuInfo")
	procGetMenuItemCount              = modUser32.NewProc("GetMenuItemCount")
	procDeleteMenu                    = modUser32.NewProc("DeleteMenu")
	procCheckMenuItem                 = modUser32.NewProc("CheckMenuItem")
	procEnableMenuItem                = modUser32.NewProc("EnableMenuItem")
	procMessageBoxW                   = modUser32.NewProc("MessageBoxW")
	procLoadCursorW                   = modUser32.NewProc("LoadCursorW")
	procLoadImageW                    = modUser32.NewProc("LoadImageW")
	procSetCursor                     = modUser32.NewProc("SetCursor")
	procSetCapture                    = modUser32.NewProc("SetCapture")
	procGetCapture                    = modUser32.NewProc("GetCapture")
	procReleaseCapture                = modUser32.NewProc("ReleaseCapture")
	procGetKeyState                   = modUser32.NewProc("GetKeyState")
	procGetAsyncKeyState              = modUser32.NewProc("GetAsyncKeyState")
	procSetWindowTextW                = modUser32.NewProc("SetWindowTextW")
	procTrackPopupMenu                = modUser32.NewProc("TrackPopupMenu")
	procGetCursorPos                  = modUser32.NewProc("GetCursorPos")
	procClientToScreen                = modUser32.NewProc("ClientToScreen")
	procSetScrollInfo                 = modUser32.NewProc("SetScrollInfo")
	procGetScrollInfo                 = modUser32.NewProc("GetScrollInfo")
	procScrollWindowEx                = modUser32.NewProc("ScrollWindowEx")
	procScreenToClient                = modUser32.NewProc("ScreenToClient")
	procSetProcessDpiAwarenessContext = modUser32.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForWindow               = modUser32.NewProc("GetDpiForWindow")
	procAddClipboardFormatListener    = modUser32.NewProc("AddClipboardFormatListener")
	procRemoveClipboardFormatListener = modUser32.NewProc("RemoveClipboardFormatListener")
	procOpenClipboard                 = modUser32.NewProc("OpenClipboard")
	procCloseClipboard                = modUser32.NewProc("CloseClipboard")
	procEmptyClipboard                = modUser32.NewProc("EmptyClipboard")
	procGetClipboardData              = modUser32.NewProc("GetClipboardData")
	procSetClipboardData              = modUser32.NewProc("SetClipboardData")
	procIsClipboardFormatAvailable    = modUser32.NewProc("IsClipboardFormatAvailable")
	procGetClipboardSequenceNumber    = modUser32.NewProc("GetClipboardSequenceNumber")
	procRegisterClipboardFormatW      = modUser32.NewProc("RegisterClipboardFormatW")
	procEnumClipboardFormats          = modUser32.NewProc("EnumClipboardFormats")
	procGetClipboardFormatNameW       = modUser32.NewProc("GetClipboardFormatNameW")
	procGetDC                         = modUser32.NewProc("GetDC")
	procGetWindowDC                   = modUser32.NewProc("GetWindowDC")
	procGetSystemMetrics              = modUser32.NewProc("GetSystemMetrics")
	procDrawIconEx                    = modUser32.NewProc("DrawIconEx")
	procReleaseDC                     = modUser32.NewProc("ReleaseDC")
	procMoveWindow                    = modUser32.NewProc("MoveWindow")
	procGetClassNameW                 = modUser32.NewProc("GetClassNameW")
	procGetWindowLongW                = modUser32.NewProc("GetWindowLongW")
	procSetWindowLongW                = modUser32.NewProc("SetWindowLongW")
	procSendMessageW                  = modUser32.NewProc("SendMessageW")
	procSetFocus                      = modUser32.NewProc("SetFocus")
	procGetFocus                      = modUser32.NewProc("GetFocus")
	procGetNextDlgTabItem             = modUser32.NewProc("GetNextDlgTabItem")
	procGetWindowTextW                = modUser32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW          = modUser32.NewProc("GetWindowTextLengthW")
	procSetTimer                      = modUser32.NewProc("SetTimer")
	procKillTimer                     = modUser32.NewProc("KillTimer")
	procSetForegroundWindow           = modUser32.NewProc("SetForegroundWindow")
	procGetSysColor                   = modUser32.NewProc("GetSysColor")
	procDrawFocusRect                 = modUser32.NewProc("DrawFocusRect")
	procEnumChildWindows              = modUser32.NewProc("EnumChildWindows")

	procDwmSetWindowAttribute = modDwmapi.NewProc("DwmSetWindowAttribute")
	procSetWindowTheme        = modUxtheme.NewProc("SetWindowTheme")

	procGetModuleHandleW   = modKernel32.NewProc("GetModuleHandleW")
	procLoadLibraryW       = modKernel32.NewProc("LoadLibraryW")
	procGetProcAddress     = modKernel32.NewProc("GetProcAddress")
	procGetCurrentThreadId = modKernel32.NewProc("GetCurrentThreadId")
	procGlobalAlloc        = modKernel32.NewProc("GlobalAlloc")
	procGlobalLock         = modKernel32.NewProc("GlobalLock")
	procGlobalUnlock       = modKernel32.NewProc("GlobalUnlock")
	procGlobalSize         = modKernel32.NewProc("GlobalSize")
	procGlobalFree         = modKernel32.NewProc("GlobalFree")

	procCreateSolidBrush       = modGdi32.NewProc("CreateSolidBrush")
	procDeleteObject           = modGdi32.NewProc("DeleteObject")
	procFillRect               = modUser32.NewProc("FillRect")
	procFrameRect              = modUser32.NewProc("FrameRect")
	procRectangle              = modGdi32.NewProc("Rectangle")
	procEllipse                = modGdi32.NewProc("Ellipse")
	procMoveToEx               = modGdi32.NewProc("MoveToEx")
	procLineTo                 = modGdi32.NewProc("LineTo")
	procCreatePen              = modGdi32.NewProc("CreatePen")
	procSelectObject           = modGdi32.NewProc("SelectObject")
	procGetStockObject         = modGdi32.NewProc("GetStockObject")
	procSetTextColor           = modGdi32.NewProc("SetTextColor")
	procSetBkMode              = modGdi32.NewProc("SetBkMode")
	procDrawTextW              = modUser32.NewProc("DrawTextW")
	procStretchDIBits          = modGdi32.NewProc("StretchDIBits")
	procSetStretchBltMode      = modGdi32.NewProc("SetStretchBltMode")
	procSetPixel               = modGdi32.NewProc("SetPixel")
	procGetPixel               = modGdi32.NewProc("GetPixel")
	procCreateCompatibleDC     = modGdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = modGdi32.NewProc("CreateCompatibleBitmap")
	procDeleteDC               = modGdi32.NewProc("DeleteDC")
	procBitBlt                 = modGdi32.NewProc("BitBlt")
	procSaveDC                 = modGdi32.NewProc("SaveDC")
	procRestoreDC              = modGdi32.NewProc("RestoreDC")
	procIntersectClipRect      = modGdi32.NewProc("IntersectClipRect")
	procGetClipBox             = modGdi32.NewProc("GetClipBox")
	procGetObjectW             = modGdi32.NewProc("GetObjectW")
	procGetDIBits              = modGdi32.NewProc("GetDIBits")
	procCreateDIBSection       = modGdi32.NewProc("CreateDIBSection")
	procCreateFontW            = modGdi32.NewProc("CreateFontW")
	procSetBkColor             = modGdi32.NewProc("SetBkColor")
	procEnumFontFamiliesExW    = modGdi32.NewProc("EnumFontFamiliesExW")
	procPatBlt                 = modGdi32.NewProc("PatBlt")
	procGetDeviceCaps          = modGdi32.NewProc("GetDeviceCaps")
	procGetTextExtentPoint32W  = modGdi32.NewProc("GetTextExtentPoint32W")
	procGetTextMetricsW        = modGdi32.NewProc("GetTextMetricsW")
	procTextOutW               = modGdi32.NewProc("TextOutW")

	procGetOpenFileNameW = modComdlg32.NewProc("GetOpenFileNameW")
	procGetSaveFileNameW = modComdlg32.NewProc("GetSaveFileNameW")
	procChooseColorW     = modComdlg32.NewProc("ChooseColorW")

	procShellExecuteW   = modShell32.NewProc("ShellExecuteW")
	procDragAcceptFiles = modShell32.NewProc("DragAcceptFiles")
	procDragQueryFileW  = modShell32.NewProc("DragQueryFileW")
	procDragFinish      = modShell32.NewProc("DragFinish")
)

// --- Helpers ---------------------------------------------------------------
func rawUTF16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}
func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(localizeUI(s))
	return p
}

func getWindowTextString(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}
	n, _, _ := procGetWindowTextLengthW.Call(hwnd)
	buf := make([]uint16, int(n)+2)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	text := syscall.UTF16ToString(buf)
	if strings.EqualFold(getWindowClassNameString(hwnd), "COMBOBOX") {
		return internalUIValue(text)
	}
	return text
}

func sendMessage(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

const transparentColor uint32 = 0x01000000

func rgb(r, g, b byte) uint32 {
	return uint32(r) | uint32(g)<<8 | uint32(b)<<16
}

func darkModeEnabled() bool {
	return app != nil && app.darkMode
}

func checkerboardColors() (uint32, uint32) {
	if darkModeEnabled() {
		return rgb(12, 12, 12), rgb(62, 62, 62)
	}
	return rgb(255, 255, 255), rgb(224, 224, 224)
}

// uiColor maps the classic light Win32 chrome palette to a stable dark palette.
// It is deliberately used only for interface chrome: canvas pixels, palette
// swatches, clipboard images and file previews always retain their real colors.
func uiColor(c uint32) uint32 {
	if !darkModeEnabled() || isTransparentColor(c) {
		return c
	}
	r := int(c & 0xff)
	g := int((c >> 8) & 0xff)
	b := int((c >> 16) & 0xff)
	mx := max(r, max(g, b))
	mn := min(r, min(g, b))
	if mx-mn <= 25 {
		v := (r + g + b) / 3
		d := 232
		switch {
		case v >= 248:
			d = 24
		case v >= 238:
			d = 30
		case v >= 225:
			d = 37
		case v >= 205:
			d = 47
		case v >= 178:
			d = 62
		case v >= 145:
			d = 82
		case v >= 110:
			d = 112
		case v >= 75:
			d = 156
		case v >= 45:
			d = 196
		}
		return rgb(byte(d), byte(d), byte(d))
	}
	// Light-blue selection fills become dark-blue panels rather than glaring
	// white blocks, while saturated blue outlines are brightened.
	if r >= 175 && g >= 195 && b >= 225 && b-r >= 30 {
		return rgb(38, 68, 98)
	}
	if b > 145 && b >= r+35 && b >= g+20 {
		return rgb(byte(clamp(r+42, 0, 255)), byte(clamp(g+34, 0, 255)), byte(clamp(b+20, 0, 255)))
	}
	return c
}

func fillUIRect(hdc uintptr, rc RECT, c uint32)  { fillRect(hdc, rc, uiColor(c)) }
func frameUIRect(hdc uintptr, rc RECT, c uint32) { frameRect(hdc, rc, uiColor(c)) }
func drawUIText(hdc uintptr, text string, rc RECT, format uint32, c uint32) {
	drawText(hdc, text, rc, format, uiColor(c))
}

func isTransparentColor(c uint32) bool { return c == transparentColor }

func gdiPreviewColor(c uint32) uint32 {
	if isTransparentColor(c) {
		return rgb(96, 96, 96)
	}
	return c
}

func colorrefToBGRA(c uint32) [4]byte {
	if isTransparentColor(c) {
		return [4]byte{0, 0, 0, 0}
	}
	return [4]byte{byte((c >> 16) & 0xff), byte((c >> 8) & 0xff), byte(c & 0xff), 255}
}

func bgraToColorref(b, g, r byte) uint32 { return rgb(r, g, b) }

func rgba16ToStraight8(r, g, b, a uint32) (byte, byte, byte, byte) {
	if a == 0 {
		return 0, 0, 0, 0
	}
	convert := func(v uint32) byte {
		straight := (uint64(v)*0xffff + uint64(a)/2) / uint64(a)
		if straight > 0xffff {
			straight = 0xffff
		}
		return byte(straight >> 8)
	}
	return convert(r), convert(g), convert(b), byte(a >> 8)
}

func loword(v uintptr) uint16      { return uint16(v & 0xffff) }
func hiword(v uintptr) uint16      { return uint16((v >> 16) & 0xffff) }
func signedLoword(v uintptr) int32 { return int32(int16(loword(v))) }
func signedHiword(v uintptr) int32 { return int32(int16(hiword(v))) }

func messageBox(hwnd uintptr, text, title string, flags uint32) int {
	r, _, _ := procMessageBoxW.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(text))), uintptr(unsafe.Pointer(utf16Ptr(title))), uintptr(flags))
	return int(r)
}

func openDiagnosticLog(hwnd uintptr) {
	if diag.path == "" {
		messageBox(hwnd, "진단 로그 경로를 찾을 수 없습니다.", "dotPaint", MB_OK|MB_ICONERROR)
		return
	}
	r, _, err := procShellExecuteW.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr("open"))), uintptr(unsafe.Pointer(utf16Ptr(diag.path))), 0, 0, SW_SHOW)
	logf("OPEN_LOG path=%q result=%d err=%v", diag.path, r, err)
	if r <= 32 {
		messageBox(hwnd, "로그 파일을 열 수 없습니다.\n\n"+diag.path, "dotPaint", MB_OK|MB_ICONERROR)
	}
}

func invalidate(hwnd uintptr) {
	procInvalidateRect.Call(hwnd, 0, 0)
}

func invalidateArea(hwnd uintptr, rc *RECT) {
	if rc == nil {
		invalidate(hwnd)
		return
	}
	procInvalidateRect.Call(hwnd, uintptr(unsafe.Pointer(rc)), 0)
}

func rectEmpty(rc RECT) bool {
	return rc.Right <= rc.Left || rc.Bottom <= rc.Top
}

func expandRECT(rc RECT, margin int) RECT {
	if margin < 0 {
		margin = 0
	}
	rc.Left -= int32(margin)
	rc.Top -= int32(margin)
	rc.Right += int32(margin)
	rc.Bottom += int32(margin)
	return rc
}

func unionRECT(a, b RECT) RECT {
	if rectEmpty(a) {
		return b
	}
	if rectEmpty(b) {
		return a
	}
	return RECT{
		Left:   int32(min(int(a.Left), int(b.Left))),
		Top:    int32(min(int(a.Top), int(b.Top))),
		Right:  int32(max(int(a.Right), int(b.Right))),
		Bottom: int32(max(int(a.Bottom), int(b.Bottom))),
	}
}

func fillRect(hdc uintptr, rc RECT, c uint32) {
	br, _, _ := procCreateSolidBrush.Call(uintptr(c))
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), br)
	procDeleteObject.Call(br)
}

func frameRect(hdc uintptr, rc RECT, c uint32) {
	br, _, _ := procCreateSolidBrush.Call(uintptr(c))
	procFrameRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), br)
	procDeleteObject.Call(br)
}

func drawCheckerboard(hdc uintptr, rc RECT, cell int) {
	if cell < 2 {
		cell = 2
	}
	base, alternate := checkerboardColors()
	fillRect(hdc, rc, base)
	for y := int(rc.Top); y < int(rc.Bottom); y += cell {
		for x := int(rc.Left); x < int(rc.Right); x += cell {
			if ((x-int(rc.Left))/cell+(y-int(rc.Top))/cell)%2 == 0 {
				continue
			}
			r := RECT{int32(x), int32(y), int32(min(x+cell, int(rc.Right))), int32(min(y+cell, int(rc.Bottom)))}
			fillRect(hdc, r, alternate)
		}
	}
}

func (c *Canvas) displayPixels(zoom int) []byte {
	return c.displayPixelsAt(zoom, 0, 0)
}

// displayPixelsAt preserves the global checkerboard phase when a small preview
// region is rendered independently from the full canvas.  A209 uses this for
// line and arrow previews, avoiding a complete canvas clone and alpha scan on
// every mouse move while keeping transparent pixels visually identical.
func (c *Canvas) displayPixelsAt(zoom, originX, originY int) []byte {
	hasAlpha := false
	for i := 3; i < len(c.Pix); i += 4 {
		if c.Pix[i] != 255 {
			hasAlpha = true
			break
		}
	}
	if !hasAlpha {
		return c.Pix
	}
	out := make([]byte, len(c.Pix))
	cell := max(1, 8/max(zoom, 1))
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			i := (y*c.W + x) * 4
			a := int(c.Pix[i+3])
			bg := 255
			alt := 224
			if darkModeEnabled() {
				bg, alt = 12, 62
			}
			if (((x+originX)/cell)+((y+originY)/cell))%2 != 0 {
				bg = alt
			}
			out[i+0] = byte((int(c.Pix[i+0])*a + bg*(255-a) + 127) / 255)
			out[i+1] = byte((int(c.Pix[i+1])*a + bg*(255-a) + 127) / 255)
			out[i+2] = byte((int(c.Pix[i+2])*a + bg*(255-a) + 127) / 255)
			out[i+3] = 255
		}
	}
	return out
}

func drawText(hdc uintptr, text string, rc RECT, format uint32, c uint32) {
	procSetTextColor.Call(hdc, uintptr(c))
	procSetBkMode.Call(hdc, TRANSPARENT)
	p := utf16Ptr(text)
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&rc)), uintptr(format))
}

// Toolbox option labels sit in a deliberately narrow 98 px column. Use a
// smaller Malgun Gothic face for Korean descriptions so labels never protrude
// beyond their panels. Anti-aliasing labels use 8 px; ordinary labels use
// 9..10 px depending on available height.
func drawToolOptionText(hdc uintptr, text string, rc RECT, format uint32, c uint32, pixelHeight int) {
	// A108 prevented overflow but made the whole option column too tiny. A109
	// restored a midpoint size. A110 raised the user-designated captions by one
	// point, and A112 raises those same captions by one more point while leaving
	// numeric slider labels and unrelated descriptions unchanged.
	if pixelHeight <= 9 {
		pixelHeight++
	}
	if toolOptionCaptionNeedsExtraPoint(text) {
		pixelHeight += 2
	}
	font := createGDIFont("맑은 고딕", pixelHeight, false, false, false)
	if font == 0 {
		drawText(hdc, text, rc, format, uiColor(c))
		return
	}
	old, _, _ := procSelectObject.Call(hdc, font)
	drawTextUsingSelectedFont(hdc, text, rc, format, uiColor(c))
	procSelectObject.Call(hdc, old)
	procDeleteObject.Call(font)
}

func toolOptionCaptionNeedsExtraPoint(text string) bool {
	switch text {
	case "투명", "불투명", "허용 불일치", "Shift 제한", "안티에일리어싱", "Shift: 정사각형", "Shift: 원", "머리 크기":
		return true
	}
	return strings.HasPrefix(text, "굽힘점 수 ") || strings.HasPrefix(text, "선 굵기 ")
}

func selectDefaultFont(hdc uintptr) uintptr {
	font, _, _ := procGetStockObject.Call(DEFAULT_GUI_FONT)
	old, _, _ := procSelectObject.Call(hdc, font)
	return old
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// --- Pixel canvas ----------------------------------------------------------
type Canvas struct {
	W, H int
	Pix  []byte // BGRA, top-down
}

func NewCanvas(w, h int, c uint32) *Canvas {
	cv := &Canvas{W: w, H: h, Pix: make([]byte, w*h*4)}
	cv.Clear(c)
	return cv
}

func (c *Canvas) ClonePixels() []byte {
	p := make([]byte, len(c.Pix))
	copy(p, c.Pix)
	return p
}

func (c *Canvas) SetPixel(x, y int, clr uint32) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return
	}
	i := (y*c.W + x) * 4
	px := colorrefToBGRA(clr)
	c.Pix[i], c.Pix[i+1], c.Pix[i+2], c.Pix[i+3] = px[0], px[1], px[2], px[3]
}

func (c *Canvas) GetPixel(x, y int) uint32 {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return rgb(255, 255, 255)
	}
	i := (y*c.W + x) * 4
	if c.Pix[i+3] == 0 {
		return transparentColor
	}
	return bgraToColorref(c.Pix[i], c.Pix[i+1], c.Pix[i+2])
}

func (c *Canvas) Clear(clr uint32) {
	px := colorrefToBGRA(clr)
	for i := 0; i < len(c.Pix); i += 4 {
		c.Pix[i], c.Pix[i+1], c.Pix[i+2], c.Pix[i+3] = px[0], px[1], px[2], px[3]
	}
}

func (c *Canvas) DrawLine(x0, y0, x1, y1 int, clr uint32) {
	dx := int(math.Abs(float64(x1 - x0)))
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -int(math.Abs(float64(y1 - y0)))
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		c.SetPixel(x0, y0, clr)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// blendPixelCoverage composites one raster-line sample into the straight-alpha
// BGRA canvas. Transparent foreground acts as a partial eraser when coverage is
// below one, which lets anti-aliased transparent lines keep a soft alpha edge.
func (c *Canvas) blendPixelCoverage(x, y int, clr uint32, coverage float64) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H || coverage <= 0 {
		return
	}
	if coverage > 1 {
		coverage = 1
	}
	i := (y*c.W + x) * 4
	dstA := float64(c.Pix[i+3]) / 255.0
	if isTransparentColor(clr) {
		outA := dstA * (1.0 - coverage)
		c.Pix[i+3] = byte(math.Round(outA * 255.0))
		if c.Pix[i+3] == 0 {
			c.Pix[i], c.Pix[i+1], c.Pix[i+2] = 0, 0, 0
		}
		return
	}
	src := colorrefToBGRA(clr)
	srcA := coverage
	outA := srcA + dstA*(1.0-srcA)
	if outA <= 0 {
		c.Pix[i], c.Pix[i+1], c.Pix[i+2], c.Pix[i+3] = 0, 0, 0, 0
		return
	}
	for ch := 0; ch < 3; ch++ {
		dst := float64(c.Pix[i+ch])
		s := float64(src[ch])
		out := (s*srcA + dst*dstA*(1.0-srcA)) / outA
		c.Pix[i+ch] = byte(clamp(int(math.Round(out)), 0, 255))
	}
	c.Pix[i+3] = byte(clamp(int(math.Round(outA*255.0)), 0, 255))
}

func pointSegmentDistance(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	denom := dx*dx + dy*dy
	if denom <= 0 {
		return math.Hypot(px-x0, py-y0)
	}
	t := ((px-x0)*dx + (py-y0)*dy) / denom
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	qx, qy := x0+t*dx, y0+t*dy
	return math.Hypot(px-qx, py-qy)
}

// applyVectorAlphaMask composites an invisible vector contribution mask once.
// Component geometry is unioned in the mask first, avoiding overlap darkening
// and making preview and commit use the exact same alpha values.
func (c *Canvas) applyVectorAlphaMask(mask *vectorAlphaMask, clr uint32) {
	if mask == nil || mask.W <= 0 || mask.H <= 0 {
		return
	}
	// A112 records exactly which alpha cells became non-zero. Long diagonal
	// arrows used to scan the entire enclosing rectangle again during compositing,
	// even though almost all of it was empty. Iterating touched cells preserves
	// every alpha value while avoiding those empty corners a second time.
	for _, idx := range mask.Touched {
		if idx < 0 || idx >= len(mask.Alpha) {
			continue
		}
		x := mask.Left + idx%mask.W
		y := mask.Top + idx/mask.W
		if x < 0 || x >= c.W || y < 0 || y >= c.H {
			continue
		}
		a := mask.Alpha[idx]
		if a == 255 {
			c.SetPixel(x, y, clr)
		} else if a != 0 {
			c.blendPixelCoverage(x, y, clr, float64(a)/255.0)
		}
	}
}

// DrawStyledLine is the single source of truth for preview and commit. A91
// shares the arrow shaft's square brush, flat caps, odd/even half-pixel axis,
// and eight-way AA bypass. Horizontal, vertical, and exact 45-degree lines stay
// fully opaque even when AA is enabled; only arbitrary slopes receive coverage.
func (c *Canvas) DrawStyledLine(x0, y0, x1, y1, size int, antialias bool, clr uint32) {
	size = clamp(size, 1, 40)
	offsetX, offsetY := arrowStrokeOffset(x0, y0, x1, y1, size)
	effectiveAA := arrowEffectiveAntialias(antialias, x0, y0, x1, y1)
	c.DrawArrowShaftLine(x0, y0, x1, y1, size, effectiveAA, offsetX, offsetY, clr)
}

// DrawStyledPixelPath keeps strict tan-n staircase geometry while using the
// same square brush and parity-aware axis as arrows. It never replaces the
// integer path with an unrelated floating vector.
func (c *Canvas) DrawStyledPixelPath(points []PixelPoint, size int, antialias bool, clr uint32) {
	if len(points) == 0 {
		return
	}
	size = clamp(size, 1, 40)
	first := points[0]
	last := points[len(points)-1]
	offsetX, offsetY := arrowStrokeOffset(first.X, first.Y, last.X, last.Y, size)
	effectiveAA := arrowEffectiveAntialias(antialias, first.X, first.Y, last.X, last.Y)
	c.DrawArrowShaftPixelPath(points, size, effectiveAA, offsetX, offsetY, clr)
}

// DrawRoundCurvePixelPath renders the curve tool with one filled circular nib
// stamped along its complete integer centreline.  The former implementation
// reused the arrow shaft's square, butt-capped rectangles for every one-pixel
// Bezier segment.  At a change of tangent those tiny rotated rectangles exposed
// their corners as outward "thorns", and their projected width varied with
// direction.  A union of identical circular nibs gives round joins/caps and a
// direction-independent nominal thickness.  Preview and commit share this
// exact renderer.
func (c *Canvas) DrawRoundCurvePixelPath(points []PixelPoint, size int, antialias bool, clr uint32) {
	if len(points) == 0 || c == nil || c.W <= 0 || c.H <= 0 {
		return
	}
	size = clamp(size, 1, 40)
	nib := buildCurveRoundNib(size, antialias)
	if len(nib) == 0 {
		return
	}

	minX, maxX := points[0].X, points[0].X
	minY, maxY := points[0].Y, points[0].Y
	minDX, maxDX := nib[0].DX, nib[0].DX
	minDY, maxDY := nib[0].DY, nib[0].DY
	for _, p := range points[1:] {
		minX, maxX = min(minX, p.X), max(maxX, p.X)
		minY, maxY = min(minY, p.Y), max(maxY, p.Y)
	}
	for _, cell := range nib[1:] {
		minDX, maxDX = min(minDX, cell.DX), max(maxDX, cell.DX)
		minDY, maxDY = min(minDY, cell.DY), max(maxDY, cell.DY)
	}
	left, right := minX+minDX, maxX+maxDX
	top, bottom := minY+minDY, maxY+maxDY
	if right < 0 || bottom < 0 || left >= c.W || top >= c.H {
		return
	}
	left, right = clamp(left, 0, c.W-1), clamp(right, 0, c.W-1)
	top, bottom = clamp(top, 0, c.H-1), clamp(bottom, 0, c.H-1)
	mask := newVectorAlphaMask(left, top, right, bottom)
	for _, p := range points {
		for _, cell := range nib {
			x, y := p.X+cell.DX, p.Y+cell.DY
			if cell.Coverage >= 1 {
				mask.setOpaque(x, y)
			} else {
				mask.setMaxCoverage(x, y, cell.Coverage)
			}
		}
	}
	c.applyVectorAlphaMask(mask, clr)
}

// DrawStyledStrictTanPath preserves the exact integer tan staircase as the
// fully opaque core, then derives its AA fringe from one ideal continuous line
// joining the first and last staircase pixels.  It deliberately does not
// antialias every one-pixel subsegment: doing so creates a grey cap around each
// micro-step and can put grey pixels on the visually wrong side of a descending
// line.  Preview and commit share this path.
func (c *Canvas) DrawStyledStrictTanPath(points []PixelPoint, stepX, stepY, majorSteps, size int, antialias bool, clr uint32) {
	if len(points) == 0 {
		return
	}
	size = clamp(size, 1, 40)
	first := points[0]
	// The selected reduced ratio, not the current partial staircase endpoint,
	// owns the parity axis and AA policy. Otherwise tan 1/3 would temporarily
	// look horizontal for its first two pixels and change phase as it grows.
	offsetX, offsetY := arrowStrokeOffset(first.X, first.Y, first.X+stepX, first.Y+stepY, size)
	effectiveAA := arrowEffectiveAntialias(antialias, 0, 0, stepX, stepY)
	c.DrawArrowShaftStrictTanPixelPath(points, stepX, stepY, majorSteps, size, effectiveAA, offsetX, offsetY, clr)
}

// DrawArrowShaftLine renders a square/butt-capped arrow shaft on the same
// parity-aware floating axis as its head.  Even widths use the upper/left
// half-pixel axis chosen by arrowStrokeOffset.  The flat tail and rear edge avoid
// the round blobs produced by distance-to-segment caps.
func (c *Canvas) DrawArrowShaftLine(x0, y0, x1, y1, size int, antialias bool, offsetX, offsetY float64, clr uint32) {
	size = clamp(size, 1, 40)
	fx0, fy0 := float64(x0)+offsetX, float64(y0)+offsetY
	fx1, fy1 := float64(x1)+offsetX, float64(y1)+offsetY
	halfWidth := float64(size) / 2.0
	margin := int(math.Ceil(halfWidth + math.Max(math.Abs(offsetX), math.Abs(offsetY)) + 1.5))
	left := clamp(min(x0, x1)-margin, 0, c.W-1)
	right := clamp(max(x0, x1)+margin, 0, c.W-1)
	top := clamp(min(y0, y1)-margin, 0, c.H-1)
	bottom := clamp(max(y0, y1)+margin, 0, c.H-1)
	mask := newVectorAlphaMask(left, top, right, bottom)
	candidates := newVectorCoverageCandidates(left, top, right, bottom)
	candidates.markStroke(fx0, fy0, fx1, fy1, size)
	candidates.forEach(func(x, y int) {
		if !antialias {
			if pointInsideArrowStroke(float64(x), float64(y), fx0, fy0, fx1, fy1, size) {
				mask.setOpaque(x, y)
			}
			return
		}
		mask.setMaxCoverage(x, y, arrowStrokeCoverageAt(float64(x), float64(y), fx0, fy0, fx1, fy1, size))
	})
	c.applyVectorAlphaMask(mask, clr)
}

// DrawArrowShaftPixelPath applies the same single perpendicular parity offset
// and square/butt-capped brush to every segment of a strict tan staircase.  The
// path remains a true integer tan-n staircase while cardinal subsegments stay
// fully opaque even with AA enabled.
func (c *Canvas) DrawArrowShaftPixelPath(points []PixelPoint, size int, antialias bool, offsetX, offsetY float64, clr uint32) {
	if len(points) == 0 {
		return
	}
	if len(points) == 1 {
		c.DrawArrowShaftLine(points[0].X, points[0].Y, points[0].X, points[0].Y, size, antialias, offsetX, offsetY, clr)
		return
	}
	size = clamp(size, 1, 40)
	halfWidth := float64(size) / 2.0
	margin := int(math.Ceil(halfWidth + math.Max(math.Abs(offsetX), math.Abs(offsetY)) + 1.0))
	if !antialias {
		for i := 1; i < len(points); i++ {
			p0, p1 := points[i-1], points[i]
			fx0, fy0 := float64(p0.X)+offsetX, float64(p0.Y)+offsetY
			fx1, fy1 := float64(p1.X)+offsetX, float64(p1.Y)+offsetY
			left := clamp(min(p0.X, p1.X)-margin, 0, c.W-1)
			right := clamp(max(p0.X, p1.X)+margin, 0, c.W-1)
			top := clamp(min(p0.Y, p1.Y)-margin, 0, c.H-1)
			bottom := clamp(max(p0.Y, p1.Y)+margin, 0, c.H-1)
			for y := top; y <= bottom; y++ {
				for x := left; x <= right; x++ {
					if pointInsideArrowStroke(float64(x), float64(y), fx0, fy0, fx1, fy1, size) {
						c.SetPixel(x, y, clr)
					}
				}
			}
		}
		return
	}

	coverage := make(map[int]float64, len(points)*(size+2))
	for i := 1; i < len(points); i++ {
		p0, p1 := points[i-1], points[i]
		fx0, fy0 := float64(p0.X)+offsetX, float64(p0.Y)+offsetY
		fx1, fy1 := float64(p1.X)+offsetX, float64(p1.Y)+offsetY
		left := clamp(min(p0.X, p1.X)-margin, 0, c.W-1)
		right := clamp(max(p0.X, p1.X)+margin, 0, c.W-1)
		top := clamp(min(p0.Y, p1.Y)-margin, 0, c.H-1)
		bottom := clamp(max(p0.Y, p1.Y)+margin, 0, c.H-1)
		for y := top; y <= bottom; y++ {
			for x := left; x <= right; x++ {
				cov := arrowStrokeCoverageAt(float64(x), float64(y), fx0, fy0, fx1, fy1, size)
				if cov <= 0 {
					continue
				}
				idx := y*c.W + x
				if cov > coverage[idx] {
					coverage[idx] = cov
				}
			}
		}
	}
	for idx, cov := range coverage {
		c.blendPixelCoverage(idx%c.W, idx/c.W, clr, cov)
	}
}

// DrawArrowShaftStrictTanPixelPath is the directed AA renderer for strict tan
// staircases.  The integer staircase itself remains fully opaque and exact.
// Only pixels outside that core may receive coverage from the single ideal
// line that the staircase approximates.  This removes repeated subsegment-cap
// halos without deleting AA outright.
func (c *Canvas) DrawArrowShaftStrictTanPixelPath(points []PixelPoint, stepX, stepY, majorSteps, size int, antialias bool, offsetX, offsetY float64, clr uint32) {
	if len(points) == 0 {
		return
	}
	if len(points) == 1 {
		c.DrawArrowShaftPixelPath(points, size, antialias, offsetX, offsetY, clr)
		return
	}

	size = clamp(size, 1, 40)
	halfWidth := float64(size) / 2.0
	margin := int(math.Ceil(halfWidth + math.Max(math.Abs(offsetX), math.Abs(offsetY)) + 1.5))
	minX, maxX := points[0].X, points[0].X
	minY, maxY := points[0].Y, points[0].Y
	for _, p := range points[1:] {
		minX, maxX = min(minX, p.X), max(maxX, p.X)
		minY, maxY = min(minY, p.Y), max(maxY, p.Y)
	}
	left := clamp(minX-margin, 0, c.W-1)
	right := clamp(maxX+margin, 0, c.W-1)
	top := clamp(minY-margin, 0, c.H-1)
	bottom := clamp(maxY+margin, 0, c.H-1)
	mask := newVectorAlphaMask(left, top, right, bottom)

	// Invisible ideal vector fringe. A97 derives it from the selected reduced
	// slope and dominant-axis length, not from the staircase's current last
	// pixel. Its fixed motif-centred phase balances AA on the two sides and does
	// not drift when the line grows by another native pixel.
	first := points[0]
	shadow, shadowOK := calculateStrictTanShadowSegment(first.X, first.Y, stepX, stepY, majorSteps, size)
	if antialias && shadowOK {
		candidates := newVectorCoverageCandidates(left, top, right, bottom)
		candidates.markStroke(shadow.X0, shadow.Y0, shadow.X1, shadow.Y1, size)
		candidates.forEach(func(x, y int) {
			mask.setMaxCoverage(x, y, arrowStrokeLongEdgesCoverageAt(float64(x), float64(y), shadow.X0, shadow.Y0, shadow.X1, shadow.Y1, size))
		})
	}

	// Exact integer staircase is unioned as the fully opaque core before the
	// mask is composited, rather than painted in a separate pass.
	for i := 1; i < len(points); i++ {
		p0, p1 := points[i-1], points[i]
		sx0, sy0 := float64(p0.X)+offsetX, float64(p0.Y)+offsetY
		sx1, sy1 := float64(p1.X)+offsetX, float64(p1.Y)+offsetY
		segLeft := clamp(min(p0.X, p1.X)-margin, left, right)
		segRight := clamp(max(p0.X, p1.X)+margin, left, right)
		segTop := clamp(min(p0.Y, p1.Y)-margin, top, bottom)
		segBottom := clamp(max(p0.Y, p1.Y)+margin, top, bottom)
		for y := segTop; y <= segBottom; y++ {
			for x := segLeft; x <= segRight; x++ {
				if shadowOK && !shadow.containsRasterCentre(float64(x), float64(y)) {
					continue
				}
				if pointInsideArrowStroke(float64(x), float64(y), sx0, sy0, sx1, sy1, size) {
					mask.setOpaque(x, y)
				}
			}
		}
	}
	c.applyVectorAlphaMask(mask, clr)
	if size == 1 && len(points) > 0 {
		// The continuous-shadow clip can reject the first raster centre for a
		// one-pixel strict tan staircase. Restore the logical origin explicitly;
		// this is visually identical to the intended exact pixel path and is used
		// by both live preview and final commit.
		c.SetPixel(points[0].X, points[0].Y, clr)
	}
}

// DrawUnifiedVectorArrow builds one hidden alpha mask for the shaft and head.
// The two vector components are unioned at every sub-pixel sample and written
// to the canvas once, eliminating neck seams and double-dark overlap pixels.
func (c *Canvas) DrawUnifiedVectorArrow(x0, y0, x1, y1, size int, shaftAA bool, head ArrowHeadGeometry, headAA bool, strictPath []PixelPoint, strictShadow *strictTanShadowSegment, clr uint32) {
	size = clamp(size, 1, 40)
	hasShaft := x0 != x1 || y0 != y1
	fx0, fy0 := float64(x0)+head.PixelOffsetX, float64(y0)+head.PixelOffsetY
	fx1, fy1 := float64(x1)+head.PixelOffsetX, float64(y1)+head.PixelOffsetY
	if strictShadow != nil && strictShadow.Length > 0 {
		hasShaft = true
		fx0, fy0 = strictShadow.X0, strictShadow.Y0
		fx1, fy1 = strictShadow.X1, strictShadow.Y1
	}
	halfWidth := float64(size) / 2.0
	minX := math.Min(head.Tip.X-head.TipHalfWidth, math.Min(head.SideA.X, head.SideB.X))
	maxX := math.Max(head.Tip.X+head.TipHalfWidth, math.Max(head.SideA.X, head.SideB.X))
	minY := math.Min(head.Tip.Y-head.TipHalfWidth, math.Min(head.SideA.Y, head.SideB.Y))
	maxY := math.Max(head.Tip.Y+head.TipHalfWidth, math.Max(head.SideA.Y, head.SideB.Y))
	if hasShaft {
		minX = math.Min(minX, math.Min(fx0, fx1)-halfWidth-1)
		maxX = math.Max(maxX, math.Max(fx0, fx1)+halfWidth+1)
		minY = math.Min(minY, math.Min(fy0, fy1)-halfWidth-1)
		maxY = math.Max(maxY, math.Max(fy0, fy1)+halfWidth+1)
	}
	left := clamp(int(math.Floor(minX))-1, 0, c.W-1)
	right := clamp(int(math.Ceil(maxX))+1, 0, c.W-1)
	top := clamp(int(math.Floor(minY))-1, 0, c.H-1)
	bottom := clamp(int(math.Ceil(maxY))+1, 0, c.H-1)
	if left > right || top > bottom {
		return
	}
	mask := newVectorAlphaMask(left, top, right, bottom)
	candidates := newVectorCoverageCandidates(left, top, right, bottom)
	if hasShaft {
		candidates.markStroke(fx0, fy0, fx1, fy1, size)
	}
	headLeft := int(math.Floor(math.Min(head.Tip.X-head.TipHalfWidth, math.Min(head.SideA.X, head.SideB.X)))) - 1
	headRight := int(math.Ceil(math.Max(head.Tip.X+head.TipHalfWidth, math.Max(head.SideA.X, head.SideB.X)))) + 1
	headTop := int(math.Floor(math.Min(head.Tip.Y-head.TipHalfWidth, math.Min(head.SideA.Y, head.SideB.Y)))) - 1
	headBottom := int(math.Ceil(math.Max(head.Tip.Y+head.TipHalfWidth, math.Max(head.SideA.Y, head.SideB.Y)))) + 1
	candidates.markRect(headLeft, headTop, headRight, headBottom)
	candidates.forEach(func(x, y int) {
		cov := arrowCompositeCoverageAt(float64(x), float64(y), hasShaft, fx0, fy0, fx1, fy1, size, shaftAA, head, headAA)
		mask.setMaxCoverage(x, y, cov)
	})

	// Strict tan mode contributes its exact integer staircase to the same mask.
	if len(strictPath) > 1 {
		margin := int(math.Ceil(halfWidth + math.Max(math.Abs(head.PixelOffsetX), math.Abs(head.PixelOffsetY)) + 1.5))
		for i := 1; i < len(strictPath); i++ {
			p0, p1 := strictPath[i-1], strictPath[i]
			sx0, sy0 := float64(p0.X)+head.PixelOffsetX, float64(p0.Y)+head.PixelOffsetY
			sx1, sy1 := float64(p1.X)+head.PixelOffsetX, float64(p1.Y)+head.PixelOffsetY
			segLeft := clamp(min(p0.X, p1.X)-margin, left, right)
			segRight := clamp(max(p0.X, p1.X)+margin, left, right)
			segTop := clamp(min(p0.Y, p1.Y)-margin, top, bottom)
			segBottom := clamp(max(p0.Y, p1.Y)+margin, top, bottom)
			for y := segTop; y <= segBottom; y++ {
				for x := segLeft; x <= segRight; x++ {
					if pointInsideArrowStroke(float64(x), float64(y), sx0, sy0, sx1, sy1, size) {
						mask.setOpaque(x, y)
					}
				}
			}
		}
	}
	c.applyVectorAlphaMask(mask, clr)
}

// FillStyledArrowHead rasterizes the parity-aware 1:3 arrow head directly
// into the native canvas. Each of the four polygon edges chooses AA from its own
// angle: horizontal, vertical, and exact 45-degree edges stay binary, while all
// other edges use 4x4 coverage. Thus a tan(1/3) arrow correctly gets one crisp
// horizontal side and one antialiased tan(3/4) side.
func (c *Canvas) FillStyledArrowHead(g ArrowHeadGeometry, antialias bool, clr uint32) {
	minX := math.Min(g.Tip.X-g.TipHalfWidth, math.Min(g.SideA.X, g.SideB.X))
	maxX := math.Max(g.Tip.X+g.TipHalfWidth, math.Max(g.SideA.X, g.SideB.X))
	minY := math.Min(g.Tip.Y-g.TipHalfWidth, math.Min(g.SideA.Y, g.SideB.Y))
	maxY := math.Max(g.Tip.Y+g.TipHalfWidth, math.Max(g.SideA.Y, g.SideB.Y))
	left := clamp(int(math.Floor(minX))-1, 0, c.W-1)
	right := clamp(int(math.Ceil(maxX))+1, 0, c.W-1)
	top := clamp(int(math.Floor(minY))-1, 0, c.H-1)
	bottom := clamp(int(math.Ceil(maxY))+1, 0, c.H-1)
	if left > right || top > bottom {
		return
	}
	for y := top; y <= bottom; y++ {
		for x := left; x <= right; x++ {
			coverage := arrowHeadCoverageAt(float64(x), float64(y), g, antialias)
			if coverage <= 0 {
				continue
			}
			if coverage >= 1 {
				c.SetPixel(x, y, clr)
			} else {
				c.blendPixelCoverage(x, y, clr, coverage)
			}
		}
	}
}

func cubicBezierPoint(p0, p1, p2, p3 int, t float64) int {
	u := 1.0 - t
	v := u*u*u*float64(p0) + 3*u*u*t*float64(p1) + 3*u*t*t*float64(p2) + t*t*t*float64(p3)
	return int(math.Round(v))
}

func (c *Canvas) DrawBezier(x0, y0, c1x, c1y, c2x, c2y, x1, y1 int, clr uint32) {
	distance := math.Hypot(float64(x1-x0), float64(y1-y0)) +
		math.Hypot(float64(c1x-x0), float64(c1y-y0)) +
		math.Hypot(float64(c2x-x1), float64(c2y-y1))
	steps := int(distance * 2)
	if steps < 48 {
		steps = 48
	}
	if steps > 4096 {
		steps = 4096
	}
	px, py := x0, y0
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		nx := cubicBezierPoint(x0, c1x, c2x, x1, t)
		ny := cubicBezierPoint(y0, c1y, c2y, y1, t)
		c.DrawLine(px, py, nx, ny, clr)
		px, py = nx, ny
	}
}

func quarticBezierPoint(p0, p1, p2, p3, p4 int, t float64) int {
	u := 1.0 - t
	v := u*u*u*u*float64(p0) + 4*u*u*u*t*float64(p1) + 6*u*u*t*t*float64(p2) + 4*u*t*t*t*float64(p3) + t*t*t*t*float64(p4)
	return int(math.Round(v))
}

// bezierCoordinate evaluates a Bezier curve of any degree with de Casteljau's
// algorithm. Curve stages use degree 1 through 5 progressively, so a bend
// point that has not yet been chosen cannot influence the visible curve.
func bezierCoordinate(values []int, t float64) int {
	if len(values) == 0 {
		return 0
	}
	work := make([]float64, len(values))
	for i, v := range values {
		work[i] = float64(v)
	}
	for n := len(work) - 1; n > 0; n-- {
		for i := 0; i < n; i++ {
			work[i] = work[i]*(1.0-t) + work[i+1]*t
		}
	}
	return int(math.Round(work[0]))
}

func appendRasterSegment(points []PixelPoint, x0, y0, x1, y1 int) []PixelPoint {
	dx := absInt(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -absInt(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		if len(points) == 0 || points[len(points)-1].X != x0 || points[len(points)-1].Y != y0 {
			points = append(points, PixelPoint{X: x0, Y: y0})
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
	return points
}

// bezierRasterPath samples a Bezier curve and connects every sample with an
// integer Bresenham segment. The resulting spine is shared by preview and
// commit, including thick and anti-aliased curves.
func bezierRasterPath(control []PixelPoint) []PixelPoint {
	if len(control) < 2 {
		return append([]PixelPoint(nil), control...)
	}
	distance := 0.0
	for i := 1; i < len(control); i++ {
		distance += math.Hypot(float64(control[i].X-control[i-1].X), float64(control[i].Y-control[i-1].Y))
	}
	steps := int(math.Ceil(distance * 2.0))
	if steps < 48 {
		steps = 48
	}
	if steps > 8192 {
		steps = 8192
	}
	xs := make([]int, len(control))
	ys := make([]int, len(control))
	for i, p := range control {
		xs[i], ys[i] = p.X, p.Y
	}
	path := make([]PixelPoint, 0, steps+1)
	px, py := control[0].X, control[0].Y
	path = append(path, PixelPoint{X: px, Y: py})
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		nx := bezierCoordinate(xs, t)
		ny := bezierCoordinate(ys, t)
		path = appendRasterSegment(path, px, py, nx, ny)
		px, py = nx, ny
	}
	return path
}

// DrawQuarticBezier draws a smooth curve with three independently adjustable
// control points. This gives dotPaint one more bend-adjustment stage than the
// classic two-control-point curve tool.
func (c *Canvas) DrawQuarticBezier(x0, y0, c1x, c1y, c2x, c2y, c3x, c3y, x1, y1 int, clr uint32) {
	distance := math.Hypot(float64(x1-x0), float64(y1-y0)) +
		math.Hypot(float64(c1x-x0), float64(c1y-y0)) +
		math.Hypot(float64(c2x-c1x), float64(c2y-c1y)) +
		math.Hypot(float64(c3x-c2x), float64(c3y-c2y)) +
		math.Hypot(float64(x1-c3x), float64(y1-c3y))
	steps := int(distance * 2)
	if steps < 64 {
		steps = 64
	}
	if steps > 8192 {
		steps = 8192
	}
	px, py := x0, y0
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		nx := quarticBezierPoint(x0, c1x, c2x, c3x, x1, t)
		ny := quarticBezierPoint(y0, c1y, c2y, c3y, y1, t)
		c.DrawLine(px, py, nx, ny, clr)
		px, py = nx, ny
	}
}

func (c *Canvas) StampCircle(cx, cy, radius int, clr uint32) {
	rr := radius * radius
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			if x*x+y*y <= rr {
				c.SetPixel(cx+x, cy+y, clr)
			}
		}
	}
}

func squareStampBounds(cx, cy, size int) (left, top, right, bottom int) {
	if size < 1 {
		size = 1
	}
	left = cx - (size-1)/2
	top = cy - (size-1)/2
	right = left + size
	bottom = top + size
	return
}

func (c *Canvas) StampSquare(cx, cy, size int, clr uint32) {
	left, top, right, bottom := squareStampBounds(cx, cy, size)
	for y := top; y < bottom; y++ {
		for x := left; x < right; x++ {
			c.SetPixel(x, y, clr)
		}
	}
}

func (c *Canvas) DrawBrushLine(x0, y0, x1, y1, radius int, clr uint32) {
	dx := int(math.Abs(float64(x1 - x0)))
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -int(math.Abs(float64(y1 - y0)))
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		c.StampCircle(x0, y0, radius, clr)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func (c *Canvas) DrawEraserLine(x0, y0, x1, y1, size int, clr uint32) {
	dx := int(math.Abs(float64(x1 - x0)))
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -int(math.Abs(float64(y1 - y0)))
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		c.StampSquare(x0, y0, size, clr)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func (c *Canvas) StampBrush(cx, cy, size int, shape BrushShape, clr uint32) {
	if size < 1 {
		size = 1
	}
	left, top, right, bottom := squareStampBounds(cx, cy, size)
	switch shape {
	case BrushCircle:
		centerX := (float64(left) + float64(right-1)) / 2.0
		centerY := (float64(top) + float64(bottom-1)) / 2.0
		radius := float64(size) / 2.0
		if radius < 0.5 {
			radius = 0.5
		}
		for y := top; y < bottom; y++ {
			for x := left; x < right; x++ {
				dx := float64(x) - centerX
				dy := float64(y) - centerY
				if dx*dx+dy*dy <= radius*radius {
					c.SetPixel(x, y, clr)
				}
			}
		}
	case BrushSlashUp, BrushSlashDown:
		// A diagonal nib uses at least a 2×2 footprint and the same minimum band
		// used from 9 px upward. This prevents the striped, detached appearance
		// produced by a nominally one-pixel diagonal on curved strokes.
		nibSize := diagonalBrushFootprintSize(size)
		nibLeft, nibTop, _, _ := squareStampBounds(cx, cy, nibSize)
		band := diagonalBrushBand(nibSize)
		for y := 0; y < nibSize; y++ {
			for x := 0; x < nibSize; x++ {
				d := x - y
				if shape == BrushSlashUp {
					d = x + y - (nibSize - 1)
				}
				if absInt(d) <= band {
					c.SetPixel(nibLeft+x, nibTop+y, clr)
				}
			}
		}
	default:
		c.StampSquare(cx, cy, size, clr)
	}
}

func (c *Canvas) DrawUnifiedBrushLine(x0, y0, x1, y1, size int, shape BrushShape, clr uint32) {
	dx := absInt(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -absInt(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		c.StampBrush(x0, y0, size, shape, clr)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// DrawRectOuter draws an unfilled rectangle whose drag coordinates are the
// outside pixel bounds. Stroke thickness grows only inward, so changing the
// width never expands the visible rectangle beyond the requested outer size.
func (c *Canvas) DrawRectOuter(x0, y0, x1, y1, size int, clr uint32) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	x0 = clamp(x0, 0, c.W-1)
	x1 = clamp(x1, 0, c.W-1)
	y0 = clamp(y0, 0, c.H-1)
	y1 = clamp(y1, 0, c.H-1)
	w := x1 - x0 + 1
	h := y1 - y0 + 1
	if w <= 0 || h <= 0 {
		return
	}
	size = clamp(size, 1, 40)
	thickness := min(size, min(w, h))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if x-x0 < thickness || x1-x < thickness || y-y0 < thickness || y1-y < thickness {
				c.SetPixel(x, y, clr)
			}
		}
	}
}

func (c *Canvas) DrawRect(x0, y0, x1, y1 int, clr uint32) {
	c.DrawRectOuter(x0, y0, x1, y1, 1, clr)
}

func (c *Canvas) DrawEllipseOuter(x0, y0, x1, y1, size int, clr uint32) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	x0 = clamp(x0, 0, c.W-1)
	x1 = clamp(x1, 0, c.W-1)
	y0 = clamp(y0, 0, c.H-1)
	y1 = clamp(y1, 0, c.H-1)
	w := x1 - x0 + 1
	h := y1 - y0 + 1
	if w <= 0 || h <= 0 {
		return
	}
	size = clamp(size, 1, 40)
	thickness := min(size, min(w, h))
	if w <= 2 || h <= 2 {
		c.DrawRectOuter(x0, y0, x1, y1, thickness, clr)
		return
	}

	// A93: build the stroke as the exact difference of two concentric analytic
	// ellipses and flatten cardinal tips on axes larger than 20px. The inner and
	// outer boundaries use the same tiny symmetric cap, removing axis vertices
	// without changing the requested bounds or the inward-only stroke thickness.
	// Quadrant mirroring keeps left/right and top/bottom pixels exactly identical.
	mask := symmetricEllipseRingMask(w, h, thickness)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mask[y*w+x] {
				c.SetPixel(x0+x, y0+y, clr)
			}
		}
	}
}

func (c *Canvas) DrawEllipse(x0, y0, x1, y1 int, clr uint32) {
	c.DrawEllipseOuter(x0, y0, x1, y1, 1, clr)
}

func (c *Canvas) FloodFill(x, y int, newColor uint32, tolerance int) bool {
	started := time.Now()
	tolerance = clampColorTolerance(tolerance)
	logf("FILL begin x=%d y=%d new=%06X tolerance=%d canvas=%dx%d pix=%d", x, y, newColor&0xFFFFFF, tolerance, c.W, c.H, len(c.Pix))
	replacement := colorrefToBGRA(newColor)
	filled := toleranceFloodFillBGRA(c.Pix, c.W, c.H, x, y, replacement, tolerance)
	logf("FILL end filled=%d tolerance=%d elapsed=%s", filled, tolerance, time.Since(started))
	return filled > 0
}

func (c *Canvas) Invert() {
	for i := 0; i < len(c.Pix); i += 4 {
		c.Pix[i] = 255 - c.Pix[i]
		c.Pix[i+1] = 255 - c.Pix[i+1]
		c.Pix[i+2] = 255 - c.Pix[i+2]
	}
}

func (c *Canvas) FlipHorizontal() {
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W/2; x++ {
			a := (y*c.W + x) * 4
			b := (y*c.W + (c.W - 1 - x)) * 4
			for k := 0; k < 4; k++ {
				c.Pix[a+k], c.Pix[b+k] = c.Pix[b+k], c.Pix[a+k]
			}
		}
	}
}

func (c *Canvas) FlipVertical() {
	row := c.W * 4
	tmp := make([]byte, row)
	for y := 0; y < c.H/2; y++ {
		a := y * row
		b := (c.H - 1 - y) * row
		copy(tmp, c.Pix[a:a+row])
		copy(c.Pix[a:a+row], c.Pix[b:b+row])
		copy(c.Pix[b:b+row], tmp)
	}
}

func (c *Canvas) ResizeNearest(nw, nh int, bg uint32) {
	if nw < 1 || nh < 1 {
		return
	}
	out := NewCanvas(nw, nh, bg)
	for y := 0; y < nh; y++ {
		sy := y * c.H / nh
		for x := 0; x < nw; x++ {
			sx := x * c.W / nw
			si := (sy*c.W + sx) * 4
			di := (y*nw + x) * 4
			copy(out.Pix[di:di+4], c.Pix[si:si+4])
		}
	}
	c.W, c.H, c.Pix = out.W, out.H, out.Pix
}

// ResizeCanvas changes only the canvas bounds. Existing pixels are copied
// without scaling. offsetX/offsetY specify where the old top-left pixel is
// placed in the new canvas, allowing all eight resize handles to crop or add
// space from any side.
func (c *Canvas) ResizeCanvas(nw, nh, offsetX, offsetY int, bg uint32) {
	if nw < 1 || nh < 1 {
		return
	}
	out := NewCanvas(nw, nh, bg)
	for y := 0; y < c.H; y++ {
		ny := y + offsetY
		if ny < 0 || ny >= nh {
			continue
		}
		for x := 0; x < c.W; x++ {
			nx := x + offsetX
			if nx < 0 || nx >= nw {
				continue
			}
			si := (y*c.W + x) * 4
			di := (ny*nw + nx) * 4
			copy(out.Pix[di:di+4], c.Pix[si:si+4])
		}
	}
	c.W, c.H, c.Pix = out.W, out.H, out.Pix
}

// --- Undo ------------------------------------------------------------------
type Snapshot struct {
	W, H int
	Pix  []byte
}
type UndoManager struct {
	undo  []Snapshot
	redo  []Snapshot
	limit int
}

func NewUndoManager(limit int) *UndoManager        { return &UndoManager{limit: limit} }
func (u *UndoManager) snapshot(c *Canvas) Snapshot { return Snapshot{c.W, c.H, c.ClonePixels()} }
func (u *UndoManager) Push(c *Canvas) {
	u.undo = append(u.undo, u.snapshot(c))
	u.redo = nil
	if len(u.undo) > u.limit {
		u.undo = append([]Snapshot(nil), u.undo[len(u.undo)-u.limit:]...)
	}
	// 20 full-size canvases can consume gigabytes. Respect a separate
	// 256 MiB undo-stack budget, preserving the latest undo checkpoint.
	sizes := make([]int, len(u.undo))
	for i := range u.undo {
		sizes[i] = len(u.undo[i].Pix)
	}
	if drop := undoOldestDropCount(sizes, undoSnapshotByteBudget); drop > 0 {
		u.undo = append([]Snapshot(nil), u.undo[drop:]...)
	}
}
func (u *UndoManager) Undo(c *Canvas) bool {
	if len(u.undo) == 0 {
		return false
	}
	u.redo = append(u.redo, u.snapshot(c))
	s := u.undo[len(u.undo)-1]
	u.undo = u.undo[:len(u.undo)-1]
	c.W, c.H, c.Pix = s.W, s.H, s.Pix
	return true
}
func (u *UndoManager) Redo(c *Canvas) bool {
	if len(u.redo) == 0 {
		return false
	}
	u.undo = append(u.undo, u.snapshot(c))
	s := u.redo[len(u.redo)-1]
	u.redo = u.redo[:len(u.redo)-1]
	c.W, c.H, c.Pix = s.W, s.H, s.Pix
	return true
}
func (u *UndoManager) Reset() { u.undo = nil; u.redo = nil }
func (u *UndoManager) DiscardLast() {
	if len(u.undo) > 0 {
		n := len(u.undo) - 1
		u.undo[n] = Snapshot{} // release the abandoned bitmap immediately
		u.undo = u.undo[:n]
	}
}

// HistoryEntry is a named post-operation canvas state shown in the Photoshop-
// style History panel. Unlike the low-level UndoManager (which stores temporary
// pre-operation states for Escape/cancel), these snapshots are user-visible
// committed states and can be selected directly.
type HistoryEntry struct {
	Name                      string
	At                        time.Time
	W, H                      int
	Pix                       []byte
	IndexedMode               bool
	IndexedSize, IndexedCount int
	IndexedPalette            []uint32
	IndexedPixels             []byte
	RGBPalette                []uint32 // classic 28-colour slots, even in indexed mode
	FG, BG                    uint32
	CustomColors              [16]uint32
}

const (
	historyLimit     = 40
	historyByteLimit = 256 * 1024 * 1024
)

// --- Tools -----------------------------------------------------------------
type Tool int

const (
	ToolFreeSelect Tool = iota
	ToolRectSelect
	ToolColorRegionSelect
	ToolColorAllSelect
	ToolEraser
	ToolFill
	ToolEyedropper
	ToolMagnifier
	ToolPencil
	ToolBrush
	ToolAirbrush
	ToolText
	ToolLine
	ToolCurve
	ToolRectangle
	ToolEllipse
	ToolRightTriangle
	ToolArrow
)

var toolNames = []string{
	"자유 선택", "사각 선택", "색 영역 선택", "동일 색 전체 선택", "지우개", "색 채우기", "색 골라내기", "확대/축소", "붓", "붓(통합됨)", "에어브러시(통합됨)", "텍스트", "직선", "곡선", "사각형", "타원", "직각삼각형", "화살표",
}

// Pencil, brush, and airbrush are presented as one unified brush tool.  The
// legacy ToolBrush and ToolAirbrush enum values remain only for source and log
// compatibility; they are no longer shown as separate toolbox buttons.
var visibleTools = []Tool{
	ToolFreeSelect, ToolRectSelect,
	ToolColorRegionSelect, ToolColorAllSelect,
	ToolEraser, ToolFill,
	ToolEyedropper, ToolMagnifier,
	ToolPencil, ToolText,
	ToolLine, ToolCurve,
	ToolRectangle, ToolEllipse,
	ToolRightTriangle, ToolArrow,
}

func isColorSelectionTool(t Tool) bool {
	return t == ToolColorRegionSelect || t == ToolColorAllSelect
}

func isSelectionTool(t Tool) bool {
	return t == ToolFreeSelect || t == ToolRectSelect || isColorSelectionTool(t)
}

type BrushShape int

const (
	BrushSquare BrushShape = iota
	BrushCircle
	BrushSlashUp
	BrushSlashDown
)

var brushShapeNames = []string{"네모", "동그라미", "우상향", "좌하향"}

type LineConstraintMode int

const (
	LineConstraint15Degrees LineConstraintMode = iota
	LineConstraintTanN
)

var lineConstraintNames = []string{"15×n˚", "tan n"}

type LineTanRatio struct {
	Rise int
	Run  int
}

// Each tan-n direction is stored as an exact reduced integer ratio. The
// rasterizer follows each ratio as a repeating one-pixel staircase rather
// than distributing rounding error over the whole line. Thus tan 1/3 keeps
// exactly three horizontal pixels per vertical step while partial prefixes can
// still appear naturally at the live endpoint.
var lineTanRatios = []LineTanRatio{
	{Rise: 1, Run: 5},
	{Rise: 1, Run: 4},
	{Rise: 1, Run: 3},
	{Rise: 1, Run: 2},
	{Rise: 2, Run: 3},
	{Rise: 1, Run: 1},
	{Rise: 3, Run: 2},
	{Rise: 2, Run: 1},
	{Rise: 3, Run: 1},
	{Rise: 4, Run: 1},
	{Rise: 5, Run: 1},
}

var fallbackTextFontChoices = []string{"함초롬바탕", "함초롬돋움", "맑은 고딕", "굴림", "궁서", "Arial", "Times New Roman"}
var textSizeChoices = []int{5, 6, 7, 8, 9, 10, 11, 12, 14, 16, 18, 20, 24, 28, 36, 48, 72}
var textScriptChoices = []string{"없음", "한글", "숫자·기호", "영문"}

type PixelPoint struct{ X, Y int }

type ClipboardImage struct {
	W, H int
	Pix  []byte
	Mask []bool
}

// ClipboardEntry is one item in dotPaint's six-slot clipboard history.
// Pinned entries are never evicted by newer clipboard images.
type ClipboardEntry struct {
	Image  ClipboardImage
	Hash   uint32
	Pinned bool
	Source string
	Added  time.Time
}

type clipboardPinMeta struct {
	File   string    `json:"file"`
	Source string    `json:"source"`
	Added  time.Time `json:"added"`
}

type SelectionState struct {
	Active bool
	// Preserve all solid palette colours, even the current background colour;
	// unlike the XP transparent-colour toggle, actual alpha=0 pixels still skip.
	ForceOpaque bool
	// IndexedPalettePaste keeps the original clipboard alpha until the floating
	// selection is composited at its CURRENT position, then quantizes the result
	// against the fixed IMG palette. Reapplying after a drag uses BasePix again.
	IndexedPalettePaste bool
	Free                bool
	X, Y, W, H          int
	Mask                []bool

	Creating                 bool
	Moving                   bool
	Floating                 bool
	StartX, StartY           int
	CurrentX, CurrentY       int
	MoveOffsetX, MoveOffsetY int
	// MoveStart is the floating selection top-left when a drag begins.
	// MovePointer stores the latest native-canvas pointer so pressing or
	// releasing Shift without moving the mouse can immediately reapply the
	// constrained/free position.
	MoveStartX, MoveStartY     int
	MovePointerX, MovePointerY int
	Lasso                      []PixelPoint

	// Shift-additive selection keeps the previous region here while the new
	// rectangle or lasso is being drawn. The two regions are merged on mouse-up.
	Adding                                 bool
	AddBaseX, AddBaseY, AddBaseW, AddBaseH int
	AddBaseMask                            []bool
	AddBaseFree                            bool

	Pix     []byte
	BasePix []byte
}

func (s *SelectionState) Reset() { *s = SelectionState{} }
func (s *SelectionState) Contains(x, y int) bool {
	if !s.Active || x < s.X || y < s.Y || x >= s.X+s.W || y >= s.Y+s.H {
		return false
	}
	if len(s.Mask) == s.W*s.H {
		return s.Mask[(y-s.Y)*s.W+(x-s.X)]
	}
	return true
}

func pointInPolygon(x, y float64, pts []PixelPoint) bool {
	inside := false
	j := len(pts) - 1
	for i := 0; i < len(pts); i++ {
		xi, yi := float64(pts[i].X), float64(pts[i].Y)
		xj, yj := float64(pts[j].X), float64(pts[j].Y)
		if ((yi > y) != (yj > y)) && (x < (xj-xi)*(y-yi)/(yj-yi)+xi) {
			inside = !inside
		}
		j = i
	}
	return inside
}

func regionContains(rx, ry, rw, rh int, mask []bool, x, y int) bool {
	if rw < 1 || rh < 1 || x < rx || y < ry || x >= rx+rw || y >= ry+rh {
		return false
	}
	if len(mask) != rw*rh {
		return true
	}
	return mask[(y-ry)*rw+(x-rx)]
}

// unionSelectionRegions merges two arbitrary selection masks into one bounding
// region. A nil result mask means the union fills the entire returned rectangle.
func unionSelectionRegions(ax, ay, aw, ah int, amask []bool, bx, by, bw, bh int, bmask []bool) (x, y, w, h int, mask []bool, count int) {
	if aw < 1 || ah < 1 {
		if bw < 1 || bh < 1 {
			return 0, 0, 0, 0, nil, 0
		}
		if len(bmask) == bw*bh {
			mask = append([]bool(nil), bmask...)
			for _, v := range mask {
				if v {
					count++
				}
			}
		} else {
			count = bw * bh
		}
		return bx, by, bw, bh, mask, count
	}
	if bw < 1 || bh < 1 {
		if len(amask) == aw*ah {
			mask = append([]bool(nil), amask...)
			for _, v := range mask {
				if v {
					count++
				}
			}
		} else {
			count = aw * ah
		}
		return ax, ay, aw, ah, mask, count
	}
	x = min(ax, bx)
	y = min(ay, by)
	right := max(ax+aw, bx+bw)
	bottom := max(ay+ah, by+bh)
	w, h = right-x, bottom-y
	mask = make([]bool, w*h)
	for py := y; py < bottom; py++ {
		for px := x; px < right; px++ {
			if regionContains(ax, ay, aw, ah, amask, px, py) || regionContains(bx, by, bw, bh, bmask, px, py) {
				mask[(py-y)*w+(px-x)] = true
				count++
			}
		}
	}
	if count == w*h {
		mask = nil
	}
	return
}

func (a *App) buildColorSelection(seedX, seedY int, selectAll bool) (x, y, w, h int, mask []bool, count int) {
	if a.canvas == nil {
		return
	}
	return buildColorSelectionMask(a.canvas.Pix, a.canvas.W, a.canvas.H, seedX, seedY, a.colorTolerance, selectAll)
}

func (a *App) selectByColor(seedX, seedY int, selectAll, additive bool) {
	var baseX, baseY, baseW, baseH int
	var baseMask []bool
	baseFree := false
	if additive && a.selection.Active {
		baseX, baseY, baseW, baseH = a.selection.X, a.selection.Y, a.selection.W, a.selection.H
		baseMask = append([]bool(nil), a.selection.Mask...)
		baseFree = a.selection.Free
	}

	x, y, w, h, mask, count := a.buildColorSelection(seedX, seedY, selectAll)
	if count == 0 {
		a.statusText = "선택할 수 있는 색 영역을 찾지 못했습니다."
		logf("COLOR_SELECT empty all=%v seed=%d,%d tolerance=%d", selectAll, seedX, seedY, a.colorTolerance)
		return
	}

	if additive && baseW > 0 && baseH > 0 {
		x, y, w, h, mask, count = unionSelectionRegions(baseX, baseY, baseW, baseH, baseMask, x, y, w, h, mask)
	}
	a.selection.Reset()
	a.selection.Active = count > 0
	a.selection.Free = len(mask) == w*h || baseFree
	a.selection.X, a.selection.Y, a.selection.W, a.selection.H = x, y, w, h
	a.selection.Mask = mask

	mode := "색 영역"
	if selectAll {
		mode = "동일 색 전체"
	}
	addition := ""
	if additive && baseW > 0 && baseH > 0 {
		addition = " + 기존 선택"
	}
	a.statusText = fmt.Sprintf("%s 선택%s: %d픽셀, 허용 불일치 %d.", mode, addition, count, a.colorTolerance)
	logf("COLOR_SELECT apply all=%v additive=%v seed=%d,%d tolerance=%d x=%d y=%d w=%d h=%d pixels=%d masked=%v", selectAll, additive, seedX, seedY, a.colorTolerance, x, y, w, h, count, len(mask) == w*h)
	a.requestCanvasRepaint(true)
}

const (
	rightPanelWidth          = 240
	panelDockSnapDistance    = 7 // both edge and corner/slot stages
	panelDockTopOffset       = 2
	panelDockGap             = 2
	panelHeaderHeight        = 16
	panelCloseSize           = 14
	paletteCellColumns       = 22
	paletteCellSize          = 24
	palettePanelHeight       = 72
	toolboxFloatingHeight    = 720
	clipboardHistoryCapacity = 10
	clipboardVisibleSlots    = 6
	clipboardColumns         = 2
	clipboardVisibleRows     = 3
)

type PanelState struct {
	X, Y        int
	W, H        int
	Docked      bool // compatibility mirror: DockStage > 0
	DockStage   int  // 0=free, 1=edge only, 2=edge+corner/slot
	DockOrder   int
	Dragging    bool
	DragOffsetX int
	DragOffsetY int
}

type TextCharStyle struct {
	FontName  string
	FontSize  int
	Bold      bool
	Italic    bool
	Underline bool
}

// TextBoundaryPos is the native-canvas position of one UTF-16 insertion
// boundary.  Cursor hit testing, selection painting, and the visible caret all
// use this same cache, so the highlighted range can never drift away from the
// range that receives formatting.
type TextBoundaryPos struct {
	Index int
	X     int
	Y     int
}

// TextGlyphCluster is one visual glyph cluster as rasterized by the native GDI
// text layout. Selection highlighting uses these exact advance rectangles rather
// than rebuilding rectangles one UTF-16 code unit at a time. This prevents the
// final selected Korean glyph, surrogate pair, combining sequence, or wrapped
// cluster from being omitted from the visible inversion overlay.
type TextGlyphCluster struct {
	Start int
	End   int
	X     int
	Y     int
	W     int
	Break bool
}

func (s TextCharStyle) Equal(other TextCharStyle) bool {
	return s.FontName == other.FontName && s.FontSize == other.FontSize && s.Bold == other.Bold && s.Italic == other.Italic && s.Underline == other.Underline
}

type TextState struct {
	Active            bool
	Editing           bool
	Creating          bool
	Opaque            bool
	X, Y, W, H        int
	Text              string
	FontName          string
	FontNames         []string
	FontSize          int
	Script            string
	Bold              bool
	Italic            bool
	Underline         bool
	EditHwnd          uintptr
	FontCombo         uintptr
	SizeCombo         uintptr
	ScriptCombo       uintptr
	BoldBtn           uintptr
	ItalicBtn         uintptr
	UnderlineBtn      uintptr
	UIFont            uintptr
	LayoutHwnd        uintptr
	LayoutFont        uintptr
	LayoutText        string
	LayoutW           int
	LayoutH           int
	LayoutRich        bool
	LayoutDirty       bool
	LayoutColor       uint32
	LayoutDPI         int
	LayoutControlDPI  int
	LayoutZoomNum     int
	LayoutZoomDen     int
	LayoutUnits       int
	LayoutInputToRich []int
	LayoutRichToInput []int
	LayoutPositions   []TextBoundaryPos
	LayoutLineHeights map[int]int
	LayoutClusters    []TextGlyphCluster
	Styles            []TextCharStyle
	// InsertionStyle is the PowerPoint-like style attached to a collapsed
	// caret.  Choosing a font/size/B/I/U with no selected text arms this style
	// at InsertionCaret. Merely returning focus to the same caret must not let
	// the neighbouring character overwrite it. Moving the caret to a genuinely
	// different insertion boundary adopts the surrounding text style instead.
	InsertionStyle       TextCharStyle
	InsertionStyleActive bool
	InsertionCaret       int
	// InsertionAnchor and InsertionSpanEnd delimit the contiguous text owned by
	// the current collapsed-caret insertion style. Korean IME composition can
	// repeatedly delete and recreate identical syllables. A plain longest-
	// prefix diff then mistakes the recreated syllable for the bold suffix on
	// the right and moves the new style behind it. Keeping this anchored span
	// makes the pre-existing suffix immutable throughout one composition run.
	InsertionAnchor  int
	InsertionSpanEnd int

	// A180: Korean IME edits are a transaction, not a stream of ordinary
	// insert/delete operations. Windows temporarily removes and recreates the
	// composing syllable, which used to make the custom caret jump backward and
	// could let the insertion style be recalculated from a transient state.
	IMEActive              bool
	IMEBaseText            string
	IMEBaseStyles          []TextCharStyle
	IMEBaseStart           int
	IMEBaseEnd             int
	IMEStyle               TextCharStyle
	IMEInsertionWasActive  bool
	IMEOwnedAnchor         int
	IMEOwnedSpanEnd        int
	IMEVisualCaret         int
	IMEVisualCaretValid    bool
	IMEPendingShrink       bool
	IMEPendingCandidate    int
	IMEPendingChangeUnixMS int64

	ControlSync       bool
	LastSizeListIndex int
	LastSizeApplyMS   int64
	Selecting         bool
	SelectAnchor      int
	VerticalGoalX     int
	VerticalGoalValid bool
	CaretVisible      bool
	CaretTimer        bool
	DragMode          int
	DragHandle        int
	DragStartMX       int
	DragStartMY       int
	OrigX             int
	OrigY             int
	OrigW             int
	OrigH             int
}

// --- Main application ------------------------------------------------------
type App struct {
	hwnd      uintptr
	hinst     uintptr
	menu      uintptr
	viewMenu  uintptr
	imageMenu uintptr
	fileMenu  uintptr
	menuBrush uintptr
	darkBrush uintptr
	// The dock headers are only 16 physical pixels high. Keep their captions
	// independent from the Windows 11 system text-size setting.
	dockPanelTitleFont uintptr

	// Ctrl+N keeps its traditional New command, but can also act as the
	// prefix of the requested Ctrl+N+M / Ctrl+N+G insertion chords.  The New
	// command is committed on N key-up unless M or G consumes the chord first.
	insertChordPending bool

	canvas *Canvas
	undo   *UndoManager

	// DejaVu IMG mode keeps the original 294-byte tile records beside the
	// ordinary RGBA canvas. The canvas remains the same editor surface used by
	// every existing Paint tool; save-time conversion writes only documented
	// pixel payload bytes back to the indexed tile records.
	dejavu            *dejavuIMGDocument
	dejavuPrevPalette []uint32
	dejavuPrevFG      uint32
	dejavuPrevBG      uint32
	dejavuMenu        uintptr

	zoom             int
	zoomLevels       []int
	scrollX, scrollY int

	// Magnifier target-selection preview. While the magnifier tool is active,
	// a dashed rectangle shows the exact source area that will fill the fixed
	// canvas viewport at zoomTarget. A left click applies the zoom to that area.
	zoomTarget       int
	zoomPreviewValid bool
	zoomPreviewX     int
	zoomPreviewY     int
	zoomPreviewW     int
	zoomPreviewH     int

	showToolbox   bool
	showPalette   bool
	showStatus    bool
	showClipboard bool
	showHistory   bool
	showGrid      bool
	showThumbnail bool
	bitmapView    bool
	opaque        bool
	darkMode      bool

	tool         Tool
	fg, bg       uint32
	palette      []uint32
	customColors [16]uint32
	// An RGB value is not a slot identity: preserve which duplicate-colour slot
	// was clicked for each foreground/background edit target (-1 = unbound).
	fgPaletteSlot, bgPaletteSlot                           int
	indexedMode                                            bool // standard documents may opt in; IMG documents always indexed
	indexedColors                                          indexed16Palette
	indexedColors256                                       []uint32
	indexedSize                                            int
	indexedActiveCount                                     int    // number of usable indices, independent of 16/256 table capacity
	indexedPixelIndices                                    []byte // best-effort stable per-pixel index identity
	indexedPalettePage                                     int
	paletteDisplayRGB, paletteDisplay16, paletteDisplay256 int      // UI-only visible swatch counts; NOT quantization limits
	standardPalette                                        []uint32 // saved when enabling 16-colour indexed palette

	eraserSize          int
	adjustingEraserSize bool

	brushSize          int
	brushShape         BrushShape
	adjustingBrushSize bool

	lineSize           int
	lineConstraintMode LineConstraintMode
	lineAntialias      bool
	adjustingLineSize  bool

	curveBendCount      int
	curveSize           int
	curveAntialias      bool
	adjustingCurveBends bool
	adjustingCurveSize  bool

	rectangleSize              int
	adjustingRectangleSize     bool
	ellipseSize                int
	adjustingEllipseSize       bool
	rightTriangleSize          int
	adjustingRightTriangleSize bool

	arrowHeadSize          int
	arrowLineSize          int
	arrowAntialias         bool
	adjustingArrowLineSize bool
	shapeShowCenter        bool

	textTool TextState

	// A click outside an active text box is a commit-only gesture. The complete
	// down/move/up sequence is consumed so the same physical click can never
	// create the next text box. A later, separate click may create one normally.
	textCommitClickPending bool

	// Clicking or dragging inside the thumbnail navigator continuously centers
	// the main viewport on the corresponding canvas point. Mouse capture keeps
	// navigation smooth even when the pointer briefly leaves the thumbnail.
	thumbnailNavigationDragging bool

	eyedropperReturnTool Tool
	eyedropperActive     bool
	eyedropperArmed      bool

	// Magnifier is a one-shot helper like the eyedropper. After a zoom is
	// actually applied, return to the tool that was active before magnifier.
	magnifierReturnTool Tool

	drawing          bool
	drawButtonRight  bool
	startX, startY   int
	lastX, lastY     int
	curX, curY       int
	cursorX, cursorY int
	preview          bool

	// Eraser Shift strokes still rebuild the entire drag from the mouse-down
	// snapshot. Brush Shift is segment-based: pressing Shift during a freehand
	// stroke captures the already-painted canvas and starts one constrained
	// segment at the current brush point; releasing Shift commits that segment
	// and resumes freehand from its snapped endpoint.
	shiftStrokeActive   bool
	shiftStrokeBasePix  []byte
	shiftStrokeAnchorX  int
	shiftStrokeAnchorY  int
	shiftStrokePreviewX int
	shiftStrokePreviewY int

	// Only the initial straight baseline of the curve tool uses Shift snapping.
	// Bend-point drags remain completely free after the baseline is released.
	curveBaseShiftConstrained bool

	// Curve tool state. curveStage 0 draws the base line; stages 1..N adjust
	// exactly N user-selected bend points. A later bend point does not enter the
	// geometry until the previous point has been released.
	curveStage         int // 0=base line, 1..curveBendCount=bend adjustments
	curveX0, curveY0   int
	curveX1, curveY1   int
	curveC1X, curveC1Y int
	curveC2X, curveC2Y int
	curveC3X, curveC3Y int
	curveC4X, curveC4Y int
	curveUndoPending   bool

	selection SelectionState

	// A non-additive rectangular-selection press outside the current marquee is
	// armed first. Releasing without a deliberate drag only cancels the old
	// selection; dragging at least five client pixels starts a new rectangle.
	// The same pending state lets a drag begin in the grey workspace, with its
	// image-space anchor clamped to the nearest outermost canvas pixel.
	pendingRectSelection              bool
	pendingRectSelectionStartClientX  int
	pendingRectSelectionStartClientY  int
	pendingRectSelectionStartImageX   int
	pendingRectSelectionStartImageY   int
	pendingRectSelectionOutsideCanvas bool

	// Color selections share one visible `허용 불일치` value from 0..100.
	// Matching uses Euclidean distance in 8-bit RGB space; the implementation
	// compares squared distances, so it avoids a square root for every pixel.
	colorTolerance          int
	adjustingColorTolerance bool

	clipboard ClipboardImage // compatibility mirror of the selected/latest history image

	clipboardHistory          []ClipboardEntry
	clipboardSelected         int
	clipboardScroll           int
	clipboardScrollDragging   bool
	clipboardScrollDragOffset int
	clipboardListening        bool
	lastClipboardSeq          uint32
	clipboardPanel            PanelState
	textPanel                 PanelState
	historyPanel              PanelState
	thumbnailPanel            PanelState
	toolboxPanel              PanelState
	palettePanel              PanelState
	panelDockSerial           int
	showTextPanel             bool

	historyEntries          []HistoryEntry
	historyIndex            int
	historyScroll           int
	historyScrollDragging   bool
	historyScrollDragOffset int

	// Canvas handles are armed first and only become an active resize after a
	// short drag. This prevents an edge click from capturing all mouse input and
	// making the application appear disabled, especially when a large canvas
	// fills the viewport.
	pendingCanvasResize                  bool
	pendingResizeHandle                  int
	pendingResizeStartMouseX             int
	pendingResizeStartMouseY             int
	resizingCanvas                       bool
	resizeHandle                         int
	resizeStartW, resizeStartH           int
	resizeStartMouseX, resizeStartMouseY int
	resizePreviewW, resizePreviewH       int
	resizeLeftDelta, resizeTopDelta      int

	dirty    bool
	filePath string
	recent   []string

	statusText string
	rng        *rand.Rand

	// Persistent back buffer avoids creating and deleting large GDI bitmaps on
	// every paint message. This is both smoother and easier to diagnose.
	backDC        uintptr
	backBitmap    uintptr
	backOldBitmap uintptr
	backW, backH  int

	paintCount         uint64
	drawMoveCount      uint64
	lastImmediateUI    time.Time
	lastDrawLog        time.Time
	lastCanvasBlitW    int
	lastCanvasBlitH    int
	canvasBlitFails    uint64
	vectorPreviewCount uint64

	// A217 chooses a light or dark version of the custom canvas cursor from
	// the luminance of the image pixel under the cursor hot spot. These fields
	// suppress redundant SetCursor/log work while the pointer stays on the same
	// contrast side of the 128 threshold.
	adaptiveCursorBase    uintptr
	adaptiveCursorVariant int
	adaptiveCursorLastLog time.Time

	// A166: a persistent top-down 32-bit DIB section is used as a bounded
	// source tile for the canvas.  Canvas pixels are copied into this stable
	// GDI-owned buffer before BitBlt/StretchBlt, so no interior pointer into a
	// large Go slice is passed to StretchDIBits.  The A165 log showed one
	// repeatable missing scan line at source y=571; this tile buffer removes the
	// failing transfer path entirely.
	canvasTileDC        uintptr
	canvasTileBitmap    uintptr
	canvasTileOldBitmap uintptr
	canvasTileBits      unsafe.Pointer
	canvasTileW         int
	canvasTileH         int
}

var app *App
var wndProcCallback uintptr
var fontEnumCallback uintptr
var fontEnumCollector map[string]string
var clipboardPNGFormat uintptr
var clipboardImagePNGFormat uintptr

// Dark menus use owner-drawn item data keys rather than pointers into Go memory.
type menuDrawEntry struct {
	Text     string
	TopLevel bool
	Popup    bool
}

var menuDrawMu sync.Mutex
var menuDrawNext uintptr = 1
var menuDrawEntries = map[uintptr]menuDrawEntry{}
var menuDrawKeysByMenu = map[uintptr][]uintptr{}

func newApp() *App {
	a := &App{
		canvas:      NewCanvas(640, 480, rgb(255, 255, 255)),
		undo:        NewUndoManager(20),
		zoom:        initialCanvasZoom,
		zoomLevels:  []int{1, 2, 4, 6, 8, 12, 15},
		zoomTarget:  8,
		showToolbox: true, showPalette: true, showStatus: true, showClipboard: true, showHistory: true, opaque: false,
		tool:                 ToolPencil,
		eyedropperReturnTool: ToolPencil,
		magnifierReturnTool:  ToolPencil,
		eraserSize:           8,
		brushSize:            1,
		brushShape:           BrushSquare,
		lineSize:             2,
		lineConstraintMode:   LineConstraintTanN,
		lineAntialias:        false,
		curveBendCount:       2,
		curveSize:            2,
		curveAntialias:       false,
		rectangleSize:        2,
		ellipseSize:          2,
		rightTriangleSize:    2,
		arrowHeadSize:        0,
		arrowLineSize:        2,
		arrowAntialias:       false,
		shapeShowCenter:      false,
		colorTolerance:       0,
		textTool:             TextState{FontName: defaultTextFontName(), FontSize: 20, Script: defaultTextScript(), Opaque: false},
		clipboardPanel:       PanelState{W: rightPanelWidth, H: 244, Docked: true, DockStage: 2, DockOrder: 1},
		historyPanel:         PanelState{W: rightPanelWidth, H: 308, Docked: true, DockStage: 2, DockOrder: 2},
		textPanel:            PanelState{W: rightPanelWidth, H: 126, Docked: true, DockStage: 2, DockOrder: 3},
		thumbnailPanel:       PanelState{W: rightPanelWidth, H: 190, Docked: true, DockStage: 2, DockOrder: 4},
		toolboxPanel:         PanelState{W: toolboxWidth, H: toolboxFloatingHeight, Docked: true, DockStage: 2},
		palettePanel:         PanelState{W: palettePanelWidth(), H: palettePanelHeight, Docked: true, DockStage: 2},
		paletteDisplayRGB:    44, paletteDisplay16: 16, paletteDisplay256: 256,
		panelDockSerial: 4,
		showTextPanel:   false,
		fg:              rgb(0, 0, 0), bg: rgb(255, 255, 255),
		fgPaletteSlot: 0, bgPaletteSlot: 14,
		statusText: "도움말이 필요하면 도움말 메뉴의 도움말 항목을 클릭하십시오.",
		cursorX:    -1, cursorY: -1,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	a.palette = []uint32{
		rgb(0, 0, 0), rgb(128, 128, 128), rgb(128, 0, 0), rgb(128, 128, 0), rgb(0, 128, 0), rgb(0, 128, 128), rgb(0, 0, 128), rgb(128, 0, 128), rgb(128, 128, 64), rgb(0, 64, 64), rgb(0, 128, 255), rgb(0, 64, 128), rgb(64, 0, 255), rgb(128, 64, 0),
		rgb(255, 255, 255), rgb(192, 192, 192), rgb(255, 0, 0), rgb(255, 255, 0), rgb(0, 255, 0), rgb(0, 255, 255), rgb(0, 0, 255), rgb(255, 0, 255), rgb(255, 255, 192), rgb(128, 255, 128), rgb(128, 255, 255), rgb(128, 128, 255), rgb(255, 0, 128), rgb(255, 128, 64),
	}
	a.indexedColors = indexedPaletteFromRefs(a.palette)
	for i := range a.customColors {
		a.customColors[i] = rgb(255, 255, 255)
	}
	a.resetHistory("시작")
	return a
}

func historySnapshot(c *Canvas, name string) HistoryEntry {
	return HistoryEntry{Name: name, At: time.Now(), W: c.W, H: c.H, Pix: c.ClonePixels()}
}

func historyMatchesCanvas(e HistoryEntry, c *Canvas) bool {
	return e.W == c.W && e.H == c.H && bytes.Equal(e.Pix, c.Pix)
}

func (a *App) makeHistorySnapshot(name string) HistoryEntry {
	e := historySnapshot(a.canvas, name)
	e.FG, e.BG, e.CustomColors = a.fg, a.bg, a.customColors
	if len(a.standardPalette) == 28 {
		e.RGBPalette = append([]uint32(nil), a.standardPalette...)
	} else if len(a.palette) == 28 {
		e.RGBPalette = append([]uint32(nil), a.palette...)
	}
	if a.dejavu == nil && a.indexedMode && validIndexedSize(len(a.palette)) {
		e.IndexedMode, e.IndexedSize = true, a.indexedSize
		e.IndexedCount = indexedUsableCount(len(a.palette), a.indexedActiveCount)
		e.IndexedPalette = append([]uint32(nil), a.palette...)
		e.IndexedPixels = indexedIndicesForSnapshot(e.Pix, e.IndexedPalette, e.IndexedCount, a.indexedPixelIndices)
		a.indexedPixelIndices = append([]byte(nil), e.IndexedPixels...)
	}
	return e
}
func (a *App) historyMatchesApp(e HistoryEntry) bool {
	if !historyMatchesCanvas(e, a.canvas) {
		return false
	}
	if a.dejavu != nil {
		return true
	}
	if e.IndexedMode != a.indexedMode {
		return false
	}
	if !e.IndexedMode {
		return true
	}
	if e.IndexedSize != a.indexedSize || e.IndexedCount != indexedUsableCount(len(a.palette), a.indexedActiveCount) || len(e.IndexedPalette) != len(a.palette) {
		return false
	}
	for i, c := range e.IndexedPalette {
		if c != a.palette[i] {
			return false
		}
	}
	return true
}

func (a *App) resetHistory(name string) {
	if strings.TrimSpace(name) == "" {
		name = "시작"
	}
	a.historyEntries = []HistoryEntry{a.makeHistorySnapshot(name)}
	a.historyIndex = 0
	a.historyScroll = 0
	logf("HISTORY reset name=%q canvas=%dx%d", name, a.canvas.W, a.canvas.H)
}

func (a *App) historyVisibleRows() int {
	rows := (a.historyPanel.H - panelHeaderHeight - 4) / 24
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (a *App) ensureHistoryIndexVisible() {
	rows := a.historyVisibleRows()
	maxScroll := max(len(a.historyEntries)-rows, 0)
	if a.historyIndex < a.historyScroll {
		a.historyScroll = a.historyIndex
	}
	if a.historyIndex >= a.historyScroll+rows {
		a.historyScroll = a.historyIndex - rows + 1
	}
	a.historyScroll = clamp(a.historyScroll, 0, maxScroll)
}

func (a *App) recordHistory(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "작업"
	}
	if len(a.historyEntries) == 0 {
		a.resetHistory("시작")
	}
	if a.historyIndex >= 0 && a.historyIndex < len(a.historyEntries) && a.historyMatchesApp(a.historyEntries[a.historyIndex]) {
		// No pixel or size change. Keep the existing state instead of creating a
		// duplicate row from a click-only or canceled operation.
		return
	}
	if a.historyIndex+1 < len(a.historyEntries) {
		a.historyEntries = a.historyEntries[:a.historyIndex+1]
	}
	a.historyEntries = append(a.historyEntries, a.makeHistorySnapshot(name))
	for len(a.historyEntries) > historyLimit {
		a.historyEntries = a.historyEntries[1:]
	}
	for len(a.historyEntries) > 2 {
		total := 0
		for _, e := range a.historyEntries {
			total += len(e.Pix) + len(e.IndexedPixels) + len(e.IndexedPalette)*4 + len(e.RGBPalette)*4
		}
		if total <= historyByteLimit {
			break
		}
		a.historyEntries = a.historyEntries[1:]
	}
	a.historyEntries = append([]HistoryEntry(nil), a.historyEntries...)
	a.historyIndex = len(a.historyEntries) - 1
	a.ensureHistoryIndexVisible()
	logf("HISTORY add index=%d count=%d name=%q canvas=%dx%d", a.historyIndex, len(a.historyEntries), name, a.canvas.W, a.canvas.H)
	invalidateDejavuTileBrowser(a)
	// The canvas repaint requested by a finished drawing does not cover an
	// overlaid history panel on the far right. Repaint the panel explicitly so
	// the new row appears on mouse-up, without waiting for a tool switch or any
	// unrelated full-window invalidation.
	a.requestHistoryRepaint(true)
}

func (a *App) gotoHistory(index int) bool {
	if index < 0 || index >= len(a.historyEntries) || index == a.historyIndex {
		return false
	}
	e := a.historyEntries[index]
	a.clearSelectionState()
	a.canvas.W, a.canvas.H = e.W, e.H
	a.canvas.Pix = append([]byte(nil), e.Pix...)
	if a.dejavu == nil {
		a.fg, a.bg, a.customColors = e.FG, e.BG, e.CustomColors
		a.standardPalette = append([]uint32(nil), e.RGBPalette...)
		a.indexedPixelIndices = append([]byte(nil), e.IndexedPixels...)
		a.indexedMode = e.IndexedMode
		a.indexedSize = e.IndexedSize
		a.indexedActiveCount = e.IndexedCount
		if e.IndexedMode && validIndexedSize(len(e.IndexedPalette)) {
			a.palette = append([]uint32(nil), e.IndexedPalette...)
			if len(e.IndexedPixels) == e.W*e.H {
				restoreIndexedSnapshot(a.canvas.Pix, e.IndexedPixels, a.palette)
			}
			if e.IndexedSize == 16 {
				a.indexedColors = indexedPaletteFromRefs(a.palette)
			}
			if e.IndexedSize == 256 {
				a.indexedColors256 = append([]uint32(nil), a.palette...)
			}
			if a.indexedPalettePage*16 >= a.indexedActiveCount {
				a.indexedPalettePage = 0
			}
		} else {
			if len(e.RGBPalette) == 28 {
				a.palette = append([]uint32(nil), e.RGBPalette...)
			} else {
				a.palette = defaultMainRGBPalette()
			}
			a.indexedPalettePage = 0
			a.standardPalette = nil
		}
	}
	a.historyIndex = index
	a.ensureHistoryIndexVisible()
	// The low-level undo stack may contain temporary pre-operation snapshots
	// from a different branch. Reset it after direct history navigation.
	a.undo.Reset()
	a.dirty = true
	a.updateTitle()
	a.updateScrollbars()
	if a.hwnd != 0 && a.dejavu == nil {
		a.rebuildMenuForTheme()
	}
	a.statusText = fmt.Sprintf("히스토리 %d/%d: %s", index+1, len(a.historyEntries), e.Name)
	logf("HISTORY goto index=%d count=%d name=%q", index, len(a.historyEntries), e.Name)
	invalidateDejavuTileBrowser(a)
	invalidate(a.hwnd)
	return true
}

func (a *App) historyUndo() bool { return a.gotoHistory(a.historyIndex - 1) }
func (a *App) historyRedo() bool { return a.gotoHistory(a.historyIndex + 1) }

func (a *App) drawHistoryName(tool Tool) string {
	switch tool {
	case ToolPencil, ToolBrush, ToolAirbrush:
		return "붓"
	case ToolEraser:
		return "지우개"
	case ToolLine:
		return "직선"
	case ToolCurve:
		return "곡선"
	case ToolRectangle:
		return "사각형"
	case ToolEllipse:
		return "타원"
	case ToolRightTriangle:
		return "직각삼각형"
	case ToolArrow:
		return "화살표"
	case ToolFill:
		return "색 채우기"
	case ToolText:
		return "텍스트"
	}
	return "그리기"
}

func fontEnumProc(logFontPtr, _ uintptr, _ uintptr, _ uintptr) uintptr {
	if logFontPtr == 0 || fontEnumCollector == nil {
		return 1
	}
	lf := (*LOGFONTW)(unsafe.Pointer(logFontPtr))
	name := strings.TrimSpace(syscall.UTF16ToString(lf.LfFaceName[:]))
	if name == "" || strings.HasPrefix(name, "@") {
		return 1
	}
	key := strings.ToLower(name)
	if _, exists := fontEnumCollector[key]; !exists {
		fontEnumCollector[key] = name
	}
	return 1
}

func enumerateInstalledFonts(hwnd uintptr) []string {
	hdc, _, _ := procGetDC.Call(hwnd)
	if hdc == 0 {
		return append([]string(nil), fallbackTextFontChoices...)
	}
	defer procReleaseDC.Call(hwnd, hdc)
	if fontEnumCallback == 0 {
		fontEnumCallback = syscall.NewCallback(fontEnumProc)
	}
	fontEnumCollector = make(map[string]string)
	lf := LOGFONTW{LfCharSet: DEFAULT_CHARSET}
	procEnumFontFamiliesExW.Call(hdc, uintptr(unsafe.Pointer(&lf)), fontEnumCallback, 0, 0)
	fonts := make([]string, 0, len(fontEnumCollector))
	for _, name := range fontEnumCollector {
		fonts = append(fonts, name)
	}
	sort.Slice(fonts, func(i, j int) bool {
		li, lj := strings.ToLower(fonts[i]), strings.ToLower(fonts[j])
		if li == lj {
			return fonts[i] < fonts[j]
		}
		return li < lj
	})
	fontEnumCollector = nil
	if len(fonts) == 0 {
		fonts = append(fonts, fallbackTextFontChoices...)
	}
	logf("TEXT fonts enumerated count=%d", len(fonts))
	return fonts
}

func comboItemText(hwnd uintptr, index int) string {
	if hwnd == 0 || index < 0 {
		return ""
	}
	n := int(sendMessage(hwnd, CB_GETLBTEXTLEN, uintptr(index), 0))
	if n < 0 || n > 4096 {
		return ""
	}
	buf := make([]uint16, n+2)
	sendMessage(hwnd, CB_GETLBTEXT, uintptr(index), uintptr(unsafe.Pointer(&buf[0])))
	return syscall.UTF16ToString(buf)
}

func (a *App) fontPreviewSample() string {
	switch a.textTool.Script {
	case "없음":
		return ""
	case "영문":
		return "AaBbCc"
	case "숫자·기호":
		return "123 !? ★"
	default:
		return "가나다 한글"
	}
}

func (a *App) updateFontPreviewDropdownWidth() {
	if a.textTool.FontCombo == 0 {
		return
	}
	width := 360
	if a.textTool.Script == "없음" {
		// With preview disabled the popup should stay exactly inside the
		// 240-pixel text panel instead of opening a wide sample column.
		width = 182
	}
	sendMessage(a.textTool.FontCombo, CB_SETDROPPEDWIDTH, uintptr(width), 0)
}

func drawTextUsingSelectedFont(hdc uintptr, text string, rc RECT, format uint32, color uint32) {
	procSetTextColor.Call(hdc, uintptr(color))
	procSetBkMode.Call(hdc, TRANSPARENT)
	p := utf16Ptr(text)
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&rc)), uintptr(format))
}

func (a *App) drawFontComboItem(dis *DRAWITEMSTRUCT) bool {
	if dis == nil || dis.CtlType != ODT_COMBOBOX || int(dis.CtlID) != ID_TEXT_FONTCOMBO || dis.HDC == 0 {
		return false
	}
	idx := int(int32(dis.ItemID))
	name := a.textTool.FontName
	if idx >= 0 {
		if s := comboItemText(dis.HwndItem, idx); s != "" {
			name = s
		}
	}
	savedDC, _, _ := procSaveDC.Call(dis.HDC)
	procIntersectClipRect.Call(dis.HDC, uintptr(dis.RcItem.Left), uintptr(dis.RcItem.Top), uintptr(dis.RcItem.Right), uintptr(dis.RcItem.Bottom))
	defer procRestoreDC.Call(dis.HDC, savedDC)

	selected := dis.ItemState&ODS_SELECTED != 0
	bgIndex, fgIndex := COLOR_WINDOW, COLOR_WINDOWTEXT
	if selected {
		bgIndex, fgIndex = COLOR_HIGHLIGHT, COLOR_HIGHLIGHTTEXT
	}
	bg, _, _ := procGetSysColor.Call(uintptr(bgIndex))
	fg, _, _ := procGetSysColor.Call(uintptr(fgIndex))
	if a.darkMode {
		bg = uintptr(rgb(30, 30, 30))
		fg = uintptr(rgb(235, 235, 235))
		if selected {
			bg = uintptr(rgb(64, 64, 64))
		}
	}
	fillRect(dis.HDC, dis.RcItem, uint32(bg))
	procSetBkMode.Call(dis.HDC, TRANSPARENT)
	procSetTextColor.Call(dis.HDC, fg)
	itemFont := createGDIFont(name, 18, false, false, false)
	var oldFont uintptr
	if itemFont != 0 {
		oldFont, _, _ = procSelectObject.Call(dis.HDC, itemFont)
	}
	rc := dis.RcItem
	rc.Left += 7
	rc.Right -= 5
	width := int(rc.Right - rc.Left)
	if a.textTool.Script != "없음" && width > 250 && idx >= 0 {
		nameRect := rc
		nameRect.Right = nameRect.Left + 165
		drawTextUsingSelectedFont(dis.HDC, name, nameRect, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS|DT_NOPREFIX, uint32(fg))
		dividerX := nameRect.Right + 3
		pen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(185, 185, 185))))
		oldPen, _, _ := procSelectObject.Call(dis.HDC, pen)
		procMoveToEx.Call(dis.HDC, uintptr(dividerX), uintptr(rc.Top+4), 0)
		procLineTo.Call(dis.HDC, uintptr(dividerX), uintptr(rc.Bottom-4))
		procSelectObject.Call(dis.HDC, oldPen)
		procDeleteObject.Call(pen)
		sampleRect := rc
		sampleRect.Left = dividerX + 9
		drawTextUsingSelectedFont(dis.HDC, a.fontPreviewSample(), sampleRect, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS|DT_NOPREFIX, uint32(fg))
	} else {
		drawTextUsingSelectedFont(dis.HDC, name, rc, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS|DT_NOPREFIX, uint32(fg))
	}
	if itemFont != 0 {
		procSelectObject.Call(dis.HDC, oldFont)
		procDeleteObject.Call(itemFont)
	}
	if dis.ItemState&ODS_FOCUS != 0 {
		fr := dis.RcItem
		fr.Left++
		fr.Top++
		fr.Right--
		fr.Bottom--
		procDrawFocusRect.Call(dis.HDC, uintptr(unsafe.Pointer(&fr)))
	}
	return true
}

func createGDIFont(face string, pixelHeight int, bold, italic, underline bool) uintptr {
	weight := FW_NORMAL
	if bold {
		weight = FW_BOLD
	}
	if pixelHeight < 1 {
		pixelHeight = 1
	}
	height := -pixelHeight
	italicFlag := uintptr(0)
	underlineFlag := uintptr(0)
	if italic {
		italicFlag = 1
	}
	if underline {
		underlineFlag = 1
	}
	h, _, _ := procCreateFontW.Call(uintptr(height), 0, 0, 0, uintptr(weight), italicFlag, underlineFlag, 0, DEFAULT_CHARSET, OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, DEFAULT_QUALITY, DEFAULT_PITCH|FF_DONTCARE, uintptr(unsafe.Pointer(utf16Ptr(face))))
	return h
}

// The formula, function/graph, multidimensional and field editors use dense
// layouts whose coordinates are deliberately expressed in physical pixels. A
// stock DEFAULT_GUI_FONT can follow the Windows text-size setting independently
// of those coordinates (most visibly on Windows 11), which makes labels
// overflow otherwise unchanged 24-28 px controls. Keep the UI font in the same
// pixel coordinate system as those layouts. Formula/canvas content fonts remain
// separate and are not affected by this helper.
const compactEditorUIFontPixelHeight = 12

// Custom dock headers are even denser than the editor controls.  Their title
// font is owned by App and reused across paints instead of allocating a GDI
// font for every panel on every WM_PAINT.
const dockPanelTitleFontPixelHeight = 12

func createCompactEditorUIFont() uintptr {
	if dotPaintEdition == "en" {
		return createGDIFont("Arial", 11, false, false, false)
	}
	return createGDIFont("맑은 고딕", compactEditorUIFontPixelHeight, false, false, false)
}

func setCompactEditorControlFont(hwnd, compactFont uintptr) {
	if hwnd == 0 {
		return
	}
	font := compactFont
	if font == 0 {
		font, _, _ = procGetStockObject.Call(DEFAULT_GUI_FONT)
	}
	if font != 0 {
		sendMessage(hwnd, WM_SETFONT, font, 1)
	}
}

func (a *App) ensureDockPanelTitleFont() uintptr {
	if a.dockPanelTitleFont == 0 {
		a.dockPanelTitleFont = createGDIFont("맑은 고딕", dockPanelTitleFontPixelHeight, false, false, false)
	}
	return a.dockPanelTitleFont
}

func (a *App) destroyDockPanelTitleFont() {
	if a.dockPanelTitleFont != 0 {
		procDeleteObject.Call(a.dockPanelTitleFont)
		a.dockPanelTitleFont = 0
	}
}

// Text sizes are presented as points.  The canvas itself has no physical DPI,
// so dotPaint uses the conventional 96-DPI mapping: 1 pt = 96/72 canvas pixel.
// This makes 20 pt and 48 pt genuinely different while keeping the result
// stable across Windows display scaling (96, 120, 144 DPI, and so on).
func textPointSizeToCanvasPixels(ptSize int) int {
	if ptSize < 1 {
		ptSize = 1
	}
	return max(1, int(math.Round(float64(ptSize)*96.0/72.0)))
}

func (a *App) destroyTextUIFont() {
	if a.textTool.UIFont != 0 {
		procDeleteObject.Call(a.textTool.UIFont)
		a.textTool.UIFont = 0
	}
	if a.textTool.LayoutFont != 0 {
		procDeleteObject.Call(a.textTool.LayoutFont)
		a.textTool.LayoutFont = 0
	}
}

func (a *App) textToolbarVisible() bool {
	return a.tool == ToolText && a.showTextPanel && !a.bitmapView
}

func (a *App) hideTextToolbarControls() {
	for _, h := range []uintptr{a.textTool.FontCombo, a.textTool.SizeCombo, a.textTool.ScriptCombo, a.textTool.BoldBtn, a.textTool.ItalicBtn, a.textTool.UnderlineBtn} {
		if h != 0 {
			procShowWindow.Call(h, SW_HIDE)
		}
	}
}

func (a *App) hideTextControls() {
	a.hideTextToolbarControls()
	if a.textTool.EditHwnd != 0 {
		procShowWindow.Call(a.textTool.EditHwnd, SW_HIDE)
	}
}

func (a *App) ensureTextControls() {
	if a.hwnd == 0 || a.textTool.EditHwnd != 0 {
		return
	}
	mk := func(class, title string, style uintptr, x, y, w, h, id int) uintptr {
		hwndCtl, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16Ptr(class))), uintptr(unsafe.Pointer(utf16Ptr(title))), style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), a.hwnd, uintptr(id), a.hinst, 0)
		return hwndCtl
	}
	a.textTool.EditHwnd = mk("EDIT", "", WS_CHILD|WS_TABSTOP|WS_VSCROLL|ES_MULTILINE|ES_AUTOVSCROLL|ES_AUTOHSCROLL, 0, 0, 100, 60, ID_TEXT_EDITBOX)
	a.textTool.FontCombo = mk("COMBOBOX", "", WS_CHILD|WS_TABSTOP|WS_VSCROLL|CBS_DROPDOWNLIST|CBS_HASSTRINGS|CBS_OWNERDRAWFIXED|CBS_NOINTEGRALHEIGHT, 0, 0, 190, 460, ID_TEXT_FONTCOMBO)
	a.textTool.SizeCombo = mk("COMBOBOX", "", WS_CHILD|WS_TABSTOP|WS_VSCROLL|CBS_DROPDOWN|CBS_HASSTRINGS|CBS_NOINTEGRALHEIGHT, 0, 0, 72, 330, ID_TEXT_SIZECOMBO)
	a.textTool.ScriptCombo = mk("COMBOBOX", "", WS_CHILD|WS_TABSTOP|WS_VSCROLL|CBS_DROPDOWNLIST|CBS_HASSTRINGS|CBS_NOINTEGRALHEIGHT, 0, 0, 112, 240, ID_TEXT_SCRIPTCOMBO)
	a.textTool.BoldBtn = mk("BUTTON", "B", WS_CHILD|WS_TABSTOP|BS_OWNERDRAW, 0, 0, 26, 24, ID_TEXT_BOLD)
	a.textTool.ItalicBtn = mk("BUTTON", "I", WS_CHILD|WS_TABSTOP|BS_OWNERDRAW, 0, 0, 26, 24, ID_TEXT_ITALIC)
	a.textTool.UnderlineBtn = mk("BUTTON", "U", WS_CHILD|WS_TABSTOP|BS_OWNERDRAW, 0, 0, 26, 24, ID_TEXT_UNDERLINE)
	// The hidden layout control is a Rich Edit 4.1 engine.  The visible/input
	// control remains a one-pixel plain EDIT for reliable Korean IME input,
	// while Rich Edit supplies mixed-font layout, hit testing, and final raster
	// rendering.  Windows 11 ships msftedit.dll; a plain EDIT fallback preserves
	// the older single-style behavior if loading ever fails.
	layoutClass := "EDIT"
	if err := modMsftedit.Load(); err == nil {
		layoutClass = "RICHEDIT50W"
		a.textTool.LayoutRich = true
	} else {
		logf("TEXT RichEdit load failed: %v", err)
	}
	themeWindowHandles(a.darkMode, a.textTool.FontCombo, a.textTool.SizeCombo, a.textTool.ScriptCombo,
		a.textTool.BoldBtn, a.textTool.ItalicBtn, a.textTool.UnderlineBtn)
	a.textTool.LayoutHwnd = mk(layoutClass, "", WS_CHILD|ES_MULTILINE|ES_NOHIDESEL|ES_AUTOVSCROLL, 0, 0, 100, 60, ID_TEXT_LAYOUTBOX)
	if a.textTool.LayoutHwnd == 0 && a.textTool.LayoutRich {
		a.textTool.LayoutRich = false
		a.textTool.LayoutHwnd = mk("EDIT", "", WS_CHILD|ES_MULTILINE|ES_AUTOVSCROLL, 0, 0, 100, 60, ID_TEXT_LAYOUTBOX)
	}
	if a.textTool.LayoutHwnd != 0 {
		sendMessage(a.textTool.LayoutHwnd, EM_SETMARGINS, EC_LEFTMARGIN|EC_RIGHTMARGIN, 0)
		procShowWindow.Call(a.textTool.LayoutHwnd, SW_HIDE)
		logf("TEXT layout engine rich=%v", a.textTool.LayoutRich)
	}
	a.textTool.FontNames = enumerateInstalledFonts(a.hwnd)
	for _, s := range a.textTool.FontNames {
		sendMessage(a.textTool.FontCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(s))))
	}
	for _, sz := range textSizeChoices {
		sendMessage(a.textTool.SizeCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(fmt.Sprintf("%d", sz)))))
	}
	for _, s := range textScriptChoices {
		sendMessage(a.textTool.ScriptCombo, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(s))))
	}
	fontIndex := -1
	for i, s := range a.textTool.FontNames {
		if strings.EqualFold(s, a.textTool.FontName) {
			fontIndex = i
			break
		}
	}
	if fontIndex < 0 {
		for _, preferred := range []string{"맑은 고딕", "굴림", "Arial"} {
			for i, s := range a.textTool.FontNames {
				if strings.EqualFold(s, preferred) {
					a.textTool.FontName = s
					fontIndex = i
					break
				}
			}
			if fontIndex >= 0 {
				break
			}
		}
	}
	if fontIndex < 0 && len(a.textTool.FontNames) > 0 {
		fontIndex = 0
		a.textTool.FontName = a.textTool.FontNames[0]
	}
	if fontIndex >= 0 {
		sendMessage(a.textTool.FontCombo, CB_SETCURSEL, uintptr(fontIndex), 0)
	}
	for i, sz := range textSizeChoices {
		if sz == a.textTool.FontSize {
			sendMessage(a.textTool.SizeCombo, CB_SETCURSEL, uintptr(i), 0)
		}
	}
	for i, s := range textScriptChoices {
		if s == a.textTool.Script {
			sendMessage(a.textTool.ScriptCombo, CB_SETCURSEL, uintptr(i), 0)
		}
	}
	sendMessage(a.textTool.FontCombo, CB_SETITEMHEIGHT, 0, 34)
	sendMessage(a.textTool.FontCombo, CB_SETITEMHEIGHT, ^uintptr(0), 26)
	a.updateFontPreviewDropdownWidth()
	sendMessage(a.textTool.SizeCombo, CB_SETDROPPEDWIDTH, 92, 0)
	sendMessage(a.textTool.ScriptCombo, CB_SETDROPPEDWIDTH, 132, 0)
	a.syncTextStyleButtons()
	procSetWindowTextW.Call(a.textTool.SizeCombo, uintptr(unsafe.Pointer(utf16Ptr(fmt.Sprintf("%d", a.textTool.FontSize)))))
	a.updateTextFontUI()
	a.hideTextControls()
}

func (a *App) syncTextStyleButtons() {
	if a.textTool.BoldBtn != 0 {
		sendMessage(a.textTool.BoldBtn, BM_SETCHECK, uintptr(map[bool]int{false: BST_UNCHECKED, true: BST_CHECKED}[a.textTool.Bold]), 0)
	}
	if a.textTool.ItalicBtn != 0 {
		sendMessage(a.textTool.ItalicBtn, BM_SETCHECK, uintptr(map[bool]int{false: BST_UNCHECKED, true: BST_CHECKED}[a.textTool.Italic]), 0)
	}
	if a.textTool.UnderlineBtn != 0 {
		sendMessage(a.textTool.UnderlineBtn, BM_SETCHECK, uintptr(map[bool]int{false: BST_UNCHECKED, true: BST_CHECKED}[a.textTool.Underline]), 0)
	}
}

func (a *App) updateTextFontUI() {
	a.ensureTextControls()
	a.destroyTextUIFont()
	canvasPixels := textPointSizeToCanvasPixels(a.textTool.FontSize)
	// The one-pixel input EDIT is not painted, but a zoom-scaled font keeps IME
	// composition and keyboard metrics consistent with the active insertion style.
	displayPixels := min(max(canvasPixels*a.zoom, 5), 960)
	a.textTool.UIFont = createGDIFont(a.textTool.FontName, displayPixels, a.textTool.Bold, a.textTool.Italic, a.textTool.Underline)
	if !a.textTool.LayoutRich {
		a.textTool.LayoutFont = createGDIFont(a.textTool.FontName, canvasPixels, a.textTool.Bold, a.textTool.Italic, a.textTool.Underline)
	}
	if a.textTool.UIFont != 0 && a.textTool.EditHwnd != 0 {
		sendMessage(a.textTool.EditHwnd, WM_SETFONT, a.textTool.UIFont, 1)
	}
	if !a.textTool.LayoutRich && a.textTool.LayoutFont != 0 && a.textTool.LayoutHwnd != 0 {
		sendMessage(a.textTool.LayoutHwnd, WM_SETFONT, a.textTool.LayoutFont, 1)
	}
	a.syncTextStyleButtons()
	if a.textTool.Active {
		a.resetTextCaretBlink()
	}
}

func (a *App) textScreenRect() RECT {
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	ws, _, _, _ := a.layout(client)
	ox, oy := a.canvasOrigin(ws)
	return RECT{int32(ox + a.textTool.X*a.zoom), int32(oy + a.textTool.Y*a.zoom), int32(ox + (a.textTool.X+a.textTool.W)*a.zoom), int32(oy + (a.textTool.Y+a.textTool.H)*a.zoom)}
}

func (a *App) updateTextUILayout(client RECT) {
	a.ensureTextControls()
	panel := a.textPanelRect(client)
	if !a.textToolbarVisible() || panel.Right <= panel.Left {
		a.hideTextToolbarControls()
	} else {
		px := int(panel.Left)
		py := int(panel.Top)
		procMoveWindow.Call(a.textTool.FontCombo, uintptr(px+48), uintptr(py+19), 182, 460, 1)
		procMoveWindow.Call(a.textTool.SizeCombo, uintptr(px+48), uintptr(py+56), 62, 330, 1)
		procMoveWindow.Call(a.textTool.ScriptCombo, uintptr(px+155), uintptr(py+56), 75, 240, 1)
		procMoveWindow.Call(a.textTool.BoldBtn, uintptr(px+48), uintptr(py+94), 32, 27, 1)
		procMoveWindow.Call(a.textTool.ItalicBtn, uintptr(px+85), uintptr(py+94), 32, 27, 1)
		procMoveWindow.Call(a.textTool.UnderlineBtn, uintptr(px+122), uintptr(py+94), 32, 27, 1)
		for _, h := range []uintptr{a.textTool.FontCombo, a.textTool.SizeCombo, a.textTool.ScriptCombo, a.textTool.BoldBtn, a.textTool.ItalicBtn, a.textTool.UnderlineBtn} {
			if h != 0 {
				procShowWindow.Call(h, SW_SHOW)
			}
		}
	}
	if a.textTool.LayoutHwnd != 0 {
		lw, lh := max(a.textTool.W, 1), max(a.textTool.H, 1)
		procMoveWindow.Call(a.textTool.LayoutHwnd, 0, 0, uintptr(lw), uintptr(lh), 0)
		formatRect := RECT{0, 0, int32(lw), int32(lh)}
		sendMessage(a.textTool.LayoutHwnd, EM_SETRECTNP, 0, uintptr(unsafe.Pointer(&formatRect)))
		procShowWindow.Call(a.textTool.LayoutHwnd, SW_HIDE)
	}
	if a.textTool.Editing && a.textTool.Active && a.textTool.EditHwnd != 0 {
		// Keep the native EDIT control focused for keyboard/IME input, but shrink
		// it to a single device pixel.  The user sees only the raster preview,
		// preventing the native vector glyphs and preview glyphs from doubling.
		r := a.textScreenRect()
		procMoveWindow.Call(a.textTool.EditHwnd, uintptr(int(r.Left)+2), uintptr(int(r.Top)+2), 1, 1, 1)
		procShowWindow.Call(a.textTool.EditHwnd, SW_SHOW)
	} else if a.textTool.EditHwnd != 0 {
		procShowWindow.Call(a.textTool.EditHwnd, SW_HIDE)
	}
}

// markTextLayoutDirty invalidates both Rich Edit formatting and the cached
// insertion-boundary positions.  Keeping these invalidations together is the
// central A39 invariant.
func (a *App) markTextLayoutDirty() {
	a.textTool.LayoutDirty = true
	a.textTool.LayoutPositions = nil
	a.textTool.LayoutLineHeights = nil
	a.textTool.LayoutClusters = nil
}

func (a *App) beginTextEditing(x, y, w, h int) {
	a.ensureTextControls()
	a.clearTextIMEState("new text box")
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	a.textTool.Active = true
	a.textTool.Editing = true
	a.textTool.Creating = false
	a.textTool.Selecting = false
	a.textTool.X, a.textTool.Y, a.textTool.W, a.textTool.H = x, y, w, h
	// A fresh text box starts with the currently chosen toolbar style attached
	// to its initial caret. This also means clicking the still-empty box cannot
	// unexpectedly restore the application's default font.
	a.armTextInsertionStyle(0, "new text box")
	a.normalizeTextStyles(utf16UnitLen(a.textTool.Text))
	a.markTextLayoutDirty()
	a.updateTextFontUI()
	a.textTool.ControlSync = true
	procSetWindowTextW.Call(a.textTool.EditHwnd, uintptr(unsafe.Pointer(rawUTF16Ptr(a.textTool.Text))))
	a.textTool.ControlSync = false
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	a.updateTextUILayout(client)
	procSetFocus.Call(a.textTool.EditHwnd)
	a.syncTextLayoutControl()
	a.resetTextCaretBlink()
	a.statusText = "텍스트를 입력하십시오. 위·아래·왼쪽·오른쪽 키로 커서를 옮기고, Ctrl+A/C/X/V로 모두 선택·복사·잘라내기·붙여넣기를 할 수 있습니다."
	logf("TEXT begin x=%d y=%d w=%d h=%d zoom=%d", x, y, w, h, a.zoom)
}

func (a *App) currentTextValue() string {
	if a.textTool.EditHwnd != 0 && a.textTool.Active {
		return getWindowTextString(a.textTool.EditHwnd)
	}
	return a.textTool.Text
}

func utf16Units(s string) []uint16 {
	return utf16.Encode([]rune(s))
}

func utf16UnitLen(s string) int {
	return len(utf16Units(s))
}

func makeEditPointParam(x, y int) uintptr {
	x = clamp(x, -32768, 32767)
	y = clamp(y, -32768, 32767)
	return uintptr(uint16(int16(x))) | uintptr(uint16(int16(y)))<<16
}

func (a *App) currentTextStyle() TextCharStyle {
	return TextCharStyle{FontName: a.textTool.FontName, FontSize: a.textTool.FontSize, Bold: a.textTool.Bold, Italic: a.textTool.Italic, Underline: a.textTool.Underline}
}

func (a *App) activeTextInsertionStyle() TextCharStyle {
	if a.textTool.InsertionStyleActive {
		return a.textTool.InsertionStyle
	}
	return a.currentTextStyle()
}

func (a *App) armTextInsertionStyle(caret int, reason string) {
	units := utf16UnitLen(a.textTool.Text)
	caret = clamp(caret, 0, units)
	a.textTool.InsertionStyle = a.currentTextStyle()
	a.textTool.InsertionStyleActive = true
	a.textTool.InsertionCaret = caret
	a.textTool.InsertionAnchor = caret
	a.textTool.InsertionSpanEnd = caret
	logf("TEXT insertion style armed caret=%d anchor=%d span=%d..%d reason=%q font=%q size=%d b=%v i=%v u=%v", caret, caret, caret, caret, reason, a.textTool.InsertionStyle.FontName, a.textTool.InsertionStyle.FontSize, a.textTool.InsertionStyle.Bold, a.textTool.InsertionStyle.Italic, a.textTool.InsertionStyle.Underline)
}

// armTextInsertionRange attaches the current toolbar style to a replacement
// range. It is used for paste so the inserted text receives one deterministic
// style while the untouched prefix and suffix keep their original formatting.
func (a *App) armTextInsertionRange(start, end int, reason string) {
	units := utf16UnitLen(a.textTool.Text)
	start = clamp(start, 0, units)
	end = clamp(end, 0, units)
	if start > end {
		start, end = end, start
	}
	a.textTool.InsertionStyle = a.currentTextStyle()
	a.textTool.InsertionStyleActive = true
	a.textTool.InsertionCaret = start
	a.textTool.InsertionAnchor = start
	a.textTool.InsertionSpanEnd = end
	logf("TEXT insertion range armed caret=%d span=%d..%d reason=%q font=%q size=%d b=%v i=%v u=%v", start, start, end, reason, a.textTool.InsertionStyle.FontName, a.textTool.InsertionStyle.FontSize, a.textTool.InsertionStyle.Bold, a.textTool.InsertionStyle.Italic, a.textTool.InsertionStyle.Underline)
}

func (a *App) setTextToolbarStyle(style TextCharStyle) {
	a.textTool.FontName = style.FontName
	a.textTool.FontSize = style.FontSize
	a.textTool.Bold = style.Bold
	a.textTool.Italic = style.Italic
	a.textTool.Underline = style.Underline
	a.syncTextToolbarControls()
	a.updateTextFontUI()
}

func textStyleAtCaretFromSlice(caret int, styles []TextCharStyle, fallback TextCharStyle) TextCharStyle {
	if len(styles) == 0 {
		return fallback
	}
	caret = clamp(caret, 0, len(styles))
	if caret > 0 {
		return styles[caret-1]
	}
	return styles[0]
}

func (a *App) clearTextInsertionStyle(reason string) {
	if a.textTool.InsertionStyleActive {
		logf("TEXT insertion style cleared caret=%d span=%d..%d reason=%q", a.textTool.InsertionCaret, a.textTool.InsertionAnchor, a.textTool.InsertionSpanEnd, reason)
	}
	a.textTool.InsertionStyleActive = false
	a.textTool.InsertionCaret = 0
	a.textTool.InsertionAnchor = 0
	a.textTool.InsertionSpanEnd = 0
}

func (a *App) textSelection() (int, int) {
	if a.textTool.EditHwnd == 0 {
		return 0, 0
	}
	var start, end uint32
	sendMessage(a.textTool.EditHwnd, EM_GETSEL, uintptr(unsafe.Pointer(&start)), uintptr(unsafe.Pointer(&end)))
	return int(start), int(end)
}

func (a *App) normalizeTextStyles(units int) {
	if units < 0 {
		units = 0
	}
	style := a.activeTextInsertionStyle()
	if len(a.textTool.Styles) > units {
		a.textTool.Styles = a.textTool.Styles[:units]
	}
	for len(a.textTool.Styles) < units {
		a.textTool.Styles = append(a.textTool.Styles, style)
	}
}

// The input control stores plain UTF-16 text while dotPaint owns a parallel
// per-code-unit style array.  A prefix/suffix diff keeps existing formatting
// stable through typing, Backspace, paste, and Korean IME composition; newly
// inserted units inherit the current toolbar/insertion style.
func utf16UnitsEqual(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// syncAnchoredInsertionModel handles the ambiguous repeated-character case
// that occurs during Korean IME composition. While a collapsed-caret style is
// active, text before InsertionAnchor and text after InsertionSpanEnd are the
// pre-existing left/right neighbours. If both neighbours are still present in
// the new EDIT value, every unit between them is unquestionably owned by the
// insertion style, even when all visible syllables are identical (for example
// bold "기" + non-bold "기" + bold "기").
func (a *App) syncAnchoredInsertionModel(oldUnits, newUnits []uint16) bool {
	if !a.textTool.InsertionStyleActive {
		return false
	}
	anchor := a.textTool.InsertionAnchor
	oldEnd := a.textTool.InsertionSpanEnd
	if anchor < 0 || oldEnd < anchor || oldEnd > len(oldUnits) || anchor > len(newUnits) {
		return false
	}
	if !utf16UnitsEqual(oldUnits[:anchor], newUnits[:anchor]) {
		return false
	}
	oldSuffix := oldUnits[oldEnd:]
	newSuffixStart := len(newUnits) - len(oldSuffix)
	if newSuffixStart < anchor || !utf16UnitsEqual(oldSuffix, newUnits[newSuffixStart:]) {
		return false
	}

	styles := make([]TextCharStyle, 0, len(newUnits))
	styles = append(styles, a.textTool.Styles[:anchor]...)
	for i := anchor; i < newSuffixStart; i++ {
		styles = append(styles, a.textTool.InsertionStyle)
	}
	styles = append(styles, a.textTool.Styles[oldEnd:]...)
	if len(styles) != len(newUnits) {
		return false
	}

	a.textTool.Styles = styles
	a.textTool.InsertionSpanEnd = newSuffixStart
	start, end := a.textSelection()
	if start == end {
		a.textTool.InsertionCaret = clamp(start, anchor, newSuffixStart)
	} else {
		a.textTool.InsertionCaret = clamp(a.textTool.InsertionCaret, anchor, newSuffixStart)
	}
	logf("TEXT model anchored oldUnits=%d newUnits=%d anchor=%d oldSpan=%d..%d newSpan=%d..%d suffixUnits=%d insertionCaret=%d", len(oldUnits), len(newUnits), anchor, anchor, oldEnd, anchor, newSuffixStart, len(oldSuffix), a.textTool.InsertionCaret)
	return true
}

func cloneTextStyles(src []TextCharStyle) []TextCharStyle {
	if len(src) == 0 {
		return nil
	}
	out := make([]TextCharStyle, len(src))
	copy(out, src)
	return out
}

// beginTextIMEComposition snapshots the committed text and styles before the
// Windows IME starts its transient delete/recreate cycle. Every intermediate
// composition value is rebuilt from this immutable base, so the suffix after
// the caret can never inherit the composing character's bold/italic style.
func (a *App) beginTextIMEComposition() {
	if !a.textTool.Active || !a.textTool.Editing || a.textTool.EditHwnd == 0 || a.textTool.IMEActive {
		return
	}
	a.syncTextModelFromInput()
	units := utf16UnitLen(a.textTool.Text)
	a.normalizeTextStyles(units)
	start, end := a.textSelection()
	if start > end {
		start, end = end, start
	}
	start = clamp(start, 0, units)
	end = clamp(end, start, units)
	a.textTool.IMEActive = true
	a.textTool.IMEBaseText = a.textTool.Text
	a.textTool.IMEBaseStyles = cloneTextStyles(a.textTool.Styles)
	a.textTool.IMEBaseStart = start
	a.textTool.IMEBaseEnd = end
	a.textTool.IMEStyle = a.activeTextInsertionStyle()
	a.textTool.IMEInsertionWasActive = a.textTool.InsertionStyleActive
	a.textTool.IMEOwnedAnchor = a.textTool.InsertionAnchor
	a.textTool.IMEOwnedSpanEnd = a.textTool.InsertionSpanEnd
	a.textTool.IMEVisualCaret = end
	a.textTool.IMEVisualCaretValid = true
	a.textTool.IMEPendingShrink = false
	a.textTool.IMEPendingCandidate = end
	a.textTool.IMEPendingChangeUnixMS = 0
	logf("TEXT IME begin baseUnits=%d selection=%d..%d insertionActive=%v owned=%d..%d", units, start, end, a.textTool.IMEInsertionWasActive, a.textTool.IMEOwnedAnchor, a.textTool.IMEOwnedSpanEnd)
}

func (a *App) clearTextIMEState(reason string) {
	if a.textTool.IMEActive || a.textTool.IMEVisualCaretValid || a.textTool.IMEPendingShrink {
		logf("TEXT IME clear reason=%q active=%v visualCaret=%d pendingShrink=%v", reason, a.textTool.IMEActive, a.textTool.IMEVisualCaret, a.textTool.IMEPendingShrink)
	}
	a.textTool.IMEActive = false
	a.textTool.IMEBaseText = ""
	a.textTool.IMEBaseStyles = nil
	a.textTool.IMEBaseStart = 0
	a.textTool.IMEBaseEnd = 0
	a.textTool.IMEVisualCaret = 0
	a.textTool.IMEVisualCaretValid = false
	a.textTool.IMEPendingShrink = false
	a.textTool.IMEPendingCandidate = 0
	a.textTool.IMEPendingChangeUnixMS = 0
}

// syncTextIMEModelFromInput applies the current EDIT value against the fixed
// composition-start snapshot. A one-unit backwards step is normally the IME's
// internal replacement phase, so it is held briefly instead of being exposed
// to the custom caret or style model. A genuine Backspace is accepted by the
// timer or when composition ends.
func (a *App) syncTextIMEModelFromInput(force bool) bool {
	if !a.textTool.IMEActive || a.textTool.EditHwnd == 0 {
		return false
	}
	newText := getWindowTextString(a.textTool.EditHwnd)
	baseUnits := utf16Units(a.textTool.IMEBaseText)
	newUnits := utf16Units(newText)
	baseStart := clamp(a.textTool.IMEBaseStart, 0, len(baseUnits))
	baseEnd := clamp(a.textTool.IMEBaseEnd, baseStart, len(baseUnits))

	editStart, editEnd, anchored := textIMEAnchoredRange(baseUnits, newUnits, baseStart, baseEnd)
	prefix, suffix := 0, 0
	if !anchored {
		for prefix < len(baseUnits) && prefix < len(newUnits) && baseUnits[prefix] == newUnits[prefix] {
			prefix++
		}
		for suffix < len(baseUnits)-prefix && suffix < len(newUnits)-prefix && baseUnits[len(baseUnits)-1-suffix] == newUnits[len(newUnits)-1-suffix] {
			suffix++
		}
		editStart = prefix
		editEnd = len(newUnits) - suffix
	}
	candidate := clamp(editEnd, 0, len(newUnits))
	currentUnits := utf16UnitLen(a.textTool.Text)
	if !force && a.textTool.IMEVisualCaretValid && candidate < a.textTool.IMEVisualCaret && len(newUnits) < currentUnits {
		a.textTool.IMEPendingShrink = true
		a.textTool.IMEPendingCandidate = candidate
		a.textTool.IMEPendingChangeUnixMS = time.Now().UnixMilli()
		logf("TEXT IME transient shrink held modelUnits=%d inputUnits=%d visualCaret=%d candidate=%d", currentUnits, len(newUnits), a.textTool.IMEVisualCaret, candidate)
		return false
	}

	baseStyles := a.textTool.IMEBaseStyles
	if len(baseStyles) < len(baseUnits) {
		fill := a.textTool.IMEStyle
		baseStyles = cloneTextStyles(baseStyles)
		for len(baseStyles) < len(baseUnits) {
			baseStyles = append(baseStyles, fill)
		}
	}
	styles := make([]TextCharStyle, 0, len(newUnits))
	if anchored {
		styles = append(styles, baseStyles[:baseStart]...)
		for i := baseStart; i < editEnd; i++ {
			styles = append(styles, a.textTool.IMEStyle)
		}
		styles = append(styles, baseStyles[baseEnd:]...)
	} else {
		styles = append(styles, baseStyles[:editStart]...)
		for i := editStart; i < editEnd; i++ {
			styles = append(styles, a.textTool.IMEStyle)
		}
		if suffix > 0 {
			styles = append(styles, baseStyles[len(baseUnits)-suffix:]...)
		}
	}
	if len(styles) != len(newUnits) {
		logf("TEXT IME style length mismatch styles=%d units=%d; normalizing", len(styles), len(newUnits))
		if len(styles) > len(newUnits) {
			styles = styles[:len(newUnits)]
		}
		for len(styles) < len(newUnits) {
			styles = append(styles, a.textTool.IMEStyle)
		}
	}

	a.textTool.Text = newText
	a.textTool.Styles = styles
	a.textTool.VerticalGoalValid = false
	a.textTool.IMEVisualCaret = candidate
	a.textTool.IMEVisualCaretValid = true
	a.textTool.IMEPendingShrink = false
	a.textTool.IMEPendingCandidate = candidate
	a.textTool.IMEPendingChangeUnixMS = 0

	if a.textTool.IMEInsertionWasActive {
		a.textTool.InsertionStyle = a.textTool.IMEStyle
		a.textTool.InsertionStyleActive = true
		delta := len(newUnits) - len(baseUnits)
		if baseStart >= a.textTool.IMEOwnedAnchor && baseEnd <= a.textTool.IMEOwnedSpanEnd {
			a.textTool.InsertionAnchor = clamp(a.textTool.IMEOwnedAnchor, 0, len(newUnits))
			a.textTool.InsertionSpanEnd = clamp(a.textTool.IMEOwnedSpanEnd+delta, a.textTool.InsertionAnchor, len(newUnits))
		} else {
			a.textTool.InsertionAnchor = clamp(editStart, 0, len(newUnits))
			a.textTool.InsertionSpanEnd = clamp(editEnd, a.textTool.InsertionAnchor, len(newUnits))
		}
		a.textTool.InsertionCaret = candidate
	}
	a.markTextLayoutDirty()
	mode := "diff"
	if anchored {
		mode = "anchored-base"
	}
	logf("TEXT IME model baseUnits=%d newUnits=%d range=%d..%d mode=%s force=%v visualCaret=%d owned=%d..%d", len(baseUnits), len(newUnits), editStart, editEnd, mode, force, a.textTool.IMEVisualCaret, a.textTool.InsertionAnchor, a.textTool.InsertionSpanEnd)
	return true
}

func (a *App) flushPendingTextIME() bool {
	if !a.textTool.IMEActive || !a.textTool.IMEPendingShrink {
		return false
	}
	return a.syncTextIMEModelFromInput(true)
}

func (a *App) endTextIMEComposition() {
	if !a.textTool.IMEActive {
		return
	}
	a.syncTextIMEModelFromInput(true)
	start, end := a.textSelection()
	if start == end {
		a.textTool.IMEVisualCaret = clamp(start, 0, utf16UnitLen(a.textTool.Text))
	}
	logf("TEXT IME end units=%d selection=%d..%d", utf16UnitLen(a.textTool.Text), start, end)
	a.clearTextIMEState("composition ended")
	a.syncTextLayoutControl()
	a.resetTextCaretBlink()
	a.requestTextRepaint(false)
}

func (a *App) displayTextCaretIndex() int {
	if a.textTool.IMEActive && a.textTool.IMEVisualCaretValid {
		return clamp(a.textTool.IMEVisualCaret, 0, utf16UnitLen(a.textTool.Text))
	}
	start, end := a.textSelection()
	if start != end {
		return -1
	}
	return clamp(start, 0, utf16UnitLen(a.textTool.Text))
}

func (a *App) syncTextModelFromInput() {
	if a.textTool.EditHwnd == 0 || a.textTool.ControlSync {
		return
	}
	if a.textTool.IMEActive {
		a.syncTextIMEModelFromInput(false)
		return
	}
	newText := getWindowTextString(a.textTool.EditHwnd)
	if newText == a.textTool.Text && len(a.textTool.Styles) == utf16UnitLen(newText) {
		return
	}
	oldUnits := utf16Units(a.textTool.Text)
	newUnits := utf16Units(newText)
	a.normalizeTextStyles(len(oldUnits))

	mode := "diff"
	prefix, suffix, inserted := 0, 0, 0
	if a.syncAnchoredInsertionModel(oldUnits, newUnits) {
		mode = "anchored"
		prefix = a.textTool.InsertionAnchor
		suffix = len(newUnits) - a.textTool.InsertionSpanEnd
		inserted = a.textTool.InsertionSpanEnd - a.textTool.InsertionAnchor
	} else {
		for prefix < len(oldUnits) && prefix < len(newUnits) && oldUnits[prefix] == newUnits[prefix] {
			prefix++
		}
		for suffix < len(oldUnits)-prefix && suffix < len(newUnits)-prefix && oldUnits[len(oldUnits)-1-suffix] == newUnits[len(newUnits)-1-suffix] {
			suffix++
		}
		inserted = len(newUnits) - prefix - suffix

		// A collapsed-caret format is valid only at its owned insertion span.
		// In A178 the hidden EDIT could move with Left/Right while dotPaint never
		// learned about the new caret. The next edit then reused a stale bold
		// style and could visually spill into a pre-existing suffix such as '?'.
		// Treat an edit well outside the owned span as an unreported caret move,
		// adopt the surrounding text style, and keep the old suffix immutable.
		if a.textTool.InsertionStyleActive {
			anchor := a.textTool.InsertionAnchor
			oldEnd := a.textTool.InsertionSpanEnd
			outsideOwnedSpan := textEditOutsideOwnedSpan(prefix, anchor, oldEnd)
			if outsideOwnedSpan {
				surrounding := textStyleAtCaretFromSlice(prefix, a.textTool.Styles, a.currentTextStyle())
				a.clearTextInsertionStyle("edit occurred after unreported caret move")
				a.setTextToolbarStyle(surrounding)
				logf("TEXT stale insertion style discarded editPrefix=%d oldSpan=%d..%d", prefix, anchor, oldEnd)
			}
		}

		styles := make([]TextCharStyle, 0, len(newUnits))
		styles = append(styles, a.textTool.Styles[:prefix]...)
		insertStyle := a.activeTextInsertionStyle()
		for i := 0; i < inserted; i++ {
			styles = append(styles, insertStyle)
		}
		if suffix > 0 {
			styles = append(styles, a.textTool.Styles[len(oldUnits)-suffix:]...)
		}
		a.textTool.Styles = styles
		if a.textTool.InsertionStyleActive {
			// The anchored invariant could not be preserved (for example Delete
			// removed a pre-existing neighbour). Rebase the owned span around the
			// actual edit so subsequent IME notifications are deterministic again.
			a.textTool.InsertionAnchor = clamp(prefix, 0, len(newUnits))
			a.textTool.InsertionSpanEnd = clamp(prefix+max(inserted, 0), a.textTool.InsertionAnchor, len(newUnits))
		}
	}

	a.textTool.Text = newText
	a.textTool.VerticalGoalValid = false
	if a.textTool.InsertionStyleActive {
		start, end := a.textSelection()
		if start == end {
			a.textTool.InsertionCaret = clamp(start, 0, len(newUnits))
		}
	}
	a.markTextLayoutDirty()
	logf("TEXT model change oldUnits=%d newUnits=%d prefix=%d suffix=%d inserted=%d mode=%s insertionActive=%v insertionCaret=%d span=%d..%d", len(oldUnits), len(newUnits), prefix, suffix, inserted, mode, a.textTool.InsertionStyleActive, a.textTool.InsertionCaret, a.textTool.InsertionAnchor, a.textTool.InsertionSpanEnd)
}

func (a *App) textStyleAt(index int) TextCharStyle {
	units := utf16UnitLen(a.textTool.Text)
	a.normalizeTextStyles(units)
	if units == 0 {
		return a.currentTextStyle()
	}
	if index >= units {
		index = units - 1
	}
	if index < 0 {
		index = 0
	}
	return a.textTool.Styles[index]
}

// buildRichLayoutText normalizes the plain EDIT's CRLF pairs to the single
// paragraph marker used by Rich Edit. Boundary maps keep UTF-16 selection
// indices stable in both directions, including Korean text and surrogate pairs.
func (a *App) buildRichLayoutText(text string) (string, []TextCharStyle, []int, []int) {
	in := utf16Units(text)
	a.normalizeTextStyles(len(in))
	out := make([]uint16, 0, len(in))
	styles := make([]TextCharStyle, 0, len(in))
	inputToRich := make([]int, len(in)+1)
	richToInput := []int{0}
	for i := 0; i < len(in); {
		j := len(out)
		inputToRich[i] = j
		if in[i] == '\r' && i+1 < len(in) && in[i+1] == '\n' {
			out = append(out, '\r')
			styles = append(styles, a.textTool.Styles[i])
			inputToRich[i+1] = j + 1
			i += 2
			inputToRich[i] = j + 1
			richToInput = append(richToInput, i)
			continue
		}
		out = append(out, in[i])
		styles = append(styles, a.textTool.Styles[i])
		i++
		inputToRich[i] = j + 1
		richToInput = append(richToInput, i)
	}
	inputToRich[len(in)] = len(out)
	return string(utf16.Decode(out)), styles, inputToRich, richToInput
}

func (a *App) inputIndexToRich(index int) int {
	if len(a.textTool.LayoutInputToRich) == 0 {
		return max(index, 0)
	}
	index = clamp(index, 0, len(a.textTool.LayoutInputToRich)-1)
	return a.textTool.LayoutInputToRich[index]
}

func (a *App) richIndexToInput(index int) int {
	if len(a.textTool.LayoutRichToInput) == 0 {
		return max(index, 0)
	}
	index = clamp(index, 0, len(a.textTool.LayoutRichToInput)-1)
	return a.textTool.LayoutRichToInput[index]
}

func (a *App) textRenderDPI() int {
	if a.hwnd != 0 {
		dpi, _, _ := procGetDpiForWindow.Call(a.hwnd)
		if dpi >= 48 && dpi <= 768 {
			return int(dpi)
		}
	}
	return 96
}

func makeTextCharFormat(style TextCharStyle, color uint32, dpi int) CHARFORMATW {
	if style.FontSize < 1 {
		style.FontSize = 1
	}
	if isTransparentColor(color) {
		color = rgb(0, 0, 0)
	}
	if dpi < 1 {
		dpi = 96
	}
	canvasPixels := textPointSizeToCanvasPixels(style.FontSize)
	// Rich Edit measures character height in twips against the target HDC DPI.
	// Convert the canvas-native pixel height back to twips so a 150% Windows
	// display does not make committed text 1.5 times larger than at 100%.
	yHeightTwips := max(1, int(math.Round(float64(canvasPixels)*1440.0/float64(dpi))))
	cf := CHARFORMATW{
		CbSize:      uint32(unsafe.Sizeof(CHARFORMATW{})),
		DwMask:      CFM_FACE | CFM_SIZE | CFM_COLOR | CFM_BOLD | CFM_ITALIC | CFM_UNDERLINE,
		YHeight:     int32(yHeightTwips),
		CrTextColor: color,
		BCharSet:    DEFAULT_CHARSET,
	}
	if style.Bold {
		cf.DwEffects |= CFE_BOLD
	}
	if style.Italic {
		cf.DwEffects |= CFE_ITALIC
	}
	if style.Underline {
		cf.DwEffects |= CFE_UNDERLINE
	}
	face := utf16.Encode([]rune(style.FontName))
	if len(face) > len(cf.SzFaceName)-1 {
		face = face[:len(cf.SzFaceName)-1]
	}
	copy(cf.SzFaceName[:], face)
	return cf
}

func setRichCharFormat(hwnd uintptr, start, end int, style TextCharStyle, color uint32, dpi int) {
	if hwnd == 0 {
		return
	}
	sendMessage(hwnd, EM_SETSEL, uintptr(start), uintptr(end))
	cf := makeTextCharFormat(style, color, dpi)
	sendMessage(hwnd, EM_SETCHARFORMAT, SCF_SELECTION, uintptr(unsafe.Pointer(&cf)))
}

// textLayoutSentinelStyle keeps the hidden zero-width layout sentinel from
// leaking the toolbar's current insertion style into the last visible glyph.
// When real text exists, the sentinel must always match the last real UTF-16
// unit. Empty text has no visible neighbour, so the active insertion style is
// appropriate and gives the caret the requested height.
func textLayoutSentinelStyle(realStyles []TextCharStyle, fallback, insertion TextCharStyle, insertionActive bool) TextCharStyle {
	if len(realStyles) > 0 {
		return realStyles[len(realStyles)-1]
	}
	if insertionActive {
		return insertion
	}
	return fallback
}

func (a *App) textLayoutCaretStyle(caret int) TextCharStyle {
	if a.textTool.InsertionStyleActive {
		return a.textTool.InsertionStyle
	}
	return textStyleAtCaretFromSlice(caret, a.textTool.Styles, a.currentTextStyle())
}

func (a *App) richCharFromPoint(x, y int) int {
	if a.textTool.LayoutHwnd == 0 {
		return 0
	}
	if a.textTool.LayoutRich {
		pt := POINT{X: int32(x), Y: int32(y)}
		richIndex := int(int32(sendMessage(a.textTool.LayoutHwnd, EM_CHARFROMPOS, 0, uintptr(unsafe.Pointer(&pt)))))
		return a.richIndexToInput(richIndex)
	}
	return int(loword(sendMessage(a.textTool.LayoutHwnd, EM_CHARFROMPOS, 0, makeEditPointParam(x, y))))
}

func (a *App) richPointFromChar(index int) (int, int, bool) {
	if a.textTool.LayoutHwnd == 0 {
		return 0, 0, false
	}
	if a.textTool.LayoutRich {
		pt := POINT{}
		richIndex := a.inputIndexToRich(index)
		r := sendMessage(a.textTool.LayoutHwnd, EM_POSFROMCHAR, uintptr(unsafe.Pointer(&pt)), uintptr(richIndex))
		if int32(r) == -1 {
			return 0, 0, false
		}
		return int(pt.X), int(pt.Y), true
	}
	pos := sendMessage(a.textTool.LayoutHwnd, EM_POSFROMCHAR, uintptr(index), 0)
	if pos == ^uintptr(0) {
		return 0, 0, false
	}
	return int(int16(loword(pos))), int(int16(hiword(pos))), true
}

func gcdPositive(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a < 1 {
		return 1
	}
	return a
}

// syncRichLayoutZoom compensates for the DPI used to rasterize the text.
// Character formats are converted so the target HDC receives the requested
// canvas-pixel height. EM_POSFROMCHAR, however, reports coordinates in the
// hidden Rich Edit control's own DPI. Applying the inverse Rich Edit zoom keeps
// those insertion boundaries in the same native-canvas pixel coordinate system
// as the raster preview and final commit.
func (a *App) syncRichLayoutZoom(targetDPI int) {
	if !a.textTool.LayoutRich || a.textTool.LayoutHwnd == 0 {
		return
	}
	controlDPI := 96
	if dpi, _, _ := procGetDpiForWindow.Call(a.textTool.LayoutHwnd); dpi >= 48 && dpi <= 768 {
		controlDPI = int(dpi)
	} else if dpi, _, _ := procGetDpiForWindow.Call(a.hwnd); dpi >= 48 && dpi <= 768 {
		controlDPI = int(dpi)
	}
	if targetDPI < 48 || targetDPI > 768 {
		targetDPI = controlDPI
	}
	g := gcdPositive(targetDPI, controlDPI)
	num, den := targetDPI/g, controlDPI/g
	if num > den*64 {
		num, den = 64, 1
	}
	if den > num*64 {
		num, den = 1, 64
	}
	if a.textTool.LayoutControlDPI == controlDPI && a.textTool.LayoutZoomNum == num && a.textTool.LayoutZoomDen == den {
		return
	}
	sendMessage(a.textTool.LayoutHwnd, EM_SETZOOM, uintptr(num), uintptr(den))
	a.textTool.LayoutControlDPI = controlDPI
	a.textTool.LayoutZoomNum = num
	a.textTool.LayoutZoomDen = den
	a.textTool.LayoutPositions = nil
	logf("TEXT layout zoom targetDPI=%d controlDPI=%d ratio=%d/%d", targetDPI, controlDPI, num, den)
}

// syncTextLayoutControl mirrors plain input text plus dotPaint's style runs
// into a hidden Rich Edit control.  The trailing sentinel provides a stable
// position for the caret after the final real character but is excluded from
// final rendering.
func (a *App) syncTextLayoutControl() int {
	return a.syncTextLayoutControlAtDPI(a.textRenderDPI())
}

func (a *App) syncTextLayoutControlAtDPI(dpi int) int {
	if a.textTool.LayoutHwnd == 0 || a.textTool.EditHwnd == 0 {
		return 0
	}
	a.syncTextModelFromInput()
	if dpi < 1 {
		dpi = 96
	}
	if a.textTool.LayoutDPI != dpi {
		a.textTool.LayoutDPI = dpi
		a.markTextLayoutDirty()
	}
	a.syncRichLayoutZoom(dpi)
	if a.textTool.LayoutColor != a.fg {
		a.textTool.LayoutColor = a.fg
		a.markTextLayoutDirty()
	}
	text := a.textTool.Text
	units := utf16UnitLen(text)
	a.normalizeTextStyles(units)
	richText, richStyles, inputToRich, richToInput := a.buildRichLayoutText(text)
	richUnits := len(richStyles)
	layoutText := richText + "\uFEFF"
	a.textTool.LayoutUnits = richUnits
	a.textTool.LayoutInputToRich = inputToRich
	a.textTool.LayoutRichToInput = richToInput
	lw, lh := max(a.textTool.W, 1), max(a.textTool.H, 1)
	textChanged := layoutText != a.textTool.LayoutText
	if textChanged {
		procSetWindowTextW.Call(a.textTool.LayoutHwnd, uintptr(unsafe.Pointer(rawUTF16Ptr(layoutText))))
		a.textTool.LayoutText = layoutText
		a.markTextLayoutDirty()
	}
	if lw != a.textTool.LayoutW || lh != a.textTool.LayoutH {
		procMoveWindow.Call(a.textTool.LayoutHwnd, 0, 0, uintptr(lw), uintptr(lh), 0)
		formatRect := RECT{0, 0, int32(lw), int32(lh)}
		sendMessage(a.textTool.LayoutHwnd, EM_SETRECTNP, 0, uintptr(unsafe.Pointer(&formatRect)))
		a.textTool.LayoutW, a.textTool.LayoutH = lw, lh
		a.textTool.LayoutPositions = nil
		a.textTool.LayoutLineHeights = nil
	}
	if a.textTool.LayoutRich && a.textTool.LayoutDirty {
		base := a.currentTextStyle()
		setRichCharFormat(a.textTool.LayoutHwnd, 0, richUnits+1, base, a.fg, dpi)
		start := 0
		for start < richUnits {
			style := richStyles[start]
			end := start + 1
			for end < richUnits && richStyles[end].Equal(style) {
				end++
			}
			setRichCharFormat(a.textTool.LayoutHwnd, start, end, style, a.fg, dpi)
			start = end
		}

		// A224: the sentinel is a real Rich Edit character even though dotPaint
		// excludes it from visible rendering. If it keeps the toolbar's current
		// style, Uniscribe/RichEdit can shape it together with the final glyph and
		// make only that last glyph appear bold, resized, italic, or underlined.
		// Match the final real character instead. The collapsed caret receives its
		// independent insertion format below, without mutating the sentinel.
		sentinelStyle := textLayoutSentinelStyle(richStyles, base, a.activeTextInsertionStyle(), a.textTool.InsertionStyleActive)
		setRichCharFormat(a.textTool.LayoutHwnd, richUnits, richUnits+1, sentinelStyle, a.fg, dpi)

		// Character-format changes can force Rich Edit to rebuild its display
		// metrics, so reassert the DPI compensation before reading boundaries.
		a.syncRichLayoutZoom(dpi)
		// Restore the layout selection to the input selection for predictable
		// internal caret metrics, even though the control itself stays hidden.
		selStart, selEnd := a.textSelection()
		richSelStart := a.inputIndexToRich(selStart)
		richSelEnd := a.inputIndexToRich(selEnd)
		sendMessage(a.textTool.LayoutHwnd, EM_SETSEL, uintptr(richSelStart), uintptr(richSelEnd))
		if richSelStart == richSelEnd {
			caretStyle := a.textLayoutCaretStyle(selStart)
			cf := makeTextCharFormat(caretStyle, a.fg, dpi)
			sendMessage(a.textTool.LayoutHwnd, EM_SETCHARFORMAT, SCF_SELECTION, uintptr(unsafe.Pointer(&cf)))
		}
		logf("TEXT layout final-style realUnits=%d sentinel=%q/%d/b%v/i%v/u%v caret=%d..%d insertionActive=%v", richUnits, sentinelStyle.FontName, sentinelStyle.FontSize, sentinelStyle.Bold, sentinelStyle.Italic, sentinelStyle.Underline, selStart, selEnd, a.textTool.InsertionStyleActive)
		a.textTool.LayoutDirty = false
	} else if !a.textTool.LayoutRich && a.textTool.LayoutFont != 0 {
		sendMessage(a.textTool.LayoutHwnd, WM_SETFONT, a.textTool.LayoutFont, 0)
	}
	return units
}

func (a *App) syncTextToolbarControls() {
	if a.textTool.FontCombo == 0 {
		return
	}
	a.textTool.ControlSync = true
	defer func() { a.textTool.ControlSync = false }()
	fontIndex := -1
	for i, name := range a.textTool.FontNames {
		if strings.EqualFold(name, a.textTool.FontName) {
			fontIndex = i
			break
		}
	}
	if fontIndex >= 0 {
		sendMessage(a.textTool.FontCombo, CB_SETCURSEL, uintptr(fontIndex), 0)
	}
	procSetWindowTextW.Call(a.textTool.SizeCombo, uintptr(unsafe.Pointer(utf16Ptr(strconv.Itoa(a.textTool.FontSize)))))
	for i, sz := range textSizeChoices {
		if sz == a.textTool.FontSize {
			sendMessage(a.textTool.SizeCombo, CB_SETCURSEL, uintptr(i), 0)
			break
		}
	}
	a.syncTextStyleButtons()
}

func (a *App) adoptTextStyleFromSelection() {
	a.syncTextModelFromInput()
	start, end := a.textSelection()
	idx := start
	if start == end && idx > 0 {
		idx--
	}
	style := a.textStyleAt(idx)
	a.textTool.FontName = style.FontName
	a.textTool.FontSize = style.FontSize
	a.textTool.Bold = style.Bold
	a.textTool.Italic = style.Italic
	a.textTool.Underline = style.Underline
	a.syncTextToolbarControls()
	a.updateTextFontUI()
}

// applyTextPropertyToSelection gives PowerPoint-like behavior: with a nonempty
// selection only those UTF-16 units change; with a caret only the insertion
// style changes, so subsequently typed text inherits the new property.
func (a *App) applyTextPropertyToSelection(kind string) {
	a.syncTextModelFromInput()
	start, end := a.textSelection()
	if start > end {
		start, end = end, start
	}
	units := utf16UnitLen(a.textTool.Text)
	start = clamp(start, 0, units)
	end = clamp(end, 0, units)
	if start < end {
		a.clearTextInsertionStyle("formatting a selected range")
		a.normalizeTextStyles(units)
		for i := start; i < end; i++ {
			s := a.textTool.Styles[i]
			switch kind {
			case "font":
				s.FontName = a.textTool.FontName
			case "size":
				s.FontSize = a.textTool.FontSize
			case "bold":
				s.Bold = a.textTool.Bold
			case "italic":
				s.Italic = a.textTool.Italic
			case "underline":
				s.Underline = a.textTool.Underline
			}
			a.textTool.Styles[i] = s
		}
	} else {
		// With no selected text, formatting belongs to the caret rather than to
		// either neighbouring character. Keep the complete toolbar style so a
		// sequence such as font -> size -> bold accumulates on the same caret.
		a.armTextInsertionStyle(start, "toolbar "+kind)
	}
	a.markTextLayoutDirty()
	a.syncTextLayoutControl()
	a.resetTextCaretBlink()
	a.requestTextRepaint(true)
	logf("TEXT format kind=%s range=%d..%d font=%q size=%d b=%v i=%v u=%v", kind, start, end, a.textTool.FontName, a.textTool.FontSize, a.textTool.Bold, a.textTool.Italic, a.textTool.Underline)
}

func (a *App) resetTextCaretBlink() {
	if !a.textTool.Active || !a.textTool.Editing {
		return
	}
	a.textTool.CaretVisible = true
	if !a.textTool.CaretTimer {
		procSetTimer.Call(a.hwnd, textCaretTimerID, 530, 0)
		a.textTool.CaretTimer = true
	}
	a.requestTextRepaint(false)
}

func (a *App) stopTextCaretBlink() {
	if a.textTool.CaretTimer {
		procKillTimer.Call(a.hwnd, textCaretTimerID)
		a.textTool.CaretTimer = false
	}
	a.textTool.CaretVisible = false
}

func (a *App) focusTextInputAtCurrentCaret() {
	if !a.textTool.Active || a.textTool.EditHwnd == 0 {
		return
	}
	a.textTool.Editing = true
	procSetFocus.Call(a.textTool.EditHwnd)
	a.syncTextLayoutControl()
	a.resetTextCaretBlink()
}

type textGDIFontInfo struct {
	handle   uintptr
	height   int
	aveWidth int
}

func utf16RuneAt(units []uint16, index int) (rune, int) {
	if index < 0 || index >= len(units) {
		return 0, 0
	}
	u := units[index]
	if 0xD800 <= u && u <= 0xDBFF && index+1 < len(units) {
		v := units[index+1]
		if 0xDC00 <= v && v <= 0xDFFF {
			return utf16.DecodeRune(rune(u), rune(v)), 2
		}
	}
	return rune(u), 1
}

func isTextClusterContinuation(r rune) bool {
	return r == 0x200D || r == 0x200C || (0xFE00 <= r && r <= 0xFE0F) || (0xE0100 <= r && r <= 0xE01EF) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r)
}

// layoutTextGDI is the single source of truth for the raster glyphs, insertion
// boundaries, line wrapping, caret and selection. It deliberately avoids Rich
// Edit's device-DPI-dependent EM_POSFROMCHAR metrics. Every coordinate returned
// here is a native canvas pixel, exactly the coordinate system used by the
// preview bitmap and final committed pixels.
func (a *App) layoutTextGDI(hdc uintptr, text string, boxW, boxH int, draw bool) ([]TextBoundaryPos, map[int]int, []TextGlyphCluster, int) {
	units := utf16Units(text)
	a.normalizeTextStyles(len(units))
	positions := make([]TextBoundaryPos, len(units)+1)
	assigned := make([]bool, len(units)+1)
	for i := range positions {
		positions[i].Index = i
	}
	setBoundary := func(index, x, y int) {
		if index < 0 || index >= len(positions) {
			return
		}
		positions[index].X = x
		positions[index].Y = y
		assigned[index] = true
	}
	lineHeights := make(map[int]int)
	clusters := make([]TextGlyphCluster, 0, max(len(units), 1))
	fontCache := make(map[TextCharStyle]textGDIFontInfo)
	defer func() {
		for _, info := range fontCache {
			if info.handle != 0 {
				procDeleteObject.Call(info.handle)
			}
		}
	}()
	fontFor := func(style TextCharStyle) textGDIFontInfo {
		if info, ok := fontCache[style]; ok {
			return info
		}
		fontPixels := textPointSizeToCanvasPixels(style.FontSize)
		h := createGDIFont(style.FontName, fontPixels, style.Bold, style.Italic, style.Underline)
		info := textGDIFontInfo{handle: h, height: max(fontPixels+2, 2), aveWidth: max(fontPixels/2, 1)}
		if h != 0 && hdc != 0 {
			old, _, _ := procSelectObject.Call(hdc, h)
			var tm TEXTMETRICW
			if ok, _, _ := procGetTextMetricsW.Call(hdc, uintptr(unsafe.Pointer(&tm))); ok != 0 {
				info.height = max(int(tm.TmHeight+tm.TmExternalLeading), 2)
				info.aveWidth = max(int(tm.TmAveCharWidth), 1)
			}
			procSelectObject.Call(hdc, old)
		}
		fontCache[style] = info
		return info
	}
	measure := func(info textGDIFontInfo, cluster string) (int, int) {
		if cluster == "\t" {
			return max(info.aveWidth*4, 1), info.height
		}
		if hdc == 0 || info.handle == 0 {
			return max(info.aveWidth*max(len([]rune(cluster)), 1), 1), info.height
		}
		old, _, _ := procSelectObject.Call(hdc, info.handle)
		var sz SIZE
		cu := utf16Units(cluster)
		if len(cu) > 0 {
			procGetTextExtentPoint32W.Call(hdc, uintptr(unsafe.Pointer(&cu[0])), uintptr(len(cu)), uintptr(unsafe.Pointer(&sz)))
		}
		procSelectObject.Call(hdc, old)
		return max(int(sz.Cx), 1), max(info.height, int(sz.Cy))
	}
	drawCluster := func(info textGDIFontInfo, cluster string, x, y int) {
		if !draw || hdc == 0 || info.handle == 0 || cluster == "\t" || cluster == "" {
			return
		}
		cu := utf16Units(cluster)
		if len(cu) == 0 {
			return
		}
		old, _, _ := procSelectObject.Call(hdc, info.handle)
		procTextOutW.Call(hdc, uintptr(x), uintptr(y), uintptr(unsafe.Pointer(&cu[0])), uintptr(len(cu)))
		procSelectObject.Call(hdc, old)
	}
	defaultInfo := fontFor(a.currentTextStyle())
	x, y := 0, 0
	lineHeight := max(defaultInfo.height, 2)
	glyphCount := 0
	setBoundary(0, 0, 0)
	finishLine := func() {
		if lineHeight < 2 {
			lineHeight = max(defaultInfo.height, 2)
		}
		if old := lineHeights[y]; lineHeight > old {
			lineHeights[y] = lineHeight
		}
	}
	for i := 0; i < len(units); {
		// Normalize CRLF and lone CR/LF to one visual paragraph break while
		// preserving every UTF-16 selection boundary used by the input EDIT.
		if units[i] == '\r' || units[i] == '\n' {
			end := i + 1
			if units[i] == '\r' && end < len(units) && units[end] == '\n' {
				end++
			}
			setBoundary(i, x, y)
			clusters = append(clusters, TextGlyphCluster{Start: i, End: end, X: x, Y: y, W: 0, Break: true})
			finishLine()
			y += lineHeight
			x = 0
			lineHeight = max(defaultInfo.height, 2)
			for j := i + 1; j <= end; j++ {
				setBoundary(j, x, y)
			}
			i = end
			continue
		}
		_, n := utf16RuneAt(units, i)
		if n < 1 {
			n = 1
		}
		end := i + n
		// Keep combining marks, variation selectors and ZWJ sequences attached
		// to the preceding base so the measured and rendered cluster is identical.
		for end < len(units) {
			r, rn := utf16RuneAt(units, end)
			if rn < 1 || !isTextClusterContinuation(r) {
				break
			}
			end += rn
			if r == 0x200D && end < len(units) {
				_, rn2 := utf16RuneAt(units, end)
				if rn2 > 0 {
					end += rn2
				}
			}
		}
		style := a.textStyleAt(i)
		info := fontFor(style)
		cluster := string(utf16.Decode(units[i:end]))
		width, height := measure(info, cluster)
		if boxW > 0 && x > 0 && x+width > boxW {
			finishLine()
			y += lineHeight
			x = 0
			lineHeight = max(defaultInfo.height, 2)
		}
		lineHeight = max(lineHeight, height)
		setBoundary(i, x, y)
		for j := i + 1; j < end; j++ {
			setBoundary(j, x, y)
		}
		drawCluster(info, cluster, x, y)
		clusters = append(clusters, TextGlyphCluster{Start: i, End: end, X: x, Y: y, W: max(width, 1)})
		x += width
		setBoundary(end, x, y)
		glyphCount++
		i = end
	}
	finishLine()
	last := TextBoundaryPos{Index: 0, X: 0, Y: 0}
	for i := range positions {
		if assigned[i] {
			last = positions[i]
		} else {
			positions[i].X = last.X
			positions[i].Y = last.Y
		}
	}
	return positions, lineHeights, clusters, glyphCount
}

func (a *App) createTextMeasureDC() (uintptr, func()) {
	windowDC, _, _ := procGetDC.Call(a.hwnd)
	memDC, _, _ := procCreateCompatibleDC.Call(windowDC)
	if windowDC != 0 {
		procReleaseDC.Call(a.hwnd, windowDC)
	}
	return memDC, func() {
		if memDC != 0 {
			procDeleteDC.Call(memDC)
		}
	}
}

// textBoundaryPositions asks Rich Edit for every insertion boundary after the
// exact formatting rectangle and styles have been synchronized.  The result is
// cached until text, style, DPI, or text-box geometry changes.
func (a *App) textBoundaryPositions() []TextBoundaryPos {
	a.syncTextModelFromInput()
	units := utf16UnitLen(a.textTool.Text)
	if len(a.textTool.LayoutPositions) == units+1 && a.textTool.LayoutLineHeights != nil {
		return a.textTool.LayoutPositions
	}
	hdc, cleanup := a.createTextMeasureDC()
	defer cleanup()
	positions, lineHeights, clusters, _ := a.layoutTextGDI(hdc, a.textTool.Text, max(a.textTool.W, 1), max(a.textTool.H, 1), false)
	a.textTool.LayoutPositions = positions
	a.textTool.LayoutLineHeights = lineHeights
	a.textTool.LayoutClusters = clusters
	return positions
}

func uniqueTextLineYs(positions []TextBoundaryPos) []int {
	seen := make(map[int]bool, len(positions))
	ys := make([]int, 0, 8)
	for _, pos := range positions {
		if !seen[pos.Y] {
			seen[pos.Y] = true
			ys = append(ys, pos.Y)
		}
	}
	sort.Ints(ys)
	return ys
}

func nearestTextLineY(localY int, ys []int) int {
	if len(ys) == 0 {
		return 0
	}
	if len(ys) == 1 || localY <= (ys[0]+ys[1])/2 {
		return ys[0]
	}
	for i := 1; i < len(ys)-1; i++ {
		upper := (ys[i-1] + ys[i]) / 2
		lower := (ys[i] + ys[i+1]) / 2
		if localY >= upper && localY < lower {
			return ys[i]
		}
	}
	return ys[len(ys)-1]
}

func (a *App) textLineHeight(lineY int, positions []TextBoundaryPos) int {
	if h := a.textTool.LayoutLineHeights[lineY]; h > 0 {
		return h
	}
	ys := uniqueTextLineYs(positions)
	for i, y := range ys {
		if y != lineY {
			continue
		}
		if i+1 < len(ys) {
			return max(ys[i+1]-y, 2)
		}
		if i > 0 {
			return max(y-ys[i-1], 2)
		}
		break
	}
	style := a.currentTextStyle()
	start, _ := a.textSelection()
	if utf16UnitLen(a.textTool.Text) > 0 {
		style = a.textStyleAt(start)
	}
	return max(textPointSizeToCanvasPixels(style.FontSize)+4, 2)
}

// textBoundaryAtLocalPoint chooses the nearest insertion boundary on the
// visually nearest line.  This fixes the old EM_CHARFROMPOS behavior where a
// click far to the right of a short line could repeatedly return the same
// character instead of the line-end caret.
func (a *App) textBoundaryAtLocalPoint(localX, localY int) TextBoundaryPos {
	positions := a.textBoundaryPositions()
	lineY := nearestTextLineY(localY, uniqueTextLineYs(positions))
	line := make([]TextBoundaryPos, 0, len(positions))
	for _, pos := range positions {
		if pos.Y == lineY {
			line = append(line, pos)
		}
	}
	if len(line) == 0 {
		return positions[0]
	}
	sort.SliceStable(line, func(i, j int) bool {
		if line[i].X == line[j].X {
			return line[i].Index < line[j].Index
		}
		return line[i].X < line[j].X
	})
	best := line[0]
	bestDist := absInt(localX - best.X)
	for _, pos := range line[1:] {
		d := absInt(localX - pos.X)
		if d < bestDist || (d == bestDist && localX >= pos.X && pos.Index > best.Index) {
			best, bestDist = pos, d
		}
	}
	return best
}

func (a *App) textIndexAtScreenPoint(mx, my int) int {
	r := a.textScreenRect()
	localX := clamp((mx-int(r.Left))/max(a.zoom, 1), 0, max(a.textTool.W, 0))
	localY := clamp((my-int(r.Top))/max(a.zoom, 1), 0, max(a.textTool.H, 0))
	pos := a.textBoundaryAtLocalPoint(localX, localY)
	return clamp(pos.Index, 0, utf16UnitLen(a.textTool.Text))
}

func (a *App) focusTextInputAtScreenPoint(mx, my int) {
	if !a.textTool.Active || a.textTool.EditHwnd == 0 || a.textTool.LayoutHwnd == 0 {
		return
	}
	index := a.textIndexAtScreenPoint(mx, my)
	a.textTool.VerticalGoalValid = false
	sendMessage(a.textTool.EditHwnd, EM_SETSEL, uintptr(index), uintptr(index))
	a.textTool.SelectAnchor = index
	a.textTool.Editing = true
	procSetFocus.Call(a.textTool.EditHwnd)
	a.resetTextCaretBlink()
	a.statusText = fmt.Sprintf("텍스트 입력 위치: %d", index)
	r := a.textScreenRect()
	localX := (mx - int(r.Left)) / max(a.zoom, 1)
	localY := (my - int(r.Top)) / max(a.zoom, 1)
	pos := a.textBoundaryAtLocalPoint(localX, localY)
	logf("TEXT caret click index=%d local=%d,%d boundary=%d,%d layoutDPI=%d controlDPI=%d zoom=%d/%d", index, localX, localY, pos.X, pos.Y, a.textTool.LayoutDPI, a.textTool.LayoutControlDPI, a.textTool.LayoutZoomNum, a.textTool.LayoutZoomDen)
}

func (a *App) beginTextRangeSelection(mx, my int) {
	index := a.textIndexAtScreenPoint(mx, my)
	a.textTool.VerticalGoalValid = false
	a.textTool.SelectAnchor = index
	a.textTool.Selecting = true
	sendMessage(a.textTool.EditHwnd, EM_SETSEL, uintptr(index), uintptr(index))
	a.textTool.Editing = true
	procSetFocus.Call(a.textTool.EditHwnd)
	procSetCapture.Call(a.hwnd)
	a.resetTextCaretBlink()
	a.requestTextRepaint(false)
	r := a.textScreenRect()
	localX := (mx - int(r.Left)) / max(a.zoom, 1)
	localY := (my - int(r.Top)) / max(a.zoom, 1)
	pos := a.textBoundaryAtLocalPoint(localX, localY)
	logf("TEXT selection begin anchor=%d local=%d,%d boundary=%d,%d layoutDPI=%d controlDPI=%d zoom=%d/%d", index, localX, localY, pos.X, pos.Y, a.textTool.LayoutDPI, a.textTool.LayoutControlDPI, a.textTool.LayoutZoomNum, a.textTool.LayoutZoomDen)
}

func (a *App) updateTextRangeSelection(mx, my int) {
	if !a.textTool.Selecting {
		return
	}
	index := a.textIndexAtScreenPoint(mx, my)
	sendMessage(a.textTool.EditHwnd, EM_SETSEL, uintptr(a.textTool.SelectAnchor), uintptr(index))
	a.resetTextCaretBlink()
	a.requestTextRepaint(false)
}

func (a *App) endTextRangeSelection(mx, my int) {
	if !a.textTool.Selecting {
		return
	}
	a.updateTextRangeSelection(mx, my)
	a.textTool.Selecting = false
	procReleaseCapture.Call()
	start, end := a.textSelection()
	if start == end && a.textTool.InsertionStyleActive && start == a.textTool.InsertionCaret {
		// Clicking the exact same insertion boundary is merely returning focus.
		// Preserve the font/size/B/I/U explicitly assigned to this caret.
		logf("TEXT insertion style retained caret=%d after same-position click", start)
		a.syncTextToolbarControls()
		a.updateTextFontUI()
	} else {
		if start == end {
			a.clearTextInsertionStyle("caret moved")
		} else {
			a.clearTextInsertionStyle("text range selected")
		}
		a.adoptTextStyleFromSelection()
	}
	if start == end {
		a.statusText = fmt.Sprintf("텍스트 입력 위치: %d", start)
	} else {
		a.statusText = fmt.Sprintf("텍스트 선택: %d~%d. 글꼴·크기·B/I/U는 선택된 글자에만 적용됩니다.", min(start, end), max(start, end))
	}
	visualStart, visualEnd := start, end
	if visualStart > visualEnd {
		visualStart, visualEnd = visualEnd, visualStart
	}
	visualSpans := a.textSelectionClusterSpans(visualStart, visualEnd)
	logf("TEXT selection end range=%d..%d clusterSpans=%d", start, end, len(visualSpans))
	a.requestTextRepaint(true)
}

// isTextNativeNavigationKey identifies keys that the hidden plain EDIT can
// handle correctly by character boundary. Up/Down are handled separately from
// the visible Rich Edit layout because the input EDIT itself is intentionally
// only one device pixel large.
func isTextNativeNavigationKey(vk int) bool {
	switch vk {
	case VK_LEFT, VK_RIGHT, VK_HOME, VK_END:
		return true
	}
	return false
}

func (a *App) afterTextSelectionNavigation(beforeStart, beforeEnd int, reason string, preserveVerticalGoal bool) {
	start, end := a.textSelection()
	if start == beforeStart && end == beforeEnd {
		return
	}
	if !preserveVerticalGoal {
		a.textTool.VerticalGoalValid = false
	}
	if start == end {
		if a.textTool.InsertionStyleActive && start != a.textTool.InsertionCaret {
			a.clearTextInsertionStyle(reason)
		}
		a.textTool.SelectAnchor = start
	} else {
		a.clearTextInsertionStyle(reason)
		if a.textTool.SelectAnchor != start && a.textTool.SelectAnchor != end {
			a.textTool.SelectAnchor = start
		}
	}
	a.adoptTextStyleFromSelection()
	a.resetTextCaretBlink()
	if start == end {
		a.statusText = fmt.Sprintf("텍스트 입력 위치: %d", start)
	} else {
		a.statusText = fmt.Sprintf("텍스트 선택: %d~%d", min(start, end), max(start, end))
	}
	a.requestTextRepaint(false)
	a.requestStatusRepaint()
	logf("TEXT keyboard navigation reason=%q before=%d..%d after=%d..%d", reason, beforeStart, beforeEnd, start, end)
}

func (a *App) moveTextCaretVertical(vk int, extend bool) {
	if !a.textTool.Active || !a.textTool.Editing || a.textTool.EditHwnd == 0 || a.textTool.LayoutHwnd == 0 {
		return
	}
	positions := a.textBoundaryPositions()
	if len(positions) == 0 {
		return
	}
	start, end := a.textSelection()
	beforeStart, beforeEnd := start, end
	if start > end {
		start, end = end, start
	}
	direction := 1
	if vk == VK_UP {
		direction = -1
	}

	anchor, caret := start, end
	if start == end {
		anchor, caret = start, start
	} else if a.textTool.SelectAnchor == start {
		anchor, caret = start, end
	} else if a.textTool.SelectAnchor == end {
		anchor, caret = end, start
	} else if direction < 0 {
		anchor, caret = end, start
	} else {
		anchor, caret = start, end
	}
	if !extend && start != end {
		if direction < 0 {
			caret = start
		} else {
			caret = end
		}
		anchor = caret
	}

	current := positions[0]
	found := false
	for _, pos := range positions {
		if pos.Index == caret {
			current = pos
			found = true
			break
		}
	}
	if !found {
		bestDist := absInt(current.Index - caret)
		for _, pos := range positions[1:] {
			d := absInt(pos.Index - caret)
			if d < bestDist {
				current, bestDist = pos, d
			}
		}
	}
	if !a.textTool.VerticalGoalValid {
		a.textTool.VerticalGoalX = current.X
		a.textTool.VerticalGoalValid = true
	}

	lines := uniqueTextLineYs(positions)
	lineIndex := 0
	bestLineDist := int(^uint(0) >> 1)
	for i, y := range lines {
		d := absInt(y - current.Y)
		if d < bestLineDist {
			lineIndex, bestLineDist = i, d
		}
	}
	targetLine := clamp(lineIndex+direction, 0, len(lines)-1)
	target := current
	bestX := int(^uint(0) >> 1)
	for _, pos := range positions {
		if pos.Y != lines[targetLine] {
			continue
		}
		d := absInt(pos.X - a.textTool.VerticalGoalX)
		if d < bestX || (d == bestX && ((direction < 0 && pos.Index < target.Index) || (direction > 0 && pos.Index > target.Index))) {
			target, bestX = pos, d
		}
	}

	if extend {
		sendMessage(a.textTool.EditHwnd, EM_SETSEL, uintptr(anchor), uintptr(target.Index))
		a.textTool.SelectAnchor = anchor
	} else {
		sendMessage(a.textTool.EditHwnd, EM_SETSEL, uintptr(target.Index), uintptr(target.Index))
		a.textTool.SelectAnchor = target.Index
	}
	a.afterTextSelectionNavigation(beforeStart, beforeEnd, map[bool]string{true: "Shift+vertical caret move", false: "vertical caret move"}[extend], true)
}

func (a *App) selectAllText() {
	units := utf16UnitLen(a.textTool.Text)
	beforeStart, beforeEnd := a.textSelection()
	sendMessage(a.textTool.EditHwnd, EM_SETSEL, 0, uintptr(units))
	a.textTool.SelectAnchor = 0
	a.textTool.VerticalGoalValid = false
	a.clearTextInsertionStyle("Ctrl+A selected text range")
	a.afterTextSelectionNavigation(beforeStart, beforeEnd, "Ctrl+A", false)
	logf("TEXT shortcut Ctrl+A units=%d", units)
}

func (a *App) cutTextSelection() {
	start, end := a.textSelection()
	if start == end {
		return
	}
	a.clearTextInsertionStyle("Ctrl+X")
	a.textTool.VerticalGoalValid = false
	sendMessage(a.textTool.EditHwnd, WM_CUT, 0, 0)
	a.syncTextModelFromInput()
	a.syncTextLayoutControl()
	start, end = a.textSelection()
	a.textTool.SelectAnchor = start
	a.adoptTextStyleFromSelection()
	a.resetTextCaretBlink()
	a.requestTextRepaint(true)
	a.requestStatusRepaint()
	logf("TEXT shortcut Ctrl+X resultSelection=%d..%d", start, end)
}

func (a *App) pasteTextAtSelection() {
	beforeText := a.textTool.Text
	beforeStart, beforeEnd := a.textSelection()
	oldStyle := a.textTool.InsertionStyle
	oldActive := a.textTool.InsertionStyleActive
	oldCaret := a.textTool.InsertionCaret
	oldAnchor := a.textTool.InsertionAnchor
	oldSpanEnd := a.textTool.InsertionSpanEnd

	a.armTextInsertionRange(beforeStart, beforeEnd, "Ctrl+V")
	a.textTool.VerticalGoalValid = false
	sendMessage(a.textTool.EditHwnd, WM_PASTE, 0, 0)
	a.syncTextModelFromInput()
	if a.textTool.Text == beforeText {
		a.textTool.InsertionStyle = oldStyle
		a.textTool.InsertionStyleActive = oldActive
		a.textTool.InsertionCaret = oldCaret
		a.textTool.InsertionAnchor = oldAnchor
		a.textTool.InsertionSpanEnd = oldSpanEnd
		logf("TEXT shortcut Ctrl+V no text inserted")
		return
	}
	a.syncTextLayoutControl()
	start, end := a.textSelection()
	a.textTool.SelectAnchor = start
	a.resetTextCaretBlink()
	a.requestTextRepaint(true)
	a.requestStatusRepaint()
	logf("TEXT shortcut Ctrl+V beforeSelection=%d..%d afterSelection=%d..%d units=%d", beforeStart, beforeEnd, start, end, utf16UnitLen(a.textTool.Text))
}

// handleTextEditKeyDown is called from the message loop before the one-pixel
// input EDIT sees the key. It routes text-local clipboard shortcuts and uses
// the visible Rich Edit boundary cache for vertical movement.
func (a *App) handleTextEditKeyDown(vk int) bool {
	if !a.textTool.Active || !a.textTool.Editing || a.textTool.EditHwnd == 0 {
		return false
	}
	ctrl := keyDown(VK_CONTROL)
	shift := keyDown(VK_SHIFT)
	if ctrl {
		switch vk {
		case 'A':
			a.selectAllText()
			return true
		case 'C':
			start, end := a.textSelection()
			sendMessage(a.textTool.EditHwnd, WM_COPY, 0, 0)
			a.statusText = "선택한 텍스트를 복사했습니다."
			a.requestStatusRepaint()
			logf("TEXT shortcut Ctrl+C selection=%d..%d", start, end)
			return true
		case 'X':
			a.cutTextSelection()
			return true
		case 'V':
			a.pasteTextAtSelection()
			return true
		}
	}
	if vk == VK_UP || vk == VK_DOWN {
		a.moveTextCaretVertical(vk, shift)
		return true
	}
	return false
}

func (a *App) textSelectionClusterSpans(start, end int) []struct{ x1, x2, y int } {
	units := utf16UnitLen(a.textTool.Text)
	start = clamp(start, 0, units)
	end = clamp(end, 0, units)
	if start > end {
		start, end = end, start
	}
	if start >= end {
		return nil
	}
	positions := a.textBoundaryPositions()
	_ = positions
	clusters := a.textTool.LayoutClusters
	if len(clusters) == 0 {
		return nil
	}
	type span struct{ x1, x2, y int }
	byLine := make(map[int]span)
	lineOrder := make([]int, 0, 8)
	for _, cluster := range clusters {
		if cluster.End <= start || cluster.Start >= end {
			continue
		}
		x1 := cluster.X
		x2 := cluster.X + max(cluster.W, 1)
		if cluster.Break {
			// A paragraph break has no ink, but when it is selected show a narrow
			// inversion marker at the line end so every selected UTF-16 range has
			// a visible counterpart.
			x2 = x1 + 2
		}
		x1 = clamp(x1, 0, max(a.textTool.W, 0))
		x2 = clamp(x2, 0, max(a.textTool.W, 0))
		if x2 <= x1 {
			x2 = min(x1+1, max(a.textTool.W, x1+1))
		}
		old, ok := byLine[cluster.Y]
		if !ok {
			byLine[cluster.Y] = span{x1: x1, x2: x2, y: cluster.Y}
			lineOrder = append(lineOrder, cluster.Y)
		} else {
			old.x1 = min(old.x1, x1)
			old.x2 = max(old.x2, x2)
			byLine[cluster.Y] = old
		}
	}
	sort.Ints(lineOrder)
	out := make([]struct{ x1, x2, y int }, 0, len(lineOrder))
	for _, y := range lineOrder {
		sp := byLine[y]
		out = append(out, struct{ x1, x2, y int }{sp.x1, sp.x2, sp.y})
	}
	return out
}

func (a *App) paintTextSelection(hdc uintptr) {
	if !a.textTool.Active || a.textTool.LayoutHwnd == 0 {
		return
	}
	start, end := a.textSelection()
	if start > end {
		start, end = end, start
	}
	if start >= end {
		return
	}
	positions := a.textBoundaryPositions()
	spans := a.textSelectionClusterSpans(start, end)
	if len(spans) == 0 {
		return
	}
	r := a.textScreenRect()
	for _, sp := range spans {
		sx := int(r.Left) + sp.x1*a.zoom
		sy := int(r.Top) + sp.y*a.zoom
		sw := max((sp.x2-sp.x1)*a.zoom, 1)
		sh := max(a.textLineHeight(sp.y, positions)*a.zoom, 1)
		if sx < int(r.Left) {
			sw -= int(r.Left) - sx
			sx = int(r.Left)
		}
		if sy < int(r.Top) {
			sh -= int(r.Top) - sy
			sy = int(r.Top)
		}
		if sy+sh > int(r.Bottom) {
			sh = max(int(r.Bottom)-sy, 0)
		}
		if sx+sw > int(r.Right) {
			sw = max(int(r.Right)-sx, 0)
		}
		if sw > 0 && sh > 0 {
			// True color inversion keeps the selected pixels readable over any
			// foreground/background combination and remains a screen-only overlay.
			procPatBlt.Call(hdc, uintptr(sx), uintptr(sy), uintptr(sw), uintptr(sh), DSTINVERT)
		}
	}
}

func (a *App) paintTextCaret(hdc uintptr) {
	if !a.textTool.Active || !a.textTool.Editing || !a.textTool.CaretVisible || a.textTool.LayoutHwnd == 0 {
		return
	}
	focused, _, _ := procGetFocus.Call()
	if focused != a.textTool.EditHwnd {
		return
	}
	start := a.displayTextCaretIndex()
	if start < 0 {
		return
	}
	units := utf16UnitLen(a.textTool.Text)
	start = clamp(start, 0, units)
	positions := a.textBoundaryPositions()
	var pos TextBoundaryPos
	found := false
	for _, candidate := range positions {
		if candidate.Index == start {
			pos, found = candidate, true
			break
		}
	}
	if !found {
		return
	}
	r := a.textScreenRect()
	caretX := int(r.Left) + pos.X*a.zoom
	caretY := int(r.Top) + pos.Y*a.zoom
	caretH := max(a.textLineHeight(pos.Y, positions)*a.zoom, 2)
	caretH = min(caretH, max(int(r.Bottom)-caretY, 0))
	if caretH <= 0 || caretX < int(r.Left) || caretX >= int(r.Right) {
		return
	}
	// The editing caret is always a clearly visible two-device-pixel blinking
	// bar. Its thickness no longer changes with zoom and it is never committed
	// to the canvas bitmap.
	const caretW = 2
	procPatBlt.Call(hdc, uintptr(caretX), uintptr(caretY), uintptr(caretW), uintptr(caretH), DSTINVERT)
}

func (a *App) setTextOpaque(opaque bool) {
	a.textTool.Opaque = opaque
	mode := "투명 텍스트"
	if opaque {
		mode = "불투명 텍스트"
	}
	a.statusText = mode + "를 적용했습니다. 현재 텍스트 상자에 즉시 반영됩니다."
	logf("TEXT opacity opaque=%v", opaque)
	a.requestTextRepaint(true)
	a.requestStatusRepaint()
}

func (a *App) hitTextBoxAction(mx, my int) (mode int, handle int) {
	if !a.textTool.Active {
		return 0, -1
	}
	r := a.textScreenRect()
	if r.Right <= r.Left || r.Bottom <= r.Top {
		return 0, -1
	}
	hs := 4
	cx := (int(r.Left) + int(r.Right)) / 2
	cy := (int(r.Top) + int(r.Bottom)) / 2
	handles := []RECT{{r.Left - int32(hs), r.Top - int32(hs), r.Left + int32(hs+1), r.Top + int32(hs+1)}, {int32(cx - hs), r.Top - int32(hs), int32(cx + hs + 1), r.Top + int32(hs+1)}, {r.Right - int32(hs+1), r.Top - int32(hs), r.Right + int32(hs), r.Top + int32(hs+1)}, {r.Right - int32(hs+1), int32(cy - hs), r.Right + int32(hs), int32(cy + hs + 1)}, {r.Right - int32(hs+1), r.Bottom - int32(hs+1), r.Right + int32(hs), r.Bottom + int32(hs)}, {int32(cx - hs), r.Bottom - int32(hs+1), int32(cx + hs + 1), r.Bottom + int32(hs)}, {r.Left - int32(hs), r.Bottom - int32(hs+1), r.Left + int32(hs+1), r.Bottom + int32(hs)}, {r.Left - int32(hs), int32(cy - hs), r.Left + int32(hs+1), int32(cy + hs + 1)}}
	for i, hr := range handles {
		if pointInRect(mx, my, hr) {
			return 2, i
		}
	}
	if pointInRect(mx, my, r) {
		inner := RECT{r.Left + 5, r.Top + 5, r.Right - 5, r.Bottom - 5}
		if !pointInRect(mx, my, inner) {
			return 1, -1
		}
		return 3, -1
	}
	return 0, -1
}

func (a *App) beginTextBoxDrag(mode, handle, mx, my int) {
	a.textTool.DragMode = mode
	a.textTool.DragHandle = handle
	a.textTool.DragStartMX = mx
	a.textTool.DragStartMY = my
	a.textTool.OrigX = a.textTool.X
	a.textTool.OrigY = a.textTool.Y
	a.textTool.OrigW = a.textTool.W
	a.textTool.OrigH = a.textTool.H
	procSetCapture.Call(a.hwnd)
	if mode == 1 {
		a.statusText = "텍스트 상자 이동 중"
	} else {
		a.statusText = "텍스트 상자 크기 조절 중"
	}
}

func (a *App) updateTextBoxDrag(mx, my int) {
	if a.textTool.DragMode == 0 {
		return
	}
	dx := int(math.Round(float64(mx-a.textTool.DragStartMX) / float64(max(a.zoom, 1))))
	dy := int(math.Round(float64(my-a.textTool.DragStartMY) / float64(max(a.zoom, 1))))
	x, y, w, h := a.textTool.OrigX, a.textTool.OrigY, a.textTool.OrigW, a.textTool.OrigH
	if a.textTool.DragMode == 1 {
		x = clamp(a.textTool.OrigX+dx, 0, max(a.canvas.W-w, 0))
		y = clamp(a.textTool.OrigY+dy, 0, max(a.canvas.H-h, 0))
	} else {
		switch a.textTool.DragHandle {
		case 0:
			x += dx
			y += dy
			w -= dx
			h -= dy
		case 1:
			y += dy
			h -= dy
		case 2:
			y += dy
			w += dx
			h -= dy
		case 3:
			w += dx
		case 4:
			w += dx
			h += dy
		case 5:
			h += dy
		case 6:
			x += dx
			w -= dx
			h += dy
		case 7:
			x += dx
			w -= dx
		}
		minW, minH := 8, 8
		if w < minW {
			if a.textTool.DragHandle == 0 || a.textTool.DragHandle == 6 || a.textTool.DragHandle == 7 {
				x -= (minW - w)
			}
			w = minW
		}
		if h < minH {
			if a.textTool.DragHandle == 0 || a.textTool.DragHandle == 1 || a.textTool.DragHandle == 2 {
				y -= (minH - h)
			}
			h = minH
		}
		if x < 0 {
			w += x
			x = 0
		}
		if y < 0 {
			h += y
			y = 0
		}
		if x+w > a.canvas.W {
			w = a.canvas.W - x
		}
		if y+h > a.canvas.H {
			h = a.canvas.H - y
		}
		w = max(w, minW)
		h = max(h, minH)
	}
	oldRect := a.textVisualScreenRect()
	a.textTool.X, a.textTool.Y, a.textTool.W, a.textTool.H = x, y, w, h
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	a.updateTextUILayout(client)
	a.syncTextLayoutControl()
	a.resetTextCaretBlink()
	newRect := a.textVisualScreenRect()
	a.requestRepaintRect(unionRECT(oldRect, newRect), true)
}

func (a *App) endTextBoxDrag() {
	a.textTool.DragMode = 0
	a.textTool.DragHandle = -1
	a.textTool.Selecting = false
	procReleaseCapture.Call()
	a.statusText = fmt.Sprintf("텍스트 상자: %d × %d", a.textTool.W, a.textTool.H)
	a.focusTextInputAtCurrentCaret()
	logf("TEXT box drag end x=%d y=%d w=%d h=%d", a.textTool.X, a.textTool.Y, a.textTool.W, a.textTool.H)
}

func (a *App) paintLiveTextPreview(hdc uintptr, ox, oy int) {
	if !a.textTool.Active || a.textTool.W <= 0 || a.textTool.H <= 0 {
		return
	}
	text := a.currentTextValue()
	if text == "" {
		return
	}

	// Rasterize at the canvas's native pixel resolution with the exact same
	// code path used by final commit.  Only after the pixels are fixed do we
	// enlarge them with COLORONCOLOR, so zoomed previews show square canvas
	// pixels instead of a second, smoothly scaled vector-font rendering.
	preview := &Canvas{W: a.canvas.W, H: a.canvas.H, Pix: a.canvas.ClonePixels()}
	a.renderTextToTarget(preview, text, false)
	displayPix := preview.displayPixels(a.zoom)
	if len(displayPix) == 0 {
		return
	}
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(preview.W), BiHeight: -int32(preview.H), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	// The text preview changes only inside its own box. Blitting the entire
	// canvas for every IME composition notification was both expensive and the
	// visible source of the box-wide flash. Reuse the stable DIB-tile path and
	// transfer only the text rectangle.
	clip := a.textScreenRect()
	a.paintCanvasPixelsTiled(hdc, clip, ox, oy, displayPix, bmi)
}

func (a *App) paintTextBoxHandles(hdc uintptr) {
	if !a.textTool.Active {
		return
	}
	r := a.textScreenRect()
	hs := 3
	cx := (int(r.Left) + int(r.Right)) / 2
	cy := (int(r.Top) + int(r.Bottom)) / 2
	pts := [][2]int{{int(r.Left), int(r.Top)}, {cx, int(r.Top)}, {int(r.Right), int(r.Top)}, {int(r.Right), cy}, {int(r.Right), int(r.Bottom)}, {cx, int(r.Bottom)}, {int(r.Left), int(r.Bottom)}, {int(r.Left), cy}}
	for _, pt := range pts {
		rc := RECT{int32(pt[0] - hs), int32(pt[1] - hs), int32(pt[0] + hs + 1), int32(pt[1] + hs + 1)}
		fillRect(hdc, rc, rgb(255, 255, 255))
		ix := a.textTool.X + clamp((pt[0]-int(r.Left))/max(a.zoom, 1), 0, max(a.textTool.W-1, 0))
		iy := a.textTool.Y + clamp((pt[1]-int(r.Top))/max(a.zoom, 1), 0, max(a.textTool.H-1, 0))
		frame := rgb(0, 0, 0)
		if a.selectionGuideUsesWhiteAtPixel(ix, iy) {
			frame = rgb(255, 255, 255)
		}
		frameRect(hdc, rc, frame)
	}
}

func (a *App) renderTextToCanvas(text string) {
	a.renderTextToTarget(a.canvas, text, true)
}

func (a *App) renderTextToTarget(target *Canvas, text string, logRender bool) {
	if text == "" || a.textTool.W <= 0 || a.textTool.H <= 0 {
		return
	}
	x0 := clamp(a.textTool.X, 0, target.W)
	y0 := clamp(a.textTool.Y, 0, target.H)
	x1 := clamp(a.textTool.X+a.textTool.W, 0, target.W)
	y1 := clamp(a.textTool.Y+a.textTool.H, 0, target.H)
	boxW, boxH := x1-x0, y1-y0
	if boxW <= 0 || boxH <= 0 {
		return
	}

	// Use a DC compatible with the actual window.  Rich Edit therefore lays out
	// the hidden control and the raster target at the same DPI; point sizes are
	// converted so the resulting canvas pixel height remains DPI-independent.
	windowDC, _, _ := procGetDC.Call(a.hwnd)
	memDC, _, _ := procCreateCompatibleDC.Call(windowDC)
	if windowDC != 0 {
		procReleaseDC.Call(a.hwnd, windowDC)
	}
	if memDC == 0 {
		logf("TEXT render failed: CreateCompatibleDC")
		return
	}
	defer procDeleteDC.Call(memDC)
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(boxW), BiHeight: -int32(boxH), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	var bits unsafe.Pointer
	hbm, _, _ := procCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbm == 0 || bits == nil {
		logf("TEXT render failed: CreateDIBSection")
		return
	}
	defer procDeleteObject.Call(hbm)
	oldBmp, _, _ := procSelectObject.Call(memDC, hbm)
	defer procSelectObject.Call(memDC, oldBmp)

	localLen := boxW * boxH * 4
	buf := unsafe.Slice((*byte)(bits), localLen)
	original := make([]byte, localLen)
	for row := 0; row < boxH; row++ {
		src := ((y0+row)*target.W + x0) * 4
		dst := row * boxW * 4
		copy(original[dst:dst+boxW*4], target.Pix[src:src+boxW*4])
	}
	copy(buf, original)

	fontPixels := textPointSizeToCanvasPixels(a.textTool.FontSize)
	var font uintptr
	if !a.textTool.LayoutRich {
		font = createGDIFont(a.textTool.FontName, fontPixels, a.textTool.Bold, a.textTool.Italic, a.textTool.Underline)
		if font != 0 {
			oldFont, _, _ := procSelectObject.Call(memDC, font)
			defer func() { procSelectObject.Call(memDC, oldFont); procDeleteObject.Call(font) }()
		}
	}
	textColor := a.fg
	if isTransparentColor(textColor) {
		textColor = rgb(0, 0, 0)
	}
	procSetTextColor.Call(memDC, uintptr(textColor))
	opaqueBackground := a.textTool.Opaque && !isTransparentColor(a.bg)
	localRect := RECT{0, 0, int32(boxW), int32(boxH)}
	if opaqueBackground {
		procSetBkMode.Call(memDC, OPAQUE)
		procSetBkColor.Call(memDC, uintptr(a.bg))
		fillRect(memDC, localRect, a.bg)
	} else {
		procSetBkMode.Call(memDC, TRANSPARENT)
	}

	var result uintptr
	positions, lineHeights, clusters, glyphCount := a.layoutTextGDI(memDC, text, boxW, boxH, true)
	a.textTool.LayoutPositions = positions
	a.textTool.LayoutLineHeights = lineHeights
	a.textTool.LayoutClusters = clusters
	result = uintptr(glyphCount)

	// Preserve alpha from the source canvas and mark only pixels touched by the
	// text/background as opaque.  The raster is then copied back at the text-box
	// origin.  Rendering locally is what makes preview, commit, and hit testing
	// share an identical 0,0-based formatting rectangle.
	for i := 3; i < len(buf); i += 4 {
		buf[i] = original[i]
	}
	changed := 0
	for i := 0; i < len(buf); i += 4 {
		rgbChanged := buf[i] != original[i] || buf[i+1] != original[i+1] || buf[i+2] != original[i+2]
		if opaqueBackground || rgbChanged {
			if isTransparentColor(a.fg) && rgbChanged && !opaqueBackground {
				buf[i], buf[i+1], buf[i+2], buf[i+3] = 0, 0, 0, 0
			} else {
				buf[i+3] = 255
			}
			changed++
		}
	}
	for row := 0; row < boxH; row++ {
		src := row * boxW * 4
		dst := ((y0+row)*target.W + x0) * 4
		copy(target.Pix[dst:dst+boxW*4], buf[src:src+boxW*4])
	}
	if logRender {
		logf("TEXT render drawResult=%d changed=%d rect=%d,%d-%d,%d opaque=%v layout=GDI-native", result, changed, x0, y0, x1, y1, opaqueBackground)
	}
}

func (a *App) finishTextEditing(commit bool) {
	if !a.textTool.Active && !a.textTool.Editing {
		return
	}
	// Text creation and text editing share the generic drawing state. Always
	// clear it when an active box is finished so a later WM_MOUSEMOVE/WM_LBUTTONUP
	// from the same physical click cannot create another box accidentally.
	a.drawing = false
	a.preview = false
	a.textTool.Creating = false
	procReleaseCapture.Call()
	if a.textTool.IMEActive {
		a.syncTextIMEModelFromInput(true)
	} else {
		a.syncTextModelFromInput()
	}
	textVal := a.textTool.Text
	if commit && strings.TrimSpace(textVal) != "" {
		a.undo.Push(a.canvas)
		a.renderTextToCanvas(textVal)
		a.markDirty()
		a.recordHistory("텍스트")
	}
	a.stopTextCaretBlink()
	procKillTimer.Call(a.hwnd, textPreviewTimerID)
	a.textTool.Active = false
	a.textTool.Editing = false
	a.textTool.Creating = false
	a.textTool.DragMode = 0
	a.textTool.DragHandle = -1
	a.textTool.Selecting = false
	if a.textTool.EditHwnd != 0 {
		procShowWindow.Call(a.textTool.EditHwnd, SW_HIDE)
		procSetWindowTextW.Call(a.textTool.EditHwnd, uintptr(unsafe.Pointer(utf16Ptr(""))))
	}
	a.textTool.Text = ""
	a.textTool.Styles = nil
	a.clearTextIMEState("text box finished")
	a.clearTextInsertionStyle("text box finished")
	a.textTool.LayoutText = ""
	a.textTool.LayoutUnits = 0
	a.textTool.LayoutDPI = 0
	a.textTool.LayoutControlDPI = 0
	a.textTool.LayoutZoomNum = 0
	a.textTool.LayoutZoomDen = 0
	a.textTool.LayoutInputToRich = nil
	a.textTool.LayoutRichToInput = nil
	a.textTool.LayoutPositions = nil
	a.textTool.LayoutLineHeights = nil
	a.textTool.LayoutClusters = nil
	a.markTextLayoutDirty()
	if !a.textToolbarVisible() {
		a.hideTextControls()
	}
	if commit {
		a.statusText = "텍스트를 그림에 적용했습니다."
		logf("TEXT commit")
	} else {
		a.statusText = "텍스트 입력을 취소했습니다."
		logf("TEXT cancel")
	}
	invalidate(a.hwnd)
}

func (a *App) setBitmapView(enabled bool) {
	if a.bitmapView == enabled {
		return
	}
	// Bitmap view only hides the surrounding UI. It is still a fully editable
	// canvas view, so drawing and text-tool mouse semantics must remain identical
	// to the normal view. Do not clear tool state or install a read-only barrier.
	a.bitmapView = enabled
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))

	// The text toolbar consists of real child windows drawn above the buffered
	// parent. Hide/reposition them synchronously when entering bitmap view, then
	// restore them synchronously on exit. Merely invalidating the parent leaves
	// child-window ghosts and can keep the combo/button surfaces hidden afterward.
	if enabled {
		a.hideTextControls()
	}
	a.updateScrollbars()
	a.updateTextUILayout(client)
	if !enabled && a.tool == ToolText && a.showTextPanel {
		a.syncTextStyleButtons()
		a.refreshTextComboSurface()
		if a.textTool.Active && a.textTool.Editing {
			a.focusTextInputAtCurrentCaret()
		}
	}
	state := "종료"
	if enabled {
		state = "시작"
	}
	logf("BITMAP_VIEW %s textActive=%v textPanel=%v", state, a.textTool.Active, a.showTextPanel)
	invalidate(a.hwnd)
	procUpdateWindow.Call(a.hwnd)
}

func (a *App) applyTextSizeFromCombo(preferSelection bool) bool {
	value := ""
	if preferSelection {
		idx := int(sendMessage(a.textTool.SizeCombo, CB_GETCURSEL, 0, 0))
		if idx >= 0 {
			value = comboItemText(a.textTool.SizeCombo, idx)
		}
	}
	if value == "" {
		value = getWindowTextString(a.textTool.SizeCombo)
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 1 || n > 512 {
		return false
	}
	if a.textTool.FontSize != n {
		a.textTool.FontSize = n
		a.updateTextFontUI()
		logf("TEXT size point=%d canvasPixels=%d", n, textPointSizeToCanvasPixels(n))
	}
	a.statusText = fmt.Sprintf("텍스트 크기: %d pt", a.textTool.FontSize)
	return true
}

func (a *App) handleTextControlCommand(id, code int) bool {
	if id == ID_TEXT_LAYOUTBOX {
		return true
	}
	if id < ID_TEXT_EDITBOX || id > ID_TEXT_UNDERLINE {
		return false
	}
	a.ensureTextControls()
	if a.textTool.ControlSync {
		return true
	}
	returnFocus := false
	switch id {
	case ID_TEXT_EDITBOX:
		if code == EN_CHANGE && a.textTool.Active {
			a.syncTextModelFromInput()
			a.syncTextLayoutControl()
			a.textTool.CaretVisible = true
			if !a.textTool.CaretTimer {
				procSetTimer.Call(a.hwnd, textCaretTimerID, 530, 0)
				a.textTool.CaretTimer = true
			}
			a.scheduleTextInputRepaint()
		}
	case ID_TEXT_FONTCOMBO:
		if code == CBN_SELCHANGE {
			idx := int(sendMessage(a.textTool.FontCombo, CB_GETCURSEL, 0, 0))
			a.textTool.FontName = comboItemText(a.textTool.FontCombo, idx)
			a.updateTextFontUI()
			a.applyTextPropertyToSelection("font")
			a.statusText = fmt.Sprintf("텍스트 글꼴: %s", a.textTool.FontName)
			returnFocus = true
		}
		if code == CBN_CLOSEUP {
			a.refreshTextComboSurface()
		}
	case ID_TEXT_SIZECOMBO:
		if code == CBN_SELCHANGE {
			// Some Windows combo-box builds emit a second CBN_SELCHANGE carrying
			// the previous list index while the dropdown is closing. Ignore only
			// that short stale burst; a later deliberate click is still accepted.
			idx := int(sendMessage(a.textTool.SizeCombo, CB_GETCURSEL, 0, 0))
			nowMS := time.Now().UnixMilli()
			if a.textTool.LastSizeApplyMS != 0 && nowMS-a.textTool.LastSizeApplyMS < 220 && idx != a.textTool.LastSizeListIndex {
				logf("TEXT size stale notification ignored index=%d last=%d age=%dms", idx, a.textTool.LastSizeListIndex, nowMS-a.textTool.LastSizeApplyMS)
				return true
			}
			if a.applyTextSizeFromCombo(true) {
				a.textTool.LastSizeListIndex = idx
				a.textTool.LastSizeApplyMS = nowMS
				a.applyTextPropertyToSelection("size")
				returnFocus = true
			}
		}
		if code == CBN_CLOSEUP {
			a.refreshTextComboSurface()
		}
	case ID_TEXT_SCRIPTCOMBO:
		if code == CBN_SELCHANGE {
			a.textTool.Script = getWindowTextString(a.textTool.ScriptCombo)
			a.updateFontPreviewDropdownWidth()
			if a.textTool.FontCombo != 0 {
				procInvalidateRect.Call(a.textTool.FontCombo, 0, 1)
			}
			if a.textTool.Script == "없음" {
				a.statusText = "글꼴 목록 미리보기를 표시하지 않습니다."
			} else {
				a.statusText = fmt.Sprintf("글꼴 목록 미리보기: %s", a.textTool.Script)
			}
			a.refreshTextComboSurface()
			returnFocus = true
		}
		if code == CBN_CLOSEUP {
			a.refreshTextComboSurface()
		}
	case ID_TEXT_BOLD:
		if code == BN_CLICKED {
			a.textTool.Bold = !a.textTool.Bold
			a.updateTextFontUI()
			a.applyTextPropertyToSelection("bold")
			returnFocus = true
		}
	case ID_TEXT_ITALIC:
		if code == BN_CLICKED {
			a.textTool.Italic = !a.textTool.Italic
			a.updateTextFontUI()
			a.applyTextPropertyToSelection("italic")
			returnFocus = true
		}
	case ID_TEXT_UNDERLINE:
		if code == BN_CLICKED {
			a.textTool.Underline = !a.textTool.Underline
			a.updateTextFontUI()
			a.applyTextPropertyToSelection("underline")
			returnFocus = true
		}
	}
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	a.updateTextUILayout(client)
	if returnFocus {
		a.focusTextInputAtCurrentCaret()
	}
	if id == ID_TEXT_EDITBOX && code == EN_CHANGE {
		// scheduleTextInputRepaint() owns the coalesced preview update.
	} else {
		a.requestTextRepaint(true)
		var panelClient RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&panelClient)))
		panel := a.textPanelRect(panelClient)
		invalidateArea(a.hwnd, &panel)
	}
	a.requestStatusRepaint()
	return true
}

func (a *App) paintTextOverlay(hdc uintptr, ox, oy int) {
	guideW := 1
	if a.zoom > 1 {
		guideW = 2
	}
	if a.textTool.Active || (a.tool == ToolText && a.drawing && a.preview) {
		x, y, w, h := a.textTool.X, a.textTool.Y, a.textTool.W, a.textTool.H
		if a.tool == ToolText && a.drawing && a.preview {
			x, y, w, h = normalizeSelectionRect(a.startX, a.startY, a.curX, a.curY)
		}
		if w > 0 && h > 0 {
			if a.textTool.Active {
				a.paintLiveTextPreview(hdc, ox, oy)
			}
			a.drawAdaptiveSelectionRegionOutline(hdc, ox, oy, a.zoom, x, y, w, h, nil, guideW, false)
			if a.textTool.Active {
				a.paintTextSelection(hdc)
				a.paintTextBoxHandles(hdc)
				a.paintTextCaret(hdc)
			}
		}
	}
}

func cloneClipboardImage(src ClipboardImage) ClipboardImage {
	out := ClipboardImage{W: src.W, H: src.H}
	if len(src.Pix) > 0 {
		out.Pix = append([]byte(nil), src.Pix...)
	}
	if len(src.Mask) > 0 {
		out.Mask = append([]bool(nil), src.Mask...)
	}
	return out
}

func validClipboardImage(img ClipboardImage) bool {
	if img.W <= 0 || img.H <= 0 || img.W > 32768 || img.H > 32768 {
		return false
	}
	pixels := int64(img.W) * int64(img.H)
	return pixels <= 40000000 && int64(len(img.Pix)) == pixels*4
}

func clipboardImageHash(img ClipboardImage) uint32 {
	h := crc32.NewIEEE()
	var dims [8]byte
	binary.LittleEndian.PutUint32(dims[0:4], uint32(img.W))
	binary.LittleEndian.PutUint32(dims[4:8], uint32(img.H))
	_, _ = h.Write(dims[:])
	_, _ = h.Write(img.Pix)
	return h.Sum32()
}

func (a *App) addClipboardHistory(img ClipboardImage, source string) bool {
	if !validClipboardImage(img) {
		logf("CLIPBOARD history reject invalid source=%q w=%d h=%d pix=%d", source, img.W, img.H, len(img.Pix))
		return false
	}
	hash := clipboardImageHash(img)
	for i := range a.clipboardHistory {
		e := &a.clipboardHistory[i]
		if e.Hash == hash && e.Image.W == img.W && e.Image.H == img.H && bytes.Equal(e.Image.Pix, img.Pix) {
			e.Source = source
			e.Added = time.Now()
			pinned := e.Pinned
			if !pinned && i > 0 {
				copyEntry := *e
				a.clipboardHistory = append(a.clipboardHistory[:i], a.clipboardHistory[i+1:]...)
				a.clipboardHistory = append([]ClipboardEntry{copyEntry}, a.clipboardHistory...)
				a.clipboardSelected = 0
			} else {
				a.clipboardSelected = i
			}
			a.clipboard = a.clipboardHistory[a.clipboardSelected].Image
			a.ensureClipboardSelectionVisible()
			logf("CLIPBOARD history dedup index=%d pinned=%v source=%q hash=%08x", a.clipboardSelected, pinned, source, hash)
			invalidate(a.hwnd)
			return true
		}
	}
	if len(a.clipboardHistory) >= clipboardHistoryCapacity {
		allPinned := true
		for _, e := range a.clipboardHistory {
			if !e.Pinned {
				allPinned = false
				break
			}
		}
		if allPinned {
			a.statusText = "클립보드 10칸이 모두 고정되어 새 이미지를 기록하지 못했습니다. 고정핀 하나를 해제하십시오."
			logf("CLIPBOARD history full all-pinned source=%q", source)
			invalidate(a.hwnd)
			return false
		}
	}
	entry := ClipboardEntry{Image: cloneClipboardImage(img), Hash: hash, Source: source, Added: time.Now()}
	a.clipboardHistory = append([]ClipboardEntry{entry}, a.clipboardHistory...)
	for len(a.clipboardHistory) > clipboardHistoryCapacity {
		remove := -1
		for i := len(a.clipboardHistory) - 1; i >= 0; i-- {
			if !a.clipboardHistory[i].Pinned {
				remove = i
				break
			}
		}
		if remove < 0 {
			break
		}
		a.clipboardHistory = append(a.clipboardHistory[:remove], a.clipboardHistory[remove+1:]...)
	}
	a.clipboardSelected = 0
	a.clipboardScroll = 0
	a.clipboard = a.clipboardHistory[0].Image
	a.statusText = fmt.Sprintf("클립보드 이미지 기록: %d × %d (%s)", img.W, img.H, source)
	logf("CLIPBOARD history add count=%d source=%q w=%d h=%d hash=%08x", len(a.clipboardHistory), source, img.W, img.H, hash)
	invalidate(a.hwnd)
	return true
}

func (a *App) toggleClipboardPin(index int) {
	if index < 0 || index >= len(a.clipboardHistory) {
		return
	}
	a.clipboardHistory[index].Pinned = !a.clipboardHistory[index].Pinned
	a.clipboardSelected = index
	a.clipboard = a.clipboardHistory[index].Image
	a.ensureClipboardSelectionVisible()
	state := "해제"
	if a.clipboardHistory[index].Pinned {
		state = "고정"
	}
	a.statusText = fmt.Sprintf("클립보드 %d번을 %s했습니다.", index+1, state)
	logf("CLIPBOARD pin index=%d pinned=%v hash=%08x", index, a.clipboardHistory[index].Pinned, a.clipboardHistory[index].Hash)
	a.savePinnedClipboardHistory()
	invalidate(a.hwnd)
}

func openClipboardRetry(hwnd uintptr) bool {
	for i := 0; i < 20; i++ {
		r, _, _ := procOpenClipboard.Call(hwnd)
		if r != 0 {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func dibHandleToClipboardImage(handle uintptr) (ClipboardImage, error) {
	if handle == 0 {
		return ClipboardImage{}, errors.New("빈 DIB 핸들")
	}
	sz, _, _ := procGlobalSize.Call(handle)
	if sz < 40 {
		return ClipboardImage{}, errors.New("DIB 데이터가 너무 작습니다")
	}
	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		return ClipboardImage{}, errors.New("DIB 메모리를 잠글 수 없습니다")
	}
	defer procGlobalUnlock.Call(handle)
	data := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), int(sz))
	hdrSize := int(binary.LittleEndian.Uint32(data[0:4]))
	if hdrSize < 40 || hdrSize > len(data) {
		return ClipboardImage{}, fmt.Errorf("지원하지 않는 DIB 헤더 크기: %d", hdrSize)
	}
	w := int(int32(binary.LittleEndian.Uint32(data[4:8])))
	hRaw := int32(binary.LittleEndian.Uint32(data[8:12]))
	planes := binary.LittleEndian.Uint16(data[12:14])
	bpp := int(binary.LittleEndian.Uint16(data[14:16]))
	compression := binary.LittleEndian.Uint32(data[16:20])
	clrUsed := binary.LittleEndian.Uint32(data[32:36])
	if w <= 0 || hRaw == 0 || planes != 1 || w > 32768 || hRaw > 32768 || hRaw < -32768 {
		return ClipboardImage{}, fmt.Errorf("잘못된 DIB 크기: %d x %d", w, hRaw)
	}
	h := int(hRaw)
	topDown := h < 0
	if h < 0 {
		h = -h
	}
	if int64(w)*int64(h) > 40000000 {
		return ClipboardImage{}, fmt.Errorf("클립보드 이미지가 너무 큽니다: %d x %d", w, h)
	}
	if compression != BI_RGB && compression != BI_BITFIELDS {
		return ClipboardImage{}, fmt.Errorf("지원하지 않는 DIB 압축 방식: %d", compression)
	}
	if bpp != 32 && bpp != 24 && bpp != 16 && bpp != 8 {
		return ClipboardImage{}, fmt.Errorf("지원하지 않는 DIB 색 깊이: %d", bpp)
	}
	offset := hdrSize
	var masks [3]uint32
	if compression == BI_BITFIELDS {
		if hdrSize >= 52 {
			masks[0] = binary.LittleEndian.Uint32(data[40:44])
			masks[1] = binary.LittleEndian.Uint32(data[44:48])
			masks[2] = binary.LittleEndian.Uint32(data[48:52])
		} else {
			if offset+12 > len(data) {
				return ClipboardImage{}, errors.New("DIB 비트필드 마스크가 잘렸습니다")
			}
			masks[0] = binary.LittleEndian.Uint32(data[offset : offset+4])
			masks[1] = binary.LittleEndian.Uint32(data[offset+4 : offset+8])
			masks[2] = binary.LittleEndian.Uint32(data[offset+8 : offset+12])
			offset += 12
		}
	}
	paletteCount := 0
	if bpp <= 8 {
		paletteCount = int(clrUsed)
		if paletteCount == 0 {
			paletteCount = 1 << bpp
		}
		if offset+paletteCount*4 > len(data) {
			return ClipboardImage{}, errors.New("DIB 색상표가 잘렸습니다")
		}
	}
	paletteOffset := offset
	offset += paletteCount * 4
	stride := ((w*bpp + 31) / 32) * 4
	if stride <= 0 || offset+stride*h > len(data) {
		return ClipboardImage{}, fmt.Errorf("DIB 픽셀 데이터가 잘렸습니다: need=%d have=%d", offset+stride*h, len(data))
	}
	out := ClipboardImage{W: w, H: h, Pix: make([]byte, w*h*4)}
	extractMask := func(v, mask uint32) byte {
		if mask == 0 {
			return 0
		}
		shift := 0
		for ((mask >> shift) & 1) == 0 {
			shift++
		}
		maxv := mask >> shift
		value := (v & mask) >> shift
		if maxv == 0 {
			return 0
		}
		return byte((value*255 + maxv/2) / maxv)
	}
	for y := 0; y < h; y++ {
		sy := y
		if !topDown {
			sy = h - 1 - y
		}
		row := data[offset+sy*stride : offset+(sy+1)*stride]
		for x := 0; x < w; x++ {
			di := (y*w + x) * 4
			switch bpp {
			case 32:
				if compression == BI_BITFIELDS && masks[0] != 0 {
					v := binary.LittleEndian.Uint32(row[x*4 : x*4+4])
					out.Pix[di+2] = extractMask(v, masks[0])
					out.Pix[di+1] = extractMask(v, masks[1])
					out.Pix[di+0] = extractMask(v, masks[2])
				} else {
					out.Pix[di+0] = row[x*4+0]
					out.Pix[di+1] = row[x*4+1]
					out.Pix[di+2] = row[x*4+2]
				}
			case 24:
				out.Pix[di+0] = row[x*3+0]
				out.Pix[di+1] = row[x*3+1]
				out.Pix[di+2] = row[x*3+2]
			case 16:
				v := uint32(binary.LittleEndian.Uint16(row[x*2 : x*2+2]))
				if compression == BI_BITFIELDS && masks[0] != 0 {
					out.Pix[di+2] = extractMask(v, masks[0])
					out.Pix[di+1] = extractMask(v, masks[1])
					out.Pix[di+0] = extractMask(v, masks[2])
				} else {
					out.Pix[di+2] = byte(((v >> 10) & 0x1f) * 255 / 31)
					out.Pix[di+1] = byte(((v >> 5) & 0x1f) * 255 / 31)
					out.Pix[di+0] = byte((v & 0x1f) * 255 / 31)
				}
			case 8:
				idx := int(row[x])
				if idx >= paletteCount {
					idx = 0
				}
				pi := paletteOffset + idx*4
				out.Pix[di+0] = data[pi+0]
				out.Pix[di+1] = data[pi+1]
				out.Pix[di+2] = data[pi+2]
			}
			out.Pix[di+3] = 255
		}
	}
	return out, nil
}

func bitmapHandleToClipboardImage(handle uintptr) (ClipboardImage, error) {
	if handle == 0 {
		return ClipboardImage{}, errors.New("빈 비트맵 핸들")
	}
	var bm BITMAP
	r, _, _ := procGetObjectW.Call(handle, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
	if r == 0 || bm.BmWidth <= 0 || bm.BmHeight <= 0 {
		return ClipboardImage{}, errors.New("클립보드 비트맵 정보를 읽을 수 없습니다")
	}
	w, h := int(bm.BmWidth), int(bm.BmHeight)
	if int64(w)*int64(h) > 40000000 {
		return ClipboardImage{}, fmt.Errorf("클립보드 비트맵이 너무 큽니다: %d x %d", w, h)
	}
	out := ClipboardImage{W: w, H: h, Pix: make([]byte, w*h*4)}
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(w), BiHeight: -int32(h), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		return ClipboardImage{}, errors.New("화면 DC를 얻을 수 없습니다")
	}
	defer procReleaseDC.Call(0, hdc)
	r, _, err := procGetDIBits.Call(hdc, handle, 0, uintptr(h), uintptr(unsafe.Pointer(&out.Pix[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS)
	if r == 0 {
		return ClipboardImage{}, fmt.Errorf("클립보드 비트맵 픽셀을 읽지 못했습니다: %v", err)
	}
	for i := 3; i < len(out.Pix); i += 4 {
		out.Pix[i] = 255
	}
	return out, nil
}

func clipboardGlobalBytes(handle uintptr) ([]byte, error) {
	if handle == 0 {
		return nil, errors.New("빈 클립보드 메모리 핸들")
	}
	sz, _, _ := procGlobalSize.Call(handle)
	if sz == 0 || sz > 512*1024*1024 {
		return nil, fmt.Errorf("잘못된 클립보드 메모리 크기: %d", sz)
	}
	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		return nil, errors.New("클립보드 메모리를 잠글 수 없습니다")
	}
	defer procGlobalUnlock.Call(handle)
	src := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), int(sz))
	out := make([]byte, len(src))
	copy(out, src)
	return out, nil
}

func imageToClipboardImage(img image.Image) ClipboardImage {
	if img == nil {
		return ClipboardImage{}
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || int64(w)*int64(h) > 40000000 {
		return ClipboardImage{}
	}
	out := ClipboardImage{W: w, H: h, Pix: make([]byte, w*h*4)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			rr, gg, bb, aa := rgba16ToStraight8(r, g, bl, a)
			i := (y*w + x) * 4
			out.Pix[i+0] = bb
			out.Pix[i+1] = gg
			out.Pix[i+2] = rr
			out.Pix[i+3] = aa
		}
	}
	return out
}

func canvasToClipboardImage(c *Canvas) ClipboardImage {
	if c == nil || c.W <= 0 || c.H <= 0 || len(c.Pix) != c.W*c.H*4 {
		return ClipboardImage{}
	}
	return ClipboardImage{W: c.W, H: c.H, Pix: append([]byte(nil), c.Pix...)}
}

func clipboardFileHandleToImage(handle uintptr) (ClipboardImage, string, error) {
	paths := queryDroppedFiles(handle)
	if len(paths) == 0 {
		return ClipboardImage{}, "", errors.New("클립보드 파일 목록이 비어 있습니다")
	}
	var lastErr error
	for _, path := range paths {
		if !isSupportedDroppedImage(path) {
			continue
		}
		cv, err := decodeImageFile(path)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", filepath.Base(path), err)
			continue
		}
		img := canvasToClipboardImage(cv)
		if validClipboardImage(img) {
			return img, path, nil
		}
		lastErr = fmt.Errorf("%s: 이미지 크기가 올바르지 않습니다", filepath.Base(path))
	}
	if lastErr != nil {
		return ClipboardImage{}, "", lastErr
	}
	return ClipboardImage{}, "", errors.New("지원되는 그림 파일이 없습니다")
}

func pngHandleToClipboardImage(handle uintptr) (ClipboardImage, error) {
	data, err := clipboardGlobalBytes(handle)
	if err != nil {
		return ClipboardImage{}, err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return ClipboardImage{}, fmt.Errorf("PNG 클립보드 해석 실패: %w", err)
	}
	out := imageToClipboardImage(img)
	if !validClipboardImage(out) {
		return ClipboardImage{}, errors.New("PNG 클립보드 이미지 크기가 올바르지 않습니다")
	}
	return out, nil
}

func clipboardFormatSummaryLocked() string {
	names := map[uintptr]string{
		CF_BITMAP: "CF_BITMAP",
		CF_DIB:    "CF_DIB",
		CF_DIBV5:  "CF_DIBV5",
		CF_HDROP:  "CF_HDROP",
	}
	var out []string
	var current uintptr
	for len(out) < 32 {
		next, _, _ := procEnumClipboardFormats.Call(current)
		if next == 0 {
			break
		}
		current = next
		name := names[next]
		if name == "" {
			buf := make([]uint16, 128)
			n, _, _ := procGetClipboardFormatNameW.Call(next, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			if n > 0 {
				name = syscall.UTF16ToString(buf[:int(n)])
			}
		}
		if name == "" {
			name = fmt.Sprintf("format-%d", next)
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		return "(비어 있음)"
	}
	return strings.Join(out, ", ")
}

func (a *App) readSystemClipboard() (ClipboardImage, string, error) {
	if !openClipboardRetry(a.hwnd) {
		return ClipboardImage{}, "", errors.New("Windows 클립보드를 열 수 없습니다")
	}
	defer procCloseClipboard.Call()
	formats := make([]struct {
		id   uintptr
		name string
		kind string
	}, 0, 5)
	if clipboardPNGFormat != 0 {
		formats = append(formats, struct {
			id         uintptr
			name, kind string
		}{clipboardPNGFormat, "Windows 클립보드 PNG", "png"})
	}
	if clipboardImagePNGFormat != 0 && clipboardImagePNGFormat != clipboardPNGFormat {
		formats = append(formats, struct {
			id         uintptr
			name, kind string
		}{clipboardImagePNGFormat, "Windows 클립보드 image/png", "png"})
	}
	formats = append(formats,
		struct {
			id         uintptr
			name, kind string
		}{CF_HDROP, "Windows 탐색기 그림 파일", "hdrop"},
		struct {
			id         uintptr
			name, kind string
		}{CF_DIBV5, "Windows 클립보드 DIBV5", "dib"},
		struct {
			id         uintptr
			name, kind string
		}{CF_DIB, "Windows 클립보드 DIB", "dib"},
		struct {
			id         uintptr
			name, kind string
		}{CF_BITMAP, "Windows 클립보드 비트맵", "bitmap"},
	)
	var lastErr error
	for _, f := range formats {
		available, _, _ := procIsClipboardFormatAvailable.Call(f.id)
		if available == 0 {
			continue
		}
		h, _, _ := procGetClipboardData.Call(f.id)
		if h == 0 {
			lastErr = fmt.Errorf("%s 데이터 핸들을 얻지 못했습니다", f.name)
			continue
		}
		var img ClipboardImage
		var err error
		sourceName := f.name
		switch f.kind {
		case "png":
			img, err = pngHandleToClipboardImage(h)
		case "bitmap":
			img, err = bitmapHandleToClipboardImage(h)
		case "hdrop":
			var path string
			img, path, err = clipboardFileHandleToImage(h)
			if err == nil {
				sourceName = "Windows 탐색기 파일: " + filepath.Base(path)
			}
		default:
			img, err = dibHandleToClipboardImage(h)
		}
		if err == nil && validClipboardImage(img) {
			logf("CLIPBOARD decode success format=%q source=%q w=%d h=%d", f.name, sourceName, img.W, img.H)
			return img, sourceName, nil
		}
		lastErr = fmt.Errorf("%s: %w", f.name, err)
		logf("CLIPBOARD decode failed format=%q err=%v", f.name, err)
	}
	summary := clipboardFormatSummaryLocked()
	if lastErr != nil {
		return ClipboardImage{}, "", fmt.Errorf("%v; 사용 가능한 형식: %s", lastErr, summary)
	}
	return ClipboardImage{}, "", fmt.Errorf("클립보드에 지원되는 이미지가 없습니다; 사용 가능한 형식: %s", summary)
}

func (a *App) captureSystemClipboard(reason string) bool {
	seq, _, _ := procGetClipboardSequenceNumber.Call()
	img, source, err := a.readSystemClipboard()
	if err != nil {
		logf("CLIPBOARD capture reason=%q seq=%d skipped: %v", reason, seq, err)
		return false
	}
	a.lastClipboardSeq = uint32(seq)
	a.clipboard = img // current Windows clipboard remains pasteable even when all six history slots are pinned
	ok := a.addClipboardHistory(img, source)
	if !ok {
		a.clipboard = img
	}
	logf("CLIPBOARD capture reason=%q seq=%d source=%q ok=%v", reason, seq, source, ok)
	return true
}

func (a *App) writeSystemClipboard(img ClipboardImage) error {
	if !validClipboardImage(img) {
		return errors.New("복사할 이미지가 올바르지 않습니다")
	}

	// A219: publish an alpha-preserving PNG as the preferred clipboard form,
	// plus an opaque-white CF_DIB fallback for legacy programs that ignore the
	// fourth byte of 32-bit BI_RGB pixels.  The previous general Ctrl+C path
	// published only raw CF_DIB; dotPaint then read it back with alpha forced to
	// 255, turning transparent RGB(0,0,0) pixels into solid black.
	dibData, err := buildOpaqueClipboardDIB(img.W, img.H, img.Pix)
	if err != nil {
		return fmt.Errorf("호환 비트맵 생성 실패: %w", err)
	}
	pngData, pngErr := buildTransparentClipboardPNG(img.W, img.H, img.Pix)

	transparentPixels := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] < 255 {
			transparentPixels++
		}
	}
	hasTransparency := transparentPixels > 0
	if hasTransparency && pngErr != nil {
		return fmt.Errorf("투명 PNG 생성 실패: %w", pngErr)
	}

	type clipboardPayload struct {
		format uintptr
		name   string
		data   []byte
		handle uintptr
		must   bool
		png    bool
	}
	items := []clipboardPayload{{format: CF_DIB, name: "CF_DIB-white", data: dibData, must: true}}
	if pngErr == nil && len(pngData) > 0 {
		if clipboardPNGFormat != 0 {
			items = append(items, clipboardPayload{format: clipboardPNGFormat, name: "PNG", data: pngData, png: true})
		}
		if clipboardImagePNGFormat != 0 && clipboardImagePNGFormat != clipboardPNGFormat {
			items = append(items, clipboardPayload{format: clipboardImagePNGFormat, name: "image/png", data: pngData, png: true})
		}
	}
	if hasTransparency {
		pngCandidates := 0
		for i := range items {
			if items[i].png {
				pngCandidates++
			}
		}
		if pngCandidates == 0 {
			return errors.New("투명 이미지를 보존할 PNG 클립보드 형식을 등록하지 못했습니다")
		}
	}

	allocBlock := func(data []byte) (uintptr, error) {
		if len(data) == 0 {
			return 0, errors.New("빈 클립보드 데이터입니다")
		}
		hMem, _, allocErr := procGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(len(data)))
		if hMem == 0 {
			return 0, fmt.Errorf("클립보드 메모리를 할당하지 못했습니다: %v", allocErr)
		}
		ptr, _, _ := procGlobalLock.Call(hMem)
		if ptr == 0 {
			procGlobalFree.Call(hMem)
			return 0, errors.New("클립보드 메모리를 잠글 수 없습니다")
		}
		copy(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), len(data)), data)
		procGlobalUnlock.Call(hMem)
		return hMem, nil
	}

	pngAllocated := 0
	for i := range items {
		h, allocErr := allocBlock(items[i].data)
		if allocErr != nil {
			if items[i].must {
				for j := 0; j < i; j++ {
					if items[j].handle != 0 {
						procGlobalFree.Call(items[j].handle)
						items[j].handle = 0
					}
				}
				return allocErr
			}
			logf("CLIPBOARD optional allocation failed format=%q err=%v", items[i].name, allocErr)
			continue
		}
		items[i].handle = h
		if items[i].png {
			pngAllocated++
		}
	}
	if hasTransparency && pngAllocated == 0 {
		for i := range items {
			if items[i].handle != 0 {
				procGlobalFree.Call(items[i].handle)
				items[i].handle = 0
			}
		}
		return errors.New("투명 PNG 클립보드 메모리를 할당하지 못했습니다")
	}
	defer func() {
		for i := range items {
			if items[i].handle != 0 {
				procGlobalFree.Call(items[i].handle)
			}
		}
	}()

	if !openClipboardRetry(a.hwnd) {
		return errors.New("Windows 클립보드를 열 수 없습니다")
	}
	defer procCloseClipboard.Call()
	if r, _, _ := procEmptyClipboard.Call(); r == 0 {
		return errors.New("Windows 클립보드를 비울 수 없습니다")
	}

	pngPublished := 0
	publishedNames := make([]string, 0, len(items))
	for i := range items {
		if items[i].handle == 0 {
			continue
		}
		r, _, setErr := procSetClipboardData.Call(items[i].format, items[i].handle)
		if r == 0 {
			if items[i].must {
				return fmt.Errorf("Windows 클립보드에 %s 이미지를 넣지 못했습니다: %v", items[i].name, setErr)
			}
			logf("CLIPBOARD optional publish failed format=%q err=%v", items[i].name, setErr)
			continue
		}
		items[i].handle = 0 // ownership transferred to the clipboard
		publishedNames = append(publishedNames, items[i].name)
		if items[i].png {
			pngPublished++
		}
	}
	if hasTransparency && pngPublished == 0 {
		return errors.New("Windows 클립보드에 투명 PNG 형식을 넣지 못했습니다")
	}

	seq, _, _ := procGetClipboardSequenceNumber.Call()
	a.lastClipboardSeq = uint32(seq)
	logf("CLIPBOARD write-system w=%d h=%d transparent_pixels=%d formats=%q seq=%d", img.W, img.H, transparentPixels, strings.Join(publishedNames, ","), seq)
	return nil
}

func (a *App) pasteClipboardImageAt(cb ClipboardImage, source string, x, y int) bool {
	if !validClipboardImage(cb) {
		return false
	}
	// DejaVu IMG has opaque stored pixels but clipboard images may have alpha.
	// Do NOT turn alpha=0 into an opaque palette entry (usually black index 0).
	// Leave the source unchanged; quantize when compositing against BasePix, so
	// transparency and semi-transparency react correctly when the paste moves.
	if a.dejavu != nil || a.indexedMode {
		logf("INDEXED clipboard-paste alpha-preserving palette-%d source=%q pixels=%d", len(a.currentIndexedColorTable()), source, cb.W*cb.H)
	}
	a.clearSelectionState()
	// Keep one pre-paste snapshot before any automatic canvas growth. Undoing a
	// paste therefore restores both the original pixels and the original canvas
	// dimensions in a single step.
	a.undo.Push(a.canvas)
	oldW, oldH := a.canvas.W, a.canvas.H
	newW, newH, grew, canFit := canvasSizeForClipboardPaste(oldW, oldH, x, y, cb.W, cb.H)
	if a.dejavu != nil {
		// DejaVu IMG is structurally fixed at a 12×12 sheet of 24×24 tiles.
		// Preserve the normal paste/selection workflow but never let the generic
		// clipboard auto-grow path change the required 288×288 document size.
		newW, newH, grew = oldW, oldH, false
		canFit = x >= 0 && y >= 0 && x+cb.W <= oldW && y+cb.H <= oldH
	}
	if grew {
		a.canvas.ResizeCanvas(newW, newH, 0, 0, a.bg)
		a.updateScrollbars()
		logf("CLIPBOARD auto-canvas-grow source=%q old=%dx%d new=%dx%d paste=%dx%d at=%d,%d", source, oldW, oldH, newW, newH, cb.W, cb.H, x, y)
	} else if !canFit {
		logf("CLIPBOARD auto-canvas-grow skipped source=%q canvas=%dx%d paste=%dx%d at=%d,%d limit=%dx%d pixels=%d", source, oldW, oldH, cb.W, cb.H, x, y, clipboardAutoCanvasMaxDimension, clipboardAutoCanvasMaxDimension, clipboardAutoCanvasMaxPixels)
	}
	previousTool := a.tool
	a.tool = ToolRectSelect
	a.selection = SelectionState{Active: true, Floating: true, ForceOpaque: a.dejavu != nil, IndexedPalettePaste: a.dejavu != nil || a.indexedMode, X: x, Y: y, W: cb.W, H: cb.H, Pix: append([]byte(nil), cb.Pix...), BasePix: a.canvas.ClonePixels()}
	if len(cb.Mask) == cb.W*cb.H {
		a.selection.Mask = append([]bool(nil), cb.Mask...)
		a.selection.Free = true
	}
	a.applyFloatingSelection(x, y)
	a.markDirty()
	a.recordHistory("붙여넣기")
	if grew {
		a.statusText = fmt.Sprintf("%s의 %d × %d 이미지를 붙이기 위해 캔버스를 %d × %d → %d × %d로 자동 확장했습니다. 선택 도구로 전환했으니 바로 끌어 이동하십시오.", source, cb.W, cb.H, oldW, oldH, newW, newH)
	} else if !canFit {
		a.statusText = fmt.Sprintf("%s의 %d × %d 이미지는 자동 확장 안전 한도를 넘어 현재 캔버스 안에 들어오는 부분만 붙였습니다.", source, cb.W, cb.H)
	} else {
		a.statusText = fmt.Sprintf("%s의 %d × %d 이미지를 좌상단 (%d,%d)에 붙였습니다. 선택 도구로 전환했으니 바로 끌어 이동하십시오.", source, cb.W, cb.H, x, y)
	}
	if a.dejavu != nil {
		a.statusText += " DejaVu 16색 변환: 완전 투명 픽셀은 기존 타일 유지, 반투명 픽셀은 겹친 바탕색과 합성합니다."
	}
	logf("CLIPBOARD paste source=%q w=%d h=%d topLeft=%d,%d tool=%d->%d canvas=%dx%d", source, cb.W, cb.H, x, y, previousTool, a.tool, a.canvas.W, a.canvas.H)
	a.requestCanvasRepaint(true)
	return true
}

func (a *App) pasteClipboardImage(cb ClipboardImage, source string) bool {
	x, y := a.viewportTopLeftImage()
	return a.pasteClipboardImageAt(cb, source, x, y)
}

func (a *App) pasteSelectedClipboard() bool {
	img, source, err := a.readSystemClipboard()
	if err == nil && validClipboardImage(img) {
		seq, _, _ := procGetClipboardSequenceNumber.Call()
		a.lastClipboardSeq = uint32(seq)
		a.clipboard = img
		a.addClipboardHistory(img, source)
		if strings.HasPrefix(source, "Windows 탐색기 파일:") {
			return a.pasteClipboardImageAt(img, source, 0, 0)
		}
		return a.pasteClipboardImage(img, "Windows 클립보드")
	}
	if a.clipboardSelected >= 0 && a.clipboardSelected < len(a.clipboardHistory) {
		e := a.clipboardHistory[a.clipboardSelected]
		a.clipboard = e.Image
		return a.pasteClipboardImage(e.Image, fmt.Sprintf("클립보드 기록 %d", a.clipboardSelected+1))
	}
	if validClipboardImage(a.clipboard) {
		return a.pasteClipboardImage(a.clipboard, "내부 클립보드")
	}
	a.statusText = "붙여넣을 이미지가 없습니다. Print Screen 또는 다른 프로그램에서 이미지를 복사해 보십시오."
	logf("CLIPBOARD paste unavailable err=%v", err)
	message := "Windows 클립보드에서 이미지를 읽지 못했습니다.\n\n"
	if err != nil {
		message += err.Error() + "\n\n"
	}
	message += "Print Screen이나 캡처 도구로 다시 복사한 뒤 Ctrl+V를 누르십시오."
	if diagnosticLoggingEnabled {
		message += "\n계속 실패하면 dotPaint.log를 올려 주십시오."
	}
	messageBox(a.hwnd, message, "클립보드 붙여넣기 실패", MB_OK|MB_ICONWARNING)
	invalidate(a.hwnd)
	return false
}

func clipboardPinsDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "dotPaint", "clipboard_pins")
}

func (a *App) savePinnedClipboardHistory() {
	dir := clipboardPinsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logf("CLIPBOARD pins mkdir failed dir=%q err=%v", dir, err)
		return
	}
	old, _ := filepath.Glob(filepath.Join(dir, "pin_*.png"))
	for _, path := range old {
		_ = os.Remove(path)
	}
	meta := make([]clipboardPinMeta, 0, 6)
	pinNo := 0
	for _, e := range a.clipboardHistory {
		if !e.Pinned || !validClipboardImage(e.Image) {
			continue
		}
		pinNo++
		name := fmt.Sprintf("pin_%02d.png", pinNo)
		path := filepath.Join(dir, name)
		canvas := &Canvas{W: e.Image.W, H: e.Image.H, Pix: e.Image.Pix}
		if err := encodeImageFile(path, canvas); err != nil {
			logf("CLIPBOARD pins image save failed path=%q err=%v", path, err)
			continue
		}
		meta = append(meta, clipboardPinMeta{File: name, Source: e.Source, Added: e.Added})
		if len(meta) >= clipboardHistoryCapacity {
			break
		}
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		logf("CLIPBOARD pins marshal failed: %v", err)
		return
	}
	tmp := filepath.Join(dir, "pins.json.tmp")
	final := filepath.Join(dir, "pins.json")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		logf("CLIPBOARD pins meta write failed: %v", err)
		return
	}
	_ = os.Remove(final)
	if err := os.Rename(tmp, final); err != nil {
		logf("CLIPBOARD pins meta rename failed: %v", err)
		return
	}
	logf("CLIPBOARD pins saved count=%d dir=%q", len(meta), dir)
}

func (a *App) loadPinnedClipboardHistory() {
	dir := clipboardPinsDir()
	data, err := os.ReadFile(filepath.Join(dir, "pins.json"))
	if err != nil {
		if !os.IsNotExist(err) {
			logf("CLIPBOARD pins meta read failed: %v", err)
		}
		return
	}
	var meta []clipboardPinMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		logf("CLIPBOARD pins meta parse failed: %v", err)
		return
	}
	for _, m := range meta {
		if len(a.clipboardHistory) >= clipboardHistoryCapacity {
			break
		}
		cv, err := decodeImageFile(filepath.Join(dir, filepath.Base(m.File)))
		if err != nil {
			logf("CLIPBOARD pin load failed file=%q err=%v", m.File, err)
			continue
		}
		img := ClipboardImage{W: cv.W, H: cv.H, Pix: cv.Pix}
		if !validClipboardImage(img) {
			logf("CLIPBOARD pin invalid file=%q w=%d h=%d", m.File, cv.W, cv.H)
			continue
		}
		added := m.Added
		if added.IsZero() {
			added = time.Now()
		}
		source := m.Source
		if source == "" {
			source = "고정 클립보드"
		}
		a.clipboardHistory = append(a.clipboardHistory, ClipboardEntry{Image: img, Hash: clipboardImageHash(img), Pinned: true, Source: source, Added: added})
	}
	if len(a.clipboardHistory) > 0 {
		a.clipboardSelected = 0
		a.clipboardScroll = 0
		a.clipboard = a.clipboardHistory[0].Image
	}
	logf("CLIPBOARD pins loaded count=%d dir=%q", len(a.clipboardHistory), dir)
}

func (a *App) title() string {
	name := localizeUI("제목 없음")
	if a.filePath != "" {
		name = filepath.Base(a.filePath)
	}
	if a.dirty {
		name += " *"
	}
	return name + " - dotPaint"
}
func (a *App) updateTitle() {
	procSetWindowTextW.Call(a.hwnd, uintptr(unsafe.Pointer(rawUTF16Ptr(a.title()))))
}
func (a *App) markDirty()  { a.dirty = true; a.updateTitle() }
func (a *App) clearDirty() { a.dirty = false; a.updateTitle() }

func (a *App) panelDockWidth() int {
	// Right-side panels are overlays, not layout columns. Docking must never
	// reserve a grey inactive strip or shrink the canvas workspace.
	return 0
}

func clampPanelRect(x, y, w, h int, client RECT) RECT {
	cw := int(client.Right - client.Left)
	ch := int(client.Bottom - client.Top)
	if w > cw {
		w = max(cw, 1)
	}
	if h > ch {
		h = max(ch, 1)
	}
	x = clamp(x, 0, max(cw-w, 0))
	y = clamp(y, 0, max(ch-h, 0))
	return RECT{int32(x), int32(y), int32(x + w), int32(y + h)}
}

func (a *App) clipboardPanelVisible() bool {
	return a.showClipboard && !a.bitmapView
}

func (a *App) textPanelVisible() bool {
	return a.showTextPanel && a.tool == ToolText && !a.bitmapView
}

func (a *App) historyPanelVisible() bool {
	return a.showHistory && !a.bitmapView
}

func (a *App) thumbnailPanelVisible() bool {
	return a.showThumbnail && a.zoom >= 4 && !a.bitmapView
}

func (a *App) toolboxPanelVisible() bool {
	return a.showToolbox && !a.bitmapView
}

func (a *App) palettePanelVisible() bool {
	return a.showPalette && !a.bitmapView
}

func statusBarHeight(a *App) int {
	if a.showStatus && !a.bitmapView {
		return 25
	}
	return 0
}

func palettePanelWidth() int {
	// Current foreground/background boxes (70 px), 44 standard+custom color
	// cells in two rows, and the final transparent swatch.
	return 70 + paletteCellColumns*paletteCellSize + 2 + 46 + 6
}

func (a *App) paletteDisplayMode() int {
	if a.dejavu != nil {
		return 16
	}
	if a.indexedMode {
		return a.indexedSize
	}
	return 0
}
func (a *App) paletteVisibleCount() int {
	if a.dejavu != nil {
		return 16
	} // DejaVu IMG always shows all fixed indices.
	mode := a.paletteDisplayMode()
	count := a.paletteDisplayRGB
	switch mode {
	case 16:
		count = a.paletteDisplay16
	case 256:
		count = a.paletteDisplay256
	}
	return paletteDisplayClamp(mode, count)
}
func (a *App) palettePanelActualWidth() int {
	cols := paletteDisplayColumns(a.paletteDisplayMode(), a.paletteVisibleCount())
	return 70 + cols*paletteCellSize + 2 + 46 + 6
}
func (a *App) palettePanelActualHeight() int {
	rows := paletteDisplayRows(a.paletteDisplayMode(), a.paletteVisibleCount())
	// Preserve original 72px height for one/two rows; expand for more colours.
	return max(palettePanelHeight, panelHeaderHeight+2+rows*paletteCellSize+8)
}

func setPanelDockStage(panel *PanelState, stage int) {
	stage = clamp(stage, 0, 2)
	panel.DockStage = stage
	panel.Docked = stage > 0
}

func panelDockStage(panel *PanelState) int {
	if panel.DockStage == 0 && panel.Docked {
		// Compatibility for any state created before A59. Old Docked=true meant
		// the fully aligned second stage.
		return 2
	}
	return clamp(panel.DockStage, 0, 2)
}

type dockedPanelRef struct {
	Panel   *PanelState
	Visible bool
}

func (a *App) dockedPanelY(panel *PanelState) int {
	refs := []dockedPanelRef{
		{&a.clipboardPanel, a.clipboardPanelVisible()},
		{&a.textPanel, a.textPanelVisible()},
		{&a.historyPanel, a.historyPanelVisible()},
		{&a.thumbnailPanel, a.thumbnailPanelVisible()},
	}
	active := make([]*PanelState, 0, len(refs))
	for _, ref := range refs {
		if ref.Visible && panelDockStage(ref.Panel) == 2 {
			active = append(active, ref.Panel)
		}
	}
	sort.SliceStable(active, func(i, j int) bool { return active[i].DockOrder < active[j].DockOrder })
	y := panelDockTopOffset
	for _, p := range active {
		if p == panel {
			return y
		}
		y += p.H + panelDockGap
	}
	return y
}

func (a *App) clipboardPanelRect(client RECT) RECT {
	if !a.clipboardPanelVisible() {
		return RECT{}
	}
	w := rightPanelWidth
	h := a.clipboardPanel.H
	stage := panelDockStage(&a.clipboardPanel)
	if stage > 0 {
		x := int(client.Right) - w
		y := a.clipboardPanel.Y
		if stage == 2 {
			y = a.dockedPanelY(&a.clipboardPanel)
		}
		return clampPanelRect(x, y, w, h, client)
	}
	return clampPanelRect(a.clipboardPanel.X, a.clipboardPanel.Y, w, h, client)
}

func (a *App) textPanelRect(client RECT) RECT {
	if !a.textPanelVisible() {
		return RECT{}
	}
	w := rightPanelWidth
	h := a.textPanel.H
	stage := panelDockStage(&a.textPanel)
	if stage > 0 {
		x := int(client.Right) - w
		y := a.textPanel.Y
		if stage == 2 {
			y = a.dockedPanelY(&a.textPanel)
		}
		return clampPanelRect(x, y, w, h, client)
	}
	return clampPanelRect(a.textPanel.X, a.textPanel.Y, w, h, client)
}

func (a *App) historyPanelRect(client RECT) RECT {
	if !a.historyPanelVisible() {
		return RECT{}
	}
	w := rightPanelWidth
	h := a.historyPanel.H
	stage := panelDockStage(&a.historyPanel)
	if stage > 0 {
		x := int(client.Right) - w
		y := a.historyPanel.Y
		if stage == 2 {
			y = a.dockedPanelY(&a.historyPanel)
		}
		return clampPanelRect(x, y, w, h, client)
	}
	return clampPanelRect(a.historyPanel.X, a.historyPanel.Y, w, h, client)
}

func (a *App) thumbnailPanelRect(client RECT) RECT {
	if !a.thumbnailPanelVisible() {
		return RECT{}
	}
	w := rightPanelWidth
	h := a.thumbnailPanel.H
	stage := panelDockStage(&a.thumbnailPanel)
	if stage > 0 {
		x := int(client.Right) - w
		y := a.thumbnailPanel.Y
		if stage == 2 {
			y = a.dockedPanelY(&a.thumbnailPanel)
		}
		return clampPanelRect(x, y, w, h, client)
	}
	return clampPanelRect(a.thumbnailPanel.X, a.thumbnailPanel.Y, w, h, client)
}

func (a *App) toolboxPanelRect(client RECT) RECT {
	if !a.toolboxPanelVisible() {
		return RECT{}
	}
	w := toolboxWidth
	stage := panelDockStage(&a.toolboxPanel)
	if stage == 2 {
		bottom := int(client.Bottom) - statusBarHeight(a)
		h := max(bottom, panelHeaderHeight+80)
		return clampPanelRect(0, 0, w, h, client)
	}
	a.toolboxPanel.W = w
	x := a.toolboxPanel.X
	if stage == 1 {
		x = 0
	}
	return clampPanelRect(x, a.toolboxPanel.Y, w, a.toolboxPanel.H, client)
}

func (a *App) palettePanelRect(client RECT) RECT {
	if !a.palettePanelVisible() {
		return RECT{}
	}
	w := a.palettePanelActualWidth()
	a.palettePanel.W = w
	h := a.palettePanelActualHeight()
	a.palettePanel.H = h
	stage := panelDockStage(&a.palettePanel)
	if stage > 0 {
		bottom := int(client.Bottom) - statusBarHeight(a)
		x := a.palettePanel.X
		if stage == 2 {
			// A243: palette docks immediately to the right of the tool strip,
			// along the bottom edge (user's preferred layout).
			x = 0
			if a.toolboxPanelVisible() && panelDockStage(&a.toolboxPanel) == 2 {
				x = toolboxWidth
			}
		}
		return clampPanelRect(x, bottom-h, w, h, client)
	}
	return clampPanelRect(a.palettePanel.X, a.palettePanel.Y, w, h, client)
}

func panelHeaderRect(panel RECT) RECT {
	return RECT{panel.Left + 1, panel.Top + 1, panel.Right - 1, panel.Top + 1 + panelHeaderHeight}
}

func panelCloseRect(panel RECT) RECT {
	return RECT{panel.Right - panelCloseSize - 2, panel.Top + 1, panel.Right - 2, panel.Top + panelCloseSize + 1}
}

func panelContentRect(panel RECT) RECT {
	return RECT{panel.Left + 1, panel.Top + 1 + panelHeaderHeight, panel.Right - 1, panel.Bottom - 1}
}

const (
	toolboxWidth       = 98
	toolCellWidth      = 45
	toolCellHeight     = 41
	toolButtonWidth    = 41
	toolButtonHeight   = 37
	toolboxOptionLeft  = 8
	toolboxOptionRight = 90
)

func toolOptionsTop() int {
	rows := (len(visibleTools) + 1) / 2
	return 4 + rows*toolCellHeight + 8
}

func (a *App) layout(client RECT) (workspace, toolbar, palette, status RECT) {
	w := int(client.Right - client.Left)
	h := int(client.Bottom - client.Top)
	statusH := statusBarHeight(a)
	// The docked palette is a finite-width overlay. It must not reserve a
	// full-width grey strip after the final transparent swatch.
	paletteH := 0
	toolbarW := 0
	if a.toolboxPanelVisible() && panelDockStage(&a.toolboxPanel) == 2 {
		toolbarW = toolboxWidth
	}
	status = RECT{0, int32(h - statusH), int32(w), int32(h)}
	workspace = RECT{int32(toolbarW), 0, int32(w), int32(h - statusH - paletteH)}
	if a.toolboxPanelVisible() {
		toolbar = panelContentRect(a.toolboxPanelRect(client))
	}
	if a.palettePanelVisible() {
		palette = panelContentRect(a.palettePanelRect(client))
	}
	return
}

func (a *App) canvasOrigin(workspace RECT) (int, int) {
	return int(workspace.Left) + 8 - a.scrollX, int(workspace.Top) + 8 - a.scrollY
}

func (a *App) viewportTopLeftImage() (int, int) {
	z := max(a.zoom, 1)
	x := clamp(a.scrollX/z, 0, max(a.canvas.W-1, 0))
	y := clamp(a.scrollY/z, 0, max(a.canvas.H-1, 0))
	return x, y
}

func (a *App) updateScrollbars() {
	var rc RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	ws, _, _, _ := a.layout(rc)
	pageW := int(ws.Right-ws.Left) - 16
	pageH := int(ws.Bottom-ws.Top) - 16
	if pageW < 1 {
		pageW = 1
	}
	if pageH < 1 {
		pageH = 1
	}
	contentW := a.canvas.W * a.zoom
	contentH := a.canvas.H * a.zoom
	maxX := contentW - pageW
	if maxX < 0 {
		maxX = 0
	}
	maxY := contentH - pageH
	if maxY < 0 {
		maxY = 0
	}
	a.scrollX = clamp(a.scrollX, 0, maxX)
	a.scrollY = clamp(a.scrollY, 0, maxY)
	si := SCROLLINFO{CbSize: uint32(unsafe.Sizeof(SCROLLINFO{})), FMask: SIF_RANGE | SIF_PAGE | SIF_POS | SIF_DISABLENOSCROLL, NMin: 0, NMax: int32(max(contentW-1, 0)), NPage: uint32(pageW), NPos: int32(a.scrollX)}
	procSetScrollInfo.Call(a.hwnd, SB_HORZ, uintptr(unsafe.Pointer(&si)), 1)
	si.NMax = int32(max(contentH-1, 0))
	si.NPage = uint32(pageH)
	si.NPos = int32(a.scrollY)
	procSetScrollInfo.Call(a.hwnd, SB_VERT, uintptr(unsafe.Pointer(&si)), 1)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (a *App) clientToImage(mx, my int) (int, int, bool) {
	var rc RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	ws, _, _, _ := a.layout(rc)
	ox, oy := a.canvasOrigin(ws)
	ix := (mx - ox) / a.zoom
	iy := (my - oy) / a.zoom
	inside := mx >= ox && my >= oy && ix >= 0 && iy >= 0 && ix < a.canvas.W && iy < a.canvas.H
	return ix, iy, inside
}

func (a *App) canvasScreenRect() RECT {
	var rc RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	ws, _, _, _ := a.layout(rc)
	ox, oy := a.canvasOrigin(ws)
	return RECT{int32(ox), int32(oy), int32(ox + a.canvas.W*a.zoom), int32(oy + a.canvas.H*a.zoom)}
}

func normalizeSelectionRect(x0, y0, x1, y1 int) (x, y, w, h int) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	return x0, y0, x1 - x0 + 1, y1 - y0 + 1
}

func (a *App) captureSelectionImage() ClipboardImage {
	s := &a.selection
	if !s.Active || s.W < 1 || s.H < 1 {
		return ClipboardImage{}
	}
	out := ClipboardImage{W: s.W, H: s.H, Pix: make([]byte, s.W*s.H*4)}
	if len(s.Mask) == s.W*s.H {
		out.Mask = append([]bool(nil), s.Mask...)
	}
	for y := 0; y < s.H; y++ {
		for x := 0; x < s.W; x++ {
			si := ((s.Y+y)*a.canvas.W + (s.X + x)) * 4
			di := (y*s.W + x) * 4
			if s.X+x >= 0 && s.Y+y >= 0 && s.X+x < a.canvas.W && s.Y+y < a.canvas.H {
				copy(out.Pix[di:di+4], a.canvas.Pix[si:si+4])
			}
		}
	}
	// Irregular selections still use a rectangular image container, but the
	// pixels outside their real mask must be actual transparent pixels. Keeping
	// the mask only as dotPaint metadata made Windows PNG/CF_DIB consumers and
	// pinned history files store the complete bounding rectangle.
	applyClipboardSelectionMask(out.Pix, out.Mask, out.W, out.H)
	return out
}

func (a *App) selectionMaskAllows(i int) bool {
	s := &a.selection
	return len(s.Mask) != s.W*s.H || (i >= 0 && i < len(s.Mask) && s.Mask[i])
}

func (a *App) clearSelectionPixels(bg uint32) {
	s := &a.selection
	if !s.Active {
		return
	}
	for y := 0; y < s.H; y++ {
		for x := 0; x < s.W; x++ {
			i := y*s.W + x
			if !a.selectionMaskAllows(i) {
				continue
			}
			a.canvas.SetPixel(s.X+x, s.Y+y, bg)
		}
	}
}

func (a *App) liftSelectionForMove() bool {
	s := &a.selection
	if !s.Active {
		return false
	}
	if s.Floating {
		return true
	}
	a.undo.Push(a.canvas)
	img := a.captureSelectionImage()
	if img.W < 1 || img.H < 1 {
		a.undo.DiscardLast()
		return false
	}
	s.Pix = img.Pix
	if len(img.Mask) > 0 {
		s.Mask = img.Mask
	}
	a.clearSelectionPixels(a.bg)
	s.BasePix = a.canvas.ClonePixels()
	s.Floating = true
	a.applyFloatingSelection(s.X, s.Y)
	a.markDirty()
	logf("SELECTION lift x=%d y=%d w=%d h=%d free=%v", s.X, s.Y, s.W, s.H, s.Free)
	return true
}

func (a *App) applyFloatingSelection(x, y int) {
	s := &a.selection
	if !s.Floating || len(s.BasePix) != len(a.canvas.Pix) || len(s.Pix) != s.W*s.H*4 {
		return
	}
	copy(a.canvas.Pix, s.BasePix)
	bgpx := colorrefToBGRA(a.bg)
	for sy := 0; sy < s.H; sy++ {
		for sx := 0; sx < s.W; sx++ {
			i := sy*s.W + sx
			if !a.selectionMaskAllows(i) {
				continue
			}
			si := i * 4
			if !a.opaque && !s.ForceOpaque && s.Pix[si] == bgpx[0] && s.Pix[si+1] == bgpx[1] && s.Pix[si+2] == bgpx[2] && s.Pix[si+3] == bgpx[3] {
				continue
			}
			dx, dy := x+sx, y+sy
			if dx < 0 || dy < 0 || dx >= a.canvas.W || dy >= a.canvas.H {
				continue
			}
			di := (dy*a.canvas.W + dx) * 4
			if s.IndexedPalettePaste && (a.dejavu != nil || a.indexedMode) {
				if a.dejavu != nil {
					dejavuCompositeIndexedPastePixel(a.canvas.Pix[di:di+4], s.Pix[si:si+4], a.dejavu.Palette)
				} else {
					compositeIndexedBGRA(a.canvas.Pix[di:di+4], s.Pix[si:si+4], a.activeIndexedPaletteColors())
				}
				continue
			}
			sa := int(s.Pix[si+3])
			if sa == 0 {
				continue
			}
			if sa == 255 {
				copy(a.canvas.Pix[di:di+4], s.Pix[si:si+4])
				continue
			}
			da := int(a.canvas.Pix[di+3])
			outA := sa + (da*(255-sa)+127)/255
			if outA <= 0 {
				a.canvas.Pix[di+0], a.canvas.Pix[di+1], a.canvas.Pix[di+2], a.canvas.Pix[di+3] = 0, 0, 0, 0
				continue
			}
			for ch := 0; ch < 3; ch++ {
				sv := int(s.Pix[si+ch])
				dv := int(a.canvas.Pix[di+ch])
				premul := sv*sa + (dv*da*(255-sa)+127)/255
				a.canvas.Pix[di+ch] = byte((premul + outA/2) / outA)
			}
			a.canvas.Pix[di+3] = byte(outA)
		}
	}
	s.X, s.Y = x, y
}

func (a *App) clearSelectionState() {
	hadOverlay := a.selection.Active || a.selection.Creating || a.selection.Adding || len(a.selection.Lasso) > 0
	if a.selection.Active {
		logf("SELECTION clear active=%v floating=%v", a.selection.Active, a.selection.Floating)
	}
	a.selection.Reset()
	if hadOverlay && a.hwnd != 0 {
		// A pasted selection may be wider or taller than the canvas, and its
		// marquee is intentionally allowed to extend into the grey workspace.
		// Repaint the complete workspace on clear; repainting only the canvas
		// rectangle leaves stale dashes behind outside the canvas bounds.
		var client RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
		ws, _, _, _ := a.layout(client)
		invalidateArea(a.hwnd, &ws)
	}
}

func (a *App) armPendingRectSelection(mx, my, ix, iy int, outsideCanvas bool) {
	if a.canvas == nil || a.canvas.W < 1 || a.canvas.H < 1 {
		return
	}
	a.pendingRectSelection = true
	a.pendingRectSelectionStartClientX = mx
	a.pendingRectSelectionStartClientY = my
	a.pendingRectSelectionStartImageX = clamp(ix, 0, a.canvas.W-1)
	a.pendingRectSelectionStartImageY = clamp(iy, 0, a.canvas.H-1)
	a.pendingRectSelectionOutsideCanvas = outsideCanvas
	procSetCapture.Call(a.hwnd)
	logf("SELECTION rect pending start client=%d,%d image=%d,%d outside=%v", mx, my, a.pendingRectSelectionStartImageX, a.pendingRectSelectionStartImageY, outsideCanvas)
}

func (a *App) cancelPendingRectSelection(reason string, releaseCapture bool) {
	if !a.pendingRectSelection {
		return
	}
	outside := a.pendingRectSelectionOutsideCanvas
	a.pendingRectSelection = false
	a.pendingRectSelectionStartClientX = 0
	a.pendingRectSelectionStartClientY = 0
	a.pendingRectSelectionStartImageX = 0
	a.pendingRectSelectionStartImageY = 0
	a.pendingRectSelectionOutsideCanvas = false
	if releaseCapture {
		procReleaseCapture.Call()
	}
	logf("SELECTION rect pending cancel reason=%q outside=%v", reason, outside)
}

func (a *App) activatePendingRectSelection(mx, my int) {
	if !a.pendingRectSelection || a.canvas == nil || a.canvas.W < 1 || a.canvas.H < 1 {
		return
	}
	startX := a.pendingRectSelectionStartImageX
	startY := a.pendingRectSelectionStartImageY
	outside := a.pendingRectSelectionOutsideCanvas
	a.pendingRectSelection = false
	a.pendingRectSelectionOutsideCanvas = false
	ix, iy, _ := a.clientToImage(mx, my)
	ix = clamp(ix, 0, a.canvas.W-1)
	iy = clamp(iy, 0, a.canvas.H-1)
	a.selection.Reset()
	a.selection.Creating = true
	a.selection.Free = false
	a.selection.StartX, a.selection.StartY = startX, startY
	a.selection.CurrentX, a.selection.CurrentY = ix, iy
	a.statusText = "사각 선택 중: 캔버스 경계 밖의 시작점은 최외각 픽셀에 고정됩니다."
	logf("SELECTION rect pending activate start=%d,%d current=%d,%d outside=%v", startX, startY, ix, iy, outside)
	a.requestCanvasRepaint(true)
	a.requestStatusRepaint()
}

func (a *App) copySelection() bool {
	img := a.captureSelectionImage()
	if !validClipboardImage(img) {
		return false
	}
	a.clipboard = img
	a.addClipboardHistory(img, "dotPaint 복사")
	if err := a.writeSystemClipboard(img); err != nil {
		a.statusText = fmt.Sprintf("dotPaint 기록에는 복사했지만 Windows 클립보드 복사에 실패했습니다: %v", err)
		logf("SELECTION copy system failed w=%d h=%d err=%v", img.W, img.H, err)
	} else {
		a.statusText = fmt.Sprintf("선택 영역 %d × %d을(를) Windows 클립보드와 기록에 복사했습니다.", img.W, img.H)
		logf("SELECTION copy system w=%d h=%d", img.W, img.H)
	}
	invalidate(a.hwnd)
	return true
}

func (a *App) replaceSelectionWith(fill uint32, historyName string) bool {
	if !a.selection.Active {
		return false
	}
	// A floating selection is already composited over BasePix. Restore the base
	// first, then replace the selected mask at its current destination. This
	// makes both normal and floating selections obey the same Del/Shift+Del
	// semantics instead of merely discarding a pasted object.
	a.undo.Push(a.canvas)
	if a.selection.Floating && len(a.selection.BasePix) == len(a.canvas.Pix) {
		copy(a.canvas.Pix, a.selection.BasePix)
	}
	a.clearSelectionPixels(fill)
	a.clearSelectionState()
	a.markDirty()
	a.recordHistory(historyName)
	a.requestCanvasRepaint(true)
	return true
}

func (a *App) deleteSelection(copyFirst bool) bool {
	if !a.selection.Active {
		return false
	}
	if copyFirst {
		a.copySelection()
		return a.replaceSelectionWith(a.bg, "잘라내기")
	}
	return a.replaceSelectionWith(a.bg, "선택 영역 배경색")
}

func (a *App) deleteSelectionTransparent() bool {
	return a.replaceSelectionWith(transparentColor, "선택 영역 투명 지우기")
}

func (a *App) pasteImageFile(path string) bool {
	cv, err := decodeImageFile(path)
	if err != nil {
		messageBox(a.hwnd, "파일을 붙여넣을 수 없습니다.\n\n"+err.Error(), "dotPaint", MB_OK|MB_ICONERROR)
		logf("PASTE_FROM_FILE failed path=%q err=%v", path, err)
		return false
	}
	img := canvasToClipboardImage(cv)
	if !validClipboardImage(img) {
		messageBox(a.hwnd, "파일의 이미지 크기가 올바르지 않습니다.", "dotPaint", MB_OK|MB_ICONERROR)
		return false
	}
	source := "파일: " + filepath.Base(path)
	a.clipboard = img
	a.addClipboardHistory(img, source)
	logf("PASTE_FROM_FILE path=%q w=%d h=%d", path, img.W, img.H)
	return a.pasteClipboardImageAt(img, source, 0, 0)
}

func (a *App) pasteInternalClipboard() bool {
	return a.pasteSelectedClipboard()
}

func (a *App) selectAll() {
	a.clearSelectionState()
	a.selection = SelectionState{Active: true, X: 0, Y: 0, W: a.canvas.W, H: a.canvas.H}
	a.tool = ToolRectSelect
	a.statusText = fmt.Sprintf("전체 선택: %d × %d", a.canvas.W, a.canvas.H)
	logf("SELECTION select-all %dx%d", a.canvas.W, a.canvas.H)
	invalidate(a.hwnd)
}

func (a *App) canvasHandleRects() [8]RECT {
	var rc RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	ws, _, _, _ := a.layout(rc)
	ox, oy := a.canvasOrigin(ws)
	left, top := ox, oy
	w, h := a.canvas.W, a.canvas.H
	if a.resizingCanvas {
		left = ox + a.resizeLeftDelta*a.zoom
		top = oy + a.resizeTopDelta*a.zoom
		w, h = a.resizePreviewW, a.resizePreviewH
	}
	right := left + w*a.zoom
	bottom := top + h*a.zoom
	const size = 8
	half := size / 2
	midX := (left + right) / 2
	midY := (top + bottom) / 2
	// Every handle is entirely outside the white canvas and touches its edge.
	// Corner handles occupy the diagonal exterior quadrant, matching the classic
	// paint resize affordance without covering even one canvas pixel.
	return [8]RECT{
		{int32(left - size), int32(top - size), int32(left), int32(top)},
		{int32(midX - half), int32(top - size), int32(midX - half + size), int32(top)},
		{int32(right), int32(top - size), int32(right + size), int32(top)},
		{int32(left - size), int32(midY - half), int32(left), int32(midY - half + size)},
		{int32(right), int32(midY - half), int32(right + size), int32(midY - half + size)},
		{int32(left - size), int32(bottom), int32(left), int32(bottom + size)},
		{int32(midX - half), int32(bottom), int32(midX - half + size), int32(bottom + size)},
		{int32(right), int32(bottom), int32(right + size), int32(bottom + size)},
	}
}

func pointInRect(x, y int, r RECT) bool {
	return x >= int(r.Left) && x < int(r.Right) && y >= int(r.Top) && y < int(r.Bottom)
}
func intersectRECT(a, b RECT) (RECT, bool) {
	r := RECT{Left: int32(max(int(a.Left), int(b.Left))), Top: int32(max(int(a.Top), int(b.Top))), Right: int32(min(int(a.Right), int(b.Right))), Bottom: int32(min(int(a.Bottom), int(b.Bottom)))}
	return r, r.Right > r.Left && r.Bottom > r.Top
}

func (a *App) hitCanvasHandle(mx, my int) (int, bool) {
	if a.bitmapView || a.pendingCanvasResize || a.resizingCanvas {
		return -1, false
	}
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	ws, tb, pal, _ := a.layout(client)
	// Handles belong only to the drawable workspace. A clipped/off-screen
	// handle must not create an invisible hit target over a docked panel.
	if !pointInRect(mx, my, ws) || pointInRect(mx, my, tb) || pointInRect(mx, my, pal) {
		return -1, false
	}
	hs := a.canvasHandleRects()
	for i, raw := range hs {
		r, ok := intersectRECT(raw, ws)
		if !ok || int(r.Right-r.Left) < 4 || int(r.Bottom-r.Top) < 4 {
			continue
		}
		if pointInRect(mx, my, r) {
			return i, true
		}
	}
	return -1, false
}

func (a *App) ownsMouseCapture() bool {
	owner, _, _ := procGetCapture.Call()
	return owner == a.hwnd
}

func (a *App) acquireCanvasResizeCapture() bool {
	// A163 called SetCapture twice: once while arming and once again when the
	// drag threshold was crossed. On the user's system the second call emitted
	// WM_CAPTURECHANGED synchronously, which the program mistook for a real
	// capture loss and left the mouse permanently captured. A164 acquires it
	// exactly once and verifies the owner.
	procSetCapture.Call(a.hwnd)
	owner, _, _ := procGetCapture.Call()
	ok := owner == a.hwnd
	logf("CANVAS_RESIZE capture-acquire owner=%#x self=%#x ok=%v", owner, a.hwnd, ok)
	return ok
}

func (a *App) releaseCanvasResizeCapture(reason string) {
	owner, _, _ := procGetCapture.Call()
	if owner == a.hwnd {
		procReleaseCapture.Call()
		logf("CANVAS_RESIZE capture-release reason=%q owner=%#x", reason, owner)
		return
	}
	logf("CANVAS_RESIZE capture-release skipped reason=%q owner=%#x self=%#x", reason, owner, a.hwnd)
}

func (a *App) armCanvasResize(handle, mx, my int) {
	if a.dejavu != nil {
		// IMG records always form a fixed 288x288 sheet.  The attributes menu
		// already blocks resizing; handles must obey the same restriction.
		a.statusText = "DejaVu IMG 타일시트는 288×288로 고정되어 있어 크기를 조절할 수 없습니다."
		logf("DEJAVU_IMG blocked canvas resize handle=%d", handle)
		invalidate(a.hwnd)
		return
	}
	a.pendingCanvasResize = true
	a.pendingResizeHandle = handle
	a.pendingResizeStartMouseX, a.pendingResizeStartMouseY = mx, my
	a.statusText = "캔버스 손잡이: 끌어서 크기를 조절합니다. 클릭만 하면 크기는 바뀌지 않습니다."
	if !a.acquireCanvasResizeCapture() {
		a.pendingCanvasResize = false
		a.statusText = fmt.Sprintf("캔버스 크기: %d × %d", a.canvas.W, a.canvas.H)
		logf("CANVAS_RESIZE arm-aborted handle=%d capture unavailable", handle)
		invalidate(a.hwnd)
		return
	}
	logf("CANVAS_RESIZE armed handle=%d client=%d,%d size=%dx%d", handle, mx, my, a.canvas.W, a.canvas.H)
	invalidate(a.hwnd)
}

func (a *App) beginCanvasResize(handle, mx, my int) {
	// Capture was already acquired by armCanvasResize. Never call SetCapture a
	// second time during the same interaction.
	if !a.ownsMouseCapture() {
		a.cancelCanvasResizeInteraction("drag threshold reached without capture", false)
		return
	}
	a.pendingCanvasResize = false
	a.clearSelectionState()
	a.resizingCanvas = true
	a.resizeHandle = handle
	a.resizeStartW, a.resizeStartH = a.canvas.W, a.canvas.H
	a.resizeStartMouseX, a.resizeStartMouseY = mx, my
	a.resizePreviewW, a.resizePreviewH = a.canvas.W, a.canvas.H
	a.resizeLeftDelta, a.resizeTopDelta = 0, 0
	a.statusText = fmt.Sprintf("캔버스 크기 조절 중: %d × %d", a.canvas.W, a.canvas.H)
	logf("CANVAS_RESIZE begin handle=%d size=%dx%d capture-owner=self", handle, a.canvas.W, a.canvas.H)
}

func (a *App) cancelCanvasResizeInteraction(reason string, releaseCapture bool) {
	if !a.pendingCanvasResize && !a.resizingCanvas {
		return
	}
	wasPending, wasActive := a.pendingCanvasResize, a.resizingCanvas
	a.pendingCanvasResize = false
	a.resizingCanvas = false
	a.resizeLeftDelta, a.resizeTopDelta = 0, 0
	if releaseCapture {
		a.releaseCanvasResizeCapture(reason)
	}
	a.statusText = fmt.Sprintf("캔버스 크기 조절을 취소했습니다. 현재 크기: %d × %d", a.canvas.W, a.canvas.H)
	logf("CANVAS_RESIZE state-reset reason=%q pending=%v active=%v", reason, wasPending, wasActive)
	invalidate(a.hwnd)
}

func (a *App) updateCanvasResize(mx, my int) {
	if !a.resizingCanvas {
		return
	}
	dx := int(math.Round(float64(mx-a.resizeStartMouseX) / float64(max(a.zoom, 1))))
	dy := int(math.Round(float64(my-a.resizeStartMouseY) / float64(max(a.zoom, 1))))
	w, h := a.resizeStartW, a.resizeStartH
	ld, td := 0, 0
	left := a.resizeHandle == 0 || a.resizeHandle == 3 || a.resizeHandle == 5
	right := a.resizeHandle == 2 || a.resizeHandle == 4 || a.resizeHandle == 7
	top := a.resizeHandle == 0 || a.resizeHandle == 1 || a.resizeHandle == 2
	bottom := a.resizeHandle == 5 || a.resizeHandle == 6 || a.resizeHandle == 7
	if left {
		ld = clamp(dx, a.resizeStartW-16384, a.resizeStartW-1)
		w = a.resizeStartW - ld
	}
	if right {
		w = clamp(a.resizeStartW+dx, 1, 16384)
	}
	if top {
		td = clamp(dy, a.resizeStartH-16384, a.resizeStartH-1)
		h = a.resizeStartH - td
	}
	if bottom {
		h = clamp(a.resizeStartH+dy, 1, 16384)
	}
	w = clamp(w, 1, 16384)
	h = clamp(h, 1, 16384)
	a.resizePreviewW, a.resizePreviewH = w, h
	a.resizeLeftDelta, a.resizeTopDelta = ld, td
	a.statusText = fmt.Sprintf("캔버스 크기 조절 중: %d × %d", w, h)
	invalidate(a.hwnd)
}

func (a *App) finishCanvasResize(cancel bool) {
	if !a.resizingCanvas {
		return
	}
	oldW, oldH := a.resizeStartW, a.resizeStartH
	nw, nh := a.resizePreviewW, a.resizePreviewH
	ld, td := a.resizeLeftDelta, a.resizeTopDelta
	// Clear the state before ReleaseCapture sends WM_CAPTURECHANGED.
	a.pendingCanvasResize = false
	a.resizingCanvas = false
	a.releaseCanvasResizeCapture("finish")
	if cancel || (nw == oldW && nh == oldH && ld == 0 && td == 0) {
		a.statusText = fmt.Sprintf("캔버스 크기: %d × %d", a.canvas.W, a.canvas.H)
		invalidate(a.hwnd)
		logf("CANVAS_RESIZE cancel=%v", cancel)
		return
	}
	a.undo.Push(a.canvas)
	a.canvas.ResizeCanvas(nw, nh, -ld, -td, a.bg)
	a.scrollX -= ld * a.zoom
	a.scrollY -= td * a.zoom
	a.markDirty()
	a.recordHistory("캔버스 크기")
	a.updateScrollbars()
	a.statusText = fmt.Sprintf("캔버스 크기: %d × %d", nw, nh)
	logf("CANVAS_RESIZE commit old=%dx%d new=%dx%d leftDelta=%d topDelta=%d", oldW, oldH, nw, nh, ld, td)
	invalidate(a.hwnd)
}

func (a *App) requestRepaintRect(rc RECT, immediate bool) {
	if a.hwnd == 0 || rectEmpty(rc) {
		return
	}
	invalidateArea(a.hwnd, &rc)
	if immediate && time.Since(a.lastImmediateUI) >= 12*time.Millisecond {
		a.lastImmediateUI = time.Now()
		procUpdateWindow.Call(a.hwnd)
	}
}

func (a *App) requestCanvasRepaint(immediate bool) {
	rc := expandRECT(a.canvasScreenRect(), 10)
	a.requestRepaintRect(rc, immediate)
	if a.thumbnailPanelVisible() {
		var client RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
		panel := a.thumbnailPanelRect(client)
		invalidateArea(a.hwnd, &panel)
	}
}

// forceCurveReleaseRepaint latches the last curve shape at every mouse-up.
// Live mouse-move previews use the normal 12 ms coalescing path, but a release
// is a semantic boundary: the accepted baseline/control point must already be
// visible before the next click.  Calling UpdateWindow unconditionally here
// prevents the final live frame from being erased while a throttled WM_PAINT is
// still pending, including click-only controls and releases outside the canvas.
func (a *App) forceCurveReleaseRepaint(oldRect RECT, reason string) {
	if a.hwnd == 0 {
		return
	}
	newRect := a.curvePreviewScreenRect()
	rc := unionRECT(oldRect, newRect)
	if rectEmpty(rc) {
		rc = expandRECT(a.canvasScreenRect(), 10)
	}
	invalidateArea(a.hwnd, &rc)
	// Do not use requestRepaintRect(..., true): its frame-rate throttle is useful
	// while dragging but is exactly what can leave a released curve invisible.
	procUpdateWindow.Call(a.hwnd)
	logf("CURVE preview-latch reason=%q stage=%d active-controls=%d drawing=%v preview=%v rect=%d,%d-%d,%d", reason, a.curveStage, a.curveActiveControlCount(), a.drawing, a.preview, rc.Left, rc.Top, rc.Right, rc.Bottom)
}

func (a *App) textVisualScreenRect() RECT {
	if a.textTool.Active {
		return expandRECT(a.textScreenRect(), 8)
	}
	if a.tool == ToolText && a.drawing && a.preview {
		var client RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
		ws, _, _, _ := a.layout(client)
		ox, oy := a.canvasOrigin(ws)
		x, y, w, h := normalizeSelectionRect(a.startX, a.startY, a.curX, a.curY)
		rc := RECT{int32(ox + x*a.zoom), int32(oy + y*a.zoom), int32(ox + (x+w)*a.zoom), int32(oy + (y+h)*a.zoom)}
		return expandRECT(rc, 8)
	}
	return RECT{}
}

func (a *App) requestTextRepaint(immediate bool) {
	a.requestRepaintRect(a.textVisualScreenRect(), immediate)
}

func (a *App) textCaretScreenRect() RECT {
	if !a.textTool.Active || !a.textTool.Editing || a.textTool.LayoutHwnd == 0 {
		return RECT{}
	}
	start := a.displayTextCaretIndex()
	if start < 0 {
		return a.textVisualScreenRect()
	}
	units := utf16UnitLen(a.textTool.Text)
	start = clamp(start, 0, units)
	positions := a.textBoundaryPositions()
	for _, pos := range positions {
		if pos.Index != start {
			continue
		}
		r := a.textScreenRect()
		x := int(r.Left) + pos.X*a.zoom
		y := int(r.Top) + pos.Y*a.zoom
		h := max(a.textLineHeight(pos.Y, positions)*a.zoom, 2)
		h = min(h, max(int(r.Bottom)-y, 0))
		if h <= 0 {
			return a.textVisualScreenRect()
		}
		return expandRECT(RECT{int32(x), int32(y), int32(x + 2), int32(y + h)}, 2)
	}
	return a.textVisualScreenRect()
}

func (a *App) requestTextCaretRepaint(immediate bool) {
	a.requestRepaintRect(a.textCaretScreenRect(), immediate)
}

func (a *App) scheduleTextInputRepaint() {
	if a.hwnd == 0 || !a.textTool.Active || !a.textTool.Editing {
		return
	}
	// A180: while the IME is active, a backwards delete state is held as a
	// pending transaction. Ninety-six milliseconds is long enough for the usual
	// delete/recreate pair to arrive, but a genuine Backspace still becomes
	// visible promptly if no replacement follows.
	delay := uintptr(28)
	if a.textTool.IMEActive {
		delay = 96
	}
	procKillTimer.Call(a.hwnd, textPreviewTimerID)
	procSetTimer.Call(a.hwnd, textPreviewTimerID, delay, 0)
}

func (a *App) vectorPreviewCanvasBounds(rawX, rawY int, constrain bool, arrow bool) (left, top, right, bottom int) {
	if a.canvas == nil || a.canvas.W < 1 || a.canvas.H < 1 {
		return 0, 0, -1, -1
	}
	nx, ny := a.constrainedLineEndpoint(a.startX, a.startY, rawX, rawY, constrain)
	size := max(a.lineSize, 1)
	margin := int(math.Ceil(float64(size)/2.0)) + 3
	if arrow {
		size = max(a.arrowLineSize, 1)
		choice := clamp(a.arrowHeadSize, 0, len(arrowHeadWingSteps)-1)
		wing := arrowHeadWingSteps[choice]
		margin = int(math.Ceil(math.Max(wing, arrowTipHalfWidth(size)))) + size + 5
	}
	left = clamp(min(a.startX, nx)-margin, 0, a.canvas.W-1)
	right = clamp(max(a.startX, nx)+margin, 0, a.canvas.W-1)
	top = clamp(min(a.startY, ny)-margin, 0, a.canvas.H-1)
	bottom = clamp(max(a.startY, ny)+margin, 0, a.canvas.H-1)
	return
}

func (a *App) rightTrianglePreviewCanvasBounds(rawX, rawY int, constrain bool) (left, top, right, bottom int) {
	if a.canvas == nil || a.canvas.W < 1 || a.canvas.H < 1 {
		return 0, 0, -1, -1
	}
	nx, ny := a.constrainedRightTriangleEndpoint(a.startX, a.startY, rawX, rawY, constrain)
	cornerX, cornerY := rightTriangleLegCorner(a.startX, a.startY, nx, ny)
	size := max(a.rightTriangleSize, 1)
	margin := int(math.Ceil(float64(size)/2.0)) + 3
	minX := min(a.startX, min(nx, cornerX))
	maxX := max(a.startX, max(nx, cornerX))
	minY := min(a.startY, min(ny, cornerY))
	maxY := max(a.startY, max(ny, cornerY))
	left = clamp(minX-margin, 0, a.canvas.W-1)
	right = clamp(maxX+margin, 0, a.canvas.W-1)
	top = clamp(minY-margin, 0, a.canvas.H-1)
	bottom = clamp(maxY+margin, 0, a.canvas.H-1)
	return
}

// rectanglePreviewCanvasBounds includes thick edges and the optional centre
// guide. Restricting both the clone and invalidation to this rectangle avoids
// full-canvas updates and makes the old/new preview change a single BitBlt.
func (a *App) rectanglePreviewCanvasBounds(rawX, rawY int, square bool) (left, top, right, bottom int) {
	if a.canvas == nil || a.canvas.W < 1 || a.canvas.H < 1 {
		return 0, 0, -1, -1
	}
	nx, ny := a.constrainedRectangleEndpoint(a.startX, a.startY, rawX, rawY, square)
	outerLeft, outerTop, outerRight, outerBottom, _, _ := rectangleOuterBounds(a.startX, a.startY, nx, ny)
	margin := int(math.Ceil(float64(max(a.rectangleSize, 1))/2.0)) + 6
	left = clamp(outerLeft-margin, 0, a.canvas.W-1)
	right = clamp(outerRight+margin, 0, a.canvas.W-1)
	top = clamp(outerTop-margin, 0, a.canvas.H-1)
	bottom = clamp(outerBottom+margin, 0, a.canvas.H-1)
	return
}

func (a *App) ellipsePreviewCanvasBounds(rawX, rawY int, circle bool) (left, top, right, bottom int) {
	if a.canvas == nil || a.canvas.W < 1 || a.canvas.H < 1 {
		return 0, 0, -1, -1
	}
	nx, ny := a.constrainedRectangleEndpoint(a.startX, a.startY, rawX, rawY, circle)
	outerLeft, outerTop, outerRight, outerBottom, _, _ := rectangleOuterBounds(a.startX, a.startY, nx, ny)
	margin := int(math.Ceil(float64(max(a.ellipseSize, 1))/2.0)) + 5
	left = clamp(outerLeft-margin, 0, a.canvas.W-1)
	right = clamp(outerRight+margin, 0, a.canvas.W-1)
	top = clamp(outerTop-margin, 0, a.canvas.H-1)
	bottom = clamp(outerBottom+margin, 0, a.canvas.H-1)
	return
}

func (a *App) curvePreviewCanvasBounds() (left, top, right, bottom int) {
	if !a.preview || a.canvas == nil || a.canvas.W < 1 || a.canvas.H < 1 {
		return 0, 0, -1, -1
	}
	polygon := a.curveControlPolygon(a.curveActiveControlCount())
	if len(polygon) == 0 {
		return 0, 0, -1, -1
	}
	minX, maxX := polygon[0].X, polygon[0].X
	minY, maxY := polygon[0].Y, polygon[0].Y
	for _, point := range polygon[1:] {
		minX = min(minX, point.X)
		maxX = max(maxX, point.X)
		minY = min(minY, point.Y)
		maxY = max(maxY, point.Y)
	}
	margin := int(math.Ceil(float64(max(a.curveSize, 1))/2.0)) + 4
	if a.curveAntialias {
		margin += 2
	}
	left = clamp(minX-margin, 0, a.canvas.W-1)
	right = clamp(maxX+margin, 0, a.canvas.W-1)
	top = clamp(minY-margin, 0, a.canvas.H-1)
	bottom = clamp(maxY+margin, 0, a.canvas.H-1)
	return
}

func (a *App) previewCanvasBoundsScreenRect(left, top, right, bottom int) RECT {
	if right < left || bottom < top || a.hwnd == 0 {
		return RECT{}
	}
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	ws, _, _, _ := a.layout(client)
	ox, oy := a.canvasOrigin(ws)
	return expandRECT(RECT{
		Left: int32(ox + left*a.zoom), Top: int32(oy + top*a.zoom),
		Right: int32(ox + (right+1)*a.zoom), Bottom: int32(oy + (bottom+1)*a.zoom),
	}, 2)
}

func (a *App) ellipsePreviewScreenRect(rawX, rawY int, circle bool) RECT {
	left, top, right, bottom := a.ellipsePreviewCanvasBounds(rawX, rawY, circle)
	return a.previewCanvasBoundsScreenRect(left, top, right, bottom)
}

func (a *App) curvePreviewScreenRect() RECT {
	left, top, right, bottom := a.curvePreviewCanvasBounds()
	return a.previewCanvasBoundsScreenRect(left, top, right, bottom)
}

func (a *App) requestEllipsePreviewTransition(oldX, oldY, newX, newY int, circle bool, immediate bool) {
	oldRect := a.ellipsePreviewScreenRect(oldX, oldY, circle)
	newRect := a.ellipsePreviewScreenRect(newX, newY, circle)
	a.requestRepaintRect(unionRECT(oldRect, newRect), immediate)
}

func (a *App) rectanglePreviewScreenRect(rawX, rawY int, square bool) RECT {
	left, top, right, bottom := a.rectanglePreviewCanvasBounds(rawX, rawY, square)
	return a.previewCanvasBoundsScreenRect(left, top, right, bottom)
}

// A1250: all constrained shape transitions invalidate the union of the *old*
// and *new* native-pixel shapes. The existing persistent window backbuffer
// composites the canvas and preview BEFORE the screen receives one BitBlt;
// there must be no intermediate erase/blank frame.
func (a *App) requestConstrainedPreviewTransition(before, after bool, immediate bool) {
	if !a.drawing || !a.preview || before == after {
		return
	}
	var oldRect, newRect RECT
	switch a.tool {
	case ToolLine, ToolArrow, ToolRightTriangle:
		oldRect = a.linePreviewScreenRect(a.curX, a.curY, before)
		newRect = a.linePreviewScreenRect(a.curX, a.curY, after)
	case ToolRectangle:
		oldRect = a.rectanglePreviewScreenRect(a.curX, a.curY, before)
		newRect = a.rectanglePreviewScreenRect(a.curX, a.curY, after)
	case ToolEllipse:
		oldRect = a.ellipsePreviewScreenRect(a.curX, a.curY, before)
		newRect = a.ellipsePreviewScreenRect(a.curX, a.curY, after)
	default:
		return
	}
	a.requestRepaintRect(unionRECT(oldRect, newRect), immediate)
	a.requestStatusRepaint()
}

func (a *App) linePreviewScreenRect(rawX, rawY int, constrain bool) RECT {
	if a.canvas == nil || a.canvas.W < 1 || a.canvas.H < 1 {
		return RECT{}
	}
	var left, top, right, bottom int
	if a.tool == ToolRightTriangle {
		left, top, right, bottom = a.rightTrianglePreviewCanvasBounds(rawX, rawY, constrain)
	} else {
		arrow := a.tool == ToolArrow
		left, top, right, bottom = a.vectorPreviewCanvasBounds(rawX, rawY, constrain, arrow)
	}
	if right < left || bottom < top {
		return RECT{}
	}
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	ws, _, _, _ := a.layout(client)
	ox, oy := a.canvasOrigin(ws)
	rc := RECT{
		Left:   int32(ox + left*a.zoom),
		Top:    int32(oy + top*a.zoom),
		Right:  int32(ox + (right+1)*a.zoom),
		Bottom: int32(oy + (bottom+1)*a.zoom),
	}
	return expandRECT(rc, 2)
}

func (a *App) requestLinePreviewTransition(oldX, oldY, newX, newY int, constrain bool, immediate bool) {
	oldRect := a.linePreviewScreenRect(oldX, oldY, constrain)
	newRect := a.linePreviewScreenRect(newX, newY, constrain)
	a.requestRepaintRect(unionRECT(oldRect, newRect), immediate)
}

func (a *App) requestStatusRepaint() {
	if !a.showStatus || a.bitmapView || a.hwnd == 0 {
		return
	}
	var rc RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	_, _, _, st := a.layout(rc)
	invalidateArea(a.hwnd, &st)
}

func (a *App) requestHistoryRepaint(immediate bool) {
	if a.hwnd == 0 || !a.historyPanelVisible() {
		return
	}
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	panel := a.historyPanelRect(client)
	if panel.Right <= panel.Left || panel.Bottom <= panel.Top {
		return
	}
	invalidateArea(a.hwnd, &panel)
	if immediate {
		procUpdateWindow.Call(a.hwnd)
	}
}

func (a *App) refreshTextComboSurface() {
	if a.hwnd == 0 {
		return
	}
	// An owner-draw combo popup can extend outside the floating panel. When the
	// popup closes, invalidate the full parent immediately so its temporary list
	// surface and any overhanging font glyphs cannot remain as a visual ghost.
	for _, h := range []uintptr{a.textTool.FontCombo, a.textTool.SizeCombo, a.textTool.ScriptCombo} {
		if h != 0 {
			procInvalidateRect.Call(h, 0, 1)
		}
	}
	procInvalidateRect.Call(a.hwnd, 0, 1)
	procUpdateWindow.Call(a.hwnd)
}

func validReturnTool(tool Tool) bool {
	return tool >= ToolFreeSelect && tool <= ToolArrow && tool != ToolEyedropper && tool != ToolMagnifier
}

func (a *App) clearZoomPreview() {
	a.zoomPreviewValid = false
	a.zoomPreviewX = 0
	a.zoomPreviewY = 0
	a.zoomPreviewW = 0
	a.zoomPreviewH = 0
}

func (a *App) rememberMagnifierReturnTool(previous Tool) {
	if previous == ToolEyedropper {
		previous = a.eyedropperReturnTool
	}
	if previous == ToolMagnifier {
		previous = a.magnifierReturnTool
	}
	if !validReturnTool(previous) {
		previous = ToolPencil
	}
	a.magnifierReturnTool = previous
	logf("ZOOM remember returnTool=%d name=%q", previous, toolNames[previous])
}

func (a *App) restoreToolAfterMagnifier(action string) {
	tool := a.magnifierReturnTool
	if !validReturnTool(tool) {
		tool = ToolPencil
	}
	a.tool = tool
	a.clearZoomPreview()
	a.drawing = false
	a.preview = false
	procReleaseCapture.Call()
	if tool == ToolPencil || tool == ToolBrush || tool == ToolAirbrush {
		a.statusText = fmt.Sprintf("%s 통합 붓(%s, %d px)으로 돌아왔습니다.", action, brushShapeNames[a.brushShape], a.brushSize)
	} else {
		a.statusText = fmt.Sprintf("%s %s 도구로 돌아왔습니다.", action, toolNames[tool])
	}
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	a.updateTextUILayout(client)
	a.updateScrollbars()
	invalidate(a.hwnd)
	logf("ZOOM restore tool=%d name=%q action=%q", tool, toolNames[tool], action)
}

func (a *App) defaultZoomTarget() int {
	// The magnifier's selected target is a tool option, not the current view
	// scale. Opening dotPaint stays at 1x; selecting the magnifier highlights
	// 8x without changing the canvas until the user clicks it.
	return chooseDefaultMagnifierZoom(a.zoomLevels)
}

func (a *App) validZoomLevel(z int) bool {
	for _, v := range a.zoomLevels {
		if v == z {
			return true
		}
	}
	return false
}

// zoomSourceRect returns the canvas-space rectangle that will exactly fill the
// fixed workspace at the requested zoom. The rectangle follows the pointer but
// is clamped to the image bounds, matching the classic Paint magnifier model.
func (a *App) zoomSourceRect(centerX, centerY, target int) (x, y, w, h int) {
	if !a.validZoomLevel(target) {
		target = a.defaultZoomTarget()
	}
	var rc RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	ws, _, _, _ := a.layout(rc)
	pageW := int(ws.Right-ws.Left) - 16
	pageH := int(ws.Bottom-ws.Top) - 16
	if pageW < 1 {
		pageW = 1
	}
	if pageH < 1 {
		pageH = 1
	}
	w = (pageW + target - 1) / target
	h = (pageH + target - 1) / target
	w = clamp(w, 1, a.canvas.W)
	h = clamp(h, 1, a.canvas.H)
	x = centerX - w/2
	y = centerY - h/2
	x = clamp(x, 0, max(a.canvas.W-w, 0))
	y = clamp(y, 0, max(a.canvas.H-h, 0))
	return
}

func (a *App) updateZoomPreviewAt(mx, my int) bool {
	oldValid := a.zoomPreviewValid
	oldX, oldY, oldW, oldH := a.zoomPreviewX, a.zoomPreviewY, a.zoomPreviewW, a.zoomPreviewH
	ix, iy, inside := a.clientToImage(mx, my)
	if a.tool != ToolMagnifier || !inside {
		a.zoomPreviewValid = false
		return oldValid
	}
	if !a.validZoomLevel(a.zoomTarget) {
		a.zoomTarget = a.defaultZoomTarget()
	}
	x, y, w, h := a.zoomSourceRect(ix, iy, a.zoomTarget)
	a.zoomPreviewValid = true
	a.zoomPreviewX, a.zoomPreviewY, a.zoomPreviewW, a.zoomPreviewH = x, y, w, h
	return !oldValid || oldX != x || oldY != y || oldW != w || oldH != h
}

func (a *App) applyZoomPreview(mx, my int) {
	if !a.updateZoomPreviewAt(mx, my) || !a.zoomPreviewValid {
		if !a.zoomPreviewValid {
			return
		}
	}
	old := a.zoom
	target := a.zoomTarget
	x, y, w, h := a.zoomPreviewX, a.zoomPreviewY, a.zoomPreviewW, a.zoomPreviewH
	a.zoom = target
	a.updateZoomDependentViewMenu()
	// Align the selected source rectangle with the workspace's inner margin.
	a.scrollX = x * target
	a.scrollY = y * target
	a.updateScrollbars()
	a.updateTextFontUI()
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	a.updateTextUILayout(client)
	a.clearZoomPreview()
	action := fmt.Sprintf("확대 완료: %dx, 선택 영역 (%d,%d) %d × %d.", target, x, y, w, h)
	logf("ZOOM preview-commit old=%d new=%d source=(%d,%d %dx%d) scroll=%d,%d", old, target, x, y, w, h, a.scrollX, a.scrollY)
	a.restoreToolAfterMagnifier(action)
}

func (a *App) paintZoomPreview(hdc uintptr, ox, oy int) {
	if a.tool != ToolMagnifier || !a.zoomPreviewValid {
		return
	}
	l := ox + a.zoomPreviewX*a.zoom
	t := oy + a.zoomPreviewY*a.zoom
	r := l + a.zoomPreviewW*a.zoom
	b := t + a.zoomPreviewH*a.zoom
	guideW := 1
	if a.zoom > 1 {
		guideW = 2
	}
	// Classic Paint-style black dotted rectangle. At enlarged zoom it uses a
	// two-pixel stroke so the pixel-aligned target boundary is easy to follow.
	drawDashedLineWidth(hdc, l, t, r, t, rgb(0, 0, 0), guideW)
	drawDashedLineWidth(hdc, r, t, r, b, rgb(0, 0, 0), guideW)
	drawDashedLineWidth(hdc, r, b, l, b, rgb(0, 0, 0), guideW)
	drawDashedLineWidth(hdc, l, b, l, t, rgb(0, 0, 0), guideW)
}

func (a *App) updateZoomDependentViewMenu() {
	if a.viewMenu == 0 {
		return
	}
	gridEnabled := a.zoom >= 6
	thumbnailEnabled := a.zoom >= 4
	if !gridEnabled && a.showGrid {
		a.showGrid = false
		logf("GRID auto-off zoom=%d threshold=6", a.zoom)
	}
	if !thumbnailEnabled && a.showThumbnail {
		a.showThumbnail = false
		logf("THUMBNAIL auto-close zoom=%d threshold=4", a.zoom)
	}
	gridFlags := uint32(MF_BYCOMMAND)
	if !gridEnabled {
		gridFlags |= MF_GRAYED
	}
	thumbnailFlags := uint32(MF_BYCOMMAND)
	if !thumbnailEnabled {
		thumbnailFlags |= MF_GRAYED
	}
	procEnableMenuItem.Call(a.viewMenu, uintptr(ID_VIEW_GRID), uintptr(gridFlags))
	procEnableMenuItem.Call(a.viewMenu, uintptr(ID_VIEW_THUMBNAIL), uintptr(thumbnailFlags))
	a.toggleMenuCheck(ID_VIEW_GRID, a.showGrid)
	a.toggleMenuCheck(ID_VIEW_THUMBNAIL, a.showThumbnail)
	if a.hwnd != 0 {
		procDrawMenuBar.Call(a.hwnd)
	}
}

func (a *App) setZoom(z int, anchorX, anchorY int) {
	if !a.validZoomLevel(z) {
		return
	}
	old := a.zoom
	if old == z {
		return
	}
	// Keep current viewport center approximately stable.
	var rc RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	ws, _, _, _ := a.layout(rc)
	if anchorX < 0 {
		anchorX = int(ws.Left + (ws.Right-ws.Left)/2)
	}
	if anchorY < 0 {
		anchorY = int(ws.Top + (ws.Bottom-ws.Top)/2)
	}
	imgX := float64(a.scrollX+anchorX-int(ws.Left)-8) / float64(old)
	imgY := float64(a.scrollY+anchorY-int(ws.Top)-8) / float64(old)
	a.zoom = z
	a.zoomPreviewValid = false
	a.updateZoomDependentViewMenu()
	a.scrollX = int(imgX*float64(z)) - anchorX + int(ws.Left) + 8
	a.scrollY = int(imgY*float64(z)) - anchorY + int(ws.Top) + 8
	a.updateScrollbars()
	a.updateTextFontUI()
	a.updateTextUILayout(rc)
	// Only the canvas viewport is repainted. Toolbox, palette, status bar and
	// menu retain their Windows 11 size and position at every zoom level.
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	ws, tb, _, st := a.layout(client)
	invalidateArea(a.hwnd, &ws)
	if a.showToolbox && !a.bitmapView {
		invalidateArea(a.hwnd, &tb) // refresh selected zoom label only; size is fixed
	}
	if !a.curveUndoPending {
		a.statusText = fmt.Sprintf("그림 영역 확대: %dx (메뉴와 도구는 고정)", z)
	}
	if a.showStatus && !a.bitmapView {
		invalidateArea(a.hwnd, &st)
	}
	if a.thumbnailPanelVisible() {
		thumbnail := a.thumbnailPanelRect(client)
		invalidateArea(a.hwnd, &thumbnail)
	}
	logf("ZOOM canvas-only old=%d new=%d scroll=%d,%d viewport=(%d,%d)-(%d,%d)", old, z, a.scrollX, a.scrollY, ws.Left, ws.Top, ws.Right, ws.Bottom)
}

func (a *App) setZoomTopLeft(z int) {
	if !a.validZoomLevel(z) || z == a.zoom {
		return
	}
	old := a.zoom
	x, y := a.viewportTopLeftImage()
	a.zoom = z
	a.zoomPreviewValid = false
	a.updateZoomDependentViewMenu()
	a.scrollX = x * z
	a.scrollY = y * z
	a.updateScrollbars()
	a.updateTextFontUI()
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	a.updateTextUILayout(client)
	a.statusText = fmt.Sprintf("축소 완료: 화면 왼쪽 위 픽셀 (%d,%d)을 유지하고 %dx로 변경했습니다.", x, y, z)
	logf("ZOOM top-left old=%d new=%d pixel=%d,%d scroll=%d,%d", old, z, x, y, a.scrollX, a.scrollY)
	invalidate(a.hwnd)
}

func (a *App) nextZoom(dir int, anchorX, anchorY int) {
	idx := 0
	for i, v := range a.zoomLevels {
		if v == a.zoom {
			idx = i
			break
		}
	}
	idx = clamp(idx+dir, 0, len(a.zoomLevels)-1)
	if dir < 0 {
		a.setZoomTopLeft(a.zoomLevels[idx])
	} else {
		a.setZoom(a.zoomLevels[idx], anchorX, anchorY)
	}
}

func editorDPIValue(hwnd uintptr, value int) int {
	dpi := uintptr(96)
	if procGetDpiForWindow.Find() == nil {
		if got, _, _ := procGetDpiForWindow.Call(hwnd); got >= 48 && got <= 768 {
			dpi = got
		}
	}
	return max(1, (value*int(dpi)+48)/96)
}

func ownerDrawPushButtonStyle(class string, style uintptr) uintptr {
	if !strings.EqualFold(class, "BUTTON") {
		return style
	}
	buttonType := style & 0x0F
	if buttonType == 0 || buttonType == 1 {
		return (style &^ 0x0F) | BS_OWNERDRAW
	}
	return style
}

func themedOwnerDrawPushButtonColors(dis *DRAWITEMSTRUCT) (bg, border, textColor, focusColor uint32) {
	dark := darkModeEnabled()
	bg = rgb(240, 240, 240)
	border = rgb(145, 145, 145)
	textColor = rgb(20, 20, 20)
	focusColor = rgb(0, 95, 190)
	if dark {
		bg = rgb(52, 52, 52)
		border = rgb(96, 96, 96)
		textColor = rgb(238, 238, 238)
		focusColor = rgb(85, 170, 255)
	}
	checked := sendMessage(dis.HwndItem, BM_GETCHECK, 0, 0) == BST_CHECKED
	if checked && dis.ItemState&ODS_SELECTED == 0 {
		if dark {
			bg = rgb(78, 78, 78)
			border = rgb(150, 150, 150)
		} else {
			bg = rgb(205, 225, 245)
			border = rgb(70, 120, 170)
		}
	}
	if dis.ItemState&ODS_SELECTED != 0 {
		if dark {
			bg = rgb(36, 36, 36)
			border = rgb(125, 125, 125)
		} else {
			bg = rgb(218, 218, 218)
			border = rgb(105, 105, 105)
		}
	}
	if dis.ItemState&(ODS_DISABLED|ODS_GRAYED) != 0 {
		if dark {
			bg = rgb(43, 43, 43)
			border = rgb(72, 72, 72)
			textColor = rgb(126, 126, 126)
		} else {
			bg = rgb(238, 238, 238)
			border = rgb(190, 190, 190)
			textColor = rgb(145, 145, 145)
		}
	}
	return
}

func paintThemedOwnerDrawPushButtonBase(dis *DRAWITEMSTRUCT) (textColor, focusColor uint32, dx, dy int, ok bool) {
	if dis == nil || dis.CtlType != ODT_BUTTON || dis.HDC == 0 || dis.HwndItem == 0 {
		return 0, 0, 0, 0, false
	}
	bg, border, textColor, focusColor := themedOwnerDrawPushButtonColors(dis)
	fillRect(dis.HDC, dis.RcItem, bg)
	frameRect(dis.HDC, dis.RcItem, border)
	if dis.ItemState&ODS_SELECTED != 0 {
		dx, dy = 1, 1
	}
	return textColor, focusColor, dx, dy, true
}

func paintThemedOwnerDrawPushButtonFocus(dis *DRAWITEMSTRUCT, focusColor uint32) {
	if dis == nil || dis.ItemState&ODS_FOCUS == 0 {
		return
	}
	fr := dis.RcItem
	fr.Left += 3
	fr.Top += 3
	fr.Right -= 3
	fr.Bottom -= 3
	frameRect(dis.HDC, fr, focusColor)
}

func drawThemedOwnerDrawPushButton(dis *DRAWITEMSTRUCT) bool {
	textColor, focusColor, dx, dy, ok := paintThemedOwnerDrawPushButtonBase(dis)
	if !ok {
		return false
	}

	font := sendMessage(dis.HwndItem, WM_GETFONT, 0, 0)
	oldFont := uintptr(0)
	if font != 0 {
		oldFont, _, _ = procSelectObject.Call(dis.HDC, font)
	}
	procSetBkMode.Call(dis.HDC, TRANSPARENT)
	procSetTextColor.Call(dis.HDC, uintptr(textColor))
	rc := dis.RcItem
	rc.Left += int32(dx)
	rc.Top += int32(dy)
	rc.Right += int32(dx)
	rc.Bottom += int32(dy)
	title := getWindowTextString(dis.HwndItem)
	if title != "" {
		drawText(dis.HDC, title, rc, DT_CENTER|DT_VCENTER|DT_SINGLELINE, textColor)
	}
	if oldFont != 0 {
		procSelectObject.Call(dis.HDC, oldFont)
	}
	paintThemedOwnerDrawPushButtonFocus(dis, focusColor)
	return true
}

var nativeDarkCaptionColorSupport atomic.Int32 // 0 fallback/not-yet-confirmed, +1 native DWM caption

// editorHasNativeDarkCaption asks Windows whether it can own the complete dark
// non-client frame. Windows 11 supports DWMWA_CAPTION_COLOR directly; when that
// succeeds, painting our own caption is both redundant and harmful because a
// WindowDC can overlap the first client rows on some DPI/frame combinations.
// Older systems that reject the attribute keep the existing manual fallback.
func editorHasNativeDarkCaption(hwnd uintptr) bool {
	return hwnd != 0 && nativeDarkCaptionColorSupport.Load() > 0
}

func drawDarkEditorCaption(hwnd uintptr) {
	if hwnd == 0 || !darkModeEnabled() {
		return
	}
	// On Windows 11, DWM already paints the title bar, icon, caption buttons and
	// bottom boundary. Do not paint a second pseudo-titlebar into WindowDC.
	if editorHasNativeDarkCaption(hwnd) {
		return
	}
	var wr RECT
	if ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr))); ok == 0 {
		return
	}
	width := int(wr.Right - wr.Left)
	if width <= 0 {
		return
	}
	hdc, _, _ := procGetWindowDC.Call(hwnd)
	if hdc == 0 {
		return
	}
	defer procReleaseDC.Call(hwnd, hdc)

	metric := func(index, fallback int) int {
		if procGetSystemMetrics.Find() == nil {
			if got, _, _ := procGetSystemMetrics.Call(uintptr(index)); int(got) > 0 {
				return int(got)
			}
		}
		return editorDPIValue(hwnd, fallback)
	}
	frameX := metric(SM_CXFRAME, 8) + metric(SM_CXPADDEDBORDER, 0)
	frameY := metric(SM_CYFRAME, 8) + metric(SM_CXPADDEDBORDER, 0)
	captionH := metric(SM_CYCAPTION, 23)
	buttonW := metric(SM_CXSIZE, 45)
	buttonH := metric(SM_CYSIZE, 29)
	// DWM's actual Windows 11 frame can be several pixels shorter than the
	// SM_CYSIZE estimate. The former max(..., buttonH+frameY) therefore painted
	// the forced caption into the first client rows and left a horizontal
	// afterimage below the title bar. Measure the real client origin and stop
	// the custom chrome exactly at that boundary.
	chromeH := frameY + captionH
	var clientOrigin POINT
	if ok, _, _ := procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&clientOrigin))); ok != 0 {
		if measured := int(clientOrigin.Y - wr.Top); measured > 0 && measured < int(wr.Bottom-wr.Top) {
			chromeH = measured
		}
	}

	caption := RECT{0, 0, int32(width), int32(chromeH)}
	fillRect(hdc, caption, rgb(28, 28, 28))
	separator := RECT{0, int32(chromeH - 1), int32(width), int32(chromeH)}
	fillRect(hdc, separator, rgb(63, 63, 63))

	iconSize := editorDPIValue(hwnd, 16)
	iconX := frameX + editorDPIValue(hwnd, 5)
	iconY := frameY + max(0, (captionH-iconSize)/2)
	icon := sendMessage(hwnd, WM_GETICON, ICON_SMALL, 0)
	if icon == 0 && app != nil {
		icon, _, _ = procLoadImageW.Call(app.hinst, IDI_DOTPAINT, IMAGE_ICON, uintptr(iconSize), uintptr(iconSize), LR_SHARED)
	}
	if icon != 0 {
		procDrawIconEx.Call(hdc, uintptr(iconX), uintptr(iconY), icon, uintptr(iconSize), uintptr(iconSize), 0, 0, DI_NORMAL)
	}

	right := width - frameX
	closeLeft := right - buttonW
	maxLeft := closeLeft - buttonW
	minLeft := maxLeft - buttonW
	buttonTop := frameY
	buttonBottom := min(max(chromeH-1, buttonTop+1), buttonTop+buttonH)
	for _, rc := range []RECT{
		{int32(minLeft), int32(buttonTop), int32(maxLeft), int32(buttonBottom)},
		{int32(maxLeft), int32(buttonTop), int32(closeLeft), int32(buttonBottom)},
		{int32(closeLeft), int32(buttonTop), int32(right), int32(buttonBottom)},
	} {
		fillRect(hdc, rc, rgb(28, 28, 28))
	}

	pen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(editorDPIValue(hwnd, 1)), uintptr(rgb(235, 235, 235)))
	oldPen, _, _ := procSelectObject.Call(hdc, pen)
	cy := buttonTop + (buttonBottom-buttonTop)/2
	// Minimize glyph.
	procMoveToEx.Call(hdc, uintptr(minLeft+buttonW/2-editorDPIValue(hwnd, 5)), uintptr(cy+editorDPIValue(hwnd, 4)), 0)
	procLineTo.Call(hdc, uintptr(minLeft+buttonW/2+editorDPIValue(hwnd, 5)), uintptr(cy+editorDPIValue(hwnd, 4)))
	// Maximize glyph.
	box := editorDPIValue(hwnd, 9)
	maxCX := maxLeft + buttonW/2
	procMoveToEx.Call(hdc, uintptr(maxCX-box/2), uintptr(cy-box/2), 0)
	procLineTo.Call(hdc, uintptr(maxCX+box/2), uintptr(cy-box/2))
	procLineTo.Call(hdc, uintptr(maxCX+box/2), uintptr(cy+box/2))
	procLineTo.Call(hdc, uintptr(maxCX-box/2), uintptr(cy+box/2))
	procLineTo.Call(hdc, uintptr(maxCX-box/2), uintptr(cy-box/2))
	// Close glyph.
	cross := editorDPIValue(hwnd, 5)
	closeCX := closeLeft + buttonW/2
	procMoveToEx.Call(hdc, uintptr(closeCX-cross), uintptr(cy-cross), 0)
	procLineTo.Call(hdc, uintptr(closeCX+cross), uintptr(cy+cross))
	procMoveToEx.Call(hdc, uintptr(closeCX+cross), uintptr(cy-cross), 0)
	procLineTo.Call(hdc, uintptr(closeCX-cross), uintptr(cy+cross))
	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(pen)

	font, _, _ := procGetStockObject.Call(DEFAULT_GUI_FONT)
	oldFont, _, _ := procSelectObject.Call(hdc, font)
	procSetBkMode.Call(hdc, TRANSPARENT)
	procSetTextColor.Call(hdc, uintptr(rgb(240, 240, 240)))
	titleLeft := iconX + iconSize + editorDPIValue(hwnd, 6)
	titleRect := RECT{int32(titleLeft), int32(frameY), int32(max(titleLeft, minLeft-editorDPIValue(hwnd, 6))), int32(frameY + captionH)}
	drawText(hdc, getWindowTextString(hwnd), titleRect, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS|DT_NOPREFIX, rgb(240, 240, 240))
	procSelectObject.Call(hdc, oldFont)
}

func scheduleDarkEditorChromeRefresh(hwnd uintptr) {
	if hwnd == 0 || !darkModeEnabled() {
		return
	}
	// Native Windows 11 caption colouring needs no forced repaint timer. Keeping
	// these timers alive was enough to redraw the old manual caption over the
	// first client scanlines after the real frame had already settled.
	if editorHasNativeDarkCaption(hwnd) {
		procKillTimer.Call(hwnd, darkEditorChromeTimerID)
		procKillTimer.Call(hwnd, darkEditorChromeLateTimerID)
		return
	}
	// Windows may repaint a newly-created caption after DWM attributes are set.
	// A short one-shot timer runs after the first visible frame and redraws the
	// forced dark caption again, avoiding the former maximize/restore workaround.
	procKillTimer.Call(hwnd, darkEditorChromeTimerID)
	procKillTimer.Call(hwnd, darkEditorChromeLateTimerID)
	procSetTimer.Call(hwnd, darkEditorChromeTimerID, 30, 0)
	procSetTimer.Call(hwnd, darkEditorChromeLateTimerID, 220, 0)
	drawDarkEditorCaption(hwnd)
}

// showThemedEditorWindow presents a top-level editor only after its final DWM
// frame and client layout are ready.  Creating these windows with WS_VISIBLE
// let Windows briefly paint child controls against the pre-dark caption
// metrics.  Applying the dark frame then moved the client origin downward and
// left the first few pixels of those controls below the title bar.  A hidden
// first layout plus one full client invalidation matches the clean light-mode
// first frame and prevents that stale strip in every themed editor.
func showThemedEditorWindow(hwnd uintptr, dark bool) {
	if hwnd == 0 {
		return
	}
	applyDarkThemeToWindow(hwnd, dark)
	procInvalidateRect.Call(hwnd, 0, 1)
	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)
	scheduleDarkEditorChromeRefresh(hwnd)
}

func handleDarkEditorNonClient(hwnd uintptr, msg uint32, wparam, lparam uintptr) (uintptr, bool) {
	// When DWM accepts native caption colours, Windows must exclusively own the
	// non-client area. Returning false lets the editor WndProc fall through to
	// DefWindowProc instead of layering our legacy fallback titlebar on top.
	if darkModeEnabled() && editorHasNativeDarkCaption(hwnd) {
		return 0, false
	}
	if msg == WM_TIMER && (wparam == darkEditorChromeTimerID || wparam == darkEditorChromeLateTimerID) {
		procKillTimer.Call(hwnd, wparam)
		if darkModeEnabled() {
			// The native dark frame was finalized before the first ShowWindow.
			// Reapplying it here used SWP_FRAMECHANGED after the children were
			// visible, shifted the client origin and recreated the toolbar ghost
			// strip that the timer was supposed to hide. The timer only needs to
			// restore our caption pixels after a possible DWM repaint.
			drawDarkEditorCaption(hwnd)
		}
		return 0, true
	}
	if !darkModeEnabled() {
		return 0, false
	}
	switch msg {
	case WM_NCPAINT, WM_NCACTIVATE, WM_SETTEXT, WM_SETICON:
		r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
		drawDarkEditorCaption(hwnd)
		return r, true
	}
	return 0, false
}

var (
	nativeDarkModeOnce                sync.Once
	nativeAllowDarkModeForWindow      uintptr
	nativeSetPreferredAppMode         uintptr
	nativeRefreshImmersiveColorPolicy uintptr
)

func initNativeDarkModeSupport() {
	nativeDarkModeOnce.Do(func() {
		module, _, loadErr := procLoadLibraryW.Call(uintptr(unsafe.Pointer(utf16Ptr("uxtheme.dll"))))
		if module == 0 {
			logf("DARK_NATIVE uxtheme load failed: %v", loadErr)
			return
		}
		// These stable ordinals are used by Windows 10 1903+ and Windows 11.
		// Resolve them dynamically so older Windows versions keep the existing
		// client-area dark painting without failing to start.
		nativeRefreshImmersiveColorPolicy, _, _ = procGetProcAddress.Call(module, 104)
		nativeAllowDarkModeForWindow, _, _ = procGetProcAddress.Call(module, 133)
		nativeSetPreferredAppMode, _, _ = procGetProcAddress.Call(module, 135)
		if nativeSetPreferredAppMode != 0 {
			// 1 = AllowDark. This permits native controls to use their dark visual
			// style, but does not force dark rendering while dotPaint is in light mode.
			syscall.SyscallN(nativeSetPreferredAppMode, 1)
		}
		if nativeRefreshImmersiveColorPolicy != 0 {
			syscall.SyscallN(nativeRefreshImmersiveColorPolicy)
		}
		logf("DARK_NATIVE initialized allowWindow=%v preferredApp=%v refreshPolicy=%v",
			nativeAllowDarkModeForWindow != 0,
			nativeSetPreferredAppMode != 0,
			nativeRefreshImmersiveColorPolicy != 0)
	})
}

func getWindowClassNameString(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}
	buf := make([]uint16, 128)
	n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:int(n)])
}

func darkControlThemeName(hwnd uintptr) string {
	switch strings.ToUpper(getWindowClassNameString(hwnd)) {
	case "COMBOBOX", "COMBOLBOX":
		return "DarkMode_CFD"
	case "EDIT", "RICHEDIT20W", "RICHEDIT50W":
		// Explorer supplies the dark non-client scrollbar used by multiline
		// edit controls. DarkMode_CFD leaves the embedded scrollbar bright on
		// some Windows 10/11 builds even though the edit background is dark.
		return "DarkMode_Explorer"
	default:
		return "DarkMode_Explorer"
	}
}

func setDwmColorAttribute(hwnd uintptr, attr uintptr, color uint32) bool {
	if hwnd == 0 || procDwmSetWindowAttribute.Find() != nil {
		return false
	}
	result, _, _ := procDwmSetWindowAttribute.Call(hwnd, attr, uintptr(unsafe.Pointer(&color)), unsafe.Sizeof(color))
	return int32(result) >= 0
}

func applyDarkThemeToWindow(hwnd uintptr, dark bool) {
	if hwnd == 0 {
		return
	}
	initNativeDarkModeSupport()
	value := int32(0)
	if dark {
		value = 1
	}
	if nativeAllowDarkModeForWindow != 0 {
		arg := uintptr(0)
		if dark {
			arg = 1
		}
		syscall.SyscallN(nativeAllowDarkModeForWindow, hwnd, arg)
	}
	if procDwmSetWindowAttribute.Find() == nil {
		result, _, _ := procDwmSetWindowAttribute.Call(hwnd, DWMWA_USE_IMMERSIVE_DARK_MODE, uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value))
		if int32(result) < 0 {
			// Windows 10 1809 used attribute 19; Windows 10 1903+ and Windows 11 use 20.
			procDwmSetWindowAttribute.Call(hwnd, DWMWA_USE_IMMERSIVE_DARK_MODE_OLD, uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value))
		}
		if dark {
			setDwmColorAttribute(hwnd, DWMWA_BORDER_COLOR, rgb(62, 62, 62))
			if setDwmColorAttribute(hwnd, DWMWA_CAPTION_COLOR, rgb(30, 30, 30)) {
				// Windows 11 can own the whole caption. Once confirmed, all editor
				// windows stop using the legacy WindowDC caption fallback.
				nativeDarkCaptionColorSupport.Store(1)
			}
			setDwmColorAttribute(hwnd, DWMWA_TEXT_COLOR, rgb(238, 238, 238))
		} else {
			setDwmColorAttribute(hwnd, DWMWA_BORDER_COLOR, DWMWA_COLOR_DEFAULT)
			setDwmColorAttribute(hwnd, DWMWA_CAPTION_COLOR, DWMWA_COLOR_DEFAULT)
			setDwmColorAttribute(hwnd, DWMWA_TEXT_COLOR, DWMWA_COLOR_DEFAULT)
		}
	}
	if procSetWindowTheme.Find() == nil {
		if dark {
			theme := darkControlThemeName(hwnd)
			procSetWindowTheme.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(theme))), 0)
		} else {
			procSetWindowTheme.Call(hwnd, 0, 0)
		}
	}
	// Recalculate non-client metrics immediately so an already-open editor
	// switches its caption, border, buttons and combo boxes without reopening.
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
		SWP_NOMOVE|SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE|SWP_FRAMECHANGED)
}

func (a *App) applyWindowTheme() {
	applyDarkThemeToWindow(a.hwnd, a.darkMode)
	if a.darkMode && a.darkBrush == 0 {
		a.darkBrush, _, _ = procCreateSolidBrush.Call(uintptr(rgb(30, 30, 30)))
	}
}

func (a *App) applyDarkMenuBackground(menu uintptr) {
	if !a.darkMode || menu == 0 {
		return
	}
	if a.menuBrush == 0 {
		a.menuBrush, _, _ = procCreateSolidBrush.Call(uintptr(rgb(28, 28, 28)))
	}
	mi := MENUINFO{
		CbSize:  uint32(unsafe.Sizeof(MENUINFO{})),
		FMask:   MIM_BACKGROUND | MIM_APPLYTOSUBMENUS,
		HbrBack: a.menuBrush,
	}
	procSetMenuInfo.Call(menu, uintptr(unsafe.Pointer(&mi)))
}

func (a *App) rebuildMenuForTheme() {
	oldMenu := a.menu
	oldMenuBrush := a.menuBrush
	a.menu = 0
	a.menuBrush = 0
	a.buildMenu()
	if oldMenu != 0 {
		procDestroyMenu.Call(oldMenu)
	}
	if oldMenuBrush != 0 {
		procDeleteObject.Call(oldMenuBrush)
	}
	procDrawMenuBar.Call(a.hwnd)
}

func themeWindowHandles(dark bool, handles ...uintptr) {
	for _, hwnd := range handles {
		if hwnd == 0 {
			continue
		}
		applyDarkThemeToWindow(hwnd, dark)
		procInvalidateRect.Call(hwnd, 0, 1)
	}
}

func colorDialogConfigureChild(hwnd, lparam uintptr) uintptr {
	if hwnd == 0 || lparam == 0 {
		return 1
	}
	class := getWindowClassNameString(hwnd)
	style, _, _ := procGetWindowLongW.Call(hwnd, uintptr(GWL_STYLE))
	flatStyle := ownerDrawPushButtonStyle(class, style)
	if flatStyle != style {
		procSetWindowLongW.Call(hwnd, uintptr(GWL_STYLE), flatStyle)
	}
	if darkModeEnabled() {
		applyDarkThemeToWindow(hwnd, true)
	}
	procInvalidateRect.Call(hwnd, 0, 1)
	return 1
}

var colorDialogConfigureChildCallback uintptr // legacy ChooseColor hook is inactive in 1.2.45

const (
	colorDialogDarkRefreshTimerID     = 0xA235
	colorDialogDarkLateRefreshTimerID = 0xA236
)

func colorDialogDarkBrush() uintptr {
	if app == nil {
		return 0
	}
	if app.darkBrush == 0 {
		app.darkBrush, _, _ = procCreateSolidBrush.Call(uintptr(rgb(30, 30, 30)))
	}
	return app.darkBrush
}

func refreshColorDialogTheme(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	if darkModeEnabled() {
		// Apply the native dark frame without SWP_FRAMECHANGED. ChooseColor is
		// already visible when the delayed refresh runs, and changing its frame
		// geometry at that point can leave the same title-bar-bottom afterimage
		// that older custom dialogs exhibited.
		initNativeDarkModeSupport()
		if nativeAllowDarkModeForWindow != 0 {
			syscall.SyscallN(nativeAllowDarkModeForWindow, hwnd, 1)
		}
		value := int32(1)
		if procDwmSetWindowAttribute.Find() == nil {
			result, _, _ := procDwmSetWindowAttribute.Call(hwnd, DWMWA_USE_IMMERSIVE_DARK_MODE, uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value))
			if int32(result) < 0 {
				procDwmSetWindowAttribute.Call(hwnd, DWMWA_USE_IMMERSIVE_DARK_MODE_OLD, uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value))
			}
			setDwmColorAttribute(hwnd, DWMWA_BORDER_COLOR, rgb(62, 62, 62))
			setDwmColorAttribute(hwnd, DWMWA_CAPTION_COLOR, rgb(30, 30, 30))
			setDwmColorAttribute(hwnd, DWMWA_TEXT_COLOR, rgb(238, 238, 238))
		}
		// The common-dialog visual style can repaint its light template after
		// WM_INITDIALOG. Disable that client theme on the parent; the title bar
		// remains dark through DWM, while WM_CTLCOLOR below owns the client
		// background. Child controls receive their individual dark themes.
		if procSetWindowTheme.Find() == nil {
			empty := utf16Ptr("")
			procSetWindowTheme.Call(hwnd, uintptr(unsafe.Pointer(empty)), uintptr(unsafe.Pointer(empty)))
		}
	}
	procEnumChildWindows.Call(hwnd, colorDialogConfigureChildCallback, 1)
	procInvalidateRect.Call(hwnd, 0, 1)
}

// colorDialogHookProc gives the native ChooseColor dialog the same flat square
// push buttons as dotPaint's own editors in both themes, and applies the dark
// control palette when needed. The system still owns all
// colour swatches, the spectrum, luminance slider and RGB/HSL calculations, so
// changing the UI theme cannot alter the selected colour or custom-colour data.
func colorDialogHookProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch uint32(msg) {
	case WM_INITDIALOG:
		refreshColorDialogTheme(hwnd)
		colorDialogInitPaletteFooter(hwnd)
		if darkModeEnabled() {
			// ChooseColor finishes part of its native initialization after the hook
			// returns. Reapply once after the first visible frame and once after the
			// late common-dialog repaint so the light template cannot roll the
			// window back to white.
			procSetTimer.Call(hwnd, colorDialogDarkRefreshTimerID, 30, 0)
			procSetTimer.Call(hwnd, colorDialogDarkLateRefreshTimerID, 220, 0)
		}
		logf("COLOR_DIALOG flat-buttons initialized hwnd=%#x dark=%v", hwnd, darkModeEnabled())
		return 0
	case WM_TIMER:
		if wparam == colorDialogDarkRefreshTimerID || wparam == colorDialogDarkLateRefreshTimerID {
			procKillTimer.Call(hwnd, wparam)
			refreshColorDialogTheme(hwnd)
			logf("COLOR_DIALOG dark refresh hwnd=%#x timer=%#x", hwnd, wparam)
			return 1
		}
	case WM_COMMAND:
		if colorDialogHandlePaletteCommand(hwnd, uintptr(loword(wparam))) {
			return 1
		}
	case WM_DRAWITEM:
		if lparam != 0 && colorDialogDrawPaletteSwatch(hwnd, (*DRAWITEMSTRUCT)(unsafe.Pointer(lparam))) {
			return 1
		}
		if lparam != 0 && drawThemedOwnerDrawPushButton((*DRAWITEMSTRUCT)(unsafe.Pointer(lparam))) {
			return 1
		}
	case WM_ERASEBKGND:
		if darkModeEnabled() && wparam != 0 {
			var client RECT
			procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
			fillRect(wparam, client, rgb(30, 30, 30))
			return 1
		}
	case WM_CTLCOLOREDIT, WM_CTLCOLORLISTBOX:
		if darkModeEnabled() && wparam != 0 {
			procSetBkMode.Call(wparam, OPAQUE)
			procSetTextColor.Call(wparam, uintptr(rgb(240, 240, 240)))
			procSetBkColor.Call(wparam, uintptr(rgb(30, 30, 30)))
			return colorDialogDarkBrush()
		}
	case WM_CTLCOLORSTATIC, WM_CTLCOLORBTN, WM_CTLCOLORSCROLLBAR, WM_CTLCOLORDLG:
		if darkModeEnabled() && wparam != 0 {
			procSetBkMode.Call(wparam, OPAQUE)
			procSetTextColor.Call(wparam, uintptr(rgb(235, 235, 235)))
			procSetBkColor.Call(wparam, uintptr(rgb(30, 30, 30)))
			return colorDialogDarkBrush()
		}
	case WM_DESTROY:
		procKillTimer.Call(hwnd, colorDialogDarkRefreshTimerID)
		procKillTimer.Call(hwnd, colorDialogDarkLateRefreshTimerID)
		if f := activeColorPaletteFooter; f != nil && f.hwnd == hwnd && f.font != 0 {
			procDeleteObject.Call(f.font)
			f.font = 0
		}
	}
	return 0
}

var colorDialogHookCallback uintptr // legacy ChooseColor hook is inactive in 1.2.45

func (a *App) refreshOpenEditorsForTheme() {
	// The text toolbox lives inside the main window rather than in a separate
	// editor. Theme its combos and owner-drawn B/I/U buttons explicitly.
	themeWindowHandles(a.darkMode, a.textTool.FontCombo, a.textTool.SizeCombo, a.textTool.ScriptCombo,
		a.textTool.BoldBtn, a.textTool.ItalicBtn, a.textTool.UnderlineBtn)
	if s := activeFormulaEditor; s != nil && s.hwnd != 0 {
		themeWindowHandles(a.darkMode, s.hwnd)
		for _, hwnd := range s.controls {
			themeWindowHandles(a.darkMode, hwnd)
		}
		for _, hwnd := range s.categoryButtons {
			themeWindowHandles(a.darkMode, hwnd)
		}
		s.invert = a.darkMode
		formulaUpdateInvertButton(s)
		formulaRefreshPreview(s)
		scheduleDarkEditorChromeRefresh(s.hwnd)
	}
	if s := activeFunctionGraphEditor; s != nil && s.hwnd != 0 {
		themeWindowHandles(a.darkMode,
			s.hwnd, s.input, s.functionCombo, s.angleCombo, s.axisAddBtn, s.axisRemoveBtn,
			s.xMinEdit, s.xMaxEdit, s.yMinEdit, s.yMaxEdit, s.proportionalCheck, s.invertBtn,
			s.copyPreview, s.copyScript, s.insertBtn, s.cancelBtn, s.statusLabel,
			s.polarSampleLabel, s.polarSampleCombo, s.polarSampleDesc,
			s.axisLabel, s.angleLabel, s.xMinLabel, s.xMaxLabel, s.yMinLabel, s.yMaxLabel)
		for _, hwnd := range s.toolbar {
			themeWindowHandles(a.darkMode, hwnd)
		}
		s.invert = a.darkMode
		graphUpdateInvertButton(s)
		graphRefreshPreview(s)
		scheduleDarkEditorChromeRefresh(s.hwnd)
	}
	if s := activeMultidimShapeEditor; s != nil && s.hwnd != 0 {
		themeWindowHandles(a.darkMode,
			s.hwnd, s.shapeLabel, s.shapeCombo, s.opacityLabel, s.opacityCombo,
			s.smoothCheck, s.invertCheck, s.previewBox,
			s.statusLabel, s.insertBtn, s.cancelBtn,
			s.xLabel, s.xSlider, s.xValueLabel, s.yLabel, s.ySlider, s.yValueLabel,
			s.zLabel, s.zSlider, s.zValueLabel, s.wLabel, s.wSlider, s.wValueLabel,
			s.sizeLabel, s.sizeSlider, s.sizeValueLabel,
			s.widthLabel, s.widthSlider, s.widthValueLabel,
			s.heightLabel, s.heightSlider, s.heightValueLabel,
			s.depthLabel, s.depthSlider, s.depthValueLabel)
		// A231 intentionally follows the active UI theme for preview contrast:
		// dark mode enables final RGB inversion, light mode disables it.
		check := uintptr(BST_UNCHECKED)
		if a.darkMode {
			check = BST_CHECKED
		}
		sendMessage(s.invertCheck, BM_SETCHECK, check, 0)
		multidimInvalidatePreview(s)
		scheduleDarkEditorChromeRefresh(s.hwnd)
	}
	if s := activeCircuitLibrary; s != nil && s.hwnd != 0 {
		themeWindowHandles(a.darkMode, s.hwnd, s.insert, s.close)
		scheduleDarkEditorChromeRefresh(s.hwnd)
		procInvalidateRect.Call(s.hwnd, 0, 1)
	}
	if s := activeAnimalLibrary; s != nil && s.hwnd != 0 {
		themeWindowHandles(a.darkMode, s.hwnd, s.insert, s.close)
		scheduleDarkEditorChromeRefresh(s.hwnd)
		procInvalidateRect.Call(s.hwnd, 0, 1)
	}
	if s := activeCustomImageLibrary; s != nil && s.hwnd != 0 {
		themeWindowHandles(a.darkMode, s.hwnd, s.save, s.insert, s.delete, s.close)
		scheduleDarkEditorChromeRefresh(s.hwnd)
		procInvalidateRect.Call(s.hwnd, 0, 1)
	}
	if s := activeDejavuTileBrowser; s != nil && s.hwnd != 0 {
		themeWindowHandles(a.darkMode, s.hwnd)
		scheduleDarkEditorChromeRefresh(s.hwnd)
		procInvalidateRect.Call(s.hwnd, 0, 1)
	}
	if s := activeHelpWindow; s != nil && s.hwnd != 0 {
		helpRefreshTheme(s)
	}
	if s := activeAboutWindow; s != nil && s.hwnd != 0 {
		aboutRefreshTheme(s)
	}

}

func canvasIsSolidColor(c *Canvas, clr uint32) bool {
	if c == nil || c.W < 1 || c.H < 1 || len(c.Pix) != c.W*c.H*4 {
		return false
	}
	px := colorrefToBGRA(clr)
	for i := 0; i < len(c.Pix); i += 4 {
		if c.Pix[i] != px[0] || c.Pix[i+1] != px[1] || c.Pix[i+2] != px[2] || c.Pix[i+3] != px[3] {
			return false
		}
	}
	return true
}

func (a *App) toggleDarkMode() {
	a.darkMode = !a.darkMode
	if a.darkMode {
		a.statusText = "다크 모드를 켰습니다. 캔버스 색은 유지하고 수식·그래프·다차원 편집기의 검은 배경을 기본으로 켭니다."
	} else {
		a.statusText = "다크 모드를 껐습니다. 캔버스 색은 유지하고 수식·그래프·다차원 편집기의 검은 배경을 기본으로 끕니다."
	}
	if !a.darkMode && a.darkBrush != 0 {
		procDeleteObject.Call(a.darkBrush)
		a.darkBrush = 0
	}
	if err := saveThemePreference(a.darkMode); err != nil {
		logf("THEME preference save after toggle failed dark=%v err=%v", a.darkMode, err)
	} else {
		logf("THEME preference saved after toggle dark=%v", a.darkMode)
	}
	a.applyWindowTheme()
	a.refreshOpenEditorsForTheme()
	a.rebuildMenuForTheme()
	logf("DARK_MODE enabled=%v canvas_unchanged=true editor_black_background_sync=true fg=%06X bg=%06X", a.darkMode, a.fg&0xffffff, a.bg&0xffffff)
	invalidate(a.hwnd)
}

func (a *App) defaultNewCanvasColor() uint32 {
	// Theme parity: new documents have the same white canvas in light and dark UI.
	return rgb(255, 255, 255)
}

func (a *App) buildMenu() {
	resetMenuDrawEntries()
	main, _, _ := procCreateMenu.Call()
	file, _, _ := procCreatePopupMenu.Call()
	edit, _, _ := procCreatePopupMenu.Call()
	view, _, _ := procCreatePopupMenu.Call()
	imageM, _, _ := procCreatePopupMenu.Call()
	a.viewMenu = view
	a.imageMenu = imageM
	colorM, _, _ := procCreatePopupMenu.Call()
	insertM, _, _ := procCreatePopupMenu.Call()
	insertImageM, _, _ := procCreatePopupMenu.Call()
	help, _, _ := procCreatePopupMenu.Call()
	dejavuM := uintptr(0)
	dejavuTransparentM := uintptr(0)
	if a.dejavu != nil {
		dejavuM, _, _ = procCreatePopupMenu.Call()
		dejavuTransparentM, _, _ = procCreatePopupMenu.Call()
	}
	a.fileMenu = file
	a.dejavuMenu = dejavuM
	zoomM, _, _ := procCreatePopupMenu.Call()

	appendItem(edit, MF_STRING, ID_EDIT_UNDO, "실행 취소(&U)\tCtrl+Z")
	appendItem(edit, MF_STRING, ID_EDIT_REDO, "반복(&R)\tCtrl+Shift+Z")
	appendSep(edit)
	appendItem(edit, MF_STRING, ID_EDIT_CUT, "잘라내기(&T)\tCtrl+X")
	appendItem(edit, MF_STRING, ID_EDIT_COPY, "복사(&C)\tCtrl+C")
	appendItem(edit, MF_STRING, ID_EDIT_PASTE, "붙여넣기(&P)\tCtrl+V")
	appendItem(edit, MF_STRING, ID_EDIT_DELETE, "선택 영역 배경색으로(&B)\tDel")
	appendItem(edit, MF_STRING, ID_EDIT_DELETE_TRANSPARENT, "선택 영역 지우기(&L)\tShift+Del")
	appendItem(edit, MF_STRING, ID_EDIT_SELECTALL, "모두 선택(&A)\tCtrl+A")
	appendSep(edit)
	appendItem(edit, MF_STRING, ID_EDIT_PASTEFROM, "파일로부터 붙여넣기(&F)...")

	appendItem(view, MF_CHECKED, ID_VIEW_TOOLBOX, "도구 상자(&T)\tCtrl+T")
	appendItem(view, MF_CHECKED, ID_VIEW_PALETTE, "색상표(&C)\tCtrl+L")
	appendItem(view, MF_CHECKED, ID_VIEW_STATUS, "상태 표시줄(&S)")
	appendItem(view, MF_CHECKED, ID_VIEW_CLIPBOARD, "클립보드 창(&B)")
	appendItem(view, MF_CHECKED, ID_VIEW_HISTORY, "히스토리 창(&H)")
	appendItem(view, MF_STRING, ID_VIEW_TEXTBAR, "텍스트 도구 상자(&E)")
	appendItem(view, MF_GRAYED, ID_VIEW_THUMBNAIL, "미리 보기 표시(&P)")
	appendSep(view)
	appendItem(zoomM, MF_STRING, ID_VIEW_ZOOM_NORMAL, "작게(&S)\tCtrl+PgUp")
	appendItem(zoomM, MF_STRING, ID_VIEW_ZOOM_LARGE, "크게(&L)\tCtrl+PgDn")
	appendSep(zoomM)
	for _, z := range a.zoomLevels {
		appendItem(zoomM, MF_STRING, ID_ZOOM_BASE+z, fmt.Sprintf("%dx", z))
	}
	appendPopup(view, zoomM, "확대/축소(&Z)")
	appendItem(view, MF_STRING, ID_VIEW_BITMAP, "비트맵 보기(&V)\tCtrl+F")
	appendItem(view, MF_GRAYED, ID_VIEW_GRID, "눈금 보기(&G)\tCtrl+G")
	darkFlags := uint32(MF_STRING)
	if a.darkMode {
		darkFlags |= MF_CHECKED
	}
	appendSep(view)
	appendItem(view, darkFlags, ID_VIEW_DARKMODE, "다크 모드(&D)")
	appendSep(view)
	languageMenu, _, _ := procCreatePopupMenu.Call()
	koFlags, enFlags := uint32(MF_STRING), uint32(MF_STRING)
	if dotPaintEdition == "ko" {
		koFlags |= MF_CHECKED
	} else {
		enFlags |= MF_CHECKED
	}
	appendItem(languageMenu, koFlags, ID_VIEW_LANGUAGE_KO, "한국어")
	appendItem(languageMenu, enFlags, ID_VIEW_LANGUAGE_EN, "English")
	appendPopup(view, languageMenu, "언어(&L)")

	appendItem(imageM, MF_STRING, ID_IMAGE_FLIP, "대칭 이동/회전(&F)...\tCtrl+R")
	appendItem(imageM, MF_STRING, ID_IMAGE_STRETCH, "늘이기/기울이기(&S)...\tCtrl+W")
	appendItem(imageM, MF_STRING, ID_IMAGE_INVERT, "색 반전(&I)\tCtrl+I")
	appendItem(imageM, MF_STRING, ID_IMAGE_ATTRIBUTES, "캔버스 크기(&A)...\tCtrl+E")
	appendItem(imageM, MF_STRING, ID_IMAGE_CLEAR, "이미지 지우기(&C)\tCtrl+Shift+N")
	opaqueFlags := uint32(MF_STRING)
	if a.opaque {
		opaqueFlags |= MF_CHECKED
	}
	appendItem(imageM, opaqueFlags, ID_IMAGE_OPAQUE, "불투명하게 그리기(&D)")

	appendItem(colorM, MF_STRING, ID_COLOR_EDIT, "색 편집(&E)...")
	appendSep(colorM)
	idxFlags := uint32(MF_STRING)
	if a.indexedPaletteActive() {
		idxFlags |= MF_CHECKED
	}
	if a.dejavu == nil && a.indexedSize != 16 {
		idxFlags = MF_STRING
	}
	appendItem(colorM, idxFlags, ID_COLOR_INDEXED_TOGGLE, "16색 인덱스 팔레트(&I)")
	idx256Flags := uint32(MF_STRING)
	if a.dejavu == nil && a.indexedMode && a.indexedSize == 256 {
		idx256Flags |= MF_CHECKED
	}
	appendItem(colorM, idx256Flags, ID_COLOR_INDEXED_256, "256색 인덱스 팔레트(&2)")
	appendItem(colorM, MF_STRING, ID_COLOR_INDEXED_LOAD, "인덱스 팔레트 불러오기(&L)...")
	appendItem(colorM, MF_STRING, ID_COLOR_INDEXED_SAVE, "인덱스 팔레트 저장(&S)...")

	appendItem(insertM, MF_STRING, ID_INSERT_FORMULA, "수식(&F)\tCtrl+N+M")
	appendItem(insertM, MF_STRING, ID_INSERT_FUNCTION_GRAPH, "함수/그래프(&G)\tCtrl+N+G")
	appendItem(insertImageM, MF_STRING, ID_INSERT_IMAGE_CIRCUIT, "전기회로(&E)")
	appendItem(insertImageM, MF_STRING, ID_INSERT_IMAGE_ANIMAL, "동물(&A)")
	appendItem(insertImageM, MF_STRING, ID_INSERT_IMAGE_CUSTOM, "사용자 정의 이미지(&U)")
	appendItem(insertImageM, MF_STRING, ID_INSERT_IMAGE_MULTIDIM, "다차원 도형(&D)")
	appendItem(insertImageM, MF_STRING, ID_INSERT_IMAGE_FIELD, "장 그리기(&F)")
	appendPopup(insertM, insertImageM, "이미지(&I)")

	appendItem(help, MF_STRING, ID_HELP_TOPICS, "도움말 센터(&H)\tF1")
	if diagnosticLoggingEnabled {
		appendItem(help, MF_STRING, ID_HELP_LOG, "진단 로그 열기(&L)")
	}
	appendSep(help)
	appendItem(help, MF_STRING, ID_HELP_ABOUT, "dotPaint 정보(&A)")

	if dejavuM != 0 {
		appendItem(dejavuM, MF_STRING, ID_DEJAVU_TILES, "타일 목록/이동(&T)...")
		appendSep(dejavuM)
		appendItem(dejavuM, MF_STRING, ID_DEJAVU_EXPORT_SHEET, "타일시트 PNG 내보내기(&E)...")
		appendItem(dejavuM, MF_STRING, ID_DEJAVU_IMPORT_SHEET, "타일시트 PNG 가져오기(&I)...")
		appendItem(dejavuM, MF_STRING, ID_DEJAVU_EXPORT_TILE, "현재 타일 PNG 내보내기(&X)...")
		appendItem(dejavuM, MF_STRING, ID_DEJAVU_IMPORT_TILE, "현재 타일 PNG 가져오기(&M)...")
		appendSep(dejavuM)
		for i := 0; i < 16; i++ {
			flags := uint32(MF_STRING)
			if a.dejavu != nil && int(a.dejavu.TransparentIndex) == i {
				flags |= MF_CHECKED
			}
			appendItem(dejavuTransparentM, flags, ID_DEJAVU_TRANSPARENT_BASE+i, fmt.Sprintf("인덱스 %d", i))
		}
		appendPopup(dejavuM, dejavuTransparentM, "투명 PNG 적용 인덱스(&A)")
		appendItem(dejavuM, MF_STRING, ID_DEJAVU_RELOAD_PALETTE, "DEJAVU.DAT 팔레트 다시 불러오기(&P)...")
	}

	appendTopLevelPopup(main, file, "파일(&F)")
	appendTopLevelPopup(main, edit, "편집(&E)")
	appendTopLevelPopup(main, view, "보기(&V)")
	appendTopLevelPopup(main, imageM, "이미지(&I)")
	appendTopLevelPopup(main, colorM, "색(&C)")
	appendTopLevelPopup(main, insertM, "삽입(&N)")
	if dejavuM != 0 {
		appendTopLevelPopup(main, dejavuM, "DejaVu IMG(&D)")
	}
	appendTopLevelPopup(main, help, "도움말(&H)")
	if a.darkMode {
		logf("THEMED_MENU A227 parity gutter=24 text_left=30 arrow_column=20 shared_geometry=true")
	}
	a.menu = main
	procSetMenu.Call(a.hwnd, main)
	a.applyDarkMenuBackground(main)
	a.toggleMenuCheck(ID_VIEW_TOOLBOX, a.showToolbox)
	a.toggleMenuCheck(ID_VIEW_PALETTE, a.showPalette)
	a.toggleMenuCheck(ID_VIEW_STATUS, a.showStatus)
	a.toggleMenuCheck(ID_VIEW_CLIPBOARD, a.showClipboard)
	a.toggleMenuCheck(ID_VIEW_HISTORY, a.showHistory)
	a.toggleMenuCheck(ID_VIEW_TEXTBAR, a.showTextPanel)
	a.toggleMenuCheck(ID_VIEW_DARKMODE, a.darkMode)
	a.updateZoomDependentViewMenu()
	a.updateRecentMenu()
}

func registerMenuDrawEntry(text string, topLevel, popup bool) uintptr {
	menuDrawMu.Lock()
	defer menuDrawMu.Unlock()
	key := menuDrawNext
	menuDrawNext++
	menuDrawEntries[key] = menuDrawEntry{Text: localizeUI(text), TopLevel: topLevel, Popup: popup}
	return key
}

func resetMenuDrawEntries() {
	menuDrawMu.Lock()
	menuDrawNext = 1
	menuDrawEntries = map[uintptr]menuDrawEntry{}
	menuDrawKeysByMenu = map[uintptr][]uintptr{}
	menuDrawMu.Unlock()
}

func menuDrawEntryFor(key uintptr) (menuDrawEntry, bool) {
	menuDrawMu.Lock()
	defer menuDrawMu.Unlock()
	e, ok := menuDrawEntries[key]
	return e, ok
}

func rememberMenuDrawKey(menu, key uintptr) {
	menuDrawMu.Lock()
	menuDrawKeysByMenu[menu] = append(menuDrawKeysByMenu[menu], key)
	menuDrawMu.Unlock()
}

func handleThemedMenuChar(wparam, lparam uintptr) (uintptr, bool) {
	wanted := unicode.ToUpper(rune(uint16(wparam & 0xffff)))
	menu := lparam
	menuDrawMu.Lock()
	keys := append([]uintptr(nil), menuDrawKeysByMenu[menu]...)
	entries := make(map[uintptr]menuDrawEntry, len(keys))
	for _, key := range keys {
		if entry, ok := menuDrawEntries[key]; ok {
			entries[key] = entry
		}
	}
	menuDrawMu.Unlock()
	for index, key := range keys {
		entry, ok := entries[key]
		if !ok || menuMnemonic(entry.Text) != wanted {
			continue
		}
		return uintptr(uint16(index)) | uintptr(MNC_EXECUTE)<<16, true
	}
	return 0, false
}

func appendItem(menu uintptr, flags uint32, id int, text string) {
	// A227: both themes use one owner-drawn geometry path. Only colors differ,
	// so light/dark text columns, check gutters, shortcut columns and hit boxes
	// are pixel-identical.
	key := registerMenuDrawEntry(text, false, false)
	procAppendMenuW.Call(menu, uintptr(flags|MF_OWNERDRAW), uintptr(id), key)
	rememberMenuDrawKey(menu, key)
}
func appendSep(menu uintptr) {
	procAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)
	rememberMenuDrawKey(menu, 0)
}
func appendPopup(menu, sub uintptr, text string) {
	key := registerMenuDrawEntry(text, false, true)
	procAppendMenuW.Call(menu, MF_POPUP|MF_OWNERDRAW, sub, key)
	rememberMenuDrawKey(menu, key)
}

func appendTopLevelPopup(menu, sub uintptr, text string) {
	key := registerMenuDrawEntry(text, true, true)
	procAppendMenuW.Call(menu, MF_POPUP|MF_OWNERDRAW, sub, key)
	rememberMenuDrawKey(menu, key)
}

func (a *App) updateRecentMenu() {
	// A61: 파일 메뉴 전체를 현재 상태로 다시 구성한다. XP 그림판의
	// 인쇄/페이지 설정/보내기/배경 지정처럼 dotPaint에서 쓰지 않는
	// 회색 비활성 항목은 표시하지 않는다. 최근 파일은 하위 메뉴를
	// 한 번 더 열지 않고 저장 항목 바로 아래에 최대 8개까지 펼친다.
	count, _, _ := procGetMenuItemCount.Call(a.fileMenu)
	for int32(count) > 0 {
		procDeleteMenu.Call(a.fileMenu, 0, MF_BYPOSITION)
		count--
	}

	appendItem(a.fileMenu, MF_STRING, ID_FILE_NEW, "새로 만들기(&N)\tCtrl+N")
	appendItem(a.fileMenu, MF_STRING, ID_FILE_OPEN, "열기(&O)...\tCtrl+O")
	appendItem(a.fileMenu, MF_STRING, ID_FILE_SAVE, "저장(&S)\tCtrl+S")
	appendItem(a.fileMenu, MF_STRING, ID_FILE_SAVEAS, "다른 이름으로 저장(&A)...\tCtrl+Shift+S")

	if len(a.recent) > 0 {
		appendSep(a.fileMenu)
		for i, p := range a.recent {
			if i >= 8 {
				break
			}
			// Windows 메뉴에서 경로 안의 '&'가 단축키 표식으로
			// 사라지지 않도록 두 번 써서 문자 그대로 표시한다.
			visiblePath := strings.ReplaceAll(p, "&", "&&")
			label := fmt.Sprintf("&%d %s", i+1, visiblePath)
			appendItem(a.fileMenu, MF_STRING, ID_RECENT_BASE+i, label)
		}
	}

	appendSep(a.fileMenu)
	appendItem(a.fileMenu, MF_STRING, ID_FILE_EXIT, "끝내기(&X)\tAlt+F4")
	if a.hwnd != 0 {
		procDrawMenuBar.Call(a.hwnd)
	}
}

func (a *App) recentFilePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "dotPaint", "recent.txt")
}
func (a *App) loadRecent() {
	p := a.recentFilePath()
	if p == "" {
		return
	}
	f, err := os.Open(p)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		s := strings.TrimSpace(sc.Text())
		if s != "" {
			a.recent = append(a.recent, s)
			if len(a.recent) >= 8 {
				break
			}
		}
	}
}
func (a *App) saveRecent() {
	p := a.recentFilePath()
	if p == "" {
		return
	}
	os.MkdirAll(filepath.Dir(p), 0755)
	f, err := os.Create(p)
	if err != nil {
		return
	}
	defer f.Close()
	for _, s := range a.recent {
		fmt.Fprintln(f, s)
	}
}
func (a *App) addRecent(p string) {
	abs, _ := filepath.Abs(p)
	if abs != "" {
		p = abs
	}
	out := []string{p}
	for _, s := range a.recent {
		if !strings.EqualFold(s, p) {
			out = append(out, s)
		}
		if len(out) >= 8 {
			break
		}
	}
	a.recent = out
	a.saveRecent()
	a.updateRecentMenu()
}

// --- File IO ---------------------------------------------------------------
func decodeImageFile(path string) (*Canvas, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".bmp" {
		return decodeBMP(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	cv := NewCanvas(b.Dx(), b.Dy(), rgb(255, 255, 255))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			rr, gg, bb, aa := rgba16ToStraight8(r, g, bl, a)
			i := (y*cv.W + x) * 4
			cv.Pix[i+0], cv.Pix[i+1], cv.Pix[i+2], cv.Pix[i+3] = bb, gg, rr, aa
		}
	}
	return cv, nil
}

func (c *Canvas) toRGBA() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, c.W, c.H))
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			si := (y*c.W + x) * 4
			di := img.PixOffset(x, y)
			img.Pix[di] = c.Pix[si+2]
			img.Pix[di+1] = c.Pix[si+1]
			img.Pix[di+2] = c.Pix[si]
			img.Pix[di+3] = c.Pix[si+3]
		}
	}
	return img
}

func (c *Canvas) toOpaqueNRGBA() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, c.W, c.H))
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			si := (y*c.W + x) * 4
			di := img.PixOffset(x, y)
			a := int(c.Pix[si+3])
			img.Pix[di+0] = byte((int(c.Pix[si+2])*a + 255*(255-a) + 127) / 255)
			img.Pix[di+1] = byte((int(c.Pix[si+1])*a + 255*(255-a) + 127) / 255)
			img.Pix[di+2] = byte((int(c.Pix[si+0])*a + 255*(255-a) + 127) / 255)
			img.Pix[di+3] = 255
		}
	}
	return img
}
func encodeImageFile(path string, c *Canvas) error {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		path += ".png"
		ext = ".png"
	}
	if ext == ".bmp" {
		return encodeBMP(path, c)
	}
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" {
		return fmt.Errorf("지원하지 않는 저장 형식: %s", ext)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	switch ext {
	case ".jpg", ".jpeg":
		return jpeg.Encode(f, c.toOpaqueNRGBA(), &jpeg.Options{Quality: 92})
	case ".gif":
		// GIF is an indexed, single-frame format. Always write genuine GIF
		// bytes; never silently put PNG data into a .gif filename.
		return encodeStillGIF(f, c.toRGBA())
	case ".png":
		return png.Encode(f, c.toRGBA())
	default:
		return fmt.Errorf("지원하지 않는 저장 형식: %s (PNG, BMP, JPEG, GIF 사용)", ext)
	}
}

func encodeBMP(path string, c *Canvas) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	row := ((c.W*3 + 3) / 4) * 4
	dataSize := row * c.H
	// BITMAPFILEHEADER
	binary.Write(f, binary.LittleEndian, uint16(0x4d42))
	binary.Write(f, binary.LittleEndian, uint32(14+40+dataSize))
	binary.Write(f, binary.LittleEndian, uint16(0))
	binary.Write(f, binary.LittleEndian, uint16(0))
	binary.Write(f, binary.LittleEndian, uint32(54))
	binary.Write(f, binary.LittleEndian, uint32(40))
	binary.Write(f, binary.LittleEndian, int32(c.W))
	binary.Write(f, binary.LittleEndian, int32(c.H))
	binary.Write(f, binary.LittleEndian, uint16(1))
	binary.Write(f, binary.LittleEndian, uint16(24))
	binary.Write(f, binary.LittleEndian, uint32(0))
	binary.Write(f, binary.LittleEndian, uint32(dataSize))
	binary.Write(f, binary.LittleEndian, int32(2835))
	binary.Write(f, binary.LittleEndian, int32(2835))
	binary.Write(f, binary.LittleEndian, uint32(0))
	binary.Write(f, binary.LittleEndian, uint32(0))
	buf := make([]byte, row)
	for y := c.H - 1; y >= 0; y-- {
		for i := range buf {
			buf[i] = 0
		}
		for x := 0; x < c.W; x++ {
			si := (y*c.W + x) * 4
			di := x * 3
			a := int(c.Pix[si+3])
			buf[di] = byte((int(c.Pix[si])*a + 255*(255-a) + 127) / 255)
			buf[di+1] = byte((int(c.Pix[si+1])*a + 255*(255-a) + 127) / 255)
			buf[di+2] = byte((int(c.Pix[si+2])*a + 255*(255-a) + 127) / 255)
		}
		if _, err = f.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func decodeBMP(path string) (*Canvas, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var sig uint16
	if err = binary.Read(f, binary.LittleEndian, &sig); err != nil {
		return nil, err
	}
	if sig != 0x4d42 {
		return nil, errors.New("BMP 파일이 아닙니다")
	}
	var size uint32
	binary.Read(f, binary.LittleEndian, &size)
	var r1, r2 uint16
	binary.Read(f, binary.LittleEndian, &r1)
	binary.Read(f, binary.LittleEndian, &r2)
	var off uint32
	binary.Read(f, binary.LittleEndian, &off)
	var hdr uint32
	binary.Read(f, binary.LittleEndian, &hdr)
	if hdr < 40 {
		return nil, errors.New("지원하지 않는 BMP 형식")
	}
	var w, h int32
	var planes, bpp uint16
	var comp, imgSize uint32
	binary.Read(f, binary.LittleEndian, &w)
	binary.Read(f, binary.LittleEndian, &h)
	binary.Read(f, binary.LittleEndian, &planes)
	binary.Read(f, binary.LittleEndian, &bpp)
	binary.Read(f, binary.LittleEndian, &comp)
	binary.Read(f, binary.LittleEndian, &imgSize)
	if planes != 1 || comp != 0 || (bpp != 24 && bpp != 32) {
		return nil, errors.New("24/32비트 무압축 BMP만 지원합니다")
	}
	if w <= 0 || h == 0 {
		return nil, errors.New("잘못된 BMP 크기")
	}
	topDown := h < 0
	if h < 0 {
		h = -h
	}
	if w > 20000 || h > 20000 {
		return nil, errors.New("이미지가 너무 큽니다")
	}
	cv := NewCanvas(int(w), int(h), rgb(255, 255, 255))
	f.Seek(int64(off), 0)
	bytespp := int(bpp / 8)
	row := ((int(w)*bytespp + 3) / 4) * 4
	buf := make([]byte, row)
	for rowi := 0; rowi < int(h); rowi++ {
		if _, err = f.Read(buf); err != nil {
			return nil, err
		}
		y := int(h) - 1 - rowi
		if topDown {
			y = rowi
		}
		for x := 0; x < int(w); x++ {
			si := x * bytespp
			di := (y*int(w) + x) * 4
			cv.Pix[di] = buf[si]
			cv.Pix[di+1] = buf[si+1]
			cv.Pix[di+2] = buf[si+2]
			cv.Pix[di+3] = 255
		}
	}
	return cv, nil
}

func filterString() []uint16 {
	s := "그림 파일 (*.png;*.bmp;*.jpg;*.jpeg;*.gif)\x00*.png;*.bmp;*.jpg;*.jpeg;*.gif\x00PNG 그림 (*.png)\x00*.png\x00비트맵 그림 (*.bmp)\x00*.bmp\x00JPEG 그림 (*.jpg;*.jpeg)\x00*.jpg;*.jpeg\x00GIF 그림 (*.gif)\x00*.gif\x00모든 파일 (*.*)\x00*.*\x00\x00"
	return utf16.Encode([]rune(s))
}

func documentFilterString() []uint16 {
	s := "지원 파일 (*.png;*.bmp;*.jpg;*.jpeg;*.gif;*.img)\x00*.png;*.bmp;*.jpg;*.jpeg;*.gif;*.img\x00DejaVu 타일 그래픽 (*.img)\x00*.img\x00그림 파일 (*.png;*.bmp;*.jpg;*.jpeg;*.gif)\x00*.png;*.bmp;*.jpg;*.jpeg;*.gif\x00모든 파일 (*.*)\x00*.*\x00\x00"
	return utf16.Encode([]rune(s))
}

func chooseOpenFile(hwnd uintptr) (string, bool) {
	buf := make([]uint16, 32768)
	filter := filterString()
	ofn := OPENFILENAMEW{LStructSize: uint32(unsafe.Sizeof(OPENFILENAMEW{})), HwndOwner: hwnd, LpstrFilter: &filter[0], LpstrFile: &buf[0], NMaxFile: uint32(len(buf)), Flags: OFN_PATHMUSTEXIST | OFN_FILEMUSTEXIST | OFN_HIDEREADONLY}
	r, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

func chooseOpenDocumentFile(hwnd uintptr) (string, bool) {
	buf := make([]uint16, 32768)
	filter := documentFilterString()
	ofn := OPENFILENAMEW{LStructSize: uint32(unsafe.Sizeof(OPENFILENAMEW{})), HwndOwner: hwnd, LpstrFilter: &filter[0], LpstrFile: &buf[0], NMaxFile: uint32(len(buf)), Flags: OFN_PATHMUSTEXIST | OFN_FILEMUSTEXIST | OFN_HIDEREADONLY}
	r, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}
func chooseSaveFile(hwnd uintptr, current string) (string, bool) {
	buf := make([]uint16, 32768)
	if current != "" {
		u := syscall.StringToUTF16(current)
		copy(buf, u)
	}
	filter := filterString()
	def := utf16Ptr("png")
	ofn := OPENFILENAMEW{LStructSize: uint32(unsafe.Sizeof(OPENFILENAMEW{})), HwndOwner: hwnd, LpstrFilter: &filter[0], LpstrFile: &buf[0], NMaxFile: uint32(len(buf)), Flags: OFN_PATHMUSTEXIST | OFN_OVERWRITEPROMPT, LpstrDefExt: def}
	r, _, _ := procGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

func (a *App) confirmLoseChanges() bool {
	if !a.dirty {
		return true
	}
	r := messageBox(a.hwnd, "현재 그림의 변경 내용을 저장하시겠습니까?", "dotPaint", MB_YESNOCANCEL|MB_ICONQUESTION)
	if r == IDCANCEL {
		return false
	}
	if r == IDYES {
		return a.save(false)
	}
	return true
}
func (a *App) newDocument() {
	if !a.confirmLoseChanges() {
		return
	}
	a.leaveDejavuIMGMode()
	w, h := 640, 480
	clipboardSized := false
	clipboardSource := ""
	if img, source, err := a.readSystemClipboard(); err == nil && validClipboardImage(img) {
		w, h = img.W, img.H
		clipboardSized = true
		clipboardSource = source
		logf("NEW_DOCUMENT clipboard-size source=%q size=%dx%d", source, w, h)
	} else if err != nil {
		logf("NEW_DOCUMENT clipboard-size unavailable: %v", err)
	}
	a.canvas = NewCanvas(w, h, a.defaultNewCanvasColor())
	a.indexedPixelIndices = nil
	a.clearSelectionState()
	a.filePath = ""
	a.undo.Reset()
	a.resetHistory("새 그림")
	a.scrollX, a.scrollY = 0, 0
	a.clearDirty()
	a.updateScrollbars()
	if clipboardSized {
		a.statusText = fmt.Sprintf("새 그림: %d × %d (클립보드 이미지 크기, %s)", w, h, clipboardSource)
	} else {
		a.statusText = fmt.Sprintf("새 그림: %d × %d", w, h)
	}
	var rc RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	a.updateTextUILayout(rc)
	invalidate(a.hwnd)
}
func (a *App) openDocument(path string) bool {
	if !a.confirmLoseChanges() {
		return false
	}
	if strings.EqualFold(filepath.Ext(path), ".img") {
		return a.openDejavuIMGDocument(path)
	}
	a.leaveDejavuIMGMode()
	cv, err := decodeImageFile(path)
	if err != nil {
		messageBox(a.hwnd, "파일을 열 수 없습니다.\n\n"+err.Error(), "dotPaint", MB_OK|MB_ICONERROR)
		return false
	}
	a.canvas = cv
	if a.indexedMode {
		a.indexedPixelIndices = quantizeIndexedBGRA(a.canvas.Pix, a.palette, a.indexedActiveCount)
	}
	a.clearSelectionState()
	a.filePath = path
	a.undo.Reset()
	a.resetHistory("열기")
	a.scrollX, a.scrollY = 0, 0
	a.clearDirty()
	a.addRecent(path)
	a.updateScrollbars()
	invalidate(a.hwnd)
	return true
}

func isSupportedDroppedImage(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".bmp", ".jpg", ".jpeg", ".gif", ".img":
		return true
	}
	return false
}

func queryDroppedFiles(hdrop uintptr) []string {
	if hdrop == 0 {
		return nil
	}
	count, _, _ := procDragQueryFileW.Call(hdrop, ^uintptr(0), 0, 0)
	if count > 4096 {
		count = 4096
	}
	paths := make([]string, 0, int(count))
	for i := uintptr(0); i < count; i++ {
		length, _, _ := procDragQueryFileW.Call(hdrop, i, 0, 0)
		if length == 0 || length > 32767 {
			continue
		}
		buf := make([]uint16, int(length)+1)
		written, _, _ := procDragQueryFileW.Call(hdrop, i, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if written == 0 {
			continue
		}
		path := syscall.UTF16ToString(buf)
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func (a *App) handleDroppedFiles(hdrop uintptr) {
	paths := queryDroppedFiles(hdrop)
	procDragFinish.Call(hdrop)
	logf("DROPFILES received count=%d", len(paths))
	if len(paths) == 0 {
		return
	}

	chosen := ""
	supportedCount := 0
	for _, path := range paths {
		if isSupportedDroppedImage(path) {
			supportedCount++
			if chosen == "" {
				chosen = path
			}
		}
	}
	if chosen == "" {
		messageBox(a.hwnd, "불러올 수 있는 그림 파일이 없습니다.\n\n지원 형식: PNG, BMP, JPG, JPEG, GIF, DejaVu IMG", "dotPaint", MB_OK|MB_ICONWARNING)
		a.statusText = "드롭한 항목에서 지원되는 그림 파일을 찾지 못했습니다."
		invalidate(a.hwnd)
		return
	}

	logf("DROPFILES opening path=%q supported=%d total=%d", chosen, supportedCount, len(paths))
	if !a.openDocument(chosen) {
		logf("DROPFILES open cancelled or failed path=%q", chosen)
		return
	}
	if supportedCount > 1 {
		a.statusText = fmt.Sprintf("여러 그림 파일 중 첫 번째 파일 %s을(를) 불러왔습니다.", filepath.Base(chosen))
	} else {
		a.statusText = fmt.Sprintf("드래그 앤 드롭으로 %s을(를) 불러왔습니다.", filepath.Base(chosen))
	}
	invalidate(a.hwnd)
}
func (a *App) save(forceAs bool) bool {
	if a.dejavu != nil {
		return a.saveDejavuIMG(forceAs)
	}
	path := a.filePath
	if forceAs || path == "" {
		var ok bool
		path, ok = chooseSaveFile(a.hwnd, path)
		if !ok {
			return false
		}
		if filepath.Ext(path) == "" {
			path += ".png"
		}
	}
	if err := encodeImageFile(path, a.canvas); err != nil {
		messageBox(a.hwnd, "파일을 저장할 수 없습니다.\n\n"+err.Error(), "dotPaint", MB_OK|MB_ICONERROR)
		return false
	}
	a.filePath = path
	a.clearDirty()
	a.addRecent(path)
	return true
}

// --- Painting UI -----------------------------------------------------------
func (a *App) destroyBackBuffer() {
	if a.backDC != 0 {
		if a.backOldBitmap != 0 {
			procSelectObject.Call(a.backDC, a.backOldBitmap)
		}
		if a.backBitmap != 0 {
			procDeleteObject.Call(a.backBitmap)
		}
		procDeleteDC.Call(a.backDC)
	}
	a.backDC = 0
	a.backBitmap = 0
	a.backOldBitmap = 0
	a.backW = 0
	a.backH = 0
}

func (a *App) ensureBackBuffer(screenDC uintptr, w, h int) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	if a.backDC != 0 && a.backBitmap != 0 && a.backW == w && a.backH == h {
		return true
	}
	logf("BACKBUFFER recreate old=%dx%d new=%dx%d", a.backW, a.backH, w, h)
	a.destroyBackBuffer()
	memDC, _, err := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		logf("BACKBUFFER CreateCompatibleDC failed: %v", err)
		return false
	}
	bitmap, _, err := procCreateCompatibleBitmap.Call(screenDC, uintptr(w), uintptr(h))
	if bitmap == 0 {
		logf("BACKBUFFER CreateCompatibleBitmap failed %dx%d: %v", w, h, err)
		procDeleteDC.Call(memDC)
		return false
	}
	oldBitmap, _, _ := procSelectObject.Call(memDC, bitmap)
	a.backDC = memDC
	a.backBitmap = bitmap
	a.backOldBitmap = oldBitmap
	a.backW = w
	a.backH = h
	logf("BACKBUFFER ready dc=%#x bitmap=%#x", memDC, bitmap)
	return true
}

func (a *App) paintBuffered(screenDC uintptr, update RECT) {
	started := time.Now()
	count := atomic.AddUint64(&a.paintCount, 1)
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	w := int(client.Right - client.Left)
	h := int(client.Bottom - client.Top)
	if w <= 0 || h <= 0 {
		return
	}

	bufferWasValid := a.backDC != 0 && a.backBitmap != 0 && a.backW == w && a.backH == h
	if !a.ensureBackBuffer(screenDC, w, h) {
		logf("PAINT #%d fallback direct", count)
		a.paint(screenDC)
		return
	}

	left, top := int(update.Left), int(update.Top)
	right, bottom := int(update.Right), int(update.Bottom)
	if !bufferWasValid || right <= left || bottom <= top {
		left, top, right, bottom = 0, 0, w, h
	}
	left = clamp(left, 0, w)
	top = clamp(top, 0, h)
	right = clamp(right, left, w)
	bottom = clamp(bottom, top, h)

	// Keep the persistent back buffer, but redraw only the invalid rectangle.
	// Previously every status/caret/mouse update rebuilt the complete window and
	// complete canvas even though only a tiny rectangle was copied to screen.
	saved, _, _ := procSaveDC.Call(a.backDC)
	procIntersectClipRect.Call(a.backDC, uintptr(left), uintptr(top), uintptr(right), uintptr(bottom))
	a.paint(a.backDC)
	if saved != 0 {
		procRestoreDC.Call(a.backDC, saved)
	}

	_, _, err := procBitBlt.Call(screenDC, uintptr(left), uintptr(top), uintptr(right-left), uintptr(bottom-top), a.backDC, uintptr(left), uintptr(top), SRCCOPY)
	elapsed := time.Since(started)
	if count <= 20 || count%100 == 0 || elapsed > 100*time.Millisecond {
		logf("PAINT #%d update=(%d,%d)-(%d,%d) client=%dx%d elapsed=%s err=%v", count, left, top, right, bottom, w, h, elapsed, err)
	}
}

func (a *App) paint(hdc uintptr) {
	var rc RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	ws, _, _, st := a.layout(rc)
	clipPanel := a.clipboardPanelRect(rc)
	textPanel := a.textPanelRect(rc)
	historyPanel := a.historyPanelRect(rc)
	thumbnailPanel := a.thumbnailPanelRect(rc)
	toolboxPanel := a.toolboxPanelRect(rc)
	palettePanel := a.palettePanelRect(rc)
	oldFont := selectDefaultFont(hdc)
	defer procSelectObject.Call(hdc, oldFont)
	fillUIRect(hdc, rc, rgb(240, 240, 240))
	if a.darkMode {
		fillRect(hdc, ws, rgb(45, 45, 45))
	} else {
		fillRect(hdc, ws, rgb(171, 171, 171))
	}
	// The canvas is painted first and strictly clipped to the workspace. The
	// fixed interface is then painted last, so zoomed pixels can never cover the
	// toolbox, palette or status bar.
	a.paintCanvas(hdc, ws)
	if a.showToolbox && !a.bitmapView {
		a.paintToolboxPanel(hdc, toolboxPanel)
	}
	if a.showPalette && !a.bitmapView {
		a.paintPalettePanel(hdc, palettePanel)
	}
	if a.showClipboard && !a.bitmapView {
		a.paintClipboardPanel(hdc, clipPanel)
	}
	if a.showHistory && !a.bitmapView {
		a.paintHistoryPanel(hdc, historyPanel)
	}
	if a.thumbnailPanelVisible() {
		a.paintThumbnailPanel(hdc, thumbnailPanel)
	}
	if a.showTextPanel && a.tool == ToolText && !a.bitmapView {
		a.paintTextPanel(hdc, textPanel)
	}
	if a.showStatus && !a.bitmapView {
		a.paintStatus(hdc, st)
	}
}

func (a *App) paintCanvas(hdc uintptr, ws RECT) {
	saved, _, _ := procSaveDC.Call(hdc)
	procIntersectClipRect.Call(hdc, uintptr(ws.Left), uintptr(ws.Top), uintptr(ws.Right), uintptr(ws.Bottom))
	defer func() {
		if saved != 0 {
			procRestoreDC.Call(hdc, saved)
		}
	}()
	var paintClip RECT
	clipKind, _, _ := procGetClipBox.Call(hdc, uintptr(unsafe.Pointer(&paintClip)))
	if clipKind == 0 || rectEmpty(paintClip) {
		return
	}
	ox, oy := a.canvasOrigin(ws)
	// A59: no black outline or drop shadow. The grey workspace itself
	// separates the white canvas, while the resize handles sit outside it.
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(a.canvas.W), BiHeight: -int32(a.canvas.H), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	procSetStretchBltMode.Call(hdc, COLORONCOLOR)
	if len(a.canvas.Pix) > 0 {
		displayPix := a.canvas.displayPixels(a.zoom)
		a.paintCanvasPixelsTiled(hdc, paintClip, ox, oy, displayPix, bmi)
		runtime.KeepAlive(displayPix)
	}
	a.paintPixelGrid(hdc, ws, ox, oy)
	if a.dejavu != nil {
		a.paintDejavuActiveTileOverlay(hdc, ox, oy)
	}
	if a.preview {
		a.paintPreview(hdc, ox, oy)
	}
	a.paintSelectionOverlay(hdc, ox, oy)
	a.paintTextOverlay(hdc, ox, oy)
	a.paintZoomPreview(hdc, ox, oy)
	a.paintEraserCursor(hdc, ox, oy)
	a.paintBrushCursor(hdc, ox, oy)
	if a.resizingCanvas {
		left := ox + a.resizeLeftDelta*a.zoom
		top := oy + a.resizeTopDelta*a.zoom
		r := RECT{int32(left), int32(top), int32(left + a.resizePreviewW*a.zoom), int32(top + a.resizePreviewH*a.zoom)}
		frameRect(hdc, r, uiColor(rgb(0, 120, 215)))
	}
	for _, r := range a.canvasHandleRects() {
		fillUIRect(hdc, r, rgb(255, 255, 255))
		frameRect(hdc, r, uiColor(rgb(0, 120, 215)))
	}
}

// destroyCanvasTileBuffer releases the persistent GDI-owned source tile used
// by the canvas renderer.
func (a *App) destroyCanvasTileBuffer() {
	if a.canvasTileDC != 0 {
		if a.canvasTileOldBitmap != 0 {
			procSelectObject.Call(a.canvasTileDC, a.canvasTileOldBitmap)
		}
		if a.canvasTileBitmap != 0 {
			procDeleteObject.Call(a.canvasTileBitmap)
		}
		procDeleteDC.Call(a.canvasTileDC)
	}
	a.canvasTileDC = 0
	a.canvasTileBitmap = 0
	a.canvasTileOldBitmap = 0
	a.canvasTileBits = nil
	a.canvasTileW = 0
	a.canvasTileH = 0
}

// ensureCanvasTileBuffer creates or grows a top-down 32-bit DIB section.  The
// buffer belongs to GDI, so BitBlt/StretchBlt receives a normal bitmap handle
// rather than an interior pointer into a large Go allocation.
func (a *App) ensureCanvasTileBuffer(hdc uintptr, w, h int) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	if a.canvasTileDC != 0 && a.canvasTileBitmap != 0 && a.canvasTileBits != nil && a.canvasTileW >= w && a.canvasTileH >= h {
		return true
	}
	newW := max(w, a.canvasTileW)
	newH := max(h, a.canvasTileH)
	if newW < 64 {
		newW = 64
	}
	if newH < 16 {
		newH = 16
	}
	// Grow in modest blocks so repeated one-pixel canvas changes do not recreate
	// a DIB section every paint.
	newW = ((newW + 255) / 256) * 256
	newH = ((newH + 31) / 32) * 32

	a.destroyCanvasTileBuffer()
	memDC, _, err := procCreateCompatibleDC.Call(hdc)
	if memDC == 0 {
		logf("CANVAS_BLIT CreateCompatibleDC failed size=%dx%d err=%v", newW, newH, err)
		return false
	}
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{
		BiSize:        uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
		BiWidth:       int32(newW),
		BiHeight:      -int32(newH),
		BiPlanes:      1,
		BiBitCount:    32,
		BiCompression: BI_RGB,
	}}
	var bits unsafe.Pointer
	bitmap, _, err := procCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 || bits == nil {
		logf("CANVAS_BLIT CreateDIBSection failed size=%dx%d err=%v", newW, newH, err)
		procDeleteDC.Call(memDC)
		return false
	}
	oldBitmap, _, _ := procSelectObject.Call(memDC, bitmap)
	a.canvasTileDC = memDC
	a.canvasTileBitmap = bitmap
	a.canvasTileOldBitmap = oldBitmap
	a.canvasTileBits = bits
	a.canvasTileW = newW
	a.canvasTileH = newH
	logf("CANVAS_BLIT dib-tile-ready size=%dx%d dc=%#x bitmap=%#x", newW, newH, memDC, bitmap)
	return true
}

// copyCanvasTileToDIB packs a visible canvas rectangle into the upper-left of
// the persistent DIB section.  Rows are contiguous in the destination even
// when the source canvas is much wider or the visible rectangle is scrolled.
func (a *App) copyCanvasTileToDIB(pixels []byte, tile CanvasBlitTile) bool {
	if a.canvas == nil || a.canvasTileBits == nil {
		return false
	}
	tileW := tile.X1 - tile.X0
	tileH := tile.Y1 - tile.Y0
	if tileW <= 0 || tileH <= 0 || tileW > a.canvasTileW || tileH > a.canvasTileH {
		return false
	}
	need := a.canvas.W * a.canvas.H * 4
	if len(pixels) < need {
		return false
	}
	dst := unsafe.Slice((*byte)(a.canvasTileBits), a.canvasTileW*a.canvasTileH*4)
	dstStride := a.canvasTileW * 4
	rowBytes := tileW * 4
	for row := 0; row < tileH; row++ {
		sy := tile.Y0 + row
		srcStart := (sy*a.canvas.W + tile.X0) * 4
		dstStart := row * dstStride
		copy(dst[dstStart:dstStart+rowBytes], pixels[srcStart:srcStart+rowBytes])
	}
	return true
}

func (a *App) blitCanvasTile(hdc uintptr, ox, oy int, tile CanvasBlitTile, rowByRow bool) int {
	tileW := tile.X1 - tile.X0
	tileH := tile.Y1 - tile.Y0
	if tileW <= 0 || tileH <= 0 || a.canvasTileDC == 0 {
		return 1
	}
	dx := ox + tile.X0*a.zoom
	dy := oy + tile.Y0*a.zoom
	failures := 0
	if !rowByRow {
		var ret uintptr
		if a.zoom == 1 {
			ret, _, _ = procBitBlt.Call(hdc, uintptr(dx), uintptr(dy), uintptr(tileW), uintptr(tileH), a.canvasTileDC, 0, 0, SRCCOPY)
		} else {
			procSetStretchBltMode.Call(hdc, COLORONCOLOR)
			procSetStretchBltMode.Call(a.canvasTileDC, COLORONCOLOR)
			ret, _, _ = procStretchBlt.Call(hdc, uintptr(dx), uintptr(dy), uintptr(tileW*a.zoom), uintptr(tileH*a.zoom), a.canvasTileDC, 0, 0, uintptr(tileW), uintptr(tileH), SRCCOPY)
		}
		if ret != 0 {
			return 0
		}
	}
	for row := 0; row < tileH; row++ {
		rowY := dy + row*a.zoom
		var ret uintptr
		if a.zoom == 1 {
			ret, _, _ = procBitBlt.Call(hdc, uintptr(dx), uintptr(rowY), uintptr(tileW), 1, a.canvasTileDC, 0, uintptr(row), SRCCOPY)
		} else {
			ret, _, _ = procStretchBlt.Call(hdc, uintptr(dx), uintptr(rowY), uintptr(tileW*a.zoom), uintptr(a.zoom), a.canvasTileDC, 0, uintptr(row), uintptr(tileW), 1, SRCCOPY)
		}
		if ret == 0 {
			failures++
		}
	}
	return failures
}

// paintCanvasPixelsTiled transfers only the visible part of the canvas in
// bounded tiles.  A165 passed interior pointers from the large canvas slice to
// StretchDIBits.  On the user's system one repeatable scan line (source y=571)
// was rejected, leaving a gray seam.  A166 first packs each tile into a stable
// top-down DIB section and then uses BitBlt/StretchBlt, eliminating that path.
func (a *App) paintCanvasPixelsTiled(hdc uintptr, ws RECT, ox, oy int, pixels []byte, _ BITMAPINFO) {
	if a.canvas == nil || a.canvas.W < 1 || a.canvas.H < 1 || len(pixels) < a.canvas.W*a.canvas.H*4 {
		return
	}
	clip := CanvasBlitClip{Left: int(ws.Left), Top: int(ws.Top), Right: int(ws.Right), Bottom: int(ws.Bottom)}
	tiles := canvasVisibleBlitTiles(a.canvas.W, a.canvas.H, a.zoom, ox, oy, clip)
	if len(tiles) == 0 {
		return
	}
	failures := 0
	rowFallbacks := 0
	for _, tile := range tiles {
		tileW := tile.X1 - tile.X0
		tileH := tile.Y1 - tile.Y0
		if tileW <= 0 || tileH <= 0 {
			continue
		}
		if !a.ensureCanvasTileBuffer(hdc, tileW, tileH) || !a.copyCanvasTileToDIB(pixels, tile) {
			failures++
			continue
		}
		if n := a.blitCanvasTile(hdc, ox, oy, tile, false); n != 0 {
			rowFallbacks++
			if a.canvasBlitFails+uint64(failures) < 12 {
				logf("CANVAS_BLIT dib-tile-failed canvas=%dx%d zoom=%d tile=(%d,%d)-(%d,%d); retry rows", a.canvas.W, a.canvas.H, a.zoom, tile.X0, tile.Y0, tile.X1, tile.Y1)
			}
			failures += a.blitCanvasTile(hdc, ox, oy, tile, true)
		}
	}

	// Verify tile centers and seams.  If anything is missing, replay every tile
	// one scan line at a time from the DIB section.  Unlike A165, the fallback no
	// longer constructs a one-row StretchDIBits source from a large slice.
	if !a.verifyCanvasBlitPixels(hdc, ox, oy, pixels, tiles) {
		logf("CANVAS_BLIT verify-mismatch canvas=%dx%d zoom=%d; forcing DIB row pass", a.canvas.W, a.canvas.H, a.zoom)
		rowFallbacks++
		for _, tile := range tiles {
			tileW := tile.X1 - tile.X0
			tileH := tile.Y1 - tile.Y0
			if !a.ensureCanvasTileBuffer(hdc, tileW, tileH) || !a.copyCanvasTileToDIB(pixels, tile) {
				failures++
				continue
			}
			failures += a.blitCanvasTile(hdc, ox, oy, tile, true)
		}
		if !a.verifyCanvasBlitPixels(hdc, ox, oy, pixels, tiles) {
			failures++
			logf("CANVAS_BLIT verify-still-mismatched canvas=%dx%d zoom=%d", a.canvas.W, a.canvas.H, a.zoom)
		} else {
			logf("CANVAS_BLIT DIB-row-recovery canvas=%dx%d zoom=%d", a.canvas.W, a.canvas.H, a.zoom)
		}
	}
	if failures > 0 {
		a.canvasBlitFails += uint64(failures)
		if a.canvasBlitFails <= 12 || a.canvasBlitFails%100 == 0 {
			logf("CANVAS_BLIT incomplete canvas=%dx%d visibleTiles=%d rowFallbacks=%d failures=%d totalFailures=%d", a.canvas.W, a.canvas.H, len(tiles), rowFallbacks, failures, a.canvasBlitFails)
		}
	}
	if a.canvas.W != a.lastCanvasBlitW || a.canvas.H != a.lastCanvasBlitH {
		a.lastCanvasBlitW, a.lastCanvasBlitH = a.canvas.W, a.canvas.H
		first, last := tiles[0], tiles[len(tiles)-1]
		logf("CANVAS_BLIT DIB-tiled-ready canvas=%dx%d zoom=%d visible=(%d,%d)-(%d,%d) tiles=%d tileBuffer=%dx%d", a.canvas.W, a.canvas.H, a.zoom, first.X0, first.Y0, last.X1, last.Y1, len(tiles), a.canvasTileW, a.canvasTileH)
	}
}

func (a *App) verifyCanvasBlitPixels(hdc uintptr, ox, oy int, pixels []byte, tiles []CanvasBlitTile) bool {
	if len(tiles) == 0 || a.canvas == nil || a.canvas.W < 1 || a.canvas.H < 1 {
		return true
	}
	first, last := tiles[0], tiles[len(tiles)-1]
	x0, x1 := first.X0, first.X1
	y0, y1 := first.Y0, last.Y1
	if x1 <= x0 || y1 <= y0 {
		return true
	}
	points := make([][2]int, 0, 3+len(tiles)*2)
	points = append(points,
		[2]int{x0 + (x1-x0)/2, y0 + (y1-y0)/2},
		[2]int{x0 + min(2, x1-x0-1), y0 + min(2, y1-y0-1)},
		[2]int{x1 - 1 - min(2, x1-x0-1), y1 - 1 - min(2, y1-y0-1)},
	)
	// A165 sampled only the overall center and corners, so the missing y=571
	// seam escaped verification.  Check both sides of every tile boundary.
	centerX := x0 + (x1-x0)/2
	for _, tile := range tiles {
		if tile.Y0 >= y0 && tile.Y0 < y1 {
			points = append(points, [2]int{centerX, tile.Y0})
		}
		if tile.Y1-1 >= y0 && tile.Y1-1 < y1 {
			points = append(points, [2]int{centerX, tile.Y1 - 1})
		}
	}
	for _, pt := range points {
		sx, sy := pt[0], pt[1]
		if sx < 0 || sy < 0 || sx >= a.canvas.W || sy >= a.canvas.H {
			continue
		}
		i := (sy*a.canvas.W + sx) * 4
		if i < 0 || i+3 >= len(pixels) {
			continue
		}
		expected := bgraToColorref(pixels[i], pixels[i+1], pixels[i+2]) & 0x00FFFFFF
		dx := ox + sx*a.zoom + max(a.zoom/2, 0)
		dy := oy + sy*a.zoom + max(a.zoom/2, 0)
		actual, _, _ := procGetPixel.Call(hdc, uintptr(dx), uintptr(dy))
		if uint32(actual) == 0xFFFFFFFF {
			// A178 repaints only the invalid rectangle. Samples outside that clip
			// return CLR_INVALID even though the persistent back buffer is intact;
			// replaying every canvas row in that case caused needless work and log
			// spam during caret blinking. A full repaint still verifies normally.
			continue
		}
		if uint32(actual)&0x00FFFFFF != expected {
			logf("CANVAS_BLIT verify-pixel mismatch canvas=%dx%d source=%d,%d screen=%d,%d expected=%#08x actual=%#08x", a.canvas.W, a.canvas.H, sx, sy, dx, dy, expected, uint32(actual))
			return false
		}
	}
	return true
}

func (a *App) paintPixelGrid(hdc uintptr, ws RECT, ox, oy int) {
	if !a.showGrid || a.zoom < 6 || a.canvas == nil || a.canvas.W < 1 || a.canvas.H < 1 {
		return
	}
	z := a.zoom
	canvasLeft, canvasTop := ox, oy
	canvasRight := ox + a.canvas.W*z
	canvasBottom := oy + a.canvas.H*z
	left := max(int(ws.Left), canvasLeft)
	top := max(int(ws.Top), canvasTop)
	right := min(int(ws.Right), canvasRight)
	bottom := min(int(ws.Bottom), canvasBottom)
	if right <= left || bottom <= top {
		return
	}

	startX := 0
	if left > canvasLeft {
		startX = (left - canvasLeft + z - 1) / z
	}
	endX := min(a.canvas.W, (right-canvasLeft)/z+1)
	startY := 0
	if top > canvasTop {
		startY = (top - canvasTop + z - 1) / z
	}
	endY := min(a.canvas.H, (bottom-canvasTop)/z+1)

	pen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(192, 192, 192))))
	oldPen, _, _ := procSelectObject.Call(hdc, pen)
	for x := startX; x <= endX; x++ {
		sx := canvasLeft + x*z
		if sx < left || sx > right {
			continue
		}
		procMoveToEx.Call(hdc, uintptr(sx), uintptr(top), 0)
		procLineTo.Call(hdc, uintptr(sx), uintptr(bottom))
	}
	for y := startY; y <= endY; y++ {
		sy := canvasTop + y*z
		if sy < top || sy > bottom {
			continue
		}
		procMoveToEx.Call(hdc, uintptr(left), uintptr(sy), 0)
		procLineTo.Call(hdc, uintptr(right), uintptr(sy))
	}
	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(pen)
}

func drawDashedLine(hdc uintptr, x0, y0, x1, y1 int, c uint32) {
	drawDashedLineWidth(hdc, x0, y0, x1, y1, c, 1)
}

func drawDashedLineWidth(hdc uintptr, x0, y0, x1, y1 int, c uint32, width int) {
	steps := max(absInt(x1-x0), absInt(y1-y0))
	if steps < 1 {
		return
	}
	if width < 1 {
		width = 1
	}
	pen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(width), uintptr(c))
	old, _, _ := procSelectObject.Call(hdc, pen)
	defer func() { procSelectObject.Call(hdc, old); procDeleteObject.Call(pen) }()
	for i := 0; i < steps; i += 6 {
		j := i + 3
		if j > steps {
			j = steps
		}
		t0 := float64(i) / float64(steps)
		t1 := float64(j) / float64(steps)
		ax := int(math.Round(float64(x0) + float64(x1-x0)*t0))
		ay := int(math.Round(float64(y0) + float64(y1-y0)*t0))
		bx := int(math.Round(float64(x0) + float64(x1-x0)*t1))
		by := int(math.Round(float64(y0) + float64(y1-y0)*t1))
		procMoveToEx.Call(hdc, uintptr(ax), uintptr(ay), 0)
		procLineTo.Call(hdc, uintptr(bx), uintptr(by))
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func drawMaskedSelectionOutline(hdc uintptr, ox, oy, zoom, x, y, w, h int, mask []bool, color uint32, width int) {
	if w < 1 || h < 1 || len(mask) != w*h {
		return
	}
	if width < 1 {
		width = 1
	}
	pen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(width), uintptr(color))
	old, _, _ := procSelectObject.Call(hdc, pen)
	defer func() { procSelectObject.Call(hdc, old); procDeleteObject.Call(pen) }()

	drawAxis := func(x0, y0, x1, y1 int) {
		if x0 == x1 {
			step := 1
			if y1 < y0 {
				y0, y1 = y1, y0
			}
			for p := y0; p < y1; p += step {
				phase := (p + x0) % 6
				if phase < 0 {
					phase += 6
				}
				if phase < 3 {
					procMoveToEx.Call(hdc, uintptr(x0), uintptr(p), 0)
					procLineTo.Call(hdc, uintptr(x0), uintptr(min(p+1, y1)))
				}
			}
			return
		}
		if y0 == y1 {
			if x1 < x0 {
				x0, x1 = x1, x0
			}
			for p := x0; p < x1; p++ {
				phase := (p + y0) % 6
				if phase < 0 {
					phase += 6
				}
				if phase < 3 {
					procMoveToEx.Call(hdc, uintptr(p), uintptr(y0), 0)
					procLineTo.Call(hdc, uintptr(min(p+1, x1)), uintptr(y0))
				}
			}
		}
	}
	selected := func(px, py int) bool {
		return px >= 0 && py >= 0 && px < w && py < h && mask[py*w+px]
	}
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			if !selected(px, py) {
				continue
			}
			l := ox + (x+px)*zoom
			t := oy + (y+py)*zoom
			r := l + zoom
			b := t + zoom
			if !selected(px, py-1) {
				drawAxis(l, t, r, t)
			}
			if !selected(px+1, py) {
				drawAxis(r, t, r, b)
			}
			if !selected(px, py+1) {
				drawAxis(l, b, r, b)
			}
			if !selected(px-1, py) {
				drawAxis(l, t, l, b)
			}
		}
	}
}

func drawSelectionRegionOutline(hdc uintptr, ox, oy, zoom, x, y, w, h int, mask []bool, color uint32, guideW int, handles bool) {
	if w < 1 || h < 1 {
		return
	}
	if len(mask) == w*h {
		drawMaskedSelectionOutline(hdc, ox, oy, zoom, x, y, w, h, mask, color, guideW)
		return
	}
	l, t := ox+x*zoom, oy+y*zoom
	r, b := l+w*zoom, t+h*zoom
	drawDashedLineWidth(hdc, l, t, r, t, color, guideW)
	drawDashedLineWidth(hdc, r, t, r, b, color, guideW)
	drawDashedLineWidth(hdc, r, b, l, b, color, guideW)
	drawDashedLineWidth(hdc, l, b, l, t, color, guideW)
	if handles {
		for _, p := range [][2]int{{l, t}, {r, t}, {l, b}, {r, b}} {
			rc := RECT{int32(p[0] - 3), int32(p[1] - 3), int32(p[0] + 4), int32(p[1] + 4)}
			fillRect(hdc, rc, rgb(255, 255, 255))
			frameRect(hdc, rc, color)
		}
	}
}

type selectionGuidePens struct {
	hdc                 uintptr
	blackPen, whitePen  uintptr
	oldPen, selectedPen uintptr
}

func newSelectionGuidePens(hdc uintptr, width int) *selectionGuidePens {
	if width < 1 {
		width = 1
	}
	blackPen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(width), uintptr(rgb(0, 0, 0)))
	whitePen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(width), uintptr(rgb(255, 255, 255)))
	oldPen, _, _ := procSelectObject.Call(hdc, blackPen)
	return &selectionGuidePens{hdc: hdc, blackPen: blackPen, whitePen: whitePen, oldPen: oldPen, selectedPen: blackPen}
}

func (p *selectionGuidePens) close() {
	if p == nil {
		return
	}
	procSelectObject.Call(p.hdc, p.oldPen)
	procDeleteObject.Call(p.blackPen)
	procDeleteObject.Call(p.whitePen)
}

func (p *selectionGuidePens) line(x0, y0, x1, y1 int, white bool) {
	if p == nil {
		return
	}
	pen := p.blackPen
	if white {
		pen = p.whitePen
	}
	if pen != p.selectedPen {
		procSelectObject.Call(p.hdc, pen)
		p.selectedPen = pen
	}
	procMoveToEx.Call(p.hdc, uintptr(x0), uintptr(y0), 0)
	procLineTo.Call(p.hdc, uintptr(x1), uintptr(y1))
}

func (a *App) selectionGuideUsesWhiteAtPixel(x, y int) bool {
	if a.canvas == nil || x < 0 || y < 0 || x >= a.canvas.W || y >= a.canvas.H {
		// The workspace is RGB(171,171,171), so its luminance is above the
		// threshold and receives the ordinary black marquee.
		return false
	}
	i := (y*a.canvas.W + x) * 4
	b, g, r := a.canvas.Pix[i], a.canvas.Pix[i+1], a.canvas.Pix[i+2]
	alpha := int(a.canvas.Pix[i+3])
	if alpha < 255 {
		cell := max(1, 8/max(a.zoom, 1))
		checker, alternate := 255, 224
		if a.darkMode {
			checker, alternate = 12, 62
		}
		if ((x/cell)+(y/cell))%2 != 0 {
			checker = alternate
		}
		b = byte((int(b)*alpha + checker*(255-alpha) + 127) / 255)
		g = byte((int(g)*alpha + checker*(255-alpha) + 127) / 255)
		r = byte((int(r)*alpha + checker*(255-alpha) + 127) / 255)
	}
	return selectionGuideUsesWhite(r, g, b)
}

func drawAdaptiveDashedHorizontal(pens *selectionGuidePens, left, right, y, imageLeft, imageY, zoom int, usesWhite func(int, int) bool) {
	if right < left {
		left, right = right, left
	}
	if right <= left || zoom < 1 {
		return
	}
	for x := left; x < right; {
		phase := (x + y) % 6
		if phase < 0 {
			phase += 6
		}
		if phase >= 3 {
			x += 6 - phase
			continue
		}
		run := min(3-phase, right-x)
		mid := x + max(run-1, 0)/2
		imageX := imageLeft + clamp((mid-left)/zoom, 0, max((right-left-1)/zoom, 0))
		pens.line(x, y, x+run, y, usesWhite(imageX, imageY))
		x += run
	}
}

func drawAdaptiveDashedVertical(pens *selectionGuidePens, x, top, bottom, imageX, imageTop, zoom int, usesWhite func(int, int) bool) {
	if bottom < top {
		top, bottom = bottom, top
	}
	if bottom <= top || zoom < 1 {
		return
	}
	for y := top; y < bottom; {
		phase := (y + x) % 6
		if phase < 0 {
			phase += 6
		}
		if phase >= 3 {
			y += 6 - phase
			continue
		}
		run := min(3-phase, bottom-y)
		mid := y + max(run-1, 0)/2
		imageY := imageTop + clamp((mid-top)/zoom, 0, max((bottom-top-1)/zoom, 0))
		pens.line(x, y, x, y+run, usesWhite(imageX, imageY))
		y += run
	}
}

func (a *App) drawAdaptiveMaskedSelectionOutline(hdc uintptr, ox, oy, zoom, x, y, w, h int, mask []bool, width int) {
	if w < 1 || h < 1 || len(mask) != w*h {
		return
	}
	pens := newSelectionGuidePens(hdc, width)
	defer pens.close()
	selected := func(px, py int) bool {
		return px >= 0 && py >= 0 && px < w && py < h && mask[py*w+px]
	}
	drawAxis := func(x0, y0, x1, y1 int, white bool) {
		if x0 == x1 {
			if y1 < y0 {
				y0, y1 = y1, y0
			}
			for p := y0; p < y1; p++ {
				phase := (p + x0) % 6
				if phase < 0 {
					phase += 6
				}
				if phase < 3 {
					pens.line(x0, p, x0, min(p+1, y1), white)
				}
			}
			return
		}
		if y0 == y1 {
			if x1 < x0 {
				x0, x1 = x1, x0
			}
			for p := x0; p < x1; p++ {
				phase := (p + y0) % 6
				if phase < 0 {
					phase += 6
				}
				if phase < 3 {
					pens.line(p, y0, min(p+1, x1), y0, white)
				}
			}
		}
	}
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			if !selected(px, py) {
				continue
			}
			white := a.selectionGuideUsesWhiteAtPixel(x+px, y+py)
			l := ox + (x+px)*zoom
			t := oy + (y+py)*zoom
			r := l + zoom
			b := t + zoom
			if !selected(px, py-1) {
				drawAxis(l, t, r, t, white)
			}
			if !selected(px+1, py) {
				drawAxis(r, t, r, b, white)
			}
			if !selected(px, py+1) {
				drawAxis(l, b, r, b, white)
			}
			if !selected(px-1, py) {
				drawAxis(l, t, l, b, white)
			}
		}
	}
}

func (a *App) drawAdaptiveSelectionRegionOutline(hdc uintptr, ox, oy, zoom, x, y, w, h int, mask []bool, guideW int, handles bool) {
	if w < 1 || h < 1 {
		return
	}
	if len(mask) == w*h {
		a.drawAdaptiveMaskedSelectionOutline(hdc, ox, oy, zoom, x, y, w, h, mask, guideW)
		return
	}
	l, t := ox+x*zoom, oy+y*zoom
	r, b := l+w*zoom, t+h*zoom
	pens := newSelectionGuidePens(hdc, guideW)
	defer pens.close()
	drawAdaptiveDashedHorizontal(pens, l, r, t, x, y, zoom, a.selectionGuideUsesWhiteAtPixel)
	drawAdaptiveDashedVertical(pens, r, t, b, x+w-1, y, zoom, a.selectionGuideUsesWhiteAtPixel)
	drawAdaptiveDashedHorizontal(pens, l, r, b, x, y+h-1, zoom, a.selectionGuideUsesWhiteAtPixel)
	drawAdaptiveDashedVertical(pens, l, t, b, x, y, zoom, a.selectionGuideUsesWhiteAtPixel)
	if handles {
		for _, item := range []struct {
			sx, sy int
			ix, iy int
		}{{l, t, x, y}, {r, t, x + w - 1, y}, {l, b, x, y + h - 1}, {r, b, x + w - 1, y + h - 1}} {
			rc := RECT{int32(item.sx - 3), int32(item.sy - 3), int32(item.sx + 4), int32(item.sy + 4)}
			fillRect(hdc, rc, rgb(255, 255, 255))
			frame := rgb(0, 0, 0)
			if a.selectionGuideUsesWhiteAtPixel(item.ix, item.iy) {
				frame = rgb(255, 255, 255)
			}
			frameRect(hdc, rc, frame)
		}
	}
}

func (a *App) paintSelectionOverlay(hdc uintptr, ox, oy int) {
	s := &a.selection
	guideW := 1
	if a.zoom > 1 {
		guideW = 2
	}
	if s.Creating {
		if s.Adding && s.AddBaseW > 0 && s.AddBaseH > 0 {
			a.drawAdaptiveSelectionRegionOutline(hdc, ox, oy, a.zoom, s.AddBaseX, s.AddBaseY, s.AddBaseW, s.AddBaseH, s.AddBaseMask, guideW, len(s.AddBaseMask) != s.AddBaseW*s.AddBaseH)
		}
		if s.Free && len(s.Lasso) > 1 {
			for i := 1; i < len(s.Lasso); i++ {
				p0, p1 := s.Lasso[i-1], s.Lasso[i]
				white := a.selectionGuideUsesWhiteAtPixel(p1.X, p1.Y)
				color := rgb(0, 0, 0)
				if white {
					color = rgb(255, 255, 255)
				}
				drawDashedLineWidth(hdc, ox+p0.X*a.zoom, oy+p0.Y*a.zoom, ox+p1.X*a.zoom, oy+p1.Y*a.zoom, color, guideW)
			}
		} else {
			x, y, w, h := normalizeSelectionRect(s.StartX, s.StartY, s.CurrentX, s.CurrentY)
			a.drawAdaptiveSelectionRegionOutline(hdc, ox, oy, a.zoom, x, y, w, h, nil, guideW, false)
		}
		return
	}
	if !s.Active {
		return
	}
	// Masked selections (free selection or a Shift-union) show only their real
	// pixel contour. They deliberately do not show a misleading bounding box.
	a.drawAdaptiveSelectionRegionOutline(hdc, ox, oy, a.zoom, s.X, s.Y, s.W, s.H, s.Mask, guideW, len(s.Mask) != s.W*s.H)
}

func (a *App) paintPreview(hdc uintptr, ox, oy int) {
	col := a.fg
	if a.drawButtonRight {
		col = a.bg
	}
	col = gdiPreviewColor(col)
	penW := a.zoom
	if penW < 1 {
		penW = 1
	}
	pen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(penW), uintptr(col))
	old, _, _ := procSelectObject.Call(hdc, pen)
	nullBrush, _, _ := procGetStockObject.Call(NULL_BRUSH)
	oldBrush, _, _ := procSelectObject.Call(hdc, nullBrush)
	switch a.tool {
	case ToolLine:
		// Preview is painted from a temporary pixel canvas using the exact same
		// line rasterizer that will be used on mouse-up.
		procSelectObject.Call(hdc, oldBrush)
		procSelectObject.Call(hdc, old)
		procDeleteObject.Call(pen)
		a.paintRasterLinePreview(hdc, ox, oy)
		return
	case ToolCurve:
		// Curve preview is rendered into a temporary canvas with the same raster
		// path, width, and anti-aliasing code used for final commit.
		procSelectObject.Call(hdc, oldBrush)
		procSelectObject.Call(hdc, old)
		procDeleteObject.Call(pen)
		a.paintRasterCurvePreview(hdc, ox, oy)
		return
	case ToolRectangle:
		// Rectangle preview uses the same native-pixel outer-bound rasterizer as
		// final commit, including inward-only thickness and Shift square locking.
		procSelectObject.Call(hdc, oldBrush)
		procSelectObject.Call(hdc, old)
		procDeleteObject.Call(pen)
		a.paintRasterRectanglePreview(hdc, ox, oy)
		return
	case ToolEllipse:
		procSelectObject.Call(hdc, oldBrush)
		procSelectObject.Call(hdc, old)
		procDeleteObject.Call(pen)
		a.paintRasterEllipsePreview(hdc, ox, oy)
		return
	case ToolRightTriangle:
		procSelectObject.Call(hdc, oldBrush)
		procSelectObject.Call(hdc, old)
		procDeleteObject.Call(pen)
		a.paintRasterRightTrianglePreview(hdc, ox, oy)
		return
	case ToolArrow:
		procSelectObject.Call(hdc, oldBrush)
		procSelectObject.Call(hdc, old)
		procDeleteObject.Call(pen)
		a.paintRasterArrowPreview(hdc, ox, oy)
		return
	}
	procSelectObject.Call(hdc, oldBrush)
	procSelectObject.Call(hdc, old)
	procDeleteObject.Call(pen)
}

// constrainedEightWayEndpoint snaps a drag to the nearest one of eight
// directions: horizontal, vertical, or either 45-degree diagonal. Diagonals
// keep equal native-canvas X/Y steps, so thick brush and eraser strokes remain
// pixel-exact at every zoom level.
func (a *App) constrainedEightWayEndpoint(x0, y0, x1, y1 int) (int, int) {
	dx, dy := x1-x0, y1-y0
	if dx == 0 && dy == 0 {
		return x0, y0
	}
	angle := math.Atan2(float64(dy), float64(dx))
	stepIndex := int(math.Round(angle / (math.Pi / 4.0)))
	stepAngle := float64(stepIndex) * (math.Pi / 4.0)
	sx := int(math.Round(math.Cos(stepAngle)))
	sy := int(math.Round(math.Sin(stepAngle)))
	steps := 0
	if sx == 0 {
		steps = absInt(dy)
	} else if sy == 0 {
		steps = absInt(dx)
	} else {
		// Project the pointer onto an exact 45-degree staircase while preserving
		// smooth live length changes in native canvas pixels.
		steps = int(math.Round(float64(absInt(dx)+absInt(dy)) / 2.0))
	}
	maxSteps := int(^uint(0) >> 2)
	if sx > 0 {
		maxSteps = min(maxSteps, a.canvas.W-1-x0)
	} else if sx < 0 {
		maxSteps = min(maxSteps, x0)
	}
	if sy > 0 {
		maxSteps = min(maxSteps, a.canvas.H-1-y0)
	} else if sy < 0 {
		maxSteps = min(maxSteps, y0)
	}
	steps = clamp(steps, 0, maxSteps)
	return x0 + sx*steps, y0 + sy*steps
}

// constrainedEightWayDelta snaps an arbitrary displacement to horizontal,
// vertical, or either 45-degree diagonal without applying canvas bounds. It is
// used for moving a floating selection because the selection top-left is
// intentionally allowed to become negative while part of the image remains
// visible.
func constrainedEightWayDelta(dx, dy int) (int, int) {
	if dx == 0 && dy == 0 {
		return 0, 0
	}
	angle := math.Atan2(float64(dy), float64(dx))
	stepIndex := int(math.Round(angle / (math.Pi / 4.0)))
	stepAngle := float64(stepIndex) * (math.Pi / 4.0)
	sx := int(math.Round(math.Cos(stepAngle)))
	sy := int(math.Round(math.Sin(stepAngle)))
	steps := 0
	if sx == 0 {
		steps = absInt(dy)
	} else if sy == 0 {
		steps = absInt(dx)
	} else {
		steps = int(math.Round(float64(absInt(dx)+absInt(dy)) / 2.0))
	}
	return sx * steps, sy * steps
}

// updateSelectionMovePosition applies the latest selection drag pointer. Shift
// constrains the displacement from the selection's drag-start position to one
// of eight straight directions. Releasing Shift returns to the raw pointer
// position, so the modifier can be toggled at any time during the same drag.
func (a *App) updateSelectionMovePosition(pointerX, pointerY int, shift bool) {
	s := &a.selection
	if !s.Moving || !s.Floating {
		return
	}
	pointerX = clamp(pointerX, -s.W+1, a.canvas.W-1)
	pointerY = clamp(pointerY, -s.H+1, a.canvas.H-1)
	s.MovePointerX, s.MovePointerY = pointerX, pointerY
	nx := pointerX - s.MoveOffsetX
	ny := pointerY - s.MoveOffsetY
	if shift {
		dx, dy := constrainedEightWayDelta(nx-s.MoveStartX, ny-s.MoveStartY)
		nx, ny = s.MoveStartX+dx, s.MoveStartY+dy
	}
	nx = clamp(nx, -s.W+1, a.canvas.W-1)
	ny = clamp(ny, -s.H+1, a.canvas.H-1)
	a.applyFloatingSelection(nx, ny)
	a.markDirty()
	if shift {
		a.statusText = fmt.Sprintf("선택 영역 직선 이동: %d × %d  위치 [%d,%d]", s.W, s.H, s.X, s.Y)
	} else {
		a.statusText = fmt.Sprintf("선택 영역 이동 중: %d × %d", s.W, s.H)
	}
	a.requestCanvasRepaint(true)
}

type strictTanStep struct {
	X int
	Y int
}

func nearestStrictTanStep(dx, dy float64) strictTanStep {
	length := math.Hypot(dx, dy)
	if length <= 0 {
		return strictTanStep{}
	}
	ux, uy := dx/length, dy/length
	candidates := []strictTanStep{{X: 1, Y: 0}, {X: -1, Y: 0}, {X: 0, Y: 1}, {X: 0, Y: -1}}
	for _, ratio := range lineTanRatios {
		for _, sx := range []int{-1, 1} {
			for _, sy := range []int{-1, 1} {
				candidates = append(candidates, strictTanStep{X: sx * ratio.Run, Y: sy * ratio.Rise})
			}
		}
	}
	best := candidates[0]
	bestDot := -2.0
	for _, d := range candidates {
		norm := math.Hypot(float64(d.X), float64(d.Y))
		dot := ux*float64(d.X)/norm + uy*float64(d.Y)/norm
		if dot > bestDot {
			bestDot = dot
			best = d
		}
	}
	return best
}

func nearestLineDirection(dx, dy float64, mode LineConstraintMode) (float64, float64) {
	length := math.Hypot(dx, dy)
	if length <= 0 {
		return 0, 0
	}
	if mode == LineConstraint15Degrees {
		step := math.Pi / 12.0
		angle := math.Round(math.Atan2(dy, dx)/step) * step
		return math.Cos(angle), math.Sin(angle)
	}
	d := nearestStrictTanStep(dx, dy)
	norm := math.Hypot(float64(d.X), float64(d.Y))
	if norm <= 0 {
		return 0, 0
	}
	return float64(d.X) / norm, float64(d.Y) / norm
}

// strictTanPointAt returns the pixel reached after exactly majorSteps one-pixel
// advances along the chosen integer-ratio staircase. For tan 1/3 the sequence
// is (0,0), (1,0), (2,0), (3,1), (4,1), (5,1), (6,2)... . Thus the body keeps
// the exact three-horizontal-per-one-vertical rhythm while a live drag can grow
// by one canvas pixel instead of waiting for a complete 3:1 block.
func strictTanPointAt(x0, y0 int, step strictTanStep, majorSteps int) PixelPoint {
	if majorSteps <= 0 || (step.X == 0 && step.Y == 0) {
		return PixelPoint{X: x0, Y: y0}
	}
	ax, ay := absInt(step.X), absInt(step.Y)
	sx, sy := 0, 0
	if step.X < 0 {
		sx = -1
	} else if step.X > 0 {
		sx = 1
	}
	if step.Y < 0 {
		sy = -1
	} else if step.Y > 0 {
		sy = 1
	}
	if ax >= ay {
		yOff := 0
		if ax > 0 {
			yOff = (majorSteps * ay) / ax
		}
		return PixelPoint{X: x0 + sx*majorSteps, Y: y0 + sy*yOff}
	}
	xOff := 0
	if ay > 0 {
		xOff = (majorSteps * ax) / ay
	}
	return PixelPoint{X: x0 + sx*xOff, Y: y0 + sy*majorSteps}
}

// strictTanMajorStepsFromPointer uses the dominant canvas axis as the live
// length control. A shallow tan 1/3 or 1/4 line therefore gains exactly one
// path pixel whenever the pointer advances one canvas pixel horizontally; a
// steep line does the same vertically. The minor pointer coordinate chooses
// the nearest allowed ratio but cannot make the endpoint jump by a whole motif.
func strictTanMajorStepsFromPointer(x0, y0, x1, y1 int, step strictTanStep) int {
	ax, ay := absInt(step.X), absInt(step.Y)
	if ax == 0 && ay == 0 {
		return 0
	}
	if ax >= ay {
		if step.X < 0 {
			return max(0, x0-x1)
		}
		return max(0, x1-x0)
	}
	if step.Y < 0 {
		return max(0, y0-y1)
	}
	return max(0, y1-y0)
}

// maxStrictTanMajorSteps clips the staircase without changing its phase. The
// dominant coordinate advances once per raster step; the minor coordinate is
// floor(k*minor/major), so the formulas below find the last k still inside the
// canvas exactly rather than clipping a completed line afterward.
func maxStrictTanMajorSteps(x0, y0 int, step strictTanStep, width, height int) int {
	ax, ay := absInt(step.X), absInt(step.Y)
	if ax == 0 && ay == 0 {
		return 0
	}
	xAvail, yAvail := 0, 0
	if step.X > 0 {
		xAvail = width - 1 - x0
	} else if step.X < 0 {
		xAvail = x0
	} else {
		xAvail = int(^uint(0) >> 2)
	}
	if step.Y > 0 {
		yAvail = height - 1 - y0
	} else if step.Y < 0 {
		yAvail = y0
	} else {
		yAvail = int(^uint(0) >> 2)
	}
	if ax >= ay {
		limit := xAvail
		if ay > 0 {
			minorLimit := ((yAvail+1)*ax - 1) / ay
			limit = min(limit, minorLimit)
		}
		return max(0, limit)
	}
	limit := yAvail
	if ax > 0 {
		minorLimit := ((xAvail+1)*ay - 1) / ax
		limit = min(limit, minorLimit)
	}
	return max(0, limit)
}

// strictTanPixelPrefix returns every successive native canvas pixel from the
// starting point through majorSteps. A partial final motif is intentional: it
// is what lets the preview and committed line grow one dot at a time. All full
// rows/columns before that tail still obey the selected exact integer ratio.
func strictTanPixelPrefix(x0, y0 int, step strictTanStep, majorSteps int) []PixelPoint {
	majorSteps = max(0, majorSteps)
	points := make([]PixelPoint, 0, majorSteps+1)
	for k := 0; k <= majorSteps; k++ {
		points = append(points, strictTanPointAt(x0, y0, step, k))
	}
	return points
}

func (a *App) constrainedStrictTanGeometry(x0, y0, x1, y1 int) ([]PixelPoint, int, int, strictTanStep, int) {
	dx, dy := float64(x1-x0), float64(y1-y0)
	step := nearestStrictTanStep(dx, dy)
	majorSteps := strictTanMajorStepsFromPointer(x0, y0, x1, y1, step)
	majorSteps = clamp(majorSteps, 0, maxStrictTanMajorSteps(x0, y0, step, a.canvas.W, a.canvas.H))
	points := strictTanPixelPrefix(x0, y0, step, majorSteps)
	last := points[len(points)-1]
	return points, last.X, last.Y, step, majorSteps
}

func (a *App) constrainedStrictTanPath(x0, y0, x1, y1 int) ([]PixelPoint, int, int) {
	points, nx, ny, _, _ := a.constrainedStrictTanGeometry(x0, y0, x1, y1)
	return points, nx, ny
}

func (a *App) constrainedLineEndpoint(x0, y0, x1, y1 int, constrain bool) (int, int) {
	if !constrain || (x0 == x1 && y0 == y1) {
		return clamp(x1, 0, a.canvas.W-1), clamp(y1, 0, a.canvas.H-1)
	}
	dx, dy := float64(x1-x0), float64(y1-y0)
	length := math.Hypot(dx, dy)
	if a.lineConstraintMode == LineConstraintTanN {
		_, nx, ny := a.constrainedStrictTanPath(x0, y0, x1, y1)
		return nx, ny
	}
	ux, uy := nearestLineDirection(dx, dy, a.lineConstraintMode)
	if ux == 0 && uy == 0 {
		return x0, y0
	}
	maxLength := length
	if ux > 0 {
		maxLength = math.Min(maxLength, float64(a.canvas.W-1-x0)/ux)
	} else if ux < 0 {
		maxLength = math.Min(maxLength, float64(0-x0)/ux)
	}
	if uy > 0 {
		maxLength = math.Min(maxLength, float64(a.canvas.H-1-y0)/uy)
	} else if uy < 0 {
		maxLength = math.Min(maxLength, float64(0-y0)/uy)
	}
	if maxLength < 0 {
		maxLength = 0
	}
	nx := x0 + int(math.Round(ux*maxLength))
	ny := y0 + int(math.Round(uy*maxLength))
	return clamp(nx, 0, a.canvas.W-1), clamp(ny, 0, a.canvas.H-1)
}

func translatePixelPath(points []PixelPoint, dx, dy int) []PixelPoint {
	if len(points) == 0 || (dx == 0 && dy == 0) {
		return points
	}
	out := make([]PixelPoint, len(points))
	for i, p := range points {
		out[i] = PixelPoint{X: p.X + dx, Y: p.Y + dy}
	}
	return out
}

// drawRasterLineAt calculates constraints in full-canvas coordinates and only
// translates the finished raster geometry into a small preview canvas.  This
// guarantees that boundary clipping, strict tan staircases, parity and AA are
// identical between the A209 dirty-region preview and final commit.
func (a *App) drawRasterLineAt(c *Canvas, x0, y0, x1, y1 int, constrain bool, col uint32, originX, originY int) (int, int) {
	if constrain && a.lineConstraintMode == LineConstraintTanN {
		points, nx, ny, step, majorSteps := a.constrainedStrictTanGeometry(x0, y0, x1, y1)
		points = translatePixelPath(points, -originX, -originY)
		c.DrawStyledStrictTanPath(points, step.X, step.Y, majorSteps, a.lineSize, a.lineAntialias, col)
		return nx, ny
	}
	nx, ny := a.constrainedLineEndpoint(x0, y0, x1, y1, constrain)
	c.DrawStyledLine(x0-originX, y0-originY, nx-originX, ny-originY, a.lineSize, a.lineAntialias, col)
	return nx, ny
}

func (a *App) drawRasterLine(c *Canvas, x0, y0, x1, y1 int, constrain bool, col uint32) (int, int) {
	return a.drawRasterLineAt(c, x0, y0, x1, y1, constrain, col, 0, 0)
}

// constrainedRectangleEndpoint keeps the drag point inside the canvas and,
// while Shift is held, makes the outside width and height equal. The drag
// endpoint remains an outside corner; stroke thickness is applied inward.
func (a *App) constrainedRectangleEndpoint(x0, y0, x1, y1 int, square bool) (int, int) {
	x1 = clamp(x1, 0, a.canvas.W-1)
	y1 = clamp(y1, 0, a.canvas.H-1)
	if !square || (x0 == x1 && y0 == y1) {
		return x1, y1
	}
	dx, dy := x1-x0, y1-y0
	sx, sy := 1, 1
	if dx < 0 {
		sx = -1
	}
	if dy < 0 {
		sy = -1
	}
	side := max(absInt(dx), absInt(dy))
	maxX := a.canvas.W - 1 - x0
	if sx < 0 {
		maxX = x0
	}
	maxY := a.canvas.H - 1 - y0
	if sy < 0 {
		maxY = y0
	}
	side = min(side, min(maxX, maxY))
	return x0 + sx*side, y0 + sy*side
}

func rectangleOuterBounds(x0, y0, x1, y1 int) (left, top, right, bottom, width, height int) {
	left, right = x0, x1
	if left > right {
		left, right = right, left
	}
	top, bottom = y0, y1
	if top > bottom {
		top, bottom = bottom, top
	}
	width = right - left + 1
	height = bottom - top + 1
	return
}

func (a *App) currentRectangleEndpoint() (int, int) {
	return a.constrainedRectangleEndpoint(a.startX, a.startY, a.curX, a.curY, keyDown(VK_SHIFT))
}

func (a *App) drawRasterRectangle(c *Canvas, x0, y0, x1, y1 int, square bool, col uint32) (int, int) {
	return a.drawRasterRectangleAt(c, x0, y0, x1, y1, square, col, 0, 0)
}

// drawRasterRectangleAt uses the same native-pixel geometry as the committed
// rectangle while allowing its temporary preview canvas to start at (originX,originY).
func (a *App) drawRasterRectangleAt(c *Canvas, x0, y0, x1, y1 int, square bool, col uint32, originX, originY int) (int, int) {
	nx, ny := a.constrainedRectangleEndpoint(x0, y0, x1, y1, square)
	c.DrawRectOuter(x0-originX, y0-originY, nx-originX, ny-originY, a.rectangleSize, col)
	if a.shapeShowCenter {
		left, top, _, _, width, height := rectangleOuterBounds(x0, y0, nx, ny)
		c.DrawCenterMarker(left-originX, top-originY, width, height, col)
	}
	return nx, ny
}

func (a *App) paintRasterRectanglePreview(hdc uintptr, ox, oy int) {
	if a.tool != ToolRectangle || !a.preview || len(a.canvas.Pix) == 0 {
		return
	}
	started := time.Now()
	constrain := keyDown(VK_SHIFT)
	left, top, right, bottom := a.rectanglePreviewCanvasBounds(a.curX, a.curY, constrain)
	preview, ok := a.cloneRasterPreviewRegion(left, top, right, bottom)
	if !ok {
		return
	}
	col := a.fg
	if a.drawButtonRight {
		col = a.bg
	}
	a.drawRasterRectangleAt(preview, a.startX, a.startY, a.curX, a.curY, constrain, col, left, top)
	a.blitRasterPreviewRegion(hdc, ox, oy, left, top, preview)
	a.logVectorPreview("rectangle", started, preview)
}
func (a *App) currentEllipseEndpoint() (int, int) {
	return a.constrainedRectangleEndpoint(a.startX, a.startY, a.curX, a.curY, keyDown(VK_SHIFT))
}

func (a *App) drawRasterEllipseAt(c *Canvas, x0, y0, x1, y1 int, circle bool, col uint32, originX, originY int) (int, int) {
	nx, ny := a.constrainedRectangleEndpoint(x0, y0, x1, y1, circle)
	c.DrawEllipseOuter(x0-originX, y0-originY, nx-originX, ny-originY, a.ellipseSize, col)
	if a.shapeShowCenter {
		left, top, _, _, width, height := rectangleOuterBounds(x0, y0, nx, ny)
		c.DrawCenterMarker(left-originX, top-originY, width, height, col)
	}
	return nx, ny
}

func (a *App) drawRasterEllipse(c *Canvas, x0, y0, x1, y1 int, circle bool, col uint32) (int, int) {
	return a.drawRasterEllipseAt(c, x0, y0, x1, y1, circle, col, 0, 0)
}

func (a *App) paintRasterEllipsePreview(hdc uintptr, ox, oy int) {
	if a.tool != ToolEllipse || !a.preview || len(a.canvas.Pix) == 0 {
		return
	}
	started := time.Now()
	circle := keyDown(VK_SHIFT)
	left, top, right, bottom := a.ellipsePreviewCanvasBounds(a.curX, a.curY, circle)
	preview, ok := a.cloneRasterPreviewRegion(left, top, right, bottom)
	if !ok {
		return
	}
	col := a.fg
	if a.drawButtonRight {
		col = a.bg
	}
	a.drawRasterEllipseAt(preview, a.startX, a.startY, a.curX, a.curY, circle, col, left, top)
	a.blitRasterPreviewRegion(hdc, ox, oy, left, top, preview)
	a.logVectorPreview("ellipse", started, preview)
}

// The right-triangle tool is deliberately the straight-line tool plus two
// axis-aligned legs. Mouse-down and mouse-up are the two hypotenuse endpoints;
// neither endpoint is the right-angle vertex. Shift uses the exact same endpoint
// constraint and tan staircase as ToolLine. The added corner is reached by moving
// horizontally from the first endpoint and then vertically to the second.
func (a *App) constrainedRightTriangleEndpoint(x0, y0, x1, y1 int, constrain bool) (int, int) {
	return a.constrainedLineEndpoint(x0, y0, x1, y1, constrain)
}

func (a *App) currentRightTriangleEndpoint() (int, int) {
	return a.constrainedRightTriangleEndpoint(a.startX, a.startY, a.curX, a.curY, keyDown(VK_SHIFT))
}

func (a *App) drawRasterRightTriangleAt(c *Canvas, x0, y0, x1, y1 int, constrain bool, col uint32, originX, originY int) (int, int) {
	nx, ny := x1, y1
	if constrain && a.lineConstraintMode == LineConstraintTanN {
		// The triangle hypotenuse must use the SAME strict-tan renderer as
		// ToolLine. The old per-microsegment square caps in DrawStyledPixelPath
		// protruded as black "feathers" at thick triangle endpoints. The
		// global tan-direction butt-cap clip removes these protrusions while
		// keeping the exact integer staircase and the existing zero-AA policy.
		points, tx, ty, step, majorSteps := a.constrainedStrictTanGeometry(x0, y0, x1, y1)
		nx, ny = tx, ty
		c.DrawStyledStrictTanPath(translatePixelPath(points, -originX, -originY), step.X, step.Y, majorSteps, a.rightTriangleSize, false, col)
	} else {
		nx, ny = a.constrainedRightTriangleEndpoint(x0, y0, x1, y1, constrain)
		c.DrawStyledLine(x0-originX, y0-originY, nx-originX, ny-originY, a.rightTriangleSize, false, col)
	}

	cornerX, cornerY := rightTriangleLegCorner(x0, y0, nx, ny)
	c.DrawStyledLine(x0-originX, y0-originY, cornerX-originX, cornerY-originY, a.rightTriangleSize, false, col)
	c.DrawStyledLine(cornerX-originX, cornerY-originY, nx-originX, ny-originY, a.rightTriangleSize, false, col)
	return nx, ny
}

func (a *App) drawRasterRightTriangle(c *Canvas, x0, y0, x1, y1 int, constrain bool, col uint32) (int, int) {
	return a.drawRasterRightTriangleAt(c, x0, y0, x1, y1, constrain, col, 0, 0)
}

func (a *App) paintRasterRightTrianglePreview(hdc uintptr, ox, oy int) {
	if a.tool != ToolRightTriangle || !a.preview || len(a.canvas.Pix) == 0 {
		return
	}
	started := time.Now()
	constrain := keyDown(VK_SHIFT)
	left, top, right, bottom := a.rightTrianglePreviewCanvasBounds(a.curX, a.curY, constrain)
	preview, ok := a.cloneRasterPreviewRegion(left, top, right, bottom)
	if !ok {
		return
	}
	col := a.fg
	if a.drawButtonRight {
		col = a.bg
	}
	a.drawRasterRightTriangleAt(preview, a.startX, a.startY, a.curX, a.curY, constrain, col, left, top)
	a.blitRasterPreviewRegion(hdc, ox, oy, left, top, preview)
	a.logVectorPreview("right-triangle", started, preview)
}

func (a *App) currentArrowEndpoint() (int, int) {
	return a.constrainedLineEndpoint(a.startX, a.startY, a.curX, a.curY, keyDown(VK_SHIFT))
}

func truncateArrowPixelPath(points []PixelPoint, x0, y0 int, head ArrowHeadGeometry, lineSize int) []PixelPoint {
	if len(points) == 0 {
		return nil
	}
	fullLength := math.Hypot(float64(points[len(points)-1].X-x0), float64(points[len(points)-1].Y-y0))
	shaftLength := fullLength - head.Length
	if shaftLength <= 0 {
		return nil
	}
	last := -1
	for i, p := range points {
		forward := float64(p.X-x0)*head.UnitX + float64(p.Y-y0)*head.UnitY
		if forward <= shaftLength+arrowShaftHeadOverlap(lineSize) {
			last = i
			continue
		}
		break
	}
	if last < 0 {
		return nil
	}
	return points[:last+1]
}

// drawRasterArrow keeps the shaft constraint fully identical to the straight
// line tool, but stops the square/butt-capped shaft at the rear edge of the
// head. The filled head is then drawn from that rear edge to the tip, so no shaft
// can pierce the point. The shaft bypasses AA in cardinal/45-degree directions;
// the head classifies each of its own four edges independently.
func translateArrowHeadGeometry(g ArrowHeadGeometry, dx, dy float64) ArrowHeadGeometry {
	g.Tip.X, g.Tip.Y = g.Tip.X+dx, g.Tip.Y+dy
	g.Base.X, g.Base.Y = g.Base.X+dx, g.Base.Y+dy
	g.SideA.X, g.SideA.Y = g.SideA.X+dx, g.SideA.Y+dy
	g.SideB.X, g.SideB.Y = g.SideB.X+dx, g.SideB.Y+dy
	return g
}

func translateStrictTanShadow(s strictTanShadowSegment, dx, dy float64) strictTanShadowSegment {
	s.X0, s.Y0 = s.X0+dx, s.Y0+dy
	s.X1, s.Y1 = s.X1+dx, s.Y1+dy
	return s
}

func (a *App) drawRasterArrowAt(c *Canvas, x0, y0, x1, y1 int, constrain bool, col uint32, originX, originY int) (int, int) {
	nx, ny := x1, y1
	var strictPath []PixelPoint
	var strictStep strictTanStep
	majorSteps := 0
	strictMode := constrain && a.lineConstraintMode == LineConstraintTanN
	if strictMode {
		strictPath, nx, ny, strictStep, majorSteps = a.constrainedStrictTanGeometry(x0, y0, x1, y1)
	} else {
		nx, ny = a.constrainedLineEndpoint(x0, y0, x1, y1, constrain)
	}

	var head ArrowHeadGeometry
	var ok bool
	var fullShadow strictTanShadowSegment
	shadowOK := false
	if strictMode {
		fullShadow, shadowOK = calculateStrictTanShadowSegment(x0, y0, strictStep.X, strictStep.Y, majorSteps, a.arrowLineSize)
		if shadowOK {
			head, ok = calculateArrowHeadDirected(
				float64(nx), float64(ny),
				fullShadow.UnitX, fullShadow.UnitY, fullShadow.Length,
				a.arrowHeadSize, a.arrowLineSize,
				fullShadow.PhaseOffsetX, fullShadow.PhaseOffsetY,
			)
		}
	} else {
		head, ok = calculateArrowHead(x0, y0, nx, ny, a.arrowHeadSize, a.arrowLineSize)
	}
	if !ok {
		return nx, ny
	}

	shaftAA := arrowEffectiveAntialias(a.arrowAntialias, x0, y0, nx, ny)
	if strictMode {
		shaftAA = arrowEffectiveAntialias(a.arrowAntialias, 0, 0, strictStep.X, strictStep.Y)
	}
	sx, sy := arrowShaftJoinedLogicalEnd(head, a.arrowLineSize)
	hasShaft := math.Hypot(float64(sx-x0), float64(sy-y0)) > 0.01
	if !hasShaft {
		sx, sy = x0, y0
	}
	var shaftPath []PixelPoint
	if len(strictPath) > 0 {
		shaftPath = truncateArrowPixelPath(strictPath, x0, y0, head, a.arrowLineSize)
	}

	var shaftShadow *strictTanShadowSegment
	if strictMode && shadowOK && hasShaft {
		shaftLength := fullShadow.Length - head.Length + arrowShaftHeadOverlap(a.arrowLineSize)
		if shaftLength > 0 {
			trimmed := fullShadow.prefix(shaftLength)
			shaftShadow = &trimmed
		}
	}
	localHead := translateArrowHeadGeometry(head, -float64(originX), -float64(originY))
	localPath := translatePixelPath(shaftPath, -originX, -originY)
	var localShadow *strictTanShadowSegment
	if shaftShadow != nil {
		translated := translateStrictTanShadow(*shaftShadow, -float64(originX), -float64(originY))
		localShadow = &translated
	}
	c.DrawUnifiedVectorArrow(x0-originX, y0-originY, sx-originX, sy-originY, a.arrowLineSize, shaftAA, localHead, a.arrowAntialias, localPath, localShadow, col)
	return nx, ny
}

func (a *App) drawRasterArrow(c *Canvas, x0, y0, x1, y1 int, constrain bool, col uint32) (int, int) {
	return a.drawRasterArrowAt(c, x0, y0, x1, y1, constrain, col, 0, 0)
}

// cloneRasterPreviewRegion copies only the native-pixel rectangle that can be
// changed by the live vector preview.  The previous implementation cloned and
// alpha-scanned the complete canvas on every mouse movement, which dominated
// line/arrow preview cost on large documents.
func (a *App) cloneRasterPreviewRegion(left, top, right, bottom int) (*Canvas, bool) {
	if a.canvas == nil || right < left || bottom < top {
		return nil, false
	}
	left = clamp(left, 0, a.canvas.W-1)
	top = clamp(top, 0, a.canvas.H-1)
	right = clamp(right, left, a.canvas.W-1)
	bottom = clamp(bottom, top, a.canvas.H-1)
	w, h := right-left+1, bottom-top+1
	if w <= 0 || h <= 0 {
		return nil, false
	}
	pix := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		src := ((top+y)*a.canvas.W + left) * 4
		dst := y * w * 4
		copy(pix[dst:dst+w*4], a.canvas.Pix[src:src+w*4])
	}
	return &Canvas{W: w, H: h, Pix: pix}, true
}

func (a *App) blitRasterPreviewRegion(hdc uintptr, ox, oy, left, top int, preview *Canvas) {
	if preview == nil || preview.W <= 0 || preview.H <= 0 || len(preview.Pix) < preview.W*preview.H*4 {
		return
	}
	displayPix := preview.displayPixelsAt(a.zoom, left, top)
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(preview.W), BiHeight: -int32(preview.H), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	procSetStretchBltMode.Call(hdc, COLORONCOLOR)
	procStretchDIBits.Call(
		hdc,
		uintptr(ox+left*a.zoom), uintptr(oy+top*a.zoom),
		uintptr(preview.W*a.zoom), uintptr(preview.H*a.zoom),
		0, 0, uintptr(preview.W), uintptr(preview.H),
		uintptr(unsafe.Pointer(&displayPix[0])), uintptr(unsafe.Pointer(&bmi)),
		DIB_RGB_COLORS, SRCCOPY,
	)
	runtime.KeepAlive(displayPix)
}

func (a *App) logVectorPreview(kind string, started time.Time, preview *Canvas) {
	if preview == nil {
		return
	}
	count := atomic.AddUint64(&a.vectorPreviewCount, 1)
	elapsed := time.Since(started)
	if count <= 12 || count%100 == 0 || elapsed > 12*time.Millisecond {
		logf("VECTOR_PREVIEW kind=%s backend=CPU-DIRTY-REGION region=%dx%d canvas=%dx%d zoom=%d elapsed=%s gpu=false", kind, preview.W, preview.H, a.canvas.W, a.canvas.H, a.zoom, elapsed)
	}
}

func (a *App) paintRasterArrowPreview(hdc uintptr, ox, oy int) {
	if a.tool != ToolArrow || !a.preview || len(a.canvas.Pix) == 0 {
		return
	}
	started := time.Now()
	constrain := keyDown(VK_SHIFT)
	left, top, right, bottom := a.vectorPreviewCanvasBounds(a.curX, a.curY, constrain, true)
	preview, ok := a.cloneRasterPreviewRegion(left, top, right, bottom)
	if !ok {
		return
	}
	col := a.fg
	if a.drawButtonRight {
		col = a.bg
	}
	a.drawRasterArrowAt(preview, a.startX, a.startY, a.curX, a.curY, constrain, col, left, top)
	a.blitRasterPreviewRegion(hdc, ox, oy, left, top, preview)
	a.logVectorPreview("arrow", started, preview)
}

func (a *App) currentLineEndpoint() (int, int) {
	return a.constrainedLineEndpoint(a.startX, a.startY, a.curX, a.curY, keyDown(VK_SHIFT))
}

func (a *App) paintRasterLinePreview(hdc uintptr, ox, oy int) {
	if a.tool != ToolLine || !a.preview || len(a.canvas.Pix) == 0 {
		return
	}
	started := time.Now()
	constrain := keyDown(VK_SHIFT)
	left, top, right, bottom := a.vectorPreviewCanvasBounds(a.curX, a.curY, constrain, false)
	preview, ok := a.cloneRasterPreviewRegion(left, top, right, bottom)
	if !ok {
		return
	}
	col := a.fg
	if a.drawButtonRight {
		col = a.bg
	}
	a.drawRasterLineAt(preview, a.startX, a.startY, a.curX, a.curY, constrain, col, left, top)
	a.blitRasterPreviewRegion(hdc, ox, oy, left, top, preview)
	a.logVectorPreview("line", started, preview)
}

func (a *App) curveActiveControlCount() int {
	if a.curveStage <= 0 {
		return 0
	}
	active := a.curveStage - 1
	if a.drawing {
		active = a.curveStage
	}
	return clamp(active, 0, clamp(a.curveBendCount, 1, 4))
}

func (a *App) curveControlPolygon(activeControls int) []PixelPoint {
	activeControls = clamp(activeControls, 0, 4)
	points := make([]PixelPoint, 0, activeControls+2)
	points = append(points, PixelPoint{X: a.curveX0, Y: a.curveY0})
	if activeControls >= 1 {
		points = append(points, PixelPoint{X: a.curveC1X, Y: a.curveC1Y})
	}
	if activeControls >= 2 {
		points = append(points, PixelPoint{X: a.curveC2X, Y: a.curveC2Y})
	}
	if activeControls >= 3 {
		points = append(points, PixelPoint{X: a.curveC3X, Y: a.curveC3Y})
	}
	if activeControls >= 4 {
		points = append(points, PixelPoint{X: a.curveC4X, Y: a.curveC4Y})
	}
	points = append(points, PixelPoint{X: a.curveX1, Y: a.curveY1})
	return points
}

func (a *App) drawRasterCurveAt(c *Canvas, activeControls int, col uint32, originX, originY int) {
	polygon := translatePixelPath(a.curveControlPolygon(activeControls), -originX, -originY)
	if len(polygon) < 2 {
		return
	}
	if activeControls <= 0 {
		path := appendRasterSegment(nil, polygon[0].X, polygon[0].Y, polygon[len(polygon)-1].X, polygon[len(polygon)-1].Y)
		c.DrawRoundCurvePixelPath(path, a.curveSize, a.curveAntialias, col)
		return
	}
	c.DrawRoundCurvePixelPath(bezierRasterPath(polygon), a.curveSize, a.curveAntialias, col)
}

func (a *App) drawRasterCurve(c *Canvas, activeControls int, col uint32) {
	a.drawRasterCurveAt(c, activeControls, col, 0, 0)
}

func (a *App) paintRasterCurvePreview(hdc uintptr, ox, oy int) {
	if a.tool != ToolCurve || !a.preview || len(a.canvas.Pix) == 0 {
		return
	}
	started := time.Now()
	left, top, right, bottom := a.curvePreviewCanvasBounds()
	preview, ok := a.cloneRasterPreviewRegion(left, top, right, bottom)
	if !ok {
		return
	}
	col := a.fg
	if a.drawButtonRight {
		col = a.bg
	}
	a.drawRasterCurveAt(preview, a.curveActiveControlCount(), col, left, top)
	a.blitRasterPreviewRegion(hdc, ox, oy, left, top, preview)
	a.logVectorPreview("curve", started, preview)
}

func drawOutlineRectWidth(hdc uintptr, left, top, right, bottom int, color uint32, width int) {
	if right <= left || bottom <= top {
		return
	}
	if width < 1 {
		width = 1
	}
	pen, _, _ := procCreatePen.Call(PS_SOLID, uintptr(width), uintptr(color))
	oldPen, _, _ := procSelectObject.Call(hdc, pen)
	nullBrush, _, _ := procGetStockObject.Call(NULL_BRUSH)
	oldBrush, _, _ := procSelectObject.Call(hdc, nullBrush)
	procRectangle.Call(hdc, uintptr(left), uintptr(top), uintptr(right), uintptr(bottom))
	procSelectObject.Call(hdc, oldBrush)
	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(pen)
}

func (a *App) paintEraserCursor(hdc uintptr, ox, oy int) {
	if a.tool != ToolEraser || a.cursorX < 0 || a.cursorY < 0 || a.resizingCanvas || a.adjustingEraserSize {
		return
	}
	left, top, right, bottom := squareStampBounds(a.cursorX, a.cursorY, a.eraserSize)
	left = clamp(left, 0, a.canvas.W)
	top = clamp(top, 0, a.canvas.H)
	right = clamp(right, 0, a.canvas.W)
	bottom = clamp(bottom, 0, a.canvas.H)
	if right <= left || bottom <= top {
		return
	}
	guideW := 1
	if a.zoom > 1 {
		guideW = 2
	}
	l := ox + left*a.zoom
	t := oy + top*a.zoom
	r := ox + right*a.zoom
	b := oy + bottom*a.zoom
	// White under-stroke plus black outline remains visible on both dark and
	// light artwork, like the classic Paint eraser-area cursor.
	drawOutlineRectWidth(hdc, l-1, t-1, r+1, b+1, rgb(255, 255, 255), guideW)
	drawOutlineRectWidth(hdc, l, t, r, b, rgb(0, 0, 0), guideW)
}

func (a *App) paintBrushCursor(hdc uintptr, ox, oy int) {
	if (a.tool != ToolPencil && a.tool != ToolBrush && a.tool != ToolAirbrush) || a.cursorX < 0 || a.cursorY < 0 || a.resizingCanvas || a.adjustingBrushSize {
		return
	}
	left, top, right, bottom := squareStampBounds(a.cursorX, a.cursorY, a.brushSize)
	left = clamp(left, 0, a.canvas.W)
	top = clamp(top, 0, a.canvas.H)
	right = clamp(right, 0, a.canvas.W)
	bottom = clamp(bottom, 0, a.canvas.H)
	if right <= left || bottom <= top {
		return
	}
	guideW := 1
	if a.zoom > 1 {
		guideW = 2
	}
	l := ox + left*a.zoom
	t := oy + top*a.zoom
	r := ox + right*a.zoom
	b := oy + bottom*a.zoom
	drawOutlineRectWidth(hdc, l-1, t-1, r+1, b+1, rgb(255, 255, 255), guideW)
	drawOutlineRectWidth(hdc, l, t, r, b, rgb(0, 0, 0), guideW)
}

func brushShapeOptionRects() [4]RECT {
	optTop := toolOptionsTop()
	return [4]RECT{
		{12, int32(optTop + 8), 45, int32(optTop + 38)},
		{53, int32(optTop + 8), 86, int32(optTop + 38)},
		{12, int32(optTop + 44), 45, int32(optTop + 74)},
		{53, int32(optTop + 44), 86, int32(optTop + 74)},
	}
}

func (a *App) hitBrushShapeOption(x, y int) (BrushShape, bool) {
	for i, rc := range brushShapeOptionRects() {
		if pointInRect(x, y, rc) {
			return BrushShape(i), true
		}
	}
	return BrushSquare, false
}

func (a *App) setBrushShape(shape BrushShape) {
	if shape < BrushSquare || shape > BrushSlashDown {
		shape = BrushSquare
	}
	a.brushShape = shape
	a.statusText = fmt.Sprintf("붓 모양: %s, 크기: %d px.", brushShapeNames[shape], a.brushSize)
	logf("BRUSH shape=%d name=%q size=%d", shape, brushShapeNames[shape], a.brushSize)
	invalidate(a.hwnd)
}

func colorToleranceSliderBounds(rc RECT) (trackX, trackTop, trackBottom int) {
	optTop := toolOptionsTop()
	// Match the eraser option slider's geometry so the tool options feel like
	// one family: centered value preview above a fixed 182 px vertical track.
	trackX = 36
	trackTop = optTop + 88
	trackBottom = trackTop + 182
	return
}

func colorToleranceSliderY(value int, rc RECT) int {
	_, top, bottom := colorToleranceSliderBounds(rc)
	value = clamp(value, 0, 100)
	if bottom <= top {
		return top
	}
	return top + int(math.Round(float64(value)*float64(bottom-top)/100.0))
}

func colorToleranceFromSliderY(y int, rc RECT) int {
	_, top, bottom := colorToleranceSliderBounds(rc)
	if bottom <= top {
		return 0
	}
	y = clamp(y, top, bottom)
	return clamp(int(math.Round(float64(y-top)*100.0/float64(bottom-top))), 0, 100)
}

func (a *App) hitColorToleranceOption(x, y int, rc RECT) (int, bool) {
	_, top, bottom := colorToleranceSliderBounds(rc)
	if x < 8 || x > toolboxOptionRight-2 || y < top-10 || y > bottom+10 {
		return 0, false
	}
	return colorToleranceFromSliderY(y, rc), true
}

func (a *App) setColorTolerance(value int) {
	value = clamp(value, 0, 100)
	if a.colorTolerance == value {
		return
	}
	a.colorTolerance = value
	a.statusText = fmt.Sprintf("허용 불일치: %d. 선택과 색 채우기에 같은 RGB 직선거리 기준을 적용합니다.", value)
	logf("COLOR_TOLERANCE mismatch=%d metric=rgb-euclidean tool=%d", value, a.tool)
}

func (a *App) paintColorToleranceOptions(hdc uintptr, rc RECT, optTop int) {
	label := RECT{8, int32(optTop + 6), 90, int32(optTop + 26)}
	drawToolOptionText(hdc, "허용 불일치", label, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 9)

	// Use the same square preview/value area as the eraser option panel.  Here it
	// displays the current numeric mismatch allowance rather than an eraser stamp.
	preview := RECT{25, int32(optTop + 29), 73, int32(optTop + 77)}
	fillUIRect(hdc, preview, rgb(245, 245, 245))
	frameUIRect(hdc, preview, rgb(190, 190, 190))
	drawUIText(hdc, fmt.Sprintf("%d", a.colorTolerance), preview, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 80, 160))

	trackX, trackTop, trackBottom := colorToleranceSliderBounds(rc)
	fillUIRect(hdc, RECT{int32(trackX), int32(trackTop), int32(trackX + 2), int32(trackBottom + 1)}, rgb(95, 95, 95))

	// Match the eraser slider's two-segment guide shape.  The guide is only a
	// visual affordance; the actual value remains a continuous integer 0..100.
	graphLeft := trackX + 8
	graphRight := toolboxOptionRight - 10
	if graphRight <= graphLeft {
		graphRight = graphLeft + 10
	}
	xAt0 := graphLeft + 1
	xAt50 := graphLeft + 18
	xAt100 := graphRight
	if xAt50 >= xAt100-4 {
		xAt50 = graphLeft + max((graphRight-graphLeft)/2, 8)
	}
	yAt0 := colorToleranceSliderY(0, rc)
	yAt50 := colorToleranceSliderY(50, rc)
	yAt100 := colorToleranceSliderY(100, rc)
	linePen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(65, 65, 65))))
	oldLinePen, _, _ := procSelectObject.Call(hdc, linePen)
	procMoveToEx.Call(hdc, uintptr(xAt0), uintptr(yAt0), 0)
	procLineTo.Call(hdc, uintptr(xAt50), uintptr(yAt50))
	procLineTo.Call(hdc, uintptr(xAt100), uintptr(yAt100))
	procSelectObject.Call(hdc, oldLinePen)
	procDeleteObject.Call(linePen)

	currentX := xAt0
	if a.colorTolerance <= 50 {
		t := float64(a.colorTolerance) / 50.0
		currentX = xAt0 + int(math.Round(t*float64(xAt50-xAt0)))
	} else {
		t := float64(a.colorTolerance-50) / 50.0
		currentX = xAt50 + int(math.Round(t*float64(xAt100-xAt50)))
	}
	currentY := colorToleranceSliderY(a.colorTolerance, rc)
	currentBrush, _, _ := procCreateSolidBrush.Call(uintptr(uiColor(rgb(0, 120, 215))))
	oldCurrentBrush, _, _ := procSelectObject.Call(hdc, currentBrush)
	currentPen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(0, 85, 170))))
	oldCurrentPen, _, _ := procSelectObject.Call(hdc, currentPen)
	procEllipse.Call(hdc, uintptr(currentX-3), uintptr(currentY-3), uintptr(currentX+4), uintptr(currentY+4))
	procSelectObject.Call(hdc, oldCurrentPen)
	procDeleteObject.Call(currentPen)
	procSelectObject.Call(hdc, oldCurrentBrush)
	procDeleteObject.Call(currentBrush)

	for _, value := range []int{0, 25, 50, 75, 100} {
		y := colorToleranceSliderY(value, rc)
		fillUIRect(hdc, RECT{int32(trackX - 4), int32(y), int32(trackX + 5), int32(y + 1)}, rgb(105, 105, 105))
		drawUIText(hdc, fmt.Sprintf("%d", value), RECT{3, int32(y - 8), int32(trackX - 8), int32(y + 9)}, DT_RIGHT|DT_VCENTER|DT_SINGLELINE, rgb(70, 70, 70))
	}
	thumbY := colorToleranceSliderY(a.colorTolerance, rc)
	thumb := RECT{int32(trackX - 6), int32(thumbY - 4), int32(trackX + 7), int32(thumbY + 5)}
	fillUIRect(hdc, thumb, rgb(205, 230, 255))
	frameUIRect(hdc, thumb, rgb(0, 120, 215))
}

func brushSliderBounds(rc RECT) (trackX, trackTop, trackBottom int) {
	optTop := toolOptionsTop()
	trackX = 36
	trackTop = optTop + 104
	trackBottom = trackTop + 182
	return
}

func brushSliderYForSize(size int, rc RECT) int {
	_, top, bottom := brushSliderBounds(rc)
	index := eraserSliderIndexForSize(size)
	return top + int(math.Round(float64(index)*float64(bottom-top)/float64(len(eraserSliderSizes)-1)))
}

func brushSizeFromSliderY(y int, rc RECT) int {
	_, top, bottom := brushSliderBounds(rc)
	y = clamp(y, top, bottom)
	index := int(math.Round(float64(y-top) * float64(len(eraserSliderSizes)-1) / float64(bottom-top)))
	index = clamp(index, 0, len(eraserSliderSizes)-1)
	return eraserSliderSizes[index]
}

func (a *App) hitBrushSizeOption(x, y int, rc RECT) (int, bool) {
	_, top, bottom := brushSliderBounds(rc)
	if x < 8 || x > toolboxOptionRight-2 || y < top-10 || y > bottom+10 {
		return 0, false
	}
	return brushSizeFromSliderY(y, rc), true
}

func (a *App) setBrushSize(size int) {
	if size <= 20 {
		size = clamp(size, 1, 20)
	} else {
		// Snap the large range to 24, 28, 32, 36, or 40 px.
		size = 20 + int(math.Round(float64(size-20)/4.0))*4
		size = clamp(size, 24, 40)
	}
	if a.brushSize == size {
		return
	}
	a.brushSize = size
	a.statusText = fmt.Sprintf("붓 크기: %d px, 모양: %s.", size, brushShapeNames[a.brushShape])
	logf("BRUSH size=%d shape=%d", size, a.brushShape)
}

func drawBrushShapeIcon(hdc uintptr, rc RECT, shape BrushShape, clr uint32) {
	cx := int(rc.Left+rc.Right) / 2
	cy := int(rc.Top+rc.Bottom) / 2
	br, _, _ := procCreateSolidBrush.Call(uintptr(clr))
	oldBr, _, _ := procSelectObject.Call(hdc, br)
	pen, _, _ := procCreatePen.Call(PS_SOLID, 2, uintptr(clr))
	oldPen, _, _ := procSelectObject.Call(hdc, pen)
	switch shape {
	case BrushCircle:
		procEllipse.Call(hdc, uintptr(cx-6), uintptr(cy-6), uintptr(cx+7), uintptr(cy+7))
	case BrushSlashUp:
		procMoveToEx.Call(hdc, uintptr(cx-7), uintptr(cy+7), 0)
		procLineTo.Call(hdc, uintptr(cx+7), uintptr(cy-7))
	case BrushSlashDown:
		procMoveToEx.Call(hdc, uintptr(cx-7), uintptr(cy-7), 0)
		procLineTo.Call(hdc, uintptr(cx+7), uintptr(cy+7))
	default:
		procRectangle.Call(hdc, uintptr(cx-6), uintptr(cy-6), uintptr(cx+7), uintptr(cy+7))
	}
	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(pen)
	procSelectObject.Call(hdc, oldBr)
	procDeleteObject.Call(br)
}

func (a *App) paintBrushOptions(hdc uintptr, rc RECT, optTop int) {
	for i, r := range brushShapeOptionRects() {
		selected := a.brushShape == BrushShape(i)
		if selected {
			fillUIRect(hdc, r, rgb(205, 230, 255))
			frameUIRect(hdc, r, rgb(0, 120, 215))
		} else {
			fillUIRect(hdc, r, rgb(248, 248, 248))
			frameUIRect(hdc, r, rgb(190, 190, 190))
		}
		shapeColor := gdiPreviewColor(a.fg)
		if darkModeEnabled() {
			shapeColor = invertToolIconColor(shapeColor)
		}
		drawBrushShapeIcon(hdc, r, BrushShape(i), shapeColor)
	}
	label := RECT{10, int32(optTop + 78), 88, int32(optTop + 98)}
	drawUIText(hdc, fmt.Sprintf("%d px", a.brushSize), label, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0))

	trackX, trackTop, trackBottom := brushSliderBounds(rc)
	fillUIRect(hdc, RECT{int32(trackX), int32(trackTop), int32(trackX + 2), int32(trackBottom + 1)}, rgb(95, 95, 95))
	graphLeft := trackX + 8
	graphRight := toolboxOptionRight - 10
	xAt1 := graphLeft + 1
	xAt20 := graphLeft + 18
	xAt40 := graphRight
	if xAt20 >= xAt40-4 {
		xAt20 = graphLeft + max((graphRight-graphLeft)/2, 8)
	}
	graphXForSize := func(size int) int {
		size = clamp(size, 1, 40)
		if size <= 20 {
			t := float64(size-1) / 19.0
			return xAt1 + int(math.Round(t*float64(xAt20-xAt1)))
		}
		t := float64(size-20) / 20.0
		return xAt20 + int(math.Round(t*float64(xAt40-xAt20)))
	}
	linePen, _, _ := procCreatePen.Call(PS_SOLID, 2, uintptr(uiColor(rgb(65, 65, 65))))
	oldLinePen, _, _ := procSelectObject.Call(hdc, linePen)
	procMoveToEx.Call(hdc, uintptr(xAt1), uintptr(trackTop), 0)
	procLineTo.Call(hdc, uintptr(xAt20), uintptr(brushSliderYForSize(20, rc)))
	procLineTo.Call(hdc, uintptr(xAt40), uintptr(trackBottom))
	procSelectObject.Call(hdc, oldLinePen)
	procDeleteObject.Call(linePen)

	currentX := graphXForSize(a.brushSize)
	currentY := brushSliderYForSize(a.brushSize, rc)
	currentBrush, _, _ := procCreateSolidBrush.Call(uintptr(uiColor(rgb(0, 120, 215))))
	oldCurrentBrush, _, _ := procSelectObject.Call(hdc, currentBrush)
	currentPen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(0, 85, 170))))
	oldCurrentPen, _, _ := procSelectObject.Call(hdc, currentPen)
	procEllipse.Call(hdc, uintptr(currentX-3), uintptr(currentY-3), uintptr(currentX+4), uintptr(currentY+4))
	procSelectObject.Call(hdc, oldCurrentPen)
	procDeleteObject.Call(currentPen)
	procSelectObject.Call(hdc, oldCurrentBrush)
	procDeleteObject.Call(currentBrush)

	for _, size := range []int{1, 10, 20, 40} {
		ty := brushSliderYForSize(size, rc)
		fillUIRect(hdc, RECT{int32(trackX - 4), int32(ty), int32(trackX + 5), int32(ty + 1)}, rgb(105, 105, 105))
		tr := RECT{5, int32(ty - 8), int32(trackX - 8), int32(ty + 9)}
		drawUIText(hdc, fmt.Sprintf("%d", size), tr, DT_RIGHT|DT_VCENTER|DT_SINGLELINE, rgb(70, 70, 70))
	}
	thumbY := brushSliderYForSize(a.brushSize, rc)
	thumb := RECT{int32(trackX - 6), int32(thumbY - 4), int32(trackX + 7), int32(thumbY + 5)}
	fillUIRect(hdc, thumb, rgb(205, 230, 255))
	frameUIRect(hdc, thumb, rgb(0, 120, 215))
}

var eraserSliderSizes = []int{
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
	24, 28, 32, 36, 40,
}

func eraserSliderBounds(rc RECT) (trackX, trackTop, trackBottom int) {
	optTop := toolOptionsTop()
	trackX = 36
	trackTop = optTop + 88
	// Keep the eraser option control visually stable even when the main window
	// is resized.  The slider and guide use a fixed vertical extent instead of
	// stretching with the window height.
	trackBottom = trackTop + 182
	return
}

func eraserSliderIndexForSize(size int) int {
	best := 0
	bestDiff := absInt(size - eraserSliderSizes[0])
	for i := 1; i < len(eraserSliderSizes); i++ {
		d := absInt(size - eraserSliderSizes[i])
		if d < bestDiff {
			best = i
			bestDiff = d
		}
	}
	return best
}

func eraserSliderYForSize(size int, rc RECT) int {
	_, top, bottom := eraserSliderBounds(rc)
	if bottom <= top || len(eraserSliderSizes) <= 1 {
		return top
	}
	index := eraserSliderIndexForSize(size)
	return top + int(math.Round(float64(index)*float64(bottom-top)/float64(len(eraserSliderSizes)-1)))
}

func eraserSizeFromSliderY(y int, rc RECT) int {
	_, top, bottom := eraserSliderBounds(rc)
	y = clamp(y, top, bottom)
	if bottom <= top || len(eraserSliderSizes) == 0 {
		return 1
	}
	index := int(math.Round(float64(y-top) * float64(len(eraserSliderSizes)-1) / float64(bottom-top)))
	index = clamp(index, 0, len(eraserSliderSizes)-1)
	return eraserSliderSizes[index]
}

func (a *App) hitEraserSizeOption(x, y int, rc RECT) (int, bool) {
	_, top, bottom := eraserSliderBounds(rc)
	if x < 8 || x > toolboxOptionRight-2 || y < top-10 || y > bottom+10 {
		return 0, false
	}
	return eraserSizeFromSliderY(y, rc), true
}

func (a *App) setEraserSize(size int) {
	size = clamp(size, 1, 40)
	if a.eraserSize == size {
		return
	}
	a.eraserSize = size
	stepText := "1 px 단위"
	if size > 20 {
		stepText = "4 px 단위"
	}
	a.statusText = fmt.Sprintf("지우개 크기: %d × %d 픽셀 (%s). 캔버스의 사각형 표시가 실제로 지워질 영역입니다.", size, size, stepText)
	logf("ERASER size=%d scaleIndex=%d", size, eraserSliderIndexForSize(size))
}

func rectangleSliderBounds(rc RECT) (trackX, trackTop, trackBottom int) {
	optTop := toolOptionsTop()
	trackX = 36
	trackTop = optTop + 74
	trackBottom = trackTop + 194
	return
}

func ellipseSliderBounds(rc RECT) (trackX, trackTop, trackBottom int) {
	return rectangleSliderBounds(rc)
}

func rectangleSliderYForSize(size int, rc RECT) int {
	_, top, bottom := rectangleSliderBounds(rc)
	index := eraserSliderIndexForSize(size)
	return top + int(math.Round(float64(index)*float64(bottom-top)/float64(len(eraserSliderSizes)-1)))
}

func rectangleSizeFromSliderY(y int, rc RECT) int {
	_, top, bottom := rectangleSliderBounds(rc)
	y = clamp(y, top, bottom)
	index := int(math.Round(float64(y-top) * float64(len(eraserSliderSizes)-1) / float64(bottom-top)))
	index = clamp(index, 0, len(eraserSliderSizes)-1)
	return eraserSliderSizes[index]
}
func ellipseSliderYForSize(size int, rc RECT) int {
	_, top, bottom := ellipseSliderBounds(rc)
	index := eraserSliderIndexForSize(size)
	return top + int(math.Round(float64(index)*float64(bottom-top)/float64(len(eraserSliderSizes)-1)))
}

func ellipseSizeFromSliderY(y int, rc RECT) int {
	_, top, bottom := ellipseSliderBounds(rc)
	y = clamp(y, top, bottom)
	index := int(math.Round(float64(y-top) * float64(len(eraserSliderSizes)-1) / float64(bottom-top)))
	index = clamp(index, 0, len(eraserSliderSizes)-1)
	return eraserSliderSizes[index]
}

func shapeCenterOptionRect() RECT {
	optTop := toolOptionsTop()
	return RECT{10, int32(optTop + 8), 88, int32(optTop + 30)}
}

func (a *App) hitShapeCenterOption(x, y int) bool {
	return pointInRect(x, y, shapeCenterOptionRect())
}

func (a *App) toggleShapeCenter() {
	a.shapeShowCenter = !a.shapeShowCenter
	state := "끔"
	if a.shapeShowCenter {
		state = "켬"
	}
	a.statusText = "도형 중심점 표시: " + state
	logf("SHAPE center-marker=%v", a.shapeShowCenter)
}

func (a *App) paintShapeCenterOption(hdc uintptr) {
	r := shapeCenterOptionRect()
	box := RECT{r.Left + 2, r.Top + 3, r.Left + 18, r.Top + 19}
	fillUIRect(hdc, box, rgb(255, 255, 255))
	frameUIRect(hdc, box, rgb(80, 80, 80))
	if a.shapeShowCenter {
		linePen, _, _ := procCreatePen.Call(PS_SOLID, 2, uintptr(uiColor(rgb(0, 120, 215))))
		oldLinePen, _, _ := procSelectObject.Call(hdc, linePen)
		procMoveToEx.Call(hdc, uintptr(int(box.Left)+3), uintptr(int(box.Top)+8), 0)
		procLineTo.Call(hdc, uintptr(int(box.Left)+7), uintptr(int(box.Bottom)-4))
		procLineTo.Call(hdc, uintptr(int(box.Right)-3), uintptr(int(box.Top)+4))
		procSelectObject.Call(hdc, oldLinePen)
		procDeleteObject.Call(linePen)
	}
	tr := RECT{box.Right + 5, r.Top, r.Right, r.Bottom}
	drawToolOptionText(hdc, "중심점 표시", tr, DT_LEFT|DT_VCENTER|DT_SINGLELINE, rgb(30, 30, 30), 9)
}

func shapeCenterMarkerBounds(left, top, width, height int) (mx0, my0, mx1, my1 int) {
	mw := 1
	mh := 1
	if width%2 == 0 {
		mw = 2
	}
	if height%2 == 0 {
		mh = 2
	}
	mx0 = left + (width-mw)/2
	my0 = top + (height-mh)/2
	mx1 = mx0 + mw - 1
	my1 = my0 + mh - 1
	return
}

func (c *Canvas) DrawCenterMarker(left, top, width, height int, clr uint32) {
	if width <= 0 || height <= 0 {
		return
	}
	mx0, my0, mx1, my1 := shapeCenterMarkerBounds(left, top, width, height)
	for y := my0; y <= my1; y++ {
		for x := mx0; x <= mx1; x++ {
			c.SetPixel(x, y, clr)
		}
	}
}

func (a *App) hitRectangleSizeOption(x, y int, rc RECT) (int, bool) {
	_, top, bottom := rectangleSliderBounds(rc)
	if x < 8 || x > toolboxOptionRight-2 || y < top-10 || y > bottom+10 {
		return 0, false
	}
	return rectangleSizeFromSliderY(y, rc), true
}

func (a *App) setRectangleSize(size int) {
	if size <= 20 {
		size = clamp(size, 1, 20)
	} else {
		size = 20 + int(math.Round(float64(size-20)/4.0))*4
		size = clamp(size, 24, 40)
	}
	if a.rectangleSize == size {
		return
	}
	a.rectangleSize = size
	a.statusText = fmt.Sprintf("사각형 선 굵기: %d px. 선은 드래그한 바깥 크기의 안쪽으로 그려집니다.", size)
	logf("RECTANGLE size=%d", size)
}

func (a *App) paintRectangleOptions(hdc uintptr, rc RECT, optTop int) {
	a.paintShapeCenterOption(hdc)
	drawToolOptionText(hdc, fmt.Sprintf("선 굵기 %d px", a.rectangleSize), RECT{8, int32(optTop + 38), 90, int32(optTop + 62)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 9)
	trackX, trackTop, trackBottom := rectangleSliderBounds(rc)
	fillUIRect(hdc, RECT{int32(trackX), int32(trackTop), int32(trackX + 2), int32(trackBottom + 1)}, rgb(95, 95, 95))
	graphLeft := trackX + 8
	graphRight := toolboxOptionRight - 10
	xAt1 := graphLeft + 1
	xAt20 := graphLeft + 18
	xAt40 := graphRight
	if xAt20 >= xAt40-4 {
		xAt20 = graphLeft + max((graphRight-graphLeft)/2, 8)
	}
	yAt1 := rectangleSliderYForSize(1, rc)
	yAt20 := rectangleSliderYForSize(20, rc)
	yAt40 := rectangleSliderYForSize(40, rc)
	linePen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(65, 65, 65))))
	oldLinePen, _, _ := procSelectObject.Call(hdc, linePen)
	procMoveToEx.Call(hdc, uintptr(xAt1), uintptr(yAt1), 0)
	procLineTo.Call(hdc, uintptr(xAt20), uintptr(yAt20))
	procLineTo.Call(hdc, uintptr(xAt40), uintptr(yAt40))
	procSelectObject.Call(hdc, oldLinePen)
	procDeleteObject.Call(linePen)
	currentX := xAt1
	if a.rectangleSize <= 20 {
		t := float64(a.rectangleSize-1) / 19.0
		currentX = xAt1 + int(math.Round(t*float64(xAt20-xAt1)))
	} else {
		t := float64(a.rectangleSize-20) / 20.0
		currentX = xAt20 + int(math.Round(t*float64(xAt40-xAt20)))
	}
	currentY := rectangleSliderYForSize(a.rectangleSize, rc)
	currentBrush, _, _ := procCreateSolidBrush.Call(uintptr(uiColor(rgb(0, 120, 215))))
	oldCurrentBrush, _, _ := procSelectObject.Call(hdc, currentBrush)
	currentPen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(0, 85, 170))))
	oldCurrentPen, _, _ := procSelectObject.Call(hdc, currentPen)
	procEllipse.Call(hdc, uintptr(currentX-3), uintptr(currentY-3), uintptr(currentX+4), uintptr(currentY+4))
	procSelectObject.Call(hdc, oldCurrentPen)
	procDeleteObject.Call(currentPen)
	procSelectObject.Call(hdc, oldCurrentBrush)
	procDeleteObject.Call(currentBrush)
	for _, size := range []int{1, 10, 20, 40} {
		ty := rectangleSliderYForSize(size, rc)
		fillUIRect(hdc, RECT{int32(trackX - 4), int32(ty), int32(trackX + 5), int32(ty + 1)}, rgb(105, 105, 105))
		tr := RECT{5, int32(ty - 8), int32(trackX - 8), int32(ty + 9)}
		drawUIText(hdc, fmt.Sprintf("%d", size), tr, DT_RIGHT|DT_VCENTER|DT_SINGLELINE, rgb(70, 70, 70))
	}
	thumbY := rectangleSliderYForSize(a.rectangleSize, rc)
	thumb := RECT{int32(trackX - 6), int32(thumbY - 4), int32(trackX + 7), int32(thumbY + 5)}
	fillUIRect(hdc, thumb, rgb(205, 230, 255))
	frameUIRect(hdc, thumb, rgb(0, 120, 215))
	drawToolOptionText(hdc, "Shift: 정사각형", RECT{8, int32(trackBottom + 10), 90, int32(trackBottom + 30)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(70, 70, 70), 8)
}

func (a *App) hitEllipseSizeOption(x, y int, rc RECT) (int, bool) {
	_, top, bottom := ellipseSliderBounds(rc)
	if x < 8 || x > toolboxOptionRight-2 || y < top-10 || y > bottom+10 {
		return 0, false
	}
	return ellipseSizeFromSliderY(y, rc), true
}

func (a *App) setEllipseSize(size int) {
	if size <= 20 {
		size = clamp(size, 1, 20)
	} else {
		size = 20 + int(math.Round(float64(size-20)/4.0))*4
		size = clamp(size, 24, 40)
	}
	if a.ellipseSize == size {
		return
	}
	a.ellipseSize = size
	a.statusText = fmt.Sprintf("타원 선 굵기: %d px. 선은 드래그한 바깥 크기의 안쪽으로 그려집니다.", size)
	logf("ELLIPSE size=%d", size)
}

func (a *App) paintEllipseOptions(hdc uintptr, rc RECT, optTop int) {
	a.paintShapeCenterOption(hdc)
	drawToolOptionText(hdc, fmt.Sprintf("선 굵기 %d px", a.ellipseSize), RECT{8, int32(optTop + 38), 90, int32(optTop + 62)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 9)
	trackX, trackTop, trackBottom := ellipseSliderBounds(rc)
	fillUIRect(hdc, RECT{int32(trackX), int32(trackTop), int32(trackX + 2), int32(trackBottom + 1)}, rgb(95, 95, 95))
	graphLeft := trackX + 8
	graphRight := toolboxOptionRight - 10
	xAt1 := graphLeft + 1
	xAt20 := graphLeft + 18
	xAt40 := graphRight
	if xAt20 >= xAt40-4 {
		xAt20 = graphLeft + max((graphRight-graphLeft)/2, 8)
	}
	yAt1 := ellipseSliderYForSize(1, rc)
	yAt20 := ellipseSliderYForSize(20, rc)
	yAt40 := ellipseSliderYForSize(40, rc)
	linePen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(65, 65, 65))))
	oldLinePen, _, _ := procSelectObject.Call(hdc, linePen)
	procMoveToEx.Call(hdc, uintptr(xAt1), uintptr(yAt1), 0)
	procLineTo.Call(hdc, uintptr(xAt20), uintptr(yAt20))
	procLineTo.Call(hdc, uintptr(xAt40), uintptr(yAt40))
	procSelectObject.Call(hdc, oldLinePen)
	procDeleteObject.Call(linePen)
	currentX := xAt1
	if a.ellipseSize <= 20 {
		t := float64(a.ellipseSize-1) / 19.0
		currentX = xAt1 + int(math.Round(t*float64(xAt20-xAt1)))
	} else {
		t := float64(a.ellipseSize-20) / 20.0
		currentX = xAt20 + int(math.Round(t*float64(xAt40-xAt20)))
	}
	currentY := ellipseSliderYForSize(a.ellipseSize, rc)
	currentBrush, _, _ := procCreateSolidBrush.Call(uintptr(uiColor(rgb(0, 120, 215))))
	oldCurrentBrush, _, _ := procSelectObject.Call(hdc, currentBrush)
	currentPen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(0, 85, 170))))
	oldCurrentPen, _, _ := procSelectObject.Call(hdc, currentPen)
	procEllipse.Call(hdc, uintptr(currentX-3), uintptr(currentY-3), uintptr(currentX+4), uintptr(currentY+4))
	procSelectObject.Call(hdc, oldCurrentPen)
	procDeleteObject.Call(currentPen)
	procSelectObject.Call(hdc, oldCurrentBrush)
	procDeleteObject.Call(currentBrush)
	for _, size := range []int{1, 10, 20, 40} {
		ty := ellipseSliderYForSize(size, rc)
		fillUIRect(hdc, RECT{int32(trackX - 4), int32(ty), int32(trackX + 5), int32(ty + 1)}, rgb(105, 105, 105))
		tr := RECT{5, int32(ty - 8), int32(trackX - 8), int32(ty + 9)}
		drawUIText(hdc, fmt.Sprintf("%d", size), tr, DT_RIGHT|DT_VCENTER|DT_SINGLELINE, rgb(70, 70, 70))
	}
	thumbY := ellipseSliderYForSize(a.ellipseSize, rc)
	thumb := RECT{int32(trackX - 6), int32(thumbY - 4), int32(trackX + 7), int32(thumbY + 5)}
	fillUIRect(hdc, thumb, rgb(205, 230, 255))
	frameUIRect(hdc, thumb, rgb(0, 120, 215))
	drawToolOptionText(hdc, "Shift: 원", RECT{8, int32(trackBottom + 10), 90, int32(trackBottom + 30)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(70, 70, 70), 9)
}

func rightTriangleConstraintOptionRects() (angleRect, tanRect RECT) {
	optTop := toolOptionsTop()
	angleRect = RECT{12, int32(optTop + 24), 86, int32(optTop + 47)}
	tanRect = RECT{12, int32(optTop + 50), 86, int32(optTop + 73)}
	return
}

func rightTriangleSliderBounds(rc RECT) (trackX, trackTop, trackBottom int) {
	optTop := toolOptionsTop()
	trackX = 36
	trackTop = optTop + 111
	trackBottom = trackTop + 176
	return
}

func rightTriangleSliderYForSize(size int, rc RECT) int {
	_, top, bottom := rightTriangleSliderBounds(rc)
	index := eraserSliderIndexForSize(size)
	return top + int(math.Round(float64(index)*float64(bottom-top)/float64(len(eraserSliderSizes)-1)))
}

func rightTriangleSizeFromSliderY(y int, rc RECT) int {
	_, top, bottom := rightTriangleSliderBounds(rc)
	y = clamp(y, top, bottom)
	index := int(math.Round(float64(y-top) * float64(len(eraserSliderSizes)-1) / float64(bottom-top)))
	index = clamp(index, 0, len(eraserSliderSizes)-1)
	return eraserSliderSizes[index]
}

func (a *App) hitRightTriangleConstraintOption(x, y int) (LineConstraintMode, bool) {
	angleRect, tanRect := rightTriangleConstraintOptionRects()
	if pointInRect(x, y, angleRect) {
		return LineConstraint15Degrees, true
	}
	if pointInRect(x, y, tanRect) {
		return LineConstraintTanN, true
	}
	return LineConstraint15Degrees, false
}

func (a *App) hitRightTriangleSizeOption(x, y int, rc RECT) (int, bool) {
	_, top, bottom := rightTriangleSliderBounds(rc)
	if x < 8 || x > toolboxOptionRight-2 || y < top-10 || y > bottom+10 {
		return 0, false
	}
	return rightTriangleSizeFromSliderY(y, rc), true
}

func (a *App) setRightTriangleSize(size int) {
	if size <= 20 {
		size = clamp(size, 1, 20)
	} else {
		size = 20 + int(math.Round(float64(size-20)/4.0))*4
		size = clamp(size, 24, 40)
	}
	if a.rightTriangleSize == size {
		return
	}
	a.rightTriangleSize = size
	a.statusText = fmt.Sprintf("직각삼각형 선 굵기: %d px. 빗변은 직선 도구와 같은 방식으로 그려집니다.", size)
	logf("RIGHT_TRIANGLE size=%d", size)
}

func (a *App) paintRightTriangleOptions(hdc uintptr, rc RECT, optTop int) {
	drawToolOptionText(hdc, "Shift 제한", RECT{8, int32(optTop + 2), 90, int32(optTop + 22)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 9)
	angleRect, tanRect := rightTriangleConstraintOptionRects()
	paintChoiceButton(hdc, angleRect, a.lineConstraintMode == LineConstraint15Degrees, lineConstraintNames[LineConstraint15Degrees])
	paintChoiceButton(hdc, tanRect, a.lineConstraintMode == LineConstraintTanN, lineConstraintNames[LineConstraintTanN])
	drawToolOptionText(hdc, fmt.Sprintf("선 굵기 %d px", a.rightTriangleSize), RECT{8, int32(optTop + 80), 90, int32(optTop + 104)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 9)
	trackX, trackTop, trackBottom := rightTriangleSliderBounds(rc)
	fillUIRect(hdc, RECT{int32(trackX), int32(trackTop), int32(trackX + 2), int32(trackBottom + 1)}, rgb(95, 95, 95))
	graphLeft := trackX + 8
	graphRight := toolboxOptionRight - 10
	xAt1 := graphLeft + 1
	xAt20 := graphLeft + 18
	xAt40 := graphRight
	if xAt20 >= xAt40-4 {
		xAt20 = graphLeft + max((graphRight-graphLeft)/2, 8)
	}
	yAt1 := rightTriangleSliderYForSize(1, rc)
	yAt20 := rightTriangleSliderYForSize(20, rc)
	yAt40 := rightTriangleSliderYForSize(40, rc)
	linePen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(65, 65, 65))))
	oldLinePen, _, _ := procSelectObject.Call(hdc, linePen)
	procMoveToEx.Call(hdc, uintptr(xAt1), uintptr(yAt1), 0)
	procLineTo.Call(hdc, uintptr(xAt20), uintptr(yAt20))
	procLineTo.Call(hdc, uintptr(xAt40), uintptr(yAt40))
	procSelectObject.Call(hdc, oldLinePen)
	procDeleteObject.Call(linePen)
	currentX := xAt1
	if a.rightTriangleSize <= 20 {
		t := float64(a.rightTriangleSize-1) / 19.0
		currentX = xAt1 + int(math.Round(t*float64(xAt20-xAt1)))
	} else {
		t := float64(a.rightTriangleSize-20) / 20.0
		currentX = xAt20 + int(math.Round(t*float64(xAt40-xAt20)))
	}
	currentY := rightTriangleSliderYForSize(a.rightTriangleSize, rc)
	currentBrush, _, _ := procCreateSolidBrush.Call(uintptr(uiColor(rgb(0, 120, 215))))
	oldCurrentBrush, _, _ := procSelectObject.Call(hdc, currentBrush)
	currentPen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(0, 85, 170))))
	oldCurrentPen, _, _ := procSelectObject.Call(hdc, currentPen)
	procEllipse.Call(hdc, uintptr(currentX-3), uintptr(currentY-3), uintptr(currentX+4), uintptr(currentY+4))
	procSelectObject.Call(hdc, oldCurrentPen)
	procDeleteObject.Call(currentPen)
	procSelectObject.Call(hdc, oldCurrentBrush)
	procDeleteObject.Call(currentBrush)
	for _, size := range []int{1, 10, 20, 40} {
		ty := rightTriangleSliderYForSize(size, rc)
		fillUIRect(hdc, RECT{int32(trackX - 4), int32(ty), int32(trackX + 5), int32(ty + 1)}, rgb(105, 105, 105))
		tr := RECT{5, int32(ty - 8), int32(trackX - 8), int32(ty + 9)}
		drawUIText(hdc, fmt.Sprintf("%d", size), tr, DT_RIGHT|DT_VCENTER|DT_SINGLELINE, rgb(70, 70, 70))
	}
	thumbY := rightTriangleSliderYForSize(a.rightTriangleSize, rc)
	thumb := RECT{int32(trackX - 6), int32(thumbY - 4), int32(trackX + 7), int32(thumbY + 5)}
	fillUIRect(hdc, thumb, rgb(205, 230, 255))
	frameUIRect(hdc, thumb, rgb(0, 120, 215))
}

func arrowHeadOptionRects() [4]RECT {
	optTop := toolOptionsTop()
	return [4]RECT{
		{12, int32(optTop + 20), 86, int32(optTop + 38)},
		{12, int32(optTop + 40), 86, int32(optTop + 58)},
		{12, int32(optTop + 60), 86, int32(optTop + 78)},
		{12, int32(optTop + 80), 86, int32(optTop + 98)},
	}
}

func arrowConstraintOptionRects() (angleRect, tanRect RECT) {
	optTop := toolOptionsTop()
	angleRect = RECT{12, int32(optTop + 119), 86, int32(optTop + 139)}
	tanRect = RECT{12, int32(optTop + 141), 86, int32(optTop + 161)}
	return
}

func arrowAAOptionRects() (offRect, onRect RECT) {
	optTop := toolOptionsTop()
	offRect = RECT{12, int32(optTop + 182), 47, int32(optTop + 202)}
	onRect = RECT{51, int32(optTop + 182), 86, int32(optTop + 202)}
	return
}

const arrowLineSliderStep = 13 // A87's ~10.4 px interval increased by exactly 25%.

func arrowLineSliderBounds(rc RECT) (trackX, trackTop, trackBottom int) {
	optTop := toolOptionsTop()
	trackX = 36
	trackTop = optTop + 225
	trackBottom = trackTop + arrowLineSliderStep*7
	return
}

func arrowLineSliderYForSize(size int, rc RECT) int {
	_, top, _ := arrowLineSliderBounds(rc)
	size = clamp(size, 1, 8)
	return top + (size-1)*arrowLineSliderStep
}

func arrowLineSizeFromSliderY(y int, rc RECT) int {
	_, top, bottom := arrowLineSliderBounds(rc)
	y = clamp(y, top, bottom)
	return clamp(1+int(math.Round(float64(y-top)/float64(arrowLineSliderStep))), 1, 8)
}

// paintHorizontalArrowGlyph draws a left-facing parity-correct 1:3 preview.
// The shaft starts at the rear edge rather than passing through the head.
func paintHorizontalArrowGlyph(hdc uintptr, rc RECT, headChoice, lineSize int, selectedBackground bool) {
	choice := clampArrowHeadChoice(headChoice)
	cx := int((rc.Left + rc.Right) / 2)
	cy := int((rc.Top + rc.Bottom) / 2)
	available := int(rc.Right-rc.Left) - 10
	if available < 24 {
		available = 24
	}
	tipX := cx - available/2
	tailX := cx + available/2
	// Keep all four previews inside the compact 18 px buttons.  The first
	// preview uses the uploaded 4-step (roughly 12x9) silhouette exactly; the
	// larger three are display-scaled while their actual canvas geometry remains
	// 5/7/9 steps.
	wing := []int{4, 5, 6, 7}[choice]
	headLength := 3 * wing
	baseX := tipX + headLength
	color := uiColor(rgb(20, 20, 20))
	shaft := clamp(lineSize, 1, 8)
	shaftTop := cy - shaft/2 // even widths use the upper-centred pair
	fillRect(hdc, RECT{int32(baseX), int32(shaftTop), int32(tailX + 1), int32(shaftTop + shaft)}, color)

	axis2 := 2 * cy
	tipHalf2 := 1 // one raster row for odd widths
	if shaft%2 == 0 {
		axis2 = 2*cy - 1 // exact symmetry around the upper half-pixel axis
		tipHalf2 = 2     // two raster rows for even widths
	}
	for x := tipX; x <= baseX; x++ {
		back := x - tipX
		allowed2 := tipHalf2 + 2*back/3
		for y := int(rc.Top); y < int(rc.Bottom); y++ {
			if absInt(2*y-axis2) <= allowed2 {
				procSetPixel.Call(hdc, uintptr(x), uintptr(y), uintptr(color))
			}
		}
	}
	_ = selectedBackground
}

func paintArrowHeadChoiceButton(hdc uintptr, rc RECT, selected bool, choice int) {
	if selected {
		fillUIRect(hdc, rc, rgb(205, 230, 255))
		frameUIRect(hdc, rc, rgb(0, 120, 215))
	} else {
		fillUIRect(hdc, rc, rgb(248, 248, 248))
		frameUIRect(hdc, rc, rgb(190, 190, 190))
	}
	paintHorizontalArrowGlyph(hdc, RECT{rc.Left + 6, rc.Top + 1, rc.Right - 6, rc.Bottom - 1}, choice, 1, selected)
}

func (a *App) hitArrowHeadOption(x, y int) (int, bool) {
	for i, rc := range arrowHeadOptionRects() {
		if pointInRect(x, y, rc) {
			return i, true
		}
	}
	return 0, false
}

func (a *App) hitArrowConstraintOption(x, y int) (LineConstraintMode, bool) {
	angleRect, tanRect := arrowConstraintOptionRects()
	if pointInRect(x, y, angleRect) {
		return LineConstraint15Degrees, true
	}
	if pointInRect(x, y, tanRect) {
		return LineConstraintTanN, true
	}
	return LineConstraint15Degrees, false
}

func (a *App) hitArrowAAOption(x, y int) (bool, bool) {
	offRect, onRect := arrowAAOptionRects()
	if pointInRect(x, y, offRect) {
		return false, true
	}
	if pointInRect(x, y, onRect) {
		return true, true
	}
	return false, false
}

func (a *App) hitArrowLineSizeOption(x, y int, rc RECT) (int, bool) {
	_, top, bottom := arrowLineSliderBounds(rc)
	if x < 8 || x > toolboxOptionRight-2 || y < top-10 || y > bottom+10 {
		return 0, false
	}
	return arrowLineSizeFromSliderY(y, rc), true
}

func (a *App) setArrowLineSize(size int) {
	size = clamp(size, 1, 8)
	if a.arrowLineSize == size {
		return
	}
	a.arrowLineSize = size
	a.statusText = fmt.Sprintf("화살표 선 굵기: %d px. 네모 붓과 평평한 끝단을 사용하며, 정사방·45도에서는 AA를 켜도 흐리지 않습니다.", size)
	logf("ARROW line-size=%d parity=%d", size, size%2)
}

func (a *App) paintArrowOptions(hdc uintptr, rc RECT, optTop int) {
	drawToolOptionText(hdc, "머리 크기", RECT{8, int32(optTop + 1), 90, int32(optTop + 18)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 9)
	for i, button := range arrowHeadOptionRects() {
		paintArrowHeadChoiceButton(hdc, button, a.arrowHeadSize == i, i)
	}

	drawToolOptionText(hdc, "Shift 제한", RECT{8, int32(optTop + 100), 90, int32(optTop + 117)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 9)
	angleRect, tanRect := arrowConstraintOptionRects()
	paintChoiceButton(hdc, angleRect, a.lineConstraintMode == LineConstraint15Degrees, lineConstraintNames[LineConstraint15Degrees])
	paintChoiceButton(hdc, tanRect, a.lineConstraintMode == LineConstraintTanN, lineConstraintNames[LineConstraintTanN])

	drawToolOptionText(hdc, "안티에일리어싱", RECT{8, int32(optTop + 163), 90, int32(optTop + 180)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 7)
	offRect, onRect := arrowAAOptionRects()
	paintChoiceButton(hdc, offRect, !a.arrowAntialias, "끔")
	paintChoiceButton(hdc, onRect, a.arrowAntialias, "켬")

	drawToolOptionText(hdc, fmt.Sprintf("선 굵기 %d px", a.arrowLineSize), RECT{8, int32(optTop + 204), 90, int32(optTop + 222)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 9)
	trackX, trackTop, trackBottom := arrowLineSliderBounds(rc)
	fillUIRect(hdc, RECT{int32(trackX), int32(trackTop), int32(trackX + 2), int32(trackBottom + 1)}, rgb(95, 95, 95))

	graphLeft := trackX + 8
	graphRight := toolboxOptionRight - 10
	if graphRight <= graphLeft {
		graphRight = graphLeft + 10
	}
	yAt1 := arrowLineSliderYForSize(1, rc)
	yAt8 := arrowLineSliderYForSize(8, rc)
	linePen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(65, 65, 65))))
	oldLinePen, _, _ := procSelectObject.Call(hdc, linePen)
	procMoveToEx.Call(hdc, uintptr(graphLeft), uintptr(yAt1), 0)
	procLineTo.Call(hdc, uintptr(graphRight), uintptr(yAt8))
	procSelectObject.Call(hdc, oldLinePen)
	procDeleteObject.Call(linePen)

	currentY := arrowLineSliderYForSize(a.arrowLineSize, rc)
	t := float64(a.arrowLineSize-1) / 7.0
	currentX := graphLeft + int(math.Round(t*float64(graphRight-graphLeft)))
	currentBrush, _, _ := procCreateSolidBrush.Call(uintptr(uiColor(rgb(0, 120, 215))))
	oldCurrentBrush, _, _ := procSelectObject.Call(hdc, currentBrush)
	currentPen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(0, 85, 170))))
	oldCurrentPen, _, _ := procSelectObject.Call(hdc, currentPen)
	procEllipse.Call(hdc, uintptr(currentX-3), uintptr(currentY-3), uintptr(currentX+4), uintptr(currentY+4))
	procSelectObject.Call(hdc, oldCurrentPen)
	procDeleteObject.Call(currentPen)
	procSelectObject.Call(hdc, oldCurrentBrush)
	procDeleteObject.Call(currentBrush)

	for size := 1; size <= 8; size++ {
		ty := arrowLineSliderYForSize(size, rc)
		fillUIRect(hdc, RECT{int32(trackX - 4), int32(ty), int32(trackX + 5), int32(ty + 1)}, rgb(105, 105, 105))
		tr := RECT{5, int32(ty - 6), int32(trackX - 8), int32(ty + 7)}
		drawUIText(hdc, fmt.Sprintf("%d", size), tr, DT_RIGHT|DT_VCENTER|DT_SINGLELINE, rgb(70, 70, 70))
	}
	thumbY := arrowLineSliderYForSize(a.arrowLineSize, rc)
	thumb := RECT{int32(trackX - 6), int32(thumbY - 4), int32(trackX + 7), int32(thumbY + 5)}
	fillUIRect(hdc, thumb, rgb(205, 230, 255))
	frameUIRect(hdc, thumb, rgb(0, 120, 215))
}

func lineConstraintOptionRects() (angleRect, tanRect RECT) {
	optTop := toolOptionsTop()
	angleRect = RECT{12, int32(optTop + 25), 86, int32(optTop + 48)}
	tanRect = RECT{12, int32(optTop + 51), 86, int32(optTop + 74)}
	return
}

func lineAAOptionRects() (offRect, onRect RECT) {
	optTop := toolOptionsTop()
	offRect = RECT{12, int32(optTop + 100), 47, int32(optTop + 123)}
	onRect = RECT{51, int32(optTop + 100), 86, int32(optTop + 123)}
	return
}

func lineSliderBounds(rc RECT) (trackX, trackTop, trackBottom int) {
	optTop := toolOptionsTop()
	trackX = 36
	trackTop = optTop + 151
	trackBottom = trackTop + 182
	return
}

func lineSliderYForSize(size int, rc RECT) int {
	_, top, bottom := lineSliderBounds(rc)
	index := eraserSliderIndexForSize(size)
	return top + int(math.Round(float64(index)*float64(bottom-top)/float64(len(eraserSliderSizes)-1)))
}

func lineSizeFromSliderY(y int, rc RECT) int {
	_, top, bottom := lineSliderBounds(rc)
	y = clamp(y, top, bottom)
	index := int(math.Round(float64(y-top) * float64(len(eraserSliderSizes)-1) / float64(bottom-top)))
	index = clamp(index, 0, len(eraserSliderSizes)-1)
	return eraserSliderSizes[index]
}

func (a *App) hitLineConstraintOption(x, y int) (LineConstraintMode, bool) {
	angleRect, tanRect := lineConstraintOptionRects()
	if pointInRect(x, y, angleRect) {
		return LineConstraint15Degrees, true
	}
	if pointInRect(x, y, tanRect) {
		return LineConstraintTanN, true
	}
	return LineConstraint15Degrees, false
}

func (a *App) hitLineAAOption(x, y int) (bool, bool) {
	offRect, onRect := lineAAOptionRects()
	if pointInRect(x, y, offRect) {
		return false, true
	}
	if pointInRect(x, y, onRect) {
		return true, true
	}
	return false, false
}

func (a *App) hitLineSizeOption(x, y int, rc RECT) (int, bool) {
	_, top, bottom := lineSliderBounds(rc)
	if x < 8 || x > toolboxOptionRight-2 || y < top-10 || y > bottom+10 {
		return 0, false
	}
	return lineSizeFromSliderY(y, rc), true
}

func (a *App) setLineSize(size int) {
	if size <= 20 {
		size = clamp(size, 1, 20)
	} else {
		size = 20 + int(math.Round(float64(size-20)/4.0))*4
		size = clamp(size, 24, 40)
	}
	if a.lineSize == size {
		return
	}
	a.lineSize = size
	aa := "끔"
	if a.lineAntialias {
		aa = "켬"
	}
	a.statusText = fmt.Sprintf("직선 굵기: %d px. 네모 붓·평평한 끝단, Shift 제한: %s, 안티에일리어싱: %s(정사방·45°는 선명 유지).", size, lineConstraintNames[a.lineConstraintMode], aa)
	logf("LINE size=%d constraint=%d aa=%v", size, a.lineConstraintMode, a.lineAntialias)
}

func paintChoiceButton(hdc uintptr, rc RECT, selected bool, text string) {
	if selected {
		fillUIRect(hdc, rc, rgb(205, 230, 255))
		frameUIRect(hdc, rc, rgb(0, 120, 215))
	} else {
		fillUIRect(hdc, rc, rgb(248, 248, 248))
		frameUIRect(hdc, rc, rgb(190, 190, 190))
	}
	drawUIText(hdc, text, rc, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0))
}

func (a *App) paintLineOptions(hdc uintptr, rc RECT, optTop int) {
	drawToolOptionText(hdc, "Shift 제한", RECT{10, int32(optTop + 4), 88, int32(optTop + 23)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 9)
	angleRect, tanRect := lineConstraintOptionRects()
	paintChoiceButton(hdc, angleRect, a.lineConstraintMode == LineConstraint15Degrees, lineConstraintNames[LineConstraint15Degrees])
	paintChoiceButton(hdc, tanRect, a.lineConstraintMode == LineConstraintTanN, lineConstraintNames[LineConstraintTanN])

	drawToolOptionText(hdc, "안티에일리어싱", RECT{8, int32(optTop + 79), 90, int32(optTop + 98)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 7)
	offRect, onRect := lineAAOptionRects()
	paintChoiceButton(hdc, offRect, !a.lineAntialias, "끔")
	paintChoiceButton(hdc, onRect, a.lineAntialias, "켬")

	drawToolOptionText(hdc, fmt.Sprintf("선 굵기 %d px", a.lineSize), RECT{8, int32(optTop + 127), 90, int32(optTop + 148)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 9)
	trackX, trackTop, trackBottom := lineSliderBounds(rc)
	fillUIRect(hdc, RECT{int32(trackX), int32(trackTop), int32(trackX + 2), int32(trackBottom + 1)}, rgb(95, 95, 95))
	graphLeft := trackX + 8
	graphRight := toolboxOptionRight - 10
	xAt1 := graphLeft + 1
	xAt20 := graphLeft + 18
	xAt40 := graphRight
	if xAt20 >= xAt40-4 {
		xAt20 = graphLeft + max((graphRight-graphLeft)/2, 8)
	}
	yAt1 := lineSliderYForSize(1, rc)
	yAt20 := lineSliderYForSize(20, rc)
	yAt40 := lineSliderYForSize(40, rc)
	linePen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(65, 65, 65))))
	oldLinePen, _, _ := procSelectObject.Call(hdc, linePen)
	procMoveToEx.Call(hdc, uintptr(xAt1), uintptr(yAt1), 0)
	procLineTo.Call(hdc, uintptr(xAt20), uintptr(yAt20))
	procLineTo.Call(hdc, uintptr(xAt40), uintptr(yAt40))
	procSelectObject.Call(hdc, oldLinePen)
	procDeleteObject.Call(linePen)

	currentX := xAt1
	if a.lineSize <= 20 {
		t := float64(a.lineSize-1) / 19.0
		currentX = xAt1 + int(math.Round(t*float64(xAt20-xAt1)))
	} else {
		t := float64(a.lineSize-20) / 20.0
		currentX = xAt20 + int(math.Round(t*float64(xAt40-xAt20)))
	}
	currentY := lineSliderYForSize(a.lineSize, rc)
	currentBrush, _, _ := procCreateSolidBrush.Call(uintptr(uiColor(rgb(0, 120, 215))))
	oldCurrentBrush, _, _ := procSelectObject.Call(hdc, currentBrush)
	currentPen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(0, 85, 170))))
	oldCurrentPen, _, _ := procSelectObject.Call(hdc, currentPen)
	procEllipse.Call(hdc, uintptr(currentX-3), uintptr(currentY-3), uintptr(currentX+4), uintptr(currentY+4))
	procSelectObject.Call(hdc, oldCurrentPen)
	procDeleteObject.Call(currentPen)
	procSelectObject.Call(hdc, oldCurrentBrush)
	procDeleteObject.Call(currentBrush)
	for _, size := range []int{1, 10, 20, 40} {
		ty := lineSliderYForSize(size, rc)
		fillUIRect(hdc, RECT{int32(trackX - 4), int32(ty), int32(trackX + 5), int32(ty + 1)}, rgb(105, 105, 105))
		drawUIText(hdc, fmt.Sprintf("%d", size), RECT{5, int32(ty - 8), int32(trackX - 8), int32(ty + 9)}, DT_RIGHT|DT_VCENTER|DT_SINGLELINE, rgb(70, 70, 70))
	}
	thumbY := lineSliderYForSize(a.lineSize, rc)
	thumb := RECT{int32(trackX - 6), int32(thumbY - 4), int32(trackX + 7), int32(thumbY + 5)}
	fillUIRect(hdc, thumb, rgb(205, 230, 255))
	frameUIRect(hdc, thumb, rgb(0, 120, 215))
}

func curveBendSliderBounds() (left, right, trackY int) {
	optTop := toolOptionsTop()
	left = 16
	right = toolboxOptionRight - 14
	trackY = optTop + 38
	return
}

func curveBendSliderXForCount(count int) int {
	left, right, _ := curveBendSliderBounds()
	count = clamp(count, 1, 4)
	return left + int(math.Round(float64(count-1)*float64(right-left)/3.0))
}

func curveBendCountFromSliderX(x int) int {
	left, right, _ := curveBendSliderBounds()
	x = clamp(x, left, right)
	count := 1 + int(math.Round(float64(x-left)*3.0/float64(right-left)))
	return clamp(count, 1, 4)
}

func curveAAOptionRects() (offRect, onRect RECT) {
	optTop := toolOptionsTop()
	offRect = RECT{12, int32(optTop + 88), 47, int32(optTop + 111)}
	onRect = RECT{51, int32(optTop + 88), 86, int32(optTop + 111)}
	return
}

func curveSliderBounds(rc RECT) (trackX, trackTop, trackBottom int) {
	optTop := toolOptionsTop()
	trackX = 36
	trackTop = optTop + 139
	trackBottom = trackTop + 182
	return
}

func curveSliderYForSize(size int, rc RECT) int {
	_, top, bottom := curveSliderBounds(rc)
	index := eraserSliderIndexForSize(size)
	return top + int(math.Round(float64(index)*float64(bottom-top)/float64(len(eraserSliderSizes)-1)))
}

func curveSizeFromSliderY(y int, rc RECT) int {
	_, top, bottom := curveSliderBounds(rc)
	y = clamp(y, top, bottom)
	index := int(math.Round(float64(y-top) * float64(len(eraserSliderSizes)-1) / float64(bottom-top)))
	index = clamp(index, 0, len(eraserSliderSizes)-1)
	return eraserSliderSizes[index]
}

func (a *App) hitCurveBendOption(x, y int) (int, bool) {
	left, right, trackY := curveBendSliderBounds()
	if x < left-8 || x > right+8 || y < trackY-13 || y > trackY+13 {
		return 0, false
	}
	return curveBendCountFromSliderX(x), true
}

func (a *App) setCurveBendCount(count int) {
	count = clamp(count, 1, 4)
	if a.curveBendCount == count {
		return
	}
	a.curveBendCount = count
	a.statusText = fmt.Sprintf("곡선 굽힘점을 %d개로 설정했습니다. 2개가 기본 그림판 방식입니다.", count)
	logf("CURVE bend-count=%d", count)
}

func (a *App) hitCurveAAOption(x, y int) (bool, bool) {
	offRect, onRect := curveAAOptionRects()
	if pointInRect(x, y, offRect) {
		return false, true
	}
	if pointInRect(x, y, onRect) {
		return true, true
	}
	return false, false
}

func (a *App) hitCurveSizeOption(x, y int, rc RECT) (int, bool) {
	_, top, bottom := curveSliderBounds(rc)
	if x < 8 || x > toolboxOptionRight-2 || y < top-10 || y > bottom+10 {
		return 0, false
	}
	return curveSizeFromSliderY(y, rc), true
}

func (a *App) setCurveSize(size int) {
	if size <= 20 {
		size = clamp(size, 1, 20)
	} else {
		size = 20 + int(math.Round(float64(size-20)/4.0))*4
		size = clamp(size, 24, 40)
	}
	if a.curveSize == size {
		return
	}
	a.curveSize = size
	aa := "끔"
	if a.curveAntialias {
		aa = "켬"
	}
	a.statusText = fmt.Sprintf("곡선 굵기: %d px. 굽힘점: %d개, 안티에일리어싱: %s.", size, a.curveBendCount, aa)
	logf("CURVE size=%d bends=%d aa=%v", size, a.curveBendCount, a.curveAntialias)
}

func (a *App) paintCurveOptions(hdc uintptr, rc RECT, optTop int) {
	drawToolOptionText(hdc, fmt.Sprintf("굽힘점 수 %d", a.curveBendCount), RECT{8, int32(optTop + 4), 90, int32(optTop + 23)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 9)
	left, right, trackY := curveBendSliderBounds()
	fillUIRect(hdc, RECT{int32(left), int32(trackY), int32(right + 1), int32(trackY + 2)}, rgb(95, 95, 95))
	for count := 1; count <= 4; count++ {
		x := curveBendSliderXForCount(count)
		fillUIRect(hdc, RECT{int32(x), int32(trackY - 4), int32(x + 1), int32(trackY + 6)}, rgb(105, 105, 105))
		drawUIText(hdc, strconv.Itoa(count), RECT{int32(x - 8), int32(trackY + 7), int32(x + 9), int32(trackY + 23)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(70, 70, 70))
	}
	thumbX := curveBendSliderXForCount(a.curveBendCount)
	bendThumb := RECT{int32(thumbX - 5), int32(trackY - 6), int32(thumbX + 6), int32(trackY + 7)}
	fillUIRect(hdc, bendThumb, rgb(205, 230, 255))
	frameUIRect(hdc, bendThumb, rgb(0, 120, 215))

	drawToolOptionText(hdc, "안티에일리어싱", RECT{8, int32(optTop + 66), 90, int32(optTop + 85)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 7)
	offRect, onRect := curveAAOptionRects()
	paintChoiceButton(hdc, offRect, !a.curveAntialias, "끔")
	paintChoiceButton(hdc, onRect, a.curveAntialias, "켬")

	drawToolOptionText(hdc, fmt.Sprintf("선 굵기 %d px", a.curveSize), RECT{8, int32(optTop + 115), 90, int32(optTop + 136)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0), 9)
	trackX, trackTop, trackBottom := curveSliderBounds(rc)
	fillUIRect(hdc, RECT{int32(trackX), int32(trackTop), int32(trackX + 2), int32(trackBottom + 1)}, rgb(95, 95, 95))
	graphLeft := trackX + 8
	graphRight := toolboxOptionRight - 10
	xAt1 := graphLeft + 1
	xAt20 := graphLeft + 18
	xAt40 := graphRight
	if xAt20 >= xAt40-4 {
		xAt20 = graphLeft + max((graphRight-graphLeft)/2, 8)
	}
	yAt1 := curveSliderYForSize(1, rc)
	yAt20 := curveSliderYForSize(20, rc)
	yAt40 := curveSliderYForSize(40, rc)
	linePen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(65, 65, 65))))
	oldLinePen, _, _ := procSelectObject.Call(hdc, linePen)
	procMoveToEx.Call(hdc, uintptr(xAt1), uintptr(yAt1), 0)
	procLineTo.Call(hdc, uintptr(xAt20), uintptr(yAt20))
	procLineTo.Call(hdc, uintptr(xAt40), uintptr(yAt40))
	procSelectObject.Call(hdc, oldLinePen)
	procDeleteObject.Call(linePen)

	currentX := xAt1
	if a.curveSize <= 20 {
		t := float64(a.curveSize-1) / 19.0
		currentX = xAt1 + int(math.Round(t*float64(xAt20-xAt1)))
	} else {
		t := float64(a.curveSize-20) / 20.0
		currentX = xAt20 + int(math.Round(t*float64(xAt40-xAt20)))
	}
	currentY := curveSliderYForSize(a.curveSize, rc)
	currentBrush, _, _ := procCreateSolidBrush.Call(uintptr(uiColor(rgb(0, 120, 215))))
	oldCurrentBrush, _, _ := procSelectObject.Call(hdc, currentBrush)
	currentPen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(0, 85, 170))))
	oldCurrentPen, _, _ := procSelectObject.Call(hdc, currentPen)
	procEllipse.Call(hdc, uintptr(currentX-3), uintptr(currentY-3), uintptr(currentX+4), uintptr(currentY+4))
	procSelectObject.Call(hdc, oldCurrentPen)
	procDeleteObject.Call(currentPen)
	procSelectObject.Call(hdc, oldCurrentBrush)
	procDeleteObject.Call(currentBrush)
	for _, size := range []int{1, 10, 20, 40} {
		ty := curveSliderYForSize(size, rc)
		fillUIRect(hdc, RECT{int32(trackX - 4), int32(ty), int32(trackX + 5), int32(ty + 1)}, rgb(105, 105, 105))
		drawUIText(hdc, fmt.Sprintf("%d", size), RECT{5, int32(ty - 8), int32(trackX - 8), int32(ty + 9)}, DT_RIGHT|DT_VCENTER|DT_SINGLELINE, rgb(70, 70, 70))
	}
	thumbY := curveSliderYForSize(a.curveSize, rc)
	thumb := RECT{int32(trackX - 6), int32(thumbY - 4), int32(trackX + 7), int32(thumbY + 5)}
	fillUIRect(hdc, thumb, rgb(205, 230, 255))
	frameUIRect(hdc, thumb, rgb(0, 120, 215))
}

func (a *App) paintToolbox(hdc uintptr, rc RECT) {
	fillUIRect(hdc, rc, rgb(240, 240, 240))
	frameUIRect(hdc, rc, rgb(160, 160, 160))
	cellW, cellH := toolCellWidth, toolCellHeight
	for i, t := range visibleTools {
		row, col := i/2, i%2
		x := 4 + col*cellW
		y := 4 + row*cellH
		box := RECT{int32(x), int32(y), int32(x + toolButtonWidth), int32(y + toolButtonHeight)}
		selected := t == a.tool || (t == ToolPencil && (a.tool == ToolBrush || a.tool == ToolAirbrush))
		if selected {
			fillUIRect(hdc, box, rgb(205, 230, 255))
			frameUIRect(hdc, box, rgb(0, 120, 215))
		} else {
			fillUIRect(hdc, box, rgb(240, 240, 240))
			frameUIRect(hdc, box, rgb(210, 210, 210))
		}
		a.drawToolIcon(hdc, t, x+toolButtonWidth/2, y+toolButtonHeight/2)

	}
	optTop := toolOptionsTop()
	opt := RECT{toolboxOptionLeft, int32(optTop), toolboxOptionRight, int32(rc.Bottom - 8)}
	fillUIRect(hdc, opt, rgb(255, 255, 255))
	frameUIRect(hdc, opt, rgb(180, 180, 180))
	if a.tool == ToolFreeSelect || a.tool == ToolRectSelect || a.tool == ToolText {
		opaqueRect, transparentRect := selectionOpacityOptionRects()
		a.paintSelectionOpacityOption(hdc, opaqueRect, true, "불투명")
		a.paintSelectionOpacityOption(hdc, transparentRect, false, "투명")
	} else if isColorSelectionTool(a.tool) || a.tool == ToolFill {
		a.paintColorToleranceOptions(hdc, rc, optTop)
	} else if a.tool == ToolMagnifier {
		y := optTop + 4
		for _, z := range a.zoomLevels {
			r := RECT{12, int32(y), 86, int32(y + 18)}
			if z == a.zoomTarget {
				fillUIRect(hdc, r, rgb(205, 230, 255))
				frameUIRect(hdc, r, rgb(0, 120, 215))
			}
			label := fmt.Sprintf("%dx", z)
			if z == a.zoom {
				label = "• " + label
			}
			drawUIText(hdc, label, r, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0))
			y += 19
			if y+18 > int(opt.Bottom) {
				break
			}
		}
	} else if a.tool == ToolCurve {
		a.paintCurveOptions(hdc, rc, optTop)
	} else if a.tool == ToolEraser {
		label := RECT{10, int32(optTop + 6), 88, int32(optTop + 26)}
		drawUIText(hdc, fmt.Sprintf("%d px", a.eraserSize), label, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(0, 0, 0))
		preview := RECT{25, int32(optTop + 29), 73, int32(optTop + 77)}
		fillUIRect(hdc, preview, rgb(245, 245, 245))
		frameUIRect(hdc, preview, rgb(190, 190, 190))
		// Show the eraser at a true 1:1 UI scale.  A 1 px eraser is one device
		// pixel here, while the maximum 40 px eraser occupies exactly 40 pixels.
		previewSize := clamp(a.eraserSize, 1, 40)
		previewCX := 49
		previewCY := optTop + 53
		pl := previewCX - previewSize/2
		pt := previewCY - previewSize/2
		pr := pl + previewSize
		pb := pt + previewSize
		previewColor := gdiPreviewColor(a.bg)
		fillUIRect(hdc, RECT{int32(pl), int32(pt), int32(pr), int32(pb)}, previewColor)
		if previewSize >= 3 {
			frameUIRect(hdc, RECT{int32(pl), int32(pt), int32(pr), int32(pb)}, rgb(0, 0, 0))
		} else {
			// Tiny sizes must remain tiny and visible; do not surround them with a
			// larger frame that would make 1 px look like a several-pixel square.
			for yy := pt; yy < pb; yy++ {
				for xx := pl; xx < pr; xx++ {
					procSetPixel.Call(hdc, uintptr(xx), uintptr(yy), uintptr(uiColor(rgb(0, 0, 0))))
				}
			}
		}

		trackX, trackTop, trackBottom := eraserSliderBounds(rc)
		fillUIRect(hdc, RECT{int32(trackX), int32(trackTop), int32(trackX + 2), int32(trackBottom + 1)}, rgb(95, 95, 95))

		graphLeft := trackX + 8
		graphRight := toolboxOptionRight - 10
		if graphRight <= graphLeft {
			graphRight = graphLeft + 10
		}
		// Match the requested reference look: a simple two-segment line.
		xAt1 := graphLeft + 1
		xAt20 := graphLeft + 18
		xAt40 := graphRight
		if xAt20 >= xAt40-4 {
			xAt20 = graphLeft + max((graphRight-graphLeft)/2, 8)
		}
		// Draw exactly two straight GDI line segments.  Do not approximate the
		// guide by connecting every slider step: 1→20 is one line and 20→40 is
		// the second line, matching the requested reference shape precisely.
		yAt1 := eraserSliderYForSize(1, rc)
		yAt20 := eraserSliderYForSize(20, rc)
		yAt40 := eraserSliderYForSize(40, rc)
		linePen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(65, 65, 65))))
		oldLinePen, _, _ := procSelectObject.Call(hdc, linePen)
		procMoveToEx.Call(hdc, uintptr(xAt1), uintptr(yAt1), 0)
		procLineTo.Call(hdc, uintptr(xAt20), uintptr(yAt20))
		procLineTo.Call(hdc, uintptr(xAt40), uintptr(yAt40))
		procSelectObject.Call(hdc, oldLinePen)
		procDeleteObject.Call(linePen)

		// Restore the blue marker on the guide line.  Its position is computed
		// from the same two exact line segments, so it always sits on the line.
		currentX := xAt1
		if a.eraserSize <= 20 {
			t := float64(a.eraserSize-1) / 19.0
			currentX = xAt1 + int(math.Round(t*float64(xAt20-xAt1)))
		} else {
			t := float64(a.eraserSize-20) / 20.0
			currentX = xAt20 + int(math.Round(t*float64(xAt40-xAt20)))
		}
		currentY := eraserSliderYForSize(a.eraserSize, rc)
		currentBrush, _, _ := procCreateSolidBrush.Call(uintptr(uiColor(rgb(0, 120, 215))))
		oldCurrentBrush, _, _ := procSelectObject.Call(hdc, currentBrush)
		currentPen, _, _ := procCreatePen.Call(PS_SOLID, 1, uintptr(uiColor(rgb(0, 85, 170))))
		oldCurrentPen, _, _ := procSelectObject.Call(hdc, currentPen)
		procEllipse.Call(hdc, uintptr(currentX-3), uintptr(currentY-3), uintptr(currentX+4), uintptr(currentY+4))
		procSelectObject.Call(hdc, oldCurrentPen)
		procDeleteObject.Call(currentPen)
		procSelectObject.Call(hdc, oldCurrentBrush)
		procDeleteObject.Call(currentBrush)

		for _, size := range []int{1, 10, 20, 40} {
			ty := eraserSliderYForSize(size, rc)
			fillUIRect(hdc, RECT{int32(trackX - 4), int32(ty), int32(trackX + 5), int32(ty + 1)}, rgb(105, 105, 105))
			tr := RECT{5, int32(ty - 8), int32(trackX - 8), int32(ty + 9)}
			drawUIText(hdc, fmt.Sprintf("%d", size), tr, DT_RIGHT|DT_VCENTER|DT_SINGLELINE, rgb(70, 70, 70))
		}
		thumbY := eraserSliderYForSize(a.eraserSize, rc)
		thumb := RECT{int32(trackX - 6), int32(thumbY - 4), int32(trackX + 7), int32(thumbY + 5)}
		fillUIRect(hdc, thumb, rgb(205, 230, 255))
		frameUIRect(hdc, thumb, rgb(0, 120, 215))
	} else if a.tool == ToolPencil || a.tool == ToolBrush || a.tool == ToolAirbrush {
		a.paintBrushOptions(hdc, rc, optTop)
	} else if a.tool == ToolLine {
		a.paintLineOptions(hdc, rc, optTop)
	} else if a.tool == ToolRectangle {
		a.paintRectangleOptions(hdc, rc, optTop)
	} else if a.tool == ToolEllipse {
		a.paintEllipseOptions(hdc, rc, optTop)
	} else if a.tool == ToolRightTriangle {
		a.paintRightTriangleOptions(hdc, rc, optTop)
	} else if a.tool == ToolArrow {
		a.paintArrowOptions(hdc, rc, optTop)
	}
}

func selectionToolIconPattern(tool Tool) (*[selectionToolIconHeight]string, bool) {
	switch tool {
	case ToolFreeSelect:
		return &selectionIconFreeSelect, true
	case ToolRectSelect:
		return &selectionIconRectSelect, true
	case ToolColorRegionSelect:
		return &selectionIconColorRegionSelect, true
	case ToolColorAllSelect:
		return &selectionIconColorAllSelect, true
	default:
		return nil, false
	}
}

func drawSelectionToolIconPattern(hdc uintptr, cx, cy int, icon *[selectionToolIconHeight]string) {
	if icon == nil {
		return
	}
	left := cx - selectionToolIconWidth/2
	top := cy - selectionToolIconHeight/2
	for y, row := range icon {
		for x := 0; x < len(row); {
			pixel := row[x]
			if pixel != '#' && pixel != 'Y' {
				x++
				continue
			}
			start := x
			for x < len(row) && row[x] == pixel {
				x++
			}
			color := rgb(0, 0, 0)
			if pixel == 'Y' {
				color = rgb(255, 255, 0)
			}
			fillUIRect(hdc, RECT{
				Left: int32(left + start), Top: int32(top + y),
				Right: int32(left + x), Bottom: int32(top + y + 1),
			}, color)
		}
	}
}

func userToolIconPixels(tool Tool) (*[userToolIconWidth * userToolIconHeight]uint32, bool) {
	switch tool {
	case ToolPencil:
		return &unifiedBrushToolIconPixels, true
	case ToolEraser:
		return &eraserToolIconPixels, true
	case ToolFill:
		return &fillToolIconPixels, true
	case ToolEyedropper:
		return &eyedropperToolIconPixels, true
	case ToolCurve:
		return &curveToolIconPixels, true
	case ToolText:
		return &textToolIconPixels, true
	default:
		return nil, false
	}
}

func invertToolIconColor(c uint32) uint32 {
	if c == userToolIconTransparent || isTransparentColor(c) {
		return c
	}
	r := byte(c & 0xff)
	g := byte((c >> 8) & 0xff)
	b := byte((c >> 16) & 0xff)
	return rgb(255-r, 255-g, 255-b)
}

func darkInvertToolIcon(t Tool) bool {
	// The eyedropper intentionally keeps its user-provided original artwork in
	// both themes.  These four tools are the set requested for literal RGB
	// inversion in dark mode.
	switch t {
	case ToolPencil, ToolEraser, ToolFill, ToolText, ToolCurve:
		return true
	default:
		return false
	}
}

func drawUserToolIconPixels(hdc uintptr, cx, cy int, pixels *[userToolIconWidth * userToolIconHeight]uint32, invert bool) {
	if pixels == nil {
		return
	}
	left := cx - userToolIconWidth/2
	top := cy - userToolIconHeight/2
	for y := 0; y < userToolIconHeight; y++ {
		row := y * userToolIconWidth
		for x := 0; x < userToolIconWidth; {
			sourceColor := pixels[row+x]
			if sourceColor == userToolIconTransparent {
				x++
				continue
			}
			start := x
			for x < userToolIconWidth && pixels[row+x] == sourceColor {
				x++
			}
			color := sourceColor
			if invert {
				color = invertToolIconColor(color)
			}
			fillRect(hdc, RECT{
				Left: int32(left + start), Top: int32(top + y),
				Right: int32(left + x), Bottom: int32(top + y + 1),
			}, color)
		}
	}
}

func (a *App) drawToolIcon(hdc uintptr, t Tool, cx, cy int) {
	if icon, ok := selectionToolIconPattern(t); ok {
		drawSelectionToolIconPattern(hdc, cx, cy, icon)
		return
	}
	if pixels, ok := userToolIconPixels(t); ok {
		drawUserToolIconPixels(hdc, cx, cy, pixels, darkModeEnabled() && darkInvertToolIcon(t))
		return
	}
	pen, _, _ := procCreatePen.Call(PS_SOLID, 2, uintptr(uiColor(rgb(20, 20, 20))))
	old, _, _ := procSelectObject.Call(hdc, pen)
	nullBrush, _, _ := procGetStockObject.Call(NULL_BRUSH)
	oldBrush, _, _ := procSelectObject.Call(hdc, nullBrush)
	defer func() {
		procSelectObject.Call(hdc, oldBrush)
		procSelectObject.Call(hdc, old)
		procDeleteObject.Call(pen)
	}()
	switch t {
	case ToolEyedropper:
		procMoveToEx.Call(hdc, uintptr(cx-11), uintptr(cy+11), 0)
		procLineTo.Call(hdc, uintptr(cx+10), uintptr(cy-10))
		procMoveToEx.Call(hdc, uintptr(cx-5), uintptr(cy+6), 0)
		procLineTo.Call(hdc, uintptr(cx+3), uintptr(cy+13))
	case ToolMagnifier:
		procEllipse.Call(hdc, uintptr(cx-12), uintptr(cy-12), uintptr(cx+8), uintptr(cy+8))
		procMoveToEx.Call(hdc, uintptr(cx+5), uintptr(cy+6), 0)
		procLineTo.Call(hdc, uintptr(cx+13), uintptr(cy+14))
	case ToolPencil:
		// Unified brush icon.
		procMoveToEx.Call(hdc, uintptr(cx-8), uintptr(cy+12), 0)
		procLineTo.Call(hdc, uintptr(cx+3), uintptr(cy-8))
		procMoveToEx.Call(hdc, uintptr(cx+3), uintptr(cy-8), 0)
		procLineTo.Call(hdc, uintptr(cx+10), uintptr(cy-12))
		procMoveToEx.Call(hdc, uintptr(cx-11), uintptr(cy+10), 0)
		procLineTo.Call(hdc, uintptr(cx-5), uintptr(cy+12))
	case ToolBrush:
		procMoveToEx.Call(hdc, uintptr(cx-7), uintptr(cy+12), 0)
		procLineTo.Call(hdc, uintptr(cx+4), uintptr(cy-8))
		procMoveToEx.Call(hdc, uintptr(cx+4), uintptr(cy-8), 0)
		procLineTo.Call(hdc, uintptr(cx+11), uintptr(cy-12))
	case ToolAirbrush:
		dots := [][2]int{{-10, -7}, {-5, -11}, {1, -9}, {8, -5}, {-9, 0}, {-3, -3}, {4, 1}, {10, 3}, {-7, 8}, {0, 10}, {7, 8}, {-1, 4}}
		for _, d := range dots {
			procSetPixel.Call(hdc, uintptr(cx+d[0]), uintptr(cy+d[1]), uintptr(uiColor(rgb(0, 0, 180))))
		}
	case ToolText:
		textColor := rgb(0, 0, 0)
		if darkModeEnabled() && darkInvertToolIcon(ToolText) {
			textColor = invertToolIconColor(textColor)
		}
		drawText(hdc, "A", RECT{int32(cx - 14), int32(cy - 15), int32(cx + 14), int32(cy + 15)}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, textColor)
	case ToolLine:
		procMoveToEx.Call(hdc, uintptr(cx-12), uintptr(cy+11), 0)
		procLineTo.Call(hdc, uintptr(cx+12), uintptr(cy-11))
	case ToolCurve:
		procMoveToEx.Call(hdc, uintptr(cx-12), uintptr(cy+9), 0)
		procLineTo.Call(hdc, uintptr(cx-3), uintptr(cy-7))
		procLineTo.Call(hdc, uintptr(cx+11), uintptr(cy-10))
	case ToolRectangle:
		procRectangle.Call(hdc, uintptr(cx-12), uintptr(cy-10), uintptr(cx+12), uintptr(cy+10))
	case ToolEllipse:
		procEllipse.Call(hdc, uintptr(cx-13), uintptr(cy-10), uintptr(cx+13), uintptr(cy+10))
	case ToolRightTriangle:
		procMoveToEx.Call(hdc, uintptr(cx-12), uintptr(cy+10), 0)
		procLineTo.Call(hdc, uintptr(cx-12), uintptr(cy-10))
		procLineTo.Call(hdc, uintptr(cx+12), uintptr(cy+10))
		procLineTo.Call(hdc, uintptr(cx-12), uintptr(cy+10))
	case ToolArrow:
		paintHorizontalArrowGlyph(hdc, RECT{int32(cx - 14), int32(cy - 11), int32(cx + 15), int32(cy + 12)}, 1, 2, false)
	}
}

func paletteTransparentRect(rc RECT) RECT {
	startX := 70
	left := startX + paletteCellColumns*paletteCellSize + 2
	return RECT{int32(left), rc.Top + 4, int32(left + 46), rc.Top + 50}
}

func paletteColorRect(rc RECT, index int) RECT {
	startX := 70
	col, row := 0, 0
	if index < 28 {
		// Preserve the original 14×2 XP-style palette exactly.
		col = index % 14
		row = index / 14
	} else {
		// Append the 16 editable custom colors as an 8×2 block.
		custom := index - 28
		col = 14 + custom%8
		row = custom / 8
	}
	left := startX + col*paletteCellSize
	top := int(rc.Top) + 4 + row*paletteCellSize
	return RECT{int32(left), int32(top), int32(left + 22), int32(top + 22)}
}

// Indexed palettes are an actual 8×2 index grid, not the first 16 slots
// of the classic 14×2 color box. Display/order match IMG indices 0..15.
func (a *App) indexedPaletteActive() bool { return a.dejavu != nil || a.indexedMode }
func (a *App) paletteColorArea(rc RECT, index int) RECT {
	mode := a.paletteDisplayMode()
	cols := paletteDisplayColumns(mode, a.paletteVisibleCount())
	col, row := paletteDisplayPosition(mode, index, cols)
	left := 70 + col*paletteCellSize
	top := int(rc.Top) + 4 + row*paletteCellSize
	return RECT{int32(left), int32(top), int32(left + 22), int32(top + 22)}
}
func (a *App) paletteTransparentArea(rc RECT) RECT {
	left := 70 + paletteDisplayColumns(a.paletteDisplayMode(), a.paletteVisibleCount())*paletteCellSize + 2
	return RECT{int32(left), rc.Top + 4, int32(left + 46), rc.Top + 50}
}

func paletteBackgroundRect(rc RECT) RECT {
	return RECT{32, int32(rc.Top + 18), 58, int32(rc.Top + 46)}
}

func paletteForegroundRect(rc RECT) RECT {
	return RECT{16, int32(rc.Top + 8), 42, int32(rc.Top + 36)}
}

func paintColorBox(hdc uintptr, rc RECT, c uint32) {
	if isTransparentColor(c) {
		drawCheckerboard(hdc, rc, 6)
	} else {
		fillRect(hdc, rc, c)
	}
}

func (a *App) paletteColors() []uint32 {
	count := a.paletteVisibleCount()
	if a.dejavu != nil || a.indexedMode {
		return a.palette[:min(count, len(a.palette))]
	}
	out := make([]uint32, 0, min(count, 44))
	out = append(out, a.palette[:min(count, len(a.palette))]...)
	if count > len(out) {
		out = append(out, a.customColors[:min(count-len(out), len(a.customColors))]...)
	}
	return out
}

func (a *App) paintPalette(hdc uintptr, rc RECT) {
	fillUIRect(hdc, rc, rgb(240, 240, 240))
	frameUIRect(hdc, rc, rgb(180, 180, 180))
	// Overlapping current-color boxes.
	back := paletteBackgroundRect(rc)
	paintColorBox(hdc, back, a.bg)
	frameRect(hdc, back, uiColor(rgb(100, 100, 100)))
	front := paletteForegroundRect(rc)
	paintColorBox(hdc, front, a.fg)
	frameRect(hdc, front, uiColor(rgb(40, 40, 40)))

	colors := a.paletteColors()
	for i, c := range colors {
		r := a.paletteColorArea(rc, i)
		fillRect(hdc, r, c)
		frameRect(hdc, r, uiColor(rgb(100, 100, 100)))
		if a.indexedPaletteActive() {
			// Distinct numeric labels make index order explicit in both themes.
			labelColor := rgb(255, 255, 255)
			if (int(byte(c))*299+int(byte(c>>8))*587+int(byte(c>>16))*114)/1000 > 130 {
				labelColor = rgb(0, 0, 0)
			}
			label := fmt.Sprintf("%d", i)
			if a.indexedMode && len(a.palette) == 256 {
				label = fmt.Sprintf("%02X", i)
			}
			drawToolOptionText(hdc, label, r, DT_CENTER|DT_VCENTER|DT_SINGLELINE, labelColor, 11)
		}
	}
	tr := a.paletteTransparentArea(rc)
	drawCheckerboard(hdc, tr, 8)
	frameRect(hdc, tr, uiColor(rgb(100, 100, 100)))
}

func (a *App) clipboardVisibleOffset() int {
	return a.clipboardScroll * clipboardColumns
}

func (a *App) clipboardMaxScroll() int {
	totalRows := (len(a.clipboardHistory) + clipboardColumns - 1) / clipboardColumns
	return max(totalRows-clipboardVisibleRows, 0)
}

func (a *App) ensureClipboardSelectionVisible() {
	maxScroll := a.clipboardMaxScroll()
	if a.clipboardSelected < 0 {
		a.clipboardScroll = clamp(a.clipboardScroll, 0, maxScroll)
		return
	}
	row := a.clipboardSelected / clipboardColumns
	if row < a.clipboardScroll {
		a.clipboardScroll = row
	}
	if row >= a.clipboardScroll+clipboardVisibleRows {
		a.clipboardScroll = row - clipboardVisibleRows + 1
	}
	a.clipboardScroll = clamp(a.clipboardScroll, 0, maxScroll)
}

func (a *App) clipboardSlotRects(panel RECT) [clipboardVisibleSlots]RECT {
	var out [clipboardVisibleSlots]RECT
	gap := 6
	innerLeft := int(panel.Left) + 7
	innerTop := int(panel.Top) + panelHeaderHeight + 4
	// Reserve the right edge for the draggable scrollbar.
	innerW := int(panel.Right-panel.Left) - 14 - 19
	slotW := (innerW - gap) / clipboardColumns
	slotH := 66
	for i := 0; i < clipboardVisibleSlots; i++ {
		row := i / clipboardColumns
		col := i % clipboardColumns
		left := innerLeft + col*(slotW+gap)
		top := innerTop + row*(slotH+gap)
		out[i] = RECT{int32(left), int32(top), int32(left + slotW), int32(top + slotH)}
	}
	return out
}

func clipboardPinRect(slot RECT) RECT {
	return RECT{slot.Right - 23, slot.Top + 3, slot.Right - 3, slot.Top + 23}
}

func (a *App) paintClipboardThumbnail(hdc uintptr, img ClipboardImage, rc RECT) {
	drawCheckerboard(hdc, rc, 8)
	frameRect(hdc, rc, uiColor(rgb(145, 145, 145)))
	if !validClipboardImage(img) {
		return
	}
	availW := max(int(rc.Right-rc.Left)-4, 1)
	availH := max(int(rc.Bottom-rc.Top)-4, 1)
	dw, dh := availW, availH
	if img.W*availH > img.H*availW {
		dh = max(img.H*availW/img.W, 1)
	} else {
		dw = max(img.W*availH/img.H, 1)
	}
	dx := int(rc.Left) + (availW-dw)/2 + 2
	dy := int(rc.Top) + (availH-dh)/2 + 2
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(img.W), BiHeight: -int32(img.H), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	procSetStretchBltMode.Call(hdc, COLORONCOLOR)
	procStretchDIBits.Call(hdc, uintptr(dx), uintptr(dy), uintptr(dw), uintptr(dh), 0, 0, uintptr(img.W), uintptr(img.H), uintptr(unsafe.Pointer(&img.Pix[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, SRCCOPY)
}

func drawClipboardPin(hdc uintptr, rc RECT, pinned bool) {
	col := rgb(100, 100, 100)
	if pinned {
		col = rgb(215, 60, 40)
	}
	cx := (int(rc.Left) + int(rc.Right)) / 2
	cy := (int(rc.Top) + int(rc.Bottom)) / 2
	pen, _, _ := procCreatePen.Call(PS_SOLID, 2, uintptr(col))
	old, _, _ := procSelectObject.Call(hdc, pen)
	procEllipse.Call(hdc, uintptr(cx-5), uintptr(cy-8), uintptr(cx+6), uintptr(cy-1))
	procMoveToEx.Call(hdc, uintptr(cx), uintptr(cy-1), 0)
	procLineTo.Call(hdc, uintptr(cx), uintptr(cy+7))
	procMoveToEx.Call(hdc, uintptr(cx-4), uintptr(cy+2), 0)
	procLineTo.Call(hdc, uintptr(cx+4), uintptr(cy+2))
	procSelectObject.Call(hdc, old)
	procDeleteObject.Call(pen)
}

func (a *App) paintPanelFrame(hdc uintptr, panel RECT, title string) {
	if panel.Right <= panel.Left || panel.Bottom <= panel.Top {
		return
	}
	fillUIRect(hdc, panel, rgb(244, 244, 244))
	frameUIRect(hdc, panel, rgb(135, 135, 135))
	header := panelHeaderRect(panel)
	fillUIRect(hdc, header, rgb(225, 232, 240))
	frameUIRect(hdc, header, rgb(170, 178, 188))
	var oldTitleFont uintptr
	if titleFont := a.ensureDockPanelTitleFont(); titleFont != 0 {
		oldTitleFont, _, _ = procSelectObject.Call(hdc, titleFont)
	}
	drawUIText(hdc, title, RECT{header.Left + 6, header.Top, header.Right - 20, header.Bottom}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, rgb(20, 20, 20))
	closeRc := panelCloseRect(panel)
	fillUIRect(hdc, closeRc, rgb(240, 240, 240))
	frameUIRect(hdc, closeRc, rgb(170, 170, 170))
	drawUIText(hdc, "×", closeRc, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(70, 70, 70))
	if oldTitleFont != 0 {
		procSelectObject.Call(hdc, oldTitleFont)
	}
}

func paintTranslatedPanelContent(hdc uintptr, dest RECT, painter func(uintptr, RECT)) {
	w := int(dest.Right - dest.Left)
	h := int(dest.Bottom - dest.Top)
	if w <= 0 || h <= 0 {
		return
	}
	memDC, _, _ := procCreateCompatibleDC.Call(hdc)
	if memDC == 0 {
		return
	}
	defer procDeleteDC.Call(memDC)
	bmp, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(w), uintptr(h))
	if bmp == 0 {
		return
	}
	old, _, _ := procSelectObject.Call(memDC, bmp)
	defer func() {
		procSelectObject.Call(memDC, old)
		procDeleteObject.Call(bmp)
	}()
	local := RECT{0, 0, int32(w), int32(h)}
	fillUIRect(memDC, local, rgb(240, 240, 240))
	painter(memDC, local)
	procBitBlt.Call(hdc, uintptr(dest.Left), uintptr(dest.Top), uintptr(w), uintptr(h), memDC, 0, 0, SRCCOPY)
}

func (a *App) paintToolboxPanel(hdc uintptr, panel RECT) {
	if panel.Right <= panel.Left || panel.Bottom <= panel.Top {
		return
	}
	a.paintPanelFrame(hdc, panel, "도구")
	content := panelContentRect(panel)
	paintTranslatedPanelContent(hdc, content, func(dc uintptr, rc RECT) { a.paintToolbox(dc, rc) })
}

func (a *App) paintPalettePanel(hdc uintptr, panel RECT) {
	if panel.Right <= panel.Left || panel.Bottom <= panel.Top {
		return
	}
	a.paintPanelFrame(hdc, panel, "색상표")
	content := panelContentRect(panel)
	paintTranslatedPanelContent(hdc, content, func(dc uintptr, rc RECT) { a.paintPalette(dc, rc) })
}

func (a *App) clipboardScrollbarRects(panel RECT) (bar, up, down, track, thumb RECT) {
	bar = RECT{panel.Right - 17, panel.Top + panelHeaderHeight + 2, panel.Right - 3, panel.Bottom - 3}
	up = RECT{bar.Left, bar.Top, bar.Right, bar.Top + 14}
	down = RECT{bar.Left, bar.Bottom - 14, bar.Right, bar.Bottom}
	track = RECT{bar.Left, up.Bottom, bar.Right, down.Top}
	trackH := max(int(track.Bottom-track.Top), 1)
	maxScroll := a.clipboardMaxScroll()
	totalRows := max((len(a.clipboardHistory)+clipboardColumns-1)/clipboardColumns, clipboardVisibleRows)
	if maxScroll <= 0 {
		thumb = track
		return
	}
	thumbH := max(20, trackH*clipboardVisibleRows/totalRows)
	thumbH = min(thumbH, trackH)
	travel := max(trackH-thumbH, 0)
	top := int(track.Top) + int(math.Round(float64(travel)*float64(a.clipboardScroll)/float64(maxScroll)))
	thumb = RECT{track.Left + 2, int32(top), track.Right - 2, int32(top + thumbH)}
	return
}

func (a *App) paintClipboardScrollbar(hdc uintptr, panel RECT) {
	bar, up, down, track, thumb := a.clipboardScrollbarRects(panel)
	fillUIRect(hdc, bar, rgb(238, 238, 238))
	frameUIRect(hdc, bar, rgb(170, 170, 170))
	fillUIRect(hdc, up, rgb(246, 246, 246))
	frameUIRect(hdc, up, rgb(190, 190, 190))
	fillUIRect(hdc, down, rgb(246, 246, 246))
	frameUIRect(hdc, down, rgb(190, 190, 190))
	drawUIText(hdc, "▲", up, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(95, 95, 95))
	drawUIText(hdc, "▼", down, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(95, 95, 95))
	fillUIRect(hdc, track, rgb(248, 248, 248))
	if thumb.Bottom > thumb.Top {
		fillUIRect(hdc, thumb, rgb(175, 175, 175))
		frameUIRect(hdc, thumb, rgb(130, 130, 130))
	}
}

func (a *App) scrollClipboard(deltaRows int) {
	a.clipboardScroll = clamp(a.clipboardScroll+deltaRows, 0, a.clipboardMaxScroll())
	invalidate(a.hwnd)
}

func (a *App) updateClipboardScrollDrag(my int, client RECT) {
	panel := a.clipboardPanelRect(client)
	_, _, _, track, thumb := a.clipboardScrollbarRects(panel)
	maxScroll := a.clipboardMaxScroll()
	if maxScroll <= 0 {
		a.clipboardScroll = 0
		return
	}
	thumbH := int(thumb.Bottom - thumb.Top)
	travel := max(int(track.Bottom-track.Top)-thumbH, 1)
	top := clamp(my-a.clipboardScrollDragOffset, int(track.Top), int(track.Bottom)-thumbH)
	ratio := float64(top-int(track.Top)) / float64(travel)
	a.clipboardScroll = clamp(int(math.Round(ratio*float64(maxScroll))), 0, maxScroll)
	invalidate(a.hwnd)
}

func (a *App) paintClipboardPanel(hdc uintptr, panel RECT) {
	if panel.Right <= panel.Left || panel.Bottom <= panel.Top {
		return
	}
	a.paintPanelFrame(hdc, panel, "클립보드")
	a.clipboardScroll = clamp(a.clipboardScroll, 0, a.clipboardMaxScroll())
	offset := a.clipboardVisibleOffset()
	slots := a.clipboardSlotRects(panel)
	for visible, slot := range slots {
		index := offset + visible
		if index == a.clipboardSelected && index < len(a.clipboardHistory) {
			fillUIRect(hdc, slot, rgb(210, 232, 255))
			frameUIRect(hdc, slot, rgb(0, 120, 215))
		} else {
			fillUIRect(hdc, slot, rgb(255, 255, 255))
			frameUIRect(hdc, slot, rgb(185, 185, 185))
		}
		thumb := RECT{slot.Left + 4, slot.Top + 4, slot.Right - 4, slot.Bottom - 4}
		if index >= len(a.clipboardHistory) {
			a.paintClipboardThumbnail(hdc, ClipboardImage{}, thumb)
			drawUIText(hdc, fmt.Sprintf("%d", index+1), RECT{slot.Left + 4, slot.Top + 3, slot.Left + 22, slot.Top + 21}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(145, 145, 145))
			continue
		}
		e := a.clipboardHistory[index]
		a.paintClipboardThumbnail(hdc, e.Image, thumb)
		drawUIText(hdc, fmt.Sprintf("%d", index+1), RECT{slot.Left + 4, slot.Top + 3, slot.Left + 22, slot.Top + 21}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(40, 40, 40))
		pinRc := clipboardPinRect(slot)
		fillUIRect(hdc, pinRc, rgb(248, 248, 248))
		frameUIRect(hdc, pinRc, rgb(180, 180, 180))
		drawClipboardPin(hdc, pinRc, e.Pinned)
	}
	a.paintClipboardScrollbar(hdc, panel)
}

func (a *App) hitClipboardPanel(mx, my int, client RECT) (string, int) {
	panel := a.clipboardPanelRect(client)
	if panel.Right <= panel.Left || !pointInRect(mx, my, panel) {
		return "", -1
	}
	if pointInRect(mx, my, panelCloseRect(panel)) {
		return "close", -1
	}
	if pointInRect(mx, my, panelHeaderRect(panel)) {
		return "drag", -1
	}
	_, up, down, track, thumb := a.clipboardScrollbarRects(panel)
	if pointInRect(mx, my, up) {
		return "scrollup", -1
	}
	if pointInRect(mx, my, down) {
		return "scrolldown", -1
	}
	if pointInRect(mx, my, thumb) {
		if a.clipboardMaxScroll() <= 0 {
			return "panel", -1
		}
		return "scrollthumb", my - int(thumb.Top)
	}
	if pointInRect(mx, my, track) {
		if a.clipboardMaxScroll() <= 0 {
			return "panel", -1
		}
		if my < int(thumb.Top) {
			return "pageup", -1
		}
		return "pagedown", -1
	}
	offset := a.clipboardVisibleOffset()
	for visible, slot := range a.clipboardSlotRects(panel) {
		if pointInRect(mx, my, slot) {
			index := offset + visible
			if pointInRect(mx, my, clipboardPinRect(slot)) {
				return "pin", index
			}
			return "paste", index
		}
	}
	return "panel", -1
}

func (a *App) thumbnailImageRect(panel RECT) (RECT, bool) {
	if panel.Right <= panel.Left || panel.Bottom <= panel.Top || a.canvas == nil || a.canvas.W < 1 || a.canvas.H < 1 {
		return RECT{}, false
	}
	content := panelContentRect(panel)
	view := RECT{content.Left + 6, content.Top + 6, content.Right - 6, content.Bottom - 6}
	if view.Right <= view.Left || view.Bottom <= view.Top {
		return RECT{}, false
	}
	availW := max(int(view.Right-view.Left)-4, 1)
	availH := max(int(view.Bottom-view.Top)-4, 1)
	scaleX := float64(availW) / float64(a.canvas.W)
	scaleY := float64(availH) / float64(a.canvas.H)
	scale := math.Min(scaleX, scaleY)
	if scale <= 0 {
		return RECT{}, false
	}
	dw := max(1, int(math.Floor(float64(a.canvas.W)*scale)))
	dh := max(1, int(math.Floor(float64(a.canvas.H)*scale)))
	dx := int(view.Left) + (int(view.Right-view.Left)-dw)/2
	dy := int(view.Top) + (int(view.Bottom-view.Top)-dh)/2
	return RECT{int32(dx), int32(dy), int32(dx + dw), int32(dy + dh)}, true
}

func (a *App) paintThumbnailPanel(hdc uintptr, panel RECT) {
	if panel.Right <= panel.Left || panel.Bottom <= panel.Top || a.canvas == nil || a.canvas.W < 1 || a.canvas.H < 1 {
		return
	}
	a.paintPanelFrame(hdc, panel, "미리 보기")
	content := panelContentRect(panel)
	view := RECT{content.Left + 6, content.Top + 6, content.Right - 6, content.Bottom - 6}
	if view.Right <= view.Left || view.Bottom <= view.Top {
		return
	}
	drawCheckerboard(hdc, view, 8)
	frameUIRect(hdc, view, rgb(160, 160, 160))

	imageRect, ok := a.thumbnailImageRect(panel)
	if !ok {
		return
	}
	dx := int(imageRect.Left)
	dy := int(imageRect.Top)
	dw := int(imageRect.Right - imageRect.Left)
	dh := int(imageRect.Bottom - imageRect.Top)

	pix := a.canvas.displayPixels(1)
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(a.canvas.W), BiHeight: -int32(a.canvas.H), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	procSetStretchBltMode.Call(hdc, COLORONCOLOR)
	if len(pix) > 0 {
		procStretchDIBits.Call(hdc, uintptr(dx), uintptr(dy), uintptr(dw), uintptr(dh), 0, 0, uintptr(a.canvas.W), uintptr(a.canvas.H), uintptr(unsafe.Pointer(&pix[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, SRCCOPY)
	}
	frameUIRect(hdc, imageRect, rgb(100, 100, 100))

	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	ws, _, _, _ := a.layout(client)
	vx, vy := a.viewportTopLeftImage()
	pageW := max((int(ws.Right-ws.Left)-16)/max(a.zoom, 1), 1)
	pageH := max((int(ws.Bottom-ws.Top)-16)/max(a.zoom, 1), 1)
	vw := min(pageW, max(a.canvas.W-vx, 1))
	vh := min(pageH, max(a.canvas.H-vy, 1))
	vrLeft := dx + int(math.Round(float64(vx)*float64(dw)/float64(a.canvas.W)))
	vrTop := dy + int(math.Round(float64(vy)*float64(dh)/float64(a.canvas.H)))
	vrRight := dx + int(math.Round(float64(vx+vw)*float64(dw)/float64(a.canvas.W)))
	vrBottom := dy + int(math.Round(float64(vy+vh)*float64(dh)/float64(a.canvas.H)))
	vrLeft = clamp(vrLeft, dx, max(dx+dw-1, dx))
	vrTop = clamp(vrTop, dy, max(dy+dh-1, dy))
	vrRight = clamp(max(vrRight, vrLeft+1), vrLeft+1, dx+dw)
	vrBottom = clamp(max(vrBottom, vrTop+1), vrTop+1, dy+dh)
	frameUIRect(hdc, RECT{int32(vrLeft), int32(vrTop), int32(vrRight), int32(vrBottom)}, rgb(0, 120, 215))
}

func (a *App) hitThumbnailPanel(mx, my int, client RECT) string {
	panel := a.thumbnailPanelRect(client)
	if panel.Right <= panel.Left || !pointInRect(mx, my, panel) {
		return ""
	}
	if pointInRect(mx, my, panelCloseRect(panel)) {
		return "close"
	}
	if pointInRect(mx, my, panelHeaderRect(panel)) {
		return "drag"
	}
	if imageRect, ok := a.thumbnailImageRect(panel); ok && pointInRect(mx, my, imageRect) {
		return "jump"
	}
	return "panel"
}

func (a *App) navigateThumbnailPoint(mx, my int, client RECT, clampToImage bool, reason string) bool {
	panel := a.thumbnailPanelRect(client)
	imageRect, ok := a.thumbnailImageRect(panel)
	if !ok || a.canvas == nil || a.canvas.W < 1 || a.canvas.H < 1 {
		return false
	}
	if clampToImage {
		mx = clamp(mx, int(imageRect.Left), max(int(imageRect.Right)-1, int(imageRect.Left)))
		my = clamp(my, int(imageRect.Top), max(int(imageRect.Bottom)-1, int(imageRect.Top)))
	} else if !pointInRect(mx, my, imageRect) {
		return false
	}
	dw := max(int(imageRect.Right-imageRect.Left), 1)
	dh := max(int(imageRect.Bottom-imageRect.Top), 1)
	// The far right and bottom edges map to the final image pixel instead of an
	// out-of-range coordinate. Centering the selected pixel matches the usual
	// navigator/overview-panel behaviour and makes distant areas reachable with
	// one click or a continuous drag.
	ix := clamp(int(math.Floor(float64(mx-int(imageRect.Left))*float64(a.canvas.W)/float64(dw))), 0, a.canvas.W-1)
	iy := clamp(int(math.Floor(float64(my-int(imageRect.Top))*float64(a.canvas.H)/float64(dh))), 0, a.canvas.H-1)
	ws, _, _, _ := a.layout(client)
	pageW := max(int(ws.Right-ws.Left)-16, 1)
	pageH := max(int(ws.Bottom-ws.Top)-16, 1)
	oldX, oldY := a.scrollX, a.scrollY
	a.scrollX = ix*a.zoom - pageW/2
	a.scrollY = iy*a.zoom - pageH/2
	a.updateScrollbars()
	a.updateTextUILayout(client)
	a.statusText = fmt.Sprintf("미리 보기에서 (%d, %d) 위치로 이동했습니다.", ix, iy)
	if oldX != a.scrollX || oldY != a.scrollY || reason != "drag" {
		logf("THUMBNAIL %s client=%d,%d image=%d,%d scroll=%d,%d zoom=%d", reason, mx, my, ix, iy, a.scrollX, a.scrollY, a.zoom)
	}
	invalidate(a.hwnd)
	return true
}

func (a *App) jumpToThumbnailPoint(mx, my int, client RECT) {
	a.navigateThumbnailPoint(mx, my, client, false, "jump")
}

func (a *App) historyScrollbarRects(panel RECT) (bar, up, down, track, thumb RECT) {
	bar = RECT{panel.Right - 17, panel.Top + panelHeaderHeight + 2, panel.Right - 3, panel.Bottom - 3}
	up = RECT{bar.Left, bar.Top, bar.Right, bar.Top + 14}
	down = RECT{bar.Left, bar.Bottom - 14, bar.Right, bar.Bottom}
	track = RECT{bar.Left, up.Bottom, bar.Right, down.Top}
	rows := a.historyVisibleRows()
	total := len(a.historyEntries)
	trackH := max(int(track.Bottom-track.Top), 1)
	if total <= rows || total <= 0 {
		thumb = track
		return
	}
	thumbH := max(20, trackH*rows/total)
	thumbH = min(thumbH, trackH)
	maxScroll := max(total-rows, 1)
	travel := max(trackH-thumbH, 0)
	top := int(track.Top) + int(math.Round(float64(travel)*float64(a.historyScroll)/float64(maxScroll)))
	thumb = RECT{track.Left + 2, int32(top), track.Right - 2, int32(top + thumbH)}
	return
}

func (a *App) historyRowRects(panel RECT) []RECT {
	rows := a.historyVisibleRows()
	out := make([]RECT, 0, rows)
	top := int(panel.Top) + panelHeaderHeight + 2
	right := panel.Right - 20
	for i := 0; i < rows; i++ {
		y := top + i*24
		if y+24 > int(panel.Bottom)-2 {
			break
		}
		out = append(out, RECT{panel.Left + 4, int32(y), right, int32(y + 23)})
	}
	return out
}

func (a *App) paintHistoryScrollbar(hdc uintptr, panel RECT) {
	bar, up, down, track, thumb := a.historyScrollbarRects(panel)
	fillUIRect(hdc, bar, rgb(238, 238, 238))
	frameUIRect(hdc, bar, rgb(170, 170, 170))
	fillUIRect(hdc, up, rgb(246, 246, 246))
	frameUIRect(hdc, up, rgb(190, 190, 190))
	fillUIRect(hdc, down, rgb(246, 246, 246))
	frameUIRect(hdc, down, rgb(190, 190, 190))
	drawUIText(hdc, "▲", up, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(95, 95, 95))
	drawUIText(hdc, "▼", down, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(95, 95, 95))
	fillUIRect(hdc, track, rgb(248, 248, 248))
	if thumb.Bottom > thumb.Top {
		fillUIRect(hdc, thumb, rgb(175, 175, 175))
		frameUIRect(hdc, thumb, rgb(130, 130, 130))
	}
}

func (a *App) paintHistoryPanel(hdc uintptr, panel RECT) {
	if panel.Right <= panel.Left || panel.Bottom <= panel.Top {
		return
	}
	a.paintPanelFrame(hdc, panel, "히스토리")
	rows := a.historyRowRects(panel)
	for row, rr := range rows {
		index := a.historyScroll + row
		if index >= len(a.historyEntries) {
			break
		}
		e := a.historyEntries[index]
		if index == a.historyIndex {
			fillUIRect(hdc, rr, rgb(205, 230, 255))
			frameUIRect(hdc, rr, rgb(0, 120, 215))
		} else {
			fillUIRect(hdc, rr, rgb(252, 252, 252))
			frameUIRect(hdc, rr, rgb(225, 225, 225))
		}
		marker := ""
		if index == a.historyIndex {
			marker = "▶"
		}
		fg := rgb(35, 35, 35)
		if index > a.historyIndex {
			fg = rgb(130, 130, 130)
		}
		numberRc := RECT{rr.Left + 4, rr.Top, rr.Left + 43, rr.Bottom}
		drawUIText(hdc, fmt.Sprintf("%s%02d", marker, index), numberRc, DT_LEFT|DT_VCENTER|DT_SINGLELINE, fg)
		nameRc := RECT{rr.Left + 45, rr.Top, rr.Right - 55, rr.Bottom}
		drawUIText(hdc, e.Name, nameRc, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS, fg)
		timeRc := RECT{rr.Right - 54, rr.Top, rr.Right - 4, rr.Bottom}
		drawUIText(hdc, e.At.Format("15:04"), timeRc, DT_RIGHT|DT_VCENTER|DT_SINGLELINE, rgb(110, 110, 110))
	}
	if len(a.historyEntries) == 0 {
		drawUIText(hdc, "기록 없음", RECT{panel.Left + 8, panel.Top + panelHeaderHeight + 4, panel.Right - 20, panel.Bottom - 8}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(120, 120, 120))
	}
	a.paintHistoryScrollbar(hdc, panel)
}

func (a *App) hitHistoryPanel(mx, my int, client RECT) (string, int) {
	panel := a.historyPanelRect(client)
	if panel.Right <= panel.Left || !pointInRect(mx, my, panel) {
		return "", -1
	}
	if pointInRect(mx, my, panelCloseRect(panel)) {
		return "close", -1
	}
	if pointInRect(mx, my, panelHeaderRect(panel)) {
		return "drag", -1
	}
	_, up, down, track, thumb := a.historyScrollbarRects(panel)
	if len(a.historyEntries) <= a.historyVisibleRows() && pointInRect(mx, my, track) {
		return "panel", -1
	}
	if pointInRect(mx, my, up) {
		return "scrollup", -1
	}
	if pointInRect(mx, my, down) {
		return "scrolldown", -1
	}
	if pointInRect(mx, my, thumb) {
		return "scrollthumb", my - int(thumb.Top)
	}
	if pointInRect(mx, my, track) {
		if my < int(thumb.Top) {
			return "pageup", -1
		}
		return "pagedown", -1
	}
	for row, rr := range a.historyRowRects(panel) {
		if pointInRect(mx, my, rr) {
			index := a.historyScroll + row
			if index >= 0 && index < len(a.historyEntries) {
				return "goto", index
			}
			return "panel", -1
		}
	}
	return "panel", -1
}

func (a *App) scrollHistory(delta int) {
	rows := a.historyVisibleRows()
	maxScroll := max(len(a.historyEntries)-rows, 0)
	a.historyScroll = clamp(a.historyScroll+delta, 0, maxScroll)
	a.requestHistoryRepaint(true)
}

func (a *App) updateHistoryScrollDrag(my int, client RECT) {
	panel := a.historyPanelRect(client)
	_, _, _, track, thumb := a.historyScrollbarRects(panel)
	rows := a.historyVisibleRows()
	maxScroll := max(len(a.historyEntries)-rows, 0)
	if maxScroll <= 0 {
		a.historyScroll = 0
		return
	}
	thumbH := int(thumb.Bottom - thumb.Top)
	travel := max(int(track.Bottom-track.Top)-thumbH, 1)
	top := clamp(my-a.historyScrollDragOffset, int(track.Top), int(track.Bottom)-thumbH)
	ratio := float64(top-int(track.Top)) / float64(travel)
	a.historyScroll = clamp(int(math.Round(ratio*float64(maxScroll))), 0, maxScroll)
	a.requestHistoryRepaint(true)
}

func (a *App) paintTextPanel(hdc uintptr, panel RECT) {
	if panel.Right <= panel.Left || panel.Bottom <= panel.Top {
		return
	}
	a.paintPanelFrame(hdc, panel, "텍스트")
	labelColor := rgb(75, 75, 75)
	drawUIText(hdc, "글꼴", RECT{panel.Left + 8, panel.Top + 20, panel.Left + 52, panel.Top + 39}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, labelColor)
	drawUIText(hdc, "크기", RECT{panel.Left + 8, panel.Top + 57, panel.Left + 52, panel.Top + 76}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, labelColor)
	drawUIText(hdc, "미리", RECT{panel.Left + 119, panel.Top + 52, panel.Left + 162, panel.Top + 68}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, labelColor)
	drawUIText(hdc, "보기", RECT{panel.Left + 119, panel.Top + 68, panel.Left + 162, panel.Top + 84}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, labelColor)
	drawUIText(hdc, "스타일", RECT{panel.Left + 8, panel.Top + 95, panel.Left + 52, panel.Top + 118}, DT_LEFT|DT_VCENTER|DT_SINGLELINE, labelColor)
}

func (a *App) hitTextPanel(mx, my int, client RECT) string {
	panel := a.textPanelRect(client)
	if panel.Right <= panel.Left || !pointInRect(mx, my, panel) {
		return ""
	}
	if pointInRect(mx, my, panelCloseRect(panel)) {
		return "close"
	}
	if pointInRect(mx, my, panelHeaderRect(panel)) {
		return "drag"
	}
	return "panel"
}

func (a *App) hitToolboxPanel(mx, my int, client RECT) string {
	panel := a.toolboxPanelRect(client)
	if panel.Right <= panel.Left || !pointInRect(mx, my, panel) {
		return ""
	}
	if pointInRect(mx, my, panelCloseRect(panel)) {
		return "close"
	}
	if pointInRect(mx, my, panelHeaderRect(panel)) {
		return "drag"
	}
	return "content"
}

func (a *App) hitPalettePanel(mx, my int, client RECT) string {
	panel := a.palettePanelRect(client)
	if panel.Right <= panel.Left || !pointInRect(mx, my, panel) {
		return ""
	}
	if pointInRect(mx, my, panelCloseRect(panel)) {
		return "close"
	}
	if pointInRect(mx, my, panelHeaderRect(panel)) {
		return "drag"
	}
	return "content"
}

func (a *App) panelName(panel *PanelState) string {
	switch panel {
	case &a.clipboardPanel:
		return "clipboard"
	case &a.textPanel:
		return "text"
	case &a.historyPanel:
		return "history"
	case &a.thumbnailPanel:
		return "thumbnail"
	case &a.toolboxPanel:
		return "toolbox"
	case &a.palettePanel:
		return "palette"
	}
	return "panel"
}

func (a *App) visibleRightStage2Panels(exclude *PanelState) []*PanelState {
	refs := []dockedPanelRef{
		{&a.clipboardPanel, a.clipboardPanelVisible()},
		{&a.textPanel, a.textPanelVisible()},
		{&a.historyPanel, a.historyPanelVisible()},
		{&a.thumbnailPanel, a.thumbnailPanelVisible()},
	}
	panels := make([]*PanelState, 0, len(refs))
	for _, ref := range refs {
		if ref.Panel != exclude && ref.Visible && panelDockStage(ref.Panel) == 2 {
			panels = append(panels, ref.Panel)
		}
	}
	sort.SliceStable(panels, func(i, j int) bool { return panels[i].DockOrder < panels[j].DockOrder })
	return panels
}

// nearestRightDockSlot returns the second-stage vertical slot. Slots exist at
// the top-right corner and directly below every already aligned right panel.
func (a *App) nearestRightDockSlot(panel *PanelState, rawY int) (targetY, insertAt int, ok bool) {
	others := a.visibleRightStage2Panels(panel)
	y := panelDockTopOffset
	bestDist := absInt(rawY - y)
	bestY, bestInsert := y, 0
	for i, p := range others {
		y += p.H + panelDockGap
		d := absInt(rawY - y)
		if d < bestDist {
			bestDist, bestY, bestInsert = d, y, i+1
		}
	}
	return bestY, bestInsert, bestDist <= panelDockSnapDistance
}

func (a *App) placeRightPanelInSlot(panel *PanelState, insertAt int) {
	visible := a.visibleRightStage2Panels(panel)
	insertAt = clamp(insertAt, 0, len(visible))
	ordered := make([]*PanelState, 0, 4)
	ordered = append(ordered, visible[:insertAt]...)
	ordered = append(ordered, panel)
	ordered = append(ordered, visible[insertAt:]...)
	// Hidden second-stage panels are appended after the visible stack so every
	// panel keeps a unique order when it becomes visible again.
	all := []*PanelState{&a.clipboardPanel, &a.textPanel, &a.historyPanel, &a.thumbnailPanel}
	for _, candidate := range all {
		if candidate == panel || panelDockStage(candidate) != 2 {
			continue
		}
		found := false
		for _, present := range ordered {
			if present == candidate {
				found = true
				break
			}
		}
		if !found {
			ordered = append(ordered, candidate)
		}
	}
	for i, p := range ordered {
		p.DockOrder = i + 1
	}
	a.panelDockSerial = max(a.panelDockSerial, len(ordered))
}

func (a *App) beginPanelDrag(panel *PanelState, panelRect RECT, mx, my int) {
	panel.Dragging = true
	panel.DragOffsetX = mx - int(panelRect.Left)
	panel.DragOffsetY = my - int(panelRect.Top)
	if panelDockStage(panel) > 0 {
		panel.X = int(panelRect.Left)
		panel.Y = int(panelRect.Top)
	}
	procSetCapture.Call(a.hwnd)
}

func (a *App) movePanel(panel *PanelState, mx, my int, client RECT) {
	cw := int(client.Right - client.Left)
	ch := int(client.Bottom - client.Top)
	nx := mx - panel.DragOffsetX
	ny := my - panel.DragOffsetY
	oldStage := panelDockStage(panel)

	if panel == &a.toolboxPanel {
		panel.W = toolboxWidth
		panel.H = toolboxFloatingHeight
		stage := 0
		if absInt(nx) <= panelDockSnapDistance {
			stage = 1 // left edge only
			if absInt(ny) <= panelDockSnapDistance {
				stage = 2 // exact top-left corner
			}
		}
		setPanelDockStage(panel, stage)
		switch stage {
		case 2:
			panel.X, panel.Y = 0, 0
		case 1:
			panel.X = 0
			panel.Y = clamp(ny, 0, max(ch-panel.H, 0))
		default:
			panel.X = clamp(nx, 0, max(cw-panel.W, 0))
			panel.Y = clamp(ny, 0, max(ch-panel.H, 0))
		}
		if stage != oldStage {
			logf("PANEL dock stage name=toolbox stage=%d snap=%d", stage, panelDockSnapDistance)
		}
		return
	}

	if panel == &a.palettePanel {
		panel.W = a.palettePanelActualWidth()
		panel.H = a.palettePanelActualHeight()
		dockBottom := ch - statusBarHeight(a)
		bottomGap := dockBottom - (ny + panel.H)
		stage := 0
		if absInt(bottomGap) <= panelDockSnapDistance {
			stage = 1 // bottom edge only
			snapX := 0
			if a.toolboxPanelVisible() && panelDockStage(&a.toolboxPanel) == 2 {
				snapX = toolboxWidth
			}
			if absInt(nx-snapX) <= panelDockSnapDistance {
				stage = 2 // immediately right of the toolbox
			}
		}
		setPanelDockStage(panel, stage)
		switch stage {
		case 2:
			panel.X = 0
			if a.toolboxPanelVisible() && panelDockStage(&a.toolboxPanel) == 2 {
				panel.X = toolboxWidth
			}
			panel.Y = dockBottom - panel.H
		case 1:
			panel.X = clamp(nx, 0, max(cw-panel.W, 0))
			panel.Y = dockBottom - panel.H
		default:
			panel.X = clamp(nx, 0, max(cw-panel.W, 0))
			panel.Y = clamp(ny, 0, max(ch-panel.H, 0))
		}
		if stage != oldStage {
			logf("PANEL dock stage name=palette stage=%d snap=%d", stage, panelDockSnapDistance)
		}
		return
	}

	panel.W = rightPanelWidth
	rightGap := cw - (nx + panel.W)
	stage := 0
	if absInt(rightGap) <= panelDockSnapDistance {
		stage = 1 // right edge only; preserve vertical position
		if targetY, insertAt, ok := a.nearestRightDockSlot(panel, ny); ok {
			stage = 2
			panel.Y = targetY
			a.placeRightPanelInSlot(panel, insertAt)
		}
	}
	setPanelDockStage(panel, stage)
	if stage > 0 {
		panel.X = cw - panel.W
	} else {
		panel.X = clamp(nx, 0, max(cw-panel.W, 0))
	}
	if stage != 2 {
		panel.Y = clamp(ny, 0, max(ch-panel.H, 0))
	}
	if stage != oldStage {
		logf("PANEL dock stage name=%s stage=%d order=%d snap=%d", a.panelName(panel), stage, panel.DockOrder, panelDockSnapDistance)
	}
}

// lineGeometryStatusText reports live line or rectangle geometry in native
// canvas pixels. Lines use endpoint deltas: (10,10) to (14,11) is [4,1].
// Rectangles use inclusive outside raster size, so the same two corners are
// [5,2]. Shift adds angle/slope information for lines and equalizes the outside
// width and height for rectangles.
func (a *App) lineGeometryStatusText() string {
	if !a.drawing || !a.preview {
		return ""
	}
	if a.tool == ToolRectangle {
		nx, ny := a.currentRectangleEndpoint()
		_, _, _, _, width, height := rectangleOuterBounds(a.startX, a.startY, nx, ny)
		return fmt.Sprintf("[%d,%d]", width, height)
	}
	if a.tool == ToolEllipse {
		nx, ny := a.currentEllipseEndpoint()
		_, _, _, _, width, height := rectangleOuterBounds(a.startX, a.startY, nx, ny)
		major := max(width, height)
		minor := min(width, height)
		e := 0.0
		if major > 0 {
			ratio := float64(minor) / float64(major)
			e = math.Sqrt(math.Max(0, 1-ratio*ratio))
		}
		return fmt.Sprintf("e=%.4f   [%d,%d]", e, width, height)
	}
	if a.tool == ToolRightTriangle {
		nx, ny := a.currentRightTriangleEndpoint()
		dx := absInt(nx - a.startX)
		dy := absInt(ny - a.startY)
		dimensions := fmt.Sprintf("[%d,%d]", dx, dy)
		angle := 0.0
		if dx == 0 && dy > 0 {
			angle = 90.0
		} else if dx > 0 || dy > 0 {
			angle = math.Atan2(float64(dy), float64(dx)) * 180.0 / math.Pi
		}
		if keyDown(VK_SHIFT) && a.lineConstraintMode == LineConstraintTanN && (a.curX != a.startX || a.curY != a.startY) {
			rawDX := float64(a.curX - a.startX)
			rawDY := float64(a.curY - a.startY)
			step := nearestStrictTanStep(rawDX, rawDY)
			if step.X != 0 || step.Y != 0 {
				angle = math.Atan2(float64(absInt(step.Y)), float64(absInt(step.X))) * 180.0 / math.Pi
			}
			if tanText := lineTanStatusLabel(rawDX, rawDY); tanText != "" {
				return fmt.Sprintf("%s   θ=%.1f°   %s", tanText, angle, dimensions)
			}
		}
		return fmt.Sprintf("θ=%.1f°   %s", angle, dimensions)
	}
	if a.tool == ToolArrow {
		nx, ny := a.currentArrowEndpoint()
		dx := absInt(nx - a.startX)
		dy := absInt(ny - a.startY)
		dimensions := fmt.Sprintf("[%d,%d]", dx, dy)
		angle := arrowDirectionAngleDegrees(a.startX, a.startY, nx, ny)
		if keyDown(VK_SHIFT) && a.lineConstraintMode == LineConstraintTanN && (a.curX != a.startX || a.curY != a.startY) {
			rawDX := float64(a.curX - a.startX)
			rawDY := float64(a.curY - a.startY)
			step := nearestStrictTanStep(rawDX, rawDY)
			if step.X != 0 || step.Y != 0 {
				angle = arrowDirectionAngleDegrees(0, 0, step.X, step.Y)
			}
			if tanText := lineTanStatusLabel(rawDX, rawDY); tanText != "" {
				return fmt.Sprintf("%s   θ=%.1f°   %s", tanText, angle, dimensions)
			}
		}
		return fmt.Sprintf("θ=%.1f°   %s", angle, dimensions)
	}
	if a.tool != ToolLine {
		return ""
	}
	nx, ny := a.currentLineEndpoint()
	dx := absInt(nx - a.startX)
	dy := absInt(ny - a.startY)
	dimensions := fmt.Sprintf("[%d,%d]", dx, dy)
	if !keyDown(VK_SHIFT) || (a.curX == a.startX && a.curY == a.startY) {
		return dimensions
	}
	rawDX := float64(a.curX - a.startX)
	rawDY := float64(a.curY - a.startY)
	if a.lineConstraintMode == LineConstraint15Degrees {
		ux, uy := nearestLineDirection(rawDX, rawDY, LineConstraint15Degrees)
		if ux == 0 && uy == 0 {
			return dimensions
		}
		degrees := int(math.Round(math.Atan2(-uy, ux) * 180.0 / math.Pi))
		for degrees > 180 {
			degrees -= 360
		}
		for degrees <= -180 {
			degrees += 360
		}
		return fmt.Sprintf("%d°   %s", degrees, dimensions)
	}
	tanText := lineTanStatusLabel(rawDX, rawDY)
	if tanText == "" {
		return dimensions
	}
	return tanText + "   " + dimensions
}

// lineTanStatusLabel formats the selected strict integer-ratio direction exactly
// as the straight-line tool does. Screen Y grows downward, so a down-right line
// has a negative mathematical tangent sign.
func lineTanStatusLabel(rawDX, rawDY float64) string {
	step := nearestStrictTanStep(rawDX, rawDY)
	if step.X == 0 && step.Y == 0 {
		return ""
	}
	if step.X == 0 {
		return "수직"
	}
	if step.Y == 0 {
		return "tan 0"
	}
	rise, run := absInt(step.Y), absInt(step.X)
	ratio := ""
	if run == 1 {
		ratio = fmt.Sprintf("%d", rise)
	} else {
		ratio = fmt.Sprintf("%d/%d", rise, run)
	}
	sign := "+"
	if step.X*step.Y > 0 {
		sign = "-"
	}
	return fmt.Sprintf("%s tan %s", sign, ratio)
}

func (a *App) selectionGeometryStatusText() string {
	if a.tool != ToolRectSelect {
		return ""
	}
	if a.selection.Creating && !a.selection.Free {
		_, _, w, h := normalizeSelectionRect(a.selection.StartX, a.selection.StartY, a.selection.CurrentX, a.selection.CurrentY)
		return fmt.Sprintf("선택 %d × %d px", w, h)
	}
	if a.selection.Active && !a.selection.Free && a.selection.W > 0 && a.selection.H > 0 {
		return fmt.Sprintf("선택 %d × %d px", a.selection.W, a.selection.H)
	}
	return ""
}

func (a *App) paintStatus(hdc uintptr, rc RECT) {
	fillUIRect(hdc, rc, rgb(245, 245, 245))
	top := RECT{rc.Left, rc.Top, rc.Right, rc.Top + 1}
	fillUIRect(hdc, top, rgb(180, 180, 180))

	coordW := 120
	geometry := a.selectionGeometryStatusText()
	if geometry == "" {
		geometry = a.lineGeometryStatusText()
	}
	geometryW := 0
	if geometry != "" {
		geometryW = 280
	}
	coordLeft := rc.Right - int32(coordW)
	sepCoord := RECT{coordLeft, rc.Top + 3, coordLeft + 1, rc.Bottom - 3}
	fillUIRect(hdc, sepCoord, rgb(190, 190, 190))

	textRight := coordLeft - 8
	if geometryW > 0 {
		geometryLeft := coordLeft - int32(geometryW)
		sepGeometry := RECT{geometryLeft, rc.Top + 3, geometryLeft + 1, rc.Bottom - 3}
		fillUIRect(hdc, sepGeometry, rgb(190, 190, 190))
		gRc := RECT{geometryLeft + 8, rc.Top, coordLeft - 8, rc.Bottom}
		drawUIText(hdc, geometry, gRc, DT_RIGHT|DT_VCENTER|DT_SINGLELINE, rgb(30, 30, 30))
		textRight = geometryLeft - 8
	}

	textRc := RECT{rc.Left + 8, rc.Top, textRight, rc.Bottom}
	drawUIText(hdc, a.statusText, textRc, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS, rgb(30, 30, 30))
	coord := ""
	if a.cursorX >= 0 && a.cursorY >= 0 {
		coord = fmt.Sprintf("%d,%d", a.cursorX, a.cursorY)
	}
	cRc := RECT{coordLeft + 8, rc.Top, rc.Right - 8, rc.Bottom}
	drawUIText(hdc, coord, cRc, DT_LEFT|DT_VCENTER|DT_SINGLELINE, rgb(30, 30, 30))
}

// --- Input -----------------------------------------------------------------
func (a *App) hitTool(x, y int) (Tool, bool) {
	rows := (len(visibleTools) + 1) / 2
	if x < 4 || x >= 4+2*toolCellWidth || y < 4 || y >= 4+rows*toolCellHeight {
		return 0, false
	}
	col := (x - 4) / toolCellWidth
	row := (y - 4) / toolCellHeight
	if col > 1 || row < 0 || row >= rows {
		return 0, false
	}
	localX := (x - 4) % toolCellWidth
	localY := (y - 4) % toolCellHeight
	if localX >= toolButtonWidth || localY >= toolButtonHeight {
		return 0, false
	}
	index := row*2 + col
	if index < 0 || index >= len(visibleTools) {
		return 0, false
	}
	return visibleTools[index], true
}
func (a *App) hitZoomOption(x, y int) (int, bool) {
	optTop := toolOptionsTop()
	if x < toolboxOptionLeft || x > toolboxOptionRight || y < optTop {
		return 0, false
	}
	idx := (y - (optTop + 4)) / 19
	if idx >= 0 && idx < len(a.zoomLevels) {
		return a.zoomLevels[idx], true
	}
	return 0, false
}
func (a *App) hitPalette(x, y int, pal RECT) (uint32, int, bool) {
	if pointInRect(x, y, a.paletteTransparentArea(pal)) {
		return transparentColor, -1, true
	}
	for i, c := range a.paletteColors() {
		if pointInRect(x, y, a.paletteColorArea(pal, i)) {
			return c, i, true
		}
	}
	return 0, -1, false
}

func selectionOpacityOptionRects() (opaqueRect, transparentRect RECT) {
	optTop := toolOptionsTop()
	opaqueRect = RECT{12, int32(optTop + 8), 86, int32(optTop + 64)}
	transparentRect = RECT{12, int32(optTop + 70), 86, int32(optTop + 126)}
	return
}

func (a *App) hitSelectionOpacityOption(x, y int) (bool, bool) {
	opaqueRect, transparentRect := selectionOpacityOptionRects()
	if pointInRect(x, y, opaqueRect) {
		return true, true
	}
	if pointInRect(x, y, transparentRect) {
		return false, true
	}
	return false, false
}

func (a *App) setSelectionOpaque(opaque bool) {
	changed := a.opaque != opaque
	a.opaque = opaque
	a.toggleMenuCheck(ID_IMAGE_OPAQUE, a.opaque)
	if a.selection.Floating {
		a.applyFloatingSelection(a.selection.X, a.selection.Y)
		a.markDirty()
		if opaque {
			a.recordHistory("선택 불투명")
		} else {
			a.recordHistory("선택 투명")
		}
	}
	mode := "투명 선택"
	if opaque {
		mode = "불투명 선택"
	}
	a.statusText = mode + "을 적용했습니다. 현재 선택 영역과 이후 붙여넣기에 즉시 반영됩니다."
	logf("SELECTION opacity opaque=%v changed=%v active=%v floating=%v", opaque, changed, a.selection.Active, a.selection.Floating)
	invalidate(a.hwnd)
	procUpdateWindow.Call(a.hwnd)
}

func (a *App) paintSelectionOpacityOption(hdc uintptr, rc RECT, opaque bool, label string) {
	selected := a.opaque == opaque
	if a.tool == ToolText {
		selected = a.textTool.Opaque == opaque
	}
	if selected {
		fillUIRect(hdc, rc, rgb(205, 230, 255))
		frameUIRect(hdc, rc, rgb(0, 120, 215))
	} else {
		fillUIRect(hdc, rc, rgb(248, 248, 248))
		frameUIRect(hdc, rc, rgb(190, 190, 190))
	}
	labelRect := RECT{rc.Left + 3, rc.Top + 2, rc.Right - 3, rc.Top + 18}
	drawToolOptionText(hdc, label, labelRect, DT_CENTER|DT_VCENTER|DT_SINGLELINE, rgb(20, 20, 20), 9)
	preview := RECT{rc.Left + 15, rc.Top + 21, rc.Right - 15, rc.Bottom - 5}
	drawCheckerboard(hdc, preview, 5)
	selectionRect := RECT{preview.Left + 5, preview.Top + 4, preview.Right - 5, preview.Bottom - 4}
	if opaque {
		fillUIRect(hdc, selectionRect, gdiPreviewColor(a.bg))
	}
	fg := gdiPreviewColor(a.fg)
	fillUIRect(hdc, RECT{selectionRect.Left + 3, selectionRect.Top + 3, selectionRect.Left + 13, selectionRect.Top + 13}, fg)
	fillUIRect(hdc, RECT{selectionRect.Right - 14, selectionRect.Bottom - 13, selectionRect.Right - 4, selectionRect.Bottom - 3}, fg)
	frameUIRect(hdc, selectionRect, rgb(35, 35, 35))
}

func (a *App) curveInstruction() string {
	count := clamp(a.curveBendCount, 1, 4)
	if a.curveStage <= 0 {
		return fmt.Sprintf("곡선 기준선: 직선을 끌어 그으십시오. 이후 굽힘점 %d개를 순서대로 정합니다.", count)
	}
	if a.curveStage >= count {
		return fmt.Sprintf("곡선 조정 %d/%d: 굽힘점을 끌어 조정한 뒤 놓으면 확정됩니다.", a.curveStage, count)
	}
	return fmt.Sprintf("곡선 조정 %d/%d: 굽힘점을 끌어 조정하십시오. 다음 점은 이 점을 놓은 뒤 생성됩니다.", a.curveStage, count)
}

func (a *App) curveFinalizedControlCount() int {
	if !a.curveUndoPending || a.curveStage <= 1 {
		return 0
	}
	// curveStage is the point currently awaited. Only points before it have
	// been released and accepted. If Escape arrives during a drag, ignore the
	// in-progress point and preserve only the already accepted preview.
	return clamp(a.curveStage-1, 0, clamp(a.curveBendCount, 1, 4))
}

func (a *App) commitCurveWithControls(activeControls int, reason string) bool {
	activeControls = clamp(activeControls, 1, 4)
	if !a.curveUndoPending {
		return false
	}
	col := a.fg
	if a.drawButtonRight {
		col = a.bg
	}
	a.drawRasterCurve(a.canvas, activeControls, col)
	a.curveStage = 0
	a.curveUndoPending = false
	a.curveBaseShiftConstrained = false
	a.preview = false
	a.drawing = false
	procReleaseCapture.Call()
	a.markDirty()
	a.recordHistory("곡선")
	if a.tool == ToolCurve {
		a.statusText = fmt.Sprintf("곡선을 굽힘점 %d개 상태로 확정했습니다.", activeControls)
	}
	logf("CURVE early-commit reason=%q active-controls=%d requested=%d p0=%d,%d c1=%d,%d c2=%d,%d c3=%d,%d c4=%d,%d p1=%d,%d size=%d aa=%v", reason, activeControls, a.curveBendCount, a.curveX0, a.curveY0, a.curveC1X, a.curveC1Y, a.curveC2X, a.curveC2Y, a.curveC3X, a.curveC3Y, a.curveC4X, a.curveC4Y, a.curveX1, a.curveY1, a.curveSize, a.curveAntialias)
	return true
}

func (a *App) finishCurveEarlyOrCancel(reason string) bool {
	if controls := a.curveFinalizedControlCount(); controls >= 1 {
		return a.commitCurveWithControls(controls, reason)
	}
	a.cancelCurve(true)
	return false
}

func (a *App) cancelCurve(discardUndo bool) {
	if a.curveUndoPending && discardUndo {
		a.undo.DiscardLast()
	}
	a.curveStage = 0
	a.curveUndoPending = false
	a.curveBaseShiftConstrained = false
	a.preview = false
	a.drawing = false
	procReleaseCapture.Call()
	if a.tool == ToolCurve {
		a.statusText = a.curveInstruction()
	}
	logf("CURVE cancel discardUndo=%v", discardUndo)
}

const screenEyedropperTimerID = 0xD07E
const textCaretTimerID = 0xD07F
const textPreviewTimerID = 0xD080

func asyncKeyDown(vk int) bool {
	r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return uint16(r)&0x8000 != 0
}

func (a *App) startScreenEyedropper(previous Tool) {
	if previous == ToolEyedropper {
		previous = ToolPencil
	}
	a.eyedropperReturnTool = previous
	a.eyedropperActive = true
	a.eyedropperArmed = false
	procSetTimer.Call(a.hwnd, screenEyedropperTimerID, 20, 0)
	a.statusText = "스포이드: dotPaint 안팎의 화면에서 색을 한 번 클릭하십시오. 왼쪽은 전경색, 오른쪽은 배경색입니다."
	logf("EYEDROPPER global start returnTool=%d", previous)
}

func (a *App) stopScreenEyedropper() {
	if a.hwnd != 0 {
		procKillTimer.Call(a.hwnd, screenEyedropperTimerID)
	}
	a.eyedropperActive = false
	a.eyedropperArmed = false
}

func (a *App) restoreToolAfterEyedropper() {
	tool := a.eyedropperReturnTool
	if tool == ToolEyedropper || tool < ToolFreeSelect || tool > ToolArrow {
		tool = ToolPencil
	}
	a.tool = tool
	a.stopScreenEyedropper()
	if tool == ToolPencil || tool == ToolBrush || tool == ToolAirbrush {
		a.statusText = fmt.Sprintf("색을 추출했습니다. 통합 붓(%s, %d px)으로 돌아왔습니다.", brushShapeNames[a.brushShape], a.brushSize)
	} else {
		a.statusText = fmt.Sprintf("색을 추출했습니다. %s 도구로 돌아왔습니다.", toolNames[tool])
	}
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	a.updateTextUILayout(client)
	a.updateScrollbars()
	invalidate(a.hwnd)
	procSetForegroundWindow.Call(a.hwnd)
	logf("EYEDROPPER restore tool=%d", tool)
}

func (a *App) sampleScreenColor(screenX, screenY int, right bool) bool {
	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		logf("EYEDROPPER screen GetDC failed")
		return false
	}
	c, _, _ := procGetPixel.Call(hdc, uintptr(screenX), uintptr(screenY))
	procReleaseDC.Call(0, hdc)
	if uint32(c) == 0xFFFFFFFF {
		logf("EYEDROPPER screen GetPixel failed at=%d,%d", screenX, screenY)
		return false
	}
	if right {
		a.bg = uint32(c)
	} else {
		a.fg = uint32(c)
	}
	which := "전경색"
	if right {
		which = "배경색"
	}
	a.statusText = fmt.Sprintf("화면 (%d,%d)에서 %s #%02X%02X%02X을 추출했습니다.", screenX, screenY, which, byte(c), byte(c>>8), byte(c>>16))
	logf("EYEDROPPER screen sample x=%d y=%d right=%v color=%#08x", screenX, screenY, right, uint32(c))
	return true
}

func (a *App) completeScreenEyedropper(right bool) {
	if !a.eyedropperActive {
		return
	}
	var pt POINT
	if r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); r == 0 {
		logf("EYEDROPPER GetCursorPos failed")
		return
	}
	if a.sampleScreenColor(int(pt.X), int(pt.Y), right) {
		a.restoreToolAfterEyedropper()
	}
}

func (a *App) pollScreenEyedropper() {
	if !a.eyedropperActive || a.tool != ToolEyedropper {
		return
	}
	left := asyncKeyDown(VK_LBUTTON)
	right := asyncKeyDown(VK_RBUTTON)
	if !a.eyedropperArmed {
		if !left && !right {
			a.eyedropperArmed = true
			logf("EYEDROPPER global armed")
		}
		return
	}
	if right {
		a.completeScreenEyedropper(true)
	} else if left {
		a.completeScreenEyedropper(false)
	}
}

func (a *App) onMouseDoubleClick(mx, my int) {
	var client RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
	if a.bitmapView || !a.showPalette {
		return
	}
	panel := a.palettePanelRect(client)
	content := panelContentRect(panel)
	if content.Right <= content.Left || !pointInRect(mx, my, content) {
		return
	}
	local := RECT{0, 0, content.Right - content.Left, content.Bottom - content.Top}
	lx := mx - int(content.Left)
	ly := my - int(content.Top)
	if pointInRect(lx, ly, paletteForegroundRect(local)) {
		logf("PALETTE double-click foreground")
		a.editColorTarget(false)
		return
	}
	if pointInRect(lx, ly, paletteBackgroundRect(local)) {
		logf("PALETTE double-click background")
		a.editColorTarget(true)
		return
	}
}

func (a *App) onMouseDown(mx, my int, right bool) {
	logf("MOUSEDOWN client=%d,%d right=%v tool=%d zoom=%d scroll=%d,%d", mx, my, right, a.tool, a.zoom, a.scrollX, a.scrollY)
	if a.textCommitClickPending {
		// Defensive guard for duplicate button-down delivery while the commit-only
		// click is still waiting for its matching button-up.
		logf("TEXT pending commit click ignored duplicate mouse-down")
		return
	}
	var rc RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	if !a.bitmapView && a.showTextPanel && a.tool == ToolText {
		action := a.hitTextPanel(mx, my, rc)
		switch action {
		case "close":
			a.showTextPanel = false
			a.toggleMenuCheck(ID_VIEW_TEXTBAR, false)
			a.updateTextUILayout(rc)
			a.updateScrollbars()
			a.statusText = "텍스트 도구 상자를 닫았습니다. A 도구를 다시 누르면 열립니다."
			invalidate(a.hwnd)
			return
		case "drag":
			a.beginPanelDrag(&a.textPanel, a.textPanelRect(rc), mx, my)
			return
		case "panel":
			return
		}
	}
	if a.thumbnailPanelVisible() {
		action := a.hitThumbnailPanel(mx, my, rc)
		switch action {
		case "drag":
			a.beginPanelDrag(&a.thumbnailPanel, a.thumbnailPanelRect(rc), mx, my)
			return
		case "close":
			a.showThumbnail = false
			a.toggleMenuCheck(ID_VIEW_THUMBNAIL, false)
			a.updateTextUILayout(rc)
			a.statusText = "미리 보기 창을 닫았습니다."
			invalidate(a.hwnd)
			return
		case "jump":
			if right {
				return
			}
			a.thumbnailNavigationDragging = true
			procSetCapture.Call(a.hwnd)
			a.jumpToThumbnailPoint(mx, my, rc)
			logf("THUMBNAIL drag begin client=%d,%d", mx, my)
			return
		case "panel":
			return
		}
	}
	if !a.bitmapView && a.showHistory {
		action, index := a.hitHistoryPanel(mx, my, rc)
		switch action {
		case "drag":
			a.beginPanelDrag(&a.historyPanel, a.historyPanelRect(rc), mx, my)
			return
		case "close":
			a.showHistory = false
			a.toggleMenuCheck(ID_VIEW_HISTORY, false)
			a.statusText = "히스토리 창을 닫았습니다. 보기 메뉴에서 다시 열 수 있습니다."
			invalidate(a.hwnd)
			return
		case "goto":
			if a.tool == ToolText && a.textTool.Active {
				a.finishTextEditing(true)
			}
			a.gotoHistory(index)
			return
		case "scrollup":
			a.scrollHistory(-1)
			return
		case "scrolldown":
			a.scrollHistory(1)
			return
		case "pageup":
			a.scrollHistory(-a.historyVisibleRows())
			return
		case "pagedown":
			a.scrollHistory(a.historyVisibleRows())
			return
		case "scrollthumb":
			a.historyScrollDragging = true
			a.historyScrollDragOffset = max(index, 0)
			procSetCapture.Call(a.hwnd)
			return
		case "panel":
			return
		}
	}
	if !a.bitmapView && a.showClipboard {
		action, index := a.hitClipboardPanel(mx, my, rc)
		switch action {
		case "drag":
			a.beginPanelDrag(&a.clipboardPanel, a.clipboardPanelRect(rc), mx, my)
			return
		case "close":
			a.showClipboard = false
			a.toggleMenuCheck(ID_VIEW_CLIPBOARD, false)
			a.updateScrollbars()
			a.updateTextUILayout(rc)
			a.statusText = "클립보드 창을 닫았습니다. 보기 메뉴에서 다시 열 수 있습니다."
			invalidate(a.hwnd)
			return
		case "scrollup":
			a.scrollClipboard(-1)
			return
		case "scrolldown":
			a.scrollClipboard(1)
			return
		case "pageup":
			a.scrollClipboard(-clipboardVisibleRows)
			return
		case "pagedown":
			a.scrollClipboard(clipboardVisibleRows)
			return
		case "scrollthumb":
			a.clipboardScrollDragging = true
			a.clipboardScrollDragOffset = max(index, 0)
			procSetCapture.Call(a.hwnd)
			return
		case "pin":
			if !right && index >= 0 && index < len(a.clipboardHistory) {
				a.toggleClipboardPin(index)
			}
			return
		case "paste":
			if !right && index >= 0 && index < len(a.clipboardHistory) {
				a.clipboardSelected = index
				a.clipboard = a.clipboardHistory[index].Image
				a.pasteClipboardImage(a.clipboardHistory[index].Image, fmt.Sprintf("클립보드 기록 %d", index+1))
			} else if index >= len(a.clipboardHistory) {
				a.statusText = "이 클립보드 칸은 비어 있습니다. Print Screen 또는 다른 프로그램에서 이미지를 복사하십시오."
				invalidate(a.hwnd)
			}
			return
		case "panel":
			return
		}
	}
	if !a.bitmapView && a.showToolbox {
		action := a.hitToolboxPanel(mx, my, rc)
		switch action {
		case "close":
			a.showToolbox = false
			a.toggleMenuCheck(ID_VIEW_TOOLBOX, false)
			a.updateScrollbars()
			a.updateTextUILayout(rc)
			a.statusText = "도구 상자를 닫았습니다. 보기 메뉴에서 다시 열 수 있습니다."
			invalidate(a.hwnd)
			return
		case "drag":
			a.beginPanelDrag(&a.toolboxPanel, a.toolboxPanelRect(rc), mx, my)
			return
		}
	}
	if !a.bitmapView && a.showPalette {
		action := a.hitPalettePanel(mx, my, rc)
		switch action {
		case "close":
			a.showPalette = false
			a.toggleMenuCheck(ID_VIEW_PALETTE, false)
			a.updateScrollbars()
			a.updateTextUILayout(rc)
			a.statusText = "색상표를 닫았습니다. 보기 메뉴에서 다시 열 수 있습니다."
			invalidate(a.hwnd)
			return
		case "drag":
			a.beginPanelDrag(&a.palettePanel, a.palettePanelRect(rc), mx, my)
			return
		}
	}
	ws, tb, pal, _ := a.layout(rc)
	if a.tool == ToolText && a.textTool.Active {
		mode, handle := a.hitTextBoxAction(mx, my)
		if mode == 1 || mode == 2 {
			a.beginTextBoxDrag(mode, handle, mx, my)
			return
		}
		if mode == 3 {
			a.textTool.Editing = true
			var client RECT
			procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
			a.updateTextUILayout(client)
			a.beginTextRangeSelection(mx, my)
			return
		}
		if _, _, insideCanvas := a.clientToImage(mx, my); insideCanvas {
			// An outside canvas click is a dedicated "finish current text" action.
			// It must not fall through to the generic text-box creation path, even if
			// Windows delivers mouse-move messages before the matching button-up.
			a.textCommitClickPending = true
			a.drawing = false
			a.preview = false
			a.textTool.Creating = false
			a.textTool.Selecting = false
			a.finishTextEditing(true)
			// Re-capture after finishTextEditing releases any prior drag capture. The
			// matching button-up is then guaranteed to return to this window and be
			// consumed by the one-shot barrier below.
			procSetCapture.Call(a.hwnd)
			logf("TEXT outside click committed only; consume click until mouse-up")
			return
		}
	}
	if !a.bitmapView && a.showToolbox && pointInRect(mx, my, tb) {
		toolX := mx - int(tb.Left)
		toolY := my - int(tb.Top)
		toolRC := RECT{0, 0, tb.Right - tb.Left, tb.Bottom - tb.Top}
		if t, ok := a.hitTool(toolX, toolY); ok {
			logf("TOOL select candidate=%d name=%q current=%d", t, toolNames[t], a.tool)
			previousTool := a.tool
			if previousTool == ToolEyedropper {
				previousTool = a.eyedropperReturnTool
			}
			if a.tool == ToolEyedropper && t != ToolEyedropper {
				a.stopScreenEyedropper()
			}
			if a.tool == ToolText && t != ToolText && a.textTool.Active {
				a.finishTextEditing(true)
			}
			if a.tool == ToolCurve && t != ToolCurve && a.curveUndoPending {
				a.finishCurveEarlyOrCancel("tool switch")
			}
			if !isSelectionTool(t) && a.selection.Active {
				a.clearSelectionState()
			}
			if t == ToolMagnifier {
				a.rememberMagnifierReturnTool(previousTool)
			} else if a.tool == ToolMagnifier {
				a.clearZoomPreview()
			}
			a.tool = t
			if t == ToolText {
				a.showTextPanel = true
				a.toggleMenuCheck(ID_VIEW_TEXTBAR, true)
				a.ensureTextControls()
				var client RECT
				procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
				a.updateTextUILayout(client)
				a.statusText = "텍스트: 캔버스에서 끌어 텍스트 상자를 만든 뒤 입력하십시오. 확대 상태에서도 사용할 수 있습니다."
				logf("TEXT tool activated zoom=%d", a.zoom)
			} else if t == ToolCurve {
				aa := "끔"
				if a.curveAntialias {
					aa = "켬"
				}
				a.statusText = fmt.Sprintf("곡선: 굽힘점 %d개, %d px, 안티에일리어싱=%s. 기준선에서 Shift를 누르면 수평·수직·45°로 고정됩니다.", a.curveBendCount, a.curveSize, aa)
			} else if t == ToolEyedropper {
				a.startScreenEyedropper(previousTool)
			} else if t == ToolMagnifier {
				a.zoomTarget = a.defaultZoomTarget()
				a.clearZoomPreview()
				a.statusText = fmt.Sprintf("확대 대상 %dx: 마우스를 움직여 점선 영역을 확인하고 클릭하십시오. 오른쪽 클릭은 한 단계 축소합니다.", a.zoomTarget)
			} else if t == ToolFreeSelect {
				a.statusText = "자유 선택: 원하는 영역의 둘레를 그리십시오. Shift를 누르면 기존 선택에 추가합니다."
			} else if t == ToolRectSelect {
				a.statusText = "사각 선택: 영역을 끌어 지정하십시오. Shift를 누르면 기존 선택에 추가합니다."
			} else if t == ToolColorRegionSelect {
				a.statusText = fmt.Sprintf("색 영역 선택: 클릭한 색과 비슷한 인접 영역만 선택합니다. 허용 불일치 %d, Shift=기존 선택에 추가.", a.colorTolerance)
			} else if t == ToolColorAllSelect {
				a.statusText = fmt.Sprintf("동일 색 전체 선택: 클릭한 색과 비슷한 모든 픽셀을 선택합니다. 허용 불일치 %d, Shift=기존 선택에 추가.", a.colorTolerance)
			} else if t == ToolEraser {
				a.statusText = fmt.Sprintf("지우개: 현재 %d px. Shift를 누른 채 끌면 수평·수직·45°로 고정됩니다. 왼쪽 옵션에서 크기를 고르십시오.", a.eraserSize)
			} else if t == ToolFill {
				a.statusText = fmt.Sprintf("색 채우기: 클릭한 지점과 이어진 비슷한 색을 채웁니다. 허용 불일치 %d.", a.colorTolerance)
			} else if t == ToolPencil {
				a.statusText = fmt.Sprintf("통합 붓: %s, %d px. 자유롭게 그리다가 Shift를 누르면 그 지점부터 직선·45° 대각선 구간이 되고, Shift를 떼면 다시 자유선으로 이어집니다.", brushShapeNames[a.brushShape], a.brushSize)
			} else if t == ToolLine {
				aa := "끔"
				if a.lineAntialias {
					aa = "켬"
				}
				a.statusText = fmt.Sprintf("직선: 네모 붓 %d px, Shift=%s, 안티에일리어싱=%s. 정사방·45°는 흐리지 않습니다.", a.lineSize, lineConstraintNames[a.lineConstraintMode], aa)
			} else if t == ToolRectangle {
				a.statusText = fmt.Sprintf("사각형: 선 굵기 %d px. 드래그 위치가 바깥 테두리가 되며 Shift를 누르면 정사각형이 됩니다.", a.rectangleSize)
			} else if t == ToolEllipse {
				a.statusText = fmt.Sprintf("타원: 선 굵기 %d px. 드래그 위치가 바깥 크기가 되며 Shift를 누르면 원이 됩니다.", a.ellipseSize)
			} else if t == ToolRightTriangle {
				a.statusText = fmt.Sprintf("직각삼각형: 직선처럼 빗변을 그리면 가로·세로 변이 추가됩니다. 선 굵기 %d px, Shift=%s.", a.rightTriangleSize, lineConstraintNames[a.lineConstraintMode])
			} else if t == ToolArrow {
				aa := "끔"
				if a.arrowAntialias {
					aa = "켬"
				}
				a.statusText = fmt.Sprintf("화살표: 꼬리에서 머리 방향으로 드래그합니다. 머리 %d단계, 네모 선 %d px, Shift=%s, 안티에일리어싱=%s(각 변은 수평·수직·45도일 때 선명 유지).", a.arrowHeadSize+1, a.arrowLineSize, lineConstraintNames[a.lineConstraintMode], aa)
			} else {
				a.statusText = toolNames[t]
			}
			a.updateScrollbars()
			var client RECT
			procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
			a.updateTextUILayout(client)
			invalidate(a.hwnd)
			return
		}
		if a.tool == ToolMagnifier {
			if z, ok := a.hitZoomOption(toolX, toolY); ok {
				if z < a.zoom {
					oldZoom := a.zoom
					a.setZoomTopLeft(z)
					a.zoomTarget = a.defaultZoomTarget()
					a.restoreToolAfterMagnifier(fmt.Sprintf("축소 완료: %dx에서 %dx로 변경했습니다.", oldZoom, a.zoom))
					return
				}
				a.zoomTarget = z
				a.clearZoomPreview()
				if z == a.zoom {
					a.statusText = fmt.Sprintf("현재 배율은 이미 %dx입니다.", z)
				} else {
					a.statusText = fmt.Sprintf("확대 대상 %dx: 캔버스에서 점선 영역을 정한 뒤 클릭하십시오. 현재 배율은 %dx입니다.", z, a.zoom)
				}
				logf("ZOOM target-select current=%d target=%d", a.zoom, z)
				invalidate(a.hwnd)
				return
			}
		}
		if a.tool == ToolPencil || a.tool == ToolBrush || a.tool == ToolAirbrush {
			if shape, ok := a.hitBrushShapeOption(toolX, toolY); ok {
				a.setBrushShape(shape)
				return
			}
			if size, ok := a.hitBrushSizeOption(toolX, toolY, toolRC); ok {
				a.setBrushSize(size)
				a.adjustingBrushSize = true
				procSetCapture.Call(a.hwnd)
				invalidate(a.hwnd)
				return
			}
		}
		if a.tool == ToolEraser {
			if size, ok := a.hitEraserSizeOption(toolX, toolY, toolRC); ok {
				a.setEraserSize(size)
				a.adjustingEraserSize = true
				procSetCapture.Call(a.hwnd)
				invalidate(a.hwnd)
				return
			}
		}
		if a.tool == ToolLine {
			if mode, ok := a.hitLineConstraintOption(toolX, toolY); ok {
				a.lineConstraintMode = mode
				a.statusText = fmt.Sprintf("Shift 직선 제한을 %s 방식으로 설정했습니다.", lineConstraintNames[mode])
				logf("LINE constraint=%d name=%q", mode, lineConstraintNames[mode])
				invalidate(a.hwnd)
				return
			}
			if aa, ok := a.hitLineAAOption(toolX, toolY); ok {
				a.lineAntialias = aa
				state := "끔"
				if aa {
					state = "켬"
				}
				a.statusText = "직선 안티에일리어싱: " + state
				logf("LINE antialias=%v", aa)
				invalidate(a.hwnd)
				return
			}
			if size, ok := a.hitLineSizeOption(toolX, toolY, toolRC); ok {
				a.setLineSize(size)
				a.adjustingLineSize = true
				procSetCapture.Call(a.hwnd)
				invalidate(a.hwnd)
				return
			}
		}
		if a.tool == ToolCurve {
			if count, ok := a.hitCurveBendOption(toolX, toolY); ok {
				if a.curveUndoPending {
					a.finishCurveEarlyOrCancel("bend-count slider")
				}
				a.setCurveBendCount(count)
				a.adjustingCurveBends = true
				procSetCapture.Call(a.hwnd)
				invalidate(a.hwnd)
				return
			}
			if aa, ok := a.hitCurveAAOption(toolX, toolY); ok {
				a.curveAntialias = aa
				state := "끔"
				if aa {
					state = "켬"
				}
				a.statusText = "곡선 안티에일리어싱: " + state
				logf("CURVE antialias=%v", aa)
				invalidate(a.hwnd)
				return
			}
			if size, ok := a.hitCurveSizeOption(toolX, toolY, toolRC); ok {
				a.setCurveSize(size)
				a.adjustingCurveSize = true
				procSetCapture.Call(a.hwnd)
				invalidate(a.hwnd)
				return
			}
		}

		if a.tool == ToolRectangle {
			if a.hitShapeCenterOption(toolX, toolY) {
				a.toggleShapeCenter()
				invalidate(a.hwnd)
				return
			}
			if size, ok := a.hitRectangleSizeOption(toolX, toolY, toolRC); ok {
				a.setRectangleSize(size)
				a.adjustingRectangleSize = true
				procSetCapture.Call(a.hwnd)
				invalidate(a.hwnd)
				return
			}
		}

		if a.tool == ToolEllipse {
			if a.hitShapeCenterOption(toolX, toolY) {
				a.toggleShapeCenter()
				invalidate(a.hwnd)
				return
			}
			if size, ok := a.hitEllipseSizeOption(toolX, toolY, toolRC); ok {
				a.setEllipseSize(size)
				a.adjustingEllipseSize = true
				procSetCapture.Call(a.hwnd)
				invalidate(a.hwnd)
				return
			}
		}

		if a.tool == ToolRightTriangle {
			if mode, ok := a.hitRightTriangleConstraintOption(toolX, toolY); ok {
				a.lineConstraintMode = mode
				a.statusText = fmt.Sprintf("Shift 직각삼각형 제한을 %s 방식으로 설정했습니다.", lineConstraintNames[mode])
				logf("RIGHT_TRIANGLE constraint=%d name=%q", mode, lineConstraintNames[mode])
				invalidate(a.hwnd)
				return
			}
			if size, ok := a.hitRightTriangleSizeOption(toolX, toolY, toolRC); ok {
				a.setRightTriangleSize(size)
				a.adjustingRightTriangleSize = true
				procSetCapture.Call(a.hwnd)
				invalidate(a.hwnd)
				return
			}
		}

		if a.tool == ToolArrow {
			if head, ok := a.hitArrowHeadOption(toolX, toolY); ok {
				a.arrowHeadSize = head
				a.statusText = fmt.Sprintf("화살표 머리 크기를 %d단계로 설정했습니다.", head+1)
				logf("ARROW head=%d", head)
				invalidate(a.hwnd)
				return
			}
			if mode, ok := a.hitArrowConstraintOption(toolX, toolY); ok {
				a.lineConstraintMode = mode
				a.statusText = fmt.Sprintf("Shift 화살표 제한을 %s 방식으로 설정했습니다.", lineConstraintNames[mode])
				logf("ARROW constraint=%d name=%q", mode, lineConstraintNames[mode])
				invalidate(a.hwnd)
				return
			}
			if aa, ok := a.hitArrowAAOption(toolX, toolY); ok {
				a.arrowAntialias = aa
				state := "끔"
				if aa {
					state = "켬"
				}
				a.statusText = "화살표 안티에일리어싱: " + state
				logf("ARROW antialias=%v", aa)
				invalidate(a.hwnd)
				return
			}
			if size, ok := a.hitArrowLineSizeOption(toolX, toolY, toolRC); ok {
				a.setArrowLineSize(size)
				a.adjustingArrowLineSize = true
				procSetCapture.Call(a.hwnd)
				invalidate(a.hwnd)
				return
			}
		}

		if isColorSelectionTool(a.tool) || a.tool == ToolFill {
			if tolerance, ok := a.hitColorToleranceOption(toolX, toolY, toolRC); ok {
				a.setColorTolerance(tolerance)
				a.adjustingColorTolerance = true
				procSetCapture.Call(a.hwnd)
				invalidate(a.hwnd)
				return
			}
		}
		if a.tool == ToolFreeSelect || a.tool == ToolRectSelect {
			if opaque, ok := a.hitSelectionOpacityOption(toolX, toolY); ok {
				a.setSelectionOpaque(opaque)
				return
			}
		}
		if a.tool == ToolText {
			if opaque, ok := a.hitSelectionOpacityOption(toolX, toolY); ok {
				a.setTextOpaque(opaque)
				return
			}
		}
	}
	if !a.bitmapView && a.showPalette && pointInRect(mx, my, pal) {
		paletteX := mx - int(pal.Left)
		paletteY := my - int(pal.Top)
		paletteRC := RECT{0, 0, pal.Right - pal.Left, pal.Bottom - pal.Top}
		// The overlapping foreground/background swatches are edit targets, not
		// canvas input. Consume the first click so a double-click cannot leak
		// through to the canvas; WM_LBUTTONDBLCLK opens the matching color editor.
		if pointInRect(paletteX, paletteY, paletteForegroundRect(paletteRC)) || pointInRect(paletteX, paletteY, paletteBackgroundRect(paletteRC)) {
			return
		}
		if c, slot, ok := a.hitPalette(paletteX, paletteY, paletteRC); ok {
			which := "전경색"
			if a.indexedPaletteActive() {
				slot = -1 // indexed slots have a separate identity space
			}
			if right {
				a.bg = c
				a.bgPaletteSlot = slot
				which = "배경색"
			} else {
				a.fg = c
				a.fgPaletteSlot = slot
			}
			if isTransparentColor(c) {
				a.statusText = which + "을 투명색으로 설정했습니다."
				logf("PALETTE transparent selected target=%s", which)
			} else {
				a.statusText = fmt.Sprintf("%s을 #%02X%02X%02X로 설정했습니다.", which, byte(c), byte(c>>8), byte(c>>16))
			}
			invalidate(a.hwnd)
			return
		}
	}
	_ = ws
	if !right && a.tool != ToolMagnifier {
		if h, ok := a.hitCanvasHandle(mx, my); ok {
			if a.curveUndoPending {
				a.finishCurveEarlyOrCancel("canvas resize")
			}
			a.armCanvasResize(h, mx, my)
			return
		}
	}
	ix, iy, inside := a.clientToImage(mx, my)
	if !inside {
		// A rectangular selection may begin in the grey workspace. Arm the
		// gesture instead of creating a one-pixel edge selection immediately;
		// a real drag starts at the nearest outermost canvas pixel.
		grayWorkspace := pointInRect(mx, my, ws) && !pointInRect(mx, my, tb) && !pointInRect(mx, my, pal)
		if grayWorkspace && !right && a.tool == ToolRectSelect {
			wasActive := a.selection.Active
			wasFloating := a.selection.Floating
			if wasActive {
				a.clearSelectionState()
			}
			a.armPendingRectSelection(mx, my, ix, iy, true)
			a.statusText = "캔버스 바깥에서 사각 선택 대기 중: 끌면 최외각 픽셀부터 선택하고, 클릭만 하면 선택을 취소합니다."
			logf("SELECTION rect outside-canvas press active=%v floating=%v client=%d,%d rawImage=%d,%d", wasActive, wasFloating, mx, my, ix, iy)
			a.requestCanvasRepaint(true)
			return
		}
		// Other tools retain the established grey-workspace cancellation.
		if grayWorkspace && a.selection.Active {
			wasFloating := a.selection.Floating
			a.clearSelectionState()
			a.statusText = "선택을 해제했습니다."
			logf("SELECTION clear by gray workspace click floating=%v client=%d,%d image=%d,%d", wasFloating, mx, my, ix, iy)
			a.requestCanvasRepaint(true)
			return
		}
		logf("MOUSEDOWN outside canvas image=%d,%d", ix, iy)
		return
	}
	logf("MOUSEDOWN image=%d,%d", ix, iy)
	a.cursorX, a.cursorY = ix, iy
	if a.tool == ToolMagnifier {
		if right {
			oldZoom := a.zoom
			a.nextZoom(-1, mx, my)
			a.zoomTarget = a.defaultZoomTarget()
			a.clearZoomPreview()
			a.restoreToolAfterMagnifier(fmt.Sprintf("축소 완료: %dx에서 %dx로 변경했습니다.", oldZoom, a.zoom))
		} else {
			a.applyZoomPreview(mx, my)
		}
		return
	}
	if a.tool == ToolEyedropper {
		a.completeScreenEyedropper(right)
		return
	}
	if isColorSelectionTool(a.tool) {
		a.selectByColor(ix, iy, a.tool == ToolColorAllSelect, keyDown(VK_SHIFT))
		return
	}
	if a.tool == ToolFreeSelect || a.tool == ToolRectSelect {
		insideExisting := a.selection.Active && a.selection.Contains(ix, iy)
		// Shift keeps its additive-selection meaning when drawing outside the
		// current region. Inside the current region it starts a constrained move,
		// including when Shift was already held before mouse-down.
		additive := keyDown(VK_SHIFT) && a.selection.Active && !insideExisting
		// In rectangular-selection mode, a plain click outside an existing
		// marquee is cancellation only. A deliberate drag from the same press
		// begins a replacement rectangle after the click/drag threshold.
		if a.tool == ToolRectSelect && !right && a.selection.Active && !insideExisting && !additive {
			wasFloating := a.selection.Floating
			a.clearSelectionState()
			a.armPendingRectSelection(mx, my, ix, iy, false)
			a.statusText = "선택 바깥 클릭: 놓으면 취소하고, 끌면 새 사각 선택을 시작합니다."
			logf("SELECTION rect outside-existing press cancel-armed floating=%v image=%d,%d", wasFloating, ix, iy)
			a.requestCanvasRepaint(true)
			return
		}
		if insideExisting {
			if a.liftSelectionForMove() {
				a.selection.Moving = true
				a.selection.MoveOffsetX = ix - a.selection.X
				a.selection.MoveOffsetY = iy - a.selection.Y
				a.selection.MoveStartX, a.selection.MoveStartY = a.selection.X, a.selection.Y
				a.selection.MovePointerX, a.selection.MovePointerY = ix, iy
				procSetCapture.Call(a.hwnd)
				a.statusText = fmt.Sprintf("선택 영역 이동 중: %d × %d. Shift를 누르면 수평·수직·45°로 제한합니다.", a.selection.W, a.selection.H)
				logf("SELECTION move begin x=%d y=%d pointer=%d,%d offset=%d,%d shift=%v", a.selection.X, a.selection.Y, ix, iy, a.selection.MoveOffsetX, a.selection.MoveOffsetY, keyDown(VK_SHIFT))
			}
		} else {
			var baseX, baseY, baseW, baseH int
			var baseMask []bool
			baseFree := false
			if additive {
				baseX, baseY, baseW, baseH = a.selection.X, a.selection.Y, a.selection.W, a.selection.H
				baseMask = append([]bool(nil), a.selection.Mask...)
				baseFree = a.selection.Free
				logf("SELECTION add begin base=%d,%d %dx%d free=%v", baseX, baseY, baseW, baseH, baseFree)
			} else {
				a.clearSelectionState()
			}
			a.selection.Reset()
			a.selection.Creating = true
			a.selection.Free = a.tool == ToolFreeSelect
			a.selection.StartX, a.selection.StartY = ix, iy
			a.selection.CurrentX, a.selection.CurrentY = ix, iy
			if additive {
				a.selection.Adding = true
				a.selection.AddBaseX, a.selection.AddBaseY = baseX, baseY
				a.selection.AddBaseW, a.selection.AddBaseH = baseW, baseH
				a.selection.AddBaseMask = baseMask
				a.selection.AddBaseFree = baseFree
			}
			if a.selection.Free {
				a.selection.Lasso = []PixelPoint{{ix, iy}}
			}
			procSetCapture.Call(a.hwnd)
			if additive {
				a.statusText = "Shift 선택 추가: 새 영역을 그리면 기존 선택에 합쳐집니다."
			}
		}
		a.requestCanvasRepaint(true)
		return
	}
	if a.tool == ToolFill {
		c := a.fg
		if right {
			c = a.bg
		}
		if a.canvas.GetPixel(ix, iy) != c || a.colorTolerance > 0 {
			a.undo.Push(a.canvas)
			if a.canvas.FloodFill(ix, iy, c, a.colorTolerance) {
				a.markDirty()
				a.recordHistory("색 채우기")
				a.requestCanvasRepaint(true)
			}
		}
		return
	}
	if a.tool == ToolCurve {
		oldCurveRect := a.curvePreviewScreenRect()
		if a.curveStage == 0 {
			a.undo.Push(a.canvas)
			a.curveUndoPending = true
			a.drawButtonRight = right
			a.curveX0, a.curveY0 = ix, iy
			a.curveX1, a.curveY1 = ix, iy
			a.curveBaseShiftConstrained = keyDown(VK_SHIFT)
			a.curveC1X, a.curveC1Y = ix, iy
			a.curveC2X, a.curveC2Y = ix, iy
			a.curveC3X, a.curveC3Y = ix, iy
			a.curveC4X, a.curveC4Y = ix, iy
			a.startX, a.startY = ix, iy
			a.curX, a.curY = ix, iy
			a.preview = true
			a.statusText = fmt.Sprintf("곡선 기준선: 끝점까지 끈 뒤 놓으십시오. 이후 굽힘점 %d개를 순서대로 정합니다.", a.curveBendCount)
			logf("CURVE base begin start=%d,%d right=%v bends=%d size=%d aa=%v shift45=%v", ix, iy, right, a.curveBendCount, a.curveSize, a.curveAntialias, a.curveBaseShiftConstrained)
		} else {
			switch a.curveStage {
			case 1:
				a.curveC1X, a.curveC1Y = ix, iy
			case 2:
				a.curveC2X, a.curveC2Y = ix, iy
			case 3:
				a.curveC3X, a.curveC3Y = ix, iy
			case 4:
				a.curveC4X, a.curveC4Y = ix, iy
			}
			a.statusText = fmt.Sprintf("곡선 조정 %d/%d 진행 중", a.curveStage, a.curveBendCount)
			logf("CURVE bend%d begin at=%d,%d active-controls=%d", a.curveStage, ix, iy, a.curveStage)
		}
		a.drawing = true
		procSetCapture.Call(a.hwnd)
		newCurveRect := a.curvePreviewScreenRect()
		a.requestRepaintRect(unionRECT(oldCurveRect, newCurveRect), true)
		return
	}
	if a.tool == ToolText {
		if a.textTool.Active {
			a.finishTextEditing(true)
		}
		a.drawing = true
		a.preview = true
		a.startX, a.startY = ix, iy
		a.curX, a.curY = ix, iy
		a.textTool.Creating = true
		procSetCapture.Call(a.hwnd)
		a.statusText = "텍스트 상자: 원하는 영역을 끌어 지정한 뒤 입력하십시오."
		a.requestCanvasRepaint(true)
		return
	}
	if a.selection.Active {
		a.clearSelectionState()
	}
	a.drawing = true
	a.drawButtonRight = right
	a.startX, a.startY = ix, iy
	a.lastX, a.lastY = ix, iy
	a.curX, a.curY = ix, iy
	// The eraser keeps the A45 whole-stroke Shift behavior. Unified brush tools
	// enter and leave constrained mode dynamically inside onMouseMove.
	a.shiftStrokeActive = keyDown(VK_SHIFT) && a.tool == ToolEraser
	if a.shiftStrokeActive {
		a.shiftStrokeBasePix = a.canvas.ClonePixels()
		a.shiftStrokeAnchorX, a.shiftStrokeAnchorY = ix, iy
		a.shiftStrokePreviewX, a.shiftStrokePreviewY = ix, iy
	} else {
		a.shiftStrokeBasePix = nil
		a.shiftStrokeAnchorX, a.shiftStrokeAnchorY = 0, 0
		a.shiftStrokePreviewX, a.shiftStrokePreviewY = 0, 0
	}
	a.undo.Push(a.canvas)
	procSetCapture.Call(a.hwnd)
	col := a.fg
	if right {
		col = a.bg
	}
	switch a.tool {
	case ToolPencil, ToolBrush, ToolAirbrush:
		a.canvas.StampBrush(ix, iy, a.brushSize, a.brushShape, col)
	case ToolEraser:
		a.canvas.StampSquare(ix, iy, a.eraserSize, a.bg)
	case ToolLine, ToolRectangle, ToolEllipse, ToolRightTriangle, ToolArrow:
		a.preview = true
	}
	if !a.preview {
		a.markDirty()
	}
	logf("DRAW begin tool=%d image=%d,%d right=%v shift45=%v", a.tool, ix, iy, right, a.shiftStrokeActive)
	if a.tool == ToolLine || a.tool == ToolRightTriangle || a.tool == ToolArrow {
		a.requestRepaintRect(a.linePreviewScreenRect(a.curX, a.curY, keyDown(VK_SHIFT)), true)
	} else if a.tool == ToolEllipse {
		a.requestRepaintRect(a.ellipsePreviewScreenRect(a.curX, a.curY, keyDown(VK_SHIFT)), true)
	} else {
		a.requestCanvasRepaint(true)
	}
	if a.tool == ToolLine || a.tool == ToolRectangle || a.tool == ToolEllipse || a.tool == ToolRightTriangle || a.tool == ToolArrow {
		a.requestStatusRepaint()
	}
}

func (a *App) spray(ix, iy int, col uint32) {
	for i := 0; i < 24; i++ {
		x := a.rng.Intn(17) - 8
		y := a.rng.Intn(17) - 8
		if x*x+y*y <= 64 {
			a.canvas.SetPixel(ix+x, iy+y, col)
		}
	}
}

func (a *App) onMouseMove(mx, my int, wparam uintptr) {
	if a.textCommitClickPending {
		return
	}
	if a.pendingCanvasResize {
		if !a.ownsMouseCapture() {
			a.cancelCanvasResizeInteraction("capture owner changed before drag threshold", false)
			return
		}
		if wparam&MK_LBUTTON == 0 {
			a.cancelCanvasResizeInteraction("left button released before drag threshold", true)
			return
		}
		if canvasResizeDragActivated(a.pendingResizeStartMouseX, a.pendingResizeStartMouseY, mx, my) {
			h := a.pendingResizeHandle
			sx, sy := a.pendingResizeStartMouseX, a.pendingResizeStartMouseY
			a.beginCanvasResize(h, sx, sy)
			a.updateCanvasResize(mx, my)
		}
		return
	}
	if a.pendingRectSelection {
		if wparam&MK_LBUTTON == 0 {
			a.cancelPendingRectSelection("left button released before drag threshold", true)
			return
		}
		if rectSelectionDragActivated(a.pendingRectSelectionStartClientX, a.pendingRectSelectionStartClientY, mx, my) {
			a.activatePendingRectSelection(mx, my)
		}
		return
	}
	if a.thumbnailNavigationDragging {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		a.navigateThumbnailPoint(mx, my, rc, true, "drag")
		return
	}
	if a.clipboardScrollDragging {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		a.updateClipboardScrollDrag(my, rc)
		return
	}
	if a.historyScrollDragging {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		a.updateHistoryScrollDrag(my, rc)
		return
	}
	if a.clipboardPanel.Dragging || a.textPanel.Dragging || a.historyPanel.Dragging || a.thumbnailPanel.Dragging || a.toolboxPanel.Dragging || a.palettePanel.Dragging {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		oldClipStage := panelDockStage(&a.clipboardPanel)
		oldTextStage := panelDockStage(&a.textPanel)
		oldHistoryStage := panelDockStage(&a.historyPanel)
		oldThumbnailStage := panelDockStage(&a.thumbnailPanel)
		oldToolboxStage := panelDockStage(&a.toolboxPanel)
		oldPaletteStage := panelDockStage(&a.palettePanel)
		if a.clipboardPanel.Dragging {
			a.movePanel(&a.clipboardPanel, mx, my, rc)
		}
		if a.textPanel.Dragging {
			a.movePanel(&a.textPanel, mx, my, rc)
		}
		if a.historyPanel.Dragging {
			a.movePanel(&a.historyPanel, mx, my, rc)
		}
		if a.thumbnailPanel.Dragging {
			a.movePanel(&a.thumbnailPanel, mx, my, rc)
		}
		if a.toolboxPanel.Dragging {
			a.movePanel(&a.toolboxPanel, mx, my, rc)
		}
		if a.palettePanel.Dragging {
			a.movePanel(&a.palettePanel, mx, my, rc)
		}
		if oldClipStage != panelDockStage(&a.clipboardPanel) || oldTextStage != panelDockStage(&a.textPanel) || oldHistoryStage != panelDockStage(&a.historyPanel) || oldThumbnailStage != panelDockStage(&a.thumbnailPanel) || oldToolboxStage != panelDockStage(&a.toolboxPanel) || oldPaletteStage != panelDockStage(&a.palettePanel) {
			a.updateScrollbars()
		}
		a.updateTextUILayout(rc)
		invalidate(a.hwnd)
		return
	}
	if a.adjustingColorTolerance {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		_, tb, _, _ := a.layout(rc)
		localTB := RECT{0, 0, tb.Right - tb.Left, tb.Bottom - tb.Top}
		a.setColorTolerance(colorToleranceFromSliderY(my-int(tb.Top), localTB))
		invalidate(a.hwnd)
		return
	}
	if a.adjustingBrushSize {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		_, tb, _, _ := a.layout(rc)
		localTB := RECT{0, 0, tb.Right - tb.Left, tb.Bottom - tb.Top}
		a.setBrushSize(brushSizeFromSliderY(my-int(tb.Top), localTB))
		invalidate(a.hwnd)
		return
	}
	if a.textTool.Selecting {
		a.updateTextRangeSelection(mx, my)
		return
	}
	if a.textTool.DragMode != 0 {
		a.updateTextBoxDrag(mx, my)
		return
	}
	if a.adjustingEraserSize {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		_, tb, _, _ := a.layout(rc)
		localTB := RECT{0, 0, tb.Right - tb.Left, tb.Bottom - tb.Top}
		a.setEraserSize(eraserSizeFromSliderY(my-int(tb.Top), localTB))
		invalidate(a.hwnd)
		return
	}
	if a.adjustingLineSize {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		_, tb, _, _ := a.layout(rc)
		localTB := RECT{0, 0, tb.Right - tb.Left, tb.Bottom - tb.Top}
		a.setLineSize(lineSizeFromSliderY(my-int(tb.Top), localTB))
		invalidate(a.hwnd)
		return
	}
	if a.adjustingCurveBends {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		_, tb, _, _ := a.layout(rc)
		a.setCurveBendCount(curveBendCountFromSliderX(mx - int(tb.Left)))
		invalidate(a.hwnd)
		return
	}
	if a.adjustingCurveSize {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		_, tb, _, _ := a.layout(rc)
		localTB := RECT{0, 0, tb.Right - tb.Left, tb.Bottom - tb.Top}
		a.setCurveSize(curveSizeFromSliderY(my-int(tb.Top), localTB))
		invalidate(a.hwnd)
		return
	}
	if a.adjustingRectangleSize {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		_, tb, _, _ := a.layout(rc)
		localTB := RECT{0, 0, tb.Right - tb.Left, tb.Bottom - tb.Top}
		a.setRectangleSize(rectangleSizeFromSliderY(my-int(tb.Top), localTB))
		invalidate(a.hwnd)
		return
	}
	if a.adjustingEllipseSize {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		_, tb, _, _ := a.layout(rc)
		localTB := RECT{0, 0, tb.Right - tb.Left, tb.Bottom - tb.Top}
		a.setEllipseSize(ellipseSizeFromSliderY(my-int(tb.Top), localTB))
		invalidate(a.hwnd)
		return
	}
	if a.adjustingRightTriangleSize {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		_, tb, _, _ := a.layout(rc)
		localTB := RECT{0, 0, tb.Right - tb.Left, tb.Bottom - tb.Top}
		a.setRightTriangleSize(rightTriangleSizeFromSliderY(my-int(tb.Top), localTB))
		invalidate(a.hwnd)
		return
	}
	if a.adjustingArrowLineSize {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		_, tb, _, _ := a.layout(rc)
		localTB := RECT{0, 0, tb.Right - tb.Left, tb.Bottom - tb.Top}
		a.setArrowLineSize(arrowLineSizeFromSliderY(my-int(tb.Top), localTB))
		invalidate(a.hwnd)
		return
	}
	if a.resizingCanvas {
		a.updateCanvasResize(mx, my)
		return
	}
	oldCursorX, oldCursorY := a.cursorX, a.cursorY
	ix, iy, inside := a.clientToImage(mx, my)
	if inside {
		a.cursorX, a.cursorY = ix, iy
	} else {
		a.cursorX, a.cursorY = -1, -1
	}
	if a.selection.Creating {
		ix = clamp(ix, 0, a.canvas.W-1)
		iy = clamp(iy, 0, a.canvas.H-1)
		a.selection.CurrentX, a.selection.CurrentY = ix, iy
		if a.selection.Free {
			pts := a.selection.Lasso
			if len(pts) == 0 || pts[len(pts)-1].X != ix || pts[len(pts)-1].Y != iy {
				a.selection.Lasso = append(pts, PixelPoint{ix, iy})
			}
		}
		a.requestCanvasRepaint(true)
		a.requestStatusRepaint()
		return
	}
	if a.selection.Moving {
		a.updateSelectionMovePosition(ix, iy, keyDown(VK_SHIFT))
		a.requestStatusRepaint()
		return
	}
	if a.drawing {
		ix = clamp(ix, 0, a.canvas.W-1)
		iy = clamp(iy, 0, a.canvas.H-1)
		oldCurX, oldCurY := a.curX, a.curY
		a.curX, a.curY = ix, iy
		if a.tool == ToolCurve {
			oldCurveRect := a.curvePreviewScreenRect()
			switch a.curveStage {
			case 0:
				if a.curveBaseShiftConstrained {
					ix, iy = a.constrainedEightWayEndpoint(a.curveX0, a.curveY0, ix, iy)
					a.curX, a.curY = ix, iy
				}
				a.curveX1, a.curveY1 = ix, iy
			case 1:
				a.curveC1X, a.curveC1Y = ix, iy
			case 2:
				a.curveC2X, a.curveC2Y = ix, iy
			case 3:
				a.curveC3X, a.curveC3Y = ix, iy
			case 4:
				a.curveC4X, a.curveC4Y = ix, iy
			}
			newCurveRect := a.curvePreviewScreenRect()
			a.requestRepaintRect(unionRECT(oldCurveRect, newCurveRect), true)
			return
		}
		col := a.fg
		if a.drawButtonRight {
			col = a.bg
		}
		isBrushStroke := a.tool == ToolPencil || a.tool == ToolBrush || a.tool == ToolAirbrush
		shiftNow := keyDown(VK_SHIFT)
		if isBrushStroke {
			rawX, rawY := ix, iy
			if shiftNow {
				if !a.shiftStrokeActive {
					a.shiftStrokeActive = true
					a.shiftStrokeBasePix = a.canvas.ClonePixels()
					a.shiftStrokeAnchorX, a.shiftStrokeAnchorY = a.lastX, a.lastY
					a.shiftStrokePreviewX, a.shiftStrokePreviewY = a.lastX, a.lastY
					logf("BRUSH shift segment begin anchor=%d,%d raw=%d,%d", a.shiftStrokeAnchorX, a.shiftStrokeAnchorY, rawX, rawY)
				}
				if len(a.shiftStrokeBasePix) == len(a.canvas.Pix) {
					copy(a.canvas.Pix, a.shiftStrokeBasePix)
					nx, ny := a.constrainedEightWayEndpoint(a.shiftStrokeAnchorX, a.shiftStrokeAnchorY, rawX, rawY)
					a.canvas.DrawUnifiedBrushLine(a.shiftStrokeAnchorX, a.shiftStrokeAnchorY, nx, ny, a.brushSize, a.brushShape, col)
					a.shiftStrokePreviewX, a.shiftStrokePreviewY = nx, ny
					a.curX, a.curY = nx, ny
					n := atomic.AddUint64(&a.drawMoveCount, 1)
					if n <= 10 || n%100 == 0 || time.Since(a.lastDrawLog) > time.Second {
						a.lastDrawLog = time.Now()
						logf("BRUSH shift segment preview #%d anchor=%d,%d raw=%d,%d snapped=%d,%d", n, a.shiftStrokeAnchorX, a.shiftStrokeAnchorY, rawX, rawY, nx, ny)
					}
					a.requestCanvasRepaint(true)
					return
				}
			}
			if a.shiftStrokeActive && len(a.shiftStrokeBasePix) == len(a.canvas.Pix) {
				// Shift was released while the mouse button is still down. Freeze the
				// exact straight preview that was visible at release, then bridge from
				// that snapped endpoint to the current raw pointer and continue as
				// ordinary freehand drawing. Do not let post-release movement reshape
				// the straight segment.
				copy(a.canvas.Pix, a.shiftStrokeBasePix)
				nx, ny := a.shiftStrokePreviewX, a.shiftStrokePreviewY
				a.canvas.DrawUnifiedBrushLine(a.shiftStrokeAnchorX, a.shiftStrokeAnchorY, nx, ny, a.brushSize, a.brushShape, col)
				if nx != rawX || ny != rawY {
					a.canvas.DrawUnifiedBrushLine(nx, ny, rawX, rawY, a.brushSize, a.brushShape, col)
				}
				logf("BRUSH shift segment release anchor=%d,%d frozen=%d,%d resume=%d,%d", a.shiftStrokeAnchorX, a.shiftStrokeAnchorY, nx, ny, rawX, rawY)
				a.shiftStrokeActive = false
				a.shiftStrokeBasePix = nil
				a.shiftStrokeAnchorX, a.shiftStrokeAnchorY = 0, 0
				a.shiftStrokePreviewX, a.shiftStrokePreviewY = 0, 0
				a.lastX, a.lastY = rawX, rawY
				a.curX, a.curY = rawX, rawY
				a.requestCanvasRepaint(true)
				return
			}
		}
		if a.shiftStrokeActive && a.tool == ToolEraser && len(a.shiftStrokeBasePix) == len(a.canvas.Pix) {
			copy(a.canvas.Pix, a.shiftStrokeBasePix)
			ix, iy = a.constrainedEightWayEndpoint(a.shiftStrokeAnchorX, a.shiftStrokeAnchorY, ix, iy)
			a.curX, a.curY = ix, iy
			a.canvas.DrawEraserLine(a.shiftStrokeAnchorX, a.shiftStrokeAnchorY, ix, iy, a.eraserSize, a.bg)
			a.shiftStrokePreviewX, a.shiftStrokePreviewY = ix, iy
			n := atomic.AddUint64(&a.drawMoveCount, 1)
			if n <= 10 || n%100 == 0 || time.Since(a.lastDrawLog) > time.Second {
				a.lastDrawLog = time.Now()
				logf("ERASER shift45 move #%d image=%d,%d", n, ix, iy)
			}
			a.requestCanvasRepaint(true)
			return
		}
		switch a.tool {
		case ToolPencil, ToolBrush, ToolAirbrush:
			a.canvas.DrawUnifiedBrushLine(a.lastX, a.lastY, ix, iy, a.brushSize, a.brushShape, col)
		case ToolEraser:
			a.canvas.DrawEraserLine(a.lastX, a.lastY, ix, iy, a.eraserSize, a.bg)
		}
		a.lastX, a.lastY = ix, iy
		n := atomic.AddUint64(&a.drawMoveCount, 1)
		if n <= 10 || n%100 == 0 || time.Since(a.lastDrawLog) > time.Second {
			a.lastDrawLog = time.Now()
			logf("DRAW move #%d image=%d,%d tool=%d", n, ix, iy, a.tool)
		}
		if a.tool == ToolLine || a.tool == ToolRightTriangle || a.tool == ToolArrow {
			a.requestLinePreviewTransition(oldCurX, oldCurY, a.curX, a.curY, keyDown(VK_SHIFT), true)
			a.requestStatusRepaint()
			return
		}
		if a.tool == ToolEllipse {
			a.requestEllipsePreviewTransition(oldCurX, oldCurY, a.curX, a.curY, keyDown(VK_SHIFT), true)
			a.requestStatusRepaint()
			return
		}
		if a.tool == ToolRectangle {
			oldRect := a.rectanglePreviewScreenRect(oldCurX, oldCurY, keyDown(VK_SHIFT))
			newRect := a.rectanglePreviewScreenRect(a.curX, a.curY, keyDown(VK_SHIFT))
			a.requestRepaintRect(unionRECT(oldRect, newRect), true)
			a.requestStatusRepaint()
			return
		}
		a.requestCanvasRepaint(true)
		return
	}
	if a.tool == ToolMagnifier && !a.drawing {
		if a.updateZoomPreviewAt(mx, my) {
			if a.zoomPreviewValid {
				a.statusText = fmt.Sprintf("확대 대상 %dx: 점선 영역 (%d,%d) %d × %d", a.zoomTarget, a.zoomPreviewX, a.zoomPreviewY, a.zoomPreviewW, a.zoomPreviewH)
			}
			a.requestCanvasRepaint(true)
		}
	}
	if (a.tool == ToolEraser || a.tool == ToolPencil || a.tool == ToolBrush || a.tool == ToolAirbrush) && (oldCursorX != a.cursorX || oldCursorY != a.cursorY) {
		a.requestCanvasRepaint(false)
	}
	if a.showStatus && !a.bitmapView && (oldCursorX != a.cursorX || oldCursorY != a.cursorY) {
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		_, _, _, st := a.layout(rc)
		invalidateArea(a.hwnd, &st)
	}
}

func (a *App) onMouseUp(mx, my int, right bool) {
	logf("MOUSEUP client=%d,%d right=%v drawing=%v tool=%d", mx, my, right, a.drawing, a.tool)
	if a.pendingCanvasResize {
		a.pendingCanvasResize = false
		a.releaseCanvasResizeCapture("armed click")
		a.statusText = fmt.Sprintf("캔버스 크기: %d × %d", a.canvas.W, a.canvas.H)
		logf("CANVAS_RESIZE armed-click ignored client=%d,%d", mx, my)
		invalidate(a.hwnd)
		return
	}
	if a.pendingRectSelection {
		outside := a.pendingRectSelectionOutsideCanvas
		a.cancelPendingRectSelection("click without drag", true)
		a.statusText = "선택을 취소했습니다."
		logf("SELECTION rect click-cancel client=%d,%d outside=%v", mx, my, outside)
		a.requestCanvasRepaint(true)
		return
	}
	if a.textCommitClickPending {
		a.textCommitClickPending = false
		a.drawing = false
		a.preview = false
		a.textTool.Creating = false
		procReleaseCapture.Call()
		logf("TEXT outside commit click fully consumed; next click may create text box")
		a.requestCanvasRepaint(true)
		return
	}
	if a.thumbnailNavigationDragging {
		a.thumbnailNavigationDragging = false
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		a.navigateThumbnailPoint(mx, my, rc, true, "drag-end")
		procReleaseCapture.Call()
		logf("THUMBNAIL drag end client=%d,%d scroll=%d,%d", mx, my, a.scrollX, a.scrollY)
		return
	}
	if a.clipboardScrollDragging {
		a.clipboardScrollDragging = false
		procReleaseCapture.Call()
		invalidate(a.hwnd)
		return
	}
	if a.historyScrollDragging {
		a.historyScrollDragging = false
		procReleaseCapture.Call()
		a.requestHistoryRepaint(true)
		return
	}
	if a.clipboardPanel.Dragging || a.textPanel.Dragging || a.historyPanel.Dragging || a.thumbnailPanel.Dragging || a.toolboxPanel.Dragging || a.palettePanel.Dragging {
		clipWas := a.clipboardPanel.Dragging
		textWas := a.textPanel.Dragging
		historyWas := a.historyPanel.Dragging
		thumbnailWas := a.thumbnailPanel.Dragging
		toolboxWas := a.toolboxPanel.Dragging
		paletteWas := a.palettePanel.Dragging
		a.clipboardPanel.Dragging = false
		a.textPanel.Dragging = false
		a.historyPanel.Dragging = false
		a.thumbnailPanel.Dragging = false
		a.toolboxPanel.Dragging = false
		a.palettePanel.Dragging = false
		procReleaseCapture.Call()
		var rc RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
		a.updateTextUILayout(rc)
		a.updateScrollbars()
		logf("PANEL drag end clipboard=%v stage=%d order=%d text=%v stage=%d order=%d history=%v stage=%d order=%d thumbnail=%v stage=%d order=%d toolbox=%v stage=%d palette=%v stage=%d snap=%d top=%d gap=%d", clipWas, panelDockStage(&a.clipboardPanel), a.clipboardPanel.DockOrder, textWas, panelDockStage(&a.textPanel), a.textPanel.DockOrder, historyWas, panelDockStage(&a.historyPanel), a.historyPanel.DockOrder, thumbnailWas, panelDockStage(&a.thumbnailPanel), a.thumbnailPanel.DockOrder, toolboxWas, panelDockStage(&a.toolboxPanel), paletteWas, panelDockStage(&a.palettePanel), panelDockSnapDistance, panelDockTopOffset, panelDockGap)
		invalidate(a.hwnd)
		return
	}
	if a.adjustingColorTolerance {
		a.adjustingColorTolerance = false
		procReleaseCapture.Call()
		a.statusText = fmt.Sprintf("허용 불일치를 %d로 설정했습니다. 선택과 색 채우기에 적용되며 0은 완전히 같은 색만 처리합니다.", a.colorTolerance)
		logf("COLOR_TOLERANCE slider-commit value=%d tool=%d", a.colorTolerance, a.tool)
		invalidate(a.hwnd)
		return
	}
	if a.adjustingBrushSize {
		a.adjustingBrushSize = false
		procReleaseCapture.Call()
		a.statusText = fmt.Sprintf("붓 크기를 %d px로 설정했습니다. 모양: %s.", a.brushSize, brushShapeNames[a.brushShape])
		logf("BRUSH slider commit size=%d shape=%d", a.brushSize, a.brushShape)
		invalidate(a.hwnd)
		return
	}
	if a.adjustingEraserSize {
		a.adjustingEraserSize = false
		procReleaseCapture.Call()
		a.statusText = fmt.Sprintf("지우개 크기를 %d × %d 픽셀로 설정했습니다.", a.eraserSize, a.eraserSize)
		logf("ERASER slider commit size=%d", a.eraserSize)
		invalidate(a.hwnd)
		return
	}
	if a.adjustingLineSize {
		a.adjustingLineSize = false
		procReleaseCapture.Call()
		a.statusText = fmt.Sprintf("직선 굵기를 %d px로 설정했습니다.", a.lineSize)
		logf("LINE slider commit size=%d", a.lineSize)
		invalidate(a.hwnd)
		return
	}
	if a.adjustingCurveBends {
		a.adjustingCurveBends = false
		procReleaseCapture.Call()
		a.statusText = fmt.Sprintf("곡선 굽힘점을 %d개로 설정했습니다. 2개가 기본 그림판 방식입니다.", a.curveBendCount)
		logf("CURVE bend-slider commit count=%d", a.curveBendCount)
		invalidate(a.hwnd)
		return
	}
	if a.adjustingCurveSize {
		a.adjustingCurveSize = false
		procReleaseCapture.Call()
		a.statusText = fmt.Sprintf("곡선 굵기를 %d px로 설정했습니다.", a.curveSize)
		logf("CURVE slider commit size=%d", a.curveSize)
		invalidate(a.hwnd)
		return
	}
	if a.adjustingRectangleSize {
		a.adjustingRectangleSize = false
		procReleaseCapture.Call()
		a.statusText = fmt.Sprintf("사각형 선 굵기를 %d px로 설정했습니다.", a.rectangleSize)
		logf("RECTANGLE slider commit size=%d", a.rectangleSize)
		invalidate(a.hwnd)
		return
	}
	if a.adjustingEllipseSize {
		a.adjustingEllipseSize = false
		procReleaseCapture.Call()
		a.statusText = fmt.Sprintf("타원 선 굵기를 %d px로 설정했습니다.", a.ellipseSize)
		logf("ELLIPSE slider commit size=%d", a.ellipseSize)
		invalidate(a.hwnd)
		return
	}
	if a.adjustingRightTriangleSize {
		a.adjustingRightTriangleSize = false
		procReleaseCapture.Call()
		a.statusText = fmt.Sprintf("직각삼각형 선 굵기를 %d px로 설정했습니다.", a.rightTriangleSize)
		logf("RIGHT_TRIANGLE slider commit size=%d", a.rightTriangleSize)
		invalidate(a.hwnd)
		return
	}
	if a.adjustingArrowLineSize {
		a.adjustingArrowLineSize = false
		procReleaseCapture.Call()
		a.statusText = fmt.Sprintf("화살표 선 굵기를 %d px로 설정했습니다.", a.arrowLineSize)
		logf("ARROW slider commit size=%d", a.arrowLineSize)
		invalidate(a.hwnd)
		return
	}
	if a.textTool.Selecting {
		a.endTextRangeSelection(mx, my)
		return
	}
	if a.textTool.DragMode != 0 {
		a.endTextBoxDrag()
		a.requestCanvasRepaint(true)
		return
	}
	if a.resizingCanvas {
		a.finishCanvasResize(false)
		return
	}
	if a.selection.Creating {
		ix, iy, _ := a.clientToImage(mx, my)
		ix = clamp(ix, 0, a.canvas.W-1)
		iy = clamp(iy, 0, a.canvas.H-1)
		s := &a.selection
		s.CurrentX, s.CurrentY = ix, iy
		s.Creating = false
		procReleaseCapture.Call()

		newX, newY, newW, newH := 0, 0, 0, 0
		var newMask []bool
		newFree := s.Free
		newCount := 0
		if s.Free {
			if len(s.Lasso) < 3 {
				if s.Adding && s.AddBaseW > 0 && s.AddBaseH > 0 {
					s.X, s.Y, s.W, s.H = s.AddBaseX, s.AddBaseY, s.AddBaseW, s.AddBaseH
					s.Mask = append([]bool(nil), s.AddBaseMask...)
					s.Free = s.AddBaseFree
					s.Active = true
					s.Adding = false
					s.AddBaseMask = nil
				} else {
					s.Reset()
				}
				invalidate(a.hwnd)
				return
			}
			minX, maxX, minY, maxY := s.Lasso[0].X, s.Lasso[0].X, s.Lasso[0].Y, s.Lasso[0].Y
			for _, p := range s.Lasso {
				if p.X < minX {
					minX = p.X
				}
				if p.X > maxX {
					maxX = p.X
				}
				if p.Y < minY {
					minY = p.Y
				}
				if p.Y > maxY {
					maxY = p.Y
				}
			}
			minX = clamp(minX, 0, a.canvas.W-1)
			maxX = clamp(maxX, 0, a.canvas.W-1)
			minY = clamp(minY, 0, a.canvas.H-1)
			maxY = clamp(maxY, 0, a.canvas.H-1)
			newX, newY, newW, newH = minX, minY, maxX-minX+1, maxY-minY+1
			newMask = make([]bool, newW*newH)
			for y := 0; y < newH; y++ {
				for x := 0; x < newW; x++ {
					// Test pixel centers so the selected mask follows the actual lasso,
					// rather than merely selecting its bounding rectangle.
					if pointInPolygon(float64(newX+x)+0.5, float64(newY+y)+0.5, s.Lasso) {
						newMask[y*newW+x] = true
						newCount++
					}
				}
			}
			if newCount == 0 {
				if s.Adding && s.AddBaseW > 0 && s.AddBaseH > 0 {
					s.X, s.Y, s.W, s.H = s.AddBaseX, s.AddBaseY, s.AddBaseW, s.AddBaseH
					s.Mask = append([]bool(nil), s.AddBaseMask...)
					s.Free = s.AddBaseFree
					s.Active = true
				} else {
					s.Reset()
				}
				invalidate(a.hwnd)
				return
			}
		} else {
			newX, newY, newW, newH = normalizeSelectionRect(s.StartX, s.StartY, ix, iy)
			newCount = newW * newH
		}

		adding := s.Adding && s.AddBaseW > 0 && s.AddBaseH > 0
		if adding {
			ux, uy, uw, uh, umask, ucount := unionSelectionRegions(s.AddBaseX, s.AddBaseY, s.AddBaseW, s.AddBaseH, s.AddBaseMask, newX, newY, newW, newH, newMask)
			s.X, s.Y, s.W, s.H = ux, uy, uw, uh
			s.Mask = umask
			s.Free = len(umask) == uw*uh || s.AddBaseFree || newFree
			s.Active = ucount > 0
			s.Lasso = nil
			s.Adding = false
			s.AddBaseMask = nil
			s.Pix, s.BasePix = nil, nil
			a.statusText = fmt.Sprintf("선택 영역 추가 완료: %d픽셀. Shift를 누른 채 다시 그리면 더 추가할 수 있습니다.", ucount)
			logf("SELECTION add commit x=%d y=%d w=%d h=%d pixels=%d mask=%v", s.X, s.Y, s.W, s.H, ucount, len(s.Mask) == s.W*s.H)
		} else {
			s.X, s.Y, s.W, s.H = newX, newY, newW, newH
			s.Mask = newMask
			s.Free = newFree
			s.Active = true
			s.Lasso = nil
			if newFree {
				a.statusText = fmt.Sprintf("자유 선택: 실제 그린 영역 %d픽셀. Shift를 누른 채 그리면 영역을 추가합니다.", newCount)
				logf("SELECTION free create x=%d y=%d w=%d h=%d pixels=%d", s.X, s.Y, s.W, s.H, newCount)
			} else {
				a.statusText = fmt.Sprintf("사각 선택: %d × %d. Shift를 누른 채 그리면 영역을 추가합니다.", s.W, s.H)
				logf("SELECTION rect create x=%d y=%d w=%d h=%d", s.X, s.Y, s.W, s.H)
			}
		}
		a.requestCanvasRepaint(true)
		a.requestStatusRepaint()
		return
	}
	if a.selection.Moving {
		shifted := keyDown(VK_SHIFT)
		ix, iy, _ := a.clientToImage(mx, my)
		a.updateSelectionMovePosition(ix, iy, shifted)
		dx, dy := a.selection.X-a.selection.MoveStartX, a.selection.Y-a.selection.MoveStartY
		a.selection.Moving = false
		procReleaseCapture.Call()
		a.statusText = fmt.Sprintf("선택 영역: %d × %d  이동 [%d,%d]", a.selection.W, a.selection.H, dx, dy)
		if dx != 0 || dy != 0 {
			a.markDirty()
			a.recordHistory("선택 이동")
		}
		logf("SELECTION move end x=%d y=%d delta=%d,%d shift=%v", a.selection.X, a.selection.Y, dx, dy, shifted)
		a.requestCanvasRepaint(true)
		a.requestStatusRepaint()
		return
	}
	if !a.drawing {
		return
	}
	ix, iy, _ := a.clientToImage(mx, my)
	ix = clamp(ix, 0, a.canvas.W-1)
	iy = clamp(iy, 0, a.canvas.H-1)
	if a.tool == ToolText {
		a.drawing = false
		a.preview = false
		a.textTool.Creating = false
		procReleaseCapture.Call()
		x, y, w, h := normalizeSelectionRect(a.startX, a.startY, ix, iy)
		if w < 8 {
			w = 120 / max(a.zoom, 1)
		}
		if h < 8 {
			h = 48 / max(a.zoom, 1)
		}
		if x+w > a.canvas.W {
			w = a.canvas.W - x
		}
		if y+h > a.canvas.H {
			h = a.canvas.H - y
		}
		if w > 0 && h > 0 {
			a.beginTextEditing(x, y, w, h)
		}
		a.requestCanvasRepaint(true)
		return
	}
	if a.tool == ToolCurve {
		stage := a.curveStage
		oldCurveRect := a.curvePreviewScreenRect()
		procReleaseCapture.Call()
		if stage == 0 {
			if a.curveBaseShiftConstrained {
				ix, iy = a.constrainedEightWayEndpoint(a.curveX0, a.curveY0, ix, iy)
			}
			a.curveX1, a.curveY1 = ix, iy
			a.curveBaseShiftConstrained = false
			a.drawing = false
			a.curveStage = 1
			a.statusText = a.curveInstruction()
			logf("CURVE base end start=%d,%d end=%d,%d next-stage=1 active-controls=0", a.curveX0, a.curveY0, a.curveX1, a.curveY1)
		} else {
			switch stage {
			case 1:
				a.curveC1X, a.curveC1Y = ix, iy
			case 2:
				a.curveC2X, a.curveC2Y = ix, iy
			case 3:
				a.curveC3X, a.curveC3Y = ix, iy
			case 4:
				a.curveC4X, a.curveC4Y = ix, iy
			}
			a.drawing = false
			if stage >= a.curveBendCount {
				col := a.fg
				if a.drawButtonRight {
					col = a.bg
				}
				a.drawRasterCurve(a.canvas, a.curveBendCount, col)
				a.curveStage = 0
				a.curveUndoPending = false
				a.curveBaseShiftConstrained = false
				a.preview = false
				a.markDirty()
				a.recordHistory("곡선")
				a.statusText = a.curveInstruction()
				logf("CURVE commit bends=%d p0=%d,%d c1=%d,%d c2=%d,%d c3=%d,%d c4=%d,%d p1=%d,%d size=%d aa=%v", a.curveBendCount, a.curveX0, a.curveY0, a.curveC1X, a.curveC1Y, a.curveC2X, a.curveC2Y, a.curveC3X, a.curveC3Y, a.curveC4X, a.curveC4Y, a.curveX1, a.curveY1, a.curveSize, a.curveAntialias)
			} else {
				a.curveStage = stage + 1
				a.statusText = a.curveInstruction()
				logf("CURVE bend%d end control=%d,%d next-stage=%d active-controls=%d", stage, ix, iy, a.curveStage, stage)
			}
		}
		// Mouse-up must leave the accepted curve visible immediately.  The live
		// preview path may be coalesced, but stage boundaries are synchronously
		// latched so the next click never starts from a blank canvas.
		reason := "base-end"
		if stage > 0 {
			if stage >= a.curveBendCount {
				reason = "commit"
			} else {
				reason = fmt.Sprintf("bend%d-end", stage)
			}
		}
		a.forceCurveReleaseRepaint(oldCurveRect, reason)
		return
	}
	a.curX, a.curY = ix, iy
	col := a.fg
	if a.drawButtonRight {
		col = a.bg
	}
	if a.shiftStrokeActive && len(a.shiftStrokeBasePix) == len(a.canvas.Pix) {
		copy(a.canvas.Pix, a.shiftStrokeBasePix)
		anchorX, anchorY := a.shiftStrokeAnchorX, a.shiftStrokeAnchorY
		if a.tool == ToolPencil || a.tool == ToolBrush || a.tool == ToolAirbrush {
			if keyDown(VK_SHIFT) {
				ix, iy = a.constrainedEightWayEndpoint(anchorX, anchorY, ix, iy)
			} else {
				ix, iy = a.shiftStrokePreviewX, a.shiftStrokePreviewY
			}
		} else {
			ix, iy = a.constrainedEightWayEndpoint(anchorX, anchorY, ix, iy)
		}
		switch a.tool {
		case ToolPencil, ToolBrush, ToolAirbrush:
			a.canvas.DrawUnifiedBrushLine(anchorX, anchorY, ix, iy, a.brushSize, a.brushShape, col)
			logf("BRUSH shift segment mouseup anchor=%d,%d end=%d,%d shiftDown=%v", anchorX, anchorY, ix, iy, keyDown(VK_SHIFT))
		case ToolEraser:
			a.canvas.DrawEraserLine(anchorX, anchorY, ix, iy, a.eraserSize, a.bg)
			logf("ERASER shift45 commit start=%d,%d end=%d,%d", anchorX, anchorY, ix, iy)
		}
		a.curX, a.curY = ix, iy
	}
	a.shiftStrokeActive = false
	a.shiftStrokeBasePix = nil
	a.shiftStrokeAnchorX, a.shiftStrokeAnchorY = 0, 0
	a.shiftStrokePreviewX, a.shiftStrokePreviewY = 0, 0
	if a.preview {
		switch a.tool {
		case ToolLine:
			ix, iy = a.drawRasterLine(a.canvas, a.startX, a.startY, ix, iy, keyDown(VK_SHIFT), col)
		case ToolRectangle:
			ix, iy = a.drawRasterRectangle(a.canvas, a.startX, a.startY, ix, iy, keyDown(VK_SHIFT), col)
		case ToolEllipse:
			ix, iy = a.drawRasterEllipse(a.canvas, a.startX, a.startY, ix, iy, keyDown(VK_SHIFT), col)
		case ToolRightTriangle:
			ix, iy = a.drawRasterRightTriangle(a.canvas, a.startX, a.startY, ix, iy, keyDown(VK_SHIFT), col)
		case ToolArrow:
			ix, iy = a.drawRasterArrow(a.canvas, a.startX, a.startY, ix, iy, keyDown(VK_SHIFT), col)
		}
		a.markDirty()
	}
	a.drawing = false
	a.preview = false
	procReleaseCapture.Call()
	if a.tool == ToolRightTriangle {
		dx, dy := ix-a.startX, iy-a.startY
		quadrant := "우하"
		switch {
		case dx < 0 && dy >= 0:
			quadrant = "좌하"
		case dx >= 0 && dy < 0:
			quadrant = "우상"
		case dx < 0 && dy < 0:
			quadrant = "좌상"
		}
		cornerX, cornerY := rightTriangleLegCorner(a.startX, a.startY, ix, iy)
		logf("RIGHT_TRIANGLE commit hypotenuse=%d,%d->%d,%d rightAngle=%d,%d delta=%d,%d quadrant=%s size=%d shift=%v constraint=%d", a.startX, a.startY, ix, iy, cornerX, cornerY, dx, dy, quadrant, a.rightTriangleSize, keyDown(VK_SHIFT), a.lineConstraintMode)
	}
	if a.tool == ToolArrow {
		logf("ARROW commit tail=%d,%d tip=%d,%d delta=%d,%d head=%d line=%d aa=%v shift=%v constraint=%d", a.startX, a.startY, ix, iy, ix-a.startX, iy-a.startY, a.arrowHeadSize, a.arrowLineSize, a.arrowAntialias, keyDown(VK_SHIFT), a.lineConstraintMode)
	}
	a.recordHistory(a.drawHistoryName(a.tool))
	logf("DRAW end tool=%d start=%d,%d end=%d,%d", a.tool, a.startX, a.startY, ix, iy)
	a.requestCanvasRepaint(true)
	a.requestStatusRepaint()
}

func (a *App) handleScroll(bar int, code int, track int) {
	var rc RECT
	procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	ws, _, _, _ := a.layout(rc)
	if bar == SB_HORZ {
		page := int(ws.Right-ws.Left) - 16
		maxPos := max(a.canvas.W*a.zoom-page, 0)
		pos := a.scrollX
		switch code {
		case SB_LINEUP:
			pos -= 20
		case SB_LINEDOWN:
			pos += 20
		case SB_PAGEUP:
			pos -= page
		case SB_PAGEDOWN:
			pos += page
		case SB_THUMBPOSITION, SB_THUMBTRACK:
			pos = track
		case SB_TOP:
			pos = 0
		case SB_BOTTOM:
			pos = maxPos
		}
		a.scrollX = clamp(pos, 0, maxPos)
	} else {
		page := int(ws.Bottom-ws.Top) - 16
		maxPos := max(a.canvas.H*a.zoom-page, 0)
		pos := a.scrollY
		switch code {
		case SB_LINEUP:
			pos -= 20
		case SB_LINEDOWN:
			pos += 20
		case SB_PAGEUP:
			pos -= page
		case SB_PAGEDOWN:
			pos += page
		case SB_THUMBPOSITION, SB_THUMBTRACK:
			pos = track
		case SB_TOP:
			pos = 0
		case SB_BOTTOM:
			pos = maxPos
		}
		a.scrollY = clamp(pos, 0, maxPos)
	}
	a.updateScrollbars()
	a.updateTextUILayout(rc)
	invalidate(a.hwnd)
}

// --- Commands --------------------------------------------------------------
func (a *App) toggleMenuCheck(id int, on bool) {
	flags := uint32(MF_BYCOMMAND)
	if on {
		flags |= MF_CHECKED
	}
	target := a.viewMenu
	if id == ID_IMAGE_OPAQUE {
		target = a.imageMenu
	}
	procCheckMenuItem.Call(target, uintptr(id), uintptr(flags))
}

func (a *App) showZoomPopup() {
	m, _, _ := procCreatePopupMenu.Call()
	if m == 0 {
		return
	}
	defer procDestroyMenu.Call(m)
	a.applyDarkMenuBackground(m)
	for _, z := range a.zoomLevels {
		flags := uint32(MF_STRING)
		if z == a.zoom {
			flags |= MF_CHECKED
		}
		appendItem(m, flags, ID_ZOOM_BASE+z, fmt.Sprintf("%d%%  (%dx)", z*100, z))
	}
	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	cmd, _, _ := procTrackPopupMenu.Call(m, TPM_LEFTALIGN|TPM_TOPALIGN|TPM_RETURNCMD|TPM_NONOTIFY, uintptr(pt.X), uintptr(pt.Y), 0, a.hwnd, 0)
	if cmd >= ID_ZOOM_BASE+1 && cmd <= ID_ZOOM_BASE+15 {
		z := int(cmd) - ID_ZOOM_BASE
		if z < a.zoom {
			a.setZoomTopLeft(z)
		} else {
			a.setZoom(z, -1, -1)
		}
	}
}

func (a *App) showFlipPopup() {
	a.showFlipRotateDialog()
}

func isSelectionScopedImageTransformCommand(id int) bool {
	switch id {
	case ID_IMAGE_FLIP, ID_IMAGE_STRETCH, ID_IMAGE_INVERT:
		return true
	}
	return false
}

// commitActiveTextAsTransformSelection rasterizes the current text box, closes
// the editor, and immediately selects the exact text-box rectangle. The image
// transform requested by the same command can then operate on the text just as
// it would on any other selection. Empty text boxes are simply closed.
func (a *App) commitActiveTextAsTransformSelection() bool {
	if !a.textTool.Active && !a.textTool.Editing {
		return false
	}
	a.syncTextModelFromInput()
	textValue := a.textTool.Text
	hasText := strings.TrimSpace(textValue) != ""
	x0, y0 := a.textTool.X, a.textTool.Y
	x1, y1 := a.textTool.X+a.textTool.W, a.textTool.Y+a.textTool.H
	x0 = clamp(x0, 0, a.canvas.W)
	y0 = clamp(y0, 0, a.canvas.H)
	x1 = clamp(x1, 0, a.canvas.W)
	y1 = clamp(y1, 0, a.canvas.H)

	// Render a second copy onto a transparent layer before committing. This
	// separates the text itself from artwork that already existed underneath
	// the text box, so later rotation/stretch/inversion transforms only text.
	baseBeforeText := a.canvas.ClonePixels()
	var textLayer *Canvas
	if hasText && x1 > x0 && y1 > y0 {
		fullLayer := NewCanvas(a.canvas.W, a.canvas.H, transparentColor)
		a.renderTextToTarget(fullLayer, textValue, false)
		textLayer = NewCanvas(x1-x0, y1-y0, transparentColor)
		for row := 0; row < textLayer.H; row++ {
			src := ((y0+row)*fullLayer.W + x0) * 4
			dst := row * textLayer.W * 4
			copy(textLayer.Pix[dst:dst+textLayer.W*4], fullLayer.Pix[src:src+textLayer.W*4])
		}
	}

	a.finishTextEditing(true)
	if !hasText {
		a.statusText = "빈 텍스트 상자를 닫았습니다. 변환할 텍스트가 없습니다."
		return false
	}
	if textLayer == nil || textLayer.W < 1 || textLayer.H < 1 {
		a.statusText = "텍스트 상자가 캔버스 밖에 있어 변환할 수 없습니다."
		return false
	}

	// Replace the committed rectangle with an equivalent floating text-only
	// layer over the exact pre-text canvas. The visual result remains the same,
	// while the requested image command now has a clean selection target.
	copy(a.canvas.Pix, baseBeforeText)
	a.clearSelectionState()
	a.selection = SelectionState{
		Active: true, Floating: true,
		X: x0, Y: y0, W: textLayer.W, H: textLayer.H,
		Pix: textLayer.ClonePixels(), BasePix: append([]byte(nil), baseBeforeText...),
	}
	oldOpaque := a.opaque
	a.opaque = true
	a.applyFloatingSelection(x0, y0)
	a.opaque = oldOpaque
	a.tool = ToolRectSelect
	a.statusText = fmt.Sprintf("텍스트를 그림에 확정하고 텍스트만 %d × %d 선택 영역으로 전환했습니다.", textLayer.W, textLayer.H)
	logf("TEXT transform selection x=%d y=%d w=%d h=%d textOnly=true", x0, y0, textLayer.W, textLayer.H)
	invalidate(a.hwnd)
	return true
}

func (a *App) resizeActiveSelectionNearest(nw, nh int) bool {
	if !a.selection.Active {
		return false
	}
	nw, nh = max(nw, 1), max(nh, 1)
	source := a.selectionTransformSource()
	if source == nil {
		return false
	}
	oldX, oldY := a.selection.X, a.selection.Y
	source.ResizeNearest(nw, nh, transparentColor)
	status := fmt.Sprintf("선택 영역을 %d × %d로 늘이기/축소했습니다. 빈 부분은 투명색입니다.", nw, nh)
	if !a.installSelectionTransform(source, oldX, oldY, "선택 크기 조정", status) {
		return false
	}
	logf("STRETCH selection result=%dx%d anchor=%d,%d", nw, nh, oldX, oldY)
	return true
}

func invertBGRASelectionPixels(pix []byte, mask []bool, w, h int) {
	for i := 0; i < w*h; i++ {
		if len(mask) == w*h && !mask[i] {
			continue
		}
		off := i * 4
		if off+3 >= len(pix) || pix[off+3] == 0 {
			continue
		}
		pix[off], pix[off+1], pix[off+2] = 255-pix[off], 255-pix[off+1], 255-pix[off+2]
	}
}

func (a *App) invertActiveSelection() bool {
	s := &a.selection
	if !s.Active || s.W < 1 || s.H < 1 {
		return false
	}
	a.undo.Push(a.canvas)
	if s.Floating && len(s.Pix) == s.W*s.H*4 {
		invertBGRASelectionPixels(s.Pix, s.Mask, s.W, s.H)
		a.applyFloatingSelection(s.X, s.Y)
	} else {
		for y := 0; y < s.H; y++ {
			for x := 0; x < s.W; x++ {
				i := y*s.W + x
				if len(s.Mask) == s.W*s.H && !s.Mask[i] {
					continue
				}
				px, py := s.X+x, s.Y+y
				if px < 0 || py < 0 || px >= a.canvas.W || py >= a.canvas.H {
					continue
				}
				off := (py*a.canvas.W + px) * 4
				if a.canvas.Pix[off+3] == 0 {
					continue
				}
				a.canvas.Pix[off], a.canvas.Pix[off+1], a.canvas.Pix[off+2] = 255-a.canvas.Pix[off], 255-a.canvas.Pix[off+1], 255-a.canvas.Pix[off+2]
			}
		}
	}
	a.markDirty()
	a.recordHistory("선택 색 반전")
	a.statusText = fmt.Sprintf("선택 영역 %d × %d의 색을 반전했습니다.", s.W, s.H)
	logf("INVERT selection floating=%v x=%d y=%d w=%d h=%d", s.Floating, s.X, s.Y, s.W, s.H)
	a.requestCanvasRepaint(true)
	return true
}

func (a *App) legacyEditColorTarget(background bool) {
	initial := a.fg
	targetName := "전경색"
	if background {
		initial = a.bg
		targetName = "배경색"
	}
	if isTransparentColor(initial) {
		initial = rgb(0, 0, 0)
	}
	before := a.customColors
	footer := &colorPaletteFooter{palette: a.currentIndexedColorTable(), mode: a.indexedPaletteActive(), selectedIndex: -1}
	activeColorPaletteFooter = footer
	defer func() { activeColorPaletteFooter = nil }()
	cc := CHOOSECOLORW{
		LStructSize:  uint32(unsafe.Sizeof(CHOOSECOLORW{})),
		HwndOwner:    a.hwnd,
		RgbResult:    initial,
		LpCustColors: &a.customColors[0],
		Flags:        CC_RGBINIT | CC_FULLOPEN | CC_ENABLEHOOK,
		LCustData:    1,
		LpfnHook:     colorDialogHookCallback,
	}
	r, _, _ := procChooseColorW.Call(uintptr(unsafe.Pointer(&cc)))
	customChanged := before != a.customColors
	if r != 0 {
		if a.dejavu == nil {
			requested := 0
			if footer.mode {
				requested = len(footer.palette)
			}
			if (a.indexedMode && requested == 0) || (requested != 0 && (!a.indexedMode || a.indexedSize != requested)) {
				a.setIndexedPaletteMode(requested)
			}
		}
		if footer.editIndex && footer.selectedIndex >= 0 && footer.selectedIndex < len(footer.palette) {
			footer.palette[footer.selectedIndex] = cc.RgbResult & 0xffffff
			footer.changed = true
		}
		if footer.changed && footer.mode {
			if err := a.applyIndexedColorTable(footer.palette, ""); err != nil {
				messageBox(a.hwnd, err.Error(), "인덱스 팔레트", MB_OK|MB_ICONERROR)
			}
		}

		chosenColor := cc.RgbResult
		if a.indexedPaletteActive() {
			// Quantize against the *active* 16- or 256-color palette.
			idx := nearestIndexedColor(byte(cc.RgbResult), byte(cc.RgbResult>>8), byte(cc.RgbResult>>16), a.palette)
			if footer.editIndex {
				idx = footer.selectedIndex
			}
			chosenColor = a.palette[idx]
		}
		if background {
			a.bg = chosenColor
		} else {
			a.fg = chosenColor
		}
		a.statusText = fmt.Sprintf("%s을 #%02X%02X%02X로 설정했습니다.", targetName, byte(chosenColor), byte(chosenColor>>8), byte(chosenColor>>16))
	}
	if r != 0 || customChanged {
		logf("PALETTE edit-color target=%s accepted=%v customChanged=%v color=%#08x", targetName, r != 0, customChanged, cc.RgbResult)
		invalidate(a.hwnd)
		procUpdateWindow.Call(a.hwnd)
	}
}

func (a *App) editColor() {
	a.editColorTarget(false)
}

func (a *App) command(id int) {
	logf("COMMAND id=%d tool=%d zoom=%d dirty=%v", id, a.tool, a.zoom, a.dirty)
	if a.tool == ToolText && a.textTool.Active {
		if isSelectionScopedImageTransformCommand(id) {
			if !a.commitActiveTextAsTransformSelection() {
				invalidate(a.hwnd)
				return
			}
		} else {
			switch id {
			case ID_VIEW_TOOLBOX, ID_VIEW_PALETTE, ID_VIEW_STATUS, ID_VIEW_CLIPBOARD, ID_VIEW_HISTORY, ID_VIEW_GRID, ID_VIEW_THUMBNAIL, ID_VIEW_DARKMODE, ID_VIEW_LANGUAGE_KO, ID_VIEW_LANGUAGE_EN, ID_VIEW_ZOOM_NORMAL, ID_VIEW_ZOOM_LARGE, ID_INSERT_FORMULA, ID_INSERT_FUNCTION_GRAPH, ID_INSERT_IMAGE_CIRCUIT, ID_INSERT_IMAGE_ANIMAL, ID_INSERT_IMAGE_CUSTOM, ID_INSERT_IMAGE_MULTIDIM, ID_INSERT_IMAGE_FIELD, ID_HELP_TOPICS, ID_HELP_LOG, ID_HELP_ABOUT:
			default:
				a.finishTextEditing(true)
			}
		}
	}
	if id >= ID_RECENT_BASE && id < ID_RECENT_BASE+8 {
		idx := id - ID_RECENT_BASE
		if idx < len(a.recent) {
			a.openDocument(a.recent[idx])
		}
		return
	}
	if id >= ID_ZOOM_BASE && id <= ID_ZOOM_BASE+15 {
		z := id - ID_ZOOM_BASE
		if z < a.zoom {
			a.setZoomTopLeft(z)
		} else {
			a.setZoom(z, -1, -1)
		}
		return
	}
	if id >= ID_DEJAVU_TRANSPARENT_BASE && id < ID_DEJAVU_TRANSPARENT_BASE+16 {
		if a.dejavu != nil {
			a.dejavu.TransparentIndex = byte(id - ID_DEJAVU_TRANSPARENT_BASE)
			a.statusText = fmt.Sprintf("투명 PNG 픽셀은 DejaVu 팔레트 인덱스 %d로 가져옵니다.", a.dejavu.TransparentIndex)
			a.rebuildMenuForTheme()
			invalidate(a.hwnd)
		}
		return
	}
	if a.curveUndoPending {
		switch id {
		case ID_VIEW_TOOLBOX, ID_VIEW_PALETTE, ID_VIEW_STATUS, ID_VIEW_CLIPBOARD, ID_VIEW_HISTORY, ID_VIEW_GRID, ID_VIEW_THUMBNAIL, ID_VIEW_DARKMODE, ID_VIEW_LANGUAGE_KO, ID_VIEW_LANGUAGE_EN, ID_VIEW_ZOOM_NORMAL, ID_VIEW_ZOOM_LARGE, ID_INSERT_FORMULA, ID_INSERT_FUNCTION_GRAPH, ID_INSERT_IMAGE_CIRCUIT, ID_INSERT_IMAGE_ANIMAL, ID_INSERT_IMAGE_CUSTOM, ID_INSERT_IMAGE_MULTIDIM, ID_INSERT_IMAGE_FIELD, ID_HELP_TOPICS, ID_HELP_LOG, ID_HELP_ABOUT:
			// These commands do not alter the document, so the unfinished curve
			// may remain visible and can still be completed.
		default:
			a.finishCurveEarlyOrCancel("command")
		}
	}
	switch id {
	case ID_FILE_NEW:
		a.newDocument()
	case ID_FILE_OPEN:
		if p, ok := chooseOpenDocumentFile(a.hwnd); ok {
			a.openDocument(p)
		}
	case ID_FILE_SAVE:
		a.save(false)
	case ID_FILE_SAVEAS:
		a.save(true)
	case ID_FILE_EXIT:
		wndProc(a.hwnd, WM_CLOSE, 0, 0)
	case ID_DEJAVU_TILES:
		a.showDejavuTileBrowser()
	case ID_DEJAVU_EXPORT_SHEET:
		a.exportDejavuSheetPNG()
	case ID_DEJAVU_IMPORT_SHEET:
		a.importDejavuSheetPNG()
	case ID_DEJAVU_EXPORT_TILE:
		a.exportDejavuTilePNG()
	case ID_DEJAVU_IMPORT_TILE:
		a.importDejavuTilePNG()
	case ID_DEJAVU_RELOAD_PALETTE:
		a.reloadDejavuPaletteInteractive()
	case ID_EDIT_UNDO:
		if !a.historyUndo() {
			a.statusText = "더 이전 히스토리가 없습니다."
		}
	case ID_EDIT_REDO:
		if !a.historyRedo() {
			a.statusText = "더 이후 히스토리가 없습니다."
		}
	case ID_EDIT_CUT:
		a.deleteSelection(true)
	case ID_EDIT_COPY:
		a.copySelection()
	case ID_EDIT_PASTE:
		a.pasteInternalClipboard()
	case ID_EDIT_DELETE:
		a.deleteSelection(false)
	case ID_EDIT_DELETE_TRANSPARENT:
		a.deleteSelectionTransparent()
	case ID_EDIT_SELECTALL:
		a.selectAll()
	case ID_EDIT_PASTEFROM:
		if p, ok := chooseOpenFile(a.hwnd); ok {
			a.pasteImageFile(p)
		}
	case ID_VIEW_TOOLBOX:
		a.showToolbox = !a.showToolbox
		a.toggleMenuCheck(id, a.showToolbox)
		a.updateScrollbars()
		invalidate(a.hwnd)
	case ID_VIEW_PALETTE:
		a.showPalette = !a.showPalette
		a.toggleMenuCheck(id, a.showPalette)
		a.updateScrollbars()
		invalidate(a.hwnd)
	case ID_VIEW_STATUS:
		a.showStatus = !a.showStatus
		a.toggleMenuCheck(id, a.showStatus)
		a.updateScrollbars()
		invalidate(a.hwnd)
	case ID_VIEW_CLIPBOARD:
		a.showClipboard = !a.showClipboard
		a.toggleMenuCheck(id, a.showClipboard)
		a.updateScrollbars()
		var client RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
		a.updateTextUILayout(client)
		if a.showClipboard {
			a.statusText = "클립보드 창을 열었습니다. 항목 클릭은 붙여넣기, 핀 버튼은 고정입니다."
		} else {
			a.statusText = "클립보드 창을 닫았습니다. 기록 수집은 계속됩니다."
		}
		logf("CLIPBOARD panel visible=%v", a.showClipboard)
		invalidate(a.hwnd)
	case ID_VIEW_HISTORY:
		a.showHistory = !a.showHistory
		a.toggleMenuCheck(id, a.showHistory)
		if a.showHistory {
			a.statusText = "히스토리 창을 열었습니다. 작업 이름을 클릭하면 해당 시점으로 이동합니다."
		} else {
			a.statusText = "히스토리 창을 닫았습니다. 작업 기록은 계속 유지됩니다."
		}
		invalidate(a.hwnd)
	case ID_VIEW_TEXTBAR:
		a.showTextPanel = !a.showTextPanel
		if a.showTextPanel {
			a.tool = ToolText
			a.ensureTextControls()
			a.statusText = "텍스트 도구 상자를 열었습니다. 제목줄을 끌어 분리하거나 오른쪽에 다시 붙일 수 있습니다."
		} else {
			a.statusText = "텍스트 도구 상자를 닫았습니다."
		}
		a.toggleMenuCheck(id, a.showTextPanel)
		var client RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&client)))
		a.updateTextUILayout(client)
		a.updateScrollbars()
		invalidate(a.hwnd)
	case ID_VIEW_ZOOM_NORMAL:
		// Ctrl+Page Up / 작게: 현재 단계에서 바로 아래 배율로 한 단계 축소합니다.
		a.nextZoom(-1, -1, -1)
	case ID_VIEW_ZOOM_LARGE:
		// Ctrl+Page Down / 크게: 현재 단계에서 바로 위 배율로 한 단계 확대합니다.
		a.nextZoom(1, -1, -1)
	case ID_VIEW_BITMAP:
		a.setBitmapView(!a.bitmapView)
	case ID_VIEW_GRID:
		if a.zoom < 6 {
			a.statusText = "눈금 보기는 6x 이상에서 사용할 수 있습니다."
			a.updateZoomDependentViewMenu()
			invalidate(a.hwnd)
			return
		}
		a.showGrid = !a.showGrid
		a.toggleMenuCheck(ID_VIEW_GRID, a.showGrid)
		if a.showGrid {
			a.statusText = "픽셀 눈금을 표시합니다. 6x 이상에서 사용할 수 있습니다."
		} else {
			a.statusText = "픽셀 눈금을 숨겼습니다."
		}
		logf("GRID visible=%v zoom=%d", a.showGrid, a.zoom)
		invalidate(a.hwnd)
	case ID_VIEW_THUMBNAIL:
		if a.zoom < 4 {
			a.statusText = "미리 보기 창은 4x 이상에서 사용할 수 있습니다."
			a.updateZoomDependentViewMenu()
			invalidate(a.hwnd)
			return
		}
		a.showThumbnail = !a.showThumbnail
		a.toggleMenuCheck(ID_VIEW_THUMBNAIL, a.showThumbnail)
		var thumbnailClient RECT
		procGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&thumbnailClient)))
		a.updateTextUILayout(thumbnailClient)
		if a.showThumbnail {
			a.statusText = "오른쪽 미리 보기 창을 열었습니다. 전체 그림과 현재 화면 범위를 표시합니다."
		} else {
			a.statusText = "미리 보기 창을 닫았습니다."
		}
		logf("THUMBNAIL visible=%v zoom=%d width=%d", a.showThumbnail, a.zoom, rightPanelWidth)
		invalidate(a.hwnd)
	case ID_VIEW_DARKMODE:
		a.toggleDarkMode()
	case ID_VIEW_LANGUAGE_KO:
		a.switchDotPaintLanguage("ko")
	case ID_VIEW_LANGUAGE_EN:
		a.switchDotPaintLanguage("en")
	case ID_IMAGE_FLIP:
		a.showFlipPopup()
	case ID_IMAGE_STRETCH:
		if a.dejavu != nil && !a.selection.Active {
			messageBox(a.hwnd, "DejaVu IMG 전체 타일시트는 288×288로 고정되어 있습니다.\n\n늘이기/기울이기는 먼저 선택 영역을 만든 뒤 사용하십시오.", "DejaVu IMG", MB_OK|MB_ICONINFORMATION)
			return
		}
		a.showStretchSkewDialog()
	case ID_IMAGE_INVERT:
		if !a.invertActiveSelection() {
			a.undo.Push(a.canvas)
			a.canvas.Invert()
			a.markDirty()
			a.recordHistory("색 반전")
			a.statusText = "전체 이미지의 색을 반전했습니다."
			logf("INVERT canvas w=%d h=%d", a.canvas.W, a.canvas.H)
			invalidate(a.hwnd)
		}
	case ID_IMAGE_ATTRIBUTES:
		if a.dejavu != nil {
			messageBox(a.hwnd, "DejaVu IMG는 144개 24×24 타일의 12×12 배열이므로 캔버스 크기는 288×288로 고정됩니다.", "DejaVu IMG", MB_OK|MB_ICONINFORMATION)
			return
		}
		a.showCanvasSizeDialog()
	case ID_IMAGE_CLEAR:
		a.clearSelectionState()
		a.undo.Push(a.canvas)
		a.canvas.Clear(a.bg)
		a.markDirty()
		a.recordHistory("모두 지우기")
		invalidate(a.hwnd)
	case ID_IMAGE_OPAQUE:
		a.setSelectionOpaque(!a.opaque)
	case ID_COLOR_INDEXED_TOGGLE:
		a.toggleIndexedPaletteMode()
	case ID_COLOR_INDEXED_256:
		if a.indexedMode && a.indexedSize == 256 {
			a.setIndexedPaletteMode(0)
		} else {
			a.setIndexedPaletteMode(256)
		}
	case ID_COLOR_INDEXED_LOAD:
		a.loadIndexedPaletteInteractive()
	case ID_COLOR_INDEXED_SAVE:
		a.saveIndexedPaletteInteractive()
	case ID_COLOR_EDIT:
		a.editColor()
	case ID_INSERT_FORMULA:
		a.insertChordPending = false
		a.showFormulaEditor()
		invalidate(a.hwnd)
	case ID_INSERT_FUNCTION_GRAPH:
		a.insertChordPending = false
		a.showFunctionGraphEditor()
		invalidate(a.hwnd)
	case ID_INSERT_IMAGE_CIRCUIT:
		a.showCircuitSymbolLibrary()
		invalidate(a.hwnd)
	case ID_INSERT_IMAGE_ANIMAL:
		a.showAnimalLibrary()
		invalidate(a.hwnd)
	case ID_INSERT_IMAGE_CUSTOM:
		a.showCustomImageLibrary()
		invalidate(a.hwnd)
	case ID_INSERT_IMAGE_MULTIDIM:
		a.showMultidimShapeEditor()
		invalidate(a.hwnd)
	case ID_INSERT_IMAGE_FIELD:
		a.showFieldEditor()
		invalidate(a.hwnd)
	case ID_HELP_TOPICS:
		a.showHelpCenter()
		invalidate(a.hwnd)
	case ID_HELP_LOG:
		if diagnosticLoggingEnabled {
			openDiagnosticLog(a.hwnd)
		}
	case ID_HELP_ABOUT:
		a.showAboutWindow()
	}
}

func keyDown(vk int) bool { r, _, _ := procGetKeyState.Call(uintptr(vk)); return int16(r&0xffff) < 0 }
func (a *App) onKey(vk int) {
	if vk == VK_F1 {
		a.showHelpCenter()
		return
	}
	ctrl := keyDown(VK_CONTROL)
	shift := keyDown(VK_SHIFT)
	if ctrl {
		// The requested notation Ctrl+N+M / Ctrl+N+G is implemented as a
		// simultaneous three-key chord: hold Ctrl+N, then press M or G.  Plain
		// Ctrl+N remains New and is committed when N is released.
		if a.insertChordPending {
			switch vk {
			case 'M':
				a.insertChordPending = false
				a.command(ID_INSERT_FORMULA)
				return
			case 'G':
				a.insertChordPending = false
				a.command(ID_INSERT_FUNCTION_GRAPH)
				return
			}
		}
		switch vk {
		case 'N':
			if shift {
				a.insertChordPending = false
				a.command(ID_IMAGE_CLEAR)
			} else if !a.insertChordPending {
				a.insertChordPending = true
				a.statusText = "Ctrl+N: 키를 놓으면 새로 만들기, 누른 채 M은 수식, G는 함수/그래프 삽입입니다."
				logf("INSERT chord armed")
				a.requestStatusRepaint()
			}
		case 'O':
			a.command(ID_FILE_OPEN)
		case 'S':
			if shift {
				a.command(ID_FILE_SAVEAS)
			} else {
				a.command(ID_FILE_SAVE)
			}
		case 'Z':
			if shift {
				a.command(ID_EDIT_REDO)
			} else {
				a.command(ID_EDIT_UNDO)
			}
		case 'Y':
			a.command(ID_EDIT_REDO)
		case 'A':
			a.command(ID_EDIT_SELECTALL)
		case 'C':
			a.command(ID_EDIT_COPY)
		case 'X':
			a.command(ID_EDIT_CUT)
		case 'V':
			a.command(ID_EDIT_PASTE)
		case 'T':
			a.command(ID_VIEW_TOOLBOX)
		case 'L':
			a.command(ID_VIEW_PALETTE)
		case 'F':
			a.command(ID_VIEW_BITMAP)
		case 'G':
			a.command(ID_VIEW_GRID)
		case 'I':
			a.command(ID_IMAGE_INVERT)
		case 'R':
			a.command(ID_IMAGE_FLIP)
		case 'W':
			a.command(ID_IMAGE_STRETCH)
		case 'E':
			a.command(ID_IMAGE_ATTRIBUTES)
		case VK_PRIOR:
			a.command(ID_VIEW_ZOOM_NORMAL)
		case VK_NEXT:
			a.command(ID_VIEW_ZOOM_LARGE)
		}
	} else if vk == VK_DELETE {
		if shift {
			a.command(ID_EDIT_DELETE_TRANSPARENT)
		} else {
			a.command(ID_EDIT_DELETE)
		}
	} else if vk == VK_ESCAPE {
		if a.tool == ToolEyedropper && a.eyedropperActive {
			tool := a.eyedropperReturnTool
			if tool == ToolEyedropper {
				tool = ToolPencil
			}
			a.stopScreenEyedropper()
			a.tool = tool
			a.statusText = "스포이드 색 추출을 취소하고 이전 도구로 돌아왔습니다."
			invalidate(a.hwnd)
		} else if a.tool == ToolText && a.textTool.Active {
			a.finishTextEditing(false)
		} else if a.pendingCanvasResize || a.resizingCanvas {
			a.cancelCanvasResizeInteraction("escape", true)
		} else if a.selection.Creating || a.selection.Moving {
			a.selection.Creating = false
			a.selection.Moving = false
			procReleaseCapture.Call()
			a.clearSelectionState()
			invalidate(a.hwnd)
		} else if a.selection.Active {
			a.clearSelectionState()
			invalidate(a.hwnd)
		} else if a.tool == ToolCurve && a.curveUndoPending {
			a.finishCurveEarlyOrCancel("escape")
			invalidate(a.hwnd)
		} else if a.drawing {
			a.drawing = false
			a.preview = false
			a.shiftStrokeActive = false
			a.shiftStrokeBasePix = nil
			a.shiftStrokeAnchorX, a.shiftStrokeAnchorY = 0, 0
			a.shiftStrokePreviewX, a.shiftStrokePreviewY = 0, 0
			a.curveBaseShiftConstrained = false
			procReleaseCapture.Call()
			a.undo.Undo(a.canvas)
			invalidate(a.hwnd)
		}
	}
}

func measureMenuText(hdc uintptr, text string) (int, int) {
	measure := strings.ReplaceAll(text, "\t", "        ")
	rc := RECT{0, 0, 2000, 200}
	p := utf16Ptr(measure)
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&rc)), DT_LEFT|DT_SINGLELINE|DT_CALCRECT)
	return max(int(rc.Right-rc.Left), 1), max(int(rc.Bottom-rc.Top), 16)
}

func (a *App) measureThemedMenuItem(mis *MEASUREITEMSTRUCT) bool {
	if mis == nil || mis.CtlType != ODT_MENU {
		return false
	}
	entry, ok := menuDrawEntryFor(mis.ItemData)
	if !ok {
		return false
	}
	hdc, _, _ := procGetDC.Call(a.hwnd)
	if hdc == 0 {
		return false
	}
	old := selectDefaultFont(hdc)
	w, h := measureMenuText(hdc, entry.Text)
	procSelectObject.Call(hdc, old)
	procReleaseDC.Call(a.hwnd, hdc)
	if entry.TopLevel {
		mis.ItemWidth = uint32(w + 18)
		mis.ItemHeight = uint32(max(h+6, 22))
	} else {
		extra := 54
		if entry.Popup {
			// Reserve the native Windows submenu-arrow column without drawing a
			// duplicate glyph ourselves.
			extra = 60
		}
		mis.ItemWidth = uint32(w + extra)
		mis.ItemHeight = uint32(max(h+8, 24))
	}
	return true
}

func (a *App) drawThemedMenuItem(dis *DRAWITEMSTRUCT) bool {
	if dis == nil || dis.CtlType != ODT_MENU || dis.HDC == 0 {
		return false
	}
	entry, ok := menuDrawEntryFor(dis.ItemData)
	if !ok {
		return false
	}
	selected := dis.ItemState&ODS_SELECTED != 0
	disabled := dis.ItemState&(ODS_DISABLED|ODS_GRAYED) != 0
	checked := dis.ItemState&ODS_CHECKED != 0
	bg := rgb(246, 246, 246)
	fg := rgb(20, 20, 20)
	gutterColor := rgb(240, 240, 240)
	if a.darkMode {
		bg = rgb(28, 28, 28)
		fg = rgb(238, 238, 238)
		gutterColor = rgb(36, 36, 36)
	}
	if selected {
		if a.darkMode {
			bg = rgb(64, 64, 64)
		} else {
			bg = rgb(204, 228, 247)
		}
	}
	if disabled {
		if a.darkMode {
			fg = rgb(118, 118, 118)
		} else {
			fg = rgb(132, 132, 132)
		}
	}
	fillRect(dis.HDC, dis.RcItem, bg)
	old := selectDefaultFont(dis.HDC)
	defer procSelectObject.Call(dis.HDC, old)

	if entry.TopLevel {
		r := dis.RcItem
		r.Left += 9
		r.Right -= 9
		drawText(dis.HDC, entry.Text, r, DT_LEFT|DT_VCENTER|DT_SINGLELINE, fg)
		if dis.ItemState&ODS_FOCUS != 0 {
			procDrawFocusRect.Call(dis.HDC, uintptr(unsafe.Pointer(&dis.RcItem)))
		}
		return true
	}

	gutter := RECT{dis.RcItem.Left, dis.RcItem.Top, dis.RcItem.Left + 24, dis.RcItem.Bottom}
	fillRect(dis.HDC, gutter, gutterColor)
	if checked {
		drawText(dis.HDC, "✓", gutter, DT_CENTER|DT_VCENTER|DT_SINGLELINE, fg)
	}
	leftText, rightText := entry.Text, ""
	if tab := strings.IndexByte(entry.Text, '\t'); tab >= 0 {
		leftText, rightText = entry.Text[:tab], entry.Text[tab+1:]
	}
	rightReserve := int32(14)
	if entry.Popup {
		rightReserve = 30
	}
	leftRc := RECT{dis.RcItem.Left + 30, dis.RcItem.Top, dis.RcItem.Right - rightReserve, dis.RcItem.Bottom}
	drawText(dis.HDC, leftText, leftRc, DT_LEFT|DT_VCENTER|DT_SINGLELINE, fg)
	if rightText != "" {
		rightRc := RECT{dis.RcItem.Left + 30, dis.RcItem.Top, dis.RcItem.Right - 10, dis.RcItem.Bottom}
		drawText(dis.HDC, rightText, rightRc, DT_RIGHT|DT_VCENTER|DT_SINGLELINE, fg)
	}
	// Do not draw a second submenu arrow here. Windows owns the native popup
	// indicator for items that have an HMENU, and drawing our own glyph caused
	// a large ▶ plus the native selected-state arrow to appear together.
	if dis.ItemState&ODS_FOCUS != 0 {
		focus := dis.RcItem
		focus.Left += 2
		focus.Right -= 2
		procDrawFocusRect.Call(dis.HDC, uintptr(unsafe.Pointer(&focus)))
	}
	return true
}

// --- Window procedure ------------------------------------------------------
func wndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) (ret uintptr) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logPanic(fmt.Sprintf("wndProc msg=%#x hwnd=%#x", msg, hwnd), recovered)
			ret = defWindowProc(hwnd, msg, wparam, lparam)
		}
	}()
	if app == nil {
		return defWindowProc(hwnd, msg, wparam, lparam)
	}
	switch msg {
	case WM_MENUCHAR:
		if result, ok := handleThemedMenuChar(wparam, lparam); ok {
			return result
		}
	case WM_MEASUREITEM:
		if lparam != 0 {
			mis := (*MEASUREITEMSTRUCT)(unsafe.Pointer(lparam))
			if app.measureThemedMenuItem(mis) {
				return 1
			}
			if mis.CtlType == ODT_COMBOBOX && int(mis.CtlID) == ID_TEXT_FONTCOMBO {
				mis.ItemHeight = 34
				return 1
			}
		}
	case WM_DRAWITEM:
		if lparam != 0 {
			dis := (*DRAWITEMSTRUCT)(unsafe.Pointer(lparam))
			if app.drawThemedMenuItem(dis) {
				return 1
			}
			if drawThemedOwnerDrawPushButton(dis) {
				return 1
			}
			if app.drawFontComboItem(dis) {
				return 1
			}
		}
	case WM_CREATE:
		logf("WM_CREATE hwnd=%#x", hwnd)
		app.hwnd = hwnd
		app.applyWindowTheme()
		procDragAcceptFiles.Call(hwnd, 1)
		logf("DROPFILES enabled")
		app.buildMenu()
		app.updateTitle()
		app.loadPinnedClipboardHistory()
		app.updateScrollbars()
		if r, _, err := procAddClipboardFormatListener.Call(hwnd); r != 0 {
			app.clipboardListening = true
			logf("CLIPBOARD listener registered")
		} else {
			logf("CLIPBOARD listener registration failed: %v", err)
		}
		app.captureSystemClipboard("startup")
		if procGetDpiForWindow.Find() == nil {
			dpi, _, _ := procGetDpiForWindow.Call(hwnd)
			logf("WINDOW dpi=%d", dpi)
		}
		return 0
	case WM_SIZE:
		logf("WM_SIZE client=%dx%d", loword(lparam), hiword(lparam))
		app.updateScrollbars()
		var rc RECT
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		app.updateTextUILayout(rc)
		invalidate(hwnd)
		return 0
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, err := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if hdc == 0 {
			logf("BeginPaint failed: %v", err)
			return 0
		}
		app.paintBuffered(hdc, ps.RcPaint)
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_CTLCOLOREDIT:
		if app != nil && wparam != 0 {
			procSetBkMode.Call(wparam, OPAQUE)
			if app.darkMode {
				procSetTextColor.Call(wparam, uintptr(rgb(240, 240, 240)))
				procSetBkColor.Call(wparam, uintptr(rgb(30, 30, 30)))
				if app.darkBrush == 0 {
					app.darkBrush, _, _ = procCreateSolidBrush.Call(uintptr(rgb(30, 30, 30)))
				}
				return app.darkBrush
			}
			procSetBkMode.Call(wparam, TRANSPARENT)
			procSetTextColor.Call(wparam, uintptr(gdiPreviewColor(app.bg)))
			brush, _, _ := procGetStockObject.Call(NULL_BRUSH)
			return brush
		}
		return 0
	case WM_CTLCOLORSTATIC, WM_CTLCOLORBTN, WM_CTLCOLORLISTBOX, WM_CTLCOLORDLG:
		if app != nil && app.darkMode && wparam != 0 {
			procSetBkMode.Call(wparam, OPAQUE)
			procSetTextColor.Call(wparam, uintptr(rgb(235, 235, 235)))
			procSetBkColor.Call(wparam, uintptr(rgb(30, 30, 30)))
			if app.darkBrush == 0 {
				app.darkBrush, _, _ = procCreateSolidBrush.Call(uintptr(rgb(30, 30, 30)))
			}
			return app.darkBrush
		}
		return 0
	case WM_TIMER:
		if wparam == screenEyedropperTimerID {
			app.pollScreenEyedropper()
			return 0
		}
		if wparam == textCaretTimerID {
			if app.textTool.Active && app.textTool.Editing {
				app.textTool.CaretVisible = !app.textTool.CaretVisible
				app.requestTextCaretRepaint(false)
			} else {
				app.stopTextCaretBlink()
			}
			return 0
		}
		if wparam == textPreviewTimerID {
			procKillTimer.Call(app.hwnd, textPreviewTimerID)
			if app.textTool.Active && app.textTool.Editing {
				if app.flushPendingTextIME() {
					app.syncTextLayoutControl()
					app.textTool.CaretVisible = true
				}
				app.requestTextRepaint(false)
			}
			return 0
		}
		return 0
	case WM_COMMAND:
		id := int(loword(wparam))
		code := int(hiword(wparam))
		if app.handleTextControlCommand(id, code) {
			return 0
		}
		app.command(id)
		return 0
	case WM_DROPFILES:
		app.handleDroppedFiles(wparam)
		return 0
	case WM_LBUTTONDOWN:
		app.onMouseDown(int(signedLoword(lparam)), int(signedHiword(lparam)), false)
		return 0
	case WM_LBUTTONDBLCLK:
		app.onMouseDoubleClick(int(signedLoword(lparam)), int(signedHiword(lparam)))
		return 0
	case WM_RBUTTONDOWN:
		app.onMouseDown(int(signedLoword(lparam)), int(signedHiword(lparam)), true)
		return 0
	case WM_MOUSEMOVE:
		mx, my := int(signedLoword(lparam)), int(signedHiword(lparam))
		app.onMouseMove(mx, my, wparam)
		// Mouse capture can suppress WM_SETCURSOR during a drag. Refresh here as
		// well so the contrast variant follows the canvas continuously.
		var client RECT
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
		procSetCursor.Call(app.cursorForClientPoint(mx, my, client))
		return 0
	case WM_CANCELMODE:
		app.cancelCanvasResizeInteraction("WM_CANCELMODE", true)
		app.cancelPendingRectSelection("WM_CANCELMODE", true)
		return 0
	case WM_CAPTURECHANGED:
		newOwner := lparam
		if app.pendingCanvasResize || app.resizingCanvas {
			if canvasResizeCaptureChangeIsLoss(newOwner, app.hwnd) {
				app.cancelCanvasResizeInteraction(fmt.Sprintf("WM_CAPTURECHANGED newOwner=%#x", newOwner), false)
			} else {
				logf("CANVAS_RESIZE capture-change ignored newOwner=self=%#x", app.hwnd)
			}
		}
		if app.pendingRectSelection && newOwner != app.hwnd {
			app.cancelPendingRectSelection(fmt.Sprintf("WM_CAPTURECHANGED newOwner=%#x", newOwner), false)
			invalidate(app.hwnd)
		}
		if app.thumbnailNavigationDragging {
			app.thumbnailNavigationDragging = false
			logf("THUMBNAIL drag cancelled: mouse capture changed")
			invalidate(app.hwnd)
		}
		return 0
	case WM_LBUTTONUP:
		app.onMouseUp(int(signedLoword(lparam)), int(signedHiword(lparam)), false)
		return 0
	case WM_RBUTTONUP:
		app.onMouseUp(int(signedLoword(lparam)), int(signedHiword(lparam)), true)
		return 0
	case WM_HSCROLL:
		si := SCROLLINFO{CbSize: uint32(unsafe.Sizeof(SCROLLINFO{})), FMask: SIF_ALL}
		procGetScrollInfo.Call(hwnd, SB_HORZ, uintptr(unsafe.Pointer(&si)))
		app.handleScroll(SB_HORZ, int(loword(wparam)), int(si.NTrackPos))
		return 0
	case WM_VSCROLL:
		si := SCROLLINFO{CbSize: uint32(unsafe.Sizeof(SCROLLINFO{})), FMask: SIF_ALL}
		procGetScrollInfo.Call(hwnd, SB_VERT, uintptr(unsafe.Pointer(&si)))
		app.handleScroll(SB_VERT, int(loword(wparam)), int(si.NTrackPos))
		return 0
	case WM_MOUSEWHEEL:
		delta := int(int16(hiword(wparam)))
		pt := POINT{int32(int16(loword(lparam))), int32(int16(hiword(lparam)))}
		procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
		var client RECT
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
		if app.clipboardPanelVisible() && pointInRect(int(pt.X), int(pt.Y), app.clipboardPanelRect(client)) {
			if delta > 0 {
				app.scrollClipboard(-1)
			} else {
				app.scrollClipboard(1)
			}
			return 0
		}
		// Palette now displays the configured visible range directly (1..256).
		// Mouse wheel no longer pages away indices while the palette is shown.

		if app.historyPanelVisible() && pointInRect(int(pt.X), int(pt.Y), app.historyPanelRect(client)) {
			if delta > 0 {
				app.scrollHistory(-3)
			} else {
				app.scrollHistory(3)
			}
			return 0
		}
		if delta > 0 {
			app.handleScroll(SB_VERT, SB_LINEUP, 0)
		} else {
			app.handleScroll(SB_VERT, SB_LINEDOWN, 0)
		}
		return 0
	case WM_KEYDOWN:
		// Bit 30 is the previous-key-state flag. A held Shift can generate
		// dozens of redundant WM_KEYDOWNs each second. Repainting on every
		// autorepeat both delays the pointer and may make the preview blink.
		// Check before logging: the development logger flushes every line.
		if wparam == VK_SHIFT && (lparam&(uintptr(1)<<30)) != 0 {
			if diagnosticLoggingEnabled {
				atomic.AddUint64(&shiftPreviewAutorepeatsIgnored, 1)
			}
			return 0
		}
		logf("WM_KEYDOWN vk=%d", wparam)
		// A98: F10 is a native menu-focus key. Passing it to DefWindowProc
		// preserves standard Windows keyboard navigation instead of consuming it
		// as a canvas shortcut. Alt+letter is handled natively through the &
		// mnemonic markers in the menu strings.
		if wparam == VK_F10 && !keyDown(VK_CONTROL) && !keyDown(VK_SHIFT) {
			logf("MENU focus requested by F10")
			return defWindowProc(hwnd, msg, wparam, lparam)
		}
		app.onKey(int(wparam))
		if wparam == VK_SHIFT {
			if app.selection.Moving {
				app.updateSelectionMovePosition(app.selection.MovePointerX, app.selection.MovePointerY, true)
				logf("SELECTION move shift down start=%d,%d pointer=%d,%d result=%d,%d", app.selection.MoveStartX, app.selection.MoveStartY, app.selection.MovePointerX, app.selection.MovePointerY, app.selection.X, app.selection.Y)
			}
			app.requestConstrainedPreviewTransition(false, true, true)
		}
		return 0
	case WM_KEYUP:
		if wparam == VK_SHIFT && diagnosticLoggingEnabled {
			if n := atomic.SwapUint64(&shiftPreviewAutorepeatsIgnored, 0); n != 0 {
				logf("PREVIEW shift-autorepeat-suppressed count=%d", n)
			}
		}
		if wparam == 'N' && app.insertChordPending {
			app.insertChordPending = false
			logf("INSERT chord released as plain Ctrl+N")
			app.command(ID_FILE_NEW)
			return 0
		}
		if wparam == VK_SHIFT {
			if app.selection.Moving {
				app.updateSelectionMovePosition(app.selection.MovePointerX, app.selection.MovePointerY, false)
				logf("SELECTION move shift up pointer=%d,%d result=%d,%d", app.selection.MovePointerX, app.selection.MovePointerY, app.selection.X, app.selection.Y)
			}
			app.requestConstrainedPreviewTransition(true, false, true)
		}
		return 0
	case WM_CLIPBOARDUPDATE:
		seq, _, _ := procGetClipboardSequenceNumber.Call()
		if uint32(seq) == app.lastClipboardSeq {
			logf("CLIPBOARD update self/duplicate seq=%d", seq)
			return 0
		}
		app.captureSystemClipboard("WM_CLIPBOARDUPDATE")
		return 0
	case WM_SETCURSOR:
		if loword(lparam) == 1 {
			var pt POINT
			procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
			var client RECT
			procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
			procSetCursor.Call(app.cursorForClientPoint(int(pt.X), int(pt.Y), client))
			return 1
		}
		// Windows owns HTLEFT/HTRIGHT/HTTOP/HTBOTTOM/HT*CORNER and uses
		// its native two-headed resize arrows. Do not override NC cursors.
		return defWindowProc(hwnd, msg, wparam, lparam)
	case WM_CLOSE:
		if app.curveUndoPending {
			app.finishCurveEarlyOrCancel("window close")
		}
		logf("WM_CLOSE dirty=%v", app.dirty)
		if app.confirmLoseChanges() {
			procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
		}
		return 0
	case WM_DESTROY:
		logf("WM_DESTROY")
		procDragAcceptFiles.Call(hwnd, 0)
		app.stopScreenEyedropper()
		app.finishTextEditing(true)
		app.destroyTextUIFont()
		app.destroyDockPanelTitleFont()
		if app.clipboardListening {
			procRemoveClipboardFormatListener.Call(hwnd)
			app.clipboardListening = false
			logf("CLIPBOARD listener removed")
		}
		app.savePinnedClipboardHistory()
		app.saveRecent()
		if err := saveThemePreference(app.darkMode); err != nil {
			logf("THEME preference save on exit failed dark=%v err=%v", app.darkMode, err)
		} else {
			logf("THEME preference saved on exit dark=%v", app.darkMode)
		}
		app.destroyCanvasTileBuffer()
		app.destroyBackBuffer()
		destroyToolCursors()
		if app.menuBrush != 0 {
			procDeleteObject.Call(app.menuBrush)
			app.menuBrush = 0
		}
		if app.darkBrush != 0 {
			procDeleteObject.Call(app.darkBrush)
			app.darkBrush = 0
		}
		procPostQuitMessage.Call(0)
		return 0
	}
	return defWindowProc(hwnd, msg, wparam, lparam)
}

func defWindowProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func main() {
	runtime.LockOSThread()
	initUILanguage()
	initDiagnosticLog()
	defer closeDiagnosticLog()
	defer func() {
		if recovered := recover(); recovered != nil {
			logPanic("main", recovered)
			message := "dotPaint에서 예기치 않은 오류가 발생했습니다."
			if diagnosticLoggingEnabled {
				message += "\n실행 파일 옆의 dotPaint.log를 보내 주십시오."
			}
			messageBox(0, message, "dotPaint", MB_OK|MB_ICONERROR)
		}
	}()

	tid, _, _ := procGetCurrentThreadId.Call()
	logf("MAIN locked OS thread id=%d", tid)
	// Native per-monitor DPI rendering for Windows 11. -4 is
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2.
	r, _, err := procSetProcessDpiAwarenessContext.Call(^uintptr(3))
	logf("SetProcessDpiAwarenessContext result=%d err=%v", r, err)
	// Enable native dark visual styles before any child controls are created.
	initNativeDarkModeSupport()

	pngName := utf16Ptr("PNG")
	clipboardPNGFormat, _, _ = procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(pngName)))
	imagePNGName := utf16Ptr("image/png")
	clipboardImagePNGFormat, _, _ = procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(imagePNGName)))
	logf("CLIPBOARD registered formats PNG=%d image/png=%d", clipboardPNGFormat, clipboardImagePNGFormat)

	app = newApp()
	if savedDarkMode, ok, themeErr := loadThemePreference(); themeErr != nil {
		logf("THEME preference load failed; using default dark=%v err=%v", app.darkMode, themeErr)
	} else if ok {
		app.darkMode = savedDarkMode
		logf("THEME preference loaded dark=%v", app.darkMode)
	} else {
		logf("THEME preference absent/invalid; using default dark=%v", app.darkMode)
	}
	logf("APP allocated canvas=%dx%d pix=%d", app.canvas.W, app.canvas.H, len(app.canvas.Pix))
	logf("VECTOR_PREVIEW policy backend=CPU-DIRTY-REGION gpu=false reason=GDI-present-avoids-D3D-upload-readback")
	app.loadRecent()
	hinst, _, err := procGetModuleHandleW.Call(0)
	if hinst == 0 {
		logf("GetModuleHandleW failed: %v", err)
	}
	app.hinst = hinst
	initToolCursors(hinst)
	wndProcCallback = syscall.NewCallback(wndProc)
	className := utf16Ptr("DOTPAINT_MAIN_A8")
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	iconBig, _, iconBigErr := procLoadImageW.Call(hinst, IDI_DOTPAINT, IMAGE_ICON, 32, 32, LR_SHARED)
	iconSmall, _, iconSmallErr := procLoadImageW.Call(hinst, IDI_DOTPAINT, IMAGE_ICON, 16, 16, LR_SHARED)
	logf("APP_ICON load resource=%d big=%#x small=%#x bigErr=%v smallErr=%v", IDI_DOTPAINT, iconBig, iconSmall, iconBigErr, iconSmallErr)
	wc := WNDCLASSEXW{CbSize: uint32(unsafe.Sizeof(WNDCLASSEXW{})), Style: CS_DBLCLKS, LpfnWndProc: wndProcCallback, HInstance: hinst, HIcon: iconBig, HCursor: cursor, HbrBackground: uintptr(COLOR_BTNFACE + 1), LpszClassName: className, HIconSm: iconSmall}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	logf("RegisterClassExW atom=%d err=%v", atom, err)
	if atom == 0 {
		messageBox(0, "창 클래스를 등록하지 못했습니다.\n"+err.Error(), "dotPaint", MB_OK|MB_ICONERROR)
		return
	}
	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(utf16Ptr("제목 없음 - dotPaint"))), WS_OVERLAPPEDWINDOW|WS_HSCROLL|WS_VSCROLL, uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), 1100, 760, 0, 0, hinst, 0)
	logf("CreateWindowExW hwnd=%#x err=%v", hwnd, err)
	if hwnd != 0 {
		if iconBig != 0 {
			procSendMessageW.Call(hwnd, WM_SETICON, ICON_BIG, iconBig)
		}
		if iconSmall != 0 {
			procSendMessageW.Call(hwnd, WM_SETICON, ICON_SMALL, iconSmall)
		}
	}
	if hwnd == 0 {
		messageBox(0, "프로그램 창을 만들지 못했습니다.\n"+err.Error(), "dotPaint", MB_OK|MB_ICONERROR)
		return
	}
	app.hwnd = hwnd
	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)
	// File Explorer's Open With/file association launches this process with
	// the selected file path as argv[1]. Use the same parser/open path as File > Open.
	// Do not run during WM_CREATE: the window and all palette controls must exist.
	if path, err := startupDocumentPath(os.Args[1:]); err != nil {
		logf("STARTUP_OPEN ignored args=%q err=%v", os.Args[1:], err)
		messageBox(hwnd, "명령줄에서 지정한 그림 파일을 열 수 없습니다.\n\n"+err.Error(), "dotPaint", MB_OK|MB_ICONWARNING)
	} else if path != "" {
		if absolutePath, absErr := filepath.Abs(path); absErr == nil {
			path = absolutePath
		}
		logf("STARTUP_OPEN requested path=%q totalArgs=%d", path, len(os.Args)-1)
		if app.openDocument(path) {
			logf("STARTUP_OPEN success path=%q", path)
		} else {
			logf("STARTUP_OPEN failed path=%q", path)
		}
	}
	logf("MESSAGE_LOOP begin")
	var msg MSG
	for {
		r, _, err := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) == -1 {
			logf("GetMessageW failed: %v", err)
			break
		}
		if r == 0 {
			logf("GetMessageW WM_QUIT")
			break
		}
		if app != nil && msg.Hwnd == app.textTool.EditHwnd {
			switch msg.Message {
			case WM_IME_STARTCOMPOSITION:
				app.beginTextIMEComposition()
				procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
				continue
			case WM_IME_COMPOSITION:
				// EN_CHANGE is delivered synchronously while this message is
				// dispatched. The active transaction rebuilds from its fixed base.
				procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
				continue
			case WM_IME_ENDCOMPOSITION:
				procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
				app.endTextIMEComposition()
				continue
			case WM_KEYDOWN:
				vk := int(msg.WParam)
				beforeStart, beforeEnd := app.textSelection()
				if app.handleTextEditKeyDown(vk) {
					continue
				}
				navigation := isTextNativeNavigationKey(vk)
				procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
				procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
				if navigation {
					app.afterTextSelectionNavigation(beforeStart, beforeEnd, "native horizontal caret move", false)
				}
				continue
			}
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
