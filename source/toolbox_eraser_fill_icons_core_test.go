package main

import (
	"encoding/binary"
	"hash/crc32"
	"testing"
)

func toolIconStats(pixels *[userToolIconWidth * userToolIconHeight]uint32) (count, minX, minY, maxX, maxY int, crc uint32) {
	minX, minY = userToolIconWidth, userToolIconHeight
	maxX, maxY = -1, -1
	buf := make([]byte, len(pixels)*4)
	for i, color := range pixels {
		binary.LittleEndian.PutUint32(buf[i*4:], color)
		if color == userToolIconTransparent {
			continue
		}
		count++
		x, y := i%userToolIconWidth, i/userToolIconWidth
		if x < minX {
			minX = x
		}
		if y < minY {
			minY = y
		}
		if x > maxX {
			maxX = x
		}
		if y > maxY {
			maxY = y
		}
	}
	return count, minX, minY, maxX, maxY, crc32.ChecksumIEEE(buf)
}

func TestUserProvidedEraserAndFillIcons(t *testing.T) {
	tests := []struct {
		name               string
		pixels             *[userToolIconWidth * userToolIconHeight]uint32
		wantCount          int
		wantMinX, wantMinY int
		wantMaxX, wantMaxY int
		wantCRC            uint32
	}{
		{"eraser", &eraserToolIconPixels, 423, 4, 7, 35, 30, 0x81B59AE5},
		{"fill", &fillToolIconPixels, 518, 4, 2, 38, 34, 0xA244DC1E},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, minX, minY, maxX, maxY, crc := toolIconStats(tt.pixels)
			if count != tt.wantCount {
				t.Fatalf("non-transparent pixel count = %d, want %d", count, tt.wantCount)
			}
			if minX != tt.wantMinX || minY != tt.wantMinY || maxX != tt.wantMaxX || maxY != tt.wantMaxY {
				t.Fatalf("bounds = (%d,%d)-(%d,%d), want (%d,%d)-(%d,%d)", minX, minY, maxX, maxY, tt.wantMinX, tt.wantMinY, tt.wantMaxX, tt.wantMaxY)
			}
			if crc != tt.wantCRC {
				t.Fatalf("pixel CRC32 = %08X, want %08X", crc, tt.wantCRC)
			}
		})
	}
}
