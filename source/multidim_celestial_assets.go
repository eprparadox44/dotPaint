package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"sync"
)

// A213 embeds the user-supplied 2048x1024 equirectangular Earth skin so the
// standalone executable and the editable skins\earth.png override start from
// exactly the same pixels. Moon and Saturn retain their NASA/JPL source imagery. Saturn uses a real
// Cassini-derived global cylindrical texture: latitude colours come from the
// natural-colour PIA06193 portrait, while longitudinal cloud/storm contrast
// comes from the 45-frame PIA06082 rotation movie. The original portrait also
// supplies the radial ring colour profile. Scene lighting is recomputed by
// dotPaint, so neither the old fixed photographic phase nor a transparent back
// hemisphere remains. Attribution and reconstruction details are recorded in
// docs/THIRD_PARTY_ASSETS_A205.txt.

//go:embed assets/user_earth_skin_2048.png
var mdEarthTextureBytes []byte

//go:embed assets/nasa_moon_lro_2048.jpg
var mdMoonTextureBytes []byte

//go:embed assets/nasa_saturn_cassini_cutout_2048.png
var mdSaturnCassiniBytes []byte

//go:embed assets/nasa_saturn_cassini_global_2048.jpg
var mdSaturnGlobalBytes []byte

type mdEmbeddedTexture struct {
	w, h int
	rgba []byte
	ok   bool
}

type mdCelestialRGB struct {
	r, g, b byte
}

var (
	mdCelestialAssetsOnce   sync.Once
	mdEarthTexture          mdEmbeddedTexture
	mdMoonTexture           mdEmbeddedTexture
	mdSaturnCassini         mdEmbeddedTexture
	mdSaturnGlobal          mdEmbeddedTexture
	mdSaturnLatitudeProfile []mdCelestialRGB
	mdSaturnRingProfile     []mdCelestialRGB

	mdEarthTextureSource       = "embedded"
	mdEarthTextureExternalPath string
	mdEarthTextureExternalErr  string
	mdEarthTextureRGBAHash     string
	mdEarthDetailColours       [2]mdCelestialRGB
	mdEarthDetailColourOK      [2]bool
)

func mdDecodeTextureImage(img image.Image) mdEmbeddedTexture {
	if img == nil {
		return mdEmbeddedTexture{}
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return mdEmbeddedTexture{}
	}
	out := mdEmbeddedTexture{w: w, h: h, rgba: make([]byte, w*h*4), ok: true}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := (y*w + x) * 4
			out.rgba[i+0] = byte(r >> 8)
			out.rgba[i+1] = byte(g >> 8)
			out.rgba[i+2] = byte(bl >> 8)
			out.rgba[i+3] = byte(a >> 8)
		}
	}
	return out
}

func mdDecodeEmbeddedTexture(data []byte) mdEmbeddedTexture {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil || img == nil {
		return mdEmbeddedTexture{}
	}
	return mdDecodeTextureImage(img)
}

// mdLoadExternalEarthTexture loads the first user override found next to the
// executable. Only earth.png is accepted as an external override so stale
// A211-era earth.jpg/earth.jpeg files cannot silently replace the newer embedded map.
// The map must be an ordinary
// north-up equirectangular world map with an approximately 2:1 aspect ratio.
// The file is intentionally read only once at startup; replace it while
// dotPaint is closed and restart the program to reload the skin.
func mdLoadExternalEarthTextureFromDir(dir string) (mdEmbeddedTexture, string, error) {
	candidates := [...]string{
		filepath.Join(dir, "earth.png"),
	}
	for _, path := range candidates {
		f, openErr := os.Open(path)
		if os.IsNotExist(openErr) {
			continue
		}
		if openErr != nil {
			return mdEmbeddedTexture{}, path, openErr
		}
		img, _, decodeErr := image.Decode(f)
		_ = f.Close()
		if decodeErr != nil {
			return mdEmbeddedTexture{}, path, decodeErr
		}
		tex := mdDecodeTextureImage(img)
		if !tex.ok || tex.w < 64 || tex.h < 32 {
			return mdEmbeddedTexture{}, path, fmt.Errorf("earth skin is too small: %dx%d (minimum 64x32)", tex.w, tex.h)
		}
		ratio := float64(tex.w) / float64(tex.h)
		if ratio < 1.80 || ratio > 2.20 {
			return mdEmbeddedTexture{}, path, fmt.Errorf("earth skin must be an equirectangular ~2:1 map: %dx%d ratio=%.3f", tex.w, tex.h, ratio)
		}
		return tex, path, nil
	}
	return mdEmbeddedTexture{}, "", nil
}

