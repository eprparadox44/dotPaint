//go:build windows

package main

import (
	"bytes"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dotpaint/internal/webplossless"
)

// TestWebP17PixelsExactlyPreserved compares all 17 embedded WebP assets
// against their pre-conversion PNG masters. The PNG files are testdata only;
// they are not embedded in the application executable.
func TestWebP17PixelsExactlyPreserved(t *testing.T) {
	type sample struct {
		original, embedded string
		data               []byte
	}
	samples := []sample{
		{"user_earth_skin_2048.png", "assets/user_earth_skin_2048.webp", mdEarthTextureBytes},
		{"nasa_saturn_cassini_cutout_2048.png", "assets/nasa_saturn_cassini_cutout_2048.webp", mdSaturnCassiniBytes},
	}
	for _, set := range []struct {
		prefix  string
		entries []struct {
			name string
			data []byte
		}
	}{
		{"animal", []struct {
			name string
			data []byte
		}{{"adelie_penguin_1", animalAdeliePenguin1WebP}, {"adelie_penguin_2", animalAdeliePenguin2WebP}, {"mouse", animalMouseWebP}}},
		{"circuit", []struct {
			name string
			data []byte
		}{{"switch", circuitSwitchWebP}, {"diode", circuitDiodeWebP}, {"resistor", circuitResistorWebP}, {"variable_resistor", circuitVariableResistorWebP}, {"capacitor", circuitCapacitorWebP}, {"ammeter", circuitAmmeterWebP}, {"voltmeter", circuitVoltmeterWebP}, {"coil", circuitCoilWebP}, {"ac_source", circuitACSourceWebP}, {"dc_source", circuitDCSourceWebP}, {"npn_transistor", circuitNPNTransistorWebP}, {"pnp_transistor", circuitPNPTransistorWebP}}},
	} {
		for _, asset := range set.entries {
			name := set.prefix + "/" + asset.name
			samples = append(samples, sample{name + ".png", "assets/" + name + ".webp", asset.data})
		}
	}
	if len(samples) != 17 {
		t.Fatalf("tested %d assets, want 17", len(samples))
	}
	for _, item := range samples {
		t.Run(strings.ReplaceAll(item.original, "/", "_"), func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("testdata/webp_png_reference", item.original))
			if err != nil {
				t.Fatal(err)
			}
			original, err := png.Decode(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := os.ReadFile(item.embedded)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(encoded, item.data) {
				t.Fatal("embedded bytes differ from on-disk WebP asset")
			}
			got, err := webplossless.Decode(item.data)
			if err != nil {
				t.Fatal(err)
			}
			if got.Bounds() != original.Bounds() {
				t.Fatalf("bounds %v != %v", got.Bounds(), original.Bounds())
			}
			for y := 0; y < original.Bounds().Dy(); y++ {
				for x := 0; x < original.Bounds().Dx(); x++ {
					first := color.NRGBAModel.Convert(original.At(x, y))
					second := color.NRGBAModel.Convert(got.At(x, y))
					if first != second {
						t.Fatalf("pixel mismatch at (%d,%d): PNG %v WebP %v", x, y, first, second)
					}
				}
			}
		})
	}
}
func TestWebPEmbeddedTexturePathAndUnsupportedInput(t *testing.T) {
	if tex := mdDecodeEmbeddedTexture(mdEarthTextureBytes); !tex.ok || tex.w != 2048 || tex.h != 1024 {
		t.Fatalf("earth decode wrong: %+v", tex)
	}
	if tex := mdDecodeEmbeddedTexture(mdSaturnCassiniBytes); !tex.ok || tex.w != 2048 || tex.h != 821 {
		t.Fatalf("saturn decode wrong: %+v", tex)
	}
	if _, err := webplossless.Decode([]byte("not a WebP file")); err == nil {
		t.Fatal("invalid WebP should fail")
	}
}
