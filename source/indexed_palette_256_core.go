package main

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Indexed editing supports 16- and 256-entry RGB lookup tables. The dedicated
// DejaVu IMG codec remains strictly 4-bit / 16 colors.
func validIndexedSize(n int) bool { return n == 16 || n == 256 }

func defaultIndexed256Palette(base []uint32) []uint32 {
	p := make([]uint32, 0, 256)
	for i := 0; i < 16; i++ {
		if i < len(base) {
			p = append(p, base[i]&0xffffff)
		} else {
			p = append(p, 0)
		}
	}
	for r := 0; r < 6; r++ {
		for g := 0; g < 6; g++ {
			for b := 0; b < 6; b++ {
				p = append(p, indexedColorRef(byte(r*51), byte(g*51), byte(b*51)))
			}
		}
	}
	for i := 0; i < 24; i++ {
		v := byte((i*255 + 11) / 23)
		p = append(p, indexedColorRef(v, v, v))
	}
	return p
}

func nearestIndexedColor(r, g, b byte, pal []uint32) int {
	best, bestDist := 0, int(^uint(0)>>1)
	for i, c := range pal {
		dr, dg, db := int(r)-int(byte(c)), int(g)-int(byte(c>>8)), int(b)-int(byte(c>>16))
		d := dr*dr + dg*dg + db*db
		if d < bestDist {
			bestDist, best = d, i
		}
	}
	return best
}

func compositeIndexedBGRA(dst, src []byte, pal []uint32) {
	if len(dst) < 4 || len(src) < 4 || len(pal) == 0 || src[3] == 0 {
		return
	}
	alpha := int(src[3])
	r, g, b := src[2], src[1], src[0]
	if alpha != 255 {
		r = byte((int(r)*alpha + int(dst[2])*(255-alpha) + 127) / 255)
		g = byte((int(g)*alpha + int(dst[1])*(255-alpha) + 127) / 255)
		b = byte((int(b)*alpha + int(dst[0])*(255-alpha) + 127) / 255)
	}
	c := pal[nearestIndexedColor(r, g, b, pal)]
	dst[0], dst[1], dst[2], dst[3] = byte(c>>16), byte(c>>8), byte(c), 255
}