func mdLoadExternalEarthTexture() (mdEmbeddedTexture, string, error) {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return mdEmbeddedTexture{}, "", nil
	}
	return mdLoadExternalEarthTextureFromDir(filepath.Join(filepath.Dir(exe), "skins"))
}

func mdLoadCelestialAssets() {
	mdCelestialAssetsOnce.Do(func() {
		mdEarthTexture = mdDecodeEmbeddedTexture(mdEarthTextureBytes)
		if external, path, err := mdLoadExternalEarthTexture(); path != "" {
			mdEarthTextureExternalPath = path
			if err != nil {
				mdEarthTextureExternalErr = err.Error()
			} else if external.ok {
				mdEarthTexture = external
				mdEarthTextureSource = "external"
			}
		}
		mdEarthTextureRGBAHash = mdTextureRGBAHash(mdEarthTexture)
		mdBuildEarthDetailColours()
		mdMoonTexture = mdDecodeEmbeddedTexture(mdMoonTextureBytes)
		mdSaturnCassini = mdDecodeEmbeddedTexture(mdSaturnCassiniBytes)
		mdSaturnGlobal = mdDecodeEmbeddedTexture(mdSaturnGlobalBytes)
		mdSaturnLatitudeProfile = mdBuildSaturnLatitudeProfile(mdSaturnCassini, 720)
		mdSaturnRingProfile = mdBuildSaturnRingProfile(mdSaturnCassini, 720)
	})
}

func mdTextureRGBAHash(tex mdEmbeddedTexture) string {
	if !tex.ok || len(tex.rgba) == 0 {
		return ""
	}
	sum := sha256.Sum256(tex.rgba)
	return hex.EncodeToString(sum[:])
}

func mdEarthDetailLandScore(r, g, b byte) int {
	// Prefer compact brown/green land pixels over blue ocean and bright cloud.
	// Multiplying by integers keeps CPU and HLSL selection deterministic.
	return int(r) + int(g) - 5*int(b)
}

func mdEarthTexturePixelNear(lon, lat float64, radius int) (mdCelestialRGB, bool) {
	tex := mdEarthTexture
	if !tex.ok || tex.w <= 0 || tex.h <= 0 || len(tex.rgba) < tex.w*tex.h*4 {
		return mdCelestialRGB{}, false
	}
	u := (mdCelestialWrapAngle(lon) + math.Pi) / (2 * math.Pi)
	v := (math.Pi/2 - lat) / math.Pi
	cx := int(math.Round(u * float64(tex.w-1)))
	cy := int(math.Round(v * float64(tex.h-1)))
	bestScore := -1 << 30
	best := mdCelestialRGB{}
	found := false
	for dy := -radius; dy <= radius; dy++ {
		y := cy + dy
		if y < 0 || y >= tex.h {
			continue
		}
		for dx := -radius; dx <= radius; dx++ {
			x := (cx + dx) % tex.w
			if x < 0 {
				x += tex.w
			}
			r, g, b, _, ok := mdTextureSampleNearest(tex, x, y)
			if !ok {
				continue
			}
			score := mdEarthDetailLandScore(r, g, b)
			if !found || score > bestScore {
				bestScore = score
				best = mdCelestialRGB{r: r, g: g, b: b}
				found = true
			}
		}
	}
	return best, found
}

func mdBuildEarthDetailColours() {
	coords := [...][2]float64{
		{130.8667 * math.Pi / 180, 37.5000 * math.Pi / 180}, // 울릉도
		{131.8697 * math.Pi / 180, 37.2411 * math.Pi / 180}, // 독도
	}
	for i, p := range coords {
		mdEarthDetailColours[i], mdEarthDetailColourOK[i] = mdEarthTexturePixelNear(p[0], p[1], 2)
	}
}

