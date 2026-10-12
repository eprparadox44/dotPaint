//go:build windows

package main

import "testing"

func TestTextLayoutSentinelMatchesLastVisibleCharacter(t *testing.T) {
	plain := TextCharStyle{FontName: "plain", FontSize: 20}
	bold := TextCharStyle{FontName: "bold", FontSize: 32, Bold: true}
	got := textLayoutSentinelStyle([]TextCharStyle{bold, plain}, bold, bold, true)
	if !got.Equal(plain) {
		t.Fatalf("sentinel=%+v want last real style=%+v", got, plain)
	}
}

func TestTextLayoutSentinelUsesInsertionStyleOnlyWhenEmpty(t *testing.T) {
	fallback := TextCharStyle{FontName: "fallback", FontSize: 20}
	insert := TextCharStyle{FontName: "insert", FontSize: 32, Bold: true}
	if got := textLayoutSentinelStyle(nil, fallback, insert, true); !got.Equal(insert) {
		t.Fatalf("empty active sentinel=%+v want insertion=%+v", got, insert)
	}
	if got := textLayoutSentinelStyle(nil, fallback, insert, false); !got.Equal(fallback) {
		t.Fatalf("empty inactive sentinel=%+v want fallback=%+v", got, fallback)
	}
}
