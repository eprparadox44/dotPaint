# dotPaint 2.1.0 — Drawing, Animation and Color Tools

## English

dotPaint 2.1.0 builds on the official [v2.0.1 release](https://github.com/eprparadox44/dotPaint/releases/tag/v2.0.1), which introduced persistent rasterization-mode preferences and bilingual license information. This release improves drawing tools, animation editing, image-format support, color adjustments, and the interface while retaining the existing editable-object and science-visualization features.

### What's new since v2.0.1

- **Animation editing:** Added GIF/ANI frame editing and a frame-list window. The same window can be shown or hidden from the Animation and View/Window menus; both menu checkmarks stay synchronized with the window's close-button state.
- **Image and cursor format support:** Expanded import and export support for ICO, CUR, PCX, TGA, WebP, and TIFF in addition to the existing formats. Added multi-size icon/cursor handling and cursor hotspot editing.
- **Smudge tool:** Added a smudge tool with adjustable size (**1–40 px**) and strength (**1–100%**). The default strength is **30%**.
- **Brightness, contrast, and saturation:** Adjust these three settings, in that order, from **−100 to +100**. Each slider is synchronized with the numeric input field on its right. The feature supports real-time preview based on the original image, full restoration on Cancel, and a single Undo step on OK. It currently applies to RGB raster pixels, preserves selection masks and transparency, and does not automatically rasterize editable objects.
- **Curve anti-aliasing fix:** Fixed curve anti-aliasing so it now actually works; previously, the UI option existed but the feature did not function.
- **New default RGB palette:** Reorganized the default palette around brighter pastel shades and vivid primary colors.
- **Interface and workflow refinement:** Renamed the top-level **View** menu to **View/Window**.
- **History and resource improvements:** Supports up to **100 Undo steps** within a shared **2 GiB** memory budget per application instance. The embedded Earth texture retains its **2048×1024** resolution while using 7-bit RGB precision to reduce its WebP file size.

### Downloads and notes

- `dotPaint_2.1.0_x64.exe` — standalone Windows x64 standard edition; no installer required.
- The original project code is MIT licensed. Third-party assets may have separate terms; see `THIRD_PARTY_NOTICES.md`.
- The developer diagnostic (`DEVLOG`) build is not part of the public release.
- Automated source tests and Windows x64 cross-compilation help validate this build; these do not fully replace real Windows GUI/GPU interaction checks.

## 한국어

dotPaint 2.1.0은 래스터화 모드 설정 기억과 한국어·영어 라이선스 안내를 도입한 [공식 v2.0.1 릴리즈](https://github.com/eprparadox44/dotPaint/releases/tag/v2.0.1)를 기반으로 한다. 기존 객체 편집·과학 시각화 기능은 유지하면서 그림 도구, 애니메이션, 이미지 형식, 색상 조절, 사용자 인터페이스를 개선했다.

### v2.0.1 이후 추가·개선 사항

- **애니메이션 편집:** GIF·ANI 프레임 편집과 프레임 목록 창을 지원한다. 애니메이션 메뉴와 보기/윈도우 메뉴에서 같은 창을 켜고 끌 수 있으며, 양쪽 체크 표시와 창의 닫기 버튼 상태가 연동된다.
- **이미지·커서 형식 확장:** 기존 형식 외에 ICO·CUR·PCX·TGA·WebP·TIFF 입출력을 확장했다. 아이콘·커서의 여러 크기와 커서 핫스팟 편집을 지원한다.
- **문지르기 도구:** 크기 **1~40 px**, 강도 **1~100%**으로 조정되는 문지르기 툴을 추가했다. 기본 강도는 **30%**이다.
- **명도·대비·채도 조절:** 세 항목을 이 순서대로 각각 **−100~+100** 범위에서 조절한다. 슬라이더와 오른쪽 숫자 입력칸이 연동된다. 원본 기준 실시간 미리보기, 취소 시 원본 복원, 확인 시 Undo 한 단계 기록을 지원한다. 현재는 RGB 래스터 픽셀을 대상으로 하며 선택 마스크·투명도를 보호하고 편집 객체를 자동 래스터화하지 않는다.
- **곡선 안티에일리어싱 개선:** ui만 있던 곡선의 안티에일리어싱을 실제로 작동하도록 수정했다.
- **새 기본 RGB 색상표:** 기본 팔레트를 밝은 파스텔과 선명한 원색 위주로 정리했다.
- **화면·작업 흐름 개선:** 상단 메뉴 명칭을 **보기/윈도우**로 변경했다.
- **히스토리·리소스 개선:** 인스턴스별 공유 메모리 예산 **2 GiB** 내에서 최대 **100회 Undo**를 지원한다. 내장 지구 텍스처는 **2048×1024** 해상도를 유지하면서 RGB 7비트 정밀도를 사용해 WebP 용량을 줄였다.

### 다운로드 및 참고 사항

- `dotPaint_2.1.0_x64.exe` — 설치가 필요 없는 Windows x64 일반판.
- 프로그램 자체 소스는 MIT 라이선스이며, 제3자 자료에는 별도 조건이 적용될 수 있다. `THIRD_PARTY_NOTICES.md`를 참고한다.
- 개발 로그판(`DEVLOG`)은 일반 공개 릴리즈에 포함하지 않는다.
- 자동 테스트와 Windows x64 크로스 빌드는 실시하지만, 실제 Windows GUI·GPU 동작 검증을 완전히 대체하지는 않는다.
