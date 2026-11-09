// A small RIFF/VP8L-only WebP adapter for dotPaint's bundled lossless assets.
// VP8L implementation: derivative of golang.org/x/image/vp8l v0.30.0 (BSD-3-Clause).
// Not a general WebP reader: lossy VP8 and animated WebP are not supported.
package webplossless

import (
	"bytes"
	"dotpaint/internal/webplossless/vp8l"
	"encoding/binary"
	"errors"
	"image"
)

var ErrUnsupported = errors.New("webplossless: unsupported WebP encoding")

func Decode(data []byte) (image.Image, error) {
	if len(data) < 12 || !bytes.Equal(data[0:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WEBP")) {
		return nil, errors.New("webplossless: invalid RIFF/WEBP header")
	}
	limit := int(binary.LittleEndian.Uint32(data[4:8])) + 8
	if limit > len(data) || limit < 12 {
		return nil, errors.New("webplossless: truncated RIFF data")
	}
	for off := 12; off+8 <= limit; {
		n := int(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		begin := off + 8
		end := begin + n
		if n < 0 || end > limit || end < begin {
			return nil, errors.New("webplossless: invalid chunk size")
		}
		if bytes.Equal(data[off:off+4], []byte("VP8L")) {
			return vp8l.Decode(bytes.NewReader(data[begin:end]))
		}
		if bytes.Equal(data[off:off+4], []byte("VP8 ")) {
			return nil, ErrUnsupported
		}
		off = end + (n & 1)
	}
	return nil, ErrUnsupported
}
