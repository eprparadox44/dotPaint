package main

import "math"

const (
	mdSphereSkinPlain = iota
	mdSphereSkinEarth
	mdSphereSkinMoon
	mdSphereSkinSaturn
)

type mdCelestialRenderParams struct {
	width, height int
	radiusPixels  float64
	rotation      mdMat3
	skin          int
	maskPass      bool
}

type mdCelestialHit struct {
	t      float64
	point  mdVec3
	normal mdVec3
	worldY float64
	valid  bool
}

func mdCelestialClamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func mdCelestialMix(a, b byte, t float64) byte {
	t = mdCelestialClamp01(t)
	return byte(math.Round(float64(a)*(1-t) + float64(b)*t))
}

func mdCelestialBlendRGB(ar, ag, ab, br, bg, bb byte, t float64) (byte, byte, byte) {
	return mdCelestialMix(ar, br, t), mdCelestialMix(ag, bg, t), mdCelestialMix(ab, bb, t)
}

func mdCelestialWrapAngle(v float64) float64 {
	for v > math.Pi {
		v -= 2 * math.Pi
	}
	for v < -math.Pi {
		v += 2 * math.Pi
	}
	return v
}

func mdCelestialBlob(lon, lat, lon0, lat0, sx, sy float64) float64 {
	dx := mdCelestialWrapAngle(lon-lon0) / sx
	dy := (lat - lat0) / sy
	return math.Exp(-(dx*dx + dy*dy))
}

func mdCelestialNoise(lon, lat float64) float64 {
	v := math.Sin(lon*3.7+lat*5.1) + 0.55*math.Sin(lon*8.3-lat*4.9) + 0.28*math.Sin(lon*17.1+lat*13.7)
	return 0.5 + 0.5*v/1.83
}

func mdCelestialEarthColor(lon, lat float64) (byte, byte, byte) {
	if r, g, b, ok := mdCelestialEarthTextureColor(lon, lat); ok {
		return mdCelestialEarthPreserveSmallIslands(lon, lat, r, g, b)
	}
	land := 0.0
	// Deliberately simplified, deterministic continental silhouettes. They are
	// built into the renderer rather than generated or downloaded as images.
	land += 1.00 * mdCelestialBlob(lon, lat, -1.75, 0.72, 0.46, 0.35)  // N America
	land += 0.90 * mdCelestialBlob(lon, lat, -1.55, -0.28, 0.30, 0.48) // S America
	land += 1.15 * mdCelestialBlob(lon, lat, 0.45, 0.62, 0.82, 0.34)   // Eurasia
	land += 0.92 * mdCelestialBlob(lon, lat, 0.25, 0.02, 0.34, 0.52)   // Africa
	land += 0.75 * mdCelestialBlob(lon, lat, 2.25, -0.48, 0.34, 0.25)  // Australia
	land += 0.50 * mdCelestialBlob(lon, lat, -0.72, 1.18, 0.22, 0.18)  // Greenland
	land += 0.30 * math.Sin(5*lon+2*lat) * math.Sin(3*lat)
	land += 0.18 * (mdCelestialNoise(lon, lat) - 0.5)

	var r, g, b byte
	if lat < -1.18 || lat > 1.34 {
		r, g, b = 226, 239, 244
	} else if land > 0.70 {
		t := mdCelestialClamp01((land - 0.70) / 0.9)
		r, g, b = mdCelestialBlendRGB(55, 126, 58, 153, 126, 62, 0.35+0.35*t)
	} else {
		deep := mdCelestialClamp01(0.46 + 0.42*math.Cos(lat)*math.Cos(lat))
		r, g, b = mdCelestialBlendRGB(12, 49, 113, 20, 113, 181, deep)
	}
	cloud := 0.5 + 0.5*math.Sin(11*lon+5*lat+2.2*math.Sin(3*lon-lat))
	cloud *= 0.55 + 0.45*mdCelestialNoise(lon+0.7, lat-0.2)
	if cloud > 0.76 && math.Abs(lat) < 1.30 {
		r, g, b = mdCelestialBlendRGB(r, g, b, 244, 247, 248, mdCelestialClamp01((cloud-0.76)*2.3))
	}
	return r, g, b
}

