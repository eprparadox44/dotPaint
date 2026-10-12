package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func mdTestPixelRGB(pix []byte, w, x, y int) (byte, byte, byte) {
	i := (y*w + x) * 4
	return pix[i+2], pix[i+1], pix[i+0]
}

func mdTestNonWhiteCount(pix []byte) int {
	count := 0
	for i := 0; i+3 < len(pix); i += 4 {
		if pix[i] != 255 || pix[i+1] != 255 || pix[i+2] != 255 {
			count++
		}
	}
	return count
}

func TestCelestialEarthAndMoonProceduralSkins(t *testing.T) {
	const w, h = 240, 200
	rot := mdEulerRotationMatrix(0.42, 0.63, 0.08)
	earth := mdCelestialRender(mdCelestialRenderParams{
		width: w, height: h, radiusPixels: 62, rotation: rot, skin: mdSphereSkinEarth,
	})
	moon := mdCelestialRender(mdCelestialRenderParams{
		width: w, height: h, radiusPixels: 62, rotation: rot, skin: mdSphereSkinMoon,
	})
	if len(earth) != w*h*4 || len(moon) != w*h*4 {
		t.Fatalf("unexpected render sizes: earth=%d moon=%d", len(earth), len(moon))
	}
	if mdTestNonWhiteCount(earth) < 6000 || mdTestNonWhiteCount(moon) < 6000 {
		t.Fatalf("sphere coverage too small: earth=%d moon=%d", mdTestNonWhiteCount(earth), mdTestNonWhiteCount(moon))
	}
	// Earth must contain visibly chromatic ocean/land pixels, while lunar pixels
	// remain close to neutral gray.
	chromaticEarth := 0
	neutralMoon := 0
	for i := 0; i+3 < len(earth); i += 4 {
		er, eg, eb := int(earth[i+2]), int(earth[i+1]), int(earth[i])
		if er < 248 || eg < 248 || eb < 248 {
			if max(er, max(eg, eb))-min(er, min(eg, eb)) > 24 {
				chromaticEarth++
			}
		}
		mr, mg, mb := int(moon[i+2]), int(moon[i+1]), int(moon[i])
		if mr < 248 || mg < 248 || mb < 248 {
			if max(mr, max(mg, mb))-min(mr, min(mg, mb)) <= 18 {
				neutralMoon++
			}
		}
	}
	if chromaticEarth < 1200 {
		t.Fatalf("earth skin lacks chromatic surface detail: %d", chromaticEarth)
	}
	if neutralMoon < 5000 {
		t.Fatalf("moon skin is not predominantly neutral: %d", neutralMoon)
	}
}

func TestCelestialSaturnRingExtendsBeyondGlobe(t *testing.T) {
	const w, h = 300, 240
	rot := mdEulerRotationMatrix(27*math.Pi/180, 0.22, 0)
	radius := 54.0
	pix := mdCelestialRender(mdCelestialRenderParams{
		width: w, height: h, radiusPixels: radius, rotation: rot, skin: mdSphereSkinSaturn,
	})
	cx, cy := w/2, h/2
	ringOutside := 0
	globe := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b := mdTestPixelRGB(pix, w, x, y)
			if r == 255 && g == 255 && b == 255 {
				continue
			}
			d := math.Hypot(float64(x-cx), float64(y-cy))
			if d > radius*1.08 {
				ringOutside++
			} else if d < radius*0.85 {
				globe++
			}
		}
	}
	if globe < 3500 {
		t.Fatalf("saturn globe missing: %d", globe)
	}
	if ringOutside < 900 {
		t.Fatalf("saturn ring does not extend beyond globe: %d", ringOutside)
	}
}

func TestCelestialMaskPassUsesWhiteKeyAndBlackGeometry(t *testing.T) {
	const w, h = 160, 140
	pix := mdCelestialRender(mdCelestialRenderParams{
		width: w, height: h, radiusPixels: 42,
		rotation: mdEulerRotationMatrix(0.4, 0.2, 0), skin: mdSphereSkinEarth, maskPass: true,
	})
	cr, cg, cb := mdTestPixelRGB(pix, w, w/2, h/2)
	if cr > 8 || cg > 8 || cb > 8 {
		t.Fatalf("mask centre should be black, got %d %d %d", cr, cg, cb)
	}
	br, bg, bb := mdTestPixelRGB(pix, w, 0, 0)
	if br != 255 || bg != 255 || bb != 255 {
		t.Fatalf("mask background should remain white, got %d %d %d", br, bg, bb)
	}
}

