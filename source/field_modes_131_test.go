package main

import (
	"math"
	"testing"
)

func TestFieldDisplayDomainSeparation(t *testing.T) {
	for _, mode := range []fieldDisplayMode{fieldDisplayEquipotential, fieldDisplayElectricLines, fieldDisplayElectricVectors} {
		if !mode.electric() || mode.magnetic() {
			t.Fatalf("electric mode misclassified: %v", mode)
		}
	}
	for _, mode := range []fieldDisplayMode{fieldDisplayMagneticLines, fieldDisplayMagneticVectors} {
		if !mode.magnetic() || mode.electric() {
			t.Fatalf("magnetic mode misclassified: %v", mode)
		}
	}
}

func TestFieldPolyhedronPointChargeLayoutsAreVolumetric(t *testing.T) {
	cases := []struct {
		layout int
		count  int
	}{
		{fieldSpecialLayoutTetrahedron, 4},
		{fieldSpecialLayoutOctahedron, 6},
		{fieldSpecialLayoutCube, 8},
	}
	for _, tc := range cases {
		objects := fieldPointChargePolyhedron(tc.layout)
		if len(objects) != tc.count {
			t.Fatalf("layout %d: got %d vertices, want %d", tc.layout, len(objects), tc.count)
		}
		seenY := false
		for i, o := range objects {
			if o.Kind != fieldObjectSphere || !o.Volumetric || o.Material != fieldMaterialInsulator || o.Charge != 1 {
				t.Fatalf("layout %d vertex %d has wrong point-charge metadata: %+v", tc.layout, i, o)
			}
			if math.Abs(o.Y) > 1e-9 {
				seenY = true
			}
			r := fieldObjectCenter(o).len()
			if math.Abs(r-1.38) > 1e-9 {
				t.Fatalf("layout %d vertex %d radius=%g", tc.layout, i, r)
			}
		}
		if !seenY {
			t.Fatalf("layout %d is still planar", tc.layout)
		}
	}
}

func TestFieldClampPreservesOnlyExplicitVolumetricY(t *testing.T) {
	planar := fieldDefaultObject(fieldObjectSphere)
	planar.Y = 1.2
	volumetric := fieldDefaultObject(fieldObjectSphere)
	volumetric.Y = -1.2
	volumetric.Volumetric = true
	s := fieldScene{Objects: []fieldObject{planar, volumetric}, Density: 10}
	fieldClampScene(&s)
	if s.Objects[0].Y != 0 {
		t.Fatalf("ordinary object escaped planar placement: %g", s.Objects[0].Y)
	}
	if math.Abs(s.Objects[1].Y+1.2) > 1e-9 {
		t.Fatalf("volumetric point charge lost Y: %g", s.Objects[1].Y)
	}
}

func TestFerromagnetIsPassiveAndInducedByExternalField(t *testing.T) {
	iron := fieldDefaultObject(fieldObjectFerromagnet)
	iron.X = 1.25
	iron.Radius = .52
	iron.Charge = 2.5
	passive := fieldBuildMagneticModel(fieldScene{Objects: []fieldObject{iron}, Density: 10})
	if len(passive.FiniteCurrents) != 0 || len(passive.Magnets) != 0 || len(passive.Wires) != 0 {
		t.Fatalf("isolated soft ferromagnet created remanent field: %+v", passive)
	}

	bar := fieldDefaultObject(fieldObjectBarMagnet)
	bar.X = -1.0
	bar.Length = 1.5
	bar.Charge = 2
	base := fieldBuildMagneticModel(fieldScene{Objects: []fieldObject{bar}, Density: 10})
	withIron := fieldBuildMagneticModel(fieldScene{Objects: []fieldObject{bar, iron}, Density: 10})
	if len(withIron.FiniteCurrents) <= len(base.FiniteCurrents) {
		t.Fatalf("ferromagnet did not add induced bound currents: base=%d with=%d", len(base.FiniteCurrents), len(withIron.FiniteCurrents))
	}
	probe := fieldVec3{X: .55, Y: .35, Z: .2}
	if withIron.at(probe).sub(base.at(probe)).len() < 1e-6 {
		t.Fatal("ferromagnet did not distort the external magnetic field")
	}
}

func TestVectorFieldModesRenderGridArrows(t *testing.T) {
	charge := fieldDefaultObject(fieldObjectSphere)
	charge.Charge = 2
	electric := fieldScene{Dimension: fieldDimension2D, Display: fieldDisplayElectricVectors, Objects: []fieldObject{charge}, Density: 8}
	r := fieldRender(electric, 320, 240, 0)
	if fieldRasterVisiblePixelCount(r) < 100 {
		t.Fatalf("electric vector field rendered too little: %d", fieldRasterVisiblePixelCount(r))
	}

	wire := fieldDefaultObject(fieldObjectCurrentWire)
	wire.Charge = 2
	magnetic := fieldScene{Dimension: fieldDimension2D, Display: fieldDisplayMagneticVectors, Objects: []fieldObject{wire}, Density: 8}
	r = fieldRender(magnetic, 320, 240, 0)
	if fieldRasterVisiblePixelCount(r) < 100 {
		t.Fatalf("magnetic vector field rendered too little: %d", fieldRasterVisiblePixelCount(r))
	}
}