// mdCelestialTextureLongitude converts a local sphere point into the
// cylindrical-map longitude. A211 keeps Korea near the default front longitude
// while restoring the ordinary exterior-view handedness: moving across the
// visible globe from screen-left to screen-right advances map longitude in the
// same direction as the editable equirectangular skin. CPU and D3D11 use this
// exact convention so switching backends cannot mirror Earth again.
func mdCelestialTextureLongitude(p mdVec3, skin int) float64 {
	base := math.Atan2(p.x, p.y)
	if skin == mdSphereSkinEarth {
		const earthFrontLongitude = 128.0 * math.Pi / 180.0
		return mdCelestialWrapAngle(earthFrontLongitude + base)
	}
	return mdCelestialWrapAngle(base)
}

// A whole Earth globe cannot represent kilometre-scale islands at literal
// scale: at the normal preview size Ulleungdo and Dokdo occupy far less than a
// pixel. A213 accidentally replaced the user's edited texels with fixed brown
// constants. A214 keeps the same cartographic minimum footprint but derives its
// colour from the actual loaded PNG, so user-painted pixels remain authoritative.
func mdCelestialEarthPreserveSmallIslands(lon, lat float64, r, g, b byte) (byte, byte, byte) {
	type island struct {
		lon, lat float64
		radius   float64
		detail   int
	}
	islands := [...]island{
		{130.8667 * math.Pi / 180, 37.5000 * math.Pi / 180, 1.10 * math.Pi / 180, 0}, // 울릉도
		{131.8697 * math.Pi / 180, 37.2411 * math.Pi / 180, 0.78 * math.Pi / 180, 1}, // 독도
	}
	bestT := 0.0
	bestRGB := [3]byte{}
	for _, item := range islands {
		d := mdCelestialAngularDistance(lon, lat, item.lon, item.lat)
		if d >= item.radius {
			continue
		}
		dr, dg, db, ok := mdCelestialEarthDetailColour(item.detail)
		if !ok {
			continue
		}
		t := 1 - d/item.radius
		// Smooth falloff avoids a square marker after supersampling.
		t = t * t * (3 - 2*t)
		if t > bestT {
			bestT = t
			bestRGB = [3]byte{dr, dg, db}
		}
	}
	if bestT > 0 {
		return mdCelestialBlendRGB(r, g, b, bestRGB[0], bestRGB[1], bestRGB[2], bestT)
	}
	return r, g, b
}

func mdCelestialAngularDistance(lon, lat, lon0, lat0 float64) float64 {
	s := math.Sin(lat)*math.Sin(lat0) + math.Cos(lat)*math.Cos(lat0)*math.Cos(mdCelestialWrapAngle(lon-lon0))
	if s < -1 {
		s = -1
	}
	if s > 1 {
		s = 1
	}
	return math.Acos(s)
}

func mdCelestialMoonColor(lon, lat float64) (byte, byte, byte) {
	if r, g, b, ok := mdCelestialMoonTextureColor(lon, lat); ok {
		return r, g, b
	}
	base := 0.54 + 0.18*(mdCelestialNoise(lon*1.2, lat*1.1)-0.5)
	maria := 0.0
	maria += 0.75 * mdCelestialBlob(lon, lat, 0.25, 0.30, 0.48, 0.32)
	maria += 0.58 * mdCelestialBlob(lon, lat, -0.70, 0.10, 0.34, 0.26)
	maria += 0.50 * mdCelestialBlob(lon, lat, 0.95, -0.20, 0.28, 0.22)
	base -= 0.18 * mdCelestialClamp01(maria)
	craters := [][3]float64{
		{-1.20, 0.55, 0.16}, {-0.35, -0.48, 0.12}, {0.55, 0.42, 0.18},
		{1.45, -0.10, 0.11}, {2.25, 0.70, 0.09}, {-2.15, -0.58, 0.14},
		{0.10, -0.02, 0.07}, {-0.85, 0.10, 0.06},
	}
	for _, c := range craters {
		d := mdCelestialAngularDistance(lon, lat, c[0], c[1])
		r := c[2]
		if d < r {
			base -= 0.18 * (1 - d/r)
		} else if d < r*1.28 {
			base += 0.11 * (1 - (d-r)/(r*0.28))
		}
	}
	base = mdCelestialClamp01(base)
	v := byte(math.Round(48 + 185*base))
	return v, byte(float64(v) * 0.98), byte(float64(v) * 0.93)
}

