package main

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// ColorRefs are Win32 COLORREF values (R at lowest byte). The order in this
// array IS the index order; do not sort by luminance or deduplicate colors.
type indexed16Palette [16]uint32

func indexedPaletteFromRefs(refs []uint32) indexed16Palette {
	var p indexed16Palette
	for i := 0; i < 16 && i < len(refs); i++ {
		p[i] = refs[i] & 0xFFFFFF
	}
	return p
}

func indexedColorRef(r, g, b byte) uint32 { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }

func decodeExternalIndexedPalette(name string, raw []byte) (indexed16Palette, error) {
	var out indexed16Palette
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".dat") {
		if len(raw) < 48 {
			return out, fmt.Errorf("DAT 팔레트에는 마지막 48바이트가 필요합니다")
		}
		raw = raw[len(raw)-48:]
		for i := 0; i < 48; i++ {
			if raw[i] > 63 {
				return out, fmt.Errorf("DAT 팔레트 6비트 범위 오류: 바이트 %d", i)
			}
		}
		for i := 0; i < 16; i++ {
			r, g, b := raw[i*3], raw[i*3+1], raw[i*3+2]
			out[i] = indexedColorRef((r<<2)|(r>>4), (g<<2)|(g>>4), (b<<2)|(b>>4))
		}
		return out, nil
	}
	if strings.HasSuffix(lower, ".dpal") {
		if len(raw) != 56 || !bytes.Equal(raw[:8], []byte{'D', 'P', '1', '6', 'R', 'G', 'B', 1}) {
			return out, fmt.Errorf("DPAL은 DP16RGB v1 + RGB 48바이트 형식이어야 합니다")
		}
		raw = raw[8:]
		for i := 0; i < 16; i++ {
			out[i] = indexedColorRef(raw[i*3], raw[i*3+1], raw[i*3+2])
		}
		return out, nil
	}
	if strings.HasSuffix(lower, ".rgb") {
		if len(raw) != 48 {
			return out, fmt.Errorf("RGB 팔레트는 48바이트여야 합니다")
		}
		for i := 0; i < 16; i++ {
			out[i] = indexedColorRef(raw[i*3], raw[i*3+1], raw[i*3+2])
		}
		return out, nil
	}
	if strings.HasSuffix(lower, ".act") {
		if len(raw) != 768 && len(raw) != 772 {
			return out, fmt.Errorf("ACT는 768 또는 772바이트여야 합니다")
		}
		for i := 0; i < 16; i++ {
			out[i] = indexedColorRef(raw[i*3], raw[i*3+1], raw[i*3+2])
		}
		return out, nil
	}
	if strings.HasSuffix(lower, ".pal") || strings.HasSuffix(lower, ".gpl") {
		lines := strings.Split(strings.ReplaceAll(string(raw), "\r", ""), "\n")
		n := 0
		if strings.HasSuffix(lower, ".pal") {
			if len(lines) < 3 || strings.TrimSpace(lines[0]) != "JASC-PAL" || strings.TrimSpace(lines[1]) != "0100" {
				return out, fmt.Errorf("JASC-PAL 0100 헤더가 없습니다")
			}
			count, err := strconv.Atoi(strings.TrimSpace(lines[2]))
			if err != nil || count != 16 {
				return out, fmt.Errorf("16색 PAL이 아닙니다")
			}
			lines = lines[3:]
		} else if len(lines) == 0 || !strings.HasPrefix(strings.TrimSpace(lines[0]), "GIMP Palette") {
			return out, fmt.Errorf("GIMP Palette 헤더가 없습니다")
		}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "Name:") || strings.HasPrefix(line, "Columns:") {
				continue
			}
			f := strings.Fields(line)
			if len(f) < 3 {
				continue
			}
			r, e1 := strconv.Atoi(f[0])
			g, e2 := strconv.Atoi(f[1])
			b, e3 := strconv.Atoi(f[2])
			if e1 != nil || e2 != nil || e3 != nil || r < 0 || r > 255 || g < 0 || g > 255 || b < 0 || b > 255 {
				return out, fmt.Errorf("잘못된 RGB 팔레트 줄: %q", line)
			}
			if n >= 16 {
				if strings.HasSuffix(lower, ".pal") {
					return out, fmt.Errorf("16색 초과")
				}
				break
			}
			out[n] = indexedColorRef(byte(r), byte(g), byte(b))
			n++
		}
		if n != 16 {
			return out, fmt.Errorf("팔레트 색상이 %d개입니다 (16개 필요)", n)
		}
		return out, nil
	}
	return out, fmt.Errorf("지원하지 않는 팔레트 형식입니다: %s", name)
}

func encodeIndexedPaletteDPAL(p indexed16Palette) []byte {
	out := make([]byte, 56)
	copy(out, []byte{'D', 'P', '1', '6', 'R', 'G', 'B', 1})
	for i, c := range p {
		out[8+i*3] = byte(c)
		out[8+i*3+1] = byte(c >> 8)
		out[8+i*3+2] = byte(c >> 16)
	}
	return out
}
func encodeIndexedPaletteJASC(p indexed16Palette) []byte {
	var b strings.Builder
	b.WriteString("JASC-PAL\r\n0100\r\n16\r\n")
	for _, c := range p {
		fmt.Fprintf(&b, "%d %d %d\r\n", byte(c), byte(c>>8), byte(c>>16))
	}
	return []byte(b.String())
}
