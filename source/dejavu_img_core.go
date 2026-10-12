package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
)

const (
	dejavuIMGTileSize      = 24
	dejavuIMGTilePixels    = dejavuIMGTileSize * dejavuIMGTileSize
	dejavuIMGRecordSize    = 294
	dejavuIMGPixelOffset   = 4
	dejavuIMGPixelBytes    = 288
	dejavuIMGTrailerOffset = 292
	dejavuIMGTileCount     = 144
	dejavuIMGSheetColumns  = 12
	dejavuIMGSheetRows     = 12
	dejavuIMGSheetWidth    = dejavuIMGTileSize * dejavuIMGSheetColumns
	dejavuIMGSheetHeight   = dejavuIMGTileSize * dejavuIMGSheetRows
	dejavuIMGExpectedSize  = dejavuIMGRecordSize * dejavuIMGTileCount
	dejavuDATPaletteBytes  = 16 * 3
)

var dejavuIMGRecordHeader = [4]byte{0x17, 0x00, 0x17, 0x00}

type dejavuPalette struct {
	RGB [16][3]byte
}

func (p dejavuPalette) Color(i int) color.NRGBA {
	if i < 0 || i >= len(p.RGB) {
		i = 0
	}
	c := p.RGB[i]
	return color.NRGBA{R: c[0], G: c[1], B: c[2], A: 255}
}

func (p dejavuPalette) ColorRef(i int) uint32 {
	c := p.Color(i)
	return uint32(c.R) | uint32(c.G)<<8 | uint32(c.B)<<16
}

func (p dejavuPalette) Palette() color.Palette {
	out := make(color.Palette, 16)
	for i := 0; i < 16; i++ {
		out[i] = p.Color(i)
	}
	return out
}

func dejavu6To8(v byte) byte {
	v &= 0x3f
	return (v << 2) | (v >> 4)
}

func decodeDejavuPaletteDAT(data []byte) (dejavuPalette, error) {
	var p dejavuPalette
	if len(data) < dejavuDATPaletteBytes {
		return p, fmt.Errorf("DEJAVU.DAT가 너무 작습니다: %d바이트 (최소 %d바이트 필요)", len(data), dejavuDATPaletteBytes)
	}
	base := len(data) - dejavuDATPaletteBytes
	for i := 0; i < 16; i++ {
		for ch := 0; ch < 3; ch++ {
			raw := data[base+i*3+ch]
			if raw > 63 {
				return p, fmt.Errorf("DEJAVU.DAT 팔레트 값이 6비트 범위를 벗어났습니다: index=%d channel=%d value=%d", i, ch, raw)
			}
			p.RGB[i][ch] = dejavu6To8(raw)
		}
	}
	return p, nil
}

func validateDejavuIMG(data []byte) error {
	if len(data) != dejavuIMGExpectedSize {
		return fmt.Errorf("DejaVu IMG 크기가 올바르지 않습니다: %d바이트 (144타일 기준 %d바이트 필요)", len(data), dejavuIMGExpectedSize)
	}
	for i := 0; i < dejavuIMGTileCount; i++ {
		off := i * dejavuIMGRecordSize
		if !bytes.Equal(data[off:off+4], dejavuIMGRecordHeader[:]) {
			return fmt.Errorf("타일 %d 레코드 헤더가 17 00 17 00이 아닙니다", i)
		}
	}
	return nil
}

func decodeDejavuTileIndices(data []byte, tileID int) ([]byte, error) {
	if tileID < 0 || tileID >= dejavuIMGTileCount {
		return nil, fmt.Errorf("타일 번호 범위 오류: %d", tileID)
	}
	if len(data) < (tileID+1)*dejavuIMGRecordSize {
		return nil, errors.New("IMG 데이터가 잘렸습니다")
	}
	out := make([]byte, dejavuIMGTilePixels)
	record := data[tileID*dejavuIMGRecordSize : (tileID+1)*dejavuIMGRecordSize]
	for y := 0; y < dejavuIMGTileSize; y++ {
		for x := 0; x < dejavuIMGTileSize; x++ {
			var ci byte
			for plane := 0; plane < 4; plane++ {
				bo := dejavuIMGPixelOffset + y*12 + plane*3 + x/8
				bit := (record[bo] >> uint(7-(x%8))) & 1
				ci |= bit << uint(3-plane)
			}
			out[y*dejavuIMGTileSize+x] = ci
		}
	}
	return out, nil
}

func decodeDejavuAllIndices(data []byte) ([]byte, error) {
	if err := validateDejavuIMG(data); err != nil {
		return nil, err
	}
	out := make([]byte, dejavuIMGTileCount*dejavuIMGTilePixels)
	for tile := 0; tile < dejavuIMGTileCount; tile++ {
		idx, err := decodeDejavuTileIndices(data, tile)
		if err != nil {
			return nil, err
		}
		copy(out[tile*dejavuIMGTilePixels:], idx)
	}
	return out, nil
}

