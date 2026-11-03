# dotPaint 1.3.9 — Text Input Responsiveness & Caret Navigation

**Korean and English in a single Windows executable, with instant interface language switching.**

## What's New

- **Faster text input handling:** Text-editing changes no longer synchronously rebuild heavy formatting/layout for every keystroke. Visual updates are coalesced, helping reduce latency when typing quickly with spaces and Enter.
- **Home / End improvements:** The text caret now uses the editor's visible line layout rather than a hidden native input control's independent wrapping boundaries.
- **Keyboard selection:** Supports Shift+Home/End and Ctrl+Home/End behavior. The goal is to preserve selection, IME composition, and editable text content.
- **Magnifier improvements carried over from 1.3.8:** 20× zoom and automatic skipping of target-area selection when the whole enlarged image fits the canvas viewport.
- All classic drawing, indexed palette, equations, graphs, 3D/4D shapes, physical-field visualization, themes, and Korean/English interface controls remain included.

## Downloads

Choose **one** file for your Windows architecture:

- `dotPaint_1.3.9_x64.exe` — Windows 64-bit
- `dotPaint_1.3.9_x86.exe` — Windows 32-bit

No installer is required. The public release contains **only these two standard executables**; developer diagnostic/log builds are separate and not intended as release assets.

## Testing note

Automated source tests and Windows cross-builds verify the code and build. **Automated Go tests, static analysis, Windows cross-builds, and standard executable size checks have passed. Real Windows interaction (fast typing, Korean IME, Space/Enter order, and Home/End) has not yet been independently verified. Please report any remaining input issues.**

## Project

Developer: **G.nerd (eprparadox44)**  
[Source code](https://github.com/eprparadox44/dotPaint) · [Development blog (Korean)](https://blog.naver.com/jdw1985/224360587153)

Original project code is MIT licensed. Third-party assets may have separate terms; refer to `THIRD_PARTY_NOTICES.md`.

*dotPaint is an independent project inspired by classic Windows Paint and is not affiliated with Microsoft.*
