package main

import "testing"

var md20VertexSink []mdVec3
var md20FaceSink []mdFace
var md20FaceCountSink int
var field20RasterSink fieldRaster

func BenchmarkDotPaint20TorusMeshSmooth(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		md20VertexSink, md20FaceSink = mdBuildTorusMesh(2, .8, 96, 48)
	}
}
func BenchmarkDotPaint20MobiusMeshSmooth(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		md20VertexSink, md20FaceSink = mdBuildMobiusMesh(2, .65, 144, 24)
	}
}
func BenchmarkDotPaint20SixHundredCellGeometry(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v, edges, faces, cells := mdBuildRegular600Cell()
		md20FaceCountSink = len(v) + len(edges) + len(faces) + len(cells)
	}
}
func BenchmarkDotPaint20TorusLayersBruteForce(b *testing.B) {
	verts, faces := mdBuildTorusMesh(2, .8, 96, 48)
	p := mdVec3{x: 1.8, y: -2, z: .1}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		md20FaceCountSink = mdUniqueSurfaceCoverCountCore(p, verts, faces, nil, 1e-5)
	}
}
func BenchmarkDotPaint20Field2DElectricVectors(b *testing.B) {
	o := fieldDefaultObject(fieldObjectSphere)
	o.Charge = 2
	s := fieldScene{Dimension: fieldDimension2D, Display: fieldDisplayElectricVectors, Objects: []fieldObject{o}, Density: 8, VectorDensity: 10, ViewDistance: 1}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		field20RasterSink = fieldRenderForBackgroundBase(s, 240, 160, 0, false)
	}
}
func BenchmarkDotPaint20Field2DElectricLines(b *testing.B) {
	o := fieldDefaultObject(fieldObjectSphere)
	o.Charge = 2
	s := fieldScene{Dimension: fieldDimension2D, Display: fieldDisplayElectricLines, Objects: []fieldObject{o}, Density: 8, VectorDensity: 10, ViewDistance: 1}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		field20RasterSink = fieldRenderForBackgroundBase(s, 240, 160, 0, false)
	}
}
func BenchmarkDotPaint20Field2DGravityPotential(b *testing.B) {
	o := fieldDefaultObject(fieldObjectSolidMassSphere)
	o.Charge = 5
	o.Radius = .5
	s := fieldScene{Dimension: fieldDimension2D, Display: fieldDisplayGravityPotential, Objects: []fieldObject{o}, Density: 8, ViewDistance: 1}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		field20RasterSink = fieldRenderForBackgroundBase(s, 240, 160, 0, false)
	}
}

func BenchmarkDotPaint20HundredTwentyCellGeometry(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v, edges, faces, cells := mdBuildRegular120Cell()
		md20FaceCountSink = len(v) + len(edges) + len(faces) + len(cells)
	}
}

func BenchmarkDotPaint20Field2DElectricVectors6x(b *testing.B) {
	o := fieldDefaultObject(fieldObjectSphere)
	o.Charge = 2
	s := fieldScene{Dimension: fieldDimension2D, Display: fieldDisplayElectricVectors, Objects: []fieldObject{o}, Density: 8, VectorDensity: 10, ViewDistance: 1, Antialias: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		field20RasterSink = fieldRenderForBackground(s, 240, 160, 0, false)
	}
}
func BenchmarkDotPaint20Field2DElectricVectors1x(b *testing.B) {
	o := fieldDefaultObject(fieldObjectSphere)
	o.Charge = 2
	s := fieldScene{Dimension: fieldDimension2D, Display: fieldDisplayElectricVectors, Objects: []fieldObject{o}, Density: 8, VectorDensity: 10, ViewDistance: 1, Antialias: false}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		field20RasterSink = fieldRenderForBackground(s, 240, 160, 0, false)
	}
}
func BenchmarkDotPaint20LargeEllipse(b *testing.B) {
	c := NewCanvas(1600, 1000, 0xffffff)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.DrawEllipseOuter(20, 30, 1500, 900, 3, 0x123456)
	}
}
