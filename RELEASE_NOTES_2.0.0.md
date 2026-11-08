# dotPaint 2.0.0 — Official Release

dotPaint 2.0.0 is the official major-version milestone for the Windows x64 drawing editor inspired by classic Windows Paint.

## Highlights

- Editable objects, object clipboard, simplified rasterization confirmation, and DOP project save/open.
- **80-step Undo** (up to 81 snapshots including the initial state), with a 768 MiB history budget.
- Optimized tan N line, arrow, and right-triangle rendering, including an approximately 35% faster arrow in measured CPU scenarios.
- Reduced allocations in torus/Möbius mesh construction and 3D/4D occlusion spatial indexing.
- Precision-preserving improvements for 3D field and potential rendering, with regression tests.
- Korean and English interfaces; light and dark themes; traditional pixel-editing tools.

## Distribution

- **Windows x64 standard executable:** `dotPaint_2.0.0_x64.exe`, standalone without installation, maximum **10 MiB (10,485,760 bytes)**.
- The developer diagnostic (DEVLOG) build is for private testing, has **no size limit**, and is not a public Release asset.

## Testing

Automated Go tests, `go vet`, Windows x64 cross-build, and executable-size checks are required before publication. Interactive Windows GUI behavior is not independently tested in the Linux build environment.

## Version history

The official 2.0.0 release retains the features and performance improvements of the previously tested 2.0.1 development build, normalizing the publicly displayed version to 2.0.0.

MIT-licensed program code. Some embedded third-party assets may have their own terms: see `THIRD_PARTY_NOTICES.md`.

*dotPaint is an independent project and is not affiliated with Microsoft.*
