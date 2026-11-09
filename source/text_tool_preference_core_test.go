package main

import "testing"

func TestTextToolPreferenceRoundTrip(t *testing.T) {
	want := TextToolPreference{Version: 1, FontName: "365애착폰트", FontSize: 20, PreviewScript: "한글", Bold: true, Italic: true, Underline: true, ScriptMode: -1}
	b, err := encodeTextToolPreference(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeTextToolPreference(b)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round trip: got %#v want %#v", got, want)
	}
	want.ScriptMode = 1
	b, err = encodeTextToolPreference(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err = decodeTextToolPreference(b)
	if err != nil || got != want {
		t.Fatalf("superscript round trip: %v %v", got, err)
	}
}
func TestTextToolPreferenceRejectsInvalid(t *testing.T) {
	tests := []string{``, `{}`, `{"version":1,"font_name":"ok","font_size":900}`, `{"version":1,"font_name":"ok","font_size":20,"script_mode":3}`, `{"version":5,"font_name":"ok","font_size":20}`, `garbage`}
	for _, x := range tests {
		if _, err := decodeTextToolPreference([]byte(x)); err == nil {
			t.Errorf("invalid accepted: %s", x)
		}
	}
}

func TestTextToolScriptModeToggle(t *testing.T) {
	for _, tc := range []struct{ current, selected, want int8 }{
		{0, 1, 1}, {1, 1, 0}, {0, -1, -1}, {-1, -1, 0}, {1, -1, -1}, {-1, 1, 1},
	} {
		if got := toggleTextScriptMode(tc.current, tc.selected); got != tc.want {
			t.Errorf("toggle(%d,%d) = %d, want %d", tc.current, tc.selected, got, tc.want)
		}
	}
}
