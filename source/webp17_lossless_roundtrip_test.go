package main

import (
	"bytes"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"dotpaint/internal/webplossless"
)

func TestWebP17LosslessRoundtripAllPlatforms(t *testing.T) {
	names := []string{"user_earth_skin_2048", "nasa_saturn_cassini_cutout_2048"}
	for _, group := range []string{"animal", "circuit"} {
		files, err := filepath.Glob(filepath.Join("assets", group, "*.webp"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			names = append(names, filepath.Join(group, filepath.Base(path[:len(path)-5])))
		}
	}
	if len(names) != 17 {
		t.Fatalf("want 17 assets, got %d", len(names))
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			originalPath := filepath.Join("testdata/webp_png_reference", name+".png")
			webpPath := filepath.Join("assets", name+".webp")
			data, err := os.ReadFile(originalPath)
			if err != nil {
				t.Fatal(err)
			}
			master, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			compressed, err := os.ReadFile(webpPath)
			if err != nil {
				t.Fatal(err)
			}
			converted, err := webplossless.Decode(compressed)
			if err != nil {
				t.Fatal(err)
			}
			if converted.Bounds() != master.Bounds() {
				t.Fatalf("dimensions %v != %v", converted.Bounds(), master.Bounds())
			}
			for y := 0; y < master.Bounds().Dy(); y++ {
				for x := 0; x < master.Bounds().Dx(); x++ {
					if color.NRGBAModel.Convert(converted.At(x, y)) != color.NRGBAModel.Convert(master.At(x, y)) {
						t.Fatalf("pixel differs at (%d,%d)", x, y)
					}
				}
			}
		})
	}
}
