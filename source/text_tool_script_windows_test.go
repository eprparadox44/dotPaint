//go:build windows

package main

import (
	"testing"
	"unsafe"
)

func TestRichEditSubscriptSuperscriptFormats(t *testing.T) {
	if got := unsafe.Sizeof(CHARFORMAT2W{}); got != 116 {
		t.Fatalf("CHARFORMAT2W ABI size=%d, expected 116", got)
	}
	for _, tc := range []struct {
		mode   int8
		effect uint32
	}{{0, 0}, {1, CFE_SUPERSCRIPT}, {-1, CFE_SUBSCRIPT}} {
		cf := makeTextCharFormat(TextCharStyle{FontName: "Arial", FontSize: 20, ScriptMode: tc.mode}, rgb(0, 0, 0), 96)
		if cf.CbSize != 116 || cf.DwMask&CFM_SUBSCRIPT != CFM_SUBSCRIPT {
			t.Fatalf("missing script mask or size: %#v", cf)
		}
		if got := cf.DwEffects & (CFE_SUBSCRIPT | CFE_SUPERSCRIPT); got != tc.effect {
			t.Errorf("mode=%d: got effect %#x want %#x", tc.mode, got, tc.effect)
		}
	}
}