func TestBlackHoleCameraDistanceChangesProjectedScale(t *testing.T) {
	p := mdVec3{x: 2, y: 0, z: 0}
	near := mdBHRenderParams{cameraDistance: 22}
	far := mdBHRenderParams{cameraDistance: 88}
	nx, _, _, _ := mdBHSpatialProject3D(p, near, 400, 300, 10)
	fx, _, _, _ := mdBHSpatialProject3D(p, far, 400, 300, 10)
	centre := 200.0
	if math.Abs(nx-centre) <= math.Abs(fx-centre)*2.5 {
		t.Fatalf("near camera should enlarge the grid strongly: near=%f far=%f", nx-centre, fx-centre)
	}
}

func TestCelestialEmbeddedAssetsDecodeA214(t *testing.T) {
	mdLoadCelestialAssets()
	if !mdEarthTexture.ok || mdEarthTexture.w != 2048 || mdEarthTexture.h != 1024 {
		t.Fatalf("earth texture decode failed: ok=%v size=%dx%d", mdEarthTexture.ok, mdEarthTexture.w, mdEarthTexture.h)
	}
	if !mdMoonTexture.ok || mdMoonTexture.w != 2048 || mdMoonTexture.h != 1024 {
		t.Fatalf("moon texture decode failed: ok=%v size=%dx%d", mdMoonTexture.ok, mdMoonTexture.w, mdMoonTexture.h)
	}
	if !mdSaturnCassini.ok || mdSaturnCassini.w != 2048 || mdSaturnCassini.h != 821 {
		t.Fatalf("saturn Cassini portrait decode failed: ok=%v size=%dx%d", mdSaturnCassini.ok, mdSaturnCassini.w, mdSaturnCassini.h)
	}
	if !mdSaturnGlobal.ok || mdSaturnGlobal.w != 2048 || mdSaturnGlobal.h != 1024 {
		t.Fatalf("saturn Cassini global texture decode failed: ok=%v size=%dx%d", mdSaturnGlobal.ok, mdSaturnGlobal.w, mdSaturnGlobal.h)
	}
}

func mdTestPixelDifference(a, b []byte) int64 {
	if len(a) != len(b) {
		return 1 << 60
	}
	var total int64
	for i := 0; i+3 < len(a); i += 4 {
		for c := 0; c < 3; c++ {
			d := int(a[i+c]) - int(b[i+c])
			if d < 0 {
				d = -d
			}
			total += int64(d)
		}
	}
	return total
}

func TestCelestialSaturnGlobalSkinRotatesToDifferentLongitudes(t *testing.T) {
	const w, h = 420, 320
	const radius = 74.0
	front := mdCelestialRender(mdCelestialRenderParams{
		width: w, height: h, radiusPixels: radius,
		rotation: mdEulerRotationMatrix(0.48, 0.12, 0.05), skin: mdSphereSkinSaturn,
	})
	back := mdCelestialRender(mdCelestialRenderParams{
		width: w, height: h, radiusPixels: radius,
		rotation: mdEulerRotationMatrix(0.48, 0.12, math.Pi+0.05), skin: mdSphereSkinSaturn,
	})
	if len(front) != w*h*4 || len(back) != w*h*4 {
		t.Fatalf("unexpected global Saturn render sizes front=%d back=%d", len(front), len(back))
	}
	if mdTestNonWhiteCount(front) < 12000 || mdTestNonWhiteCount(back) < 12000 {
		t.Fatalf("rotatable Saturn coverage too small front=%d back=%d", mdTestNonWhiteCount(front), mdTestNonWhiteCount(back))
	}
	for _, pix := range [][]byte{front, back} {
		r, g, b := mdTestPixelRGB(pix, w, w/2, h/2)
		if r > 248 && g > 248 && b > 248 {
			t.Fatalf("rotated Saturn centre became empty: %d %d %d", r, g, b)
		}
	}
	if diff := mdTestPixelDifference(front, back); diff < 170000 {
		t.Fatalf("Saturn axial rotation did not expose a visibly different Cassini longitude: diff=%d", diff)
	}
}

func TestCelestialSaturnGlobalTextureContainsLongitudeDetail(t *testing.T) {
	latitudes := []float64{-0.85, -0.45, -0.10, 0.22}
	different := 0
	absInt := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	for _, lat := range latitudes {
		r0, g0, b0 := mdCelestialSaturnColor(-2.4, lat)
		r1, g1, b1 := mdCelestialSaturnColor(1.7, lat)
		delta := absInt(int(r0)-int(r1)) + absInt(int(g0)-int(g1)) + absInt(int(b0)-int(b1))
		if delta >= 5 {
			different++
		}
		if int(r0)+int(g0)+int(b0) < 120 || int(r1)+int(g1)+int(b1) < 120 {
			t.Fatalf("Cassini global texture unexpectedly dark at lat=%f: %v vs %v", lat, []byte{r0, g0, b0}, []byte{r1, g1, b1})
		}
	}
	if different < 2 {
		t.Fatalf("Saturn global texture still behaves like A204's longitude-independent strip: differing latitudes=%d", different)
	}
}

