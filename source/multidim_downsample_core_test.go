package main

import "testing"

func TestMDAreaDownsampleCreatesIntermediateCoverage(t *testing.T) {
	src := make([]byte, 4*4*4)
	for i := 0; i < 16; i++ {
		src[i*4], src[i*4+1], src[i*4+2], src[i*4+3] = 255, 255, 255, 255
	}
	for _, i := range []int{0, 1, 4, 5} {
		src[i*4], src[i*4+1], src[i*4+2] = 0, 0, 0
	}
	dst := make([]byte, 2*2*4)
	if !mdAreaDownsampleBGRA(dst, src, 2, 2, 2) {
		t.Fatal("downsample failed")
	}
	if dst[0] != 0 || dst[4] != 255 || dst[8] != 255 || dst[12] != 255 {
		t.Fatalf("unexpected pixels: %v", dst)
	}
}

func TestMDAreaDownsampleRounding(t *testing.T) {
	src := []byte{0, 0, 0, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}
	dst := make([]byte, 4)
	if !mdAreaDownsampleBGRA(dst, src, 1, 1, 2) {
		t.Fatal("downsample failed")
	}
	if dst[0] != 191 || dst[1] != 191 || dst[2] != 191 {
		t.Fatalf("got %v want 191", dst[:3])
	}
}
