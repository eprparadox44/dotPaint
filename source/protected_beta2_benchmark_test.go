package main

import "testing"

var beta2ProtectedFloat float64
var beta2Protected3D []mdVec3
var beta2Protected4D []mdVec4
var beta2ProtectedFaces []mdFace
var beta2ProtectedFaces4 []mdFace4

func BenchmarkBeta2ProtectedLineCoverage(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		beta2ProtectedFloat = arrowStrokeCoverageAt(float64(i%500), float64(i%300), 1, 1, 600, 330, 5)
	}
}
func BenchmarkBeta2ProtectedArrowCoverage(b *testing.B) {
	g, _ := calculateArrowHead(1, 1, 600, 330, 2, 5)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		beta2ProtectedFloat = arrowHeadCoverageAt(float64(i%500), float64(i%300), g, true)
	}
}
func BenchmarkBeta2Protected3DMesh(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		beta2Protected3D, beta2ProtectedFaces, _, _ = mdBuildEllipsoidMesh(1, 1.3, 0.8, 32, 64)
	}
}
func BenchmarkBeta2Protected4DMesh(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		beta2Protected4D, beta2ProtectedFaces4, _, _ = mdBuildKleinBottle4D(32, 24, 2, 0.6)
	}
}