func mdCelestialSaturnColor(lon, lat float64) (byte, byte, byte) {
	if r, g, b, ok := mdCelestialSaturnGlobalColor(lon, lat); ok {
		// A205 uses an actual cylindrical texture rather than the A204
		// longitude-independent latitude strip.  The colour bands come from the
		// natural-colour Cassini portrait and the moving cloud/oval contrast comes
		// from the Cassini rotation movie, so axial rotation now changes the
		// visible face instead of leaving the old photograph apparently fixed.
		return r, g, b
	}
	if r, g, b, ok := mdCelestialSaturnLatitudeColor(lat); ok {
		return r, g, b
	}
	band := 0.5 + 0.24*math.Sin(28*lat) + 0.10*math.Sin(71*lat)
	band = mdCelestialClamp01(band)
	r, g, b := mdCelestialBlendRGB(164, 125, 82, 231, 204, 145, band)
	if math.Abs(lat) > 1.18 {
		r, g, b = mdCelestialBlendRGB(r, g, b, 190, 166, 116, 0.45)
	}
	return r, g, b
}

func mdCelestialShade(r, g, b byte, normalWorld mdVec3, skin int) (byte, byte, byte) {
	light := mdNormCore(mdVec3{-0.42, 0.78, 0.46})
	diffuse := math.Max(0, mdDotCore(normalWorld, light))
	ambient := 0.19
	if skin == mdSphereSkinEarth {
		ambient = 0.14
	}
	shade := ambient + (1-ambient)*diffuse
	// A mild camera-facing sheen helps the ocean and Saturn bands read as a
	// curved sphere without turning the procedural maps into plastic.
	view := mdVec3{0, 1, 0}
	halfv := mdNormCore(mdBHAdd(light, view))
	spec := math.Pow(math.Max(0, mdDotCore(normalWorld, halfv)), 24)
	if skin == mdSphereSkinMoon {
		spec = 0
	}
	apply := func(c byte) byte {
		v := float64(c)*shade + 26*spec
		if v > 255 {
			v = 255
		}
		return byte(math.Round(v))
	}
	return apply(r), apply(g), apply(b)
}

func mdCelestialSphereHit(screenX, screenZ float64, rotation mdMat3, polarScale float64) mdCelestialHit {
	cameraY := 4.0
	if polarScale <= 0 {
		polarScale = 1
	}
	oWorld := mdVec3{screenX, cameraY, screenZ}
	dWorld := mdVec3{0, -1, 0}
	o := mdMat3ApplyTranspose(rotation, oWorld)
	d := mdMat3ApplyTranspose(rotation, dWorld)
	invP2 := 1 / (polarScale * polarScale)
	a := d.x*d.x + d.y*d.y + d.z*d.z*invP2
	b := 2 * (o.x*d.x + o.y*d.y + o.z*d.z*invP2)
	c := o.x*o.x + o.y*o.y + o.z*o.z*invP2 - 1
	disc := b*b - 4*a*c
	if disc < 0 || math.Abs(a) < 1e-12 {
		return mdCelestialHit{}
	}
	t := (-b - math.Sqrt(disc)) / (2 * a)
	if t < 0 {
		return mdCelestialHit{}
	}
	p := mdBHAdd(o, mdBHMul(d, t))
	localNormal := mdNormCore(mdVec3{x: p.x, y: p.y, z: p.z * invP2})
	n := mdNormCore(mdMat3Apply(rotation, localNormal))
	return mdCelestialHit{t: t, point: p, normal: n, worldY: cameraY - t, valid: true}
}

func mdCelestialRayHitsEllipsoid(origin, dir mdVec3, polarScale float64) bool {
	if polarScale <= 0 {
		polarScale = 1
	}
	invP2 := 1 / (polarScale * polarScale)
	a := dir.x*dir.x + dir.y*dir.y + dir.z*dir.z*invP2
	b := 2 * (origin.x*dir.x + origin.y*dir.y + origin.z*dir.z*invP2)
	c := origin.x*origin.x + origin.y*origin.y + origin.z*origin.z*invP2 - 1
	disc := b*b - 4*a*c
	if disc <= 0 || math.Abs(a) < 1e-12 {
		return false
	}
	sqrtDisc := math.Sqrt(disc)
	t0 := (-b - sqrtDisc) / (2 * a)
	t1 := (-b + sqrtDisc) / (2 * a)
	return t0 > 1e-4 || t1 > 1e-4
}

