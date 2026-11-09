package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRasterModePreferenceRoundTrip(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		got, ok := parseRasterModePreference(encodeRasterModePreference(enabled))
		if !ok || got != enabled {
			t.Fatalf("round trip enabled=%v -> enabled=%v ok=%v", enabled, got, ok)
		}
	}
}

func TestRasterModePreferenceParserIsStrictAndCaseInsensitive(t *testing.T) {
	for _, tc := range []struct {
		data        string
		enabled, ok bool
	}{
		{" ON\r\n", true, true},
		{" off \n", false, true},
		{"enabled", false, false},
		{"", false, false},
		{"1", false, false},
	} {
		got, ok := parseRasterModePreference([]byte(tc.data))
		if got != tc.enabled || ok != tc.ok {
			t.Fatalf("parse %q got (%v,%v) want (%v,%v)", tc.data, got, ok, tc.enabled, tc.ok)
		}
	}
}

func TestRasterModePreferenceFilePersistsOnAndOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dotPaint", "raster_mode.txt")
	if enabled, ok, err := readRasterModePreferenceFile(path); err != nil || ok || enabled {
		t.Fatalf("missing preference returned (%v,%v,%v)", enabled, ok, err)
	}
	for _, want := range []bool{true, false, true} {
		if err := writeRasterModePreferenceFile(path, want); err != nil {
			t.Fatal(err)
		}
		got, ok, err := readRasterModePreferenceFile(path)
		if err != nil || !ok || got != want {
			t.Fatalf("persist want=%v got=%v valid=%v err=%v", want, got, ok, err)
		}
		if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
			t.Fatalf("temporary file remains after saving: %v", err)
		}
	}
}

func TestRasterModePreferenceFileIgnoresCorruptContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raster_mode.txt")
	if err := os.WriteFile(path, []byte("maybe?"), 0o644); err != nil {
		t.Fatal(err)
	}
	enabled, valid, err := readRasterModePreferenceFile(path)
	if err != nil || valid || enabled {
		t.Fatalf("unexpected corrupt preference state %v %v %v", enabled, valid, err)
	}
}
