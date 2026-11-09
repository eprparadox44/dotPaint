# dotPaint 2.0.1

**A lightweight, standalone Windows x64 graphics editor for pixel art, mathematics, physics and 3D/4D visualization.**

> 공식 배포판은 **2.0.1**이며 Windows x64 실행 파일의 용량 상한은 **10 MiB**입니다. 개발 로그판에는 용량 제한이 없습니다.

## Download

- [Latest stable release](https://github.com/eprparadox44/dotPaint/releases/latest)
- [dotPaint 2.0.1 release notes](RELEASE_NOTES_2.0.1.md)

## Main features

- Windows Paint-inspired pixel-precision drawing with lines, brush, eraser, selection, color editing and clipboard
- Editable vector and text objects, object clipboard, rasterization and native DOP projects
- Straight lines, arrows and right triangles with **tan N** constraints, plus anti-aliasing controls
- Mathematical equations and graphs
- 3D/4D shapes including torus, Möbius strip and regular polytopes
- Electric, magnetic and gravitational field and potential visualization
- Light/dark themes and Korean/English interface
- Up to **80 undo actions** under a 768 MiB history memory budget
- Rasterization mode preference persists across application restarts
- The About dialog includes MIT license and third-party notices in Korean and English

## Building

Requires **Go 1.23.2 or later**. Source and resources are in `source/`.

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go -C source build -buildvcs=false -trimpath -ldflags='-H=windowsgui -s -w -buildid=' -o dotPaint_2.0.1_x64.exe .
```

The standard Windows x64 executable must remain at or below **10,485,760 bytes**. Developer log builds are exempt. See `build_2.0.1.sh` or `build_2.0.1.bat`.

Run tests:

```bash
go -C source test ./...
go -C source vet ./...
```

## Verification

Automated Go regression tests and Windows x64 cross-compilation are used. Full GUI interaction is not exercised in the Linux build environment.

## Licensing / credits

Original program code: **MIT License**. Third-party and embedded assets may have separate licenses; consult [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

dotPaint is an independent project inspired by classic Windows Paint, not affiliated with Microsoft.
