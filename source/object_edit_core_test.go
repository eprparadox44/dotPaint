package main

import (
	"bytes"
	"fmt"
	"image/png"
	"math"
	"testing"
)

func testObject(kind Tool, x, y, w, h int, b, g, r, alpha byte) MovableObject {
	o := MovableObject{Kind: kind, X: x, Y: y, W: w, H: h, SpriteW: w, SpriteH: h, Pix: make([]byte, w*h*4), ShapeW: w, ShapeH: h, ShapeX1: w - 1, ShapeY1: h - 1, Stroke: 1}
	for p := 0; p < len(o.Pix); p += 4 {
		copy(o.Pix[p:p+4], []byte{b, g, r, alpha})
	}
	return o
}

func TestBeta2MultipleTypesAndTopmostSelection(t *testing.T) {
	objects := []MovableObject{
		testObject(ToolRectangle, 10, 10, 100, 80, 255, 0, 0, 255),
		testObject(ToolRightTriangle, 25, 25, 70, 60, 0, 255, 0, 255),
		testObject(ToolEllipse, 30, 30, 60, 50, 0, 0, 255, 255),
		testObject(ToolText, 40, 40, 40, 30, 0, 0, 0, 255),
	}
	if got := hitObjectBody(objects, 50, 50); got != 3 {
		t.Fatalf("topmost=%d", got)
	}
	for i := len(objects) - 1; i >= 0; i-- {
		if got := hitObjectBody(objects[:i+1], 50, 50); got != i {
			t.Fatalf("type %v hit=%d", objects[i].Kind, got)
		}
	}
	if hitObjectBody(objects, -1, 50) != -1 {
		t.Fatal("outside hit")
	}
}

func TestBeta2GeometricInteriorsAndTransparentExteriors(t *testing.T) {
	base := testObject(ToolRectangle, 0, 0, 100, 100, 0, 0, 0, 255)
	for _, kind := range []Tool{ToolRectangle, ToolEllipse, ToolRightTriangle, ToolText} {
		o := testObject(kind, 10, 10, 80, 80, 0, 0, 0, 0)
		if !objectBodyContains(&o, 49, 49) {
			t.Fatalf("hollow interior %v", kind)
		}
	}
	ellipse := testObject(ToolEllipse, 10, 10, 80, 80, 0, 0, 0, 0)
	if got := hitObjectBody([]MovableObject{base, ellipse}, 11, 11); got != 0 {
		t.Fatal("ellipse corner blocked lower object")
	}
	triangle := testObject(ToolRightTriangle, 10, 10, 80, 80, 0, 0, 0, 0)
	if got := hitObjectBody([]MovableObject{base, triangle}, 11, 88); got != 0 {
		t.Fatal("triangle exterior blocked lower object")
	}
	if !objectBodyContains(&triangle, 88, 11) {
		t.Fatal("triangle interior missed")
	}
	triangle.ShapeX0, triangle.ShapeX1 = 79, 0
	if !objectBodyContains(&triangle, 11, 11) {
		t.Fatal("flipped triangle missed")
	}
	ellipse.Transparency = 100
	if !objectBodyContains(&ellipse, 50, 50) {
		t.Fatal("invisible object cannot be recovered")
	}
}

func TestBeta2EightResizeHandlesAnchorsAndNoCrossing(t *testing.T) {
	b := ObjectBounds{10, 20, 30, 40}
	want := []ObjectBounds{
		{15, 27, 25, 33}, {10, 27, 30, 33}, {10, 27, 35, 33}, {10, 20, 35, 40},
		{10, 20, 35, 47}, {10, 20, 30, 47}, {15, 20, 25, 47}, {15, 20, 25, 40},
	}
	for i, w := range want {
		h := ObjectHandle(i + 1)
		got, ok := resizedObjectBounds(b, h, 5, 7)
		if !ok || got != w {
			t.Errorf("handle=%d got=%+v want=%+v", h, got, w)
		}
		for _, d := range []int{-100000, 100000} {
			got, ok = resizedObjectBounds(b, h, d, d)
			if ok && !validObjectSize(got.W, got.H) {
				t.Fatalf("unstable handle=%d %+v", h, got)
			}
		}
	}
	if _, ok := resizedObjectBounds(b, ObjectBody, 0, 0); ok {
		t.Fatal("body treated as handle")
	}
	if _, ok := resizedObjectBounds(ObjectBounds{0, 0, 6000, 6000}, ObjectSE, 5000, 5000); ok {
		t.Fatal("unbounded object allocation")
	}
}