func mdCelestialEarthDetailColour(index int) (byte, byte, byte, bool) {
	mdLoadCelestialAssets()
	if index < 0 || index >= len(mdEarthDetailColours) || !mdEarthDetailColourOK[index] {
		return 0, 0, 0, false
	}
	c := mdEarthDetailColours[index]
	return c.r, c.g, c.b, true
}

func mdTextureSampleBilinear(tex mdEmbeddedTexture, u, v float64, wrapX bool) (byte, byte, byte, byte, bool) {
	if !tex.ok || tex.w <= 0 || tex.h <= 0 || len(tex.rgba) < tex.w*tex.h*4 {
		return 0, 0, 0, 0, false
	}
	if wrapX {
		u -= math.Floor(u)
	} else if u < 0 || u > 1 {
		return 0, 0, 0, 0, false
	}
	if v < 0 || v > 1 {
		return 0, 0, 0, 0, false
	}
	x := u * float64(tex.w-1)
	y := v * float64(tex.h-1)
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1 := x0 + 1
	y1 := y0 + 1
	if wrapX {
		x1 %= tex.w
	} else if x1 >= tex.w {
		x1 = tex.w - 1
	}
	if y1 >= tex.h {
		y1 = tex.h - 1
	}
	fx, fy := x-float64(x0), y-float64(y0)
	at := func(px, py, c int) float64 { return float64(tex.rgba[(py*tex.w+px)*4+c]) }
	interp := func(c int) byte {
		a := at(x0, y0, c)*(1-fx) + at(x1, y0, c)*fx
		b := at(x0, y1, c)*(1-fx) + at(x1, y1, c)*fx
		value := a*(1-fy) + b*fy
		if value < 0 {
			value = 0
		}
		if value > 255 {
			value = 255
		}
		return byte(math.Round(value))
	}
	return interp(0), interp(1), interp(2), interp(3), true
}

func mdCelestialMapTextureSample(tex mdEmbeddedTexture, lon, lat float64) (byte, byte, byte, bool) {
	// Cylindrical maps store -180 degrees at the left edge and north at the top.
	u := (mdCelestialWrapAngle(lon) + math.Pi) / (2 * math.Pi)
	v := (math.Pi/2 - lat) / math.Pi
	r, g, b, _, ok := mdTextureSampleBilinear(tex, u, v, true)
	return r, g, b, ok
}

func mdCelestialEarthTextureColor(lon, lat float64) (byte, byte, byte, bool) {
	mdLoadCelestialAssets()
	return mdCelestialMapTextureSample(mdEarthTexture, lon, lat)
}

func mdCelestialMoonTextureColor(lon, lat float64) (byte, byte, byte, bool) {
	mdLoadCelestialAssets()
	return mdCelestialMapTextureSample(mdMoonTexture, lon, lat)
}

func mdCelestialSaturnGlobalTexture() mdEmbeddedTexture {
	mdLoadCelestialAssets()
	return mdSaturnGlobal
}

func mdCelestialSaturnGlobalColor(lon, lat float64) (byte, byte, byte, bool) {
	tex := mdCelestialSaturnGlobalTexture()
	if !tex.ok {
		return 0, 0, 0, false
	}
	u := mdCelestialWrapAngle(lon)/(2*math.Pi) + 0.5
	v := 0.5 - lat/math.Pi
	r, g, b, _, ok := mdTextureSampleBilinear(tex, u, v, true)
	return r, g, b, ok
}

func mdCelestialSaturnCassiniTexture() mdEmbeddedTexture {
	mdLoadCelestialAssets()
	return mdSaturnCassini
}

func mdTextureSampleNearest(tex mdEmbeddedTexture, x, y int) (byte, byte, byte, byte, bool) {
	if !tex.ok || x < 0 || y < 0 || x >= tex.w || y >= tex.h {
		return 0, 0, 0, 0, false
	}
	i := (y*tex.w + x) * 4
	return tex.rgba[i], tex.rgba[i+1], tex.rgba[i+2], tex.rgba[i+3], true
}

