# dotPaint 2.0.0 — Official Release

The first official 2.x milestone of dotPaint: a lightweight, Windows x64 drawing editor for pixel art, math diagrams, and physical-field visualizations.

## Highlights

- Editable object workflow, object clipboard, DOP project files, and simplified rasterization confirmation.
- Up to 80 Undo steps (up to 81 snapshots including initial state; 768 MiB memory budget).
- Faster tan N arrows (about 35% CPU improvement in measured scenarios); improvements to lines and right triangles.
- More efficient 3D/4D face occlusion spatial index and torus/Möbius mesh allocations.
- Electric/magnetic/gravitational field rendering with precision-preserving numerical tests and adaptive processing.
- Korean/English UI and light/dark themes.

## Downloads and limits

`dotPaint_2.0.0_x64.exe`: standalone Windows x64 standard edition, under **10 MiB**.
The `DEVLOG` build is for diagnostics and is not a public release asset.

## Validation

Go tests, vet and Windows x64 cross-compilation are used to validate the source. Actual GUI interactions on Windows are not exercised in the Linux build environment.

Original code MIT licensed. Third-party assets are subject to their corresponding notices.

## History

This official 2.0.0 release is a version-number normalization of the internally validated 2.0.1 performance build. No rendering or object-model feature removals were intended.
