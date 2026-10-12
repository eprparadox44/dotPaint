package main

import (
	"strings"
	"testing"
)

func TestFieldFinal3DLinePreviewRequestsPreciseRetrace134(t *testing.T) {
	scene := fieldDefaultScene()
	scene.Dimension = fieldDimension3D

	for _, display := range []fieldDisplayMode{fieldDisplayElectricLines, fieldDisplayMagneticLines, fieldDisplayGravityLines} {
		scene.Display = display
		fast := fieldPreviewSceneForInteraction(scene, true)
		if fast.precise3DLines {
			t.Fatalf("interactive display=%d unexpectedly enabled precise 3-D lines", display)
		}
		final := fieldPreviewSceneForInteraction(scene, false)
		if !final.precise3DLines {
			t.Fatalf("final display=%d did not enable precise 3-D lines", display)
		}
	}

	scene.Display = fieldDisplayElectricVectors
	if fieldPreviewSceneForInteraction(scene, false).precise3DLines {
		t.Fatal("vector display should not request streamline retracing")
	}
	scene.Dimension = fieldDimension2D
	scene.Display = fieldDisplayElectricLines
	if fieldPreviewSceneForInteraction(scene, false).precise3DLines {
		t.Fatal("2-D field lines should retain the established fast evaluator")
	}
}

func TestFieldPrecise3DLineCacheKeyIsDistinct134(t *testing.T) {
	scene := fieldDefaultScene()
	scene.Dimension = fieldDimension3D
	scene.Display = fieldDisplayElectricLines
	fast := fieldElectricLineSceneKey(scene, nil)
	scene.precise3DLines = true
	precise := fieldElectricLineSceneKey(scene, nil)
	if fast == precise {
		t.Fatal("precise 3-D electric lines share the interactive cache key")
	}

	scene.Display = fieldDisplayMagneticLines
	scene.precise3DLines = false
	fast = fieldMagneticLineSceneKey(scene)
	scene.precise3DLines = true
	precise = fieldMagneticLineSceneKey(scene)
	if fast == precise {
		t.Fatal("precise 3-D magnetic lines share the interactive cache key")
	}
}

func TestFieldPrecise3DElectricEvaluatorBypassesGrid134(t *testing.T) {
	scene := fieldDefaultScene()
	scene.Dimension = fieldDimension3D
	scene.Display = fieldDisplayElectricLines
	scene.precise3DLines = true
	eval, backend := fieldElectricVectorEvaluator(scene, nil)
	if eval == nil || !strings.Contains(backend, "CPU-DIRECT-PRECISE-3D") {
		t.Fatalf("backend=%q want precise direct evaluator", backend)
	}
}

func TestField3DLineDepthAlphaMakesNearerLinesStronger134(t *testing.T) {
	far := field3DLineAlpha(-4, 6)
	middle := field3DLineAlpha(0, 6)
	near := field3DLineAlpha(4, 6)
	if !(far < middle && middle < near) {
		t.Fatalf("depth alpha not monotonic: far=%g middle=%g near=%g", far, middle, near)
	}
	if far < .20 || near > .96 {
		t.Fatalf("depth alpha escaped readable range: far=%g near=%g", far, near)
	}
}

func TestFieldPrecisePoliciesAreTighter134(t *testing.T) {
	fast := fieldDefaultScene()
	fast.Dimension = fieldDimension3D
	fast.Display = fieldDisplayMagneticLines
	precise := fast
	precise.precise3DLines = true
	a := fieldMagneticStreamlinePolicy(fast)
	b := fieldMagneticStreamlinePolicy(precise)
	if !(b.MaxStep < a.MaxStep && b.MinStep < a.MinStep && b.AbsTolerance < a.AbsTolerance && b.MaxTurnRadians < a.MaxTurnRadians && b.MaxSteps > a.MaxSteps) {
		t.Fatalf("precise magnetic policy not tighter: fast=%+v precise=%+v", a, b)
	}
}