func mdMedianByte(values []byte) byte {
	if len(values) == 0 {
		return 0
	}
	counts := [256]int{}
	for _, v := range values {
		counts[int(v)]++
	}
	target := (len(values) - 1) / 2
	n := 0
	for i, c := range counts {
		n += c
		if n > target {
			return byte(i)
		}
	}
	return values[len(values)-1]
}

func mdSmoothCelestialProfile(in []mdCelestialRGB, passes int) []mdCelestialRGB {
	if len(in) < 3 || passes <= 0 {
		return in
	}
	cur := append([]mdCelestialRGB(nil), in...)
	for pass := 0; pass < passes; pass++ {
		next := append([]mdCelestialRGB(nil), cur...)
		for i := 1; i+1 < len(cur); i++ {
			next[i].r = byte((int(cur[i-1].r) + 2*int(cur[i].r) + int(cur[i+1].r) + 2) / 4)
			next[i].g = byte((int(cur[i-1].g) + 2*int(cur[i].g) + int(cur[i+1].g) + 2) / 4)
			next[i].b = byte((int(cur[i-1].b) + 2*int(cur[i].b) + int(cur[i+1].b) + 2) / 4)
		}
		cur = next
	}
	return cur
}

// mdBuildSaturnLatitudeProfile converts the real Cassini portrait into a
// longitude-independent atmospheric profile.  Samples come from the illuminated
// right-hand hemisphere, while the source photo's day/night gradient is removed
// by normalising each latitude to a common luminance.  This preserves the real
// latitude colours and bands while allowing the hidden hemisphere to be filled
// and rotated as a complete globe.
func mdBuildSaturnLatitudeProfile(tex mdEmbeddedTexture, bins int) []mdCelestialRGB {
	if !tex.ok || bins < 8 {
		return nil
	}
	const (
		cx       = 1077.0
		cy       = 458.0
		equatorR = 439.0
		polarR   = 330.0
	)
	out := make([]mdCelestialRGB, bins)
	valid := make([]bool, bins)
	for i := 0; i < bins; i++ {
		lat := -math.Pi/2 + math.Pi*float64(i)/float64(bins-1)
		y := int(math.Round(cy - math.Sin(lat)*polarR))
		half := math.Cos(lat) * equatorR
		if y < 0 || y >= tex.h || half < 2 {
			continue
		}
		type sample struct {
			lum     float64
			r, g, b byte
		}
		samples := make([]sample, 0, 96)
		for k := 0; k < 120; k++ {
			nx := 0.06 + 0.76*float64(k)/119.0
			x := int(math.Round(cx + nx*half))
			r, g, b, a, ok := mdTextureSampleNearest(tex, x, y)
			if !ok || a < 220 {
				continue
			}
			lum := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
			if lum < 24 {
				continue
			}
			samples = append(samples, sample{lum: lum, r: r, g: g, b: b})
		}
		if len(samples) < 6 {
			continue
		}
		// Counting sort is unnecessary here; there are at most 120 samples.
		for a := 1; a < len(samples); a++ {
			v := samples[a]
			b := a - 1
			for b >= 0 && samples[b].lum > v.lum {
				samples[b+1] = samples[b]
				b--
			}
			samples[b+1] = v
		}
		lo := len(samples) * 68 / 100
		hi := len(samples) * 92 / 100
		if hi <= lo {
			lo, hi = 0, len(samples)
		}
		rv, gv, bv := make([]byte, 0, hi-lo), make([]byte, 0, hi-lo), make([]byte, 0, hi-lo)
		for _, q := range samples[lo:hi] {
			rv, gv, bv = append(rv, q.r), append(gv, q.g), append(bv, q.b)
		}
		r, g, b := mdMedianByte(rv), mdMedianByte(gv), mdMedianByte(bv)
		lum := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
		if lum > 1 {
			scale := 188.0 / lum
			if scale < 0.72 {
				scale = 0.72
			}
			if scale > 2.35 {
				scale = 2.35
			}
			clamp := func(v byte) byte {
				x := math.Round(float64(v) * scale)
				if x > 255 {
					x = 255
				}
				return byte(x)
			}
			r, g, b = clamp(r), clamp(g), clamp(b)
		}
		out[i] = mdCelestialRGB{r: r, g: g, b: b}
		valid[i] = true
	}
	// Fill cropped or occluded polar rows from the nearest usable latitude.
	last := -1
	for i := 0; i < bins; i++ {
		if valid[i] {
			last = i
		} else if last >= 0 {
			out[i] = out[last]
		}
	}
	last = -1
	for i := bins - 1; i >= 0; i-- {
		if valid[i] {
			last = i
		} else if last >= 0 {
			out[i] = out[last]
		}
	}
	return mdSmoothCelestialProfile(out, 3)
}

