package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/png"
)

// buildOpaqueClipboardDIB converts dotPaint's top-down straight-alpha BGRA pixels
// into a bottom-up 32-bit CF_DIB payload composited on white.  Classic Windows
// applications often ignore the fourth byte of BI_RGB pixels; leaving RGB=0 in
// transparent areas therefore turns the entire formula preview black.  The
// opaque white fallback keeps HWP, classic Paint and similar programs correct.
func buildOpaqueClipboardDIB(w, h int, pix []byte) ([]byte, error) {
	if w <= 0 || h <= 0 || len(pix) != w*h*4 {
		return nil, errors.New("invalid clipboard image")
	}
	stride := w * 4
	data := make([]byte, 40+stride*h)
	binary.LittleEndian.PutUint32(data[0:4], 40)
	binary.LittleEndian.PutUint32(data[4:8], uint32(w))
	binary.LittleEndian.PutUint32(data[8:12], uint32(h)) // positive = bottom-up
	binary.LittleEndian.PutUint16(data[12:14], 1)
	binary.LittleEndian.PutUint16(data[14:16], 32)
	binary.LittleEndian.PutUint32(data[16:20], 0) // BI_RGB
	binary.LittleEndian.PutUint32(data[20:24], uint32(stride*h))

	for dy := 0; dy < h; dy++ {
		sy := h - 1 - dy
		for x := 0; x < w; x++ {
			si := (sy*w + x) * 4
			di := 40 + dy*stride + x*4
			a := int(pix[si+3])
			// Straight-alpha source composited over opaque white, rounded.
			data[di+0] = byte((int(pix[si+0])*a + 255*(255-a) + 127) / 255)
			data[di+1] = byte((int(pix[si+1])*a + 255*(255-a) + 127) / 255)
			data[di+2] = byte((int(pix[si+2])*a + 255*(255-a) + 127) / 255)
			data[di+3] = 255
		}
	}
	return data, nil
}

// buildTransparentClipboardPNG preserves the same formula preview as RGBA PNG.
// Programs that understand the registered PNG clipboard formats can retain the
// transparent background, while CF_DIB remains the broad compatibility fallback.
func buildTransparentClipboardPNG(w, h int, pix []byte) ([]byte, error) {
	if w <= 0 || h <= 0 || len(pix) != w*h*4 {
		return nil, errors.New("invalid clipboard image")
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := (y*w + x) * 4
			di := y*img.Stride + x*4
			img.Pix[di+0] = pix[si+2]
			img.Pix[di+1] = pix[si+1]
			img.Pix[di+2] = pix[si+0]
			img.Pix[di+3] = pix[si+3]
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
