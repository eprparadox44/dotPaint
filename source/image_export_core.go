package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

func (c *Canvas) toRGBA() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, c.W, c.H))
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			si := (y*c.W + x) * 4
			di := img.PixOffset(x, y)
			img.Pix[di] = c.Pix[si+2]
			img.Pix[di+1] = c.Pix[si+1]
			img.Pix[di+2] = c.Pix[si]
			img.Pix[di+3] = c.Pix[si+3]
		}
	}
	return img
}

func (c *Canvas) toOpaqueNRGBA() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, c.W, c.H))
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			si := (y*c.W + x) * 4
			di := img.PixOffset(x, y)
			a := int(c.Pix[si+3])
			img.Pix[di+0] = byte((int(c.Pix[si+2])*a + 255*(255-a) + 127) / 255)
			img.Pix[di+1] = byte((int(c.Pix[si+1])*a + 255*(255-a) + 127) / 255)
			img.Pix[di+2] = byte((int(c.Pix[si+0])*a + 255*(255-a) + 127) / 255)
			img.Pix[di+3] = 255
		}
	}
	return img
}
func encodeImageFile(path string, c *Canvas) error {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		path += ".png"
		ext = ".png"
	}
	if ext == ".bmp" {
		return encodeBMP(path, c)
	}
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" {
		return fmt.Errorf("지원하지 않는 저장 형식: %s", ext)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	switch ext {
	case ".jpg", ".jpeg":
		return jpeg.Encode(f, c.toOpaqueNRGBA(), &jpeg.Options{Quality: 92})
	case ".gif":
		// GIF is an indexed, single-frame format. Always write genuine GIF
		// bytes; never silently put PNG data into a .gif filename.
		return encodeStillGIF(f, c.toRGBA())
	case ".png":
		return png.Encode(f, c.toRGBA())
	default:
		return fmt.Errorf("지원하지 않는 저장 형식: %s (PNG, BMP, JPEG, GIF 사용)", ext)
	}
}

func encodeBMP(path string, c *Canvas) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	row := ((c.W*3 + 3) / 4) * 4
	dataSize := row * c.H
	// BITMAPFILEHEADER
	binary.Write(f, binary.LittleEndian, uint16(0x4d42))
	binary.Write(f, binary.LittleEndian, uint32(14+40+dataSize))
	binary.Write(f, binary.LittleEndian, uint16(0))
	binary.Write(f, binary.LittleEndian, uint16(0))
	binary.Write(f, binary.LittleEndian, uint32(54))
	binary.Write(f, binary.LittleEndian, uint32(40))
	binary.Write(f, binary.LittleEndian, int32(c.W))
	binary.Write(f, binary.LittleEndian, int32(c.H))
	binary.Write(f, binary.LittleEndian, uint16(1))
	binary.Write(f, binary.LittleEndian, uint16(24))
	binary.Write(f, binary.LittleEndian, uint32(0))
	binary.Write(f, binary.LittleEndian, uint32(dataSize))
	binary.Write(f, binary.LittleEndian, int32(2835))
	binary.Write(f, binary.LittleEndian, int32(2835))
	binary.Write(f, binary.LittleEndian, uint32(0))
	binary.Write(f, binary.LittleEndian, uint32(0))
	buf := make([]byte, row)
	for y := c.H - 1; y >= 0; y-- {
		for i := range buf {
			buf[i] = 0
		}
		for x := 0; x < c.W; x++ {
			si := (y*c.W + x) * 4
			di := x * 3
			a := int(c.Pix[si+3])
			buf[di] = byte((int(c.Pix[si])*a + 255*(255-a) + 127) / 255)
			buf[di+1] = byte((int(c.Pix[si+1])*a + 255*(255-a) + 127) / 255)
			buf[di+2] = byte((int(c.Pix[si+2])*a + 255*(255-a) + 127) / 255)
		}
		if _, err = f.Write(buf); err != nil {
			return err
		}
	}
	return nil
}
