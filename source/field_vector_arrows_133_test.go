package main

import "testing"

func TestFieldVectorArrowGridCountsDoubleDensity132(t *testing.T) {
	scene2D := fieldDefaultScene()
	scene2D.Display = fieldDisplayElectricVectors
	scene2D.VectorDensity = 20
	nx, ny, nz := fieldVectorArrowGridCounts(scene2D)
	if nx != 14 || ny != 1 || nz != 14 {
		t.Fatalf("2D vector lattice=%dx%dx%d want 14x1x14", nx, ny, nz)
	}

	scene3D := scene2D
	scene3D.Dimension = fieldDimension3D
	nx, ny, nz = fieldVectorArrowGridCounts(scene3D)
	if nx != 14 || ny != 10 || nz != 14 {
		t.Fatalf("3D vector lattice=%dx%dx%d want 14x10x14", nx, ny, nz)
	}
}

func TestFieldVectorArrowLengthFractionClampsAndVaries133(t *testing.T) {
	zero := fieldVectorArrowLengthFraction(0, 1)
	nearZero := fieldVectorArrowLengthFraction(1e-12, 1)
	low := fieldVectorArrowLengthFraction(0.1, 1)
	mid := fieldVectorArrowLengthFraction(1, 1)
	high := fieldVectorArrowLengthFraction(3, 1)
	veryHigh := fieldVectorArrowLengthFraction(500, 1)
	if zero != 0 {
		t.Fatalf("zero field should have zero arrow length: %g", zero)
	}
	if !(zero <= nearZero && nearZero < low && low < mid && mid < high) {
		t.Fatalf("arrow length fraction does not increase from zero: zero=%g near=%g low=%g mid=%g high=%g", zero, nearZero, low, mid, high)
	}
	if high != veryHigh {
		t.Fatalf("arrow length fraction should cap: high=%g veryHigh=%g", high, veryHigh)
	}
	if high > fieldVectorArrowMaxLengthFraction+1e-9 {
		t.Fatalf("arrow length fraction exceeded cap: %g", high)
	}
	if low < fieldVectorArrowMinLengthFraction-1e-9 {
		t.Fatalf("arrow length fraction below minimum: %g", low)
	}
}

func TestFieldClampSceneVectorDensity132(t *testing.T) {
	low := fieldScene{Density: 10, VectorDensity: 0}
	fieldClampScene(&low)
	if low.VectorDensity != fieldVectorDensityMinTenths {
		t.Fatalf("low vector density clamp=%d", low.VectorDensity)
	}
	high := fieldScene{Density: 10, VectorDensity: 99}
	fieldClampScene(&high)
	if high.VectorDensity != fieldVectorDensityMaxTenths {
		t.Fatalf("high vector density clamp=%d", high.VectorDensity)
	}
}
