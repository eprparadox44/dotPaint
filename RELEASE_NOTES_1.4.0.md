# dotPaint v1.4.0 — Performance & Editing Stability / 최적화 및 편집 안정화

**정식 버전 / Stable release · 2026-11-06 · Windows x64**

---

## 한국어

dotPaint 1.4.0은 1.3 계열에서 축적된 성능 최적화와 안정성 개선을 통합하고, 텍스트 상자 편집 및 실행 취소 동작을 개선한 정식 버전입니다.

### 1. 각종 최적화 및 안정성 개선
- 그리기, 렌더링, 입력 및 편집 처리에 적용해 온 최적화 사항을 통합했습니다.
- 용량 절감을 위해 제한했던 Go 표준 라이브러리의 인라인 최적화를 정상 복구했습니다.
- 이미지 리소스를 무손실 재압축해 디코딩한 픽셀과 화질을 그대로 유지했습니다.
- 기존 수식, 함수 그래프, 3D·4D 도형 및 물리장 시각화 등 주요 기능을 삭제하지 않고 유지했습니다.

### 2. 텍스트 상자 버그 수정
- 글자 크기 직접 입력란과 드롭다운 목록이 겹치는 문제를 수정했습니다.
- 글자 크기를 직접 입력할 때 포커스와 입력값 확정 처리를 개선했습니다.
- 선택한 텍스트에 글꼴 및 크기 서식을 적용하는 동작을 개선했습니다.
- 텍스트 상자 편집 과정의 안정성을 강화했습니다.

### 3. 곡선 실행 취소 단계별 적용
- 곡선의 기준선과 각 꺾기 단계를 별도의 작업 기록으로 관리합니다.
- `Ctrl+Z`를 누르면 마지막 꺾기부터 순서대로 취소합니다.
- `Ctrl+Y`로 취소한 단계를 다시 실행할 수 있습니다.
- 기존 곡선의 최대 4회 꺾기 기능을 유지합니다.
- 앞 단계로 되돌아간 뒤 꺾기를 다시 지정할 수 있습니다.

### 4. 실행 취소 시 선택 색상 유지
- 선 그리기 등 캔버스 작업을 취소하거나 다시 실행해도 현재 선택한 전경색·배경색은 유지됩니다.
- 사용자 16색 팔레트의 선택 상태를 유지합니다.
- 그림을 다시 그릴 때 색상을 불필요하게 재선택해야 하는 문제를 해결했습니다.

### 다운로드 및 실행
- **지원 환경:** Windows x64 (64비트)
- **정식 실행파일:** `dotPaint_1.4.0_x64.exe`
- **실행파일 크기:** 10,253,312바이트
- 별도의 설치 과정 없이 실행할 수 있습니다.
- 공개 릴리즈에는 일반판 x64 실행파일만 포함하며 개발 로그판은 별도로 관리합니다.

### 검증 및 참고 사항
- 전체 Go 자동 테스트 통과
- Windows x64 교차 컴파일 통과
- 일반판 용량 상한 10,485,759바이트 충족
- Windows PC에서 최적화 복구판이 정상 실행됨을 사용자 확인
- 모든 GUI 조작을 독립적으로 검증한 것은 아니므로 일부 세부 기능은 추가 실사용 검증이 필요할 수 있습니다.

---

## English

dotPaint 1.4.0 is a stable release that consolidates performance optimizations and stability improvements from the 1.3 series, with refinements to text-box editing and undo behavior.

### 1. Performance Optimization and Stability
- Consolidated existing improvements to drawing, rendering, input handling, and editing.
- Restored standard Go library inlining optimizations that had been restricted to reduce executable size.
- Losslessly recompressed image resources, preserving the decoded pixels and image quality exactly.
- Retained major features without removing equation editing, function graphs, 3D/4D geometry, or physical-field visualization.

### 2. Text Box Bug Fixes
- Fixed the overlap between the direct font-size input field and its dropdown list.
- Improved focus handling and value confirmation when typing a font size directly.
- Improved the application of font-family and font-size formatting to selected text.
- Strengthened stability during text-box editing.

### 3. Step-by-Step Undo for Curves
- Records the initial curve baseline and each bend as separate history steps.
- `Ctrl+Z` removes the most recent bend first, one step at a time.
- `Ctrl+Y` reapplies an undone step.
- Retains support for up to four curve bends.
- Allows users to return to an earlier step and redefine subsequent bends.

### 4. Preserve Selected Colors During Undo
- Keeps the currently selected foreground and background colors when undoing or redoing canvas operations such as drawing a line.
- Preserves the selected state of the 16 custom palette colors.
- Eliminates the need to select the same color again when redrawing after an undo.

### Download and Run
- **Supported platform:** Windows x64 (64-bit)
- **Standard executable:** `dotPaint_1.4.0_x64.exe`
- **Executable size:** 10,253,312 bytes
- No installation is required; run the executable directly.
- The public release contains only the standard x64 executable. The development log build is maintained separately.

### Validation and Notes
- All Go automated tests passed.
- Windows x64 cross-compilation passed.
- The standard executable meets the 10,485,759-byte size limit.
- The user confirmed that the restored-optimization build runs normally on a Windows PC.
- Not every GUI interaction has been independently validated; some detailed functionality may require additional real-world testing.

---

**Developer / 개발자:** G.nerd (eprparadox44)  
**Source / 소스:** https://github.com/eprparadox44/dotPaint

dotPaint is an independent graphics editor inspired by classic Windows Paint and is not affiliated with Microsoft.  
dotPaint는 고전 Windows 그림판에서 영감을 받은 독립 그래픽 편집기이며 Microsoft의 공식 제품이 아닙니다.
