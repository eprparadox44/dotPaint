//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
)

func themePreferencePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", errors.New("Windows 사용자 설정 폴더가 비어 있습니다")
	}
	return filepath.Join(dir, "dotPaint", "theme.txt"), nil
}

func loadThemePreference() (dark bool, ok bool, err error) {
	path, err := themePreferencePath()
	if err != nil {
		return false, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	dark, ok = parseThemePreference(data)
	return dark, ok, nil
}

func saveThemePreference(dark bool) error {
	path, err := themePreferencePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encodeThemePreference(dark), 0o644); err != nil {
		return err
	}
	// os.Rename cannot replace an existing file on every supported Windows
	// version. The temporary file still prevents a partial preference write.
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