func TestBeta2TriangleResizeRetainsNativeStrokeMargins(t *testing.T) {
	for _, width := range []int{1, 2, 5, 20, 100, 2000} {
		a, b := resizedShapeAxis(2, 52, 55, width)
		if a < 0 || b < a || b >= width {
			t.Fatalf("invalid triangle axis %d -> %d,%d", width, a, b)
		}
		if width >= 5 && (a != 2 || b != width-3) {
			t.Fatal("stroke margins scaled with box")
		}
		c, d := resizedShapeAxis(52, 2, 55, width)
		if c != b || d != a {
			t.Fatal("triangle orientation changed")
		}
	}
}

func TestBeta2ZoomScrollAndAllClientHandles(t *testing.T) {
	b := ObjectBounds{50, 70, 30, 40}
	for _, z := range []int{1, 2, 4, 6, 8, 12, 15, 20} {
		for _, origin := range []ObjectPoint{{0, 0}, {91, 38}, {-617, -893}} {
			for i, p := range objectHandlePoints(b, z, origin.X, origin.Y) {
				if got := hitObjectHandle(b, z, origin.X, origin.Y, p.X, p.Y); got != ObjectHandle(i+1) {
					t.Errorf("zoom=%d handle=%d got=%d", z, i+1, got)
				}
			}
		}
	}
	for _, z := range []int{1, 2, 8, 20} {
		for d := -2 * z; d <= 2*z; d++ {
			want := int(math.Floor(float64(d) / float64(z)))
			if floorDivInt(d, z) != want {
				t.Fatal("negative workspace rounding")
			}
		}
	}
}

func TestBeta2TinyObjectsAndStackingHandlePriority(t *testing.T) {
	for _, size := range []int{1, 2, 3, 4, 5} {
		o := testObject(ToolEllipse, 10, 10, size, size, 0, 0, 0, 255)
		i, h := hitObjectAtClient([]MovableObject{o}, 1, 0, 0, 15+size, 15+size)
		if i != 0 || h != ObjectSE {
			t.Fatalf("tiny=%d hit=%d/%d", size, i, h)
		}
	}
	low := testObject(ToolRectangle, 20, 20, 20, 20, 0, 0, 0, 255)
	top := testObject(ToolText, 0, 0, 100, 100, 0, 0, 0, 0)
	i, h := hitObjectAtClient([]MovableObject{low, top}, 1, 0, 0, 20, 20)
	if i != 1 || h != ObjectBody {
		t.Fatal("lower handle stole top object's body")
	}
}

func TestBeta2OpacityCoverageAndAlpha(t *testing.T) {
	for p := 0; p <= 100; p++ {
		for _, alpha := range []byte{0, 64, 128, 255} {
			o := testObject(ToolRectangle, 0, 0, 1, 1, 240, 120, 60, alpha)
			o.Transparency = uint8(100 - p)
			dst := []byte{20, 40, 80, 255}
			compositeObjectsToPixels(dst, 1, 1, 0, 0, []MovableObject{o})
			sa := float64(alpha) * float64(p) / 100 / 255
			for c, base := range []byte{20, 40, 80} {
				want := float64(o.Pix[c])*sa + float64(base)*(1-sa)
				if math.Abs(float64(dst[c])-want) > 1.6 {
					t.Fatalf("p=%d alpha=%d got=%v channel=%d want=%.2f", p, alpha, dst, c, want)
				}
			}
			if dst[3] != 255 {
				t.Fatal("opaque background damaged")
			}
		}
	}
	o := testObject(ToolText, 0, 0, 1, 1, 255, 100, 0, 255)
	o.Transparency = 50
	dst := make([]byte, 4)
	compositeObjectsToPixels(dst, 1, 1, 0, 0, []MovableObject{o})
	if dst[0] != 255 || dst[1] != 100 || dst[3] != 128 {
		t.Fatalf("straight alpha clipboard %v", dst)
	}
}

