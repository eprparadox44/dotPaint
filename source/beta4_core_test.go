package main

import (
	"bytes"
	"fmt"
	"testing"
)

func b4PixelObject(col uint32, op PaintOperation) MovableObject {
	p := colorrefToBGRA(col)
	return MovableObject{Kind: ToolRectangle, W: 1, H: 1, SpriteW: 1, SpriteH: 1, Pix: append([]byte(nil), p[:]...), Operation: op}
}

func TestBeta4SequentialInvertComposition(t *testing.T) {
	for _, alpha := range []byte{0, 1, 64, 128, 254, 255} {
		original := []byte{19, 73, 211, alpha}
		inv := b4PixelObject(0xFFFFFF, PaintInvert)
		for n := 0; n < 6; n++ {
			out := append([]byte(nil), original...)
			objects := make([]MovableObject, n)
			for i := range objects {
				objects[i] = inv
			}
			compositeObjectsToPixels(out, 1, 1, 0, 0, objects)
			expected := append([]byte(nil), original...)
			if n%2 != 0 {
				invertBGRAPixel(expected, 0)
			}
			if !bytes.Equal(out, expected) {
				t.Fatalf("alpha=%d inversions=%d: %v", alpha, n, out)
			}
		}
	}
	out := []byte{0, 0, 0, 255}
	compositeObjectsToPixels(out, 1, 1, 0, 0, []MovableObject{b4PixelObject(rgb(255, 0, 0), PaintRGB), b4PixelObject(0xFFFFFF, PaintInvert), b4PixelObject(rgb(0, 0, 255), PaintRGB), b4PixelObject(0xFFFFFF, PaintInvert)})
	if !bytes.Equal(out, []byte{0, 255, 255, 255}) {
		t.Fatalf("opaque blue must reset prior inversion: %v", out)
	}
	for _, coverage := range []byte{0, 1, 127, 128, 254, 255} {
		o := b4PixelObject(0xFFFFFF, PaintInvert)
		o.Pix[3] = coverage
		out := []byte{0, 10, 30, 255}
		compositeObjectsToPixels(out, 1, 1, 0, 0, []MovableObject{o})
		if (out[0] == 255) != (coverage >= 128) {
			t.Fatal("binary coverage boundary")
		}
	}
}

func TestBeta4InverseTextForegroundBackgroundParity(t *testing.T) {
	for _, fg := range []PaintOperation{PaintRGB, PaintTransparent, PaintInvert} {
		for _, bg := range []PaintOperation{PaintRGB, PaintTransparent, PaintInvert} {
			if fg != PaintInvert && bg != PaintInvert {
				continue
			}
			for _, glyph := range []byte{0, 127, 128, 255} {
				o := b4PixelObject(rgb(255, 0, 0), fg)
				o.Kind = ToolText
				o.Opaque = true
				o.BackgroundOperation = bg
				o.BG = rgb(0, 0, 255)
				o.Pix[3] = glyph
				out := []byte{10, 40, 80, 200}
				want := append([]byte(nil), out...)
				if bg == PaintInvert {
					invertBGRAPixel(want, 0)
				} else if bg == PaintRGB {
					p := colorrefToBGRA(o.BG)
					blendBGRA(want, 0, p[:], 0)
				}
				if fg == PaintInvert && glyph >= 128 {
					invertBGRAPixel(want, 0)
				} else if fg == PaintRGB {
					blendBGRA(want, 0, o.Pix, 0)
				}
				compositeObjectsToPixels(out, 1, 1, 0, 0, []MovableObject{o})
				if !bytes.Equal(out, want) {
					t.Fatalf("fg=%d bg=%d glyph=%d %v != %v", fg, bg, glyph, out, want)
				}
			}
		}
	}
}

func TestBeta4RasterStrokeOnceAndFootprints(t *testing.T) {
	for _, shape := range []BrushShape{BrushSquare, BrushCircle, BrushSlashUp, BrushSlashDown} {
		for _, size := range []int{1, 2, 3, 8, 17, 40} {
			base := NewCanvas(100, 90, rgb(255, 0, 0))
			mask := &Canvas{W: 100, H: 90, Pix: make([]byte, 100*90*4)}
			stroke := InvertStroke{}
			path := []PixelPoint{{1, 1}, {60, 40}, {12, 70}, {60, 40}, {1, 1}}
			for i := 1; i < len(path); i++ {
				a, b := path[i-1], path[i]
				mask.DrawUnifiedBrushLine(a.X, a.Y, b.X, b.Y, size, shape, 0xFFFFFF)
				stroke.Line(base, a.X, a.Y, b.X, b.Y, size, shape)
			}
			for j := 0; j < len(mask.Pix); j += 4 {
				covered := mask.Pix[j+3] != 0
				if (base.Pix[j] == 255) != covered {
					t.Fatalf("shape=%d size=%d pixel=%d", shape, size, j/4)
				}
			}
			stroke.Reset()
			for i := 1; i < len(path); i++ {
				a, b := path[i-1], path[i]
				stroke.Line(base, a.X, a.Y, b.X, b.Y, size, shape)
			}
			if !bytes.Equal(base.Pix, NewCanvas(100, 90, rgb(255, 0, 0)).Pix) {
				t.Fatal("new stroke must invert again")
			}
		}
	}
	c := NewCanvas(4096, 4096, 0)
	s := InvertStroke{}
	s.Stamp(c, 2000, 2000, 1, BrushSquare)
	if len(s.tiles) != 1 {
		t.Fatal("sparse stroke allocated untouched tiles")
	}
}

