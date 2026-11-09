package main
import "testing"
func BenchmarkDotPaint201TanRightTriangle20(b *testing.B) {
    c:=NewCanvas(1024,768,0xffffff)
    r:=vectorRenderer{canvas:c,lineConstraintMode:LineConstraintTanN,rightTriangleSize:20}
    b.ReportAllocs();b.ResetTimer()
    for i:=0;i<b.N;i++ {r.drawRasterRightTriangleAt(c,14,24,900,400,true,0,0,0)}
}