func mdCelestialRingOpacity(rho float64) float64 {
	band := 0.72 + 0.16*math.Sin((rho-1.28)*58) + 0.08*math.Sin((rho-1.28)*143)
	if rho > 1.72 && rho < 1.80 {
		band *= 0.24
	}
	return mdCelestialClamp01(0.46 + 0.42*band)
}

func mdCelestialSaturnRingShadowFactor(point mdVec3, rotation mdMat3) float64 {
	// Test the ray from the globe surface toward the common scene light against
	// Saturn's equatorial ring plane.  This recreates the moving ring shadow
	// without baking the fixed day/night pattern from the source photograph.
	lightWorld := mdNormCore(mdVec3{-0.42, 0.78, 0.46})
	lightLocal := mdMat3ApplyTranspose(rotation, lightWorld)
	if math.Abs(lightLocal.z) < 1e-8 {
		return 1
	}
	t := -point.z / lightLocal.z
	if t <= 1e-4 {
		return 1
	}
	q := mdBHAdd(point, mdBHMul(lightLocal, t))
	rho := math.Hypot(q.x, q.y)
	if rho < 1.28 || rho > 2.12 {
		return 1
	}
	alpha := mdCelestialRingOpacity(rho)
	// The source photograph's hard night-side band made A204 look like the
	// original fixed cutout.  A205 keeps only a soft, pose-dependent ring shadow.
	return 1 - 0.34*alpha
}

func mdCelestialRingHit(screenX, screenZ float64, rotation mdMat3) (mdCelestialHit, float64) {
	cameraY := 4.0
	oWorld := mdVec3{screenX, cameraY, screenZ}
	dWorld := mdVec3{0, -1, 0}
	o := mdMat3ApplyTranspose(rotation, oWorld)
	d := mdMat3ApplyTranspose(rotation, dWorld)
	if math.Abs(d.z) < 1e-7 {
		return mdCelestialHit{}, 0
	}
	t := -o.z / d.z
	if t < 0 {
		return mdCelestialHit{}, 0
	}
	p := mdBHAdd(o, mdBHMul(d, t))
	rho := math.Hypot(p.x, p.y)
	if rho < 1.28 || rho > 2.12 {
		return mdCelestialHit{}, 0
	}
	alpha := mdCelestialRingOpacity(rho)
	return mdCelestialHit{t: t, point: p, worldY: cameraY - t, valid: true}, alpha
}

func mdCelestialRingColor(rho float64) (byte, byte, byte) {
	if r, g, b, ok := mdCelestialSaturnRingColor(rho); ok {
		return r, g, b
	}
	t := mdCelestialClamp01((rho - 1.28) / (2.12 - 1.28))
	r, g, b := mdCelestialBlendRGB(155, 128, 91, 231, 211, 170, 0.35+0.50*t)
	stripe := 0.5 + 0.5*math.Sin(86*rho)
	return mdCelestialBlendRGB(r, g, b, 116, 94, 72, 0.12*stripe)
}

// A205 deliberately removes the old fixed Cassini cutout renderer. Saturn is
// always rendered as a complete oblate globe plus a separate ring system.