func TestBeta4FreezeInverseAtStackPosition(t *testing.T) {
	base := NewCanvas(12, 8, rgb(80, 30, 10))
	base.Pix[3] = 91
	normal := b4PixelObject(rgb(200, 0, 0), PaintRGB)
	normal.Pix[3] = 103
	inv := b4PixelObject(0xFFFFFF, PaintInvert)
	top := b4PixelObject(rgb(0, 200, 50), PaintRGB)
	top.Pix[3] = 83
	objects := []MovableObject{normal, inv, top}
	before := base.ClonePixels()
	compositeObjectsToPixels(before, 12, 8, 0, 0, objects)
	snapshots := copyObjects(objects)
	frozen := freezeInverseObject(base, objects, 1)
	frozen.Rasterized = true
	objects[1] = frozen
	after := base.ClonePixels()
	compositeObjectsToPixels(after, 12, 8, 0, 0, objects)
	if !bytes.Equal(before, after) {
		t.Fatal("freezing inverse double-blended RGBA")
	}
	if objectsEqual(objects, snapshots) {
		t.Fatal("history must distinguish locked replacement")
	}
	objects[0].Pix = []byte{255, 0, 0, 255}
	after = base.ClonePixels()
	compositeObjectsToPixels(after, 12, 8, 0, 0, objects)
	if !bytes.Equal(before, after) {
		t.Fatal("frozen inverse still follows lower object")
	}
	restored := base.ClonePixels()
	compositeObjectsToPixels(restored, 12, 8, 0, 0, snapshots)
	if !bytes.Equal(before, restored) {
		t.Fatal("snapshot caches were mutated")
	}
	// Prefix consolidation is exactly the same operation as the layer stack.
	merged := &Canvas{W: 12, H: 8, Pix: base.ClonePixels()}
	compositeObjectsToPixels(merged.Pix, 12, 8, 0, 0, objects[:2])
	compositeObjectsToPixels(merged.Pix, 12, 8, 0, 0, objects[2:])
	if !bytes.Equal(merged.Pix, before) {
		t.Fatal("locked prefix consolidation changed output")
	}
}

func b4Vector(t *testing.T, kind Tool, count, width int, aa bool) MovableObject {
	t.Helper()
	o := MovableObject{Kind: kind, Stroke: width, FG: rgb(220, 60, 20), Vector: ObjectVector{Count: count, Antialias: aa, ArrowHead: 2}}
	pts := []ObjectPoint{{35, 80}, {110, 8}, {200, 200}, {280, 5}, {320, 180}, {360, 90}}
	if count == 2 {
		pts[1] = pts[5]
	}
	for i := 0; i < count; i++ {
		o.Vector.Points[i] = pts[i]
	}
	out, ok := renderVectorObject(o, 400, 240)
	if !ok {
		t.Fatal("render vector")
	}
	return out
}

