package main

import "strings"

func encodeThemePreference(dark bool) []byte {
	if dark {
		return []byte("dark\n")
	}
	return []byte("light\n")
}

func parseThemePreference(data []byte) (dark bool, ok bool) {
	switch strings.ToLower(strings.TrimSpace(string(data))) {
	case "dark":
		return true, true
	case "light":
		return false, true
	default:
		return false, false
	}
}