func encodeDejavuTileIntoRecord(dst []byte, indices []byte) error {
	if len(dst) < dejavuIMGRecordSize {
		return errors.New("IMG 레코드 버퍼가 너무 작습니다")
	}
	if len(indices) != dejavuIMGTilePixels {
		return fmt.Errorf("타일 인덱스 수가 올바르지 않습니다: %d", len(indices))
	}
	// Only the documented 288-byte pixel payload is rewritten. Header [0:4]
	// and the unknown trailer [292:294] are deliberately preserved.
	for i := dejavuIMGPixelOffset; i < dejavuIMGTrailerOffset; i++ {
		dst[i] = 0
	}
	for y := 0; y < dejavuIMGTileSize; y++ {
		for x := 0; x < dejavuIMGTileSize; x++ {
			ci := indices[y*dejavuIMGTileSize+x] & 0x0f
			for plane := 0; plane < 4; plane++ {
				bit := (ci >> uint(3-plane)) & 1
				if bit == 0 {
					continue
				}
				bo := dejavuIMGPixelOffset + y*12 + plane*3 + x/8
				dst[bo] |= 1 << uint(7-(x%8))
			}
		}
	}
	return nil
}

func encodeDejavuAllIndices(original []byte, allIndices []byte) ([]byte, error) {
	if err := validateDejavuIMG(original); err != nil {
		return nil, err
	}
	if len(allIndices) != dejavuIMGTileCount*dejavuIMGTilePixels {
		return nil, fmt.Errorf("전체 타일 인덱스 수가 올바르지 않습니다: %d", len(allIndices))
	}
	out := append([]byte(nil), original...)
	for tile := 0; tile < dejavuIMGTileCount; tile++ {
		rec := out[tile*dejavuIMGRecordSize : (tile+1)*dejavuIMGRecordSize]
		idx := allIndices[tile*dejavuIMGTilePixels : (tile+1)*dejavuIMGTilePixels]
		if err := encodeDejavuTileIntoRecord(rec, idx); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func dejavuIndicesToSheet(indices []byte, pal dejavuPalette) (*image.Paletted, error) {
	if len(indices) != dejavuIMGTileCount*dejavuIMGTilePixels {
		return nil, fmt.Errorf("전체 타일 인덱스 수가 올바르지 않습니다: %d", len(indices))
	}
	img := image.NewPaletted(image.Rect(0, 0, dejavuIMGSheetWidth, dejavuIMGSheetHeight), pal.Palette())
	for tile := 0; tile < dejavuIMGTileCount; tile++ {
		tx := (tile % dejavuIMGSheetColumns) * dejavuIMGTileSize
		ty := (tile / dejavuIMGSheetColumns) * dejavuIMGTileSize
		src := indices[tile*dejavuIMGTilePixels : (tile+1)*dejavuIMGTilePixels]
		for y := 0; y < dejavuIMGTileSize; y++ {
			for x := 0; x < dejavuIMGTileSize; x++ {
				img.SetColorIndex(tx+x, ty+y, src[y*dejavuIMGTileSize+x]&0x0f)
			}
		}
	}
	return img, nil
}

func dejavuSheetToTileIndices(sheet []byte) ([]byte, error) {
	if len(sheet) != dejavuIMGSheetWidth*dejavuIMGSheetHeight {
		return nil, fmt.Errorf("타일시트 인덱스 수가 올바르지 않습니다: %d", len(sheet))
	}
	out := make([]byte, dejavuIMGTileCount*dejavuIMGTilePixels)
	for tile := 0; tile < dejavuIMGTileCount; tile++ {
		tx := (tile % dejavuIMGSheetColumns) * dejavuIMGTileSize
		ty := (tile / dejavuIMGSheetColumns) * dejavuIMGTileSize
		dst := out[tile*dejavuIMGTilePixels : (tile+1)*dejavuIMGTilePixels]
		for y := 0; y < dejavuIMGTileSize; y++ {
			for x := 0; x < dejavuIMGTileSize; x++ {
				dst[y*dejavuIMGTileSize+x] = sheet[(ty+y)*dejavuIMGSheetWidth+(tx+x)] & 0x0f
			}
		}
	}
	return out, nil
}

func dejavuNearestPaletteIndex(r, g, b byte, pal dejavuPalette) byte {
	best := 0
	bestD := math.MaxInt
	for i := 0; i < 16; i++ {
		c := pal.RGB[i]
		dr := int(r) - int(c[0])
		dg := int(g) - int(c[1])
		db := int(b) - int(c[2])
		d := dr*dr + dg*dg + db*db
		if d < bestD {
			bestD = d
			best = i
		}
	}
	return byte(best)
}

// dejavuQuantizeBGRAForClipboard converts a straight-alpha, top-down BGRA
// clipboard image to the 16 colours of the currently loaded DejaVu palette.
// It returns a *new* pixel buffer so the original Windows/internal clipboard
// and pinned clipboard history remain untouched. The IMG format has no alpha:
// transparent pixels become the user-selected palette index, while partial
// alpha is composited over that same palette colour before quantization.
// Retaining the original width/height/mask preserves the floating-selection,
// move, Undo and clipping workflows of normal clipboard paste.
func dejavuQuantizeBGRAForClipboard(pix []byte, w, h int, pal dejavuPalette, transparentIndex byte) ([]byte, error) {
	if w < 1 || h < 1 || w > 16384 || h > 16384 || len(pix) != w*h*4 {
		return nil, fmt.Errorf("DejaVu clipboard image size mismatch: %dx%d, %d bytes", w, h, len(pix))
	}
	out := make([]byte, len(pix))
	bg := pal.RGB[int(transparentIndex&0x0f)]
	for i := 0; i < w*h; i++ {
		o := i * 4
		a := int(pix[o+3])
		var index byte
		if a == 0 {
			index = transparentIndex & 0x0f
		} else {
			r, g, b := pix[o+2], pix[o+1], pix[o]
			if a != 255 {
				r = byte((int(r)*a + int(bg[0])*(255-a) + 127) / 255)
				g = byte((int(g)*a + int(bg[1])*(255-a) + 127) / 255)
				b = byte((int(b)*a + int(bg[2])*(255-a) + 127) / 255)
			}
			index = dejavuNearestPaletteIndex(r, g, b, pal)
		}
		c := pal.RGB[index]
		out[o+0], out[o+1], out[o+2], out[o+3] = c[2], c[1], c[0], 255
	}
	return out, nil
}

// dejavuCompositeIndexedPastePixel paints one clipboard BGRA pixel over the
// existing opaque IMG canvas pixel. PNG alpha belongs to the *paste operation*,
// not the IMG format: alpha=0 leaves the original index exactly as-is; alpha=255
// maps to the closest of the sixteen palette colours; intermediate alpha blends
// with the actual destination colour before the nearest-palette conversion.
// Replaying this onto a floating selection's BasePix makes drag/move reversible.
// This helper mutates only dst; source clipboard pixels remain intact.
func dejavuCompositeIndexedPastePixel(dst, src []byte, pal dejavuPalette) {
	if len(dst) < 4 || len(src) < 4 {
		return
	}
	a := int(src[3])
	if a == 0 {
		return
	}
	r, g, b := src[2], src[1], src[0]
	if a < 255 {
		r = byte((int(r)*a + int(dst[2])*(255-a) + 127) / 255)
		g = byte((int(g)*a + int(dst[1])*(255-a) + 127) / 255)
		b = byte((int(b)*a + int(dst[0])*(255-a) + 127) / 255)
	}
	index := dejavuNearestPaletteIndex(r, g, b, pal)
	c := pal.RGB[index]
	dst[0], dst[1], dst[2], dst[3] = c[2], c[1], c[0], 255
}

func dejavuImageToIndices(src image.Image, w, h int, pal dejavuPalette, transparentIndex byte) []byte {
	out := make([]byte, w*h)
	b := src.Bounds()
	bg := pal.Color(int(transparentIndex & 0x0f))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rr, gg, bb, aa := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			a8 := byte(aa >> 8)
			if a8 == 0 {
				out[y*w+x] = transparentIndex & 0x0f
				continue
			}
			r8, g8, b8 := byte(rr>>8), byte(gg>>8), byte(bb>>8)
			if a8 != 255 {
				a := int(a8)
				r8 = byte((int(r8)*a + int(bg.R)*(255-a) + 127) / 255)
				g8 = byte((int(g8)*a + int(bg.G)*(255-a) + 127) / 255)
				b8 = byte((int(b8)*a + int(bg.B)*(255-a) + 127) / 255)
			}
			out[y*w+x] = dejavuNearestPaletteIndex(r8, g8, b8, pal)
		}
	}
	return out
}

