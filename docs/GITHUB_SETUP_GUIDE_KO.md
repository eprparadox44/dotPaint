# dotPaint v1.3.3 GitHub 업로드 가이드

## 1단계: 로컬 GitHub 저장소 확인

GitHub Desktop에서 저장소가 `eprparadox44/dotPaint`, 브랜치가 `main`인지 확인합니다. `Repository > Show in Explorer`를 누릅니다.

## 2단계: 이번 ZIP 내용을 로컬 저장소에 복사

이 ZIP의 **루트 내용** (`source/`, `README.md`, `LICENSE`, `.github/`, `docs/`, `build_1.3.3.bat` 등)을 로컬 `dotPaint` 저장소 폴더에 복사합니다. 같은 이름의 **source 파일은 덮어씁니다**. GitHub용 ZIP 자체는 저장소에 넣지 않습니다.

`.github`와 `.gitignore`처럼 점으로 시작하는 파일도 복사되어야 합니다. Windows 탐색기에서 압축 해제한 뒤 폴더를 열어 전체 선택·복사하면 됩니다.

## 3단계: 커밋 + 푸시

GitHub Desktop Changes에서 소스 코드 변경과 README/LICENSE 등을 확인합니다. Commit summary: `Update to v1.3.3 and add project documentation`. `Commit to main` 클릭 후 상단의 `Push origin` 클릭. 원격으로 푸시하기 전에는 GitHub 사이트에 반영되지 않습니다.

## 4단계: 소스/README/MIT/Actions 확인

`https://github.com/eprparadox44/dotPaint`에서 README와 LICENSE가 보이는지 확인합니다. `Actions` 탭에서 `Build and test dotPaint`가 녹색 PASS인지 확인합니다. CI 성공은 자동 테스트·컴파일 성공이지 Windows GUI 실기 시험을 의미하지 않습니다.

## 5단계: 스크린샷 업로드

실제 dotPaint v1.3.3에서 Windows `Win + Shift + S`로 화면 캡처 → `docs/screenshots/`에 `main-dark.png`, `main-light.png`, `stretch-skew.png` 등을 저장. 저장소 루트 `README.md`의 주석으로 막아 둔 예시 이미지 링크를 활성화하면 GitHub 소개 화면에 이미지가 나타납니다.

**주의:** 학생/개인정보, 파일 경로, 민감한 창 제목이 캡처에 포함되지 않게 확인하세요.

## 6단계: GitHub Pages 소개 홈페이지 만들기

GitHub 웹 저장소에서 `Settings > Pages > Build and deployment`로 이동합니다. `Source: Deploy from a branch`, `Branch: main`, `Folder: /docs`, `Save`를 선택합니다. 게시 후 `https://eprparadox44.github.io/dotPaint/`에 소개 페이지가 열립니다. Windows EXE 자체가 웹에서 돌아가는 것은 아닙니다.

## 7단계: 1.3.3 새 Release 발행

`Releases > Draft a new release` 클릭 → Tag `v1.3.3` (대상 main), 제목 `dotPaint 1.3.3`. `dotPaint_1.3.3_Windows_Release_2026-10-13.zip`를 Assets 영역에 업로드하거나 두 일반판 EXE를 별도 첨부합니다. `Pre-release`는 선택하지 않고, 파일 업로드 완료 후 Publish release 클릭.

**중요:** 이 GitHub용 소스팩에는 Windows 실행파일이 포함되지 않습니다. 실행파일은 기존에 제작된 정식 Release ZIP 또는 별도 EXE 파일을 사용하세요.

## 8단계: 소개 문구(About) 설정

저장소 오른쪽 `About`의 톱니 아이콘 클릭. Description: `XP Paint-inspired Windows graphics editor with math, physics and pixel-art tools`. Website: `https://eprparadox44.github.io/dotPaint/`. Topics: `paint`, `graphics-editor`, `golang`, `windows`, `pixel-art`, `physics`, `math`.

## 9단계: 법적 주의

LICENSE와 README 및 웹사이트 푸터의 저작권자 표기는 울람 나선 HTML과 동일한 `Copyright (c) 2026 지너드, G.nerd (eprparadox44)`입니다. MIT는 별도 전자서명이 필요하지 않습니다. 제3자 이미지/저작물은 본인 소유 코드의 MIT 사용허가와 별개이므로 `THIRD_PARTY_NOTICES.md`에 따라 출처와 재배포 권리를 점검해야 합니다.
