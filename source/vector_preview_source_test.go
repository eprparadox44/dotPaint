package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVectorPreviewUsesDirtyRegionInsteadOfFullCanvasClone(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("시험 파일 경로를 확인하지 못했습니다")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(here), "main_windows.go"))
	if err != nil {
		t.Fatalf("Windows 미리보기 소스를 읽지 못했습니다: %v", err)
	}
	source := string(data)
	checks := []string{
		"func (a *App) cloneRasterPreviewRegion",
		"func (a *App) blitRasterPreviewRegion",
		"func (a *App) drawRasterLineAt",
		"func (a *App) drawRasterArrowAt",
		`backend=CPU-DIRTY-REGION`,
	}
	for _, check := range checks {
		if !strings.Contains(source, check) {
			t.Fatalf("A209 벡터 미리보기 지역 렌더 표식이 없습니다: %s", check)
		}
	}

	for _, functionName := range []string{"paintRasterLinePreview", "paintRasterArrowPreview"} {
		startMarker := "func (a *App) " + functionName + "("
		start := strings.Index(source, startMarker)
		if start < 0 {
			t.Fatalf("%s 함수 본문을 찾지 못했습니다", functionName)
		}
		rest := source[start+len(startMarker):]
		end := strings.Index(rest, "\nfunc ")
		if end < 0 {
			end = len(rest)
		}
		body := rest[:end]
		if strings.Contains(body, "a.canvas.ClonePixels()") {
			t.Fatalf("%s가 전체 캔버스를 다시 복제합니다", functionName)
		}
		if !strings.Contains(body, "cloneRasterPreviewRegion") || !strings.Contains(body, "blitRasterPreviewRegion") {
			t.Fatalf("%s가 지역 복사/전송 경로를 사용하지 않습니다", functionName)
		}
	}
}

func TestVectorPreviewKeepsGlobalConstraintBeforeTranslation(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("시험 파일 경로를 확인하지 못했습니다")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(here), "main_windows.go"))
	if err != nil {
		t.Fatalf("Windows 미리보기 소스를 읽지 못했습니다: %v", err)
	}
	source := string(data)
	if !strings.Contains(source, "constraints in full-canvas coordinates") {
		t.Fatal("전체 캔버스 좌표에서 제약을 먼저 계산한다는 보존 표식이 없습니다")
	}
	if !strings.Contains(source, "translateArrowHeadGeometry") || !strings.Contains(source, "translateStrictTanShadow") {
		t.Fatal("화살촉 또는 strict tan 그림자 좌표 변환 함수가 없습니다")
	}
}