// mdCelestialRender returns a white-background BGRA image. The caller already
// performs final supersampling/downscaling, so a binary high-resolution edge
// becomes antialiased in both preview and insertion.
func mdCelestialRender(params mdCelestialRenderParams) []byte {
	w, h := params.width, params.height
	if w < 1 || h < 1 || params.radiusPixels <= 0 {
		return nil
	}
	pix := make([]byte, w*h*4)
	for i := 0; i < w*h; i++ {
		pix[i*4+0], pix[i*4+1], pix[i*4+2], pix[i*4+3] = 255, 255, 255, 0
	}
	cx, cy := float64(w)*0.5, float64(h)*0.5
	radius := params.radiusPixels
	bound := radius
	if params.skin == mdSphereSkinSaturn {
		bound = radius * 2.14
	}
	minX := max(0, int(math.Floor(cx-bound-2)))
	maxX := min(w-1, int(math.Ceil(cx+bound+2)))
	minY := max(0, int(math.Floor(cy-bound-2)))
	maxY := min(h-1, int(math.Ceil(cy+bound+2)))
	for py := minY; py <= maxY; py++ {
		for px := minX; px <= maxX; px++ {
			sx := (float64(px) + 0.5 - cx) / radius
			sz := (cy - (float64(py) + 0.5)) / radius
			polarScale := 1.0
			if params.skin == mdSphereSkinSaturn {
				polarScale = 0.902
			}
			sphere := mdCelestialSphereHit(sx, sz, params.rotation, polarScale)
			ring, ringAlpha := mdCelestialHit{}, 0.0
			if params.skin == mdSphereSkinSaturn {
				ring, ringAlpha = mdCelestialRingHit(sx, sz, params.rotation)
			}
			if !sphere.valid && !ring.valid {
				continue
			}
			outR, outG, outB := byte(255), byte(255), byte(255)
			if sphere.valid {
				if params.maskPass {
					outR, outG, outB = 0, 0, 0
				} else {
					p := sphere.point
					if params.skin == mdSphereSkinSaturn {
						p.z /= polarScale
					}
					p = mdNormCore(p)
					lon := mdCelestialTextureLongitude(p, params.skin)
					lat := math.Asin(mdBHClamp(p.z, -1, 1))
					switch params.skin {
					case mdSphereSkinEarth:
						outR, outG, outB = mdCelestialEarthColor(lon, lat)
					case mdSphereSkinMoon:
						outR, outG, outB = mdCelestialMoonColor(lon, lat)
					case mdSphereSkinSaturn:
						outR, outG, outB = mdCelestialSaturnColor(lon, lat)
					default:
						outR, outG, outB = 192, 192, 192
					}
					outR, outG, outB = mdCelestialShade(outR, outG, outB, sphere.normal, params.skin)
					if params.skin == mdSphereSkinSaturn {
						shadow := mdCelestialSaturnRingShadowFactor(sphere.point, params.rotation)
						outR = byte(math.Round(float64(outR) * shadow))
						outG = byte(math.Round(float64(outG) * shadow))
						outB = byte(math.Round(float64(outB) * shadow))
					}
					// Darken only the final fraction of the limb, producing a crisp but
					// antialiased silhouette after downsampling.
					limb := mdCelestialClamp01((sphere.normal.y - 0.01) / 0.10)
					outR, outG, outB = mdCelestialBlendRGB(18, 18, 18, outR, outG, outB, limb)
				}
			}
			if ring.valid && (!sphere.valid || ring.t < sphere.t) {
				if params.maskPass {
					v := byte(math.Round(255 * (1 - ringAlpha)))
					outR, outG, outB = v, v, v
				} else {
					rho := math.Hypot(ring.point.x, ring.point.y)
					rr, rg, rb := mdCelestialRingColor(rho)
					// The Cassini source's day/night asymmetry is not baked into the
					// global ring.  Re-light the reconstructed annulus in the same
					// scene light as the globe instead.
					ringNormal := mdNormCore(mdMat3Apply(params.rotation, mdVec3{z: 1}))
					light := mdNormCore(mdVec3{-0.42, 0.78, 0.46})
					shade := 0.42 + 0.58*math.Abs(mdDotCore(ringNormal, light))
					lightLocal := mdMat3ApplyTranspose(params.rotation, light)
					if mdCelestialRayHitsEllipsoid(ring.point, lightLocal, polarScale) {
						shade *= 0.34
					}
					rr = byte(math.Round(math.Min(255, float64(rr)*shade)))
					rg = byte(math.Round(math.Min(255, float64(rg)*shade)))
					rb = byte(math.Round(math.Min(255, float64(rb)*shade)))
					outR, outG, outB = mdCelestialBlendRGB(outR, outG, outB, rr, rg, rb, ringAlpha)
				}
			}
			i := (py*w + px) * 4
			pix[i+0], pix[i+1], pix[i+2], pix[i+3] = outB, outG, outR, 0
		}
	}
	return pix
}