func dejavuResizeNearest(src image.Image, w, h int) *image.NRGBA {
	if w <= 0 || h <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw <= 0 || sh <= 0 {
		return dst
	}
	for y := 0; y < h; y++ {
		sy := sb.Min.Y + min(sh-1, int(float64(y)*float64(sh)/float64(h)))
		for x := 0; x < w; x++ {
			sx := sb.Min.X + min(sw-1, int(float64(x)*float64(sw)/float64(w)))
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func dejavuCropTileFromSheet(src image.Image, tileID int) (*image.NRGBA, error) {
	if tileID < 0 || tileID >= dejavuIMGTileCount {
		return nil, fmt.Errorf("타일 번호 범위 오류: %d", tileID)
	}
	b := src.Bounds()
	if b.Dx() != dejavuIMGSheetWidth || b.Dy() != dejavuIMGSheetHeight {
		return nil, fmt.Errorf("타일시트 크기가 %d×%d가 아닙니다: %d×%d", dejavuIMGSheetWidth, dejavuIMGSheetHeight, b.Dx(), b.Dy())
	}
	tx := (tileID % dejavuIMGSheetColumns) * dejavuIMGTileSize
	ty := (tileID / dejavuIMGSheetColumns) * dejavuIMGTileSize
	out := image.NewNRGBA(image.Rect(0, 0, dejavuIMGTileSize, dejavuIMGTileSize))
	draw.Draw(out, out.Bounds(), src, image.Point{X: b.Min.X + tx, Y: b.Min.Y + ty}, draw.Src)
	return out, nil
}
