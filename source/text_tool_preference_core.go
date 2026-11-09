package main

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// TextToolPreference is a user's last explicitly selected toolbar format.
// It is independent of the per-character formatting of the current canvas.
type TextToolPreference struct {
	Version       int    `json:"version"`
	FontName      string `json:"font_name"`
	FontSize      int    `json:"font_size"`
	PreviewScript string `json:"preview_script"`
	Bold          bool   `json:"bold"`
	Italic        bool   `json:"italic"`
	Underline     bool   `json:"underline"`
	ScriptMode    int8   `json:"script_mode"` // -1 subscript, 0 normal, 1 superscript
}

func validateTextToolPreference(p TextToolPreference) error {
	if p.Version != 1 {
		return errors.New("unsupported text toolbar settings version")
	}
	if p.FontSize < 1 || p.FontSize > 512 {
		return errors.New("font size out of range")
	}
	if p.ScriptMode < -1 || p.ScriptMode > 1 {
		return errors.New("script mode out of range")
	}
	if len(p.FontName) == 0 || len(p.FontName) > 128 || !utf8.ValidString(p.FontName) || strings.ContainsAny(p.FontName, "\x00\n\r") {
		return errors.New("invalid font name")
	}
	if len(p.PreviewScript) > 64 || !utf8.ValidString(p.PreviewScript) || strings.ContainsAny(p.PreviewScript, "\x00\n\r") {
		return errors.New("invalid preview script")
	}
	return nil
}

func encodeTextToolPreference(p TextToolPreference) ([]byte, error) {
	p.Version = 1
	if err := validateTextToolPreference(p); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func decodeTextToolPreference(data []byte) (TextToolPreference, error) {
	var p TextToolPreference
	if len(data) > 8192 {
		return p, errors.New("text toolbar settings too large")
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, err
	}
	if err := validateTextToolPreference(p); err != nil {
		return p, err
	}
	return p, nil
}

// toggleTextScriptMode keeps superscript and subscript mutually exclusive.
func toggleTextScriptMode(current, selected int8) int8 {
	if selected != -1 && selected != 1 {
		return 0
	}
	if current == selected {
		return 0
	}
	return selected
}