func TestCelestialSaturnUsesOblateGlobe(t *testing.T) {
	rot := mdMat3Identity()
	if !mdCelestialSphereHit(0.95, 0, rot, 0.902).valid {
		t.Fatal("Saturn equatorial edge should intersect the oblate globe")
	}
	if mdCelestialSphereHit(0, 0.95, rot, 0.902).valid {
		t.Fatal("Saturn polar extent should be smaller than its equatorial radius")
	}
}

func TestCelestialEarthLongitudeMatchesEditableMapHandedness(t *testing.T) {
	centre := mdCelestialTextureLongitude(mdVec3{0, 1, 0}, mdSphereSkinEarth)
	left := mdCelestialTextureLongitude(mdNormCore(mdVec3{-0.35, 1, 0}), mdSphereSkinEarth)
	right := mdCelestialTextureLongitude(mdNormCore(mdVec3{0.35, 1, 0}), mdSphereSkinEarth)
	if !(left < centre && centre < right) {
		t.Fatalf("A211 Earth orientation must match the editable map left-to-right order: left=%f centre=%f right=%f", left, centre, right)
	}
	want := 128.0 * math.Pi / 180.0
	if math.Abs(mdCelestialWrapAngle(centre-want)) > 1e-9 {
		t.Fatalf("earth default front longitude mismatch: got=%f want=%f", centre, want)
	}
	moonCentre := mdCelestialTextureLongitude(mdVec3{0, 1, 0}, mdSphereSkinMoon)
	moonLeft := mdCelestialTextureLongitude(mdNormCore(mdVec3{-0.35, 1, 0}), mdSphereSkinMoon)
	moonRight := mdCelestialTextureLongitude(mdNormCore(mdVec3{0.35, 1, 0}), mdSphereSkinMoon)
	if !(moonLeft < moonCentre && moonCentre < moonRight) {
		t.Fatalf("Moon longitude convention should remain unchanged: left=%f centre=%f right=%f", moonLeft, moonCentre, moonRight)
	}
}

func TestCelestialEarthPreservesUserPNGDetailColoursA214(t *testing.T) {
	tests := []struct {
		name     string
		lon, lat float64
		index    int
	}{
		{"Ulleungdo", 130.8667, 37.5000, 0},
		{"Dokdo", 131.8697, 37.2411, 1},
	}
	for _, tc := range tests {
		lon := tc.lon * math.Pi / 180
		lat := tc.lat * math.Pi / 180
		wantR, wantG, wantB, ok := mdCelestialEarthDetailColour(tc.index)
		if !ok {
			t.Fatalf("%s PNG detail sample missing", tc.name)
		}
		r, g, b := mdCelestialEarthColor(lon, lat)
		if r != wantR || g != wantG || b != wantB {
			t.Fatalf("%s centre must use the actual PNG detail colour: got=%d,%d,%d want=%d,%d,%d", tc.name, r, g, b, wantR, wantG, wantB)
		}
	}
}

func TestExternalEarthSkinPNGPriorityAndValidation(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 128, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 128; x++ {
			img.SetRGBA(x, y, color.RGBA{R: byte(x), G: byte(y * 3), B: 91, A: 255})
		}
	}
	path := filepath.Join(dir, "earth.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	tex, loadedPath, err := mdLoadExternalEarthTextureFromDir(dir)
	if err != nil {
		t.Fatalf("external earth skin load failed: %v", err)
	}
	if !tex.ok || tex.w != 128 || tex.h != 64 || loadedPath != path {
		t.Fatalf("unexpected external skin result: ok=%v size=%dx%d path=%q", tex.ok, tex.w, tex.h, loadedPath)
	}
}

func TestExternalEarthSkinIgnoresLegacyJPEGNamesA213(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"earth.jpg", "earth.jpeg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("legacy-earth-skin"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tex, loadedPath, err := mdLoadExternalEarthTextureFromDir(dir)
	if err != nil {
		t.Fatalf("legacy JPEG names must be ignored, not decoded: %v", err)
	}
	if tex.ok || loadedPath != "" {
		t.Fatalf("legacy JPEG unexpectedly overrode embedded Earth: ok=%v path=%q", tex.ok, loadedPath)
	}
}

func TestExternalEarthSkinRejectsNonEquirectangularRatio(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 96, 96))
	path := filepath.Join(dir, "earth.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()
	_, loadedPath, err := mdLoadExternalEarthTextureFromDir(dir)
	if err == nil || loadedPath != path {
		t.Fatalf("invalid aspect ratio should be rejected: path=%q err=%v", loadedPath, err)
	}
}
