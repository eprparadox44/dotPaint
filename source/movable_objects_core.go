package main

import "bytes"

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
	ToolObjectSelect // appended to preserve every existing numeric tool ID
)

var toolNames = []string{
	"자유 선택", "사각 선택", "색 영역 선택", "동일 색 전체 선택", "지우개", "색 채우기", "색 골라내기", "확대/축소", "붓", "붓(통합됨)", "에어브러시(통합됨)", "텍스트", "직선", "곡선", "사각형", "타원", "직각삼각형", "화살표", "객체 선택",
}

type TextCharStyle struct {
	FontName   string
	FontSize   int
	Bold       bool
	Italic     bool
	Underline  bool
	ScriptMode int8 // 0 normal, 1 superscript, -1 subscript
	Color      uint32
	ColorSet   bool
	ColorOp    PaintOperation
}

func (s TextCharStyle) Equal(other TextCharStyle) bool {
	return s.FontName == other.FontName && s.FontSize == other.FontSize && s.Bold == other.Bold && s.Italic == other.Italic && s.Underline == other.Underline && s.ScriptMode == other.ScriptMode && s.Color == other.Color && s.ColorSet == other.ColorSet && s.ColorOp == other.ColorOp
}

type MovableObject struct {
	GroupID                        uint64 // zero means independently editable; grouped members keep their original rendering and Z-order
	Operation, BackgroundOperation PaintOperation
	Vector                         ObjectVector
	Path                           []ObjectPoint // immutable sampled original renderer path
	ReplaceMask                    []byte
	GlyphInvertMask                []byte // per-pixel inversion for mixed-color text        // immutable frozen inversion coverage

	Kind                                               Tool
	X, Y, W, H                                         int
	Pix                                                []byte // BGRA, straight alpha; pixel-exact committed sprite
	Text                                               string
	Styles                                             []TextCharStyle
	FontName                                           string
	FontSize                                           int
	Bold, Italic, Underline, Opaque                    bool
	ScriptMode                                         int8
	FG, BG                                             uint32
	Rasterized                                         bool  // true=locked pixel layer, never editable/pickable
	Transparency                                       uint8 // 0=opaque, 100=invisible; UI always exposes opacity=100-Transparency
	SpriteW, SpriteH                                   int   // immutable sprite dimensions, independent of live drag bounds
	ShapeW, ShapeH, ShapeX0, ShapeY0, ShapeX1, ShapeY1 int
	Stroke                                             int
	ShowCenter                                         bool
}

func copyObjects(src []MovableObject) []MovableObject {
	if len(src) == 0 {
		return nil
	}
	out := make([]MovableObject, len(src))
	for i := range src {
		out[i] = src[i]
		out[i].Pix = src[i].Pix // immutable raster cache, shared by history snapshots
		out[i].Styles = append([]TextCharStyle(nil), src[i].Styles...)
		out[i].GlyphInvertMask = src[i].GlyphInvertMask // immutable render cache
	}
	return out
}

func objectsEqual(a, b []MovableObject) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.GroupID != y.GroupID || x.Operation != y.Operation || x.BackgroundOperation != y.BackgroundOperation || x.Vector != y.Vector || !bytes.Equal(x.ReplaceMask, y.ReplaceMask) || !bytes.Equal(x.GlyphInvertMask, y.GlyphInvertMask) || !objectPathsEqual(x.Path, y.Path) || x.Rasterized != y.Rasterized || x.Transparency != y.Transparency || x.SpriteW != y.SpriteW || x.SpriteH != y.SpriteH || x.ShapeW != y.ShapeW || x.ShapeH != y.ShapeH || x.ShapeX0 != y.ShapeX0 || x.ShapeY0 != y.ShapeY0 || x.ShapeX1 != y.ShapeX1 || x.ShapeY1 != y.ShapeY1 || x.Stroke != y.Stroke || x.ShowCenter != y.ShowCenter || x.Kind != y.Kind || x.X != y.X || x.Y != y.Y || x.W != y.W || x.H != y.H || x.Text != y.Text || x.FG != y.FG || x.BG != y.BG || x.Opaque != y.Opaque || x.FontName != y.FontName || x.FontSize != y.FontSize || x.Bold != y.Bold || x.Italic != y.Italic || x.Underline != y.Underline || x.ScriptMode != y.ScriptMode || !bytes.Equal(x.Pix, y.Pix) || len(x.Styles) != len(y.Styles) {
			return false
		}
		for j := range x.Styles {
			if !x.Styles[j].Equal(y.Styles[j]) {
				return false
			}
		}
	}
	return true
}

func objectsMemoryBytes(src []MovableObject) int {
	n := 0
	for _, v := range src {
		n += len(v.ReplaceMask) + len(v.GlyphInvertMask) + len(v.Path)*16 + len(v.Pix) + len(v.Text) + len(v.FontName) + len(v.Styles)*96 + 96
	}
	return n
}

// Alpha compositing is shared between tiles, export and tests.  Raster objects
// have alpha 255 on their edges; antialiased text uses fractional coverage.
func blendBGRA(dst []byte, di int, src []byte, si int) {
	sa := int(src[si+3])
	if sa == 0 {
		return
	}
	if sa == 255 {
		copy(dst[di:di+4], src[si:si+4])
		return
	}
	da := int(dst[di+3])
	alphaNumerator := sa*255 + da*(255-sa)
	oa := (alphaNumerator + 127) / 255
	if oa == 0 {
		dst[di], dst[di+1], dst[di+2], dst[di+3] = 0, 0, 0, 0
		return
	}
	for ch := 0; ch < 3; ch++ {
		n := int(src[si+ch])*sa*255 + int(dst[di+ch])*da*(255-sa)
		dst[di+ch] = byte((n + alphaNumerator/2) / alphaNumerator)
	}
	dst[di+3] = byte(oa)
}

func trimObjectSprite(o MovableObject) (MovableObject, bool) {
	x0, y0, x1, y1 := o.W, o.H, -1, -1
	for y := 0; y < o.H; y++ {
		for x := 0; x < o.W; x++ {
			if o.Pix[(y*o.W+x)*4+3] == 0 {
				continue
			}
			x0 = min(x0, x)
			y0 = min(y0, y)
			x1 = max(x1, x)
			y1 = max(y1, y)
		}
	}
	if x1 < x0 || y1 < y0 {
		return MovableObject{}, false
	}
	w, h := x1-x0+1, y1-y0+1
	pixels := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		copy(pixels[y*w*4:(y+1)*w*4], o.Pix[((y+y0)*o.W+x0)*4:((y+y0)*o.W+x0+w)*4])
	}
	o.X += x0
	o.Y += y0
	o.W = w
	o.H = h
	o.Pix = pixels
	return o, true
}

func objectPathsEqual(a, b []ObjectPoint) bool {
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
