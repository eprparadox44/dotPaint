//go:build windows

package main

import "path/filepath"

// Keep the preference beside theme.txt in %APPDATA%/dotPaint; a portable EXE
// must not need write permission in its installation folder.
func rasterModePreferencePath() (string, error) {
	theme, err := themePreferencePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(theme), "raster_mode.txt"), nil
}

func loadRasterModePreference() (enabled, ok bool, err error) {
	path, err := rasterModePreferencePath()
	if err != nil {
		return false, false, err
	}
	return readRasterModePreferenceFile(path)
}

func saveRasterModePreference(enabled bool) error {
	path, err := rasterModePreferencePath()
	if err != nil {
		return err
	}
	return writeRasterModePreferenceFile(path, enabled)
}
