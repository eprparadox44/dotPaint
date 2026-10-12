package main

import (
	"os"
	"strings"
	"testing"
)

func TestRelease125FieldEditorSourceMarkers(t *testing.T) {
	coreBytes, err := os.ReadFile("field_core.go")
	if err != nil {
		t.Fatal(err)
	}
	core := string(coreBytes)
	for _, token := range []string{
		"fieldObjectSphere",
		"fieldObjectRod",
		"fieldObjectPlane",
		"fieldObjectHorseshoeMagnet",
		"fieldObjectCurrentLoop",
		"fieldMaterialInsulator",
		"fieldMaterialConductor",
		"fieldSolveLinear",
		"A[i][unknown+ci] = -1",
		"o.Y = 0 // planar placement",
		"fieldScreenToWorldPlanar",
		"fieldRenderForBackground",
		"fieldFluxLineCounts",
		"fieldVisibleContourPieces",
		"fieldConductorCoversProjection",
		"fieldClipSegmentOutsideConductors",
		"fieldElectricAt",
		"fieldPotentialAt",
		"fieldNumericalInfinityRadius",
		"fieldDownsampleRaster",
		"fieldRodLengthMax = 27.5",
		"fieldPlaneSizeMax = 24.5",
		"fieldWidePlateImageSamples",
		"fieldEquipotentialSamplingSpan",
		"fieldFilterClosedIsoComponents",
		"fieldFilterStableIsoComponents",
		"fieldInsulatingPlaneSurface",
		"fieldEquipotentialGridCenter",
		"fieldPotentialBandsFromGrid",
		"fieldPotentialSurfaceLevels",
		"fieldRenderPotentialVolume",
		"ZeroAnchored",
		"fieldFilterLargeOpenIsoComponents",
		"fieldPositiveEmissionSeedGroups",
		"Clearance: clearance",
		"div(E)=rho/epsilon0",
	} {
		if !strings.Contains(core, token) {
			t.Fatalf("field core missing %q", token)
		}
	}
	for _, forbidden := range []string{"fieldObjectPositiveSphere", "fieldObjectNegativeSphere", "fieldObjectPositiveRod", "fieldObjectNegativeRod"} {
		if strings.Contains(core, forbidden) {
			t.Fatalf("separate charge-sign object remains: %q", forbidden)
		}
	}

	uiBytes, err := os.ReadFile("field_editor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	ui := string(uiBytes)
	for _, token := range []string{
		"multidimSliderSet(s.charge, -40, 40, 10)",
		"절연체: 고정 균일 전하",
		"도체: 정전기 유도 계산",
		"fieldSetText(s.sizeLabel, \"반지름\")",
		"fieldSetText(s.sizeLabel, \"길이\")",
		"fieldSetText(s.sizeLabel, \"면 크기\")",
		"말굽자석",
		"원형 고리 전류",
		"fieldSetText(s.sizeLabel, \"고리 반지름\")",
		"fieldRenderForBackground(snapshot, width, height, 2, backgroundBlack)",
		"물체 자체 x 자전",
		"procEnableWindow.Call(s.objectRotX, enable)",
		"procEnableWindow.Call(s.objectRotY, enable)",
		"procEnableWindow.Call(s.objectRotZ, enable)",
		"procEnableWindow.Call(h, 1)",
		"부드럽게 (6×)",
		"fieldExtendedSizeSliderPosition",
		"fieldExtendedSizeFromSlider",
		"fieldPreviewTimerID",
		"procCreateCompatibleBitmap.Call",
		"Render a coalesced 1x physical preview",
		"fieldPreviewSceneForInteraction",
		"fieldPreviewResultCurrentEnough",
		"fieldCommitFinalPreview",
		"fieldInstallReleasePreview",
		"fieldPreviewReasonMustCommit",
		"Provisional: provisional",
		"object-drag-end",
		"capture-changed",
		"fieldEquipotentialPreviewUsable",
		"FIELD_PREVIEW final-retry",
		"fieldInsertDoneMessage",
		"fieldFinishAsyncInsert",
		"insertBusy",
	} {
		if !strings.Contains(ui, token) {
			t.Fatalf("field editor missing %q", token)
		}
	}

	mainBytes, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	mainSrc := string(mainBytes)
	for _, token := range []string{
		`const appVersion = "1.2.38"`,
		`ID_INSERT_IMAGE_FIELD`,
		`"장 그리기(&F)"`,
		`a.showFieldEditor()`,
	} {
		if !strings.Contains(mainSrc, token) {
			t.Fatalf("main source missing %q", token)
		}
	}
}
