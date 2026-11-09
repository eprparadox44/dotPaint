package main

import "testing"

// CPU raster engines only: this deliberately makes no Windows GUI/FPS claim.
func BenchmarkBeta4ProtectedRasterLine(b *testing.B) {
	c := NewCanvas(1024, 768, 0xFFFFFF)
	r := vectorRenderer{canvas: c, lineSize: 5, lineAntialias: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.drawRasterLineAt(c, 14, 24, 900, 650, false, 0, 0, 0)
	}
}
func BenchmarkBeta4ProtectedRasterTriangleTan(b *testing.B) {
	c := NewCanvas(1024, 768, 0xFFFFFF)
	r := vectorRenderer{canvas: c, rightTriangleSize: 20, lineConstraintMode: LineConstraintTanN}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.drawRasterRightTriangleAt(c, 14, 24, 900, 400, true, 0, 0, 0)
	}
}
func BenchmarkBeta4ProtectedRasterArrow(b *testing.B) {
	c := NewCanvas(1024, 768, 0xFFFFFF)
	r := vectorRenderer{canvas: c, arrowLineSize: 5, arrowHeadSize: 3, arrowAntialias: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.drawRasterArrowAt(c, 14, 24, 900, 650, false, 0, 0, 0)
	}
}

func BenchmarkBeta4CurveCachedHit(b *testing.B) {
	o := MovableObject{Kind: ToolCurve, Stroke: 5, Vector: ObjectVector{Count: 6, Antialias: true}}
	o.Vector.Points = [6]ObjectPoint{{10, 100}, {100, 5}, {200, 200}, {300, 5}, {400, 200}, {500, 100}}
	o, _ = renderVectorObject(o, 600, 300)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		beta2ProtectedFloat = 0
		if vectorObjectContains(&o, 250, 110, 5) {
			beta2ProtectedFloat = 1
		}
	}
}
