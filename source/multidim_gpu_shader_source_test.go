package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestD3D11BlackHoleShaderAvoidsReservedTextureIdentifier(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("시험 파일 경로를 확인하지 못했습니다")
	}
	sourcePath := filepath.Join(filepath.Dir(here), "multidim_gpu_d3d11_windows.go")
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("GPU 소스를 읽지 못했습니다: %v", err)
	}
	source := string(data)
	reservedDeclaration := regexp.MustCompile(`(?m)\b(?:float|float[234]|int|uint|bool)\s+texture\b`)
	if reservedDeclaration.MatchString(source) {
		t.Fatal("HLSL 예약어 texture가 변수 이름으로 다시 사용되었습니다")
	}
	if !strings.Contains(source, "float diskModulation=") || !strings.Contains(source, "1.18*diskModulation*radialGlow") {
		t.Fatal("블랙홀 강착원반 변조 변수 수정이 소스에 유지되지 않았습니다")
	}
}

func TestD3D11PipelinesRemainIndependent(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("시험 파일 경로를 확인하지 못했습니다")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(here), "multidim_gpu_d3d11_windows.go"))
	if err != nil {
		t.Fatalf("GPU 소스를 읽지 못했습니다: %v", err)
	}
	source := string(data)
	checks := map[string]string{
		"blackhole state":        `(?m)\bblackHoleReady\s+bool\b`,
		"celestial state":        `(?m)\bcelestialReady\s+bool\b`,
		"face state":             `(?m)\bfaceReady\s+bool\b`,
		"occlusion state":        `(?m)\bocclusionReady\s+bool\b`,
		"field state":            `(?m)\bfieldReady\s+bool\b`,
		"blackhole availability": `func mdD3D11BlackHoleAvailable\(\) bool`,
		"celestial availability": `func mdD3D11CelestialAvailable\(\) bool`,
		"face availability":      `func mdD3D11FaceAvailable\(\) bool`,
		"occlusion availability": `func mdD3D11OcclusionAvailable\(\) bool`,
		"field availability":     `func mdD3D11FieldAvailable\(\) bool`,
	}
	for name, pattern := range checks {
		if !regexp.MustCompile(pattern).MatchString(source) {
			t.Fatalf("독립 GPU 파이프라인 표식이 없습니다: %s", name)
		}
	}
}

func TestD3D11CelestialShaderAvoidsReservedPointIdentifier(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("시험 파일 경로를 확인하지 못했습니다")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(here), "multidim_gpu_d3d11_windows.go"))
	if err != nil {
		t.Fatalf("GPU 소스를 읽지 못했습니다: %v", err)
	}
	source := string(data)
	reservedParameter := regexp.MustCompile(`(?m)\b(?:float|float[234]|int|uint|bool)\s+point\b`)
	if reservedParameter.MatchString(source) {
		t.Fatal("HLSL 예약어 point가 변수 또는 매개변수 이름으로 다시 사용되었습니다")
	}
	if !strings.Contains(source, "float saturnRingShadow(float3 surfacePoint)") ||
		!strings.Contains(source, "float t=-surfacePoint.z/lightLocal.z") ||
		!strings.Contains(source, "float3 q=surfacePoint+lightLocal*t") {
		t.Fatal("토성 고리 그림자 함수의 surfacePoint 수정이 소스에 유지되지 않았습니다")
	}
}

