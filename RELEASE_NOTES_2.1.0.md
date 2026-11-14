# dotPaint 2.1.0 — Drawing, Animation and Color Tools

## English

dotPaint 2.1.0 builds on the official [v2.0.1 release](https://github.com/eprparadox44/dotPaint/releases/tag/v2.0.1), which introduced persistent rasterization-mode preferences and bilingual license information. This release focuses on the drawing workflow, animation, image-format support, color adjustments, and UI refinements while retaining the editable-object and science-visualization tools.

### What's new since v2.0.1

- **Animation workflow:** GIF/ANI frame editing, a frame-list window, and synchronized Show/Hide controls in both the Animation and View/Window menus. Closing the panel also updates both menu checkmarks.
- **Image and cursor formats:** Extended file support for ICO, CUR, PCX, TGA, WebP and TIFF, alongside existing formats. Icon/cursor tools include multi-size handling and cursor hotspot editing.
- **Smudge tool:** Soft circular brush with adjustable size (1–40 px) and strength (1–100), defaulting to a gentler **30%** strength. A circle-only brush cursor and dedicated light/dark toolbox icons are provided.
- **Brightness / Contrast / Saturation:** Three adjustments in that order, each ranging from **−100 to +100**. Sliders and numeric input fields stay synchronized. Preview uses the original pixels; Cancel restores the image and OK creates one Undo step. Current implementation targets RGB raster pixels and respects selection masks and transparency without automatically rasterizing editable objects.
- **Improved curve anti-aliasing:** Subpixel-aware Bézier rendering improves smoothness, especially at small stroke widths; the original pixel-sharp mode remains available when AA is disabled.
- **New default RGB palette:** **28 standard colors plus 16 editable custom colors**, arranged with brighter pastels for manual anti-aliasing and vivid colors for drawing. Existing indexed/DejaVu palettes and palette values saved in DOP projects are preserved.
- **UI and workflow refinements:** Renamed View to **View/Window**; improved recent-file refresh across multiple windows; updated app/toolbox icons and selection cursors; removed the experimental label from the application title.
- **History and resource updates:** Up to **100 Undo steps**, subject to a shared **2 GiB** instance history budget. The embedded Earth texture retains its **2048×1024** resolution with 7-bit RGB precision, reducing its WebP asset size without downscaling.

### Downloads and notes

- `dotPaint_2.1.0_x64.exe` — standalone Windows x64 standard edition; no installer required.
- The original project code is MIT licensed. Third-party assets may have separate terms; see `THIRD_PARTY_NOTICES.md`.
- The developer diagnostic (`DEVLOG`) build is not part of the public release.
- Automated source tests and Windows x64 cross-compilation help validate this build; these do not replace real Windows GUI/GPU interaction checks.

## 한국어

dotPaint 2.1.0은 래스터화 모드 설정 기억과 한국어·영어 라이선스 안내를 도입한 [공식 v2.0.1 릴리즈](https://github.com/eprparadox44/dotPaint/releases/tag/v2.0.1)를 기반으로 한다. 기존 객체 편집·과학 시각화 기능은 유지하면서 그림 도구, 애니메이션, 이미지 형식, 색상 조절, 사용자 인터페이스를 개선했다.

### v2.0.1 이후 추가·개선 사항

- **애니메이션 편집:** GIF·ANI 프레임 편집과 프레임 목록 창을 지원한다. 애니메이션 메뉴와 보기/윈도우 메뉴에서 같은 창을 켜고 끌 수 있으며, 양쪽 체크 표시와 창의 닫기 버튼 상태가 연동된다.
- **이미지·커서 형식 확장:** 기존 형식 외에 ICO·CUR·PCX·TGA·WebP·TIFF 입출력을 확장했다. 아이콘·커서의 여러 크기와 커서 핫스팟 편집을 지원한다.
- **문지르기 도구:** 크기 **1~40 px**, 강도 **1~100%**의 부드러운 원형 붓을 추가했다. 기본 강도는 **30%**이며, 십자선 없는 원형 커서와 라이트·다크 모드 전용 도구 아이콘을 사용한다.
- **명도·대비·채도 조절:** 세 항목을 이 순서대로 각각 **−100~+100** 범위에서 조절한다. 슬라이더와 오른쪽 숫자 입력칸이 연동된다. 원본 기준 실시간 미리보기, 취소 시 원본 복원, 확인 시 Undo 한 단계 기록을 지원한다. 현재는 RGB 래스터 픽셀을 대상으로 하며 선택 마스크·투명도를 보호하고 편집 객체를 자동 래스터화하지 않는다.
- **곡선 안티에일리어싱 개선:** 소수점 좌표를 보존하는 베지어 곡선 렌더링으로 가는 선의 계단 현상을 줄였다. AA를 끄면 기존 도트형 결과를 유지한다.
- **새 기본 RGB 색상표:** 기본 **28색 + 편집 가능한 사용자 16색**으로 구성했다. 수동 AA에 쓰기 좋은 밝은 파스텔과 선명한 원색 위주로 정리했으며, 기존 인덱스·DejaVu 팔레트와 DOP 프로젝트에 저장된 팔레트는 유지한다.
- **화면·작업 흐름 개선:** 상단 메뉴를 **보기/윈도우**로 명확히 하고, 여러 창의 최근 파일 목록 갱신을 개선했다. 앱·도구 아이콘과 선택 커서를 정리했으며 프로그램 제목에서 시험판 표기를 제거했다.
- **히스토리·리소스 개선:** 인스턴스별 공유 메모리 예산 **2 GiB** 내에서 최대 **100회 Undo**를 지원한다. 내장 지구 텍스처는 **2048×1024** 해상도를 유지하면서 RGB 7비트 정밀도를 사용해 WebP 용량을 줄였다.

### 다운로드 및 참고 사항

- `dotPaint_2.1.0_x64.exe` — 설치가 필요 없는 Windows x64 일반판.
- 프로그램 자체 소스는 MIT 라이선스이며, 제3자 자료에는 별도 조건이 적용될 수 있다. `THIRD_PARTY_NOTICES.md`를 참고한다.
- 개발 로그판(`DEVLOG`)은 일반 공개 릴리즈에 포함하지 않는다.
- 자동 테스트와 Windows x64 크로스 빌드는 실시하지만, 실제 Windows GUI·GPU 동작 검증을 완전히 대체하지는 않는다.
