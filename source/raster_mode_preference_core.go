package main

import "strings"

// Persist the user's last effective rasterization mode independently of any
// document history. Unknown or corrupt values do not override the default.
func encodeRasterModePreference(enabled bool) []byte {
	if enabled {
		return []byte("on\n")
	}
	return []byte("off\n")
}

func parseRasterModePreference(data []byte) (enabled bool, ok bool) {
	switch strings.ToLower(strings.TrimSpace(string(data))) {
	case "on":
		return true, true
	case "off":
		return false, true
	default:
		return false, false
	}
}
