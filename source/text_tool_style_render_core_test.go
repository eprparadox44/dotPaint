package main

import (
	"fmt"
	"testing"
)

func TestTextScriptMetricsAndStyleDifference(t *testing.T) {
	for _, pt := range []int{1, 3, 10, 20, 42, 100} {
		normal := textScriptPointSize(pt, 0)
		sup := textScriptPointSize(pt, 1)
		sub := textScriptPointSize(pt, -1)
		if normal != pt || sup != sub || sup < 1 || sup > pt {
			t.Fatalf("pt=%d normal=%d super=%d sub=%d", pt, normal, sup, sub)
		}
	}
	if textScriptTopOffset(20, 13, 1) != 0 || textScriptTopOffset(20, 13, -1) <= 0 {
		t.Fatal("wrong baseline adjustment")
	}
}

func TestArbitraryTypedTextSizes1Through100(t *testing.T) {
	for n := 1; n <= 100; n++ {
		got, ok := parseTextPointSize(fmt.Sprintf("%d", n))
		if !ok || got != n {
			t.Errorf("typed size %d not accepted", n)
		}
	}
	for _, invalid := range []string{"", "0", "-1", "0.5", "abc", "513"} {
		if _, ok := parseTextPointSize(invalid); ok {
			t.Errorf("invalid typed size accepted: %q", invalid)
		}
	}
	if n, ok := parseTextPointSize(" 512 "); !ok || n != 512 {
		t.Fatal("previous extended size range must be preserved")
	}
}
