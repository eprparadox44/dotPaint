# dotPaint 2.0.1 — Remember Raster Mode (Official Release)

Based on the official 2.0.0 stable release, with all features and performance improvements preserved.

## Improvement

- Remember the last **Rasterization Mode** state across application restarts, similar to the light/dark theme preference.
- Save per-user preference in `%APPDATA%\dotPaint\raster_mode.txt` (values: `on` / `off`).
- Save on explicit UI changes and normal shutdown.
- Restore the option on a fresh canvas without warning prompts, flattening, or creating an extra undo-history entry.
- Continue to prompt before manually enabling rasterization and flattening existing objects.
- Opening editable DOP projects still automatically disables raster mode to protect editable objects.

## About / License information

- The **About dotPaint** window now displays the program's **MIT License**, project copyright (**Copyright © 2026 지너드 · G.nerd / eprparadox44**), and a separate third-party assets/code notice.
- Dedicated **MIT 라이선스 전문** and **제3자 자료 고지** buttons open the authoritative `LICENSE` and `THIRD_PARTY_NOTICES.md` documents on GitHub in the default browser.
- The notice makes clear that MIT does not automatically relicense third-party images, fonts, or source dependencies.
- The About window was enlarged to prevent the new lines from colliding with the close button, and the new controls retain the existing dark/light styling.

## Korean / English About dialog localization

- All About-dialog descriptive text, version/build label, MIT license summary, copyright credit, and third-party asset notice follow the application's saved Korean/English language preference.
- The license and third-party notice buttons are localized as **MIT 라이선스 전문 / Full MIT License**, and **제3자 자료 고지 / Third-Party Notices**.
- The About title and controls refresh immediately when switching languages while that dialog remains open. No need to close the About window or restart dotPaint.
- Browser links, existing About content, the app's theme, and document state remain unchanged.

## Validation

- New parsing and file read/write tests, including missing, corrupt, and repeated overwrite scenarios.
- Windows x64 cross-compile and existing regression tests.
- Standard executable <= 10 MiB; dev-log executable exempt.

The raster-mode persistence has already been confirmed in Windows by the user. The new About-dialog layout, language-switch rendering, and URL buttons still require a Windows GUI check. The public GitHub `v2.0.0` release is unchanged, and `v2.0.1` is published as the next stable release.
