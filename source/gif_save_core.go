package main

import (
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"io"
)

// GIF is a 256-colour, one-bit transparency format. Quantize an opaque
// copy and reserve index 0 for fully transparent pixels so loss of alpha
// cannot silently change the transparent background into an opaque colour.
// Partial alpha is composited over white, consistent with JPEG export.
func encodeStillGIF(w io.Writer, img image.Image) error {
	bounds := img.Bounds()
	colors := make(color.Palette, 0, 256)
	colors = append(colors, color.NRGBA{A: 0})
	colors = append(colors, palette.Plan9[:255]...)
	opaque := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			// RGBA() returns premultiplied 16-bit values, hence white + (r-a).
			opaque.SetNRGBA(x, y, color.NRGBA{
				R: uint8((r + 65535 - a + 128) / 257),
				G: uint8((g + 65535 - a + 128) / 257),
				B: uint8((b + 65535 - a + 128) / 257),
				A: 255,
			})
		}
	}
	out := image.NewPaletted(bounds, colors)
	draw.FloydSteinberg.Draw(out, bounds, opaque, bounds.Min)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a == 0 {
				out.SetColorIndex(x, y, 0)
			}
		}
	}
	return gif.Encode(w, out, nil)
}