func TestD3D11OcclusionShaderAndFullQualityRotationPolicy(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("시험 파일 경로를 확인하지 못했습니다")
	}
	dir := filepath.Dir(here)
	gpuData, err := os.ReadFile(filepath.Join(dir, "multidim_gpu_d3d11_windows.go"))
	if err != nil {
		t.Fatalf("GPU 소스를 읽지 못했습니다: %v", err)
	}
	gpu := string(gpuData)
	for _, marker := range []string{
		"const mdD3D11OcclusionShader",
		"RWStructuredBuffer<uint> Counts",
		"InterlockedAdd(Counts[pair.QueryIndex], 1)",
		"func mdD3D11RenderOcclusionCounts",
		"shape=%s backend=D3D11 queries=%d pairs=%d",
	} {
		if !strings.Contains(gpu, marker) {
			t.Fatalf("GPU 가림 계산 표식이 없습니다: %s", marker)
		}
	}
	editorData, err := os.ReadFile(filepath.Join(dir, "multidim_shape_editor_windows.go"))
	if err != nil {
		t.Fatalf("다차원 렌더러 소스를 읽지 못했습니다: %v", err)
	}
	editor := string(editorData)
	for _, marker := range []string{
		"return multidimEditorSmoothEnabled(s)",
		"return multidimOpacityIndex(s)",
		"mdSphereMeshSteps(smooth, false, guideMode)",
		"mdTorusMeshSteps(smooth, false)",
		"mdMobiusMeshSteps(smooth, false)",
		"mdCollect4DEdgesWithFaceFadeGPU",
		"mdCollectParametricSurfaceLinesGPU",
		"translucent-4d-faces",
		"translucent-parametric-faces",
	} {
		if !strings.Contains(editor, marker) {
			t.Fatalf("회전 중 완전 품질/GPU 표식이 없습니다: %s", marker)
		}
	}
}

func TestD3D11EarthLongitudeMatchesCPUAndExternalSkinSupport(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("시험 파일 경로를 확인하지 못했습니다")
	}
	dir := filepath.Dir(here)
	gpuData, err := os.ReadFile(filepath.Join(dir, "multidim_gpu_d3d11_windows.go"))
	if err != nil {
		t.Fatalf("GPU 소스를 읽지 못했습니다: %v", err)
	}
	gpu := string(gpuData)
	if !strings.Contains(gpu, "float lon=Skin==1 ? wrapAngle(128.0*PI/180.0+base) : wrapAngle(base);") {
		t.Fatal("D3D11 지구 경도 방향이 A211 외부 스킨 방향과 일치하지 않습니다")
	}
	assetData, err := os.ReadFile(filepath.Join(dir, "multidim_celestial_assets.go"))
	if err != nil {
		t.Fatalf("천체 자산 소스를 읽지 못했습니다: %v", err)
	}
	assets := string(assetData)
	for _, marker := range []string{"skins", "earth.png", "mdEarthTextureSource = \"external\""} {
		if !strings.Contains(assets, marker) {
			t.Fatalf("사용자 지구 스킨 지원 표식이 없습니다: %s", marker)
		}
	}
}

func TestD3D11EarthDetailPreservationUsesLoadedPNGNotFixedColoursA214(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("시험 파일 경로를 확인하지 못했습니다")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(here), "multidim_gpu_d3d11_windows.go"))
	if err != nil {
		t.Fatalf("GPU 소스를 읽지 못했습니다: %v", err)
	}
	source := string(data)
	for _, marker := range []string{"EarthTex.Load(int3(px,py,0)).rgb", "earthDetailLandScore", "land=earthDetailColour", "rgba_sha256=%s", "detail_rgb=%d,%d,%d/%d,%d,%d"} {
		if !strings.Contains(source, marker) {
			t.Fatalf("A214 PNG 세부 보존 표식이 없습니다: %s", marker)
		}
	}
	for _, forbidden := range []string{"float3(124,119,66)/255.0", "float3(142,126,72)/255.0"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("A213 고정색 덮어쓰기가 남아 있습니다: %s", forbidden)
		}
	}
}

func TestD3D11BlackHoleShaderContainsKerrGeodesicsAndSpinConstantsA215(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("시험 파일 경로를 확인하지 못했습니다")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(here), "multidim_gpu_d3d11_windows.go"))
	if err != nil {
		t.Fatalf("GPU 소스를 읽지 못했습니다: %v", err)
	}
	source := string(data)
	for _, marker := range []string{
		"float Spin; float KerrHorizon; float KerrDiskInner",
		"void kerrPotentials",
		"float3 kerrDerivative",
		"float3 traceKerrColor",
		"Spin<1e-5?traceSchwarzschildColor(dir,n):traceKerrColor(dir,n)",
		"mdBHKerrHorizon(params.spin)",
		"mdBHKerrISCO(params.spin)",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("A215 Kerr GPU 표식이 없습니다: %s", marker)
		}
	}
}