func TestBeta4OriginalVectorRendererAndControlPoints(t *testing.T) {
	for _, kind := range []Tool{ToolLine, ToolArrow, ToolCurve} {
		for _, aa := range []bool{false, true} {
			for _, width := range []int{1, 2, 7} {
				for count := 2; count <= 6; count++ {
					if kind != ToolCurve && count != 2 {
						continue
					}
					t.Run(fmt.Sprintf("%d/%d/%d/%v", kind, count, width, aa), func(t *testing.T) {
						o := b4Vector(t, kind, count, width, aa)
						direct := NewCanvas(400, 240, 0xFFFFFF)
						actual := NewCanvas(400, 240, 0xFFFFFF)
						polygon := make([]PixelPoint, count)
						for i := range polygon {
							p := o.Vector.Points[i]
							polygon[i] = PixelPoint{p.X + o.X, p.Y + o.Y}
						}
						p, q := polygon[0], polygon[count-1]
						r := vectorRenderer{canvas: direct, lineSize: width, lineAntialias: aa, arrowLineSize: width, arrowAntialias: aa, arrowHeadSize: 2}
						if kind == ToolLine {
							r.drawRasterLineAt(direct, p.X, p.Y, q.X, q.Y, false, o.FG, 0, 0)
						} else if kind == ToolArrow {
							r.drawRasterArrowAt(direct, p.X, p.Y, q.X, q.Y, false, o.FG, 0, 0)
						} else {
							path := bezierRasterPath(polygon)
							if count == 2 {
								path = appendRasterSegment(nil, p.X, p.Y, q.X, q.Y)
							}
							direct.DrawRoundCurvePixelPath(path, width, aa, o.FG)
						}
						compositeObjectsToPixels(actual.Pix, 400, 240, 0, 0, []MovableObject{o})
						if !bytes.Equal(actual.Pix, direct.Pix) {
							t.Fatal("persistent render differs from original raster")
						}
						original := o
						for vertex := 0; vertex < count; vertex++ {
							next := o
							next.Vector.Points[vertex].X += 11
							next.Vector.Points[vertex].Y -= 4
							edited, ok := renderVectorObject(next, 400, 240)
							if !ok {
								t.Fatal("edit")
							}
							for i := 0; i < count; i++ {
								if i == vertex {
									continue
								}
								a, b := original.Vector.Points[i], edited.Vector.Points[i]
								if a.X+original.X != b.X+edited.X || a.Y+original.Y != b.Y+edited.Y {
									t.Fatal("editing one point moved a different point")
								}
							}
							o = original
						}
						snap := copyObjects([]MovableObject{o})
						next := o
						next.Vector.Points[0].X += 3
						renderVectorObject(next, 400, 240)
						if !objectsEqual(snap, []MovableObject{o}) {
							t.Fatal("renderer mutated cached history")
						}
					})
				}
			}
		}
	}
}

func TestBeta4GeometricPickingAtZoomAndScroll(t *testing.T) {
	o := b4Vector(t, ToolCurve, 6, 2, true)
	for _, zoom := range []int{1, 2, 4, 8, 20} {
		for _, origin := range []ObjectPoint{{0, 0}, {-800, -900}, {71, 43}} {
			handles := objectVertexPoints(&o, zoom, origin.X, origin.Y)
			for i, p := range handles {
				idx, h := hitObjectAtClient([]MovableObject{o}, zoom, origin.X, origin.Y, p.X, p.Y)
				if idx != 0 || h != ObjectVertex0+ObjectHandle(i) {
					t.Fatal("control handle hit")
				}
			}
			p := o.Path[len(o.Path)/2]
			x, y := origin.X+(o.X+p.X)*zoom, origin.Y+(o.Y+p.Y)*zoom
			if idx, _ := hitObjectAtClient([]MovableObject{o}, zoom, origin.X, origin.Y, x+3, y); idx != 0 {
				t.Fatal("five screen-pixel body tolerance")
			}
		}
	}
	if vectorObjectContains(&o, float64(o.X), float64(o.Y), 0) {
		t.Fatal("curve bounding-box corner selected as curve")
	}
	o.Rasterized = true
	if idx, _ := hitObjectAtClient([]MovableObject{o}, 1, 0, 0, o.X+o.Vector.Points[0].X, o.Y+o.Vector.Points[0].Y); idx != -1 {
		t.Fatal("locked vector exposes handles")
	}
}

func TestBeta4StrictTanAndVeryShortArrow(t *testing.T) {
	for _, kind := range []Tool{ToolLine, ToolArrow} {
		for _, width := range []int{1, 2, 9} {
			for _, delta := range []ObjectPoint{{240, 80}, {-90, 37}, {1, 0}, {0, 0}} {
				o := MovableObject{Kind: kind, Stroke: width, FG: 0, Vector: ObjectVector{Count: 2, Constrained: true, Constraint: LineConstraintTanN, ArrowHead: 3, Antialias: true}}
				o.Vector.Points[0] = ObjectPoint{100, 100}
				o.Vector.Points[1] = ObjectPoint{100 + delta.X, 100 + delta.Y}
				rendered, ok := renderVectorObject(o, 400, 300)
				if !ok {
					t.Fatal("short vector")
				}
				r := vectorRenderer{canvas: &Canvas{W: 400, H: 300}, lineConstraintMode: LineConstraintTanN, lineSize: width, lineAntialias: true, arrowLineSize: width, arrowHeadSize: 3, arrowAntialias: true}
				direct, actual := NewCanvas(400, 300, 0xFFFFFF), NewCanvas(400, 300, 0xFFFFFF)
				if kind == ToolLine {
					r.drawRasterLineAt(direct, 100, 100, 100+delta.X, 100+delta.Y, true, 0, 0, 0)
				} else {
					r.drawRasterArrowAt(direct, 100, 100, 100+delta.X, 100+delta.Y, true, 0, 0, 0)
				}
				compositeObjectsToPixels(actual.Pix, 400, 300, 0, 0, []MovableObject{rendered})
				if !bytes.Equal(direct.Pix, actual.Pix) {
					t.Fatal("strict tan / short arrow changed")
				}
			}
		}
	}
}