// File formats are deliberately strict to avoid silently truncating a 256-color
// table to 16 entries. DAT is a distinct fixed-size 16-color VGA6 format.
func decodeIndexedPaletteFile(name string, raw []byte) ([]uint32, error) {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".dat") {
		p, err := decodeExternalIndexedPalette(name, raw)
		if err != nil {
			return nil, err
		}
		return append([]uint32(nil), p[:]...), nil
	}
	if strings.HasSuffix(lower, ".dpal") {
		if len(raw) == 56 && bytes.Equal(raw[:8], []byte{'D', 'P', '1', '6', 'R', 'G', 'B', 1}) {
			p, _ := decodeExternalIndexedPalette(name, raw)
			return append([]uint32(nil), p[:]...), nil
		}
		if len(raw) != 9+256*3 || !bytes.Equal(raw[:9], []byte{'D', 'P', '2', '5', '6', 'R', 'G', 'B', 1}) {
			return nil, fmt.Errorf("DPAL 형식 오류: DP16RGB 또는 DP256RGB 형식이어야 합니다")
		}
		raw = raw[9:]
	} else if strings.HasSuffix(lower, ".rgb") {
		if len(raw) != 16*3 && len(raw) != 256*3 {
			return nil, fmt.Errorf("RGB 팔레트는 48 또는 768바이트여야 합니다")
		}
	} else if strings.HasSuffix(lower, ".act") {
		if len(raw) != 768 && len(raw) != 772 {
			return nil, fmt.Errorf("ACT는 768 또는 772바이트여야 합니다")
		}
		if len(raw) == 772 {
			count := int(raw[768])<<8 | int(raw[769])
			if count != 0 {
				if !validIndexedSize(count) {
					return nil, fmt.Errorf("ACT 팔레트 색상 수 %d개: 16 또는 256개 필요", count)
				}
				raw = raw[:count*3]
			} else {
				raw = raw[:768]
			}
		} else {
			raw = raw[:768]
		}
	} else if strings.HasSuffix(lower, ".pal") || strings.HasSuffix(lower, ".gpl") {
		lines := strings.Split(strings.ReplaceAll(string(raw), "\r", ""), "\n")
		count := -1
		if strings.HasSuffix(lower, ".pal") {
			if len(lines) < 3 || strings.TrimSpace(strings.TrimPrefix(lines[0], "\ufeff")) != "JASC-PAL" || strings.TrimSpace(lines[1]) != "0100" {
				return nil, fmt.Errorf("JASC-PAL 0100 헤더가 없습니다")
			}
			n, e := strconv.Atoi(strings.TrimSpace(lines[2]))
			if e != nil || !validIndexedSize(n) {
				return nil, fmt.Errorf("PAL은 16색 또는 256색이어야 합니다")
			}
			count = n
			lines = lines[3:]
		} else if len(lines) == 0 || !strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(lines[0], "\ufeff")), "GIMP Palette") {
			return nil, fmt.Errorf("GIMP Palette 헤더가 없습니다")
		}
		colors := make([]uint32, 0, 256)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "Name:") || strings.HasPrefix(line, "Columns:") {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 3 {
				continue
			}
			r, e1 := strconv.Atoi(parts[0])
			g, e2 := strconv.Atoi(parts[1])
			b, e3 := strconv.Atoi(parts[2])
			if e1 != nil || e2 != nil || e3 != nil || r < 0 || g < 0 || b < 0 || r > 255 || g > 255 || b > 255 {
				return nil, fmt.Errorf("잘못된 RGB 팔레트 행: %q", line)
			}
			if len(colors) >= 256 {
				return nil, fmt.Errorf("팔레트는 최대 256색입니다")
			}
			colors = append(colors, indexedColorRef(byte(r), byte(g), byte(b)))
		}
		if !validIndexedSize(len(colors)) || (count != -1 && count != len(colors)) {
			return nil, fmt.Errorf("팔레트 색상 수 %d개: 16 또는 256개 필요", len(colors))
		}
		return colors, nil
	} else {
		return nil, fmt.Errorf("지원하지 않는 팔레트 형식: %s", name)
	}
	colors := make([]uint32, len(raw)/3)
	if !validIndexedSize(len(colors)) {
		return nil, fmt.Errorf("팔레트 수가 16 또는 256이 아닙니다")
	}
	for i := range colors {
		colors[i] = indexedColorRef(raw[i*3], raw[i*3+1], raw[i*3+2])
	}
	return colors, nil
}

func encodeIndexedPaletteDPALAny(p []uint32) ([]byte, error) {
	if !validIndexedSize(len(p)) {
		return nil, fmt.Errorf("16 또는 256색만 저장합니다")
	}
	if len(p) == 16 {
		var old indexed16Palette
		copy(old[:], p)
		return encodeIndexedPaletteDPAL(old), nil
	}
	buf := make([]byte, 9+256*3)
	copy(buf, []byte{'D', 'P', '2', '5', '6', 'R', 'G', 'B', 1})
	for i, c := range p {
		buf[9+i*3] = byte(c)
		buf[9+i*3+1] = byte(c >> 8)
		buf[9+i*3+2] = byte(c >> 16)
	}
	return buf, nil
}
func encodeIndexedPaletteJASCAny(p []uint32) ([]byte, error) {
	if !validIndexedSize(len(p)) {
		return nil, fmt.Errorf("16 또는 256색만 저장합니다")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "JASC-PAL\r\n0100\r\n%d\r\n", len(p))
	for _, c := range p {
		fmt.Fprintf(&b, "%d %d %d\r\n", byte(c), byte(c>>8), byte(c>>16))
	}
	return []byte(b.String()), nil
}