func TestBlackHoleEditorReusesDisabledSlidersForDensityAndKerrSpinA215(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("시험 파일 경로를 확인하지 못했습니다")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(here), "multidim_shape_editor_windows.go"))
	if err != nil {
		t.Fatalf("편집기 소스를 읽지 못했습니다: %v", err)
	}
	source := string(data)
	for _, marker := range []string{"좌표망 밀도", "커 회전량 a*", "multidimBlackHoleGridLines(s)", "multidimBlackHoleSpin(s)", "widthMin, widthMax, widthPos = 1, 8, 2", "heightMin, heightMax, heightPos = 0, 99, 0"} {
		if !strings.Contains(source, marker) {
			t.Fatalf("A215 블랙홀 슬라이더 표식이 없습니다: %s", marker)
		}
	}
}

func TestD3D11FieldPotentialGridPipeline125(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("시험 파일 경로를 확인하지 못했습니다")
	}
	dir := filepath.Dir(here)
	gpuData, err := os.ReadFile(filepath.Join(dir, "multidim_gpu_d3d11_windows.go"))
	if err != nil {
		t.Fatalf("GPU 소스를 읽지 못했습니다: %v", err)
	}
	gpu := string(gpuData)
	for _, marker := range []string{
		"const mdD3D11FieldPotentialShader",
		"StructuredBuffer<ChargeSample> Samples",
		"RWStructuredBuffer<float> Potentials",
		"potential += s.ChargeGateXY.x / r",
		"GateZEnabledPad.y > 0.5",
		"func mdD3D11RenderFieldPotentialGrid",
		"FIELD_GPU potential backend=D3D11",
		"func mdD3D11RenderFieldMembraneTriangles",
		"CreateBlendState(field-membrane)",
		"FIELD_GPU membranes backend=D3D11",
		"mdD3D11PrewarmField",
	} {
		if !strings.Contains(gpu, marker) {
			t.Fatalf("1.2.5 전위격자 GPU 표식이 없습니다: %s", marker)
		}
	}
	coreData, err := os.ReadFile(filepath.Join(dir, "field_core.go"))
	if err != nil {
		t.Fatalf("장 계산 소스를 읽지 못했습니다: %v", err)
	}
	core := string(coreData)
	for _, marker := range []string{
		"fieldTryGPUPotentialGrid(samples, steps, minCoord, maxCoord)",
		"backend = \"D3D11\"",
		"fieldConductorPotentialValues",
		"fieldRecordEquipotentialTiming",
		"fieldTryGPUMembraneRaster",
		"fieldBuildMembraneCoverIndex",
	} {
		if !strings.Contains(core, marker) {
			t.Fatalf("1.2.5 장 GPU 연결 표식이 없습니다: %s", marker)
		}
	}
}

func TestD3D11FieldVectorGridPipeline128(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("시험 파일 경로를 확인하지 못했습니다")
	}
	dir := filepath.Dir(here)
	gpuData, err := os.ReadFile(filepath.Join(dir, "multidim_gpu_d3d11_windows.go"))
	if err != nil {
		t.Fatalf("GPU 소스를 읽지 못했습니다: %v", err)
	}
	gpu := string(gpuData)
	for _, marker := range []string{
		"const mdD3D11FieldVectorShader",
		"StructuredBuffer<CurrentSegment> Segments",
		"StructuredBuffer<WireSample> Wires",
		"RWStructuredBuffer<float4> Vectors",
		"cross(s.DLCurrent.xyz, r)",
		"cross(axis, perp)",
		"func mdD3D11RenderFieldVectorGrid",
		"FIELD_GPU %s-vector backend=D3D11",
		"fieldVectorReady",
	} {
		if !strings.Contains(gpu, marker) {
			t.Fatalf("1.2.8 장 벡터 GPU 표식이 없습니다: %s", marker)
		}
	}
	bridgeData, err := os.ReadFile(filepath.Join(dir, "field_vector_grid_scene.go"))
	if err != nil {
		t.Fatalf("벡터 격자 연결 소스를 읽지 못했습니다: %v", err)
	}
	bridge := string(bridgeData)
	for _, marker := range []string{
		"fieldTryGPUElectricVectorGrid",
		"fieldTryGPUMagneticVectorGrid",
		"grid.evaluator(direct, near",
		"D3D11-GRID-CACHE",
		"fieldVectorGridDirectDistance",
	} {
		if !strings.Contains(bridge, marker) {
			t.Fatalf("1.2.8 장 벡터 연결 표식이 없습니다: %s", marker)
		}
	}
}
