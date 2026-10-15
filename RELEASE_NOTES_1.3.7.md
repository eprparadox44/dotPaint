# dotPaint 1.3.7 — Multidimensional Shape Editor UI cleanup

## Changes
- Removed the two large read-only information panes from the multidimensional shape editor: technical status/debug information and extended shape explanation.
- Expanded the live shape preview horizontally into their former space, with 10-pixel left/right margins.
- Preserved shape selection, face mode, options, rotation/size sliders, bottom status line, preview rendering and insertion.
- Applies equally to Korean and English modes in the same EXE; immediate language switching remains implemented.
- No drawing feature was intentionally removed.

## Validation
- Windows x64 and x86 binaries cross-compiled with Go 1.23.2 on Linux: PASS.
- New UI layout regression test plus release/language/source smoke tests: PASS.
- Linux go vet ./...: PASS.
- Windows go vet -unsafeptr=false ./...: PASS. Full Windows go vet reports existing unsafe.Pointer warnings across Win32 interop and GPU source; NOT a clean default Windows vet pass.
- Whole go test ./...: attempted, no completion within execution limit; NOT a PASS.
- Real Win32 GUI visual/hands-on test: NOT PERFORMED in Linux environment.

## Required real Windows check
Run x64 EXE -> Insert -> Multidimensional Shape. Verify two info areas are absent, the preview occupies the full area, 4D shapes rotate normally, selection/face mode and Insert still work; repeat in Korean/English, light/dark, and on resize. Monitor performance of full-width GPU-heavy previews.

## EXE size rule
Standard EXEs are limited to 10,485,760 bytes each. Development log EXE has NO size limit.
