package main

import (
	"errors"
	"os"
	"path/filepath"
)

// Store helpers are OS-neutral so the persistence behavior can be exercised
// with real files on Linux CI, even though the app UI targets Windows.
func readRasterModePreferenceFile(path string) (enabled, valid bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	enabled, valid = parseRasterModePreference(data)
	return enabled, valid, nil
}

func writeRasterModePreferenceFile(path string, enabled bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encodeRasterModePreference(enabled), 0o644); err != nil {
		return err
	}
	// Match dotPaint's theme.txt persistence behavior on Windows: Rename
	// cannot reliably overwrite a pre-existing destination there.
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