func TestBeta2PartialAlphaWhiteNeverOverflows(t *testing.T) {
	// Rounding the output alpha before normalizing RGB used to produce 256,
	// which wrapped white to black on a nearly transparent destination.
	for _, da := range []byte{1, 2, 17, 64, 128, 200, 254} {
		for sa := 1; sa < 255; sa++ {
			dst := []byte{255, 255, 255, da}
			src := []byte{255, 255, 255, byte(sa)}
			blendBGRA(dst, 0, src, 0)
			if dst[0] != 255 || dst[1] != 255 || dst[2] != 255 {
				t.Fatalf("white wrap sa=%d da=%d: %v", sa, da, dst)
			}
		}
	}
}

func TestBeta2AllOrderOperationsPreserveOtherItems(t *testing.T) {
	for _, tc := range []struct {
		from, to int
		want     string
	}{{1, 2, "ACBD"}, {2, 1, "ACBD"}, {1, 3, "ACDB"}, {2, 0, "CABD"}} {
		objects := make([]MovableObject, 4)
		for i := range objects {
			objects[i].Text = string(rune('A' + i))
		}
		i, ok := reorderObject(objects, tc.from, tc.to)
		got := ""
		for _, o := range objects {
			got += o.Text
		}
		if !ok || i != tc.to || got != tc.want {
			t.Fatalf("order %+v got=%s", tc, got)
		}
	}
}

func TestBeta2OrderChangesCompositeAndHitTogether(t *testing.T) {
	objects := []MovableObject{testObject(ToolRectangle, 0, 0, 1, 1, 0, 0, 255, 255), testObject(ToolEllipse, 0, 0, 1, 1, 255, 0, 0, 255)}
	dst := make([]byte, 4)
	compositeObjectsToPixels(dst, 1, 1, 0, 0, objects)
	if dst[0] != 255 {
		t.Fatal("wrong initial order")
	}
	reorderObject(objects, 0, 1)
	compositeObjectsToPixels(dst, 1, 1, 0, 0, objects)
	if dst[2] != 255 || objects[hitObjectBody(objects, 0, 0)].Kind != ToolRectangle {
		t.Fatal("render and hit order differ")
	}
}

func TestBeta2ClippedTileMatchesFullExportAtOpacityAndResize(t *testing.T) {
	objects := []MovableObject{testObject(ToolRectangle, -3, -2, 16, 20, 50, 200, 30, 255), testObject(ToolEllipse, 5, 7, 30, 20, 200, 20, 220, 128)}
	objects[0].Transparency = 50
	objects[1].W, objects[1].H = 17, 33 // scaled live preview
	full := bytes.Repeat([]byte{10, 20, 30, 255}, 40*40)
	compositeObjectsToPixels(full, 40, 40, 0, 0, objects)
	const stride = 27
	tile := bytes.Repeat([]byte{10, 20, 30, 255}, stride*22)
	compositeObjectRows(tile, 19, 22, stride, 4, 5, objects, -1)
	for y := 0; y < 22; y++ {
		if !bytes.Equal(tile[y*stride*4:(y*stride+19)*4], full[((y+5)*40+4)*4:((y+5)*40+23)*4]) {
			t.Fatalf("tile row=%d", y)
		}
		if !bytes.Equal(tile[(y*stride+19)*4:(y+1)*stride*4], bytes.Repeat([]byte{10, 20, 30, 255}, 8)) {
			t.Fatal("tile padding overwritten")
		}
	}
}

func TestBeta2MovementAndPreviewNeverMutateRasterOrSprite(t *testing.T) {
	base := bytes.Repeat([]byte{40, 80, 120, 255}, 100*100)
	original := append([]byte(nil), base...)
	o := testObject(ToolEllipse, 10, 10, 20, 20, 20, 50, 200, 200)
	sprite := append([]byte(nil), o.Pix...)
	saved := copyObjects([]MovableObject{o})
	for n := 0; n < 50; n++ {
		o.X, o.Y, o.W, o.H = n, n, 20+n, 20+n
		dst := append([]byte(nil), base...)
		compositeObjectsToPixels(dst, 100, 100, 0, 0, []MovableObject{o})
	}
	if !bytes.Equal(base, original) || !bytes.Equal(o.Pix, sprite) {
		t.Fatal("underlying raster or immutable cache changed")
	}
	if saved[0].W != 20 || saved[0].X != 10 {
		t.Fatal("snapshot geometry changed")
	}
}

