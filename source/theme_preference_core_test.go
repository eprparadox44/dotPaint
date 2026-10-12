package main

import "testing"

func TestThemePreferenceRoundTrip(t *testing.T) {
	for _, wantDark := range []bool{false, true} {
		gotDark, ok := parseThemePreference(encodeThemePreference(wantDark))
		if !ok || gotDark != wantDark {
			t.Fatalf("round trip dark=%v -> dark=%v ok=%v", wantDark, gotDark, ok)
		}
	}
}

func TestThemePreferenceParserRejectsUnknownValue(t *testing.T) {
	if dark, ok := parseThemePreference([]byte("automatic\n")); ok || dark {
		t.Fatalf("unknown theme accepted: dark=%v ok=%v", dark, ok)
	}
}

func TestThemePreferenceParserIgnoresCaseAndWhitespace(t *testing.T) {
	if dark, ok := parseThemePreference([]byte("  DARK\r\n")); !ok || !dark {
		t.Fatalf("dark preference not parsed: dark=%v ok=%v", dark, ok)
	}
	if dark, ok := parseThemePreference([]byte(" Light \n")); !ok || dark {
		t.Fatalf("light preference not parsed: dark=%v ok=%v", dark, ok)
	}
}
