//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
)

func textToolPreferencePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", errors.New("empty user config directory")
	}
	return filepath.Join(dir, "dotPaint", "text_tool.json"), nil
}
func loadTextToolPreference() (TextToolPreference, bool, error) {
	path, err := textToolPreferencePath()
	if err != nil {
		return TextToolPreference{}, false, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return TextToolPreference{}, false, nil
	}
	if err != nil {
		return TextToolPreference{}, false, err
	}
	p, err := decodeTextToolPreference(b)
	if err != nil {
		return TextToolPreference{}, false, err
	}
	return p, true, nil
}
func saveTextToolPreference(p TextToolPreference) error {
	b, err := encodeTextToolPreference(p)
	if err != nil {
		return err
	}
	path, err := textToolPreferencePath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	// Windows cannot atomically rename over an existing destination.
	if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(tmp)
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
func preferenceFromTextTool(t *TextState) TextToolPreference {
	return TextToolPreference{Version: 1, FontName: t.FontName, FontSize: t.FontSize, PreviewScript: t.Script,
		Bold: t.Bold, Italic: t.Italic, Underline: t.Underline, ScriptMode: t.ScriptMode}
}
func applyTextToolPreference(t *TextState, p TextToolPreference) {
	if validateTextToolPreference(p) != nil {
		return
	}
	t.FontName = p.FontName
	t.FontSize = p.FontSize
	if p.PreviewScript != "" {
		t.Script = p.PreviewScript
	}
	t.Bold = p.Bold
	t.Italic = p.Italic
	t.Underline = p.Underline
	t.ScriptMode = p.ScriptMode
}