// mdBuildSaturnRingProfile samples the unobscured right-hand Cassini rings by
// radial distance.  Lighting asymmetry is discarded later and reapplied from
// the renderer's scene light, while real A/B/C-ring colour changes and the
// Cassini division remain in the radial profile.
func mdBuildSaturnRingProfile(tex mdEmbeddedTexture, bins int) []mdCelestialRGB {
	if !tex.ok || bins < 8 {
		return nil
	}
	const (
		cx       = 1077.0
		cy       = 458.0
		equatorR = 439.0
		tilt     = 0.245
	)
	out := make([]mdCelestialRGB, bins)
	valid := make([]bool, bins)
	for i := 0; i < bins; i++ {
		rho := 1.28 + (2.12-1.28)*float64(i)/float64(bins-1)
		rv, gv, bv := make([]byte, 0, 12), make([]byte, 0, 12), make([]byte, 0, 12)
		for _, deg := range [...]float64{22, 30, 38, 46, 54, 62} {
			theta := deg * math.Pi / 180
			x := int(math.Round(cx + rho*equatorR*math.Cos(theta)))
			y := int(math.Round(cy - rho*equatorR*tilt*math.Sin(theta)))
			r, g, b, a, ok := mdTextureSampleNearest(tex, x, y)
			if !ok || a < 180 {
				continue
			}
			lum := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
			if lum < 18 {
				continue
			}
			rv, gv, bv = append(rv, r), append(gv, g), append(bv, b)
		}
		if len(rv) == 0 {
			continue
		}
		out[i] = mdCelestialRGB{r: mdMedianByte(rv), g: mdMedianByte(gv), b: mdMedianByte(bv)}
		valid[i] = true
	}
	last := -1
	for i := range out {
		if valid[i] {
			last = i
		} else if last >= 0 {
			out[i] = out[last]
		}
	}
	last = -1
	for i := len(out) - 1; i >= 0; i-- {
		if valid[i] {
			last = i
		} else if last >= 0 {
			out[i] = out[last]
		}
	}
	return mdSmoothCelestialProfile(out, 1)
}

func mdSampleCelestialProfile(profile []mdCelestialRGB, t float64) (byte, byte, byte, bool) {
	if len(profile) < 2 {
		return 0, 0, 0, false
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	x := t * float64(len(profile)-1)
	i := int(math.Floor(x))
	if i >= len(profile)-1 {
		q := profile[len(profile)-1]
		return q.r, q.g, q.b, true
	}
	f := x - float64(i)
	a, b := profile[i], profile[i+1]
	return mdCelestialMix(a.r, b.r, f), mdCelestialMix(a.g, b.g, f), mdCelestialMix(a.b, b.b, f), true
}

func mdCelestialSaturnLatitudeColor(lat float64) (byte, byte, byte, bool) {
	mdLoadCelestialAssets()
	// The Cassini portrait is cropped near the poles and contains a small blue
	// optical artefact above the globe.  Hold the reconstructed polar caps at
	// the nearest reliable observed latitude instead of wrapping that artefact
	// around the full planet.
	const reliablePole = 1.22
	if lat > reliablePole {
		lat = reliablePole
	}
	if lat < -reliablePole {
		lat = -reliablePole
	}
	return mdSampleCelestialProfile(mdSaturnLatitudeProfile, (lat+math.Pi/2)/math.Pi)
}

func mdCelestialSaturnRingColor(rho float64) (byte, byte, byte, bool) {
	mdLoadCelestialAssets()
	return mdSampleCelestialProfile(mdSaturnRingProfile, (rho-1.28)/(2.12-1.28))
}
