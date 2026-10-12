package main

import "testing"

func TestEyedropperIconUsesWin32COLORREFOrder(t *testing.T) {
	const yellowCOLORREF uint32 = 0x0000FFFF     // RGB(255,255,0)
	const cyanSampleCOLORREF uint32 = 0x00FFDA42 // RGB(66,218,255)
	var yellowCount, cyanCount int
	for _, px := range eyedropperToolIconPixels {
		switch px {
		case yellowCOLORREF:
			yellowCount++
		case cyanSampleCOLORREF:
			cyanCount++
		}
	}
	if yellowCount == 0 {
		t.Fatal("yellow rubber bulb pixels are missing")
	}
	if cyanCount == 0 {
		t.Fatal("cyan ink pixels are missing")
	}
}