func TestBeta2SnapshotPreservesTextStylesOpacityAndRecipe(t *testing.T) {
	o := testObject(ToolText, 1, 2, 4, 5, 1, 2, 3, 255)
	o.Text = "한글 Ab"
	o.Styles = []TextCharStyle{{FontName: "함초롬바탕", FontSize: 20, Bold: true}, {FontName: "Arial", FontSize: 12, ScriptMode: 1}}
	o.Transparency = 75
	snapshot := copyObjects([]MovableObject{o})
	o.Styles[0].FontSize = 40
	o.X = 100
	o.Transparency = 0
	if snapshot[0].Styles[0].FontSize != 20 || snapshot[0].Transparency != 75 || snapshot[0].Text != "한글 Ab" {
		t.Fatal("text formatting/opacity history damaged")
	}
	if &snapshot[0].Pix[0] != &o.Pix[0] {
		t.Fatal("snapshot duplicated immutable image")
	}
}

func TestBeta2ReferenceIconsAndBinaryCursors(t *testing.T) {
	im, err := png.Decode(bytes.NewReader(objectToolReferencePNG))
	if err != nil {
		t.Fatal(err)
	}
	for theme := 0; theme < 2; theme++ {
		count := 0
		for y := 0; y < objectIconH; y++ {
			for x := 0; x < objectIconW; x++ {
				p := objectToolIcons[theme][y*objectIconW+x]
				if p == objectArtTransparent {
					continue
				}
				count++
				r, g, b, _ := im.At(theme*45+x, y).RGBA()
				want := uint32(r>>8) | uint32(g>>8)<<8 | uint32(b>>8)<<16
				if p != want {
					t.Fatal("user artwork changed")
				}
			}
		}
		if count < 200 {
			t.Fatal("empty reference icon")
		}
	}
	seen := map[string]bool{}
	for i := -1; i < 5; i++ {
		p := objectPointerPixels()
		if i >= 0 {
			p = objectArrowCursorPixels(i)
		}
		black, white, inverse := 0, 0, 0
		for _, v := range p {
			switch v {
			case 0:
			case 1:
				black++
			case 2:
				white++
			case 3:
				inverse++
			default:
				t.Fatal("antialiased cursor")
			}
		}
		if i < 0 {
			// The new user-supplied pointer is an exact 214-pixel XOR silhouette.
			if inverse != 214 || black != 0 || white != 0 {
				t.Fatal("selection cursor is not an exact monochrome XOR reference")
			}
		} else if black == 0 || white == 0 {
			t.Fatal("resize/move cursor lacks contrasting opaque colors")
		}
		key := fmt.Sprint(p)
		if seen[key] {
			t.Fatal("resize cursor directions identical")
		}
		seen[key] = true
	}
	if p := objectPointerPixels(); p[0] != 3 || p[33] != 3 {
		t.Fatal("new XOR cursor tip must remain at the top-left user-art pixel")
	}
}

var beta2BenchmarkObjects []MovableObject
var beta2BenchmarkBounds ObjectBounds

func BenchmarkBeta2ObjectInactive(b *testing.B) {
	dst := make([]byte, 128*128*4)
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		compositeObjectsToPixels(dst, 128, 128, 0, 0, beta2BenchmarkObjects)
	}
}

func BenchmarkBeta2ObjectOpaqueTile(b *testing.B) {
	dst := make([]byte, 256*128*4)
	objects := []MovableObject{testObject(ToolEllipse, 0, 0, 200, 100, 1, 2, 3, 255)}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		compositeObjectsToPixels(dst, 256, 128, 0, 0, objects)
	}
}

func BenchmarkBeta2ObjectResizeGeometry(b *testing.B) {
	bounds := ObjectBounds{100, 200, 300, 400}
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		beta2BenchmarkBounds, _ = resizedObjectBounds(bounds, ObjectSE, n%200, n%150)
	}
}
